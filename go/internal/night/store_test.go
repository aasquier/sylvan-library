package night_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// ticking is the injectable clock: it starts where a test says and moves one
// second per look, so no two rows share an updated_at and nothing here reads
// the wall. Not safe for concurrent use, and no test here shares one.
type ticking struct{ at time.Time }

func (c *ticking) now() time.Time {
	c.at = c.at.Add(time.Second)
	return c.at
}

// scratch is a night store over a real app.db built from the recorded schema
// — the same rows the deployed ladder builds, foreign keys and partial
// indexes included, which is what the store's refusals are about.
func scratch(t *testing.T) (*night.Store, *ticking) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &ticking{at: time.Date(2026, 9, 6, 6, 0, 0, 0, time.UTC)}
	return night.FromDB(db, clock.now), clock
}

func owned(id int64, slug string) night.Seat { return night.Seat{Owner: &id, Slug: slug} }

func house(slug string) night.Seat { return night.Seat{Slug: slug} }

func plans() []night.Plan {
	return []night.Plan{
		{SeatA: owned(1, "gyome"), SeatB: house("goreclaw"), Games: 10, Seed: 41},
		{SeatA: owned(2, "tivit"), SeatB: owned(1, "arahbo"), Games: 10, Seed: 42},
		{SeatA: house("atla"), SeatB: house("trostani"), Games: 10, Seed: 43},
	}
}

func TestARunAndItsBoutsRoundTrip(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	closes := clock.at.Add(2 * time.Hour)
	run, err := s.StartRun(ctx, "2026-09-06", false, closes)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.ID == 0 || run.NightKey != "2026-09-06" || run.Sample || !run.Open() {
		t.Fatalf("the run came back wrong: %+v", run)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}

	got, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("OpenRun: ok=%v err=%v", ok, err)
	}
	if got.ID != run.ID || got.NightKey != run.NightKey ||
		!got.ClosesAt.Equal(closes.Truncate(time.Microsecond)) ||
		!got.OpenedAt.Equal(run.OpenedAt) || got.FinishedAt != nil {
		t.Errorf("the run read back different:\n got %+v\nwant %+v", got, run)
	}

	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatalf("Bouts: %v", err)
	}
	if len(bouts) != 3 {
		t.Fatalf("planned 3 bouts, read %d", len(bouts))
	}
	for i, want := range plans() {
		b := bouts[i]
		if b.State != night.StatePlanned || b.Reason != "" || b.MatchID != nil {
			t.Errorf("bout %d was born %s/%q/%v, want planned and blank", i, b.State, b.Reason, b.MatchID)
		}
		if b.Games != want.Games || b.Seed != want.Seed ||
			b.SeatA.Slug != want.SeatA.Slug || b.SeatB.Slug != want.SeatB.Slug {
			t.Errorf("bout %d read back different: %+v", i, b)
		}
		if b.SeatA.House() != want.SeatA.House() || b.SeatB.House() != want.SeatB.House() {
			t.Errorf("bout %d confused the house with a player: %+v", i, b)
		}
		if want.SeatA.Owner != nil && (b.SeatA.Owner == nil || *b.SeatA.Owner != *want.SeatA.Owner) {
			t.Errorf("bout %d lost seat A's owner: %+v", i, b.SeatA)
		}
	}
	// The order the pairing dealt is the order the night plays.
	if bouts[0].Seed != 41 || bouts[1].Seed != 42 || bouts[2].Seed != 43 {
		t.Errorf("the plan's order did not survive: %d %d %d",
			bouts[0].Seed, bouts[1].Seed, bouts[2].Seed)
	}
}

func TestOneNightAtATime(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := s.StartRun(ctx, "2026-09-07", false, clock.at.Add(time.Hour)); err == nil {
		t.Fatal("a second run opened while the first was still open")
	} else if !strings.Contains(err.Error(), "one night at a time") {
		t.Errorf("the refusal should say why: %v", err)
	}
	// A sample is a run like any other for this rule: the lane it feeds is
	// one wide.
	if _, err := s.StartRun(ctx, "2026-09-06", true, clock.at.Add(time.Hour)); err == nil {
		t.Fatal("a sample opened over the open run")
	}

	if err := s.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	// With the night finished the arena is free — but not for the same
	// scheduled night twice: that refusal is the schema's own.
	if _, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour)); err == nil {
		t.Fatal("tonight was scheduled twice")
	}
	if _, err := s.StartRun(ctx, "2026-09-07", false, clock.at.Add(time.Hour)); err != nil {
		t.Fatalf("the next night was refused: %v", err)
	}
}

func TestHasScheduledRunRemembersFinishedNightsAndIgnoresSamples(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	if has, err := s.HasScheduledRun(ctx, "2026-09-06"); err != nil || has {
		t.Fatalf("an empty ledger claims a night: has=%v err=%v", has, err)
	}
	sample, err := s.StartRun(ctx, "2026-09-06", true, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatalf("StartRun sample: %v", err)
	}
	// A sample on the date is not the scheduled night having happened.
	if has, _ := s.HasScheduledRun(ctx, "2026-09-06"); has {
		t.Error("a sample run counted as the scheduled night")
	}
	if err := s.FinishRun(ctx, sample.ID); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	// Still true after the night finishes — a long window must not reopen it.
	if has, _ := s.HasScheduledRun(ctx, "2026-09-06"); !has {
		t.Error("the finished night was forgotten, so a long window would reopen it")
	}
}

func TestClaimNextHandsOutBoutsOneAtATimeInOrder(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}

	first, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("ClaimNext: ok=%v err=%v", ok, err)
	}
	if first.Seed != 41 || first.State != night.StatePlaying {
		t.Fatalf("the first claim was not the first planned bout, playing: %+v", first)
	}
	// One in flight means nothing more is handed out, however often asked.
	if _, ok, err := s.ClaimNext(ctx, run.ID); ok || err != nil {
		t.Fatalf("a second bout was claimed while one was in flight: ok=%v err=%v", ok, err)
	}

	if err := s.MarkDone(ctx, first.ID, 900); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	second, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok || second.Seed != 42 {
		t.Fatalf("the second claim should be the second bout: %+v ok=%v err=%v", second, ok, err)
	}
	if err := s.MarkFailed(ctx, second.ID, "the shim went quiet"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	third, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok || third.Seed != 43 {
		t.Fatalf("the third claim should be the last bout: ok=%v err=%v", ok, err)
	}
	if err := s.MarkSkipped(ctx, third.ID, "2 of 100 cards are strangers to the arena"); err != nil {
		t.Fatalf("MarkSkipped: %v", err)
	}
	// The card is spent.
	if _, ok, _ := s.ClaimNext(ctx, run.ID); ok {
		t.Error("a bout was claimed from a night with nothing left")
	}

	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatalf("Bouts: %v", err)
	}
	if bouts[0].State != night.StateDone || bouts[0].MatchID == nil || *bouts[0].MatchID != 900 {
		t.Errorf("the done bout should carry its match: %+v", bouts[0])
	}
	if bouts[1].State != night.StateFailed || bouts[1].Reason != "the shim went quiet" {
		t.Errorf("the failed bout should carry its diagnosis: %+v", bouts[1])
	}
	if bouts[2].State != night.StateSkipped || bouts[2].MatchID != nil {
		t.Errorf("the skipped bout should carry no match: %+v", bouts[2])
	}
}

func TestASettledBoutStaysSettled(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()[:1]); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}
	bout, _, err := s.ClaimNext(ctx, run.ID)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}

	// A bout that has not played cannot be done: done means a match id, and
	// only a playing bout has been anywhere near one.
	if err := s.MarkDone(ctx, bout.ID+999, 1); err == nil {
		t.Error("a bout that does not exist was marked done")
	}
	if err := s.MarkDone(ctx, bout.ID, 900); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	for name, attempt := range map[string]func() error{
		"done again":  func() error { return s.MarkDone(ctx, bout.ID, 901) },
		"failed late": func() error { return s.MarkFailed(ctx, bout.ID, "no") },
		"skipped late": func() error {
			return s.MarkSkipped(ctx, bout.ID, "no")
		},
	} {
		if err := attempt(); err == nil {
			t.Errorf("a settled bout was rewritten (%s)", name)
		}
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatalf("Bouts: %v", err)
	}
	if bouts[0].State != night.StateDone || *bouts[0].MatchID != 900 {
		t.Errorf("the settled bout moved: %+v", bouts[0])
	}
	// A planned bout can fail or skip without ever playing — a submit that
	// refused on the spot, a pre-flight that said no.
	if err := s.PlanBouts(ctx, run.ID, plans()[1:]); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}
	bouts, _ = s.Bouts(ctx, run.ID)
	if err := s.MarkFailed(ctx, bouts[1].ID, "the lane refused the job"); err != nil {
		t.Errorf("a planned bout could not fail on the spot: %v", err)
	}
	if err := s.MarkSkipped(ctx, bouts[2].ID, "the gate cannot play it"); err != nil {
		t.Errorf("a planned bout could not be skipped: %v", err)
	}
}

func TestTheWindowCloseSkipsTheRemainderAndOnlyTheRemainder(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}
	first, _, err := s.ClaimNext(ctx, run.ID)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}
	if err := s.MarkDone(ctx, first.ID, 900); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	second, _, err := s.ClaimNext(ctx, run.ID)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}

	// The window closes with one bout done, one in flight, one still planned.
	n, err := s.SkipRemaining(ctx, run.ID, "the window closed")
	if err != nil {
		t.Fatalf("SkipRemaining: %v", err)
	}
	if n != 1 {
		t.Errorf("skipped %d bouts, want just the planned one", n)
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatalf("Bouts: %v", err)
	}
	if bouts[0].State != night.StateDone {
		t.Errorf("the finished bout was disturbed: %+v", bouts[0])
	}
	// The bout in flight finishes (ADR 46 decision 6) — the sweep must not
	// touch it.
	if bouts[1].State != night.StatePlaying {
		t.Errorf("the sweep abandoned the bout in flight: %+v", bouts[1])
	}
	if bouts[2].State != night.StateSkipped || bouts[2].Reason != "the window closed" {
		t.Errorf("the planned bout should be skipped with the reason: %+v", bouts[2])
	}
	if err := s.MarkDone(ctx, second.ID, 901); err != nil {
		t.Errorf("the in-flight bout could not finish after the close: %v", err)
	}

	// Skipping again finds nothing: zero is a night that spent its card, not
	// an error.
	if n, err := s.SkipRemaining(ctx, run.ID, "the window closed"); err != nil || n != 0 {
		t.Errorf("a second sweep should find nothing: n=%d err=%v", n, err)
	}
}

// The restart story: the job registry's memory is gone, so a bout still
// marked playing is an orphan. It is re-marked failed with the reason, its
// match_id stays NULL — whether the match recorded before the process died
// is genuinely unknown — and it is never handed out again.
func TestARestartFailsTheOrphanedBoutHonestly(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()[:2]); err != nil {
		t.Fatalf("PlanBouts: %v", err)
	}
	orphan, _, err := s.ClaimNext(ctx, run.ID)
	if err != nil {
		t.Fatalf("ClaimNext: %v", err)
	}

	// The process restarts here. The resume read finds the orphan...
	playing, err := s.Playing(ctx, run.ID)
	if err != nil {
		t.Fatalf("Playing: %v", err)
	}
	if len(playing) != 1 || playing[0].ID != orphan.ID {
		t.Fatalf("the resume read should find the orphan: %+v", playing)
	}
	// ...and the sweep settles it.
	n, err := s.FailPlaying(ctx, run.ID, "the process restarted mid-bout")
	if err != nil || n != 1 {
		t.Fatalf("FailPlaying: n=%d err=%v", n, err)
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatalf("Bouts: %v", err)
	}
	if bouts[0].State != night.StateFailed ||
		bouts[0].Reason != "the process restarted mid-bout" || bouts[0].MatchID != nil {
		t.Errorf("the orphan should be failed, explained, and matchless: %+v", bouts[0])
	}
	// The night then goes on with the next bout, not the orphan again.
	next, ok, err := s.ClaimNext(ctx, run.ID)
	if err != nil || !ok || next.ID == orphan.ID {
		t.Errorf("the night should move on: %+v ok=%v err=%v", next, ok, err)
	}
}

// NewStore is the standalone way in — its own `mode=rw` handle over the file
// the ladder built — and a nil clock means the wall's, which is what every
// caller outside a test wants.
func TestNewStoreOpensTheLadderBuiltFileAndCloseLetsGo(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	s, err := night.NewStore(path, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, ok, err := s.OpenRun(context.Background()); err != nil || ok {
		t.Errorf("a fresh file has no open run: ok=%v err=%v", ok, err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	var absent *night.Store
	if err := absent.Close(); err != nil {
		t.Errorf("a nil store should close as a no-op: %v", err)
	}
}

// A row whose timestamp is not the recorded format is an error with the bad
// bytes in it, never a zero time a comparison would quietly treat as the
// distant past — the runner compares closes_at to now, and a zero there
// would close every window ever.
func TestAMangledTimestampIsAnErrorNotAZeroTime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	s := night.FromDB(db, nil)
	good := "2026-09-06T06:00:00+00:00"
	// One run per mangled column, inserted highest-id-last so each read hits
	// exactly the row whose bytes are bad.
	if _, err := db.Exec(
		`INSERT INTO night_runs (id, night_key, sample, opened_at, closes_at)
		 VALUES (1, '2026-09-06', 0, 'last tuesday', ?)`, good); err != nil {
		t.Fatalf("seeding the mangled run: %v", err)
	}
	if _, _, err := s.OpenRun(ctx); err == nil ||
		!strings.Contains(err.Error(), "last tuesday") {
		t.Errorf("a mangled opened_at should be an error carrying the bad bytes: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO night_runs (id, night_key, sample, opened_at, closes_at)
		 VALUES (2, '2026-09-07', 1, ?, 'whenever')`, good); err != nil {
		t.Fatalf("seeding the mangled run: %v", err)
	}
	if _, _, err := s.OpenRun(ctx); err == nil ||
		!strings.Contains(err.Error(), "whenever") {
		t.Errorf("a mangled closes_at should be an error carrying the bad bytes: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO night_runs (id, night_key, sample, opened_at, closes_at, finished_at)
		 VALUES (3, '2026-09-08', 1, ?, ?, 'never')`, good, good); err != nil {
		t.Fatalf("seeding the mangled run: %v", err)
	}
	if _, _, err := s.LatestRun(ctx); err == nil ||
		!strings.Contains(err.Error(), "never") {
		t.Errorf("a mangled finished_at should be an error carrying the bad bytes: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO night_bouts (run_id, seat_a_slug, seat_b_slug, games, seed,
		                          state, created_at, updated_at)
		 VALUES (1, 'gyome', 'arahbo', 10, 7, 'planned', 'dawn', ?),
		        (2, 'tivit', 'atla', 10, 8, 'planned', ?, 'dusk')`,
		good, good); err != nil {
		t.Fatalf("seeding the mangled bouts: %v", err)
	}
	if _, err := s.Bouts(ctx, 1); err == nil ||
		!strings.Contains(err.Error(), "dawn") {
		t.Errorf("a mangled created_at should be an error carrying the bad bytes: %v", err)
	}
	if _, err := s.Bouts(ctx, 2); err == nil ||
		!strings.Contains(err.Error(), "dusk") {
		t.Errorf("a mangled updated_at should be an error carrying the bad bytes: %v", err)
	}
}

// A store over a handle the database has taken away reports every verb as an
// error — never a false, a zero count, or an absent run that would read as a
// quiet night. The runner treats these as "skip this tick", and it can only
// do that if they arrive as errors.
func TestABrokenHandleReportsInsteadOfInventing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := night.FromDB(db, nil)
	ctx := context.Background()
	if _, err := s.StartRun(ctx, "2026-09-06", false, time.Now()); err == nil {
		t.Error("StartRun invented a run")
	}
	if err := s.PlanBouts(ctx, 1, plans()); err == nil {
		t.Error("PlanBouts claimed to plan")
	}
	if _, _, err := s.OpenRun(ctx); err == nil {
		t.Error("OpenRun answered as if it had looked")
	}
	if _, err := s.HasScheduledRun(ctx, "2026-09-06"); err == nil {
		t.Error("HasScheduledRun answered as if it had looked")
	}
	if _, err := s.Bouts(ctx, 1); err == nil {
		t.Error("Bouts invented a roster")
	}
	if _, err := s.Playing(ctx, 1); err == nil {
		t.Error("Playing invented orphans")
	}
	if _, _, err := s.ClaimNext(ctx, 1); err == nil {
		t.Error("ClaimNext claimed from nothing")
	}
	if err := s.MarkDone(ctx, 1, 1); err == nil {
		t.Error("MarkDone settled nothing and said so with silence")
	}
	if _, err := s.SkipRemaining(ctx, 1, "x"); err == nil {
		t.Error("SkipRemaining counted a sweep it never made")
	}
	if _, err := s.FailPlaying(ctx, 1, "x"); err == nil {
		t.Error("FailPlaying counted a sweep it never made")
	}
	if err := s.CloseRun(ctx, 1); err == nil {
		t.Error("CloseRun closed nothing quietly")
	}
	if err := s.FinishRun(ctx, 1); err == nil {
		t.Error("FinishRun finished nothing quietly")
	}
}

func TestCloseRunPullsTheDeadlineAndFinishRunEndsTheNight(t *testing.T) {
	t.Parallel()
	s, clock := scratch(t)
	ctx := context.Background()

	run, err := s.StartRun(ctx, "2026-09-06", true, clock.at.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := s.CloseRun(ctx, run.ID); err != nil {
		t.Fatalf("CloseRun: %v", err)
	}
	got, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("OpenRun after close: ok=%v err=%v", ok, err)
	}
	if !got.Sample {
		t.Error("the sample mark did not survive the round trip")
	}
	if !got.ClosesAt.Before(run.ClosesAt) {
		t.Errorf("the close did not pull the deadline in: %v -> %v", run.ClosesAt, got.ClosesAt)
	}
	if !got.Open() {
		t.Error("closing the window is not finishing the run")
	}

	if err := s.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if _, ok, err := s.OpenRun(ctx); err != nil || ok {
		t.Errorf("a finished run is not the open one: ok=%v err=%v", ok, err)
	}
	latest, ok, err := s.LatestRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestRun: ok=%v err=%v", ok, err)
	}
	if latest.ID != run.ID || latest.FinishedAt == nil || latest.Open() {
		t.Errorf("the admin's read should still show the finished night: %+v", latest)
	}

	// Both verbs refuse a run that is already over.
	if err := s.CloseRun(ctx, run.ID); err == nil {
		t.Error("a finished run was closed")
	}
	if err := s.FinishRun(ctx, run.ID); err == nil {
		t.Error("a finished run was finished again")
	}
	// And an empty ledger answers plainly.
	if _, ok, err := s.LatestRun(ctx); err != nil || !ok {
		t.Errorf("LatestRun forgot the night: ok=%v err=%v", ok, err)
	}
}
