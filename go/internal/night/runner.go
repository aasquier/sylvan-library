package night

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// The runner: the app's first scheduler (ADR 46 decision 3), and deliberately
// the dullest one that can work. A ticker wakes every minute, asks the store
// what tonight owes, does at most one thing, and goes back to sleep. Every
// question is answered from rows, so a tick after a restart resumes the night
// a tick before it was holding — the loop carries no plan of its own, only a
// clock and the id set of bouts *this process* put in flight, which is
// exactly the memory a restart is allowed to lose (its loss is what the
// orphan sweep reads).
//
// Three rules the loop keeps, all ADR 46's:
//
//   - **The person in the room wins** (decision 4). The night submits into
//     the same one-wide arena lane an interactive match uses, and it defers
//     the moment that lane holds anyone's work — a nightly batch that makes
//     somebody watch a spinner has taken the room's purpose away to feed a
//     statistic about it.
//   - **A bout in flight finishes** (decision 6). The window closing stops
//     new bouts; it never abandons the one playing, whose JVM minutes are
//     already spent. The remainder is skipped with the reason on each row.
//   - **An orphan is failed, never replayed.** A bout left `playing` by a
//     restart may or may not have recorded its match — the truth is gone
//     with the job registry's memory — so it is marked failed with
//     `match_id` NULL, and never resubmitted blind (the same match recorded
//     twice would be worse than one honest hole).

// BoutPlayer is the seam the actual fighting hides behind, so this package
// never imports the route layer and every runner test fakes the arena.
type BoutPlayer interface {
	// Play fights one bout to its end: resolve the seats, run the match,
	// record it, and return the recorded `forge_matches` id — or 0 when the
	// match played and the ledger declined the row, which is that ledger's
	// own contract. A [Skip] error asks for a `skipped` row instead of a
	// `failed` one. **Play must return when ctx is done** — the runner's
	// stop leans on that promise, and a player that outlives it is the leak
	// the shutdown tests exist to catch.
	Play(ctx context.Context, b Bout) (matchID int64, err error)
}

// Skip is the refusal a player answers when a bout should be recorded
// skipped rather than failed: a pre-flight that said no, a seat whose deck
// has left the library. The reason is log-grade words for the row.
type Skip struct{ Reason string }

func (s Skip) Error() string { return s.Reason }

// RunnerConfig is what a runner needs. Store and Player are required; every
// other zero value is a working default.
type RunnerConfig struct {
	Store    *Store
	Settings Settings
	Player   BoutPlayer
	// LaneBusy reports whether the arena's one lane holds anyone's work —
	// queued or running, the night's own included. Nil never defers, which
	// is only for tests: the served process wires the job registry's answer.
	LaneBusy func() bool
	// House lists the house's decks — every file-tier slug, no flag
	// consulted, because the house always plays. Nil is an empty house.
	House func(ctx context.Context) ([]string, error)
	// Log, or slog.Default().
	Log *slog.Logger
	// Now is the clock, for the same reason the store's is a field. Nil is
	// the wall's.
	Now func() time.Time
	// Interval is the ticker's, zero taking the minute ADR 46 sketched.
	Interval time.Duration
}

// Runner is the loop and the one piece of memory it is allowed: which bouts
// this process is waiting on.
type Runner struct {
	store    *Store
	set      Settings
	player   BoutPlayer
	laneBusy func() bool
	house    func(ctx context.Context) ([]string, error)
	log      *slog.Logger
	now      func() time.Time
	interval time.Duration

	// ctx is the runner's lifetime: Play calls take it, the ticker selects
	// on it, and Stop cancels it. Held on the struct because the goroutines
	// it governs outlive any one call.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	// nudge wakes the loop early — a settled bout wants the next one
	// submitted now, not up to a minute from now. Buffered one deep: a nudge
	// while one is pending is the same nudge.
	nudge chan struct{}

	mu       sync.Mutex
	inFlight map[int64]bool
}

// NewRunner builds a runner. It does not start the ticker — [Runner.Start]
// does, so a caller that only wants [Runner.StartSample] and the tick-driven
// reads never spawns a goroutine it has to stop.
func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.LaneBusy == nil {
		cfg.LaneBusy = func() bool { return false }
	}
	if cfg.House == nil {
		cfg.House = func(context.Context) ([]string, error) { return nil, nil }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{store: cfg.Store, set: cfg.Settings, player: cfg.Player,
		laneBusy: cfg.LaneBusy, house: cfg.House, log: cfg.Log, now: cfg.Now,
		interval: cfg.Interval, ctx: ctx, cancel: cancel,
		nudge: make(chan struct{}, 1), inFlight: map[int64]bool{}}
}

// Start spawns the ticker. One call; the loop runs until [Runner.Stop].
func (r *Runner) Start() {
	r.wg.Go(func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
			case <-r.nudge:
			}
			r.Tick(r.ctx)
		}
	})
}

// Stop ends the runner and waits for its goroutines — the ticker, and any
// waiter parked on a bout, which [BoutPlayer.Play]'s contract frees promptly.
// The bout's own job is the registry's and keeps running (that registry has
// never had cancellation); the row it would have settled stays `playing`,
// which is the honest orphan the next boot's sweep fails. Safe to call twice,
// and safe on a runner that never started.
func (r *Runner) Stop() {
	r.cancel()
	r.wg.Wait()
}

// Nudge wakes the loop for an early tick — how a freshly opened sample run
// starts fighting now rather than a minute from now. A nudge nobody is
// looping on is dropped, so it is safe before Start and after Stop.
func (r *Runner) Nudge() {
	select {
	case r.nudge <- struct{}{}:
	default:
	}
}

// Tick is one beat of the night, and the whole scheduler: resume the open
// run if there is one, else open tonight's if the window says so, else rest.
// Errors are logged rather than returned — a tick answers to nobody, and the
// next one asks the same questions from the same rows.
func (r *Runner) Tick(ctx context.Context) {
	run, ok, err := r.store.OpenRun(ctx)
	if err != nil {
		r.log.Error("the night could not read its own run", "error", err)
		return
	}
	if !ok {
		run, ok = r.openTonight(ctx)
		if !ok {
			return
		}
	}
	r.work(ctx, run)
}

// openTonight opens the scheduled run when the window is open and tonight has
// not already happened — the second check is what makes a restart inside the
// window resume instead of re-run, and what stops a finished night reopening
// while its window is still open.
func (r *Runner) openTonight(ctx context.Context) (Run, bool) {
	if !r.set.Scheduled {
		return Run{}, false
	}
	open, key, closes := r.set.Window.At(r.now().In(r.set.Zone))
	if !open {
		return Run{}, false
	}
	has, err := r.store.HasScheduledRun(ctx, key)
	if err != nil {
		r.log.Error("the night could not ask whether tonight has happened",
			"error", err)
		return Run{}, false
	}
	if has {
		return Run{}, false
	}
	house, players, err := r.roster(ctx)
	if err != nil {
		r.log.Error("the night could not muster its roster", "error", err)
		return Run{}, false
	}
	plans := PlanScheduled(key, house, players, r.set)
	run, err := r.store.StartRun(ctx, key, false, closes)
	if err != nil {
		// A second opener beat this one to it — another tick, an admin's
		// sample. Either way tonight is somebody else's to work now.
		r.log.Warn("the night did not open", "night", key, "error", err)
		return Run{}, false
	}
	if err := r.store.PlanBouts(ctx, run.ID, plans); err != nil {
		// The run is open with no card. It will idle to its close and finish
		// empty — loud here, honest in the rows.
		r.log.Error("the night opened but its card did not write",
			"run", run.ID, "error", err)
		return run, true
	}
	r.log.Info("the night opened", "run", run.ID, "night", key,
		"bouts", len(plans), "closes", run.ClosesAt)
	return run, true
}

// roster is the two halves of the draw: the house through the seam, the
// players through rung 13's index.
func (r *Runner) roster(ctx context.Context) ([]string, []Seat, error) {
	house, err := r.house(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing the house's decks: %w", err)
	}
	players, err := r.store.PlayerDecks(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing the players' decks: %w", err)
	}
	return house, players, nil
}

// work advances one open run by at most one step.
func (r *Runner) work(ctx context.Context, run Run) {
	// The orphan sweep. A row playing while this process waits on nothing is
	// a bout some earlier process died holding: the job's memory is gone, so
	// failed-with-the-reason is the honest state, and `match_id` stays NULL
	// because whether the match recorded is genuinely unknown. Never
	// resubmitted — see the package comment. The sweep is skipped entirely
	// while a local waiter exists, because the store holds one bout in
	// flight at a time and that one is ours.
	if r.playing() == 0 {
		n, err := r.store.FailPlaying(ctx, run.ID, "the process restarted mid-bout")
		if err != nil {
			r.log.Error("the orphan sweep failed", "run", run.ID, "error", err)
			return
		}
		if n > 0 {
			r.log.Warn("orphaned bouts were failed", "run", run.ID, "bouts", n)
		}
	}

	if !r.now().Before(run.ClosesAt) {
		// The window has closed. A bout in flight finishes (decision 6) —
		// its settle will nudge a tick that lands back here.
		if r.playing() > 0 {
			return
		}
		skipped, err := r.store.SkipRemaining(ctx, run.ID, "the window closed")
		if err != nil {
			r.log.Error("the remainder would not skip", "run", run.ID, "error", err)
			return
		}
		if err := r.store.FinishRun(ctx, run.ID); err != nil {
			r.log.Error("the night would not finish", "run", run.ID, "error", err)
			return
		}
		r.log.Info("the night finished", "run", run.ID, "skipped", skipped)
		return
	}

	if r.playing() > 0 {
		return // one bout at a time; its settle nudges the next
	}
	if r.laneBusy() {
		return // the person in the room wins (decision 4)
	}
	bout, ok, err := r.store.ClaimNext(ctx, run.ID)
	if err != nil {
		r.log.Error("the next bout would not claim", "run", run.ID, "error", err)
		return
	}
	if !ok {
		return // the card is settled; the close will finish the night
	}
	if reason, withdrawn := r.consentWithdrawn(ctx, bout); withdrawn {
		if err := r.store.MarkSkipped(ctx, bout.ID, reason); err != nil {
			r.log.Error("a withdrawn bout would not settle", "bout", bout.ID, "error", err)
			return
		}
		r.Nudge() // the seat is free now; the next bout need not wait a minute
		return
	}
	r.fight(bout)
}

// consentWithdrawn re-checks rung 13's flag at the bout's own turn — the deal
// read it at run open, but the flag is standing consent and an owner may have
// stepped back out since; their deck is skipped unread rather than fought on
// how the flag looked hours ago ([Store.Entered] carries the argument). A
// failed read plays on: the deal-time consent stands recorded, and a store
// this broken will fail the bout honestly on its own.
func (r *Runner) consentWithdrawn(ctx context.Context, b Bout) (string, bool) {
	for _, seat := range []Seat{b.SeatA, b.SeatB} {
		if seat.House() {
			continue // the house always plays
		}
		in, err := r.store.Entered(ctx, *seat.Owner, seat.Slug)
		if err != nil {
			r.log.Error("the consent re-check would not read", "bout", b.ID, "error", err)
			return "", false
		}
		if !in {
			return fmt.Sprintf("account %d's %s has left the night",
				*seat.Owner, seat.Slug), true
		}
	}
	return "", false
}

// fight hands one claimed bout to the player and parks a waiter on the
// answer. The waiter settles the row *before* untracking it — the other
// order would open a window where the sweep reads "playing, nobody waiting"
// on a bout that is a millisecond from being done.
func (r *Runner) fight(b Bout) {
	r.mu.Lock()
	r.inFlight[b.ID] = true
	r.mu.Unlock()
	r.wg.Go(func() {
		matchID, err := r.player.Play(r.ctx, b)
		if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
			// The runner is stopping. The job may still be fighting in the
			// registry, so the row stays `playing` — the honest orphan the
			// next boot's sweep fails.
			r.untrack(b.ID)
			return
		}
		// The settle owes nothing to the runner's context: it is a local
		// write recording work already done, and it must land even when the
		// answer arrived in the same breath as a stop.
		ctx := context.Background()
		var skip Skip
		switch {
		case err == nil:
			if e := r.store.MarkDone(ctx, b.ID, matchID); e != nil {
				r.log.Error("a finished bout would not settle", "bout", b.ID, "error", e)
			}
		case errors.As(err, &skip):
			if e := r.store.MarkSkipped(ctx, b.ID, skip.Reason); e != nil {
				r.log.Error("a skipped bout would not settle", "bout", b.ID, "error", e)
			}
		default:
			if e := r.store.MarkFailed(ctx, b.ID, err.Error()); e != nil {
				r.log.Error("a failed bout would not settle", "bout", b.ID, "error", e)
			}
		}
		// Untracked after the settle (see above) and before the nudge, so
		// the tick the nudge wakes finds the seat genuinely empty.
		r.untrack(b.ID)
		r.Nudge()
	})
}

func (r *Runner) playing() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.inFlight)
}

func (r *Runner) untrack(id int64) {
	r.mu.Lock()
	delete(r.inFlight, id)
	r.mu.Unlock()
}

// StartSample opens an admin-triggered measurement run: the full round-robin
// over the entire roster, bounded by nothing but its own deadline — how the
// first real window is chosen from a measured hour rather than a guess
// (Aaron, 2026-09-05). Returns the run and how many bouts were dealt; a
// still-open run refuses with [ErrRunOpen] for the route's 409.
func (r *Runner) StartSample(ctx context.Context, minutes int) (Run, int, error) {
	now := r.now()
	house, players, err := r.roster(ctx)
	if err != nil {
		return Run{}, 0, err
	}
	key := r.sampleKey(now)
	plans := PlanSample(key, house, players, r.set.Games)
	run, err := r.store.StartRun(ctx, key, true, now.Add(time.Duration(minutes)*time.Minute))
	if err != nil {
		return Run{}, 0, err
	}
	if err := r.store.PlanBouts(ctx, run.ID, plans); err != nil {
		// A sample with no card measures nothing: finish it on the spot so
		// the failed attempt does not block the next one. On the compensation's
		// own authority, not the caller's — the likeliest way PlanBouts fails
		// is the admin's connection dropping mid-request, and a cleanup that
		// dies of the same cancellation leaves the empty run blocking every
		// night until its deadline.
		if e := r.store.FinishRun(context.WithoutCancel(ctx), run.ID); e != nil {
			r.log.Error("an empty sample would not finish", "run", run.ID, "error", e)
		}
		return Run{}, 0, err
	}
	r.Nudge()
	return run, len(plans), nil
}

// sampleKey is a measurement run's night key: the local date where a zone is
// configured, UTC's otherwise — a sample must work before any zone is chosen,
// since it is how the window gets chosen at all.
func (r *Runner) sampleKey(now time.Time) string {
	if r.set.Zone != nil {
		now = now.In(r.set.Zone)
	} else {
		now = now.UTC()
	}
	return now.Format("2006-01-02")
}

// Store hands back the rows the admin's watching read needs — the runner is
// what the route layer holds, and the store rides along rather than being
// wired twice.
func (r *Runner) Store() *Store { return r.store }
