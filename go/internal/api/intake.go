package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckedit"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// IntakeKind is what `/api/jobs` calls one of these.
const IntakeKind = "claude.intake"

// The intake sheet: what a freshly imported deck may be offered (ADR 41).
//
// **This file is where ADR 41's narrowing actually lives**, and it is worth
// being blunt about that. Nothing in `internal/claude` changed: every mode is
// still read-only, `may_write` is still empty everywhere, and
// `boundary_test.go` still bans that whole tree from the write engine. What is
// new is here -- a caller that takes what a mode answered and writes it -- and
// the two gates ADR 41 requires are both checked in this file, one of them
// twice.
//
// The five actions, and which of them writes:
//
//   - `rationales` writes a `why` and marks it `why_by: claude`. **The one
//     that needed an ADR.** Gated on the toggle AND on the stance's write axis.
//   - `categories` writes a `category`. Allowed since ADR 13 and simply never
//     built; a category has always been a field a person sets and this sets it
//     for them, which is a smaller claim than a rationale by some distance.
//   - `description` writes the deck's `notes.gameplan` and its `themes`.
//   - `dossier` writes nothing. It fills the deck's cached dossier.
//   - `argue` writes nothing. It is the existing slot sweep, run at intake.
//
// **It is one job, not five.** A person who ticked four boxes wants one thing
// to watch, and the actions have to run in an order -- filing before arguing,
// because a sweep that reads the categories should read the new ones -- which
// five independent jobs could not promise.
//
// **Every action is independent of every other one's failure.** A dossier that
// times out must not cost the ninety rationales that already landed, so each
// action reports its own outcome and the job's result is the five of them.

// intakeActions is the sheet, as the request sends it.
type intakeActions struct {
	Rationales  bool
	Categories  bool
	Description bool
	Dossier     bool
	Argue       bool
}

func (a intakeActions) any() bool {
	return a.Rationales || a.Categories || a.Description || a.Dossier || a.Argue
}

// intakeDeck is `POST /api/decks/{owner}/{slug}/intake`.
//
// A write route, so it goes through `writeTarget`: a deck this caller cannot
// see is a 404 (ADR 5), and a read-only source is a 403, both decided before
// anything here reads an action.
func (a *API) intakeDeck(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	actions := intakeActions{
		Rationales:  truthy(body["rationales"]),
		Categories:  truthy(body["categories"]),
		Description: truthy(body["description"]),
		Dossier:     truthy(body["dossier"]),
		Argue:       truthy(body["argue"]),
	}
	if !actions.any() {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"nothing was asked for; tick at least one thing for the intake to do")
		return
	}

	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")

	var requested any
	if truthy(body["stance"]) {
		requested = body["stance"]
	}
	effective, err := claude.Resolve(requested, claude.DeckWithStatus(d.Status), a.claude.Ceiling)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// **ADR 41's second gate, checked here and again before the write.**
	//
	// Refused rather than silently dropped: somebody who ticked the box and
	// got a deck full of blanks back would reasonably conclude the feature is
	// broken, and the honest answer names the setting that decided it. The UI
	// does not offer the toggle at this stance, so reaching this is either a
	// stale page or a direct call -- and both deserve the sentence.
	if actions.Rationales && !effective.MayWrite() {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"drafting rationales needs a stance that allows a write; yours is set "+
				"to change nothing, so nothing here will write a `why` for you")
		return
	}

	if !effective.AllowsCalls() {
		a.submit(w, r, jobs.Plan{Kind: IntakeKind, Lane: jobs.NET,
			Label:  "intake: " + slug,
			Result: intakeResult(slug, false, claude.DialNever, nil)})
		return
	}
	if err := a.claude.Endpoint.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Captured at plan time with everything else the job carries: an intake
	// outlives the request that planned it by minutes (ADR 39).
	tier := auth.ScopeFrom(r.Context()).ModelTier
	ledgerOf := a.claudeLedger
	endpoint := a.claude.Endpoint
	ceiling := a.claude.Ceiling
	owner := src.OwnerID()
	actor := a.actor(r.Context())

	a.submit(w, r, jobs.Plan{
		Kind: IntakeKind, Lane: jobs.NET, Label: "intake: " + slug,
		// The slug plus what was asked for: a double-click joins the intake in
		// flight, and a different sheet is different work.
		Key: slug + ":" + intakeKey(actions),
		Run: func(rep jobs.Progress) (any, error) {
			ctx := context.Background()
			run := &intakeRun{
				api: a, src: src, slug: slug, deck: d, owner: owner, actor: actor,
				req: claude.IntakeRequest{
					Endpoint: endpoint, Limit: ceiling, Requested: requested,
					Tier: tier, Ledger: ledgerOf,
				},
				stance: effective,
			}
			return run.all(ctx, actions, rep)
		},
	})
}

// intakeRun is one intake in flight, and the state the five actions share.
//
// `deck` is re-read after every action that wrote, because the next action
// reads it: filing writes categories that the argue sweep should see, and
// drafting writes rationales the description should be able to quote. A run
// that held one snapshot would describe the deck it started with.
type intakeRun struct {
	api    *API
	src    library.Source
	slug   string
	deck   *deck.Deck
	owner  *int64
	actor  string
	req    claude.IntakeRequest
	stance claude.Stance
}

// all runs the sheet in order and reports each action's own outcome.
//
// The order is not arbitrary. Filing first, because every later action reads
// better categories. Rationales next, because the description should be able
// to hear how the deck talks about itself. The two that write nothing last,
// because they are the slowest and a person watching progress would rather see
// the deck change early.
func (run *intakeRun) all(ctx context.Context, actions intakeActions,
	rep jobs.Progress) (any, error) {

	steps := 0
	for _, on := range []bool{actions.Categories, actions.Rationales,
		actions.Description, actions.Dossier, actions.Argue} {
		if on {
			steps++
		}
	}
	done := 0
	rep.Report(0, steps)
	step := func() {
		done++
		rep.Report(done, steps)
	}

	out := []wire.KV{}
	record := func(name string, value any) {
		out = append(out, wire.KV{Key: name, Value: value})
		step()
	}

	if actions.Categories {
		record("categories", run.file(ctx))
	}
	if actions.Rationales {
		record("rationales", run.draft(ctx))
	}
	if actions.Description {
		record("description", run.describe(ctx))
	}
	if actions.Dossier {
		record("dossier", run.dossier(ctx))
	}
	if actions.Argue {
		record("argue", run.argue(ctx))
	}
	return intakeResult(run.slug, true, "", out), nil
}

// reload re-reads the deck after a write, so the next action sees it.
func (run *intakeRun) reload(ctx context.Context) {
	if d, err := run.src.Get(ctx, run.slug); err == nil {
		run.deck = d
	}
}

// write applies one editor operation per card and writes once.
//
// One write for the whole pass rather than one per card, which is the
// difference between a deck file rewritten ninety-nine times and a deck file
// rewritten once. A card whose operation is refused is counted and skipped:
// `DraftRationale` refuses a card that already has a rationale, and that
// refusal is a normal outcome here rather than a fault.
func (run *intakeRun) write(ctx context.Context, field string,
	names []string, apply func(text, name string) (string, error)) (int, error) {

	writer, err := library.WriterFor(run.src, run.slug)
	if err != nil {
		return 0, err
	}
	text, err := run.src.ReadText(ctx, run.slug)
	if err != nil {
		return 0, err
	}
	written := []string{}
	for _, name := range names {
		updated, err := apply(text, name)
		if err != nil {
			// A refusal costs its own card. The commonest one is a card that
			// already has a rationale, which is the guard working.
			continue
		}
		text = updated
		written = append(written, name)
	}
	if len(written) == 0 {
		return 0, nil
	}
	if err := writer.WriteText(ctx, run.slug, text); err != nil {
		return 0, err
	}
	// ADR 28: which cards, never what was written. The text here is a
	// rationale, which is the one thing this log has never carried.
	run.api.recorder().Record(ctx, run.slug, run.owner, run.actor,
		decklog.Edit{Kind: decklog.EditIntake, Field: field, Cards: written})
	run.reload(ctx)
	return len(written), nil
}

// file is the categories pass.
func (run *intakeRun) file(ctx context.Context) wire.OrderedMap {
	// Only the cards still sitting on the importer's default. A card somebody
	// already filed is a decision, and re-filing it would overwrite a person's
	// judgement with a model's -- the same rule the rationale pass follows,
	// for the same reason.
	wanted := []string{}
	for _, c := range run.deck.Cards {
		if c.Category == "utility" {
			wanted = append(wanted, c.Name)
		}
	}
	if len(wanted) == 0 {
		return intakeStep(0, 0, "Every card was already filed, so nothing was changed.")
	}

	var filings []claude.Filing
	err := run.api.withPool(ctx, func(c *pool.Conn) error {
		req := run.req
		req.Deps = tools.Deps{Source: run.src, Pool: c}
		var outcome claude.IntakeOutcome
		var err error
		filings, outcome, err = claude.FileCards(ctx, run.deck, wanted, req)
		_ = outcome
		return err
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}

	byName := map[string]string{}
	names := make([]string, 0, len(filings))
	for _, f := range filings {
		byName[f.Card] = f.Category
		names = append(names, f.Card)
	}
	n, err := run.write(ctx, "category", names, func(text, name string) (string, error) {
		return deckedit.SetCardField(text, name, "category", byName[name])
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	return intakeStep(n, len(wanted), "")
}

// draft is the rationales pass, and the one ADR 41 exists for.
func (run *intakeRun) draft(ctx context.Context) wire.OrderedMap {
	// **The write gate, a second time.** It was checked at the route, and it
	// is checked again here because this is the last place before the write
	// and because the two checks answer different questions: the route's
	// refuses the request, this one refuses the write. A stance that somehow
	// reached here without a write level gets nothing rather than a deck full
	// of drafts.
	if !run.stance.MayWrite() {
		return intakeStep(0, 0, "The stance does not allow a write, so nothing was drafted.")
	}
	wanted := []string{}
	for _, c := range run.deck.Cards {
		if strings.TrimSpace(c.Why) == "" {
			wanted = append(wanted, c.Name)
		}
	}
	if len(wanted) == 0 {
		return intakeStep(0, 0, "Every card already had a reason, so nothing was drafted.")
	}

	var drafts []claude.Draft
	err := run.api.withPool(ctx, func(c *pool.Conn) error {
		req := run.req
		req.Deps = tools.Deps{Source: run.src, Pool: c}
		var err error
		drafts, _, err = claude.DraftRationales(ctx, run.deck, wanted, req)
		return err
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}

	byName := map[string]string{}
	names := make([]string, 0, len(drafts))
	for _, dr := range drafts {
		byName[dr.Card] = dr.Why
		names = append(names, dr.Card)
	}
	n, err := run.write(ctx, "why", names, func(text, name string) (string, error) {
		return deckedit.DraftRationale(text, name, byName[name])
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	return intakeStep(n, len(wanted), "")
}

// describe writes the deck's game plan and its themes.
func (run *intakeRun) describe(ctx context.Context) wire.OrderedMap {
	// Left alone if the deck already says what it is doing, by either route:
	// its own `strategy`, or the note this writes.
	if strategy, ok := run.deck.Strategy.(string); ok && strings.TrimSpace(strategy) != "" {
		return intakeStep(0, 0, "The deck already had a description, which was left alone.")
	}
	for _, note := range run.deck.Notes {
		if note.Key == "gameplan" && strings.TrimSpace(fmt.Sprint(note.Value)) != "" {
			return intakeStep(0, 0, "The deck already had a game plan, which was left alone.")
		}
	}
	var got claude.Description
	err := run.api.withPool(ctx, func(c *pool.Conn) error {
		req := run.req
		req.Deps = tools.Deps{Source: run.src, Pool: c}
		var err error
		got, _, err = claude.DescribeDeck(ctx, run.deck, req)
		return err
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	if strings.TrimSpace(got.Strategy) == "" {
		return intakeStep(0, 0, "No description came back, so the deck keeps the one it had.")
	}

	writer, err := library.WriterFor(run.src, run.slug)
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	text, err := run.src.ReadText(ctx, run.slug)
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	// **`notes.gameplan` and not the top-level `strategy`, and that is not a
	// near miss.** `strategy` has a place in the file's key order and no
	// editor operation that writes it -- `SettableDeckFields` leaves it out
	// deliberately, on the grounds that prose belongs to `SetNote`. So the
	// description goes where the deck file already keeps an author's prose,
	// under the key the generated primer already renders as the game plan.
	//
	// Found by an end-to-end test reading the file back rather than by
	// reading the editor: the write was refused, the step reported a failure
	// note, and every other assertion in the test still passed.
	updated, err := deckedit.SetNote(text, "gameplan", got.Strategy)
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	if len(got.Themes) > 0 {
		if withThemes, err := deckedit.SetDeckField(updated, "themes", got.Themes); err == nil {
			updated = withThemes
		}
	}
	if err := writer.WriteText(ctx, run.slug, updated); err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	run.api.recorder().Record(ctx, run.slug, run.owner, run.actor,
		decklog.Edit{Kind: decklog.EditNote, Note: "gameplan"})
	run.reload(ctx)
	return intakeStep(1, 1, "")
}

// dossier fills the deck's cached commander dossier. It writes nothing to the
// deck file -- the dossier lives in its own store -- so it needs no gate
// beyond the stance every Claude call already answers to.
func (run *intakeRun) dossier(ctx context.Context) wire.OrderedMap {
	req := claude.DossierRequest{
		Requested: run.req.Requested,
		Tier:      run.req.Tier,
		Store:     run.api.dossierStore(),
		Limit:     run.req.Limit,
		Endpoint:  run.req.Endpoint,
	}
	var plan *claude.DossierPlan
	err := run.api.withPool(ctx, func(c *pool.Conn) error {
		var checkErr error
		plan, checkErr = claude.CheckDossier(ctx, c, run.slug, run.deck, req)
		return checkErr
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	if plan.Answer != nil {
		// Already stored, or a stance that makes no call. Both are real
		// answers and neither costs anything.
		return intakeStep(0, 1, "The dossier was already written, so it was not asked for again.")
	}
	err = run.api.leasePool(ctx, func(c *pool.Conn) error {
		_, runErr := claude.RunDossier(ctx, c, plan, claude.DossierRun{
			Deps:   tools.Deps{Source: run.src, Pool: c},
			Ledger: run.req.Ledger,
		})
		return runErr
	})
	if err != nil {
		return intakeFailed(err, run.req.Endpoint)
	}
	return intakeStep(1, 1, "")
}

// argue runs the existing slot sweep over the 99 (ADR 25), writing nothing.
//
// It is last and it is the slowest, because it is one call per card by
// construction: an argument is about one slot and batching it would be the
// balanced answer ADR 25 exists to prevent, wearing a different hat.
func (run *intakeRun) argue(ctx context.Context) wire.OrderedMap {
	names := make([]string, 0, len(run.deck.Cards))
	for _, c := range run.deck.Cards {
		if c.Category != "land" {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return intakeStep(0, 0, "There was nothing outside the mana base to argue about.")
	}
	made := 0
	for _, name := range names {
		err := run.api.withPool(ctx, func(c *pool.Conn) error {
			req := run.req
			_, argueErr := claude.Argue(ctx, c, run.deck, name, claude.ArgueRequest{
				Endpoint: req.Endpoint, Limit: req.Limit, Requested: req.Requested,
				Deps: tools.Deps{Source: run.src, Pool: c},
				Tier: req.Tier, Ledger: req.Ledger,
			})
			return argueErr
		})
		if errors.Is(err, claude.ErrUnavailable) {
			// The credential went away mid-sweep. Stopping beats making the
			// same doomed call once per remaining slot.
			return intakeStep(made, len(names),
				"The sweep stopped part way through and the rest was not attempted.")
		}
		if err == nil {
			made++
		}
	}
	return intakeStep(made, len(names), "")
}

// intakeStep is one action's outcome: what it changed, out of what it looked
// at, and a sentence when the number alone would read as a failure.
func intakeStep(changed, looked int, note string) wire.OrderedMap {
	out := wire.OrderedMap{
		{Key: "changed", Value: changed},
		{Key: "considered", Value: looked},
	}
	if note != "" {
		out = append(out, wire.KV{Key: "note", Value: note})
	}
	return out
}

// intakeFailed renders one action's failure in words a player can read.
//
// `claude.Explain` is the only place a call failure becomes a sentence, and it
// is used here for the same reason `api.forgeTrouble` exists: the diagnosis
// goes to the log, and what reaches the page never names the machinery
// (commandment 10).
func intakeFailed(err error, endpoint claude.Endpoint) wire.OrderedMap {
	return wire.OrderedMap{
		{Key: "changed", Value: 0},
		{Key: "considered", Value: 0},
		{Key: "note", Value: claude.Explain(err, endpoint.ModelFor(""))},
	}
}

// intakeResult is the job's answer.
func intakeResult(slug string, asked bool, reason string, steps []wire.KV) wire.OrderedMap {
	out := wire.OrderedMap{
		{Key: "slug", Value: slug},
		{Key: "asked", Value: asked},
	}
	if reason != "" {
		out = append(out, wire.KV{Key: "reason", Value: reason})
	}
	if steps == nil {
		steps = []wire.KV{}
	}
	return append(out, wire.KV{Key: "steps", Value: wire.OrderedMap(steps)})
}

// intakeKey is what this sheet IS, for job de-duplication: the actions that
// were ticked, in a fixed order so two spellings of the same sheet are one
// piece of work.
func intakeKey(a intakeActions) string {
	on := []string{}
	for _, row := range []struct {
		name string
		set  bool
	}{
		{"rationales", a.Rationales}, {"categories", a.Categories},
		{"description", a.Description}, {"dossier", a.Dossier}, {"argue", a.Argue},
	} {
		if row.set {
			on = append(on, row.name)
		}
	}
	slices.Sort(on)
	return strings.Join(on, "+")
}
