package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The rationale interview's route: `POST /api/decks/{owner}/{slug}/interview`,
// over `claude.Interview`. Deliberately the smallest Claude
// surface -- one
// handler, no job, no cache, nothing stored.
//
// **A plain route rather than a job**, and the reason stands
// measured: the interview costs ~4,900 input tokens and makes no
// tool calls in the ordinary case, because `Brief` hands the facts over instead
// of making the model go and fetch them. It sits in the seconds class rather
// than the theme proposal's minutes. ADR 20's rule still applies to it -- a
// duration measured for one surface is a question to ask of every sibling --
// and this is the sibling that has been asked and answered no.
//
// **A POST because it costs money and reaches the network**, not because it
// writes: nothing under `internal/claude` can name a write function at all, and
// `boundary_test.go` fails the commit that adds one.
//
// The four failure modes are kept apart, deliberately, because
// collapsing them tells somebody their key is missing when the model was merely
// rate limited:
//
//   - **422** -- the question was wrong: no card, a card the deck does not run,
//     a stance that will not parse.
//   - **404** -- the deck is not one this caller can see (ADR 5, through
//     `Library` like every other per-deck route).
//   - **503** -- no call was possible at all: no key in the environment.
//   - **502** -- a call was made and came back unusable.

// rationaleInterview is `POST .../interview`.
func (a *API) rationaleInterview(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck -- the recorded order: the body is
	// validated before the owner is resolved, so a malformed body is a
	// 422 before any 404 about the deck it was aimed at. Same order as the
	// artifacts rebuild, and for the same reason.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// The absent-key default, stripped. Absent and explicit null are
	// **different** here: absent takes the `""` default, while a null
	// stringifies to the four-letter string "None", which then fails
	// as a card the deck does not run rather than as a missing field. Two 422s
	// either way, with different sentences, and keeping it costs one
	// argument.
	card := strings.TrimSpace(strDefault(body, "card", ""))
	if card == "" {
		wire.Detail(w, http.StatusUnprocessableEntity, "card is required")
		return
	}

	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "interview", err) {
		return
	}

	// The or-nothing reads:
	// a falsy value is not a value. An empty object for the stance is the
	// deck's own default rather than a refusal, and an empty focus is no
	// focus rather than the string "False".
	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	focus := ""
	if truthy(body["focus"]) {
		focus = str(body, "focus")
	}

	var report claude.InterviewReport
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var runErr error
		report, runErr = claude.Interview(r.Context(), c, d, card, claude.InterviewRequest{
			Endpoint:  a.claude.Endpoint,
			Limit:     a.claude.Ceiling,
			Requested: requested,
			Focus:     focus,
			// The caller's own source, so a tool the conversation reaches for
			// sees exactly the library the caller does -- ADR 22 holding one
			// hop further in than the route. The pool is the leased one, so
			// nothing here opens a second connection.
			Deps: tools.Deps{Source: src, Pool: c},
			// Which Claude answers this seat, read fresh off the request's
			// scope. Empty is the house model, which is every account until a
			// maintainer says otherwise.
			Tier:   auth.ScopeFrom(r.Context()).ModelTier,
			Ledger: a.claudeLedger,
		})
		return runErr
	})
	if a.refuseClaude(w, "interview", err) {
		return
	}
	wire.JSON(w, http.StatusOK, report)
}

// refuseClaude turns a mode's errors into the route layer's answers.
//
// The mapping sorts by what the caller
// can do about it rather than by where the failure happened: a stance they
// typed and a card they named are theirs to fix (422), a key the operator has
// not set is the instance's (503), and everything else is a call that was made
// and did not come back usable (502).
//
// **The default is 502 and not 500**, which is the recorded contract and is
// worth stating
// because it looks like a mistake. The recorded route wraps the whole of
// the ask -- the brief included -- into one reported failure. So a pool
// failure while assembling the brief has always been a 502, and is a 502
// here. Narrowing it to 500 would be a nicer answer to a
// different question than the one being asked.
func (a *API) refuseClaude(w http.ResponseWriter, where string, err error) bool {
	if err == nil {
		return false
	}
	var notInDeck *claude.ErrCardNotInDeck
	switch {
	case errors.As(err, &notInDeck):
		wire.Detail(w, http.StatusUnprocessableEntity, notInDeck.Error())
	case errors.Is(err, claude.ErrStanceRejected):
		// **422, since 2026-08-23 -- and it was 502 for a day,
		// deliberately.**
		//
		// The recorded route *intended* a 422 for a malformed stance and
		// never reached it: the branch was DEAD CODE, because the stance
		// refusal fell into the broad catch-everything, became a generic
		// failure, and answered 502 before
		// its own branch was consulted. Measured on the live wire, kept as
		// measured at first -- a byte-for-byte reproduction is not the
		// place to decide which of two spellings was meant -- then ruled
		// with Aaron in one change, the way
		// the share toggle went. `ErrStanceRejected` is the typed refusal,
		// and this is the line. The
		// request was wrong, not the call.
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, claude.ErrUnavailable):
		// No key, no client, no call. The sentence names the variable and
		// where to set it, because the person reading it is the operator.
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
	default:
		// The explained failure -- `Explain` is the function that
		// already knows how to turn a 401 into "your key may have expired"
		// rather than a stack trace.
		a.log.Error("the Claude route failed", "route", where, "error", err)
		wire.Detail(w, http.StatusBadGateway, claude.Explain(err, a.claude.Endpoint.ModelFor("")))
	}
	return true
}

// strDefault is the absent-key read: the recorded stringification over
// whatever
// JSON put there, with the default reached only when the key is **absent**
// -- so an explicit null renders as "None".
//
// The `str` helper beside it answers "" for a null, which is right for the
// or-empty read and wrong for this one
// -- the two spellings differ on exactly one input
// and this route uses the second.
func strDefault(body map[string]any, key, missing string) string {
	if _, present := body[key]; !present {
		return missing
	}
	if body[key] == nil {
		return "None"
	}
	return str(body, key)
}
