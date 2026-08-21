// Package auth is the Go reading of `mtglab/auth`: who a session token
// belongs to, and whether a password matches its Argon2id hash. In Phase 2
// of the migration (docs/go-migration/PLAN.md) the front door needs exactly
// one thing from it -- resolve the `sid` cookie to a caller before routing --
// and the password half is here so the compatibility ADR 38 promises
// ("Argon2id PHC hashes verify as-is") is proven by a test rather than by a
// sentence.
//
// **This package never writes `app.db`.** Python owns the schema ladder until
// Phase 8 and, during coexistence, Python's own middleware still runs behind
// the door and performs the session touch and the expired-row delete that
// `auth/sessions.py:lookup` does. Two writers is the risk the plan names
// (risk 6), so the door opens the file read-only and the cost is nil: a
// reader in WAL mode blocks nobody and is blocked by nobody.
//
// The driver is modernc.org/sqlite, the pure-Go translation, decided at this
// spike over mattn/go-sqlite3 (CGO): DuckDB is already the one CGO dependency
// this module carries, and keeping SQLite out of CGO means the front door is
// a static binary with no C toolchain in its build and the auth tests run
// race-detected without one. PLAN.md section 6 records the call.
package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	// Registers the "sqlite" driver.
	_ "modernc.org/sqlite"
)

// Open opens `app.db` at path, read-only. It does not create the file: an
// instance whose Python half has not booted yet has no `app.db`, and the
// honest answer to a cookie then is "anonymous", not an empty database that
// Python's ladder would later have to migrate from version zero.
//
// The busy timeout matches `auth/db.py` (5000ms). Journal mode is not set
// here -- WAL is persistent in the file, Python set it, and a read-only
// connection may not change it anyway.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(path) + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db: %w", err)
	}
	// A handful of readers is plenty: every lookup is a primary-key hit.
	db.SetMaxOpenConns(4)
	return db, nil
}

// Ping opens a connection and proves the users table is there -- the check a
// door wants at startup so a wrong MTGLAB_DATA_DIR fails loudly rather than as
// a 401 on every request.
func Ping(ctx context.Context, db *sql.DB) error {
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1 FROM users LIMIT 1").Scan(&one); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("app.db is not readable: %w", err)
	}
	return nil
}
