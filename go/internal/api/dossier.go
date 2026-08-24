package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The commander dossier's two routes (ADR 19): `GET .../dossier`, the free
// half, and `POST .../dossier`, which writes one -- as a **job**.
//
// The surface that had to be written twice. The
// dossier was measured at **236 seconds on the deployed instance** -- longer
// than the theme proposal the job pattern was built for -- and stayed a
// synchronous POST because nobody re-measured it when that pattern landed.
// Deployed, it presented as a spinner and then Safari's `Load failed`: a
// transport error, so no status code reached the client, and no access-log
// line was written either, because the log line lands when a response
// completes and that one never did. The work itself was fine and sat in
// `dossier_cache` while the page showed a failure. A job id is what turns
// that from a coincidence into the contract.
//
// **Checking happens in the request; calling happens in the job.** A deck
// with no commander the pool can find is a 422 and a fact about the deck;
// four minutes is a long time to wait to be told a slug was wrong. A stored
// dossier is a job born finished, and so is a stance of `off`. No key is a
// 503 -- asked *after* the store, so a dossier somebody else wrote is served
// on an instance with no key at all. Only the Anthropic call is queued, on
// the NET lane, keyed on the cache key so that a second click inside the
// four-minute window joins the run already going rather than paying twice
// (2026-08-13, two paid runs for one commander, concurrently, on the
// instance).
//
// The GET is a **different function from the POST rather than the same one
// with a flag**: free and idempotent, so the deck page can ask on every load,
// and no amount of refreshing can turn it into spend.

// DossierKind is what `/api/jobs` calls one of these.
const DossierKind = "claude.dossier"

// dossierStore is the `dossier_cache` table over the door's read-write
// app.db handle, or a store that caches nothing on an instance with none.
func (a *API) dossierStore() *claude.DossierStore {
	return claude.NewDossierStore(a.writeDB, a.log)
}

// claudeDossierCached is `GET .../dossier` -- `service.claude_dossier_cached`:
// a stored dossier, or a payload saying there is none. Never calls Anthropic.
func (a *API) claudeDossierCached(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuse(w, "dossier", err) {
		return
	}
	var payload any
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var readErr error
		payload, readErr = claude.ReadCachedDossier(r.Context(), c, slug, d, a.dossierStore(), a.claude.Endpoint)
		return readErr
	})
	if a.refuse(w, "dossier", err) {
		return
	}
	wire.JSON(w, http.StatusOK, payload)
}

// claudeDossier is `POST .../dossier`. Returns a job, not a dossier.
func (a *API) claudeDossier(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck -- the recorded order; see
	// rationaleInterview.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuse(w, "dossier", err) {
		return
	}
	// The or-nothing stance read, and the truthy refresh flag.
	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	req := claude.DossierRequest{
		Requested: requested,
		Refresh:   truthy(body["refresh"]),
		Tier:      auth.ScopeFrom(r.Context()).ModelTier,
		Store:     a.dossierStore(),
		Limit:     a.claude.Ceiling,
		Endpoint:  a.claude.Endpoint,
	}
	var plan *claude.DossierPlan
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var checkErr error
		plan, checkErr = claude.CheckDossier(r.Context(), c, slug, d, req)
		return checkErr
	})
	if a.refuseDossier(w, err) {
		return
	}
	label := "dossier: " + plan.Commander
	if plan.Answer != nil {
		// A stored dossier, or a stance of `off`. Both are real answers and
		// neither costs anything, so they are returned now rather than a
		// second from now on a worker -- a job born finished.
		a.submit(w, r, jobs.Plan{Kind: DossierKind, Label: label, Result: *plan.Answer, Lane: jobs.NET})
		return
	}
	// Raised here rather than four minutes into a job that was never going to
	// work, which preserves the 503 the UI already handles.
	if err := a.claude.Endpoint.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	ledgerOf, deps := a.claudeLedger, tools.Deps{Source: src}
	a.submit(w, r, jobs.Plan{
		Kind: DossierKind, Label: label, Lane: jobs.NET,
		// The cache key is exactly the right identity for "somebody is
		// already writing this": a dossier is about a character, so two decks
		// led by Gyome asked at the same moment are one four-minute search.
		// Empty when the pool has no oracle id, which disables the dedupe
		// exactly as it disables the caching.
		Key: plan.Key,
		Run: func(rep jobs.Progress) (any, error) {
			// The request is over by the time this runs, and its context with
			// it -- so the job takes its own, never the request's (a
			// cancelled context is the sim cache's old bug). The pool lease
			// is held for the conversation, which is a
			// count and not a lock: the model's `get_cards` calls and the
			// competitor lookup both need the connection, and the pool stays
			// open for as long as anything holds one.
			ctx := context.Background()
			var report claude.DossierReport
			err := a.leasePool(ctx, func(c *pool.Conn) error {
				deps.Pool = c
				var runErr error
				report, runErr = claude.RunDossier(ctx, c, plan, claude.DossierRun{
					Deps: deps, Ledger: ledgerOf,
					OnTurn: func(done, max int) { rep.Report(done, max) },
				})
				return runErr
			})
			if err != nil {
				return nil, claudeJobError(err, a.claude.Endpoint.ModelFor(""))
			}
			return report, nil
		},
	})
}

// refuseDossier turns `CheckDossier`'s errors into the route's answers: no
// commander the pool can find and a stance that will not read are both 422
// (facts about the request), a deck the caller cannot see is the 404 `refuse`
// already gives, and anything else is the library failing.
//
// The stance 422 here was never the interview's 502 wart: `plan_dossier`
// runs `check_dossier` in the request, where the route's own `except
// ValueError` is live, and `NoCommander` is a ValueError too.
func (a *API) refuseDossier(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var headless *claude.ErrNoCommander
	switch {
	case errors.As(err, &headless):
		wire.Detail(w, http.StatusUnprocessableEntity, headless.Error())
	case errors.Is(err, claude.ErrStanceRejected):
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		return a.refuse(w, "dossier", err)
	}
	return true
}

// leasePool runs fn with a leased pool, or with nil when there is none --
// `withPool` for a job, which has no request to degrade.
func (a *API) leasePool(ctx context.Context, fn func(c *pool.Conn) error) error {
	if a.pool == nil {
		return fn(nil)
	}
	err := a.pool.Use(ctx, fn)
	if errors.Is(err, pool.ErrNoPool) {
		return fn(nil)
	}
	return err
}

// claudeJobError is what a failed Claude job records, in the recorded
// words: the error's own sentence for the turn ceiling and for a key that
// vanished
// between the check and the worker, and `claude.Explain` for everything else
// -- the function that turns a 401 into "your key may have expired" rather
// than a stack trace in a job's error field.
func claudeJobError(err error, model string) error {
	if errors.Is(err, claude.ErrModeExhausted) || errors.Is(err, claude.ErrUnavailable) {
		return err
	}
	return errors.New(claude.Explain(err, model))
}
