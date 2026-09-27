package mana

// A symbol with nothing inside it.
//
// Costs arrive from a bulk file somebody else writes, and a brace with
// nothing in it is not a symbol this parser has a rule for. The recorded
// answer is the same one every unrecognised symbol gets -- one generic --
// and the part that matters is what it is *not*: it is not read as a number,
// which is the reading that would let `{ }` and `{0}` differ from each other
// while both looking like nothing on the page.

import "testing"

func TestAnEmptySymbolIsNotANumberAndCostsOneGeneric(t *testing.T) {
	t.Parallel()
	if isDigits("") {
		t.Fatal("the empty string was read as a number")
	}
	got := Parse("{2}{ }{G}")
	if got.Generic != 3 {
		t.Fatalf("generic %d, want 3 -- two plus the unrecognised symbol", got.Generic)
	}
	if len(got.Pips) != 1 || got.Pips[0][0] != "G" {
		t.Fatalf("pips %v, want one green", got.Pips)
	}
	if got.ManaValue() != 4 {
		t.Fatalf("mana value %d, want 4", got.ManaValue())
	}
	// It is the unrecognised-symbol rule and not the digit rule: a symbol
	// that really is a number lands in the same place by a different door,
	// and the two agree only because one is written `{1}`.
	if one := Parse("{2}{1}{G}"); one.Generic != got.Generic || one.ManaValue() != got.ManaValue() {
		t.Fatalf("%+v differs from %+v", one, got)
	}
	if zero := Parse("{2}{0}{G}"); zero.Generic != 2 {
		t.Fatalf("an explicit zero cost %d generic", zero.Generic)
	}
}
