package gate_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The two refusals that say *why* a card cannot be the commander, rather than
// only that it cannot.
//
// Both are cards that look like commanders and are not, and both are the kind
// of mistake a newcomer makes reading the card rather than the rules -- which
// is commandment 2's whole territory. "Type line is Creature — Human Soldier
// and it does not say it can be your commander" is true and useless when the
// card in your hand plainly says `Partner with`. The extra clause is the
// sentence that answers the question the player is actually asking.
//
// Every record here is synthetic: what is being read is the *shape* of a type
// line and a pairing line, and rule 1 forbids typing a real card's text from
// memory. The golden corpus beside this file is where real cards are asserted.

func deckLedBy(names ...string) *deck.Deck {
	return &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: names,
		Cards:     []deck.CardEntry{{Name: "Forest", Why: "a land"}},
	}
}

// A `Partner with` ability never waives the legendary requirement, and the
// refusal says so in the same breath -- otherwise the card's own text reads
// as a contradiction of the gate.
func TestANonlegendaryPartnerIsRefusedWithTheRulingThatExplainsIt(t *testing.T) {
	t.Parallel()
	cards := map[string]*pool.CardRecord{
		"Fixture Squire": rec("Fixture Squire", "Creature — Human Soldier",
			"Partner with Fixture Knight\nVigilance", []string{"W"}, true),
		"Forest": rec("Forest", "Basic Land — Forest", "({T}: Add {G}.)", []string{"G"}, true),
	}

	report := gate.Validate(deckLedBy("Fixture Squire"), cards, 100)
	if !hasCode(report, "not-a-commander") {
		t.Fatalf("a nonlegendary creature led a deck: %v", codes(report))
	}
	message := find(report, "not-a-commander").Message
	if !strings.Contains(message, "nonlegendary") {
		t.Errorf("the refusal reads %q, which does not answer the question the "+
			"card's own `Partner with` line raises", message)
	}
}

// A Background is legal as a *second* commander and as nothing else, so a
// deck that lists one alone is told which half is wrong.
func TestABackgroundListedAloneIsToldItNeedsTheOtherHalf(t *testing.T) {
	t.Parallel()
	cards := map[string]*pool.CardRecord{
		"Fixture Calling": rec("Fixture Calling", "Enchantment — Background",
			"Commander creatures you own get +1/+1.", []string{"G"}, true),
		"Forest": rec("Forest", "Basic Land — Forest", "({T}: Add {G}.)", []string{"G"}, true),
	}

	report := gate.Validate(deckLedBy("Fixture Calling"), cards, 100)
	if !hasCode(report, "not-a-commander") {
		t.Fatalf("a lone Background led a deck: %v", codes(report))
	}
	message := find(report, "not-a-commander").Message
	if !strings.Contains(message, "Background") {
		t.Errorf("the refusal reads %q and never says the word on the card", message)
	}

	// And beside a commander that may choose one, the same card is legal --
	// which is what makes the sentence above a diagnosis rather than a ban.
	cards["Fixture Patron"] = rec("Fixture Patron", "Legendary Creature — Elf Noble",
		"Choose a Background", []string{"G"}, true)
	paired := gate.Validate(deckLedBy("Fixture Patron", "Fixture Calling"), cards, 100)
	if hasCode(paired, "not-a-commander") {
		t.Errorf("a Background chosen by a commander that may choose one was "+
			"refused: %v", codes(paired))
	}
}
