package auth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// The write side of `app.db`, and the two transaction shapes `mtglab/auth`
// uses.
//
// **`mode=rw`, never `rwc`.** The ladder (`schema.go`) runs once, at the
// serving command's boot, and is the only creator: a file this created would
// be a database at version zero beside the real one, discovered at whatever
// write happened to come first. The same rule `internal/decklog` states, and
// the same one `Open` above it states for the read side.
//
// An absent `app.db` is therefore **read as an empty one**, never created, and
// that is the honest answer rather than a shortcut -- measured on the live
// wire 2026-08-22, on a fresh data directory with `MTGLAB_ADMIN_EMAIL`
// unset: the recorded answer to a first login on such an instance is 401,
// because an empty `users` table has nobody in it. A
// reader cannot tell an empty database from an absent one, so answering as
// if it were empty is the recorded contract, and it leaves the ladder
// where it belongs.

// OpenReadWrite opens `app.db` for the writes the account routes make:
// sessions opened and closed, passwords set, tokens spent, rate-limit windows
// counted. It does not create the file.
//
// Two writers, and it is worth saying which: the serving process (the
// session touch, the expired-row deletes, the account routes) and `mtglab
// users` on the machine. Two processes writing one file is safe here and
// not by luck: the file is in WAL mode (set at creation, and WAL is
// persistent in the file), where a writer blocks readers not at all, and
// the 5000ms busy timeout means two writers collide as a short wait rather
// than as `database is locked`.
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

// A transaction that cannot end cleanly takes its connection with it.
//
// Both shapes below run on a pinned [sql.Conn], and the pin is what makes
// them correct: `exclusive`'s hand-written BEGIN IMMEDIATE cannot be handed
// to another caller mid-transaction, and `inTx`'s COMMIT lands on the
// connection that began it. The pin is also where the failure that used to
// live here hid. When the COMMIT fails -- SQLITE_BUSY past the timeout, a
// full disk, a volume that detached mid-write -- or when a ROLLBACK after a
// failed step fails the same way, the driver's transaction is **still open
// on that connection**, and `database/sql` returns the connection to the
// pool anyway. Every later statement on it then runs inside a transaction
// nobody will ever commit: reads answer perfectly well, writes go nowhere,
// and the next BEGIN IMMEDIATE is refused with "cannot start a transaction
// within a transaction". With one connection in the pool (`OpenReadWrite`)
// that is the whole handle -- a live instance that keeps answering while
// writing nothing, until restart. The test that found it had to roll the
// stale transaction back by hand between cases to stay green
// (`halfwritten_test.go`), which is how it was noticed.
//
// [discard] is the fix, and it is the only honest one: `database/sql`
// closes a connection rather than pooling it when the driver reports
// [driver.ErrBadConn], and [sql.Conn.Raw] is the documented way to say so
// from outside the driver. SQLite rolls back whatever a closing connection
// still held, and the next caller gets a fresh connection with no history.
// A second ROLLBACK would be the cheaper-looking move and is wrong twice:
// it fails for the same reason the first statement did, and a ROLLBACK that
// happens to succeed leaves a connection that just proved unreliable in the
// pool for the next write to find out about.
func discard(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}

// inTx is a deferred transaction -- opened on the first
// statement, committed on a clean return and rolled back on anything else.
func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = conn.Close() }()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rollback := tx.Rollback(); rollback != nil {
			discard(conn)
			return errors.Join(err, fmt.Errorf("the rollback failed: %w", rollback))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		discard(conn)
		return err
	}
	return nil
}

// exclusive is a transaction that takes the write lock
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
			discard(conn)
			return errors.Join(err, fmt.Errorf("the rollback failed: %w", rollback))
		}
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		discard(conn)
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// nowISO is the timestamp every column carries:
// UTC, six fractional digits and a `+00:00`
// offset.
//
// One hair of difference from the recorded rows, noted rather than chased,
// and it is the same one
// `decklog.nowISO` records: the recorded format omits the fractional part
// entirely when the microsecond is exactly zero, which happens about once
// in a million writes and which `ParseTimestamp` reads either way.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}

// isoAt is nowISO for a moment that is not now -- an expiry, a window start.
func isoAt(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000-07:00")
}
