package brew

import (
	cryptorand "crypto/rand"
	"errors"
	"strings"
	"testing"
)

// emptyWell is a machine with no randomness left.
type emptyWell struct{}

func (emptyWell) Read([]byte) (int, error) { return 0, errors.New("the well is dry") }

// An unseeded brew mints the number it will be remembered by, and a machine
// that cannot produce eight random bytes says so instead of handing every
// visitor the same pot.
func TestAMintedSeedRefusesToBrewWithoutEntropy(t *testing.T) {
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
	_ = mintSeed(emptyWell{})
}

// The default, which is the line the served app runs: a real seed, inside the
// range the client is promised, and not the same number twice.
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
	if len(seen) < 2 {
		t.Error("64 minted seeds were all the same number")
	}
}
