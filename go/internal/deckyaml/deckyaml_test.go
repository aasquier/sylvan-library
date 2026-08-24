package deckyaml

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The parse held to the corpus: read the recorded fixture and agree with
// its recorded parse, value for value. Both files are frozen goldens
// (testdata/rich-deck.yaml and its .parsed.json), never regenerated.
func TestParsesTheFixtureAsRecorded(t *testing.T) {
	t.Parallel()
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
		t.Fatalf("goccy refused the recorded fixture: %v", err)
	}
	// Compare through JSON so both sides carry the same number and string
	// types: the recorded ints and goccy's both become JSON numbers, and
	// nothing in a deck file is a float the two encodings could round
	// differently.
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var viaGoccy, viaCorpus any
	if err := json.Unmarshal(gotJSON, &viaGoccy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &viaCorpus); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(viaCorpus, viaGoccy); diff != "" {
		t.Fatalf("the parse disagrees with the recorded reading (-golden +got):\n%s", diff)
	}
}

// The shapes the fixture was built to carry, asserted by name so a quieter
// fixture cannot pass this file by saying less.
func TestTheShapesThatMatter(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	if _, err := Parse([]byte("- just\n- a list\n")); err == nil {
		t.Fatal("a list parsed as a deck")
	}
	if _, err := Parse([]byte("slug: [unterminated\n")); err == nil {
		t.Fatal("malformed YAML parsed")
	}
	if _, err := ParseOrdered([]byte("- just\n- a list\n")); err == nil {
		t.Fatal("a list parsed as an ordered deck")
	}
}

// The order survives at every depth, and `Plain` is the same parse with the
// order thrown away -- which is what every caller that only looks keys up
// still gets from `Parse`.
//
// Written backwards through the alphabet on purpose. A Go map iterated at
// random passes an in-order assertion often enough to be no assertion at all,
// and a *reversed* one about once in n!.
func TestParseOrderedKeepsTheDocumentsOrder(t *testing.T) {
	t.Parallel()
	const text = `zebra: last in the file, first in nothing
notes:
  wincons: win
  pitfalls: lose
  mulligan: keep
  nested:
    third: 3
    second: 2
    first: 1
alpha: 1
`
	doc, err := ParseOrdered([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := keysOf(doc); !equal(got, []string{"zebra", "notes", "alpha"}) {
		t.Fatalf("top level %v", got)
	}
	notes, ok := doc.Get("notes")
	if !ok {
		t.Fatal("no notes")
	}
	inner, ok := notes.(Map)
	if !ok {
		t.Fatalf("notes is %T, not an ordered mapping", notes)
	}
	if got := keysOf(inner); !equal(got, []string{"wincons", "pitfalls", "mulligan", "nested"}) {
		t.Fatalf("notes %v", got)
	}
	nested, _ := inner.Get("nested")
	if got := keysOf(nested.(Map)); !equal(got, []string{"third", "second", "first"}) {
		t.Fatalf("nested %v", got)
	}

	// JSON in the file's order -- the recorded wire order, which the stock
	// map encoder would alphabetise.
	raw, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"wincons":"win","pitfalls":"lose","mulligan":"keep",` +
		`"nested":{"third":3,"second":2,"first":1}}`
	if string(raw) != want {
		t.Errorf("json is %s\n    want %s", raw, want)
	}

	// And `Parse` is this with the order dropped: same values, plain maps all
	// the way down, which is what `deckedit` reads.
	plain, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	flat, ok := plain["notes"].(map[string]any)
	if !ok {
		t.Fatalf("Parse gave notes as %T; every caller of it expects a map", plain["notes"])
	}
	if _, ok := flat["nested"].(map[string]any); !ok {
		t.Fatalf("Parse left an ordered mapping nested inside: %T", flat["nested"])
	}
	if diff := cmp.Diff(plain, doc.Plain()); diff != "" {
		t.Errorf("Parse and ParseOrdered().Plain() disagree (-parse +plain):\n%s", diff)
	}
}

func keysOf(m Map) []string {
	out := make([]string, 0, len(m))
	for _, p := range m {
		out = append(out, p.Key)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
