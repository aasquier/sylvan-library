package pool

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The write side's refusals, and the tidying that surrounds them.
//
// **The load is the one operation in this repo that can destroy a library**,
// and everything here is a guard standing between a bad run and that outcome:
// the transaction that rolls a half-load back, the appender that refuses a
// table it cannot bind, the half-built file that must clear before the next
// run starts, and the mode and owner the new pool has to inherit before it
// takes the old one's place. Each of them is a `return err` that had never
// been driven, and each of them is invisible in a green refresh.

// aWritablePool is a real pool file, schema and all, opened read-write the way
// a refresh opens it.
func aWritablePool(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pool.duckdb")
	db, err := OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// aBulkFile parks lines under a name the loader will read.
func aBulkFile(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oracle_cards-2026-08-24.jsonl")
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A load into a pool that is no longer open is refused by name, rather than
// counting rows into nothing. The refresh holds one handle for the whole run,
// so "the handle went" is what a shutdown mid-refresh looks like from here.
func TestALoadIntoAClosedPoolIsRefusedByTable(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	n, err := LoadOracle(context.Background(), db,
		aBulkFile(t, `{"oracle_id":"o1","name":"Fixture Chef"}`+"\n"))
	if err == nil {
		t.Fatalf("a closed pool accepted %d rows", n)
	}
	if !strings.Contains(err.Error(), "oracle_cards") {
		t.Errorf("the refusal does not name the table being loaded: %v", err)
	}
}

// A load aimed at a catalog that is not attached is refused at the emptying
// step, before a single row is read. The catalog is how a rebuild fills a new
// file from a connection holding the old one, and an appender pointed at the
// wrong one would cheerfully fill the *old* pool while the DELETE emptied the
// new one -- so this is the guard that has to fire first.
func TestALoadIntoACatalogThatIsNotThereEmptiesNothing(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	path := aBulkFile(t, `{"oracle_id":"o1","name":"Fixture Chef"}`+"\n")
	if _, err := loadInto(context.Background(), db, "no_such_catalog", path,
		"oracle_cards", OracleColumns, SkipOracleLayout, OracleRow); err == nil {
		t.Fatal("a load into a catalog nothing attached reported success")
	}
	// The connection's own pool is untouched: nothing was emptied anywhere.
	var n int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM oracle_cards").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the served pool holds %d rows after a load that went nowhere", n)
	}
}

// A table the appender cannot bind is refused before any row is written --
// and the emptying it already did rolls back with it, which is the whole
// reason the DELETE and the load share one transaction.
func TestATableTheAppenderCannotBindKeepsItsRows(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE half_a_schema (oracle_id VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO half_a_schema VALUES ('the row that was already there')`); err != nil {
		t.Fatal(err)
	}
	path := aBulkFile(t, `{"oracle_id":"o1","name":"Fixture Chef"}`+"\n")
	if _, err := loadInto(ctx, db, "", path, "half_a_schema", OracleColumns,
		SkipOracleLayout, OracleRow); err == nil {
		t.Fatal("a table with one of twenty-eight columns accepted the load")
	}
	var n int64
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM half_a_schema").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("the table holds %d rows; the refused load emptied it and "+
			"left it emptied, which is the outage the transaction exists to "+
			"prevent", n)
	}
}

// **The half-read bulk file**, which is the fault the transaction was put
// there for: 500MB arrives over a network and stops making sense in the
// middle. Every row before the break is discarded and the pool is the pool it
// was -- a library with the cards it had beats a library with two thirds of
// them and no way to tell.
func TestABulkFileThatStopsMakingSenseLeavesTheRowsThatWereThere(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oracle_cards (oracle_id, name) VALUES ('o0', 'Fixture Elder')`); err != nil {
		t.Fatal(err)
	}

	truncated := aBulkFile(t,
		`{"oracle_id":"o1","name":"Fixture Chef"}`+"\n"+
			`{"oracle_id":"o2","name":"Fixture Sq`)
	n, err := LoadOracle(ctx, db, truncated)
	if err == nil {
		t.Fatalf("a bulk file cut in half loaded %d rows and reported success", n)
	}
	var left int64
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM oracle_cards").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("the pool holds %d rows after a failed load, want the one it "+
			"started with", left)
	}
	var name string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM oracle_cards").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Fixture Elder" {
		t.Errorf("the surviving row is %q, want the one that was there before", name)
	}
}

// The layouts a Commander pool has no use for never reach the appender, and
// the count a refresh reports is what it actually shelved.
func TestTheLayoutsACommanderPoolHasNoUseForAreNeverShelved(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	ctx := context.Background()
	path := aBulkFile(t, strings.Join([]string{
		`{"oracle_id":"o1","name":"Fixture Chef","layout":"normal"}`,
		`{"oracle_id":"o2","name":"Fixture Chef (Art)","layout":"art_series"}`,
		`{"oracle_id":"o3","name":"Fixture Food","layout":"token"}`,
		`{"oracle_id":"o4","name":"Fixture Twins","layout":"double_faced_token"}`,
		`{"oracle_id":"o5","name":"Fixture Squire","layout":"normal"}`,
	}, "\n")+"\n")

	n, err := LoadOracle(ctx, db, path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("the load reported %d rows shelved out of five, want the two "+
			"that are cards", n)
	}
	var shelved int64
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM oracle_cards").Scan(&shelved); err != nil {
		t.Fatal(err)
	}
	if shelved != n {
		t.Errorf("the pool holds %d rows and the run reported %d", shelved, n)
	}
}

// A price snapshot taken against a pool that is gone is a refusal, not a
// green `snapshotted 0 prices`. The command already has a true zero -- a pool
// with no priced printings in it -- and the two must not share a sentence.
func TestASnapshotAgainstAPoolThatIsGoneIsRefused(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	n, err := SnapshotPrices(context.Background(), db)
	if err == nil {
		t.Fatalf("a closed pool reported %d prices snapshotted", n)
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("the refusal does not say what was being done: %v", err)
	}
}

// A file that is not this pool is refused at the open, rather than half
// migrated into one. `OpenWriter` is the only place the schema and the
// added-column ladder are applied, and it runs against whatever is at the
// path -- a restore from somewhere else, a name typed wrong, a database some
// other tool made.
func TestAFileThatIsSomeOtherDatabaseIsRefusedRatherThanMigrated(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "somebody-elses.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	// The name this pool wants, already taken by something that is not a table.
	if _, err := db.ExecContext(context.Background(),
		`CREATE VIEW oracle_cards AS SELECT 1 AS oracle_id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := OpenWriter(context.Background(), path)
	if err == nil {
		_ = opened.Close()
		t.Fatal("a database that is not this pool was opened as one")
	}
	if !strings.Contains(err.Error(), "pool schema") {
		t.Errorf("the refusal does not say which step refused: %v", err)
	}
}

// ---- what surrounds a rebuild ---------------------------------------------

// A rebuild that cannot take a connection to the pool tidies its half-built
// file away before it reports. The file is named after the served pool, so
// leaving one behind would meet the *next* run as a stale build to clear --
// which works, but only because that run tidies up after this one's mess.
func TestARebuildThatCannotStartLeavesNoHalfBuiltFile(t *testing.T) {
	t.Parallel()
	db, dbPath := aWritablePool(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := startRebuild(context.Background(), db, dbPath)
	if err == nil {
		r.abandon(context.Background())
		t.Fatal("a rebuild started on a pool handle that is closed")
	}
	if _, statErr := os.Stat(dbPath + rebuildSuffix); statErr == nil {
		t.Errorf("a rebuild that never started left %s behind",
			dbPath+rebuildSuffix)
	}
}

// **The one step a rebuild may not skip.** `price_history` is append-only and
// no bulk file can reconstruct it, so a rebuild that cannot carry it across
// must fail rather than rename a fresh file over the only copy. The loss would
// be silent for weeks -- every other count right, and a price chart with
// nothing behind it.
func TestARebuildThatCannotCarryThePriceHistoryRefusesToPublish(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, dbPath := aWritablePool(t)
	r, err := startRebuild(ctx, db, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.abandon(ctx) })

	// The old pool loses the table between the start and the finish, which is
	// what a restore from a schema that predates it looks like.
	if _, err := db.ExecContext(ctx, "DROP TABLE price_history"); err != nil {
		t.Fatal(err)
	}
	err = r.finish(ctx)
	if err == nil {
		t.Fatal("a rebuild published a pool without carrying the price history")
	}
	if !strings.Contains(err.Error(), "price_history") {
		t.Errorf("the refusal does not name the table that could not be "+
			"carried: %v", err)
	}
	// And the served pool is still the served pool: nothing was renamed.
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Errorf("the served pool is gone after a refused rebuild: %v", statErr)
	}
}

// The half-built file belongs to the next run by name, so a run that cannot
// clear it refuses to start rather than loading into a file it does not
// understand. Absence is the ordinary case and not an error at all.
func TestABuildPathThatWillNotClearStopsTheRunBeforeItStarts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := removeBuild(filepath.Join(dir, "never-existed")); err != nil {
		t.Errorf("clearing a file that was not there was treated as a fault: %v", err)
	}

	// A directory with something in it: `os.Remove` will not take it, and the
	// run has nowhere to build.
	blocked := filepath.Join(dir, "pool.duckdb"+rebuildSuffix)
	if err := os.MkdirAll(filepath.Join(blocked, "in the way"), 0o750); err != nil {
		t.Fatal(err)
	}
	err := removeBuild(blocked)
	if err == nil {
		t.Fatal("a build path that will not clear was reported clear")
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Errorf("the refusal does not name the path an operator has to go and "+
			"look at: %v", err)
	}
}

// **The new pool wears the old one's skin before it takes its place.** A
// refresh driven over a shell runs as root while the app runs as its own user,
// so a file that arrives root-owned is a pool the app cannot write at the next
// refresh -- a fault that shows up a day later, on somebody else's shift.
func TestTheNewPoolInheritsTheOldOnesMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	served := filepath.Join(dir, "pool.duckdb")
	built := filepath.Join(dir, "pool.duckdb"+rebuildSuffix)

	// The first refresh on an empty volume: there is no old pool, and that is
	// not a fault -- it is the file being created for the first time.
	if err := wearTheOldSkin(served, built); err != nil {
		t.Errorf("a first refresh was refused for having no pool to inherit "+
			"from: %v", err)
	}

	for _, path := range []string{served, built} {
		if err := os.WriteFile(path, []byte("a pool"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(served, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := wearTheOldSkin(served, built); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(built)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("the new pool wears %v, want the served pool's %v", got, os.FileMode(0o640))
	}
}

// And the two ways it refuses: a pool it cannot look at, and a build file that
// is no longer where it was put. Both are reported rather than shrugged off,
// because either one means the rename that follows would publish a file whose
// permissions nobody has checked.
func TestSkinThatCannotBeReadOrWornIsReported(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	shut := filepath.Join(dir, "shut")
	if err := os.Mkdir(shut, 0o750); err != nil {
		t.Fatal(err)
	}
	served := filepath.Join(shut, "pool.duckdb")
	if err := os.WriteFile(served, []byte("a pool"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A build file that is not there: the mode cannot be set on it.
	if err := wearTheOldSkin(served, filepath.Join(dir, "never-built")); err == nil {
		t.Error("a mode was reported set on a file that does not exist")
	}

	// A pool inside a directory nothing may walk into: the stat is neither an
	// answer nor an honest absence.
	if err := os.Chmod(shut, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, 0o750) })
	if os.Geteuid() == 0 {
		t.Skip("root walks into anything, so there is no refusal to observe")
	}
	if err := wearTheOldSkin(served, filepath.Join(dir, "never-built")); err == nil {
		t.Error("a pool that could not be read was treated as a pool that is " +
			"not there, which is the one reading that lets the rename through")
	}
}

// The three refusals of the rebuild itself, which is the step that decides
// whether a refresh can destroy a library.
//
// Each of these is a `return err` on the road between "the old pool is
// served" and "the new pool is served", and the property every one of them
// has to keep is the same: **the old pool is still there and still the one
// on disk.** The rebuild's whole safety argument is that it never touches
// the served file until the rename, so a refusal anywhere above the rename
// must leave nothing but litter — and the litter must be gone too, because a
// half-built file left under the build name is what the next run's
// `removeBuild` is for and what a *third* run would otherwise trip over.

// A handle that already has the rebuild catalog attached is a previous
// rebuild that never let go — `abandon`'s DETACH failed, or the process died
// between the ATTACH and the tidy. ATTACH is per DuckDB instance rather than
// per connection, so the stale name is visible on the fresh connection this
// one takes, and the second ATTACH is refused by name.
//
// What the test is really about is the tidying: the file this run created
// before it got to the ATTACH has to be removed on the way out, or the next
// run finds a pool-shaped file under the build name and `removeBuild` is the
// only thing standing between it and a load into somebody else's rows.
func TestARebuildOntoAHandleThatNeverLetGoIsRefusedAndLeavesNoFile(t *testing.T) {
	t.Parallel()
	db, dbPath := aWritablePool(t)
	ctx := context.Background()

	// The leftover: the catalog name taken by a file that is not this run's.
	stale := filepath.Join(t.TempDir(), "a-previous-run.duckdb")
	leftover, err := OpenWriter(ctx, stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := leftover.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ATTACH '"+stale+"' AS "+rebuildCatalog); err != nil {
		t.Fatal(err)
	}

	r, err := startRebuild(ctx, db, dbPath)
	if err == nil {
		r.abandon(ctx)
		t.Fatal("a second rebuild attached over the first one")
	}
	if !strings.Contains(err.Error(), dbPath+rebuildSuffix) {
		t.Errorf("the refusal reads %q and does not name the file it was "+
			"attaching", err)
	}
	if _, statErr := os.Stat(dbPath + rebuildSuffix); statErr == nil {
		t.Errorf("the half-built %s survived a refused ATTACH, so the next "+
			"refresh inherits it", dbPath+rebuildSuffix)
	}
}

// The rename is the last act and the only one that touches the served path,
// so the one fault it can meet is a path that cannot become a file: a
// directory where the pool should be, which is what a volume mounted one
// level too deep looks like from in here. Everything before it has already
// succeeded — the history carried, the prices recorded, both handles
// released — and the refusal still has to name the step.
func TestARebuildThatCannotTakeTheOldPoolsPlaceSaysSo(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	ctx := context.Background()

	// The served path is a non-empty directory. `db` is a real pool handle
	// at its own path, which is all `startRebuild` asks of it -- the two are
	// separate arguments precisely because the file being replaced and the
	// handle being read are separate things.
	mount := filepath.Join(t.TempDir(), "mtg.duckdb")
	if err := os.Mkdir(mount, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mount, "something"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := startRebuild(ctx, db, mount)
	if err != nil {
		t.Fatal(err)
	}
	err = r.finish(ctx)
	if err == nil {
		t.Fatal("a rebuild renamed a pool over a directory")
	}
	if !strings.Contains(err.Error(), "in place") {
		t.Errorf("the refusal reads %q and does not name the rename", err)
	}
	// The directory is still a directory with its contents in it: the rename
	// is all-or-nothing and the served path was not emptied on the way.
	if entries, readErr := os.ReadDir(mount); readErr != nil || len(entries) != 1 {
		t.Errorf("the served path holds %v entries (err %v) after a refused "+
			"rename", entries, readErr)
	}
}
