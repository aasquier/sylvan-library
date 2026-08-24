package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Research's route (ADR 26): `POST /api/claude/research`, a **job** from the
// first commit rather than after a deployed failure taught it to be.
//
// ADR 20's lesson -- *a duration measured for one
// surface is a question to ask of every sibling surface* -- had cost three
// incidents by the time this was written (the proposal at 226 seconds, the
// theme turn whose docstring said "a few seconds", the dossier at 236), and
// research searches more than the dossier does. It was never a candidate for
// a synchronous POST.
//
// **Takes no deck, no owner and no deck source, and that absence is the
// feature.** The handler reads a body and a seat and nothing else; the mode
// it runs cannot reach the library, so it cannot critique a deck and cannot
// quietly become the deck conversation ADR 15 leaves unbuilt. The way to
// make this route into that one is to add a dependency on purpose, in a
// diff, superseding ADR 26.
//
// What stays in the request is every refusal: 422 for an empty question or
// one long enough to be a pasted decklist, 422 for a stance that will not
// read, 503 for no key. A stance of `off` is a job born finished. A call that
// comes back unusable is a job in state `error`, where it belongs once the
// response has been sent.
//
// **Deduplicated in flight, and that is the opposite call from the theme
// conversation's `key=None`.** Nothing is cached (ADR 26), but two identical
// question strings from one seat inside the minutes a search takes are one
// question asked twice: the question text *is* the whole input, and there is
// no client-held transcript making two identical-looking requests into two
// conversations. `jobs.Submit` matches per owner as well as per key, which
// matters more here than for the dossier -- a question is somebody's own
// words, and two accounts that happened to type the same sentence must not
// be handed each other's job (ADR 5's shape, one layer down).

// ResearchKind is what `/api/jobs` calls one of these.
const ResearchKind = "claude.research"

// researchLabelChars is how much of the question goes in the job's label. A
// job list is a list of one-liners, and the full 2,000 characters would make
// it a wall.
const researchLabelChars = 60

// researchLabel is `plan_research`'s label: `research: ` and the question's
// first sixty characters, an ellipsis when it was cut. Characters, not bytes
// -- `question[:60]` is a `str` slice -- and the cut end is right-stripped
// before the ellipsis so it never reads "research: why does ...".
func researchLabel(question string) string {
	short := textutil.Head(question, researchLabelChars)
	if textutil.Len(question) > researchLabelChars {
		short = textutil.RStrip(short) + "..."
	}
	return "research: " + short
}

// claudeResearch is `POST /api/claude/research` -- `researchruns.plan_research`
// behind `app.claude_research`. Returns a job.
func (a *API) claudeResearch(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `payload.get("stance") or None`: a falsy stance is no stance.
	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	plan, err := claude.CheckResearch(body["question"], requested,
		auth.ScopeFrom(r.Context()).ModelTier, nil)
	if err != nil {
		// `except (QuestionRejected, ValueError)`: a question that is not one,
		// or a stance that will not read. Both are the caller's to fix.
		var rejected *claude.ErrQuestionRejected
		if errors.As(err, &rejected) || errors.Is(err, claude.ErrStanceRejected) {
			wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		a.log.Error("the research route failed", "error", err)
		wire.Detail(w, http.StatusInternalServerError, "the research surface could not answer that right now")
		return
	}
	label := researchLabel(plan.Question)
	if plan.Answer != nil {
		// The stance is `off`. A real position and a real answer, and it costs
		// nothing -- a job born finished.
		a.submit(w, r, jobs.Plan{Kind: ResearchKind, Label: label, Result: *plan.Answer, Lane: jobs.NET})
		return
	}
	if err := claude.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	ledgerOf := a.claudeLedger
	a.submit(w, r, jobs.Plan{
		Kind: ResearchKind, Label: label, Lane: jobs.NET, Key: plan.Key,
		Run: func(rep jobs.Progress) (any, error) {
			// Its own context: the request is over. The pool is leased for the
			// conversation so `get_cards` and the final card lookup share one
			// open database, and nil when the instance has none -- every card
			// then comes back unresolved, which is the honest answer rather
			// than a refusal to search at all.
			ctx := context.Background()
			var report claude.ResearchReport
			err := a.leasePool(ctx, func(c *pool.Conn) error {
				var runErr error
				report, runErr = claude.RunResearch(ctx, c, plan, claude.ResearchRun{
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
