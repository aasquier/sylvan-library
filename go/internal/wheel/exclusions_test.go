package wheel_test

// Two things the spin owes the deck it is spinning for.
//
// The wheel hands somebody a card they are not already playing -- that is the
// whole of what makes it a suggestion rather than a shrug -- so every name the
// deck already holds is excluded from the draw. A companion is one of those
// names: it is bought into the hand rather than played out of the 99, which is
// exactly the kind of card a list that only read `cards:` would offer back.

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wheel"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// cardNameOf reads the card's name out of a spin, or "" when the wheel had
// nothing to hand over.
func cardNameOf(t *testing.T, spun wire.OrderedMap) string {
	t.Helper()
	raw, err := wire.MarshalOrdered(spun)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Card *struct {
			Name string `json:"name"`
		} `json:"card"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Card == nil {
		return ""
	}
	return payload.Card.Name
}

func TestACompanionIsACardTheDeckAlreadyHolds(t *testing.T) {
	t.Parallel()
	fx := loadSpins(t)
	d, err := deck.FromText(fx.Decks["mono-green"], "mono-green")
	if err != nil {
		t.Fatal(err)
	}
	p := pooltest.Open(t)
	ctx := context.Background()
	err = p.Use(ctx, func(c *pool.Conn) error {
		identity := map[string]bool{"G": true}
		seed := big.NewInt(7)

		spun, err := wheel.Spin(ctx, d, identity, c, seed)
		if err != nil {
			t.Fatal(err)
		}
		offered := cardNameOf(t, spun)
		if offered == "" {
			t.Fatal("the fixture deck and seed handed nothing over, so there is " +
				"nothing to exclude")
		}

		// Name it as the deck's companion and spin the same seed again. The
		// card is now one of the deck's own, and the wheel does not hand back
		// a card somebody is already playing.
		withCompanion := *d
		withCompanion.Companion = &offered
		again, err := wheel.Spin(ctx, &withCompanion, identity, c, seed)
		if err != nil {
			t.Fatal(err)
		}
		if got := cardNameOf(t, again); got == offered {
			t.Fatalf("the wheel handed back %q, which the deck holds as its companion", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestASpinAbandonedMidQueryIsAnErrorRatherThanAnEmptyWheel(t *testing.T) {
	t.Parallel()
	// Somebody closed the tab. The count never comes back, and the answer
	// has to be an error -- "the pool holds no legal card" is a sentence
	// about the deck, and saying it here would be a lie about somebody's
	// colours told because a connection went away.
	fx := loadSpins(t)
	d, err := deck.FromText(fx.Decks["mono-green"], "mono-green")
	if err != nil {
		t.Fatal(err)
	}
	p := pooltest.Open(t)
	err = p.Use(context.Background(), func(c *pool.Conn) error {
		gone, cancel := context.WithCancel(context.Background())
		cancel()
		spun, err := wheel.Spin(gone, d, map[string]bool{"G": true}, c, big.NewInt(7))
		if err == nil {
			t.Fatalf("an abandoned spin answered %v", spun)
		}
		if spun != nil {
			t.Fatalf("a refused spin handed back %v", spun)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
