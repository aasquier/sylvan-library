package night_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The tick over a database that is failing under it.
//
// Every error the runner meets is logged rather than returned — a tick
// answers to nobody — so the log *is* the assertion here, and there is no
// other way to tell the difference between "the sweep failed and the tick
// stopped" and "there was nothing to sweep". Each test below names the
// sentence it expects and, where a row could have moved, checks that it did
// not.
//
// The fault is armed from inside the seams the runner calls while it works —
// House, LaneBusy, the arena's Play — which is what makes these tests
// deterministic without counting a whole tick's statements: the seam runs at
// a known point in the flow, and the budget only has to cover what happens
// after it.

// syncLog is a log a test can read while the runner's waiters are still
// writing to it.
type syncLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// rig is a runner whose store can be made to fail at a chosen moment.
type rig struct {
	runner  *night.Runner
	store   *night.Store
	fault   *authtest.Fault
	clock   *fakeClock
	log     *syncLog
	settled chan night.Bout
}

// newRig wires a runner over a faulty app.db. house and busy are the two
// seams a test arms the fault from; either may be nil.
func newRig(t *testing.T, set night.Settings, arena night.BoutPlayer,
	house func(context.Context) ([]string, error), busy func() bool) *rig {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &fakeClock{at: time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)}
	store := night.FromDB(db, clock.now)
	log := &syncLog{}
	settled := make(chan night.Bout, 16)
	if house == nil {
		house = func(context.Context) ([]string, error) {
			return []string{"kaheera", "goreclaw", "atla"}, nil
		}
	}
	r := night.NewRunner(night.RunnerConfig{Store: store, Settings: set,
		Player: arena, LaneBusy: busy, House: house,
		Log:      slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Now:      clock.now,
		Interval: time.Hour,
		Settled:  func(b night.Bout) { settled <- b }})
	t.Cleanup(r.Stop)
	return &rig{runner: r, store: store, fault: fault, clock: clock,
		log: log, settled: settled}
}

func (g *rig) said(t *testing.T, sentence string) {
	t.Helper()
	if !strings.Contains(g.log.String(), sentence) {
		t.Fatalf("the night never said %q; it said:\n%s", sentence, g.log.String())
	}
}

// Everything the opening of a night can trip over, one statement at a time.
// The budgets are counted from inside the House seam, which the runner calls
// after it has already read its own run and asked whether tonight happened —
// so the count below starts at the players' roster and walks forward.
func TestANightThatCannotOpenSaysWhichStepFailedAndOpensNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		budget   int
		sentence string
	}{
		// The house's half of the roster answers, and from that moment the
		// store has `budget` statements left: nought stops the players'
		// read, two stops the run's own insert.
		{"the players' half of the roster", 0, "the night could not muster its roster"},
		{"the run's own row", 2, "the night did not open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newRigArming(t, tonight(t), tc.budget)
			g.runner.Tick(context.Background())
			g.fault.Heal()
			g.said(t, tc.sentence)
			if _, ok, err := g.store.OpenRun(context.Background()); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Fatal("a night opened over a database that was failing under it")
			}
		})
	}
}

// newRigArming is newRig with the House seam arming the fault on its way
// past: the roster's house half answers, and from that moment the store has
// `budget` statements left.
func newRigArming(t *testing.T, set night.Settings, budget int) *rig {
	t.Helper()
	var g *rig
	g = newRig(t, set, &fakeArena{}, func(context.Context) ([]string, error) {
		g.fault.After(budget)
		return []string{"kaheera", "goreclaw", "atla"}, nil
	}, nil)
	return g
}

// The card that does not write. The run is open and empty, and ADR 46's
// answer is to say so loudly and let it idle to its close rather than to
// pretend the night never opened — so the tick goes on to work the run it
// just opened, and trips over the same broken database there.
func TestANightWhoseCardDoesNotWriteIsLoudAndStillOpen(t *testing.T) {
	t.Parallel()
	// One statement for the players' roster, four for StartRun (its begin,
	// its check, its insert, its commit) — and then the deal's first
	// statement is the one that goes.
	g := newRigArming(t, tonight(t), 5)
	g.runner.Tick(context.Background())
	g.fault.Heal()
	g.said(t, "the night opened but its card did not write")
	// And the tick carried on into the open run, where the orphan sweep met
	// the same database.
	g.said(t, "the orphan sweep failed")

	run, ok, err := g.store.OpenRun(context.Background())
	if err != nil || !ok {
		t.Fatalf("the empty run is not open: ok=%v err=%v", ok, err)
	}
	bouts, err := g.store.Bouts(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bouts) != 0 {
		t.Fatalf("a card that did not write left %d bouts", len(bouts))
	}
}

// A tick that cannot read its own run does nothing at all — it must not fall
// through to opening a second night beside the one it failed to see.
func TestATickThatCannotReadItsOwnRunOpensNothing(t *testing.T) {
	t.Parallel()
	g := newRig(t, tonight(t), &fakeArena{}, nil, nil)
	g.fault.After(0)
	g.runner.Tick(context.Background())
	g.fault.Heal()
	g.said(t, "the night could not read its own run")
	if _, ok, err := g.store.OpenRun(context.Background()); err != nil || ok {
		t.Fatalf("a run appeared anyway: ok=%v err=%v", ok, err)
	}
}

// The same question one step later: the run reads, and the "has tonight
// already happened" check is the statement that goes. Opening a night
// because that check failed would re-run a night that already ran.
func TestANightThatCannotAskWhetherItHappenedDoesNotHappenAgain(t *testing.T) {
	t.Parallel()
	g := newRig(t, tonight(t), &fakeArena{}, nil, nil)
	// One statement: the open-run read lands, the scheduled-run check does
	// not.
	g.fault.After(1)
	g.runner.Tick(context.Background())
	g.fault.Heal()
	g.said(t, "the night could not ask whether tonight has happened")
	if _, ok, err := g.store.OpenRun(context.Background()); err != nil || ok {
		t.Fatalf("a night opened over the unanswered question: ok=%v err=%v", ok, err)
	}
}

// openNight seeds an open run and its card directly, so a test about the
// working half of the tick does not have to drive the opening half first.
func (g *rig) openNight(t *testing.T, closesAt time.Time, seats ...[]night.Seat) night.Run {
	t.Helper()
	ctx := context.Background()
	run, err := g.store.StartRun(ctx, "2026-09-06", false, closesAt)
	if err != nil {
		t.Fatal(err)
	}
	plans := make([]night.Plan, 0, len(seats))
	for i, s := range seats {
		plans = append(plans, night.Plan{Seats: s, Games: 3, Clock: 300, Seed: int64(i + 1)})
	}
	if err := g.store.PlanBouts(ctx, run.ID, plans); err != nil {
		t.Fatal(err)
	}
	return run
}

func houseSeats(a, b string) []night.Seat {
	return []night.Seat{{Slug: a}, {Slug: b}}
}

// The window has shut and the remainder will not settle. Both halves of the
// close are a write, and each failing has to stop the tick where it is: a
// night declared finished over bouts that are still `planned` is a card the
// next boot will deal out again.
func TestACloseThatWillNotWriteLeavesTheNightOpen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		budget   int
		sentence string
	}{
		// The open-run read, then the orphan sweep, then the step named.
		{"the remainder will not skip", 2, "the remainder would not skip"},
		{"the night will not finish", 3, "the night would not finish"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newRig(t, tonight(t), &fakeArena{}, nil, nil)
			closed := g.clock.now().Add(-time.Hour)
			run := g.openNight(t, closed, houseSeats("kaheera", "goreclaw"))

			g.fault.After(tc.budget)
			g.runner.Tick(context.Background())
			g.fault.Heal()
			g.said(t, tc.sentence)

			ctx := context.Background()
			after, ok, err := g.store.OpenRun(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !ok || after.ID != run.ID {
				t.Fatal("the night was finished over a close that never wrote")
			}
		})
	}
}

// The claim itself failing. Nothing is handed to the arena, and the bout
// stays planned for the next tick rather than being lost to a claim nobody
// recorded.
func TestAClaimThatWillNotWriteFightsNothing(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{}
	var g *rig
	g = newRig(t, tonight(t), arena, nil, func() bool {
		// Armed here: everything before the claim has already happened.
		g.fault.After(0)
		return false
	})
	g.openNight(t, g.clock.now().Add(time.Hour), houseSeats("kaheera", "goreclaw"))
	g.runner.Tick(context.Background())
	g.fault.Heal()
	g.said(t, "the next bout would not claim")
	if fights := arena.fights(); len(fights) != 0 {
		t.Fatalf("the arena was handed %d bouts over a claim that failed", len(fights))
	}
	states := boutStates(t, g.store, 1)
	if states[night.StatePlanned] != 1 {
		t.Fatalf("the unclaimed bout is not still planned: %v", states)
	}
}

// The consent re-check that cannot read plays on, deliberately: the
// deal-time consent stands recorded, and a store this broken will fail the
// bout honestly on its own rather than skipping somebody's deck on the
// strength of a read that never happened.
func TestAConsentRecheckThatCannotReadPlaysOn(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{entered: make(chan struct{})}
	owner := int64(1)
	var g *rig
	g = newRig(t, tonight(t), arena, nil, func() bool {
		// The claim is six statements — its begin, the in-flight check, the
		// next-bout read, the seats, the update, the commit — so the seventh
		// is the consent re-check.
		g.fault.After(6)
		return false
	})
	g.openNight(t, g.clock.now().Add(time.Hour),
		[]night.Seat{{Owner: &owner, Slug: "gyome"}, {Slug: "kaheera"}})

	go g.runner.Tick(context.Background())
	<-arena.entered
	<-g.settled
	g.fault.Heal()
	g.said(t, "the consent re-check would not read")
	if fights := arena.fights(); len(fights) != 1 {
		t.Fatalf("the bout was skipped rather than played: %d fights", len(fights))
	}
}

// A bout that fought and cannot be settled. The row stays `playing` — the
// honest orphan the next boot's sweep fails — and the log carries which
// settle it was, because "done", "skipped" and "failed" are three different
// sentences about the same bout and only one of them is true.
func TestABoutThatCannotSettleSaysSoAndStaysInFlight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		answer   func(b night.Bout) (int64, error)
		sentence string
	}{
		{"it won", func(night.Bout) (int64, error) { return 7, nil },
			"a finished bout would not settle"},
		{"it was skipped", func(night.Bout) (int64, error) {
			return 0, night.Skip{Reason: "the deck has left the library"}
		}, "a skipped bout would not settle"},
		{"it broke", func(night.Bout) (int64, error) {
			return 0, errors.New("the arena fell over")
		}, "a failed bout would not settle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var g *rig
			arena := &fakeArena{}
			arena.answer = func(b night.Bout) (int64, error) {
				// Armed from inside the fight: the claim has committed and
				// the row says `playing`, and the settle is the next write.
				g.fault.After(0)
				return tc.answer(b)
			}
			g = newRig(t, tonight(t), arena, nil, nil)
			g.openNight(t, g.clock.now().Add(time.Hour), houseSeats("kaheera", "goreclaw"))

			g.runner.Tick(context.Background())
			<-g.settled
			g.fault.Heal()
			g.said(t, tc.sentence)
			states := boutStates(t, g.store, 1)
			if states[night.StatePlaying] != 1 {
				t.Fatalf("a bout that could not settle is not still in flight: %v", states)
			}
		})
	}
}

// Skip carries its reason as its message: the row's `reason` column is that
// string, so a Skip whose Error said anything else would write the wrong
// sentence into the night's own record.
func TestASkipsReasonIsItsMessage(t *testing.T) {
	t.Parallel()
	skip := night.Skip{Reason: "the deck has left the library"}
	if skip.Error() != skip.Reason {
		t.Fatalf("Skip.Error() = %q, want its reason", skip.Error())
	}
	var as night.Skip
	if !errors.As(error(skip), &as) || as.Reason != skip.Reason {
		t.Fatal("a Skip did not come back out of errors.As carrying its reason")
	}
}

// A deck whose owner stepped back out is skipped unread — and the skip is
// itself a write. When that write cannot land the bout is left exactly where
// the claim put it, `playing`, with nobody waiting on it: the honest orphan
// the next boot's sweep fails. What must not happen is the night moving on as
// though the seat had been settled.
func TestAWithdrawnSeatThatCannotSettleLeavesTheBoutForTheNextTick(t *testing.T) {
	t.Parallel()
	owner := int64(404) // an account with no user_decks row: consent has gone
	var g *rig
	g = newRig(t, tonight(t), &fakeArena{}, nil, func() bool {
		// The claim is six statements and the consent re-check is the
		// seventh, so the skip that follows it is the one that goes.
		g.fault.After(7)
		return false
	})
	g.openNight(t, g.clock.now().Add(time.Hour),
		[]night.Seat{{Owner: &owner, Slug: "gone"}, {Slug: "kaheera"}})

	g.runner.Tick(context.Background())
	g.fault.Heal()
	g.said(t, "a withdrawn bout would not settle")
	states := boutStates(t, g.store, 1)
	if states[night.StateSkipped] != 0 || states[night.StatePlaying] != 1 {
		t.Fatalf("the bout that would not skip is %v", states)
	}
}

// A sample whose card will not write finishes itself on the spot so the empty
// run does not block every night until its deadline — and when even that
// compensation cannot write, it says so rather than going quietly. The run is
// then genuinely stuck, and the log is the only thing that will ever say why.
func TestASampleThatCannotEvenFinishItselfSaysSo(t *testing.T) {
	t.Parallel()
	// The players' roster, then StartRun's four statements — and the deal's
	// first statement, and the compensating finish after it, both go.
	g := newRigArming(t, tonight(t), 5)
	if _, _, err := g.runner.StartSample(context.Background(), 30); err == nil {
		t.Fatal("a sample with no card reported success")
	}
	g.fault.Heal()
	g.said(t, "an empty sample would not finish")
	run, ok, err := g.store.LatestRun(context.Background())
	if err != nil || !ok {
		t.Fatalf("the sample left no run at all: ok=%v err=%v", ok, err)
	}
	if !run.Open() {
		t.Fatal("the run finished after the finish that failed")
	}
}

// A result set that fails partway is not a shorter card. Both reads pull the
// bout rows and then their seats, and a loop that stopped early without
// asking would hand back a night with bouts missing from it — which the
// admin's watching read would draw as a card that had already settled.
func TestACardReadThatFailsPartwayIsNotAShorterCard(t *testing.T) {
	t.Parallel()
	g := newRig(t, tonight(t), &fakeArena{}, nil, nil)
	ctx := context.Background()
	g.openNight(t, g.clock.now().Add(time.Hour),
		houseSeats("kaheera", "goreclaw"), houseSeats("atla", "tivit"))
	if _, _, err := g.store.ClaimNext(ctx, 1); err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func() error{
		"the whole card":      func() error { _, err := g.store.Bouts(ctx, 1); return err },
		"the bouts in flight": func() error { _, err := g.store.Playing(ctx, 1); return err },
	} {
		g.fault.Heal()
		// One row through, and the read after it fails rather than ending.
		g.fault.RowsAfter(1)
		err := call()
		g.fault.Heal()
		if err == nil {
			t.Errorf("%s came back whole over an iteration that failed", name)
		}
	}

	g.fault.Heal()
	if bouts, err := g.store.Bouts(ctx, 1); err != nil || len(bouts) != 2 {
		t.Fatalf("the recovered read: %d bouts, %v", len(bouts), err)
	}
}
