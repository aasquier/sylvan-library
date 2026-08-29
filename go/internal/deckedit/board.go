package deckedit

import (
	"slices"
	"strings"
)

// The eleventh operation, and the only one that changes a deck file's *shape*
// rather than what it says.
//
// That is why it is a separate operation rather than a branch inside
// `AddCard`. ADR 12's surgical rule -- an edit changes what a deck says, never
// what shape it has -- is what makes `AddCard` refuse a board that is not
// there, and that refusal is right and stays: a mistyped `to` must never
// quietly grow a deck a section nobody asked for, and a card silently filed in
// the 99 instead of the board is a card in the deck that its owner does not
// think is in the deck. The recorded corpus says so on nine fixture decks and
// still does, unchanged.
//
// What was missing was never a looser `AddCard`. It was the sentence that
// *asks* for a board, which is a different sentence from the one that asks for
// a card, and which nothing in the app could say. Aaron, 2026-08-29: "when a
// deck doesn't already have a Sideboard a user should be able to start one
// from scratch and add cards".
//
// `EntombCard` is the precedent and the argument is the same in both places:
// the graveyard and the board are shelves beside the deck that a deck may
// simply not have started yet, and the first burial writes its own
// `graveyard:` block. `cards:` is not such a shelf, which is why nothing
// scaffolds it -- a deck file with no `cards:` block is damaged rather than
// empty, and inventing one would invent the deck.

// swapBoard is the board's key. Spelled once here rather than three times;
// `CardLists` in edit.go is the same string in its other role.
const swapBoard = "swap_board"

// AddToBoard puts a card on the swap board, opening the board first when this
// deck has never had one.
//
// A fold of the shape change and `AddCard` rather than any new text surgery,
// so everything `AddCard` refuses is still refused here in the same words: the
// card already in the deck or the graveyard, the commander, the blank `why` on
// a curated deck (ADR 13 -- the board carries no obligation to be *finished*,
// but a card put there is still a card somebody chose, and rule 4 asks the
// same question of it), the missing name, the missing category, the quantity
// below one.
//
// A deck that already has a board takes the same path minus the first step. A
// caller therefore never has to know which kind of deck it is holding, which
// is the whole point: "put this on the board" is one sentence whether or not
// there is a board yet.
func AddToBoard(text, name, category, why string, qty int) (string, error) {
	if _, err := blockHeader(strings.Split(text, "\n"), swapBoard); err != nil {
		opened, err := openBoard(text)
		if err != nil {
			return "", err
		}
		text = opened
	}
	return AddCard(text, name, category, why, qty, swapBoard)
}

// openBoard writes an empty swap board into a deck file that has none.
//
// Placed where `Deck.dump` would put it -- after the 99, before the graveyard
// -- by anchoring on the `cards:` block rather than on the end of the file,
// which is what keeps a deck that has buried something from getting its board
// written underneath the graveyard. `cards:` is the anchor because every deck
// has one; a file without it is the damaged file `blockSpan` refuses over.
//
// Written as `swap_board: []` rather than a bare header, because a block key
// with nothing under it parses to None rather than to an empty list and
// `Deck.from_text` would iterate it. `RemoveCard` says `[]` explicitly when it
// empties a list for exactly the same reason, and `AddCard` already knows how
// to reopen one that has been said.
func openBoard(text string) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	if _, err := blockHeader(lines, swapBoard); err == nil {
		return "", failf("this deck already has a `%s:` block", swapBoard)
	}
	_, end, err := blockSpan(lines, "cards")
	if err != nil {
		return "", err
	}
	// The blank line the 99 ends on belongs to whatever follows the 99, and
	// the graveyard keeps its own. Back up over the gap and lay the board's
	// own blank line down, so a deck that has just started a board reads like
	// a deck that always had one.
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	expected := copyDoc(doc)
	expected[swapBoard] = []any{}
	updated := slices.Concat(lines[:end], []string{"", swapBoard + ": []"}, lines[end:])
	return verified(strings.Join(updated, "\n"), expected)
}
