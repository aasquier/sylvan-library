package api

import (
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The deck description's route: `POST /api/decks/{owner}/{slug}/describe`,
// over `claude.DescribeDeck` -- the same mode the import intake runs, asked
// from the deck page instead.
//
// **It writes nothing, and that is the design and not an omission.** The
// intake writes the paragraph because on an import there is nobody looking:
// the deck is seconds old, the field is empty by construction, and the sheet
// skips the step entirely for a deck that already says something. Here there
// may be a paragraph its owner wrote, and a route that overwrote it would be
// a surface writing over somebody's words on a button press whose label said
// "ask" -- which is ADR 8 and ADR 11's principle wearing a different field's
// name.
//
// So this route answers and stops. The paragraph goes back to the browser,
// lands in the editor's own box beside whatever is already there, and reaches
// the deck file -- if it reaches it at all -- through `PATCH .../decks/{owner}/{slug}`,
// the same door the person's typing uses, because once they have read it and
// pressed save it *is* their typing. One consequence worth stating: this route
// records nothing in the activity log (ADR 28), because it changes no field.
// The `PATCH` that follows records the edit, exactly as it does when somebody
// types the paragraph themselves.
//
// **The write axis does not gate it**, and that is consistent rather than
// loose. ADR 41 gates the intake's `rationales` on `stance.MayWrite()` because
// that step writes a `why`; its `description` step is not gated, and this
// route writes less than that one does. What governs it is the same thing that
// governs the interview and the slot argument: `initiative`, which
// `DescribeDeck` resolves and clamps itself, and which answers `off` by making
// no call at all and saying so.
//
// **`writeTarget` rather than `sourceFor`**, though, which is the one place
// this diverges from its two siblings. The interview and the argument are
// useful to anybody who can see a deck -- they are ways of thinking about a
// list. A description proposal is only useful to somebody who can save it, so
// the route reaches exactly as far as the control does: 404 for a deck this
// caller cannot see (ADR 5), 403 for a source they cannot write. The
// alternative is a call somebody pays for and nobody can use.
//
// A plain route rather than a job, on the interview's measured argument: one
// call, a brief that hands the facts over, and no chunking -- this is one
// question about the whole deck, which is why `DescribeDeck` does not chunk it
// the way its two intake siblings do.

// describeDeck is `POST .../describe`.
func (a *API) describeDeck(w http.ResponseWriter, r *http.Request) {
	// Body before deck, the order every Claude route here takes: a malformed
	// body is a 422 before any 404 about the deck it was aimed at.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}

	// The or-nothing read: a falsy value is not a value, so an empty object
	// for the stance is the deck's own default rather than a refusal.
	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}

	var got claude.Description
	var outcome claude.IntakeOutcome
	err := a.withPool(r.Context(), func(c *pool.Conn) error {
		var runErr error
		got, outcome, runErr = claude.DescribeDeck(r.Context(), d, claude.IntakeRequest{
			Endpoint:  a.claude.Endpoint,
			Limit:     a.claude.Ceiling,
			Requested: requested,
			// The caller's own source, so a tool the mode reaches for sees
			// exactly the library the caller does (ADR 22, one hop in from
			// the route). The pool is the leased one.
			Deps: tools.Deps{Source: src, Pool: c},
			// Which Claude answers this seat, read fresh off the request's
			// scope. Empty is the house model.
			Tier:   auth.ScopeFrom(r.Context()).ModelTier,
			Ledger: a.claudeLedger,
		})
		return runErr
	})
	if a.refuseClaude(w, "describe", err) {
		return
	}
	wire.JSON(w, http.StatusOK, claude.DescriptionFor(r.PathValue("slug"), got, outcome))
}
