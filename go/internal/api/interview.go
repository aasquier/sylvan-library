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
// over `service.claude_interview` over `claude.Interview`. **The first Claude
// surface to answer from the door**, and deliberately the smallest -- one
// handler, no job, no cache, nothing stored.
//
// **A plain route rather than a job**, which is Python's shape and is kept for
// Python's stated reason: the interview costs ~4,900 input tokens and makes no
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
// The four failure modes are kept apart, exactly as Python keeps them, because
// collapsing them tells somebody their key is missing when the model was merely
// rate limited:
//
//   - **422** -- the question was wrong: no card, a card the deck does not run,
//     a stance that will not parse.
//   - **404** -- the deck is not one this caller can see (ADR 5, through
//     `Library` like every other per-deck route).
//   - **503** -- no call was possible at all: no key in the environment.
//   - **502** -- a call was made and came back unusable.

// rationaleInterview is `POST .../interview` -- `service.claude_interview`.
func (a *API) rationaleInterview(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck. FastAPI validates `payload: dict[str,
	// Any]` while it is solving the dependencies, and `lib.source_for(owner)`
	// is not reached until the handler's own body -- so a malformed body is a
	// 422 before any 404 about the deck it was aimed at. Same order as the
	// artifacts rebuild, and for the same reason.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `str(payload.get("card", "")).strip()`. Absent and explicit null are
	// **different** here: absent takes the `""` default, while a null reaches
	// `str(None)` and becomes the four-letter string "None", which then fails
	// as a card the deck does not run rather than as a missing field. Two 422s
	// either way, with different sentences, and reproducing it costs one
	// argument.
	card := strings.TrimSpace(pyStrDefault(body, "card", ""))
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

	// `payload.get("stance") or None` and `str(payload.get("focus") or "")`:
	// a falsy value is not a value. An empty object for the stance is the
	// deck's own default rather than a refusal, and an empty focus is no
	// focus rather than the string "False".
	var requested any
	if pyTruthy(body["stance"]) {
		requested = body["stance"]
	}
	focus := ""
	if pyTruthy(body["focus"]) {
		focus = str(body, "focus")
	}

	var report claude.InterviewReport
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var runErr error
		report, runErr = claude.Interview(r.Context(), c, d, card, claude.InterviewRequest{
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
// The mapping is `service.claude_interview`'s, which sorts by what the caller
// can do about it rather than by where the failure happened: a stance they
// typed and a card they named are theirs to fix (422), a key the operator has
// not set is the instance's (503), and everything else is a call that was made
// and did not come back usable (502).
//
// **The default is 502 and not 500**, which is Python's and is worth stating
// because it looks like a mistake. `service.claude_interview` catches bare
// `Exception` around the whole of `ask()` -- the brief included -- and reports
// it through `explain`. So a pool failure while assembling the brief is a 502
// there, and is a 502 here. Narrowing it to 500 would be a nicer answer to a
// different question than the one Python is being asked.
func (a *API) refuseClaude(w http.ResponseWriter, where string, err error) bool {
	if err == nil {
		return false
	}
	var notInDeck *claude.ErrCardNotInDeck
	switch {
	case errors.As(err, &notInDeck):
		wire.Detail(w, http.StatusUnprocessableEntity, notInDeck.Error())
	case errors.Is(err, claude.ErrStanceRejected):
		// **422, since 2026-08-23 -- and it was 502 for a day, deliberately.**
		//
		// `api/app.py` has an `except ValueError` branch here whose comment
		// says "A malformed stance" and which raises 422 -- and until
		// 2026-08-23 that branch was DEAD CODE: `service.claude_interview`
		// re-raised only `ClaudeUnavailable`, `CardNotInDeck` and
		// `DeckNotFound`, so a stance ValueError fell into its broad `except
		// Exception`, became `ClaudeFailed`, and the route answered 502 before
		// its own branch was consulted. Measured against the running pair when
		// this route flipped, and reproduced rather than fixed: a flip is not
		// the place to decide which of two spellings Python meant. Then ruled
		// with Aaron and fixed in both runtimes in one change, the way
		// `edit.set_shared` went -- `stance.StanceRejected` is the name the
		// service layer re-raises by there, and this is the line here. The
		// request was wrong, not the call.
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, claude.ErrUnavailable):
		// No key, no client, no call. The sentence names the variable and
		// where to set it, because the person reading it is the operator.
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
	default:
		// `ClaudeFailed(explain(exc))` -- and `explain` is the function that
		// already knows how to turn a 401 into "your key may have expired"
		// rather than a stack trace.
		a.log.Error("the Claude route failed", "route", where, "error", err)
		wire.Detail(w, http.StatusBadGateway, claude.Explain(err))
	}
	return true
}

// pyStrDefault is `str(body.get(key, missing))`: Python's `str()` over whatever
// JSON put there, with the default reached only when the key is **absent**.
//
// The `str` helper beside it answers "" for a null, which is right where the
// Python is `str(payload.get(k) or "")` and wrong where it is
// `str(payload.get(k, ""))` -- the two spellings differ on exactly one input
// and this route uses the second.
func pyStrDefault(body map[string]any, key, missing string) string {
	if _, present := body[key]; !present {
		return missing
	}
	if body[key] == nil {
		return "None"
	}
	return str(body, key)
}
