package night_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The runner over a fake clock, a fake arena and a real scratch app.db.
// Every assertion reads rows or the fake's log — the tick is driven and its
// effects are read back, never reproduced by hand.

// fakeClock is [ticking]'s guarded sibling: the runner's waiters read it from
// their own goroutines, so this one takes a lock and moves a millisecond per
// look — stamps stay distinct without a jump big enough to cross a window.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(time.Millisecond)
	return c.at
}

func (c *fakeClock) set(t time.Time) {
	c.mu.Lock()
	c.at = t
	c.mu.Unlock()
}

// fakeArena is the BoutPlayer the tests seat: it logs what it was asked to
// fight, optionally parks until the test lets the bout end, and honours ctx
// the way the seam's contract demands.
type fakeArena struct {
	mu     sync.Mutex
	played []night.Bout
	// answer decides a bout's fate; nil wins every match with id 1000+bout.
	answer func(b night.Bout) (int64, error)
	// gate, when non-nil, holds every Play until the test sends one release.
	gate chan struct{}
}

func (f *fakeArena) Play(ctx context.Context, b night.Bout) (int64, error) {
	f.mu.Lock()
	f.played = append(f.played, b)
	f.mu.Unlock()
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if f.answer != nil {
		return f.answer(b)
	}
	return 1000 + b.ID, nil
}

func (f *fakeArena) fights() []night.Bout {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]night.Bout(nil), f.played...)
}

// tonight is a scheduled deployment: open 22:00-23:00 UTC, two bouts of
// three games, one bout per account.
func tonight(t *testing.T) night.Settings {
	t.Helper()
	w, err := night.ParseWindow("22:00-23:00")
	if err != nil {
		t.Fatal(err)
	}
	return night.Settings{Scheduled: true, Window: w, Zone: time.UTC,
		Bouts: 2, BoutsPerAccount: 1, Games: 3}
}

func quietRunner(t *testing.T, set night.Settings, arena night.BoutPlayer,
	busy func() bool, house []string) (*night.Runner, *night.Store, *fakeClock) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	// One clock for the store and the runner, the way the served process
	// wires them — a run's stamps and the tick's window reads must agree.
	clock := &fakeClock{at: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)}
	s, err := night.NewStore(path, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: set,
		Player: arena, LaneBusy: busy,
		House: func(context.Context) ([]string, error) { return house, nil },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:   clock.now, Interval: time.Hour})
	t.Cleanup(r.Stop)
	return r, s, clock
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func boutStates(t *testing.T, s *night.Store, runID int64) map[night.State]int {
	t.Helper()
	bouts, err := s.Bouts(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[night.State]int{}
	for _, b := range bouts {
		out[b.State]++
	}
	return out
}

func TestATickOpensTonightOnceAndWorksItToTheEnd(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{}
	house := []string{"kaheera", "goreclaw", "atla"}
	r, s, clock := quietRunner(t, tonight(t), arena, nil, house)
	ctx := context.Background()

	// Before the window: nothing opens.
	clock.set(time.Date(2026, 9, 6, 21, 0, 0, 0, time.UTC))
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("a run opened outside the window")
	}

	// Inside the window: tonight opens with the deterministic card, and the
	// first bout is claimed in the same beat.
	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	run, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("no run opened inside the window: ok=%v err=%v", ok, err)
	}
	if run.NightKey != "2026-09-06" || run.Sample {
		t.Fatalf("the run came out wrong: %+v", run)
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := night.PlanScheduled("2026-09-06", house, nil, tonight(t))
	if len(bouts) != len(want) {
		t.Fatalf("the card holds %d bouts, want %d", len(bouts), len(want))
	}
	for i, b := range bouts {
		got := night.Plan{SeatA: b.SeatA, SeatB: b.SeatB, Games: b.Games, Seed: b.Seed}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("bout %d is %+v, want the recomputed %+v", i, got, want[i])
		}
	}

	// The first bout settles; the second is claimed on the next tick; the
	// card ends with both done and their match ids on the rows.
	waitFor(t, "the first bout to settle", func() bool {
		return boutStates(t, s, run.ID)[night.StateDone] == 1
	})
	r.Tick(ctx)
	waitFor(t, "the second bout to settle", func() bool {
		return boutStates(t, s, run.ID)[night.StateDone] == 2
	})
	final, _ := s.Bouts(ctx, run.ID)
	for _, b := range final {
		if b.MatchID == nil || *b.MatchID != 1000+b.ID {
			t.Errorf("bout %d carries match %v, want %d", b.ID, b.MatchID, 1000+b.ID)
		}
	}

	// A tick with the card settled and the window open changes nothing.
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); !ok {
		t.Fatal("the run finished before its window closed")
	}

	// Past the close: the night finishes with nothing to skip, and the still
	// open window does not deal tonight twice.
	clock.set(time.Date(2026, 9, 6, 23, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("the night did not finish at its close")
	}
	clock.set(time.Date(2026, 9, 6, 22, 30, 0, 0, time.UTC))
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("a finished night reopened inside its own window")
	}

	// The next evening opens its own run under its own key.
	clock.set(time.Date(2026, 9, 7, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	next, ok, _ := s.OpenRun(ctx)
	if !ok || next.NightKey != "2026-09-07" {
		t.Fatalf("the next night did not open: ok=%v run=%+v", ok, next)
	}
}

func TestThePersonInTheRoomWins(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{}
	var busy atomic.Bool
	busy.Store(true)
	r, s, clock := quietRunner(t, tonight(t), arena, busy.Load,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()

	// The window opens while somebody's match holds the lane: the night
	// still opens and deals, and submits nothing.
	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	run, ok, _ := s.OpenRun(ctx)
	if !ok {
		t.Fatal("the busy lane stopped the night opening; it should only stop the fighting")
	}
	if got := boutStates(t, s, run.ID); got[night.StatePlanned] != 2 || len(arena.fights()) != 0 {
		t.Fatalf("the night fought over a busy lane: states %v, fights %d",
			got, len(arena.fights()))
	}

	// The room empties; the next tick fights.
	busy.Store(false)
	r.Tick(ctx)
	waitFor(t, "the first bout", func() bool { return len(arena.fights()) == 1 })
}

func TestTheCloseFinishesTheFlightAndSkipsTheRest(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	r, s, clock := quietRunner(t, tonight(t), arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()

	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	run, _, _ := s.OpenRun(ctx)
	waitFor(t, "a bout in flight", func() bool {
		return boutStates(t, s, run.ID)[night.StatePlaying] == 1
	})

	// The window closes mid-bout: the flight is left to finish (ADR 46
	// decision 6) and the night stays open for it.
	clock.set(time.Date(2026, 9, 6, 23, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 || got[night.StateSkipped] != 0 {
		t.Fatalf("the close did not wait for the flight: %v", got)
	}
	if _, ok, _ := s.OpenRun(ctx); !ok {
		t.Fatal("the night finished with a bout still fighting")
	}

	// The bout ends; the next tick skips the remainder with the reason and
	// declares the night over.
	arena.gate <- struct{}{}
	waitFor(t, "the flight to settle", func() bool {
		return boutStates(t, s, run.ID)[night.StateDone] == 1
	})
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("the night did not finish once the flight settled")
	}
	bouts, _ := s.Bouts(ctx, run.ID)
	skipped := 0
	for _, b := range bouts {
		if b.State == night.StateSkipped {
			skipped++
			if b.Reason != "the window closed" {
				t.Errorf("bout %d skipped with %q", b.ID, b.Reason)
			}
		}
	}
	if skipped != 1 {
		t.Errorf("%d bouts skipped, want 1", skipped)
	}
}

func TestAStopAbandonsNothingAndTheNextBootFailsTheOrphan(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	house := []string{"kaheera", "goreclaw", "atla"}
	r, s, clock := quietRunner(t, tonight(t), arena, nil, house)
	ctx := context.Background()

	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Start()
	r.Nudge()
	run, ok := night.Run{}, false
	waitFor(t, "a bout in flight", func() bool {
		if !ok {
			run, ok, _ = s.OpenRun(ctx)
		}
		return ok && boutStates(t, s, run.ID)[night.StatePlaying] == 1
	})

	// The stop returns even with a bout parked in the arena — the doneness
	// assertion: Stop waits for every goroutine the runner started, so its
	// return *is* the leak check.
	stopped := make(chan struct{})
	go func() { r.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung on a bout in flight; the runner leaks its waiter")
	}

	// The row was left honestly in flight, and the next process — a fresh
	// runner over the same rows — fails it as the orphan it is, match_id
	// still NULL, then carries the night on from where it stood.
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 {
		t.Fatalf("the stop rewrote the flight: %v", got)
	}
	reborn := night.NewRunner(night.RunnerConfig{Store: s, Settings: tonight(t),
		Player: &fakeArena{},
		House:  func(context.Context) ([]string, error) { return house, nil },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now})
	t.Cleanup(reborn.Stop)
	reborn.Tick(ctx)
	bouts, _ := s.Bouts(ctx, run.ID)
	orphans := 0
	for _, b := range bouts {
		if b.State == night.StateFailed {
			orphans++
			if b.Reason != "the process restarted mid-bout" || b.MatchID != nil {
				t.Errorf("the orphan settled wrong: reason %q match %v", b.Reason, b.MatchID)
			}
		}
	}
	if orphans != 1 {
		t.Fatalf("%d orphans failed, want 1", orphans)
	}
	waitFor(t, "the night to carry on past the orphan", func() bool {
		return boutStates(t, s, run.ID)[night.StateDone] == 1
	})
}

func TestASampleDealsTheWholeRosterAndRefusesASecond(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	// No window, no zone: the sample is how the window gets chosen, so it
	// must run on a deployment that has not chosen one.
	set := night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 5}
	r, s, clock := quietRunner(t, set, arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()
	clock.set(time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC))

	run, dealt, err := r.StartSample(ctx, 60)
	if err != nil {
		t.Fatalf("StartSample: %v", err)
	}
	if !run.Sample {
		t.Fatal("the sample run is not marked sample")
	}
	// Three decks, caps ignored: the full round-robin of 3, at the settings'
	// games — and the deadline is the asked-for hour.
	if dealt != 3 {
		t.Fatalf("the sample dealt %d bouts, want 3", dealt)
	}
	bouts, _ := s.Bouts(ctx, run.ID)
	for _, b := range bouts {
		if b.Games != 5 {
			t.Errorf("bout %d plays %d games, want 5", b.ID, b.Games)
		}
	}
	if wait := run.ClosesAt.Sub(run.OpenedAt); wait < 59*time.Minute || wait > 61*time.Minute {
		t.Errorf("the deadline is %s past open, want the asked-for hour", wait)
	}

	if _, _, err := r.StartSample(ctx, 30); !errors.Is(err, night.ErrRunOpen) {
		t.Fatalf("a second sample was answered %v, want ErrRunOpen", err)
	}
}

func TestABoutSettlesTheWayItsPlayerAnswered(t *testing.T) {
	t.Parallel()
	// One arena, three verdicts: a skip (the pre-flight said no), a failure,
	// and a match that played but whose ledger declined the row — done with
	// match_id NULL, because pointing at a match that does not exist would
	// be worse than pointing at nothing.
	arena := &fakeArena{answer: func(b night.Bout) (int64, error) {
		switch b.SeatA.Slug + " vs " + b.SeatB.Slug {
		case "kaheera vs goreclaw", "goreclaw vs kaheera":
			return 0, night.Skip{Reason: "the pre-flight said no"}
		case "kaheera vs atla", "atla vs kaheera":
			return 0, errors.New("the arena fell over")
		default:
			return 0, nil // played, unrecorded
		}
	}}
	set := night.Settings{Bouts: 3, BoutsPerAccount: 1, Games: 3}
	r, s, clock := quietRunner(t, set, arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()
	clock.set(time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC))

	run, dealt, err := r.StartSample(ctx, 60)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < dealt; i++ {
		r.Tick(ctx)
		waitFor(t, "the bout to settle", func() bool {
			got := boutStates(t, s, run.ID)
			return got[night.StatePlanned]+got[night.StatePlaying] == dealt-i-1
		})
	}
	bouts, _ := s.Bouts(ctx, run.ID)
	for _, b := range bouts {
		pair := b.SeatA.Slug + " vs " + b.SeatB.Slug
		switch b.State {
		case night.StateSkipped:
			if b.Reason != "the pre-flight said no" {
				t.Errorf("%s skipped with %q", pair, b.Reason)
			}
		case night.StateFailed:
			if b.Reason != "the arena fell over" {
				t.Errorf("%s failed with %q", pair, b.Reason)
			}
		case night.StateDone:
			if b.MatchID != nil {
				t.Errorf("%s went unrecorded yet carries match %d", pair, *b.MatchID)
			}
		default:
			t.Errorf("%s is still %s", pair, b.State)
		}
	}
	got := boutStates(t, s, run.ID)
	if got[night.StateSkipped] != 1 || got[night.StateFailed] != 1 || got[night.StateDone] != 1 {
		t.Fatalf("the three verdicts settled as %v", got)
	}
}

func TestARosterThatWillNotMusterOpensNothing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{at: time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)}
	s, err := night.NewStore(path, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: tonight(t),
		Player: &fakeArena{},
		House: func(context.Context) ([]string, error) {
			return nil, errors.New("the shelf would not open")
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now})
	t.Cleanup(r.Stop)
	ctx := context.Background()
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("a night opened over a roster that would not muster")
	}
	if _, _, err := r.StartSample(ctx, 30); err == nil {
		t.Fatal("a sample started over a roster that would not muster")
	}
}

func TestARunnersZeroValuesAreWorkingDefaults(t *testing.T) {
	t.Parallel()
	// Store and Player alone: the wall clock, an empty house, a lane never
	// busy, the default log — and with no window configured, a tick rests.
	s, _ := scratchAndDB(t)
	r := night.NewRunner(night.RunnerConfig{Store: s, Player: &fakeArena{}})
	t.Cleanup(r.Stop)
	r.Tick(context.Background())
	if _, ok, _ := s.OpenRun(context.Background()); ok {
		t.Fatal("an unscheduled deployment opened a night")
	}
	// And the store rides on the runner for the route layer's reads.
	if r.Store() != s {
		t.Fatal("Store() hands back a different store than the runner holds")
	}
}

func TestASamplesKeyWearsTheConfiguredZone(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{}
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	set := night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 3, Zone: zone}
	r, s, clock := quietRunner(t, set, arena, nil, []string{"kaheera", "goreclaw"})
	ctx := context.Background()
	// 03:00 UTC on the 7th is still the evening of the 6th on the west
	// coast, and the sample's key says whose evening it was.
	clock.set(time.Date(2026, 9, 7, 3, 0, 0, 0, time.UTC))
	run, _, err := r.StartSample(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if run.NightKey != "2026-09-06" {
		t.Fatalf("the sample's key is %s, want the zone's own 2026-09-06", run.NightKey)
	}
	if _, ok, _ := s.OpenRun(ctx); !ok {
		t.Fatal("the sample did not open")
	}
}

func TestASettledBoutNudgesTheNextWithoutWaitingForTheTicker(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	set := night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 3}
	r, _, clock := quietRunner(t, set, arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	clock.set(time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC))

	// The ticker's interval is an hour, so everything below moves on nudges
	// alone: the sample's own, then the one each settled bout sends.
	r.Start()
	if _, _, err := r.StartSample(context.Background(), 60); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first bout", func() bool { return len(arena.fights()) == 1 })
	arena.gate <- struct{}{}
	waitFor(t, "the second bout, on the settle's nudge", func() bool {
		return len(arena.fights()) == 2
	})
	arena.gate <- struct{}{}
	waitFor(t, "the third bout, on the settle's nudge", func() bool {
		return len(arena.fights()) == 3
	})
}
