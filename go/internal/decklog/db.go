package decklog

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	// Registers the "sqlite" driver. Same choice as `internal/auth`: the
	// pure-Go translation, so the door stays a binary with no C toolchain
	// behind SQLite.
	_ "modernc.org/sqlite"
)

// openReadWrite opens `app.db` for the log's inserts.
//
// **`mode=rw`, never `rwc`.** `internal/auth` gives the reason for read-only
// and this gives the reason for not creating: the ladder (`auth.Migrate`)
// runs once at boot and is the only creator, so a file this created would be
// a database at version zero beside the real one. An absent `app.db` is a
// warned, dropped entry.
//
// Not the only writer, and it is worth saying so. The door performs the
// session touch and the expired-row delete on every authenticated request,
// so the file has more than one writing handle. That is safe here and not
// by luck: the file is in WAL mode (persistent in the file once set), where
// a writer blocks readers not at all, and the busy timeout matches the auth
// side's 5000ms so two writers collide as a short wait rather than as
// `database is locked`. `deck_log` is append-only and nothing else writes
// it, so there is no row two writers can contend over -- only the file's
// write lock, for the microseconds an insert holds it.
func openReadWrite(path string) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(path) +
		"?mode=rw&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db for writing: %w", err)
	}
	// One writer. SQLite serialises them anyway, and a pool of them would
	// only convert waiting-in-Go into waiting-on-the-file lock.
	db.SetMaxOpenConns(1)
	return db, nil
}

// ping proves the file opens and `deck_log` is there. `sql.Open` only records
// the DSN, so without this a missing `app.db` would be discovered by the first
// insert -- which is exactly where a failure must not be discovered.
func ping(db *sql.DB) error {
	var one int
	err := db.QueryRow("SELECT 1 FROM deck_log LIMIT 1").Scan(&one)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("app.db is not writable: %w", err)
	}
	return nil
}
