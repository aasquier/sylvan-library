package curve_test

// What the curve does when it is asked something the arithmetic cannot mean.
//
// These are not defensive lines for their own sake. The turn and the mana
// asked for come off a query string, so both are whatever somebody typed; the
// accelerant's shape is averaged over the deck's own pieces, and an average
// can round to a rock that makes no mana at all. Every one of them has to
// come back as a number a person can read rather than an empty answer, a
// negative recommendation or a divide by zero.

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/curve"
)

func TestATurnBeforeTheGameStartedHasNoDistributionToDraw(t *testing.T) {
	t.Parallel()
	// Turn zero is one bucket -- nought lands -- and anything below it is no
	// buckets at all rather than a slice of negative length.
	if got := curve.LandDistribution(99, 38, 0, true); len(got) != 1 {
		t.Fatalf("turn 0 gave %d buckets", len(got))
	}
	for _, turn := range []int{-1, -5, -99} {
		got := curve.LandDistribution(99, 38, turn, true)
		if len(got) != 0 {
			t.Fatalf("turn %d gave %d buckets", turn, len(got))
		}
	}
	// And a real turn still answers a real distribution that sums to one, so
	// the above is not passing on a function that returns nothing.
	dist := curve.LandDistribution(99, 38, 4, true)
	if len(dist) != 5 {
		t.Fatalf("turn 4 gave %d buckets", len(dist))
	}
	total := 0.0
	for _, p := range dist {
		total += p
	}
	if total < 0.999 || total > 1.001 {
		t.Fatalf("the distribution sums to %v", total)
	}
}

func TestAskingForMoreManaThanAnyGameHasIsCappedRatherThanRefused(t *testing.T) {
	t.Parallel()
	// The number comes off a query string. Twenty is the ceiling and one is
	// the floor, and both answer the same as asking for the bound itself --
	// which is how a newcomer who typed 500 gets an answer rather than a
	// blank page.
	library := make([]sim.Card, 0, 99)
	for i := 0; i < 38; i++ {
		library = append(library, sim.Card{Name: "Synthetic Waste", IsLand: true,
			Produces: []sim.Source{{Colors: []string{"C"}, Amount: 1}}})
	}
	for i := 0; i < 61; i++ {
		library = append(library, sim.Card{Name: "Synthetic Spell",
			Cost: sim.Cost{Generic: 3}})
	}
	ask := func(mana int) curve.ManaCurve {
		opts := curve.DefaultOptions()
		opts.TargetMana = &mana
		return curve.Curve(library, opts)
	}
	huge, capped := ask(500), ask(20)
	if huge.TargetMana != capped.TargetMana {
		t.Fatalf("asking for 500 answered about %d mana, the cap answered %d",
			huge.TargetMana, capped.TargetMana)
	}
	if capped.TargetMana != 20 {
		t.Fatalf("the cap is %d", capped.TargetMana)
	}
	tiny, floor := ask(-4), ask(1)
	if tiny.TargetMana != floor.TargetMana || floor.TargetMana != 1 {
		t.Fatalf("below the floor answered about %d mana, the floor answered %d",
			tiny.TargetMana, floor.TargetMana)
	}
}

func TestAnAccelerantThatMakesNothingIsStillWorthOneMana(t *testing.T) {
	t.Parallel()
	// "One more accelerant like the ones this deck already plays" is an
	// average, and an average over pieces the pool could not price rounds to
	// nothing. A rock that makes no mana is not advice -- it would put a
	// zero in the denominator of the ramp question -- so the output is
	// floored at one.
	library := []sim.Card{}
	for i := 0; i < 4; i++ {
		library = append(library, sim.Card{
			Name:     "Synthetic Cipher",
			Cost:     sim.Cost{Generic: 1},
			Produces: []sim.Source{{Colors: []string{"C"}, Amount: 0}},
		})
	}
	piece, standIn := curve.TypicalAccelerant(library, 4)
	if standIn {
		t.Fatal("a deck with four accelerants was told it had none")
	}
	if piece.Output < 1 {
		t.Fatalf("the typical accelerant makes %d mana", piece.Output)
	}
	if piece.Cost != 1 || piece.Delay != 0 {
		t.Fatalf("the typical accelerant is %+v", piece)
	}
	// A deck with nothing that could be online by the turn asked about gets
	// the stand-in, and says so.
	rock, isStandIn := curve.TypicalAccelerant(library, 0)
	if !isStandIn || rock != curve.GenericRock() {
		t.Fatalf("a deck with no usable accelerant got %+v (stand-in: %v)", rock, isStandIn)
	}
}
