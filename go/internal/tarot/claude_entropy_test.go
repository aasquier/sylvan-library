package tarot

import (
	cryptorand "crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/mt19937"
)

// refusingReader is a source of entropy that has none.
type refusingReader struct{}

func (refusingReader) Read([]byte) (int, error) { return 0, errors.New("the well is dry") }

// A deal with no seed mints one, and a machine that cannot hand over eight
// random bytes is told about loudly rather than quietly dealing every visitor
// the same three cards.
//
// The panic is the behaviour under test, not an accident of it: a fixed seed
// here would mean two people who never met getting the same reading and
// believing it, which is the one failure this table cannot have.
func TestAMintedSeedRefusesToBeDealtWithoutEntropy(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a reader with no entropy minted a seed anyway")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "entropy") {
			t.Errorf("the panic does not say what was missing: %v", r)
		}
	}()
	_ = mintSeed(refusingReader{})
}

// And the default, which is the line the served app runs: real entropy mints a
// seed inside the range the client is promised.
//
// Masked to 31 bits because the seed is rendered into a URL, so a negative one
// would be a surprise. Several draws rather than one: a mask that had been
// dropped would pass a single draw about half the time.
func TestAMintedSeedIsAThirtyOneBitNumber(t *testing.T) {
	t.Parallel()
	seen := map[int64]bool{}
	for range 64 {
		got := mintSeed(cryptorand.Reader)
		if got < 0 || got >= 1<<31 {
			t.Fatalf("minted %d, which is outside [0, 2**31)", got)
		}
		seen[got] = true
	}
	// Not a statistical claim, just the one that catches a constant: 64 draws
	// from 2**31 values collide with probability about one in a billion.
	if len(seen) < 2 {
		t.Error("64 minted seeds were all the same number")
	}
}

// The fall-through in the weighted draw, reached on purpose.
//
// A real deal cannot get here more than about once in 10**14 readings: it needs
// `mark` to land in the hair between the compensated total and the running one.
// A deck whose cards all weigh nothing reaches the same place for the same
// reason in the open — the total is zero, so `mark` is zero, and no card's
// running total is ever strictly greater. The card that comes back is the last
// one, which is what that branch promises; what it must not do is hand back
// nothing, or index past the end of the pool.
func TestADrawAgainstNoWeightAtAllStillAnswersACard(t *testing.T) {
	t.Parallel()
	weightless := []Card{
		{Key: "a", Weight: 0},
		{Key: "b", Weight: 0},
		{Key: "c", Weight: 0},
	}
	drawn, totals := weightedSample(mt19937.New(1), weightless, 3)
	if len(drawn) != 3 {
		t.Fatalf("drew %d cards from a weightless deck of three, want 3", len(drawn))
	}
	for i, total := range totals {
		if total != 0 {
			t.Errorf("draw %d totalled %v over a weightless deck, want 0", i, total)
		}
	}
	// Distinct, which is the other half of the contract: the fall-through picks
	// the last of the remaining pool and that card is removed like any other.
	keys := map[string]bool{}
	for _, c := range drawn {
		if keys[c.Key] {
			t.Fatalf("%q was drawn twice from a deck without replacement", c.Key)
		}
		keys[c.Key] = true
	}
	// The real deck is unchanged by any of this: the seeded spread is the
	// promise, and it is the one thing a new parameter could have broken.
	if first := Deal(nil); len(first.Cards) != len(Spread) {
		t.Errorf("an unseeded deal laid out %d cards, want %d", len(first.Cards), len(Spread))
	}
}
