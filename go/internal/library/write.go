package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The write side of a Source, Phase 4's addition to the read-only tiers.
//
// One verb only: replacing a deck's YAML. `create` and `delete` are a later
// flip and are deliberately absent rather than stubbed -- they have opposite
// safety requirements to an update (an update to a deck that has vanished is a
// bug; a create over a deck that exists destroys somebody's work), and a
// method that exists but refuses is a method somebody wires up.
//
// **Who may write is decided by which Source you were handed**, never by a
// branch in a handler: the shared-only view refuses because a deck you can see
// is not a deck you own, and a read-only tier refuses because the caller is
// not its owner. That is the same arrangement the reads use, where a private
// deck is *absent* rather than forbidden -- so a 403 here is only ever an
// answer about a deck the caller can already read (ADR 5, ADR 22).

// ErrReadOnly is `ReadOnlySource`: this caller may see these decks and may not
// change them. The route layer turns it into a 403, which is the one refusal
// in the deck family that is not a 404 -- because the caller has already been
// shown the deck, so its existence is not the secret.
type ErrReadOnly struct{ Slug string }

func (e ErrReadOnly) Error() string {
	if e.Slug == "" {
		return "this library is read-only"
	}
	return "'" + e.Slug + "' is not yours to edit"
}

// Writer is the write half of a Source. Sources implement it; the interface is
// separate so a handler that only reads cannot accidentally hold one.
type Writer interface {
	// WriteText replaces the deck's YAML. ErrNotFound if the deck is not
	// there, ErrReadOnly if the caller may not edit it.
	WriteText(ctx context.Context, slug, text string) error
}

// WriterFor is how a handler asks a Source for its write half. A Source that
// cannot write at all -- there is none today, and there will be while
// `create` and `delete` are still Python's -- refuses in the same words a
// read-only tier does, so the route layer has one path rather than two.
func WriterFor(s Source, slug string) (Writer, error) {
	w, ok := s.(Writer)
	if !ok {
		return nil, ErrReadOnly{Slug: slug}
	}
	if !s.Writable() {
		return nil, ErrReadOnly{Slug: slug}
	}
	return w, nil
}

// ---- the file tier ---------------------------------------------------------

// WriteText replaces `<root>/<slug>/deck.yaml`.
//
// **Written through a temporary file and renamed**, where Python writes the
// path directly. The bytes that land are identical and nothing observable
// changes; what changes is the failure mode. `deck.yaml` is the source of
// truth (ADR 1), a truncating write that dies halfway leaves it truncated, and
// since ADR 30 there is no revision to restore it from. A rename within the
// same directory is atomic, so the file is either the old deck or the new one.
// This is a deliberate improvement on the original rather than a port of it,
// recorded here so it is not mistaken for a divergence somebody should
// "fix" back.
func (f *FileSource) WriteText(ctx context.Context, slug, text string) error {
	if !f.writable {
		return ErrReadOnly{Slug: slug}
	}
	path, err := f.path(slug)
	if err != nil {
		return err
	}
	return writeAtomically(path, text)
}

func writeAtomically(path, text string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".deck-*.yaml")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	// A no-op once the rename has succeeded, and the only cleanup that matters
	// when it has not: a temporary file left in the deck's directory would be
	// picked up by nothing -- the glob wants `deck.yaml` -- but would sit
	// there forever.
	defer func() { _ = os.Remove(name) }()

	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	// Flush to the disk before the rename, so a power loss cannot leave the
	// directory entry pointing at a file whose contents never arrived.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	// The temporary file is created 0600; the deck files are the app's own and
	// are read by nothing else, but a mode that changes on the first edit is
	// the kind of surprise a volume backup notices.
	if info, err := os.Stat(path); err == nil {
		if err := os.Chmod(name, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ---- the SQL tier ----------------------------------------------------------

// WriteText stores the deck's YAML and re-derives the column that summarises
// it.
//
// `name` is a denormalised copy of what the YAML says, so the library list
// does not parse every row to render a title. Re-derived on every write rather
// than accepted from a caller, which is what stops it drifting from the text
// it describes.
//
// **`shared` is deliberately not touched.** It is this tier's own truth and it
// is changed by the sharing route and nothing else -- otherwise editing a
// card's rationale would silently republish, or silently hide, the deck it
// belongs to.
//
// This writes `app.db`, which Python also writes. Same argument as the
// activity log's: WAL is persistent in the file, the busy timeout is set on
// the handle, and the two runtimes are not racing for one row -- a deck edit
// goes through the door or through the CLI, never both at once.
func (s *SQLSource) WriteText(ctx context.Context, slug, text string) error {
	if !s.writable {
		return ErrReadOnly{Slug: slug}
	}
	if s.write == nil {
		return ErrReadOnly{Slug: slug}
	}
	// The row must exist and be visible to this caller before it is updated:
	// an update to a deck that has vanished is a bug, not a create.
	if _, err := s.row(ctx, slug); err != nil {
		return err
	}
	d, err := deck.FromText(text, slug)
	if err != nil {
		return fmt.Errorf("the edited deck no longer parses: %w", err)
	}
	result, err := s.write.ExecContext(ctx,
		"UPDATE user_decks SET yaml = ?, name = ?, updated_at = ?"+
			" WHERE owner_id = ? AND slug = ? AND deleted_at IS NULL",
		text, d.Name, nowISO(), s.ownerID, slug)
	if err != nil {
		return fmt.Errorf("write deck %q: %w", slug, err)
	}
	// Nothing updated means the row went away between the read and the write.
	// Reporting that as success would tell somebody their edit landed when the
	// deck it belonged to had just been deleted.
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound{Slug: slug}
	}
	return nil
}

// nowISO is `_now()` in `decks/sqlsource.py`:
// `datetime.now(UTC).isoformat(timespec="seconds")`, which carries an offset
// rather than a `Z`. `parseISO` on the read side accepts it.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05-07:00")
}

// ---- the shared-only view --------------------------------------------------

// WriteText refuses. A deck somebody shared with you is a deck you may read.
//
// The refusal is ErrReadOnly and therefore a 403, and that is right *because
// the caller can see this deck*: a deck they could not see is absent from this
// view entirely and every verb against it is already ErrNotFound, so no 403
// here can reveal a private deck's existence.
func (v *SharedOnly) WriteText(ctx context.Context, slug, _ string) error {
	if err := v.visible(ctx, slug); err != nil {
		return err
	}
	return ErrReadOnly{Slug: slug}
}

// IsReadOnly and IsNotFound let the route layer choose a status without
// knowing which tier answered.
func IsReadOnly(err error) bool {
	var e ErrReadOnly
	return errors.As(err, &e)
}

func IsNotFound(err error) bool {
	var e ErrNotFound
	return errors.As(err, &e)
}

// ensure the tiers still satisfy the interface after any refactor.
var (
	_ Writer = (*FileSource)(nil)
	_ Writer = (*SQLSource)(nil)
	_ Writer = (*SharedOnly)(nil)
	_        = sql.ErrNoRows
)
