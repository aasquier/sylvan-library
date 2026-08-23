package api

import (
	"net/http"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The slot argument's route: `POST /api/decks/{owner}/{slug}/argue`, over
// `service.claude_argue` over `claude.Argue`.
//
// **Deliberately the interview's twin, down to the status codes.** The two
// per-card modes differ in what they answer, not in how they are asked, and a
// client driving one should not have to learn a second shape to drive the
// other. So this shares `refuseClaude` with the interview rather than growing
// a second mapping that could drift -- including the stance wart, which is one
// wart in one place rather than two spellings of it.
//
// **Synchronous, and that is a measured claim rather than an assumption.**
// Python's docstring is explicit about it: the interview costs ~4,900 input
// tokens and makes no tool calls because `Brief` hands the facts over; this
// mode shares that brief and adds a tool set it uses only when it goes
// shopping, so it sits in the same seconds class rather than the theme
// proposal's minutes. ADR 20's rule -- a duration measured for one surface is
// a question to ask of every sibling -- was asked here and answered no.
//
// The SWEEP is the other question and a different answer: `/argue/deck`
// multiplies this by a selection, which is minutes, and it is a job. It has
// not crossed.

// argueSlot is `POST .../argue` -- `service.claude_argue`.
func (a *API) argueSlot(w http.ResponseWriter, r *http.Request) {
	// Body before deck, as FastAPI resolves them; see rationaleInterview.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
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
	if a.refuse(w, "argue", err) {
		return
	}

	var requested any
	if pyTruthy(body["stance"]) {
		requested = body["stance"]
	}
	focus := ""
	if pyTruthy(body["focus"]) {
		focus = str(body, "focus")
	}

	var report []wire.KV
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var runErr error
		report, runErr = claude.Argue(r.Context(), c, d, card, claude.ArgueRequest{
			Requested: requested,
			Focus:     focus,
			Deps:      tools.Deps{Source: src, Pool: c},
			Tier:      auth.ScopeFrom(r.Context()).ModelTier,
			Ledger:    a.claudeLedger,
		})
		return runErr
	})
	if a.refuseClaude(w, "argue", err) {
		return
	}
	// Ordered rather than a struct, because `alternatives_dropped` carries a
	// different SET of keys depending on whether a call happened -- see
	// claude.DroppedAlternatives.
	raw, err := wire.MarshalOrdered(report)
	if a.refuse(w, "argue", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}
