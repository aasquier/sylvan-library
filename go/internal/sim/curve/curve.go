// Package curve is `sim/curve.py`: the mana curve. Do you have T mana on turn
// T, and what fixes it if not.
//
// Aaron asked for a land count that guarantees a land drop every turn. **The
// answer is 54 lands, and that is the finding rather than the feature.** You
// need T lands among the 6+T cards you have seen, and that ratio climbs toward
// 100%, so the requirement grows without bound: 48 lands to make every drop
// through turn 3 at 90% confidence, 54 through turn 4, 59 through turn 5. At a
// real 34-36 lands every deck in this library makes all its drops through turn
// 4 about half the time. "A land drop every turn" is only ever answerable as
// "through turn *what*", and the honest answer to the question as asked is
// that no deck you would build gets there. `LandsForEveryDrop` exists to talk
// somebody out of it.
//
// So the question is reframed, and the reframing is the useful part: **do you
// have T mana available on turn T?** A land drop is only one way to get it,
// and for these decks it is not the main one -- ramp is worth between 28 and
// 58 percentage points by turn six.
//
// # The formula
//
//	E[mana on turn T] = E[min(T, lands drawn)] + sum(accelerants online at T)
//
// Two details carry it, and both were found by being wrong first.
//
// **The one-land-per-turn cap goes inside the expectation.** `E[min(T,
// lands)]` is not `min(T, E[lands])`; the second is optimistic in exactly the
// region where flooding begins, because it lets a hand of nine lands on turn
// four count as nine.
//
// **A land-fetch spell is not a mana rock.** Cultivate compiles with no
// `Produces` at all and a `FetchesLands` of two, and it does not use your land
// drop -- so it adds *on top of* the cap rather than through it. Leaving it
// out biased the whole formula by -0.54 mana, and the shape of the error is
// what named the missing term: Esper Tivit was accurate to +0.1 while every
// green deck sat about 0.9 low. **A systematic bias that varies by deck colour
// is a missing term, not noise.**
//
// Validated against 3,000 Tier 1 games per deck on 2026-08-21, six decks by
// six turns: **mean error -0.06 mana, mean absolute error 0.25.**
//
// # Lands or ramp, and the rule that decides it
//
// Recommending either is Aaron's ruling of 2026-08-21. It is a different
// *kind* of advice from anything else in this project -- every other
// recommendation is about which card, and this one is about which kind of card
// -- so it was built and then measured, and the measurement changed the
// design.
//
// **Asked for "T mana on turn T", ramp never wins.** Six decks by five target
// turns, thirty comparisons, and a land was ahead or level in every one. That
// is not a bug and it is not ramp being bad; it is the objective. A land gives
// one mana per turn and you may play one a turn, so at exactly T mana on turn
// T a land is the most reliable route there and nothing can beat it.
//
// The parameter that makes the question real is **how much mana you want**,
// not just when:
//
//	up to the curve, lands.  past the curve, ramp.
//
// A fifth land does **nothing at all** for turn four -- you cannot play it --
// while an accelerant can push you past the cap. Measured at turn four: at
// four mana lands win or tie in all six decks; at five, six and seven mana
// **ramp wins in all six, at every level.** That is why `OnCurveOdds` takes a
// `need` and why the surface asks for a mana target rather than assuming one.
//
// Where the two are within `TooClose` the advice says "either" rather than
// resolving a tie with a coin.
//
// # One ulp, and the interpreter it came from
//
// The two float sums in this package go through `floats.Fsum`, and the Python
// they are ported from says `fsum` for the same reason -- **since
// 2026-08-22, and the port is why**. Both said `sum` before that, and
// CPython 3.12 gave `sum()` compensated (Neumaier) accumulation over floats
// where 3.11 adds them left to right. Same deck, same arithmetic, one ulp
// apart, on a project that supports both interpreters and ships 3.12 in the
// container. `ExpectedLandsInPlay` and `OnCurveOdds` were the two lines
// affected; the accumulations written as explicit loops never were.
//
// It matters because the outputs of this package are an integer and a word:
// `slotsToTarget` scans against `>= target` and `Curve` branches on
// `abs(perLand - perRamp) < TooClose`. A one-ulp difference is a different
// slot count or a different recommendation, not a different last decimal.
//
// No sampling, no seed, no model. Every number here has a right answer, and it
// is `karsten`'s arithmetic underneath -- `Exactly`, `CardsSeen` and
// `HypergeometricAtLeast` are imported from there exactly as Python imports
// them, so the two shelves cannot drift apart about what a hypergeometric is.
package curve

import (
	"math"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
)

// Horizon is the turns reported. Matches `karsten.Horizon` deliberately: two
// shelves on one screen that stopped at different turns would invite a
// comparison neither supports.
const Horizon = 10

// DefaultTargetTurn is the turn the advice is aimed at unless the caller says
// otherwise. Four, because it is where most Commander decks are trying to
// deploy something that matters and where the land-only answer has already
// fallen to a coin flip. Exposed as a control -- Aaron's ruling -- because a
// cEDH deck cares about turn two and a battlecruiser deck about turn six.
const DefaultTargetTurn = 4

// DefaultTarget is the consistency bar the advice measures against unless the
// caller sets one. The same figure `karsten.Target` uses, and on the Simulator
// the same control drives both -- one "how often do you want this to work"
// dial rather than two that could disagree.
const DefaultTarget = 0.90

// TooClose is odds. Below this difference the advice refuses to choose between
// a land and an accelerant, because the closed form's own agreement with Tier
// 1 is several points and a recommendation finer than the instrument is a
// guess wearing a confident face.
const TooClose = 0.01

// Piece is an accelerant reduced to what the formula needs: what it costs,
// how much it makes, and how long after it lands before it makes it.
type Piece struct {
	Cost   int `json:"cost"`
	Output int `json:"output"`
	Delay  int `json:"delay"`
}

// genericRock is what a generic two-mana rock is worth, for a deck that runs
// no accelerants at all and so has no profile to average. A Signet: costs two,
// makes one, usable the turn after it lands.
var genericRock = Piece{Cost: 2, Output: 1, Delay: 0}

// Accelerants is every nonland mana source in the library, as
// `(cost, output, delay)`, in the deck's own order.
//
// Both kinds are here and they are not the same thing. A permanent that
// produces mana carries its `ProduceDelay` -- a mana creature is summoning
// sick and pays for itself a turn late. A spell that *fetches lands* has no
// `Produces` at all; it resolves into lands that are usable immediately, so
// its delay is zero, and crucially it does not consume the land drop.
func Accelerants(library []sim.Card) []Piece {
	out := []Piece{}
	for _, card := range library {
		if card.IsLand {
			continue
		}
		if len(card.Produces) > 0 {
			amount := 0
			for _, s := range card.Produces {
				amount += s.Amount
			}
			out = append(out, Piece{Cost: card.MV(), Output: amount, Delay: card.ProduceDelay})
		} else if card.FetchesLands > 0 {
			out = append(out, Piece{Cost: card.MV(), Output: card.FetchesLands, Delay: 0})
		}
	}
	return out
}

// ExpectedLandsInPlay is E[min(turn, lands drawn)] -- lands actually on the
// battlefield.
//
// The cap is applied inside the expectation because you may play one land a
// turn. Outside it, `min(turn, mean)` counts a seven-land hand on turn three
// as seven, which is precisely the flooding case the number exists to price.
func ExpectedLandsInPlay(deckSize, lands, turn int, onThePlay bool) float64 {
	if deckSize <= 0 || lands <= 0 || turn <= 0 {
		return 0.0
	}
	seen := karsten.CardsSeen(turn, onThePlay)
	if seen > deckSize {
		seen = deckSize
	}
	top := seen
	if lands < top {
		top = lands
	}
	// `floats.Fsum`, matching Python's `fsum` -- and that line was `sum` on both
	// sides until 2026-08-22. `sum` over floats is compensated on CPython 3.12
	// and left to right on 3.11, so this expectation answered differently
	// depending on the interpreter underneath it. The port found it and both
	// runtimes were fixed at once; `sim/curve.py`'s docstring carries the
	// argument for choosing the correctly-rounded answer over either.
	terms := make([]float64, 0, top+1)
	for k := 0; k <= top; k++ {
		capped := k
		if turn < capped {
			capped = turn
		}
		terms = append(terms, floats.Rounded(float64(capped)*karsten.Exactly(deckSize, lands, seen, k)))
	}
	return floats.Fsum(terms)
}

// ExpectedRamp is mana from accelerants that are online on `turn`.
//
// "Online" under Aaron's own framing -- everything played on the most
// reasonable turn -- means drawn early enough to have been cast and to have
// shed any summoning sickness: drawn by turn `turn - delay`, and cheap enough
// that `cost + delay` has arrived at all.
//
// Each accelerant is a singleton, so the chance of having drawn it by turn X
// is simply the fraction of the deck seen by then.
func ExpectedRamp(library []sim.Card, turn int, onThePlay bool) float64 {
	deckSize := len(library)
	if deckSize <= 0 {
		return 0.0
	}
	total := 0.0
	for _, p := range Accelerants(library) {
		if p.Cost+p.Delay > turn {
			continue
		}
		seen := karsten.CardsSeen(turn-p.Delay, onThePlay)
		total += floats.Rounded(float64(p.Output) * atMostOne(float64(seen)/float64(deckSize)))
	}
	return total
}

// atMostOne is Python's `min(1.0, x)`, which returns x only when it is
// strictly below one -- the distinction that keeps a negative zero out.
func atMostOne(x float64) float64 {
	if x < 1.0 {
		return x
	}
	return 1.0
}

// LandDistribution is P(exactly t lands in play on `turn`), for t in 0..turn.
//
// Everything at or above the cap piles into the last bucket, because a seventh
// land on turn four is not a seventh mana -- it is a card you did not get to
// play.
func LandDistribution(deckSize, lands, turn int, onThePlay bool) []float64 {
	size := turn + 1
	if size < 0 {
		// Python's `[0.0] * -1` is the empty list, and a negative turn reaches
		// here from a caller that did not guard.
		size = 0
	}
	dist := make([]float64, size)
	if deckSize <= 0 || turn <= 0 {
		return dist
	}
	seen := karsten.CardsSeen(turn, onThePlay)
	if seen > deckSize {
		seen = deckSize
	}
	top := seen
	if lands < top {
		top = lands
	}
	for k := 0; k <= top; k++ {
		at := k
		if turn < at {
			at = turn
		}
		dist[at] += karsten.Exactly(deckSize, lands, seen, k)
	}
	return dist
}

// RampDistribution is P(exactly m mana from accelerants on `turn`), for m in
// 0..total.
//
// A small dynamic program over the accelerants, each an independent Bernoulli:
// drawn by the turn it needs to be, or not. Independence between *accelerants*
// is a mild approximation -- they are drawn from one deck without replacement,
// so they are weakly negatively correlated -- and it is a much safer one than
// the independence this package refuses elsewhere. `karsten.CastableOdds` has
// to condition on the draw because "four lands" and "one green source" are
// very nearly the same event; two different rocks are not.
//
// `extra` adds `extraCount` hypothetical copies, which is how the advice below
// asks "what would one more do" without rebuilding a deck to find out. Pass
// nil for none.
func RampDistribution(library []sim.Card, turn int, onThePlay bool, extra *Piece, extraCount int) []float64 {
	deckSize := len(library)
	pieces := []Piece{}
	for _, p := range Accelerants(library) {
		if p.Cost+p.Delay <= turn {
			pieces = append(pieces, p)
		}
	}
	if extra != nil && extraCount > 0 && extra.Cost+extra.Delay <= turn {
		for i := 0; i < extraCount; i++ {
			pieces = append(pieces, *extra)
		}
	}
	dist := []float64{1.0}
	if deckSize <= 0 {
		return dist
	}
	for _, p := range pieces {
		seen := karsten.CardsSeen(turn-p.Delay, onThePlay)
		prob := atMostOne(float64(seen) / float64(deckSize))
		next := make([]float64, len(dist)+p.Output)
		for total, weight := range dist {
			if weight == 0.0 {
				continue
			}
			next[total] += floats.Rounded(weight * (1.0 - prob))
			next[total+p.Output] += floats.Rounded(weight * prob)
		}
		dist = next
	}
	return dist
}

// Extra is the hypothetical additions `OnCurveOdds` prices: some number of
// extra lands, and some number of copies of one accelerant.
type Extra struct {
	Lands     int
	Ramp      *Piece
	RampCount int
}

// OnCurveOdds is P(at least `need` mana available on `turn`).
//
// **`need` is the parameter that makes "lands or ramp" a real question**, and
// it was added because without it the answer was always "lands". See the
// package comment: a land is capped at one mana per turn, so at `need == turn`
// it is the most reliable way to get there and ramp cannot beat it; at `need >
// turn` a land is worth *nothing at all* and ramp is the only thing that can
// help.
//
// **This is the number the advice is keyed on, and the reason expectation was
// not enough.** Every deck in this library expects four or more mana on turn
// four, and every one of them still misses it a third of the time or worse --
// an average is exactly the statistic that hides a coin flip.
//
// Lands and ramp are convolved as independent. They are drawn from one deck,
// so that is an approximation; it is a defensible one because they are
// different cards competing only for slots, and the alternative -- a joint
// distribution over the whole 99 -- is not something anybody could read off a
// screen even if it were cheap.
//
// `need` is a pointer because Python's is `None`-defaulting and **zero is a
// real value there**: `need=0` asks for no mana at all and correctly answers
// 1.0, while `need=None` asks for `turn`. A sentinel would have collapsed the
// two. `extra` may be nil.
func OnCurveOdds(library []sim.Card, turn int, need *int, onThePlay bool, extra *Extra) float64 {
	deckSize := len(library)
	if deckSize <= 0 || turn <= 0 {
		return 0.0
	}
	want := turn
	if need != nil {
		want = *need
	}
	lands := 0
	for _, c := range library {
		if c.IsLand {
			lands++
		}
	}
	var extraRamp *Piece
	extraCount := 0
	if extra != nil {
		lands += extra.Lands
		extraRamp = extra.Ramp
		extraCount = extra.RampCount
	}
	landDist := LandDistribution(deckSize, lands, turn, onThePlay)
	rampDist := RampDistribution(library, turn, onThePlay, extraRamp, extraCount)

	total := 0.0
	for landCount, lw := range landDist {
		if lw == 0.0 {
			continue
		}
		short := want - landCount
		switch {
		case short <= 0:
			total += lw
		case short < len(rampDist):
			// The inner sum is `Fsum` for the interpreter reason above. The
			// outer accumulation is a loop in Python too, so it never had the
			// problem and stays a running total.
			total += floats.Rounded(lw * floats.Fsum(rampDist[short:]))
		}
	}
	return atMostOne(total)
}

// LandsForEveryDrop is the lands needed to make **every** drop through `turn`,
// or nil if no land count in the deck reaches the target.
//
// The binding constraint is the last turn, not a product over all of them:
// lands seen only grows, so holding `turn` lands among the cards seen by
// `turn` means you held enough at every earlier turn too. That makes this one
// hypergeometric rather than a dependent chain, which is worth knowing because
// the chain is the thing people compute by mistake.
//
// Nil is a real answer and happens sooner than anybody expects.
func LandsForEveryDrop(deckSize, turn int, target float64, onThePlay bool) *int {
	seen := karsten.CardsSeen(turn, onThePlay)
	for count := turn; count <= deckSize; count++ {
		if karsten.HypergeometricAtLeast(deckSize, count, seen, turn) >= target {
			n := count
			return &n
		}
	}
	return nil
}

// TypicalAccelerant is one more accelerant *like the ones this deck already
// plays*.
//
// Averaged over the deck's own pieces rather than over an idealised one,
// because "add more ramp" means adding the kind of ramp this deck runs -- a
// deck of three-mana sorceries does not acquire a Sol Ring by being told to
// accelerate. Only pieces that could be online by the target turn are
// averaged; a six-mana rock is not ramp for turn four.
//
// The second return is true when the deck has no accelerants at all, so a
// plain Signet stands in -- advice built on a stand-in has to admit that it
// is.
func TypicalAccelerant(library []sim.Card, turn int) (Piece, bool) {
	usable := []Piece{}
	for _, p := range Accelerants(library) {
		if p.Cost+p.Delay <= turn {
			usable = append(usable, p)
		}
	}
	if len(usable) == 0 {
		return genericRock, true
	}
	sumCost, sumOutput, sumDelay := 0, 0, 0
	for _, p := range usable {
		sumCost += p.Cost
		sumOutput += p.Output
		sumDelay += p.Delay
	}
	n := float64(len(usable))
	cost := floats.Round(float64(sumCost) / n)
	output := floats.Round(float64(sumOutput) / n)
	delay := floats.Round(float64(sumDelay) / n)
	if cost < 0 {
		cost = 0
	}
	if output < 1 {
		output = 1
	}
	if delay < 0 {
		delay = 0
	}
	return Piece{Cost: cost, Output: output, Delay: delay}, false
}

// Turn is one turn: what you can expect, where it came from, and the odds.
//
// `Odds` and `ExpectedMana` answer different questions, and carrying both is
// the point rather than redundancy. **Every deck in this library expects four
// or more mana on turn four, and every one of them still misses it a quarter
// of the time or worse** -- an average is exactly the statistic that hides a
// coin flip. So the advice is keyed on the odds, and the expectation is kept
// for the one thing it gives that the odds cannot: how much of the mana came
// from lands and how much from ramp.
type Turn struct {
	Turn      int     `json:"turn"`
	FromLands float64 `json:"from_lands"`
	FromRamp  float64 `json:"from_ramp"`
	// LandDropOdds is P(a land available every turn up to and including this
	// one).
	LandDropOdds float64 `json:"land_drop_odds"`
	// Odds is P(at least `Turn` mana available on `Turn`). The headline
	// figure.
	Odds float64 `json:"odds"`
}

// ExpectedMana is the two halves added back together.
func (t Turn) ExpectedMana() float64 { return t.FromLands + t.FromRamp }

// Advice is what to add to hit `TargetMana` on `TargetTurn`, and how sure it
// is. `Recommend` is one of `lands`, `ramp`, `either` or `none`.
type Advice struct {
	TargetTurn int `json:"target_turn"`
	TargetMana int `json:"target_mana"`
	// Target is the consistency bar this is measured against.
	Target float64 `json:"target"`
	// Odds is P(TargetMana available on TargetTurn), as things stand.
	Odds float64 `json:"odds"`
	// OddsPerLand and OddsPerRamp are the odds after one more of each, so the
	// reader sees the comparison rather than being handed its conclusion.
	OddsPerLand float64 `json:"odds_per_land"`
	OddsPerRamp float64 `json:"odds_per_ramp"`
	Recommend   string  `json:"recommend"`
	// Slots of the recommended kind needed to reach Target, or nil when twenty
	// of them still would not.
	Slots *int `json:"slots"`
	// RampIsGeneric is true when the deck runs no accelerants and
	// OddsPerRamp stands in.
	RampIsGeneric bool `json:"ramp_is_generic"`
	// BeyondTheCurve is true when TargetMana exceeds TargetTurn -- the region
	// where a land is worth nothing and only ramp can help. The reason this
	// feature has two controls rather than one.
	BeyondTheCurve bool `json:"beyond_the_curve"`
	// LandsForEveryDrop would make every land drop through the target turn, or
	// nil. Almost always absurd, and carried for exactly that reason.
	LandsForEveryDrop *int `json:"lands_for_every_drop"`
}

// ManaCurve is the whole closed form for one deck's mana.
type ManaCurve struct {
	DeckSize    int     `json:"deck_size"`
	Lands       int     `json:"lands"`
	Accelerants int     `json:"accelerants"`
	TargetTurn  int     `json:"target_turn"`
	TargetMana  int     `json:"target_mana"`
	Target      float64 `json:"target"`
	OnThePlay   bool    `json:"on_the_play"`
	Turns       []Turn  `json:"turns"`
	Advice      Advice  `json:"advice"`
}

// slotsToTarget is how many added lands or accelerants reach `target`, or nil.
//
// A search rather than a division, because odds are not linear in slots: the
// tenth land buys far less than the first, and dividing a shortfall by a
// marginal rate quietly assumes otherwise. Capped at twenty, past which the
// honest answer is that this deck does not get there by adding one kind of
// card.
func slotsToTarget(library []sim.Card, turn, need int, target float64,
	onThePlay bool, kind string, piece Piece) *int {
	for count := 1; count <= 20; count++ {
		var odds float64
		if kind == "lands" {
			odds = OnCurveOdds(library, turn, &need, onThePlay, &Extra{Lands: count})
		} else {
			odds = OnCurveOdds(library, turn, &need, onThePlay, &Extra{Ramp: &piece, RampCount: count})
		}
		if odds >= target {
			n := count
			return &n
		}
	}
	return nil
}

// Options are `curve`'s keyword arguments.
//
// `TargetTurn` and `Target` have real defaults in Python rather than `None`
// ones, and **zero is a legal value for each** -- `curve(target_turn=0)`
// clamps to turn 1, and a test pins that -- so they are plain fields and
// `DefaultOptions` is how you ask for the defaults. `TargetMana` is `None` in
// Python and a pointer here for the same reason `OnCurveOdds`'s `need` is.
//
// `OnTheDraw` is inverted from Python's `on_the_play=True` deliberately, so
// that the zero value is the default and the harder case.
type Options struct {
	TargetTurn int
	TargetMana *int
	Target     float64
	OnTheDraw  bool
}

// DefaultOptions is `curve(library)` with nothing passed.
func DefaultOptions() Options {
	return Options{TargetTurn: DefaultTargetTurn, Target: DefaultTarget}
}

// Curve is `curve.curve`: the whole mana curve for one compiled deck, and what
// to do about it.
//
// `TargetMana` defaults to `TargetTurn` -- the on-curve question. Asking for
// more is what turns this into a question about ramp; see the package comment
// for why that is not a preference but arithmetic.
func Curve(library []sim.Card, opts Options) ManaCurve {
	deckSize := len(library)
	lands := 0
	for _, c := range library {
		if c.IsLand {
			lands++
		}
	}
	onThePlay := !opts.OnTheDraw

	targetTurn := opts.TargetTurn
	if targetTurn > Horizon {
		targetTurn = Horizon
	}
	if targetTurn < 1 {
		targetTurn = 1
	}
	want := targetTurn
	if opts.TargetMana != nil {
		want = *opts.TargetMana
		if want > 20 {
			want = 20
		}
		if want < 1 {
			want = 1
		}
	}
	target := opts.Target
	if target > 0.99 {
		target = 0.99
	}
	if target < 0.5 {
		target = 0.5
	}

	turns := make([]Turn, 0, Horizon)
	for t := 1; t <= Horizon; t++ {
		turns = append(turns, Turn{
			Turn:      t,
			FromLands: ExpectedLandsInPlay(deckSize, lands, t, onThePlay),
			FromRamp:  ExpectedRamp(library, t, onThePlay),
			LandDropOdds: karsten.HypergeometricAtLeast(
				deckSize, lands, karsten.CardsSeen(t, onThePlay), t),
			Odds: OnCurveOdds(library, t, nil, onThePlay, nil),
		})
	}

	odds := OnCurveOdds(library, targetTurn, &want, onThePlay, nil)
	piece, generic := TypicalAccelerant(library, targetTurn)
	perLand := OnCurveOdds(library, targetTurn, &want, onThePlay, &Extra{Lands: 1})
	perRamp := OnCurveOdds(library, targetTurn, &want, onThePlay, &Extra{Ramp: &piece, RampCount: 1})

	// The two branches genuinely differ: a met target has zero slots to add,
	// and an unmet one may have no reachable count at all. Note that the
	// comparisons are against the *unrounded* odds -- the rounding below is
	// what the wire shows, never what the decision is made on.
	var recommend string
	var slots *int
	if odds >= target {
		recommend = "none"
		zero := 0
		slots = &zero
	} else {
		var kind string
		switch {
		case math.Abs(perLand-perRamp) < TooClose:
			recommend = "either"
			kind = "ramp"
			if perLand >= perRamp {
				kind = "lands"
			}
		case perRamp > perLand:
			recommend, kind = "ramp", "ramp"
		default:
			recommend, kind = "lands", "lands"
		}
		slots = slotsToTarget(library, targetTurn, want, target, onThePlay, kind, piece)
	}

	return ManaCurve{
		DeckSize:    deckSize,
		Lands:       lands,
		Accelerants: len(Accelerants(library)),
		TargetTurn:  targetTurn,
		TargetMana:  want,
		Target:      target,
		OnThePlay:   onThePlay,
		Turns:       turns,
		Advice: Advice{
			TargetTurn:        targetTurn,
			TargetMana:        want,
			Target:            target,
			Odds:              floats.RoundTo(odds, 4),
			OddsPerLand:       floats.RoundTo(perLand, 4),
			OddsPerRamp:       floats.RoundTo(perRamp, 4),
			Recommend:         recommend,
			Slots:             slots,
			RampIsGeneric:     generic,
			BeyondTheCurve:    want > targetTurn,
			LandsForEveryDrop: LandsForEveryDrop(deckSize, targetTurn, target, onThePlay),
		},
	}
}

// GenericRock is the stand-in accelerant, exposed for the test that pins it.
func GenericRock() Piece { return genericRock }
