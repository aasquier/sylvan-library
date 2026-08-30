package pool_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The download shelf's housekeeping, from both ends: what a sweep will take,
// and when a refresh is allowed to ask for one.
//
// The bug was arithmetic rather than subtle -- nothing ever deleted a dated
// bulk file, so `/data/scryfall` grew by half a gigabyte a refresh and reached
// seven files and 317MB on the deployed volume. What is worth testing is not
// that a delete deletes; it is the two rules around it. **A failed refresh
// sweeps nothing**, because the previous copy is the only local rollback for
// rows that are still on the shelves. And **the sweep deletes by shape**, so
// the `.part` of a download in flight, a hand-decompressed file, a directory
// wearing a plausible name and anything belonging to somebody else all
// survive a directory that is not exclusively ours.

// seedShelf parks a file of the given size on the shelf. The sizes differ per
// file so the byte count is an assertion rather than a coincidence.
func seedShelf(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name),
		bytes.Repeat([]byte("x"), size), 0o600); err != nil {
		t.Fatal(err)
	}
}

// onTheShelf is every name in dir, sorted, so a test can assert the survivors
// as a set rather than one os.Stat at a time.
func onTheShelf(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// sweepFixtureCards is the frozen tiny pool, as the raw card objects a bulk
// file holds.
func sweepFixtureCards(t *testing.T) (oracle, printings []map[string]any) {
	t.Helper()
	fx := loadRefresh(t)
	for _, c := range fx.Oracle {
		oracle = append(oracle, c.Raw)
	}
	for _, p := range fx.Printings {
		printings = append(printings, p.Raw)
	}
	return oracle, printings
}

// The whole shape rule in one directory: two kinds, several dates, the copy
// each kind's refresh just used, and six things that must still be there
// afterwards.
func TestASweepTakesTheOlderDatedCopiesAndSparesEverythingElse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// The leavings: older dates, and an older *suffix* on the same date,
	// because Scryfall has changed formats before and the copy left behind is
	// no more use for that.
	seedShelf(t, dir, "oracle_cards-2026-08-20.jsonl.gz", 100)
	seedShelf(t, dir, "oracle_cards-2026-08-22.jsonl", 200)
	seedShelf(t, dir, "oracle_cards-2026-08-24.json", 400)
	seedShelf(t, dir, "default_cards-2026-08-20.jsonl.gz", 800)

	// The two the refresh just read.
	seedShelf(t, dir, "oracle_cards-2026-08-24.jsonl.gz", 1600)
	seedShelf(t, dir, "default_cards-2026-08-24.jsonl.gz", 3200)

	// Everything else on a shared volume directory.
	seedShelf(t, dir, "oracle_cards-2026-08-25.jsonl.gz.part", 16) // in flight
	seedShelf(t, dir, "oracle_cards.jsonl", 16)                    // no date
	seedShelf(t, dir, "oracle_cards-2026-8-4.jsonl", 16)           // not the date shape
	seedShelf(t, dir, "oracle-cards-2026-08-20.jsonl", 16)         // not the kind
	seedShelf(t, dir, "rulings-2026-08-20.json", 16)               // a kind we never fetch
	seedShelf(t, dir, "notes.txt", 16)
	// A directory wearing a name that would otherwise match exactly.
	if err := os.MkdirAll(filepath.Join(dir, "default_cards-2026-08-19.jsonl"), 0o750); err != nil {
		t.Fatal(err)
	}

	swept, err := pool.SweepBulk(dir, map[string]string{
		pool.OracleBulk:    filepath.Join(dir, "oracle_cards-2026-08-24.jsonl.gz"),
		pool.PrintingsBulk: filepath.Join(dir, "default_cards-2026-08-24.jsonl.gz"),
	})
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if swept.Files != 4 {
		t.Errorf("the sweep took %d files, want the 4 older dated copies", swept.Files)
	}
	if want := int64(100 + 200 + 400 + 800); swept.Bytes != want {
		t.Errorf("the sweep reported %d bytes freed, want %d", swept.Bytes, want)
	}

	want := []string{
		"default_cards-2026-08-19.jsonl", // the directory
		"default_cards-2026-08-24.jsonl.gz",
		"notes.txt",
		"oracle-cards-2026-08-20.jsonl",
		"oracle_cards-2026-08-24.jsonl.gz",
		"oracle_cards-2026-08-25.jsonl.gz.part",
		"oracle_cards-2026-8-4.jsonl",
		"oracle_cards.jsonl",
		"rulings-2026-08-20.json",
	}
	if got := onTheShelf(t, dir); !slices.Equal(got, want) {
		t.Errorf("the shelf holds\n  %v\nwant\n  %v", got, want)
	}
}

// **An oracle-only run has no opinion about the printings.** It never looked
// at that half, so the newest printings copy is still the only local record of
// rows sitting in the pool right now, and taking it would throw away a
// rollback for work this run did not do.
func TestASweepOfOneKindLeavesTheOtherKindsCopiesAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedShelf(t, dir, "oracle_cards-2026-08-20.jsonl.gz", 100)
	seedShelf(t, dir, "oracle_cards-2026-08-24.jsonl.gz", 200)
	seedShelf(t, dir, "default_cards-2026-08-01.jsonl.gz", 400)
	seedShelf(t, dir, "default_cards-2026-08-20.jsonl.gz", 800)

	swept, err := pool.SweepBulk(dir, map[string]string{
		pool.OracleBulk: filepath.Join(dir, "oracle_cards-2026-08-24.jsonl.gz"),
	})
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if swept.Files != 1 || swept.Bytes != 100 {
		t.Errorf("the sweep took %+v, want the one older oracle copy", swept)
	}
	want := []string{
		"default_cards-2026-08-01.jsonl.gz",
		"default_cards-2026-08-20.jsonl.gz",
		"oracle_cards-2026-08-24.jsonl.gz",
	}
	if got := onTheShelf(t, dir); !slices.Equal(got, want) {
		t.Errorf("the shelf holds\n  %v\nwant\n  %v", got, want)
	}
}

// A kind this code does not download sweeps nothing at all, so a caller who
// names one by mistake deletes nothing rather than reaching for
// `<anything>-<date>.json` in a directory that happens to hold some. And a
// shelf that cannot be read says so, because nothing else on this path would.
func TestASweepWillNotTouchAKindThisCodeDoesNotDownload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedShelf(t, dir, "rulings-2026-08-20.json", 100)
	seedShelf(t, dir, "rulings-2026-08-24.json", 200)

	swept, err := pool.SweepBulk(dir, map[string]string{
		"rulings": filepath.Join(dir, "rulings-2026-08-24.json"),
	})
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if swept.Files != 0 || swept.Bytes != 0 {
		t.Errorf("a kind this code never downloads swept %+v", swept)
	}
	if got := onTheShelf(t, dir); len(got) != 2 {
		t.Errorf("the shelf holds %v, want both rulings files untouched", got)
	}

	// An empty request is a no-op rather than a sweep of everything.
	if swept, err = pool.SweepBulk(dir, nil); err != nil || swept.Files != 0 {
		t.Errorf("a sweep of no kinds answered %+v (%v)", swept, err)
	}

	// A shelf that is not there is the caller's error, not a silent zero --
	// and it is reported without a panic on the way.
	if _, err = pool.SweepBulk(filepath.Join(dir, "gone"),
		map[string]string{pool.OracleBulk: "oracle_cards-2026-08-24.jsonl"}); err == nil {
		t.Error("a sweep of a directory that is not there reported success")
	}
}

// aShelfThatFallsOverOnThePrintings serves the oracle bulk file honestly and
// then answers the printings download with a 500.
//
// That is the failure worth building a stub for: the oracle half is *loaded*
// when the run breaks, so an implementation that swept after each kind rather
// than after the whole run would already have deleted the older oracle copies
// by then -- and the rollback for the rows now in the pool with it.
func aShelfThatFallsOverOnThePrintings(t *testing.T, oracle []map[string]any) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{"type": pool.OracleBulk, "updated_at": bulkUpdatedAt,
				"jsonl_download_uri": srv.URL + "/files/" + pool.OracleBulk + ".jsonl"},
			map[string]any{"type": pool.PrintingsBulk, "updated_at": bulkUpdatedAt,
				"jsonl_download_uri": srv.URL + "/files/" + pool.PrintingsBulk + ".jsonl"},
		}})
	})
	mux.HandleFunc("/files/"+pool.OracleBulk+".jsonl", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jsonl(t, oracle))
	})
	mux.HandleFunc("/files/"+pool.PrintingsBulk+".jsonl", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// **A refresh that broke sweeps nothing**, even the half that succeeded.
func TestARefreshThatBrokeHalfwaySweepsNothing(t *testing.T) {
	t.Parallel()
	oracle, _ := sweepFixtureCards(t)
	scryfall := aShelfThatFallsOverOnThePrintings(t, oracle)

	dir := t.TempDir()
	shelf := filepath.Join(dir, "scryfall")
	seedShelf(t, shelf, "oracle_cards-2026-08-20.jsonl", 100)
	seedShelf(t, shelf, "default_cards-2026-08-20.jsonl", 200)

	var shelved []string
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: shelf,
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{
		Shelved: func(kind string, _ int64) { shelved = append(shelved, kind) },
	})
	if err == nil {
		t.Fatal("a refresh whose printings download 500'd reported success")
	}
	if got := pool.PhaseOf(err); got != pool.PhaseGather {
		t.Errorf("the failure is phase %q, want %q", got, pool.PhaseGather)
	}
	// Without this the test would pass against a refresh that never got as
	// far as the interesting moment.
	if !slices.Contains(shelved, pool.OracleBulk) {
		t.Fatalf("the oracle half never shelved (%v); this test measures nothing", shelved)
	}
	if counts.Swept.Files != 0 || counts.Swept.Bytes != 0 {
		t.Errorf("a failed refresh reported sweeping %+v", counts.Swept)
	}

	want := []string{
		"default_cards-2026-08-20.jsonl",
		"oracle_cards-2026-08-20.jsonl", // the rollback for the rows now in the pool
		"oracle_cards-2026-08-24.jsonl", // what this run downloaded before it broke
	}
	if got := onTheShelf(t, shelf); !slices.Equal(got, want) {
		t.Errorf("a failed refresh left\n  %v\nwant everything it found, plus its own download:\n  %v",
			got, want)
	}
}

// **A refresh whose downloads were all skipped still sweeps**, which is the
// property that makes an over-full shelf reclaimable: the operator runs one
// refresh, Scryfall's date has not moved, nothing is downloaded, and the older
// copies go anyway. How much the shelf is holding is not a fact about whether
// Scryfall published this morning.
func TestARefreshThatSkippedItsDownloadsStillSweepsTheShelf(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	shelf := filepath.Join(dir, "scryfall")
	if err := os.MkdirAll(shelf, 0o750); err != nil {
		t.Fatal(err)
	}
	// Today's copies, already there: the dated skip will read these rather
	// than ask for them.
	for name, body := range map[string][]byte{
		"oracle_cards-2026-08-24.jsonl":  jsonl(t, oracle),
		"default_cards-2026-08-24.jsonl": jsonl(t, printings),
	} {
		if err := os.WriteFile(filepath.Join(shelf, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seedShelf(t, shelf, "oracle_cards-2026-07-30.jsonl.gz", 1000)
	seedShelf(t, shelf, "default_cards-2026-07-30.jsonl.gz", 2000)

	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: shelf,
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if counts.Oracle == 0 || counts.Printings == 0 {
		t.Fatalf("the refresh loaded nothing from the copies it found: %+v", counts)
	}
	if scryfall.asked[pool.OracleBulk] != 0 || scryfall.asked[pool.PrintingsBulk] != 0 {
		t.Errorf("the refresh downloaded again (%v); this test measures a skipped download",
			scryfall.asked)
	}
	if counts.Swept.Files != 2 || counts.Swept.Bytes != 3000 {
		t.Errorf("the refresh swept %+v, want the two older copies and their 3,000 bytes",
			counts.Swept)
	}
	want := []string{"default_cards-2026-08-24.jsonl", "oracle_cards-2026-08-24.jsonl"}
	if got := onTheShelf(t, shelf); !slices.Equal(got, want) {
		t.Errorf("the shelf holds\n  %v\nwant only what the refresh read:\n  %v", got, want)
	}
}

// The oracle-only flag carries all the way through to the sweep: the small
// half tidies its own older copies and leaves the large half's alone.
func TestAnOracleOnlyRefreshSweepsOnlyTheOracleCopies(t *testing.T) {
	t.Parallel()
	oracle, _ := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, nil)

	dir := t.TempDir()
	shelf := filepath.Join(dir, "scryfall")
	seedShelf(t, shelf, "oracle_cards-2026-08-20.jsonl", 100)
	seedShelf(t, shelf, "default_cards-2026-08-20.jsonl", 200)

	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: shelf,
		IndexURL:    scryfall.URL + "/bulk-data",
		OracleOnly:  true,
	}, pool.RefreshWatcher{})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if counts.Swept.Files != 1 || counts.Swept.Bytes != 100 {
		t.Errorf("an oracle-only refresh swept %+v, want the one older oracle copy",
			counts.Swept)
	}
	want := []string{
		"default_cards-2026-08-20.jsonl", // untouched: this run never read it
		"oracle_cards-2026-08-24.jsonl",
	}
	if got := onTheShelf(t, shelf); !slices.Equal(got, want) {
		t.Errorf("the shelf holds\n  %v\nwant\n  %v", got, want)
	}
}
