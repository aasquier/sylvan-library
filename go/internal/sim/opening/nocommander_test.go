package opening

// A hand dealt for a deck with no commander in it.
//
// Every deck the library holds has one, which is exactly why this is worth
// holding: the state reached here is a deck somebody is part-way through
// building, and that is the person commandment 2 is about. The reading has to
// arrive with the seven cards in it and simply nothing in the command zone --
// no colours, no card beside the hand -- rather than a page that refuses
// until the deck is finished.

import (
	"math/big"
	"testing"
)

func TestADeckWithNoCommanderStillDealsASevenCardHand(t *testing.T) {
	t.Parallel()
	headless := tinyDeck()
	headless.Commander = nil
	hand, err := Deal(headless, tinyRecords(t), big.NewInt(20260924))
	if err != nil {
		t.Fatal(err)
	}
	if len(hand.Cards) != Size {
		t.Fatalf("dealt %d cards, want %d", len(hand.Cards), Size)
	}
	if hand.Commander != nil {
		t.Fatalf("a deck with no commander was dealt one: %+v", hand.Commander)
	}
	// No commander is no colour identity, so the reading has nothing to
	// cover and nothing to be missing -- both empty together, rather than
	// every colour reported as missing from a hand that owes none.
	if len(hand.Reading.ColorsCovered) != 0 || len(hand.Reading.ColorsMissing) != 0 {
		t.Fatalf("a deck with no commander covered %v and missed %v",
			hand.Reading.ColorsCovered, hand.Reading.ColorsMissing)
	}
}

func TestACommanderThePoolDoesNotKnowLeavesTheIdentityEmpty(t *testing.T) {
	t.Parallel()
	// The identity comes off Scryfall's own field (rule 2), so a commander
	// the pool has never heard of has no identity to report -- and reporting
	// none is the honest answer, where deriving one from a mana cost would
	// be a number nobody could check.
	stranger := tinyDeck()
	stranger.Commander = []string{"A Commander No Pool Has"}
	hand, err := Deal(stranger, tinyRecords(t), big.NewInt(20260924))
	if err != nil {
		t.Fatal(err)
	}
	if len(hand.Reading.ColorsCovered) != 0 || len(hand.Reading.ColorsMissing) != 0 {
		t.Fatalf("an unresolved commander covered %v and missed %v",
			hand.Reading.ColorsCovered, hand.Reading.ColorsMissing)
	}
	if len(hand.Cards) != Size {
		t.Fatalf("dealt %d cards, want %d", len(hand.Cards), Size)
	}
	// And the deck the library actually has still reports its colours, so
	// the two above are not passing on a reading that never fills the field.
	full, err := Deal(tinyDeck(), tinyRecords(t), big.NewInt(20260924))
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Reading.ColorsCovered)+len(full.Reading.ColorsMissing) == 0 {
		t.Fatal("a deck with a resolved commander reported no colours at all")
	}
}
