package api

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// What the simulations do when the thing they rest on gives way: a stored
// answer that will not read back, a deck with nothing to sweep, a pool that
// opens and cannot answer, a lane the registry has never heard of.
//
// ADR 18's rule is that a cached number says it is cached. The branch under all
// of these is the other half of that promise: an answer the cache cannot
// *understand* must be recomputed rather than half-decoded, because a landRow
// decoded from `"not a row"` is a chart of zeroes that still renders.

// cacheRig is a sim API with a real `sim_cache` and a handle to write into it.
type cacheRig struct {
	api   *API
	jobs  *jobs.Registry
	write *sql.DB
	decks string
}

func newCacheRig(t *testing.T) *cacheRig {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := cache.Open(dbPath, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	write, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = write.Close() })
	decks := decksDir(t)
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks, Logger: quiet,
		AdminEmail: "alice@example.com", AppDB: db, Jobs: reg, SimCache: store})
	return &cacheRig{api: a, jobs: reg, write: write, decks: decks}
}

// spoil turns every stored answer into a payload none of the result structs can
// read -- the shape that arrives when a schema moves under a cache that was
// written by an older binary.
func (r *cacheRig) spoil(t *testing.T) {
	t.Helper()
	res, err := r.write.Exec(`UPDATE sim_cache SET result_json = '"not a result at all"'`)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Fatal("there was nothing cached to spoil -- the run before this stored nothing")
	}
}

// submitted runs a simulation and hands back the job's own payload.
func (r *cacheRig) submitted(t *testing.T, target, body string) map[string]any {
	t.Helper()
	status, payload, raw := callAs(t, r.api, alice, "POST", target, body)
	if status != http.StatusOK {
		t.Fatalf("%s answered %d: %s", target, status, raw)
	}
	return payload
}

// The mana run: a stored answer that will not decode is recomputed, and the
// answer that comes back says it was computed rather than replayed.
func TestAManaResultThatWillNotDecodeIsRecomputedRatherThanHalfRead(t *testing.T) {
	t.Parallel()
	rig := newCacheRig(t)
	const body = `{"slug":"kaheera","games":120,"turns":8,"seed":11}`

	first := rig.submitted(t, "/api/sim/mana", body)
	rig.jobs.Wait()
	if done := rig.jobs.Get(first["id"].(string), alice.UserID); done == nil ||
		done.Status() != jobs.Done {
		t.Fatalf("the first run ended %v", done)
	}
	// The replay, so the cache is known to be answering before it is spoiled.
	replay := rig.submitted(t, "/api/sim/mana", body)
	if replay["status"] != jobs.Done {
		t.Fatalf("the second ask was not a replay: %v", replay["status"])
	}

	rig.spoil(t)
	again := rig.submitted(t, "/api/sim/mana", body)
	if again["status"] == jobs.Done {
		t.Fatalf("a stored answer nobody can read was served as finished: %v", again)
	}
	rig.jobs.Wait()
	job := rig.jobs.Get(again["id"].(string), alice.UserID)
	if job == nil || job.Status() != jobs.Done {
		t.Fatalf("the recomputed run ended %v", job)
	}
	out := asMap(t, job.Result())
	if out["cached"] != false {
		t.Errorf("the recomputed answer reports cached=%v", out["cached"])
	}
}

// The land sweep, which caches one row per count and so has the same question
// twice over: the plan's own decode of every row up front, and the worker's
// re-read of each count as it reaches it. Spoiling the rows drives both, and the
// sweep still answers a full curve.
func TestASpoiledLandRowIsReSweptAtBothPlacesItIsRead(t *testing.T) {
	t.Parallel()
	rig := newCacheRig(t)
	const body = `{"slug":"kaheera","low":33,"high":34,"games":120,"turns":8,"seed":5}`

	first := rig.submitted(t, "/api/sim/lands", body)
	rig.jobs.Wait()
	if done := rig.jobs.Get(first["id"].(string), alice.UserID); done == nil ||
		done.Status() != jobs.Done {
		t.Fatalf("the first sweep ended %v", done)
	}
	replay := rig.submitted(t, "/api/sim/lands", body)
	if replay["status"] != jobs.Done {
		t.Fatalf("the second sweep was not a replay: %v", replay["status"])
	}

	rig.spoil(t)
	again := rig.submitted(t, "/api/sim/lands", body)
	if again["status"] == jobs.Done {
		t.Fatalf("spoiled rows were served as a finished curve: %v", again)
	}
	rig.jobs.Wait()
	job := rig.jobs.Get(again["id"].(string), alice.UserID)
	if job == nil || job.Status() != jobs.Done {
		t.Fatalf("the re-swept run ended %v", job)
	}
	out := asMap(t, job.Result())
	if out["cached"] != false {
		t.Errorf("the re-swept curve reports cached=%v", out["cached"])
	}
	rows, _ := out["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("the re-swept curve has %d rows, want 33 and 34", len(rows))
	}
}

// A deck with no lands at all cannot be swept, and the refusal reaches the
// caller the same way a missing deck does: the sweep's whole measure is the land
// count, and cycling nothing is not an answer at zero.
func TestASweepOfADeckWithNoLandsFailsTheJobRatherThanTheRequest(t *testing.T) {
	t.Parallel()
	rig := newCacheRig(t)

	dir := filepath.Join(rig.decks, "landless")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(
		"slug: landless\nname: Landless\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
			"  - name: Sol Ring\n    category: ramp\n    why: it ramps\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := rig.submitted(t, "/api/sim/lands",
		`{"slug":"landless","low":33,"high":34,"games":120,"turns":8,"seed":5}`)
	rig.jobs.Wait()
	job := rig.jobs.Get(payload["id"].(string), alice.UserID)
	if job == nil {
		t.Fatal("the registry lost the sweep")
	}
	if job.Status() != jobs.Errored {
		t.Fatalf("a landless sweep ended %q", job.Status())
	}
	if job.Payload().Error == nil {
		t.Error("the failed sweep carries no sentence")
	}
}

// Every simulation compiles its deck against the pool first, and a pool that
// opens and then cannot answer a query has to fail the job rather than simulate
// a deck it never read. All four are asked, because each one wraps the same
// compile step in its own plan.
func TestEverySimulationFailsRatherThanRunOverAPoolThatCannotAnswer(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Pool: schemalessPool(t), DecksDir: decksDir(t), Logger: quiet,
		AdminEmail: "alice@example.com", AppDB: db, Jobs: reg})

	for _, target := range []string{"/api/sim/mana", "/api/sim/lands", "/api/sim/policy"} {
		status, payload, raw := callAs(t, a, alice, "POST", target,
			`{"slug":"kaheera","games":120,"turns":8,"seed":3}`)
		if status != http.StatusOK {
			t.Errorf("%s answered %d rather than a job that fails: %s", target, status, raw)
			continue
		}
		id, _ := payload["id"].(string)
		if id == "" {
			t.Errorf("%s answered with no job: %s", target, raw)
			continue
		}
		reg.Wait()
		job := reg.Get(id, alice.UserID)
		if job == nil || job.Status() != jobs.Errored {
			t.Errorf("%s ended %v over a pool that cannot answer", target, job)
			continue
		}
		if job.Payload().Error == nil {
			t.Errorf("%s failed with no sentence", target)
		}
	}

	// The shelf is the one that answers in the response rather than through a
	// job, so the same fault has to arrive as a refusal with words in it.
	status, payload, raw := callAs(t, a, alice, "POST", "/api/sim/shelf",
		`{"slug":"kaheera","games":120}`)
	if status == http.StatusOK {
		t.Fatalf("the shelf answered 200 over a pool that cannot answer: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the shelf's refusal carries nothing a person could read: %s", raw)
	}
}

// A plan aimed at a lane the registry does not run is a refusal rather than a
// job nobody will ever pick up. No route builds one -- the lanes are constants
// -- which is why it is asked of the function that turns a plan into an answer.
func TestAPlanForALaneNobodyRunsIsRefusedRatherThanLost(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Jobs: reg, Logger: quiet})

	rec := httptest.NewRecorder()
	a.submit(rec, httptest.NewRequest(http.MethodPost, "/api/sim/mana", nil), jobs.Plan{
		Kind: "sim.mana", Label: "a plan with nowhere to run", Lane: "no-such-lane",
		Run: func(jobs.Progress) (any, error) { return nil, nil },
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("a plan with no lane answered %d: %s", rec.Code, rec.Body)
	}
	if rec.Body.Len() == 0 {
		t.Error("it answered with no body")
	}

	// And a plan with a lane that is run still becomes a job.
	rec = httptest.NewRecorder()
	a.submit(rec, httptest.NewRequest(http.MethodPost, "/api/sim/mana", nil), jobs.Plan{
		Kind: "sim.mana", Label: "a plan that runs", Lane: jobs.CPU,
		Run: func(jobs.Progress) (any, error) { return 1, nil },
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("a plan with a real lane answered %d: %s", rec.Code, rec.Body)
	}
	reg.Wait()
}

// A deck that compiles to no cards is the one state the opening hand refuses,
// and it refuses it as a question about the deck -- 422 with the simulator's own
// sentence -- rather than as the no-pool answer, which is a different sentence
// that sends a person somewhere else entirely.
//
// The fixture is a shape a real library produces: a commander the pool knows and
// a 99 it does not, which is a deck imported against a pool that has since been
// rebuilt. The pool answers -- the commander resolved -- so this is not the
// no-pool arm, and there is still nothing to shuffle.
func TestDealingFromADeckThatCompilesToNothingIsARefusalAboutTheDeck(t *testing.T) {
	t.Parallel()
	rig := newCacheRig(t)

	dir := filepath.Join(rig.decks, "all-strangers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(
		"slug: all-strangers\nname: All Strangers\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
			"  - name: Nothing This Pool Has Heard Of\n    category: ramp\n    why: a stranger\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/all-strangers/opening-hand", `{}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("dealing from a deck that compiles to nothing answered %d: %s", status, raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries nothing a person could read: %s", raw)
	}
}
