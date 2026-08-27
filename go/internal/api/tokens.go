package api

import (
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// deckTokens is `GET /api/decks/{owner}/{slug}/tokens`: everything this deck
// can put onto the battlefield that was never in it.
//
// **A GET, where the Wheel and the opening hand are POSTs.** Those two are
// fresh draws — each press is a new deal and must not be a URL a proxy may
// repeat — and this is the opposite: a derived fact about the deck, the same
// answer every time, in the same family as `/stats` and `/validate`. It reads
// the deck and nothing else, so a reader of a shared deck may ask it exactly
// as they may ask for the stats.
//
// **The commanders go in with the 99.** A commander makes tokens like anything
// else, and Gyome, Master Chef is the whole reason anybody asked for this.
//
// Three ways this declines, and they are three different sentences because
// they send a reader to three different places:
//
//   - no card pool at all -- the degraded 200 every deck route answers with,
//     so the page renders and says why it is empty
//   - a library that predates the column, or carries it empty -- `read` false
//     and no tokens. **This is the deploy window** (ADR 23): merging is
//     deploying, the pool cannot migrate itself, and until somebody refreshes
//     it this is the honest answer. The page must say "not read yet" and never
//     "this deck makes nothing"
//   - anything else -- `refuse`, which is 404 for a deck that is not the
//     caller's to see (ADR 5) and 500 with a sentence otherwise
func (a *API) deckTokens(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "tokens", err) {
		return
	}
	var sheet pool.TokenSheet
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		var readErr error
		sheet, readErr = c.TokensMade(r.Context(), d.CardNames(true))
		return readErr
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, wire.OrderedMap{
			{Key: "pool_available", Value: false},
			{Key: "read", Value: false},
			{Key: "tokens", Value: []any{}},
			{Key: "message", Value: noPoolMessage},
		})
		return
	}
	if a.refuse(w, "tokens", err) {
		return
	}
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "pool_available", Value: true},
		{Key: "read", Value: sheet.Read},
		{Key: "tokens", Value: tokenPlates(sheet.Tokens)},
	})
}

// tokenPlates renders the sheet.
//
// The painting's fields are absent together or present together — a token this
// pool has no printing for is a plate with no picture on it, never a picture
// credited to nobody. `art_crop` is derived from the whole face exactly as the
// deck page derives every other one, so a token thumbnail and a card thumbnail
// are cut the same way; the whole face rides along because a token is a thing
// you put on a table, and the face is what that looks like.
func tokenPlates(tokens []pool.TokenMade) []wire.OrderedMap {
	out := make([]wire.OrderedMap, 0, len(tokens))
	for _, made := range tokens {
		var image, artist, setCode, setName *string
		if made.Art != nil {
			img, who := made.Art.Image, made.Art.Artist
			code, printing := made.Art.Set, made.Art.Printing
			image, artist, setCode, setName = &img, &who, &code, &printing
		}
		madeBy := made.MadeBy
		if madeBy == nil {
			madeBy = []string{}
		}
		out = append(out, wire.OrderedMap{
			{Key: "name", Value: made.Name},
			{Key: "type_line", Value: made.TypeLine},
			{Key: "image", Value: image},
			{Key: "art_crop", Value: pool.ArtCropFrom(image)},
			{Key: "artist", Value: artist},
			{Key: "set_code", Value: setCode},
			{Key: "set_name", Value: setName},
			{Key: "made_by", Value: madeBy},
		})
	}
	return out
}
