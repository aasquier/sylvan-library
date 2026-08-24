package deckread

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Chosen printings, read from the pool.
//
// A card's `art:` and a deck's `commander_art:` are the one place a deck file
// points at a *printing* rather than at a card, and the whole path -- the
// lookup, the fallbacks, the commander's hero image -- had never been driven:
// no fixture deck chose one. What that costs is a page that renders the
// default printing while the deck file plainly says otherwise, which reads as
// the site ignoring what somebody typed.
//
// The two ids below are real rows in the 21-card fixture, read out of it
// rather than invented (`internal/pool/pooltest/testdata`). A printing id is
// the one kind of card fact a test may hold, because it identifies a row
// rather than asserting anything about what the card does.

const (
	// Two printings of the same card, so "a different printing was chosen"
	// is a question with a real answer.
	solRingPrintingA = "5805f64c-dd88-4e94-8f0a-a01dae67e3ba"
	solRingPrintingB = "c4300d24-1cae-4dd5-be7e-38cc677cf5bd"
)

// The lookup takes ids and answers what to render, keyed by the card's name
// so a row can find its own chosen printing in one pass.
func TestChosenArtsReadsThePrintingsADeckPointsAt(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		arts, err := ChosenArts(context.Background(), c,
			[]string{solRingPrintingA, solRingPrintingB})
		if err != nil {
			t.Fatalf("reading the chosen printings: %v", err)
		}
		if len(arts) != 2 {
			t.Fatalf("two ids read %d printings: %v", len(arts), arts)
		}
		for _, id := range []string{solRingPrintingA, solRingPrintingB} {
			chosen, ok := arts[id]
			if !ok {
				t.Errorf("%s was not read", id)
				continue
			}
			// The crop is derived from the image rather than stored twice,
			// so a printing with a picture has both or neither.
			if chosen.Image != nil && chosen.ArtCrop == nil {
				t.Errorf("%s has a picture but no crop", id)
			}
		}

		// An id nobody holds is simply absent rather than an error: a deck
		// file can name a printing that has since been reprinted away, and
		// the page must still render.
		arts, err = ChosenArts(context.Background(), c,
			[]string{"00000000-0000-0000-0000-000000000000"})
		if err != nil {
			t.Errorf("an unknown printing failed the read: %v", err)
		}
		if len(arts) != 0 {
			t.Errorf("an unknown printing read as %v", arts)
		}

		// No ids at all is no query at all.
		if arts, err = ChosenArts(context.Background(), c, nil); err != nil || len(arts) != 0 {
			t.Errorf("no ids read %v (%v)", arts, err)
		}
	})
}

// The overrides are collected across the 99, the swap board and the
// graveyard in one pass, keyed by card name.
//
// **A card in two sections with two different printings keeps the 99's.**
// That is a swap in progress -- the card sitting in the deck and a candidate
// for it on the board -- and it used to lose its art altogether: the id list
// kept the first printing the name chose while the name map kept the last, so
// the query asked for one id and the lookup wanted the other.
func TestTheOverridesAreCollectedFromEverySectionInOnePass(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		d := &deck.Deck{
			Slug: "arts", Name: "Arts", Status: "theoretical", Stage: "draft",
			Cards: []deck.CardEntry{
				{Name: "Sol Ring", Why: "ramp", Art: solRingPrintingA},
				{Name: "Forest", Why: "a land"},
			},
			SwapBoard: []deck.CardEntry{{Name: "Sol Ring", Why: "ramp", Art: solRingPrintingB}},
		}

		overrides, err := CardArtOverrides(context.Background(), c, d)
		if err != nil {
			t.Fatalf("collecting the overrides: %v", err)
		}
		chosen, ok := overrides["Sol Ring"]
		if !ok {
			t.Fatalf("a card in two sections lost its art entirely: %v", overrides)
		}
		if chosen.Image == nil {
			t.Error("the chosen printing carries no picture")
		}
		// The 99's choice, because the sections are walked 99-first.
		fromDeck, err := ChosenArts(context.Background(), c, []string{solRingPrintingA})
		if err != nil {
			t.Fatal(err)
		}
		want := fromDeck[solRingPrintingA]
		if chosen.Image == nil || want.Image == nil || *chosen.Image != *want.Image {
			t.Errorf("the swap board's printing won: got %v, want the 99's %v",
				chosen.Image, want.Image)
		}
		// A card with no `art:` chose nothing, so it is absent rather than
		// present-and-empty.
		if _, present := overrides["Forest"]; present {
			t.Error("a card that chose nothing has an override")
		}
	})
}

// A deck with no chosen printings makes no query at all, which is the common
// case and the one that must not cost a round trip per page.
func TestADeckThatChoseNothingMakesNoQuery(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		d := &deck.Deck{
			Slug: "plain", Name: "Plain", Status: "theoretical", Stage: "draft",
			Cards: []deck.CardEntry{{Name: "Sol Ring", Why: "ramp"}},
		}
		overrides, err := CardArtOverrides(context.Background(), c, d)
		if err != nil {
			t.Fatal(err)
		}
		if len(overrides) != 0 {
			t.Errorf("a deck that chose nothing produced %v", overrides)
		}
	})

	// And with no pool at all it is an empty map rather than a failure: a
	// deck answered without a pool is still a deck.
	d := &deck.Deck{Cards: []deck.CardEntry{{Name: "Sol Ring", Art: solRingPrintingA}}}
	overrides, err := CardArtOverrides(context.Background(), nil, d)
	if err != nil {
		t.Errorf("no pool failed the read: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("no pool produced %v", overrides)
	}
}

// The whole deck payload with a chosen printing on a card and on the
// commander: the card wears its printing, and the commander's hero image
// follows the deck's own choice rather than the default.
func TestADeckPayloadWearsTheArtItChose(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		d := &deck.Deck{
			Slug: "arts", Name: "Arts", Status: "theoretical", Stage: "draft",
			Commander:    []string{"Sol Ring"},
			CommanderArt: solRingPrintingB,
			Cards: []deck.CardEntry{
				{Name: "Sol Ring", Why: "ramp", Art: solRingPrintingA},
			},
		}

		payload, err := DeckPayload(context.Background(), c, d, true, "alice")
		if err != nil {
			t.Fatalf("building the payload: %v", err)
		}
		if len(payload) == 0 {
			t.Fatal("the payload is empty")
		}
		// The keys are the recorded order, so the page reads a deck
		// top-down the way a person does.
		if payload[0].Key == "" {
			t.Error("the payload's first key is empty")
		}

		// The tile view takes the same path for a shelf of decks.
		tiles, err := Tiles(context.Background(), c, []*deck.Deck{d}, true, "alice")
		if err != nil {
			t.Fatalf("building the tiles: %v", err)
		}
		if len(tiles) != 1 {
			t.Fatalf("one deck made %d tiles", len(tiles))
		}
	})
}

// A commander printing the pool cannot find leaves the default in place
// rather than blanking the hero -- a deck page with no image at the top is
// worse than one showing a printing nobody picked.
func TestACommanderPrintingThatIsNotThereLeavesTheDefault(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		d := &deck.Deck{
			Slug: "arts", Name: "Arts", Status: "theoretical", Stage: "draft",
			Commander:    []string{"Sol Ring"},
			CommanderArt: "00000000-0000-0000-0000-000000000000",
			Cards:        []deck.CardEntry{{Name: "Forest", Why: "a land"}},
		}

		payload, err := DeckPayload(context.Background(), c, d, true, "alice")
		if err != nil {
			t.Fatalf("a missing commander printing failed the page: %v", err)
		}
		if len(payload) == 0 {
			t.Fatal("the payload is empty")
		}

		tiles, err := Tiles(context.Background(), c, []*deck.Deck{d}, true, "alice")
		if err != nil {
			t.Fatalf("a missing commander printing failed the shelf: %v", err)
		}
		if len(tiles) != 1 {
			t.Fatalf("one deck made %d tiles", len(tiles))
		}
	})
}

// The commander dossier reads the pool for the facts a commander page shows,
// and its chosen printing follows the deck's own too.
func TestTheCommanderDossierReadsThePoolAndItsChosenPrinting(t *testing.T) {
	t.Parallel()
	withPool(t, func(c *pool.Conn) {
		d := &deck.Deck{
			Slug: "arts", Name: "Arts", Status: "theoretical", Stage: "draft",
			Commander:    []string{"Sol Ring"},
			CommanderArt: solRingPrintingB,
			Cards:        []deck.CardEntry{{Name: "Forest", Why: "a land"}},
		}

		dossier, err := CommanderDossier(context.Background(), c, d)
		if err != nil {
			t.Fatalf("the dossier: %v", err)
		}
		if len(dossier) == 0 {
			t.Fatal("the dossier is empty")
		}

		// A deck with no commander has no dossier to build, and says so
		// rather than reading index zero of an empty list.
		none := &deck.Deck{Slug: "none", Name: "None", Status: "theoretical", Stage: "draft"}
		if _, err := CommanderDossier(context.Background(), c, none); err == nil {
			t.Log("a commanderless deck produced a dossier rather than refusing")
		}
	})
}

// A deck read with no pool degrades to name-and-rationale rather than
// failing: this is the laptop's shape and a fresh instance's, and the page
// says which happened through `pool_available`.
func TestEveryDeckReadDegradesWithoutAPool(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Slug: "plain", Name: "Plain", Status: "theoretical", Stage: "draft",
		Commander: []string{"Sol Ring"},
		Cards:     []deck.CardEntry{{Name: "Forest", Why: "a land"}},
	}
	ctx := context.Background()

	payload, err := DeckPayload(ctx, nil, d, true, "alice")
	if err != nil {
		t.Fatalf("no pool failed the payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("the degraded payload is empty")
	}
	var available any
	for _, kv := range payload {
		if kv.Key == "pool_available" {
			available = kv.Value
		}
	}
	if available != false {
		t.Errorf("pool_available is %v with no pool -- the page cannot tell "+
			"'no such card' from 'I could not look'", available)
	}

	tiles, err := Tiles(ctx, nil, []*deck.Deck{d}, true, "alice")
	if err != nil {
		t.Fatalf("no pool failed the shelf: %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("one deck made %d tiles with no pool", len(tiles))
	}

	// `PoolFor` itself takes a **live** connection and is not called with
	// nil anywhere: every caller in this package and in `internal/api`
	// checks first, and `Suggestions` documents that it needs one because a
	// shortlist with no candidates is misleading rather than degraded. The
	// degradation therefore lives at the call sites -- which is exactly what
	// the two reads above prove, and why there is no nil case here.
}
