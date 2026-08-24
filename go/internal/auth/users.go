package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Accounts, and the single path that verifies one -- `mtglab/auth/users.py`.
//
// `Authenticate` is the only function in this module that turns a password
// into an identity, and everything the login checklist asks for lives inside
// it rather than in the route that calls it: one generic answer for every
// failure, a hash computed even when the account does not exist, and
// `disabled` checked *after* the verification so that "no such user", "wrong
// password" and "account disabled" cost the same and say the same thing.
// Worth stating plainly, because the temptation runs the other way: a helpful
// login form that says "that account is disabled" is a login form that
// confirms the account exists to anybody who asks.
//
// Email is a column here and nowhere else. ADR 16 makes it the first personal
// data this project stores, and CLAUDE.md rule 5 is unconditional: **it must
// never reach a log line, an artifact, or a Claude tool result.** `User`
// carries it and `AsDict` withholds it unless asked, which is opt-in rather
// than opt-out because the rule is easy to state and hard to remember.
//
// `SetAdmin`, `SetDisabled` and `Delete` are the three that refuse to remove
// the last admin **who can sign in**, and each does it inside a `BEGIN
// IMMEDIATE` transaction rather than in the handler that called it. ADR 17:
// the maintainer is the only admin this deployment has by design, so each of
// those calls is one keystroke away from a lockout only a shell on the
// machine could undo.

// UsernamePattern is deliberately narrow. A username is a login handle, not a
// display name -- it ends up in URLs, log lines and `mtglab users list`
// output, and the set of characters unambiguous in all three is small.
// Case-insensitive at the database (COLLATE NOCASE), so `Aaron` and `aaron`
// are one account.
var UsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,31}$`)

// MaxEmail is RFC 5321 §4.5.3.1.3's cap on a whole address, so this rejects
// nothing that was ever deliverable. Checked first because NormaliseEmail sits
// on the unauthenticated claim path -- the one place a stranger chooses the
// input.
const MaxEmail = 254

// The refusals, each named because a route answers a different status for it.
var (
	// ErrUserExists is raised rather than silently taking over an account.
	ErrUserExists = errors.New("user exists")
	// ErrNoSuchUser is raised when an operation names an account that is not
	// there.
	ErrNoSuchUser = errors.New("no such user")
	// ErrInvalidUsername is raised when a username could not be a username.
	ErrInvalidUsername = errors.New("invalid username")
	// ErrInvalidEmail is raised when an address could not be an address.
	ErrInvalidEmail = errors.New("invalid email")
	// ErrLastAdmin is raised rather than leaving an instance with nobody who
	// can administer it. ADR 17: `SetAdmin(false)`, `SetDisabled(true)` and
	// `Delete` are each one call from a lockout whose only remedy is a shell
	// on the machine, and this lives here rather than in a handler so the CLI,
	// the admin routes and anything written later inherit the refusal instead
	// of each remembering it.
	ErrLastAdmin = errors.New("last admin")
	// ErrUnknownTier is raised rather than writing a tier key nothing
	// resolves. The read path deliberately tolerates an unknown key -- a stale
	// value resolves to the default, which is what makes a rolled-back deploy
	// survivable -- and the write path must not inherit that tolerance.
	ErrUnknownTier = errors.New("unknown tier")
)

// LooksLikeEmail is the shape test `^[^@\s]+@[^@\s]+\.[^@\s]+$` used to make.
//
// Not an RFC 5322 validator and not trying to be: the only check that means
// anything about deliverability is sending a message, which is what the invite
// flow does, and everything short of that is a shape test whose false
// negatives are somebody's perfectly real address.
//
// Taken apart rather than left as a pattern, exactly as Python takes it apart:
// both `[^@\s]+` runs also match dots, so every dot in the domain is a
// candidate separator and an address that cannot match makes a backtracking
// engine try all of them. Go's RE2 has no backtracking, so the quadratic is
// not this runtime's problem -- but the two halves must agree case for case,
// and the way to guarantee that is to run the same decision procedure.
func LooksLikeEmail(candidate string) bool {
	local, domain, found := strings.Cut(candidate, "@")
	if !found || local == "" || strings.Contains(domain, "@") {
		return false
	}
	// A dot with something on either side of it, expressed as a slice because
	// the first character and the last are exactly the two positions where a
	// dot leaves one of the pattern's two runs with nothing to match.
	if len(domain) < 2 || !strings.Contains(domain[1:len(domain)-1], ".") {
		return false
	}
	return !strings.ContainsFunc(candidate, unicode.IsSpace)
}

// NormaliseUsername trims and validates. It does not change case.
func NormaliseUsername(username string) (string, error) {
	candidate := strings.TrimSpace(username)
	if !UsernamePattern.MatchString(candidate) {
		return "", failf("%w: %s is not a usable username -- 2 to 32 characters, "+
			"letters, digits, dot, dash or underscore, starting with a letter "+
			"or digit", ErrInvalidUsername, wire.Quote(username))
	}
	return candidate, nil
}

// NormaliseEmail trims, lowercases and shape-checks. An empty string means
// absent, which is `None` in Python and not an error.
func NormaliseEmail(email string) (string, error) {
	if strings.TrimSpace(email) == "" {
		return "", nil
	}
	candidate := strings.ToLower(strings.TrimSpace(email))
	if len(candidate) > MaxEmail || !LooksLikeEmail(candidate) {
		// The message quotes a bounded slice rather than the input: an
		// oversized address is exactly the case that reaches here, and echoing
		// it whole would put the caller's own megabyte back into an error
		// string, a log line and an API response. Sliced by *bytes*, as
		// Python slices `str` by characters -- the cap is a guard against
		// size, and either reading of it holds the sentence to a line.
		clipped := email
		if len(clipped) > MaxEmail {
			clipped = clipped[:MaxEmail]
		}
		return "", failf("%w: %s does not look like an email address",
			ErrInvalidEmail, wire.Quote(clipped))
	}
	return candidate, nil
}

// User is an account, without its password hash.
//
// The hash is not a field on purpose. This record is returned from the API,
// logged and printed; a hash never loaded into it cannot leak out of it.
// `Authenticate` reads the column directly and keeps it local.
type User struct {
	ID       int64
	Username string
	// Email is empty when the account has none. Never serialised unless
	// AsDict is asked (ADR 17: an address may be serialised only into a
	// response an admin authenticated for).
	Email string
	// HasEmail separates "no address" from "the empty string", which the
	// column cannot hold but a zero value can.
	HasEmail bool
	IsAdmin  bool
	Disabled bool
	// CreatedAt is the stored ISO 8601 string, echoed rather than reformatted.
	// Python serialises `datetime.fromisoformat(col).isoformat()`, which is
	// the identity on everything `datetime.now(UTC).isoformat()` writes -- so
	// the column *is* the answer, and re-deriving it would only introduce a
	// way to disagree about trailing zeros.
	CreatedAt string
	// ModelTier is which Claude answers this account (`internal/tiers`).
	// Empty is the ordinary case and means the house default -- stored as NULL
	// rather than as the default's key, so changing which tier is the default
	// never means rewriting every row that never asked for anything.
	//
	// Unlike Email this is **not** withheld from AsDict: a tier is a fact
	// about the instance's spending, not about a person, and the account it
	// belongs to already knows which Claude answered it.
	ModelTier string
}

// AsDict is the account, serialised. **Without the address unless asked.**
//
// Two callers pass true, and the pair is the whole of the rule as ADR 17
// restates it: an address may be serialised only into a response an admin
// authenticated for. `mtglab users list` prints to the maintainer's own
// terminal, and the admin routes answer a caller the middleware has already
// established is an admin. A third caller needs the argument made again;
// `tests/test_isolation.py` is where it is pinned.
func (u *User) AsDict(includeEmail bool) map[string]any {
	body := map[string]any{
		"id":       u.ID,
		"username": u.Username,
		"is_admin": u.IsAdmin,
		"disabled": u.Disabled,
		// The stored string; see CreatedAt.
		"created_at": u.CreatedAt,
		// The *key*, resolved through the roster so a row holding a tier this
		// build no longer knows about reports the tier it will actually be
		// answered by rather than the string in the column. A screen showing
		// `opus` for an account served Sonnet would be a lie of exactly the
		// kind ADR 18's cached-number rule forbids.
		"model_tier": tiers.Get(u.ModelTier).Key,
	}
	if includeEmail {
		if u.HasEmail {
			body["email"] = u.Email
		} else {
			body["email"] = nil
		}
	}
	return body
}

// userColumns is what every read selects. Python uses `SELECT *` and probes
// the cursor for `model_tier`, so that a row assembled from a pre-v10 schema
// still builds a User. Here the columns are named, which is the same tolerance
// arrived at from the other side: the ladder runs at boot, so a missing column
// is a deployment fault to fail loudly on rather than one to paper over.
const userColumns = "id, username, email, is_admin, disabled_at, created_at, model_tier"

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var u User
	var email, disabledAt, tier sql.NullString
	var isAdmin int64
	if err := row.Scan(&u.ID, &u.Username, &email, &isAdmin, &disabledAt,
		&u.CreatedAt, &tier); err != nil {
		return nil, err
	}
	u.Email, u.HasEmail = email.String, email.Valid
	u.IsAdmin = isAdmin != 0
	u.Disabled = disabledAt.Valid && disabledAt.String != ""
	u.ModelTier = tier.String
	return &u, nil
}

// Create adds an account.
//
// An empty password creates an account that **cannot log in yet** -- which is
// what ADR 16's invite issues, and why the column is nullable.
func Create(ctx context.Context, db *sql.DB, username, email string, isAdmin bool) (*User, error) {
	name, err := NormaliseUsername(username)
	if err != nil {
		return nil, err
	}
	address, err := NormaliseEmail(email)
	if err != nil {
		return nil, err
	}
	var stored any
	if address != "" {
		stored = address
	}
	admin := 0
	if isAdmin {
		admin = 1
	}
	res, err := db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash, email, is_admin, created_at)"+
			" VALUES (?, NULL, ?, ?, ?)", name, stored, admin, nowISO())
	if err != nil {
		// Both unique columns land here. Say which, because "that is taken"
		// for an address the caller cannot see is an unhelpful place to stop.
		if isUniqueViolation(err) {
			which := "username"
			if strings.Contains(err.Error(), "email") {
				which = "email address"
			}
			return nil, failf("%w: that %s is already registered", ErrUserExists, which)
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	fetched, err := GetByID(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if fetched == nil { // unreachable in practice
		return nil, failf("%w: %s", ErrNoSuchUser, name)
	}
	return fetched, nil
}

// isUniqueViolation reports whether err is SQLite's UNIQUE constraint. Matched
// on the message because modernc's driver reports a wrapped error rather than
// a typed constraint code, and the two spellings SQLite uses ("UNIQUE
// constraint failed", "constraint failed") both carry the column.
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") ||
		strings.Contains(err.Error(), "constraint failed")
}

// Get is one account by name, case-insensitively (COLLATE NOCASE on the
// column). Nil when there is no such account.
func Get(ctx context.Context, db *sql.DB, username string) (*User, error) {
	return oneUser(ctx, db, "SELECT "+userColumns+" FROM users WHERE username = ?",
		strings.TrimSpace(username))
}

// GetByID reads one account, or nil when there is none.
func GetByID(ctx context.Context, db *sql.DB, id int64) (*User, error) {
	return oneUser(ctx, db, "SELECT "+userColumns+" FROM users WHERE id = ?", id)
}

// GetByEmail is one account by address, the lookup the reset flow needs.
//
// A nil from here must produce the same response as a hit (ADR 16), so this
// returning nil is not permission to tell the caller anything.
func GetByEmail(ctx context.Context, db *sql.DB, email string) (*User, error) {
	address, err := NormaliseEmail(email)
	if err != nil {
		return nil, err
	}
	if address == "" {
		return nil, nil
	}
	return oneUser(ctx, db, "SELECT "+userColumns+" FROM users WHERE email = ?", address)
}

func oneUser(ctx context.Context, db *sql.DB, query string, args ...any) (*User, error) {
	u, err := scanUser(db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user lookup: %w", err)
	}
	return u, nil
}

// AllUsers is every account, by name.
func AllUsers(ctx context.Context, db *sql.DB) ([]*User, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM users ORDER BY username COLLATE NOCASE")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return out, nil
}

// usableAdmins is an admin who could actually log in right now: the flag, plus
// a password to log in with, plus not disabled. All three, and the two beyond
// the flag are what make this worth a named query.
//
// An instance whose only remaining admin is an unclaimed invite is locked out
// exactly as thoroughly as one with no admin at all -- nobody can sign in and
// nobody can promote anybody. A guard that counted that row would report
// success while the lockout happened, which is the failure it exists to
// prevent. ADR 17.
const usableAdmins = "SELECT id FROM users WHERE is_admin = 1" +
	" AND password_hash IS NOT NULL AND disabled_at IS NULL"

// UsableAdminIDs is every account that is an admin *and* able to sign in as
// one.
//
// Exported because "how many admins does this instance have" is a question the
// admin surface asks too -- a page that greys out the last demote button is
// friendlier than one whose button returns 409 -- and a second spelling of the
// predicate is a second thing to keep in step.
func UsableAdminIDs(ctx context.Context, q querier) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, usableAdmins)
	if err != nil {
		return nil, fmt.Errorf("usable admins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("usable admins: %w", err)
		}
		ids[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usable admins: %w", err)
	}
	return ids, nil
}

// querier is whatever can run a query: the pool, a pinned connection, or a
// transaction. `refuseIfLastAdmin` runs inside the caller's own transaction,
// so it cannot take the pool.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// refuseIfLastAdmin returns ErrLastAdmin if this operation would take the last
// one away. Called from inside the caller's transaction, after BEGIN
// IMMEDIATE, so the count cannot change between the check and the write.
//
// An operation on an account that is not currently a usable admin changes the
// count by nothing and is always allowed -- including when the count is
// already zero, where refusing would mean an instance with no admin could
// never disable anybody.
func refuseIfLastAdmin(ctx context.Context, q querier, userID int64, doing string) error {
	admins, err := UsableAdminIDs(ctx, q)
	if err != nil {
		return err
	}
	if admins[userID] && len(admins) == 1 {
		return failf("%w: refusing to %s: this is the only admin who can sign in. "+
			"Promote somebody else first.", ErrLastAdmin, doing)
	}
	return nil
}

// HasPassword reports whether this account can log in at all.
func HasPassword(ctx context.Context, db *sql.DB, userID int64) (bool, error) {
	var hash sql.NullString
	err := db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?",
		userID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, failf("%w: %d", ErrNoSuchUser, userID)
	}
	if err != nil {
		return false, fmt.Errorf("has password: %w", err)
	}
	return hash.Valid, nil
}

// SetPassword sets a password and revokes every session for that account,
// returning the number of sessions ended.
//
// One transaction, so there is no window in which the password has changed and
// the old sessions are still live -- the failure ADR 16 points at when it says
// a reset that leaves the attacker logged in has answered the wrong question.
func SetPassword(ctx context.Context, db *sql.DB, userID int64, password string) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	var revoked int64
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?",
			hash, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return failf("%w: %d", ErrNoSuchUser, userID)
		}
		gone, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
		if err != nil {
			return err
		}
		revoked, _ = gone.RowsAffected()
		return nil
	})
	return revoked, err
}

// applyUsername renames an account **inside a transaction the caller holds.**
//
// `tokens.Redeem` is the reason it exists: a name and a password chosen on the
// same form have to land in the same transaction as the token being spent, or
// a rejected name leaves a spent link behind -- an invite that cannot be
// retried.
//
// Returns ErrUserExists if the name is taken, which COLLATE NOCASE on the
// column makes case-insensitive -- so `Ada` collides with `ada`, while *this*
// account changing its own capitalisation does not collide with itself.
func applyUsername(ctx context.Context, tx *sql.Tx, userID int64, username string) (string, error) {
	name, err := NormaliseUsername(username)
	if err != nil {
		return "", err
	}
	res, err := tx.ExecContext(ctx, "UPDATE users SET username = ? WHERE id = ?", name, userID)
	if err != nil {
		if isUniqueViolation(err) {
			return "", failf("%w: that username is already taken", ErrUserExists)
		}
		return "", fmt.Errorf("rename user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", failf("%w: %d", ErrNoSuchUser, userID)
	}
	return name, nil
}

// SetUsername renames an account in a transaction of its own.
//
// **Sessions are deliberately left alone.** A username is an identifier, not a
// credential: nothing is authenticated by it once a session exists, since
// `sessions` keys on `user_id`. That is the opposite of SetPassword, which
// revokes everything, and the difference is worth stating rather than
// inferring -- changing your handle should not sign you out of your other
// browser.
func SetUsername(ctx context.Context, db *sql.DB, userID int64, username string) (string, error) {
	var name string
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		name, err = applyUsername(ctx, tx, userID, username)
		return err
	})
	return name, err
}

// SetDisabled disables or re-enables an account, returning sessions ended.
//
// Disabling revokes sessions too: an account that can no longer log in but
// whose existing cookie still works has not been disabled in any sense the
// person doing it meant. Returns ErrLastAdmin rather than disabling the only
// admin who can sign in.
func SetDisabled(ctx context.Context, db *sql.DB, userID int64, disabled bool) (int64, error) {
	var stamp any
	if disabled {
		stamp = nowISO()
	}
	var revoked int64
	err := exclusive(ctx, db, func(c *sql.Conn) error {
		if disabled {
			if err := refuseIfLastAdmin(ctx, c, userID, "disable that account"); err != nil {
				return err
			}
		}
		res, err := c.ExecContext(ctx, "UPDATE users SET disabled_at = ? WHERE id = ?",
			stamp, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return failf("%w: %d", ErrNoSuchUser, userID)
		}
		if !disabled {
			return nil
		}
		gone, err := c.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
		if err != nil {
			return err
		}
		revoked, _ = gone.RowsAffected()
		return nil
	})
	return revoked, err
}

// SetAdmin grants or revokes admin. Returns ErrLastAdmin rather than revoking
// the last.
//
// Sessions are deliberately *not* revoked either way. Admin is read fresh from
// the account on every request (`auth.Resolve`), so a demotion takes effect on
// the demoted party's very next call without signing them out of an app they
// are still entitled to use as themselves.
func SetAdmin(ctx context.Context, db *sql.DB, userID int64, isAdmin bool) error {
	flag := 0
	if isAdmin {
		flag = 1
	}
	return exclusive(ctx, db, func(c *sql.Conn) error {
		if !isAdmin {
			if err := refuseIfLastAdmin(ctx, c, userID, "revoke admin"); err != nil {
				return err
			}
		}
		res, err := c.ExecContext(ctx, "UPDATE users SET is_admin = ? WHERE id = ?",
			flag, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return failf("%w: %d", ErrNoSuchUser, userID)
		}
		return nil
	})
}

// SetModelTier chooses which Claude answers this account. An empty key
// restores the default.
//
// No LastAdmin-style guard, because there is nothing to lock anybody out of:
// every tier can hold a conversation, and the worst outcome of the worst value
// here is a cheaper answer. Sessions are not revoked for the same reason
// SetAdmin does not revoke them -- the tier is read fresh per call.
//
// Stored as NULL rather than as the default's key when the tier is absent or
// already the default: one representation for "nobody has chosen anything", so
// changing which tier is the default never means rewriting rows.
func SetModelTier(ctx context.Context, db *sql.DB, userID int64, tier string) error {
	if tier != "" && !tiers.Known(tier) {
		return failf("%w: %s", ErrUnknownTier, tier)
	}
	var stored any
	if tier != "" && tier != tiers.DefaultKey {
		stored = tier
	}
	return exclusive(ctx, db, func(c *sql.Conn) error {
		res, err := c.ExecContext(ctx, "UPDATE users SET model_tier = ? WHERE id = ?",
			stored, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return failf("%w: %d", ErrNoSuchUser, userID)
		}
		return nil
	})
}

// Delete removes an account outright, returning the number of sessions ended.
//
// The irreversible sibling of SetDisabled, and wanted for the case disabling
// cannot serve: `username` and `email` are UNIQUE COLLATE NOCASE, so a
// disabled account still holds both and an address cannot be re-invited while
// a row is sitting on it. Disabling revokes; this releases.
//
// **Sessions and tokens go with it, and not by a DELETE written here.** Both
// tables declare ON DELETE CASCADE, and the read-write DSN turns
// `foreign_keys` on, which is what makes those clauses more than a comment.
// The count is taken first only so the caller has something true to print.
//
// Returns ErrLastAdmin rather than deleting the only admin who can sign in --
// the same guard the other two use, for the reason ADR 17 gives: the rule
// belongs in the core so the CLI and the admin route cannot disagree about it.
// Deleting is the third door to a lockout and the only one with nothing to
// undo it.
func Delete(ctx context.Context, db *sql.DB, userID int64) (int64, error) {
	var revoked int64
	err := exclusive(ctx, db, func(c *sql.Conn) error {
		if err := refuseIfLastAdmin(ctx, c, userID, "delete that account"); err != nil {
			return err
		}
		if err := c.QueryRowContext(ctx,
			"SELECT count(*) FROM sessions WHERE user_id = ?", userID).Scan(&revoked); err != nil {
			return err
		}
		res, err := c.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return failf("%w: %d", ErrNoSuchUser, userID)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return revoked, nil
}

// Authenticate is the one place a password becomes an identity. A nil user
// means no.
//
// Every refusal costs one Argon2 verification, including the ones that could
// be answered without doing any work at all -- an unknown username, and an
// account created by an invite that has not been claimed. That is the point: a
// login that returns faster for a name nobody has registered has told the
// person asking that nobody has registered it.
//
// It takes the read-write handle because of the last step: the one moment an
// old hash can be upgraded for free is when the plaintext is in hand and has
// just been proven correct.
func Authenticate(ctx context.Context, db *sql.DB, username, password string) (*User, error) {
	var id int64
	var stored, disabledAt sql.NullString
	err := db.QueryRowContext(ctx,
		"SELECT id, password_hash, disabled_at FROM users WHERE username = ?",
		strings.TrimSpace(username)).Scan(&id, &stored, &disabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		VerifyDummy(password)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	var hash *string
	if stored.Valid {
		hash = &stored.String
	}
	if !Verify(hash, password) {
		return nil, nil
	}

	// Checked *after* the hash, not before, so a disabled account is refused
	// in the same time and with the same message as a wrong password.
	if disabledAt.Valid && disabledAt.String != "" {
		return nil, nil
	}

	if hash != nil && NeedsRehash(*hash) {
		fresh, err := HashPassword(password)
		if err == nil {
			if _, err := db.ExecContext(ctx,
				"UPDATE users SET password_hash = ? WHERE id = ?", fresh, id); err != nil {
				return nil, fmt.Errorf("rehash: %w", err)
			}
		}
	}
	return GetByID(ctx, db, id)
}
