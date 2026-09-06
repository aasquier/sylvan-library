package auth

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// The scheduled sweep over app.db's three self-cleaning tables -- and the
// serving process's answer to a shape this package had left standing: three
// purge routines, written and tested, that nothing in production ever called.
// Expired sessions are deleted on the way past by `LookupTouching`, but only
// when their own token is presented again; spent invite and reset links and
// lapsed rate-limit windows had no janitor at all, and the rate-limit rows in
// particular grow with *traffic* rather than with accounts, so a table on the
// volume grew forever with nothing to notice (Aaron's yes, 2026-09-05).
//
// The sweeper is the night runner's smallest sibling
// (`internal/night/runner.go` is the house ticker pattern): built by the
// door beside the runner, one sweep the moment it starts and then one a day,
// stopped with the door so nothing of it outlives a shutdown. Every removal
// is logged in counts -- rows deleted from the accounts database are exactly
// the kind of write that should say so out loud -- and a purge that fails is
// logged and stepped past, because the other two tables are not hostage to
// it and the next day asks again.

// SweepInterval is how often the standing sweep runs after the one at boot.
// Daily: the tables grow by human hands and lapse on clocks measured in
// hours and days, so anything more eager would be a janitor polishing an
// empty room.
const SweepInterval = 24 * time.Hour

// KeepLimitsFor is how old a rate-limit window must be before the sweep takes
// it.
//
// The longest budget window in this package is an hour (`ResetPerMailbox` and
// `ResetPerAddress` both), and `current` already treats a window past its
// budget as no window at all -- so a row this old hands nothing back when it
// goes, and `TestTheSweepOutlastsEveryBudgetWindow` holds this constant above
// every `Limit` there is rather than above the one that happens to be longest
// today. A full day is a wide margin on purpose: erring long costs bytes for a
// day, where erring short would delete a window somebody is still inside.
const KeepLimitsFor = 24 * time.Hour

// SweepCounts is what one sweep removed from each table. All three zero is
// the ordinary answer on an instance swept yesterday.
type SweepCounts struct {
	Sessions int64
	Tokens   int64
	Limits   int64
}

// SweeperConfig is what a sweeper needs. DB is required; every other zero
// value is a working default.
type SweeperConfig struct {
	// DB is the app.db write handle the purges delete through.
	DB *sql.DB
	// Log, or slog.Default().
	Log *slog.Logger
	// Interval is the standing cadence, zero taking [SweepInterval]. A knob
	// for the same reason the night runner's is: the tests drive a day in
	// milliseconds rather than waiting for one.
	Interval time.Duration
	// Swept, when non-nil, is called after each sweep with its counts -- the
	// seam a test awaits instead of a deadline, exactly the night runner's
	// own Settled. Production wiring leaves it nil.
	Swept func(SweepCounts)
}

// Sweeper is the loop. One per door, started with it, stopped with it.
type Sweeper struct {
	db       *sql.DB
	log      *slog.Logger
	interval time.Duration
	swept    func(SweepCounts)

	// ctx is the sweeper's lifetime: the ticker selects on it, the purges
	// take it, and Stop cancels it.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSweeper builds a sweeper. Nothing runs until [Sweeper.Start].
func NewSweeper(cfg SweeperConfig) *Sweeper {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = SweepInterval
	}
	if cfg.Swept == nil {
		cfg.Swept = func(SweepCounts) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Sweeper{db: cfg.DB, log: cfg.Log, interval: cfg.Interval,
		swept: cfg.Swept, ctx: ctx, cancel: cancel}
}

// Start runs the boot sweep -- synchronously, so a caller that has started
// the sweeper holds a database already swept, which is what "one sweep on
// boot" means and what lets a test assert it without waiting on anything --
// and then spawns the daily ticker. One call; the loop runs until
// [Sweeper.Stop].
func (s *Sweeper) Start() {
	s.SweepOnce(s.ctx)
	s.wg.Go(func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
			s.SweepOnce(s.ctx)
		}
	})
}

// Stop ends the sweeper and waits for its goroutine, so nothing of it
// outlives the door that started it. Safe to call twice, and safe on a
// sweeper that never started.
func (s *Sweeper) Stop() {
	s.cancel()
	s.wg.Wait()
}

// SweepOnce is one pass over the three tables, split out so the decision is
// testable without a clock. A purge that fails is logged and stepped past --
// its rows wait for tomorrow, and the other tables still get their sweep --
// but a sweep cut short by its own context goes quietly: that is a shutdown
// crossing a tick, not a fault, and the next boot's sweep takes the rows
// (the night runner's fight makes the same distinction on its way out).
func (s *Sweeper) SweepOnce(ctx context.Context) SweepCounts {
	var c SweepCounts
	var err error
	if c.Sessions, err = PurgeExpiredSessions(ctx, s.db); err != nil {
		if ctx.Err() != nil {
			return c
		}
		s.log.Error("the session sweep failed", "error", err)
	}
	if c.Tokens, err = PurgeExpiredTokens(ctx, s.db, KeepUsedTokensFor); err != nil {
		if ctx.Err() != nil {
			return c
		}
		s.log.Error("the link sweep failed", "error", err)
	}
	if c.Limits, err = PurgeStaleLimits(ctx, s.db, KeepLimitsFor); err != nil {
		if ctx.Err() != nil {
			return c
		}
		s.log.Error("the rate-limit sweep failed", "error", err)
	}
	s.log.Info("the accounts database was swept",
		"sessions", c.Sessions, "links", c.Tokens, "limits", c.Limits)
	s.swept(c)
	return c
}
