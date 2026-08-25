package pool_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// Token art, and the one ruling it exists to enforce.
//
// The fixture carries the two real Food printings that make the case: Throne
// of Eldraine's from 2019, painted by Randy Gallegos — the pie everybody who
// has played with a Food would recognise — and a Secret Lair from 2026 whose
// flavour text is about Bilbo's second breakfast. Both are Scryfall rows read
// out of the real pool. Answering "Food" with the second one is the same
// mistake that put Teenage Mutant Ninja Turtles art on the Grand Coliseum,
// and it is what `GetCards` would do, because `GetCards` answers with a card's
// newest printing.

func TestATokenIsAnsweredWithItsOriginalArt(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := c.TokenArtFor(ctx, []string{"Food"})
		if err != nil {
			return err
		}
		art, ok := found["food"]
		if !ok {
			t.Fatal("Food is in `printings` and was not found; a token's art " +
				"is the whole reason this method exists")
		}
		if art.Artist != "Randy Gallegos" {
			t.Errorf("Food was painted by %q, want Randy Gallegos — the "+
				"newest printing is a Secret Lair, and answering with it is "+
				"the Ninja Turtles mistake", art.Artist)
		}
		if art.Set != "TELD" {
			t.Errorf("Food came from %q, want TELD (Throne of Eldraine "+
				"Tokens), which is the original", art.Set)
		}
		if art.Image == "" {
			t.Error("Food resolved with no painting, which is the one field " +
				"a board needs")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The property that makes this a separate method rather than a clever
// `GetCards`: a token is deliberately absent from `oracle_cards`, because a
// token is not a card anybody puts in a deck.
func TestATokenIsNotInTheOracleTableAndThatIsCorrect(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := c.GetCards(ctx, []string{"Food"})
		if err != nil {
			return err
		}
		if len(found) != 0 {
			t.Errorf("`GetCards` answered Food with %v; `SkipOracleLayout` "+
				"drops tokens from `oracle_cards` on purpose", found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Forge spells a token with a suffix Scryfall does not use.
func TestForgesTokenSuffixIsStripped(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ forge, want string }{
		{"Food Token", "Food"},
		{"Treasure Token", "Treasure"},
		// A name that merely ends in the word is not a suffix to cut — only a
		// trailing " Token" is, and only on a card Forge flagged as one.
		{"Food", "Food"},
		{"Token of Unity", "Token of Unity"},
	} {
		if got := pool.TokenName(c.forge); got != c.want {
			t.Errorf("TokenName(%q) = %q, want %q", c.forge, got, c.want)
		}
	}
}

// A name nothing has printed is absent rather than an error: a board draws a
// plate with no painting on it, and never refuses to draw.
func TestAnUnknownTokenIsSimplyMissing(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := c.TokenArtFor(ctx, []string{"Nonexistent Token", ""})
		if err != nil {
			return err
		}
		if len(found) != 0 {
			t.Errorf("an unprinted token resolved to %v", found)
		}
		empty, err := c.TokenArtFor(ctx, nil)
		if err != nil || len(empty) != 0 {
			t.Errorf("no names came back as %v, %v", empty, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
