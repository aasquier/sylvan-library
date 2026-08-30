package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The crypt: the decks a library has entombed, and the way back out of it.
//
// ADR 27 named the shape at the card level -- *entomb is the delete, and the
// graveyard is the undo* -- and gave the 99 both halves. The deck level only
// ever had the first. `Delete` moved the whole directory somewhere safe and
// returned where it went, and *where it went* was the entire way back: the
// answer was rendered to the player as a filesystem path and an instruction
// to open a shell. That is commandment 10 twice over, and for anybody who has
// never opened a terminal it is not a way back at all -- it is a receipt for
// a deck they cannot reach.
//
// This is the second half. A Source that can bury a deck can also say what it
// has buried and raise one of them again, and **the identifier that travels
// between those two answers is opaque on purpose**: the file tier's handle is
// a directory name and the SQL tier's is a row id, and neither is a fact
// about anybody's deck. `cryptID` hashes the tier's own handle, so what
// crosses the wire means nothing anywhere else -- there is nothing in it to
// leak. It is also why `Restore` takes an id rather than a slug: a deck can
// be entombed, remade and entombed again, and "the one you just buried" has
// to name exactly one of them.
//
// **Only an owner has a crypt.** Both tiers refuse a source they may not
// write, rather than answering an empty list: an entombed deck was never
// shared with anybody, and "you have nothing buried" is a different sentence
// from "these are not yours to see".

// Entombed is one deck in the crypt, as a shelf needs to show it: enough to
// recognise which deck this is, and the handle that raises it.
type Entombed struct {
	// ID is the handle Restore takes. Stable while the entry stays in the
	// crypt, and meaningless outside it.
	ID string
	// Slug is the name it will come back under. A restore never renames --
	// which is why one is refused while a living deck holds the slug.
	Slug string
	Name string
	// At is when it was entombed, or the zero time when nothing recorded it.
	// **Zero is not a date and must not be rendered as one**: the file tier
	// reads this off the entry's own name, and an entry somebody put there by
	// hand has no stamp to read. The route layer sends null for it.
	At        time.Time
	Cards     int
	Commander []string
}

// Crypt is the optional half of a Writer: what this source has entombed, and
// the way back. Optional in the same sense Writer itself is optional on a
// Source -- asked for through `CryptFor`, so a caller that only reads cannot
// hold one, and a tier with no crypt refuses in the same words a read-only
// tier does rather than growing two methods that exist only to say no.
type Crypt interface {
	// Entombed is everything in the crypt, newest first. ErrReadOnly when
	// the crypt is not the caller's.
	Entombed(ctx context.Context) ([]Entombed, error)
	// Restore raises one entry and says which slug it came back under.
	// ErrNotFound for an id this crypt does not hold, ErrExists when a
	// living deck already holds the slug, ErrReadOnly when the crypt is not
	// the caller's.
	Restore(ctx context.Context, id string) (string, error)
	// Empty destroys every entry in the crypt and says how many went, which
	// is the one operation here with no way back. ErrReadOnly when the crypt
	// is not the caller's.
	//
	// **It destroys what `Entombed` would have listed, entry by entry, and
	// never the crypt itself.** A crypt is a place rather than a container
	// somebody swaps out: removing the whole directory would also take
	// anything a future version of this code puts beside the entries, and it
	// would report a count nobody measured. Emptying it one entry at a time
	// costs a syscall each and means the number returned is the number of
	// decks that are actually gone.
	//
	// An empty crypt is emptied successfully, and returns zero. Refusing
	// there would make the button an error message for the state it is
	// supposed to produce.
	Empty(ctx context.Context) (int, error)
}

// CryptFor is how a handler asks a Source for its crypt.
func CryptFor(s Source) (Crypt, error) {
	c, ok := s.(Crypt)
	if !ok || !s.Writable() {
		return nil, ErrReadOnly{}
	}
	return c, nil
}

// cryptID is the opaque handle: a digest of the tier's own name for the entry
// -- a directory name here, a row id there -- so the two tiers hand out the
// same *shape* of thing and neither hands out its own plumbing. Sixteen hex
// characters over a crypt that holds a handful of decks. It identifies, it
// does not authenticate: the tier that resolves one only ever looks inside a
// single owner's crypt, so a guessed id reaches nothing that was not already
// the caller's.
func cryptID(handle string) string {
	sum := sha256.Sum256([]byte(handle))
	return hex.EncodeToString(sum[:8])
}

// ---- the file tier ---------------------------------------------------------

// trashDir is the directory `Delete` moves a deck into, dot-prefixed so
// `deckPaths` cannot see it.
const trashDir = ".trash"

// entombedName is what `Delete` names an entry: the slug, the second-
// resolution stamp, and the collision suffix it adds when a deck is buried
// twice inside one second. The slug is greedy because slugs contain hyphens
// and the stamp does not -- `ishai-ojutai-dragonspeaker-20260811T220000Z`
// has to come apart in exactly one place.
var entombedName = regexp.MustCompile(`^(.+)-(\d{8}T\d{6}Z)(?:-(\d+))?$`)

// entombedStamp is the layout both halves use: written by `Delete`, read back
// here. One constant, so a change to either cannot desynchronise them.
const entombedStamp = "20060102T150405Z"

// cryptEntry is one crypt directory: what a caller may see, plus the folder
// name that is nobody's business above this file.
type cryptEntry struct {
	Entombed
	dir string
}

// Entombed reads `.trash`, newest first.
//
// Each entry is parsed rather than trusted: the directory's name says when it
// was buried and under what slug, and the deck file inside says what the deck
// was called, how big it was and who led it.
//
// **The slug comes off the directory and never off the file**, which reads
// backwards until you check: `deck.yaml` may carry a `slug:` key that does not
// match the folder it is in -- `mono-green-clean/deck.yaml` says `slug:
// mono-green` in this repo's own fixtures -- and the folder is what every
// route addresses the deck by. Trusting the file would raise a deck to an
// address nothing links to.
//
// An entry that will not parse is still listed. It is somebody's deck, and a
// crypt that silently omits what it cannot read is a crypt that tells you
// your deck is gone.
func (f *FileSource) Entombed(_ context.Context) ([]Entombed, error) {
	found, err := f.crypt()
	if err != nil {
		return nil, err
	}
	out := make([]Entombed, 0, len(found))
	for _, e := range found {
		out = append(out, e.Entombed)
	}
	return out, nil
}

// crypt is the whole directory read, entries and folder names together.
func (f *FileSource) crypt() ([]cryptEntry, error) {
	if !f.writable {
		return nil, ErrReadOnly{}
	}
	entries, err := os.ReadDir(filepath.Join(f.Root, trashDir))
	if err != nil {
		// No crypt yet is an empty crypt, and it is the normal state of a
		// library nobody has deleted anything from. Every other failure is
		// reported: "I cannot read the crypt" is not "your crypt is empty".
		if errors.Is(err, os.ErrNotExist) {
			return []cryptEntry{}, nil
		}
		return nil, fmt.Errorf("read the crypt: %w", err)
	}
	out := []cryptEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, f.entombedEntry(e.Name()))
	}
	// Newest first, and the folder name breaks a tie: two burials inside one
	// second differ only by `Delete`'s numeric suffix, and the suffixed one
	// is the later one.
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].dir > out[j].dir
	})
	return out, nil
}

// entombedEntry reads one crypt directory. Total by construction: a directory
// that says nothing about itself still comes back as an entry named after its
// own folder, because the alternative is dropping somebody's deck out of the
// only list that says it still exists.
func (f *FileSource) entombedEntry(name string) cryptEntry {
	out := cryptEntry{Entombed: Entombed{ID: cryptID(name), Slug: name, Name: name}, dir: name}
	if m := entombedName.FindStringSubmatch(name); m != nil {
		out.Slug, out.Name = m[1], m[1]
		if at, err := time.Parse(entombedStamp, m[2]); err == nil {
			out.At = at
		}
	}
	text, err := os.ReadFile(filepath.Join(f.Root, trashDir, name, "deck.yaml")) //nolint:gosec // a folder name this tier itself wrote
	if err != nil {
		return out
	}
	d, err := deck.FromText(string(text), out.Slug)
	if err != nil {
		return out
	}
	// Everything but the slug: see above.
	out.Name, out.Cards, out.Commander = d.Name, d.TotalCards(), d.Commander
	return out
}

// Restore moves the entry back out of `.trash`, under its own slug.
//
// **It never renames.** A deck comes back as itself or not at all: a restore
// that quietly became `goreclaw-2` would hand back a deck whose every
// artifact, link and log entry named something else. So a slug a living deck
// already holds is ErrExists, which the route turns into a sentence asking
// which of the two to keep -- a question only the player can answer.
func (f *FileSource) Restore(_ context.Context, id string) (string, error) {
	found, err := f.crypt()
	if err != nil {
		return "", err
	}
	var entry cryptEntry
	for _, e := range found {
		if e.ID == id {
			entry = e
			break
		}
	}
	if entry.dir == "" {
		return "", ErrNotFound{Slug: id}
	}
	// The slug comes off a file on a volume anybody with the disk can write,
	// so it is held to `path`'s own rule before it becomes a directory name
	// again -- the same reason that guard exists there.
	if !safeSegment(entry.Slug) {
		return "", fmt.Errorf("restore: %q is not a name a deck can live under", entry.Slug)
	}
	target := filepath.Join(f.Root, entry.Slug)
	if _, err := os.Stat(target); err == nil {
		return "", ErrExists{Slug: entry.Slug}
	}
	if err := os.Rename(filepath.Join(f.Root, trashDir, entry.dir), target); err != nil {
		return "", fmt.Errorf("restore deck %q: %w", entry.Slug, err)
	}
	return entry.Slug, nil
}

// Empty removes every crypt directory. The only thing in this package that
// destroys a deck.
//
// **It goes through `crypt()` rather than through `os.RemoveAll(.trash)`**,
// and the difference is the whole safety of it: `crypt()` is what `Entombed`
// answers with, so what is destroyed is exactly the list the player was shown
// and agreed to. A blanket removal would also delete whatever else is under
// `.trash` -- a file this tier did not write, a directory a later version
// keeps -- and could not say how many decks that was.
//
// A failure stops at the first entry and reports it, leaving the rest of the
// crypt where it is. The alternative is carrying on and returning a count that
// is a lower bound on what went and tells nobody which ones: a half-emptied
// crypt the player can look at is recoverable attention, and a number they
// cannot trust is not.
func (f *FileSource) Empty(_ context.Context) (int, error) {
	found, err := f.crypt()
	if err != nil {
		return 0, err
	}
	gone := 0
	for _, e := range found {
		// The same guard `Restore` applies before this name becomes a path
		// again, and for a sharper reason here: this call deletes a tree.
		if !safeSegment(e.dir) {
			return gone, fmt.Errorf("empty the crypt: %q is not a name this crypt can hold", e.dir)
		}
		if err := os.RemoveAll(filepath.Join(f.Root, trashDir, e.dir)); err != nil {
			return gone, fmt.Errorf("empty the crypt: %w", err)
		}
		gone++
	}
	return gone, nil
}

// safeSegment is `path`'s guard, said once: a slug is one path component or it
// is not a slug.
func safeSegment(s string) bool {
	return s != "" && strings.Trim(s, ".") != "" && !strings.ContainsAny(s, `/\`)
}

// ---- the SQL tier ----------------------------------------------------------

// Entombed is every row of this owner's carrying a `deleted_at`, newest first.
func (s *SQLSource) Entombed(ctx context.Context) ([]Entombed, error) {
	found, err := s.crypt(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Entombed, 0, len(found))
	for _, e := range found {
		out = append(out, e.entry)
	}
	return out, nil
}

// sqlCryptEntry pairs what a caller may see with the row id that is this
// tier's own business.
type sqlCryptEntry struct {
	entry Entombed
	id    int64
}

func (s *SQLSource) crypt(ctx context.Context) ([]sqlCryptEntry, error) {
	if !s.writable || s.sharedOnly {
		return nil, ErrReadOnly{}
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, slug, yaml, deleted_at FROM user_decks"+
			" WHERE owner_id = ? AND deleted_at IS NOT NULL"+
			" ORDER BY deleted_at DESC, id DESC", s.ownerID)
	if err != nil {
		return nil, fmt.Errorf("read the crypt: %w", err)
	}
	defer rows.Close()
	out := []sqlCryptEntry{}
	for rows.Next() {
		var (
			id      int64
			slug    string
			text    string
			deleted sql.NullString
		)
		if err := rows.Scan(&id, &slug, &text, &deleted); err != nil {
			return nil, err
		}
		e := Entombed{ID: cryptID(strconv.FormatInt(id, 10)), Slug: slug, Name: slug}
		if deleted.Valid {
			// The column's own format. A parse that fails leaves the zero
			// time rather than inventing a date.
			if at, err := time.Parse(time.RFC3339, deleted.String); err == nil {
				e.At = at.UTC()
			}
		}
		if d, err := deck.FromText(text, slug); err == nil {
			e.Name, e.Cards, e.Commander = d.Name, d.TotalCards(), d.Commander
		}
		out = append(out, sqlCryptEntry{entry: e, id: id})
	}
	return out, rows.Err()
}

// Restore clears the mark, which is this tier's whole undo.
//
// The partial unique index on `(owner_id, slug) WHERE deleted_at IS NULL` is
// what refuses a slug a living deck already holds -- the same guard `Create`
// leans on, and for the same reason: a SELECT first would let two restores
// racing for one slug both pass the look.
//
// `owner_id` is repeated in the UPDATE even though the id came out of this
// owner's own crypt a moment ago. It costs nothing and it means the one
// statement that can raise a deck cannot be aimed outside the library that
// found it.
func (s *SQLSource) Restore(ctx context.Context, id string) (string, error) {
	if !s.writable || s.write == nil || s.sharedOnly {
		return "", ErrReadOnly{}
	}
	found, err := s.crypt(ctx)
	if err != nil {
		return "", err
	}
	var entry sqlCryptEntry
	for _, e := range found {
		if e.entry.ID == id {
			entry = e
			break
		}
	}
	if entry.id == 0 {
		return "", ErrNotFound{Slug: id}
	}
	if _, err := s.write.ExecContext(ctx,
		"UPDATE user_decks SET deleted_at = NULL, updated_at = ? WHERE id = ? AND owner_id = ?",
		nowISO(), entry.id, s.ownerID); err != nil {
		if isUniqueViolation(err) {
			return "", ErrExists{Slug: entry.entry.Slug}
		}
		return "", fmt.Errorf("restore deck %q: %w", entry.entry.Slug, err)
	}
	return entry.entry.Slug, nil
}

// Empty deletes this owner's entombed rows outright, which is where the two
// tiers stop resembling each other: the file tier's crypt is a place and this
// one is a mark, so emptying it is the row going rather than the mark being
// cleared. `Restore` clears the mark; nothing clears this.
//
// One statement rather than a read and a loop. The `WHERE` is the crypt's own
// definition -- this owner, marked deleted -- so a row that arrives between
// the list the player looked at and this call is covered by the sentence they
// agreed to ("everything in your crypt"), and `RowsAffected` counts what
// actually went instead of what was expected to.
//
// `owner_id` is in the clause for the same reason `Restore` repeats it: the
// one statement here that can destroy a deck must not be able to reach outside
// the library that issued it.
func (s *SQLSource) Empty(ctx context.Context) (int, error) {
	if !s.writable || s.write == nil || s.sharedOnly {
		return 0, ErrReadOnly{}
	}
	result, err := s.write.ExecContext(ctx,
		"DELETE FROM user_decks WHERE owner_id = ? AND deleted_at IS NOT NULL", s.ownerID)
	if err != nil {
		return 0, fmt.Errorf("empty the crypt: %w", err)
	}
	gone, err := result.RowsAffected()
	if err != nil {
		// The decks are gone either way -- the DELETE succeeded. Only the
		// count is unavailable, and saying so beats reporting a zero that
		// would read as "there was nothing there".
		return 0, fmt.Errorf("empty the crypt: the crypt was emptied but not counted: %w", err)
	}
	return int(gone), nil
}

// The tiers that have a crypt, checked at compile time. `SharedOnly` is
// deliberately not one: somebody else's shelf is not somebody else's crypt,
// and a view that cannot even be *asked* is stronger than one that refuses.
var (
	_ Crypt = (*FileSource)(nil)
	_ Crypt = (*SQLSource)(nil)
)
