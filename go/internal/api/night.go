package api

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/night"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The Coliseum at Night's two faces here (ADR 46): the real [night.BoutPlayer]
// — the seam's route-layer half, driving the same play-and-record core the
// interactive match drives — and the three admin routes the sample hour is
// started, watched and ended through. Nothing on this surface renders to a
// player: the routes live under the admin prefix, the jobs belong to the
// house, and the night's own reads wait for the Coliseum's shelf (its own PR).
//
// NightForgeKind is what the registry calls one night bout's match.
const NightForgeKind = "night.forge"

// SetNightRunner ties the knot the two-sided seam leaves: the runner needs
// this API's player, and the admin routes need that runner, so the door
// builds the API first, the runner second, and hands it back through here
// before the route table is read. Nil is an instance with no `app.db` — the
// night has nowhere to keep its rows, and the routes say so.
func (a *API) SetNightRunner(r *night.Runner) { a.nightRunner = r }

// NightPlayer is the real arena, as the runner's seam takes it.
func (a *API) NightPlayer() night.BoutPlayer { return nightPlayer{a} }

type nightPlayer struct{ a *API }

func (p nightPlayer) Play(ctx context.Context, b night.Bout) (int64, error) {
	return p.a.playNightBout(ctx, b)
}

// playNightBout fights one bout: resolve the seats, pre-flight, and drive
// [API.playForgeMatch] through a job on the one-wide FORGE lane — the same
// lane an interactive match takes, which is what makes the person-wins rule
// enforceable at all. It blocks until the match settles or ctx is done, per
// the seam's contract; a [night.Skip] return asks for a `skipped` row, every
// other error for a `failed` one.
func (a *API) playNightBout(ctx context.Context, b night.Bout) (int64, error) {
	if a.jobs == nil {
		return 0, errors.New("no job registry to fight the bout in")
	}
	if available, why := a.forgeStatus(); !available {
		// The environment's fault, not the pairing's: failed, so a fixed
		// arena replays the deck another night. The why names paths and
		// variables, so it goes to the log and the row gets a sentence.
		if why != nil {
			a.log.Error("the night found no arena", "why", *why)
		}
		return 0, errors.New("no arena to play in")
	}

	decks := make([]*deck.Deck, 0, 2)
	addresses := make([]string, 0, 2)
	ownerIDs := make([]*int64, 0, 2)
	for _, seat := range []night.Seat{b.SeatA, b.SeatB} {
		d, address, ownerID, err := a.nightDeck(ctx, seat)
		if err != nil {
			var missing library.ErrNotFound
			if errors.As(err, &missing) {
				// Withdrawn, entombed or deleted since the card was dealt —
				// the deck chose to leave, so the bout is skipped, not failed.
				return 0, night.Skip{Reason: address + " has left the library"}
			}
			return 0, err
		}
		if d.TotalCards() == 0 {
			// The one deck Forge cannot make a game out of — the same guard
			// the interactive route refuses with, as a skip.
			return 0, night.Skip{Reason: address + " has no cards in it"}
		}
		decks = append(decks, d)
		addresses = append(addresses, address)
		ownerIDs = append(ownerIDs, ownerID)
	}

	hosted := a.forge.Configured()
	if err := a.preflight(ctx, hosted, decks); err != nil {
		if errors.Is(err, tier3.ErrCoverageFailed) {
			// The pre-flight's own sentence names the counts and the cards,
			// which is exactly the diagnosis the row should hold.
			return 0, night.Skip{Reason: err.Error()}
		}
		return 0, err
	}

	plural := "s"
	if b.Games == 1 {
		plural = ""
	}
	m := forgeMatch{decks: decks, addresses: addresses, ownerIDs: ownerIDs,
		games: b.Games, seed: big.NewInt(b.Seed), hosted: hosted}
	// The settle rides a channel out of the closure because the registry's
	// contract is a polled payload, and the runner is not a poller. Buffered:
	// the job must never block on a waiter that gave up at shutdown.
	type outcome struct {
		matchID int64
		err     error
	}
	settled := make(chan outcome, 1)
	plan := jobs.Plan{Kind: NightForgeKind, Lane: jobs.FORGE,
		Label: fmt.Sprintf("Night: %s vs %s, %d game%s",
			b.SeatA.Slug, b.SeatB.Slug, b.Games, plural),
		// Keyed per bout id. Bout ids are unique and a bout is claimed once,
		// so no second submit can exist to join — the key is a guard against
		// a retry bug ever paying for the same bout twice, not a feature.
		Key: fmt.Sprintf("night|%d", b.ID),
		Run: func(rep jobs.Progress) (any, error) {
			out, matchID, err := a.playForgeMatch(rep, m)
			settled <- outcome{matchID, err}
			if err != nil {
				return nil, err
			}
			return out, nil
		},
	}
	if _, err := a.jobs.FromPlan(plan, jobs.HouseOwner); err != nil {
		return 0, err
	}
	select {
	case o := <-settled:
		return o.matchID, o.err
	case <-ctx.Done():
		// The runner is stopping. The job fights on in the registry — it has
		// never had cancellation — and the row stays `playing` for the next
		// boot's sweep, which is the seam's documented shape.
		return 0, ctx.Err()
	}
}

// nightDeck resolves one seat to its deck: the house's off the file tier,
// a player's out of their own SQL-tier library — no shared-only veil,
// because rung 13's flag is the owner's standing consent to exactly this
// read. The address is log-grade, for rows and labels nobody but the house
// and the admin ever sees.
func (a *API) nightDeck(ctx context.Context, seat night.Seat) (*deck.Deck, string, *int64, error) {
	if seat.House() {
		d, err := library.NewFileSource(a.decksDir, false).Get(ctx, seat.Slug)
		return d, "the house's " + seat.Slug, nil, err
	}
	address := fmt.Sprintf("account %d's %s", *seat.Owner, seat.Slug)
	db, present := a.accountsDB()
	if !present {
		return nil, address, nil, errors.New("no app.db to read a player's deck from")
	}
	owner := *seat.Owner
	d, err := library.NewSQLSource(db, nil, owner, false, false).Get(ctx, seat.Slug)
	return d, address, &owner, err
}

// ---- the three admin routes (ADR 17: 403 by prefix, requireAdmin behind) --

// adminNightSample is `POST /api/admin/night/sample`: open a measurement run
// for the asked-for minutes, right now. This is how the window gets chosen —
// Aaron runs the night for an hour, counts bouts, and configures what the
// count argues for — so it must work on an instance with no window set.
func (a *API) adminNightSample(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if a.nightRunner == nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"the night has nowhere to keep its rows on this instance")
		return
	}
	minutes, err := nightMinutes(body)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	run, bouts, err := a.nightRunner.StartSample(r.Context(), minutes)
	if err != nil {
		if errors.Is(err, night.ErrRunOpen) {
			wire.Detail(w, http.StatusConflict,
				"a night is already open — watch it on GET /api/admin/night, or close it first")
			return
		}
		a.refuse(w, "night", err)
		return
	}
	a.log.Info("a sample night opened", "run", run.ID, "minutes", minutes,
		"bouts", bouts, "by", a.actor(r.Context()))
	wire.JSON(w, http.StatusCreated, wire.OrderedMap{
		{Key: "run_id", Value: run.ID},
		{Key: "closes_at", Value: textutil.Isoformat(run.ClosesAt)},
		{Key: "bouts", Value: bouts},
	})
}

// nightMinutes reads the sample's one dial through the recorded integer
// grammar, bounded so a typo cannot park the arena for a week.
func nightMinutes(body map[string]any) (int, error) {
	raw, given := body["minutes"]
	if !given || raw == nil {
		return 0, errors.New("say how long: minutes, a whole number from 1 to 180")
	}
	n, err := claude.IntValue(raw)
	if err != nil || !n.IsInt64() || n.Int64() < 1 || n.Int64() > 180 {
		return 0, errors.New("minutes runs from 1 to 180")
	}
	return int(n.Int64()), nil
}

// adminNightClose is `POST /api/admin/night/close`: pull the open run's close
// up to now. The bout in flight still finishes (ADR 46 decision 6); the
// nudged tick settles the remainder and declares the night over.
func (a *API) adminNightClose(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	if a.nightRunner == nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"the night has nowhere to keep its rows on this instance")
		return
	}
	store := a.nightRunner.Store()
	run, ok, err := store.OpenRun(r.Context())
	if err != nil {
		a.refuse(w, "night", err)
		return
	}
	if !ok {
		wire.Detail(w, http.StatusNotFound, "no night is open")
		return
	}
	if err := store.CloseRun(r.Context(), run.ID); err != nil {
		a.refuse(w, "night", err)
		return
	}
	a.nightRunner.Nudge()
	closed, ok, err := store.LatestRun(r.Context())
	if err != nil || !ok {
		a.refuse(w, "night", err)
		return
	}
	a.log.Info("the night was closed early", "run", run.ID, "by", a.actor(r.Context()))
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "run_id", Value: closed.ID},
		{Key: "closes_at", Value: textutil.Isoformat(closed.ClosesAt)},
	})
}

// adminNight is `GET /api/admin/night`: the open run — or the most recent,
// once the night is over — with its card and the tally. The watching read
// for the sample hour, refreshed by whoever is counting. Admin-only wire,
// deliberately plain: owner ids and slugs, states and log-grade reasons —
// none of it is commandment-10 territory, and none of it renders to a player.
func (a *API) adminNight(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	if a.nightRunner == nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"the night has nowhere to keep its rows on this instance")
		return
	}
	store := a.nightRunner.Store()
	run, ok, err := store.OpenRun(r.Context())
	if err != nil {
		a.refuse(w, "night", err)
		return
	}
	if !ok {
		if run, ok, err = store.LatestRun(r.Context()); err != nil {
			a.refuse(w, "night", err)
			return
		}
	}
	if !ok {
		wire.Detail(w, http.StatusNotFound, "no night has run yet")
		return
	}
	bouts, err := store.Bouts(r.Context(), run.ID)
	if err != nil {
		a.refuse(w, "night", err)
		return
	}

	var finished any
	if run.FinishedAt != nil {
		finished = textutil.Isoformat(*run.FinishedAt)
	}
	tally := map[night.State]int{}
	card := make([]wire.OrderedMap, 0, len(bouts))
	for _, b := range bouts {
		tally[b.State]++
		var reason any
		if b.Reason != "" {
			reason = b.Reason
		}
		var match any
		if b.MatchID != nil {
			match = *b.MatchID
		}
		card = append(card, wire.OrderedMap{
			{Key: "id", Value: b.ID},
			{Key: "seat_a", Value: nightSeat(b.SeatA)},
			{Key: "seat_b", Value: nightSeat(b.SeatB)},
			{Key: "games", Value: b.Games},
			{Key: "seed", Value: b.Seed},
			{Key: "state", Value: string(b.State)},
			{Key: "reason", Value: reason},
			{Key: "match_id", Value: match},
		})
	}
	counts := wire.OrderedMap{}
	for _, state := range []night.State{night.StatePlanned, night.StatePlaying,
		night.StateDone, night.StateFailed, night.StateSkipped} {
		counts = append(counts, wire.KV{Key: string(state), Value: tally[state]})
	}
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "run", Value: wire.OrderedMap{
			{Key: "id", Value: run.ID},
			{Key: "night_key", Value: run.NightKey},
			{Key: "sample", Value: run.Sample},
			{Key: "opened_at", Value: textutil.Isoformat(run.OpenedAt)},
			{Key: "closes_at", Value: textutil.Isoformat(run.ClosesAt)},
			{Key: "finished_at", Value: finished},
		}},
		{Key: "tally", Value: counts},
		{Key: "bouts", Value: card},
	})
}

func nightSeat(s night.Seat) wire.OrderedMap {
	var owner any
	if s.Owner != nil {
		owner = *s.Owner
	}
	return wire.OrderedMap{
		{Key: "owner", Value: owner},
		{Key: "slug", Value: s.Slug},
	}
}
