package library_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The write half of the file tier, and the shared-only view over it.
//
// Two things are being held here, and the second is the important one.
//
// The first is that the writes do what their comments claim: a create refuses
// to overwrite rather than landing one person's deck on top of another's, a
// delete moves the directory whole into `.trash` (since ADR 30 there is no
// revision to restore from, so a mistake has to stay survivable), and a share
// toggle is a surgical edit rather than a round trip -- it rewrote whole files
// until 2026-08-22 and took hand-written banners and comments with it.
//
// The second is **ADR 5's isolation, at the source rather than at the route**.
// `SharedOnly` is somebody else's library seen through a keyhole, and the rule
// it enforces is subtle: a deck the caller cannot see is *absent* (404), while
// a deck they can see but may not change is *refused* (403) -- and the 403 is
// safe precisely because the deck has already been shown. Get that backwards
// and a 403 becomes a way to ask whether a private deck exists.

// A deck's `shared:` key is absent when it IS shared -- `true` removes the
// key rather than asserting the default -- so a private fixture has to say
// so explicitly.
const oneDeck = "slug: test\nname: Test Deck\nstatus: draft\ncards: []\n"

const privateDeck = "slug: test\nname: Test Deck\nstatus: draft\nshared: false\ncards: []\n"

// writableTier is a writable file library with the named decks in it.
func writableTier(t *testing.T, slugs ...string) (*library.FileSource, string) {
	t.Helper()
	root := t.TempDir()
	src := library.NewFileSource(root, true)
	for _, slug := range slugs {
		text := strings.Replace(oneDeck, "slug: test", "slug: "+slug, 1)
		if err := src.Create(context.Background(), slug, text); err != nil {
			t.Fatalf("seeding %s: %v", slug, err)
		}
	}
	return src, root
}

// A create refuses to overwrite, and the refusal names the slug so the caller
// can choose another.
func TestACreateRefusesToLandOnTopOfAnExistingDeck(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()

	err := src.Create(ctx, "gyome", "slug: gyome\nname: Something Else\n")
	if err == nil {
		t.Fatal("a second create overwrote the first")
	}
	var exists library.ErrExists
	if !errors.As(err, &exists) {
		t.Fatalf("the refusal is %T, want ErrExists", err)
	}
	if !strings.Contains(err.Error(), "gyome") {
		t.Errorf("the refusal does not name the slug: %q", err)
	}

	// The original is untouched, which is the whole point of the O_EXCL.
	raw, readErr := os.ReadFile(filepath.Join(root, "gyome", "deck.yaml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(raw), "Test Deck") {
		t.Errorf("the existing deck was overwritten:\n%s", raw)
	}
}

// The same guard `path` applies has to be applied on the way in, because a
// create would otherwise happily make a deck outside the root.
func TestACreateCannotBeWalkedOutOfTheRoot(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t)
	ctx := context.Background()

	for _, slug := range []string{"", ".", "..", "...", "../escape", "a/b", `a\b`} {
		if err := src.Create(ctx, slug, oneDeck); err == nil {
			t.Errorf("%q was created", slug)
		}
	}
	// Nothing landed anywhere near the root's parent.
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); err == nil {
		t.Fatal("a create escaped the root")
	}
	slugs, err := src.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("the library holds %v", slugs)
	}
}

// A delete moves the directory whole into `.trash` and says where it went.
// Artifacts travel with it: a folder of primers for a deck that no longer
// exists is worse than no folder at all.
func TestADeleteMovesTheWholeDirectoryIntoTheTrash(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()

	art := filepath.Join(root, "gyome", "artifacts")
	if err := os.MkdirAll(art, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(art, "primer.md"), []byte("# Primer"), 0o600); err != nil {
		t.Fatal(err)
	}

	where, err := src.Delete(ctx, "gyome")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(where, ".trash") {
		t.Errorf("the deck went to %q", where)
	}
	if _, err := os.Stat(filepath.Join(root, "gyome")); err == nil {
		t.Error("the deck is still in the library")
	}
	// The artifacts went with it rather than being stranded.
	if _, err := os.Stat(filepath.Join(where, "artifacts", "primer.md")); err != nil {
		t.Errorf("the artifacts were stranded: %v", err)
	}
	// And `.trash` is invisible to the library itself.
	slugs, err := src.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("the trashed deck is still listed: %v", slugs)
	}

	if _, err := src.Delete(ctx, "gyome"); !library.IsNotFound(err) {
		t.Errorf("deleting it twice answered %v", err)
	}
}

// Import, delete, re-import, delete again inside one second is a real
// sequence and the stamp is only second-resolution. A collision must not bury
// the earlier deletion -- `.trash` exists to make a mistake survivable.
func TestTwoDeletesInOneSecondBothSurvive(t *testing.T) {
	t.Parallel()
	src, _ := writableTier(t, "gyome")
	ctx := context.Background()

	first, err := src.Delete(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Create(ctx, "gyome", oneDeck); err != nil {
		t.Fatal(err)
	}
	second, err := src.Delete(ctx, "gyome")
	if err != nil {
		t.Fatalf("the second delete failed: %v", err)
	}
	if first == second {
		t.Fatal("the second deletion buried the first")
	}
	for _, where := range []string{first, second} {
		if _, err := os.Stat(where); err != nil {
			t.Errorf("%s is gone: %v", where, err)
		}
	}
}

// The share toggle is a surgical edit (ADR 12): a hand-written file keeps its
// banners, its comments and its folded blocks. It rewrote whole files until
// 2026-08-22, and one press of the deck page's toggle took all of that with it.
func TestTheShareToggleIsSurgicalAndLeavesTheHandWritingAlone(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t)
	ctx := context.Background()

	handWritten := "# ---- The Deck ----------------------------------------\n" +
		"slug: gyome\n" +
		"name: Gyome, Master Chef # the one that matters\n" +
		"status: draft\n" +
		"\n" +
		"# The ninety-nine.\n" +
		"cards: []\n"
	if err := src.Create(ctx, "gyome", handWritten); err != nil {
		t.Fatal(err)
	}

	// Toggling OFF is the direction that writes: `true` removes the key
	// rather than asserting the default, so sharing an already-shared deck
	// would change nothing and prove nothing.
	if err := src.SetShared(ctx, "gyome", false); err != nil {
		t.Fatalf("unsharing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "gyome", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	after := string(raw)
	if !strings.Contains(after, "shared:") {
		t.Fatalf("the toggle wrote nothing:\n%s", after)
	}
	for _, keep := range []string{
		"# ---- The Deck",
		"# the one that matters",
		"# The ninety-nine.",
	} {
		if !strings.Contains(after, keep) {
			t.Errorf("the edit took %q with it:\n%s", keep, after)
		}
	}

	d, err := src.Get(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if d.Shared {
		t.Error("the deck is still shared after being unshared")
	}

	// And back on again, which takes the key back out -- the hand-writing
	// still surviving both passes.
	if err := src.SetShared(ctx, "gyome", true); err != nil {
		t.Fatalf("re-sharing: %v", err)
	}
	raw, err = os.ReadFile(filepath.Join(root, "gyome", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "shared:") {
		t.Errorf("sharing asserted the default instead of removing the key:\n%s", raw)
	}
	for _, keep := range []string{"# ---- The Deck", "# the one that matters", "# The ninety-nine."} {
		if !strings.Contains(string(raw), keep) {
			t.Errorf("the round trip took %q with it:\n%s", keep, raw)
		}
	}
}

// Two standing rules: nothing is written when the flag already says what was
// asked for, and the round trip returns the deck to where it started.
func TestAskingForTheStateADeckIsAlreadyInWritesNothing(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()
	path := filepath.Join(root, "gyome", "deck.yaml")

	// Both directions, from the state that direction would be a no-op in.
	for _, shared := range []bool{true, false} {
		if err := src.SetShared(ctx, "gyome", shared); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := src.SetShared(ctx, "gyome", shared); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Errorf("asking for shared=%v twice rewrote the file:\n%s\n%s",
				shared, before, after)
		}
		d, err := src.Get(ctx, "gyome")
		if err != nil {
			t.Fatal(err)
		}
		if d.Shared != shared {
			t.Errorf("asked for shared=%v, the deck says %v", shared, d.Shared)
		}
	}
}

// Every write refuses a deck that is not there, rather than creating one --
// `WriteText` in particular, because a write to a missing slug is the shape
// that would quietly scaffold a deck nobody asked for.
func TestEveryWriteRefusesADeckThatIsNotThere(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t)
	ctx := context.Background()

	if err := src.WriteText(ctx, "ghost", oneDeck); !library.IsNotFound(err) {
		t.Errorf("WriteText answered %v", err)
	}
	if _, err := src.Delete(ctx, "ghost"); !library.IsNotFound(err) {
		t.Errorf("Delete answered %v", err)
	}
	if err := src.SetShared(ctx, "ghost", true); !library.IsNotFound(err) {
		t.Errorf("SetShared answered %v", err)
	}
	if _, err := src.WriteArtifacts(ctx, "ghost", artifacts.Files{{Name: "primer-quick.md", Text: "x"}}); !library.IsNotFound(err) {
		t.Errorf("WriteArtifacts answered %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ghost")); err == nil {
		t.Fatal("a refused write scaffolded the deck anyway")
	}
}

// A read-only file tier refuses every verb in the same words, so the route
// layer has one path rather than two.
func TestAReadOnlyFileTierRefusesEveryVerbTheSameWay(t *testing.T) {
	t.Parallel()
	writable, root := writableTier(t, "gyome")
	_ = writable
	readOnly := library.NewFileSource(root, false)
	ctx := context.Background()

	// It still reads.
	if d, err := readOnly.Get(ctx, "gyome"); err != nil || d == nil {
		t.Fatalf("a read-only tier cannot read: %v", err)
	}

	for name, err := range map[string]error{
		"WriteText": readOnly.WriteText(ctx, "gyome", oneDeck),
		"Create":    readOnly.Create(ctx, "new", oneDeck),
		"SetShared": readOnly.SetShared(ctx, "gyome", true),
	} {
		if !library.IsReadOnly(err) {
			t.Errorf("%s answered %v, want ErrReadOnly", name, err)
		}
	}
	if _, err := readOnly.Delete(ctx, "gyome"); !library.IsReadOnly(err) {
		t.Errorf("Delete answered %v", err)
	}
	if _, err := readOnly.WriteArtifacts(ctx, "gyome", artifacts.Files{}); !library.IsReadOnly(err) {
		t.Errorf("WriteArtifacts answered %v", err)
	}

	// And `WriterFor` refuses before a handler ever holds the write half.
	if _, err := library.WriterFor(readOnly, "gyome"); !library.IsReadOnly(err) {
		t.Errorf("WriterFor handed out a writer: %v", err)
	}
	if _, err := library.WriterFor(library.NewFileSource(root, true), "gyome"); err != nil {
		t.Errorf("WriterFor refused a writable tier: %v", err)
	}
}

// **ADR 5 at the source.** A deck the caller cannot see is absent from this
// view entirely, and every verb against it is ErrNotFound -- never a 403,
// which would answer the question of whether it exists.
func TestTheSharedOnlyViewMakesAPrivateDeckAbsent(t *testing.T) {
	t.Parallel()
	inner, _ := writableTier(t)
	ctx := context.Background()
	if err := inner.Create(ctx, "private",
		strings.Replace(privateDeck, "slug: test", "slug: private", 1)); err != nil {
		t.Fatal(err)
	}
	if err := inner.Create(ctx, "shared",
		strings.Replace(oneDeck, "slug: test", "slug: shared", 1)); err != nil {
		t.Fatal(err)
	}
	view := library.NewSharedOnly(inner)

	// Reads.
	if _, err := view.Get(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("Get answered %v -- a private deck must be absent", err)
	}
	if _, err := view.ReadText(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("ReadText answered %v", err)
	}
	if _, err := view.Artifacts(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("Artifacts answered %v", err)
	}
	if _, err := view.ReadArtifact(ctx, "private", "primer.md"); !library.IsNotFound(err) {
		t.Errorf("ReadArtifact answered %v", err)
	}
	if _, _, err := view.ReadBaseline(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("ReadBaseline answered %v", err)
	}

	// Writes. **Not read-only** -- absent, because a 403 here would be the
	// leak this whole arrangement exists to prevent.
	if err := view.WriteText(ctx, "private", oneDeck); !library.IsNotFound(err) {
		t.Errorf("WriteText answered %v -- a 403 would confirm the deck exists", err)
	}
	if err := view.SetShared(ctx, "private", true); !library.IsNotFound(err) {
		t.Errorf("SetShared answered %v", err)
	}
	if _, err := view.Delete(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("Delete answered %v", err)
	}
	if _, err := view.WriteArtifacts(ctx, "private", artifacts.Files{}); !library.IsNotFound(err) {
		t.Errorf("WriteArtifacts answered %v", err)
	}

	// And it is absent from the listings too, which is where a leak would be
	// loudest.
	slugs, err := view.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "shared" {
		t.Errorf("the view lists %v", slugs)
	}
	decks, err := view.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(decks) != 1 || decks[0].Slug != "shared" {
		t.Errorf("All listed %d decks", len(decks))
	}
}

// A deck somebody shared with you is a deck you may read -- and the refusal
// on writing it is ErrReadOnly, which is right *because the caller has
// already been shown the deck*.
func TestASharedDeckReadsAndRefusesWritesAsReadOnly(t *testing.T) {
	t.Parallel()
	inner, root := writableTier(t, "shared")
	ctx := context.Background()
	view := library.NewSharedOnly(inner)

	d, err := view.Get(ctx, "shared")
	if err != nil || d == nil {
		t.Fatalf("a shared deck did not read: %v", err)
	}
	if _, err := view.ReadText(ctx, "shared"); err != nil {
		t.Errorf("ReadText: %v", err)
	}
	// The deliverables are the shareable surface, so a reader may have them.
	if _, err := view.Artifacts(ctx, "shared"); err != nil {
		t.Errorf("Artifacts: %v", err)
	}

	for name, err := range map[string]error{
		"WriteText": view.WriteText(ctx, "shared", oneDeck),
		"SetShared": view.SetShared(ctx, "shared", false),
	} {
		if !library.IsReadOnly(err) {
			t.Errorf("%s answered %v, want ErrReadOnly", name, err)
		}
	}
	if _, err := view.Delete(ctx, "shared"); !library.IsReadOnly(err) {
		t.Errorf("Delete answered %v", err)
	}
	// Rebuilding the deliverables is a write to somebody else's library.
	if _, err := view.WriteArtifacts(ctx, "shared", artifacts.Files{}); !library.IsReadOnly(err) {
		t.Errorf("WriteArtifacts answered %v", err)
	}

	// Nothing was written while it was refusing.
	raw, err := os.ReadFile(filepath.Join(root, "shared", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Test Deck") {
		t.Errorf("a refused write landed anyway:\n%s", raw)
	}

	if view.Writable() {
		t.Error("the shared-only view reports itself writable")
	}
}

// Create refuses without asking what is there. This view is somebody else's
// library seen through a keyhole, and a slug it does not show is one it must
// not be able to test for -- so unlike the others it does not look first.
func TestTheSharedOnlyViewsCreateDoesNotLookFirst(t *testing.T) {
	t.Parallel()
	inner, _ := writableTier(t)
	ctx := context.Background()
	if err := inner.Create(ctx, "private",
		strings.Replace(privateDeck, "slug: test", "slug: private", 1)); err != nil {
		t.Fatal(err)
	}
	view := library.NewSharedOnly(inner)

	// Both a slug that exists privately and one that does not answer the
	// SAME refusal, which is what makes create useless as an oracle.
	taken := view.Create(ctx, "private", oneDeck)
	free := view.Create(ctx, "definitely-not-there", oneDeck)
	if !library.IsReadOnly(taken) || !library.IsReadOnly(free) {
		t.Fatalf("create answered %v and %v", taken, free)
	}
	if taken.Error() != free.Error() {
		t.Errorf("create tells the two apart:\n  %q\n  %q -- that is an "+
			"oracle for a private deck's existence", taken, free)
	}
}

// The refusals' sentences are assembled in one place, so every route answers
// a 403 in the same words. They said the wrong words until 2026-08-22, and a
// shape test could not see it -- a shape records `{"detail": "string"}` and a
// person reads the string.
func TestTheRefusalsSaySomethingAPersonCanRead(t *testing.T) {
	t.Parallel()
	named := library.ErrReadOnly{Slug: "gyome"}.Error()
	if !strings.Contains(named, "gyome") || !strings.Contains(named, "not yours to change") {
		t.Errorf("the named refusal is %q", named)
	}
	// A create has no slug to name, so the subject is the whole library.
	whole := library.ErrReadOnly{}.Error()
	if !strings.Contains(whole, "this library") {
		t.Errorf("the slugless refusal is %q", whole)
	}
	if strings.Contains(whole, "  ") {
		t.Errorf("the slugless refusal has a hole where the slug was: %q", whole)
	}

	exists := library.ErrExists{Slug: "gyome"}.Error()
	if !strings.Contains(exists, "'gyome'") || !strings.Contains(exists, "already exists") {
		t.Errorf("the exists refusal is %q", exists)
	}
}

// The two predicates let the route layer choose a status without knowing
// which tier answered, so they have to see through wrapping and say no to
// everything else.
func TestTheStatusPredicatesSeeThroughWrapping(t *testing.T) {
	t.Parallel()
	readOnly := library.ErrReadOnly{Slug: "gyome"}
	notFound := library.ErrNotFound{Slug: "gyome"}

	if !library.IsReadOnly(readOnly) || !library.IsReadOnly(fmtWrap(readOnly)) {
		t.Error("IsReadOnly does not see a read-only refusal")
	}
	if !library.IsNotFound(notFound) || !library.IsNotFound(fmtWrap(notFound)) {
		t.Error("IsNotFound does not see a missing deck")
	}
	// And they do not see each other, which is the 403-versus-404 decision.
	if library.IsReadOnly(notFound) || library.IsNotFound(readOnly) {
		t.Error("the two refusals are interchangeable")
	}
	for _, err := range []error{nil, errors.New("something else")} {
		if library.IsReadOnly(err) || library.IsNotFound(err) {
			t.Errorf("%v was classified as one of ours", err)
		}
	}
}

// fmtWrap buries an error one level down, the way a caller's context does.
func fmtWrap(err error) error {
	return errors.Join(errors.New("while doing something"), err)
}

// A write is a rename rather than a truncate: `deck.yaml` is the source of
// truth (ADR 1) and since ADR 30 there is no revision to restore a
// half-written one from. What is observable is that the mode survives, which
// is the property a plain create-and-write would lose.
func TestAWriteReplacesTheFileWithoutLosingItsMode(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()
	path := filepath.Join(root, "gyome", "deck.yaml")

	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(oneDeck, "name: Test Deck", "name: Renamed", 1)
	if err := src.WriteText(ctx, "gyome", updated); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Renamed") {
		t.Errorf("the write did not land:\n%s", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("the file's mode became %v -- a library whose decks wear two "+
			"sets of permissions is a difference somebody has to explain", info.Mode().Perm())
	}
	// No temporary file was left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".") {
			t.Errorf("the write left %q behind", e.Name())
		}
	}
}

// Artifacts are stored whole, snapshot included: `Deliverables` governs what
// may be read, not what a build may write.
func TestWritingArtifactsStoresWhatTheBuildProduced(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	ctx := context.Background()

	names, err := src.WriteArtifacts(ctx, "gyome", artifacts.Files{
		{Name: "primer-quick.md", Text: "# Primer\n"},
		{Name: "swaps.md", Text: "# Swaps\n"},
	})
	if err != nil {
		t.Fatalf("storing: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("stored %v", names)
	}
	// Names rather than paths -- the SQL tier has none and no caller needs one.
	for _, name := range names {
		if strings.ContainsAny(name, `/\`) {
			t.Errorf("%q is a path, not a name", name)
		}
	}
	for _, name := range []string{"primer-quick.md", "swaps.md"} {
		if _, err := os.Stat(filepath.Join(root, "gyome", "artifacts", name)); err != nil {
			t.Errorf("%s was not stored: %v", name, err)
		}
	}

	listed, err := src.Artifacts(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) == 0 {
		t.Error("the stored artifacts do not list")
	}
}
