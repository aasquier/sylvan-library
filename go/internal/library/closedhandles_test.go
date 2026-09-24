package library_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// What the SQL tier says when `app.db` has gone out from under it.
//
// The tier holds **two handles on purpose** -- a read-only one and a
// read-write one, argued at `Resolver.AppWriteDB` -- and that separation is
// what makes this file possible: taking one away leaves the other, so a read
// failure and a write failure can be asked about one at a time rather than as
// one undifferentiated outage.
//
// The question is the one `internal/api/closeddb_test.go` asks a layer up,
// and it is not "did it return an error". It is **"did it lie"**. A `Slugs`
// that answered `[]` over a database that has gone says *you have no decks*,
// which is a different sentence from *I cannot read your decks*, and only one
// of the two is false -- a player whose shelf briefly looked empty has every
// reason to think something ate it. On the write side the lie is worse still:
// a caller told their edit landed has no reason to try again, and the deck
// they were told about is the one that did not change.
//
// The deployed fault this describes is real rather than theoretical: the
// volume holding `app.db` is a mount, and a mount can go.

// twoHandles is a writable SQL tier holding one deck, with the read and the
// write handle returned separately so a test can take away exactly one.
//
// The deck is created while both are live, so every refusal below is about
// the handle rather than about an empty library.
func twoHandles(t *testing.T) (src *library.SQLSource, read, write *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	read, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close() })
	write, err = auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = write.Close() })
	if _, err := auth.Create(context.Background(), write, "alice", "alice@example.com", false); err != nil {
		t.Fatal(err)
	}
	src = library.NewSQLSource(read, write, 1, true, false)
	if err := src.Create(context.Background(), "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	return src, read, write
}

// Every read, over a handle that has gone: a fault, never an empty shelf.
func TestTheSQLTierReportsAFaultRatherThanAnEmptyShelfWhenItCannotRead(t *testing.T) {
	t.Parallel()
	src, read, _ := twoHandles(t)
	ctx := context.Background()
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		what string
		run  func() (any, error)
	}{
		{"Slugs", func() (any, error) { return src.Slugs(ctx) }},
		{"All", func() (any, error) { return src.All(ctx) }},
		{"Get", func() (any, error) { return src.Get(ctx, "gyome") }},
		{"ReadText", func() (any, error) { return src.ReadText(ctx, "gyome") }},
		{"Artifacts", func() (any, error) { return src.Artifacts(ctx, "gyome") }},
		{"ReadArtifact", func() (any, error) { return src.ReadArtifact(ctx, "gyome", "primer-quick.md") }},
		{"ReadBaseline", func() (any, error) {
			text, _, err := src.ReadBaseline(ctx, "gyome")
			return text, err
		}},
		{"Entombed", func() (any, error) { return src.Entombed(ctx) }},
	} {
		got, err := tc.run()
		if err == nil {
			t.Errorf("%s answered %v over a database that has gone -- that reads "+
				"as an empty library rather than as a library it cannot read", tc.what, got)
			continue
		}
		// And the refusal is not mistaken for "no such deck": a 404 would
		// tell somebody their deck had been deleted.
		if library.IsNotFound(err) {
			t.Errorf("%s reported a missing deck when the database was the "+
				"thing that was missing: %v", tc.what, err)
		}
	}
}

// Every write, over a write handle that has gone. The read handle is left
// live on purpose, so each refusal is the write's own rather than a row that
// could not be found.
func TestNoSQLWriteReportsSuccessOverAHandleThatHasGone(t *testing.T) {
	t.Parallel()
	src, _, write := twoHandles(t)
	ctx := context.Background()
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		what string
		run  func() error
	}{
		{"WriteText", func() error {
			return src.WriteText(ctx, "gyome", strings.Replace(sqlDeck, "Gyome", "Edited", 1))
		}},
		{"Create", func() error { return src.Create(ctx, "another", sqlDeck) }},
		{"Delete", func() error { _, e := src.Delete(ctx, "gyome"); return e }},
		{"SetShared", func() error { return src.SetShared(ctx, "gyome", true) }},
		{"SetColiseumAtNight", func() error { return src.SetColiseumAtNight(ctx, "gyome", true) }},
		{"WriteArtifacts", func() error {
			_, e := src.WriteArtifacts(ctx, "gyome",
				artifacts.Files{{Name: "primer-quick.md", Text: "# a primer\n"}})
			return e
		}},
		{"Empty", func() error { _, e := src.Empty(ctx); return e }},
	} {
		if err := tc.run(); err == nil {
			t.Errorf("%s reported success over a handle that has gone -- the "+
				"caller has no reason to try again", tc.what)
		}
	}

	// `Restore` needs something in the crypt to aim at, and the crypt is a
	// mark this tier can no longer write -- so it is driven against an id
	// the live read handle can still see, marked before the handle went.
	if _, err := src.Restore(ctx, "nothing-here"); err == nil {
		t.Error("a restore reported success over a handle that has gone")
	}
}

// A restore that reaches the UPDATE, over a write handle that has gone.
//
// Separate from the sweep above because it needs a row already in the crypt:
// the mark goes on while both handles are live, and only then does the write
// handle go. Without that the refusal would come from the empty crypt and
// prove nothing about the write.
func TestARestoreOverAHandleThatHasGoneRefusesRatherThanRaisingTheDeck(t *testing.T) {
	t.Parallel()
	src, _, write := twoHandles(t)
	ctx := context.Background()
	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	buried, err := src.Entombed(ctx)
	if err != nil || len(buried) != 1 {
		t.Fatalf("the crypt holds %v (%v)", buried, err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	if slug, err := src.Restore(ctx, buried[0].ID); err == nil {
		t.Errorf("a deck was reported raised as %q over a handle that has gone", slug)
	}
}

// The edit that lands on nothing.
//
// The tier reads the row through one handle and writes it through another, so
// a row that disappears between the two is a real sequence rather than a
// contrivance -- somebody deletes the deck in one tab while another is
// saving. The guard says so where it stands: "nothing updated means the row
// went away between the read and the write", and reporting that as success
// would tell somebody their edit landed on a deck that had just been deleted.
//
// Two databases stand in for the race, because a race cannot be asked for: the
// read handle holds the row and the write handle is a library where it is
// already gone.
func TestAnEditThatUpdatesNoRowIsReportedAsAMissingDeckRatherThanASave(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	holdsIt := aliceDB(t, true) // what the read handle sees
	lostIt := aliceDB(t, false) // what the write handle finds a moment later
	src := library.NewSQLSource(holdsIt, lostIt, 1, true, false)

	err := src.WriteText(ctx, "gyome", sqlDeck)
	if err == nil {
		t.Fatal("an edit that changed no row was reported as a save")
	}
	if !library.IsNotFound(err) {
		t.Errorf("the refusal was %v, which does not say the deck is gone", err)
	}
}

// aliceDB is a library with the one account in it, with or without her deck.
func aliceDB(t *testing.T, withDeck bool) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := auth.Create(ctx, db, "alice", "alice@example.com", false); err != nil {
		t.Fatal(err)
	}
	if !withDeck {
		return db
	}
	const stamp = "2026-01-01T00:00:00+00:00"
	if _, err := db.ExecContext(ctx,
		"INSERT INTO user_decks (owner_id, slug, name, yaml, shared, created_at, updated_at)"+
			" VALUES (1, 'gyome', 'Gyome, Master Chef', ?, 0, ?, ?)",
		sqlDeck, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return db
}

// The crypt is the owner's alone, and a view that is not the owner's is
// refused before any query runs -- so a shared shelf cannot be asked what its
// owner deleted.
func TestSomebodyElsesShelfIsNotSomebodyElsesCrypt(t *testing.T) {
	t.Parallel()
	_, read, write := twoHandles(t)
	ctx := context.Background()

	for _, tc := range []struct {
		what string
		src  *library.SQLSource
	}{
		{"a shared-only view", library.NewSQLSource(read, write, 1, true, true)},
		{"a read-only tier", library.NewSQLSource(read, nil, 1, false, false)},
	} {
		if _, err := tc.src.Entombed(ctx); !library.IsReadOnly(err) {
			t.Errorf("%s answered the crypt with %v", tc.what, err)
		}
		if _, err := tc.src.Restore(ctx, "anything"); !library.IsReadOnly(err) {
			t.Errorf("%s accepted a restore with %v", tc.what, err)
		}
		if _, err := tc.src.Empty(ctx); !library.IsReadOnly(err) {
			t.Errorf("%s accepted an emptying with %v", tc.what, err)
		}
	}
}

// A row whose YAML stopped parsing is reported, never skipped.
//
// `All` is the shelf, and a deck silently dropped out of it is a deck the
// owner will believe is gone. The row is written past the tier on purpose --
// every write path re-parses, so this is the shape a hand-edited database or
// a half-finished restore leaves rather than anything the app can produce.
func TestARowThatNoLongerParsesFailsTheShelfRatherThanVanishingFromIt(t *testing.T) {
	t.Parallel()
	src, read, _ := twoHandles(t)
	ctx := context.Background()

	if _, err := read.ExecContext(ctx,
		"INSERT INTO user_decks (owner_id, slug, name, yaml, shared, created_at, updated_at)"+
			" VALUES (1, 'broken', 'Broken', ?, 0, ?, ?)",
		"cards: [\n", "2026-01-01T00:00:00+00:00", "2026-01-01T00:00:00+00:00"); err != nil {
		t.Fatal(err)
	}
	decks, err := src.All(ctx)
	if err == nil {
		t.Fatalf("a shelf of %d decks was served over a row that does not parse", len(decks))
	}
}

// An artifact whose build time is not a timestamp fails the list rather than
// being handed to the page as a zero date -- 1 January year one on a primer
// built this morning is a wrong answer dressed as a right one.
func TestAnArtifactWithAnUnreadableBuildTimeFailsTheList(t *testing.T) {
	t.Parallel()
	src, read, _ := twoHandles(t)
	ctx := context.Background()

	if _, err := read.ExecContext(ctx,
		"INSERT INTO user_deck_artifacts (deck_id, name, body, built_at) VALUES (1, ?, ?, ?)",
		"primer-quick.md", "# a primer\n", "the day before yesterday"); err != nil {
		t.Fatal(err)
	}
	got, err := src.Artifacts(ctx, "gyome")
	if err == nil {
		t.Fatalf("the artifact list came back as %v over an unreadable build time", got)
	}
	if !strings.Contains(err.Error(), "the day before yesterday") {
		t.Errorf("the refusal was %q without quoting what it could not read", err)
	}
}

// A Source that cannot write at all is refused in the same words a read-only
// tier is, so the route layer has one path rather than two. There is no such
// source in the app today; this holds the promise that adding one would not
// need a second branch at every call site.
func TestASourceThatCannotWriteAtAllIsRefusedLikeAReadOnlyTier(t *testing.T) {
	t.Parallel()
	var readOnly library.Source = notAWriter{}
	w, err := library.WriterFor(readOnly, "gyome")
	if w != nil {
		t.Error("a source with no write half handed one over")
	}
	if !library.IsReadOnly(err) {
		t.Fatalf("the refusal was %v", err)
	}
	if !strings.Contains(err.Error(), "gyome") {
		t.Errorf("the refusal %q does not name the deck it is about", err)
	}
}

// notAWriter is a Source and nothing more: the embedded interface supplies
// the read half's signatures and none of the write half's, which is exactly
// the shape `WriterFor`'s type assertion exists to catch.
type notAWriter struct{ library.Source }

// The tiers still answer errors.As, which is how the route layer picks a
// status without knowing which one answered.
func TestTheTypedRefusalsSurviveWrapping(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		err  error
		is   func(error) bool
	}{
		{"read-only", library.ErrReadOnly{Slug: "gyome"}, library.IsReadOnly},
		{"not found", library.ErrNotFound{Slug: "gyome"}, library.IsNotFound},
		{"no night games", library.ErrNoNightGames{Slug: "gyome"}, library.IsNoNightGames},
	} {
		wrapped := errWrap{tc.err}
		if !tc.is(wrapped) {
			t.Errorf("a wrapped %s refusal was not recognised", tc.what)
		}
		if tc.is(errors.New("something else")) {
			t.Errorf("the %s test accepted an unrelated error", tc.what)
		}
	}

	// The whole-library subject: a create has no slug to name, and the
	// sentence still has to read.
	if got := (library.ErrReadOnly{}).Error(); !strings.Contains(got, "this library") {
		t.Errorf("a slugless refusal reads %q", got)
	}
	// Commandment 10: the night refusal names no machinery.
	night := (library.ErrNoNightGames{Slug: "gyome"}).Error()
	for _, word := range []string{"column", "SQL", "database", "tier", "file"} {
		if strings.Contains(strings.ToLower(night), strings.ToLower(word)) {
			t.Errorf("the night refusal says %q to a player: %q", word, night)
		}
	}
}

type errWrap struct{ inner error }

func (e errWrap) Error() string { return "while saving: " + e.inner.Error() }
func (e errWrap) Unwrap() error { return e.inner }
