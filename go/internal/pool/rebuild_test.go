package pool_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The rebuild path: a refresh fills a new file beside the pool and renames it
// into place, rather than emptying and refilling the served one.
//
// These tests are about the two things that change when you do that -- the
// space comes back, and a table nothing upstream can reconstruct must be
// carried over by hand -- plus the failure shapes, which are the reason the
// rebuild is worth having at all: the served pool is now untouched until the
// last instant.

// seedPriceHistory puts snapshots in a pool the way `mtglab data snapshot`
// would, so a later refresh can be asked whether it kept them.
func seedPriceHistory(t *testing.T, path string, rows ...string) {
	t.Helper()
	db, err := pool.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("opening %s to seed price history: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	for i, id := range rows {
		day := time.Date(2026, 9, i+1, 0, 0, 0, 0, time.UTC)
		if _, err := db.ExecContext(context.Background(),
			`INSERT INTO price_history VALUES (?, ?, 'o1', 'Sol Ring', 1.5, 9.0)`,
			day, id); err != nil {
			t.Fatalf("seeding price history: %v", err)
		}
	}
}

// countRows is one number out of a pool opened by path, for tests that care
// what survived rather than how it is stored.
func countRows(t *testing.T, path, table string) int {
	t.Helper()
	db, err := sql.Open("duckdb", path+"?access_mode=read_only")
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("counting %s in %s: %v", table, path, err)
	}
	return n
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("measuring %s: %v", path, err)
	}
	return info.Size()
}

// **The one a rebuild could silently destroy.** `price_history` is
// append-only and no bulk file can put it back, so a refresh that rebuilt
// without carrying it would throw away every snapshot ever taken -- and would
// look completely healthy doing it, because every other count would be right.
func TestARebuiltPoolKeepsThePriceHistoryNoBulkFileCouldReplace(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	seedPriceHistory(t, dbPath, "p1", "p2", "p3")

	if got := countRows(t, dbPath, "price_history"); got != 3 {
		t.Fatalf("the seed did not take (%d rows); this test measures nothing", got)
	}

	if _, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Asked of the seeded rows by name rather than of the total, because the
	// total is no longer only theirs: a refresh records the prices it loaded
	// as well (see [TestARefreshRecordsThePricesItLoaded]). Counting
	// everything made this read "kept 13 of 3" the moment that landed -- a
	// green-to-red on a test whose subject had not changed at all. The
	// question here was always "did the old days survive", so it asks that.
	if got := countRows(t, dbPath,
		"price_history WHERE printing_id IN ('p1','p2','p3')"); got != 3 {
		t.Errorf("the rebuild kept %d of 3 seeded price snapshots -- a refresh "+
			"has destroyed history nothing can reconstruct", got)
	}
	// And the rows it was supposed to replace really were replaced, so this
	// is not passing because the refresh quietly did nothing.
	if got := countRows(t, dbPath, "oracle_cards"); got != len(oracle) {
		t.Errorf("oracle_cards has %d rows, want %d", got, len(oracle))
	}
}

// **The space actually comes back.** Refresh the same rows repeatedly: the
// file settles at a size and stays there. Against the in-place reload this
// grew every run, which is the leak the whole change exists to close.
func TestRepeatedRefreshesDoNotGrowThePoolFile(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	shelf := filepath.Join(dir, "scryfall")

	var sizes []int64
	for range 3 {
		if _, err := pool.Refresh(context.Background(), pool.RefreshOptions{
			DBPath:      dbPath,
			ScryfallDir: shelf,
			IndexURL:    scryfall.URL + "/bulk-data",
		}, pool.RefreshWatcher{}); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		sizes = append(sizes, sizeOf(t, dbPath))
	}

	// The second and third runs load byte-identical rows into a fresh file,
	// so they must produce a byte-identical file. A growing tail here is the
	// old behaviour coming back.
	if sizes[2] != sizes[1] {
		t.Errorf("the pool file grew across identical refreshes: %v -- a "+
			"rebuild produces the same file for the same rows, so a rising "+
			"tail means rows are being written over the old ones again", sizes)
	}
}

// **A refresh that breaks does not touch the served pool.** The old path's
// promise was a rollback; this one never writes to the file at all until the
// rename, so the pool a reader sees is bit-for-bit the one it saw before.
func TestAFailedRefreshLeavesTheServedPoolExactlyAsItWas(t *testing.T) {
	t.Parallel()
	oracle, _ := sweepFixtureCards(t)
	scryfall := aShelfThatFallsOverOnThePrintings(t, oracle)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	seedPriceHistory(t, dbPath, "p1", "p2")
	before := sizeOf(t, dbPath)
	history := countRows(t, dbPath, "price_history")

	_, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatal("a refresh whose printings download 500'd reported success")
	}

	if got := sizeOf(t, dbPath); got != before {
		t.Errorf("the served pool changed size during a failed refresh: %d -> %d", before, got)
	}
	if got := countRows(t, dbPath, "price_history"); got != history {
		t.Errorf("price history changed during a failed refresh: %d -> %d", history, got)
	}
	// The oracle half *loaded* before the break, and it must have gone into
	// the build file rather than the pool -- otherwise "untouched" is untrue.
	if got := countRows(t, dbPath, "oracle_cards"); got != 0 {
		t.Errorf("a failed refresh left %d oracle rows in the served pool; "+
			"the half-loaded rows belong in the discarded build file", got)
	}
	assertNoLeavings(t, dbPath)
}

// A crashed run leaves its half-built file behind. The next refresh owns that
// name, so it clears it rather than refusing to start.
func TestAStaleBuildFileFromACrashedRunDoesNotBlockTheNextRefresh(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	stale := dbPath + ".rebuilding"
	if err := os.WriteFile(stale, []byte("not a database at all"), 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{}); err != nil {
		t.Fatalf("a stale build file blocked the next refresh: %v", err)
	}
	if got := countRows(t, dbPath, "oracle_cards"); got != len(oracle) {
		t.Errorf("oracle_cards has %d rows, want %d", got, len(oracle))
	}
	assertNoLeavings(t, dbPath)
}

// A finished refresh leaves nothing beside the pool. Worth its own assertion
// because the build file is invisible to every other test -- a rebuild that
// stopped cleaning up would go unnoticed until a volume filled.
func assertNoLeavings(t *testing.T, dbPath string) {
	t.Helper()
	_, err := os.Stat(dbPath + ".rebuilding")
	if err == nil {
		t.Errorf("a half-built pool was left at %s.rebuilding", dbPath)
		return
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("checking for leavings beside %s: %v", dbPath, err)
	}
}

// **A refresh records the prices it loaded.** Left to a separate command the
// table gained a row whenever somebody remembered, which on the instance came
// to two days seventeen days apart. A refresh is also the only moment the
// numbers change at all -- nothing but the loader writes `printings.price_usd`
// -- so this is both the useful moment and the only useful moment.
func TestARefreshRecordsThePricesItLoaded(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")

	if _, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      dbPath,
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	priced := countRows(t, dbPath, "printings WHERE price_usd IS NOT NULL OR price_usd_foil IS NOT NULL")
	if priced == 0 {
		t.Skip("the frozen fixture carries no priced printings; nothing to record")
	}
	if got := countRows(t, dbPath, "price_history"); got != priced {
		t.Errorf("the refresh recorded %d prices, want %d -- a refresh is the "+
			"only moment these numbers move, so a refresh that does not record "+
			"them loses that day permanently", got, priced)
	}
}

// Refreshing twice in a day replaces that day rather than doubling it. The
// key is (snapshot_date, printing_id) and the statement is INSERT OR REPLACE;
// this is the test that says so out loud, because the alternative failure --
// a history that grows a duplicate set per run -- would look like data.
func TestTwoRefreshesInADayLeaveOneDayOfPrices(t *testing.T) {
	t.Parallel()
	oracle, printings := sweepFixtureCards(t)
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	shelf := filepath.Join(dir, "scryfall")

	for range 2 {
		if _, err := pool.Refresh(context.Background(), pool.RefreshOptions{
			DBPath: dbPath, ScryfallDir: shelf,
			IndexURL: scryfall.URL + "/bulk-data",
		}, pool.RefreshWatcher{}); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	}

	priced := countRows(t, dbPath, "printings WHERE price_usd IS NOT NULL OR price_usd_foil IS NOT NULL")
	if priced == 0 {
		t.Skip("the frozen fixture carries no priced printings; nothing to record")
	}
	if got := countRows(t, dbPath, "price_history"); got != priced {
		t.Errorf("two refreshes on one day left %d price rows, want %d -- the "+
			"second run must replace the day, not add a second copy of it", got, priced)
	}
}
