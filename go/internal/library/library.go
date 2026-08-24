package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// LocalOwner is the owner segment for a laptop with auth off: one person,
// they own everything they can see, and the URL still has to say so.
const LocalOwner = "local"

// Library is every deck a caller may reach, in
// both tiers. Built per request from the scope; holds no connection of its
// own -- the app.db handle is the door's read-only one, or nil on a laptop
// that has none.
type Library struct {
	scope      auth.Scope
	decksDir   string
	appDB      *sql.DB
	appWriteDB *sql.DB // nil when nothing may write app.db here
	maintainer string  // the maintainer's handle, or "" when there is none
}

// Resolver is what a Library needs from the process: the decks root, the
// read-only app.db (nil when there is none), and the maintainer's handle.
type Resolver struct {
	DecksDir string
	AppDB    *sql.DB
	// AppWriteDB is the read-write app.db handle the SQL tier's writes use,
	// or nil. Separate from AppDB because the read handle is opened
	// `mode=ro` and a write through it would fail at the driver rather than
	// at the gate that is supposed to answer.
	AppWriteDB *sql.DB
	// Maintainer resolves `MTGLAB_ADMIN_EMAIL` to a username through app.db
	// (`MaintainerUsername`), or "" when unconfigured or
	// unknown. Called only when the caller is authenticated: with auth off
	// the file owner is `local` regardless, and asking would open a
	// database a laptop must not acquire as a side effect of listing decks.
	Maintainer func(ctx context.Context) (string, error)
}

// For builds the caller's Library.
func (r Resolver) For(ctx context.Context, scope auth.Scope) (*Library, error) {
	lib := &Library{scope: scope, decksDir: r.DecksDir, appDB: r.AppDB,
		appWriteDB: r.AppWriteDB}
	if scope.Authenticated && r.Maintainer != nil {
		m, err := r.Maintainer(ctx)
		if err != nil {
			return nil, err
		}
		lib.maintainer = m
	}
	return lib, nil
}

// FileOwner is `_file_owner`: the owner segment the curated six sit under --
// the maintainer when one is configured, `local` otherwise and always with
// auth off (the bug that was made a rule: a laptop's `.env` names a
// maintainer and `mtglab ui` runs with auth off, and the shelf showed every
// deck twice).
func (l *Library) FileOwner() string {
	if !l.scope.Authenticated {
		return LocalOwner
	}
	if l.maintainer != "" {
		return l.maintainer
	}
	return LocalOwner
}

// MyOwner is `my_owner`: the segment the caller's own decks live under.
func (l *Library) MyOwner() string {
	if !l.scope.Authenticated || l.scope.Username == "" {
		return LocalOwner
	}
	return l.scope.Username
}

// Actor is who to attribute an edit to (ADR 28): the username, or "" for
// whoever is at this machine.
func (l *Library) Actor() string { return l.scope.Username }

func (l *Library) isMe(owner string) bool {
	return l.scope.Username != "" && strings.EqualFold(owner, l.scope.Username)
}

func (l *Library) isMaintainer(owner string) bool {
	return l.maintainer != "" && strings.EqualFold(owner, l.maintainer)
}

// SourceFor is `source_for`: the decks of `owner`, as this caller may see
// them. ErrNotFound for an account that does not exist -- **not a distinct
// error**, deliberately: one answer for "no such person" and "nothing of
// theirs for you", or the owner segment would enumerate the account list
// (ADR 5).
func (l *Library) SourceFor(ctx context.Context, owner string) (Source, error) {
	// Auth off: one person, one library, filed under `local`. Nothing else
	// exists to be asked about, and asking would reach for app.db.
	if !l.scope.Authenticated {
		if strings.EqualFold(owner, LocalOwner) {
			return NewFileSource(l.decksDir, true), nil
		}
		return nil, ErrNotFound{Slug: owner}
	}
	mine := l.isMe(owner)
	// The file tier, under whichever segment owns it here: writable by its
	// owner, or by an admin when no maintainer is configured and the six
	// are therefore nobody's in particular.
	if strings.EqualFold(owner, l.FileOwner()) {
		if mine || (l.maintainer == "" && l.scope.IsAdmin) {
			return NewFileSource(l.decksDir, true), nil
		}
		return NewSharedOnly(NewFileSource(l.decksDir, false)), nil
	}
	// The SQL tier: somebody's own decks.
	ownerID, err := l.ownerID(ctx, owner)
	if err != nil {
		return nil, err
	}
	if ownerID == nil {
		return nil, ErrNotFound{Slug: owner}
	}
	if mine {
		return NewSQLSource(l.appDB, l.appWriteDB, *ownerID, true, false), nil
	}
	return NewSQLSource(l.appDB, nil, *ownerID, false, true), nil
}

// ownerID is `_owner_id`: `users.get(owner)`, case-insensitively (the column
// is COLLATE NOCASE), or nil.
func (l *Library) ownerID(ctx context.Context, owner string) (*int64, error) {
	if l.appDB == nil {
		return nil, nil
	}
	var id int64
	err := l.appDB.QueryRowContext(ctx, "SELECT id FROM users WHERE username = ?", strings.TrimSpace(owner)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("owner lookup: %w", err)
	}
	return &id, nil
}

// Mine is `mine`: the caller's own decks, whichever tier they are in.
func (l *Library) Mine() (Source, error) {
	if !l.scope.Authenticated {
		return NewFileSource(l.decksDir, true), nil
	}
	if l.scope.Username != "" && l.isMaintainer(l.scope.Username) {
		return NewFileSource(l.decksDir, true), nil
	}
	if l.scope.UserID == 0 {
		return nil, ErrNotFound{Slug: "no account"}
	}
	return NewSQLSource(l.appDB, l.appWriteDB, l.scope.UserID, true, false), nil
}

// Owned is one library the caller may read, with the segment it is shown
// under.
type Owned struct {
	Owner  string
	Source Source
}

// Visible is `visible`: every library this caller may read, owner-first --
// the caller's own, then the maintainer's showcase when it is not already
// theirs, then everybody else's shared decks alphabetically. **Nothing below
// the showcase runs with auth off**: there are no accounts to share with,
// and the query would open app.db.
func (l *Library) Visible(ctx context.Context) ([]Owned, error) {
	out := []Owned{}
	seen := map[string]bool{}
	add := func(owner string, src Source) {
		key := strings.ToLower(owner)
		if !seen[key] {
			seen[key] = true
			out = append(out, Owned{Owner: owner, Source: src})
		}
	}
	mine, err := l.Mine()
	if err != nil {
		return nil, err
	}
	add(l.MyOwner(), mine)
	showcase, err := l.SourceFor(ctx, l.FileOwner())
	if err != nil {
		return nil, err
	}
	add(l.FileOwner(), showcase)
	if !l.scope.Authenticated {
		return out, nil
	}
	shared, err := SharedDecks(ctx, l.appDB)
	if err != nil {
		return nil, err
	}
	for _, s := range shared {
		if seen[strings.ToLower(s.Username)] {
			continue
		}
		src, err := l.SourceFor(ctx, s.Username)
		if err != nil {
			var missing ErrNotFound
			if errors.As(err, &missing) {
				continue
			}
			return nil, err
		}
		add(s.Username, src)
	}
	return out, nil
}

// Shared is one shared deck in the SQL tier, as `shared_decks` lists it.
type Shared struct {
	Username string
	Slug     string
	Name     string
}

// SharedDecks is `sqlsource.shared_decks`: every shared deck in the SQL tier,
// ordered by owner so the grouping the browse tab renders is the order it
// arrives in. Only ever assembles what is already shared.
func SharedDecks(ctx context.Context, db *sql.DB) ([]Shared, error) {
	if db == nil {
		return []Shared{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT u.username, d.slug, d.name FROM user_decks d
		JOIN users u ON u.id = d.owner_id
		WHERE d.shared = 1 AND d.deleted_at IS NULL
		ORDER BY u.username COLLATE NOCASE, d.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Shared{}
	for rows.Next() {
		var s Shared
		if err := rows.Scan(&s.Username, &s.Slug, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// MaintainerUsername is the
// maintainer's *handle*, resolved through the address `MTGLAB_ADMIN_EMAIL`
// names (ADR 17) -- and never the address itself, which goes into no URL.
// "" when unconfigured, malformed, or not yet an account.
func MaintainerUsername(ctx context.Context, db *sql.DB, adminEmail string) (string, error) {
	address := normaliseEmail(adminEmail)
	if address == "" || db == nil {
		return "", nil
	}
	var username string
	err := db.QueryRowContext(ctx, "SELECT username FROM users WHERE email = ?", address).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("maintainer lookup: %w", err)
	}
	return username, nil
}

// normaliseEmail is `auth.NormaliseEmail`'s rule without the error: trimmed,
// lowered, and shape-checked (`local@domain.tld`, no whitespace, no second
// `@`); anything else is "" -- which `MaintainerUsername` answers as an
// unconfigured maintainer rather than a refusal.
func normaliseEmail(email string) string {
	candidate := strings.ToLower(strings.TrimSpace(email))
	if candidate == "" || len(candidate) > 254 {
		return ""
	}
	local, domain, found := strings.Cut(candidate, "@")
	if !found || local == "" || domain == "" || strings.ContainsAny(candidate, " \t\r\n") ||
		strings.Contains(domain, "@") || !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return ""
	}
	return candidate
}
