package library_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The memo's contract, held from outside the package: what a visit to the
// shelf costs is one parse per file *that has moved*, the counters say which
// answers came from memory, and the memory belongs to the process rather
// than to the request -- so every one of these reads through a fresh
// FileSource or a fresh Library, the way a route does, over one Memo.

// memoDeck is a small deck file whose name is the one thing a test varies.
func memoDeck(slug, name string) string {
	return "slug: " + slug + "\nname: " + name + "\ncommander:\n  - Fixture Commander\n" +
		"cards:\n  - name: Fixture Card\n    why: 'Because.'\n"
}

func writeMemoDeck(t *testing.T, root, slug, name string) {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(memoDeck(slug, name)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// memoShelf is n decks under a fresh root.
func memoShelf(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := range n {
		writeMemoDeck(t, root, fmt.Sprintf("deck-%02d", i), fmt.Sprintf("Deck %02d", i))
	}
	return root
}

func wantCounts(t *testing.T, m *library.Memo, hits, misses int64) {
	t.Helper()
	h, mi := m.Counts()
	if h != hits || mi != misses {
		t.Fatalf("memo counts hits=%d misses=%d, want hits=%d misses=%d", h, mi, hits, misses)
	}
}

// shelfNames is every deck's name off a fresh file tier over root, through m.
func shelfNames(t *testing.T, root string, m *library.Memo) map[string]string {
	t.Helper()
	decks, err := library.NewFileSource(root, false).WithMemo(m).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, d := range decks {
		out[d.Slug] = d.Name
	}
	return out
}

func TestTheShelfIsParsedOnceWhileItsFilesStandStill(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 5)
	m := library.NewMemo()
	first := shelfNames(t, root, m)
	wantCounts(t, m, 0, 5)
	second := shelfNames(t, root, m)
	wantCounts(t, m, 5, 5)
	if len(first) != 5 || len(second) != 5 || first["deck-03"] != "Deck 03" || second["deck-03"] != "Deck 03" {
		t.Fatalf("shelves %v / %v", first, second)
	}
}

func TestARewrittenDeckIsReadAgainAndTheRestAreNot(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 5)
	m := library.NewMemo()
	shelfNames(t, root, m)
	// A real edit: different bytes, a different size, a new mtime.
	writeMemoDeck(t, root, "deck-02", "Deck 02, renamed")
	names := shelfNames(t, root, m)
	wantCounts(t, m, 4, 6)
	if names["deck-02"] != "Deck 02, renamed" {
		t.Fatalf("the rewritten deck read %q", names["deck-02"])
	}
	// A touch: the same bytes with the mtime moved. The stamp is the whole
	// key, so this is read again too -- a moved mtime is the one signal a
	// write leaves and nothing second-guesses it.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(root, "deck-03", "deck.yaml"), later, later); err != nil {
		t.Fatal(err)
	}
	shelfNames(t, root, m)
	wantCounts(t, m, 8, 7)
}

func TestGetAndAllShareOneMemory(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 4)
	m := library.NewMemo()
	ctx := context.Background()
	if _, err := library.NewFileSource(root, false).WithMemo(m).Get(ctx, "deck-01"); err != nil {
		t.Fatal(err)
	}
	wantCounts(t, m, 0, 1)
	shelfNames(t, root, m)
	wantCounts(t, m, 1, 4)
	d, err := library.NewFileSource(root, false).WithMemo(m).Get(ctx, "deck-03")
	if err != nil || d.Name != "Deck 03" {
		t.Fatalf("%v %v", d, err)
	}
	wantCounts(t, m, 2, 4)
}

// A deck that leaves the library -- its directory renamed into the crypt, as
// Delete does -- is forgotten at the next shelf visit, and forgotten for
// real: brought back with its stamp intact (a rename moves no mtime), it is
// read again rather than answered from a memory nobody pruned.
func TestADeckThatLeftTheLibraryIsForgotten(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 3)
	m := library.NewMemo()
	shelfNames(t, root, m)
	wantCounts(t, m, 0, 3)
	gone := filepath.Join(root, ".trash", "deck-01")
	if err := os.MkdirAll(filepath.Dir(gone), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "deck-01"), gone); err != nil {
		t.Fatal(err)
	}
	if names := shelfNames(t, root, m); len(names) != 2 {
		t.Fatalf("after the deletion the shelf reads %v", names)
	}
	wantCounts(t, m, 2, 3)
	if err := os.Rename(gone, filepath.Join(root, "deck-01")); err != nil {
		t.Fatal(err)
	}
	d, err := library.NewFileSource(root, false).WithMemo(m).Get(context.Background(), "deck-01")
	if err != nil || d.Name != "Deck 01" {
		t.Fatalf("%v %v", d, err)
	}
	wantCounts(t, m, 2, 4)
}

// The pool's stated hazard, pinned as the contract's edge rather than left
// to be discovered: a file replaced by different bytes of the same size with
// the mtime put back is the same stamp, and the same stamp is the same deck.
// Nothing in this app writes a deck that way (every write is a fresh file
// renamed into place), and this test is the sentence that says so.
func TestTheSameStampIsTheSameDeckByContract(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 2)
	m := library.NewMemo()
	path := filepath.Join(root, "deck-00", "deck.yaml")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	shelfNames(t, root, m)
	// "Deck 00" and "Deck XX" are the same length, and the mtime goes back.
	if err := os.WriteFile(path, []byte(memoDeck("deck-00", "Deck XX")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("the fixture did not reproduce the stamp: %v %v", after, err)
	}
	names := shelfNames(t, root, m)
	wantCounts(t, m, 2, 2)
	if names["deck-00"] != "Deck 00" {
		t.Fatalf("an identical stamp was read again: %q", names["deck-00"])
	}
}

func TestATierWithNoMemoParsesEveryTimeAndCountsNothing(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 3)
	var none *library.Memo
	for range 2 {
		if names := shelfNames(t, root, none); len(names) != 3 {
			t.Fatalf("%v", names)
		}
	}
	wantCounts(t, none, 0, 0)
	if d, err := library.NewFileSource(root, false).Get(context.Background(), "deck-02"); err != nil || d.Name != "Deck 02" {
		t.Fatalf("%v %v", d, err)
	}
}

// The live-data half of the guarantee: a write through the tier's own verbs
// moves the stamp, so the next read is fresh -- through a new FileSource, as
// the next request would be.
func TestAWriteThroughTheTierIsReadBackFresh(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 2)
	m := library.NewMemo()
	ctx := context.Background()
	if _, err := library.NewFileSource(root, true).WithMemo(m).Get(ctx, "deck-01"); err != nil {
		t.Fatal(err)
	}
	if err := library.NewFileSource(root, true).WithMemo(m).WriteText(ctx, "deck-01",
		memoDeck("deck-01", "Deck 01, edited")); err != nil {
		t.Fatal(err)
	}
	d, err := library.NewFileSource(root, true).WithMemo(m).Get(ctx, "deck-01")
	if err != nil || d.Name != "Deck 01, edited" || !d.Shared {
		t.Fatalf("%v %v", d, err)
	}
	wantCounts(t, m, 0, 2)
	if err := library.NewFileSource(root, true).WithMemo(m).SetShared(ctx, "deck-01", false); err != nil {
		t.Fatal(err)
	}
	d, err = library.NewFileSource(root, true).WithMemo(m).Get(ctx, "deck-01")
	if err != nil || d.Shared {
		t.Fatalf("the share toggle was answered from memory: %v %v", d, err)
	}
}

// The ownership argument, proven rather than described: a Library is built
// per request, and every file tier it hands out -- the caller's own, the
// showcase, the shared-only view of it -- reads through the resolver's memo.
func TestALibraryHandsItsMemoToEveryFileTierItBuilds(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 4)
	m := library.NewMemo()
	ctx := context.Background()
	visit := func(scope auth.Scope) {
		t.Helper()
		lib, err := library.Resolver{DecksDir: root, Memo: m}.For(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		owned, err := lib.Visible(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range owned {
			if decks, err := o.Source.All(ctx); err != nil || len(decks) != 4 {
				t.Fatalf("%s: %d %v", o.Owner, len(decks), err)
			}
		}
	}
	visit(auth.Local)
	wantCounts(t, m, 0, 4)
	visit(auth.Local)
	wantCounts(t, m, 4, 4)
	// A signed-in player who is not the maintainer sees the showcase
	// through the shared-only view -- the same files, the same memory.
	bob := auth.Scope{UserID: 2, Username: "bob", Authenticated: true}
	lib, err := library.Resolver{DecksDir: root, Memo: m}.For(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	showcase, err := lib.SourceFor(ctx, lib.FileOwner())
	if err != nil {
		t.Fatal(err)
	}
	if decks, err := showcase.All(ctx); err != nil || len(decks) != 4 || showcase.Writable() {
		t.Fatalf("%d %v writable=%v", len(decks), err, showcase.Writable())
	}
	wantCounts(t, m, 8, 4)
}

// Many requests at once over one memo, with an edit landing in the middle
// through the tier's own write -- a fresh file renamed into place, so no
// reader can ever see half a deck. `-race` is the assertion on the memo's
// locking; the rest holds the accounting: every lookup is a hit or a miss,
// every file was missed at least once, and once the visitors have gone the
// memory converges within one quiet visit -- the visit after that is all
// hits and says the new name. **One, not zero**: the last visitor's read may
// have raced the write, learning the new bytes under the stamp it saw before
// the rename, and that entry is unreachable rather than wrong (`Memo.learned`
// argues it) -- so the first quiet visit may miss once more, and CI's arm64
// leg is where that window first opened. (Several visitors missing the same
// file at once each parse it; nothing single-flights a miss, because a parse
// is 1.6 ms and a stampede is bounded by the number of people at the door.)
func TestTheMemoIsSafeUnderConcurrentVisits(t *testing.T) {
	t.Parallel()
	root := memoShelf(t, 6)
	m := library.NewMemo()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 5 {
				if i == 0 && j == 2 {
					if err := library.NewFileSource(root, true).WithMemo(m).WriteText(ctx, "deck-03",
						memoDeck("deck-03", "Deck 03, edited")); err != nil {
						t.Error(err)
						return
					}
				}
				decks, err := library.NewFileSource(root, false).WithMemo(m).All(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				for _, d := range decks {
					if d.Name == "" || len(d.Cards) != 1 || len(d.Commander) != 1 {
						t.Errorf("a partial deck was handed out: %+v", d)
					}
				}
			}
		}()
	}
	wg.Wait()
	hits, misses := m.Counts()
	if hits+misses != 8*5*6 || misses < 6 {
		t.Fatalf("hits=%d misses=%d over 240 lookups", hits, misses)
	}
	// The quiet visit: at most one miss, for a last read that raced the write.
	names := shelfNames(t, root, m)
	if h, mi := m.Counts(); h+mi != hits+misses+6 || mi > misses+1 {
		t.Fatalf("the first quiet visit read hits=%d misses=%d after hits=%d misses=%d", h, mi, hits, misses)
	}
	if names["deck-03"] != "Deck 03, edited" {
		t.Fatalf("after the visitors left, deck-03 reads %q", names["deck-03"])
	}
	// And the one after it is all hits: the memory has converged.
	hits, misses = m.Counts()
	shelfNames(t, root, m)
	wantCounts(t, m, hits+6, misses)
}
