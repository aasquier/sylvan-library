package mana

import (
	"reflect"
	"testing"
)

// `tests/test_mana.py`'s castability cases, as a table.
//
// These are not samples. Every one of them is somewhere somebody was once
// wrong -- CLAUDE.md says `mana.py` is subtle and that this is the file
// pinning "the cases where naive source-counting gives the wrong answer" --
// and they are carried over one for one because a port that passes the
// enumerated case set and fails one of these has reproduced the arithmetic
// and lost the reasoning. The enumeration next door is deliberately built out
// of small abstract alphabets; this is the same solver asked about Sol Ring.

var (
	w   = []string{"W"}
	u   = []string{"U"}
	g   = []string{"G"}
	c   = []string{"C"}
	wu  = []string{"W", "U"}
	gw  = []string{"G", "W"}
	any = []string{"W", "U", "B", "R", "G"}
)

func TestCastabilityTraps(t *testing.T) {
	cases := []struct {
		why     string
		cost    string
		sources []Source
		x       int
		want    bool
	}{
		// A W/U dual plus a Forest. "I have a white source" is true, "I have
		// a blue source" is true, and there are two mana -- but the dual does
		// not tap twice, so {W}{U} is not castable. This is the case that
		// makes the whole file a matching problem instead of a count.
		{"a lone dual cannot pay two different pips", "{W}{U}",
			[]Source{NewSource(wu, 1), NewSource(g, 1)}, 0, false},
		{"the same hand with a real Island can", "{W}{U}",
			[]Source{NewSource(wu, 1), NewSource(u, 1)}, 0, true},

		// A wide pip standing BEFORE the narrow pip whose only payer it would
		// otherwise take. {W/U} may be paid by either land, {W} only by the
		// Plains, so the hybrid has to be made to move to the Island -- and a
		// matching that does not reset its search between pips never looks at
		// the Plains again and calls this uncastable.
		//
		// This one is not from `test_mana.py`. It is the counterexample the
		// **enumerated case set cannot express**: its pips are drawn with
		// `combinations_with_replacement` over an alphabet whose hybrid is
		// last, so no cost among the 13,944 puts a two-colour pip ahead of a
		// one-colour pip. Deleting the reset in `maxMatching` passes every
		// other test in this package. See `fuzz_test.go`, which found it.
		{"a hybrid pip before a mono pip still yields its land", "{W/U}{W}",
			[]Source{NewSource(w, 1), NewSource(u, 1)}, 0, true},
		{"and the same hand cannot pay it twice over", "{W/U}{W}{U}",
			[]Source{NewSource(w, 1), NewSource(u, 1)}, 0, false},

		// The other direction: duals are not second-class. Two G/W lands do
		// pay {G}{G}, because each taps for the green half.
		{"two duals cover a double pip", "{G}{G}",
			[]Source{NewSource(gw, 1), NewSource(gw, 1)}, 0, true},
		{"one dual and a Plains does not", "{G}{G}",
			[]Source{NewSource(gw, 1), NewSource(w, 1)}, 0, false},

		// Generic is paid by whatever the pips did not take, which is safe
		// because any source pays generic -- and the pool still has to be big
		// enough overall.
		{"leftovers pay the generic", "{2}{G}",
			[]Source{NewSource(g, 1), NewSource(w, 1), NewSource(u, 1)}, 0, true},
		{"two mana never casts a three-drop", "{2}{G}",
			[]Source{NewSource(g, 1), NewSource(w, 1)}, 0, false},

		// Sol Ring is two colourless mana, not one mana worth two. It pays
		// {2} and it never pays a coloured pip.
		{"a two-mana source pays two generic", "{2}",
			[]Source{{Colors: c, Amount: 2}}, 0, true},
		{"and still pays no coloured pip", "{G}",
			[]Source{{Colors: c, Amount: 2}}, 0, false},

		// {C} demands genuinely colourless mana, which "any colour" is not.
		// A Command Tower does not cast Kozilek.
		{"any colour is not colourless", "{C}",
			[]Source{NewSource(any, 1)}, 0, false},
		{"colourless is", "{C}",
			[]Source{NewSource(c, 1)}, 0, true},

		// Five pips need five sources however wide each one is.
		{"five any-colour sources cast a five-colour cost", "{W}{U}{B}{R}{G}",
			[]Source{NewSource(any, 1), NewSource(any, 1), NewSource(any, 1), NewSource(any, 1), NewSource(any, 1)},
			0, true},
		{"four do not", "{W}{U}{B}{R}{G}",
			[]Source{NewSource(any, 1), NewSource(any, 1), NewSource(any, 1), NewSource(any, 1)},
			0, false},

		// X eats mana beyond the printed cost, so the same hand answers
		// differently at a different X.
		{"X consumes the extra mana", "{X}{G}",
			[]Source{NewSource(g, 1), NewSource(g, 1), NewSource(g, 1)}, 2, true},
		{"and is refused when it cannot be paid", "{X}{G}",
			[]Source{NewSource(g, 1), NewSource(g, 1)}, 2, false},
		{"X on an empty board is free at X=0", "{X}", nil, 0, true},

		// Phyrexian is paid with two life, so it constrains the mana base not
		// at all -- an empty board casts {G/P}, and the mana value it carries
		// must not leak into the requirement. This is the regression that put
		// Mental Misstep in the wrong bucket of the curve.
		{"Phyrexian needs no mana at all", "{G/P}", nil, 0, true},
		{"nor does a two-colour Phyrexian", "{G/U/P}", nil, 0, true},
		{"but its generic still does", "{1}{G/P}", nil, 0, false},
		{"Kozilek, Compleated needs eight and not ten", "{8}{C/P}{C/P}",
			[]Source{{Colors: c, Amount: 8}}, 0, true},

		// Monocolour hybrid is costed at its dear branch, so {2/G} is two
		// generic and never claims a hand castable when it is not.
		{"a monocolour hybrid costs its generic branch", "{2/G}",
			[]Source{NewSource(g, 1)}, 0, false},
		{"and is paid by any two mana", "{2/G}",
			[]Source{NewSource(g, 1), NewSource(w, 1)}, 0, true},

		// An unknown symbol is one generic, same reason.
		{"snow is read as generic", "{S}", []Source{NewSource(w, 1)}, 0, true},

		// Nothing costs nothing.
		{"an empty cost is always payable", "", nil, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.why, func(t *testing.T) {
			cost := Parse(tc.cost)
			if got := CanPay(cost, tc.sources, tc.x); got != tc.want {
				t.Errorf("CanPay(%s, %s, x=%d) = %v, want %v",
					cost, poolString(tc.sources), tc.x, got, tc.want)
			}
			// Every trap is also a case for the oracles, which is cheap here
			// and means a hand-written expectation that is simply wrong shows
			// up as three disagreeing implementations rather than as one
			// passing test.
			if got := bruteForceCanPay(cost, tc.sources, tc.x); got != tc.want {
				t.Errorf("brute force says %v, want %v", got, tc.want)
			}
			if got := hallCanPay(cost, tc.sources, tc.x); got != tc.want {
				t.Errorf("Hall's condition says %v, want %v", got, tc.want)
			}
		})
	}
}

// A source's amount is Python's, including the parts of it that look like
// mistakes. `ManaSource(colors)` defaults to one mana and Go's zero value
// cannot; `ManaSource(colors, 0)` really does produce nothing, and
// `[colors] * -1` is the empty list rather than an error.
func TestASourcesAmountIsExactlyWhatItSays(t *testing.T) {
	if got := NewSource([]string{"W"}, 1).Units(); len(got) != 1 {
		t.Errorf("NewSource(W, 1) made %d units, want 1", len(got))
	}
	if got := (Source{Colors: w}).Units(); got != nil {
		t.Errorf("a zero amount made %v, want no units", got)
	}
	if got := (Source{Colors: w, Amount: -3}).Units(); got != nil {
		t.Errorf("a negative amount made %v, want no units", got)
	}
	if got := ExpandUnits([]Source{{Colors: c, Amount: 2}, NewSource([]string{"G"}, 1)}); len(got) != 3 {
		t.Errorf("expanded to %d units, want 3", len(got))
	}
	if CanPay(Parse("{W}"), []Source{{Colors: w, Amount: 0}}, 0) {
		t.Error("a source making no mana paid a pip")
	}
	// The oracles have to agree about this, and the fuzzer cannot ask them:
	// its decoder only ever generates amounts 0 to 2. A judge that counted a
	// negative amount as mana would quietly bless a wrong solver, which is
	// worse than a wrong solver.
	empty := []Source{{Colors: w, Amount: -3}, {Colors: w, Amount: 0}}
	for _, cost := range []string{"{W}", "{1}", "{X}"} {
		if bruteForceCanPay(Parse(cost), empty, 0) != CanPay(Parse(cost), empty, 0) {
			t.Errorf("%s: the brute force reads a non-positive amount differently", cost)
		}
		if hallCanPay(Parse(cost), empty, 0) != CanPay(Parse(cost), empty, 0) {
			t.Errorf("%s: Hall's condition reads a non-positive amount differently", cost)
		}
	}
}

// One source of N is N sources of one. Sol Ring is two colourless mana, not
// one mana that is worth two, and the expansion in [Source.Units] is the only
// thing that makes the matching see it that way.
func TestOneSourceOfNEqualsNSourcesOfOne(t *testing.T) {
	for _, cost := range []string{"{2}", "{C}{C}", "{G}{G}", "{1}{C}", "{W}{U}"} {
		for amount := 1; amount <= 4; amount++ {
			var spread []Source
			for range amount {
				spread = append(spread, NewSource(c, 1))
			}
			one := CanPay(Parse(cost), []Source{{Colors: c, Amount: amount}}, 0)
			many := CanPay(Parse(cost), spread, 0)
			if one != many {
				t.Errorf("%s: one source of %d says %v, %d sources of one say %v",
					cost, amount, one, amount, many)
			}
		}
	}
}

// The colour packing has a slow path for strings that are not one of the six
// producible mana symbols, because Python compares sets of arbitrary strings
// and nothing structural stops a caller doing the same here. It is unreachable
// through `Parse` and through `compile.py`, which filters `produced_mana` to
// WUBRGC -- and this project's standing lesson is that unreachable-by-argument
// is exactly the claim that rots, so the path is exercised rather than argued.
func TestAColourOutsideTheSixStillComparesAsASet(t *testing.T) {
	odd := Cost{Pips: [][]string{{"Z"}}}
	if !CanPay(odd, []Source{NewSource([]string{"Z"}, 1)}, 0) {
		t.Error("a Z source did not pay a Z pip")
	}
	if CanPay(odd, []Source{NewSource(any, 1)}, 0) {
		t.Error("a WUBRG source paid a Z pip")
	}
	// And it does not leak into the mask: two different unknowns are two
	// different colours, not one bucket that matches itself.
	if CanPay(odd, []Source{NewSource([]string{"Y"}, 1)}, 0) {
		t.Error("a Y source paid a Z pip")
	}
	mixed := Cost{Pips: [][]string{{"Z"}, {"W"}}}
	if !CanPay(mixed, []Source{NewSource([]string{"Z"}, 1), NewSource(w, 1)}, 0) {
		t.Error("a mixed known/unknown cost was refused a hand that pays it")
	}
	if CanPay(mixed, []Source{NewSource([]string{"Z", "W"}, 1)}, 0) {
		t.Error("one source paid two pips")
	}
}

// String() is the case set's renderer, so it is checked against Python's
// wording for the shapes the enumeration cannot reach: X, Phyrexian, and the
// empty cost.
func TestCostRendersAsPythonDoes(t *testing.T) {
	cases := map[string]string{
		"":                 "{0}",
		"{0}":              "{0}",
		"{X}":              "{X}",
		"{X}{2}{G}":        "{X}{2}{G}",
		"{2}{G}{W}":        "{2}{G}{W}",
		"{U/R}":            "{R/U}",
		"{G/P}":            "{G/P}",
		"{2}{G}{G/U/P}{U}": "{2}{G}{U}{G/U/P}",
		"{8}{C/P}{C/P}":    "{8}{C/P}{C/P}",
		"{2/G}":            "{2}",
		"{S}":              "{1}",
		"{X}{C}{R}":        "{X}{C}{R}",
	}
	for in, want := range cases {
		if got := Parse(in).String(); got != want {
			t.Errorf("Parse(%q).String() = %q, want %q", in, got, want)
		}
	}
	// An unsorted hand-built cost renders sorted, and is not itself reordered
	// by having been rendered.
	pip := []string{"W", "G"}
	built := Cost{Pips: [][]string{pip}}
	if got := built.String(); got != "{G/W}" {
		t.Errorf("built.String() = %q, want {G/W}", got)
	}
	if !reflect.DeepEqual(pip, []string{"W", "G"}) {
		t.Errorf("String() reordered its receiver's pip: %v", pip)
	}
}
