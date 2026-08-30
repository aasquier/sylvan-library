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
	t.Parallel()
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

// The rationale column, and the five card names that make it ambiguous.
//
// The interesting half of this table is not the happy path -- it is
// `Kongming, "Sleeping Dragon"`, where the same shape of line means two
// different things and this package deliberately refuses to choose. Every
// case that ends with a quote therefore records BOTH readings, and the pair
// is the contract `deckimport` resolves against the pool.
func TestParseReadsTheRationaleColumn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		line     string
		wantName string
		wantWhy  string
		wantWhol string
		why      string
	}{
		{
			name: "the format Aaron asked for", why: "quantity, name, printing, reason",
			line:     `1 Access Tunnel (MKC) 247 "Taps for colorless but also lets small creatures through"`,
			wantName: "Access Tunnel",
			wantWhy:  "Taps for colorless but also lets small creatures through",
			wantWhol: "",
		},
		{
			name: "no printing", why: "the column does not depend on a set code being present",
			line:     `1 Beast Within "Single target removal for stubborn blockers"`,
			wantName: "Beast Within",
			wantWhy:  "Single target removal for stubborn blockers",
			wantWhol: `Beast Within "Single target removal for stubborn blockers"`,
		},
		{
			name: "the reason comes before the printing", why: "annotations peel right to left in a loop, so their order is free",
			line:     `1 Sol Ring "fast mana, every deck" (LTC) 284`,
			wantName: "Sol Ring",
			wantWhy:  "fast mana, every deck",
			wantWhol: `Sol Ring "fast mana, every deck"`,
		},
		{
			name: "a marker rides along", why: "*CMDR* still has to reach the section logic",
			line:     `1 Arahbo, Roar of the World (C17) 27 *CMDR* "the whole deck"`,
			wantName: "Arahbo, Roar of the World",
			wantWhy:  "the whole deck",
			wantWhol: "",
		},
		{
			name: "a quoted epithet mid-name", why: "the NEAREST opener wins, or this card becomes `Henzie`",
			line:     `1 Henzie "Toolbox" Torre (NCC) 27 "the reason he is here"`,
			wantName: `Henzie "Toolbox" Torre`,
			wantWhy:  "the reason he is here",
			wantWhol: "",
		},
		{
			name: "a name that ends in a quoted epithet", why: "Kongming is one card; both readings are handed up for the pool",
			line:     `1 Kongming, "Sleeping Dragon"`,
			wantName: "Kongming,",
			wantWhy:  "Sleeping Dragon",
			wantWhol: `Kongming, "Sleeping Dragon"`,
		},
		{
			name: "that same name WITH a reason", why: "two quoted runs, and only the last one is the column",
			line:     `1 Kongming, "Sleeping Dragon" "a five-mana lord I keep cutting"`,
			wantName: `Kongming, "Sleeping Dragon"`,
			wantWhy:  "a five-mana lord I keep cutting",
			wantWhol: `Kongming, "Sleeping Dragon" "a five-mana lord I keep cutting"`,
		},
		{
			name: "a name that is nothing but a quoted run", why: "peeling leaves no card, so there is no choice to offer",
			line:     `1 "Ach! Hans, Run!" (UNG) 3`,
			wantName: `"Ach! Hans, Run!"`,
			wantWhy:  "",
			wantWhol: "",
		},
		{
			name: "curly quotes from a document", why: "a reason written in Word and pasted here is still a reason",
			line:     "1 Cultivate (M21) 177 “ramp and fixing in one card”",
			wantName: "Cultivate",
			wantWhy:  "ramp and fixing in one card",
			wantWhol: "",
		},
		{
			name: "an empty quoted run", why: "a rationale nobody wrote is not a rationale; the card still lands",
			line:     `1 Sol Ring ""`,
			wantName: "Sol Ring",
			wantWhy:  "",
			wantWhol: `Sol Ring ""`,
		},
		{
			name: "no quote at all", why: "every existing dialect keeps parsing exactly as it did",
			line:     `1 Sol Ring (LTC) 284`,
			wantName: "Sol Ring",
			wantWhy:  "",
			wantWhol: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Parse(tc.line)
			if len(got.Cards) != 1 {
				t.Fatalf("%s: read %d cards, wanted 1 (%s)\n%+v",
					tc.line, len(got.Cards), tc.why, got)
			}
			c := got.Cards[0]
			if c.Name != tc.wantName {
				t.Errorf("name: got %q, wanted %q (%s)", c.Name, tc.wantName, tc.why)
			}
			if c.Why != tc.wantWhy {
				t.Errorf("why: got %q, wanted %q (%s)", c.Why, tc.wantWhy, tc.why)
			}
			if c.Unpeeled != tc.wantWhol {
				t.Errorf("unpeeled: got %q, wanted %q (%s)", c.Unpeeled, tc.wantWhol, tc.why)
			}
		})
	}
}

// The category column, and the two columns it has to sit beside without
// disturbing either.
//
// The cases worth having are the ones where the bracket meets the quote. A
// rationale that CONTAINS brackets must keep them, because the quoted run is
// peeled before anything looks inside it; a card name that ends in a quoted
// epithet must still hand up both readings with the bracket gone from both,
// because the bracket peels first and the adjacency test that decides those
// readings is therefore asked the same question it was always asked.
func TestParseReadsTheCategoryColumn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		line     string
		wantName string
		wantWhy  string
		wantCat  string
		wantWhol string
		wantCmdr bool
		why      string
	}{
		{
			name: "the format Aaron asked for", why: "quantity, name, reason, category",
			line:     `1 Llanowar Elves "one-mana ramp" [ramp]`,
			wantName: "Llanowar Elves", wantWhy: "one-mana ramp", wantCat: "ramp",
			wantWhol: `Llanowar Elves "one-mana ramp"`,
		},
		{
			name: "with a printing too", why: "the new column peels beside the old ones, not instead of them",
			line:     `1 Acidic Slime (ZNC) 59 "deathtouch that eats artifacts" [interaction]`,
			wantName: "Acidic Slime", wantWhy: "deathtouch that eats artifacts",
			wantCat: "interaction",
		},
		{
			name: "a category and no reason", why: "the columns are independent; either may stand alone",
			line:     `1 Sol Ring (LTC) 284 [ramp]`,
			wantName: "Sol Ring", wantWhy: "", wantCat: "ramp",
		},
		{
			name: "no category at all", why: "every existing dialect keeps parsing exactly as it did",
			line:     `1 Sol Ring "fast mana, and it never gets cut"`,
			wantName: "Sol Ring", wantWhy: "fast mana, and it never gets cut", wantCat: "",
			wantWhol: `Sol Ring "fast mana, and it never gets cut"`,
		},
		{
			name: "a word this library does not file by", why: "the grammar reads the token; placing it is deckimport's job",
			line:     `1 Sol Ring "fast mana" [Big Beaters]`,
			wantName: "Sol Ring", wantWhy: "fast mana", wantCat: "Big Beaters",
			wantWhol: `Sol Ring "fast mana"`,
		},
		{
			name: "the word is carried verbatim", why: "so an unplaceable word can be quoted back as it was typed",
			line:     `1 Sol Ring [RaMp]`,
			wantName: "Sol Ring", wantCat: "RaMp",
		},
		{
			name: "Archidekt's brace suffix", why: "`{top}` is a cursor position, never part of the word",
			line:     `1 Sol Ring [Ramp{top}]`,
			wantName: "Sol Ring", wantCat: "Ramp",
		},
		{
			name: "a reason with brackets inside it", why: "the quoted run is peeled whole; nothing looks inside it",
			line:     `1 Sol Ring "ramp [and it never gets cut]" [ramp]`,
			wantName: "Sol Ring", wantWhy: "ramp [and it never gets cut]", wantCat: "ramp",
			wantWhol: `Sol Ring "ramp [and it never gets cut]"`,
		},
		{
			name: "a reason that is only brackets, no category", why: "a bracket inside quotes is not the category column",
			line:     `1 Sol Ring "[ramp]"`,
			wantName: "Sol Ring", wantWhy: "[ramp]", wantCat: "",
			wantWhol: `Sol Ring "[ramp]"`,
		},
		{
			name: "a quoted name plus a category", why: "Kongming keeps both readings, and neither carries the bracket",
			line:     `1 Kongming, "Sleeping Dragon" [threat]`,
			wantName: "Kongming,", wantWhy: "Sleeping Dragon", wantCat: "threat",
			wantWhol: `Kongming, "Sleeping Dragon"`,
		},
		{
			name: "a quoted name, a reason AND a category", why: "three columns, and the name survives all of them",
			line:     `1 Kongming, "Sleeping Dragon" "a lord I keep cutting" [threat]`,
			wantName: `Kongming, "Sleeping Dragon"`, wantWhy: "a lord I keep cutting",
			wantCat:  "threat",
			wantWhol: `Kongming, "Sleeping Dragon" "a lord I keep cutting"`,
		},
		{
			name: "the commander label is not a category", why: "`[Commander]` is the section marker it has always been",
			line:     `1 Arahbo, Roar of the World [Commander{top}]`,
			wantName: "Arahbo, Roar of the World", wantCat: "", wantCmdr: true,
		},
		{
			name: "the commander label beside a real category", why: "one bracket routes the line, the other files the card",
			line:     `1 Arahbo, Roar of the World [threat] [Commander]`,
			wantName: "Arahbo, Roar of the World", wantCat: "threat", wantCmdr: true,
		},
		{
			name: "two categories", why: "the rightmost keeps the slot, as the rationale column does",
			line:     `1 Sol Ring [ramp] [payoff]`,
			wantName: "Sol Ring", wantCat: "payoff",
		},
		{
			name: "a marker rides along", why: "*F* still peels, and the bracket is not it",
			line:     `1 Sol Ring (2X2) 297 *F* [ramp]`,
			wantName: "Sol Ring", wantCat: "ramp",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Parse(tc.line)
			if len(got.Cards) != 1 {
				t.Fatalf("%s: read %d cards, wanted 1 (%s)\n%+v",
					tc.line, len(got.Cards), tc.why, got)
			}
			c := got.Cards[0]
			if c.Name != tc.wantName {
				t.Errorf("name: got %q, wanted %q (%s)", c.Name, tc.wantName, tc.why)
			}
			if c.Why != tc.wantWhy {
				t.Errorf("why: got %q, wanted %q (%s)", c.Why, tc.wantWhy, tc.why)
			}
			if c.Category != tc.wantCat {
				t.Errorf("category: got %q, wanted %q (%s)", c.Category, tc.wantCat, tc.why)
			}
			if c.Unpeeled != tc.wantWhol {
				t.Errorf("unpeeled: got %q, wanted %q (%s)", c.Unpeeled, tc.wantWhol, tc.why)
			}
			if where := c.Section == "commander"; where != tc.wantCmdr {
				t.Errorf("section: got %q, wanted commander=%v (%s)",
					c.Section, tc.wantCmdr, tc.why)
			}
		})
	}
}
