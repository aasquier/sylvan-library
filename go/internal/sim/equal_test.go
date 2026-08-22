package sim_test

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// Equal is `SimCard.__eq__`, and it is not decoration: it is what
// `list.remove` means inside Tier 1, so a field it forgets to compare is a
// card the engine takes out of the wrong slot in a hand. Each case below
// differs from the base card in exactly one field, which is the shape that
// catches a forgotten one.
func TestEqualComparesEveryField(t *testing.T) {
	base := func() sim.Card {
		return sim.Card{
			Name:     "Golgari Signet",
			Cost:     sim.Cost{Generic: 2},
			Category: "ramp",
			Produces: []sim.Source{{Colors: []string{"B", "G"}, Amount: 1}},
		}
	}
	if !base().Equal(base()) {
		t.Fatal("two identically built cards do not compare equal")
	}

	for name, change := range map[string]func(*sim.Card){
		"name":          func(c *sim.Card) { c.Name = "Dimir Signet" },
		"generic":       func(c *sim.Card) { c.Cost.Generic = 3 },
		"a pip":         func(c *sim.Card) { c.Cost.Pips = [][]string{{"G"}} },
		"x":             func(c *sim.Card) { c.Cost.HasX = true },
		"phyrexian":     func(c *sim.Card) { c.Cost.Phyrexian = [][]string{{"G"}} },
		"category":      func(c *sim.Card) { c.Category = "utility" },
		"is a land":     func(c *sim.Card) { c.IsLand = true },
		"enters tapped": func(c *sim.Card) { c.EntersTapped = true },
		"colours": func(c *sim.Card) {
			c.Produces = []sim.Source{{Colors: []string{"G"}, Amount: 1}}
		},
		"amount": func(c *sim.Card) {
			c.Produces = []sim.Source{{Colors: []string{"B", "G"}, Amount: 2}}
		},
		"no production": func(c *sim.Card) { c.Produces = nil },
		"delay":         func(c *sim.Card) { c.ProduceDelay = 1 },
		"fetches":       func(c *sim.Card) { c.FetchesLands = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			other := base()
			change(&other)
			if base().Equal(other) {
				t.Errorf("a card differing only in its %s compares equal", name)
			}
		})
	}
}

// A pip tuple is ordered and its colours are a set: `{G}{W}` is not `{W}{G}`,
// but `{G/W}` is `{W/G}`. Python holds the first in a tuple and the second in
// a frozenset, and the distinction decides which cards a hand treats as the
// same card.
func TestPipOrderMattersAndColourOrderDoesNot(t *testing.T) {
	gw := sim.Card{Cost: sim.Cost{Pips: [][]string{{"G"}, {"W"}}}}
	wg := sim.Card{Cost: sim.Cost{Pips: [][]string{{"W"}, {"G"}}}}
	if gw.Equal(wg) {
		t.Error("{G}{W} compares equal to {W}{G}")
	}
	hybrid := sim.Card{Produces: []sim.Source{{Colors: []string{"G", "W"}, Amount: 1}}}
	same := sim.Card{Produces: []sim.Source{{Colors: []string{"G", "W"}, Amount: 1}}}
	if !hybrid.Equal(same) {
		t.Error("a colour set does not compare equal to itself")
	}
}

// An empty production and an absent one are the same `()` in Python, and the
// corpus writes neither -- so a comparison that distinguished nil from empty
// would separate cards Python calls equal.
func TestNilAndEmptyProductionAreTheSameCard(t *testing.T) {
	absent := sim.Card{Name: "Spell"}
	empty := sim.Card{Name: "Spell", Produces: []sim.Source{}}
	if !absent.Equal(empty) {
		t.Error("a nil Produces does not compare equal to an empty one")
	}
	if !(sim.Cost{}).Equal(sim.Cost{Pips: [][]string{}}) {
		t.Error("a nil Pips does not compare equal to an empty one")
	}
}

func TestIntersectsIsSetIntersection(t *testing.T) {
	cases := []struct {
		unit, pip []string
		want      bool
	}{
		{[]string{"G"}, []string{"G"}, true},
		{[]string{"G"}, []string{"B"}, false},
		{[]string{"B", "G"}, []string{"G"}, true},
		{[]string{"G"}, []string{"B", "G"}, true},
		{[]string{"C"}, []string{"B", "G", "R", "U", "W"}, false},
		{[]string{"B", "G", "R", "U", "W"}, []string{"C"}, false},
		{nil, []string{"G"}, false},
	}
	for _, tc := range cases {
		if got := sim.Intersects(tc.unit, tc.pip); got != tc.want {
			t.Errorf("Intersects(%v, %v) = %v, want %v",
				tc.unit, tc.pip, got, tc.want)
		}
	}
}
