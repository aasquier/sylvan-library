package wheel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wheel"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

type spinsFile struct {
	Decks map[string]string `json:"decks"`
	Cases []struct {
		Deck     string      `json:"deck"`
		Identity []string    `json:"identity"`
		Seed     json.Number `json:"seed"`
		Rendered string      `json:"rendered"`
	} `json:"cases"`
}

func loadSpins(t *testing.T) spinsFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/spins.json")
	if err != nil {
		t.Fatalf("spins.json: %v (testdata/spins.json is a frozen golden)", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var fx spinsFile
	if err := dec.Decode(&fx); err != nil {
		t.Fatalf("spins.json: %v", err)
	}
	return fx
}

// TestEverySpinDealsTheRecordedFate is the corpus: the same seed over the
// same pool and the same deck deals the same fate, the same face and the
// same card, compared as the marshalled payload bytes -- most ways an
// encoding could drift present as the same-looking value, so the bytes are
// the claim.
func TestEverySpinDealsTheRecordedFate(t *testing.T) {
	fx := loadSpins(t)
	if len(fx.Cases) < 20 {
		t.Fatalf("only %d spin cases; the corpus has thinned", len(fx.Cases))
	}
	p := pooltest.Open(t)
	decks := map[string]*deck.Deck{}
	for name, text := range fx.Decks {
		d, err := deck.FromText(text, name)
		if err != nil {
			t.Fatalf("deck %q: %v", name, err)
		}
		decks[name] = d
	}
	ctx := context.Background()
	err := p.Use(ctx, func(c *pool.Conn) error {
		for i, tc := range fx.Cases {
			seed, ok := new(big.Int).SetString(tc.Seed.String(), 10)
			if !ok {
				t.Fatalf("case %d: unreadable seed %q", i, tc.Seed)
			}
			identity := map[string]bool{}
			for _, col := range tc.Identity {
				identity[col] = true
			}
			spun, err := wheel.Spin(ctx, decks[tc.Deck], identity, c, seed)
			if err != nil {
				t.Fatalf("case %d (seed %s): %v", i, tc.Seed, err)
			}
			got, err := wire.MarshalOrdered(spun)
			if err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
			if string(got) != tc.Rendered {
				t.Errorf("case %d (deck %s, seed %s) diverged:\n got %s\nwant %s",
					i, tc.Deck, tc.Seed, got, tc.Rendered)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// An unseeded spin invents its own seed -- drawn from entropy, under
// 2**32 -- and reports it, so any spin can be spun again.
func TestAnUnseededSpinReportsAReplayableSeed(t *testing.T) {
	fx := loadSpins(t)
	d, err := deck.FromText(fx.Decks["mono-green"], "mono-green")
	if err != nil {
		t.Fatal(err)
	}
	p := pooltest.Open(t)
	ctx := context.Background()
	err = p.Use(ctx, func(c *pool.Conn) error {
		first, err := wheel.Spin(ctx, d, map[string]bool{"G": true}, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		var seed *big.Int
		for _, kv := range first {
			if kv.Key == "seed" {
				seed = kv.Value.(*big.Int)
			}
		}
		if seed == nil || seed.Sign() < 0 ||
			seed.Cmp(new(big.Int).Lsh(big.NewInt(1), 32)) >= 0 {
			t.Fatalf("invented seed %v is not in [0, 2**32)", seed)
		}
		again, err := wheel.Spin(ctx, d, map[string]bool{"G": true}, c, seed)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := wire.MarshalOrdered(first)
		b, _ := wire.MarshalOrdered(again)
		if string(a) != string(b) {
			t.Fatalf("replaying the reported seed dealt differently:\n%s\n%s", a, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
