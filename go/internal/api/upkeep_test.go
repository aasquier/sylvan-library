package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// Upkeep: ADR 6's admin refresh endpoint, and the reading beside it.
//
// **No test here downloads anything.** The refresh's own sequence is proven in
// `internal/pool` against a stub index; what is proven here is the part this
// package owns — that the work is a *job* and not a request, that a second
// press cannot start a second gathering, that the latch comes back when the
// gathering fails, and that whatever broke, the words that reach a person name
// nothing that computes.

// upkeepRig is an instance with a library, a registry, and somewhere to fail.
type upkeepRig struct {
	api      *API
	poolPath string
}

// newUpkeepRig builds one. `broken` points the pool at a path that can never
// be opened for writing — a file where a directory has to be — which is how a
// refresh is made to fail **without a clock**: `OpenWriter` makes any missing
// directory it is given, so an absent path is not a failure at all, and a
// blocked one fails on its first syscall every time.
func newUpkeepRig(t *testing.T, broken bool) *upkeepRig {
	t.Helper()
	dir := t.TempDir()
	built := pooltest.Build(t)
	p := pool.New(built, nil)
	t.Cleanup(p.Close)

	poolPath := built
	if broken {
		blocked := filepath.Join(dir, "blocked")
		if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		poolPath = filepath.Join(blocked, "pool.duckdb")
	}
	a := New(Config{
		Pool:        p,
		PoolPath:    poolPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		DecksDir:    t.TempDir(),
		Jobs:        jobs.New(jobs.Config{}),
	})
	return &upkeepRig{api: a, poolPath: poolPath}
}

func (r *upkeepRig) post(t *testing.T, scope auth.Scope) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
	r.api.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), scope)))
	return rec
}

func (r *upkeepRig) get(t *testing.T, scope auth.Scope) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/upkeep", nil)
	r.api.upkeep(rec, req.WithContext(auth.WithScope(req.Context(), scope)))
	if rec.Code != http.StatusOK {
		t.Fatalf("the upkeep reading answered %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the upkeep reading is not JSON: %v", err)
	}
	return body
}

// The two new routes are on the table, under the prefix the door refuses to
// anybody who is not an admin before routing (ADR 17). Derived from
// `Routes()` rather than written down, so a route moved out from under the
// prefix is caught here rather than in production.
func TestTheUpkeepRoutesLiveUnderTheAdminPrefix(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"/api/admin/upkeep":          http.MethodGet,
		"/api/admin/library/refresh": http.MethodPost,
	}
	for _, route := range New(Config{}).Routes() {
		method, wanted := want[route.Pattern]
		if !wanted {
			continue
		}
		if route.Method != method {
			t.Errorf("%s is served as %s, want %s", route.Pattern, route.Method, method)
		}
		if !strings.HasPrefix(route.Pattern, "/api/admin/") {
			t.Errorf("%s is not under the admin prefix", route.Pattern)
		}
		delete(want, route.Pattern)
	}
	for pattern := range want {
		t.Errorf("%s is not served at all", pattern)
	}
}

// The second check, on the handlers themselves — the one that would catch an
// admin route mounted somewhere the prefix does not cover.
func TestTheUpkeepRoutesRefuseANonAdminThemselves(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, false)

	rec := rig.post(t, plainScope)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a gathering asked for by a non-admin answered %d", rec.Code)
	}
	if _, running := rig.api.gathering.running(); running {
		t.Error("a refused request took the latch anyway")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/upkeep", nil)
	rig.api.upkeep(rec, req.WithContext(auth.WithScope(req.Context(), plainScope)))
	if rec.Code != http.StatusForbidden {
		t.Errorf("the upkeep reading answered a non-admin %d", rec.Code)
	}
}

// **The gathering is a job that reached a worker**, and this is asserted on
// something that distinguishes it from a job born finished: a job that
// short-circuited could not have produced an error, and this rig's refresh
// cannot succeed. `status == "done"` would not have told the two apart.
func TestAGatheringIsWorkThatRuns(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, true)

	rec := rig.post(t, adminScope)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("starting a gathering answered %d: %s", rec.Code, rec.Body.String())
	}
	var started map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatal("the response carries no job to poll")
	}
	if kind, _ := started["kind"].(string); kind != LibraryRefreshKind {
		t.Errorf("the job's kind is %q", kind)
	}

	rig.api.jobs.Wait()

	job := rig.api.jobs.Get(id, adminScope.UserID)
	if job == nil {
		t.Fatal("the job vanished from the registry")
	}
	payload := job.Payload()
	if payload.Status != jobs.Errored {
		t.Fatalf("a gathering with nowhere to write finished as %q", payload.Status)
	}
	if payload.Error == nil {
		t.Fatal("a failed gathering recorded no reason")
	}
	// The worker ran: a job born finished never reaches one, and a
	// short-circuit has no error to report.
	if *payload.Error == "" {
		t.Error("the failed gathering's reason is empty")
	}
}

// One gathering at a time, whoever asks. The latch is per process because
// that is the scope the write lock has — the registry's own dedupe key is per
// owner, which two admins would walk straight past.
func TestASecondGatheringIsRefusedWhileOneIsUnderWay(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, false)

	// The latch taken directly rather than by starting a real refresh: what is
	// under test is the route's answer to a busy instance, and a real refresh
	// would go to Scryfall.
	if !rig.api.gathering.claim() {
		t.Fatal("a fresh instance would not take the latch")
	}
	rig.api.gathering.name("abc123")

	rec := rig.post(t, adminScope)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a second gathering answered %d, want a refusal", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got, _ := body["job_id"].(string); got != "abc123" {
		t.Errorf("the refusal points at %v, not at the gathering already going", body["job_id"])
	}
	// Nothing was queued by the refusal: a 409 that had also started a job
	// would be exactly the double-run this guard exists to prevent.
	if n := len(rig.api.jobs.All(adminScope.UserID)); n != 0 {
		t.Errorf("a refused second gathering left %d jobs in the registry", n)
	}

	// And the reading agrees with the refusal, so a page reloaded mid-gather
	// attaches instead of offering to start another.
	library, _ := rig.get(t, adminScope)["library"].(map[string]any)
	if running, _ := library["refreshing"].(bool); !running {
		t.Error("the reading says nothing is being gathered while one is")
	}
	if got, _ := library["job_id"].(string); got != "abc123" {
		t.Errorf("the reading points at %v", library["job_id"])
	}
}

// A failed gathering hands the latch back, or the button is dead until the
// process restarts. (A latch that outlives its reason is a recorded trap in
// this repository.)
func TestAFailedGatheringHandsTheLatchBack(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, true)

	if rec := rig.post(t, adminScope); rec.Code != http.StatusAccepted {
		t.Fatalf("the first gathering answered %d", rec.Code)
	}
	rig.api.jobs.Wait()

	if _, running := rig.api.gathering.running(); running {
		t.Fatal("the latch is still held after the gathering failed")
	}
	if rec := rig.post(t, adminScope); rec.Code != http.StatusAccepted {
		t.Fatalf("a second gathering after a failure answered %d", rec.Code)
	}
	rig.api.jobs.Wait()
}

// The library is given back afterwards: a refresh seals the pool so it can
// take the writer, and a seal that did not lift would leave every card lookup
// answering "no pool" until the process restarted.
func TestAGatheringGivesTheLibraryBackWhenItFails(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, true)

	if rec := rig.post(t, adminScope); rec.Code != http.StatusAccepted {
		t.Fatalf("the gathering answered %d", rec.Code)
	}
	rig.api.jobs.Wait()

	if rig.api.pool.Sealed() {
		t.Fatal("the library is still sealed after the gathering finished")
	}
	library, _ := rig.get(t, adminScope)["library"].(map[string]any)
	if present, _ := library["present"].(bool); !present {
		t.Error("the library did not come back after a failed gathering")
	}
}

// The reading is what the page renders before anybody presses anything.
func TestTheUpkeepReadingSaysWhatTheLibraryHolds(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, false)

	body := rig.get(t, adminScope)
	library, ok := body["library"].(map[string]any)
	if !ok {
		t.Fatalf("the reading has no library in it: %v", body)
	}
	if present, _ := library["present"].(bool); !present {
		t.Error("an instance with a pool reports no library")
	}
	cards, _ := library["cards"].(float64)
	if cards <= 0 {
		t.Errorf("the library holds %v cards", library["cards"])
	}
	if refreshing, _ := library["refreshing"].(bool); refreshing {
		t.Error("a quiet instance says it is gathering")
	}
	if library["job_id"] != nil {
		t.Errorf("a quiet instance points at job %v", library["job_id"])
	}
	if gathered, _ := library["gathered"].(string); len(gathered) != len("2026-08-24") {
		t.Errorf("the day the library was gathered reads %q", library["gathered"])
	}

	// The arena half is a reading and never a lever: no game has been played
	// on this instance, so it says nothing rather than guessing.
	arena, ok := body["arena"].(map[string]any)
	if !ok {
		t.Fatalf("the reading has no arena in it: %v", body)
	}
	if arena["playing_with"] != nil {
		t.Errorf("an instance that has played no games claims %v", arena["playing_with"])
	}
	if here, _ := arena["here"].(bool); here {
		t.Error("an instance with no arena configured says it has one")
	}
}

// The two refusals about how a deployment is put together, held to the same
// vocabulary rule as the failures: neither may name the thing that is
// missing. Swept over the real responses rather than over a copy of the
// strings, so a reworded refusal is checked rather than remembered.
func TestTheConfigurationRefusalsNameNothingEither(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"nothing to run work on", Config{PoolPath: "/somewhere/pool"}},
		{"nowhere to keep a library", Config{Jobs: jobs.New(jobs.Config{})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := New(tc.cfg)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
			a.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("answered %d: %s", rec.Code, rec.Body.String())
			}
			said := detail(t, rec)
			for _, leak := range append([]string{"registry", "pool", "job",
				"config", "MTGLAB_", "duckdb"}, neverSaid...) {
				if strings.Contains(said, leak) {
					t.Errorf("the refusal %q contains %q", said, leak)
				}
			}
		})
	}
}

// An instance with no registry cannot start work, and says so instead of
// taking a latch nothing will ever release.
func TestAnInstanceWithNoRegistryStartsNoGathering(t *testing.T) {
	t.Parallel()
	a := New(Config{PoolPath: filepath.Join(t.TempDir(), "pool.duckdb")})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
	a.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("an instance with no registry answered %d", rec.Code)
	}
	if _, running := a.gathering.running(); running {
		t.Error("a refusal took the latch")
	}
}

// An instance that was never told where its library lives has nowhere to put
// one, which is a different thing from a refresh that failed.
func TestAnInstanceWithNowhereToKeepALibraryRefuses(t *testing.T) {
	t.Parallel()
	a := New(Config{Jobs: jobs.New(jobs.Config{})})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
	a.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("an instance with no pool path answered %d", rec.Code)
	}
	if _, running := a.gathering.running(); running {
		t.Error("a refusal took the latch")
	}
}

// **Whatever broke, the room names nothing that computes.**
//
// The same rule and the same word list `forgeTrouble` is held to
// (`forgevoice_test.go`), because the fault it was written for was live on
// this site once already: an admin is still a user, and commandment 10 has no
// exception for the person who pays the bills.
func TestTheLibraryNeverRecitesTheMachine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"the app's own readers would not let go", pool.ErrStillReading},
		{"a rewrite was already going", pool.ErrSealed},
		{"another process holds the file", &pool.RefreshError{
			Phase: pool.PhaseShelves,
			Err: errors.New(`Could not set lock on file "/data/mtg.duckdb": ` +
				`Conflicting lock is held in /usr/local/bin/mtglab (PID 654)`)}},
		{"nowhere to write", &pool.RefreshError{
			Phase: pool.PhaseShelves,
			Err:   errors.New("pool: mkdir /data/blocked/mtg.duckdb: not a directory")}},
		{"the source answered nothing", &pool.RefreshError{
			Phase: pool.PhaseGather,
			Err:   errors.New("bulk download: HTTP 503 Service Unavailable")}},
		{"the index moved", &pool.RefreshError{
			Phase: pool.PhaseGather,
			Err: errors.New("bulk entry oracle_cards has no download URL; " +
				"Scryfall's index format may have changed again (keys: [id type])")}},
		{"a row would not go in", &pool.RefreshError{
			Phase: pool.PhaseShelve,
			Err:   errors.New("duckdb appender: INTERNAL Error: Attempted to flush")}},
		{"something nobody classified", errors.New("dial tcp 151.101.0.0:443: i/o timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			said := libraryTrouble(tc.err)
			if said == "" {
				t.Fatal("a failure with nothing to say leaves the room silent")
			}
			for _, leak := range neverSaid {
				if strings.Contains(said, leak) {
					t.Errorf("the room said %q, which contains %q (from %v)",
						said, leak, tc.err)
				}
			}
			for _, leak := range []string{"duckdb", "DuckDB", "Scryfall",
				"scryfall", "bulk", "URL", "PID", "mkdir", "/data", "mtglab"} {
				if strings.Contains(said, leak) {
					t.Errorf("the room said %q, which contains %q (from %v)",
						said, leak, tc.err)
				}
			}
			if len(said) < 40 {
				t.Errorf("the room said %q, which is not an explanation", said)
			}
		})
	}
	// Nothing to say about nothing, so the caller can test for silence.
	if said := libraryTrouble(nil); said != "" {
		t.Errorf("a refresh that did not fail said %q", said)
	}
}

// A gathering that failed reports the room's words and not the machine's —
// end to end, through the job's error field, which is what the browser
// renders.
func TestAFailedGatheringReportsTheRoomsWords(t *testing.T) {
	t.Parallel()
	rig := newUpkeepRig(t, true)

	rec := rig.post(t, adminScope)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the gathering answered %d", rec.Code)
	}
	var started map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	rig.api.jobs.Wait()

	job := rig.api.jobs.Get(started["id"].(string), adminScope.UserID)
	if job == nil {
		t.Fatal("the job vanished")
	}
	said := job.Payload().Error
	if said == nil {
		t.Fatal("the failed gathering recorded no reason")
	}
	for _, leak := range append([]string{"duckdb", "mkdir", "not a directory"}, neverSaid...) {
		if strings.Contains(*said, leak) {
			t.Errorf("the job's reason %q contains %q", *said, leak)
		}
	}
	if !strings.Contains(*said, "gathering") && !strings.Contains(*said, "library") {
		t.Errorf("the job's reason %q does not sound like this room", *said)
	}
}

// The progress a page renders while it waits: five beats, each with words,
// because five beats over several minutes is a bar that barely moves and the
// words are the only sign of life.
func TestTheGatheringSaysWhatItIsDoing(t *testing.T) {
	t.Parallel()
	if gatherSteps < 2 {
		t.Fatalf("a progress bar with %d beats is not a progress bar", gatherSteps)
	}
	raw, err := json.Marshal(gatherPartial("clearing the shelves"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); got != `{"saying":"clearing the shelves"}` {
		t.Errorf("the partial reads %s", got)
	}
	// Every line a person can be shown while they wait, held to the same rule
	// the failures are. The value under test is the sentence, not the
	// envelope it travels in.
	for _, saying := range GatherSayings {
		for _, leak := range neverSaid {
			if strings.Contains(saying, leak) {
				t.Errorf("the progress line %q contains %q", saying, leak)
			}
		}
	}
}
