package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
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
// The interview costs ~4,900 input
// tokens and makes no tool calls because `Brief` hands the facts over; this
// mode shares that brief and adds a tool set it uses only when it goes
// shopping, so it sits in the same seconds class rather than the theme
// proposal's minutes. ADR 20's rule -- a duration measured for one surface is
// a question to ask of every sibling -- was asked here and answered no.
//
// The SWEEP is the other question and a different answer: `/argue/deck`
// multiplies this by a selection, which is minutes, and it is a job. It is at
// the bottom of this file.

// argueSlot is `POST .../argue`.
func (a *API) argueSlot(w http.ResponseWriter, r *http.Request) {
	// Body before deck -- the recorded order; see rationaleInterview.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
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
	if a.refuse(w, "argue", err) {
		return
	}

	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	focus := ""
	if truthy(body["focus"]) {
		focus = str(body, "focus")
	}

	var report []wire.KV
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var runErr error
		report, runErr = claude.Argue(r.Context(), c, d, card, claude.ArgueRequest{
			Endpoint:  a.claude.Endpoint,
			Limit:     a.claude.Ceiling,
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

// The sweep: `POST /api/decks/{owner}/{slug}/argue/deck`.
// A **job**, where the single-card route one screen up is
// synchronous -- and the difference is arithmetic rather than taste. That one
// is measured in the seconds class; this multiplies it by a selection, so a
// few dozen slots is minutes and a full 99 is tens of minutes.
//
// **One job for the whole sweep, sequential inside it.** The NET lane is two
// workers wide and shared with the theme proposal and the dossier; a sweep
// submitted as N jobs would occupy the whole lane for its duration and starve
// every sibling surface. One job argues one card at a time and reports
// progress, which is also what makes the bar honest.
//
// **A failed card is recorded and the sweep continues**, because partial
// results are the point of paying for a sweep and one flaky call must not cost
// the other forty answers. The single exception is the credential vanishing
// mid-run: every remaining card would fail the same way, so the rest are
// marked unattempted and the sweep stops rather than burning the selection on
// a dead key.
//
// **Deduplicated in flight on the selection itself**, sorted and casefolded so
// the same slots picked in a different order are the same sweep, and nothing
// is cached across runs -- like the theme proposal, two runs over the same
// slots may legitimately differ.

// ArgueSweepKind is what `/api/jobs` calls one of these.
const ArgueSweepKind = "claude.argue.deck"

// argueOffReason is the answer when the stance permits no calls: one report
// for the sweep, not N copies of "no call was made".
const argueOffReason = "The stance is off, so no calls were made. " +
	"Everything else about this deck still works."

// argueSweepResult renders the six keys in the recorded order.
//
// `reports` is a list of ordered reports and `errors` is a dict built in
// SWEEP ORDER -- so both halves need ordered rendering, and a
// `map[string]string` for the errors would alphabetise a list whose order is
// the order things went wrong in.
func argueSweepResult(slug string, asked bool, reason string, total int,
	reports [][]wire.KV, errs []wire.KV) wire.OrderedMap {
	renderedReports := make([]any, 0, len(reports))
	for _, report := range reports {
		renderedReports = append(renderedReports, wire.OrderedMap(report))
	}
	return wire.OrderedMap{
		{Key: "slug", Value: slug},
		{Key: "asked", Value: asked},
		{Key: "reason", Value: reason},
		{Key: "total", Value: total},
		{Key: "reports", Value: renderedReports},
		{Key: "errors", Value: wire.OrderedMap(errs)},
	}
}

// argueSweep is `POST .../argue/deck` -- `argueruns.plan_review`.
func (a *API) argueSweep(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `isinstance(cards, list) and all(isinstance(c, str) ...)`: absent, a
	// string, a number and a list with one number in it are all the same
	// refusal, because all four are "that is not a selection".
	raw, isList := body["cards"].([]any)
	if !isList {
		wire.Detail(w, http.StatusUnprocessableEntity, "cards must be a list of card names")
		return
	}
	asked := make([]string, 0, len(raw))
	for _, item := range raw {
		name, isString := item.(string)
		if !isString {
			wire.Detail(w, http.StatusUnprocessableEntity, "cards must be a list of card names")
			return
		}
		// `[c.strip() for c in cards if c and c.strip()]` -- the falsy test
		// runs on the RAW value and the strip on the kept ones, so a card that
		// is only whitespace is dropped rather than kept as "".
		if trimmed := textutil.Strip(name); trimmed != "" {
			asked = append(asked, trimmed)
		}
	}
	if len(asked) == 0 {
		wire.Detail(w, http.StatusUnprocessableEntity, "no cards selected")
		return
	}

	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuse(w, "argue", err) {
		return
	}
	// `{c["name"].casefold(): c["name"] for c in deck["cards"]}` -- the 99 and
	// only the 99: not the command zone, not the graveyard, not the swap
	// board, because `get_deck`'s `cards` is `deck.cards`. And **casefold and
	// not lower**, which is the one place in this family the two can disagree:
	// the neighbours fold a pool name against the pool's own spelling, where
	// both sides are the same string, and this folds a name somebody TYPED
	// against the deck's.
	inDeck := make(map[string]string, len(d.Cards))
	for _, entry := range d.Cards {
		inDeck[claude.Casefold(entry.Name)] = entry.Name
	}
	missing := []string{}
	for _, name := range asked {
		if _, held := inDeck[claude.Casefold(name)]; !held {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// Named, not counted: "which card" is the actionable part, and the
		// deck page sent these, so a mismatch means its list is stale.
		wire.Detail(w, http.StatusUnprocessableEntity,
			"not in this deck: "+strings.Join(missing, ", "))
		return
	}

	// The pool's spelling, de-duplicated with order kept -- the order is the
	// user's selection order, and it is the order the reports come back in.
	seen := map[string]bool{}
	ordered := make([]string, 0, len(asked))
	for _, name := range asked {
		proper := inDeck[claude.Casefold(name)]
		folded := claude.Casefold(proper)
		if seen[folded] {
			continue
		}
		seen[folded] = true
		ordered = append(ordered, proper)
	}

	// Resolved here only to answer "would any call be made at all"; each
	// per-card report still carries the stance `Argue` resolved for it, from
	// the same inputs.
	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	effective, err := claude.Resolve(requested, claude.DeckWithStatus(d.Status), a.claude.Ceiling)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	plural := "s"
	if len(ordered) == 1 {
		plural = ""
	}
	label := fmt.Sprintf("argue: %d slot%s of %s", len(ordered), plural, slug)

	if !effective.AllowsCalls() {
		// A real position and a real answer, and it costs nothing.
		a.submit(w, r, jobs.Plan{Kind: ArgueSweepKind, Label: label, Lane: jobs.NET,
			Result: argueSweepResult(slug, false, argueOffReason, len(ordered), nil, nil)})
		return
	}
	// Raised here rather than minutes into a sweep that was never going to
	// work, which preserves the 503 the UI already handles.
	if err := a.claude.Endpoint.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// The slug plus the selection: a double-click joins the sweep in flight, a
	// different selection is different work. Sorted and casefolded so the same
	// slots picked in a different order are still the same sweep.
	folded := make([]string, 0, len(ordered))
	for _, name := range ordered {
		folded = append(folded, claude.Casefold(name))
	}
	sort.Strings(folded)
	sum := sha256.Sum256([]byte(strings.Join(folded, "\x00")))
	key := slug + ":" + hex.EncodeToString(sum[:])[:16]

	tier := auth.ScopeFrom(r.Context()).ModelTier
	ledgerOf := a.claudeLedger
	endpoint := a.claude.Endpoint
	ceiling := a.claude.Ceiling
	a.submit(w, r, jobs.Plan{
		Kind: ArgueSweepKind, Label: label, Lane: jobs.NET, Key: key,
		Run: func(rep jobs.Progress) (any, error) {
			// Its own context: the request is over, and a sweep outlives it by
			// minutes.
			ctx := context.Background()
			reports := [][]wire.KV{}
			errs := []wire.KV{}
			total := len(ordered)
			rep.Report(0, total)
			for i, name := range ordered {
				var report []wire.KV
				runErr := a.withPool(ctx, func(c *pool.Conn) error {
					var argueErr error
					report, argueErr = claude.Argue(ctx, c, d, name, claude.ArgueRequest{
						// Captured at plan time with everything else the job
						// carries: a sweep outlives the request that knew who
						// was asking.
						Endpoint:  endpoint,
						Limit:     ceiling,
						Requested: requested,
						Deps:      tools.Deps{Source: src, Pool: c},
						Tier:      tier,
						Ledger:    ledgerOf,
					})
					return argueErr
				})
				if errors.Is(runErr, claude.ErrUnavailable) {
					// Defence in depth, and no longer reachable: the endpoint
					// is a value captured before this job was made (ADR 39), so
					// the credential it holds cannot go away underneath the
					// loop. It used to be able to -- `Connect` re-read the
					// environment on every card -- and the branch is kept
					// because a sweep that somehow lost its endpoint should
					// still stop rather than make the same doomed call once per
					// slot. The reachable check is the `Require` above, before
					// the job is submitted at all.
					errs = append(errs, wire.KV{Key: name, Value: claude.Explain(runErr, endpoint.ModelFor(""))})
					for _, rest := range ordered[i+1:] {
						errs = append(errs, wire.KV{Key: rest,
							Value: "not attempted: the credential went away"})
					}
					rep.Report(total, total)
					break
				}
				if runErr != nil {
					// One bad call must not cost the rest of the sweep.
					errs = append(errs, wire.KV{Key: name, Value: claude.Explain(runErr, endpoint.ModelFor(""))})
				} else {
					reports = append(reports, report)
				}
				rep.Report(i+1, total)
			}
			return argueSweepResult(slug, true, "", total, reports, errs), nil
		},
	})
}
