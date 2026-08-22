package tier1

import (
	"sort"

	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
	"github.com/aasquier/sylvan-library/go/internal/pyrand"
	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// Number is a Python number that may be an int or a float.
//
// It exists for one field. `statistics.median` over a list of ints returns
// `data[n//2]` -- an int -- when the count is odd, and the mean of the middle
// two, a float, when it is even. So `SimSummary.median_commander_turn` is
// sometimes `5` and sometimes `5.5`, and `repr` renders those differently.
// Its annotation says `float | None`, which is the annotation being wrong
// rather than this being pedantic: the digest hashes the repr.
type Number struct {
	IsFloat bool
	Int     int
	Float   float64
}

// Value is the number as a float64, for arithmetic that does not care.
func (n Number) Value() float64 {
	if n.IsFloat {
		return n.Float
	}
	return float64(n.Int)
}

// medianInt is `statistics.median` over ints: it sorts, and it returns an int
// for an odd count.
func medianInt(data []int) Number {
	sorted := append([]int(nil), data...)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return Number{Int: sorted[n/2]}
	}
	return Number{IsFloat: true,
		Float: float64(sorted[n/2-1]+sorted[n/2]) / 2}
}

// CardTiming is `engine.CardTiming`: when one card actually comes online,
// over many games.
//
// The gap between MV and MedianTurn is the fun number: a four-drop with a
// median first cast of seven is telling you something about the mana base (or
// about how much else competes for the same turn) that no curve histogram
// shows.
type CardTiming struct {
	Name string `json:"name"`
	MV   int    `json:"mv"`
	// CastRate is the fraction of games that cast it at all inside the
	// horizon.
	CastRate float64 `json:"cast_rate"`
	// MedianTurn is the median turn of the first cast, over the games that
	// cast it; nil when no game did.
	MedianTurn *float64 `json:"median_turn"`
	// ByT8 is the fraction of games in which it was down by end of turn 8.
	ByT8 float64 `json:"by_t8"`
}

// SimSummary is `engine.SimSummary`.
type SimSummary struct {
	Games               int             `json:"games"`
	Turns               int             `json:"turns"`
	KeepRule            string          `json:"keep_rule"`
	MulliganRate        float64         `json:"mulligan_rate"`
	AvgMulligans        float64         `json:"avg_mulligans"`
	CommanderByTurn     map[int]float64 `json:"commander_by_turn"`
	MedianCommanderTurn *Number         `json:"median_commander_turn"`
	NeverCastCommander  float64         `json:"never_cast_commander"`
	AvgLandsByTurn      []float64       `json:"avg_lands_by_turn"`
	AvgManaByTurn       []float64       `json:"avg_mana_by_turn"`
	AvgUnusedByTurn     []float64       `json:"avg_unused_by_turn"`
	AvgSpellsByTurn     []float64       `json:"avg_spells_by_turn"`
	ColorScrewRate      float64         `json:"color_screw_rate"`
	// CardTimings is per nonland spell, when it comes online. Sorted latest
	// median first, because the interesting rows are the ones running late.
	CardTimings []CardTiming `json:"card_timings"`
	// MissedDropByTurn is P(no land to play on turn t); the cumulative sum is
	// the reader's own, because the per-turn number is the honest unit.
	MissedDropByTurn []float64 `json:"missed_drop_by_turn"`
	// MedianFirstSpellTurn is the median turn of the first nonland spell,
	// over games that cast one.
	MedianFirstSpellTurn *float64 `json:"median_first_spell_turn"`
	// AvgStalledTurns is turns per game that ended castless with a spell in
	// hand.
	AvgStalledTurns float64 `json:"avg_stalled_turns"`
}

// SpellsThrough is total spells deployed by end of `turn` -- the flood-aware
// measure of whether extra lands are actually buying anything.
func (s SimSummary) SpellsThrough(turn int) float64 {
	return sumPrefix(s.AvgSpellsByTurn, turn)
}

// WastedThrough is total mana left unspent by end of `turn`.
func (s SimSummary) WastedThrough(turn int) float64 {
	return sumPrefix(s.AvgUnusedByTurn, turn)
}

// sumPrefix is `fsum(xs[:turn])`, and neither half of that is as plain as it
// looks.
//
// **The slice.** Python's bounds are Python's: `xs[:-1]` is everything but
// the last element, not nothing, and `xs[:100]` on a ten-element list is all
// ten. No caller passes a negative turn -- `mulligan.py` and `simruns.py`
// both ask for 8 -- which is exactly why it is worth being right about while
// it is still two lines.
//
// **The sum is `math.fsum`, and it says `fsum` on the Python side too, as of
// this port.** It said `sum`, which answers differently on the two
// interpreters this project supports: CPython 3.12 gave `sum()` compensated
// (Neumaier) accumulation and 3.11 adds left to right, so `sum([0.1] * 10)`
// is 1.0 under 3.12.13 and 0.9999999999999999 under 3.11.15. That made
// `SimSummary.spells_through` a fact about the interpreter -- and
// `sim/mulligan.py` *ranks* keep rules on it and measures a spread against
// `FLAT`, so a ranking could in principle depend on which Python answered.
// `sim/curve.py` hit the same trap from the other direction on the same day
// and fixed it the same way, fsum rather than pinning either interpreter's
// answer; this follows that call rather than inventing a second one, and
// `pyfloat.Fsum` is already CPython's `math_fsum_impl` in Go.
func sumPrefix(xs []float64, turn int) float64 {
	if turn < 0 {
		turn = len(xs) + turn
	}
	turn = min(max(turn, 0), len(xs))
	return pyfloat.Fsum(xs[:turn])
}

// Options is `engine.run`'s keyword arguments.
//
// Seed nil is `seed=None` -- an unseeded generator, and therefore a run
// nobody can reproduce. Every served caller passes one.
type Options struct {
	Games    int
	Turns    int
	KeepRule *KeepRule
	Seed     *int64
	Progress func(done, total int)
}

// Run is `engine.run`: `Games` independent goldfish games, summarised.
//
// Progress is called roughly 100 times over the run, so a UI can show a bar;
// it consumes no randomness and touches no state, which is what lets a
// watched run and a plain one produce the same answer.
func Run(library []*sim.Card, commander *sim.Card, opts Options) SimSummary {
	if opts.Games <= 0 {
		// Python divides by `games` in a dozen places and raises
		// ZeroDivisionError; silently answering NaN would be worse.
		panic("tier1: Run needs at least one game")
	}
	seed := int64(0)
	if opts.Seed != nil {
		seed = *opts.Seed
	} else {
		seed = unseededSeed()
	}
	rng := pyrand.New(seed)
	keepRule := DefaultKeepRule()
	if opts.KeepRule != nil {
		keepRule = *opts.KeepRule
	}
	// Report ~1% at a time: frequent enough to animate, rare enough that the
	// callback never shows up in a profile.
	reportEvery := max(1, opts.Games/100)

	var commanderTurns []int
	never, mullTotal, mullAny := 0, 0, 0
	landsAcc := make([]float64, opts.Turns)
	manaAcc := make([]float64, opts.Turns)
	unusedAcc := make([]float64, opts.Turns)
	spellsAcc := make([]float64, opts.Turns)
	screwAcc := 0
	castTurns := map[string][]int{}
	missedAcc := make([]int, opts.Turns)
	var firstSpells []int
	stalledTotal := 0

	gameOpts := GameOptions{Turns: opts.Turns, KeepRule: &keepRule, RNG: rng}
	for gameIndex := 0; gameIndex < opts.Games; gameIndex++ {
		if opts.Progress != nil && gameIndex%reportEvery == 0 {
			opts.Progress(gameIndex, opts.Games)
		}
		res := SimulateGame(library, commander, gameOpts)
		if res.CommanderTurn == nil {
			never++
		} else {
			commanderTurns = append(commanderTurns, *res.CommanderTurn)
		}
		mullTotal += res.Mulligans
		if res.Mulligans != 0 {
			mullAny++
		}
		screwAcc += res.ColorScrewedTurns
		stalledTotal += res.StalledTurns
		if res.FirstSpellTurn != nil {
			firstSpells = append(firstSpells, *res.FirstSpellTurn)
		}
		for _, name := range res.FirstCast.Names() {
			castOn, _ := res.FirstCast.Get(name)
			castTurns[name] = append(castTurns[name], castOn)
		}
		for i := 0; i < opts.Turns; i++ {
			landsAcc[i] += float64(res.LandsByTurn[i])
			manaAcc[i] += float64(res.ManaByTurn[i])
			unusedAcc[i] += float64(res.UnusedByTurn[i])
			spellsAcc[i] += float64(res.SpellsByTurn[i])
			if res.MissedDropByTurn[i] {
				missedAcc[i]++
			}
		}
	}
	if opts.Progress != nil {
		opts.Progress(opts.Games, opts.Games)
	}

	games := float64(opts.Games)
	cum := map[int]int{}
	for _, t := range commanderTurns {
		cum[t]++
	}
	byTurn := map[int]float64{}
	running := 0
	for t := 1; t <= opts.Turns; t++ {
		running += cum[t]
		byTurn[t] = float64(running) / games
	}

	// Every distinct nonland spell in the deck, whether or not any game cast
	// it -- a card the simulation *never* reached is the most interesting row
	// of all, and dropping it would hide exactly that. Duplicates collapse by
	// name, which in a singleton format is only the handful of spells a deck
	// runs twice by special dispensation. The order is the dict's own
	// insertion order, which the stable sort below preserves within ties.
	var mvNames []string
	mvByName := map[string]int{}
	setDefaultMV := func(name string, mv int) {
		if _, seen := mvByName[name]; seen {
			return
		}
		mvByName[name] = mv
		mvNames = append(mvNames, name)
	}
	for _, c := range library {
		if !c.IsLand {
			setDefaultMV(c.Name, c.MV())
		}
	}
	if commander != nil {
		setDefaultMV(commander.Name, commander.MV())
	}

	timings := make([]CardTiming, 0, len(mvNames))
	for _, name := range mvNames {
		casts := castTurns[name]
		var medianTurn *float64
		if len(casts) > 0 {
			v := medianInt(casts).Value()
			medianTurn = &v
		}
		byT8 := 0
		for _, t := range casts {
			if t <= 8 {
				byT8++
			}
		}
		timings = append(timings, CardTiming{
			Name:       name,
			MV:         mvByName[name],
			CastRate:   float64(len(casts)) / games,
			MedianTurn: medianTurn,
			ByT8:       float64(byT8) / games,
		})
	}
	// Latest median first; the never-cast rows (median None) lead outright.
	// Python's key is `(median is not None, -(median or 0), name)`, and False
	// sorts before True.
	sort.SliceStable(timings, func(a, b int) bool {
		ta, tb := timings[a], timings[b]
		hasA, hasB := ta.MedianTurn != nil, tb.MedianTurn != nil
		if hasA != hasB {
			return !hasA
		}
		va, vb := 0.0, 0.0
		if hasA {
			va = -*ta.MedianTurn
		}
		if hasB {
			vb = -*tb.MedianTurn
		}
		if va != vb {
			return va < vb
		}
		return ta.Name < tb.Name
	})

	var medianCommander *Number
	if len(commanderTurns) > 0 {
		m := medianInt(commanderTurns)
		medianCommander = &m
	}
	var medianFirstSpell *float64
	if len(firstSpells) > 0 {
		v := medianInt(firstSpells).Value()
		medianFirstSpell = &v
	}

	return SimSummary{
		Games:                opts.Games,
		Turns:                opts.Turns,
		KeepRule:             keepRule.Describe(),
		MulliganRate:         float64(mullAny) / games,
		AvgMulligans:         float64(mullTotal) / games,
		CommanderByTurn:      byTurn,
		MedianCommanderTurn:  medianCommander,
		NeverCastCommander:   float64(never) / games,
		AvgLandsByTurn:       divideAll(landsAcc, games),
		AvgManaByTurn:        divideAll(manaAcc, games),
		AvgUnusedByTurn:      divideAll(unusedAcc, games),
		AvgSpellsByTurn:      divideAll(spellsAcc, games),
		ColorScrewRate:       float64(screwAcc) / games,
		CardTimings:          timings,
		MissedDropByTurn:     divideInts(missedAcc, games),
		MedianFirstSpellTurn: medianFirstSpell,
		AvgStalledTurns:      float64(stalledTotal) / games,
	}
}

func divideAll(xs []float64, by float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = x / by
	}
	return out
}

func divideInts(xs []int, by float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = float64(x) / by
	}
	return out
}

// LandCount is one point of a land sweep.
type LandCount struct {
	Lands   int
	Summary SimSummary
}

// SweepLandCounts is `engine.sweep_land_counts`: find the knee of the curve
// by running the same deck at several land counts.
//
// `build(n)` returns (library, commander) for a deck with n lands. Every
// point starts from the same seed, which is what makes the sweep a comparison
// rather than a collection of unrelated samples.
func SweepLandCounts(
	build func(int) ([]*sim.Card, *sim.Card),
	counts []int,
	opts Options,
) []LandCount {
	out := make([]LandCount, 0, len(counts))
	for _, n := range counts {
		library, commander := build(n)
		out = append(out, LandCount{Lands: n, Summary: Run(library, commander, opts)})
	}
	return out
}
