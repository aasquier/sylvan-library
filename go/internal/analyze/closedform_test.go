package analyze

// The opening hand's arithmetic at its edges, and the one report that will
// take a nil pool.
//
// The closed forms below are asked about decks that are being *built*, which
// is the state where the numbers are nonsense: a deck with nothing in it yet,
// a category nobody has filled, a card nobody is playing any copies of. Every
// one of those has to answer zero rather than divide by it -- a newcomer
// mid-build must never be shown NaN, and commandment 2 is the whole reason
// the guards are there.

import (
	"math"
	"math/big"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

func TestTheOpeningOddsAnswerZeroRatherThanDivideByIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		deckSize, copies, seen int
		want                   float64
	}{
		{"no copies of the card", 99, 0, 7, 0},
		{"a deck with nothing in it", 0, 4, 7, 0},
		{"a negative number of copies", 99, -1, 7, 0},
		{"fewer cards seen than none", 99, 1, -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := atLeastOne(tc.deckSize, tc.copies, tc.seen); got != tc.want {
				t.Fatalf("atLeastOne(%d, %d, %d) = %v, want %v",
					tc.deckSize, tc.copies, tc.seen, got, tc.want)
			}
		})
	}
	// The sweep above would pass against a function that answered zero to
	// everything, so one real question is asked too: a single copy in a
	// hundred-card deck, seen seven cards deep, is seven percent.
	if got := atLeastOne(100, 1, 7); math.Abs(got-0.07) > 1e-12 {
		t.Fatalf("one copy in a hundred, seven seen = %v", got)
	}
	// Asking to see more cards than the deck holds is asking to see the whole
	// deck, so the card is certainly there.
	if got := atLeastOne(60, 1, 500); got != 1 {
		t.Fatalf("the whole deck seen = %v", got)
	}
	// And the division itself: nothing over nothing is zero, not NaN.
	if got := ratio(big.NewInt(1), big.NewInt(0)); got != 0 {
		t.Fatalf("a ratio over zero = %v", got)
	}
}

func TestSomethingThatIsNotOneOfTheFiveIsNotAColour(t *testing.T) {
	t.Parallel()
	// The pip census reads a cost's colour set, and a cost can carry a
	// symbol that is not a colour at all -- colourless mana, or a symbol the
	// parser did not recognise. Neither belongs in a colour requirement.
	for _, c := range []string{"W", "U", "B", "R", "G"} {
		if !isColor(c) {
			t.Fatalf("%q is one of the five", c)
		}
	}
	for _, c := range []string{"C", "", "S", "wubrg", "w"} {
		if isColor(c) {
			t.Fatalf("%q was read as a colour", c)
		}
	}
}

func TestTheDeckReportStandsUpWithNoPoolBehindIt(t *testing.T) {
	t.Parallel()
	// A deck page asked for before the pool has ever been refreshed: the
	// report is thinner, but it is a report. Refusing here would take the
	// diagnosis away exactly when somebody is trying to work out why their
	// cards will not resolve.
	d, err := deck.FromText("slug: empty-shelf\nname: An Empty Shelf\ncommander: []\ncards: []\n", "empty-shelf")
	if err != nil {
		t.Fatal(err)
	}
	var absent map[string]*pool.CardRecord
	stats := DeckStats(d, absent)
	if stats.Slug != "empty-shelf" || stats.Name != "An Empty Shelf" {
		t.Fatalf("the deck's own facts were lost: %+v", stats)
	}
	if stats.TotalCards != 0 || stats.LandCount != 0 {
		t.Fatalf("an empty deck counted %d cards and %d lands",
			stats.TotalCards, stats.LandCount)
	}
}
