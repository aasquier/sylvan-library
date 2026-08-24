package decklist

import (
	"encoding/json"
	"os"
	"testing"
)

// The grammar's oracle: every dialect and every edge, beside the recorded
// structure for it (testdata/lists.json, a frozen golden).
//
// A corpus rather than hand-written expectations because the parser is pure
// text in, structure out -- no pool, no filesystem, no database -- so there
// is nothing to arrange and the only interesting question is whether the
// parse matches the recording. Three of the cases exist for the three
// places a careless class would not: the grammar's space includes U+00A0,
// its line splitting breaks on eleven characters, and its quantities are
// any Unicode decimal digit.

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

func TestParseReadsTheRecordedStructure(t *testing.T) {
	raw, err := os.ReadFile("testdata/lists.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases map[string]wantList
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; testdata/lists.json is a frozen golden and should never be")
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got := Parse(want.Text)
			if len(got.Cards) != len(want.Cards) {
				t.Fatalf("read %d cards, the corpus reads %d\ngot    %+v\nwanted %+v",
					len(got.Cards), len(want.Cards), got.Cards, want.Cards)
			}
			for i, c := range got.Cards {
				w := want.Cards[i]
				if c.Name != w.Name || c.Qty != w.Qty || c.Section != w.Section ||
					c.LineNo != w.LineNo {
					t.Errorf("card %d is %+v, the corpus reads %+v", i, c, w)
				}
			}
			checkLines(t, "unreadable", got.Unreadable, want.Unreadable)
			checkLines(t, "skipped", got.Skipped, want.Skipped)

			commander := got.Commander()
			if len(commander) != len(want.Commander) {
				t.Errorf("commander %v, the corpus says %v", commander, want.Commander)
			} else {
				for i, n := range commander {
					if n != want.Commander[i] {
						t.Errorf("commander %d is %q, the corpus says %q", i, n, want.Commander[i])
					}
				}
			}
			// The corpus records `companion` as null when the list
			// nominates none, which is this model's empty string -- the one
			// place the two spellings differ, and `deckimport` reads it the
			// same way.
			companion := ""
			if want.Companion != nil {
				companion = *want.Companion
			}
			if got.Companion() != companion {
				t.Errorf("companion %q, the corpus says %q", got.Companion(), companion)
			}
		})
	}
}

func checkLines(t *testing.T, what string, got []Line, want []wantLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d lines, the corpus has %d\ngot    %+v\nwanted %+v",
			what, len(got), len(want), got, want)
	}
	for i, line := range got {
		if line.LineNo != want[i].LineNo || line.Text != want[i].Text {
			t.Errorf("%s %d is %+v, the corpus has %+v", what, i, line, want[i])
		}
	}
}
