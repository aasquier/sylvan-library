package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/night"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The night's remaining faults: a deck the instance cannot read at all, a bout
// whose core failed, a registry that will not take the plan, and a close that
// half-landed.
//
// `nightfaults_test.go` covers the instance that is *missing* something — no
// registry, no arena, no database. These are the faults where something is
// there and stops working partway, which is the volume detaching and the shape
// that costs a night rather than a bout.

// nightOverHandle is an instance whose arena is configured (so the bout gets
// past the gate) and whose `app.db` is the handle given.
func nightOverHandle(t *testing.T, db *sql.DB) *API {
	t.Helper()
	return New(Config{Logger: quietLogger(),
		Jobs: jobs.New(jobs.Config{Logger: quietLogger()}),
		// A worker URL is all `forgeStatus` needs to call the arena available;
		// nothing here gets as far as asking it to play.
		Forge:    tier3.Settings{WorkerURL: "https://worker.invalid"},
		DecksDir: decksDir(t), AppDB: db, AppWriteDB: db,
		AdminEmail: "alice@example.com"})
}

// **A deck the instance cannot read is a failure, not a skip.**
//
// A seat whose deck has *left* the library is a skip — the deck chose to go, and
// the night retires the card. A seat whose deck cannot be read because the
// volume stopped answering is the machine's fault, and recording it against the
// pairing would quietly retire a deck that did nothing wrong (ADR 46).
func TestABoutWhoseDeckCannotBeReadFailsRatherThanSkipping(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file:"+appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a := nightOverHandle(t, db)

	owner := int64(2)
	id, err := a.playNightBout(t.Context(), night.Bout{ID: 7, Games: 1,
		Seats: []night.Seat{{Owner: &owner, Slug: "bobs-public"}, {Slug: "kaheera"}}})
	if err == nil {
		t.Fatal("a bout over a database that will not answer reported success")
	}
	if id != 0 {
		t.Errorf("it claims match %d", id)
	}
	var skip night.Skip
	if errors.As(err, &skip) {
		t.Errorf("a database that stopped answering was recorded against the "+
			"pairing: %q", skip.Reason)
	}
}

// **A bout whose core failed fails the bout, and the waiter hears it.**
//
// The core runs inside a job so the Coliseum's own machinery carries it, and the
// one thing the night needs back is the error — a bout settled `done` because
// the waiter saw the job finish would record a match that never happened.
func TestABoutWhoseCoreFailedIsHeardByTheWaiter(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, reg, _, _ := nightAPI(t, shim)
	a.playCore = func(jobs.Progress, forgeMatch) (forgeResult, int64, error) {
		return forgeResult{}, 0, errors.New("the arena would not answer")
	}

	id, err := a.playNightBout(t.Context(), night.Bout{ID: 8, Games: 1, Seed: 9,
		Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}})
	if err == nil {
		t.Fatal("a bout whose core failed was reported as played")
	}
	if id != 0 {
		t.Errorf("a failed bout claims match %d", id)
	}
	// The core's own words reach the waiter rather than a generic failure: the
	// night writes this into the row, and a reason that said nothing would
	// leave the morning with a failed bout and no diagnosis.
	if !strings.Contains(err.Error(), "the arena would not answer") {
		t.Errorf("the waiter heard %q rather than the core's own reason", err)
	}
	reg.Wait()
}

// A registry that cannot take the plan is a failure with a sentence rather than
// a nil result nobody looks at.
//
// The fixture is a registry that was never built — no lanes in its table, which
// is the one thing `Submit` has an error for. It stands for the state the arm
// exists to answer: the bout could not be queued, and the night must hear that
// rather than settle a bout it never ran.
func TestABoutTheRegistryWillNotTakeFails(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, _, _, _ := nightAPI(t, shim)
	a.jobs = &jobs.Registry{}

	id, err := a.playNightBout(t.Context(), night.Bout{ID: 9, Games: 1, Seed: 10,
		Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}})
	if err == nil {
		t.Fatal("a bout no registry would take was reported as played")
	}
	if id != 0 {
		t.Errorf("an unqueued bout claims match %d", id)
	}
}

// ---- the admin rooms over a night that half answers -----------------------

// faultyNight is an instance whose night store answers a fixed number of
// statements: one open run, two planned bouts, and one of them carrying a match
// id so the watching read has a bout to name a match for.
func faultyNight(t *testing.T) (*API, *authtest.Fault) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, fault, err := authtest.OpenFaulty(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fault.Heal(); _ = db.Close() })

	store := night.FromDB(db, nil)
	ctx := context.Background()
	run, err := store.StartRun(ctx, "2026-10-03", false, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PlanBouts(ctx, run.ID, []night.Plan{
		{Games: 3, Clock: 300, Seed: 7,
			Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}},
		{Games: 3, Clock: 300, Seed: 8,
			Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "gyome"}}},
	}); err != nil {
		t.Fatal(err)
	}
	// A bout that already has a match behind it. Written by hand rather than
	// played: the watching read's job is to name the match, and claiming and
	// settling a bout to get there would be testing the store's lifecycle
	// instead. Raw SQL against a scratch database is this package's normal
	// instrument for a seeded row.
	if _, err := db.ExecContext(ctx,
		`UPDATE night_bouts SET state = 'done', match_id = 42, reason = ''`+
			` WHERE seed = 7`); err != nil {
		t.Fatal(err)
	}

	a := New(Config{Logger: quietLogger(), AppDB: db, AppWriteDB: db,
		AppDBPath: path, DecksDir: decksDir(t), AdminEmail: "alice@example.com"})
	a.SetNightRunner(night.NewRunner(night.RunnerConfig{Store: store,
		Settings: night.Settings{Bouts: 2, BoutsPerAccount: 1, Games: 3},
		Log:      a.log}))
	return a, fault
}

// The watching read names the match a finished bout produced.
func TestTheWatchingReadNamesTheMatchABoutProduced(t *testing.T) {
	t.Parallel()
	a, _ := faultyNight(t)
	status, _, raw := callAs(t, a, alice, "GET", "/api/admin/night", "")
	if status != http.StatusOK {
		t.Fatalf("the night read answered %d: %s", status, raw)
	}
	if !strings.Contains(string(raw), `"match_id":42`) {
		t.Errorf("the finished bout's match is not on the card: %s", raw)
	}
}

// **Once the night is over, the watching read falls back to the last one.**
//
// The room is read in the morning as well as at midnight, and a night that has
// finished has no *open* run — so the read asks for the most recent instead. A
// 404 here would say "no night has run yet" about a night that ran all night,
// which is the morning's whole question answered backwards.
func TestTheWatchingReadFallsBackToTheNightThatJustFinished(t *testing.T) {
	t.Parallel()
	a, _ := faultyNight(t)
	store := a.nightRunner.Store()
	run, ok, err := store.OpenRun(t.Context())
	if err != nil || !ok {
		t.Fatalf("the fixture has no open run: %v", err)
	}
	if err := store.FinishRun(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := callAs(t, a, alice, "GET", "/api/admin/night", "")
	if status != http.StatusOK {
		t.Fatalf("the read of a finished night answered %d: %s", status, raw)
	}
	finished, _ := payload["run"].(map[string]any)
	if finished == nil || finished["finished_at"] == nil {
		t.Errorf("the finished night came back without the hour it ended: %s", raw)
	}
}

// **Closing the night early over a handle that stops answering mid-request.**
//
// Two statements decide this route — the close itself, and the read-back that
// tells the page when the night now ends — and a failure at either must refuse.
// The answer that would be wrong is a 200 carrying a `closes_at` nobody wrote,
// because the page would then show an hour that is not the hour.
func TestClosingTheNightEarlyRefusesAHandleThatStopsAnswering(t *testing.T) {
	t.Parallel()
	refused, allowed, settled := 0, 0, 0
	for budget := 0; budget <= 24 && settled < 2; budget++ {
		a, fault := faultyNight(t)
		fault.After(budget)
		status, payload, raw := callAs(t, a, alice, "POST",
			"/api/admin/night/close", `{}`)
		fault.Heal()

		what := fmt.Sprintf("closing the night at budget %d", budget)
		answersHonestly(t, what, status, raw)
		switch status {
		case http.StatusOK:
			allowed++
			settled++
			if payload["closes_at"] == nil {
				t.Errorf("%s answered 200 with no hour on it: %s", what, raw)
			}
		case http.StatusNotFound:
			t.Errorf("%s answered 404 -- \"no night is open\" is a different "+
				"sentence from \"the rows cannot be read\": %s", what, raw)
		default:
			refused++
			settled = 0
		}
	}
	if refused == 0 {
		t.Error("closing the night succeeded at every budget, so the sweep " +
			"never reached a failure")
	}
	if allowed == 0 {
		t.Error("closing the night was refused at every budget, so the sweep " +
			"is measuring a broken fixture rather than the route")
	}
}

// The morning read, swept: once the night has finished there is no open run, so
// the read asks for the most recent — and **that** question failing must refuse
// rather than answer "no night has run yet", which is the one sentence the
// morning would act on and the one that would be false.
func TestTheMorningReadRefusesWhenTheLastNightCannotBeFound(t *testing.T) {
	t.Parallel()
	refused, allowed, settled := 0, 0, 0
	for budget := 0; budget <= 24 && settled < 2; budget++ {
		a, fault := faultyNight(t)
		store := a.nightRunner.Store()
		run, ok, err := store.OpenRun(t.Context())
		if err != nil || !ok {
			t.Fatalf("the fixture has no open run: %v", err)
		}
		if err := store.FinishRun(t.Context(), run.ID); err != nil {
			t.Fatal(err)
		}

		fault.After(budget)
		status, _, raw := callAs(t, a, alice, "GET", "/api/admin/night", "")
		fault.Heal()

		what := fmt.Sprintf("the morning read at budget %d", budget)
		answersHonestly(t, what, status, raw)
		switch status {
		case http.StatusOK:
			allowed++
			settled++
		case http.StatusNotFound:
			t.Errorf("%s answered \"no night has run yet\" about a night that "+
				"ran: %s", what, raw)
		default:
			refused++
			settled = 0
		}
	}
	if refused == 0 {
		t.Error("the morning read succeeded at every budget, so the sweep " +
			"never reached a failure")
	}
	if allowed == 0 {
		t.Error("the morning read was refused at every budget, so the sweep " +
			"is measuring a broken fixture rather than the route")
	}
}

// The watching read, swept the same way: a night whose rows half answer must
// never render as an hour with nothing in it.
func TestTheWatchingReadRefusesAHandleThatStopsAnswering(t *testing.T) {
	t.Parallel()
	a, fault := faultyNight(t)
	refused, allowed, settled := 0, 0, 0
	for budget := 0; budget <= 24 && settled < 2; budget++ {
		fault.After(budget)
		status, payload, raw := callAs(t, a, alice, "GET", "/api/admin/night", "")
		fault.Heal()

		what := fmt.Sprintf("the night read at budget %d", budget)
		answersHonestly(t, what, status, raw)
		switch status {
		case http.StatusOK:
			allowed++
			settled++
			bouts, _ := payload["bouts"].([]any)
			if len(bouts) == 0 {
				t.Errorf("%s answered 200 with an empty card over a night that "+
					"has two bouts in it: %s", what, raw)
			}
		case http.StatusNotFound:
			t.Errorf("%s answered 404 over a night that has run: %s", what, raw)
		default:
			refused++
			settled = 0
		}
	}
	if refused == 0 {
		t.Error("the night read succeeded at every budget, so the sweep never " +
			"reached a failure")
	}
	if allowed == 0 {
		t.Error("the night read was refused at every budget, so the sweep is " +
			"measuring a broken fixture rather than the route")
	}
}
