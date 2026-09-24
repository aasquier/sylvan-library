package cache_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"

	_ "modernc.org/sqlite"
)

// The store on a database that answers reads and refuses writes.
//
// **A closed handle cannot ask this question**, which is why this file stands
// beside `closedstore_test.go` rather than inside it: when the handle has gone,
// the very first call fails and every later step of a write is unreachable. The
// shape that reaches them is a database that is perfectly healthy for reading
// and will not take a row — a volume that filled up, a file gone read-only
// under a process that already has it open, a replica somebody pointed the app
// at by mistake.
//
// The fixture is three `BEFORE` triggers that `RAISE(ABORT)`. They are the
// smallest thing that says "this statement will not run" to SQLite while
// leaving the connection, the schema and every `SELECT` exactly as they were.
//
// The assertion is the one `closedstore_test.go` makes and it is the whole of
// ADR 18's contract: **a cache failure is never a failure.** `Get` still hands
// back the row it found even though it could not restamp it; `Put` reports
// nothing because there is nothing a caller could do; `Clear` says zero rather
// than lying about rows it did not remove.

// refusing is a real store over a real migrated database whose `sim_cache`
// table will not accept a write. `seed` rows are inserted before the triggers
// are armed, so there is something to read.
func refusing(t *testing.T, seed map[string]string) *cache.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatalf("build the scratch app.db: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("open the scratch database: %v", err)
	}
	defer func() { _ = db.Close() }()
	for key, kind := range seed {
		if _, err := db.Exec(
			"INSERT INTO sim_cache (key, kind, result_json, created_at, "+
				"last_used_at) VALUES (?, ?, ?, ?, ?)",
			key, kind, `{"games":1}`, "2026-01-01T00:00:00+00:00",
			"2026-01-01T00:00:00+00:00"); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	for _, on := range []string{"INSERT", "UPDATE", "DELETE"} {
		if _, err := db.Exec("CREATE TRIGGER refuse_" + on +
			" BEFORE " + on + " ON sim_cache BEGIN " +
			"SELECT RAISE(ABORT, 'this database will not take a write'); END"); err != nil {
			t.Fatalf("arm the %s refusal: %v", on, err)
		}
	}

	store, err := cache.Open(path, nil)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// A read that cannot restamp its row still hands back the row. The touch is
// what makes eviction least-recently-*used*, and losing one row's place in
// that queue is not a reason to withhold an answer that is already in hand.
func TestAReadThatCannotBeRestampedStillAnswers(t *testing.T) {
	t.Parallel()
	store := refusing(t, map[string]string{"stored": "sim.mana"})

	hit := store.Get(context.Background(), "stored")
	if hit == nil {
		t.Fatal("a row that is there did not come back because it could not be touched")
	}
	if string(hit.Result) != `{"games":1}` {
		t.Errorf("the row came back as %s", hit.Result)
	}
	if hits, misses := store.Counts(); hits != 1 || misses != 0 {
		t.Errorf("the read counted %d hits and %d misses", hits, misses)
	}
}

// A write the database refuses is a write nobody hears about, because there is
// nothing a caller could do with the news: the answer it was about to serve is
// already correct, and the only thing lost is the next request's speed.
func TestAWriteTheDatabaseRefusesIsSilentAndHarmless(t *testing.T) {
	t.Parallel()
	store := refusing(t, nil)
	ctx := context.Background()

	store.Put(ctx, "k1", "tier1", result{Games: 20000, Rate: 0.25})
	if hit := store.Get(ctx, "k1"); hit != nil {
		t.Error("a refused write came back on the next read")
	}
	// And the miss was counted as a miss rather than as anything stranger: the
	// caller computes, which is exactly what a cold cache asks of it.
	if _, misses := store.Counts(); misses != 1 {
		t.Errorf("the read after a refused write counted %d misses", misses)
	}
}

// Clear reports what it actually removed. A count it could read and rows it
// could not delete is zero, not the number it hoped for -- `mtglab sim cache
// clear` printing "2 rows" over a table still holding two is the one answer
// that would send somebody looking for a bug somewhere else.
func TestAClearThatRemovedNothingSaysZero(t *testing.T) {
	t.Parallel()
	store := refusing(t, map[string]string{"a": "tier1", "b": "sim.mana"})
	ctx := context.Background()

	if got := store.Clear(ctx); got != 0 {
		t.Errorf("a clear that removed nothing reported %d rows", got)
	}
	if stats := store.Stats(ctx); stats.Rows != 2 {
		t.Errorf("after the refused clear the table holds %d rows", stats.Rows)
	}
}
