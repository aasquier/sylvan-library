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
