package deckyaml

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The spike: parse the fixture Python wrote and agree with PyYAML's reading
// of it, value for value. Both files come from `tests/go_fixtures.py`, and
// `tests/test_go_fixtures.py` holds them current against the dumper.
func TestParsesTheFixtureAsPyYAMLDoes(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "rich-deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "rich-deck.parsed.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(text)
	if err != nil {
		t.Fatalf("goccy refused what PyYAML wrote: %v", err)
	}
	// Compare through JSON so both sides carry the same number and string
	// types: PyYAML's ints and goccy's become JSON numbers, and nothing in a
	// deck file is a float PyYAML and goccy could round differently.
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var viaGo, viaPython any
	if err := json.Unmarshal(gotJSON, &viaGo); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &viaPython); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(viaPython, viaGo); diff != "" {
		t.Fatalf("goccy and PyYAML read the same deck differently (-python +go):\n%s", diff)
	}
}

// The shapes the fixture was built to carry, asserted by name so a quieter
// fixture cannot pass this file by saying less (the Python twin of this test
// is `test_the_fixture_exercises_the_shapes_it_claims_to`).
func TestTheShapesThatMatter(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "rich-deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	deck, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	cards, _ := deck["cards"].([]any)
	if len(cards) != 10 {
		t.Fatalf("%d cards, want 10", len(cards))
	}
	why := func(i int) string {
		c, _ := cards[i].(map[string]any)
		s, _ := c["why"].(string)
		return s
	}
	// A single-quoted scalar folded across three lines reads back as one
	// line, and the doubled apostrophe as one.
	if got := why(0); !strings.HasSuffix(got, "format, the first thing every primer tells a newcomer to find, and the card this line exists to make room for -- which is why it is first.") {
		t.Fatalf("folded quoted scalar read as %q", got)
	}
	if got := why(1); !strings.HasPrefix(got, "It's the card") {
		t.Fatalf("doubled apostrophe read as %q", got[:4])
	}
	// Quoted look-alikes stay strings.
	for i, want := range map[int]string{5: "yes", 6: "null", 7: "12"} {
		c, _ := cards[i].(map[string]any)
		if s, ok := c["why"].(string); !ok || s != want {
			t.Fatalf("card %d why = %#v, want the string %q", i, c["why"], want)
		}
	}
	// A plain scalar folded across two lines, braces inside it.
	if got := why(9); !strings.HasPrefix(got, "cost {1}{W}{W} -- braces again, inside a longer sentence that also runs past the hundred-character fold so") {
		t.Fatalf("plain folded scalar read as %q", got)
	}
	// The newline inside a quoted scalar (a blank line in the text) is one
	// newline in the value.
	notes, _ := deck["notes"].(map[string]any)
	if s, _ := notes["mulligan"].(string); s != "Keep any seven with two lands and an equipment; ship a hand\nwith no knight by turn three." {
		t.Fatalf("mulligan note read as %q", s)
	}
	if s, _ := notes["weird"].(string); s != "colon: and # hash, plus braces {G}{W} and a trailing space " {
		t.Fatalf("weird note read as %q", s)
	}
	if v, ok := deck["shared"].(bool); !ok || v {
		t.Fatalf("shared = %#v, want false", deck["shared"])
	}
	if v, ok := deck["bracket"].(int64); !ok || v != 3 {
		t.Fatalf("bracket = %#v (%T), want int64 3", deck["bracket"], deck["bracket"])
	}
}

func TestRefusesANonMapping(t *testing.T) {
	if _, err := Parse([]byte("- just\n- a list\n")); err == nil {
		t.Fatal("a list parsed as a deck")
	}
	if _, err := Parse([]byte("slug: [unterminated\n")); err == nil {
		t.Fatal("malformed YAML parsed")
	}
}
