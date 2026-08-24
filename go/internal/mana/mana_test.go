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

func normalise(c Cost) Cost {
	if len(c.Pips) == 0 {
		c.Pips = nil
	}
	if len(c.Phyrexian) == 0 {
		c.Phyrexian = nil
	}
	return c
}
