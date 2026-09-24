package mulligan_test

// The standing defaults.
//
// They are a number of games, a horizon and a seed, and all three are part of
// what a quoted figure means: the grid the deck page shows was searched under
// these, and a session that changed one and forgot would be comparing two
// runs that never asked the same question. A seed in particular is a promise
// (ADR 18), so the default one is pinned here rather than left to whoever
// reads the struct next.

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/mulligan"
)

func TestTheStandingDefaultsAreTwoThousandGamesToTenTurnsOnSeedSeven(t *testing.T) {
	t.Parallel()
	got := mulligan.DefaultOptions()
	if got.Games != 2000 {
		t.Fatalf("games %d, want 2000", got.Games)
	}
	if got.Turns != 10 {
		t.Fatalf("turns %d, want 10", got.Turns)
	}
	if got.Seed != 7 {
		t.Fatalf("seed %d, want 7", got.Seed)
	}
	// Workers is deliberately left at zero: the search fills it in from the
	// machine, and a number written down here would make the grid's answer
	// depend on where it ran.
	if got.Workers != 0 {
		t.Fatalf("workers %d, want the search to decide", got.Workers)
	}
}
