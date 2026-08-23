package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The theme interview's two routes (ADR 20), over `api/themeruns.py`. Both
// are **jobs**, and the second of them is why the pattern exists at all.
//
// The proposal was measured at **226 seconds**, reading a dozen-odd pages and
// resolving every legend it names against the pool; no hosted proxy holds a
// POST open for that. The conversation turn joined it later and against its
// own docstring, which had justified staying synchronous with "it is a few
// seconds" -- word for word the sentence that left the dossier synchronous
// until it broke deployed at 236s. Measured, a turn is 4.3-37.7 seconds
// across eleven of them, **with one at 133.8s**, and the ceiling it has to
// fit under is known only to be at or below 236s. One outlier inside an
// unmeasured region is not a reason to restructure a chat box on its own; the
// reason is the sentence.
//
// **Checking happens in the request; calling happens in the job**, and here
// that division carries three distinct statuses rather than two. A malformed
// transcript is 422, an unknown persona or unusable seed is 422, a floor not
// yet reached is **409** -- its own status on purpose, because nothing is
// malformed and nothing failed, there simply is not enough yet, and a 422
// would read as "you sent something wrong" to a client that sent exactly the
// right thing too early. No key is 503. Carried into a worker all four would
// arrive as a job in state `error`, which is four answers flattened into one
// string and a status code for none of them.
//
// **Neither takes a deck source, and the absence is the feature.** This
// surface exists to help somebody *start* a deck; a mode that cannot reach one
// cannot critique one. The transcript is the client's (ADR 20 keeps
// conversation state off the server), so these handlers are the door -- plain
// `{role, text}` turns and never Anthropic message blocks, because an
// endpoint that accepted those would be a free proxy for somebody else's
// spend, which on a hosted instance is the entire game.
//
// **Both take `Key: ""`, which is the opposite call from research's.**
// `jobs.Submit`'s dedupe is right for a dossier and for a question string,
// where two requests inside the window are one question asked twice. Two
// turns in flight here are two different conversations -- the transcript is
// client-held, so a second tab is a second person's evening -- and collapsing
// them would hand one of them the other's question.
//
// **Nothing is cached.** ADR 18 caches a simulation because it is
// reproducible; a proposal is not, and its subject -- a ten-minute
// conversation -- does not outlive the conversation. Caching would mean the
// one moment somebody wants a different answer, clicking again on an
// unchanged conversation, is precisely the moment they cannot have one. What
// the client keeps instead is the job *id*, so a reload reattaches rather
// than paying twice.

const (
	// ThemeAskKind is the conversational half. Its own kind rather than a flag
	// on the other, so a job list distinguishes "asked me something" from
	// "spent four minutes".
	ThemeAskKind = "claude.theme.ask"
	// ThemeProposalKind is the expensive half.
	ThemeProposalKind = "claude.theme.proposal"
)

// claudeTheme is `POST /api/claude/theme` -- `themeruns.plan_ask`. Returns a
// job, which for a turn that reaches nobody is born finished.
func (a *API) claudeTheme(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	plan, err := claude.CheckAsk(body["transcript"], body["slots"],
		// `payload.get("stance") or None`: a falsy stance is no stance. The
		// persona reads the same way -- and an unknown one is an
		// `UnknownPersona`, which is a `ValueError`, which is a 422 below.
		orNil(body["stance"]), orNil(body["persona"]), body["seed"],
		// The facts already shown, client-held like the transcript, so "never
		// give the same fact twice" is enforceable rather than aspirational.
		body["facts"],
		auth.ScopeFrom(r.Context()).ModelTier, nil)
	if a.refuseTheme(w, "theme ask", err) {
		return
	}

	label := claude.AskLabel(plan)
	if plan.Answer != nil {
		// Stance `off`, or the conversation ceiling. Both are answers rather
		// than errors, and both are already in hand -- so this is the honest
		// "this job took no time" rather than a pretence that it is not a job.
		a.submit(w, r, jobs.Plan{Kind: ThemeAskKind, Label: label,
			Result: *plan.Answer, Lane: jobs.NET})
		return
	}
	// Raised here rather than inside a job that was never going to work, which
	// preserves the 503 the UI already handles.
	if err := claude.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	a.submitTheme(w, r, ThemeAskKind, label, func(ctx context.Context, c *pool.Conn,
		run claude.ThemeRun) (any, error) {
		return claude.RunAsk(ctx, c, plan, run)
	})
}

// claudeThemeProposal is `POST /api/claude/theme/proposal` --
// `themeruns.plan_proposal`. Returns a job, not a proposal.
func (a *API) claudeThemeProposal(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `float(budget) if budget else None`: a falsy budget is no budget, and a
	// budget that will not read as a number is a 422 like everything else
	// about the request.
	budget, err := themeBudget(body["budget"])
	if errors.Is(err, errBudgetType) {
		// **A reproduced wart, and it should be a 422.** `float(budget)` sits
		// inside a `try` catching `TranscriptRejected` and `ValueError`; a
		// list or an object raises the third thing `float()` can raise, so
		// Python answers an unhandled 500 to a request that is plainly
		// malformed. Reproduced rather than tidied -- a flip that changes
		// behaviour is not a flip -- and raised with Aaron, the way the stance
		// wart was before it was ruled and fixed in both runtimes at once.
		//
		// The **status** is Python's; the **body** is the door's. Python's is
		// Starlette's plain-text `Internal Server Error`, because nothing
		// caught the exception, while every 500 the door writes is
		// `{"detail": ...}` -- the convention `cards`, `decks` and `research`
		// already answer with. Measured across the pair, recorded here rather
		// than special-cased: matching one framework default in one handler
		// would make this route the odd one out among six.
		a.log.Error("the theme proposal route failed", "error", err)
		wire.Detail(w, http.StatusInternalServerError,
			"the theme surface could not answer that right now")
		return
	}
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	plan, err := claude.CheckProposal(body["transcript"], body["slots"],
		orNil(body["stance"]), budget, strOr(body, "avoid"),
		orNil(body["persona"]), body["seed"],
		auth.ScopeFrom(r.Context()).ModelTier, nil)
	if a.refuseTheme(w, "theme proposal", err) {
		return
	}

	label := claude.ProposalLabel(plan)
	if plan.Answer != nil {
		a.submit(w, r, jobs.Plan{Kind: ThemeProposalKind, Label: label,
			Result: *plan.Answer, Lane: jobs.NET})
		return
	}
	if err := claude.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	a.submitTheme(w, r, ThemeProposalKind, label, func(ctx context.Context, c *pool.Conn,
		run claude.ThemeRun) (any, error) {
		return claude.RunProposal(ctx, c, plan, run)
	})
}

// submitTheme queues one half's call on the NET lane.
//
// **The job's context is its own.** `r.Context()` is cancelled by net/http the
// moment the handler returns, and the handler returns as soon as the job id is
// written -- so a worker that reached for it would be cancelled before it
// spoke to anybody. The sim cache stored nothing at all from v183 for exactly
// this reason. The pool is leased *inside* the job for the same reason, and
// nil when the instance has none: the conversation half needs no pool, and the
// proposal then drops every commander it names, which is the honest answer
// rather than a refusal to run.
//
// `Key` is empty, deliberately -- see the file comment.
func (a *API) submitTheme(w http.ResponseWriter, r *http.Request, kind, label string,
	run func(context.Context, *pool.Conn, claude.ThemeRun) (any, error)) {
	ledgerOf := a.claudeLedger
	a.submit(w, r, jobs.Plan{
		Kind: kind, Label: label, Lane: jobs.NET,
		Run: func(rep jobs.Progress) (any, error) {
			ctx := context.Background()
			var report any
			err := a.leasePool(ctx, func(c *pool.Conn) error {
				var runErr error
				report, runErr = run(ctx, c, claude.ThemeRun{
					Ledger: ledgerOf,
					OnTurn: func(done, max int) { rep.Report(done, max) },
				})
				return runErr
			})
			if err != nil {
				return nil, claudeJobError(err)
			}
			return report, nil
		},
	})
}

// refuseTheme maps what the two checks raise onto the statuses Python gives
// them, and reports whether it answered.
//
// **`NotReady` is 409 and nothing else is**, which is the whole reason this
// is not `refuseClaude`: the per-card modes have no floor to fail and this
// one's is its most likely refusal.
func (a *API) refuseTheme(w http.ResponseWriter, what string, err error) bool {
	if err == nil {
		return false
	}
	var notReady *claude.ErrNotReady
	if errors.As(err, &notReady) {
		wire.Detail(w, http.StatusConflict, err.Error())
		return true
	}
	var rejected *claude.ErrTranscriptRejected
	var persona *claude.UnknownPersonaError
	if errors.As(err, &rejected) || errors.As(err, &persona) ||
		errors.Is(err, claude.ErrStanceRejected) {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return true
	}
	if errors.Is(err, claude.ErrUnavailable) {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return true
	}
	a.log.Error("the "+what+" route failed", "error", err)
	wire.Detail(w, http.StatusInternalServerError,
		"the theme surface could not answer that right now")
	return true
}

// orNil is `payload.get(key) or None`: a falsy value is no value.
func orNil(v any) any {
	if pyTruthy(v) {
		return v
	}
	return nil
}

// strOr is `str(payload.get(key) or "")` -- the third spelling in the family,
// copied per call site because each differs from the others on exactly one
// input. This one turns `false` and `0` into `""` where the `get(key, "")`
// spelling beside it renders them as `False` and `0`.
func strOr(body map[string]any, key string) string {
	if !pyTruthy(body[key]) {
		return ""
	}
	return str(body, key)
}

// errBudgetType stands for `float()`'s `TypeError`, which the route does
// **not** catch: `except (TranscriptRejected, ValueError)` lists two, and a
// list or an object handed to `float()` raises the third. It is a 500 in
// Python and it is a 500 here -- reproduced rather than tidied, because a
// flip that answers 422 where Python answers 500 is not a flip.
var errBudgetType = errors.New("budget is not a string or a number")

// themeBudget is `float(payload["budget"]) if payload["budget"] else None`.
//
// Python's `float()` and not `strconv.ParseFloat`: it takes underscores
// between digits, any Unicode decimal digit, and the words `inf` and `nan`,
// and its refusal message is the one that reaches the client as `detail`.
func themeBudget(raw any) (*float64, error) {
	if !pyTruthy(raw) {
		return nil, nil
	}
	value, err := claude.PyFloat(raw)
	if err != nil {
		if errors.Is(err, claude.ErrFloatType) {
			return nil, errBudgetType
		}
		return nil, err
	}
	return &value, nil
}
