package mana

import "math/bits"

// Two answers to "can these sources pay this cost?" that are obviously
// correct and far too slow to ship -- `tests/mana_oracle.py`'s
// `brute_force_can_pay` and `hall_can_pay`, in Go.
//
// PLAN section 5 item 2 asks for them by name: Hypothesis's generative role on
// the Python side is covered here by native fuzzing "against the same
// brute-force/Hall's-theorem oracles, re-implemented once in Go". The word
// doing the work is **re-implemented**. Transliterating the Python would
// inherit any mistake the Python makes, and the whole point of a second and
// third opinion is that they can be wrong in different ways than the first.
// So these were written from the definitions:
//
//   - the brute force enumerates injective assignments of pips to units, by
//     recursive backtracking rather than by Python's `itertools.permutations`;
//   - Hall's condition walks subsets as bitmasks rather than by Python's
//     size-stratified `itertools.combinations`.
//
// Both come out at the same answers by different routes, which is what makes
// agreement mean something.
//
// **Nothing here may call into the solver.** `oracleUnits` re-does what
// [ExpandUnits] does and `oracleOverlap` re-does what colorSet.intersects
// does, deliberately -- `mana_oracle.py`'s own docstring gives the reason for
// its `_units`, and it applies verbatim: an oracle that shares code with the
// implementation cannot catch a bug in the shared part. In particular these
// compare colour **strings**, where the solver compares bits, so the packing
// is checked rather than assumed.

// oracleUnits flattens sources into one entry per mana produced.
func oracleUnits(sources []Source) [][]string {
	var out [][]string
	for _, source := range sources {
		for range source.Amount { // a non-positive amount produces nothing
			out = append(out, source.Colors)
		}
	}
	return out
}

// oracleRequired is the total mana a cost demands. Nothing taps twice, so
// every point of it needs its own unit and this is a hard lower bound on the
// pool -- Phyrexian symbols excepted, which are paid with life and are
// therefore not here.
func oracleRequired(cost Cost, xValue int) int {
	needed := cost.Generic + len(cost.Pips)
	if cost.HasX {
		needed += xValue
	}
	return needed
}

// oracleOverlap: can this unit pay into this pip? Plain string comparison,
// which is the definition.
func oracleOverlap(unit, pip []string) bool {
	for _, u := range unit {
		for _, p := range pip {
			if u == p {
				return true
			}
		}
	}
	return false
}

// bruteForceCanPay searches every injective assignment of pips to distinct
// mana units.
//
// Factorial in the pip count, and that is the point: there is nothing here to
// get wrong beyond the definition itself. The recursion places pip 0, then
// pip 1, and so on, never reusing a unit; it succeeds exactly when some
// complete placement exists. The only thing it declines to try is a unit that
// cannot pay the pip in hand, which is not an optimisation -- it is the
// predicate.
func bruteForceCanPay(cost Cost, sources []Source, xValue int) bool {
	units := oracleUnits(sources)
	if len(units) < oracleRequired(cost, xValue) {
		return false
	}
	used := make([]bool, len(units))
	var place func(pip int) bool
	place = func(pip int) bool {
		if pip == len(cost.Pips) {
			return true
		}
		for u := range units {
			if used[u] || !oracleOverlap(units[u], cost.Pips[pip]) {
				continue
			}
			used[u] = true
			if place(pip + 1) {
				return true
			}
			used[u] = false
		}
		return false
	}
	return place(0)
}

// hallCanPay applies Hall's marriage theorem: a matching covering every pip
// exists if and only if every set S of pips has at least |S| units able to pay
// into it.
//
// Exponential in the pip count, and derived from a theorem rather than from a
// search, so it fails differently than the brute force above. Subsets are
// bitmasks over the pips; a mask's population count is |S|, and a unit counts
// toward S if it can pay any pip in it.
func hallCanPay(cost Cost, sources []Source, xValue int) bool {
	units := oracleUnits(sources)
	if len(units) < oracleRequired(cost, xValue) {
		return false
	}
	n := len(cost.Pips)
	for subset := 1; subset < 1<<n; subset++ {
		size := bits.OnesCount(uint(subset))
		usable := 0
		for _, unit := range units {
			for i := range n {
				if subset&(1<<i) != 0 && oracleOverlap(unit, cost.Pips[i]) {
					usable++
					break
				}
			}
		}
		if usable < size {
			return false
		}
	}
	return true
}
