package analyze_test

import (
	"slices"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Which two-faced cards count as spells, and which as lands.
//
// The curve and the pip requirements both skip lands in order to count
// spells, so "is this a land" decides whether a card is *analysed at all*.
// Asking it of the combined type line answered wrongly for every `transform`
// card with a land on the back -- 32 of them in the pool -- because those are
// artifacts and creatures you cast, and the land only arrives by flipping.
// The rule that matters is [pool.CardRecord.IsLand]'s, and these cases pin
// both sides of it.

func cost(s string) *string { return &s }

// theTwoShapes is a transform card whose back is a land (a spell: you cast
// the front) and a modal DFC whose back is a land (a land: you may simply
// play that face), filed the way a player would file each.
func theTwoShapes() (*deck.Deck, map[string]*pool.CardRecord) {
	d := &deck.Deck{Cards: []deck.CardEntry{
		{Name: "Treasure Map", Category: "engine", Qty: 1},
		{Name: "Stump Stomp", Category: "interaction", Qty: 1},
		{Name: "Forest", Category: "land", Qty: 1},
	}}
	return d, map[string]*pool.CardRecord{
		// Artifact you cast for {2}; flips into Treasure Cove later.
		"Treasure Map": {
			Name: "Treasure Map", TypeLine: "Artifact // Land",
			Layout: "transform", ManaCost: cost("{2}"),
		},
		// Modal DFC: {1}{R} sorcery, or play the back as a land.
		"Stump Stomp": {
			Name: "Stump Stomp", TypeLine: "Sorcery // Land",
			Layout: "modal_dfc", ManaCost: cost("{1}{R}"),
		},
		"Forest": {Name: "Forest", TypeLine: "Basic Land — Forest"},
	}
}

func TestATransformCardWithALandBackIsStillASpellOnTheCurve(t *testing.T) {
	t.Parallel()
	d, cards := theTwoShapes()

	curve := analyze.CurveOf(d, cards)

	var named []string
	for _, b := range curve.Buckets {
		named = append(named, b.Names...)
	}
	if !slices.Contains(named, "Treasure Map") {
		t.Errorf("Treasure Map is missing from the curve (%v) -- it is an "+
			"artifact you cast for {2}, and the land only arrives by flipping, "+
			"so a curve that drops it is hiding a two-drop from the reader", named)
	}
	// The modal DFC is genuinely a land drop, so it stays out.
	if slices.Contains(named, "Stump Stomp") {
		t.Errorf("Stump Stomp is on the curve (%v) -- its back may simply be "+
			"played as a land, which is the case IsLand is written to catch", named)
	}
	if curve.NonlandCards != 1 {
		t.Errorf("the curve counted %d nonland cards, want 1 (Treasure Map "+
			"alone)", curve.NonlandCards)
	}
}

func TestATransformCardsPipsAreStillAskedFor(t *testing.T) {
	t.Parallel()
	d, cards := theTwoShapes()
	// Give the transform card a coloured cost, so there is a pip to lose.
	cards["Treasure Map"].ManaCost = cost("{1}{G}")

	needs := analyze.PipRequirements(d, cards)

	var green *analyze.ColorNeed
	for i := range needs {
		if needs[i].Color == "G" {
			green = &needs[i]
		}
	}
	if green == nil || green.Pips == 0 {
		t.Errorf("no green pip demand was counted (%+v) -- a spell skipped as "+
			"a land takes its coloured requirements out of the mana base's "+
			"arithmetic with it", needs)
	}
}
