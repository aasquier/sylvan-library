package deckedit

import (
	"strings"
	"testing"
)

// A deck with no board at all, and no graveyard either -- the shape of most
// of the library.
const boardless = `slug: fixture
name: Fixture
commander:
  - Goreclaw, Terror of Qal Sisma
cards:
  - name: Sol Ring
    category: ramp
    why: Fast mana.
  - name: Forest
    category: land
    qty: 30
    why: Green sources.
`

// **The board can be started, which is the whole point of this file.** Before
// 2026-08-29 a deck with no `swap_board:` block had nowhere to put a card it
// was weighing, and no surface could give it one.
func TestABoardCanBeStartedOnADeckThatHasNone(t *testing.T) {
	t.Parallel()
	got, err := AddToBoard(boardless, "Beast Within", "interaction", "Catch-all answer.", 1)
	if err != nil {
		t.Fatalf("starting a board: %v", err)
	}

	at := strings.Index(got, "swap_board:")
	if at < 0 {
		t.Fatalf("no board was opened:\n%s", got)
	}
	if !strings.Contains(got[at:], "Beast Within") {
		t.Errorf("the card did not land on the board:\n%s", got)
	}
	// And it is not in the 99, which is the half that matters: a card
	// silently in the deck is one its owner does not know is in the deck.
	if strings.Contains(got[:at], "Beast Within") {
		t.Errorf("the card landed in the 99 as well:\n%s", got)
	}
	// The 99 is untouched -- every card, its quantity and its rationale.
	for _, want := range []string{"Sol Ring", "qty: 30", "Green sources."} {
		if !strings.Contains(got[:at], want) {
			t.Errorf("the 99 lost %q:\n%s", want, got)
		}
	}
	// The board is a top-level key, not something nested under the last card.
	if !strings.Contains(got, "\n\nswap_board:") {
		t.Errorf("the board is not laid out as its own block:\n%s", got)
	}
}

// **A second card goes onto the board that is now there**, rather than the
// deck growing a second `swap_board:` block. This is the case the composition
// exists for: one call site, whether or not there is a board yet.
func TestASecondCardJoinsTheBoardRatherThanOpeningAnother(t *testing.T) {
	t.Parallel()
	first, err := AddToBoard(boardless, "Beast Within", "interaction", "Catch-all answer.", 1)
	if err != nil {
		t.Fatalf("starting a board: %v", err)
	}
	got, err := AddToBoard(first, "Heroic Intervention", "protection", "Answer to a wrath.", 1)
	if err != nil {
		t.Fatalf("adding to the board: %v", err)
	}
	if n := strings.Count(got, "\nswap_board:"); n != 1 {
		t.Fatalf("the deck has %d board blocks, not 1:\n%s", n, got)
	}
	at := strings.Index(got, "swap_board:")
	for _, want := range []string{"Beast Within", "Heroic Intervention"} {
		if !strings.Contains(got[at:], want) {
			t.Errorf("%q is not on the board:\n%s", want, got)
		}
	}
}

// **The board goes above the graveyard, not below it.** Anchoring on the end
// of the file rather than on the end of the 99 would have written a board
// underneath a deck's buried cards -- legal YAML, and unlike every deck in the
// library.
func TestTheBoardIsWrittenAboveTheGraveyard(t *testing.T) {
	t.Parallel()
	buried, err := EntombCard(boardless, "Sol Ring")
	if err != nil {
		t.Fatalf("burying a card to make a graveyard: %v", err)
	}
	if !strings.Contains(buried, "graveyard:") {
		t.Fatalf("the fixture did not get a graveyard:\n%s", buried)
	}

	got, err := AddToBoard(buried, "Beast Within", "interaction", "Catch-all answer.", 1)
	if err != nil {
		t.Fatalf("starting a board on a deck with a graveyard: %v", err)
	}
	board, grave := strings.Index(got, "swap_board:"), strings.Index(got, "graveyard:")
	if board < 0 || grave < 0 {
		t.Fatalf("board at %d, graveyard at %d:\n%s", board, grave, got)
	}
	if board > grave {
		t.Errorf("the board was written below the graveyard:\n%s", got)
	}
	// The buried card is still buried, and still carries its own rationale.
	if !strings.Contains(got[grave:], "Sol Ring") || !strings.Contains(got[grave:], "Fast mana.") {
		t.Errorf("the graveyard was damaged:\n%s", got)
	}
}

// **Everything AddCard refuses is still refused**, in the same words. Starting
// a board is a shape change; it is not a licence to skip the rules about what
// may go on one. Rule 4 is the one worth naming: the board carries no
// obligation to be finished, but a card put there was still chosen, and an
// empty `why` on a curated deck is refused rather than invented.
func TestStartingABoardRefusesEverythingAddingACardRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, card, category, why string
		qty                       int
		want                      string
	}{
		{"a blank why on a curated deck", "Beast Within", "interaction", "  ", 1,
			"needs a `why`"},
		{"no name", "", "interaction", "A reason.", 1, "needs a name"},
		{"no category", "Beast Within", "", "A reason.", 1, "needs a category"},
		{"a quantity below one", "Beast Within", "interaction", "A reason.", 0,
			"at least 1"},
		{"a card already in the 99", "Sol Ring", "ramp", "A reason.", 1,
			"already in the deck"},
		{"the commander", "Goreclaw, Terror of Qal Sisma", "threat", "A reason.", 1,
			"the commander"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := AddToBoard(boardless, tc.card, tc.category, tc.why, tc.qty)
			if err == nil {
				t.Fatalf("it was accepted:\n%s", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal said %q, which does not carry %q", err, tc.want)
			}
			// **A refused edit has changed nothing** -- including the shape.
			// A deck that was told no must not be left holding an empty board
			// it never asked for.
			if got != "" {
				t.Errorf("a refusal handed back text:\n%s", got)
			}
		})
	}
}

// **`AddCard` itself has not moved.** The corpus records its refusal on nine
// boardless fixture decks, and the whole reason the shape change is a separate
// operation is that the refusal is still right: a mistyped `to` must never
// grow a deck a section nobody asked for.
func TestAddCardStillRefusesToInventABoard(t *testing.T) {
	t.Parallel()
	got, err := AddCard(boardless, "Beast Within", "interaction", "A reason.", 1, swapBoard)
	if err == nil {
		t.Fatalf("AddCard scaffolded a board:\n%s", got)
	}
	if !strings.Contains(err.Error(), "no `swap_board:` block") {
		t.Errorf("the refusal said %q", err)
	}
}

// A draft owes its rationales rather than being refused them (ADR 13), and the
// board is reached by the same rule the 99 is.
func TestADraftMayStartABoardWithoutARationale(t *testing.T) {
	t.Parallel()
	draft, err := SetDeckField(boardless, "stage", "draft")
	if err != nil {
		t.Fatalf("making the fixture a draft: %v", err)
	}
	got, err := AddToBoard(draft, "Beast Within", "interaction", "", 1)
	if err != nil {
		t.Fatalf("a draft was refused a blank why on the board: %v", err)
	}
	at := strings.Index(got, "swap_board:")
	if at < 0 || !strings.Contains(got[at:], "Beast Within") {
		t.Errorf("the card did not land on the board:\n%s", got)
	}
}

// The shape change on its own refuses a deck that already has a board, rather
// than writing a second one. `AddToBoard` never reaches this -- it asks first
// -- but the operation is the thing that must not be able to do it.
func TestOpeningABoardTwiceIsRefused(t *testing.T) {
	t.Parallel()
	opened, err := openBoard(boardless)
	if err != nil {
		t.Fatalf("opening a board: %v", err)
	}
	if !strings.Contains(opened, "swap_board: []") {
		t.Errorf("an opened board is not written as an empty list:\n%s", opened)
	}
	if _, err := openBoard(opened); err == nil {
		t.Fatal("a second board was opened on the same deck")
	}
}
