package deckread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The defaults and the ceilings the card doors fall back on.
//
// None of these is a fault path. They are the answers a caller gets when it
// asks for nothing in particular, and every one of them is a promise made to
// somebody who is not there to complain: a search with no ordering asked for
// still comes back in a sensible order, a search with no size asked for comes
// back a page long rather than the whole index, and a lookup handed a hundred
// and one names stops at the hundred rather than building a query nobody
// bounded. A wrong answer here is invisible -- it simply looks like a search.

// A search that asks for nothing in particular is still a bounded, ordered
// page, and a sort nobody offers falls back to the one the page expects
// rather than to the database's own order.
func TestASearchWithNothingAskedForIsStillBoundedAndOrdered(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	withPool(t, func(c *pool.Conn) {
		// No sort, no limit: the page's defaults.
		plain, err := SearchCards(ctx, c, SearchQuery{})
		if err != nil {
			t.Fatalf("a bare search: %v", err)
		}
		if len(plain) == 0 {
			t.Fatal("a bare search found nothing, so nothing below is being asked")
		}
		if len(plain) > 60 {
			t.Errorf("a search with no size asked for returned %d cards", len(plain))
		}

		// A sort key nobody offers is not an error and not the database's
		// own order: it is the same order the bare search gives.
		nonsense, err := SearchCards(ctx, c, SearchQuery{Sort: "by-vibes"})
		if err != nil {
			t.Fatalf("an unknown sort: %v", err)
		}
		if len(nonsense) != len(plain) {
			t.Fatalf("an unknown sort returned %d cards where the default returned %d",
				len(nonsense), len(plain))
		}
		for i := range plain {
			if nonsense[i].Name != plain[i].Name {
				t.Errorf("an unknown sort ordered the results differently at %d: "+
					"%q rather than %q", i, nonsense[i].Name, plain[i].Name)
				break
			}
		}

		// A negative size is the same as none: the alternative is a query
		// with `LIMIT -1` in it.
		negative, err := SearchCards(ctx, c, SearchQuery{Limit: -5})
		if err != nil {
			t.Fatalf("a negative size: %v", err)
		}
		if len(negative) != len(plain) {
			t.Errorf("a negative size returned %d cards", len(negative))
		}
	})
}

// A lookup of nothing is an empty answer rather than a query, and blank
// entries in a list of names are dropped rather than looked up.
func TestALookupOfNothingAsksThePoolNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// **No pool is needed and none is used.** A list of blanks resolves to
	// nothing to ask about, so the answer arrives before the pool is
	// consulted -- which is why this can be asserted with a nil connection.
	for _, names := range [][]string{nil, {}, {""}, {"   ", "\t", ""}} {
		got, err := CardsNamed(ctx, nil, names)
		if err != nil {
			t.Fatalf("%v: %v", names, err)
		}
		if len(got.Cards) != 0 || len(got.NotFound) != 0 {
			t.Errorf("%v resolved to %+v", names, got)
		}
		// And it does not claim the pool was missing, because it was never
		// asked: `pool_available: false` is what the page renders the "I
		// could not look" sentence from.
		if !got.PoolAvailable {
			t.Errorf("%v reported the pool as absent without ever asking it", names)
		}
	}
}

// A lookup stops at its ceiling rather than building a query as long as
// whatever arrived.
func TestALookupStopsAtItsCeiling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A hundred and one obviously invented names, plus one real card at the
	// very end that the ceiling must cut off.
	names := make([]string, 0, MaxNamedCards+1)
	for i := 0; i < MaxNamedCards; i++ {
		names = append(names, fmt.Sprintf("Fixture Card Number %d", i))
	}
	names = append(names, "Sol Ring")

	withPool(t, func(c *pool.Conn) {
		got, err := CardsNamed(ctx, c, names)
		if err != nil {
			t.Fatalf("the lookup: %v", err)
		}
		if len(got.Cards)+len(got.NotFound) != MaxNamedCards {
			t.Errorf("a list of %d names resolved to %d answers",
				len(names), len(got.Cards)+len(got.NotFound))
		}
		// The name past the ceiling is simply not in the answer -- not
		// reported as missing, which would be a claim about the pool.
		for _, miss := range got.NotFound {
			if miss == "Sol Ring" {
				t.Error("a name cut off by the ceiling was reported as not found, " +
					"which says the pool does not hold a card it does hold")
			}
		}
		for _, card := range got.Cards {
			if card.Name == "Sol Ring" {
				t.Error("the ceiling did not cut")
			}
		}
	})
}

// A combo block says who wrote it only when somebody other than a person did.
//
// ADR 41's rule applied to a block rather than to a sentence: a mark is a
// thing that is *there*, and writing an empty one onto every entry hands the
// page a value to weigh on each of them -- which is how a blank badge appears
// beside ninety-nine combos nobody drafted.
func TestACombosMarkIsWrittenOnlyWhenThereIsOneToWrite(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Slug: "combos", Name: "Combos", Status: "theoretical", Stage: "draft",
		Cards: []deck.CardEntry{{Name: "Sol Ring", Why: "ramp"}},
		Combos: []deck.Combo{
			{Cards: []string{"Sol Ring"}, Produces: "mana", How: "it taps"},
			{Cards: []string{"Sol Ring"}, Produces: "mana", How: "it taps", By: "claude"},
		},
	}
	rows := ComboRows(d, map[string]*pool.CardRecord{})
	if len(rows) != 2 {
		t.Fatalf("two combos made %d rows", len(rows))
	}
	written, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), `"by"`) {
		t.Errorf("a combo nobody drafted carries a mark: %s", written)
	}
	drafted, err := json.Marshal(rows[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(drafted), `"by":"claude"`) {
		t.Errorf("a drafted combo lost its mark: %s", drafted)
	}
}
