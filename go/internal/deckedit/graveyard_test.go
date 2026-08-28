package deckedit

import (
	"strings"
	"testing"
)

// The graveyard's two edges, driven directly rather than through the frozen
// chain in `testdata/edits.json`.
//
// A buried card is the one state where the same name legitimately exists
// twice in a deck file's world -- once as a card that was, once as a card
// that could be again -- and every operation that touches it has to answer
// **which one you meant**. The oracle chains bury-then-return and proves the
// bytes; what it does not reach is the pair of answers given to somebody who
// tries to work around the graveyard instead of through it.
//
// Both are refusals a person reads, and both have to say what to do instead:
// commandment 2 is not satisfied by "no".

// buried is a deck with one card in the graveyard and one in the 99, written
// the way the emitter writes them.
const buried = `slug: fixture
name: Fixture
status: theoretical
stage: draft
commander:
  - Fixture Commander
cards:
  - name: Fixture Ramp
    category: ramp
    why: Mana, early.
graveyard:
  - name: Fixture Buried
    category: removal
    why: Cut for speed, kept for the note.
`

// Adding a card that is already in the graveyard is refused **as a graveyard
// card**, with its own sentence -- not with the "already in the 99" one.
//
// The distinction is the whole point. "Already in the deck" tells somebody to
// change a quantity, which is not possible here and would send them looking
// for a card that is not in the list they are staring at. The graveyard
// sentence names the two operations that actually exist.
func TestAddingACardThatIsBuriedSaysToReturnOrExileIt(t *testing.T) {
	t.Parallel()
	for _, listKey := range append([]string{}, CardLists...) {
		_, err := AddCard(buried, "Fixture Buried", "ramp", "Wanted after all.", 1, listKey)
		if err == nil {
			t.Errorf("adding a buried card to %s was allowed, which leaves it in "+
				"two places with two rationales", listKey)
			continue
		}
		if !strings.Contains(err.Error(), "graveyard") {
			t.Errorf("%s: the refusal does not mention the graveyard: %v", listKey, err)
		}
		for _, verb := range []string{"return", "exile"} {
			if !strings.Contains(err.Error(), verb) {
				t.Errorf("%s: the refusal does not offer %q: %v", listKey, verb, err)
			}
		}
		if strings.Contains(err.Error(), "change its quantity") {
			t.Errorf("%s: a buried card got the already-in-the-deck sentence, which "+
				"tells the reader to edit a row that is not there: %v", listKey, err)
		}
		if !strings.Contains(err.Error(), "Fixture Buried") {
			t.Errorf("%s: the refusal does not name the card: %v", listKey, err)
		}
	}
	// The name is matched the way the deck matches names, so a different
	// capitalisation is the same card and gets the same answer.
	if _, err := AddCard(buried, "fixture buried", "ramp", "Wanted.", 1, "cards"); err == nil ||
		!strings.Contains(err.Error(), "graveyard") {
		t.Errorf("a buried card in another case was not recognised: %v", err)
	}
	// ...and a card that is genuinely in the 99 still gets the other sentence,
	// so this is a distinction rather than one message for both.
	_, err := AddCard(buried, "Fixture Ramp", "ramp", "Again.", 1, "cards")
	if err == nil || !strings.Contains(err.Error(), "change its quantity") {
		t.Errorf("a card already in the 99 did not get its own sentence: %v", err)
	}
}

// TestAReturnInventsNothingEvenForACategorylessCard is the shape of a return,
// asked of the one entry the placement code has to guess about.
//
// `utility` is where a categoryless card **lands**, not a field written onto
// it: the entry returns exactly as it left, which is what makes a return
// rule-4-safe -- restoring the user's own `why` invents nothing, and neither
// does restoring their own (absent) category. A return that filled the gap in
// would be the app writing a field on the user's behalf on the one path that
// exists precisely because it does not.
func TestAReturnInventsNothingEvenForACategorylessCard(t *testing.T) {
	t.Parallel()
	const noCategory = `slug: fixture
name: Fixture
status: theoretical
stage: draft
commander:
  - Fixture Commander
cards:
  - name: Fixture Ramp
    category: ramp
    why: Mana, early.
graveyard:
  - name: Fixture Buried
    why: Hand-edited, and the category went with it.
`
	out, err := ReturnCard(noCategory, "Fixture Buried")
	if err != nil {
		t.Fatalf("returning a card with no category: %v", err)
	}
	if !strings.Contains(out, "Fixture Buried") {
		t.Fatalf("the card did not come back:\n%s", out)
	}
	// The user's own words came back with it, and nothing else did.
	if !strings.Contains(out, "Hand-edited, and the category went with it.") {
		t.Errorf("the rationale did not survive the return:\n%s", out)
	}
	if strings.Contains(out, "category: utility") {
		t.Errorf("the return wrote a category the user never chose; `utility` is "+
			"where it files, not what it writes:\n%s", out)
	}
	// It landed in the 99, and the graveyard section went with the last card
	// in it rather than being left behind empty.
	body := out[strings.Index(out, "cards:"):]
	if !strings.Contains(body, "Fixture Buried") {
		t.Errorf("the card is not in the 99:\n%s", out)
	}
	if strings.Contains(out, "graveyard:") && strings.Contains(
		out[strings.Index(out, "graveyard:"):], "Fixture Buried") {
		t.Errorf("the card is in both places:\n%s", out)
	}
	// And a buried card that DOES carry a category keeps it, so the branch
	// above is a fallback rather than the only path.
	kept, err := ReturnCard(buried, "Fixture Buried")
	if err != nil {
		t.Fatalf("returning a card with a category: %v", err)
	}
	if !strings.Contains(kept, "category: removal") {
		t.Errorf("the buried card's own category did not come back:\n%s", kept)
	}
}

// Returning a card that is not buried refuses by name and says where it
// looked -- "not found" alone would leave somebody wondering whether they
// spelled it wrong or buried something else.
func TestAReturnOfACardThatIsNotBuriedSaysSoByName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"Fixture Ramp",      // in the deck, not the graveyard
		"Fixture Commander", // the commander
		"Nothing Like This", // nowhere at all
	} {
		_, err := ReturnCard(buried, name)
		if err == nil {
			t.Errorf("%q was returned from a graveyard it is not in", name)
			continue
		}
		if !strings.Contains(err.Error(), "graveyard") || !strings.Contains(err.Error(), name) {
			t.Errorf("%q refused with %v, want the name and the graveyard", name, err)
		}
	}
}

// A deck with no graveyard section at all -- which is most of them -- refuses
// a return the same way rather than raising something structural at somebody
// who simply typed the wrong card.
func TestAReturnFromADeckWithNoGraveyardIsTheSameRefusal(t *testing.T) {
	t.Parallel()
	const plain = `slug: fixture
name: Fixture
status: theoretical
stage: draft
commander:
  - Fixture Commander
cards:
  - name: Fixture Ramp
    category: ramp
    why: Mana, early.
`
	_, err := ReturnCard(plain, "Fixture Ramp")
	if err == nil {
		t.Fatal("a deck with no graveyard returned a card from it")
	}
	if !strings.Contains(err.Error(), "not in the graveyard") {
		t.Errorf("the refusal reads %v", err)
	}
}
