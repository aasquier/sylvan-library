package night_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The night's memory, half-written.
//
// This store *is* the runner's memory (its own doc comment says so), which
// makes every one of its error returns load-bearing in a way a read's usually
// is not: a claim that a bout was settled when the UPDATE never landed leaves
// a row the next tick will hand out again, and a run reported open when the
// SELECT failed sends the scheduler to work on a night that does not exist.
//
// A closed handle proves the first statement of each of those says so.
// `authtest.OpenFaulty` proves the rest: the volume detaching *between* the
// BEGIN and the COMMIT, which is the only fixture that can reach the branch
// where a transaction has already written something and then has to decide
// what to tell its caller. The budgets below are counted in statements and
// the assertion beside each one is about the rows, never about the count —
// a budget that lands on the wrong statement fails on the row that did or
// did not change.

func faultyNight(t *testing.T) (*night.Store, *authtest.Fault, *fakeClock) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &fakeClock{at: time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)}
	return night.FromDB(db, clock.now), fault, clock
}

// Every question the night asks its rows, asked of a database that has gone.
// What is asserted is not the message — SQLite's wording is not ours — but
// that an error comes back at all and that **nothing claims success**: no run
// reported open, no bout reported claimed, no count reported settled.
func TestEveryNightReadAndWriteSaysSoWhenTheVolumeGoesAway(t *testing.T) {
	t.Parallel()
	s, fault, clock := faultyNight(t)
	ctx := context.Background()
	fault.After(0)

	asked := 0
	fails := func(what string, err error) {
		t.Helper()
		asked++
		if err == nil {
			t.Errorf("%s answered a gone database without an error", what)
		}
	}

	if run, ok, err := s.OpenRun(ctx); ok || run.ID != 0 {
		t.Errorf("OpenRun reported a run over a gone database: %+v", run)
	} else {
		fails("OpenRun", err)
	}
	if run, ok, err := s.LatestRun(ctx); ok || run.ID != 0 {
		t.Errorf("LatestRun reported a run over a gone database: %+v", run)
	} else {
		fails("LatestRun", err)
	}
	if has, err := s.HasScheduledRun(ctx, "2026-09-06"); has {
		t.Error("HasScheduledRun said tonight had happened over a gone database")
	} else {
		fails("HasScheduledRun", err)
	}
	if run, err := s.StartRun(ctx, "2026-09-06", false, clock.now()); run.ID != 0 {
		t.Errorf("StartRun handed back a run it never wrote: %+v", run)
	} else {
		fails("StartRun", err)
	}
	fails("PlanBouts", s.PlanBouts(ctx, 1, []night.Plan{
		{Seats: []night.Seat{{Slug: "one"}, {Slug: "two"}}, Games: 3, Clock: 300, Seed: 7},
	}))
	if bouts, err := s.Bouts(ctx, 1); bouts != nil {
		t.Errorf("Bouts handed back %d bouts over a gone database", len(bouts))
	} else {
		fails("Bouts", err)
	}
	if bouts, err := s.Playing(ctx, 1); bouts != nil {
		t.Errorf("Playing handed back %d bouts over a gone database", len(bouts))
	} else {
		fails("Playing", err)
	}
	if b, ok, err := s.ClaimNext(ctx, 1); ok || b.ID != 0 {
		t.Errorf("ClaimNext claimed a bout over a gone database: %+v", b)
	} else {
		fails("ClaimNext", err)
	}
	fails("MarkDone", s.MarkDone(ctx, 1, 2))
	fails("MarkFailed", s.MarkFailed(ctx, 1, "the reason"))
	fails("MarkSkipped", s.MarkSkipped(ctx, 1, "the reason"))
	if n, err := s.SkipRemaining(ctx, 1, "the window closed"); n != 0 {
		t.Errorf("SkipRemaining reported %d rows skipped over a gone database", n)
	} else {
		fails("SkipRemaining", err)
	}
	if n, err := s.FailPlaying(ctx, 1, "the process restarted"); n != 0 {
		t.Errorf("FailPlaying reported %d rows failed over a gone database", n)
	} else {
		fails("FailPlaying", err)
	}
	fails("CloseRun", s.CloseRun(ctx, 1))
	fails("FinishRun", s.FinishRun(ctx, 1))
	if seats, err := s.PlayerDecks(ctx); seats != nil {
		t.Errorf("PlayerDecks mustered %d seats over a gone database", len(seats))
	} else {
		fails("PlayerDecks", err)
	}
	if in, err := s.Entered(ctx, 1, "gyome"); in {
		t.Error("Entered reported standing consent over a gone database")
	} else {
		fails("Entered", err)
	}

	// A sweep that sweeps nothing passes: this floor is what stops a
	// refactor that renames half these methods from leaving a green test
	// asking three questions.
	if asked < 17 {
		t.Fatalf("the sweep asked %d of the store's questions; it should ask them all", asked)
	}
}

// StartRun is three statements long and each of the last two has a branch
// that a closed handle cannot reach. What matters is the same thing every
// time: when the write did not land, no row is left behind and no run is
// handed back.
func TestARunThatDiesMidTransactionLeavesNoRunBehind(t *testing.T) {
	t.Parallel()
	s, fault, clock := faultyNight(t)
	ctx := context.Background()
	closes := clock.now().Add(time.Hour)

	for _, budget := range []int{1, 2, 3} {
		fault.Heal()
		// The BEGIN, the "is one already open" read, the INSERT, the COMMIT:
		// the budget picks which of them is the last one to land.
		fault.After(budget)
		run, err := s.StartRun(ctx, "2026-09-06", false, closes)
		fault.Heal()
		if err == nil {
			t.Fatalf("a run opened with only %d statements' worth of database", budget)
		}
		if run.ID != 0 {
			t.Fatalf("a run that did not commit was handed back anyway: %+v", run)
		}
		if _, ok, err := s.LatestRun(ctx); err != nil {
			t.Fatal(err)
		} else if ok {
			t.Fatalf("a run row survived a transaction that died at statement %d", budget+1)
		}
	}

	// And the same store opens a night perfectly well once the volume is
	// back — the fixture arms a fault, it does not break the file.
	fault.Heal()
	if _, err := s.StartRun(ctx, "2026-09-06", false, closes); err != nil {
		t.Fatalf("the store did not recover: %v", err)
	}
	if _, ok, err := s.LatestRun(ctx); err != nil || !ok {
		t.Fatalf("the recovered run is not there: ok=%v err=%v", ok, err)
	}
}

// A card is planned whole or not at all, and the two statements it takes per
// bout are two chances to stop halfway: a bout row with no seats is a bout
// the runner cannot fight, and it must not survive the failure that made it.
func TestACardThatDiesMidDealPlansNoBoutAtAll(t *testing.T) {
	t.Parallel()
	s, fault, clock := faultyNight(t)
	ctx := context.Background()
	run, err := s.StartRun(ctx, "2026-09-06", false, clock.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plans := []night.Plan{
		{Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "goreclaw"}}, Games: 3, Clock: 300, Seed: 1},
		{Seats: []night.Seat{{Slug: "atla"}, {Slug: "tivit"}}, Games: 3, Clock: 300, Seed: 2},
	}
	// Budget 1 stops on the first bout's own INSERT; budget 2 stops on its
	// first seat, with the bout row already written inside the transaction.
	for _, budget := range []int{1, 2} {
		fault.Heal()
		fault.After(budget)
		err := s.PlanBouts(ctx, run.ID, plans)
		fault.Heal()
		if err == nil {
			t.Fatalf("a card was dealt with only %d statements' worth of database", budget)
		}
		bouts, err := s.Bouts(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(bouts) != 0 {
			t.Fatalf("%d bout rows survived a deal that died at statement %d",
				len(bouts), budget+1)
		}
	}
	fault.Heal()
	if err := s.PlanBouts(ctx, run.ID, plans); err != nil {
		t.Fatal(err)
	}
	if bouts, err := s.Bouts(ctx, run.ID); err != nil || len(bouts) != 2 {
		t.Fatalf("the recovered deal wrote %d bouts: %v", len(bouts), err)
	}
}

// The claim is the transaction the whole one-bout-at-a-time promise rests
// on: five statements, and a bout must come back either fully seated and
// marked playing, or not at all. A bout handed over without its seats, or
// marked playing without being handed over, are both nights that lose work.
func TestAClaimThatDiesMidTransactionHandsBackNothingAndMarksNothing(t *testing.T) {
	t.Parallel()
	s, fault, clock := faultyNight(t)
	ctx := context.Background()
	run, err := s.StartRun(ctx, "2026-09-06", false, clock.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlanBouts(ctx, run.ID, []night.Plan{
		{Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "goreclaw"}}, Games: 3, Clock: 300, Seed: 1},
	}); err != nil {
		t.Fatal(err)
	}

	// BEGIN, the in-flight check, the next-bout read, the seats, the UPDATE,
	// the COMMIT — each budget stops one statement later than the last.
	for _, budget := range []int{1, 2, 3, 4, 5} {
		fault.Heal()
		fault.After(budget)
		bout, ok, err := s.ClaimNext(ctx, run.ID)
		fault.Heal()
		if err == nil {
			t.Fatalf("a bout was claimed with only %d statements' worth of database", budget)
		}
		if ok || bout.ID != 0 {
			t.Fatalf("a failed claim handed back a bout: %+v", bout)
		}
		bouts, err := s.Bouts(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(bouts) != 1 || bouts[0].State != night.StatePlanned {
			t.Fatalf("after a claim that died at statement %d the bout is %v",
				budget+1, bouts[0].State)
		}
	}

	fault.Heal()
	bout, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("the recovered claim: ok=%v err=%v", ok, err)
	}
	if len(bout.Seats) != 2 || bout.State != night.StatePlaying {
		t.Fatalf("the recovered claim handed back %+v", bout)
	}
}

// A run's bouts are two queries — the rows, then every seat in one go — and
// the second one failing must not hand back bouts with no seats in them.
func TestBoutsWithoutTheirSeatsAreARefusalRatherThanAnEmptyChair(t *testing.T) {
	t.Parallel()
	s, fault, clock := faultyNight(t)
	ctx := context.Background()
	run, err := s.StartRun(ctx, "2026-09-06", false, clock.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlanBouts(ctx, run.ID, []night.Plan{
		{Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "goreclaw"}}, Games: 3, Clock: 300, Seed: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimNext(ctx, run.ID); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"Bouts", func() error { _, err := s.Bouts(ctx, run.ID); return err }},
		{"Playing", func() error { _, err := s.Playing(ctx, run.ID); return err }},
	} {
		fault.Heal()
		// One statement: the bout rows land, the seats do not.
		fault.After(1)
		err := tc.call()
		fault.Heal()
		if err == nil {
			t.Errorf("%s handed back bouts whose seats it could not read", tc.name)
		}
	}
}

// Settling is one UPDATE and it refuses to rewrite history: a bout that has
// already settled cannot settle again, and the refusal names the bout rather
// than shrugging. This is the branch beside the gone-database one — the
// database is fine and the caller is confused.
func TestASettledBoutRefusesToSettleTwice(t *testing.T) {
	t.Parallel()
	s, _, clock := faultyNight(t)
	ctx := context.Background()
	run, err := s.StartRun(ctx, "2026-09-06", false, clock.now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlanBouts(ctx, run.ID, []night.Plan{
		{Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "goreclaw"}}, Games: 3, Clock: 300, Seed: 1},
	}); err != nil {
		t.Fatal(err)
	}
	bout, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if err := s.MarkDone(ctx, bout.ID, 0); err != nil {
		t.Fatal(err)
	}
	for name, again := range map[string]error{
		"done again":    s.MarkDone(ctx, bout.ID, 5),
		"failed after":  s.MarkFailed(ctx, bout.ID, "too late"),
		"skipped after": s.MarkSkipped(ctx, bout.ID, "too late"),
	} {
		if again == nil {
			t.Errorf("a settled bout was %s without complaint", name)
		}
	}
	// A bout that played and went unrecorded is done with nothing to join
	// on, rather than done pointing at a match that does not exist.
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bouts[0].State != night.StateDone || bouts[0].MatchID != nil {
		t.Fatalf("a declined ledger row left %v / %v", bouts[0].State, bouts[0].MatchID)
	}

	// And a run finishes once: the second finish is the same refusal one
	// level up, and so is closing a run that is already over.
	if err := s.FinishRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRun(ctx, run.ID); err == nil {
		t.Error("a finished run finished a second time")
	}
	if err := s.CloseRun(ctx, run.ID); err == nil {
		t.Error("a finished run was closed as though it were still open")
	}
	if _, ok, err := s.OpenRun(ctx); err != nil || ok {
		t.Fatalf("a finished run still reads as the open one: ok=%v err=%v", ok, err)
	}
}
