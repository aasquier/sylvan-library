package library_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The file tier's remaining refusals, and the night gate's.
//
// Three families sit here. **The night games** (ADR 46): the flag is kept in
// one place only -- rung 13's column -- so the file tier has nowhere to put
// it and says so, and the refusal is deliberately not "not yours to change",
// because the caller may well own this deck and be able to change everything
// else about it. **A volume that has gone read-only underneath a library
// that still believes it may write**, which is the deployed fault and the
// reason the trash-and-rename dance exists at all. And **a deck file that
// stopped being a deck**, which a hand edit on the volume produces and which
// no write path of ours can.
//
// What is asked of all of them is the same: say so. A refusal a caller can
// act on beats a green answer about a deck that did not change.

// The file tier has no room for the night flag, and the sentence it answers
// with is about the deck rather than about the caller.
func TestTheShowcaseDecksKeepToDaylight(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()

	err := src.SetColiseumAtNight(ctx, "gyome", true)
	if !library.IsNoNightGames(err) {
		t.Fatalf("the file tier answered %v", err)
	}
	// **Not read-only.** "Not yours to change" would be false: this caller
	// owns the deck and may edit every other thing about it.
	if library.IsReadOnly(err) {
		t.Error("the night refusal reads as a permission refusal, which is a " +
			"different and untrue sentence")
	}

	// A deck that is not there is a 404 like every other verb, rather than a
	// strange sentence about a gate for a deck nobody has.
	if err := src.SetColiseumAtNight(ctx, "no-such-deck", true); !library.IsNotFound(err) {
		t.Errorf("an absent deck answered %v", err)
	}

	// And writability comes first, so the answer does not depend on whether
	// the deck is there -- the tier's standing order, the same one
	// `WriteArtifacts` keeps.
	readOnly := library.NewFileSource(root, false)
	for _, slug := range []string{"gyome", "no-such-deck"} {
		if err := readOnly.SetColiseumAtNight(ctx, slug, true); !library.IsReadOnly(err) {
			t.Errorf("a read-only tier answered %v for %q", err, slug)
		}
	}
}

// Entering somebody else's deck for the night games is refused as theirs,
// not as the tier's -- whose deck this is comes first, whatever is underneath.
func TestEnteringSomebodyElsesDeckForTheNightIsRefusedAsTheirs(t *testing.T) {
	t.Parallel()
	inner, _ := writableTier(t, "shared")
	ctx := context.Background()
	if err := inner.Create(ctx, "private",
		strings.Replace(privateDeck, "slug: test", "slug: private", 1)); err != nil {
		t.Fatal(err)
	}
	view := library.NewSharedOnly(inner)

	if err := view.SetColiseumAtNight(ctx, "shared", true); !library.IsReadOnly(err) {
		t.Errorf("a shared deck answered %v", err)
	}
	// A deck this view cannot see is absent, so the question never gets as
	// far as the gate -- ADR 5, at the source rather than at the route.
	if err := view.SetColiseumAtNight(ctx, "private", true); !library.IsNotFound(err) {
		t.Errorf("a private deck answered %v -- that is an oracle for its existence", err)
	}
}

// The keyhole reports a fault when the library behind it cannot be read, in
// the same words the library used. An empty shelf would read as "nobody has
// shared anything with you", which is not what happened.
func TestTheKeyholeReportsALibraryItCannotReadRatherThanAnEmptyShelf(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everywhere, so there is no unreadable library to build")
	}
	_, root := writableTier(t, "shared")
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })

	view := library.NewSharedOnly(library.NewFileSource(root, false))
	ctx := context.Background()
	if slugs, err := view.Slugs(ctx); err == nil {
		t.Errorf("the keyhole listed %v over a library it cannot read", slugs)
	}
	if decks, err := view.All(ctx); err == nil {
		t.Errorf("the keyhole listed %d decks over a library it cannot read", len(decks))
	}
}

// A deck file the tier can see and cannot read is a fault, never an absence:
// `Slugs` finds it, so answering "no such deck" would be the library
// contradicting itself.
func TestADeckFileThatWillNotOpenIsAFaultRatherThanAnAbsence(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everywhere, so there is no unreadable deck to build")
	}
	src, root := writableTier(t, "gyome")
	path := filepath.Join(root, "gyome", "deck.yaml")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o640) })
	ctx := context.Background()

	slugs, err := src.Slugs(ctx)
	if err != nil || len(slugs) != 1 {
		t.Fatalf("the shelf lists %v (%v) -- this test rests on the deck being visible", slugs, err)
	}
	if text, err := src.ReadText(ctx, "gyome"); err == nil {
		t.Errorf("a deck that will not open read back as %q", text)
	} else if library.IsNotFound(err) {
		t.Errorf("a deck the shelf lists was reported missing: %v", err)
	}
	if decks, err := src.All(ctx); err == nil {
		t.Errorf("All returned %d decks over one that will not open", len(decks))
	}
}

// A deck file that stopped parsing refuses the share toggle rather than
// rewriting itself from a parse that did not happen. Only a hand edit on the
// volume gets a library into this state, which is exactly why the write path
// has to survive finding one.
func TestTheShareToggleRefusesADeckFileThatStoppedBeingADeck(t *testing.T) {
	t.Parallel()
	_, root := writableTier(t)
	dir := filepath.Join(root, "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const wreckage = "slug: broken\ncards: [\n"
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(wreckage), 0o644); err != nil {
		t.Fatal(err)
	}

	src := library.NewFileSource(root, true)
	err := src.SetShared(context.Background(), "broken", false)
	if err == nil {
		t.Fatal("the share toggle reported success over a file it could not read")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("the refusal is %q and does not name the deck", err)
	}
	raw, readErr := os.ReadFile(filepath.Join(dir, "deck.yaml")) //nolint:gosec // the test's own temp dir
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != wreckage {
		t.Errorf("a refused toggle rewrote the file:\n%s", raw)
	}
}

// A create into a directory the disk will not take one in is a refusal, and
// **not** the "already exists" refusal -- which would send the caller off to
// pick another slug over a library that would not have taken that one either.
func TestACreateIntoADirectoryThatWillNotTakeOneIsNotMistakenForACollision(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable directory to build")
	}
	src, root := writableTier(t)
	// A directory where the deck would go, with no deck in it and no room
	// to put one: the shape a half-finished restore leaves behind.
	dir := filepath.Join(root, "ghost")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	err := src.Create(context.Background(), "ghost", oneDeck)
	if err == nil {
		t.Fatal("a create reported success into a directory that will not take one")
	}
	if strings.Contains(err.Error(), "already exists") {
		t.Errorf("the refusal is %q, which sends the caller off to pick "+
			"another slug over a fault that has nothing to do with the name", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("the refusal is %q and does not name the deck", err)
	}
}

// A delete that cannot move the deck into the crypt refuses rather than
// reporting a burial that did not happen. The deck is still on the shelf
// afterwards, which is the half a caller would otherwise never find out.
func TestADeleteThatCannotReachTheCryptLeavesTheDeckWhereItIs(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable library to build")
	}
	src, root := writableTier(t, "gyome")
	// The crypt already exists, so the refusal comes from the move itself
	// rather than from making the directory.
	if err := os.MkdirAll(filepath.Join(root, ".trash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o500); err != nil { // readable, listable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })

	ctx := context.Background()
	where, err := src.Delete(ctx, "gyome")
	if err == nil {
		t.Fatalf("a delete reported the deck buried at %q over a library that "+
			"will not take the move", where)
	}
	if !strings.Contains(err.Error(), "gyome") {
		t.Errorf("the refusal is %q and does not name the deck", err)
	}
	if _, err := src.Get(ctx, "gyome"); err != nil {
		t.Errorf("the deck went missing after a refused delete: %v", err)
	}
}

// A machine with no accounts database answers "no such person" for every
// owner but its own, rather than reaching for a handle it has not got.
//
// This is the laptop's shape with auth on and nothing behind it, and the rule
// it keeps is ADR 5's: one answer for "no such person" and "nothing of theirs
// for you", or the owner segment would enumerate the account list.
func TestALibraryWithNoAccountsAnswersNoSuchPersonRatherThanReachingForOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, err := library.Resolver{DecksDir: t.TempDir()}.For(ctx,
		auth.Scope{Authenticated: true, Username: "alice", UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SourceFor(ctx, "bob"); !library.IsNotFound(err) {
		t.Errorf("an owner segment over no accounts database answered %v", err)
	}
}
