package compile

// The oracle reader's edges, and the copy the closed forms take.
//
// The reader is a substring parser over prose somebody else writes, so its
// interesting inputs are the ones that look like mana and are not: a word
// with "add" inside it, a symbol with nothing in it, a number too long to be
// a number. Each one has a recorded answer, and each answer is the
// conservative one -- the floor, never a guess -- because an overcounted
// accelerant moves every land recommendation the deck gets.

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
)

func TestAWordThatMerelyContainsAddIsNotAManaAbility(t *testing.T) {
	t.Parallel()
	// The line-level scan is a substring test, so it finds "add" inside a
	// keyword in the activation cost. The reader then splits at the colon
	// and looks again in the part after it, finds nothing, and moves on --
	// which is what keeps the clause from being cut out of a string that
	// does not contain it. The card falls to the floor of one, the same
	// answer a line with no "add" at all gets.
	const saddled = "Saddle 3: Whenever this creature attacks, it gets +2/+2 until end of turn."
	if got := ManaProduced(saddled); got != 1 {
		t.Fatalf("a line whose only \"add\" is inside another word produced %d mana", got)
	}
	if got := ManaProduced("{T}: Draw a card."); got != 1 {
		t.Fatalf("drawing a card produced %d mana", got)
	}
	// And a line that really does say it, on a card carrying the same
	// keyword, is still read: the skip above is the cost's line, not the
	// card's.
	if got := ManaProduced(saddled + "\n{T}: Add {G}{G}."); got != 2 {
		t.Fatalf("a real mana line beside it produced %d mana, want 2", got)
	}
}

func TestASymbolWithNothingInItStillCountsAsOne(t *testing.T) {
	t.Parallel()
	// The recorded colour test is a substring test, and the empty string is a
	// substring of everything -- so a symbol that trims to nothing counts as
	// one pip. No real oracle text reaches it; the frozen corpus says what
	// happens when something does, and a "cleaner" set test would answer
	// differently.
	if got := ManaProduced("{T}: Add { }."); got != 1 {
		t.Fatalf("an empty symbol produced %d mana, want 1", got)
	}
	if got := manaSymbols("{ }"); got != 1 {
		t.Fatalf("manaSymbols(\"{ }\") = %d", got)
	}
	if isASCIIDigits("") {
		t.Fatal("the empty string was read as digits")
	}
}

func TestANumberTooLongToBeANumberCountsAsNothing(t *testing.T) {
	t.Parallel()
	// Twenty digits is past what an int holds. The recorded parser crashed on
	// inputs like this; answering the floor is the safer of the two, and the
	// symbol is skipped rather than guessed at.
	if got := manaSymbols("{99999999999999999999}"); got != 0 {
		t.Fatalf("an unreadable number counted as %d", got)
	}
	// The ordinary spellings still count, so the above is not passing on a
	// parser that counts nothing.
	for text, want := range map[string]int{
		"{2}": 2, "{G}": 1, "{T}": 0, "{G}{G}": 2, "{2}{U}": 3, "{G/W}": 1,
	} {
		if got := manaSymbols(text); got != want {
			t.Fatalf("manaSymbols(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestTheClosedFormsGetCopiesRatherThanTheEnginesAliases(t *testing.T) {
	t.Parallel()
	// The library repeats the same pointer once per copy, on purpose: Tier 1
	// removes a card from a hand by first-equal and identifies the commander
	// by pointer. The closed forms count and multiply instead, so they take
	// values -- and a value they change must not reach back into the deck
	// the engine is about to shuffle.
	forest := &sim.Card{Name: "Synthetic Forest", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
	spell := &sim.Card{Name: "Synthetic Spell", Cost: sim.Cost{Generic: 2, Pips: [][]string{{"G"}}}}
	r := &Report{Library: []*sim.Card{forest, forest, spell}}

	values := r.Values()
	if len(values) != 3 {
		t.Fatalf("%d values for a three-card library", len(values))
	}
	if values[0].Name != "Synthetic Forest" || values[2].Name != "Synthetic Spell" {
		t.Fatalf("the library came back in another order: %+v", values)
	}
	values[0].Name = "Changed"
	if forest.Name != "Synthetic Forest" {
		t.Fatal("a closed form's copy reached back into the engine's card")
	}
	if r.SimulatedSize() != 3 {
		t.Fatalf("simulated size %d", r.SimulatedSize())
	}
}
