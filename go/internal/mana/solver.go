package mana

import "sort"

// The castability solver: `CanPay`, and Kuhn's augmenting-path matching
// under it.
//
// The question -- "given these untapped sources, can I pay this cost?" -- is a
// bipartite matching problem and not a counting problem, which is the whole
// reason this file is longer than a loop. A W/U dual beside a Forest satisfies
// "I have a white source" and "I have a blue source" and holds two mana, and
// it still cannot cast {W}{U}, because the dual does not tap twice. Counting
// says yes; matching says no; matching is right.
//
// So the risk here is not that Kuhn's is hard to write. It is that a
// subtly wrong matching answers correctly on every case anybody thinks to try.
// `solver_test.go` is where that is dealt with, and it is worth reading before
// this file is edited: the recorded 13,944-case enumeration rebuilt in full,
// two reference implementations resting on different definitions, and a fuzz
// target that plays all three against each other. None of it is optional
// scaffolding -- it is the reason a rewrite of this function can be trusted.

// A colour set, packed. The six producible mana symbols are bits, so the
// question a matching asks ten thousand times -- can this unit pay this pip --
// is one AND.
//
// `other` is the faithfulness half, and it is empty in every call this
// application can make. The contract compares sets of arbitrary strings, and
// nothing stops a caller building a source out of "Z"; that it
// never happens is an argument, and this project has learned twice over that
// an argument about equivalence rots where a check does not. Both paths are
// exercised by tests. On the real path `other` is nil, so the loop below is
// zero iterations and nothing is allocated.
type colorSet struct {
	mask  uint8
	other []string
}

const (
	bitW uint8 = 1 << iota
	bitU
	bitB
	bitR
	bitG
	bitC
)

func toColorSet(colors []string) colorSet {
	var out colorSet
	for _, c := range colors {
		switch c {
		case "W":
			out.mask |= bitW
		case "U":
			out.mask |= bitU
		case "B":
			out.mask |= bitB
		case "R":
			out.mask |= bitR
		case "G":
			out.mask |= bitG
		case Colorless:
			out.mask |= bitC
		default:
			out.other = append(out.other, c)
		}
	}
	return out
}

// intersects asks: do these two colour
// sets share a colour?
func (s colorSet) intersects(other colorSet) bool {
	if s.mask&other.mask != 0 {
		return true
	}
	for _, a := range s.other {
		for _, b := range other.other {
			if a == b {
				return true
			}
		}
	}
	return false
}

// Source is something that produces mana.
//
// Colors is what it can produce -- {"C"} for genuinely colourless, all five
// for an any-colour source. Amount is how many mana one activation makes, so
// Sol Ring is {"C"} at 2; it is the field Scryfall's `produced_mana` does not
// carry and `sim/compile` reads off the oracle text instead.
//
// **Amount has no default and the zero value is 0 mana, not 1.**
// `Source{Colors: colors}`
// is a source producing nothing -- deliberate, because a source that quietly
// produced one would make absent-mindedness castable. [NewSource] is the way
// to say the common case out loud.
//
// This is deliberately field-for-field `sim.Source`, in the same order, so
// the two convert freely (`mana.Source(s)`; Go's conversion rule ignores the
// struct tags `sim` carries for its corpora). They are separate types because
// the packages are layered the other way round -- `mana` is below `sim` and
// must not import it -- and because `sim`'s is what a *compiled deck* carries.
// `sim.Cost` beside [Cost] is
// the same split, argued in that package's own comment. Tier 1's private
// `canPay` answers through [CanPay], and that
// conversion is the whole of the seam.
type Source struct {
	Colors []string
	Amount int
}

// NewSource builds a source with the colour-set invariant
// applied: colours sorted and deduplicated, because a colour set has only
// membership, and two sources are equal exactly when their colour *sets*
// are.
func NewSource(colors []string, amount int) Source {
	seen := make(map[string]bool, len(colors))
	out := make([]string, 0, len(colors))
	for _, c := range colors {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return Source{Colors: out, Amount: amount}
}

// Units is the colour set repeated once per mana the source makes.
//
// The expansion is what makes the matching correct for a source that makes
// more than one mana -- two units of {"C"} for Sol Ring, not one unit that
// somehow counts twice. A non-positive amount produces nothing.
//
// The colour slice is **shared, not copied** -- one set, repeated. Nothing
// guards it beyond this sentence, so: do
// not mutate a returned unit.
func (s Source) Units() [][]string {
	if s.Amount <= 0 {
		return nil
	}
	out := make([][]string, s.Amount)
	for i := range out {
		out[i] = s.Colors
	}
	return out
}

// ExpandUnits flattens sources into individual mana.
func ExpandUnits(sources []Source) [][]string {
	out := make([][]string, 0, len(sources))
	for _, src := range sources {
		out = append(out, src.Units()...)
	}
	return out
}

// unitCount is `len(ExpandUnits(sources))` without the slice.
//
// It exists because the refusal below is the cheap half of CanPay and Tier 1
// calls it in a loop: allocating an intermediate [][]string only to measure it
// would be the whole cost of the fast path. It is held equal to the real thing
// by a test, which is the price of writing a second way to count.
func unitCount(sources []Source) int {
	total := 0
	for _, src := range sources {
		if src.Amount > 0 {
			total += src.Amount
		}
	}
	return total
}

// CanPay asks: can this set of sources pay this cost?
//
// Two conditions, both of which must hold. Every coloured pip is
// simultaneously assigned a distinct unit that produces a colour it accepts --
// which is a maximum matching covering the pips -- and the units left over
// cover the generic portion, which is safe because any unit pays generic.
//
// xValue is counted only when the cost
// carries an X. Note what is deliberately *not* counted: Phyrexian symbols.
// They raise a cost's mana value and they are paid with two life, so they
// demand no mana at all, and using [Cost.ManaValue] here would refuse hands
// that can cast the spell.
func CanPay(cost Cost, sources []Source, xValue int) bool {
	needed := cost.Generic + len(cost.Pips)
	if cost.HasX {
		needed += xValue
	}
	if unitCount(sources) < needed {
		return false
	}
	if len(cost.Pips) == 0 {
		return true
	}
	pips := make([]colorSet, len(cost.Pips))
	for i, pip := range cost.Pips {
		pips[i] = toColorSet(pip)
	}
	units := make([]colorSet, 0, unitCount(sources))
	for _, src := range sources {
		if src.Amount <= 0 {
			continue
		}
		set := toColorSet(src.Colors)
		for range src.Amount {
			units = append(units, set)
		}
	}
	return maxMatching(pips, units) == len(pips)
}

// maxMatching is the size of the maximum bipartite
// matching between pips and mana units, by Kuhn's augmenting-path algorithm.
//
// A pip may take a unit that produces a colour it accepts. Each pip is tried
// in turn; when every unit it can use is already taken, the pips holding them
// are asked -- recursively -- to move, and the pip is matched if any chain of
// moves frees one up. `seen` stops a chain revisiting a unit within one
// search, which is what makes it terminate.
//
// The matching it finds is one of several a case may admit, and which one is
// not meaningful: two correct implementations routinely disagree about the
// assignment while agreeing about its size. Only the size is ever read.
func maxMatching(pips, units []colorSet) int {
	matchUnit := make([]int, len(units))
	for i := range matchUnit {
		matchUnit[i] = -1
	}
	seen := make([]bool, len(units))

	var tryAssign func(pip int) bool
	tryAssign = func(pip int) bool {
		for u := range units {
			if seen[u] || !units[u].intersects(pips[pip]) {
				continue
			}
			seen[u] = true
			if matchUnit[u] == -1 || tryAssign(matchUnit[u]) {
				matchUnit[u] = pip
				return true
			}
		}
		return false
	}

	matched := 0
	for pip := range pips {
		for i := range seen {
			seen[i] = false
		}
		if tryAssign(pip) {
			matched++
		}
	}
	return matched
}
