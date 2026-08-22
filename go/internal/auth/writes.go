package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// The write side of `app.db`, and the two transaction shapes `mtglab/auth`
// uses.
//
// **`mode=rw`, never `rwc`.** Python owns the schema ladder until Phase 8
// (`auth/db.py` is a twelve-step migration list), so a file this created would
// be a database at version zero that Python would then have to migrate -- or
// worse, one Go had filled in from a second copy of the schema. The same rule
// `internal/decklog` states, and the same one `Open` above it states for the
// read side.
//
// An absent `app.db` is therefore **read as an empty one**, never created, and
// that is the honest answer rather than a shortcut. Measured against Python
// 2026-08-22 on a fresh data directory with `MTGLAB_ADMIN_EMAIL` unset: the
// database does not exist until the first login, which Python creates and then
// answers 401 against -- because an empty `users` table has nobody in it. A
// reader cannot tell an empty database from an absent one, so answering as if
// it were empty is answering what Python answers, and it leaves the ladder
// where it belongs.

// OpenReadWrite opens `app.db` for the writes the account routes make:
// sessions opened and closed, passwords set, tokens spent, rate-limit windows
// counted. It does not create the file.
//
// Two writers, and it is worth saying which. Python still performs the session
// touch, the expired-row delete and every schema migration, so this makes two
// processes writing one file. That is safe here and not by luck: the file is
// in WAL mode (Python set it, and WAL is persistent in the file), where a
// writer blocks readers not at all, and the busy timeout matches
// `auth/db.py`'s 5000ms so two writers collide as a short wait rather than as
// `database is locked`.
//
// `foreign_keys` is on because `Delete` leans on it: `sessions` and
// `auth_tokens` declare ON DELETE CASCADE, and with the pragma off those
// clauses are a comment and a deleted account leaves its sessions live.
func OpenReadWrite(path string) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(path) +
		"?mode=rw&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db for writing: %w", err)
	}
	// One writer. SQLite serialises them anyway, and a pool of them would only
	// convert waiting-in-Go into waiting-on-the-file lock. It also makes
	// `exclusive` cheap: the connection it pins is the only one there is.
	db.SetMaxOpenConns(1)
	return db, nil
}

// PingWritable proves the file opens and `users` is there. `sql.Open` only
// records the DSN, so without this a missing `app.db` would be discovered by
// the first insert -- which is exactly where a failure must not be discovered.
func PingWritable(ctx context.Context, db *sql.DB) error {
	var one int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM users LIMIT 1").Scan(&one)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("app.db is not writable: %w", err)
	}
	return nil
}

// inTx is Python's `with con:` -- a deferred transaction, opened on the first
// statement, committed on a clean return and rolled back on anything else.
func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// exclusive is `users._exclusive`: `with con:`, except the write lock is taken
// before the first read.
//
// SQLite in its default mode opens a transaction on the first *DML* statement
// and not on a SELECT, so the obvious spelling -- check inside a transaction,
// then update -- runs the check outside any transaction at all. Two callers
// demoting two different admins would each read two, each write one, and the
// instance would end with none.
//
// BEGIN IMMEDIATE takes the write lock up front, so the second caller waits
// (busy_timeout) and then re-reads a world in which the first has already
// committed. **A rule enforced by a read followed by an unrelated write is not
// enforced.** ADR 17.
//
// It is spelled out on a pinned `*sql.Conn` rather than through `BeginTx`
// because `database/sql` has no way to ask for an immediate transaction, and a
// DSN-wide `_txlock=immediate` would make every write on this handle take the
// lock up front -- including the log's appends, which have nothing to check.
// Pinning is what makes the manual BEGIN safe: the connection cannot be
// handed to another caller mid-transaction.
func exclusive(ctx context.Context, db *sql.DB, fn func(*sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	if err := fn(conn); err != nil {
		if _, rollback := conn.ExecContext(ctx, "ROLLBACK"); rollback != nil {
			return errors.Join(err, fmt.Errorf("the rollback failed: %w", rollback))
		}
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// nowISO is what Python writes into every timestamp column:
// `datetime.now(UTC).isoformat()`, six fractional digits and a `+00:00`
// offset.
//
// One hair of difference, recorded rather than chased, and it is the same one
// `decklog.nowISO` records: `isoformat` omits the fractional part entirely
// when the microsecond is exactly zero, which happens about once in a million
// writes and which both runtimes parse either way.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}

// isoAt is nowISO for a moment that is not now -- an expiry, a window start.
func isoAt(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000-07:00")
}
