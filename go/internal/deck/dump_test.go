package deck

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
)

// The dump oracle: `Dump` over every field combination the two lifecycle
// writers can produce, beside the exact recorded bytes for each
// (testdata/dumps.json, a frozen golden that is never regenerated).
//
// Driven as parse-then-dump -- read the case's source, dump it, compare
// bytes with the recorded dump of that same source -- rather than by
// rebuilding each fixture deck in code, which would be a dozen
// constructions free to drift from the dozen recorded ones. It catches
// everything a parse-surviving shape could go wrong on: a key in the wrong
// order, a default asserted that should have been omitted, a scalar quoted
// differently, a fold at the wrong column. The one branch it cannot reach
// is the archetype the dump *drops*, which has its own test below.
//
// For a constructed deck, `source` is its own dump and this is the round
// trip the oracle has always run. For the hand-written decks it is the file
// a person typed, which is the only shape that proves anything about
// reading one: a recorded dump has already been through the emitter's
// choices, so feeding it back in shows the parser the tidied version and
// hides every disagreement about the untidied one.
func TestDumpWritesTheRecordedBytes(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/dumps.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases map[string]struct{ Source, Want string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; testdata/dumps.json is a frozen golden and should never be")
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := FromText(c.Source, name)
			if err != nil {
				t.Fatalf("parsing the recorded file: %v", err)
			}
			got, err := d.Dump()
			if err != nil {
				t.Fatalf("dumping: %v", err)
			}
			if got != c.Want {
				t.Errorf("dump differs from the recorded bytes\n--- want ---\n%s\n--- got ---\n%s",
					c.Want, got)
			}
		})
	}
}

// A blank rationale is written on a draft and omitted on a curated deck, and
// on a draft it lands *after* `qty` -- `why` sits in second place only when
// there is one, and the draft's blank entry is appended after every present
// field. Pinned apart from the oracle because it is the one ordering rule
// that looks like a mistake.
func TestADraftsBlankRationaleFollowsTheQuantity(t *testing.T) {
	t.Parallel()
	d := &Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"},
		Cards:     []CardEntry{{Name: "Forest", Category: "land", Qty: 4}}}
	got, err := d.Dump()
	if err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if !strings.Contains(got, "    qty: 4\n    why: ''\n") {
		t.Errorf("a draft's blank why should follow its qty:\n%s", got)
	}
	d.Stage = "curated"
	if got, err = d.Dump(); err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if strings.Contains(got, "why") {
		t.Errorf("a curated deck should not pre-type a blank rationale:\n%s", got)
	}
}

// The pre-ADR-37 `archetype:` key is written back while it is the deck's only
// board and dropped the moment the themes name a class word -- the
// self-cleaning round trip a file gets on its next write.
func TestTheLegacyArchetypeIsDroppedOnceShadowed(t *testing.T) {
	t.Parallel()
	d := &Deck{Slug: "d", Name: "D", Status: "built", Stage: "curated",
		Commander:       []string{"Atla Palani, Nest Tender"},
		LegacyArchetype: "midrange", Themes: []string{"dinosaurs", "sacrifice"}}
	got, err := d.Dump()
	if err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if !strings.Contains(got, "archetype: midrange") {
		t.Errorf("an unshadowed archetype is still the deck's board:\n%s", got)
	}
	d.Themes = append(d.Themes, "midrange")
	if got, err = d.Dump(); err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if strings.Contains(got, "archetype: midrange") {
		t.Errorf("a shadowed archetype should leave on the next write:\n%s", got)
	}
}

// The notes come back out in the order they went in, at every depth. Written
// deliberately backwards through the alphabet, because a Go map iterated at
// random passes an ordered assertion often enough to be useless and passes a
// *reversed* one about one time in `n!`.
func TestNotesKeepTheFilesOrder(t *testing.T) {
	t.Parallel()
	const text = `slug: ordered
name: Ordered
status: built
stage: curated
commander:
  - Gyome, Master Chef
notes:
  wincons: Feed everybody, then stop.
  pitfalls: The table remembers who fed them.
  mulligan: Keep two lands and a sacrifice outlet.
  gameplan: Cook.
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
`
	d, err := FromText(text, "ordered")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"wincons", "pitfalls", "mulligan", "gameplan"}
	for i, key := range want {
		if d.Notes[i].Key != key {
			t.Fatalf("note %d is %q, want %q", i, d.Notes[i].Key, key)
		}
	}
	got, err := d.Dump()
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	if !strings.Contains(got, "notes:\n  wincons: Feed everybody, then stop.\n  pitfalls:") {
		t.Fatalf("the dump reordered the notes:\n%s", got)
	}
	// And back again: a snapshot is parsed by the *next* build, so the order
	// has to survive the round trip rather than only the first half of it.
	again, err := FromText(got, "ordered")
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !again.SameAs(d) {
		t.Fatalf("a round trip changed the deck:\n%s", got)
	}
	twice, err := again.Dump()
	if err != nil || twice != got {
		t.Fatalf("a second dump differs (%v):\n%s\n---\n%s", err, got, twice)
	}
}

// A deck whose notes are the same pairs in a different order is a
// *different* deck to the baseline comparison, because that comparison is
// dump against dump and the two files differ. A bare map would have called
// them equal and reported artifacts as `current` that were not.
func TestReorderedNotesAreADifferentDeck(t *testing.T) {
	t.Parallel()
	head := "slug: d\nname: D\nstatus: built\nstage: curated\ncommander:\n  - Gyome, Master Chef\n"
	tail := "cards:\n  - name: Sol Ring\n    category: ramp\n    why: Two mana.\n"
	one, err := FromText(head+"notes:\n  a: first\n  b: second\n"+tail, "d")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	other, err := FromText(head+"notes:\n  b: second\n  a: first\n"+tail, "d")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if one.SameAs(other) {
		t.Fatal("two note orders must not compare equal: their dumps differ")
	}
}

// The refusal that outlived the notes. A strategy that is not prose still
// cannot be written, for the reason the notes could not be until they were
// ordered -- a hand-edited file may hold anything and `FromText` passes it
// through -- and a note holding a mapping is written now rather than refused,
// because the parse keeps its order too.
func TestAStrategyThatIsNotProseRefusesToDump(t *testing.T) {
	t.Parallel()
	d := &Deck{Slug: "d", Name: "D", Status: "built", Stage: "curated",
		Commander: []string{"Gyome, Master Chef"},
		Strategy:  map[string]any{"plan": "not prose"}}
	if _, err := d.Dump(); err == nil {
		t.Fatal("a strategy that is not prose must not be dumped")
	}
	// A note the emitter has no way to write is the same refusal one field
	// over, and the route answers it as a 500.
	d.Strategy = ""
	d.Notes = deckyaml.Map{{Key: "mulligan", Value: 1.5}}
	if _, err := d.Dump(); err == nil {
		t.Fatal("a note the emitter cannot write must not be dumped")
	}
}
