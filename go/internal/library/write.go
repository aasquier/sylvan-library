package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckedit"
)

// The write side of a Source.
//
// Five verbs, grouped in three families on purpose. `WriteText` belongs to
// the nine editing routes; `Create`, `Delete` and `SetShared` to the
// lifecycle, because an update and a create have opposite safety
// requirements -- an update to a deck that has vanished is a bug, a create
// over a deck that exists destroys somebody's work -- and a method that exists
// but refuses is a method somebody wires up. `WriteArtifacts` belongs to the
// rebuild, for the same reason again: it is the only write here that does not
// touch `deck.yaml` at all.
//
// **`Delete` returns where the deck went, and that is the protocol's
// requirement rather than a nicety.** A tier that can only answer "yes" has
// destroyed the deck rather than removed it, and since ADR 30 no deck has a
// revision behind it: the file tier's `.trash/` directory and the SQL tier's
// `deleted_at` are the only things standing between a misclick and the work.
//
// **Who may write is decided by which Source you were handed**, never by a
// branch in a handler: the shared-only view refuses because a deck you can see
// is not a deck you own, and a read-only tier refuses because the caller is
// not its owner. That is the same arrangement the reads use, where a private
// deck is *absent* rather than forbidden -- so a 403 here is only ever an
// answer about a deck the caller can already read (ADR 5, ADR 22).

// ErrReadOnly is the read-only refusal: this caller may see these decks and
// may not change them. The route layer turns it into a 403, which is the one
// refusal in the deck family that is not a 404 -- because the caller has
// already been shown the deck, so its existence is not the secret.
//
// **The whole served sentence is assembled here, not at the route layer.**
// The subject and its wrapping are one string on purpose: built in one
// place, every route answers a 403 in the same words. It said the wrong
// words until 2026-08-22, and a shape test could not see that -- a shape
// records `{"detail": "string"}` and a user reads the string.
type ErrReadOnly struct{ Slug string }

func (e ErrReadOnly) Error() string {
	// The whole-library subject, for a refusal about a deck that does not
	// exist yet: a create has no slug to name.
	subject := e.Slug
	if subject == "" {
		subject = "this library"
	}
	return "read-only: " + subject + " is not yours to change"
}

// ErrExists refuses to overwrite a deck that is
// already there. The route layer turns it into the refusal `create` and
// `import` both answer with, which names the slug and asks for another.
type ErrExists struct{ Slug string }

func (e ErrExists) Error() string { return "a deck called '" + e.Slug + "' already exists" }

// Writer is the write half of a Source. Sources implement it; the interface is
// separate so a handler that only reads cannot accidentally hold one.
type Writer interface {
	// WriteText replaces the deck's YAML. ErrNotFound if the deck is not
	// there, ErrReadOnly if the caller may not edit it.
	WriteText(ctx context.Context, slug, text string) error
	// Create adds a deck that does not exist yet. ErrExists rather than an
	// overwrite; ErrReadOnly if the caller may not write here at all.
	Create(ctx context.Context, slug, text string) error
	// Delete removes a deck and says where it went. ErrNotFound or
	// ErrReadOnly.
	Delete(ctx context.Context, slug string) (string, error)
	// SetShared puts the deck on display to other accounts, or takes it off
	// (ADR 22). Its own verb rather than a field on WriteText, because the two
	// tiers keep this fact in different places -- `deck.yaml` here, a column
	// there -- and a caller should not have to know which.
	SetShared(ctx context.Context, slug string, shared bool) error
	// WriteArtifacts stores a build's output and says what was stored.
	//
	// Rendered text rather than a deck, because generating and storing are
	// different jobs and only one of them is a Source's (`artifacts.RenderAll`).
	// The mapping is stored **whole**, snapshot included: `Deliverables`
	// governs what may be *read*, not what a build may write, and the baseline
	// is part of the build.
	//
	// Names are returned rather than paths -- the SQL tier has none, and no
	// caller needs one. ErrNotFound or ErrReadOnly.
	WriteArtifacts(ctx context.Context, slug string, files artifacts.Files) ([]string, error)
}

// WriterFor is how a handler asks a Source for its write half. A Source that
// cannot write at all -- there is none today -- refuses in the same words a
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
// **Written through a temporary file and renamed**, where a plain
// truncating write would leave the same bytes. Nothing observable changes;
// what changes is the failure mode. `deck.yaml` is the source of truth
// (ADR 1), a truncating write that dies halfway leaves it truncated, and
// since ADR 30 there is no revision to restore it from. A rename within the
// same directory is atomic, so the file is either the old deck or the new
// one. The rename is deliberate hardening, recorded here so it is not
// mistaken for ceremony somebody should simplify away.
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

// WriteArtifacts writes a build into `<root>/<slug>/artifacts/`.
//
// Writability first, ahead of the deck's own existence, which is the tier's
// standing order and the reason for it: a
// caller who may not write gets the same answer whether or not the deck is
// there, so no sequence of refused builds maps out the library. The route
// above has already resolved the deck, so this ordering is never what a client
// sees -- it is what the tier promises on its own.
//
// `artifacts.Store` rather than a loop, so this tier and `mtglab decks build`
// cannot disagree about what a rebuild leaves behind. They did until
// 2026-08-21, over a stale `swaps.md`.
func (f *FileSource) WriteArtifacts(_ context.Context, slug string, files artifacts.Files) ([]string, error) {
	if !f.writable {
		return nil, ErrReadOnly{Slug: slug}
	}
	dir, err := f.artifactDir(slug) // ErrNotFound for a deck that is not there
	if err != nil {
		return nil, err
	}
	return artifacts.Store(files, dir)
}

// Create writes `<root>/<slug>/deck.yaml`, refusing to overwrite one.
//
// The existence check is `os.OpenFile` with O_EXCL rather than a Stat followed
// by a write: the two-step version has a window between the look and the
// write, and this is the operation where landing in that window means one
// person's deck arriving on top of another's.
func (f *FileSource) Create(_ context.Context, slug, text string) error {
	if !f.writable {
		return ErrReadOnly{Slug: slug}
	}
	// The same guard `path` applies, and it has to be applied here too: `path`
	// answers ErrNotFound for a slug with a separator in it because there is
	// no such deck, and a create would happily make one outside the root.
	if slug == "" || strings.Trim(slug, ".") == "" || strings.ContainsAny(slug, `/\`) {
		return ErrExists{Slug: slug}
	}
	// 0755 and 0644 are what a 022 umask leaves, which is what the container
	// runs. Tighter modes would be safer in the abstract and are wrong here:
	// the server and the CLI write this directory as the same user, nothing
	// else reads it, and a library whose decks wear two different sets of
	// permissions depending on which surface made them is a difference
	// somebody has to explain to a volume backup. `writeAtomically` preserves
	// an existing file's mode for the same reason.
	dir := filepath.Join(f.Root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the library's standing 022-umask mode
		return fmt.Errorf("create deck %q: %w", slug, err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "deck.yaml"), //nolint:gosec // the library's standing 022-umask mode
		os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists{Slug: slug}
		}
		return fmt.Errorf("create deck %q: %w", slug, err)
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return fmt.Errorf("create deck %q: %w", slug, err)
	}
	return file.Close()
}

// Delete moves the deck's whole directory into `.trash/`, and says where.
//
// A move rather than an unlink, for one reason: since ADR 30 no deck is in
// git, so no deck has a revision to restore from -- the draft imported ten
// minutes ago and the curated deck of a year's thinking are equally gone.
// Moving costs one rename and makes the mistake survivable.
//
// The directory goes whole -- `artifacts/` with it -- because the artifacts
// are generated from the deck file and a folder of primers for a deck that no
// longer exists is worse than no folder at all. `.trash` is dot-prefixed so
// `deckPaths` cannot see it.
//
// **The name it is given is read back**, by `crypt.go`: the stamp is how the
// crypt says when a deck was buried, and the format is a constant both halves
// share so that changing it here cannot quietly stop the list over there from
// parsing. Where it went used to be the only way back and was shown to the
// player as a path; the crypt is the way back now, and this return value is
// the protocol's proof that a deck was moved rather than destroyed.
func (f *FileSource) Delete(_ context.Context, slug string) (string, error) {
	if !f.writable {
		return "", ErrReadOnly{Slug: slug}
	}
	path, err := f.path(slug)
	if err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format(entombedStamp)
	trash := filepath.Join(f.Root, trashDir, slug+"-"+stamp)
	if err := os.MkdirAll(filepath.Dir(trash), 0o755); err != nil { //nolint:gosec // the deck's own directory mode
		return "", fmt.Errorf("delete deck %q: %w", slug, err)
	}
	// Import, delete, re-import, delete again inside one second is a real
	// sequence, and the stamp is only second-resolution. A collision must
	// not bury the earlier deletion -- `.trash` exists to make a mistake
	// survivable -- and `os.Rename` onto a non-empty target fails, which is
	// louder and still not what anybody wants. So the loop below finds a
	// free name instead.
	if _, err := os.Stat(trash); err == nil {
		for n := 2; ; n++ {
			candidate := trash + "-" + strconv.Itoa(n)
			if _, err := os.Stat(candidate); err != nil {
				trash = candidate
				break
			}
		}
	}
	// `filepath.Dir(path)`, not `path`: the deck is a directory, and leaving
	// the artifacts behind would strand them beside a deck that is gone.
	if err := os.Rename(filepath.Dir(path), trash); err != nil {
		return "", fmt.Errorf("delete deck %q: %w", slug, err)
	}
	return trash, nil
}

// SetShared writes `shared:` into the deck file. The YAML is this tier's
// truth.
//
// A **surgical** edit like every other write (ADR 12), through
// `deckedit.SetShared`. It was a load-and-dump round trip until 2026-08-22
// -- the only thing on the write path that rewrote a whole existing file --
// so one press of the deck page's share toggle took a hand-written deck's
// section banners, its trailing comments and its folded blocks with it.
// Reproducing the bytes meant asking what they should be; Aaron ruled, and
// the surgical edit is the ruling.
//
// Two standing rules: nothing is written when the flag already says what
// was asked for, and `true` removes the key rather than asserting the default.
func (f *FileSource) SetShared(ctx context.Context, slug string, shared bool) error {
	if !f.writable {
		return ErrReadOnly{Slug: slug}
	}
	text, err := f.ReadText(ctx, slug)
	if err != nil {
		return err
	}
	d, err := deck.FromText(text, slug)
	if err != nil {
		return fmt.Errorf("read deck %q: %w", slug, err)
	}
	if d.Shared == shared {
		return nil
	}
	updated, err := deckedit.SetShared(text, shared)
	if err != nil {
		return err
	}
	path, err := f.path(slug)
	if err != nil {
		return err
	}
	return writeAtomically(path, updated)
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
// This writes `app.db`, which the auth layer also writes. Same argument as
// the activity log's: WAL is persistent in the file, the busy timeout is set
// on the handle, and no two writers race for one row -- a deck edit
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

// nowISO is the tier's timestamp: UTC to the second, carrying an offset
// rather than a `Z` -- the recorded column format. `parseISO` on the read
// side accepts it.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05-07:00")
}

// Create inserts the row, refusing to bury one that is already there.
//
// **Private.** ADR 22's default for this tier, and the argument is `decks
// import`: it writes a draft with an empty `why` on all 99 cards, and
// publishing that the instant it exists is nobody's intent. `SetShared` is how
// it goes on display.
func (s *SQLSource) Create(ctx context.Context, slug, text string) error {
	if !s.writable || s.write == nil {
		return ErrReadOnly{Slug: slug}
	}
	d, err := deck.FromText(text, slug)
	if err != nil {
		return fmt.Errorf("the new deck does not parse: %w", err)
	}
	now := nowISO()
	// The partial unique index is the real guard and this INSERT leans on it:
	// a SELECT-then-INSERT would let two creates racing for one slug both pass
	// the look, and the index answers the same refusal however the race falls.
	_, err = s.write.ExecContext(ctx,
		"INSERT INTO user_decks"+
			" (owner_id, slug, name, yaml, shared, created_at, updated_at)"+
			" VALUES (?, ?, ?, ?, 0, ?, ?)",
		s.ownerID, slug, d.Name, text, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrExists{Slug: slug}
		}
		return fmt.Errorf("create deck %q: %w", slug, err)
	}
	return nil
}

// Delete marks the row and says where it went.
//
// A mark rather than a DELETE, which is the protocol's requirement and not a
// preference: an implementation that cannot say where the deck went has
// destroyed it rather than removed it.
func (s *SQLSource) Delete(ctx context.Context, slug string) (string, error) {
	if !s.writable || s.write == nil {
		return "", ErrReadOnly{Slug: slug}
	}
	r, err := s.row(ctx, slug)
	if err != nil {
		return "", err
	}
	if _, err := s.write.ExecContext(ctx,
		"UPDATE user_decks SET deleted_at = ? WHERE id = ?", nowISO(), r.id); err != nil {
		return "", fmt.Errorf("delete deck %q: %w", slug, err)
	}
	return "user_decks:" + strconv.FormatInt(r.id, 10), nil
}

// SetShared flips the column this tier treats as the truth. The owner's call
// alone.
func (s *SQLSource) SetShared(ctx context.Context, slug string, shared bool) error {
	if !s.writable || s.write == nil {
		return ErrReadOnly{Slug: slug}
	}
	r, err := s.row(ctx, slug)
	if err != nil {
		return err
	}
	flag := 0
	if shared {
		flag = 1
	}
	if _, err := s.write.ExecContext(ctx,
		"UPDATE user_decks SET shared = ?, updated_at = ? WHERE id = ?",
		flag, nowISO(), r.id); err != nil {
		return fmt.Errorf("share deck %q: %w", slug, err)
	}
	return nil
}

// WriteArtifacts replaces this deck's rows in `user_deck_artifacts`.
//
// Deleted then inserted, in one transaction, so a rebuild leaves exactly what
// this build produced -- the same pruning `artifacts.Store` does to a
// directory, and for the same reason: a `swaps.md` from an older build
// describes a diff that no longer exists. **Only `Deliverables` are swept**,
// so the snapshot survives a build that writes no swap list, as it must.
//
// One transaction is doing real work here, unlike the single-statement writes
// above: a failure between the sweep and the insert would leave the deck with
// no artifacts at all, which is a worse answer than either the old set or the
// new one.
func (s *SQLSource) WriteArtifacts(ctx context.Context, slug string, files artifacts.Files) ([]string, error) {
	if !s.writable || s.write == nil {
		return nil, ErrReadOnly{Slug: slug}
	}
	r, err := s.row(ctx, slug)
	if err != nil {
		return nil, err
	}
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("build %q: %w", slug, err)
	}
	defer func() { _ = tx.Rollback() }() // a no-op once Commit has succeeded

	for _, name := range Deliverables {
		if files.Has(name) {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM user_deck_artifacts WHERE deck_id = ? AND name = ?",
			r.id, name); err != nil {
			return nil, fmt.Errorf("build %q: %w", slug, err)
		}
	}
	now := nowISO()
	written := make([]string, 0, len(files))
	for _, file := range files {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO user_deck_artifacts (deck_id, name, body, built_at)"+
				" VALUES (?, ?, ?, ?) ON CONFLICT(deck_id, name) DO UPDATE SET"+
				" body = excluded.body, built_at = excluded.built_at",
			r.id, file.Name, file.Text, now); err != nil {
			return nil, fmt.Errorf("build %q: %w", slug, err)
		}
		written = append(written, file.Name)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("build %q: %w", slug, err)
	}
	return written, nil
}

// isUniqueViolation asks the driver's error whether the partial unique index
// refused. Matched on the message because `modernc.org/sqlite` reports its
// codes through one error type and the constraint's name is what identifies
// it; a wrong answer here reports a real failure as "already exists", which is
// why the match is on the constraint word rather than on "unique" alone.
func isUniqueViolation(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") ||
		strings.Contains(text, "constraint failed: user_decks")
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

// Create refuses without asking what is there. This view is somebody else's
// library seen through a keyhole, and a slug it does not show is one it must
// not be able to test for -- so unlike the three below it does not look first.
func (v *SharedOnly) Create(context.Context, string, string) error {
	return ErrReadOnly{}
}

func (v *SharedOnly) Delete(ctx context.Context, slug string) (string, error) {
	if err := v.visible(ctx, slug); err != nil {
		return "", err
	}
	return "", ErrReadOnly{Slug: slug}
}

func (v *SharedOnly) SetShared(ctx context.Context, slug string, _ bool) error {
	if err := v.visible(ctx, slug); err != nil {
		return err
	}
	return ErrReadOnly{Slug: slug}
}

// WriteArtifacts refuses. The deliverables are the *shareable* surface, so a
// reader may have every one of them -- and rebuilding them is a write to
// somebody else's library, which is the deck's owner's alone.
func (v *SharedOnly) WriteArtifacts(ctx context.Context, slug string, _ artifacts.Files) ([]string, error) {
	if err := v.visible(ctx, slug); err != nil {
		return nil, err
	}
	return nil, ErrReadOnly{Slug: slug}
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
