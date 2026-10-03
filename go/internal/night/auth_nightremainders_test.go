package night_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The night's last branches, and they are two shapes.
//
// **A row the night cannot read.** Every read here ends in a scan, and a scan
// that fails has exactly one honest answer: say so. The dangerous answer is
// the quiet one -- a bout in flight reported as none in flight is a bout the
// orphan sweep fails and a seat the scheduler hands out again -- so each of
// these asks for the error *and* for the absence of an invented answer.
// SQLite applies column affinity rather than enforcing it, so one hand-run
// UPDATE through a handle with foreign keys off leaves text in an INTEGER
// column: the shape a half-finished restore or an incident repair leaves, and
// the only fixture that reaches a scan's refusal.
//
// **A scheduler that defers.** The two `return`s that mean "not now" rather
// than "something is wrong": a house nobody configured, and a bout already
// fighting while the window is still open. ADR 46 decision: one bout at a
// time, and the person in the room outranks the schedule.

// poisonableNight is a night store over a real scratch `app.db`, plus a way to
// write a value into it that a scan cannot take. The poison goes through its
// own handle with foreign keys off, which is what a repair by hand is, and one
// row at a time -- a whole-table UPDATE is refused by any UNIQUE the column
// takes part in.
func poisonableNight(t *testing.T) (*night.Store, *ticking, func(string)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db := auth.OpenReadWrite(path)
	t.Cleanup(func() { _ = db.Close() })
	clock := &ticking{at: time.Date(2026, 9, 6, 22, 0, 0, 0, time.UTC)}
	poison := func(statement string) {
		t.Helper()
		w, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w.Close() }()
		if _, err := w.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return night.FromDB(db, clock.now), clock, poison
}

// A bout in flight that cannot be read is said out loud, never reported as
// nothing in flight: the runner reads this to decide whether a row is an
// orphan some dead process left, and an empty answer is the one that fails a
// bout somebody is still playing.
func TestABoutInFlightThatCannotBeReadIsNotReportedAsNoneInFlight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, clock, poison := poisonableNight(t)

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ClaimNext(ctx, run.ID); err != nil || !ok {
		t.Fatalf("no bout was claimed: ok=%v err=%v", ok, err)
	}
	if in, err := s.Playing(ctx, run.ID); err != nil || len(in) != 1 {
		t.Fatalf("the healthy read is (%d bouts, %v)", len(in), err)
	}

	poison(`UPDATE night_bouts SET games = 'a handful'` +
		` WHERE state = 'playing'`)

	in, err := s.Playing(ctx, run.ID)
	if err == nil {
		t.Fatalf("a bout that cannot be read came back as %d bouts in flight", len(in))
	}
	if in != nil {
		t.Errorf("a failed read also returned %d bouts", len(in))
	}
}

// A seat that cannot be read refuses the whole card rather than handing back
// bouts with seats missing. A pod with a chair dropped out of it is a
// different night from the one that was dealt.
func TestASeatThatCannotBeReadRefusesTheWholeCard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, clock, poison := poisonableNight(t)

	run, err := s.StartRun(ctx, "2026-09-06", false, clock.at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PlanBouts(ctx, run.ID, plans()); err != nil {
		t.Fatal(err)
	}
	if bouts, err := s.Bouts(ctx, run.ID); err != nil || len(bouts) != len(plans()) {
		t.Fatalf("the healthy card is (%d bouts, %v)", len(bouts), err)
	}

	// `night_bout_seats.owner_id` carries no REFERENCES by design (rung 14
	// argues it), so a seat's owner is the one column here a repair can leave
	// unreadable without the schema noticing.
	poison(`UPDATE night_bout_seats SET owner_id = 'somebody'` +
		` WHERE rowid = (SELECT MIN(rowid) FROM night_bout_seats)`)

	bouts, err := s.Bouts(ctx, run.ID)
	if err == nil {
		t.Fatalf("a card with an unreadable seat came back as %d bouts", len(bouts))
	}
	// The rows read before the seats failed are handed back beside the error,
	// which is this reader's shape -- what must not happen is a seated card:
	// bouts with *some* of their chairs filled would be a different night
	// presented as the one that was dealt.
	for _, b := range bouts {
		if len(b.Seats) != 0 {
			t.Errorf("bout %d came back seated beside the error: %+v", b.ID, b.Seats)
		}
	}
}

// A player's deck row that cannot be read refuses the roster rather than
// dealing a night without that player in it. The deal is a function of the
// roster, so a roster quietly one deck short is a night nobody can reproduce.
func TestAPlayerDeckThatCannotBeReadRefusesTheRoster(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, poison := poisonableNight(t)

	poison(`INSERT INTO users (id, username, is_admin, created_at)` +
		` VALUES (7, 'ada', 0, '2026-09-06T22:00:00+00:00');` +
		` INSERT INTO user_decks (owner_id, slug, name, yaml, coliseum_at_night,` +
		` created_at, updated_at) VALUES (7, 'gyome', 'Gyome', 'name: Gyome', 1,` +
		` '2026-09-06T22:00:00+00:00', '2026-09-06T22:00:00+00:00')`)
	if decks, err := s.PlayerDecks(ctx); err != nil || len(decks) != 1 {
		t.Fatalf("the healthy roster is (%d decks, %v)", len(decks), err)
	}

	poison(`UPDATE user_decks SET owner_id = 'ada' WHERE slug = 'gyome'`)

	decks, err := s.PlayerDecks(ctx)
	if err == nil {
		t.Fatalf("an unreadable deck row came back as a roster of %d", len(decks))
	}
	if decks != nil {
		t.Errorf("a failed roster also returned %d decks", len(decks))
	}
}

// Every one of the three counts refuses a value that is not a number, and each
// says which switch it means. The middle one had never been asked: the table in
// `settings_test.go` checks a non-numeric bouts and games, and a per-account
// share of *zero* -- which is refused by the floor below rather than by the
// read, so the read's own refusal went untested for the one switch of three.
func TestEveryNightCountNamesItsOwnSwitchWhenItIsNotANumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		cfg   config.Config
		names string
	}{
		{config.Config{NightBouts: "a few"}, "MTGLAB_NIGHT_BOUTS"},
		{config.Config{NightBoutsPerAccount: "a few"}, "MTGLAB_NIGHT_BOUTS_PER_ACCOUNT"},
		{config.Config{NightGames: "a few"}, "MTGLAB_NIGHT_GAMES"},
	} {
		got, err := night.SettingsFromConfig(tc.cfg)
		if err == nil {
			t.Errorf("%s accepted a word as a count: %+v", tc.names, got)
			continue
		}
		if !strings.Contains(err.Error(), tc.names) {
			t.Errorf("the refusal does not name %s: %v", tc.names, err)
		}
		if got != (night.Settings{}) {
			t.Errorf("%s: a refused deployment handed back %+v", tc.names, got)
		}
	}
}

// A night with no house configured deals nothing. The house seam is nil on
// every surface that only wants a sample, and nil has to mean "an empty
// house" rather than a crash or a night of nobody against nobody.
func TestANightWithNoHouseConfiguredDealsAnEmptyCard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
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
	// No House, no LaneBusy, no Settled: the zero values production leaves on
	// the surfaces that do not wire them.
	r := night.NewRunner(night.RunnerConfig{Store: s, Settings: tonight(t),
		Player: &fakeArena{},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    clock.now, Interval: time.Hour})
	t.Cleanup(r.Stop)

	r.Tick(ctx)
	run, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("no run opened inside the window: ok=%v err=%v", ok, err)
	}
	bouts, err := s.Bouts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bouts) != 0 {
		t.Fatalf("a night with nobody in it dealt %d bouts", len(bouts))
	}
}

// One bout at a time: a tick that lands while a bout is still fighting claims
// nothing, and the rest of the card waits for the settle to nudge the next
// one. ADR 46 decision 6 -- and the row state is the assertion, because
// "claimed nothing" is only visible as the second bout still being planned.
func TestASecondTickWhileABoutIsFightingClaimsNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	arena := &fakeArena{gate: make(chan struct{}), entered: make(chan struct{})}
	r, s, clock, settled := quietRunner(t, tonight(t), arena, nil,
		[]string{"kaheera", "goreclaw", "atla"})

	clock.set(time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC))
	r.Tick(ctx)
	<-arena.entered // the first bout's row is `playing` and the fight is parked

	run, ok, err := s.OpenRun(ctx)
	if err != nil || !ok {
		t.Fatalf("no run is open: ok=%v err=%v", ok, err)
	}
	before := boutStates(t, s, run.ID)
	if before[night.StatePlaying] != 1 {
		t.Fatalf("the card is %v, want exactly one bout in flight", before)
	}

	// The tick that is the whole point: the window is open, there is more
	// card to deal, and the lane is this night's own bout.
	r.Tick(ctx)
	if got := boutStates(t, s, run.ID); got[night.StatePlaying] != 1 ||
		got[night.StatePlanned] != before[night.StatePlanned] {
		t.Errorf("a tick during a fight moved the card from %v to %v", before, got)
	}
	if fights := arena.fights(); len(fights) != 1 {
		t.Errorf("the arena was asked to play %d bouts while one was in flight",
			len(fights))
	}

	// Let it finish, so the cleanup's Stop has nothing parked to wait on.
	close(arena.gate)
	<-settled
}
