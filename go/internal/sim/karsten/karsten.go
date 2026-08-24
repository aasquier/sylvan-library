// Package karsten is `sim/karsten.py`: Tier 1.5, the closed form. What
// arithmetic can answer about a compiled deck without shuffling it.
//
// Tier 1 shuffles a deck twenty thousand times and counts what happened. This
// package asks the same questions of the *same compiled 99* and answers them
// with a hypergeometric distribution instead -- exactly, in microseconds, with
// no sampling error and no seed.
//
// That is not a replacement for Tier 1 and it must never be rendered as one:
// the closed form is exact about a simplified game, the simulation is
// approximate about a fuller one, and **where they disagree is the whole
// point**. The closed form assumes you keep your opening seven, play a land
// every turn, and have every drawn source in play; it cannot see ramp at all,
// so a compiled Cultivate produces no mana and is invisible here. Which way
// that cuts is a fact about the deck rather than about the method -- measured
// on 2026-08-21 against 6,000 Tier 1 games per deck, the gap on the commander
// ran from -12.3 points (Atla Palani, dominated by its tapped lands) to +15.1
// (Arahbo). Mono-green Goreclaw sits at +13.2, dominated by its dorks.
//
// One caution governs every other number here. `CastableOdds` asks *"if this
// card were in your hand, could you pay for it?"* -- not *"will you have cast
// it?"*. The commander is the only card where those coincide. Sylvan
// Safekeeper is 96.7% castable on turn 1 and cast in 4.3% of games, because
// the other 92% is simply not having drawn it. Both numbers are correct and
// they answer different questions; a surface that stacks them in one column is
// lying with true figures.
//
// Named for Frank Karsten, whose hypergeometric analysis of coloured mana
// requirements is the method this implements. **The numbers are not his
// table.** Nothing here is copied from a published figure, and they also do
// not match his: he models the London mulligan and this does not, because Tier
// 1 sitting beside it models mulligans properly. Digging for a keepable hand
// is worth roughly two sources, so **every requirement reported here is a
// shade stricter than the published one** -- 86.1% where his table reads 90%,
// at 14 white sources in 60 cards for a turn-one single pip. Stricter in a
// known direction is a usable number; silently different is not, and the test
// beside this file pins the gap so it stays known. Do not "fix" it toward the
// table.
//
// # On agreeing with Python
//
// The Python module is `math.comb` and nothing else -- exact integer
// arithmetic, one division at the end. Go has no `math.Comb`, and a
// float64 binomial for a 99-card deck loses the agreement in the binomials
// long before the comparison: C(99, 49) is about 2.5e28, six orders of
// magnitude past the last integer float64 can hold exactly. So the binomials
// here are `math/big.Int` and the single division is a `big.Rat`, whose
// `Float64` is the same correctly-rounded, ties-to-even value CPython's
// int/int true division produces. The result is not "close to" Python's; it
// is the same bits, which is what the differential corpus asserts.
//
// That exactness is not decoration. `RequiredSources` scans for the first
// count where the odds clear a float target, and `CardOdds.ReliableTurn` does
// the same against 0.90 -- so one ulp is one more land in a recommendation, or
// one row in a different place in the shelf's table.
package karsten

import (
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// Target is the consistency bar every requirement here is measured against.
// Karsten's choice, and a choice rather than a fact: 90% means one game in ten
// goes wrong on colour, which is the most a deck can reasonably ask for
// without spending its whole list on lands. Exposed so a cEDH deck can ask for
// more and a kitchen-table deck can ask for less.
const Target = 0.90

// Horizon is the turns the shelf reports on. Past this a Commander deck has
// drawn a quarter of itself and the arithmetic stops being interesting --
// everything is castable, which is a statement about turn 12 rather than about
// the deck.
const Horizon = 10

// ---------------------------------------------------------- the arithmetic

// cacheLimit mirrors `lru_cache(maxsize=100_000)` on the two Python
// functions: a bound, not a policy. The heatmap asks for the same tuple once
// per card sharing a cost and a 99-card deck asks a few thousand times, so a
// memo is the difference between milliseconds and seconds once the binomials
// are big integers. Eviction is wholesale rather than least-recently-used,
// because these are pure functions of four small ints and the only property
// that matters is that the map cannot grow without bound.
const cacheLimit = 100_000

type hyperKey struct{ population, successes, draws, wanted int }

var (
	hyperMu    sync.Mutex
	hyperCache = map[hyperKey]float64{}
	exactCache = map[hyperKey]float64{}

	binomMu    sync.Mutex
	binomCache = map[[2]int]*big.Int{}
)

// binom is `math.comb`: exact, and zero where Python's is zero (k < 0 or
// k > n). Memoised because every term of every sum below is one of these and
// the distinct (n, k) pairs a deck asks for number in the hundreds.
func binom(n, k int) *big.Int {
	if k < 0 || n < 0 || k > n {
		return big.NewInt(0)
	}
	key := [2]int{n, k}
	binomMu.Lock()
	defer binomMu.Unlock()
	if v, ok := binomCache[key]; ok {
		return v
	}
	v := new(big.Int).Binomial(int64(n), int64(k))
	if len(binomCache) >= cacheLimit {
		binomCache = map[[2]int]*big.Int{}
	}
	binomCache[key] = v
	return v
}

// ratio is CPython's `int / int`: the correctly-rounded float64 nearest the
// exact rational, ties to even. `big.Rat.Float64` is documented to be exactly
// that, which is why this is an equality rather than a tolerance.
func ratio(num, den *big.Int) float64 {
	f, _ := new(big.Rat).SetFrac(num, den).Float64()
	return f
}

// HypergeometricAtLeast is P(at least `wanted` successes) when drawing
// `draws` from `population`, of which `successes` qualify.
//
// The one piece of mathematics this package is built on. Exact rational
// arithmetic, then one division -- not a normal approximation, which is wrong
// in exactly the tail this cares about.
func HypergeometricAtLeast(population, successes, draws, wanted int) float64 {
	key := hyperKey{population, successes, draws, wanted}
	hyperMu.Lock()
	if v, ok := hyperCache[key]; ok {
		hyperMu.Unlock()
		return v
	}
	hyperMu.Unlock()

	v := hypergeometricAtLeast(population, successes, draws, wanted)

	hyperMu.Lock()
	if len(hyperCache) >= cacheLimit {
		hyperCache = map[hyperKey]float64{}
	}
	hyperCache[key] = v
	hyperMu.Unlock()
	return v
}

func hypergeometricAtLeast(population, successes, draws, wanted int) float64 {
	if wanted <= 0 {
		return 1.0
	}
	if successes < wanted || draws < wanted {
		return 0.0
	}
	// Clamped *after* the check above, exactly as Python clamps it: asking for
	// more draws than there are cards is asking to see the whole deck.
	if draws > population {
		draws = population
	}
	total := binom(population, draws)
	if total.Sign() == 0 {
		return 0.0
	}
	// Sum the complement when it is the shorter sum: P(X >= k) needs
	// min(successes, draws) - k + 1 terms and P(X < k) needs k. For a single
	// pip on turn 1 the complement is one term against twenty.
	upper := successes
	if draws < upper {
		upper = draws
	}
	if wanted <= upper-wanted+1 {
		below := new(big.Int)
		for i := 0; i < wanted; i++ {
			if draws-i < 0 || draws-i > population-successes {
				continue
			}
			below.Add(below, new(big.Int).Mul(binom(successes, i), binom(population-successes, draws-i)))
		}
		// `max(0.0, 1.0 - below / total)`: Python's max returns the second
		// operand only when it is strictly greater, which is what keeps a
		// negative zero out of the answer.
		if v := 1.0 - ratio(below, total); v > 0.0 {
			return v
		}
		return 0.0
	}
	hits := new(big.Int)
	for i := wanted; i <= upper; i++ {
		if draws-i < 0 || draws-i > population-successes {
			continue
		}
		hits.Add(hits, new(big.Int).Mul(binom(successes, i), binom(population-successes, draws-i)))
	}
	if v := ratio(hits, total); v < 1.0 {
		return v
	}
	return 1.0
}

// Exactly is `karsten._exactly`: P(exactly `count` successes). The
// conditioning step's weight, and `sim/curve.py` imports it by name, which is
// why it is exported here where Python keeps it private.
func Exactly(population, successes, draws, count int) float64 {
	key := hyperKey{population, successes, draws, count}
	hyperMu.Lock()
	if v, ok := exactCache[key]; ok {
		hyperMu.Unlock()
		return v
	}
	hyperMu.Unlock()

	v := exactly(population, successes, draws, count)

	hyperMu.Lock()
	if len(exactCache) >= cacheLimit {
		exactCache = map[hyperKey]float64{}
	}
	exactCache[key] = v
	hyperMu.Unlock()
	return v
}

func exactly(population, successes, draws, count int) float64 {
	if draws > population {
		draws = population
	}
	if count > successes || count > draws || draws-count > population-successes {
		return 0.0
	}
	total := binom(population, draws)
	if total.Sign() == 0 {
		return 0.0
	}
	num := new(big.Int).Mul(binom(successes, count), binom(population-successes, draws-count))
	return ratio(num, total)
}

// CardsSeen is how many cards you have looked at by turn `turn`: seven plus a
// draw step per turn, minus the one the starting player skips.
//
// On the play is the harder case and the default everywhere here, because a
// mana base that holds on the play holds on the draw.
func CardsSeen(turn int, onThePlay bool) int {
	extra := turn
	if onThePlay {
		extra = turn - 1
	}
	if extra < 0 {
		extra = 0
	}
	return 7 + extra
}

// RequiredSources is the fewest sources of one colour that cast a `pips`-pip
// spell on curve.
//
// The number the shelf is for. Recomputed rather than looked up: pass a
// 99-card deck and it answers for 99, which is why there is no table in this
// file to go stale. Monotone in the source count, so a linear scan from zero
// is both correct and the clearest thing to read.
func RequiredSources(deckSize, pips, turn int, target float64, onThePlay bool) int {
	if pips <= 0 {
		return 0
	}
	seen := CardsSeen(turn, onThePlay)
	for count := pips; count <= deckSize; count++ {
		if HypergeometricAtLeast(deckSize, count, seen, pips) >= target {
			return count
		}
	}
	return deckSize
}

// ------------------------------------------------------ reading the deck

// pipDemand is the card's coloured demand, grouped by what may pay it, in
// first-appearance order.
//
// Keyed on the pip's own colour *set*, so a hybrid {G/W} lands in its own
// bucket rather than being charged to green or to white. Phyrexian is absent
// by construction -- `mana.Cost` keeps it apart precisely because two life
// always pays it, so it places no demand on a mana base.
//
// The order is load-bearing and not cosmetic: `CastableOdds` multiplies one
// conditional term per bucket, and float multiplication is not associative, so
// a different iteration order is a different last bit. Python's dicts preserve
// insertion order; this preserves the same one.
type demandEntry struct {
	colors []string // the pip's colour set, sorted -- `mana.Cost` sorts it
	key    string   // the canonical join of that set, for identity
	pips   int
}

func pipDemand(cost sim.Cost) []demandEntry {
	out := make([]demandEntry, 0, len(cost.Pips))
	index := map[string]int{}
	for _, pip := range cost.Pips {
		key := strings.Join(pip, "/")
		if at, ok := index[key]; ok {
			out[at].pips++
			continue
		}
		index[key] = len(out)
		out = append(out, demandEntry{colors: pip, key: key, pips: 1})
	}
	return out
}

func colorSet(colors []string) map[string]bool {
	out := make(map[string]bool, len(colors))
	for _, c := range colors {
		out[c] = true
	}
	return out
}

// SourcesFor is how many cards in the deck can produce a colour in `colors`.
//
// Counts every mana-producing permanent, not only lands: a Signet and a dual
// land both answer "can I make white this turn", and a shelf that counted
// lands alone would tell an artifact deck it was short of colours it has
// plenty of. Lands are reported separately by `Read` so the split stays
// visible.
func SourcesFor(library []sim.Card, colors map[string]bool) int {
	n := 0
	for _, c := range library {
		for _, src := range c.Produces {
			if src.Makes(colors) {
				n++
				break
			}
		}
	}
	return n
}

// PipTier is one colour at one pip count: what that demand needs, and who
// makes it.
//
// The shelf reports a colour as a *ladder* of these rather than as a single
// verdict, and that is a commandment 2 decision before it is a modelling one.
// A mono-green deck with 37 green sources meets every single- and double-pip
// demand it has and fails only its one triple-pip card; collapsing that to
// "green: short by 11" tells a beginner their mana base is broken when what is
// true is that one card is greedy. The ladder says which rung broke.
type PipTier struct {
	Pips int `json:"pips"`
	// Turn this is judged on: the cheapest card in the deck making this
	// demand, since that is the earliest the deck asks the question.
	Turn int `json:"turn"`
	// Need is sources this demand wants at the target; Have is what the deck
	// has.
	Need int `json:"need"`
	Have int `json:"have"`
	// OddsNow is P(making this demand on Turn) at the count the deck has.
	OddsNow float64 `json:"odds_now"`
	// Cards making this demand, cheapest first. Named, because "which card is
	// doing this to me" is the actionable half of the answer.
	Cards []string `json:"cards"`
}

// Met reports whether the deck holds what this rung asks.
func (t PipTier) Met() bool { return t.Have >= t.Need }

// Shortfall is how many sources short this rung is, never negative.
func (t PipTier) Shortfall() int {
	if d := t.Need - t.Have; d > 0 {
		return d
	}
	return 0
}

// ColorRequirement is one colour, what the deck holds, and what its own cards
// demand. `Tiers` is the ladder; `Met` is true only when every rung is, but a
// deck that fails one rung is not the same as a deck that fails all of them,
// and the surface renders the rungs rather than the verdict.
type ColorRequirement struct {
	Color string `json:"color"`
	// Have is every producer of this colour in the 99; HaveLands is the
	// subset that are lands.
	Have      int       `json:"have"`
	HaveLands int       `json:"have_lands"`
	Tiers     []PipTier `json:"tiers"`
}

// Met reports whether every rung of the ladder is met.
func (c ColorRequirement) Met() bool {
	for _, t := range c.Tiers {
		if !t.Met() {
			return false
		}
	}
	return true
}

// Worst is the rung that fails hardest, or the most demanding one if all pass.
// The second return is false for a colour with no rungs at all.
func (c ColorRequirement) Worst() (PipTier, bool) {
	if len(c.Tiers) == 0 {
		return PipTier{}, false
	}
	// `max(tiers, key=...)` keeps the *first* maximal element, so the
	// comparison is strictly-greater and ties fall to the earlier rung.
	best := c.Tiers[0]
	for _, t := range c.Tiers[1:] {
		if t.Shortfall() > best.Shortfall() || (t.Shortfall() == best.Shortfall() && t.Pips > best.Pips) {
			best = t
		}
	}
	return best, true
}

// Shortfall is the worst rung's shortfall, or zero for a colour with no rungs.
func (c ColorRequirement) Shortfall() int {
	worst, ok := c.Worst()
	if !ok {
		return 0
	}
	return worst.Shortfall()
}

// CardOdds is one card's chance of being castable on each turn, 1..Horizon.
type CardOdds struct {
	Name string `json:"name"`
	MV   int    `json:"mv"`
	// ByTurn index 0 is turn 1. Length is always Horizon.
	ByTurn []float64 `json:"by_turn"`
}

// OnCurve is P(castable on the turn it costs), or nil past the horizon.
//
// Nil rather than zero for a card costing more than Horizon. Zero would be a
// claim -- that a twelve-drop is uncastable -- and what is true is that this
// shelf was not asked about turn 12. A number that cannot tell "no" from "not
// asked" is how a sorted list ends up leading with every expensive card.
func (o CardOdds) OnCurve() *float64 {
	if o.MV < 1 || o.MV > Horizon {
		return nil
	}
	v := o.ByTurn[o.MV-1]
	return &v
}

// ReliableTurn is the first turn this card is castable at Target, if ever by
// then; nil otherwise.
func (o CardOdds) ReliableTurn() *int {
	for i, odds := range o.ByTurn {
		if odds >= Target {
			turn := i + 1
			return &turn
		}
	}
	return nil
}

// Lag is turns between what the card costs and when you can rely on it.
//
// The number this whole table exists to produce. A three-drop you cannot rely
// on until turn six is three turns late, and a deck's worst lags are its real
// mana problems -- unlike raw castability, which just ranks the deck by mana
// value and tells you the expensive cards are expensive.
func (o CardOdds) Lag() *int {
	reliable := o.ReliableTurn()
	if reliable == nil {
		return nil
	}
	lag := *reliable - o.MV
	return &lag
}

// Lateness is Lag with a floor for cards that never get there. Sort on this.
//
// A card that never reaches Target inside the horizon has no lag and the
// ranking still has to place it. Charging it *at least* the lag it would have
// if it arrived one turn past the horizon is what makes the list read
// correctly, because it normalises by what the card costs: a three-drop that
// never becomes reliable is eight turns late, and an eight-drop that never
// becomes reliable is three. The three-drop is the real complaint.
//
// It also puts anything costing more than the horizon *last*, at a negative
// lateness -- which is right, because the shelf was never asked about turn 12.
func (o CardOdds) Lateness() int {
	if lag := o.Lag(); lag != nil {
		return *lag
	}
	return Horizon + 1 - o.MV
}

// CastableOdds is P(the mana for this card is there on `turn`), given the deck
// it lives in.
//
// **Assuming the card is in your hand.** This is a question about the mana
// base and only about the mana base: it does not ask whether you drew the
// card, and for anything except the commander those are very different numbers
// -- see the package comment, where a 96.7% turn-one card is cast in 4.3% of
// games. Every surface rendering this must say which one it is showing.
//
// The model, stated plainly because the number is only as honest as it is:
//
//  1. You must have reached the turn at all -- nothing with mana value 4 is
//     castable on turn 3, whatever you drew.
//  2. You must have drawn at least `mana_value` mana sources, and they must
//     all be in play. That is the optimistic half: it plays tapped lands as
//     though untapped and casts a mana creature the turn it lands.
//  3. Given exactly `n` sources drawn, the coloured requirements are checked
//     *within those n* rather than against the deck at large.
//
// Step 3 is what makes this worth writing instead of multiplying two
// unconditional probabilities together. In a mono-green deck "I have three
// lands" and "I have two green sources" are very nearly the same event, and
// multiplying them prices the deck as though they were independent -- which
// reports the most consistent mana base in Magic as a coin flip. Conditioning
// on the draw fixes that, and makes the answer **exact** for any card whose
// coloured demand is a single colour, which is most of them.
//
// Where a card demands two or more different colours the conditional terms are
// still multiplied, so those remain an approximation. It errs low -- duals and
// any-colour sources satisfy two demands at once and this counts them once
// each -- and `Read` reports which cards are affected rather than hiding it.
func CastableOdds(card sim.Card, library []sim.Card, turn int, onThePlay bool) float64 {
	mv := card.MV()
	if mv > turn {
		return 0.0
	}
	deckSize := len(library)
	if deckSize == 0 {
		return 0.0
	}

	seen := CardsSeen(turn, onThePlay)
	pool := 0
	for _, c := range library {
		if len(c.Produces) > 0 {
			pool++
		}
	}
	if pool < mv {
		return 0.0
	}

	demand := pipDemand(card.Cost)
	// Free spells are a land-count question and nothing else.
	if len(demand) == 0 {
		return HypergeometricAtLeast(deckSize, pool, seen, mv)
	}

	// How many of the deck's sources answer each distinct pip set.
	answering := make([]int, len(demand))
	for i, d := range demand {
		answering[i] = SourcesFor(library, colorSet(d.colors))
	}

	// Condition on the number of sources drawn. `drawn` cannot exceed the turn
	// for lands, but a deck's sources are not all lands and a rock cast on
	// turn two is producing on turn two, so the ceiling here is the draw
	// itself.
	ceiling := seen
	if pool < ceiling {
		ceiling = pool
	}
	terms := make([]float64, 0, ceiling-mv+1)
	for drawn := mv; drawn <= ceiling; drawn++ {
		weight := Exactly(deckSize, pool, seen, drawn)
		if weight == 0.0 {
			continue
		}
		odds := 1.0
		for i, d := range demand {
			odds *= HypergeometricAtLeast(pool, answering[i], drawn, d.pips)
			if odds == 0.0 {
				break
			}
		}
		terms = append(terms, floats.Rounded(weight*odds))
	}
	// fsum rather than a running total: a hundred small products of
	// probabilities is exactly the shape that accumulates float error, and
	// `ReliableTurn` compares this against 0.90.
	if v := floats.Fsum(terms); v < 1.0 {
		return v
	}
	return 1.0
}

// ------------------------------------------------------------ the land count

// Frank Karsten's regression over winning 60-card tournament decklists ("How
// Many Lands Do You Need?", ChannelFireball): an intercept, a slope on average
// mana value, and a discount per cheap accelerant.
//
// These three numbers are the only figures in this package that were *read*
// rather than derived, and they are kept together, named, and cited here so
// that stays obvious. Everything they are multiplied by -- the curve, the
// accelerant count, the deck's size -- is measured off the compiled 99.
const (
	fitIntercept         = 19.59
	fitPerManaValue      = 1.90
	fitPerCheapAccelrant = 0.28

	// fitDeckSize is the size the fit was made at. A 99-card singleton deck
	// draws the same fraction of itself per turn as a 60-card deck draws of
	// itself, so the recommendation scales with deck size rather than
	// surviving unchanged.
	fitDeckSize = 60
)

// LandEstimate is a land count from the deck's curve, and everything it did
// not know.
//
// `Caveats` is not decoration. This is a regression fit to 60-card tournament
// decks and rescaled, so it is a starting point rather than an answer, and the
// three inputs it cannot see from a compiled deck are named in it every time.
// Read it against the land sweep, which simulates this actual deck and prices
// flood, and against `LandsNow`.
type LandEstimate struct {
	LandsNow         int      `json:"lands_now"`
	Recommended      int      `json:"recommended"`
	AverageManaValue float64  `json:"average_mana_value"`
	CheapAccelerants int      `json:"cheap_accelerants"`
	DeckSize         int      `json:"deck_size"`
	Caveats          []string `json:"caveats"`
}

// Delta is what the fit would change the land count by.
func (e LandEstimate) Delta() int { return e.Recommended - e.LandsNow }

// RegressionLands is a land count for this deck's curve, in arithmetic rather
// than games.
//
// Three inputs, all measured here: the average mana value of the nonland
// cards, the number of cheap accelerants, and the deck's size. The published
// fit carries three more terms this cannot supply, and they are returned as
// caveats rather than silently treated as zero -- each one would lower the
// recommendation, so the number errs high and says so.
func RegressionLands(library []sim.Card) LandEstimate {
	deckSize := len(library)
	landsNow := 0
	spells := make([]sim.Card, 0, len(library))
	for _, c := range library {
		if c.IsLand {
			landsNow++
		} else {
			spells = append(spells, c)
		}
	}
	if len(spells) == 0 || deckSize == 0 {
		return LandEstimate{
			LandsNow: landsNow, Recommended: landsNow, AverageManaValue: 0.0,
			CheapAccelerants: 0, DeckSize: deckSize,
			Caveats: []string{"this deck has no nonland cards to fit a curve to"},
		}
	}

	totalMV := 0
	accelerants := 0
	for _, c := range spells {
		totalMV += c.MV()
		// Karsten's discount is for cheap card draw *and* cheap ramp. A
		// compiled card knows what it produces and knows nothing about drawing
		// cards, so only the ramp half is counted and the other half is
		// declared below.
		if c.IsRamp() && c.MV() <= 2 {
			accelerants++
		}
	}
	average := float64(totalMV) / float64(len(spells))

	fitted := fitIntercept + floats.Rounded(fitPerManaValue*average) - floats.Rounded(fitPerCheapAccelrant*float64(accelerants))
	// No guard on this one: a multiply feeding a *divide* has no fused form.
	scaled := fitted * float64(deckSize) / fitDeckSize

	recommended := floats.Round(scaled)
	if recommended < 0 {
		recommended = 0
	}
	return LandEstimate{
		LandsNow:         landsNow,
		Recommended:      recommended,
		AverageManaValue: floats.RoundTo(average, 2),
		CheapAccelerants: accelerants,
		DeckSize:         deckSize,
		Caveats: []string{
			"fitted to 60-card tournament decks and scaled to this deck's size, " +
				"not fitted to Commander",
			"cheap card draw is not counted -- a compiled card knows what it " +
				"produces, not what it draws -- so this reads high for a deck that " +
				"cantrips",
			"fast mana counts once here however much it makes, because the pool " +
				"reports which colours a card produces and not how many",
		},
	}
}

// ------------------------------------------------------------- the shelf

// Shelf is everything the closed form has to say about one deck.
//
// The name is Aaron's: a shelf of reference numbers you can take down beside
// the simulation rather than instead of it.
type Shelf struct {
	DeckSize     int                `json:"deck_size"`
	Lands        int                `json:"lands"`
	Target       float64            `json:"target"`
	OnThePlay    bool               `json:"on_the_play"`
	Colors       []ColorRequirement `json:"colors"`
	LandEstimate LandEstimate       `json:"land_estimate"`
	Odds         []CardOdds         `json:"odds"`
	// Approximated names the cards whose coloured demand spans two or more
	// colours, for which `CastableOdds` is an approximation rather than exact.
	// Named rather than counted, because "which of my cards is this softest
	// about" is a question with a useful answer.
	Approximated []string `json:"approximated"`
}

// Unmet is the colours with at least one rung the deck does not meet.
func (s Shelf) Unmet() []ColorRequirement {
	out := []ColorRequirement{}
	for _, c := range s.Colors {
		if !c.Met() {
			out = append(out, c)
		}
	}
	return out
}

type askKey struct {
	color string
	pips  int
}

// Read is `karsten.shelf`: the whole closed form off one compiled deck.
//
// The commander is included in the card-by-card odds and in what sets each
// colour's requirement -- it is the one card you are guaranteed to be casting
// every game, so a mana base that cannot support it is the first thing to know
// -- but it is not part of the 99 the probabilities are drawn from, because it
// is not in the library. Pass nil for a deck with no commander.
func Read(library []sim.Card, commander *sim.Card, target float64, onThePlay bool) Shelf {
	deckSize := len(library)
	lands := 0
	for _, c := range library {
		if c.IsLand {
			lands++
		}
	}
	considered := make([]sim.Card, 0, len(library)+1)
	considered = append(considered, library...)
	if commander != nil {
		considered = append(considered, *commander)
	}

	// ---- colour requirements: a ladder per colour ------------------------
	// Every (colour, pip count) the deck actually asks for, and the cheapest
	// card asking it. The cheapest is the right one to judge on: it is the
	// earliest turn the deck puts the question, so a rung that passes there
	// passes everywhere later.
	asked := map[askKey][]sim.Card{}
	for _, card := range considered {
		for _, d := range pipDemand(card.Cost) {
			// A hybrid pip is charged to every colour that can pay it: each of
			// those colours could be the one carrying the card, so each has to
			// be able to. It is the honest reading of "how many white sources
			// do I need" for a deck holding {G/W} cards.
			//
			// A card demanding both {G/W} and {G} therefore lands in ("G", 1)
			// twice, which is Python's behaviour and is reproduced rather than
			// tidied: the tier's card list is what a reader sees.
			for _, color := range d.colors {
				key := askKey{color, d.pips}
				asked[key] = append(asked[key], card)
			}
		}
	}

	colorsAsked := []string{}
	seenColor := map[string]bool{}
	for key := range asked {
		if !seenColor[key.color] {
			seenColor[key.color] = true
			colorsAsked = append(colorsAsked, key.color)
		}
	}
	sort.Strings(colorsAsked)

	requirements := make([]ColorRequirement, 0, len(colorsAsked))
	for _, color := range colorsAsked {
		want := map[string]bool{color: true}
		have := SourcesFor(library, want)
		haveLands := 0
		for _, c := range library {
			if !c.IsLand {
				continue
			}
			for _, s := range c.Produces {
				if s.Makes(want) {
					haveLands++
					break
				}
			}
		}
		pipCounts := []int{}
		for key := range asked {
			if key.color == color {
				pipCounts = append(pipCounts, key.pips)
			}
		}
		sort.Ints(pipCounts)

		tiers := make([]PipTier, 0, len(pipCounts))
		for _, pips := range pipCounts {
			cards := append([]sim.Card(nil), asked[askKey{color, pips}]...)
			sort.SliceStable(cards, func(i, j int) bool {
				if cards[i].MV() != cards[j].MV() {
					return cards[i].MV() < cards[j].MV()
				}
				return cards[i].Name < cards[j].Name
			})
			turn := cards[0].MV()
			if turn < 1 {
				turn = 1
			}
			names := make([]string, len(cards))
			for i, c := range cards {
				names[i] = c.Name
			}
			tiers = append(tiers, PipTier{
				Pips: pips,
				Turn: turn,
				Need: RequiredSources(deckSize, pips, turn, target, onThePlay),
				Have: have,
				OddsNow: HypergeometricAtLeast(
					deckSize, have, CardsSeen(turn, onThePlay), pips),
				Cards: names,
			})
		}
		requirements = append(requirements, ColorRequirement{
			Color: color, Have: have, HaveLands: haveLands, Tiers: tiers,
		})
	}

	// ---- per card, per turn ----------------------------------------------
	odds := []CardOdds{}
	approximated := []string{}
	seenNames := map[string]bool{}
	for _, card := range considered {
		if card.IsLand || seenNames[card.Name] {
			continue
		}
		seenNames[card.Name] = true
		byTurn := make([]float64, Horizon)
		for t := 1; t <= Horizon; t++ {
			byTurn[t-1] = CastableOdds(card, library, t, onThePlay)
		}
		odds = append(odds, CardOdds{Name: card.Name, MV: card.MV(), ByTurn: byTurn})

		demand := pipDemand(card.Cost)
		colorsDemanded := map[string]bool{}
		for _, d := range demand {
			for _, c := range d.colors {
				colorsDemanded[c] = true
			}
		}
		if len(demand) > 1 && len(colorsDemanded) > 1 {
			approximated = append(approximated, card.Name)
		}
	}

	// Latest first, by how far a card runs behind its own cost. Sorting by raw
	// castability instead just ranks the deck by mana value -- it leads with
	// every twelve-drop and reports that expensive cards are expensive, which
	// is the one thing the reader already knew. Lag leads with the three-drop
	// that lands on turn six, which is the row worth acting on.
	sort.SliceStable(odds, func(i, j int) bool {
		if a, b := odds[i].Lateness(), odds[j].Lateness(); a != b {
			return a > b
		}
		if odds[i].MV != odds[j].MV {
			return odds[i].MV < odds[j].MV
		}
		return odds[i].Name < odds[j].Name
	})

	sort.Strings(approximated)

	return Shelf{
		DeckSize:     deckSize,
		Lands:        lands,
		Target:       target,
		OnThePlay:    onThePlay,
		Colors:       requirements,
		LandEstimate: RegressionLands(library),
		Odds:         odds,
		Approximated: approximated,
	}
}
