package artifacts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The artifacts oracle: `render_all` over every fixture deck, beside the exact
// markdown Python writes for it (`tests/go_fixtures.py`, which writes
// testdata/artifacts.json).
//
// Byte for byte, and in order, because both of those are the product: a primer
// is markdown somebody reads and `moxfield.txt` is pasted into a website, and
// `store` writes the files in the order `render_all` builds them and relies on
// the snapshot being last.
//
// The date is taken from the oracle rather than from the clock. Every
// deliverable ends in `_Generated <today>_`, so a fixture that asked the clock
// would pass all day and fail at midnight -- which is the sort of red build
// that gets rerun rather than read.

type oracleFile struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type oracleCase struct {
	Name     string             `json:"name"`
	Deck     string             `json:"deck"`
	Previous *string            `json:"previous"`
	Prices   map[string]float64 `json:"prices"`
	Stats    [][]string         `json:"stats"`
	OK       bool               `json:"ok"`
	Files    []oracleFile       `json:"files"`
	Error    string             `json:"error"`
}

type oracle struct {
	Today string             `json:"today"`
	Cards map[string]*string `json:"cards"`
	Cases []oracleCase       `json:"cases"`
}

func loadOracle(t *testing.T) oracle {
	t.Helper()
	raw, err := os.ReadFile("testdata/artifacts.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var o oracle
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(o.Cases) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}
	return o
}

func TestRenderAllWritesWhatPythonWrites(t *testing.T) {
	o := loadOracle(t)
	today, err := time.Parse("2006-01-02", o.Today)
	if err != nil {
		t.Fatalf("the oracle's date: %v", err)
	}
	cards := map[string]*pool.CardRecord{}
	for name, cost := range o.Cards {
		cards[name] = &pool.CardRecord{Name: name, ManaCost: cost}
	}

	for _, c := range o.Cases {
		t.Run(c.Name, func(t *testing.T) {
			d, err := deck.FromText(c.Deck, c.Name)
			if err != nil {
				t.Fatalf("parsing the deck: %v", err)
			}
			opts := Options{Cards: cards, Prices: c.Prices, Today: today}
			if c.Previous != nil {
				previous, err := deck.FromText(*c.Previous, c.Name)
				if err != nil {
					t.Fatalf("parsing the baseline: %v", err)
				}
				opts.Previous = previous
			}
			for _, s := range c.Stats {
				opts.Stats = append(opts.Stats, Stat{Key: s[0], Value: s[1]})
			}

			files, err := RenderAll(d, opts)
			if !c.OK {
				if err == nil {
					t.Fatalf("a draft must be refused; got %d files", len(files))
				}
				if !errors.Is(err, ErrDraft) {
					t.Fatalf("refused with the wrong kind of error: %v", err)
				}
				if got := Message(err); got != c.Error {
					t.Errorf("refusal differs from Python's\n want %q\n  got %q",
						c.Error, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("rendering: %v", err)
			}
			if len(files) != len(c.Files) {
				t.Fatalf("built %d files, Python built %d", len(files), len(c.Files))
			}
			for i, want := range c.Files {
				if files[i].Name != want.Name {
					t.Fatalf("file %d is %q, Python's is %q -- the order is what "+
						"`Store` writes in", i, files[i].Name, want.Name)
				}
				if files[i].Text != want.Text {
					t.Errorf("%s differs from Python's\n--- want ---\n%s\n--- got ---\n%s",
						want.Name, want.Text, files[i].Text)
				}
			}
		})
	}
}

// The oracle is only worth what it covers, and three of its branches are the
// ones a smaller corpus would quietly have skipped.
func TestTheOracleCoversTheBranchesItClaimsTo(t *testing.T) {
	o := loadOracle(t)
	var refused, withSwaps, withShopping, withStats int
	for _, c := range o.Cases {
		if !c.OK {
			refused++
			continue
		}
		for _, f := range c.Files {
			if f.Name == "swaps.md" {
				withSwaps++
				if len(c.Prices) > 0 {
					withShopping++
				}
			}
		}
		if len(c.Stats) > 0 {
			withStats++
		}
	}
	if refused < 2 {
		t.Errorf("%d refusals: the draft rule has two branches, owed and settled", refused)
	}
	if withSwaps < 1 || withShopping < 1 || withStats < 1 {
		t.Errorf("swaps %d, shopping lists %d, stats blocks %d: each is a "+
			"kwarg no caller passes today, which is exactly why the oracle "+
			"has to", withSwaps, withShopping, withStats)
	}
}

// `store` writes what a build made and removes the deliverables it did not --
// and leaves everything else alone, the snapshot included.
func TestStorePrunesOnlyTheDeliverablesItDidNotMake(t *testing.T) {
	dir := t.TempDir()
	// A previous build: all five, plus the baseline and something a person put
	// there.
	stale := append(Files{}, File{Name: Snapshot, Text: "old baseline"})
	for _, name := range Deliverables {
		stale = append(stale, File{Name: name, Text: "from the last build"})
	}
	if _, err := Store(stale, dir); err != nil {
		t.Fatalf("first store: %v", err)
	}
	mine := filepath.Join(dir, "notes-to-self.md")
	if err := os.WriteFile(mine, []byte("mine"), 0o644); err != nil {
		t.Fatalf("writing a file of my own: %v", err)
	}

	// A rebuild with no baseline: no `swaps.md`, so the previous one must go.
	// It described a diff that no longer exists, which is stale in the one way
	// indistinguishable from current.
	fresh := Files{
		{Name: "primer-quick.md", Text: "quick"},
		{Name: "primer-advanced.md", Text: "advanced"},
		{Name: "decklist-annotated.md", Text: "annotated"},
		{Name: "moxfield.txt", Text: "moxfield"},
		{Name: Snapshot, Text: "new baseline"},
	}
	written, err := Store(fresh, dir)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if len(written) != len(fresh) || written[len(written)-1] != Snapshot {
		t.Fatalf("store reported %v; the snapshot is written last", written)
	}
	if _, err := os.Stat(filepath.Join(dir, "swaps.md")); !os.IsNotExist(err) {
		t.Error("a rebuild that wrote no swaps.md must not leave the old one")
	}
	for _, name := range []string{"primer-quick.md", Snapshot, "notes-to-self.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should still be there: %v", name, err)
		}
	}
	// The snapshot is not a deliverable, so it is never pruned -- and it was
	// replaced by this build rather than left at the old text.
	body, err := os.ReadFile(filepath.Join(dir, Snapshot))
	if err != nil || string(body) != "new baseline" {
		t.Errorf("the baseline should be this build's: %q (%v)", body, err)
	}
	// A second rebuild of the same shape must not fail on the `swaps.md` that
	// is now already gone -- `unlink(missing_ok=True)`.
	if _, err := Store(fresh, dir); err != nil {
		t.Fatalf("a second rebuild: %v", err)
	}
}

// `Deliverables` is the served set and the path-traversal guard in one, and
// the snapshot is deliberately outside it: nobody asked for a copy of the deck
// they already have, and a name that is not one of the five never becomes a
// path at all.
func TestTheSnapshotIsNotADeliverable(t *testing.T) {
	if IsDeliverable(Snapshot) {
		t.Error("the baseline is the build's own bookkeeping, not a deliverable")
	}
	for _, name := range []string{"../../etc/passwd", "", "swaps.md.bak", "SWAPS.MD"} {
		if IsDeliverable(name) {
			t.Errorf("%q is not one of the five", name)
		}
	}
	if len(Deliverables) != 5 || !IsDeliverable("swaps.md") {
		t.Errorf("the five deliverables are %v", Deliverables)
	}
}

// Python's `str.title()`, which is the annotated list's fallback heading for a
// category the model does not declare -- and *not* the quick primer's, which
// prints the raw word. Two fallbacks for one missing key is a fact about the
// Python rather than a rule, so both are reproduced and this pins the odd one.
func TestAnInventedCategoryIsTitleCasedInOnePlaceOnly(t *testing.T) {
	for word, want := range map[string]string{
		"stax-piece": "Stax-Piece", "aggro-plan": "Aggro-Plan",
		"win-con": "Win-Con", "aBc": "Abc", "a1b": "A1B", "": "",
	} {
		if got := pyTitle(word); got != want {
			t.Errorf("pyTitle(%q) = %q, want %q", word, got, want)
		}
	}
}
