package artifacts

// What a build refuses, and what the store says when the disk will not take
// it.
//
// The refusals matter more here than in most places because a build *prunes*:
// the deliverables it did not produce are deleted. A renderer that answered
// half a primer instead of an error would therefore replace a good file with
// a bad one and remove the rest, and the deck's own directory is where a
// person goes to read what they built. So a refusal has to arrive before a
// single byte is stored, and it has to carry a sentence saying which note is
// the problem.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// deckWithNote is the smallest curated deck, carrying one note written as
// something that is not prose.
func deckWithNote(t *testing.T, key, value string) *deck.Deck {
	t.Helper()
	text := "slug: bare\nname: Bare\nstatus: theoretical\nstage: curated\n" +
		"commander: []\nbracket: 0\nnotes:\n  " + key + ": " + value + "\n" +
		"cards:\n  - name: Forest\n    category: land\n    why: Green.\n    qty: 99\n"
	d, err := deck.FromText(text, "bare")
	if err != nil {
		t.Fatalf("parsing the deck: %v", err)
	}
	return d
}

func TestANoteThatIsNotProseRefusesTheWholeBuildAndSaysWhich(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
	}{
		{"a note the quick primer reads", "gameplan"},
		{"a note the advanced primer reads", "lines"},
		{"the commander's own note", "commander_why"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A list where prose belongs: somebody wrote bullet points into
			// deck.yaml the way a person naturally would.
			d := deckWithNote(t, tc.key, "[one, two]")
			d.Commander = []string{"Fixture Commander"}

			files, err := RenderAll(d, Options{})
			if err == nil {
				t.Fatalf("a %s that is not prose built %d files", tc.key, len(files))
			}
			if files != nil {
				t.Fatalf("a refused build still handed back %d files", len(files))
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("the refusal does not name the note: %v", err)
			}
			if !strings.Contains(err.Error(), "only render prose") {
				t.Fatalf("the refusal does not say why: %v", err)
			}
		})
	}
	// An empty note is no note at all, so the placeholder stands and the
	// build succeeds -- which is what says the refusals above are about the
	// *type* and not about the note being unset.
	blank := deckWithNote(t, "gameplan", `""`)
	if _, err := RenderAll(blank, Options{}); err != nil {
		t.Fatalf("an empty note refused the build: %v", err)
	}
}

func TestEachPrimerRefusesOnItsOwnBeforeTheBuildEverStarts(t *testing.T) {
	t.Parallel()
	// The renderers are called directly by the deck page's Artifacts tab as
	// well as through RenderAll, so each has to refuse for itself rather
	// than relying on the one above it.
	d := deckWithNote(t, "engine_detail", "[one, two]")
	if _, err := AdvancedPrimer(d, Options{}); err == nil {
		t.Fatal("the advanced primer rendered a note that is not prose")
	}
	quick := deckWithNote(t, "mulligan", "[one, two]")
	if _, err := QuickPrimer(quick, Options{}); err == nil {
		t.Fatal("the quick primer rendered a note that is not prose")
	}
	annotated := deckWithNote(t, "commander_why", "[one, two]")
	annotated.Commander = []string{"Fixture Commander"}
	if _, err := AnnotatedDecklist(annotated, Options{}); err == nil {
		t.Fatal("the annotated list rendered a note that is not prose")
	}
	// A deck with no commander never reads `commander_why`, so the same
	// broken note builds cleanly: the Command Zone section is simply absent.
	annotated.Commander = nil
	if _, err := AnnotatedDecklist(annotated, Options{}); err != nil {
		t.Fatalf("a deck with no commander refused: %v", err)
	}
}

func TestADeckThatWillNotWriteBackRefusesBeforeTheSnapshotIsTaken(t *testing.T) {
	t.Parallel()
	// The snapshot is the deck file itself, re-emitted, and it is what the
	// *next* build's `swaps.md` diffs against. A deck holding a value the
	// emitter has no spelling for cannot be written back, and a build that
	// shipped the other four files with no snapshot would leave the next
	// build diffing against the one before it -- a swap list that is not
	// merely stale but wrong.
	//
	// A note holding a number with a decimal point in it is the state: YAML
	// reads it as a float and the deck file's one style has no float in it.
	odd := deckWithNote(t, "odd", "3.5")
	files, err := RenderAll(odd, Options{})
	if err == nil {
		t.Fatalf("a deck that cannot be written back built %d files", len(files))
	}
	if files != nil {
		t.Fatalf("a refused build still handed back %d files", len(files))
	}
	if !strings.Contains(err.Error(), "cannot be written back") {
		t.Fatalf("the refusal does not say what went wrong: %v", err)
	}
	// The primers themselves are fine with it -- the key is not one they
	// read -- so the refusal really is the snapshot's and not theirs.
	if _, err := QuickPrimer(odd, Options{}); err != nil {
		t.Fatalf("the quick primer refused a note it does not read: %v", err)
	}
}

func TestFilesAnswersOnlyForWhatTheBuildProduced(t *testing.T) {
	t.Parallel()
	files := Files{{Name: "primer-quick.md", Text: "# Quick"}, {Name: Snapshot, Text: "slug: x\n"}}
	if text, ok := files.Text("primer-quick.md"); !ok || text != "# Quick" {
		t.Fatalf("Text(primer-quick.md) = %q, %v", text, ok)
	}
	if text, ok := files.Text(Snapshot); !ok || text != "slug: x\n" {
		t.Fatalf("Text(snapshot) = %q, %v", text, ok)
	}
	// A file this build did not make answers "" and false -- the two
	// together, so a caller cannot read an absent file as an empty one.
	text, ok := files.Text("swaps.md")
	if ok || text != "" {
		t.Fatalf("Text(swaps.md) = %q, %v for a build that made none", text, ok)
	}
	if files.Has("swaps.md") {
		t.Fatal("Has said a file this build did not make is present")
	}
	if !files.Has(Snapshot) {
		t.Fatal("Has said the snapshot is absent")
	}
}

func TestStoreReportsADirectoryItCannotWriteOrPrune(t *testing.T) {
	t.Parallel()
	// A build whose files cannot be written must say so. Reporting success
	// here would tell somebody their primer was rebuilt while the old one
	// sat on the disk.
	readonly := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(readonly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })
	if written, err := Store(Files{{Name: "primer-quick.md", Text: "#"}}, readonly); err == nil {
		t.Fatalf("a read-only directory took %v", written)
	}

	// A file standing where the deck's directory belongs: the directory
	// cannot even be made, which is the first thing `Store` tries.
	blocked := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if written, err := Store(Files{{Name: "primer-quick.md", Text: "#"}},
		filepath.Join(blocked, "artifacts")); err == nil {
		t.Fatalf("a directory was made inside a file: %v", written)
	}

	// And the prune's own failure: a deliverable's name standing on a
	// directory with something in it. The pruning is what keeps a stale
	// `swaps.md` from describing a diff that no longer exists, so a prune
	// that could not happen is not a build that succeeded.
	dir := t.TempDir()
	stale := filepath.Join(dir, "swaps.md")
	if err := os.MkdirAll(filepath.Join(stale, "inside"), 0o750); err != nil {
		t.Fatal(err)
	}
	if written, err := Store(Files{{Name: "primer-quick.md", Text: "#"}}, dir); err == nil {
		t.Fatalf("a prune that could not happen reported %v", written)
	}
	// The ordinary case still lands, and still prunes.
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "swaps.md"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	written, err := Store(Files{{Name: "primer-quick.md", Text: "#"}}, plain)
	if err != nil {
		t.Fatalf("an ordinary store: %v", err)
	}
	if len(written) != 1 || written[0] != "primer-quick.md" {
		t.Fatalf("wrote %v", written)
	}
	if _, err := os.Stat(filepath.Join(plain, "swaps.md")); err == nil {
		t.Fatal("a swap list this build did not produce survived the prune")
	}
}

func TestACardTheOtherBuildNeverHeldFallsBackToThePlaceholder(t *testing.T) {
	t.Parallel()
	// The swap list reads each side's rationale off the deck that held the
	// card. A name that is in neither -- a commander change, a card renamed
	// between builds -- has no rationale to print, and the placeholder is
	// what a reader sees instead of a blank line.
	cards := []deck.CardEntry{
		{Name: "Fixture Rock", Why: "It makes mana."},
		{Name: "Fixture Blank"},
	}
	if got := firstWhy(cards, "Fixture Rock", "_(cut)_"); got != "It makes mana." {
		t.Fatalf("a card with a rationale printed %q", got)
	}
	if got := firstWhy(cards, "Fixture Blank", "_(cut)_"); got != "_(cut)_" {
		t.Fatalf("a card with no rationale printed %q", got)
	}
	if got := firstWhy(cards, "Fixture Stranger", "_(added)_"); got != "_(added)_" {
		t.Fatalf("a card the deck never held printed %q", got)
	}
	if got := firstWhy(nil, "Fixture Stranger", "_(cut)_"); got != "_(cut)_" {
		t.Fatalf("a deck with no cards printed %q", got)
	}
}
