package decklist

import (
	"encoding/json"
	"os"
	"testing"
)

// The grammar's oracle: every dialect and every edge, beside the structure
// Python's parser gives back (`tests/go_fixtures.py`, which writes
// testdata/lists.json).
//
// A corpus rather than hand-written expectations because the parser is pure
// text in, structure out -- no pool, no filesystem, no database -- so there is
// nothing to arrange and the only interesting question is whether the two
// engines agree. Three of the cases exist for the three places they might not:
// Python's `\s` includes U+00A0, its `splitlines()` breaks on eleven
// characters, and its `\d` matches any Unicode decimal digit.

type wantCard struct {
	Name    string `json:"name"`
	Qty     int    `json:"qty"`
	Section string `json:"section"`
	LineNo  int    `json:"line_no"`
}

type wantLine struct {
	LineNo int    `json:"line_no"`
	Text   string `json:"text"`
}

type wantList struct {
	Text       string     `json:"text"`
	Cards      []wantCard `json:"cards"`
	Unreadable []wantLine `json:"unreadable"`
	Skipped    []wantLine `json:"skipped"`
	Commander  []string   `json:"commander"`
	Companion  *string    `json:"companion"`
}

func TestParseReadsWhatPythonReads(t *testing.T) {
	raw, err := os.ReadFile("testdata/lists.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases map[string]wantList
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got := Parse(want.Text)
			if len(got.Cards) != len(want.Cards) {
				t.Fatalf("read %d cards, Python read %d\ngot    %+v\nwanted %+v",
					len(got.Cards), len(want.Cards), got.Cards, want.Cards)
			}
			for i, c := range got.Cards {
				w := want.Cards[i]
				if c.Name != w.Name || c.Qty != w.Qty || c.Section != w.Section ||
					c.LineNo != w.LineNo {
					t.Errorf("card %d is %+v, Python read %+v", i, c, w)
				}
			}
			checkLines(t, "unreadable", got.Unreadable, want.Unreadable)
			checkLines(t, "skipped", got.Skipped, want.Skipped)

			commander := got.Commander()
			if len(commander) != len(want.Commander) {
				t.Errorf("commander %v, Python %v", commander, want.Commander)
			} else {
				for i, n := range commander {
					if n != want.Commander[i] {
						t.Errorf("commander %d is %q, Python %q", i, n, want.Commander[i])
					}
				}
			}
			// Python's `companion` is None when the list nominates none, which
			// is this side's empty string -- the one place the two models are
			// spelled differently, and `deckimport` reads it the same way.
			companion := ""
			if want.Companion != nil {
				companion = *want.Companion
			}
			if got.Companion() != companion {
				t.Errorf("companion %q, Python %q", got.Companion(), companion)
			}
		})
	}
}

func checkLines(t *testing.T, what string, got []Line, want []wantLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d lines, Python had %d\ngot    %+v\nwanted %+v",
			what, len(got), len(want), got, want)
	}
	for i, line := range got {
		if line.LineNo != want[i].LineNo || line.Text != want[i].Text {
			t.Errorf("%s %d is %+v, Python had %+v", what, i, line, want[i])
		}
	}
}
