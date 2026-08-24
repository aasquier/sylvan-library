// The schema ladder for `app.db` — the ladder the deployed instance runs
// at every boot.
//
// The twelve scripts under `migrations/` are the recorded schema, byte for
// byte: SQLite stores a CREATE statement's exact text in `sqlite_master`,
// so executing identical SQL is what keeps a freshly-minted `app.db`
// indistinguishable from every one already in the field — which
// `TestMigrateBuildsTheRecordedSchema` holds against
// `authtest/app_schema.sql`, and which is the property every `mode=rw`
// handle in this module quietly relies on. A new rung is a new script under
// `migrations/`, and the recorded schema beside the tests moves in the
// same change, never by drift.
//
// Two behaviours are the contract rather than detail:
// foreign keys are OFF for the duration and `foreign_key_check` signs the
// file off afterwards (rung 5 rebuilds `users`, which `sessions` and
// `auth_tokens` reference ON DELETE CASCADE — with the pragma on, that DROP
// would silently sign everyone out); and the ladder is forward-only,
// applied at boot, so a schema change deploys itself exactly as ADR 23
// warns — land one on its own branch and watch it.

package auth

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// SchemaVersion is the ladder's height, stored in SQLite's own
// `user_version` — no table to migrate, nothing to forget in a dump.
// Bumped when a script is added under `migrations/`.
const SchemaVersion = 12

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrations returns the ladder in order. A missing rung is an error, not a
// shorter ladder: `%04d.sql` names are looked up one by one rather than
// globbed, so a misnamed file cannot silently drop the rungs after it.
func migrations() ([]string, error) {
	scripts := make([]string, 0, SchemaVersion)
	for i := 1; i <= SchemaVersion; i++ {
		b, err := migrationFS.ReadFile(fmt.Sprintf("migrations/%04d.sql", i))
		if err != nil {
			return nil, fmt.Errorf("schema ladder: %w", err)
		}
		scripts = append(scripts, string(b))
	}
	return scripts, nil
}

// Migrate opens `app.db` at path — creating the file if it is absent — and
// brings it to SchemaVersion. It is the one place in the served app allowed
// to create the file (`mode=rwc`): every request-path handle stays `mode=rw`
// or `mode=ro`, so a wrong data directory fails loudly at the first write
// rather than minting an empty database beside the real one.
//
// Idempotent, and cheap when there is nothing to do: a file already at
// SchemaVersion costs one pragma read.
func Migrate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	// WAL is a property of the file and persists once set; busy_timeout is
	// this connection's, matching every other handle in the module.
	dsn := "file:" + url.PathEscape(path) +
		"?mode=rwc&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	// The pragma dance below is per-connection state, and database/sql is a
	// pool: pin one connection so `foreign_keys=OFF` and the scripts that
	// rely on it cannot land on different handles.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	defer func() { _ = conn.Close() }()
	return migrate(ctx, conn)
}

func migrate(ctx context.Context, conn *sql.Conn) error {
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("app.db: reading user_version: %w", err)
	}
	if version >= SchemaVersion {
		return nil
	}
	scripts, err := migrations()
	if err != nil {
		return err
	}
	// Off for the whole ladder, not just the rung that rebuilds a table: the
	// pragma is a no-op inside a transaction, so a script that opens one
	// could not set it — and the next rebuild inherits the safety instead of
	// rediscovering the trap.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys=ON") }()
	for i, script := range scripts[version:] {
		if _, err := conn.ExecContext(ctx, script); err != nil {
			return fmt.Errorf("app.db: migration %d: %w", version+i+1, err)
		}
	}
	// Pragmas are parsed, not bound; the value is this package's constant,
	// never input.
	if _, err := conn.ExecContext(ctx,
		fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	// The other half of the bargain: having switched enforcement off, prove
	// nothing broke while it was off before anyone is served from this file.
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	defer func() { _ = rows.Close() }()
	violations := 0
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("app.db: %w", err)
	}
	if violations > 0 {
		return fmt.Errorf("migrating app.db to version %d left %d foreign key "+
			"violation(s); the file has not been signed off and must be "+
			"restored from a backup (docs/HOSTING.md §5)",
			SchemaVersion, violations)
	}
	return nil
}
