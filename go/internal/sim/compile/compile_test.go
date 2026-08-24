package compile_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
)

// The compiler, held to the frozen corpus `testdata/compile.json`.
//
// Two corpora in one file, and they are separate on purpose. The **texts** are
// the three readers on their own -- the functions that read prose, where every
// real bug this module has had lived. The **decks** are the whole chain: the
// same YAML, parsed by `internal/deck`, resolved against the same 21-card pool
// through `pooltest`, compiled, and compared card for card in order.
//
// The deck cases are what make the text cases trustworthy rather than the
// other way round: a text case proves `ManaProduced` reads a sentence the same
// way, and only a deck case proves that the sentence it was handed came off
// the same column of the same row.

type costJSON struct {
	Generic   int        `json:"generic"`
	Pips      [][]string `json:"pips"`
	Phyrexian [][]string `json:"phyrexian"`
	HasX      bool       `json:"has_x"`
}

type cardJSON struct {
	Name         string       `json:"name"`
	Cost         costJSON     `json:"cost"`
	Category     string       `json:"category"`
	IsLand       bool         `json:"is_land"`
	EntersTapped bool         `json:"enters_tapped"`
	Produces     []sim.Source `json:"produces"`
	ProduceDelay int          `json:"produce_delay"`
	FetchesLands int          `json:"fetches_lands"`
}

// want turns a recorded card into the `sim.Card` the compiler must build.
//
// Every field is written in the corpus, `category` included. That is not
// tidiness: the recorded default for `category` is the word "utility" while
// Go's zero value is
// "" -- a field no tier reads and the ADR 18 cache key serialises. A corpus
// that
// omitted the field would have agreed with the wrong answer.
func (c cardJSON) want() sim.Card {
	produces := c.Produces
	if len(produces) == 0 {
		produces = nil
	}
	return sim.Card{
		Name: c.Name,
		Cost: sim.Cost{
			Generic:   c.Cost.Generic,
			Pips:      emptyToNil(c.Cost.Pips),
			Phyrexian: emptyToNil(c.Cost.Phyrexian),
			HasX:      c.Cost.HasX,
		},
		Category:     c.Category,
		IsLand:       c.IsLand,
		EntersTapped: c.EntersTapped,
		Produces:     produces,
		ProduceDelay: c.ProduceDelay,
		FetchesLands: c.FetchesLands,
	}
}

func emptyToNil(in [][]string) [][]string {
	if len(in) == 0 {
		return nil
	}
	return in
}

type reportJSON struct {
	Library             []cardJSON `json:"library"`
	Commander           *cardJSON  `json:"commander"`
	Unresolved          []string   `json:"unresolved"`
	DeclaredSize        int        `json:"declared_size"`
	SimulatedSize       int        `json:"simulated_size"`
	CommanderUnresolved bool       `json:"commander_unresolved"`
	Complete            bool       `json:"complete"`
}

type corpus struct {
	Note  string `json:"note"`
	Texts []struct {
		Source       string `json:"source"`
		Label        string `json:"label"`
		Text         string `json:"text"`
		EntersTapped bool   `json:"enters_tapped"`
		ManaProduced int    `json:"mana_produced"`
		FetchesLands int    `json:"fetches_lands"`
	} `json:"texts"`
	Records []struct {
		Label  string `json:"label"`
		Record struct {
			Name         string   `json:"name"`
			ManaCost     *string  `json:"mana_cost"`
			TypeLine     string   `json:"type_line"`
			OracleText   string   `json:"oracle_text"`
			ProducedMana []string `json:"produced_mana"`
			Layout       string   `json:"layout"`
			IsLand       bool     `json:"is_land"`
		} `json:"record"`
		Card cardJSON `json:"card"`
	} `json:"records"`
	Decks []struct {
		Name     string      `json:"name"`
		Why      string      `json:"why"`
		WithPool bool        `json:"with_pool"`
		YAML     string      `json:"yaml"`
		Report   *reportJSON `json:"report"`
		Error    string      `json:"error"`
		Message  string      `json:"message"`
	} `json:"decks"`
}

func load(t *testing.T) corpus {
	t.Helper()
	body, err := os.ReadFile("testdata/compile.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var c corpus
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(c.Texts) == 0 || len(c.Decks) == 0 {
		t.Fatal("the corpus is empty; testdata/compile.json is a frozen " +
			"golden -- restore it from version control")
	}
	return c
}

func TestTheTextReadersMatchTheCorpus(t *testing.T) {
	for _, tc := range load(t).Texts {
		t.Run(tc.Source+"/"+tc.Label, func(t *testing.T) {
			if got := compile.EntersTapped(tc.Text); got != tc.EntersTapped {
				t.Errorf("EntersTapped = %v, the corpus says %v\ntext: %q",
					got, tc.EntersTapped, tc.Text)
			}
			if got := compile.ManaProduced(tc.Text); got != tc.ManaProduced {
				t.Errorf("ManaProduced = %d, the corpus says %d\ntext: %q",
					got, tc.ManaProduced, tc.Text)
			}
			if got := compile.FetchesLands(tc.Text); got != tc.FetchesLands {
				t.Errorf("FetchesLands = %d, the corpus says %d\ntext: %q",
					got, tc.FetchesLands, tc.Text)
			}
		})
	}
}

// TestCardShapesThePoolDoesNotHold runs the compiler over records built by
// hand, and it is where four mutations died.
//
// The 21-card pool is a real pool, which is its whole value and also its
// limit: it holds no card whose front is an artifact and whose back is a
// creature, no land that is also a creature, no fetchland, and nothing whose
// `produced_mana` carries a string Scryfall would never send. Each of those
// decides one line here, and a deck fixture cannot reach any of them. So the
// corpus carries the records themselves and the recorded answer for each.
func TestCardShapesThePoolDoesNotHold(t *testing.T) {
	cases := load(t).Records
	if len(cases) == 0 {
		t.Fatal("the corpus carries no hand-built records any more")
	}
	for _, tc := range cases {
		t.Run(tc.Label, func(t *testing.T) {
			rec := &pool.CardRecord{
				Name:         tc.Record.Name,
				ManaCost:     tc.Record.ManaCost,
				TypeLine:     tc.Record.TypeLine,
				OracleText:   tc.Record.OracleText,
				ProducedMana: tc.Record.ProducedMana,
				Layout:       tc.Record.Layout,
			}
			// The record's own `is_land` is the recorded answer for it, so a
			// disagreement here is `internal/pool`'s rather than this
			// package's -- worth separating, since both feed the same field.
			if rec.IsLand() != tc.Record.IsLand {
				t.Fatalf("IsLand = %v, the corpus says %v (type line %q, layout %q)",
					rec.IsLand(), tc.Record.IsLand, rec.TypeLine, rec.Layout)
			}
			d := &deck.Deck{
				Slug:      "one-card",
				Commander: []string{},
				Cards:     []deck.CardEntry{{Name: rec.Name, Category: "utility", Qty: 1}},
			}
			library, commander, err := compile.Deck(d,
				map[string]*pool.CardRecord{rec.Name: rec})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if commander != nil {
				t.Errorf("a deck with no commander compiled one")
			}
			if len(library) != 1 {
				t.Fatalf("compiled %d cards, want 1", len(library))
			}
			if diff := cmp.Diff(tc.Card.want(), *library[0]); diff != "" {
				t.Errorf("differs (-corpus +got):\n%s", diff)
			}
		})
	}
}

func TestCompilingADeckMatchesTheCorpus(t *testing.T) {
	ctx := context.Background()
	p := pooltest.Open(t)
	cases := load(t).Decks

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			parsed, err := deck.FromText(tc.YAML, tc.Name)
			if err != nil {
				t.Fatalf("parse the deck: %v", err)
			}
			var cards map[string]*pool.CardRecord
			if tc.WithPool {
				names := append([]string{}, parsed.Commander...)
				for _, c := range parsed.Cards {
					names = append(names, c.Name)
				}
				if err := p.Use(ctx, func(conn *pool.Conn) error {
					var err error
					cards, err = conn.GetCards(ctx, names)
					return err
				}); err != nil {
					t.Fatalf("look the names up: %v", err)
				}
			}

			report, err := compile.Compile(parsed, cards)
			switch tc.Error {
			case "nothing_to_simulate":
				var want *compile.NothingToSimulate
				if !asNothing(err, &want) {
					t.Fatalf("want NothingToSimulate, got %v (report %v)", err, report)
				}
				if want.Error() != tc.Message {
					t.Errorf("message = %q, the corpus says %q", want.Error(), tc.Message)
				}
				return
			case "pool_required":
				var want *compile.PoolRequired
				if !asPool(err, &want) {
					t.Fatalf("want PoolRequired, got %v (report %v)", err, report)
				}
				if want.Error() != tc.Message {
					t.Errorf("message = %q, the corpus says %q", want.Error(), tc.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			if len(report.Library) != len(tc.Report.Library) {
				t.Fatalf("library has %d cards, the corpus says %d",
					len(report.Library), len(tc.Report.Library))
			}
			for i, want := range tc.Report.Library {
				if diff := cmp.Diff(want.want(), *report.Library[i]); diff != "" {
					t.Errorf("library[%d] (%s) differs (-corpus +got):\n%s",
						i, want.Name, diff)
				}
			}
			switch {
			case tc.Report.Commander == nil && report.Commander != nil:
				t.Errorf("commander is %q, the corpus has none", report.Commander.Name)
			case tc.Report.Commander != nil && report.Commander == nil:
				t.Errorf("commander is nil, the corpus has %q", tc.Report.Commander.Name)
			case tc.Report.Commander != nil:
				if diff := cmp.Diff(tc.Report.Commander.want(), *report.Commander); diff != "" {
					t.Errorf("commander differs (-corpus +got):\n%s", diff)
				}
			}
			if diff := cmp.Diff(tc.Report.Unresolved, report.Unresolved); diff != "" {
				t.Errorf("unresolved differs (-corpus +got):\n%s", diff)
			}
			if report.DeclaredSize != tc.Report.DeclaredSize {
				t.Errorf("declared %d, the corpus says %d",
					report.DeclaredSize, tc.Report.DeclaredSize)
			}
			if report.SimulatedSize() != tc.Report.SimulatedSize {
				t.Errorf("simulated %d, the corpus says %d",
					report.SimulatedSize(), tc.Report.SimulatedSize)
			}
			if report.CommanderUnresolved != tc.Report.CommanderUnresolved {
				t.Errorf("commanderUnresolved %v, the corpus says %v",
					report.CommanderUnresolved, tc.Report.CommanderUnresolved)
			}
			if report.Complete() != tc.Report.Complete {
				t.Errorf("complete %v, the corpus says %v",
					report.Complete(), tc.Report.Complete)
			}
		})
	}
}

// asNothing and asPool name the two refusals, so the switch above reads as
// the two named failure modes rather than as reflection.
func asNothing(err error, out **compile.NothingToSimulate) bool {
	return errors.As(err, out)
}

func asPool(err error, out **compile.PoolRequired) bool {
	return errors.As(err, out)
}

// TestQtyRepeatsShareOnePointer is the aliasing the qty expansion
// produces, asserted rather than assumed.
//
// It is not decoration. Tier 1 removes a card from a hand by first-EQUAL and
// compares the commander by identity, and a compiled deck's basics are the
// same object many times over. Allocating a fresh card per copy
// would be the obvious implementation and would quietly change what a `==` on
// pointers means anywhere above this package.
// It runs over **commander-in-the-99** as well as the legal deck, and the
// illegal one is the load-bearing half: with the commander absent from the
// library, "the commander is not an alias" is true of any implementation. The
// gate refuses that deck; the compiler has no opinion about it, which is what
// makes it usable here.
func TestQtyRepeatsShareOnePointer(t *testing.T) {
	ctx := context.Background()
	p := pooltest.Open(t)
	decks := map[string]*deck.Deck{}
	for _, tc := range load(t).Decks {
		if tc.Name != "mono-green" && tc.Name != "commander-in-the-99" {
			continue
		}
		parsed, err := deck.FromText(tc.YAML, tc.Name)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.Name, err)
		}
		decks[tc.Name] = parsed
	}
	if len(decks) != 2 {
		t.Fatalf("the corpus carries %d of the two decks this needs", len(decks))
	}

	for name, parsed := range decks {
		t.Run(name, func(t *testing.T) {
			names := append([]string{}, parsed.Commander...)
			for _, c := range parsed.Cards {
				names = append(names, c.Name)
			}
			var cards map[string]*pool.CardRecord
			if err := p.Use(ctx, func(conn *pool.Conn) error {
				var err error
				cards, err = conn.GetCards(ctx, names)
				return err
			}); err != nil {
				t.Fatalf("look the names up: %v", err)
			}
			report, err := compile.Compile(parsed, cards)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			seen := map[string]*sim.Card{}
			repeats := 0
			for _, card := range report.Library {
				first, ok := seen[card.Name]
				if !ok {
					seen[card.Name] = card
					continue
				}
				repeats++
				if first != card {
					t.Fatalf("%q is two different cards in one library; "+
						"the qty expansion puts the same object in "+
						"qty times", card.Name)
				}
			}
			if repeats == 0 {
				t.Fatal("no card repeats in this deck, so this proves nothing")
			}

			// The commander is a SEPARATE card even when the same name is in
			// the 99: the compiler builds it afresh, and Tier 1 matches it
			// by identity.
			if report.Commander == nil {
				t.Fatal("the deck lost its commander")
			}
			for _, card := range report.Library {
				if card == report.Commander {
					t.Fatal("the commander is an alias of a library card")
				}
			}
			if name == "commander-in-the-99" {
				var alsoInLibrary bool
				for _, card := range report.Library {
					if card.Name == report.Commander.Name {
						alsoInLibrary = true
						break
					}
				}
				if !alsoInLibrary {
					t.Fatal("this deck no longer has its commander in the 99, " +
						"so the aliasing check above proves nothing")
				}
			}
		})
	}
}

// TestTheCompilerAlwaysSetsTheCategory is `compile.Category`, pinned.
//
// A compiled card's `category` is invisible to every tier and visible in the
// cache key, so this is the one field whose wrongness has no failing test
// anywhere above it. `compile.json` records the value for every card, and this
// says the same thing in one line so the reason survives a corpus trim.
func TestTheCompilerAlwaysSetsTheCategory(t *testing.T) {
	if compile.Category != "utility" {
		t.Fatalf("Category is %q; the recorded default is \"utility\"",
			compile.Category)
	}
	for _, tc := range load(t).Decks {
		if tc.Report == nil {
			continue
		}
		for _, card := range tc.Report.Library {
			if card.Category != compile.Category {
				t.Fatalf("%s/%s carries category %q, not %q",
					tc.Name, card.Name, card.Category, compile.Category)
			}
		}
	}
}
