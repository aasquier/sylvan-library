package deckimport

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The importer's oracle: a paste, resolved against the same 21-card pool
// Python resolved it against, beside the draft `deck.yaml` Python wrote
// (`tests/go_fixtures.py`, which writes testdata/imports.json).
//
// The pool is the load-bearing half. `canonicalName` corrects casing and keeps
// a double-faced card written by its front face, `buildEntries` files a card
// as `land` on `IsLand` and never on a heading, and an unknown name is kept
// verbatim rather than dropped -- none of which can be checked without real
// records, and all of which decide what lands in somebody's file.

type importCase struct {
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Text           string   `json:"text"`
	Commander      []string `json:"commander"`
	Companion      string   `json:"companion"`
	Bracket        *int     `json:"bracket"`
	DeckName       string   `json:"deck_name"`
	Status         string   `json:"status"`
	Refused        string   `json:"refused"`
	YAML           string   `json:"yaml"`
	Unknown        []string `json:"unknown"`
	Notes          []string `json:"notes"`
	Unreadable     []line   `json:"unreadable"`
	Skipped        []line   `json:"skipped"`
	NeedsRationale int      `json:"needs_rationale"`
}

type line struct {
	LineNo int    `json:"line_no"`
	Text   string `json:"text"`
}

func TestBuildDeckWritesWhatPythonWrites(t *testing.T) {
	raw, err := os.ReadFile("testdata/imports.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases []importCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}

	cards := pooltest.Open(t)
	ctx := context.Background()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			parsed := decklist.Parse(c.Text)
			var found map[string]*pool.CardRecord
			if err := cards.Use(ctx, func(conn *pool.Conn) error {
				var err error
				found, err = conn.GetCards(ctx, NamesIn(parsed, c.Commander, c.Companion))
				return err
			}); err != nil {
				t.Fatalf("asking the pool: %v", err)
			}

			report, err := BuildDeck(parsed, found, Options{
				Slug: c.Slug, Name: c.DeckName, Commander: c.Commander,
				Companion: c.Companion, Bracket: c.Bracket, Status: c.Status})

			if c.Refused != "" {
				if err == nil {
					t.Fatalf("Python refused with %q; this built a deck", c.Refused)
				}
				if !IsRefused(err) {
					t.Fatalf("refused with the wrong kind of error: %v", err)
				}
				if err.Error() != c.Refused {
					t.Errorf("refusal is\n  %s\nPython's is\n  %s", err.Error(), c.Refused)
				}
				return
			}
			if err != nil {
				t.Fatalf("Python built a deck; this refused: %v", err)
			}

			// The header carries the day it was written. Substituting our own
			// date proves the format as well as the value: a Go side that
			// wrote it differently would leave the literal date in place and
			// the comparison below would show it.
			got := strings.Replace(report.YAML, time.Now().Format("2006-01-02"), "DATE", 1)
			if got != c.YAML {
				t.Errorf("the file differs from Python's\n--- want ---\n%s\n--- got ---\n%s",
					c.YAML, got)
			}
			checkStrings(t, "unknown", report.Unknown, c.Unknown)
			checkStrings(t, "notes", report.Notes, c.Notes)
			checkLines(t, "unreadable", report.Unreadable, c.Unreadable)
			checkLines(t, "skipped", report.Skipped, c.Skipped)
			if report.NeedsRationale() != c.NeedsRationale {
				t.Errorf("needs_rationale %d, Python %d",
					report.NeedsRationale(), c.NeedsRationale)
			}
		})
	}
}

func checkStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %v, Python %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s %d is %q, Python %q", what, i, got[i], want[i])
		}
	}
}

func checkLines(t *testing.T, what string, got []decklist.Line, want []line) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d, Python %d", what, len(got), len(want))
	}
	for i := range got {
		if got[i].LineNo != want[i].LineNo || got[i].Text != want[i].Text {
			t.Errorf("%s %d is %+v, Python %+v", what, i, got[i], want[i])
		}
	}
}

// The companion hint has no card in the 21-card pool to fire on, so it gets a
// record of its own. It is a *hint* and never a decision: having a Companion
// ability is a pool fact, and concluding that this deck runs the card as its
// companion is a judgement the import does not make.
func TestACompanionOnTheBoardIsPointedAtRatherThanAssumed(t *testing.T) {
	text := "1 Sol Ring\n"
	parsed := decklist.Parse(text + "Sideboard\n1 Kaheera, the Orphanguard\n")
	kaheera := &pool.CardRecord{
		Name:     "Kaheera, the Orphanguard",
		TypeLine: "Legendary Creature — Cat Beast",
		OracleText: "Companion — Each creature card in your starting deck is a " +
			"Cat, Elemental, Nightmare, Dinosaur, or Beast card.",
	}
	report, err := BuildDeck(parsed,
		map[string]*pool.CardRecord{"Kaheera, the Orphanguard": kaheera},
		Options{Slug: "hint", Commander: []string{"Goreclaw, Terror of Qal Sisma"},
			Status: "theoretical"})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if report.Deck.Companion != nil {
		t.Error("a companion on the board must not be assumed to be the deck's")
	}
	var hinted bool
	for _, note := range report.Notes {
		if strings.Contains(note, "has a Companion ability") {
			hinted = true
		}
	}
	if !hinted {
		t.Errorf("the board's companion should be pointed at; notes were %v", report.Notes)
	}
}
