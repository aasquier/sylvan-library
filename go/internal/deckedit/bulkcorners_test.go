package deckedit

import (
	"strings"
	"testing"
)

// What the bulk plan does with a paste, and a deck, that are not tidy.
//
// The plan is the half of the operation anybody agrees to: it is shown on
// screen, somebody reads it, and only then is anything written. So every
// oddity has to survive the reading rather than the fold -- a paste with a
// blank line in it, a list that names the same card twice, a deck file a hand
// has been in. What none of them may do is put a card in the wrong column,
// because **`Entomb` is the column somebody agrees to** and a card that landed
// there by accident is a card that leaves the 99.

// A card that sits outside the 99 is left where it is, named, with the reason
// it was left -- never added to the deck proper and never buried.
func TestAPasteNamingTheCompanionLeavesItInTheCommandZone(t *testing.T) {
	t.Parallel()
	const withCompanion = `slug: companion
name: Companion
status: theoretical
stage: draft
commander:
  - Fixture Commander
companion: Fixture Companion
cards:
  - name: Fixture Signet
    category: ramp
    why: It makes mana.
`
	p := plan(t, withCompanion,
		BulkCard{Name: "Fixture Signet", Qty: 1},
		BulkCard{Name: "Fixture Companion", Qty: 1},
		BulkCard{Name: "Fixture Commander", Qty: 1})

	left := map[string]string{}
	for _, l := range p.Left {
		left[l.Name] = l.Reason
	}
	if !strings.Contains(left["Fixture Companion"], "companion") {
		t.Errorf("the companion was left with %q", left["Fixture Companion"])
	}
	if !strings.Contains(left["Fixture Commander"], "command zone") {
		t.Errorf("the commander was left with %q", left["Fixture Commander"])
	}
	// And neither reappears anywhere a write would act on.
	for _, add := range p.Add {
		if add.Name == "Fixture Companion" || add.Name == "Fixture Commander" {
			t.Errorf("%s was queued to be added to the 99", add.Name)
		}
	}
	if len(p.Entomb) != 0 {
		t.Errorf("the paste buried %v", p.Entomb)
	}
}

// A deck file a hand has been in -- a bare string where a card entry belongs,
// the same card written twice, a quantity that is not a number -- is read for
// what can be read and nothing is invented from the rest.
func TestThePlanReadsADeckFileAHandHasBeenIn(t *testing.T) {
	t.Parallel()
	const messy = `slug: messy
name: Messy
status: theoretical
stage: draft
commander:
  - Fixture Commander
cards:
  - name: Fixture Signet
    category: ramp
    why: It makes mana.
    qty: plenty
  - name: Fixture Signet
    category: ramp
    why: A second line for the same card.
  - name: ''
    category: ramp
    why: A card with no name.
`
	p := plan(t, messy, BulkCard{Name: "Fixture Signet", Qty: 1, Why: "It makes mana."})

	// The unreadable quantity reads as one -- the deck files write `qty:`
	// only when it is not 1, so one is what absence and nonsense both mean.
	// Asking for one of it is therefore no change at all.
	if len(p.Requantify) != 0 {
		t.Errorf("an unreadable quantity was read as %v", p.Requantify)
	}
	if len(p.Unchanged) != 1 || p.Unchanged[0] != "Fixture Signet" {
		t.Errorf("the card the paste matched came back as %v", p.Unchanged)
	}
	// The second copy and the nameless entry are not cards the plan can act
	// on, so nothing is queued to bury them: burying a card nobody can name
	// is how a hand-written file loses a line.
	if len(p.Entomb) != 0 {
		t.Errorf("the plan buried %v", p.Entomb)
	}
}

// A paste that names the same card on two lines folds it into one, adds the
// quantities, and **takes the first reason somebody wrote** -- choosing
// between two reasons would be composing one, which no surface does (ADR 12
// rule 3). A line with no reason at all is not a choice, so the other line's
// reason stands.
func TestTwoLinesForOneCardFoldWithoutComposingAReason(t *testing.T) {
	t.Parallel()
	p := plan(t, cornerDeck,
		BulkCard{Name: "Fixture Signet", Qty: 1},
		BulkCard{Name: "Fixture Bear", Qty: 1},
		// The new card arrives twice: once bare, once with a reason.
		BulkCard{Name: "Fixture Wastes", Qty: 2, Category: "land"},
		BulkCard{Name: "Fixture Wastes", Qty: 3, Category: "land", Why: "The reason on the second line."},
		// And a blank line in the paste is not a card.
		BulkCard{Name: "   ", Qty: 1},
	)

	if len(p.Add) != 1 {
		t.Fatalf("the paste added %v", names(p.Add))
	}
	added := p.Add[0]
	if added.Name != "Fixture Wastes" {
		t.Fatalf("the added card is %q", added.Name)
	}
	if added.Qty != 5 {
		t.Errorf("two lines of 2 and 3 folded to a quantity of %d", added.Qty)
	}
	if added.Why != "The reason on the second line." {
		t.Errorf("the folded reason is %q -- a blank first line is not a "+
			"choice to leave it blank", added.Why)
	}
	if len(p.Merged) != 1 || p.Merged[0] != "Fixture Wastes" {
		t.Errorf("the fold was reported as %v -- somebody has to be told it "+
			"happened", p.Merged)
	}
}
