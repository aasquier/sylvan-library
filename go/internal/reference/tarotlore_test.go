package reference

import (
	"strings"
	"testing"
)

// The tarot lore's three readers. The theme corpus pins what `TarotOffer`
// writes for a real spread, byte for byte -- what it cannot reach is the
// branches a spread never produces: an offer with nothing left to give, a
// card nobody wrote about, and an id spelled the way a model shouts it.

// The deck tier comes first and always, which is why a reading of three
// minors is not a reading with nothing to say.
func TestTheDeckTierLeadsEveryReading(t *testing.T) {
	t.Parallel()
	deckTier := 0
	for _, f := range Tarot().Facts {
		if f.Card == "" {
			deckTier++
		}
	}
	if deckTier == 0 {
		t.Fatal("no fact is true of the deck itself")
	}
	// Three cards from the minor arcana: the tier with the least written
	// about it, and the spread that would arrive empty without the deck tier.
	minors := []string{}
	for _, f := range Tarot().Facts {
		if f.Card != "" && strings.Contains(f.Card, "-") &&
			!strings.HasPrefix(f.Card, "0") && !strings.HasPrefix(f.Card, "1") &&
			!strings.HasPrefix(f.Card, "2") {
			minors = append(minors, f.Card)
		}
		if len(minors) == 3 {
			break
		}
	}
	if len(minors) != 3 {
		t.Fatalf("could not find three minor cards: %v", minors)
	}
	facts := TarotFactsForReading(minors)
	if len(facts) <= deckTier {
		t.Fatalf("%d facts for three cards, and %d of them are the deck's -- "+
			"the cards contributed nothing", len(facts), deckTier)
	}
	for i := 0; i < deckTier; i++ {
		if facts[i].Card != "" {
			t.Errorf("fact %d is a card's; the deck tier does not lead", i)
		}
	}
	if facts[deckTier].Card != minors[0] {
		t.Errorf("the first card fact is %q, want %q -- the cards follow in "+
			"dealt order", facts[deckTier].Card, minors[0])
	}
	// A card nobody wrote about contributes nothing rather than failing.
	if got := TarotFactsForCard("not-a-card"); len(got) != 0 {
		t.Errorf("a card nobody wrote about has %d facts", len(got))
	}
}

// **An offer with nothing left is the empty string, not a heading over
// nothing.** The caller appends it to the frame unconditionally, so a header
// with no facts under it would be an instruction to cite from an empty list.
// No real spread reaches this -- the deck tier alone is eighteen facts -- so
// it is driven directly.
func TestAnOfferWithNothingLeftIsEmpty(t *testing.T) {
	t.Parallel()
	keys := []string{"00-fool"}
	full := TarotOffer(keys, nil)
	// The offer lists fact **ids**, never the card key -- the id is what the
	// reader puts in `source`, and `KeepFact` looks it up by that.
	if full == "" || !strings.Contains(full, "\n- fool-") {
		t.Fatalf("a full offer carries no fact for the card dealt:\n%s", full)
	}
	if strings.Contains(full, keys[0]) {
		t.Error("the offer names the card key, which is not what a reader cites")
	}

	told := []string{}
	for _, f := range TarotFactsForReading(keys) {
		told = append(told, f.Text)
	}
	if got := TarotOffer(keys, told); got != "" {
		t.Errorf("everything has been told and the offer is still %d "+
			"characters:\n%s", len(got), got)
	}
	// Telling all but one leaves exactly that one, and the header with it.
	got := TarotOffer(keys, told[:len(told)-1])
	if !strings.Contains(got, "True things you know") || strings.Count(got, "\n- ") != 1 {
		t.Errorf("one fact left and the offer reads:\n%s", got)
	}
	// A told fact is matched after stripping, the way `theme.Repeats` sees it.
	padded := append([]string{}, told...)
	padded[0] = "   " + padded[0] + "  "
	if a, b := TarotOffer(keys, padded), TarotOffer(keys, told); a != b {
		t.Error("a told fact with whitespace round it was offered again")
	}
}

// An id is folded and stripped before it is looked up, which is not
// politeness: `theme.KeepFact` matches the `tarot:` prefix case-insensitively,
// so a reader that shouts `TAROT:PIXIE-FEE` gets past the prefix check and
// would then miss here on the one difference nobody would ever debug from a
// dropped-fact counter.
func TestAFactIsFoundHoweverItsIdIsSpelled(t *testing.T) {
	t.Parallel()
	want := Tarot().Facts[0].ID
	for _, spelling := range []string{want, strings.ToUpper(want), "  " + want + " "} {
		got := TarotFactByID(spelling)
		if got == nil || got.ID != want {
			t.Errorf("%q resolved to %v", spelling, got)
		}
	}
	// An id nobody wrote is nil rather than a raise: one bad reference costs
	// the fact, never the turn.
	for _, missing := range []string{"", "   ", "not-a-fact", "pixie"} {
		if got := TarotFactByID(missing); got != nil {
			t.Errorf("%q resolved to %q", missing, got.ID)
		}
	}
}
