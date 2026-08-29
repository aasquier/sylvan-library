package api

import (
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/deckedit"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The tenth deck write, and the one the comment in `edits.go` predicted: "the
// tenth edit operation is the one somebody adds in a year, and it inherits
// both". It does. This handler carries no gate call and no log call of its
// own; it goes out through `answer` like the other nine.
//
// **Why it is not `POST .../cards` with `to: "swap_board"`.** That route
// refuses a deck whose file has no `swap_board:` block, and the refusal is
// correct: an edit changes what a deck says, never what shape it has (ADR 12),
// and a mistyped `to` must never quietly grow a deck a section nobody asked
// for. Nothing about that changes here -- `deckedit.AddCard` still refuses,
// and the recorded corpus still says so on nine fixture decks.
//
// What was missing was the sentence that *asks* for a board. Until 2026-08-29
// no surface in the app could say it, so a deck that had never kept a swap
// board could never start one: the section did not render, and the one route
// that could have written to it answered 422. Aaron: "when a deck doesn't
// already have a Sideboard a user should be able to start one from scratch and
// add cards".
//
// **One route rather than an open-then-add pair**, which matters for more than
// tidiness. Two calls can half-succeed, and the state they leave behind -- an
// empty board on a deck whose card was refused -- is a shape the user did not
// ask for, written by a request that failed. One call is one write: either the
// board exists with the card on it, or the file is exactly as it was.

// startBoard is `POST .../board`: put a card on this deck's swap board,
// opening the board first if the deck has never had one.
//
// The body is `POST .../cards`'s, minus `to` -- there is only one destination
// here, and a route that named it would be asking a question with one answer.
// A deck that already has a board is served by the same call, so the frontend
// has one request for "put this on the board" rather than a rule about which
// kind of deck it is looking at.
func (a *API) startBoard(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	qty, err := bodyQty(body)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, "qty must be a number: "+err.Error())
		return
	}
	category, err := checkCategory(str(body, "category"))
	if a.refuseWrite(w, "board", err) {
		return
	}

	// The same pool check the 99 gets, and deliberately so. A swap board is
	// where a card is weighed rather than played, which is an argument for
	// letting an unplayable card sit there -- but the check that would have to
	// relax is `playableCard`, which is one function serving every write path,
	// and loosening it here would loosen it for the deck as well. The board
	// holds cards this commander could legally play; what it does not hold is
	// an obligation to play them.
	rec, err := a.playableCard(r.Context(), d, str(body, "name"),
		"adding a card needs the card pool")
	if a.refuseWrite(w, "board", err) {
		return
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "board", err) {
		return
	}
	updated, err := deckedit.AddToBoard(text, rec.Name, category, str(body, "why"), qty)
	a.answer(w, r, src, updated, commitOutcome{
		// `into` rides along at its frozen spelling and frozen value, so a
		// board add reads identically in the response and in the history
		// whichever of the two routes wrote it.
		extra: map[string]any{"added": rec.Name, "category": category, "into": "swap_board"},
		edit: decklog.Edit{Kind: decklog.EditAdd, Card: rec.Name,
			Category: category, Into: "swap_board"},
	}, err)
}
