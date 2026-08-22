package mulligan_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/mulligan"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// `sim/mulligan.py`, held to Python by `testdata/mulligan.json` (written by
// `python tests/go_fixtures.py`) over the same fixture decks the closed forms
// use.
//
// # Every float is compared as bits, and here is why that is not excessive
//
// This module produces two kinds of number and both are read by a comparison
// rather than by a person. `SpellsThroughT8` is the sort key of the table AND
// the operand of `Gain`, which is then read against `Flat` -- so one ulp is a
// different recommended rule, or a sweep that reports `flat` where Python
// reports a winner. `MulliganRate` is the tie-breaker on both `Best` and
// `Gentlest`. There is no field here whose last decimal is decoration, which
// is exactly `karsten`'s argument for the same choice.
//
// # What the corpus deliberately contains
//
// A sweep whose grid **excludes the default rule**, which is the only way to
// reach the thirty-fourth simulation `search` runs when the baseline is not a
// cell of the grid. A sweep where every row **ties**, which is where `Best` is
// decided by the grid's order rather than by the numbers. And at least one
// sweep of each `flat` verdict, since the flat branch and the decided branch
// are different sentences on the screen.

type numberJSON struct {
	IsInt bool    `json:"is_int"`
	Value float64 `json:"value"`
}

// want is the `tier1.Number` Python recorded. The float leg arrives as bits
// and the int leg as itself, because `statistics.median` really does answer
// two types and the type is part of the answer.
func (n *numberJSON) want() *tier1.Number {
	if n == nil {
		return nil
	}
	if n.IsInt {
		return &tier1.Number{Int: int(n.Value)}
	}
	return &tier1.Number{IsFloat: true, Float: math.Float64frombits(uint64(n.Value))}
}

type rowJSON struct {
	MinLands            int         `json:"min_lands"`
	MaxLands            int         `json:"max_lands"`
	MinPieces           int         `json:"min_pieces"`
	SpellsThroughT8     uint64      `json:"spells_through_t8"`
	MulliganRate        uint64      `json:"mulligan_rate"`
	AvgMulligans        uint64      `json:"avg_mulligans"`
	MedianCommanderTurn *numberJSON `json:"median_commander_turn"`
	ColorScrewRate      uint64      `json:"color_screw_rate"`
	StalledTurns        uint64      `json:"stalled_turns"`
	Describe            string      `json:"describe"`
}

type ruleJSON struct {
	MinLands      int `json:"min_lands"`
	MaxLands      int `json:"max_lands"`
	MinManaPieces int `json:"min_mana_pieces"`
	CheapRampMV   int `json:"cheap_ramp_mv"`
	MaxMulligans  int `json:"max_mulligans"`
	Describe      string
}

type sweepJSON struct {
	Label    string     `json:"label"`
	Deck     string     `json:"deck"`
	Why      string     `json:"why"`
	Games    int        `json:"games"`
	Turns    int        `json:"turns"`
	Seed     int        `json:"seed"`
	Rules    []ruleJSON `json:"rules"`
	Rows     []rowJSON  `json:"rows"`
	Best     rowJSON    `json:"best"`
	Baseline rowJSON    `json:"baseline"`
	Spread   uint64     `json:"spread"`
	Gain     uint64     `json:"gain"`
	Flat     bool       `json:"flat"`
	Gentlest rowJSON    `json:"gentlest"`
}

type deckJSON struct {
	Name string `json:"name"`
	Why  string `json:"why"`
	// `sim.Card`'s own JSON tags do the decoding, exactly as the closed forms'
	// corpora decode theirs -- one shape, written once by `_card_json`.
	Library   []sim.Card `json:"library"`
	Commander *sim.Card  `json:"commander"`
}

type corpus struct {
	Note       string     `json:"note"`
	Decks      []deckJSON `json:"decks"`
	Through    int        `json:"through"`
	Flat       uint64     `json:"flat"`
	MinLands   []int      `json:"min_lands"`
	MaxLands   []int      `json:"max_lands"`
	MinPieces  []int      `json:"min_pieces"`
	Candidates []struct {
		ruleJSON
		Describe string `json:"describe"`
	} `json:"candidates"`
	Sweeps []sweepJSON `json:"sweeps"`
}

type deckSpec struct {
	library   []*sim.Card
	commander *sim.Card
}

// simDecks is the corpus's decks as Tier 1 wants them: pointers, because the
// engine removes by first-equal and compares the commander by identity.
//
// Each card gets its own allocation here, where `sim/compile` would alias the
// `qty` repeats. That difference is invisible to Tier 1 -- nothing mutates a
// compiled card -- and the corpus decks are written out card by card anyway,
// with no `qty` to expand.
func simDecks(t *testing.T) map[string]deckSpec {
	t.Helper()
	out := map[string]deckSpec{}
	for _, d := range load(t).Decks {
		spec := deckSpec{commander: d.Commander}
		for i := range d.Library {
			spec.library = append(spec.library, &d.Library[i])
		}
		out[d.Name] = spec
	}
	if len(out) == 0 {
		t.Fatal("the corpus carries no decks; regenerate the fixtures")
	}
	return out
}

func load(t *testing.T) corpus {
	t.Helper()
	body, err := os.ReadFile("testdata/mulligan.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var c corpus
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(c.Sweeps) == 0 {
		t.Fatal("the corpus is empty; regenerate with `python tests/go_fixtures.py`")
	}
	return c
}

func TestTheConstantsAreThePythonOnes(t *testing.T) {
	c := load(t)
	if mulligan.Through != c.Through {
		t.Errorf("Through = %d, Python says %d", mulligan.Through, c.Through)
	}
	if got := math.Float64bits(mulligan.Flat); got != c.Flat {
		t.Errorf("Flat = %v (bits %#x), Python says bits %#x",
			mulligan.Flat, got, c.Flat)
	}
	for name, pair := range map[string][2][]int{
		"MinLands":  {mulligan.MinLands, c.MinLands},
		"MaxLands":  {mulligan.MaxLands, c.MaxLands},
		"MinPieces": {mulligan.MinPieces, c.MinPieces},
	} {
		if len(pair[0]) != len(pair[1]) {
			t.Errorf("%s = %v, Python says %v", name, pair[0], pair[1])
			continue
		}
		for i := range pair[0] {
			if pair[0][i] != pair[1][i] {
				t.Errorf("%s = %v, Python says %v", name, pair[0], pair[1])
				break
			}
		}
	}
}

// TestTheGridIsPythonsGrid checks the rules AND their order.
//
// The order is real input, not presentation: `Best` keeps the first maximum,
// so two cells that tie on deployment and on mulligan rate are separated by
// nothing except which came first out of the nested loops.
func TestTheGridIsPythonsGrid(t *testing.T) {
	c := load(t)
	got := mulligan.Candidates()
	if len(got) != len(c.Candidates) {
		t.Fatalf("the grid holds %d rules, Python says %d",
			len(got), len(c.Candidates))
	}
	for i, want := range c.Candidates {
		g := got[i]
		if g.MinLands != want.MinLands || g.MaxLands != want.MaxLands ||
			g.MinManaPieces != want.MinManaPieces ||
			g.CheapRampMV != want.CheapRampMV ||
			g.MaxMulligans != want.MaxMulligans {
			t.Errorf("rule %d = %+v, Python says %+v", i, g, want.ruleJSON)
		}
		if g.Describe() != want.Describe {
			t.Errorf("rule %d describes as %q, Python says %q",
				i, g.Describe(), want.Describe)
		}
	}
}

func TestEverySweepAgreesWithPython(t *testing.T) {
	c := load(t)
	decks := simDecks(t)
	for _, want := range c.Sweeps {
		t.Run(want.Label, func(t *testing.T) {
			spec, ok := decks[want.Deck]
			if !ok {
				t.Fatalf("the closed-form corpus no longer carries %q", want.Deck)
			}
			opts := mulligan.Options{Games: want.Games, Turns: want.Turns,
				Seed: want.Seed}
			for _, r := range want.Rules {
				opts.Rules = append(opts.Rules, tier1.KeepRule{
					MinLands: r.MinLands, MaxLands: r.MaxLands,
					MinManaPieces: r.MinManaPieces, CheapRampMV: r.CheapRampMV,
					MaxMulligans: r.MaxMulligans,
				})
			}
			got, err := mulligan.Search(spec.library, spec.commander, opts)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if got.Games != want.Games || got.Turns != want.Turns || got.Seed != want.Seed {
				t.Errorf("parameters echoed back as %d/%d/%d, Python says %d/%d/%d",
					got.Games, got.Turns, got.Seed, want.Games, want.Turns, want.Seed)
			}
			if len(got.Rows) != len(want.Rows) {
				t.Fatalf("%d rows, Python says %d", len(got.Rows), len(want.Rows))
			}
			for i := range want.Rows {
				checkRow(t, "rows["+itoa(i)+"]", got.Rows[i], want.Rows[i])
			}
			checkRow(t, "best", got.Best, want.Best)
			checkRow(t, "baseline", got.Baseline, want.Baseline)
			checkRow(t, "gentlest", got.Gentlest(), want.Gentlest)
			checkFloat(t, "spread", got.Spread, want.Spread)
			checkFloat(t, "gain", got.Gain(), want.Gain)
			if got.IsFlat() != want.Flat {
				t.Errorf("flat = %v, Python says %v (gain %v)",
					got.IsFlat(), want.Flat, got.Gain())
			}
		})
	}
}

func checkRow(t *testing.T, what string, got mulligan.Row, want rowJSON) {
	t.Helper()
	if got.MinLands != want.MinLands || got.MaxLands != want.MaxLands ||
		got.MinPieces != want.MinPieces {
		t.Errorf("%s is rule %d-%d/%d, Python says %d-%d/%d", what,
			got.MinLands, got.MaxLands, got.MinPieces,
			want.MinLands, want.MaxLands, want.MinPieces)
		return
	}
	checkFloat(t, what+".spells_through_t8", got.SpellsThroughT8, want.SpellsThroughT8)
	checkFloat(t, what+".mulligan_rate", got.MulliganRate, want.MulliganRate)
	checkFloat(t, what+".avg_mulligans", got.AvgMulligans, want.AvgMulligans)
	checkFloat(t, what+".color_screw_rate", got.ColorScrewRate, want.ColorScrewRate)
	checkFloat(t, what+".stalled_turns", got.StalledTurns, want.StalledTurns)
	if got.Describe != want.Describe {
		t.Errorf("%s.describe = %q, Python says %q", what, got.Describe, want.Describe)
	}
	wantMedian := want.MedianCommanderTurn.want()
	switch {
	case wantMedian == nil && got.MedianCommanderTurn != nil:
		t.Errorf("%s.median_commander_turn = %v, Python says None",
			what, *got.MedianCommanderTurn)
	case wantMedian != nil && got.MedianCommanderTurn == nil:
		t.Errorf("%s.median_commander_turn is nil, Python says %v", what, *wantMedian)
	case wantMedian != nil && *got.MedianCommanderTurn != *wantMedian:
		t.Errorf("%s.median_commander_turn = %+v, Python says %+v",
			what, *got.MedianCommanderTurn, *wantMedian)
	}
}

func checkFloat(t *testing.T, what string, got float64, want uint64) {
	t.Helper()
	if bits := math.Float64bits(got); bits != want {
		t.Errorf("%s = %v (bits %#x), Python says %v (bits %#x)",
			what, got, bits, math.Float64frombits(want), want)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestSearchRefusesAnEmptyGrid is `ValueError("no keep rules to search")`.
//
// A caller can hand `Rules` an empty non-nil slice, which is a different thing
// from handing it nil -- nil means "use the grid" and empty means "there is
// nothing to run". Python distinguishes them with `rules is not None`, and Go
// with a nil check rather than a length check, which is the line that is easy
// to write the other way.
func TestSearchRefusesAnEmptyGrid(t *testing.T) {
	if _, err := mulligan.Search(nil, nil, mulligan.Options{
		Games: 1, Turns: 1, Seed: 1, Rules: []tier1.KeepRule{},
	}); !errors.Is(err, mulligan.ErrNoRules) {
		t.Fatalf("an empty grid gave %v, want ErrNoRules", err)
	}
}

// TestProgressIsCalledAsPythonCallsIt: once before each rule, and once more at
// the end. It consumes no randomness, so a watched sweep and a plain one must
// answer identically -- which is also asserted here rather than assumed.
func TestProgressIsCalledAsPythonCallsIt(t *testing.T) {
	library := []*sim.Card{
		{Name: "Forest", IsLand: true, Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}},
		{Name: "Bear", Cost: sim.Cost{Generic: 1, Pips: [][]string{{"G"}}}},
	}
	rules := mulligan.Candidates()[:3]
	var seen [][2]int
	watched, err := mulligan.Search(library, nil, mulligan.Options{
		Games: 5, Turns: 3, Seed: 2, Rules: rules,
		Progress: func(done, total int) { seen = append(seen, [2]int{done, total}) },
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := [][2]int{{0, 3}, {1, 3}, {2, 3}, {3, 3}}
	if len(seen) != len(want) {
		t.Fatalf("progress called %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("progress called %v, want %v", seen, want)
		}
	}
	plain, err := mulligan.Search(library, nil, mulligan.Options{
		Games: 5, Turns: 3, Seed: 2, Rules: rules,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if watched.Best != plain.Best {
		t.Fatal("watching the sweep changed its answer")
	}
}
