// Package tier1 is the stochastic goldfish.
//
// It shuffles, draws, plays a land, and pays costs until nothing else can be
// paid for. It does NOT model opponents, interaction, tutors, cost reduction,
// or card text beyond mana production and land-fetch ramp -- and nothing
// here improves on any of that, deliberately. Every number a primer quotes
// comes
// out of here, so a "better" engine is a silently different product.
//
// # What makes this engine checkable
//
// Tier 1 consumes randomness through exactly one call -- the shuffle
// in SimulateGame -- and `internal/mt19937` reproduces the recorded
// generator bit
// for bit, held to a corpus that includes all 99,274 draws of the reference
// run below. So the dice are a solved problem, separately from the engine:
// a divergence here is an engine divergence, and the fixture in
// testdata says which game and which field.
//
// The gate is ReferenceDigest, a sha256 over
// the canonical rendering of one game, one 300-game run and a three-point
// land sweep. That
// is why this package carries that rendering (repr.go): the digest is a hash
// of text, so reproducing the numbers is not enough -- the *rendering* is
// part of the equivalence. Nothing serves those strings; they exist to be
// hashed.
//
// # The five details a reimplementation gets wrong
//
// **Removal takes the first EQUAL element, not the one you handed
// it.** A compiled deck repeats one card per `qty`, so a hand of basics is
// a hand of equal cards; taking out the wrong one reorders the rest, and the
// order of the rest picks the next land. `removeFirstEqual` is that, and
// `sim.Card.Equal` is why it can be.
//
// **The extremes keep the FIRST extreme.** `pickLand` maxes over lands
// and the casting loop mins over castable spells; a sort or a `>=` would
// break ties the other way and play a different land on turn three.
//
// **The commander is matched by identity, not equality.** It is a
// separate object from anything in the library even when a name matches, and
// it is never removed from hand because it was never in hand.
//
// **The tables are ordered.** `FirstCast` and the timing table iterate in
// insertion order, and both are rendered into the digest.
//
// **The median of ints is an int for an odd count.** So a median commander
// turn renders `5`, not `5.0`, in the text the digest hashes --
// a whole median stays a whole number. Number carries that.
//
// # What is deliberately absent
//
// The fixed-width text table a run prints in a terminal is the CLI's
// rendering, not an engine answer, and no served route reads it; it lives
// with the CLI. Nothing else is missing.
package tier1

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/aasquier/sylvan-library/go/internal/mana"
	"github.com/aasquier/sylvan-library/go/internal/mt19937"
	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// KeepRule is the mulligan policy, tuned per deck.
//
// MinLands/MaxLands bound the opener's land count. MinManaPieces counts lands
// plus cheap ramp (mv <= CheapRampMV); it is the dial most often worth
// turning, since a ramp-dense deck keeps two-landers a land-light deck
// cannot.
type KeepRule struct {
	MinLands      int `json:"min_lands"`
	MaxLands      int `json:"max_lands"`
	MinManaPieces int `json:"min_mana_pieces"`
	CheapRampMV   int `json:"cheap_ramp_mv"`
	MaxMulligans  int `json:"max_mulligans"`
}

// DefaultKeepRule is every field at its recorded default.
func DefaultKeepRule() KeepRule {
	return KeepRule{MinLands: 2, MaxLands: 5, MinManaPieces: 3,
		CheapRampMV: 2, MaxMulligans: 3}
}

// Keeps reports whether the rule keeps this opening hand.
func (k KeepRule) Keeps(hand []*sim.Card) bool {
	lands := 0
	for _, c := range hand {
		if c.IsLand {
			lands++
		}
	}
	if lands < k.MinLands || lands > k.MaxLands {
		return false
	}
	cheapRamp := 0
	for _, c := range hand {
		if !c.IsLand && c.IsRamp() && c.MV() <= k.CheapRampMV {
			cheapRamp++
		}
	}
	return lands+cheapRamp >= k.MinManaPieces
}

// Describe renders the rule as a sentence, and its exact text is in the
// digest.
func (k KeepRule) Describe() string {
	return fmt.Sprintf("keep %d-%d lands AND lands + ramp(mv<=%d) >= %d",
		k.MinLands, k.MaxLands, k.CheapRampMV, k.MinManaPieces)
}

// FirstCast is a name-to-turn table that iterates in insertion order.
//
// It is a type rather than a map because the order is not an implementation
// detail here: the game's rendering includes it, and the digest hashes that.
type FirstCast struct {
	names []string
	turns map[string]int
}

// SetDefault records the turn only the first
// time a name is seen, which is what makes this a *first* cast.
func (f *FirstCast) SetDefault(name string, turn int) {
	if f.turns == nil {
		f.turns = map[string]int{}
	}
	if _, seen := f.turns[name]; seen {
		return
	}
	f.turns[name] = turn
	f.names = append(f.names, name)
}

// Names is the keys in insertion order.
func (f *FirstCast) Names() []string { return f.names }

// Get is the recorded turn for a name, with the second result reporting
// membership.
func (f *FirstCast) Get(name string) (int, bool) {
	turn, ok := f.turns[name]
	return turn, ok
}

// Len is how many names have been recorded.
func (f *FirstCast) Len() int { return len(f.names) }

// GameResult is one game, as it happened.
type GameResult struct {
	CommanderTurn *int  `json:"commander_turn"`
	LandsByTurn   []int `json:"lands_by_turn"`
	ManaByTurn    []int `json:"mana_by_turn"`
	SpellsByTurn  []int `json:"spells_by_turn"`
	// UnusedByTurn is mana available and never spent. Without it the model
	// has no cost for flooding and would always recommend more lands.
	UnusedByTurn      []int `json:"unused_by_turn"`
	Mulligans         int   `json:"mulligans"`
	ColorScrewedTurns int   `json:"color_screwed_turns"`
	// FirstCast is the turn each named nonland spell was first cast.
	FirstCast FirstCast `json:"-"`
	// MissedDropByTurn is true for a turn where no land was played while
	// none was in hand -- a missed drop, not a chosen one.
	MissedDropByTurn []bool `json:"missed_drop_by_turn"`
	// FirstSpellTurn is the first turn any nonland spell resolved, or nil
	// for a game that never got moving inside the horizon.
	FirstSpellTurn *int `json:"first_spell_turn"`
	// StalledTurns counts turns that ended with nothing cast while a nonland
	// spell sat in hand -- blocked on quantity or colour alike.
	StalledTurns int `json:"stalled_turns"`
}

// expandUnits flattens sources into single mana,
// each unit carrying the colour set it can produce.
//
// It lives here rather than in `internal/sim` because it is the matching's
// own vocabulary and nothing else in the simulator asks for it -- the closed
// forms count sources, never units. The expansion is what makes castability
// correct for a source that makes more than one mana: Sol Ring is two units
// of {C}, not one unit that somehow counts twice.
func expandUnits(sources []sim.Source) [][]string {
	var units [][]string
	for _, src := range sources {
		for i := 0; i < src.Amount; i++ {
			units = append(units, src.Colors)
		}
	}
	return units
}

// consume pays a cost from a pool, and says what is left.
//
// Coloured pips are assigned by exact matching (Kuhn's augmenting paths).
// Generic is then paid from the leftovers, spending the least flexible
// sources first so the survivors stay as useful as possible for the next
// spell this turn.
//
// The second result is load-bearing: a payable cost that
// consumes the whole pool returns an empty slice and true, which is a
// different answer from unpayable and must stay so.
func consume(cost sim.Cost, sources []sim.Source) ([]sim.Source, bool) {
	units := expandUnits(sources)
	// Not the cost's mana value: a Phyrexian symbol is paid with life, so it
	// adds to mana value without ever costing mana here.
	need := cost.Generic + len(cost.Pips)
	if len(units) < need {
		return nil, false
	}

	used := make([]bool, len(units))
	if len(cost.Pips) > 0 {
		adj := make([][]int, len(cost.Pips))
		for i, pip := range cost.Pips {
			for j, unit := range units {
				if sim.Intersects(unit, pip) {
					adj[i] = append(adj[i], j)
				}
			}
		}
		matchUnit := make([]int, len(units))
		for j := range matchUnit {
			matchUnit[j] = -1
		}
		// `seen` persists across the recursion within one augmenting search;
		// resetting it inside would revisit units and loop forever.
		var assign func(pipIdx int, seen []bool) bool
		assign = func(pipIdx int, seen []bool) bool {
			for _, unitIdx := range adj[pipIdx] {
				if seen[unitIdx] {
					continue
				}
				seen[unitIdx] = true
				if matchUnit[unitIdx] == -1 || assign(matchUnit[unitIdx], seen) {
					matchUnit[unitIdx] = pipIdx
					return true
				}
			}
			return false
		}
		for i := range cost.Pips {
			if !assign(i, make([]bool, len(units))) {
				return nil, false
			}
		}
		for j, pip := range matchUnit {
			if pip != -1 {
				used[j] = true
			}
		}
	}

	// Pay generic with the most restrictive remaining units. The recorded
	// order is a stable sort over ascending indices, so units of
	// equal breadth stay in index order -- SliceStable, never Slice.
	leftovers := make([]int, 0, len(units))
	for j := range units {
		if !used[j] {
			leftovers = append(leftovers, j)
		}
	}
	sort.SliceStable(leftovers, func(a, b int) bool {
		return len(units[leftovers[a]]) < len(units[leftovers[b]])
	})
	if len(leftovers) < cost.Generic {
		return nil, false
	}
	for _, j := range leftovers[:cost.Generic] {
		used[j] = true
	}

	out := make([]sim.Source, 0, len(units))
	for j := range units {
		if !used[j] {
			out = append(out, sim.Source{Colors: units[j], Amount: 1})
		}
	}
	return out, true
}

// canPay is `mana.CanPay` at Tier 1's one call site, inside
// `pickLand`'s scoring -- and since 2026-08-22 it really is that call.
//
// It used to answer through `consume` instead, with an argument attached: the
// two run the same matching over the same units and agree by construction.
// The argument was sound, and #243 retired it anyway, because the recorded
// engine answers this question through the shared solver -- **delegating is
// the contract and the local answer was the deviation**, however well
// argued. Two solvers each
// checked against the corpus and neither against the other is one line of
// reasoning away from a silent divergence.
//
// The equivalence it used to assert has not been thrown away -- it has been
// promoted from an argument to a **property test**. `consume` still solves the
// same problem for every other call site (it has to: it returns the
// leftovers), and `TestConsumeAgreesWithCanPay` holds the two to each other
// over random costs and pools.
//
// The conversion is the whole of the seam and it is free by design: #243 laid
// `mana.Source` out field for field like `sim.Source`, in order, so the two
// convert directly. `Cost` does not -- `mana.Cost` puts `HasX` before
// `Phyrexian` -- so that one is written out.
func canPay(cost sim.Cost, sources []sim.Source) bool {
	pool := make([]mana.Source, len(sources))
	for i, s := range sources {
		// Legal because the fields match name for name and type for type;
		// Go's conversion rule ignores the struct tags `sim.Source` carries.
		pool[i] = mana.Source(s)
	}
	// xValue 0: Tier 1 never casts for X, which is why the compiled card
	// carries `HasX` and the engine never reads it.
	return mana.CanPay(mana.Cost{
		Generic:   cost.Generic,
		Pips:      cost.Pips,
		HasX:      cost.HasX,
		Phyrexian: cost.Phyrexian,
	}, pool, 0)
}

// removeFirstEqual removes one card from the list.
//
// The first EQUAL element leaves, not the one that was handed in. See the
// package comment: with a compiled deck those are routinely different cards.
func removeFirstEqual(cards []*sim.Card, card *sim.Card) []*sim.Card {
	for i, c := range cards {
		if c.Equal(*card) {
			return append(cards[:i:i], cards[i+1:]...)
		}
	}
	panic("tier1: removed a card that is not in the list")
}

// pickLand is the land that maximises this turn's
// options.
//
// Ranked by (mana value of the best spell it lets us cast now, new colours it
// adds, untapped) -- the standard human heuristic: untapped when it enables a
// play, otherwise fix colours.
func pickLand(hand []*sim.Card, pool []sim.Source, castables []*sim.Card) *sim.Card {
	var lands []*sim.Card
	for _, c := range hand {
		if c.IsLand {
			lands = append(lands, c)
		}
	}
	if len(lands) == 0 {
		return nil
	}

	haveColors := map[string]bool{}
	for _, src := range pool {
		for _, col := range src.Colors {
			haveColors[col] = true
		}
	}

	score := func(land *sim.Card) [3]int {
		trial := make([]sim.Source, 0, len(pool)+len(land.Produces))
		trial = append(trial, pool...)
		if !land.EntersTapped {
			trial = append(trial, land.Produces...)
		}
		bestMV := 0
		for _, spell := range castables {
			if canPay(spell.Cost, trial) && spell.MV() > bestMV {
				bestMV = spell.MV()
			}
		}
		newColors := 0
		counted := map[string]bool{}
		for _, src := range land.Produces {
			for _, col := range src.Colors {
				if !haveColors[col] && !counted[col] {
					counted[col] = true
					newColors++
				}
			}
		}
		untapped := 1
		if land.EntersTapped {
			untapped = 0
		}
		return [3]int{bestMV, newColors, untapped}
	}

	// `max` keeps the FIRST maximum: replace only on strictly greater.
	best, bestScore := lands[0], score(lands[0])
	for _, land := range lands[1:] {
		s := score(land)
		if s[0] > bestScore[0] ||
			(s[0] == bestScore[0] && s[1] > bestScore[1]) ||
			(s[0] == bestScore[0] && s[1] == bestScore[1] && s[2] > bestScore[2]) {
			best, bestScore = land, s
		}
	}
	return best
}

// GameOptions is SimulateGame's optional levers.
//
// Turns has no default here: a zero simulates zero turns, deliberately --
// the run layer above owns the conventional horizon. RNG nil means an
// unseeded generator, and therefore a game that is not reproducible.
type GameOptions struct {
	Turns     int
	KeepRule  *KeepRule
	RNG       *mt19937.Random
	OnThePlay bool
}

// SimulateGame plays one game, start to horizon.
func SimulateGame(library []*sim.Card, commander *sim.Card, opts GameOptions) GameResult {
	rng := opts.RNG
	if rng == nil {
		rng = mt19937.New(unseededSeed())
	}
	keepRule := DefaultKeepRule()
	if opts.KeepRule != nil {
		keepRule = *opts.KeepRule
	}

	// --- London mulligan ---------------------------------------------
	deck := append([]*sim.Card(nil), library...)
	mulligans := 0
	var hand []*sim.Card
	for {
		mt19937.ShuffleSlice(rng, deck)
		hand = deck[:min(7, len(deck))]
		if keepRule.Keeps(hand) || mulligans >= keepRule.MaxMulligans {
			break
		}
		mulligans++
	}
	libraryLeft := append([]*sim.Card(nil), deck[min(7, len(deck)):]...)
	hand = append([]*sim.Card(nil), hand...)
	// Bottom `mulligans` cards -- put back the least useful (excess lands
	// first, then the highest-cost spells). They are not returned to the
	// library; the recorded engine drops them, and so does this.
	//
	// The bound is computed ONCE, off the hand the loop starts with -- the
	// recorded semantics. A condition re-read each pass shrinks with the
	// hand and
	// stops early -- at five mulligans it bottoms four cards instead of five,
	// and every card drawn after that is a different card. It cannot be seen
	// below four mulligans, so the reference run (max_mulligans 3) is blind to
	// it; the corpus mulligans to nine on purpose.
	toBottom := min(mulligans, len(hand)-1)
	for i := 0; i < toBottom; i++ {
		var landsInHand []*sim.Card
		for _, c := range hand {
			if c.IsLand {
				landsInHand = append(landsInHand, c)
			}
		}
		if len(landsInHand) > keepRule.MinLands {
			hand = removeFirstEqual(hand, landsInHand[len(landsInHand)-1])
		} else {
			// `max` keeps the first maximum.
			worst := hand[0]
			for _, c := range hand[1:] {
				if c.MV() > worst.MV() {
					worst = c
				}
			}
			hand = removeFirstEqual(hand, worst)
		}
	}

	// --- game state ---------------------------------------------------
	type entry struct {
		card    *sim.Card
		entered int
	}
	var battlefield []entry
	var commanderTurn *int
	landsByTurn := []int{}
	manaByTurn := []int{}
	spellsByTurn := []int{}
	unusedByTurn := []int{}
	colorScrewed := 0
	firstCast := FirstCast{}
	missedDropByTurn := []bool{}
	var firstSpellTurn *int
	stalledTurns := 0

	for turn := 1; turn <= opts.Turns; turn++ {
		// The player on the play skips their first draw step, and only that
		// one. Named rather than inlined so the negation reads as the rule:
		// skip only the very first draw, and only on the play.
		skipsTheDraw := turn == 1 && opts.OnThePlay
		if !skipsTheDraw {
			if len(libraryLeft) > 0 {
				hand = append(hand, libraryLeft[0])
				libraryLeft = libraryLeft[1:]
			}
		}

		// Available mana this turn: every permanent past its delay.
		//
		// The delay test here is dead weight and is kept because the recorded
		// engine keeps it: at the top of a turn every entry on the
		// battlefield arrived on
		// an earlier one, so `turn - entered >= 1`, and no compiled card has a
		// delay above 1 (`sim/compile` writes 0 or 1, nothing else). Deleting
		// it changes no result, which a mutation run confirmed -- it is the
		// one surviving mutant in this package, and it survives honestly.
		// Summoning sickness is really enforced in the two places below: the
		// same-turn pool extension after a cast, and the mana_by_turn count.
		var pool []sim.Source
		for _, e := range battlefield {
			if turn-e.entered >= e.card.ProduceDelay {
				pool = append(pool, e.card.Produces...)
			}
		}

		var spellsInHand []*sim.Card
		for _, c := range hand {
			if !c.IsLand {
				spellsInHand = append(spellsInHand, c)
			}
		}
		land := pickLand(hand, pool, spellsInHand)
		if land != nil {
			hand = removeFirstEqual(hand, land)
			battlefield = append(battlefield, entry{land, turn})
			if !land.EntersTapped {
				pool = append(pool, land.Produces...)
			}
		}
		// A missed drop is having none to play, not choosing not to --
		// pickLand always plays one when the hand holds one.
		//
		// Which makes the second half of the test unreachable, and it is kept
		// unreachable on purpose: `pickLand` returns nil exactly when the hand
		// holds no land, so `land == nil` already implies the scan below finds
		// none. The recorded engine tests both halves, so this tests both
		// too. Simplifying it would be correct today and would quietly stop
		// being correct the day `pickLand` learns to decline a land drop.
		missed := land == nil
		if missed {
			for _, c := range hand {
				if c.IsLand {
					missed = false
					break
				}
			}
		}
		missedDropByTurn = append(missedDropByTurn, missed)

		// --- casting ---------------------------------------------------
		castCount := 0
		for {
			var options []*sim.Card
			for _, c := range hand {
				if c.IsLand {
					continue
				}
				if _, ok := consume(c.Cost, pool); ok {
					options = append(options, c)
				}
			}
			if commander != nil && commanderTurn == nil {
				if _, ok := consume(commander.Cost, pool); ok {
					options = append(options, commander)
				}
			}

			if len(options) == 0 {
				// Was anything blocked purely on colour rather than quantity?
				total := len(expandUnits(pool))
				for _, c := range hand {
					if !c.IsLand && c.MV() <= total {
						colorScrewed++
						break
					}
				}
				break
			}

			// Priority: cheap ramp first (it compounds), then the commander,
			// then the biggest thing we can deploy. `min` keeps the first
			// minimum.
			priority := func(c *sim.Card) [2]int {
				if c.IsRamp() && commander != nil && c.MV() < commander.MV() {
					return [2]int{0, -c.MV()}
				}
				if c == commander {
					return [2]int{1, 0}
				}
				return [2]int{2, -c.MV()}
			}
			chosen, chosenKey := options[0], priority(options[0])
			for _, c := range options[1:] {
				k := priority(c)
				if k[0] < chosenKey[0] || (k[0] == chosenKey[0] && k[1] < chosenKey[1]) {
					chosen, chosenKey = c, k
				}
			}

			remaining, ok := consume(chosen.Cost, pool)
			if !ok {
				panic("tier1: a castable spell became unpayable")
			}
			pool = remaining

			if firstSpellTurn == nil {
				t := turn
				firstSpellTurn = &t
			}
			firstCast.SetDefault(chosen.Name, turn)
			if chosen == commander {
				t := turn
				commanderTurn = &t
				battlefield = append(battlefield, entry{chosen, turn})
			} else {
				hand = removeFirstEqual(hand, chosen)
				switch {
				case len(chosen.Produces) > 0:
					battlefield = append(battlefield, entry{chosen, turn})
					if chosen.ProduceDelay == 0 {
						pool = append(pool, chosen.Produces...)
					}
				case chosen.FetchesLands > 0:
					for i := 0; i < chosen.FetchesLands; i++ {
						var found *sim.Card
						for _, c := range libraryLeft {
							if c.IsLand {
								found = c
								break
							}
						}
						if found == nil {
							break
						}
						libraryLeft = removeFirstEqual(libraryLeft, found)
						battlefield = append(battlefield, entry{found, turn})
					}
				}
			}
			castCount++
		}

		lands := 0
		for _, e := range battlefield {
			if e.card.IsLand {
				lands++
			}
		}
		landsByTurn = append(landsByTurn, lands)
		var online []sim.Source
		for _, e := range battlefield {
			if turn-e.entered >= e.card.ProduceDelay {
				online = append(online, e.card.Produces...)
			}
		}
		manaByTurn = append(manaByTurn, len(expandUnits(online)))
		spellsByTurn = append(spellsByTurn, castCount)
		unusedByTurn = append(unusedByTurn, len(expandUnits(pool)))
		if castCount == 0 {
			for _, c := range hand {
				if !c.IsLand {
					stalledTurns++
					break
				}
			}
		}
	}

	return GameResult{
		CommanderTurn:     commanderTurn,
		LandsByTurn:       landsByTurn,
		ManaByTurn:        manaByTurn,
		SpellsByTurn:      spellsByTurn,
		UnusedByTurn:      unusedByTurn,
		Mulligans:         mulligans,
		ColorScrewedTurns: colorScrewed,
		FirstCast:         firstCast,
		MissedDropByTurn:  missedDropByTurn,
		FirstSpellTurn:    firstSpellTurn,
		StalledTurns:      stalledTurns,
	}
}

func unseededSeed() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("tier1: no entropy for an unseeded run: " + err.Error())
	}
	return int64(binary.LittleEndian.Uint64(b[:]) >> 1)
}
