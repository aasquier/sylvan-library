package cache

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The cache when the volume goes away partway through a write.
//
// `closedstore_test.go` asks what happens when the handle has gone, and
// `refusingstore_test.go` what happens when the table will not take a row.
// Both fail at the *first* statement of [Store.Put]. A write here is five
// statements long — begin, upsert, count, maybe evict, commit — and the four
// after the first had no fixture at all, which matters more here than the
// coverage number suggests: they are exactly the statements that decide
// whether a **half-written row can be served as a cached answer**. A row a
// caller reads back as `cached: true` is quoted to a player as a settled
// number (CLAUDE.md's invariant), so a row this method left behind after
// giving up would be worse than no cache at all.
//
// The fixture is `authtest.OpenFaulty`: a real migrated `app.db` whose handle
// answers a set number of statements and then refuses every one. Counted, not
// guessed — each test spells out which statement of the write its budget ends
// on, and the statement after it is the branch under test.
//
// The assertion is never the driver's wording. It is ADR 18's contract in two
// halves: **a cache failure is never a failure** (Put reports nothing, the
// process carries on) and **a failed write leaves nothing readable** (the key
// misses afterwards, so no partial row is ever served as settled).

// hush is a logger for the warnings these paths write on the way out; they are
// the intended behaviour, not test output.
func hush() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// faulty is a store over a real migrated `app.db` reached through a handle
// with a statement budget. Built in-package because there is no seam for one:
// `Open` owns its handle, deliberately, and a store is two fields.
func faulty(t *testing.T) (*Store, *authtest.Fault) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Store{db: db, log: hush()}, fault
}

// fill puts n rows in the table directly, in one statement, so a test can
// stand a write up against a table that is already over [MaxRows] without
// two thousand round trips.
func fill(t *testing.T, s *Store, n int) {
	t.Helper()
	if _, err := s.db.Exec(
		`WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL`+
			` SELECT i + 1 FROM seq WHERE i < ?)`+
			` INSERT INTO sim_cache (key, kind, result_json, created_at,`+
			` last_used_at) SELECT 'filler-' || i, 'sim.mana', '{}',`+
			` '2026-01-01T00:00:00+00:00', '2026-01-01T00:00:00+00:00' FROM seq`,
		n); err != nil {
		t.Fatalf("filling the cache with %d rows: %v", n, err)
	}
	var total int
	if err := s.db.QueryRow("SELECT count(*) FROM sim_cache").Scan(&total); err != nil {
		t.Fatalf("counting the filled rows: %v", err)
	}
	if total != n {
		t.Fatalf("the fill put %d rows in, not %d", total, n)
	}
}

type stored struct {
	Games int `json:"games"`
}

// Each budget ends on a different statement of the write, so each subtest
// names one branch: the count that decides whether to evict, the eviction
// itself, and the commit.
//
// The row count is what separates the first two: the eviction only runs on a
// table already over [MaxRows], which is why the middle case fills the table
// first and the others do not.
func TestAWriteThatDiesPartwayThroughStoresNothingReadable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// rows is how many are in the table before the write.
		rows int
		// budget is how many statements answer before the handle refuses.
		// One BEGIN, one upsert, one count, then (over MaxRows) one delete,
		// then the commit.
		budget int
	}{
		{"the count that decides on eviction", 0, 2},
		{"the eviction itself", MaxRows + 1, 3},
		{"the commit", 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, fault := faulty(t)
			if tc.rows > 0 {
				fill(t, store, tc.rows)
			}
			fault.After(tc.budget)

			// Never fails the caller: no panic, no error to return, nothing
			// for the simulation that is holding the result to do.
			store.Put(t.Context(), "half-written", "sim.mana", stored{Games: 20000})

			fault.Heal()
			if hit := store.Get(t.Context(), "half-written"); hit != nil {
				t.Errorf("a write that died on %s left a row that reads back "+
					"as cached: %s", tc.name, hit.Result)
			}
		})
	}
}

// The same write with the budget one statement wider lands — which is what
// makes every subtest above a statement about the branch it names rather than
// about the fixture refusing everything.
func TestTheSameWriteLandsWhenTheBudgetReachesTheCommit(t *testing.T) {
	t.Parallel()
	store, fault := faulty(t)
	fault.After(4)

	store.Put(t.Context(), "written", "sim.mana", stored{Games: 20000})

	fault.Heal()
	hit := store.Get(t.Context(), "written")
	if hit == nil {
		t.Fatal("a write with the whole budget stored nothing -- the counts in " +
			"the test above are no longer the statements Put runs")
	}
	if string(hit.Result) != `{"games":20000}` {
		t.Errorf("the stored row came back as %s", hit.Result)
	}
}

// The eviction branch, proven to be the branch: over [MaxRows] the write
// deletes the oldest rows, and the table is back at the ceiling afterwards.
//
// Without this the subtest above could be passing because the eviction never
// ran at all, which is the failure mode a budget-shaped fixture has.
func TestAWriteOverTheCeilingEvictsDownToIt(t *testing.T) {
	t.Parallel()
	store, fault := faulty(t)
	fill(t, store, MaxRows+5)
	fault.Heal()

	store.Put(t.Context(), "the-newest", "sim.mana", stored{Games: 1})

	var total int
	if err := store.db.QueryRow("SELECT count(*) FROM sim_cache").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != MaxRows {
		t.Errorf("the table holds %d rows after the eviction, not %d", total, MaxRows)
	}
	if hit := store.Get(t.Context(), "the-newest"); hit == nil {
		t.Error("the eviction took the row the write had just put in")
	}
}

// A read cut short partway down its result set: the query ran, and the
// iteration failed rather than ended.
//
// `Stats` is what `mtglab sim cache` prints, and its whole reason for existing
// is that "the cache is empty" and "the cache is disabled" want different
// responses. A third state — "I could not finish reading it" — is recorded
// here rather than invented: the counts come back short and `Enabled` stays
// true, with the warning in the log. The row count a person reads is a
// partial one, which is the honest consequence of a method that may not fail
// its caller; what must never happen is the process going down over a cache.
func TestStatsCutShortMidIterationReportsWhatItReadRatherThanFailing(t *testing.T) {
	t.Parallel()
	store, fault := faulty(t)
	fill(t, store, 3)

	whole := store.Stats(t.Context())
	if whole.Rows != 3 {
		t.Fatalf("the healthy read reports %d rows, not 3", whole.Rows)
	}

	// The set's very first read fails, so nothing is scanned and the loop's
	// exit is a fault rather than an end.
	fault.RowsAfter(0)
	cut := store.Stats(t.Context())
	if cut.Rows != 0 {
		t.Errorf("a read that failed at its first row reported %d rows", cut.Rows)
	}
	if !cut.Enabled {
		t.Error("a failed read turned the cache off, which is a different sentence")
	}
	if cut.ByKind == nil {
		t.Error("the by-kind map came back nil rather than empty")
	}
}
