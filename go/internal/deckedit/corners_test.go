package deckedit

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The edges of the edit engine: the values a caller can hand it that are not
// the values anybody expected, and the sentences it answers with.
//
// Two things are being held here and neither is about YAML. The first is
// **rule 4 and ADR 13 at the door they are actually enforced at**: a curated
// deck justifies every slot, so the promotion refuses while a rationale is
// blank and the refusal names the cards -- and a blank `why` cannot be
// written over a curated deck's card at all. The second is **the refusal a
// person reads**, which is commandment 2's whole substance: "quantity must be
// a whole number, not 2.5" is a sentence somebody can act on, and a stack
// trace is not.
//
// Every card name below is invented. What is being read is the shape of a
// value, never a fact about a card.

const cornerDeck = `slug: corners
name: Corners
status: theoretical
stage: draft
commander:
  - Fixture Commander
cards:
  - name: Fixture Signet
    category: ramp
    why: It makes mana.
  - name: Fixture Bear
    category: threat
    why: It attacks.
`

// ---- promotion ------------------------------------------------------------

// **Vacuously true is not true enough.** "Every card is justified" is
// trivially satisfied by a deck with no cards, so an empty deck would promote
// itself to curated -- a claim that the thinking is done about a deck that
// does not exist yet.
func TestAnEmptyDeckCannotPromoteItselfToCurated(t *testing.T) {
	t.Parallel()
	const empty = "slug: empty\nname: Empty\nstatus: theoretical\nstage: draft\ncards: []\n"
	got, err := SetDeckField(empty, "stage", "curated")
	if err == nil {
		t.Fatalf("a deck with no cards promoted itself:\n%s", got)
	}
	if !strings.Contains(err.Error(), "no cards") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

// The promotion refusal names the cards, caps the list, and counts the rest --
// the report's house habit, and the difference between a sentence somebody can
// act on and one they have to go hunting behind.
func TestThePromotionRefusalNamesSixCardsAndCountsTheRest(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("slug: blanks\nname: Blanks\nstatus: theoretical\nstage: draft\ncards:\n")
	// A card entry with no name at all, first so it is inside the six the
	// refusal shows: a hand-written file can carry one, and a refusal that
	// listed an empty string would read as a bug.
	b.WriteString("  - category: ramp\n    why: ''\n")
	for i := 0; i < 8; i++ {
		b.WriteString("  - name: Fixture Card ")
		b.WriteByte(byte('A' + i))
		b.WriteString("\n    category: ramp\n    why: ''\n")
	}

	got, err := SetDeckField(b.String(), "stage", "curated")
	if err == nil {
		t.Fatalf("a deck of blank rationales promoted itself:\n%s", got)
	}
	message := err.Error()
	if !strings.Contains(message, "9 card(s)") {
		t.Errorf("the refusal reads %q and does not count them", message)
	}
	if !strings.Contains(message, "and 3 more") {
		t.Errorf("the refusal reads %q -- it names six and counts the rest", message)
	}
	// The nameless one is shown as a placeholder rather than as a blank.
	if !strings.Contains(message, "?") {
		t.Errorf("the refusal reads %q and drops the card with no name", message)
	}
}

// A `cards:` list holding something that is not a card mapping is stepped
// over rather than crashed on -- a hand-written file can carry a bare string
// where an entry belongs, and the promotion check is not the place to
// discover it.
func TestThePromotionCheckStepsOverSomethingThatIsNotACard(t *testing.T) {
	t.Parallel()
	const odd = "slug: odd\nname: Odd\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - Fixture Bare String\n  - name: Fixture Signet\n    why: It makes mana.\n"
	if _, err := SetDeckField(odd, "stage", "curated"); err != nil {
		t.Errorf("the promotion refused a file it could read: %v", err)
	}
}

// ---- the values a caller hands in -----------------------------------------

// A theme list arrives as a comma-separated string from the CLI and as a list
// from the browser, and an empty entry in either is dropped rather than
// refused -- a trailing comma is a typo, not a decision.
func TestAThemeListDropsItsBlanksAndRefusesItsTypos(t *testing.T) {
	t.Parallel()
	out, err := SetDeckField(cornerDeck, "themes", "aggro,,  ,aggro")
	if err != nil {
		t.Fatalf("a list with blanks in it was refused: %v", err)
	}
	if strings.Count(out, "aggro") != 1 {
		t.Errorf("the themes came out as:\n%s", out)
	}

	// Nothing at all is an empty list rather than a refusal: clearing the
	// themes is a real edit.
	cleared, err := SetDeckField(cornerDeck, "themes", nil)
	if err != nil {
		t.Fatalf("clearing the themes was refused: %v", err)
	}
	if strings.Contains(cleared, "aggro") {
		t.Errorf("clearing the themes left one behind:\n%s", cleared)
	}

	// And a value that is not a list at all is still read for what it says,
	// so the refusal quotes what somebody typed rather than its shape.
	if _, err := SetDeckField(cornerDeck, "themes", 42); err == nil {
		t.Error("a number passed as a theme")
	} else if !strings.Contains(err.Error(), "42") {
		t.Errorf("the refusal reads %q and does not quote what was typed", err)
	}
}

// A quantity is a whole number, and every shape that is not one is refused in
// the same sentence -- which is the sentence a person reads.
func TestAQuantityIsAWholeNumberInEveryShapeItArrivesAs(t *testing.T) {
	t.Parallel()
	for _, value := range []any{2.5, "three", true, nil} {
		got, err := SetCardField(cornerDeck, "Fixture Signet", "qty", value)
		if err == nil {
			t.Errorf("%v was written as a quantity:\n%s", value, got)
			continue
		}
		if !strings.Contains(err.Error(), "whole number") {
			t.Errorf("%v was refused with %q", value, err)
		}
	}

	// The shapes that ARE whole numbers all land, whichever door they came
	// through: the browser sends JSON numbers, the CLI sends text.
	for _, value := range []any{2, int64(2), 2.0, " 2 "} {
		out, err := SetCardField(cornerDeck, "Fixture Signet", "qty", value)
		if err != nil {
			t.Errorf("%#v was refused: %v", value, err)
			continue
		}
		if !strings.Contains(out, "qty: 2") {
			t.Errorf("%#v was written as:\n%s", value, out)
		}
	}

	// Zero is a different refusal, because it means something: "take this
	// card out", which is a different operation with its own undo.
	if _, err := SetCardField(cornerDeck, "Fixture Signet", "qty", 0); err == nil {
		t.Error("a quantity of zero was written")
	} else if !strings.Contains(err.Error(), "remove the card instead") {
		t.Errorf("zero was refused with %q, which does not say what to do", err)
	}
}

// **A curated deck's card cannot have its reason taken away.** ADR 13: the
// promotion is refused while a rationale is blank, so blanking one afterwards
// would walk the deck back out of the state it was promoted into, without
// anything saying so.
func TestACuratedDecksRationaleCannotBeBlanked(t *testing.T) {
	t.Parallel()
	curated := strings.Replace(cornerDeck, "stage: draft", "stage: curated", 1)

	got, err := SetCardField(curated, "Fixture Signet", "why", "   ")
	if err == nil {
		t.Fatalf("a curated deck's rationale was blanked:\n%s", got)
	}
	if !strings.Contains(err.Error(), "curated") {
		t.Errorf("the refusal reads %q and does not say which rule it is", err)
	}

	// A draft's may be: that is the difference the two stages carry.
	if _, err := SetCardField(cornerDeck, "Fixture Signet", "why", ""); err != nil {
		t.Errorf("a draft's rationale could not be blanked: %v", err)
	}
}

// A `notes:` key that is not a mapping is refused rather than written over --
// somebody's prose is under there in some shape this package did not write.
func TestANoteRefusesAFileWhoseNotesAreNotAMapping(t *testing.T) {
	t.Parallel()
	const odd = "slug: odd\nname: Odd\nstatus: theoretical\nstage: draft\n" +
		"notes: just a sentence\ncards: []\n"
	got, err := SetNote(odd, "plan", "a note")
	if err == nil {
		t.Fatalf("a note was written into something that is not a mapping:\n%s", got)
	}
	if !strings.Contains(err.Error(), "notes") {
		t.Errorf("the refusal reads %q", err)
	}
}

// A combo listing a blank piece drops it rather than writing an empty name
// into the block -- the browser sends a row per input and an empty one is a
// row somebody did not fill in.
func TestACombosBlankPieceIsDroppedRatherThanWritten(t *testing.T) {
	t.Parallel()
	out, err := SetCombos(cornerDeck, []deck.Combo{{
		Cards:    []string{"Fixture Signet", "   ", "Fixture Bear"},
		Produces: "infinite mana", How: "it goes around",
	}})
	if err != nil {
		t.Fatalf("a combo with a blank piece was refused: %v", err)
	}
	if strings.Contains(out, "- ''") {
		t.Errorf("a blank piece was written into the block:\n%s", out)
	}
	if !strings.Contains(out, "Fixture Signet") || !strings.Contains(out, "Fixture Bear") {
		t.Errorf("the pieces that were there did not survive:\n%s", out)
	}
}

// A comment somebody wrote beside `shared:` survives the toggle.
//
// Every write here is surgical (ADR 12) and this is what surgical means: the
// share toggle rewrote whole files until 2026-08-22 and took hand-written
// banners, trailing comments and folded blocks with it.
func TestTheShareToggleKeepsTheCommentBesideIt(t *testing.T) {
	t.Parallel()
	const commented = "slug: corners\nname: Corners\nstatus: theoretical\nstage: draft\n" +
		"shared: true  # Aaron asked for this one to be on the shelf\ncards: []\n"
	out, err := SetShared(commented, false)
	if err != nil {
		t.Fatalf("the toggle refused: %v", err)
	}
	if !strings.Contains(out, "# Aaron asked for this one to be on the shelf") {
		t.Errorf("the toggle ate the comment beside it:\n%s", out)
	}
	if !strings.Contains(out, "shared: false") {
		t.Errorf("the toggle did not write the flag:\n%s", out)
	}
}
