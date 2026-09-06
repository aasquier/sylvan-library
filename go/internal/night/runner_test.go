package night_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
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
//
// Every wait in this file is a channel receive on an event the runner or the
// fake signals — never a poll of row state under a wall-clock deadline. The
// first draft polled with a 5-second budget, and that greenness was a fact
// about the runner's load: it failed two consecutive loaded CI runs at the
// same line while passing every quiet one. Two events cover every wait: the
// arena's `entered` (the bout is claimed and fighting) and the runner's
// Settled seam (the row is settled and the seat is free, so the next Tick
// can claim). A wait that can no longer time out can still hang on a real
// bug — that is the package timeout's job, and its goroutine dump names the
// parked waiter, which is a better diagnosis than "timed out waiting" ever
// was.

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

// slugsOf names a bout the way these tests talk about one — every seat in
// seat order, joined. A pod has four, so the old "a vs b" no longer names a
// bout on its own.
func slugsOf(b night.Bout) string {
	out := make([]string, 0, len(b.Seats))
	for _, s := range b.Seats {
		out = append(out, s.Slug)
	}
	return strings.Join(out, " vs ")
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
	// entered, when non-nil, takes one blocking send at the top of every
	// Play. The claim precedes the fight, so by the time a test receives
	// this the bout's row is already `playing` and the run is open — the
	// event that replaces polling for either. Blocking on purpose: a Play
	// the test did not expect parks the waiter, and the package timeout's
	// dump names it.
	entered chan struct{}
}

func (f *fakeArena) Play(ctx context.Context, b night.Bout) (int64, error) {
	f.mu.Lock()
	f.played = append(f.played, b)
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
	}
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
	busy func() bool, house []string) (*night.Runner, *night.Store, *fakeClock, chan night.Bout) {
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
	// Buffered past any test's bout count, so a waiter never blocks sending
	// a settle a test has not received yet (or never will — Stop waits on
	// that waiter, and a blocked send would turn cleanup into a hang).
	settled := make(chan night.Bout, 16)
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: set,
		Player: arena, LaneBusy: busy,
		House: func(context.Context) ([]string, error) { return house, nil },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:   clock.now, Interval: time.Hour,
		Settled: func(b night.Bout) { settled <- b }})
	t.Cleanup(r.Stop)
	return r, s, clock, settled
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
	r, s, clock, settled := quietRunner(t, tonight(t), arena, nil, house)
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
		got := night.Plan{Seats: b.Seats, Games: b.Games, Clock: b.Clock, Seed: b.Seed}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("bout %d is %+v, want the recomputed %+v", i, got, want[i])
		}
	}

	// The first bout settles; the second is claimed on the next tick; the
	// card ends with both done and their match ids on the rows. Each settle
	// is awaited on the seam, which fires only after the seat untracks — so
	// the tick after a receive is guaranteed to find the seat free and claim.
	<-settled
	r.Tick(ctx)
	<-settled
	if got := boutStates(t, s, run.ID); got[night.StateDone] != 2 {
		t.Fatalf("the card did not finish done: %v", got)
	}
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
	r, s, clock, settled := quietRunner(t, tonight(t), arena, busy.Load,
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
	<-settled
	if len(arena.fights()) != 1 {
		t.Fatalf("the empty room got %d fights, want 1", len(arena.fights()))
	}
}

func TestTheCloseFinishesTheFlightAndSkipsTheRest(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	r, s, clock, settled := quietRunner(t, tonight(t), arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()

	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	run, _, _ := s.OpenRun(ctx)
	// The claim is the tick's own synchronous work: the row is `playing`
	// before Tick returns, gate or no gate.
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 {
		t.Fatalf("the tick did not put a bout in flight: %v", got)
	}

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
	// declares the night over. The settle receive is what makes one tick
	// enough — it fires after the seat untracks, so the tick cannot land in
	// the settled-but-still-held gap.
	arena.gate <- struct{}{}
	<-settled
	r.Tick(ctx)
	if _, ok, _ := s.OpenRun(ctx); ok {
		t.Fatal("the night did not finish once its flight settled")
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
	arena := &fakeArena{gate: make(chan struct{}), entered: make(chan struct{})}
	house := []string{"kaheera", "goreclaw", "atla"}
	r, s, clock, _ := quietRunner(t, tonight(t), arena, nil, house)
	ctx := context.Background()

	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Start()
	r.Nudge()
	// The arena's entered signal: by its receive the nudged tick has opened
	// the night, claimed the bout, and parked the fight on the gate.
	<-arena.entered
	run, ok, _ := s.OpenRun(ctx)
	if !ok {
		t.Fatal("a bout entered the arena with no open run")
	}
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 {
		t.Fatalf("the entered bout is not in flight: %v", got)
	}

	// The stop returns even with a bout parked in the arena — the doneness
	// assertion: Stop waits for every goroutine the runner started, so its
	// return *is* the leak check. A leaked waiter hangs right here, and the
	// package timeout's goroutine dump names it.
	r.Stop()

	// The row was left honestly in flight, and the next process — a fresh
	// runner over the same rows — fails it as the orphan it is, match_id
	// still NULL, then carries the night on from where it stood.
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 {
		t.Fatalf("the stop rewrote the flight: %v", got)
	}
	rebornSettled := make(chan night.Bout, 16)
	reborn := night.NewRunner(night.RunnerConfig{Store: s, Settings: tonight(t),
		Player: &fakeArena{},
		House:  func(context.Context) ([]string, error) { return house, nil },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now,
		Settled: func(b night.Bout) { rebornSettled <- b }})
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
	// The same tick that failed the orphan claimed the next bout; its settle
	// is the proof the night carried on rather than wedging on the wreck.
	<-rebornSettled
	if got := boutStates(t, s, run.ID); got[night.StateDone] != 1 {
		t.Fatalf("the night did not carry on past the orphan: %v", got)
	}
}

func TestASampleDealsTheWholeRosterAndRefusesASecond(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{})}
	// No window, no zone: the sample is how the window gets chosen, so it
	// must run on a deployment that has not chosen one.
	set := night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 5}
	r, s, clock, _ := quietRunner(t, set, arena, nil,
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
	// Three decks, caps ignored: the full round-robin of 3 — and the deadline
	// is the asked-for hour.
	if dealt != 3 {
		t.Fatalf("the sample dealt %d bouts, want 3", dealt)
	}
	// A sample carries the same per-table dials a scheduled night does, which
	// is the whole reason it can be trusted to measure one: a duel plays the
	// settings' games at the duel clock, a pod plays [night.PodGames] at
	// [night.PodClock].
	bouts, _ := s.Bouts(ctx, run.ID)
	for _, b := range bouts {
		wantGames, wantClock := 5, night.DuelClock
		if b.Pod() {
			wantGames, wantClock = night.PodGames, night.PodClock
		}
		if b.Games != wantGames {
			t.Errorf("%s plays %d games, want %d", slugsOf(b), b.Games, wantGames)
		}
		if b.Clock != wantClock {
			t.Errorf("%s runs at clock %d, want %d", slugsOf(b), b.Clock, wantClock)
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
	// Keyed on the bout's order rather than on its seats: with pods in the mix
	// a table is four slugs in a dealt order, and a switch over spellings of
	// "a vs b" was only ever a way of naming the first, second and third bout.
	// Naming them directly says what the test means and cannot silently stop
	// matching the day the deal changes.
	//
	// The count comes off the arena's own mutex-guarded log rather than a
	// captured int: `Play` records the bout before it calls `answer`, so the
	// length is this bout's ordinal, and a plain counter here would be shared
	// state between the runner's waiter goroutines with no edge to order it.
	arena := &fakeArena{}
	arena.answer = func(night.Bout) (int64, error) {
		switch len(arena.fights()) {
		case 1:
			return 0, night.Skip{Reason: "the pre-flight said no"}
		case 2:
			return 0, errors.New("the arena fell over")
		default:
			return 0, nil // played, unrecorded
		}
	}
	set := night.Settings{Bouts: 3, BoutsPerAccount: 1, Games: 3}
	r, s, clock, settled := quietRunner(t, set, arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	ctx := context.Background()
	clock.set(time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC))

	run, dealt, err := r.StartSample(ctx, 60)
	if err != nil {
		t.Fatal(err)
	}
	// Each hand-driven tick claims exactly one bout, and its settle is
	// received on the seam before the next tick — which is what guarantees
	// that tick finds the seat free rather than landing in the gap where a
	// settling bout still holds it.
	for i := 0; i < dealt; i++ {
		r.Tick(ctx)
		<-settled
	}
	bouts, _ := s.Bouts(ctx, run.ID)
	for _, b := range bouts {
		pair := slugsOf(b)
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
	r, s, clock, _ := quietRunner(t, set, arena, nil, []string{"kaheera", "goreclaw"})
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

func TestAScheduledNightSeatsTheOptedInPlayers(t *testing.T) {
	t.Parallel()
	// The whole road from rung 13's rows to a dealt card: the tick musters
	// the players through the store, not just the house through the seam.
	// Without this, the roster's player half could return nothing and every
	// other runner test would stay green — their scratch databases hold no
	// user_decks at all.
	s, db := scratchAndDB(t)
	mustExec(t, db, `INSERT INTO users (id, username, created_at) VALUES
		(1, 'alice', '2026-09-01T00:00:00+00:00')`)
	mustExec(t, db, `INSERT INTO user_decks
		(owner_id, slug, name, yaml, coliseum_at_night, created_at, updated_at) VALUES
		(1, 'gyome', 'Gyome', 'name: G', 1, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00'),
		(1, 'arahbo', 'Arahbo', 'name: A', 0, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00')`)
	arena := &fakeArena{}
	clock := &fakeClock{at: time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)}
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: tonight(t),
		Player: arena,
		House:  func(context.Context) ([]string, error) { return []string{"kaheera", "goreclaw"}, nil },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now})
	t.Cleanup(r.Stop)
	ctx := context.Background()

	r.Tick(ctx)
	run, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("the night did not open: ok=%v err=%v", ok, err)
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	seated := false
	for _, b := range bouts {
		for _, seat := range b.Seats {
			if seat.Slug == "arahbo" {
				t.Errorf("a deck whose owner never opted in was seated: %+v", b)
			}
			if !seat.House() && *seat.Owner == 1 && seat.Slug == "gyome" {
				seated = true
			}
		}
	}
	if !seated {
		t.Fatal("the opted-in player deck never reached the card; the roster's player half is dead")
	}
}

func TestAWithdrawnConsentSkipsTheBoutUnread(t *testing.T) {
	t.Parallel()
	// The flag is standing consent, and standing consent can be withdrawn
	// between the deal and the fight: an owner who steps back out mid-window
	// gets a skipped row, and the arena never reads the deck. The busy lane
	// holds the dealt bout planned long enough for the owner to change their
	// mind — exactly the gap a real night has.
	s, db := scratchAndDB(t)
	mustExec(t, db, `INSERT INTO users (id, username, created_at) VALUES
		(1, 'alice', '2026-09-01T00:00:00+00:00')`)
	mustExec(t, db, `INSERT INTO user_decks
		(owner_id, slug, name, yaml, coliseum_at_night, created_at, updated_at) VALUES
		(1, 'gyome', 'Gyome', 'name: G', 1, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00')`)
	arena := &fakeArena{}
	var busy atomic.Bool
	busy.Store(true)
	w, err := night.ParseWindow("22:00-23:00")
	if err != nil {
		t.Fatal(err)
	}
	set := night.Settings{Scheduled: true, Window: w, Zone: time.UTC,
		Bouts: 1, BoutsPerAccount: 1, Games: 3}
	clock := &fakeClock{at: time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)}
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: set,
		Player: arena, LaneBusy: busy.Load,
		House: func(context.Context) ([]string, error) { return []string{"kaheera", "goreclaw"}, nil },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now})
	t.Cleanup(r.Stop)
	ctx := context.Background()

	r.Tick(ctx)
	run, ok, _ := s.OpenRun(ctx)
	if !ok {
		t.Fatal("the night did not open")
	}
	if got := boutStates(t, s, run.ID); got[night.StatePlanned] != 1 {
		t.Fatalf("the card came out %v, want the one planned bout", got)
	}

	// The owner steps back out between the deal and the bout's turn.
	mustExec(t, db, `UPDATE user_decks SET coliseum_at_night = 0 WHERE slug = 'gyome'`)
	busy.Store(false)
	r.Tick(ctx)
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil || len(bouts) != 1 {
		t.Fatalf("the card holds %d bouts: %v", len(bouts), err)
	}
	if bouts[0].State != night.StateSkipped ||
		bouts[0].Reason != "account 1's gyome has left the night" {
		t.Fatalf("the withdrawn bout settled as %s with %q", bouts[0].State, bouts[0].Reason)
	}
	if len(arena.fights()) != 0 {
		t.Fatal("the arena read a deck whose consent was withdrawn")
	}
}

func TestAnAbandonedSampleFinishesItselfAndDoesNotBlockTheNext(t *testing.T) {
	t.Parallel()
	// The admin's connection dropping between StartRun and PlanBouts must
	// not park an open, empty run that blocks every night until its
	// deadline: the compensating finish runs on its own authority, not the
	// dead caller's. The cancellation is landed in exactly that seam through
	// the store's clock — its first look stamps StartRun, its second stamps
	// PlanBouts — and the assertion below fails loudly rather than
	// vacuously if that ordering ever moves: a run must exist *and* be
	// finished, so a cancel landing too early (no run) or the compensation
	// dying with the caller (an open run) are both caught.
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	looks, at := 0, time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC)
	storeNow := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		looks++
		if looks == 2 {
			cancel()
		}
		at = at.Add(time.Millisecond)
		return at
	}
	s, err := night.NewStore(path, storeNow)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	clock := &fakeClock{at: time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC)}
	r := night.NewRunner(night.RunnerConfig{Store: s, Player: &fakeArena{},
		House: func(context.Context) ([]string, error) { return []string{"kaheera", "goreclaw"}, nil },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.now})
	t.Cleanup(r.Stop)

	if _, _, err := r.StartSample(ctx, 30); err == nil {
		t.Fatal("the sample survived its caller hanging up mid-plan")
	}
	run, ok, err := s.LatestRun(context.Background())
	if err != nil || !ok || run.Open() {
		t.Fatalf("the abandoned sample was not finished on the spot: ok=%v open=%v err=%v",
			ok, ok && run.Open(), err)
	}
	// And the next sample opens over the finished wreck.
	if _, _, err := r.StartSample(context.Background(), 30); err != nil {
		t.Fatalf("the next sample was blocked by the abandoned one: %v", err)
	}
}

func TestASettledBoutNudgesTheNextWithoutWaitingForTheTicker(t *testing.T) {
	t.Parallel()
	arena := &fakeArena{gate: make(chan struct{}), entered: make(chan struct{})}
	set := night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 3}
	r, _, clock, _ := quietRunner(t, set, arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})
	clock.set(time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC))

	// The ticker's interval is an hour, so every entry below happens on
	// nudges alone: the sample's own, then the one each settled bout sends.
	// Each gate release lets one bout settle; the next entered receive is
	// the proof its nudge — and nothing else — woke the loop to claim again.
	r.Start()
	if _, _, err := r.StartSample(context.Background(), 60); err != nil {
		t.Fatal(err)
	}
	<-arena.entered // the first bout, on the sample's nudge
	arena.gate <- struct{}{}
	<-arena.entered // the second, on the settle's nudge
	arena.gate <- struct{}{}
	<-arena.entered // the third, on the settle's nudge
	if got := len(arena.fights()); got != 3 {
		t.Fatalf("%d bouts entered the arena, want 3", got)
	}
}
