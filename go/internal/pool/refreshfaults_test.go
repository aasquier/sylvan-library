package pool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// A refresh that breaks, and **which of its three phases it broke in**.
//
// The phase is the whole reason these are worth a test rather than an
// inspection. The admin page turns it into a sentence a player reads -- the
// library was busy, the source could not be reached, a row would not go in --
// and a failure that came back unclassified reaches them as the wrong sentence
// about the wrong thing, or as a database's own lock error, which commandment
// 10 forbids outright. The download's faults are held next door; these are the
// two either side of it: taking the shelves, and putting rows on them.

// aShelfServing is a bulk index over bodies exactly as given, so a test can
// serve a file that is not a bulk file.
func aShelfServing(t *testing.T, bodies map[string][]byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		data := []any{}
		for _, kind := range []string{pool.OracleBulk, pool.PrintingsBulk} {
			data = append(data, map[string]any{
				"type": kind, "updated_at": "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": srv.URL + "/files/" + kind + ".jsonl",
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		kind := filepath.Base(r.URL.Path)
		kind = kind[:len(kind)-len(".jsonl")]
		body, ok := bodies[kind]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// oneGoodCard and aFileCutInHalf are the two bodies every test here serves.
const (
	oneGoodCard    = `{"oracle_id":"o1","name":"Fixture Chef","layout":"normal"}` + "\n"
	aFileCutInHalf = `{"oracle_id":"o1","name":"Fixture Chef","layout":"normal"}` + "\n" +
		`{"oracle_id":"o2","name":"Fixture Squ`
)

// An oracle file that stops making sense is a **shelving** failure, not a
// gathering one: the bytes arrived, and it is the putting-away that broke.
// The distinction is the difference between "the source could not be reached"
// and "a row would not go in", and only one of those is worth retrying.
func TestAnOracleFileCutInHalfIsAShelvingFailure(t *testing.T) {
	t.Parallel()
	shelf := aShelfServing(t, map[string][]byte{pool.OracleBulk: []byte(aFileCutInHalf)})
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")

	// An oracle-only run writes in place, which is what makes the pool below
	// an honest witness to the rollback.
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		OracleOnly:  true,
		IndexURL:    shelf.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatalf("a bulk file cut in half was shelved: %+v", counts)
	}
	if got := pool.PhaseOf(err); got != pool.PhaseShelve {
		t.Errorf("the failure is classified %q, want %q -- the page says a "+
			"different sentence for each", got, pool.PhaseShelve)
	}
	if counts.Oracle != 0 {
		t.Errorf("a failed refresh reported %d rows shelved", counts.Oracle)
	}
	if got := countRows(t, dbPath, "oracle_cards"); got != 0 {
		t.Errorf("the pool holds %d rows from a load that rolled back", got)
	}
}

// The same fault on the printings half, which is the one that goes through the
// rebuild: the oracle rows are already in the new file, and the run has to
// throw the whole file away rather than publish half a library.
func TestAPrintingsFileCutInHalfLeavesTheServedPoolAlone(t *testing.T) {
	t.Parallel()
	shelf := aShelfServing(t, map[string][]byte{
		pool.OracleBulk:    []byte(oneGoodCard),
		pool.PrintingsBulk: []byte(aFileCutInHalf),
	})
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")

	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    shelf.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatalf("a printings file cut in half was shelved: %+v", counts)
	}
	if got := pool.PhaseOf(err); got != pool.PhaseShelve {
		t.Errorf("the failure is classified %q, want %q", got, pool.PhaseShelve)
	}
	// The oracle half loaded, and it loaded into the file that was discarded.
	if got := countRows(t, dbPath, "oracle_cards"); got != 0 {
		t.Errorf("the served pool holds %d oracle rows from a refresh that "+
			"never finished", got)
	}
	assertNoLeavings(t, dbPath)
}

// A build path that will not clear stops the run **at the shelves**, before a
// byte is downloaded. It is the same phase as a locked pool for the same
// reason: from where a caller stands, the refresh could not take the shelves
// it needs, and no amount of retrying the network changes that.
func TestABlockedBuildPathStopsTheRunAtTheShelves(t *testing.T) {
	t.Parallel()
	shelf := aShelfServing(t, map[string][]byte{
		pool.OracleBulk:    []byte(oneGoodCard),
		pool.PrintingsBulk: []byte(oneGoodCard),
	})
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	// A directory with something in it where the half-built file goes: the
	// shape a hand-rolled backup leaves behind.
	if err := os.MkdirAll(filepath.Join(dbPath+".rebuilding", "in the way"), 0o750); err != nil {
		t.Fatal(err)
	}

	scryfall := filepath.Join(dir, "scryfall")
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: scryfall,
		IndexURL:    shelf.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatalf("a blocked build path was built over: %+v", counts)
	}
	if got := pool.PhaseOf(err); got != pool.PhaseShelves {
		t.Errorf("the failure is classified %q, want %q", got, pool.PhaseShelves)
	}
	// Nothing was downloaded: the shelves are taken before the source is asked,
	// which is what keeps a broken volume from costing 500MB of somebody's
	// bandwidth every time an operator retries.
	if entries, err := os.ReadDir(scryfall); err == nil && len(entries) > 0 {
		t.Errorf("a refresh that could not take the shelves downloaded %d files",
			len(entries))
	}
}

// **A refresh against a library somebody is reading.** The door is shut, the
// operator is told once that it is being waited for, and the refusal at the
// end is classified as the shelves rather than reaching a page as a lock
// error. This is the one that was reported twice as a broken database.
func TestARefreshAgainstABusyLibrarySaysSoAndCallsItTheShelves(t *testing.T) {
	t.Parallel()
	dbPath := heldByAnotherProcess(t)
	dir := t.TempDir()

	said := 0
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		Wait:        500 * time.Millisecond,
		IndexURL:    "http://example.invalid/bulk-data",
	}, pool.RefreshWatcher{Waiting: func() { said++ }})
	if err == nil {
		t.Fatalf("a refresh took a pool another process is holding: %+v", counts)
	}
	if said != 1 {
		t.Errorf("the operator was told %d times that the door was shut, want "+
			"once -- silence and a stream both read as hung", said)
	}
	if got := pool.PhaseOf(err); got != pool.PhaseShelves {
		t.Errorf("a locked pool is classified %q, want %q; the page turns that "+
			"into the sentence about the library being busy", got, pool.PhaseShelves)
	}
}
