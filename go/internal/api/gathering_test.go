package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	matchledger "github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The refresh button's own job body, run the whole way through.
//
// `upkeep_test.go` proves everything around [API.gatherTheLibrary] -- that the
// work is a job, that a second press is refused, that a failure hands the
// latch back and says so in the room's words. What it could never prove is the
// gathering *succeeding*, because the success path calls `pool.Refresh`
// against Scryfall: five hundred megabytes and several minutes of somebody
// else's bandwidth, which is not a thing a test may do (ADR 6's own
// constraint, and CLAUDE.md rule 9 besides). So the whole middle of this
// feature -- the seal, the five progress beats in order, the counts the page
// is handed, the library actually changing on disk -- had never run anywhere
// but production.
//
// [Config.BulkIndex] is what closes that, and it is the field `pool.Refresh`
// already takes rather than a seam invented for the test: the index URL is
// handed down from the composition root exactly as [Config.SetsFeed] is, and
// its zero value is still Scryfall's own. The stub below serves the two bulk
// files the sequence asks for.

// bulkStub is a stand-in for the bulk index and the two files behind it.
//
// The cards are invented on purpose (`pooltest.Card`'s rule): what this
// fixture is for is the *shape* of a refresh -- an index, two downloads, two
// loads, two counts -- and naming a row after a real card would turn a
// fixture into a claim about Magic that nobody looked up (CLAUDE.md rule 1).
type bulkStub struct {
	*httptest.Server
	asked map[string]int
}

func newBulkStub(t *testing.T, oracle, printings int) *bulkStub {
	t.Helper()
	s := &bulkStub{asked: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{"type": pool.OracleBulk,
				"updated_at":         "2026-09-24T09:00:00.000+00:00",
				"jsonl_download_uri": s.URL + "/files/" + pool.OracleBulk + ".jsonl"},
			map[string]any{"type": pool.PrintingsBulk,
				"updated_at":         "2026-09-24T09:00:00.000+00:00",
				"jsonl_download_uri": s.URL + "/files/" + pool.PrintingsBulk + ".jsonl"},
		}})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		kind := filepath.Base(r.URL.Path)
		kind = kind[:len(kind)-len(".jsonl")]
		s.asked[kind]++
		enc := json.NewEncoder(w)
		switch kind {
		case pool.OracleBulk:
			for i := 0; i < oracle; i++ {
				_ = enc.Encode(map[string]any{
					"oracle_id": fixtureID(i), "name": fixtureName(i),
					"mana_cost": "{1}{G}", "cmc": 2, "type_line": "Creature — Fixture",
					"oracle_text": "A shape, not a card.", "layout": "normal",
					"color_identity": []string{"G"}, "set": "fix",
				})
			}
		case pool.PrintingsBulk:
			for i := 0; i < printings; i++ {
				_ = enc.Encode(map[string]any{
					"id": fixtureID(1000 + i), "oracle_id": fixtureID(i),
					"name": fixtureName(i), "set": "fix", "set_name": "Fixtures",
					"collector_number": "1", "rarity": "common",
					"released_at": "2026-09-01", "digital": false,
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func fixtureID(i int) string {
	return "00000000-0000-4000-8000-" + string([]byte{
		'0' + byte(i/100000%10), '0' + byte(i/10000%10), '0' + byte(i/1000%10),
		'0' + byte(i/100%10), '0' + byte(i/10%10), '0' + byte(i%10),
		'0', '0', '0', '0', '0', '0'})
}

func fixtureName(i int) string {
	return "Fixture Shape " + string(rune('A'+i%26))
}

// gatheringRig is an instance whose refresh reaches the stub rather than
// Scryfall: no pool open over the file yet, so the seal is a no-op and the
// sequence is the one a fresh volume runs.
type gatheringRig struct {
	api      *API
	bulk     *bulkStub
	poolPath string
}

func newGatheringRig(t *testing.T, oracle, printings int) *gatheringRig {
	t.Helper()
	dir := t.TempDir()
	bulk := newBulkStub(t, oracle, printings)
	poolPath := filepath.Join(dir, "pool.duckdb")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := New(Config{
		Logger:      quiet,
		PoolPath:    poolPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		DecksDir:    t.TempDir(),
		BulkIndex:   bulk.URL + "/bulk-data",
		Jobs:        jobs.New(jobs.Config{Logger: quiet}),
	})
	return &gatheringRig{api: a, bulk: bulk, poolPath: poolPath}
}

// An instance nobody told where the bulk index lives asks Scryfall, which is
// the whole reason the field could be added without changing a deployment.
func TestAnInstanceToldNothingGathersFromScryfall(t *testing.T) {
	t.Parallel()
	if got := New(Config{}).bulkIndex; got != pool.BulkIndex {
		t.Fatalf("an unconfigured instance gathers from %q", got)
	}
}

// The gathering, end to end: five beats in the order a person reads them, a
// library on disk that was not there before, and the two counts the page
// renders.
func TestAGatheringShelvesTheCardsAndSaysSoInOrder(t *testing.T) {
	t.Parallel()
	rig := newGatheringRig(t, 4, 6)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
	rig.api.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("the gathering answered %d: %s", rec.Code, rec.Body)
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
	payload := job.Payload()
	if payload.Error != nil {
		t.Fatalf("the gathering failed: %s", *payload.Error)
	}
	if payload.Status != string(jobs.Done) {
		t.Fatalf("the gathering finished %q", payload.Status)
	}

	// The answer the page is handed: what the library now holds, in the two
	// counts and nothing else. The sweep's file count is deliberately absent
	// -- it is the operator's business, and it goes to the log.
	result, ok := payload.Result.(wire.OrderedMap)
	if !ok {
		t.Fatalf("the gathering answered %T, not the room's shape", payload.Result)
	}
	got := map[string]any{}
	for _, kv := range result {
		got[kv.Key] = kv.Value
	}
	if len(result) != 2 {
		t.Errorf("the gathering's answer carries %d keys: %v", len(result), got)
	}
	if got["cards"] != int64(4) {
		t.Errorf("the gathering shelved %v cards, want the stub's 4", got["cards"])
	}
	if got["printings"] != int64(6) {
		t.Errorf("the gathering shelved %v printings, want the stub's 6", got["printings"])
	}

	// Both halves were actually fetched -- a sequence that shelved one and
	// reported two would answer exactly the same way above.
	if rig.bulk.asked[pool.OracleBulk] != 1 || rig.bulk.asked[pool.PrintingsBulk] != 1 {
		t.Errorf("the bulk files were asked for %v", rig.bulk.asked)
	}

	// And a library exists where there was none.
	if _, err := os.Stat(rig.poolPath); err != nil {
		t.Fatalf("the gathering reported success and shelved nothing: %v", err)
	}

	// The latch is back, so the button works again.
	if _, held := rig.api.gathering.running(); held {
		t.Error("a finished gathering kept the latch")
	}
}

// The beats, as the page receives them. What is asserted is the *order* --
// clear, gather, shelve, gather, shelve -- because the words are the only
// thing that moves over several minutes, and a bar that said "shelving the
// printings" before anything was gathered would be worse than no bar at all.
func TestTheGatheringReportsItsBeatsInTheOrderTheyHappen(t *testing.T) {
	t.Parallel()
	rig := newGatheringRig(t, 2, 2)

	said := &sayings{}
	_, err := rig.api.gatherTheLibrary(said)
	if err != nil {
		t.Fatalf("the gathering failed: %v", err)
	}
	want := []string{sayClearing, sayGatheringCards, sayShelvingCards,
		sayGatheringPrints, sayShelvingPrints}
	if len(said.beats) != len(want) {
		t.Fatalf("the gathering said %d beats, want %d: %v",
			len(said.beats), len(want), said.beats)
	}
	for i, w := range want {
		if said.beats[i] != w {
			t.Errorf("beat %d said %q, want %q (the whole run: %v)",
				i+1, said.beats[i], w, said.beats)
		}
	}
	// The bar counts up as well as speaking: a beat that never advanced the
	// step would leave the words moving over a frozen bar.
	for i, done := range said.done {
		if done != i {
			t.Errorf("beat %d reported step %d of %d", i+1, done, said.total[i])
		}
	}
}

// sayings is a [jobs.Progress] that keeps the saying off each partial, in the
// order they arrive.
type sayings struct {
	beats []string
	done  []int
	total []int
}

func (s *sayings) Report(done, total int) { s.ReportPartial(done, total, nil) }

func (s *sayings) ReportPartial(done, total int, partial any) {
	row, ok := partial.(wire.OrderedMap)
	if !ok || len(row) == 0 {
		return
	}
	saying, _ := row[0].Value.(string)
	s.beats = append(s.beats, saying)
	s.done = append(s.done, done)
	s.total = append(s.total, total)
}

// The arena's reading: the Forge the last recorded match was played with.
//
// From the ledger rather than from the worker, so what this asserts is that
// the *most recent* row wins and that a row with no version recorded is
// skipped rather than answered as a blank -- a blank would render as a
// version of "", which reads as an arena playing with nothing.
func TestTheUpkeepReadingNamesTheForgeTheLastMatchWasPlayedWith(t *testing.T) {
	t.Parallel()
	dbPath := appDB(t)
	db, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	recordMatch(t, db, "2.0.98")
	recordMatch(t, db, "2.0.99")

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := New(Config{Logger: quiet, MatchLedger: matchledger.FromDB(db, quiet)})
	if got := a.arenaVersion(t.Context()); got != "2.0.99" {
		t.Errorf("the arena plays with %q, want the newest recorded version", got)
	}

	// A newest match that recorded no version is not an arena playing with
	// nothing: the reading falls through to absent.
	recordMatch(t, db, "")
	if got := a.arenaVersion(t.Context()); got != "" {
		t.Errorf("a match with no version recorded read as %q", got)
	}

	// And a ledger that cannot be read says nothing rather than claiming a
	// version -- the warning goes to the log, which is where it is useful.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := a.arenaVersion(t.Context()); got != "" {
		t.Errorf("a ledger that would not answer claimed %q", got)
	}
}

// recordMatch writes one finished match with the given Forge version, or with
// none when it is empty. Straight SQL rather than through the recorder,
// because what is under test here is the *reading*.
func recordMatch(t *testing.T, db *sql.DB, version string) {
	t.Helper()
	var stored any
	if version != "" {
		stored = version
	}
	if _, err := db.Exec(
		`INSERT INTO forge_matches (created_at, seed, clock, games_requested,`+
			` forge_version, hosted, wall_seconds) VALUES (?, NULL, 300, 1, ?, 0, 1.0)`,
		time.Now().UTC().Format(time.RFC3339Nano), stored); err != nil {
		t.Fatal(err)
	}
}

// With no ledger at all -- an instance with no app.db -- the reading answers
// null rather than an empty string, so a client's `?? fallback` reaches its
// fallback.
func TestWithNoLedgerTheArenaVersionIsAbsentRatherThanBlank(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if got := a.arenaVersion(t.Context()); got != "" {
		t.Fatalf("an instance with no ledger claims to play with %q", got)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/upkeep", nil)
	a.upkeep(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	arena, _ := body["arena"].(map[string]any)
	if got, present := arena["playing_with"]; !present || got != nil {
		t.Errorf("the arena reads %v, want a null a client can fall back from", got)
	}
}
