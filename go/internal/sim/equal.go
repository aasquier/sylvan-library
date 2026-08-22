package sim

// Card equality, which is a rule of the simulation rather than a convenience.
//
// Tier 1 takes cards out of a hand with `list.remove`, and CPython's
// `list.remove` removes the first element **equal** to its argument, not the
// one it was handed. A compiled deck repeats one object for a basic's `qty`
// (`compile_deck` writes `[compiled] * qty`), so a hand can hold several cards
// that compare equal -- and which of them leaves changes the order of the
// rest, which decides the next land played and the next spell cast.
//
// So "equal" has to be defined here, once, rather than left to whatever a
// caller reaches for. `==` on a Card does not compile (it holds slices), and
// `reflect.DeepEqual` would be both slower and wrong in one respect that
// matters: it distinguishes a nil slice from an empty one, where Python's
// `()` and `()` are the same empty tuple.
//
// The colour sets are compared as *sets* because Python holds them in
// frozensets. Every producer of these values -- `sim/compile.py` through the
// generated corpora, and the parser through `mana.Cost` -- writes them
// sorted, so a sorted comparison is that. A caller that builds one by hand
// keeps the invariant or gets an answer about a card nobody has.

// Equal is `SimCard.__eq__`, field for field.
func (c Card) Equal(o Card) bool {
	if c.Name != o.Name || c.Category != o.Category ||
		c.IsLand != o.IsLand || c.EntersTapped != o.EntersTapped ||
		c.ProduceDelay != o.ProduceDelay || c.FetchesLands != o.FetchesLands {
		return false
	}
	if !c.Cost.Equal(o.Cost) {
		return false
	}
	if len(c.Produces) != len(o.Produces) {
		return false
	}
	for i := range c.Produces {
		if !c.Produces[i].Equal(o.Produces[i]) {
			return false
		}
	}
	return true
}

// Equal is `ManaCost.__eq__`: the pip *tuples* in order, each pip's colours as
// a set. `{G}{W}` is not `{W}{G}`; `{G/W}` is `{W/G}`.
func (c Cost) Equal(o Cost) bool {
	if c.Generic != o.Generic || c.HasX != o.HasX {
		return false
	}
	return samePips(c.Pips, o.Pips) && samePips(c.Phyrexian, o.Phyrexian)
}

// Equal is `ManaSource.__eq__`: the same colours, and the same amount.
func (s Source) Equal(o Source) bool {
	return s.Amount == o.Amount && SameColors(s.Colors, o.Colors)
}

// SameColors reports whether two colour sets are the frozensets Python would
// call equal. Both are expected sorted and deduplicated, which is how every
// producer in this project writes them.
func SameColors(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Intersects reports whether a mana unit can pay a coloured pip: `unit & pip`
// non-empty, in Python. It is the innermost test of the castability matching,
// which is why it takes two plain slices rather than a Source.
func Intersects(unit, pip []string) bool {
	for _, u := range unit {
		for _, p := range pip {
			if u == p {
				return true
			}
		}
	}
	return false
}

func samePips(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !SameColors(a[i], b[i]) {
			return false
		}
	}
	return true
}
