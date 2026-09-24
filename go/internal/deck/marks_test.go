package deck

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
)

// Two things about what the deck file does and does not carry.
//
// **A drafted rationale wears its mark and nothing else does** (ADR 41).
// `why_by: claude` says a model wrote this sentence; the moment a person
// writes over it the sentence is theirs and the mark goes. So the dump writes
// the key only where there is one -- a deck nobody has run the intake over
// says nothing about authorship rather than saying "not claude" ninety-nine
// times, which would be ninety-nine lines of noise in a file people read.
//
// **And a value the file cannot hold is refused by position**, so the
// operator is told which entry was the problem rather than that something,
// somewhere, would not serialise.

func aDeck(cards ...CardEntry) *Deck {
	return &Deck{
		Slug: "marks", Name: "Marks", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Cards: cards,
	}
}

// The mark is written where there is one and nowhere else.
func TestOnlyADraftedRationaleWearsItsMark(t *testing.T) {
	t.Parallel()
	text, err := aDeck(
		CardEntry{Name: "Fixture Signet", Category: "ramp", Qty: 1,
			Why: "It makes mana.", WhyBy: "claude"},
		CardEntry{Name: "Fixture Bear", Category: "threat", Qty: 1,
			Why: "A person wrote this one."},
	).Dump()
	if err != nil {
		t.Fatalf("the dump refused: %v", err)
	}
	if strings.Count(text, "why_by") != 1 {
		t.Errorf("the file carries %d marks:\n%s", strings.Count(text, "why_by"), text)
	}
	signet := text[strings.Index(text, "Fixture Signet"):]
	if !strings.Contains(signet[:strings.Index(signet, "Fixture Bear")], "why_by") {
		t.Errorf("the drafted card lost its mark:\n%s", text)
	}
}

// The mark travels the comparison as well as the file.
//
// `Payload` is the mapping the dump serialises, without the serialising, and
// the build's baseline check asks whether two of them are equal. A mark that
// reached the file and not the payload would make a deck whose rationales
// were just drafted compare equal to the deck before the intake ran -- and
// the artifacts would be reported up to date over a deck that changed.
func TestTheMarkTravelsTheComparisonAsWellAsTheFile(t *testing.T) {
	t.Parallel()
	drafted := aDeck(CardEntry{Name: "Fixture Signet", Category: "ramp", Qty: 1,
		Why: "It makes mana.", WhyBy: "claude"})
	byHand := aDeck(CardEntry{Name: "Fixture Signet", Category: "ramp", Qty: 1,
		Why: "It makes mana."})

	cards, ok := drafted.Payload()["cards"].([]map[string]any)
	if !ok || len(cards) != 1 {
		t.Fatalf("the payload's cards are %#v", drafted.Payload()["cards"])
	}
	if cards[0]["why_by"] != "claude" {
		t.Errorf("the payload's card is %#v and carries no mark", cards[0])
	}

	plain, ok := byHand.Payload()["cards"].([]map[string]any)
	if !ok || len(plain) != 1 {
		t.Fatalf("the payload's cards are %#v", byHand.Payload()["cards"])
	}
	if _, marked := plain[0]["why_by"]; marked {
		t.Errorf("a rationale a person wrote carries a mark: %#v", plain[0])
	}
}

// A note holding something the file cannot carry is refused, and the refusal
// says **which** entry -- a deck's notes are a person's prose and a failure
// that named nothing would leave them hunting through it.
func TestAValueTheFileCannotHoldIsRefusedByPosition(t *testing.T) {
	t.Parallel()
	d := aDeck(CardEntry{Name: "Fixture Signet", Category: "ramp", Qty: 1, Why: "It makes mana."})
	d.Notes = deckyaml.Map{{Key: "plan", Value: []any{
		"a line that is fine",
		make(chan int), // nothing a YAML file can hold
	}}}

	got, err := d.Dump()
	if err == nil {
		t.Fatalf("a value the file cannot hold was written:\n%s", got)
	}
	if got != "" {
		t.Errorf("%d bytes came back alongside the refusal", len(got))
	}
	if !strings.Contains(err.Error(), "[1]") {
		t.Errorf("the refusal is %q and does not say which entry", err)
	}
	if !strings.Contains(err.Error(), "plan") {
		t.Errorf("the refusal is %q and does not name the note", err)
	}
}
