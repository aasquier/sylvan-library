package library_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// What the file tier does when the disk refuses the write.
//
// `writeAtomically` is six failure branches deep -- create the temporary,
// write it, sync it, close it, match the mode, rename -- and every one of them
// wraps the error with the path, because the operator reading it has to know
// *which* deck would not save. A working directory takes none of those
// branches, so they had never run.
//
// The fault is the deployed one: the library is on a volume, and a volume
// mounted read-only, a directory owned by another user, or a disk with nothing
// left on it all present the same way. What matters is that the refusal is a
// refusal -- **never a silent success**, because a caller told its edit landed
// has no reason to try again, and the deck it was told about is the one that
// did not change.

// readOnlyDeck is a writable library whose one deck's directory has been made
// unwritable underneath it -- the tier still believes it may write, and the
// filesystem disagrees. That is the shape a read-only volume has: the
// configuration says yes and the disk says no.
func readOnlyDeck(t *testing.T) (*library.FileSource, string, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable directory to build")
	}
	src, root := writableTier(t, "gyome")
	dir := filepath.Join(root, "gyome")
	if err := os.Chmod(dir, 0o500); err != nil { // readable, listable, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
	return src, root, dir
}

// Every write against a deck directory that will not take one.
func TestNoFileWriteReportsSuccessOnADirectoryThatWillNotTakeOne(t *testing.T) {
	t.Parallel()
	src, _, dir := readOnlyDeck(t)
	ctx := context.Background()

	before, err := os.ReadFile(filepath.Join(dir, "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		what string
		run  func() error
	}{
		{"WriteText", func() error {
			return src.WriteText(ctx, "gyome", "slug: gyome\nname: Changed\ncards: []\n")
		}},
		// **false, not true.** The fixture has no `shared:` key, which means
		// it IS shared, so `SetShared(…, true)` is the standing no-op --
		// "nothing is written when the flag already says what was asked for"
		// -- and returns nil without ever reaching the disk. Asking for the
		// other direction is what makes this a write.
		{"SetShared", func() error { return src.SetShared(ctx, "gyome", false) }},
		{"WriteArtifacts", func() error {
			_, e := src.WriteArtifacts(ctx, "gyome",
				artifacts.Files{{Name: "primer-quick.md", Text: "# a primer\n"}})
			return e
		}},
	} {
		err := tc.run()
		if err == nil {
			t.Errorf("%s reported success on a directory that will not take a "+
				"write -- the caller has no reason to try again", tc.what)
			continue
		}
		// The operator reading this has to know which deck would not save.
		if !strings.Contains(err.Error(), "gyome") {
			t.Errorf("%s refused with %q, which does not name the deck", tc.what, err)
		}
	}

	// The no-op direction stays a no-op even here: a share toggle that asks
	// for the state the deck is already in writes nothing, so an unwritable
	// directory is not its problem. Pinning it beside the refusals is what
	// keeps somebody from "fixing" the rule by making it always write.
	if err := src.SetShared(ctx, "gyome", true); err != nil {
		t.Errorf("asking for the state the deck is already in refused: %v", err)
	}

	// And the deck on disk is the one that was there: a refused write that
	// truncated the file would be worse than one that failed cleanly.
	after, err := os.ReadFile(filepath.Join(dir, "deck.yaml"))
	if err != nil {
		t.Fatalf("the deck went missing under a refused write: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("a refused write changed the file anyway:\nwas:  %q\nnow:  %q",
			before, after)
	}
}

// A create into a root that will not take a directory, and a delete out of
// one that will not take a rename. Both reach the disk from a different
// direction than `writeAtomically` does.
func TestCreateAndDeleteRefuseOnARootThatWillNotTakeThem(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	src, root := writableTier(t, "gyome")
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
	ctx := context.Background()

	if err := src.Create(ctx, "new-deck", "slug: new-deck\nname: New\ncards: []\n"); err == nil {
		t.Error("a deck was created in a root that will not take a directory")
	} else if !strings.Contains(err.Error(), "new-deck") {
		t.Errorf("the refusal %q does not name the deck", err)
	}

	if _, err := src.Delete(ctx, "gyome"); err == nil {
		t.Error("a deck was deleted out of a root that will not take a rename")
	}
	// The deck is still there, which is the half that matters: a delete
	// reported as failed must not have half-happened.
	if _, err := os.Stat(filepath.Join(root, "gyome", "deck.yaml")); err != nil {
		t.Errorf("a refused delete took the deck anyway: %v", err)
	}
}

// A deck file whose mode cannot be matched is still a refusal rather than a
// write with the wrong permissions: `writeAtomically` copies the existing
// file's mode onto the temporary before the rename, so a volume backup does
// not see the mode change on the first edit.
func TestTheWrittenFileKeepsTheModeItHad(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	path := filepath.Join(root, "gyome", "deck.yaml")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteText(context.Background(), "gyome",
		"slug: gyome\nname: Changed\ncards: []\n"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("the deck's mode became %v -- an edit that changes the mode is "+
			"the kind of surprise a volume backup notices", got)
	}
}
