package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/night"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The night's route-layer half, driven the way the runner drives it: the
// real BoutPlayer against the stub shim, and the three admin routes against
// the same fixtures every other admin test uses. What matters most here is
// ADR 46 decision 7 — the night and the interactive match drive one core and
// one ledger call site — so the first test plays one of each and counts rows.

// nightAPI is [forgeAPIIn] plus the night: a store over the same app.db, a
// runner around this API's own player, and the knot tied the way the door
// ties it.
func nightAPI(t *testing.T, shim *stubShim) (*API, *jobs.Registry, *night.Runner, string) {
	t.Helper()
	a, reg, dbPath, dir := forgeAPIIn(t, shim)
	store, err := night.NewStore(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runner := night.NewRunner(night.RunnerConfig{Store: store,
		Settings: night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 3},
		Player:   a.NightPlayer(),
		LaneBusy: func() bool { return reg.LaneBusy(jobs.FORGE) },
		House: func(ctx context.Context) ([]string, error) {
			return library.NewFileSource(dir, false).Slugs(ctx)
		},
		Log: a.log})
	t.Cleanup(runner.Stop)
	a.SetNightRunner(runner)
	return a, reg, runner, dbPath
}

func countMatches(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forge_matches`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestANightBoutAndAnInteractiveMatchDriveTheOneCore(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, games: []tier3.WireGame{
		won(1, 5421, 1, 11), won(2, 4000, 2, 9), won(3, 6800, 1, 12)}}
	a, reg, _, dbPath := nightAPI(t, shim)

	// The night's half: one bout, played through the player seam.
	matchID, err := a.playNightBout(context.Background(), night.Bout{
		ID: 7, SeatA: night.Seat{Slug: "kaheera"},
		SeatB: night.Seat{Slug: "mono-green"}, Games: 3, Seed: 41})
	if err != nil {
		t.Fatalf("the bout failed: %v", err)
	}
	if matchID <= 0 {
		t.Fatalf("the bout came back with match id %d; the core did not record", matchID)
	}
	if got := countMatches(t, dbPath); got != 1 {
		t.Fatalf("the ledger holds %d matches after the bout, want 1", got)
	}

	// The interactive half, over the same API and the same stub.
	srv := forgeServer(t, a)
	status, payload := postForge(t, srv,
		`{"a_slug":"kaheera","b_slug":"mono-green","games":3}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	reg.Wait()
	if got := countMatches(t, dbPath); got != 2 {
		t.Fatalf("the ledger holds %d matches after both paths, want 2", got)
	}

	// The night's job belongs to the house and to nobody's listing (ADR 5):
	// alice, who just watched her own interactive match, sees exactly that
	// one job and never the bout's.
	houseJobs := reg.All(jobs.HouseOwner)
	if len(houseJobs) != 1 || houseJobs[0].Kind != NightForgeKind {
		t.Fatalf("the house holds %d jobs, want the one bout", len(houseJobs))
	}
	for _, job := range reg.All(alice.UserID) {
		if job.Kind == NightForgeKind {
			t.Fatal("a night bout appeared in a person's job listing")
		}
	}
}

func TestAPanickingCoreStillSettlesTheBout(t *testing.T) {
	t.Parallel()
	// The registry recovers a panicking job, but that recovery unwinds past
	// the night closure — if the settle rode after the core's return alone, a
	// panic would skip it and the waiter would park until shutdown, wedging
	// the whole night on one bad bout. The waiter must hear the panic as a
	// failure instead, and the registry must still contain it as one errored
	// job. The panic is injected through the core's seam because every real
	// panic here is by definition a path nobody predicted.
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, reg, _, _ := nightAPI(t, shim)
	a.playCore = func(jobs.Progress, forgeMatch) (forgeResult, int64, error) {
		panic("the board fell over")
	}
	type answer struct {
		matchID int64
		err     error
	}
	got := make(chan answer, 1)
	go func() {
		id, err := a.playNightBout(context.Background(), night.Bout{
			ID: 11, SeatA: night.Seat{Slug: "kaheera"},
			SeatB: night.Seat{Slug: "mono-green"}, Games: 1, Seed: 45})
		got <- answer{id, err}
	}()
	select {
	case o := <-got:
		if o.err == nil || !strings.Contains(o.err.Error(), "the board fell over") {
			t.Fatalf("the waiter heard %v, want the panic's own words as a failure", o.err)
		}
		if o.matchID != 0 {
			t.Errorf("a panicked bout claims match %d", o.matchID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter never heard the settle; a panicking core wedges the night")
	}
	// And the containment still held: the job errored, the process lived.
	reg.Wait()
	house := reg.All(jobs.HouseOwner)
	if len(house) != 1 || house[0].Status() != jobs.Errored {
		t.Fatalf("the house's jobs read %+v, want the one errored bout", house)
	}
}

func TestACoverageFailureSkipsTheBoutWithTheCounts(t *testing.T) {
	t.Parallel()
	shim := &stubShim{coverage: []tier3.WireReport{{Slug: "kaheera",
		Checked: 100, Missing: []string{"Kaheera, the Orphanguard"}}}}
	a, _, _, _ := nightAPI(t, shim)
	_, err := a.playNightBout(context.Background(), night.Bout{
		ID: 8, SeatA: night.Seat{Slug: "kaheera"},
		SeatB: night.Seat{Slug: "mono-green"}, Games: 3, Seed: 42})
	var skip night.Skip
	if !errors.As(err, &skip) {
		t.Fatalf("a failed pre-flight answered %v, want a skip", err)
	}
	if !strings.Contains(skip.Reason, "Forge does not implement every card") {
		t.Errorf("the skip's reason is %q, not the pre-flight's own sentence", skip.Reason)
	}
}

func TestAMissingSeatAndAnEmptyDeckAreSkipsNotFailures(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, _, _, _ := nightAPI(t, shim)

	// A house deck that has left the library since the card was dealt.
	_, err := a.playNightBout(context.Background(), night.Bout{
		ID: 9, SeatA: night.Seat{Slug: "no-such-deck"},
		SeatB: night.Seat{Slug: "mono-green"}, Games: 3, Seed: 43})
	var skip night.Skip
	if !errors.As(err, &skip) || !strings.Contains(skip.Reason, "has left the library") {
		t.Fatalf("a vanished seat answered %v, want a skip naming it", err)
	}

	// A player's deck with nothing in it — bob's private shell from the
	// fixture db, read through his own tier because rung 13's flag is the
	// standing consent to exactly this read.
	owner := int64(2)
	_, err = a.playNightBout(context.Background(), night.Bout{
		ID: 10, SeatA: night.Seat{Owner: &owner, Slug: "bobs-private"},
		SeatB: night.Seat{Slug: "mono-green"}, Games: 3, Seed: 44})
	if !errors.As(err, &skip) || !strings.Contains(skip.Reason, "has no cards in it") {
		t.Fatalf("an empty deck answered %v, want a skip saying so", err)
	}
}

func TestTheNightRoutesAnswerTheAdminAndOnlyTheAdmin(t *testing.T) {
	t.Parallel()
	a, _, runner, _ := nightAPI(t, nil)

	// The second check behind the door's prefix rule: a signed-in non-admin
	// is refused by the handlers themselves.
	for _, ask := range [][2]string{
		{"GET", "/api/admin/night"},
		{"POST", "/api/admin/night/sample"},
		{"POST", "/api/admin/night/close"},
	} {
		if status, _, _ := callAs(t, a, bob, ask[0], ask[1], `{"minutes":5}`); status != 403 {
			t.Errorf("%s %s answered bob %d, want 403", ask[0], ask[1], status)
		}
	}

	// Before any night: the watching read and the close both say so.
	if status, body, _ := as(t, a, alice, "/api/admin/night"); status != 404 {
		t.Fatalf("GET with no night answered %d %v", status, body)
	}
	if status, _, _ := callAs(t, a, alice, "POST", "/api/admin/night/close", `{}`); status != 404 {
		t.Errorf("close with no night answered %d, want 404", status)
	}

	// The sample's one dial is bounded and required.
	for _, bad := range []string{`{}`, `{"minutes":0}`, `{"minutes":500}`, `{"minutes":"an hour"}`} {
		if status, _, _ := callAs(t, a, alice, "POST", "/api/admin/night/sample", bad); status != 422 {
			t.Errorf("sample %s answered %d, want 422", bad, status)
		}
	}

	// A real sample opens: the full round-robin over the fixture library.
	status, body, raw := callAs(t, a, alice, "POST", "/api/admin/night/sample", `{"minutes":60}`)
	if status != 201 {
		t.Fatalf("sample answered %d %s", status, raw)
	}
	if body["run_id"] == nil || body["closes_at"] == nil {
		t.Fatalf("the sample's answer is missing its fields: %v", body)
	}
	// Nine fixture decks, every pair once.
	if got := body["bouts"].(float64); got != 36 {
		t.Errorf("the sample dealt %v bouts, want the fixture library's 36", got)
	}

	// One night at a time, in words, as a conflict.
	if status, _, _ := callAs(t, a, alice, "POST", "/api/admin/night/sample", `{"minutes":5}`); status != 409 {
		t.Errorf("a second sample answered %d, want 409", status)
	}

	// The watching read: the open run, its card, the tally.
	status, watch, raw := as(t, a, alice, "/api/admin/night")
	if status != 200 {
		t.Fatalf("GET answered %d %s", status, raw)
	}
	run := watch["run"].(map[string]any)
	if run["sample"] != true || run["finished_at"] != nil {
		t.Errorf("the open run reads %v", run)
	}
	if tally := watch["tally"].(map[string]any); tally["planned"].(float64) != 36 {
		t.Errorf("the tally reads %v", tally)
	}
	if bouts := watch["bouts"].([]any); len(bouts) != 36 {
		t.Errorf("the card lists %d bouts", len(bouts))
	} else {
		first := bouts[0].(map[string]any)
		seat := first["seat_a"].(map[string]any)
		if seat["owner"] != nil || seat["slug"] == "" {
			t.Errorf("the first seat reads %v", seat)
		}
	}

	// The close pulls the deadline up; the nudged tick would settle the
	// remainder, and here the tick is driven by hand so the test owns time.
	if status, _, _ := callAs(t, a, alice, "POST", "/api/admin/night/close", `{}`); status != 200 {
		t.Fatalf("close answered %d", status)
	}
	runner.Tick(context.Background())
	status, after, _ := as(t, a, alice, "/api/admin/night")
	if status != 200 {
		t.Fatalf("GET after the close answered %d", status)
	}
	closedRun := after["run"].(map[string]any)
	if closedRun["finished_at"] == nil {
		t.Errorf("the closed night never finished: %v", closedRun)
	}
	if tally := after["tally"].(map[string]any); tally["skipped"].(float64) != 36 {
		t.Errorf("the closed night's tally reads %v", tally)
	}
	// And with the night over, there is nothing left to close.
	if status, _, _ := callAs(t, a, alice, "POST", "/api/admin/night/close", `{}`); status != 404 {
		t.Errorf("closing a finished night answered %d, want 404", status)
	}
}

func TestWithNoStoreTheNightRoutesSaySo(t *testing.T) {
	t.Parallel()
	// An API nobody handed a runner — the no-app.db instance. 503 in words,
	// never a nil dereference.
	a, done := deckAPI(t, noCredential, false)
	defer done()
	for _, ask := range [][2]string{
		{"GET", "/api/admin/night"},
		{"POST", "/api/admin/night/sample"},
		{"POST", "/api/admin/night/close"},
	} {
		if status, _, _ := callAs(t, a, alice, ask[0], ask[1], `{"minutes":5}`); status != 503 {
			t.Errorf("%s %s answered %d with no store, want 503", ask[0], ask[1], status)
		}
	}
}
