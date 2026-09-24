package authtest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
)

// The volume that goes away mid-transaction.
//
// A closed handle is this module's oldest coverage lever and it answers one
// question: what happens when the *first* statement of a function fails.
// Almost every write here is longer than that — begin, check, update, commit —
// and the branches between the first statement and the last had no fixture at
// all: a closed handle never reaches them, and a healthy database never takes
// them. They are the branches where a half-finished write decides whether to
// roll back, whether to say so, and whether to claim it did the work.
//
// [OpenFaulty] is the fixture for those. It is a real, fully-migrated `app.db`
// on a real disk, reached through a real driver, with one thing added: a
// counter that says how many more statements this handle will answer before it
// starts refusing every one. Seed the rows a test needs with the counter off,
// arm it at the statement the test is about, and the function under test runs
// against a database that worked a moment ago and does not any more.
//
// That is not a contrived fault. `app.db` lives on a network volume (ADR 23's
// deployment), and a volume that detaches between a transaction's BEGIN and
// its COMMIT is the exact shape of this: the first statements land, the rest
// do not, and what the code does next is the whole question.

// ErrGoneAway is what a faulty handle answers with once its budget is spent.
// The wording is a test fixture's rather than SQLite's, and deliberately so:
// nothing in the app may match on a driver's message, so a fixture whose
// message is obviously not SQLite's proves the assertions above it are about
// behaviour rather than about a string.
var ErrGoneAway = errors.New("authtest: the database went away mid-statement")

// Fault is the knob on a handle from [OpenFaulty]: how many more statements
// it will answer.
//
// Safe from any goroutine, because the handle under it is — the runner's
// waiters and its ticker both write through one.
type Fault struct {
	// remaining is the budget: negative is unlimited, zero refuses.
	remaining atomic.Int64
	// rows is the same budget for row iteration, counted separately because
	// the two faults answer different questions and a test wants one at a
	// time.
	rows atomic.Int64
}

// After arms the fault: the next n statements answer normally and every one
// after that fails with [ErrGoneAway]. n of zero fails the very next one.
//
// A "statement" is one thing the driver is asked to run — an exec, a query,
// or the BEGIN that opens a transaction. Preparing does not count; running
// the prepared statement does.
func (f *Fault) After(n int) { f.remaining.Store(int64(n)) }

// RowsAfter arms the other half of the fault: the next n rows any query on
// this handle hands back are read normally, and the read after that fails
// with [ErrGoneAway] instead of ending the result set.
//
// It exists because `rows.Err()` is a branch nothing else can reach. Every
// loop over a result set here ends by asking whether the iteration itself
// failed rather than merely finished, and the difference is a partial answer
// presented as a whole one — the worst shape a read can fail in, and the one
// a healthy database and a closed handle both hide.
func (f *Fault) RowsAfter(n int) { f.rows.Store(int64(n)) }

// Heal puts the handle back: every statement and every row answers again. For
// the seeding a test does after it has proven the refusal.
func (f *Fault) Heal() {
	f.remaining.Store(-1)
	f.rows.Store(-1)
}

// allow spends one statement of the budget, reporting whether it may run.
func (f *Fault) allow() bool { return spend(&f.remaining) }

// allowRow is allow for one step of a result set.
func (f *Fault) allowRow() bool { return spend(&f.rows) }

func spend(budget *atomic.Int64) bool {
	for {
		left := budget.Load()
		if left < 0 {
			return true
		}
		if left == 0 {
			return false
		}
		if budget.CompareAndSwap(left, left-1) {
			return true
		}
	}
}

// OpenFaulty builds a scratch `app.db` at path (the recorded schema, as
// [NewScratchDB] writes it) and returns a read-write handle over it together
// with the [Fault] that governs it. The handle starts healthy; nothing fails
// until somebody calls [Fault.After].
//
// The handle is single-connection, like the app's own write side: a budget
// counted across a pool of connections would be a budget nobody could predict.
// Close it when the test is done.
func OpenFaulty(path string) (*sql.DB, *Fault, error) {
	if err := NewScratchDB(path); err != nil {
		return nil, nil, err
	}
	return faultyHandle(path, "rw")
}

// OpenFaultyEmpty is [OpenFaulty] over a file with nothing in it — no schema,
// version zero, created here.
//
// It exists for the one caller that cannot be handed a finished database: the
// schema ladder's own tests. Everything else in this module runs *on* the
// recorded schema, and the ladder is what produces it, so a fixture that
// applied the schema first would leave the ladder with nothing to climb.
func OpenFaultyEmpty(path string) (*sql.DB, *Fault, error) {
	return faultyHandle(path, "rwc")
}

func faultyHandle(path, mode string) (*sql.DB, *Fault, error) {
	dsn := "file:" + path + "?mode=" + mode +
		"&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	// The driver is borrowed from a handle rather than named: `sql.OpenDB`
	// wants a connector, and taking the registered driver off a throwaway
	// handle avoids registering a second name for what is the same driver.
	plain, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("faulty app.db: %w", err)
	}
	inner := plain.Driver()
	if err := plain.Close(); err != nil {
		return nil, nil, fmt.Errorf("faulty app.db: %w", err)
	}
	fault := &Fault{}
	fault.Heal()
	db := sql.OpenDB(&faultyConnector{dsn: dsn, driver: inner, fault: fault})
	db.SetMaxOpenConns(1)
	return db, fault, nil
}

type faultyConnector struct {
	dsn    string
	driver driver.Driver
	fault  *Fault
}

func (c *faultyConnector) Connect(context.Context) (driver.Conn, error) {
	real, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	// This fixture wraps the context forms and nothing else, and the check is
	// made here rather than guarded at every call. A driver that lacked one of
	// them would send `database/sql` down the prepared-statement road instead,
	// where nothing counts the budget -- so the fault would quietly stop being
	// injected and every test standing on it would go green for the wrong
	// reason. The app has exactly one SQL driver and it speaks all three; if
	// that ever changes, this fixture is to be rewritten rather than trusted.
	exec, execOK := real.(driver.ExecerContext)
	query, queryOK := real.(driver.QueryerContext)
	begin, beginOK := real.(driver.ConnBeginTx)
	if !execOK || !queryOK || !beginOK {
		_ = real.Close()
		return nil, fmt.Errorf("authtest: %T does not speak the context forms "+
			"this fixture counts statements through", real)
	}
	return &faultyConn{real: real, exec: exec, query: query, begin: begin,
		fault: c.fault}, nil
}

func (c *faultyConnector) Driver() driver.Driver { return c.driver }

// faultyConn forwards the three calls that actually run something and spends
// one of the budget on each.
type faultyConn struct {
	real  driver.Conn
	exec  driver.ExecerContext
	query driver.QueryerContext
	begin driver.ConnBeginTx
	fault *Fault
}

// Prepare and Begin are [driver.Conn]'s two required methods and neither is
// ever called: `database/sql` prefers the context forms below whenever a
// connection offers them, and [faultyConnector.Connect] refuses a driver that
// does not. They refuse rather than delegate so that a road this fixture does
// not count statements on cannot be taken silently.
func (c *faultyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("authtest: this fixture prepares nothing; use the context forms")
}

func (c *faultyConn) Begin() (driver.Tx, error) {
	return nil, errors.New("authtest: this fixture begins nothing; use BeginTx")
}

func (c *faultyConn) Close() error { return c.real.Close() }

func (c *faultyConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if !c.fault.allow() {
		return nil, ErrGoneAway
	}
	tx, err := c.begin.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &faultyTx{real: tx, fault: c.fault}, nil
}

func (c *faultyConn) ExecContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Result, error) {
	if !c.fault.allow() {
		return nil, ErrGoneAway
	}
	return c.exec.ExecContext(ctx, query, args)
}

func (c *faultyConn) QueryContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Rows, error) {
	if !c.fault.allow() {
		return nil, ErrGoneAway
	}
	rows, err := c.query.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return &faultyRows{real: rows, fault: c.fault}, nil
}

// faultyTx spends the budget on the commit and never on the rollback. Both
// halves of that are deliberate: a commit is the statement whose failure the
// interesting branches are about, and a rollback is what a `defer` runs on
// the way out of every one of them -- counting it would make the budget mean
// something different depending on which branch was taken.
type faultyTx struct {
	real  driver.Tx
	fault *Fault
}

func (t *faultyTx) Commit() error {
	if !t.fault.allow() {
		// The transaction is still open on the connection, and this handle
		// is one connection wide: abandon it, or every later statement in
		// the test runs inside a transaction nobody will ever close.
		_ = t.real.Rollback()
		return ErrGoneAway
	}
	return t.real.Commit()
}

func (t *faultyTx) Rollback() error { return t.real.Rollback() }

// faultyRows is the result set under [Fault.RowsAfter]: it hands back the
// rows it was given until the budget runs out, and then fails the iteration
// rather than ending it. Failing is the point — a result set that simply
// stopped early would be indistinguishable from a short answer, which is the
// very confusion `rows.Err()` exists to settle.
type faultyRows struct {
	real  driver.Rows
	fault *Fault
}

func (r *faultyRows) Columns() []string { return r.real.Columns() }
func (r *faultyRows) Close() error      { return r.real.Close() }

func (r *faultyRows) Next(dest []driver.Value) error {
	if !r.fault.allowRow() {
		return ErrGoneAway
	}
	return r.real.Next(dest)
}
