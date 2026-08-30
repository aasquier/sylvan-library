package deckedit

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// A deck with a graveyard, so the block's placement has something to be after.
const uncatalogued = `slug: fixture
name: Fixture
commander:
  - Arcades, the Strategist
cards:
  - name: Axebane Guardian
    category: ramp
    why: Taps for one per defender.
  - name: High Alert
    category: engine
    why: Turns the walls sideways.
  - name: Suspicious Bookcase
    category: utility
    why: A body that blocks.
  - name: Forest
    category: land
    qty: 30
    why: Green sources.

graveyard:
  - name: Wall of Omens
    category: draw
    why: Left the deck for a better wall.
`

func machine() deck.Combo {
	return deck.Combo{
		Cards:    []string{"Axebane Guardian", "High Alert"},
		Produces: "infinite colored mana; infinite untaps of your creatures",
		How: "1) Tap Axebane Guardian for X, where X is your defender count. " +
			"2) Pay {2}{W}{U} to High Alert to untap it. 3) Each crank costs four and makes X.",
		Setup: "six mana across two quiet turns, then the fifth defender",
	}
}

func nearMiss() deck.Combo {
	return deck.Combo{
		Cards:    []string{"Axebane Guardian"},
		Needs:    "Umbral Mantle",
		Cut:      "Suspicious Bookcase",
		Produces: "infinite colored mana",
		How:      "1) Equip. 2) Untap for {3}. 3) Repeat while the defenders hold.",
	}
}

// The block a deck has never had is written, and it lands where `deck.Dump`
// puts it: after everything the deck is made of, so cataloguing the first
// machine appends rather than pushing the 99 down the file.
func TestTheFirstComboOpensTheBlockBelowEverythingElse(t *testing.T) {
	t.Parallel()
	got, err := SetCombos(uncatalogued, []deck.Combo{machine()})
	if err != nil {
		t.Fatalf("cataloguing a combo: %v", err)
	}
	at := strings.Index(got, "\ncombos:")
	if at < 0 {
		t.Fatalf("no combos block was written:\n%s", got)
	}
	if grave := strings.Index(got, "\ngraveyard:"); grave < 0 || grave > at {
		t.Errorf("the block is not below the graveyard:\n%s", got)
	}
	if !strings.Contains(got, "\n\ncombos:") {
		t.Errorf("the block has no blank line above it:\n%s", got)
	}
	// The pieces are a list, and there is no `name:` line anywhere in the
	// entry -- the heading is the cards, and a title would be a second thing
	// to keep true.
	for _, want := range []string{
		"  - cards:\n      - Axebane Guardian\n      - High Alert",
		"produces:", "how:", "setup:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the entry is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got[at:], "name:") {
		t.Errorf("a combo grew a name field:\n%s", got[at:])
	}
	// And nothing above it moved: the 99, its quantities and its rationales
	// are the bytes they were, down to the blank line before the graveyard.
	// Only the separator the block brought with it is new, which is why the
	// trailing newlines are trimmed off both sides rather than counted.
	if head := strings.TrimRight(got[:at], "\n"); head != strings.TrimRight(uncatalogued, "\n") {
		t.Errorf("the rest of the file changed:\n--- was ---\n%s\n--- now ---\n%s",
			uncatalogued, head)
	}
}

// The whole block is the unit of the edit, so a second entry joins the first
// rather than the deck growing a second `combos:` key.
func TestASecondComboJoinsTheBlockRatherThanOpeningAnother(t *testing.T) {
	t.Parallel()
	first, err := SetCombos(uncatalogued, []deck.Combo{machine()})
	if err != nil {
		t.Fatalf("cataloguing: %v", err)
	}
	got, err := SetCombos(first, []deck.Combo{machine(), nearMiss()})
	if err != nil {
		t.Fatalf("cataloguing a second: %v", err)
	}
	if n := strings.Count(got, "\ncombos:"); n != 1 {
		t.Fatalf("the deck has %d combos blocks, not 1:\n%s", n, got)
	}
	if n := strings.Count(got, "  - cards:"); n != 2 {
		t.Fatalf("the block holds %d entries, not 2:\n%s", n, got)
	}
	if !strings.Contains(got, "needs: Umbral Mantle") ||
		!strings.Contains(got, "cut: Suspicious Bookcase") {
		t.Errorf("the near-miss lost its trade:\n%s", got)
	}
}

// A round trip through the model: what the editor wrote is what the parser
// reads back, field for field. The two halves are written independently, and
// this is the only thing that holds them equal.
func TestACataloguedBlockReadsBackAsItWasWritten(t *testing.T) {
	t.Parallel()
	want := []deck.Combo{machine(), nearMiss()}
	text, err := SetCombos(uncatalogued, want)
	if err != nil {
		t.Fatalf("cataloguing: %v", err)
	}
	d, err := deck.FromText(text, "fixture")
	if err != nil {
		t.Fatalf("re-reading the deck: %v", err)
	}
	if len(d.Combos) != len(want) {
		t.Fatalf("read back %d combos, wrote %d", len(d.Combos), len(want))
	}
	for i, got := range d.Combos {
		if got.Heading() != want[i].Heading() {
			t.Errorf("combo %d is headed %q, wrote %q", i, got.Heading(), want[i].Heading())
		}
		for _, field := range []struct{ what, got, want string }{
			{"produces", got.Produces, want[i].Produces},
			{"how", got.How, want[i].How},
			{"setup", got.Setup, want[i].Setup},
			{"needs", got.Needs, want[i].Needs},
			{"cut", got.Cut, want[i].Cut},
		} {
			if field.got != field.want {
				t.Errorf("combo %d %s is %q, wrote %q", i, field.what, field.got, field.want)
			}
		}
	}
	// The near-miss is the second entry and knows it is one.
	if !d.Combos[1].NearMiss() || d.Combos[0].NearMiss() {
		t.Errorf("the near-miss is not the one that is a card short: %+v", d.Combos)
	}
}

// Emptying the shelf removes the block rather than writing `combos: []`, and
// takes the blank line that led to it -- otherwise a deck that has emptied
// twice keeps a widening gap at the foot of its file.
func TestRemovingTheLastComboTakesTheBlockWithIt(t *testing.T) {
	t.Parallel()
	full, err := SetCombos(uncatalogued, []deck.Combo{machine()})
	if err != nil {
		t.Fatalf("cataloguing: %v", err)
	}
	got, err := SetCombos(full, nil)
	if err != nil {
		t.Fatalf("emptying: %v", err)
	}
	if strings.Contains(got, "combos:") {
		t.Errorf("the emptied block is still asserted:\n%s", got)
	}
	if got != uncatalogued {
		t.Errorf("emptying did not put the file back:\n--- was ---\n%s\n--- now ---\n%s",
			uncatalogued, got)
	}
	// And doing it again is not an error. A client that removed the last entry
	// twice has asked for a state the deck is already in.
	again, err := SetCombos(got, nil)
	if err != nil {
		t.Fatalf("emptying an empty shelf: %v", err)
	}
	if again != uncatalogued {
		t.Errorf("the second emptying changed the file:\n%s", again)
	}
}

// **The mark is the deck file's, never the caller's.** A client that sends
// `by: claude` on an entry it composed does not get to say Claude wrote it.
func TestAMarkSentByTheCallerIsDiscarded(t *testing.T) {
	t.Parallel()
	forged := machine()
	forged.By = "claude"
	got, err := SetCombos(uncatalogued, []deck.Combo{forged})
	if err != nil {
		t.Fatalf("cataloguing: %v", err)
	}
	if strings.Contains(got, "by:") {
		t.Errorf("a caller forged a provenance mark:\n%s", got)
	}
}

// A deck file that already carries a mark keeps it while the entry says the
// same thing -- and loses it the instant a person changes a word, which is
// what adopting a draft means (ADR 41's rule, one block over).
func TestAMarkSurvivesUntilSomebodyEditsWhatItMarks(t *testing.T) {
	t.Parallel()
	drafted := uncatalogued + `
combos:
  - cards:
      - Axebane Guardian
      - High Alert
    produces: infinite colored mana
    how: 1) Tap. 2) Untap. 3) Again.
    by: claude
`
	unchanged := deck.Combo{
		Cards:    []string{"Axebane Guardian", "High Alert"},
		Produces: "infinite colored mana",
		How:      "1) Tap. 2) Untap. 3) Again.",
	}
	kept, err := SetCombos(drafted, []deck.Combo{unchanged})
	if err != nil {
		t.Fatalf("rewriting an unchanged block: %v", err)
	}
	if !strings.Contains(kept, "by: claude") {
		t.Errorf("re-saving an unchanged entry dropped its mark:\n%s", kept)
	}

	edited := unchanged
	edited.Setup = "six mana and a fifth defender"
	adopted, err := SetCombos(drafted, []deck.Combo{edited})
	if err != nil {
		t.Fatalf("editing a drafted entry: %v", err)
	}
	if strings.Contains(adopted, "by:") {
		t.Errorf("an edited entry kept the mark that said somebody else wrote it:\n%s", adopted)
	}
}

// Reordering is not editing. A person who drags a drafted combo above another
// one has adopted neither, so the mark travels with the entry rather than with
// its position.
func TestAMarkTravelsWithItsEntryRatherThanItsPosition(t *testing.T) {
	t.Parallel()
	drafted := uncatalogued + `
combos:
  - cards:
      - Suspicious Bookcase
    produces: a wall that moves
    how: 1) Block. 2) Look pleased.
  - cards:
      - Axebane Guardian
      - High Alert
    produces: infinite colored mana
    how: 1) Tap. 2) Untap. 3) Again.
    by: claude
`
	d, err := deck.FromText(drafted, "fixture")
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	// The marked entry moves to the front, unchanged. Its own `By` is cleared
	// first, because that is exactly what the wire hands a route back.
	swapped := []deck.Combo{d.Combos[1], d.Combos[0]}
	swapped[0].By = ""
	got, err := SetCombos(drafted, swapped)
	if err != nil {
		t.Fatalf("reordering: %v", err)
	}
	after, err := deck.FromText(got, "fixture")
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if after.Combos[0].By != "claude" {
		t.Errorf("the mark did not follow its entry to the front: %+v", after.Combos)
	}
	if after.Combos[1].By != "" {
		t.Errorf("the mark landed on an entry nobody drafted: %+v", after.Combos)
	}
}

// The refusals, each about the entry being readable rather than about the
// Magic being right -- which is not a question this can answer and does not
// pretend to.
func TestAnUnreadableEntryIsRefusedWithItsOwnSentence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		combo deck.Combo
		says  string
	}{
		{"no pieces", deck.Combo{Produces: "infinite mana"}, "at least one piece"},
		{"nothing produced", deck.Combo{Cards: []string{"Sol Ring"}}, "say what it produces"},
		{"a piece listed twice", deck.Combo{
			Cards: []string{"Sol Ring", "sol ring"}, Produces: "mana"}, "listed twice"},
		{"a cut with nothing to bring in", deck.Combo{
			Cards: []string{"Sol Ring"}, Produces: "mana", Cut: "Forest"},
			"not missing anything"},
		{"a piece it already has", deck.Combo{
			Cards: []string{"Sol Ring"}, Produces: "mana", Needs: "Sol Ring"},
			"both a piece this deck has"},
		{"a card cut for itself", deck.Combo{
			Cards: []string{"Sol Ring"}, Produces: "mana",
			Needs: "Basalt Monolith", Cut: "basalt monolith"}, "come in for itself"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := SetCombos(uncatalogued, []deck.Combo{tc.combo})
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if !IsFailed(err) {
				t.Errorf("%s is a bug rather than a refusal: %v", tc.name, err)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the refusal does not say %q: %v", tc.says, err)
			}
			// The entry is named by its place in the list, because it has no
			// name to be named by.
			if !strings.Contains(err.Error(), "combo 1:") {
				t.Errorf("the refusal does not say which entry: %v", err)
			}
		})
	}
}

// A whole-block PUT is the one shape here a caller could hand an unbounded
// list, so it is bounded.
func TestAnUnboundedListIsRefused(t *testing.T) {
	t.Parallel()
	many := make([]deck.Combo, MaxCombos+1)
	for i := range many {
		many[i] = machine()
	}
	_, err := SetCombos(uncatalogued, many)
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("a list past the cap was accepted: %v", err)
	}
}

// The verification is what makes a whole-block rewrite safe: an edit that
// reached anything but `combos:` fails rather than being written. Driven
// through a file the editor cannot read at all, which is the reachable half of
// that promise.
func TestABlockCannotBeWrittenIntoAFileTheEditorCannotRead(t *testing.T) {
	t.Parallel()
	_, err := SetCombos("- this is a list, not a deck\n", []deck.Combo{machine()})
	if err == nil {
		t.Fatal("a non-mapping file was edited")
	}
	if !IsFailed(err) {
		t.Errorf("that is a refusal, not a bug: %v", err)
	}
}

// Prose keeps its own punctuation through the emitter, including the colon
// that would end a line early if anything wrote this by hand.
func TestProseWithAColonSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()
	awkward := deck.Combo{
		Cards:    []string{"Sol Ring"},
		Produces: "mana: two of it, every turn",
		How:      "1) Tap it. 2) Note that {C}{C} is not {2}: it is colourless.",
		Setup:    "one turn, and #1 on the curve",
	}
	text, err := SetCombos(uncatalogued, []deck.Combo{awkward})
	if err != nil {
		t.Fatalf("cataloguing awkward prose: %v", err)
	}
	d, err := deck.FromText(text, "fixture")
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	got := d.Combos[0]
	if got.Produces != awkward.Produces || got.How != awkward.How || got.Setup != awkward.Setup {
		t.Errorf("prose did not survive the round trip:\n%+v\n%s", got, text)
	}
}
