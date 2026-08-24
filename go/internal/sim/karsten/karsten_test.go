package karsten_test

import (
	"encoding/json"
	"math"
	"math/big"
	"math/bits"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
)

// `sim/karsten.py`, held to Python by `testdata/karsten.json` (written by
// `python tests/go_fixtures.py`), and to arithmetic that came from neither by
// an exhaustive enumeration in exact rationals.
//
// # The epsilon, pinned per function
//
// PLAN section 5 item 4 asks for "an epsilon pinned per function", and a
// single global tolerance is the lazy answer because it hides which function
// is drifting. Every one below is pinned at **zero -- the float64 bits must
// match** -- and that is a stronger claim than the plan asked for, so each
// carries the reason it is affordable *and* the reason it is wanted. The
// short version of "wanted", for all of them: every integer this package
// produces comes out of a `>=` against one of these floats, so a tolerance
// large enough to absorb a rounding difference is also large enough to hide a
// different answer.
const (
	// hypergeometricAtLeast: exact `big.Int` binomials summed exactly, then
	// one `big.Rat` division that is correctly rounded to the same definition
	// CPython's int/int true division uses. There is no float arithmetic
	// before the last operation, so there is nothing for an epsilon to absorb.
	// Wanted because `RequiredSources` scans against `>= target`: one ulp is
	// one more land in the requirement.
	epsilonHypergeometric = 0.0

	// exactly: the same, one division and no sum.
	epsilonExactly = 0.0

	// castableOdds: the first function with real float arithmetic -- a chain
	// of multiplications per term, then `Fsum` over up to a hundred terms.
	// Exact anyway, because the multiplication order is Python's (the pip
	// buckets keep first-appearance order, which is what makes float
	// multiplication's non-associativity harmless) and `floats.Fsum` is CPython's
	// own summation. Wanted because `CardOdds.ReliableTurn` compares this
	// against 0.90, and one ulp there moves a card's `Lateness`, which is the
	// shelf's sort key.
	epsilonCastableOdds = 0.0

	// regressionLands: a fitted line over an exact integer mean. Its float
	// output is `average_mana_value`, rounded to two places by `floats.RoundTo`;
	// its integer output is a land count rounded ties-to-even. Exact because
	// the three constants are the same doubles and the evaluation order is the
	// same, with the fused multiply-add guarded at both product sites.
	epsilonRegressionLands = 0.0

	// shelf: no arithmetic of its own -- it is the two above, arranged. Pinned
	// separately anyway, because the arrangement is an output: `Odds` is
	// sorted by lateness, and a port with every probability right and the
	// comparator wrong would pass every other line of this file.
	epsilonShelf = 0.0
)

// ------------------------------------------------------------- the corpus

type corpusCard = sim.Card

type deckCase struct {
	Name      string       `json:"name"`
	Why       string       `json:"why"`
	Library   []corpusCard `json:"library"`
	Commander *corpusCard  `json:"commander"`
}

type tierCase struct {
	Pips      int      `json:"pips"`
	Turn      int      `json:"turn"`
	Need      int      `json:"need"`
	Have      int      `json:"have"`
	Met       bool     `json:"met"`
	Shortfall int      `json:"shortfall"`
	OddsNow   float64  `json:"odds_now"`
	Cards     []string `json:"cards"`
}

type colorCase struct {
	Color     string     `json:"color"`
	Have      int        `json:"have"`
	HaveLands int        `json:"have_lands"`
	Met       bool       `json:"met"`
	Shortfall int        `json:"shortfall"`
	Tiers     []tierCase `json:"tiers"`
}

type estimateCase struct {
	LandsNow         int      `json:"lands_now"`
	Recommended      int      `json:"recommended"`
	Delta            int      `json:"delta"`
	AverageManaValue float64  `json:"average_mana_value"`
	CheapAccelerants int      `json:"cheap_accelerants"`
	DeckSize         int      `json:"deck_size"`
	Caveats          []string `json:"caveats"`
}

type oddsCase struct {
	Name         string    `json:"name"`
	MV           int       `json:"mv"`
	ByTurn       []float64 `json:"by_turn"`
	OnCurve      *float64  `json:"on_curve"`
	ReliableTurn *int      `json:"reliable_turn"`
	Lag          *int      `json:"lag"`
	Lateness     int       `json:"lateness"`
}

type shelfCase struct {
	DeckSize     int          `json:"deck_size"`
	Lands        int          `json:"lands"`
	Target       float64      `json:"target"`
	OnThePlay    bool         `json:"on_the_play"`
	Colors       []colorCase  `json:"colors"`
	LandEstimate estimateCase `json:"land_estimate"`
	Odds         []oddsCase   `json:"odds"`
	Approximated []string     `json:"approximated"`
	Unmet        []string     `json:"unmet"`
}

type karstenCorpus struct {
	Target         float64    `json:"target"`
	Horizon        int        `json:"horizon"`
	Hypergeometric [][]any    `json:"hypergeometric"`
	Exactly        [][]any    `json:"exactly"`
	CardsSeen      [][]any    `json:"cards_seen"`
	RequiredSource [][]any    `json:"required_sources"`
	Decks          []deckCase `json:"decks"`
	SourcesFor     []struct {
		Deck   string   `json:"deck"`
		Colors []string `json:"colors"`
		Value  int      `json:"value"`
	} `json:"sources_for"`
	CastableOdds []struct {
		Deck      string    `json:"deck"`
		Card      string    `json:"card"`
		OnThePlay bool      `json:"on_the_play"`
		ByTurn    []float64 `json:"by_turn"`
	} `json:"castable_odds"`
	RegressionLands []struct {
		Deck  string       `json:"deck"`
		Value estimateCase `json:"value"`
	} `json:"regression_lands"`
	Shelves []struct {
		Deck      string    `json:"deck"`
		Target    float64   `json:"target"`
		OnThePlay bool      `json:"on_the_play"`
		Value     shelfCase `json:"value"`
	} `json:"shelves"`
}

func load(t *testing.T) (karstenCorpus, map[string]deckCase) {
	t.Helper()
	raw, err := os.ReadFile("testdata/karsten.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var corpus karstenCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the corpus: %v", err)
	}
	byName := map[string]deckCase{}
	for _, d := range corpus.Decks {
		byName[d.Name] = d
	}
	return corpus, byName
}

// A JSON number decodes into `any` as a float64, which is exact for every
// integer these tables hold and for every probability, because Python wrote
// them through `repr` -- the shortest string that round-trips -- and Go reads
// them with `ParseFloat`, which is correctly rounded.
func asInt(v any) int       { return int(v.(float64)) }
func asFloat(v any) float64 { return v.(float64) }
func asBool(v any) bool     { return v.(bool) }

func exact(got, want, epsilon float64) bool {
	if epsilon == 0 {
		return math.Float64bits(got) == math.Float64bits(want)
	}
	return math.Abs(got-want) <= epsilon
}

// exactFloats makes go-cmp compare float64 by bits, so a positive and a
// negative zero are told apart and a one-ulp drift is a failure rather than a
// rounding.
func exactFloats(epsilon float64) cmp.Option {
	return cmp.Comparer(func(a, b float64) bool { return exact(a, b, epsilon) })
}

// ------------------------------------------------- the arithmetic, differentially

func TestTheConstantsAreStillPythons(t *testing.T) {
	corpus, _ := load(t)
	if corpus.Target != karsten.Target {
		t.Errorf("Target = %v, Python = %v", karsten.Target, corpus.Target)
	}
	if corpus.Horizon != karsten.Horizon {
		t.Errorf("Horizon = %v, Python = %v", karsten.Horizon, corpus.Horizon)
	}
}

func TestHypergeometricAtLeastMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	if len(corpus.Hypergeometric) < 2000 {
		t.Fatalf("the grid has shrunk to %d rows", len(corpus.Hypergeometric))
	}
	for _, row := range corpus.Hypergeometric {
		pop, suc, dra, wan := asInt(row[0]), asInt(row[1]), asInt(row[2]), asInt(row[3])
		want := asFloat(row[4])
		got := karsten.HypergeometricAtLeast(pop, suc, dra, wan)
		if !exact(got, want, epsilonHypergeometric) {
			t.Errorf("HypergeometricAtLeast(%d, %d, %d, %d) = %v (%#016x), Python = %v (%#016x)",
				pop, suc, dra, wan, got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
}

func TestExactlyMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	for _, row := range corpus.Exactly {
		pop, suc, dra, cnt := asInt(row[0]), asInt(row[1]), asInt(row[2]), asInt(row[3])
		want := asFloat(row[4])
		got := karsten.Exactly(pop, suc, dra, cnt)
		if !exact(got, want, epsilonExactly) {
			t.Errorf("Exactly(%d, %d, %d, %d) = %v, Python = %v", pop, suc, dra, cnt, got, want)
		}
	}
}

func TestCardsSeenMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	for _, row := range corpus.CardsSeen {
		turn, otp, want := asInt(row[0]), asBool(row[1]), asInt(row[2])
		if got := karsten.CardsSeen(turn, otp); got != want {
			t.Errorf("CardsSeen(%d, %v) = %d, Python = %d", turn, otp, got, want)
		}
	}
}

func TestRequiredSourcesMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	if len(corpus.RequiredSource) < 2000 {
		t.Fatalf("the grid has shrunk to %d rows", len(corpus.RequiredSource))
	}
	for _, row := range corpus.RequiredSource {
		deckSize, pips, turn := asInt(row[0]), asInt(row[1]), asInt(row[2])
		target, otp, want := asFloat(row[3]), asBool(row[4]), asInt(row[5])
		got := karsten.RequiredSources(deckSize, pips, turn, target, otp)
		if got != want {
			t.Errorf("RequiredSources(%d, %d, %d, %v, %v) = %d, Python = %d",
				deckSize, pips, turn, target, otp, got, want)
		}
	}
}

func TestSourcesForMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.SourcesFor {
		want := map[string]bool{}
		for _, c := range row.Colors {
			want[c] = true
		}
		got := karsten.SourcesFor(decks[row.Deck].Library, want)
		if got != row.Value {
			t.Errorf("SourcesFor(%s, %v) = %d, Python = %d", row.Deck, row.Colors, got, row.Value)
		}
	}
}

func TestCastableOddsMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	if len(corpus.CastableOdds) < 50 {
		t.Fatalf("the probe set has shrunk to %d rows", len(corpus.CastableOdds))
	}
	for _, row := range corpus.CastableOdds {
		deck := decks[row.Deck]
		var card *sim.Card
		for i := range deck.Library {
			if deck.Library[i].Name == row.Card {
				card = &deck.Library[i]
				break
			}
		}
		if card == nil && deck.Commander != nil && deck.Commander.Name == row.Card {
			card = deck.Commander
		}
		if card == nil {
			t.Fatalf("%s: the corpus names a card %q the deck does not hold", row.Deck, row.Card)
		}
		for i, want := range row.ByTurn {
			got := karsten.CastableOdds(*card, deck.Library, i+1, row.OnThePlay)
			if !exact(got, want, epsilonCastableOdds) {
				t.Errorf("CastableOdds(%s/%s, turn %d, on_the_play=%v) = %v (%#016x), Python = %v (%#016x)",
					row.Deck, row.Card, i+1, row.OnThePlay,
					got, math.Float64bits(got), want, math.Float64bits(want))
			}
		}
	}
}

func TestRegressionLandsMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.RegressionLands {
		got := projectEstimate(karsten.RegressionLands(decks[row.Deck].Library))
		if diff := cmp.Diff(row.Value, got, exactFloats(epsilonRegressionLands)); diff != "" {
			t.Errorf("RegressionLands(%s) differs from Python (-python +go):\n%s", row.Deck, diff)
		}
	}
}

func TestTheWholeShelfMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	if len(corpus.Shelves) < 10 {
		t.Fatalf("the shelf set has shrunk to %d decks", len(corpus.Shelves))
	}
	for _, row := range corpus.Shelves {
		deck := decks[row.Deck]
		got := projectShelf(karsten.Read(deck.Library, deck.Commander, row.Target, row.OnThePlay))
		if diff := cmp.Diff(row.Value, got, exactFloats(epsilonShelf)); diff != "" {
			t.Errorf("Read(%s, target=%v, on_the_play=%v) differs from Python (-python +go):\n%s",
				row.Deck, row.Target, row.OnThePlay, diff)
		}
	}
}

func projectEstimate(e karsten.LandEstimate) estimateCase {
	return estimateCase{
		LandsNow: e.LandsNow, Recommended: e.Recommended, Delta: e.Delta(),
		AverageManaValue: e.AverageManaValue, CheapAccelerants: e.CheapAccelerants,
		DeckSize: e.DeckSize, Caveats: e.Caveats,
	}
}

// projectShelf renders a Go shelf in Python's own JSON shape, calling every
// derived property on the way -- `Met`, `Shortfall`, `OnCurve`,
// `ReliableTurn`, `Lag`, `Lateness`, `Unmet`. They are methods here and
// fields in the corpus, so projecting is also how they get tested.
func projectShelf(s karsten.Shelf) shelfCase {
	out := shelfCase{
		DeckSize: s.DeckSize, Lands: s.Lands, Target: s.Target,
		OnThePlay: s.OnThePlay, LandEstimate: projectEstimate(s.LandEstimate),
		Colors: []colorCase{}, Odds: []oddsCase{},
		Approximated: s.Approximated, Unmet: []string{},
	}
	for _, c := range s.Colors {
		cc := colorCase{
			Color: c.Color, Have: c.Have, HaveLands: c.HaveLands,
			Met: c.Met(), Shortfall: c.Shortfall(), Tiers: []tierCase{},
		}
		for _, tier := range c.Tiers {
			cc.Tiers = append(cc.Tiers, tierCase{
				Pips: tier.Pips, Turn: tier.Turn, Need: tier.Need, Have: tier.Have,
				Met: tier.Met(), Shortfall: tier.Shortfall(),
				OddsNow: tier.OddsNow, Cards: tier.Cards,
			})
		}
		out.Colors = append(out.Colors, cc)
	}
	for _, o := range s.Odds {
		out.Odds = append(out.Odds, oddsCase{
			Name: o.Name, MV: o.MV, ByTurn: o.ByTurn, OnCurve: o.OnCurve(),
			ReliableTurn: o.ReliableTurn(), Lag: o.Lag(), Lateness: o.Lateness(),
		})
	}
	for _, c := range s.Unmet() {
		out.Unmet = append(out.Unmet, c.Color)
	}
	if out.Approximated == nil {
		out.Approximated = []string{}
	}
	return out
}

// ------------------------------------------- arithmetic from somewhere else

// bruteForceAtLeast is the same probability by enumerating every hand, in
// exact rationals.
//
// Ported from `tests/test_karsten.py`, and deliberately the stupidest correct
// implementation: label the population, look at every subset of size `draws`,
// count the ones that qualify. It is unusable past about twenty cards and it
// cannot be wrong, which is the trade a test wants -- and it is the one check
// in this file whose answer does not come from the corpus, so a Python bug
// faithfully ported would still fail it.
func bruteForceAtLeast(population, successes, draws, wanted int) *big.Rat {
	hits := big.NewInt(0)
	total := big.NewInt(0)
	for mask := 0; mask < 1<<population; mask++ {
		if bits.OnesCount(uint(mask)) != draws {
			continue
		}
		total.Add(total, big.NewInt(1))
		if bits.OnesCount(uint(mask)&(1<<successes-1)) >= wanted {
			hits.Add(hits, big.NewInt(1))
		}
	}
	return new(big.Rat).SetFrac(hits, total)
}

func TestHypergeometricMatchesExhaustiveEnumeration(t *testing.T) {
	// The cases `tests/test_karsten.py` names, which are the cases somebody
	// was once wrong about.
	for _, c := range [][4]int{
		{12, 5, 4, 1}, {12, 5, 4, 2}, {12, 5, 4, 3}, {12, 5, 4, 5},
		{15, 6, 7, 2}, {15, 6, 7, 0}, {10, 10, 3, 3}, {10, 0, 3, 1},
		{14, 3, 9, 3}, {14, 3, 9, 4},
	} {
		want, _ := bruteForceAtLeast(c[0], c[1], c[2], c[3]).Float64()
		got := karsten.HypergeometricAtLeast(c[0], c[1], c[2], c[3])
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("HypergeometricAtLeast%v = %v, enumeration = %v", c, got, want)
		}
	}
}

func TestBothSummationBranchesAreExercisedAndAgree(t *testing.T) {
	// Two code paths for one number is two chances to be wrong, so drive a
	// case on each side of the branch and check both against the enumeration.
	// wanted=1 of 5 in 4 draws: the complement is one term, the direct sum is
	// four. wanted=4: the direct sum is two terms, the complement is four.
	low := karsten.HypergeometricAtLeast(12, 5, 4, 1)
	high := karsten.HypergeometricAtLeast(12, 5, 4, 4)
	lowWant, _ := bruteForceAtLeast(12, 5, 4, 1).Float64()
	highWant, _ := bruteForceAtLeast(12, 5, 4, 4).Float64()
	if math.Abs(low-lowWant) > 1e-12 || math.Abs(high-highWant) > 1e-12 {
		t.Errorf("branches disagree with the enumeration: %v vs %v, %v vs %v",
			low, lowWant, high, highWant)
	}
	if low <= high {
		t.Errorf("asking for more cannot be easier: %v <= %v", low, high)
	}
}

func TestTheProbabilityMassFunctionSumsToOne(t *testing.T) {
	total := 0.0
	for k := 0; k < 10; k++ {
		total += karsten.Exactly(99, 36, 9, k)
	}
	if math.Abs(total-1.0) > 1e-12 {
		t.Errorf("the mass function sums to %v", total)
	}
}

func TestAtLeastIsMonotoneInEveryArgumentThatShouldMoveIt(t *testing.T) {
	// Properties that hold whatever the arithmetic is. More sources cannot
	// hurt, more draws cannot hurt, and asking for more pips cannot help. A
	// sign error survives a spot-check of one number and does not survive
	// this.
	base := karsten.HypergeometricAtLeast(99, 30, 10, 2)
	if karsten.HypergeometricAtLeast(99, 31, 10, 2) < base {
		t.Error("a thirty-first source made the deck worse")
	}
	if karsten.HypergeometricAtLeast(99, 30, 11, 2) < base {
		t.Error("an eleventh card seen made the deck worse")
	}
	if karsten.HypergeometricAtLeast(99, 30, 10, 3) > base {
		t.Error("a third pip made the deck better")
	}
	if karsten.HypergeometricAtLeast(99, 30, 10, 0) != 1.0 {
		t.Error("asking for nothing must be certain")
	}
	if karsten.HypergeometricAtLeast(99, 1, 10, 2) != 0.0 {
		t.Error("two pips from one source must be impossible")
	}
}

// ------------------------------------------------- the pinned Python traps

func TestCardsSeenCountsTheSkippedDrawStep(t *testing.T) {
	for _, c := range []struct {
		turn      int
		onThePlay bool
		want      int
	}{{1, true, 7}, {1, false, 8}, {4, true, 10}, {4, false, 11}} {
		if got := karsten.CardsSeen(c.turn, c.onThePlay); got != c.want {
			t.Errorf("CardsSeen(%d, %v) = %d, want %d", c.turn, c.onThePlay, got, c.want)
		}
	}
}

func TestRequiredSourcesIsTheSmallestCountThatClearsTheBar(t *testing.T) {
	// The definition, checked as a definition rather than as a number.
	// Whatever `RequiredSources` returns must clear the target, and one fewer
	// must not. That cannot be satisfied by a lookup table that has drifted.
	for _, pips := range []int{1, 2, 3} {
		for _, turn := range []int{max(1, pips), pips + 2} {
			need := karsten.RequiredSources(99, pips, turn, karsten.Target, true)
			seen := karsten.CardsSeen(turn, true)
			if karsten.HypergeometricAtLeast(99, need, seen, pips) < karsten.Target {
				t.Errorf("pips=%d turn=%d: %d sources do not clear the bar", pips, turn, need)
			}
			if karsten.HypergeometricAtLeast(99, need-1, seen, pips) >= karsten.Target {
				t.Errorf("pips=%d turn=%d: %d sources already clear it", pips, turn, need-1)
			}
		}
	}
}

func TestASinglePipWantsAboutAFifthOfACommanderDeck(t *testing.T) {
	// The landmark, as a band rather than as a figure. A range this can never
	// leave would test nothing.
	need := karsten.RequiredSources(99, 1, 4, karsten.Target, true)
	if need < 18 || need > 22 {
		t.Errorf("a single pip on turn four wants %d sources", need)
	}
}

func TestThisPackageIsStricterThanThePublishedTableAndByHowMuch(t *testing.T) {
	// Pins the documented gap, in the documented direction. The package
	// comment claims 86.1% where Karsten's table reads 90%, at 14 sources in
	// 60 cards for a turn-one single pip, and explains the difference as the
	// London mulligan this does not model. That is a load-bearing claim -- it
	// is why these requirements may disagree with a published table without
	// either being wrong -- so it is pinned. If this fails, the package
	// comment is now wrong and must be edited rather than the assertion
	// loosened.
	odds := karsten.HypergeometricAtLeast(60, 14, karsten.CardsSeen(1, true), 1)
	if math.Abs(odds-0.861) > 0.001 {
		t.Errorf("the gap has moved: %v, want 0.861 +/- 0.001", odds)
	}
	if odds >= 0.90 {
		t.Error("we must be stricter than the table, never looser")
	}
}

func TestLatenessRanksACheapCardThatNeverArrivesAboveAnExpensiveOne(t *testing.T) {
	// The ranking's whole job, and the bug it was written against. Sorting by
	// raw castability leads with every twelve-drop and reports that expensive
	// cards are expensive.
	cheap := karsten.CardOdds{Name: "Cheap", MV: 3, ByTurn: make([]float64, karsten.Horizon)}
	dear := karsten.CardOdds{Name: "Dear", MV: 8, ByTurn: make([]float64, karsten.Horizon)}
	if cheap.Lateness() <= dear.Lateness() {
		t.Errorf("a three-drop that never arrives (%d) must outrank an eight-drop (%d)",
			cheap.Lateness(), dear.Lateness())
	}
	if cheap.Lag() != nil || dear.Lag() != nil {
		t.Error("neither ever becomes reliable, so neither has a lag")
	}
}

func TestACardPastTheHorizonReportsNotAskedRatherThanNever(t *testing.T) {
	// Nil, not zero. Zero would be a claim the shelf did not make.
	huge := karsten.CardOdds{Name: "Ghalta", MV: 12, ByTurn: make([]float64, karsten.Horizon)}
	if huge.OnCurve() != nil {
		t.Error("the shelf was never asked about turn 12")
	}
	if huge.Lateness() >= 0 {
		t.Errorf("a twelve-drop must sort last, not first: lateness %d", huge.Lateness())
	}
}

func TestLagIsTheGapBetweenCostAndReliability(t *testing.T) {
	odds := karsten.CardOdds{
		Name: "Three Drop", MV: 3,
		ByTurn: []float64{0.0, 0.0, 0.5, 0.7, 0.95, 1.0, 1.0, 1.0, 1.0, 1.0},
	}
	if rt := odds.ReliableTurn(); rt == nil || *rt != 5 {
		t.Errorf("ReliableTurn = %v, want 5", rt)
	}
	if lag := odds.Lag(); lag == nil || *lag != 2 {
		t.Errorf("Lag = %v, want 2", lag)
	}
	if odds.Lateness() != 2 {
		t.Errorf("Lateness = %d, want 2", odds.Lateness())
	}
	if oc := odds.OnCurve(); oc == nil || *oc != 0.5 {
		t.Errorf("OnCurve = %v, want 0.5", oc)
	}
}

func TestTheRegressionScalesWithDeckSizeRatherThanIgnoringIt(t *testing.T) {
	// A 60-card fit applied to 99 cards without scaling recommends a 60-card
	// mana base for a Commander deck, which is the whole trap.
	_, decks := load(t)
	e := karsten.RegressionLands(decks["mono-green"].Library)
	if e.DeckSize != 99 {
		t.Fatalf("the fixture deck is %d cards", e.DeckSize)
	}
	if e.Recommended <= 30 {
		t.Errorf("the intercept alone is 19.59 at 60 cards; scaled it must clear 30, got %d",
			e.Recommended)
	}
}

func TestADeckWithNoSpellsDoesNotDivideByZero(t *testing.T) {
	_, decks := load(t)
	e := karsten.RegressionLands(decks["all-lands"].Library)
	if e.Recommended != 99 || e.LandsNow != 99 {
		t.Errorf("all lands: recommended %d, now %d, want 99 and 99", e.Recommended, e.LandsNow)
	}
	if len(e.Caveats) == 0 {
		t.Error("a deck with no curve to fit must say so")
	}
}

func TestTheCommanderSetsRequirementsButIsNotDrawnFromTheLibrary(t *testing.T) {
	// Both halves matter: a mana base that cannot cast the commander is the
	// first thing to know, and counting it as a hundredth card would make
	// every probability on the shelf slightly wrong.
	_, decks := load(t)
	deck := decks["commanded"]
	shelf := karsten.Read(deck.Library, deck.Commander, karsten.Target, true)
	if shelf.DeckSize != 99 {
		t.Errorf("DeckSize = %d; the commander is not part of the library", shelf.DeckSize)
	}
	found := false
	for _, c := range shelf.Colors {
		if c.Color == "W" {
			found = true
		}
	}
	if !found {
		t.Error("the commander still sets a white demand")
	}
	named := false
	for _, o := range shelf.Odds {
		if o.Name == "Legend" {
			named = true
		}
	}
	if !named {
		t.Error("the commander is in the card-by-card odds")
	}
}

func TestAnEmptyLibraryDoesNotPanic(t *testing.T) {
	shelf := karsten.Read(nil, nil, karsten.Target, true)
	if shelf.DeckSize != 0 || len(shelf.Colors) != 0 {
		t.Errorf("an empty library gave %d cards and %d colours", shelf.DeckSize, len(shelf.Colors))
	}
}

func TestHybridPipsAreChargedToBothColours(t *testing.T) {
	// A {G/W} card needs green sources *or* white ones, so both are asked.
	// The alternative -- charging it to neither, or picking one -- is how a
	// deck full of hybrid cards reports a mana base it does not have.
	_, decks := load(t)
	shelf := karsten.Read(decks["hybrid-heavy"].Library, nil, karsten.Target, true)
	seen := map[string]bool{}
	for _, c := range shelf.Colors {
		seen[c.Color] = true
	}
	if !seen["G"] || !seen["W"] {
		t.Errorf("a hybrid deck asked for %v", seen)
	}
}

func TestPhyrexianPipsPlaceNoDemandOnTheManaBase(t *testing.T) {
	// Two life always pays, so a Phyrexian symbol asks the mana base nothing.
	// Checked here because this package reads the parsed cost directly, and
	// the property it depends on is one somebody could reasonably "fix".
	_, decks := load(t)
	naya := decks["naya"]
	var compleated *sim.Card
	for i := range naya.Library {
		if naya.Library[i].Name == "Compleated One" {
			compleated = &naya.Library[i]
		}
	}
	if compleated == nil {
		t.Fatal("the naya fixture no longer holds a Phyrexian cost")
	}
	if len(compleated.Cost.Phyrexian) == 0 {
		t.Fatal("Compleated One parsed with no Phyrexian symbol")
	}
	if compleated.MV() != 3 {
		t.Errorf("a Phyrexian symbol still counts toward mana value: mv %d", compleated.MV())
	}
	// {2}{U/P} demands nothing coloured, so its castability is the plain
	// land-count question the whole-shelf comparison already pins; what is
	// asserted here is that blue never appears as a requirement.
	shelf := karsten.Read(naya.Library, naya.Commander, karsten.Target, true)
	for _, c := range shelf.Colors {
		if c.Color == "U" {
			for _, tier := range c.Tiers {
				for _, name := range tier.Cards {
					if name == "Compleated One" {
						t.Error("a Phyrexian pip placed a demand on the mana base")
					}
				}
			}
		}
	}
}

func TestACardIsNeverCastableBeforeItsOwnManaValue(t *testing.T) {
	_, decks := load(t)
	library := decks["mono-green"].Library
	card := sim.Card{Name: "Four Drop", Cost: sim.Cost{Generic: 3, Pips: [][]string{{"G"}}}}
	for turn := 1; turn <= 3; turn++ {
		if got := karsten.CastableOdds(card, library, turn, true); got != 0.0 {
			t.Errorf("turn %d: a four-drop is castable at %v", turn, got)
		}
	}
	if got := karsten.CastableOdds(card, library, 4, true); got <= 0.0 {
		t.Errorf("turn 4: a four-drop is castable at %v", got)
	}
}

func TestConditioningOnTheDrawBeatsMultiplyingTwoProbabilities(t *testing.T) {
	// The reason `CastableOdds` is not two hypergeometrics multiplied. In a
	// mono-green deck "I have four lands" and "I have one green source" are
	// very nearly the same event; multiplying them unconditionally prices the
	// most consistent mana base in Magic as though the two were independent.
	library := make([]sim.Card, 0, 99)
	for i := 0; i < 40; i++ {
		library = append(library, sim.Card{
			Name:   "Forest " + string(rune('A'+i%26)) + string(rune('a'+i/26)),
			IsLand: true, Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}},
		})
	}
	for i := 0; i < 59; i++ {
		library = append(library, sim.Card{
			Name: "Bear " + string(rune('A'+i%26)) + string(rune('a'+i/26)),
			Cost: sim.Cost{Generic: 1, Pips: [][]string{{"G"}}},
		})
	}
	card := sim.Card{Name: "Four Drop", Cost: sim.Cost{Generic: 3, Pips: [][]string{{"G"}}}}
	conditional := karsten.CastableOdds(card, library, 4, true)
	landsOnly := karsten.HypergeometricAtLeast(99, 40, karsten.CardsSeen(4, true), 4)
	naive := landsOnly * karsten.HypergeometricAtLeast(99, 40, karsten.CardsSeen(4, true), 1)
	if math.Abs(conditional-landsOnly) > 0.001 {
		t.Errorf("every source is green, so the colour must cost nothing: %v vs %v",
			conditional, landsOnly)
	}
	if conditional <= naive {
		t.Errorf("the conditional form must beat the independent one: %v <= %v", conditional, naive)
	}
}

func TestMultiColourCardsAreNamedAsApproximated(t *testing.T) {
	// The one place the method stops being exact, reported rather than hidden.
	_, decks := load(t)
	naya := decks["naya"]
	shelf := karsten.Read(naya.Library, naya.Commander, karsten.Target, true)
	named := map[string]bool{}
	for _, n := range shelf.Approximated {
		named[n] = true
	}
	if !named["Naya Charm"] {
		t.Errorf("a {R}{G}{W} card must be named as approximated; got %v", shelf.Approximated)
	}
	if named["Hybrid Hero"] {
		t.Error("{G/W}{G/W} is one pip set, so it is exact and must not be named")
	}
	if named["Beast 0"] {
		t.Error("a single-colour card is exact")
	}
}
