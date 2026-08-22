package deck

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The dump oracle: `Deck.dump` over every field combination the two lifecycle
// writers can produce, beside the exact bytes PyYAML gives for it
// (`tests/go_fixtures.py`, which writes testdata/dumps.json).
//
// Driven as a round trip -- parse the recorded file, dump it again, compare
// bytes -- rather than by rebuilding each fixture deck in Go, which would be
// eight constructions free to drift from the eight Python ones. The round trip
// catches everything either half could get wrong about a shape that survives a
// parse: a key in the wrong order, a default asserted that should have been
// omitted, a scalar quoted differently, a fold at the wrong column. The one
// branch it cannot reach is the archetype the dump *drops*, which has its own
// test below.
func TestDumpWritesWhatPythonWrites(t *testing.T) {
	raw, err := os.ReadFile("testdata/dumps.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases map[string]string
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := FromText(want, name)
			if err != nil {
				t.Fatalf("parsing the recorded file: %v", err)
			}
			got, err := d.Dump()
			if err != nil {
				t.Fatalf("dumping: %v", err)
			}
			if got != want {
				t.Errorf("dump differs from Python's\n--- want ---\n%s\n--- got ---\n%s",
					want, got)
			}
		})
	}
}

// A blank rationale is written on a draft and omitted on a curated deck, and
// on a draft it lands *after* `qty` -- because `to_obj` writes `why` in second
// place only when there is one, and Python's `setdefault` appends a key the
// mapping has not got. Pinned apart from the oracle because it is the one
// ordering rule that looks like a mistake.
func TestADraftsBlankRationaleFollowsTheQuantity(t *testing.T) {
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

// Notes are refused rather than written in whatever order a Go map hands them
// over. Neither lifecycle caller can reach it -- a created deck has none and
// an imported one carries none across -- and a refusal is the honest answer
// until something needs the ordering.
func TestADeckWithNotesRefusesToDump(t *testing.T) {
	d := &Deck{Slug: "d", Name: "D", Status: "built", Stage: "curated",
		Commander: []string{"Gyome, Master Chef"},
		Notes:     map[string]any{"mulligan": "Keep two lands."}}
	if _, err := d.Dump(); err == nil {
		t.Fatal("a deck with notes must not be dumped in map order")
	}
	d.Notes = nil
	d.Strategy = map[string]any{"plan": "not prose"}
	if _, err := d.Dump(); err == nil {
		t.Fatal("a strategy that is not prose must not be dumped either")
	}
}
