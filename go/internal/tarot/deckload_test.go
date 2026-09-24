package tarot

// The one refusal the fortune-teller's table has.
//
// There is no fallback deck and no half a reading: the seventy-eight cards
// are the whole room. So a deck file that does not parse is a build that must
// not ship, caught at start rather than at the table with somebody sitting at
// it. Nothing a querent does can reach this, which is exactly why it is
// written down -- a panic nobody has ever seen fire is a promise.

import (
	"strings"
	"testing"
)

func TestADeckFileThatDoesNotParseIsRefusedAtStart(t *testing.T) {
	t.Parallel()
	var said string
	func() {
		defer func() {
			if r := recover(); r != nil {
				said, _ = r.(string)
			}
		}()
		loadDeck([]byte(`{"cards": [`))
	}()
	if !strings.Contains(said, "the embedded deck is unreadable") {
		t.Fatalf("a damaged deck said %q", said)
	}
}

func TestTheCommittedDeckHoldsTheWholeRiderDeckAndThreeSeats(t *testing.T) {
	t.Parallel()
	// The refusal above would pass against a loader that refused everything,
	// so the committed file is loaded here too. The count that is pinned is
	// the traditional deck's -- twenty-two majors and four suits of
	// fourteen, which is what makes it a tarot deck rather than a pile of
	// cards -- and the entries keyed `mtg-` beside them are the Magic
	// pairings, which grow.
	full, spread, byKey := loadDeck(deckJSON)
	if len(spread) != 3 {
		t.Fatalf("the spread has %d seats", len(spread))
	}
	traditional := 0
	for _, c := range full {
		if !strings.HasPrefix(c.Key, "mtg-") {
			traditional++
		}
	}
	if traditional != 78 {
		t.Fatalf("the deck holds %d traditional cards of a possible 78", traditional)
	}
	if len(full) <= traditional {
		t.Fatalf("no Magic card stands beside the %d traditional ones", traditional)
	}
	// A key written twice loses a card silently, so the index is checked to
	// be the deck seen a second way rather than a map of the right size.
	if len(byKey) != len(full) {
		t.Fatalf("%d cards indexed as %d keys -- a key is written twice", len(full), len(byKey))
	}
	for _, c := range full {
		if byKey[c.Key].Name != c.Name {
			t.Fatalf("%q indexes to %q", c.Key, byKey[c.Key].Name)
		}
	}
}
