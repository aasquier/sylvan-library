// Package pool opens the card pool -- the DuckDB file `mtglab data refresh`
// writes -- the way `cards/db.py:connect_readonly` does: read-only, so a
// running refresh (which wants DuckDB's single writer lock) degrades the
// app rather than being locked out by it.
//
// This is the Go migration's CGO spike (docs/go-migration/PLAN.md, Phase 2):
// the driver is github.com/duckdb/duckdb-go (the community driver that used to
// live at marcboeker/go-duckdb, moved to the DuckDB organisation), which links
// a prebuilt libduckdb per platform. Nothing in the front door imports this
// package yet; it exists so that the build on this Mac and on both CI
// architectures is proven before Phase 3 leans on it.
package pool

import (
	"context"
	"database/sql"
	"fmt"

	// The driver registers itself as "duckdb".
	_ "github.com/duckdb/duckdb-go/v2"
)

// Open opens the pool at path read-only. An empty path opens an in-memory
// database, which is what the spike test uses to prove the link on a machine
// that has no pool.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := path
	if path != "" {
		dsn = path + "?access_mode=READ_ONLY"
	}
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return db, nil
}

// Count answers `SELECT count(*) FROM <table>`, the query `service.health`
// runs for `oracle_cards` and `printings`.
func Count(ctx context.Context, db *sql.DB, table string) (int64, error) {
	if table != "oracle_cards" && table != "printings" {
		return 0, fmt.Errorf("count: %q is not a pool table", table)
	}
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}
