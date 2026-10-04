package claude

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The card pool when it stops answering **partway through** a per-card mode.
//
// `failingpool_test.go` is the other half of this and it argues the first
// distinction: a pool with no tables fails every read at its *first* query, so
// it proves the brief refuses and can reach nothing after that. The brief is
// four pool reads long, and the slot argument adds two more *after the model has
// answered and been paid for* -- every one of them behind a read that worked. A
// volume that detaches between two of them (ADR 23: the pool file is on the same
// mounted volume as everything else) was a shape nothing here had ever driven.
//
// `pooltest.FaultyPoolOver` is that shape: the fixture pool behind a connector
// with a counter saying how many more statements the library will answer,
// reached the way the app reaches the real pool -- `Use`, a lease, a
// `*pool.Conn`. `pool.Connect` carries the argument for the seam.
//
// **Budgets are swept rather than counted.** A counted budget restates today's
// query order and goes quietly meaningless the day a read is added to the middle
// of the brief. The sweep asks the question these modes actually have to answer:
// *at no point may a half-read brief be sent to the model, and at no point may a
// half-resolved argument be handed to a person.* The second is the expensive
// one -- the call has been made, the prose is in hand, and all that is left is
// turning the names it used into pool rows; a silent failure there publishes an
// argument with every alternative missing, which reads as "the model named
// nothing the pool has" rather than as an outage. Worse, `ResolveAlternatives`
// files an unresolved name under `not_in_pool`, and that is an accusation.

// w2Alternative is a green card the fixture pool holds and the mono-green
// fixture deck does not, read off `tiny_pool.json` rather than remembered. It is
// what makes the alternatives resolution ask the pool anything at all: an empty
// alternatives list returns before the lookup.
const w2Alternative = "Craterhoof Behemoth"

// w2ArgueReply is a model answer with both halves filled, so the two post-call
// reads -- the commanders' own cards for the Rulebreaker clause, and the
// alternatives against the pool -- are both made.
func w2ArgueReply() string {
	return reply{stop: "end_turn", in: 500, out: 20, content: textBlock(
		`{"charges":[{"claim":"Two mana is not the question any more.",` +
			`"ground":"redundancy","fact":"the deck runs 95 Forests",` +
			`"strength":"minor"}],"alternatives":["` + w2Alternative + `"]}`)}.json()
}

// The brief either comes back whole or comes back as an error, at every
// statement the pool can stop answering.
//
// The brief is what the model reads. One assembled over a pool that went away
// halfway through is a prompt with the card's text missing, or the gate's
// verdict missing, or the category's counts missing -- and the model would
// answer anyway, confidently, about a card it had not been shown. That is rule 1
// failing in the one place these modes spend all their time, and it is why
// `Brief` has an `if err != nil` after every read rather than a best-effort
// assembly.
func TestTheCardBriefRefusesAtEveryStatementThePoolCanStopAnswering(t *testing.T) {
	t.Parallel()
	d := fixtureDeck(t, "mono-green")
	path := pooltest.Build(t)

	// Above the longest the brief is: the columns read, the deck's own card
	// lookup, the commander's oracle id, the art overrides, and the single-card
	// lookup made for the card under discussion.
	const widest = 14
	refused, whole := 0, 0
	for budget := 0; budget <= widest; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		var facts wire.OrderedMap
		err := p.Use(context.Background(), func(c *pool.Conn) error {
			got, e := Brief(context.Background(), c, d, "Sol Ring")
			facts = got
			return e
		})
		fault.Heal()
		p.Close()
		if err != nil {
			if facts != nil {
				t.Errorf("at budget %d the brief refused and handed back facts "+
					"beside the refusal: %v", budget, facts)
			}
			refused++
			continue
		}
		if missing := w2BriefHole(facts); missing != "" {
			t.Errorf("at budget %d the brief answered with no %s in it and no "+
				"error", budget, missing)
		}
		whole++
	}
	// Both halves have to happen or the sweep measured one state fifteen times.
	if refused == 0 || whole == 0 {
		t.Errorf("%d budgets refused and %d answered in full -- the sweep never "+
			"crossed from one to the other", refused, whole)
	}
}

// w2BriefHole names the first thing a complete brief has that this one does not,
// or "" when it is whole. A brief that came back without an error and without
// one of these would be the hole the sweep exists to refuse.
func w2BriefHole(facts wire.OrderedMap) string {
	card, _ := kv(facts, "card").(wire.OrderedMap)
	deckFacts, _ := kv(facts, "deck").(wire.OrderedMap)
	gateFacts, _ := kv(facts, "gate").(wire.OrderedMap)
	category, _ := kv(facts, "category").(wire.OrderedMap)
	switch {
	case card == nil:
		return "card"
	case deckFacts == nil:
		return "deck"
	case gateFacts == nil:
		return "gate verdict"
	case category == nil:
		return "category"
	}
	if asString(kv(card, "name")) == "" {
		return "card name"
	}
	// The identity the slot argument asserts the type of, downstream of here.
	if _, ok := kv(deckFacts, "color_identity").([]string); !ok {
		return "colour identity"
	}
	// The one that would go quietly false over a pool that stopped answering
	// between the deck's read and the card's own.
	if inPool, _ := kv(card, "in_pool").(bool); !inPool {
		return "card the pool answered for"
	}
	if _, ok := kv(gateFacts, "deck_errors").(int); !ok {
		return "gate error count"
	}
	return ""
}

// The slot argument's two reads that happen AFTER the model has answered: the
// commanders' own cards, for the clause printed on them (ADR 51), and the
// alternatives it offered, resolved against the pool.
//
// Both sit behind a call that was made and paid for, and both have an
// `if err != nil` that nothing had reached. The report either comes back whole
// or comes back as a refusal; what it may never be is a report with
// `answered_by: claude` over it, an empty alternatives list under it, and no
// word anywhere that the library had stopped answering.
func TestTheSlotArgumentRefusesWhenThePoolGoesAwayAfterTheModelAnswered(t *testing.T) {
	t.Parallel()
	d := fixtureDeck(t, "mono-green")
	path := pooltest.Build(t)

	// Above the longest the whole mode is: the brief's reads, the commanders'
	// lookup, and the alternatives lookup.
	const widest = 16
	refused, answered, called := 0, 0, 0
	for budget := 0; budget <= widest; budget++ {
		api := &scriptedAPI{replies: []string{w2ArgueReply()}}
		ep := api.start(t)
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		var report wire.OrderedMap
		err := p.Use(context.Background(), func(c *pool.Conn) error {
			got, e := Argue(context.Background(), c, d, "Sol Ring",
				ArgueRequest{Endpoint: ep, Requested: "second-opinion"})
			report = got
			return e
		})
		fault.Heal()
		p.Close()
		if api.served > 0 {
			called++
		}
		if err != nil {
			if report != nil {
				t.Errorf("at budget %d the argument refused and handed back a "+
					"report beside the refusal", budget)
			}
			refused++
			continue
		}
		if report == nil {
			t.Errorf("at budget %d the argument answered neither a report nor an error", budget)
			continue
		}
		// A report that came back at all carries the resolved alternative: the
		// model offered one the pool holds, so a report with an empty list is a
		// resolution that failed and said nothing.
		alts, ok := kv(report, "alternatives").([]deckread.NamedCard)
		if !ok {
			t.Errorf("at budget %d the report's alternatives are %T, not a card list",
				budget, kv(report, "alternatives"))
		} else if asked, _ := kv(report, "asked").(bool); asked && len(alts) == 0 {
			t.Errorf("at budget %d a paid-for argument came back with none of the "+
				"alternatives it offered and no refusal beside it", budget)
		}
		answered++
	}
	if refused == 0 || answered == 0 {
		t.Errorf("%d budgets refused and %d answered -- the sweep never crossed "+
			"from one to the other", refused, answered)
	}
	// The expensive half is the point: some budget has to get as far as a call
	// and then fail, or this sweep only re-proves what the brief's own test says.
	if called == 0 {
		t.Error("no budget got as far as asking the model, so no post-call " +
			"refusal was driven at all")
	}
	if called == widest+1 {
		t.Error("every budget got as far as asking the model, so the brief's own " +
			"reads were never the ones that failed")
	}
}

// Every name the model offered is accounted for, including the ones past the
// lookup's own cap.
//
// `ResolveAlternatives` asks `deckread.CardsNamed` to resolve the whole list and
// that lookup caps itself at `deckread.MaxNamedCards` names -- so a longer list
// comes back with the overflow in *neither* `Cards` nor `NotFound`. The
// belt-and-braces arm is what catches those, and without it a model that offered
// a hundred and one cards would have the hundred and first silently disappear:
// not kept, not dropped, not counted anywhere. The count is the assertion,
// because a number is the only thing that notices a name going missing rather
// than going somewhere.
func TestEveryAlternativeIsAccountedForEvenPastTheLookupsOwnCap(t *testing.T) {
	t.Parallel()
	// One more than the lookup will take. Invented names, so none of them
	// resolves and every one has to be reported as unresolved rather than as a
	// card: the fixture pool holds real cards, and this test is about the
	// bookkeeping rather than about Magic.
	asked := deckread.MaxNamedCards + 1
	names := make([]any, 0, asked)
	for i := range asked {
		names = append(names, fmt.Sprintf("Fixture Alternative %03d", i))
	}

	p := pooltest.Open(t)
	err := p.Use(context.Background(), func(c *pool.Conn) error {
		kept, dropped, err := ResolveAlternatives(context.Background(), c, names,
			[]string{"G"}, gate.Rulebreakers{}, map[string]bool{})
		if err != nil {
			return err
		}
		if len(kept) != 0 {
			t.Errorf("%d invented cards survived the pool: %v", len(kept), kept)
		}
		if got := len(dropped.NotInPool); got != asked {
			t.Errorf("%d of %d names came back as unresolved -- the rest went "+
				"nowhere, which is the one thing this resolution exists not to do",
				got, asked)
		}
		// And nothing was filed under a verdict it did not earn.
		if len(dropped.Banned)+len(dropped.OffColour)+len(dropped.AlreadyInDeck)+
			len(dropped.NoPool) != 0 {
			t.Errorf("an unresolved name was filed under a verdict about a real "+
				"card: %+v", dropped)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resolving the alternatives: %v", err)
	}
}

// The premises the two sweeps above rest on, asserted rather than assumed: the
// fixture deck names a commander the pool answers for, so the clause lookup is a
// query rather than a no-op, and the alternative the script offers is a card the
// pool holds and the deck does not, so the resolution has something to resolve.
// If either stopped being true the sweeps would keep passing while driving
// nothing.
func TestTheSlotArgumentSweepsPremisesHold(t *testing.T) {
	t.Parallel()
	d := fixtureDeck(t, "mono-green")
	if len(d.Commander) == 0 {
		t.Fatal("the fixture deck names no commander, so the clause lookup is never made")
	}
	withPool(t, func(c *pool.Conn) {
		found, err := c.GetCards(context.Background(), d.Commander)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range d.Commander {
			if found[name] == nil {
				t.Errorf("the pool does not hold %q", name)
			}
		}
		alt, err := c.GetCards(context.Background(), []string{w2Alternative})
		if err != nil {
			t.Fatal(err)
		}
		if alt[w2Alternative] == nil {
			t.Errorf("the pool does not hold %q, so the sweep's alternative never "+
				"resolves", w2Alternative)
		}
	})
	for _, c := range d.Cards {
		if strings.EqualFold(c.Name, w2Alternative) {
			t.Errorf("%q is already in the fixture deck, so the sweep's "+
				"alternative is dropped before the lookup", w2Alternative)
		}
	}
}
