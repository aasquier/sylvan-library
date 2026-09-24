package library_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/library"
)

// What the file tier's crypt does when `.trash` is not what it expects.
//
// The crypt is the only thing standing between a misclick and somebody's
// deck: since ADR 30 nothing is in git, so a delete that destroyed the
// directory would take the draft imported ten minutes ago and the curated
// deck of a year's thinking equally. That makes two of its rules load-bearing
// and both of them are about **not** being clever.
//
// **"I cannot read the crypt" is not "your crypt is empty."** An absent
// `.trash` is the normal state of a library nobody has deleted anything from
// and reads as empty on purpose; every other failure is reported, because a
// player looking at an empty crypt concludes their deck is gone for good.
//
// **A name that is not a name is refused before it becomes a path.** The
// crypt is a directory on a volume, and `Empty` deletes a tree per entry --
// so a folder name that walks out of `.trash` is refused rather than
// followed. `Restore` applies the same guard for the same reason one step
// earlier.
//
// And an entry that will not parse is still listed. It is somebody's deck,
// and a crypt that silently omits what it cannot read is a crypt that tells
// you your deck is gone.

// aCrypt is a writable library with the named folders already sitting in
// `.trash`, written directly rather than through `Delete` -- these are the
// shapes a hand on the volume leaves, which is the whole point.
func aCrypt(t *testing.T, folders ...string) (*library.FileSource, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range folders {
		if err := os.MkdirAll(filepath.Join(root, ".trash", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return library.NewFileSource(root, true), root
}

// A crypt that cannot be read is reported, never reported as empty.
func TestACryptThatCannotBeReadIsNotReportedAsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := library.NewFileSource(root, true)
	ctx := context.Background()

	// An absent crypt IS empty, and that is the normal state.
	buried, err := src.Entombed(ctx)
	if err != nil || len(buried) != 0 {
		t.Fatalf("a library nobody has deleted from answered %v (%v)", buried, err)
	}

	// `.trash` as a file rather than a directory: something else wrote here,
	// or a restore was interrupted. Not an empty crypt.
	if err := os.WriteFile(filepath.Join(root, ".trash"), []byte("not a crypt"), 0o644); err != nil {
		t.Fatal(err)
	}
	buried, err = src.Entombed(ctx)
	if err == nil {
		t.Fatalf("a crypt that cannot be read answered %v", buried)
	}
	if !strings.Contains(err.Error(), "crypt") {
		t.Errorf("the refusal is %q and does not say what could not be read", err)
	}
	// Every verb agrees, including the destructive one: a crypt that cannot
	// be listed must not be emptiable either.
	if _, err := src.Restore(ctx, "whatever"); err == nil {
		t.Error("a restore ran over a crypt that cannot be read")
	}
	if gone, err := src.Empty(ctx); err == nil {
		t.Errorf("an emptying destroyed %d entries over a crypt that cannot be read", gone)
	}
}

// A buried deck whose file is missing or unreadable is still listed, under
// the only name the crypt has for it: its folder.
func TestABuriedDeckThatWillNotParseIsStillListed(t *testing.T) {
	t.Parallel()
	src, root := aCrypt(t, "no-file-20260101T000000Z", "wreckage-20260102T000000Z")
	if err := os.WriteFile(
		filepath.Join(root, ".trash", "wreckage-20260102T000000Z", "deck.yaml"),
		[]byte("cards: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	buried, err := src.Entombed(context.Background())
	if err != nil {
		t.Fatalf("listing the crypt: %v", err)
	}
	if len(buried) != 2 {
		t.Fatalf("the crypt lists %d entries: %v", len(buried), buried)
	}
	byName := map[string]library.Entombed{}
	for _, e := range buried {
		byName[e.Slug] = e
	}
	for _, slug := range []string{"no-file", "wreckage"} {
		e, ok := byName[slug]
		if !ok {
			t.Errorf("%q fell out of the crypt: %v", slug, buried)
			continue
		}
		// Nothing was invented about a deck nobody could read: the name is
		// the slug and the count is nothing.
		if e.Name != slug || e.Cards != 0 || len(e.Commander) != 0 {
			t.Errorf("%q came back as %+v -- something was invented about a "+
				"deck that could not be read", slug, e)
		}
		if e.ID == "" {
			t.Errorf("%q has no handle, so it cannot be restored", slug)
		}
	}
}

// A folder name that is not a name a deck can live under is refused, in both
// directions: the restore that would write it back out, and the emptying that
// would delete a tree under it.
func TestACryptFolderThatIsNotANameIsRefusedRatherThanFollowed(t *testing.T) {
	t.Parallel()
	// `...` is a directory a hand can make and a slug nothing may become:
	// stripped of its dots there is nothing left, which is the same guard
	// `path` applies to every slug that arrives from outside. It carries no
	// burial stamp either, so the crypt falls back to the folder's own name
	// for both the slug it would restore to and the tree it would delete --
	// which is what puts the guard on both paths at once.
	src, _ := aCrypt(t, "...")
	ctx := context.Background()

	buried, err := src.Entombed(ctx)
	if err != nil || len(buried) != 1 {
		t.Fatalf("the crypt lists %v (%v)", buried, err)
	}
	if _, err := src.Restore(ctx, buried[0].ID); err == nil {
		t.Error("a deck was restored to a name it cannot live under")
	} else if !strings.Contains(err.Error(), "...") {
		t.Errorf("the refusal is %q and does not quote the name it refused", err)
	}

	gone, err := src.Empty(ctx)
	if err == nil {
		t.Errorf("the crypt deleted %d trees under a name it cannot hold", gone)
	}
	if gone != 0 {
		t.Errorf("%d entries were destroyed before the refusal", gone)
	}
}

// A restore the volume will not take refuses rather than reporting a deck
// raised, and the deck stays in the crypt where the player can find it.
func TestARestoreTheVolumeWillNotTakeLeavesTheDeckInTheCrypt(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable library to build")
	}
	src, root := aCrypt(t, "gyome-20260101T000000Z")
	ctx := context.Background()
	buried, err := src.Entombed(ctx)
	if err != nil || len(buried) != 1 {
		t.Fatalf("the crypt lists %v (%v)", buried, err)
	}
	if err := os.Chmod(root, 0o500); err != nil { // readable, listable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })

	slug, err := src.Restore(ctx, buried[0].ID)
	if err == nil {
		t.Fatalf("a deck was reported raised as %q over a library that will not take it", slug)
	}
	if !strings.Contains(err.Error(), "gyome") {
		t.Errorf("the refusal is %q and does not name the deck", err)
	}
	still, err := src.Entombed(ctx)
	if err != nil || len(still) != 1 {
		t.Errorf("after a refused restore the crypt holds %v (%v)", still, err)
	}
}

// An emptying the volume will not take stops at the entry that refused and
// says so, rather than carrying on and returning a count nobody can trust.
func TestAnEmptyingStopsAtTheEntryThatRefused(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable crypt to build")
	}
	src, root := aCrypt(t, "gyome-20260101T000000Z")
	trash := filepath.Join(root, ".trash")
	if err := os.Chmod(trash, 0o500); err != nil { // listable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(trash, 0o750) })

	gone, err := src.Empty(context.Background())
	if err == nil {
		t.Fatalf("the crypt reported %d entries destroyed over a directory "+
			"that will not take a removal", gone)
	}
	if gone != 0 {
		t.Errorf("the count says %d went while the removal refused", gone)
	}
	if !strings.Contains(err.Error(), "crypt") {
		t.Errorf("the refusal is %q and does not say what it was doing", err)
	}
}
