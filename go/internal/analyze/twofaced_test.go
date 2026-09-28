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

// And the other half of the same rule: what the land COUNT does with them.
//
// The curve used to ask `IsLand` while the land count asked the category, so a
// modal DFC filed under a spell fell between the two — off the curve as a
// land, out of the land count as a spell — and the opening hand's land odds
// were computed one land short. These pin the agreement.

func TestAModalDFCFiledAsASpellStillCountsAsALandDrop(t *testing.T) {
	t.Parallel()
	d, cards := theTwoShapes()

	if got := analyze.LandCountOf(d, cards); got != 2 {
		t.Errorf("the land count is %d, want 2 (Forest and Stump Stomp) -- a "+
			"modal DFC filed under %q is still a land you may play, and a count "+
			"that reads the shelf label instead of the card leaves it out",
			got, "interaction")
	}
}

// TestTheLandCountAndTheCurveAlwaysPartitionTheDeck is the invariant, and it
// is the one worth having: every card is either a land drop or a card with a
// mana value, and nothing is both or neither. Before the fix this read 1 + 1
// against a deck of 3.
func TestTheLandCountAndTheCurveAlwaysPartitionTheDeck(t *testing.T) {
	t.Parallel()
	d, cards := theTwoShapes()

	lands := analyze.LandCountOf(d, cards)
	spells := analyze.CurveOf(d, cards).NonlandCards
	if total := d.TotalCards(); lands+spells != total {
		t.Errorf("%d lands + %d nonland cards = %d, but the deck holds %d -- a "+
			"card counted by neither is a card the opening-hand arithmetic and "+
			"the curve disagree about", lands, spells, lands+spells, total)
	}
}

func TestTheOpeningHandDealsFromTheRealLandCount(t *testing.T) {
	t.Parallel()
	d, cards := theTwoShapes()

	withPool := analyze.OpeningHand(d, cards)
	if withPool.Lands.Count != analyze.LandCountOf(d, cards) {
		t.Errorf("the opening table says %d lands and the count says %d -- they "+
			"are the same number or the distribution below is about a different "+
			"deck", withPool.Lands.Count, analyze.LandCountOf(d, cards))
	}
	// The probabilities have to move with it, or the count is decoration: the
	// same deck read with no card records behind it sees only the categories.
	blind := analyze.OpeningHand(d, nil)
	if blind.Lands.Count != 1 {
		t.Fatalf("with no records the count is %d, want 1 (the one card filed "+
			"under 'land') -- this case is what the test compares against",
			blind.Lands.Count)
	}
	if withPool.Lands.Keepable == blind.Lands.Keepable {
		t.Errorf("a two-land reading and a one-land reading of the same deck give "+
			"the same keepable odds (%v) -- then the land count reaches no "+
			"arithmetic at all", withPool.Lands.Keepable)
	}
}

// The union is the rule, so a land filed anywhere counts — this is `messy`'s
// Llanowar Reborn, a plain Land under `ramp`, and the shape that moved the one
// golden this change moves.
func TestALandFiledUnderARampSlotIsStillALandDrop(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{Cards: []deck.CardEntry{
		{Name: "Llanowar Reborn", Category: "ramp", Qty: 1},
		{Name: "Forest", Category: "land", Qty: 1},
	}}
	cards := map[string]*pool.CardRecord{
		"Llanowar Reborn": {Name: "Llanowar Reborn", TypeLine: "Land"},
		"Forest":          {Name: "Forest", TypeLine: "Basic Land — Forest"},
	}

	if got := analyze.LandCountOf(d, cards); got != 2 {
		t.Errorf("the land count is %d, want 2 -- the category is where somebody "+
			"shelved the card and the count is what the card does", got)
	}
}

// And the two directions in which nothing may stop being counted: a name the
// shelves cannot resolve has only its own filing to go on, and a card filed
// under `land` that is not one keeps counting rather than vanishing. The gate
// warns about both; this arithmetic is allowed to gain lands, never to lose
// them.
func TestNothingFiledUnderLandStopsBeingCounted(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{Cards: []deck.CardEntry{
		{Name: "Not A Real Card", Category: "land", Qty: 1},
		{Name: "Sol Ring", Category: "land", Qty: 1},
	}}
	cards := map[string]*pool.CardRecord{
		"Sol Ring": {Name: "Sol Ring", TypeLine: "Artifact", ManaCost: cost("{1}")},
	}

	if got := analyze.LandCountOf(d, cards); got != 2 {
		t.Errorf("the land count is %d, want 2 -- an unresolvable name has only "+
			"the deck's own filing behind it, and a card the reader put under "+
			"'land' is not quietly taken off the count by this rule", got)
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
