package pool

import (
	"context"
	"os"
	"testing"
)

// The CGO spike: if this links and runs, the driver's prebuilt libduckdb is
// usable on this platform. An in-memory database so the test needs no pool.
func TestInMemoryLinksAndAnswers(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "")
	if err != nil {
		t.Fatalf("open in-memory duckdb: %v", err)
	}
	defer db.Close()
	var answer int
	if err := db.QueryRowContext(ctx, "SELECT 42").Scan(&answer); err != nil {
		t.Fatalf("SELECT 42: %v", err)
	}
	if answer != 42 {
		t.Fatalf("SELECT 42 answered %d", answer)
	}
	var version string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err != nil {
		t.Fatalf("version(): %v", err)
	}
	t.Logf("libduckdb %s", version)
}

// A pool file Python wrote, read by Go: set MTGLAB_TEST_POOL to a DuckDB
// file built by `tests/tiny_pool.py` (or the real pool). Skipped without it,
// because CI's Go jobs have no Python and no pool; the file-level
// compatibility is proven on the maintainer's machine and, from Phase 3, by
// the contract suite through the front door.
func TestReadsAPoolPythonWrote(t *testing.T) {
	path := os.Getenv("MTGLAB_TEST_POOL")
	if path == "" {
		t.Skip("MTGLAB_TEST_POOL not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open %s read-only: %v", path, err)
	}
	defer db.Close()
	for _, table := range []string{"oracle_cards", "printings"} {
		n, err := Count(ctx, db, table)
		if err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n == 0 {
			t.Fatalf("%s is empty in %s", table, path)
		}
		t.Logf("%s: %d rows", table, n)
	}
}

func TestCountRefusesAnUnknownTable(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Count(ctx, db, "users; DROP TABLE x"); err == nil {
		t.Fatal("Count accepted a table name it should have refused")
	}
}
