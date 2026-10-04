package compile

import "testing"

// The digit test's two bounds, pinned directly. The corpus never holds a
// mana symbol with a 0 or a 9 in it -- no card the fixtures carry adds ten
// mana or taps for a cost of nine -- so a mutant that moved the lower bound
// (`r <= '0'`) lived through the whole corpus: `{0}` reads as zero either way,
// and nothing larger with a zero in it ever reached the reader. Each case
// here is the arithmetic of the symbol itself, not a recording of what the
// reader happened to answer.
func TestManaSymbolsReadsEveryASCIIDigit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		want int
	}{
		{"{10}", 10},
		{"{0}", 0},
		{"{9}", 9},
		{"{90}", 90},
		{"{2}{W}", 3},
		{"{W/U}", 1},
		{"{X}", 0},
		{"{C}{C}", 2},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			t.Parallel()
			if got := manaSymbols(c.text); got != c.want {
				t.Fatalf("manaSymbols(%q) = %d, want %d", c.text, got, c.want)
			}
		})
	}
}
