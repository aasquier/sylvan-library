// Package authtest hands out `app.db`'s schema as Python's migration ladder
// leaves it, for the tests that need a real one.
//
// It exists because the same fixture was written by hand in three places --
// `internal/auth`, `internal/decklog` and `internal/door` -- each a
// transcription of whichever rung of `auth/db.py`'s ladder the author had open
// at the time, and each therefore frozen at a different version. Two of the
// three broke on the same afternoon when the accounts flip first read
// `users.model_tier`, a column rung 10 adds; both failed as `no such column`,
// from fixtures that had been quietly claiming to be a schema they were eight
// rungs behind.
//
// So the bytes are generated rather than typed. `tests/go_fixtures.py` reads
// `sqlite_master` out of a database the Python remnant's ladder just built and
// writes `app_schema.sql` beside this file; `tests/test_go_fixtures.py` fails
// when the committed copy is not what that ladder writes today. Since Phase 8
// **the Go ladder owns the deployed file** (`internal/auth/schema.go`), and
// this fixture is the lockstep between the two:
// `TestMigrateBuildsWhatPythonBuilt` holds `auth.Migrate`'s result to these
// bytes, so a rung added to either ladder fails a test until the other side
// carries it too.
//
// The lesson is the fixture-decks one from Phase 4's edit engine, in a new
// place: a fixture somebody wrote by hand is not the artefact it stands in
// for. `internal/pool/pooltest` is the same arrangement for the card pool.
//
// Only tests import this. It embeds a few kilobytes of DDL and nothing else.
package authtest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// Registers the "sqlite" driver, so a caller needs only this package.
	_ "modernc.org/sqlite"

	_ "embed"
)

//go:embed app_schema.sql
var schema string

// Schema is `app.db`'s DDL, including its `PRAGMA user_version`, so a database
// built from it reports the version Python would refuse to migrate further.
func Schema() string { return schema }

// NewScratchDB builds an empty `app.db` at path and returns it, in WAL mode
// with foreign keys on -- the two pragmas the ladder sets and that the
// cascades and the busy timeout depend on.
//
// It executes the fixture rather than calling `auth.Migrate` so that this
// package needs no import of `internal/auth` (whose own tests import this
// one) -- and so a ladder bug cannot quietly shape every fixture that would
// otherwise have been built through it.
func NewScratchDB(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("scratch app.db: %w", err)
	}
	db, err := sql.Open("sqlite",
		"file:"+path+"?mode=rwc&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return fmt.Errorf("scratch app.db: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("building the scratch app.db: %w", err)
	}
	return nil
}
