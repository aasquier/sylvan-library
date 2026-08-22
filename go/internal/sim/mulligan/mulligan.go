// Package mulligan is `sim/mulligan.py`: searching for a deck's mulligan
// policy instead of assuming one.
//
// `KeepRule`'s docstring has said "tuned per deck -- this is a real lever, not
// a default" since it was written, and then every caller in the project used
// the default anyway. This module is that sentence made executable: run the
// same deck under a grid of 33 keep rules and report which one deploys the
// most.
//
// **The objective is spells deployed through turn 8**, and it is the same
// objective the land sweep uses, for the same reason. Judging a mulligan
// policy by mulligan rate recommends keeping everything; judging it by
// opening-hand quality recommends mulliganing forever. Deployment prices both
// -- a hand thrown away costs a card off the top, and a hand kept on two lands
// costs the turns it spends stuck -- so the policy that deploys most is the
// policy that was right.
//
// # The three decisions a reimplementation would get wrong
//
// **`Flat` is measured against the DEFAULT, never against the grid's range.**
// The land sweep reads flatness off its spread because every count in its
// range is a real candidate. This grid is not like that: it deliberately holds
// rules nobody would play -- "keep only hands with five mana pieces"
// mulligans 95% of the time -- so its spread is always wide, and a `flat` keyed
// on it would never fire. Goreclaw is the case that found it: its grid spans
// 1.62 spells, so by spread the sweep looks decisive, while its best rule
// beats its default by 0.01, which is noise. `Spread` is carried anyway,
// because context is not a verdict.
//
// **Every rule runs on the same seed.** The comparison is between policies,
// not between shuffles, so the shuffles are held still. A grid where each cell
// drew its own sample would rank the rules by luck at these sample sizes.
//
// **`Best` is picked before the rows are sorted, and Python's `max` keeps the
// FIRST extreme.** So a tie on the key goes to the rule earlier in the grid,
// which is the rule with the lower `MinLands`, and re-picking it after the
// sort would silently answer with a different rule. `Gentlest` is the mirror
// image and does read the sorted rows, because Python computes it from
// `self.rows`, which by then is the sorted tuple.
//
// # What is deliberately absent
//
// Nothing. The whole module crosses.
package mulligan

import (
	"errors"

	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// Through is the turn deployment is counted through. Eight, matching the land
// sweep and `stat.spells_through_t8` -- one horizon across the whole
// simulator, because two decks compared at different horizons are not compared
// at all.
const Through = 8

// The grid. Small on purpose: these are the three fields the app's own
// mulligan controls expose, so every rule the search can recommend is a rule
// somebody can then dial in by hand and re-run. A search whose answer had no
// control would be a curiosity.
var (
	MinLands  = []int{1, 2, 3}
	MaxLands  = []int{4, 5, 6}
	MinPieces = []int{2, 3, 4, 5}
)

// Flat is the spread, in spells through T8, below which the grid is reported
// as flat and the recommendation withheld. A quarter of a spell is the same
// bar `_land_summary` draws, and it is drawn from the same place: two runs of
// the same rule at different seeds move by about this much.
const Flat = 0.25

// ErrNoRules is `search`'s `ValueError("no keep rules to search")`.
var ErrNoRules = errors.New("no keep rules to search")

// Row is `mulligan.PolicyRow`: one keep rule, and what it did.
//
// `SpellsThroughT8` is the objective; everything else is here so a person can
// see *why* a rule won rather than being told that it did. A rule that wins by
// mulliganing less will show it in `MulliganRate`, and one that wins by
// finding better hands will show it in `MedianCommanderTurn`.
//
// Field order is Python's, because a ported job result is a struct precisely
// so that `encoding/json` writes the fields in a dict's insertion order rather
// than sorted.
type Row struct {
	MinLands  int `json:"min_lands"`
	MaxLands  int `json:"max_lands"`
	MinPieces int `json:"min_pieces"`
	// SpellsThroughT8 is the objective, rounded to two places -- which is the
	// resolution the verdict is read at, and the resolution `Spread` and
	// `Gain` are then computed over.
	SpellsThroughT8 float64 `json:"spells_through_t8"`
	MulliganRate    float64 `json:"mulligan_rate"`
	AvgMulligans    float64 `json:"avg_mulligans"`
	// MedianCommanderTurn is `statistics.median` of the turns the commander
	// first landed, and it is an INT for an odd-length list -- which is why it
	// is `tier1.Number` and not a float. Carried through unrounded.
	MedianCommanderTurn *tier1.Number `json:"median_commander_turn"`
	ColorScrewRate      float64       `json:"color_screw_rate"`
	StalledTurns        float64       `json:"stalled_turns"`
	Describe            string        `json:"describe"`
}

// Sweep is `mulligan.PolicySweep`.
type Sweep struct {
	Games int `json:"games"`
	Turns int `json:"turns"`
	Seed  int `json:"seed"`
	// Rows is the grid sorted best-first, which is the order the table renders
	// in and the order the CLI prints. The rows are a ranking, not a curve, so
	// unlike the land sweep there is no reading order to preserve.
	Rows []Row `json:"rows"`
	Best Row   `json:"best"`
	// Baseline is the rule the simulator uses when nobody chooses one, so the
	// answer can be read as a change from where the deck stands rather than in
	// a vacuum.
	Baseline Row `json:"baseline"`
	// Spread is best minus worst across the whole grid. Context, not the
	// verdict -- see `Flat`.
	Spread float64 `json:"spread"`
}

// Gain is spells the best rule deploys over the default. The whole verdict.
func (s *Sweep) Gain() float64 {
	return pyfloat.RoundTo(s.Best.SpellsThroughT8-s.Baseline.SpellsThroughT8, 2)
}

// IsFlat is `PolicySweep.flat`: is the winner meaningfully better than the
// rule you already have?
//
// Measured against the **default**, not against the grid's range -- see the
// package comment for why that distinction is the difference between an honest
// answer and a confident one.
func (s *Sweep) IsFlat() bool { return s.Gain() < Flat }

// Gentlest is `PolicySweep.gentlest`: of the rules that deploy as well as the
// best, the one that keeps most.
//
// The answer for a deck whose sweep is flat, and for most decks that is most
// decks -- four of the six here. "Your mulligan rule is already right" is true
// and useless; "these two rules deploy the same and this one throws away half
// as many hands" is the same fact with something to do about it. Arahbo is the
// case: 8.49 spells either way, at a 21% mulligan rate instead of 40%.
//
// Deployment is the objective and this never overrides it -- only rules within
// `Flat` of the winner are eligible, which is exactly the band the objective
// cannot tell apart anyway.
//
// Ties go to the FIRST such row in `Rows`, because Python's `min` keeps the
// first minimum and `Rows` is already the sorted tuple by the time this is
// asked.
func (s *Sweep) Gentlest() Row {
	best := s.Best.SpellsThroughT8
	var out Row
	found := false
	for _, r := range s.Rows {
		if best-r.SpellsThroughT8 >= Flat {
			continue
		}
		if !found || less2(r.MulliganRate, -r.SpellsThroughT8,
			out.MulliganRate, -out.SpellsThroughT8) {
			out, found = r, true
		}
	}
	return out
}

// less2 is a two-key tuple comparison, `<` with no tie-breaking beyond the
// second element -- which is what makes "first wins" the caller's job rather
// than the comparator's.
func less2(a1, a2, b1, b2 float64) bool {
	if a1 != b1 {
		return a1 < b1
	}
	return a2 < b2
}

// Candidates is `mulligan.candidates`: every rule in the grid worth running.
//
// A rule whose minimum exceeds its maximum keeps nothing and would mulligan to
// `MaxMulligans` every game -- not a policy, an error -- so those cells are
// dropped rather than run and reported as terrible. `pieces >= lo` is the same
// argument: a rule demanding fewer mana pieces than lands is asking for
// something it already got.
//
// The order is Python's nested-loop order, and it is load-bearing: `Best`
// keeps the first maximum, so the grid's order decides ties.
func Candidates() []tier1.KeepRule {
	out := []tier1.KeepRule{}
	base := tier1.DefaultKeepRule()
	for _, lo := range MinLands {
		for _, hi := range MaxLands {
			for _, pieces := range MinPieces {
				if lo > hi || pieces < lo {
					continue
				}
				rule := base
				rule.MinLands, rule.MaxLands, rule.MinManaPieces = lo, hi, pieces
				out = append(out, rule)
			}
		}
	}
	return out
}

// Options is `search`'s keyword arguments.
//
// `Games` defaults low against Tier 1's usual twenty thousand, and that is a
// real trade rather than an oversight: this runs thirty-odd simulations where
// the mana screen runs one. Two thousand games puts the sampling error on
// deployment at well under `Flat`, which is the resolution the answer is
// reported at anyway -- a finer sample would buy precision the verdict throws
// away.
type Options struct {
	Games int
	Turns int
	Seed  int
	// Rules overrides the grid. Nil is `rules=None`: use `Candidates`.
	Rules []tier1.KeepRule
	// Progress is called with (index, total) before each rule and once more
	// at the end, exactly as Python's is.
	Progress func(done, total int)
}

// DefaultOptions is `search`'s signature defaults: 2,000 games, 10 turns,
// seed 7.
func DefaultOptions() Options { return Options{Games: 2000, Turns: 10, Seed: 7} }

func row(rule tier1.KeepRule, library []*sim.Card, commander *sim.Card,
	games, turns, seed int) Row {
	s := int64(seed)
	keep := rule
	summary := tier1.Run(library, commander, tier1.Options{
		Games: games, Turns: turns, KeepRule: &keep, Seed: &s,
	})
	return Row{
		MinLands:            rule.MinLands,
		MaxLands:            rule.MaxLands,
		MinPieces:           rule.MinManaPieces,
		SpellsThroughT8:     pyfloat.RoundTo(summary.SpellsThrough(Through), 2),
		MulliganRate:        pyfloat.RoundTo(summary.MulliganRate, 4),
		AvgMulligans:        pyfloat.RoundTo(summary.AvgMulligans, 3),
		MedianCommanderTurn: summary.MedianCommanderTurn,
		ColorScrewRate:      pyfloat.RoundTo(summary.ColorScrewRate, 3),
		StalledTurns:        pyfloat.RoundTo(summary.AvgStalledTurns, 3),
		Describe:            rule.Describe(),
	}
}

// Search is `mulligan.search`: run the grid and report the best rule, or that
// there is no best rule.
func Search(library []*sim.Card, commander *sim.Card, opts Options) (*Sweep, error) {
	grid := opts.Rules
	if grid == nil {
		grid = Candidates()
	}
	if len(grid) == 0 {
		return nil, ErrNoRules
	}

	rows := make([]Row, 0, len(grid))
	for i, rule := range grid {
		if opts.Progress != nil {
			opts.Progress(i, len(grid))
		}
		rows = append(rows, row(rule, library, commander, opts.Games, opts.Turns, opts.Seed))
	}
	if opts.Progress != nil {
		opts.Progress(len(grid), len(grid))
	}

	baselineRule := tier1.DefaultKeepRule()
	baseline, found := Row{}, false
	for _, r := range rows {
		if r.MinLands == baselineRule.MinLands && r.MaxLands == baselineRule.MaxLands &&
			r.MinPieces == baselineRule.MinManaPieces {
			baseline, found = r, true
			break
		}
	}
	if !found {
		baseline = row(baselineRule, library, commander, opts.Games, opts.Turns, opts.Seed)
	}

	lo, hi := rows[0].SpellsThroughT8, rows[0].SpellsThroughT8
	for _, r := range rows[1:] {
		if r.SpellsThroughT8 < lo {
			lo = r.SpellsThroughT8
		}
		if r.SpellsThroughT8 > hi {
			hi = r.SpellsThroughT8
		}
	}

	// Ties broken toward the rule that mulligans less. Two policies that
	// deploy identically are not equally good to play: the one that keeps more
	// hands is the one that spends less of the game having already lost cards.
	//
	// Picked over the GRID order and keeping the first extreme, which is
	// `max`'s own rule.
	//
	// **Moving this below the sort is an equivalent mutation, and it is worth
	// saying so rather than leaving somebody to rediscover it.** The sort key
	// `(-spells, mulliganRate)` is the exact reverse of this comparison, and
	// `sortRows` is stable, so `rows[0]` after sorting is the same row: the
	// maximum, and among full ties the earliest grid cell. The order is kept
	// this way round because it matches Python line for line and because the
	// equivalence rests on the sort being stable -- a fact one careless
	// `sort.Slice` away from being false.
	best := rows[0]
	for _, r := range rows[1:] {
		if less2(best.SpellsThroughT8, -best.MulliganRate,
			r.SpellsThroughT8, -r.MulliganRate) {
			best = r
		}
	}

	sortRows(rows)

	return &Sweep{
		Games:    opts.Games,
		Turns:    opts.Turns,
		Seed:     opts.Seed,
		Rows:     rows,
		Best:     best,
		Baseline: baseline,
		Spread:   pyfloat.RoundTo(hi-lo, 2),
	}, nil
}

// sortRows is `rows.sort(key=lambda r: (-r.spells_through_t8, r.mulligan_rate))`
// -- and it must be a STABLE sort, because Python's is: two rules tying on
// both keys keep their grid order, which is the order `Gentlest` then walks.
func sortRows(rows []Row) {
	// Insertion sort: 33 rows, stable by construction, and no dependency on
	// which sort the standard library happens to choose for a small slice.
	for i := 1; i < len(rows); i++ {
		cur := rows[i]
		j := i - 1
		for j >= 0 && less2(-cur.SpellsThroughT8, cur.MulliganRate,
			-rows[j].SpellsThroughT8, rows[j].MulliganRate) {
			rows[j+1] = rows[j]
			j--
		}
		rows[j+1] = cur
	}
}
