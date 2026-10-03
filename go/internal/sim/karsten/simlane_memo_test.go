package karsten

import (
	"math/big"
	"testing"
)

// The memo's one promise: it cannot grow forever.
//
// Three tables memoise this package's arithmetic and every one of them is
// bounded by [cacheLimit], which is a hundred thousand entries — a bound nothing
// could check, because reaching it meant asking for a hundred thousand distinct
// tuples inside a test. So the rule is a function of its arguments now
// ([remembering]) and the ceiling is handed in, which is this tree's standing
// move: a package-level value became a parameter.
//
// What is asserted is the whole of what the bound claims — a table at its
// ceiling is emptied rather than grown, and the entry that arrived is the one
// left in it. Nothing here touches the real tables, so it runs beside
// everything else in this package.

func TestAFullMemoIsEmptiedRatherThanGrown(t *testing.T) {
	t.Parallel()

	// Under the ceiling: the table grows and keeps what it had.
	table := map[int]string{1: "one"}
	table = remembering(table, 2, "two", 3)
	if len(table) != 2 || table[1] != "one" || table[2] != "two" {
		t.Fatalf("a table under its ceiling reads %v", table)
	}

	// At the ceiling: emptied, and the new entry is all there is.
	table = remembering(table, 3, "three", 2)
	if len(table) != 1 {
		t.Fatalf("a table at its ceiling holds %d entries, want the one that "+
			"arrived: %v", len(table), table)
	}
	if table[3] != "three" {
		t.Errorf("the entry that arrived is missing: %v", table)
	}

	// The tables this package actually keeps are of two shapes, and the rule is
	// the same for both — a float answer and a big integer.
	odds := remembering(map[hyperKey]float64{{99, 36, 7, 1}: 0.9}, hyperKey{60, 14, 1, 1}, 0.861, 1)
	if len(odds) != 1 || odds[hyperKey{60, 14, 1, 1}] != 0.861 {
		t.Errorf("the odds table reads %v", odds)
	}
	binomials := remembering(map[[2]int]*big.Int{{99, 49}: big.NewInt(1)},
		[2]int{4, 2}, big.NewInt(6), 1)
	if len(binomials) != 1 || binomials[[2]int{4, 2}].Int64() != 6 {
		t.Errorf("the binomial table reads %v", binomials)
	}
}

// And the memo still answers the same numbers it did: a second ask for the same
// tuple comes out of the table, and it is the same bits — which is what the
// recorded corpus rests on.
func TestAMemoisedAnswerIsTheSameBitsAsTheFirst(t *testing.T) {
	t.Parallel()
	first := HypergeometricAtLeast(99, 36, 7, 1)
	if got := HypergeometricAtLeast(99, 36, 7, 1); got != first {
		t.Errorf("the second ask answered %v, the first %v", got, first)
	}
	exact := Exactly(99, 36, 7, 2)
	if got := Exactly(99, 36, 7, 2); got != exact {
		t.Errorf("the second exact ask answered %v, the first %v", got, exact)
	}
	if got := binom(99, 49); got.Cmp(binom(99, 49)) != 0 {
		t.Errorf("a memoised binomial answered %v twice over", got)
	}
}
