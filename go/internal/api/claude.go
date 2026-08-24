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
// which is what internal/mt19937 is for.
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

// claudeStatus is `GET /api/claude` -- `service.claude_status`: is the Claude
// surface installed, configured, and switched on?
//
// **Three separate answers**, because a UI that collapses them tells somebody
// their key is missing when they have merely turned it off. It reaches no
// network at all: the stance is arithmetic over a table and availability is a
// question about this process's environment, so it answers on an instance with
// no pool and no account.
//
// `surface` names which mode is asking, and it exists because of a bug worth
// remembering: the create flow has no deck, so the dial beside it resolved
// `off` while `theme.stance_for` was about to run that conversation at
// `second-opinion`. All 42 tests on this endpoint passed, because every one of
// them asked about a deck. Rendering a value is what audits it.
//
// Two orderings here are contract rather than convenience, both measured
// against the running app:
//
//   - **The owner is resolved even with no slug.** Python passes
//     `lib.source_for(owner or lib.my_owner)` as an *argument*, so it runs
//     whether or not a deck is going to be read -- and `?owner=nobody` alone
//     is a 404 rather than a dial.
//   - **The deck is read before the stance is parsed.** `?stance=garbage&
//     slug=nope` is the deck's 404, not the stance's 422.
func (a *API) claudeStatus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Starlette's QueryParams.get returns the LAST repeated value where Go's
	// Query().Get returns the first -- `?slug=probe&slug=nope` asks about
	// `nope`. Measured, not assumed; it is the difference nobody sees until a
	// client appends rather than replaces.
	slug := last(q, "slug")
	owner := last(q, "owner")
	surface := last(q, "surface")
	stance := last(q, "stance")

	lib, err := a.library(r.Context())
	if a.refuse(w, "claude", err) {
		return
	}
	if owner == "" {
		owner = lib.MyOwner()
	}
	// Before the slug check, deliberately: see the second ordering above.
	src, err := lib.SourceFor(r.Context(), owner)
	if a.refuse(w, "claude", err) {
		return
	}
	var asked claude.DeckStatused
	if slug != "" {
		// `Deck.from_text(decks.read_text(slug))` -- a bare parse. The stance
		// reads one field off it and the pool is never opened, which is what
		// lets this route answer on an instance with no cards at all.
		d, getErr := src.Get(r.Context(), slug)
		if a.refuse(w, "claude", getErr) {
			return
		}
		asked = claude.DeckWithStatus(d.Status)
	}
	// `stance or None`: an empty parameter is no pin rather than a bad one.
	var requested any
	if stance != "" {
		requested = stance
	}
	dial, err := claude.Status(requested, asked, surface)
	if err != nil {
		// The one thing that can fail here, and it is the caller's: a preset
		// name that is not one. `except ValueError` in Python, and the same
		// 422.
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	wire.JSON(w, http.StatusOK, dial)
}
