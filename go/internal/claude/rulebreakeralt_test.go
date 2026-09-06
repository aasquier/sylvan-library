package claude

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The slot argument's alternatives under a Rulebreaker commander (ADR 51).
//
// This filter is the one place the clause matters most, and not because of the
// card it hides. `ResolveAlternatives` does not merely drop an off-colour card
// -- it files it under `off_colour` and hands the user a list of the model's
// supposed mistakes. Under Seluma an off-colour Angel is the *correct*
// suggestion, so without the clause the mode would accuse the model of an error
// it did not make, in writing, on the page.
//
// The commander's clause below is the sentence printed on the real card, read
// out of the card pool on 2026-09-06; the cards it is tested against are
// invented, as `pooltest.Card` requires.
func TestAnAlternativeTheCommandersClauseAllowsIsNotCalledOffColour(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pooltest.OpenWith(t,
		pooltest.Card{Name: "Fixture Seluma", ManaCost: "{4}{W}", CMC: 5,
			TypeLine: "Legendary Creature — Angel Warrior", ColorIdentity: []string{"W"},
			OracleText: "Rulebreaker — A deck with this commander can have Angel cards of any " +
				"color identity and any basic land cards.\nFlying"},
		pooltest.Card{Name: "Fixture Red Angel", ManaCost: "{5}{R}{W}", CMC: 7,
			TypeLine: "Legendary Creature — Angel", OracleText: "Flying, first strike",
			ColorIdentity: []string{"R", "W"}},
		pooltest.Card{Name: "Fixture Red Dragon", ManaCost: "{5}{R}{W}", CMC: 7,
			TypeLine: "Legendary Creature — Dragon", OracleText: "Flying, haste",
			ColorIdentity: []string{"R", "W"}},
	)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := c.GetCards(ctx, []string{"Fixture Seluma"})
		if err != nil {
			return err
		}
		breakers := gate.ReadRulebreakers([]*pool.CardRecord{found["Fixture Seluma"]})
		names := []any{"Fixture Red Angel", "Fixture Red Dragon"}

		// Mono-white, no clause: both are off-colour, which is the answer
		// every deck in the library gets and must keep getting.
		kept, dropped, err := ResolveAlternatives(ctx, c, names, []string{"W"}, nil, nil)
		if err != nil {
			return err
		}
		if len(kept) != 0 || len(dropped.OffColour) != 2 {
			t.Errorf("without a clause: kept %d, off-colour %v", len(kept), dropped.OffColour)
		}

		// The same two names under the commander that names Angels.
		kept, dropped, err = ResolveAlternatives(ctx, c, names, []string{"W"}, breakers, nil)
		if err != nil {
			return err
		}
		if len(kept) != 1 || kept[0].Name != "Fixture Red Angel" {
			t.Errorf("the Angel the commander allows was not kept: %+v", kept)
		}
		if len(dropped.OffColour) != 1 || dropped.OffColour[0] != "Fixture Red Dragon" {
			t.Errorf("off-colour is %v, want only the Dragon -- the clause names Angels, "+
				"and widening it further would pass a card the commander never allowed",
				dropped.OffColour)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A base install has no card pool, and this mode still has to answer.
// `deckread.CardsNamed` is built for that -- it reports every name under
// `no_pool` rather than accusing the model of inventing them -- but
// `pool.Conn.GetCards`, which the clause lookup uses, has no such guard and
// dereferences its receiver. Reading the commander's clause on that path was
// one unguarded call away from turning "no card pool yet" into a panic.
func TestReadingTheCommandersClauseSurvivesNoPoolAtAll(t *testing.T) {
	t.Parallel()
	kept, dropped, err := ResolveAlternatives(context.Background(), nil,
		[]any{"Fixture Red Angel"}, []string{"W"}, nil, nil)
	if err != nil {
		t.Fatalf("no pool is a reported outcome, not an error: %v", err)
	}
	if len(kept) != 0 || len(dropped.NoPool) != 1 || len(dropped.OffColour) != 0 {
		t.Fatalf("kept %d, no_pool %v, off_colour %v -- a name nobody could look up is "+
			"neither kept nor blamed on its colours", len(kept), dropped.NoPool, dropped.OffColour)
	}
}
