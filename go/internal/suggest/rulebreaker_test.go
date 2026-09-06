package suggest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
)

// Suggestions under a Rulebreaker commander (ADR 51).
//
// This is the quietest of the surfaces the clause reaches, and the reason it is
// worth a test of its own: nothing here ever reports an error. A Seluma deck
// that is never offered an off-colour Angel looks exactly like a Seluma deck
// for which no good Angel exists, and the only way to notice is to wonder.
//
// The commander's clause is the sentence printed on the real card, read out of
// the card pool on 2026-09-06 (`mtglab cards show 'Seluma, Light of Aysen'`);
// everything else below is invented, because what is under test is the shape of
// a type line against the shape of a clause.
const selumaClause = "Rulebreaker — A deck with this commander can have Angel cards of any " +
	"color identity and any basic land cards.\nFlying"

func selumaPool(t *testing.T) *pool.Pool {
	t.Helper()
	return pooltest.OpenWith(t,
		pooltest.Card{Name: "Fixture Seluma", ManaCost: "{4}{W}", CMC: 5,
			TypeLine: "Legendary Creature — Angel Warrior", OracleText: selumaClause,
			ColorIdentity: []string{"W"}},
		// A red-white Angel: outside mono-white, and named by the clause.
		pooltest.Card{Name: "Fixture Red Angel", ManaCost: "{5}{R}{W}", CMC: 7,
			TypeLine: "Legendary Creature — Angel", OracleText: "Flying, first strike",
			ColorIdentity: []string{"R", "W"}},
		// A red-white creature that is not an Angel: outside mono-white, and
		// not named. Its presence is what stops "the clause widened
		// everything" from passing this test.
		pooltest.Card{Name: "Fixture Red Dragon", ManaCost: "{5}{R}{W}", CMC: 7,
			TypeLine: "Legendary Creature — Dragon", OracleText: "Flying, haste",
			ColorIdentity: []string{"R", "W"}},
		// The card the deck is replacing: a plain white Angel, in colour.
		pooltest.Card{Name: "Fixture White Angel", ManaCost: "{5}{W}{W}", CMC: 7,
			TypeLine: "Creature — Angel", OracleText: "Flying, vigilance",
			ColorIdentity: []string{"W"}},
	)
}

func namesOf(found []*pool.CardRecord) map[string]bool {
	out := map[string]bool{}
	for _, rec := range found {
		out[rec.Name] = true
	}
	return out
}

// The candidate pool widens for the cards the clause names, and for nothing
// else. Both halves are asserted from one query, because a widening that let
// the Dragon through as well would still pass the first half.
func TestTheCandidatePoolWidensForTheCardsTheClauseNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := selumaPool(t)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := c.GetCards(ctx, []string{"Fixture Seluma", "Fixture White Angel"})
		if err != nil {
			return err
		}
		target, commander := found["Fixture White Angel"], found["Fixture Seluma"]
		if target == nil || commander == nil {
			t.Fatalf("the doctored rows did not come back: %v", found)
		}
		mono := map[string]bool{"W": true}

		// Without the clause: mono-white, and the red-white Angel is out.
		plain, err := suggest.CandidatePool(ctx, c, target, mono, nil, 400)
		if err != nil {
			return err
		}
		if got := namesOf(plain); got["Fixture Red Angel"] || got["Fixture Red Dragon"] {
			t.Errorf("an off-identity card reached a deck with no Rulebreaker: %v", got)
		}

		// With it: the Angel arrives, the Dragon does not.
		breakers := gate.ReadRulebreakers([]*pool.CardRecord{commander})
		widened, err := suggest.CandidatePool(ctx, c, target, mono, breakers, 400)
		if err != nil {
			return err
		}
		got := namesOf(widened)
		if !got["Fixture Red Angel"] {
			t.Error("the clause names Angels of any colour, and the red-white one was not offered")
		}
		if got["Fixture Red Dragon"] {
			t.Error("the clause names Angels, and an off-colour Dragon was offered")
		}
		// Everything the unwidened query found is still there: widening adds,
		// it never takes away.
		for name := range namesOf(plain) {
			if !got[name] {
				t.Errorf("%s was in the pool before the clause and is gone after it", name)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// The whole path, through the function the deck page actually calls.
func TestReplacementsUnderARulebreakerCommanderOfferTheAngel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := selumaPool(t)
	d := &deck.Deck{
		Slug: "seluma", Name: "Seluma", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture Seluma"},
		Cards: []deck.CardEntry{{Name: "Fixture White Angel", Category: "threat", Qty: 1,
			Why: "The Angel this deck is replacing."}},
	}
	if err := p.Use(ctx, func(c *pool.Conn) error {
		cards, err := c.GetCards(ctx, []string{"Fixture Seluma", "Fixture White Angel"})
		if err != nil {
			return err
		}
		got, err := suggest.ReplacementsFor(ctx, c, d, cards, "Fixture White Angel", 25)
		if err != nil {
			return err
		}
		names := []string{}
		for _, cand := range got {
			names = append(names, cand.Name())
		}
		if !contains(names, "Fixture Red Angel") {
			t.Errorf("replacements for an Angel in a Seluma deck did not include the "+
				"off-colour Angel: %s", strings.Join(names, ", "))
		}
		if contains(names, "Fixture Red Dragon") {
			t.Errorf("replacements included a card the clause does not name: %s",
				strings.Join(names, ", "))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
