package library_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The crypt: the half of ADR 27 the deck level never had.
//
// The bug these are written against did not look like a bug. `Delete` moved
// the deck somewhere safe and returned the path, and the path was rendered to
// the player under a sentence telling them to go and get it from a shell. The
// deck really was safe; the way back was addressed to somebody who does not
// exist.
//
// Four things are held here, and the middle two are the ones a reading would
// have got wrong.
//
//  1. A round trip: bury it, find it, raise it, and the deck is back where
//     every link to it already points.
//  2. **The slug comes off the folder, not out of the file.** `deck.yaml` may
//     carry a `slug:` key that disagrees with the directory it lives in --
//     this repo's own `mono-green-clean` fixture does -- and the folder is
//     what every route addresses. A restore that trusted the file would raise
//     the deck to an address nothing links to.
//  3. **A restore never renames.** A slug a living deck already holds is a
//     refusal, not a `-2` suffix, because a deck that comes back under
//     another name has left its artifacts, its log and every link behind.
//  4. Two burials of one slug stay two things, and the id says which.

func cryptOf(t *testing.T, src library.Source) library.Crypt {
	t.Helper()
	c, err := library.CryptFor(src)
	if err != nil {
		t.Fatalf("no crypt: %v", err)
	}
	return c
}

// The whole round trip, and the artifacts riding along in both directions.
func TestADeckComesBackOutOfTheCryptWhereItWent(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()
	crypt := cryptOf(t, src)

	art := filepath.Join(root, "gyome", "artifacts")
	if err := os.MkdirAll(art, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(art, "primer.md"), []byte("# Primer"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Empty until something is buried -- and an empty crypt is a real answer,
	// not a missing directory reported as a failure.
	before, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatalf("a library with no crypt yet: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("a library nobody has deleted from holds %d entombed decks", len(before))
	}

	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 1 {
		t.Fatalf("the crypt holds %d decks after one deletion", len(entombed))
	}
	e := entombed[0]
	if e.Slug != "gyome" || e.Name != "Test Deck" {
		t.Errorf("the crypt describes the deck as %q / %q", e.Slug, e.Name)
	}
	if e.ID == "" {
		t.Error("an entry with no handle is an entry nothing can raise")
	}
	if e.At.IsZero() {
		t.Error("the burial has no time on it, and `Delete` stamps one")
	}

	slug, err := crypt.Restore(ctx, e.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if slug != "gyome" {
		t.Errorf("the deck came back as %q", slug)
	}
	if _, err := src.Get(ctx, "gyome"); err != nil {
		t.Errorf("the raised deck is not readable: %v", err)
	}
	// The artifacts went into the crypt with the deck and came back with it.
	if _, err := os.Stat(filepath.Join(root, "gyome", "artifacts", "primer.md")); err != nil {
		t.Errorf("the artifacts did not come back: %v", err)
	}
	// And the crypt is empty again: a raised deck is not still buried.
	after, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("the crypt still holds %d decks after the restore", len(after))
	}
}

// The regression a reading would have got backwards: the deck file's own
// `slug:` key is not the deck's address. `mono-green-clean/deck.yaml` says
// `slug: mono-green` in this repo's fixtures, and it is the folder every route
// addresses.
func TestARestoreUsesTheFoldersSlugAndNotTheFilesOwn(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t)
	ctx := context.Background()
	crypt := cryptOf(t, src)

	const disagreeing = "slug: mono-green\nname: Clean Green\nstatus: draft\ncards: []\n"
	if err := src.Create(ctx, "mono-green-clean", disagreeing); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Delete(ctx, "mono-green-clean"); err != nil {
		t.Fatal(err)
	}
	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 1 || entombed[0].Slug != "mono-green-clean" {
		t.Fatalf("the crypt filed it under %+v", entombed)
	}
	// The name still comes off the file: that one is the deck's own, and the
	// folder has no better answer.
	if entombed[0].Name != "Clean Green" {
		t.Errorf("the crypt calls it %q", entombed[0].Name)
	}
	slug, err := crypt.Restore(ctx, entombed[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "mono-green-clean" {
		t.Fatalf("the deck came back as %q -- every link to it says mono-green-clean", slug)
	}
	if _, err := os.Stat(filepath.Join(root, "mono-green", "deck.yaml")); err == nil {
		t.Error("the deck was raised under the file's slug rather than its own address")
	}
}

// A restore never renames. The refusal names the slug, because choosing
// between two decks with one name is the player's call and nobody else's.
func TestARestoreRefusesRatherThanRenaming(t *testing.T) {
	t.Parallel()
	src, _ := writableTier(t, "gyome")
	ctx := context.Background()
	crypt := cryptOf(t, src)

	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	// The slug is free again, so a new deck takes it.
	if err := src.Create(ctx, "gyome", oneDeck); err != nil {
		t.Fatal(err)
	}
	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = crypt.Restore(ctx, entombed[0].ID)
	var exists library.ErrExists
	if !errors.As(err, &exists) {
		t.Fatalf("restoring onto a living deck answered %v", err)
	}
	if exists.Slug != "gyome" {
		t.Errorf("the refusal names %q", exists.Slug)
	}
	// And nothing moved: the living deck is untouched and the buried one is
	// still buried.
	still, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(still) != 1 {
		t.Errorf("the refused restore left %d decks in the crypt", len(still))
	}
}

// Two burials of one slug are two entries, and the id is what tells them
// apart -- which is the whole reason `Restore` takes one rather than a slug.
func TestTwoBurialsOfOneSlugStayTwoThings(t *testing.T) {
	t.Parallel()
	src, _ := writableTier(t, "gyome")
	ctx := context.Background()
	crypt := cryptOf(t, src)

	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	if err := src.Create(ctx, "gyome", strings.Replace(oneDeck, "Test Deck", "Second Try", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}

	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 2 {
		t.Fatalf("two deletions left %d entries", len(entombed))
	}
	if entombed[0].ID == entombed[1].ID {
		t.Fatal("both burials answer to the same handle")
	}
	// Newest first, so the one on top is the one somebody just lost.
	if entombed[0].Name != "Second Try" {
		t.Errorf("the crypt's first entry is %q, not the most recent", entombed[0].Name)
	}
	if _, err := crypt.Restore(ctx, entombed[0].ID); err != nil {
		t.Fatal(err)
	}
	d, err := src.Get(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Second Try" {
		t.Errorf("the handle raised %q", d.Name)
	}
}

// A handle nobody holds is absent, not an error about the shape of the
// handle: the route turns this into the 404 that says "it may already be back
// on your shelf".
func TestAHandleTheCryptDoesNotHoldIsNotFound(t *testing.T) {
	t.Parallel()
	src, _ := writableTier(t, "gyome")
	crypt := cryptOf(t, src)
	if _, err := crypt.Restore(context.Background(), "0123456789abcdef"); !library.IsNotFound(err) {
		t.Fatalf("an unknown handle answered %v", err)
	}
}

// A directory somebody put in the crypt by hand is still somebody's deck. It
// is listed rather than skipped -- a crypt that silently omits what it cannot
// parse is a crypt that tells you your deck is gone -- and its burial time is
// **zero rather than a guess**, which is the difference the route layer sends
// as null.
func TestAnUnstampedEntryIsListedWithNoDateRatherThanDropped(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t)
	crypt := cryptOf(t, src)

	dir := filepath.Join(root, ".trash", "handplaced")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"),
		[]byte("name: Hand Placed\nstatus: draft\ncards: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entombed, err := crypt.Entombed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 1 {
		t.Fatalf("the crypt dropped an entry it could not parse: %+v", entombed)
	}
	if !entombed[0].At.IsZero() {
		t.Errorf("an entry with no stamp was given the date %v", entombed[0].At)
	}
	if entombed[0].Name != "Hand Placed" {
		t.Errorf("the entry is named %q", entombed[0].Name)
	}
}

// **A crypt belongs to its owner and to nobody else.** A read-only tier and
// the shared-only keyhole both refuse rather than answering an empty list:
// "you have nothing buried" and "these are not yours" are different sentences,
// and an entombed deck was never shared with anybody in the first place.
func TestOnlyAnOwnerHasACrypt(t *testing.T) {
	t.Parallel()
	_, root := writableTier(t, "gyome")

	readOnly := library.NewFileSource(root, false)
	if _, err := library.CryptFor(readOnly); !library.IsReadOnly(err) {
		t.Errorf("a read-only tier handed out a crypt: %v", err)
	}
	if _, err := readOnly.Entombed(context.Background()); !library.IsReadOnly(err) {
		t.Errorf("a read-only tier listed its crypt: %v", err)
	}
	if _, err := readOnly.Restore(context.Background(), "whatever"); !library.IsReadOnly(err) {
		t.Errorf("a read-only tier raised a deck: %v", err)
	}
	// The keyhole view has no crypt at all -- not one that refuses, none.
	// Stronger than a refusal: there is no method to call.
	view := library.NewSharedOnly(library.NewFileSource(root, false))
	if _, err := library.CryptFor(view); !library.IsReadOnly(err) {
		t.Errorf("the shared-only view handed out a crypt: %v", err)
	}
}

// ---- the SQL tier ----------------------------------------------------------

// The same round trip over rows, where the burial is a `deleted_at` rather
// than a rename. Held separately because the two tiers fail differently: this
// one leans on the partial unique index to refuse a slug a living deck holds.
func TestTheSQLCryptRaisesTheRowItBuried(t *testing.T) {
	t.Parallel()
	src, db := ownedSQLTier(t)
	ctx := context.Background()
	crypt := cryptOf(t, src)

	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Get(ctx, "gyome"); !library.IsNotFound(err) {
		t.Fatal("the buried deck is still readable")
	}

	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 1 {
		t.Fatalf("the crypt holds %d rows", len(entombed))
	}
	if entombed[0].Slug != "gyome" || entombed[0].Name != "Gyome, Master Chef" {
		t.Errorf("the crypt describes it as %+v", entombed[0])
	}
	if entombed[0].At.IsZero() {
		t.Error("the row's own deleted_at did not reach the crypt")
	}

	slug, err := crypt.Restore(ctx, entombed[0].ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if slug != "gyome" {
		t.Errorf("the deck came back as %q", slug)
	}
	if _, err := src.Get(ctx, "gyome"); err != nil {
		t.Errorf("the raised deck is not readable: %v", err)
	}
	// The row was marked, not re-inserted: one row, ever.
	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_decks WHERE slug = 'gyome'").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("the restore left %d rows for one deck", rows)
	}
}

// The index is the guard, and it answers the same refusal the file tier does.
func TestTheSQLCryptWillNotRaiseOntoALivingSlug(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()
	crypt := cryptOf(t, src)

	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	entombed, err := crypt.Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var exists library.ErrExists
	if _, err := crypt.Restore(ctx, entombed[0].ID); !errors.As(err, &exists) {
		t.Fatalf("raising onto a living slug answered %v", err)
	}
	if exists.Slug != "gyome" {
		t.Errorf("the refusal names %q", exists.Slug)
	}
}

// One account's crypt never answers about another's. Structural at the route
// -- neither crypt route takes an owner segment -- and held here too, because
// the tier is what makes that true.
func TestACryptOnlyEverHoldsItsOwnersDecks(t *testing.T) {
	t.Parallel()
	mine, db := ownedSQLTier(t)
	ctx := context.Background()
	if err := mine.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.Delete(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	theirs := library.NewSQLSource(db, db, 2, true, false)
	entombed, err := cryptOf(t, theirs).Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entombed) != 0 {
		t.Fatalf("another account's crypt shows %+v", entombed)
	}
	// And the handle is no use across the boundary either: it names a row
	// that is not in this owner's crypt, so there is nothing to raise.
	ids, err := cryptOf(t, mine).Entombed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cryptOf(t, theirs).Restore(ctx, ids[0].ID); !library.IsNotFound(err) {
		t.Fatalf("one account raised another's deck: %v", err)
	}
	var deleted sql.NullString
	if err := db.QueryRow("SELECT deleted_at FROM user_decks WHERE slug = 'gyome'").Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted.Valid {
		t.Error("the deck was raised out of somebody else's crypt")
	}
}
