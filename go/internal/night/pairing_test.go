package night_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The deal is asserted, not eyeballed: every test here recomputes a plan from
// the same inputs and reads properties off it — fairness, caps, house fill,
// determinism — because the pairing is a pure function and pure functions are
// what property assertions are for.

func caps(bouts, perAccount, games int) night.Settings {
	return night.Settings{Bouts: bouts, BoutsPerAccount: perAccount, Games: games}
}

// scratchAndDB is [scratch] with the raw handle alongside, for a test that
// has to seed `user_decks` rows underneath the store.
func scratchAndDB(t *testing.T) (*night.Store, *sql.DB) {
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
	return night.FromDB(db, clock.now), db
}

func mustExec(t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("%v\n%s", err, stmt)
	}
}

// appearances counts how many bouts each account's decks sit in; the house is
// counted under owner 0, which no account holds (rowids begin at one).
func appearances(plans []night.Plan) map[int64]int {
	out := map[int64]int{}
	count := func(s night.Seat) {
		if s.Owner == nil {
			out[0]++
			return
		}
		out[*s.Owner]++
	}
	for _, p := range plans {
		count(p.SeatA)
		count(p.SeatB)
	}
	return out
}

func shelfOf(owner int64, slugs ...string) []night.Seat {
	out := make([]night.Seat, 0, len(slugs))
	for _, s := range slugs {
		out = append(out, owned(owner, s))
	}
	return out
}

func TestTheSameNightDealsTheSameCard(t *testing.T) {
	t.Parallel()
	house := []string{"kaheera", "goreclaw", "atla"}
	players := append(shelfOf(1, "gyome", "arahbo"), shelfOf(2, "tivit")...)
	first := night.PlanScheduled("2026-09-06", house, players, caps(6, 2, 10))
	again := night.PlanScheduled("2026-09-06", house, players, caps(6, 2, 10))
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("the same night dealt two different cards:\n%v\n%v", first, again)
	}
	if len(first) != 6 {
		t.Fatalf("dealt %d bouts, want the cap of 6", len(first))
	}
}

func TestADifferentNightDealsADifferentCard(t *testing.T) {
	t.Parallel()
	house := []string{"kaheera", "goreclaw", "atla", "trostani"}
	players := append(shelfOf(1, "gyome", "arahbo"), shelfOf(2, "tivit")...)
	first := night.PlanScheduled("2026-09-06", house, players, caps(6, 2, 10))
	other := night.PlanScheduled("2026-09-07", house, players, caps(6, 2, 10))
	if reflect.DeepEqual(first, other) {
		t.Fatal("two different nights dealt the identical card")
	}
	// And the seeds moved with the key, not just the order.
	seeds := map[int64]bool{}
	for _, p := range first {
		seeds[p.Seed] = true
	}
	fresh := 0
	for _, p := range other {
		if !seeds[p.Seed] {
			fresh++
		}
	}
	if fresh == 0 {
		t.Fatal("a different night re-dealt every seed of the first")
	}
}

func TestNoTwoBoutsOfANightShareASeed(t *testing.T) {
	t.Parallel()
	// The degenerate deal a shared seed would hide in: the same two house
	// decks fighting again and again.
	plans := night.PlanScheduled("2026-09-06", []string{"kaheera", "goreclaw"},
		nil, caps(6, 2, 7))
	seen := map[int64]int{}
	for i, p := range plans {
		if p.Seed < 0 {
			t.Fatalf("bout %d dealt a negative seed %d", i, p.Seed)
		}
		if p.Games != 7 {
			t.Fatalf("bout %d plays %d games, want the settings' 7", i, p.Games)
		}
		if prior, ok := seen[p.Seed]; ok {
			t.Fatalf("bouts %d and %d share seed %d", prior, i, p.Seed)
		}
		seen[p.Seed] = i
	}
}

func TestAnEagerShelfIsHeldToItsShare(t *testing.T) {
	t.Parallel()
	// One account opts in a wall of decks; another opts in one. The wall gets
	// its share, never the night (ADR 46 decision 5).
	wall := []night.Seat{}
	for i := 0; i < 40; i++ {
		wall = append(wall, owned(7, fmt.Sprintf("deck-%02d", i)))
	}
	players := append(wall, owned(9, "tivit"))
	plans := night.PlanScheduled("2026-09-06", []string{"kaheera", "atla"},
		players, caps(6, 2, 10))
	if len(plans) != 6 {
		t.Fatalf("dealt %d bouts, want 6", len(plans))
	}
	got := appearances(plans)
	if got[7] > 2 {
		t.Errorf("the wall sat in %d bouts, cap is 2", got[7])
	}
	if got[9] != 2 {
		t.Errorf("the one-deck account sat in %d bouts, want its full share of 2", got[9])
	}
}

func TestTheDrawIsFairAcrossAccounts(t *testing.T) {
	t.Parallel()
	players := append(append(shelfOf(1, "gyome", "arahbo"),
		shelfOf(2, "tivit", "hylda")...), shelfOf(3, "goreclaw-2")...)
	plans := night.PlanScheduled("2026-09-06", []string{"kaheera"}, players,
		caps(6, 2, 10))
	got := appearances(plans)
	for _, owner := range []int64{1, 2, 3} {
		if got[owner] != 2 {
			t.Errorf("account %d sat in %d bouts, want everyone's 2", owner, got[owner])
		}
	}
	// And on a scheduled night nobody is seated against themselves.
	for i, p := range plans {
		if p.SeatA.Owner != nil && p.SeatB.Owner != nil &&
			*p.SeatA.Owner == *p.SeatB.Owner {
			t.Errorf("bout %d seats account %d against itself", i, *p.SeatA.Owner)
		}
	}
}

func TestTheHouseFillsTheCard(t *testing.T) {
	t.Parallel()
	// One player, one deck, a share of three: the house completes their
	// bouts and the leftover capacity, with two different decks in every
	// house-only bout.
	plans := night.PlanScheduled("2026-09-06",
		[]string{"kaheera", "goreclaw", "atla"}, shelfOf(4, "gyome"),
		caps(4, 3, 10))
	if len(plans) != 4 {
		t.Fatalf("dealt %d bouts, want 4", len(plans))
	}
	got := appearances(plans)
	if got[4] != 3 {
		t.Errorf("the lone player sat in %d bouts, want their whole share of 3", got[4])
	}
	if got[0] != 5 {
		t.Errorf("the house filled %d seats, want the remaining 5", got[0])
	}
	for i, p := range plans {
		if p.SeatA.House() && p.SeatB.House() && p.SeatA.Slug == p.SeatB.Slug {
			t.Errorf("bout %d seats the house deck %q against itself", i, p.SeatA.Slug)
		}
	}
}

func TestAnEmptyLibraryDealsNothing(t *testing.T) {
	t.Parallel()
	if plans := night.PlanScheduled("2026-09-06", nil, nil, caps(6, 2, 10)); len(plans) != 0 {
		t.Fatalf("an empty roster dealt %d bouts", len(plans))
	}
	if plans := night.PlanSample("2026-09-06", nil, nil, 10); len(plans) != 0 {
		t.Fatalf("an empty sample dealt %d bouts", len(plans))
	}
	// A house of one deck has nobody to fight either.
	if plans := night.PlanScheduled("2026-09-06", []string{"kaheera"}, nil,
		caps(6, 2, 10)); len(plans) != 0 {
		t.Fatalf("a house of one dealt %d bouts against itself", len(plans))
	}
}

func TestASampleDealsTheFullRoundRobin(t *testing.T) {
	t.Parallel()
	house := []string{"kaheera", "goreclaw", "atla"}
	players := append(shelfOf(1, "gyome", "arahbo"), shelfOf(2, "tivit")...)
	plans := night.PlanSample("2026-09-06", house, players, 10)
	// Six decks, every pair once, caps nowhere in sight: 15 bouts.
	if len(plans) != 15 {
		t.Fatalf("the sample dealt %d bouts, want the full 15", len(plans))
	}
	key := func(s night.Seat) string {
		if s.Owner == nil {
			return "house/" + s.Slug
		}
		return fmt.Sprintf("%d/%s", *s.Owner, s.Slug)
	}
	seen := map[string]bool{}
	for _, p := range plans {
		a, b := key(p.SeatA), key(p.SeatB)
		if a > b {
			a, b = b, a
		}
		pair := a + " vs " + b
		if seen[pair] {
			t.Errorf("the sample dealt %s twice", pair)
		}
		seen[pair] = true
		if p.Games != 10 {
			t.Errorf("%s plays %d games, want 10", pair, p.Games)
		}
	}
	// Deterministic like the scheduled deal.
	if again := night.PlanSample("2026-09-06", house, players, 10); !reflect.DeepEqual(plans, again) {
		t.Fatal("the same sample dealt two different cards")
	}
}

func TestPlayerDecksReadsTheStandingConsent(t *testing.T) {
	t.Parallel()
	s, db := scratchAndDB(t)
	ctx := context.Background()
	mustExec(t, db, `INSERT INTO users (id, username, created_at) VALUES
		(1, 'alice', '2026-09-01T00:00:00+00:00'),
		(2, 'bob', '2026-09-01T00:00:00+00:00')`)
	mustExec(t, db, `INSERT INTO user_decks
		(owner_id, slug, name, yaml, coliseum_at_night, created_at, updated_at) VALUES
		(1, 'gyome', 'Gyome', 'name: G', 1, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00'),
		(1, 'arahbo', 'Arahbo', 'name: A', 0, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00'),
		(2, 'tivit', 'Tivit', 'name: T', 1, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00'),
		(2, 'crypted', 'Crypted', 'name: C', 1, '2026-09-01T00:00:00+00:00', '2026-09-01T00:00:00+00:00')`)
	mustExec(t, db, `UPDATE user_decks SET deleted_at = '2026-09-02T00:00:00+00:00'
		WHERE slug = 'crypted'`)

	decks, err := s.PlayerDecks(ctx)
	if err != nil {
		t.Fatalf("PlayerDecks: %v", err)
	}
	want := []night.Seat{owned(1, "gyome"), owned(2, "tivit")}
	if len(decks) != len(want) {
		t.Fatalf("mustered %v, want %v", decks, want)
	}
	for i := range want {
		if *decks[i].Owner != *want[i].Owner || decks[i].Slug != want[i].Slug {
			t.Errorf("seat %d is %d/%s, want %d/%s", i, *decks[i].Owner,
				decks[i].Slug, *want[i].Owner, want[i].Slug)
		}
	}
}
