package mana

import (
	"reflect"
	"testing"
)

func TestParseReadsCostsAsRecorded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cost string
		want Cost
		mv   int
	}{
		{"", Cost{}, 0},
		{"{2}{G}{W}", Cost{Generic: 2, Pips: [][]string{{"G"}, {"W"}}}, 4},
		{"{X}{U/R}", Cost{HasX: true, Pips: [][]string{{"R", "U"}}}, 1},
		{"{G/P}", Cost{Phyrexian: [][]string{{"G"}}}, 1},
		{"{8}{C/P}{C/P}", Cost{Generic: 8, Phyrexian: [][]string{{"C"}, {"C"}}}, 10},
		{"{G/U/P}", Cost{Phyrexian: [][]string{{"G", "U"}}}, 1},
		{"{2/G}", Cost{Generic: 2}, 2},
		{"{C}{C}", Cost{Pips: [][]string{{"C"}, {"C"}}}, 2},
		{"{S}", Cost{Generic: 1}, 1},
		{"{1000000}", Cost{Generic: 1000000}, 1000000},
		{"{w}{ u }", Cost{Pips: [][]string{{"W"}, {"U"}}}, 2},
	}
	for _, c := range cases {
		got := Parse(c.cost)
		if got.Pips == nil {
			got.Pips = nil
		}
		if !reflect.DeepEqual(normalise(got), normalise(c.want)) {
			t.Errorf("Parse(%q) = %+v, want %+v", c.cost, got, c.want)
		}
		if got.ManaValue() != c.mv {
			t.Errorf("Parse(%q).ManaValue() = %d, want %d", c.cost, got.ManaValue(), c.mv)
		}
	}
	if cols := Parse("{1}{G}{U/P}{C}").ColorsOf(); !reflect.DeepEqual(cols, []string{"G", "U"}) {
		t.Fatalf("colors %v", cols)
	}
}

// The generic-cost reader's two bounds, swept rather than sampled.
//
// `isDigits` tests `r < '0' || r > '9'`, and the upper bound had nothing
// holding it: every recorded cost in this file's table and in the oracles
// stops at 8, so `r >= '9'` reads `{9}` as *not digits*, the symbol falls to
// the `default` arm, and a nine-generic cost quietly becomes one generic — a
// mana value four off, with no error anywhere. Mutation testing is how that
// came out (`gremlins unleash ./internal/mana/`), and it is the same fault the
// compile kernel had at its own digit test, found the same way: a reader's
// bounds are only pinned by a symbol that *sits on* them, and nothing a deck
// happens to contain is a bound.
//
// So the sweep is all ten digits and then past them, because the second digit
// is read by the same loop and a cost can carry two.
func TestParseReadsEveryASCIIDigitAsGeneric(t *testing.T) {
	t.Parallel()
	for d := 0; d <= 9; d++ {
		cost := "{" + string(rune('0'+d)) + "}{G}"
		got := Parse(cost)
		if got.Generic != d {
			t.Errorf("Parse(%q).Generic = %d, want %d", cost, got.Generic, d)
		}
		if len(got.Pips) != 1 {
			t.Errorf("Parse(%q).Pips = %v, want the one green pip", cost, got.Pips)
		}
		if want := d + 1; got.ManaValue() != want {
			t.Errorf("Parse(%q).ManaValue() = %d, want %d", cost, got.ManaValue(), want)
		}
	}
	for _, c := range []struct {
		cost    string
		generic int
	}{
		{"{10}", 10},
		{"{19}", 19},
		{"{90}", 90},
		{"{99}", 99},
		{"{9}{9}", 18},
	} {
		if got := Parse(c.cost); got.Generic != c.generic {
			t.Errorf("Parse(%q).Generic = %d, want %d", c.cost, got.Generic, c.generic)
		}
	}
}

func normalise(c Cost) Cost {
	if len(c.Pips) == 0 {
		c.Pips = nil
	}
	if len(c.Phyrexian) == 0 {
		c.Phyrexian = nil
	}
	return c
}
