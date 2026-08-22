package api

import (
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/tarot"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The Claude surface's free corners: the roster of voices, and the tarot
// table's deal. Both are deterministic, need no key, no pool and no network,
// and answer on a base install with no account — which is exactly why they
// cross first. The rest of the family waits on the pipe.

// personaRoster is `GET /api/claude/personas`: the voices the theme interview
// can adopt, for the door to render.
//
// Free, deterministic and reaching nothing: this is a checked-in table, the
// same class of thing as /api/colors. It answers with no key set and no card
// pool, which matters because the door renders before anybody has committed to
// spending anything.
//
// `voice` is deliberately not in the payload — see internal/claude, where the
// roster is its own type so that a field added to Persona cannot silently
// publish a prompt.
func (a *API) personaRoster(w http.ResponseWriter, _ *http.Request) {
	wire.JSON(w, http.StatusOK, struct {
		Personas []claude.RosterEntry `json:"personas"`
		Default  string               `json:"default"`
	}{Personas: claude.Roster(), Default: claude.DefaultPersona})
}

// tarotReading is `GET /api/tarot/reading`: deal three cards. No model, no card
// pool, no network, no cost.
//
// **Python decides** (ADR 14): a shuffle has a right answer and belongs in
// code, while what a spread means has none and belongs to the reader. Seeded
// and returning its seed, so the client can carry one integer and get the same
// three cards for the whole conversation — the same stateless trick the
// transcript uses, and the reason a reading needs no table either.
//
// A seed may be supplied to re-deal an existing reading, which is what a reload
// does. That is the promise the whole port rests on here: a seed minted by the
// Python door before the cutover must deal the same three cards from this one,
// which is what internal/pyrand is for.
func (a *API) tarotReading(w http.ResponseWriter, r *http.Request) {
	// Last value wins, as Starlette's query mapping does — `?seed=7&seed=9`
	// is nine. Go's Query().Get() returns the FIRST, which is the kind of
	// difference that never shows up until somebody's client appends.
	vals := r.URL.Query()["seed"]
	if len(vals) == 0 {
		wire.JSON(w, http.StatusOK, tarot.Deal(nil))
		return
	}
	raw := vals[len(vals)-1]
	seed, ok := tarot.ParseSeed(raw)
	if !ok {
		// An absent parameter is a fresh deal; a present but unreadable one is
		// a 422. `?seed=` with no value takes this branch, which is measured
		// rather than assumed — Pydantic refuses the empty string.
		wire.Unprocessable(w, wire.IntParsing("query", "seed", raw))
		return
	}
	wire.JSON(w, http.StatusOK, tarot.Deal(seed))
}
