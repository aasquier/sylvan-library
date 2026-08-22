package pyyaml

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
	"github.com/google/go-cmp/cmp"
)

// renderCase is one row of the oracle: what `edit._render` was asked, and the
// exact lines PyYAML gave back. Written by `tests/go_fixtures.py`.
type renderCase struct {
	Group  string          `json:"group"`
	Key    string          `json:"key"`
	Kind   string          `json:"kind"`
	Value  json.RawMessage `json:"value"`
	Indent int             `json:"indent"`
	Width  int             `json:"width"`
	Fold   bool            `json:"fold"`
	Want   []string        `json:"want"`
}

func (c renderCase) value(t *testing.T) any {
	t.Helper()
	switch c.Kind {
	case "int":
		var v int
		mustUnmarshal(t, c.Value, &v)
		return v
	case "bool":
		var v bool
		mustUnmarshal(t, c.Value, &v)
		return v
	case "list":
		v := []string{}
		mustUnmarshal(t, c.Value, &v)
		return v
	default:
		var v string
		mustUnmarshal(t, c.Value, &v)
		return v
	}
}

func mustUnmarshal(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
}

func loadCases(t *testing.T) []renderCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/render.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases []renderCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}
	return cases
}

// TestRenderMatchesPyYAML is the gate this package exists to pass. Every case
// is a `(key, value, indent, width, fold)` Python was asked for, beside the
// bytes PyYAML wrote; anything but equality is a file this runtime would
// write differently from the other one.
func TestRenderMatchesPyYAML(t *testing.T) {
	cases := loadCases(t)
	byGroup := map[string]int{}
	for _, c := range cases {
		byGroup[c.Group]++
	}
	// A corpus that quietly lost a group would still pass every case in it.
	for _, group := range []string{
		"resolver-lookalikes", "resolver-near-misses", "indicators",
		"whitespace", "prose", "names", "control", "width-sweep",
		"unicode-width", "int", "list", "bool",
	} {
		if byGroup[group] == 0 {
			t.Errorf("the oracle has no %q cases; regenerate it", group)
		}
	}

	for i, c := range cases {
		if c.Fold && startsWithASeparator(c.Value) {
			// The one place this port deliberately answers differently, and
			// it answers *more* safely. See TestTheSeparatorDivergence.
			continue
		}
		got, err := Render(c.Key, c.value(t), c.Indent, c.Width, c.Fold)
		if err != nil {
			t.Errorf("case %d (%s, key=%q, value=%s, indent=%d, width=%d, fold=%v): %v",
				i, c.Group, c.Key, c.Value, c.Indent, c.Width, c.Fold, err)
			continue
		}
		if diff := cmp.Diff(c.Want, got); diff != "" {
			t.Errorf("case %d (%s, key=%q, value=%s, indent=%d, width=%d, fold=%v)\n%s",
				i, c.Group, c.Key, c.Value, c.Indent, c.Width, c.Fold, diff)
		}
	}
}

// TestTheOracleWouldNoticeADrift proves the comparison above can fail. A
// corpus this large is exactly the kind of test that passes because nothing
// reached it, so one case is checked against a deliberately wrong answer.
func TestTheOracleWouldNoticeADrift(t *testing.T) {
	got, err := Render("why", "yes", 6, 96, true)
	if err != nil {
		t.Fatal(err)
	}
	// The word "yes" is a YAML 1.1 boolean, so it may not be written plain.
	// If this ever renders as `why: yes`, the resolver has stopped being
	// consulted and every look-alike in the corpus is wrong with it.
	want := []string{"      why: >-", "        yes"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the resolver is not being consulted\n%s", diff)
	}
	if plain, _ := Render("why", "yes", 6, 96, false); len(plain) != 1 || plain[0] != "      why: 'yes'" {
		t.Errorf("unfolded, a boolean look-alike must be quoted; got %q", plain)
	}
}

// startsWithASeparator spots the one value shape whose folded form the two
// runtimes disagree about: a scalar beginning with U+2028 or U+2029.
func startsWithASeparator(raw json.RawMessage) bool {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return strings.HasPrefix(value, "\u2028") || strings.HasPrefix(value, "\u2029")
}

// TestTheSeparatorDivergence records the one case where this package does not
// reproduce PyYAML, why that is right, and what it costs.
//
// A scalar beginning with U+2028 or U+2029 -- the Unicode line and paragraph
// separators -- is a line break to YAML 1.1 and therefore to PyYAML, which
// folds it into a block scalar with an explicit indentation hint (`>2-`).
// goccy/go-yaml implements YAML 1.2, where those two characters are ordinary
// content, so it does not merely *emit* that block differently: it **cannot
// parse the block PyYAML writes**, failing with "non-map value is specified".
//
// So `Render`'s round-trip check -- the same check `_render` does, asking
// whether the folded form still reads back as the value -- correctly answers
// no, and the fallback picks a quoted form both parsers read. The bytes differ
// from Python's; the meaning does not, and the Go file is the one both
// runtimes can open.
//
// Two consequences worth knowing rather than discovering. The divergence is
// **only reachable at the start of a value** -- a separator in the middle of a
// rationale folds identically in both, which the corpus covers -- and it is
// pre-existing rather than introduced here: a `why` Python already wrote this
// way is a deck file the Go door cannot read at all. An edit made through Go
// can no longer put one there.
func TestTheSeparatorDivergence(t *testing.T) {
	for _, sep := range []string{"\u2028", "\u2029"} {
		value := sep + " sep"
		got, err := Render("why", value, 6, 96, true)
		if err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if len(got) != 1 || !strings.Contains(got[0], "why: ") {
			t.Fatalf("%q: expected a single quoted line, got %q", value, got)
		}
		// The whole point: what this writes, both parsers can read back.
		doc, err := deckyaml.Parse([]byte(strings.TrimLeft(got[0], " ") + "\n"))
		if err != nil {
			t.Fatalf("%q: the fallback does not parse: %v", value, err)
		}
		if doc["why"] != value {
			t.Errorf("%q: read back as %q", value, doc["why"])
		}

		// And the shape PyYAML would have written, for the record.
		block, err := dump("why", Folded(value), 90)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := deckyaml.Parse([]byte(block)); err == nil {
			t.Errorf("%q: goccy now parses %q -- the divergence is gone, and "+
				"this test, the skip in TestRenderMatchesPyYAML and the note "+
				"in the package doc should all go with it", value, block)
		}
	}
}
