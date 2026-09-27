package karsten_test

// The shelf's arithmetic where the deck runs out from under it.
//
// Every number here is read by somebody mid-build, which is the state where
// the inputs stop making sense: a colour nothing demands yet, a library with
// nothing in it, a pile that is all cheap ramp and no spells. The closed form
// divides, so each of those is a crash or a NaN unless something catches it,
// and commandment 2 says what a newcomer sees instead is a number.

import (
	"math/big"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
)

func TestAPopulationThatCannotBeDrawnFromAnswersZeroRatherThanDividingByIt(t *testing.T) {
	t.Parallel()
	// The total is a binomial coefficient and it becomes the denominator of
	// the one division this package does. Out of range it is zero, and a
	// rational over zero is a panic rather than a wrong number -- so the
	// answer has to be refused before the division, not after.
	if got := karsten.HypergeometricAtLeast(-1, 1, 1, 1); got != 0 {
		t.Fatalf("an impossible population answered %v", got)
	}
	if got := karsten.Exactly(-1, -1, 5, -1); got != 0 {
		t.Fatalf("an impossible population answered %v exactly", got)
	}
	// And the ordinary questions still answer, so the above is not passing on
	// arithmetic that returns zero to everything.
	if got := karsten.HypergeometricAtLeast(99, 38, 7, 3); got <= 0 || got >= 1 {
		t.Fatalf("three lands in an opening seven = %v", got)
	}
}

func TestAChanceTooSmallToSeparateFromZeroIsReportedAsZeroRatherThanNegative(t *testing.T) {
	t.Parallel()
	// The short sum is the complement, so the answer is `1 - P(none)`. Past
	// about sixteen digits the complement rounds to exactly one and the
	// subtraction lands on zero -- or, with the wrong comparison, on negative
	// zero, which renders as "-0.0" on a page. The floor is taken only when
	// the value is not strictly greater than zero, which is the distinction.
	huge := 1 << 60
	got := karsten.HypergeometricAtLeast(huge, 1, 1, 1)
	if got != 0 {
		t.Fatalf("one card in 2^60, one draw = %v", got)
	}
	if got := big.NewFloat(got).Signbit(); got {
		t.Fatal("the answer is negative zero")
	}
}

func TestAColourNothingDemandsHasNoWorstRungAndNoShortfall(t *testing.T) {
	t.Parallel()
	// A deck mid-build holds sources for a colour before it holds any card
	// that asks for one. The ladder is empty, and an empty ladder has no
	// hardest rung -- which the caller has to be told rather than handed a
	// zeroed one it cannot tell from a rung that is met.
	bare := karsten.ColorRequirement{Color: "G", Have: 12, HaveLands: 10}
	if worst, ok := bare.Worst(); ok {
		t.Fatalf("an empty ladder named %+v as its worst rung", worst)
	}
	if got := bare.Shortfall(); got != 0 {
		t.Fatalf("an empty ladder is %d sources short", got)
	}
	if !bare.Met() {
		t.Fatal("a colour nothing demands is not met")
	}
	// A ladder with rungs names the one that fails hardest, and a tie on the
	// shortfall is broken by the more demanding rung.
	ladder := karsten.ColorRequirement{Color: "G", Have: 5, Tiers: []karsten.PipTier{
		{Pips: 1, Turn: 1, Need: 17, Have: 5},
		{Pips: 2, Turn: 2, Need: 14, Have: 5},
		{Pips: 3, Turn: 3, Need: 14, Have: 5},
	}}
	worst, ok := ladder.Worst()
	if !ok || worst.Pips != 1 {
		t.Fatalf("the worst rung is %+v (%v), want the one pip short by twelve", worst, ok)
	}
	if got := ladder.Shortfall(); got != 12 {
		t.Fatalf("the shortfall is %d, want 12", got)
	}
	tied := karsten.ColorRequirement{Color: "G", Have: 5, Tiers: ladder.Tiers[1:]}
	if worst, _ := tied.Worst(); worst.Pips != 3 {
		t.Fatalf("a tie named %+v, want the three-pip rung", worst)
	}
}

func TestACardInAnEmptyLibraryIsNeverCastable(t *testing.T) {
	t.Parallel()
	// The pool is counted as a fraction of the deck, so an empty deck is the
	// denominator gone. Zero is the honest answer: there is nothing to draw.
	card := sim.Card{Name: "Synthetic Bolt", Cost: sim.Cost{Pips: [][]string{{"R"}}}}
	if got := karsten.CastableOdds(card, nil, 1, true); got != 0 {
		t.Fatalf("an empty library cast it %v of the time", got)
	}
	// A card that costs more than the turn allows is zero for a different
	// reason, and both have to be zero rather than one of them a panic.
	if got := karsten.CastableOdds(sim.Card{Cost: sim.Cost{Generic: 6}},
		[]sim.Card{{Name: "Synthetic Waste", IsLand: true}}, 1, true); got != 0 {
		t.Fatalf("a six-drop was cast on turn one %v of the time", got)
	}
}

func TestADeckThatIsAllCheapRampIsNeverToldToPlayNegativeLands(t *testing.T) {
	t.Parallel()
	// The published fit subtracts for every cheap accelerant, and a pile of
	// ninety-nine of them drives it below zero. A negative recommendation is
	// not a recommendation; the floor is nought, and the caveats still say
	// the fit was made for a different format.
	library := make([]sim.Card, 0, 99)
	for i := 0; i < 99; i++ {
		library = append(library, sim.Card{
			Name:     "Synthetic Dork",
			Cost:     sim.Cost{Pips: [][]string{{"G"}}},
			Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}},
		})
	}
	est := karsten.RegressionLands(library)
	if est.Recommended != 0 {
		t.Fatalf("a deck of ninety-nine one-mana dorks was told to play %d lands",
			est.Recommended)
	}
	if est.CheapAccelerants != 99 || est.LandsNow != 0 || est.DeckSize != 99 {
		t.Fatalf("the fit read the deck as %+v", est)
	}
	if len(est.Caveats) == 0 {
		t.Fatal("the fit reported no caveats")
	}
	// And a deck the fit was made for gets a number above the floor, so the
	// clamp is not swallowing every answer.
	ordinary := make([]sim.Card, 0, 99)
	for i := 0; i < 99; i++ {
		ordinary = append(ordinary, sim.Card{Name: "Synthetic Spell",
			Cost: sim.Cost{Generic: 3}})
	}
	if got := karsten.RegressionLands(ordinary).Recommended; got < 20 {
		t.Fatalf("a three-mana pile was told to play %d lands", got)
	}
}
