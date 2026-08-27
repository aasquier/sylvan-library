package api

import (
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
	"github.com/aasquier/sylvan-library/go/internal/sim/opening"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// deckOpeningHand is `POST /api/decks/{owner}/{slug}/opening-hand`: shuffle
// this deck and turn over seven, with the arithmetic about them.
//
// **POST, and it writes nothing** -- the Wheel's shape, for the Wheel's
// reason. Each press is a fresh deal rather than a resource with a URL, so it
// cannot be a GET a browser or a proxy is entitled to repeat behind your
// back; and it is read-only with respect to the deck, so a reader of a shared
// deck may deal from it exactly as they may read its stats. Nothing here
// touches the activity log (ADR 28): dealing a practice hand is not something
// that happened to the deck.
//
// **There is no seed on the wire, in either direction.** `internal/sim/
// opening` argues that at length; the short of it is that a practice hand is
// not a fortune somebody replays, and a seed control on a beginner's toy is
// machinery rendered at a person who came here for cards (commandment 10).
// The shuffle is still seeded, still through the project's one generator, and
// still exactly reproducible -- in the tests, which are where a replay is
// worth anything.
//
// The three ways this can decline are three different sentences, because they
// send a person to three different places:
//
//   - no pool on this machine -- the degraded 200 every deck route answers
//     with, so the page renders and says why it is empty rather than erroring
//   - a deck that compiles to no cards at all -- 422, the one state the
//     simulator refuses (`compile.NothingToSimulate`), and the only refusal
//     here that is about the deck
//   - anything else -- `refuse`, which is 404 for a deck that is not yours to
//     see (ADR 5) and 500 with a sentence otherwise
func (a *API) deckOpeningHand(w http.ResponseWriter, r *http.Request) {
	if _, ok := readOptionalBody(w, r); !ok {
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "opening-hand", err) {
		return
	}
	var hand *opening.Hand
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		dealt, dealErr := opening.DealFromPool(r.Context(), c, d, nil)
		if dealErr != nil {
			return dealErr
		}
		hand = dealt
		return nil
	})
	var needsPool *compile.PoolRequired
	if errors.As(err, &needsPool) || errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, wire.OrderedMap{
			{Key: "pool_available", Value: false},
			{Key: "cards", Value: []any{}},
			{Key: "message", Value: noPoolMessage},
		})
		return
	}
	var empty *compile.NothingToSimulate
	if errors.As(err, &empty) {
		wire.Detail(w, http.StatusUnprocessableEntity, empty.Error())
		return
	}
	if a.refuse(w, "opening-hand", err) {
		return
	}
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "pool_available", Value: true},
		{Key: "cards", Value: hand.Cards},
		{Key: "reading", Value: hand.Reading},
		{Key: "deck_size", Value: hand.DeckSize},
		{Key: "declared_size", Value: hand.DeclaredSize},
		{Key: "unresolved_count", Value: hand.UnresolvedCount},
		{Key: "commander", Value: hand.Commander},
		{Key: "answered_by", Value: hand.AnsweredBy},
		{Key: "caveat", Value: hand.Caveat},
	})
}
