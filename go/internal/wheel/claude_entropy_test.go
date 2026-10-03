package wheel

import (
	crand "crypto/rand"
	"errors"
	"math/big"
	"strings"
	"testing"
)

// dryWell is a machine with no randomness left to spin with.
type dryWell struct{}

func (dryWell) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// An unseeded spin mints the number it will be remembered by, and a machine
// that cannot produce one says so rather than spinning a fixed seed.
//
// The fixed seed is the whole hazard: the wheel's promise is that a spin can be
// spun again from the number in the URL, and a mint that quietly fell back to a
// constant would deal every visitor the same fate and the same card while still
// handing each of them a seed that looked like theirs.
func TestASpinRefusesToBeMintedWithoutEntropy(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a reader with no entropy minted a spin seed anyway")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "entropy") {
			t.Errorf("the panic does not say what was missing: %v", r)
		}
	}()
	_ = mintSpinSeed(dryWell{})
}

// And the default, which is the line the served app runs: a seed inside the
// range the client is handed back, and not the same number twice.
func TestAMintedSpinSeedIsAThirtyTwoBitNumber(t *testing.T) {
	t.Parallel()
	ceiling := new(big.Int).Lsh(big.NewInt(1), 32)
	seen := map[string]bool{}
	for range 64 {
		got := mintSpinSeed(crand.Reader)
		if got.Sign() < 0 || got.Cmp(ceiling) >= 0 {
			t.Fatalf("minted %s, which is outside [0, 2**32)", got)
		}
		seen[got.String()] = true
	}
	if len(seen) < 2 {
		t.Error("64 minted spin seeds were all the same number")
	}
}
