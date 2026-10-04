package pooltest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The pool that answers for a while and then stops.
//
// **The fixture this tree already had fails at the *first* query, and that is
// the whole gap.** A schemaless DuckDB file -- `internal/cards` and
// `internal/deckread` both build one, and each argues it in its own words -- is
// a real deploy state and the right fixture for "the pool opened and cannot
// answer": every read fails where it starts. What it cannot reach is anything
// *after* the first statement. [pool.Conn.Columns] iterating a result set to its
// end, [pool.Conn.GetCards] asking whether the iteration failed rather than
// finished, the four probes `Stale` makes one after another -- a healthy pool
// never takes those branches and a schemaless one never gets to them, so
// nothing in this module had ever driven a single one.
//
// [OpenFaulty] is the fixture for those, and it is `authtest.OpenFaulty` one
// database across: a real pool file on a real disk, reached through the real
// driver, with a counter saying how many more statements -- or how many more
// *rows* -- this handle will answer before it refuses every one. Seed nothing,
// arm the budget at the statement the test is about, and the read under test
// runs against a library that worked a moment ago and does not any more.
//
// **It is a wrapper, never a fork.** DuckDB is cgo and its driver is a
// prebuilt library; the fault is a [driver.Connector] and a [driver.Conn]
// forwarding to the real ones and spending a budget on the way past, so what
// answers is DuckDB and what refuses is this file.
//
// That fault is not contrived either. The pool file lives on the same mounted
// volume as everything else (ADR 23), it is opened read-only and held on a
// lease, and a volume that detaches between one query and the next is exactly
// this shape: the first statements land, the rest do not, and what the read
// does with a half-answer is the whole question. The answer must never be a
// short list presented as a whole one -- a deck page that renders "we have
// never heard of any of these cards" over a broken pool is telling somebody
// their decklist is wrong (commandment 2).
//
// **Two limits, named rather than guarded.** The wrapper forwards
// [driver.Conn]'s own calls, the two context forms, and -- load-bearing --
// [driver.NamedValueChecker], without which `?::VARCHAR[]` bound from a Go
// `[]string` is refused by `database/sql`'s default converter and every list
// query in this package breaks. It does *not* forward the optional
// column-type interfaces, so `rows.ColumnTypes()` degrades over this handle;
// nothing in `internal/pool` calls it, and a caller that needs it should
// extend this rather than trust it.

// ErrPoolGoneAway is what a faulty handle answers with once its budget is
// spent.
//
// The wording is this fixture's rather than DuckDB's, deliberately: nothing in
// the app may match on a driver's message, so an error that is obviously not
// DuckDB's proves the assertions above it are about behaviour rather than
// about a string.
var ErrPoolGoneAway = errors.New("pooltest: the card pool went away mid-statement")

// Fault is the knob on a handle from [OpenFaulty]: how many more statements,
// and how many more rows, it will answer.
//
// Safe from any goroutine, because a `*sql.DB` is.
type Fault struct {
	// remaining is the statement budget: negative is unlimited, zero refuses.
	remaining atomic.Int64
	// rows is the same budget for row iteration, counted separately because
	// the two faults answer different questions and a test wants one at a
	// time.
	rows atomic.Int64
}

// After arms the fault: the next n statements answer normally and every one
// after that fails with [ErrPoolGoneAway]. n of zero fails the very next one.
//
// A "statement" is one thing the driver is asked to run -- an exec or a query.
// Preparing does not count, and neither does opening a connection.
func (f *Fault) After(n int) { f.remaining.Store(int64(n)) }

// RowsAfter arms the other half: the next n rows any query on this handle
// hands back are read normally, and the read after that fails with
// [ErrPoolGoneAway] instead of ending the result set.
//
// It exists because `rows.Err()` is a branch nothing else here can reach.
// Every loop over a result set in this package ends by asking whether the
// iteration failed rather than merely finished, and the difference is a
// partial answer presented as a whole one -- which for a card lookup is a
// sentence about somebody's deck that is not true.
//
// **The budget is spent across every query on the handle, not per query**, so
// a read that asks the pool what columns it has before asking for cards has
// already spent one row per column. Sweep a range rather than guessing a
// number: [pool.Conn.GetCards] pays for twenty-eight columns before its own
// first row.
func (f *Fault) RowsAfter(n int) { f.rows.Store(int64(n)) }

// Heal puts the handle back: every statement and every row answers again.
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

// OpenFaulty builds the tiny pool ([Build]) and returns a handle over it
// together with the [Fault] that governs it. The handle starts healthy;
// nothing fails until somebody calls [Fault.After] or [Fault.RowsAfter]. It is
// closed when the test ends.
//
// The handle is **single-connection**, because a budget counted across a pool
// of connections is a budget nobody could predict.
//
// It is opened **read-write**, which is the one place this differs from the app
// -- [pool.Open] takes the served pool read-only. The subject here is the
// fault, and a read-only handle would refuse [pool.SnapshotPrices]' write for a
// reason of its own rather than for the reason under test. Every file is a
// fresh `t.TempDir()` copy, so there is nothing to damage; the app's own
// read-only-ness is proved where it is written.
func OpenFaulty(tb testing.TB) (*sql.DB, *Fault) {
	tb.Helper()
	db, fault, err := faultyHandle(Build(tb))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db, fault
}

// OpenFaultyPool is [OpenFaulty] as a `*pool.Pool`: the tiny pool reached the
// way the *app* reaches the real one — `Use`, a lease, a `*pool.Conn` — with a
// budget saying how many more statements, or rows, the library will answer.
//
// **This is the fixture every reader outside `internal/pool` wanted and could
// not have.** `deckread`, `cards`, `api` and the rest take a `*pool.Conn` they
// got from a `*pool.Pool`, and a Pool used to open its own file; a schemaless
// pool therefore failed them at their *first* statement and nothing could
// reach the arm after it. `pool.NewOver` is the door, `pool.Connect` argues it,
// and this is the only caller.
//
// Two things to know before arming it:
//
//   - **The budget is spent across every statement on the handle**, and a
//     reader's first move is usually `Columns`, which costs one statement and
//     one row per column of the table it asks about. Sweep a range rather than
//     guessing a number.
//   - **A Pool remembers.** `GetCards` and `Columns` are memoised for as long
//     as the file's stamp stands, so a second call inside one test may not
//     reach the database at all. Arm the fault for the call you mean and read
//     [pool.Pool]'s own comment about the memory before being surprised.
//
// Every open the Pool makes — including a re-open after the reaper has handed
// the file back — goes through the same [Fault], so a budget armed now still
// governs a lease taken later.
func OpenFaultyPool(tb testing.TB) (*pool.Pool, *Fault) {
	tb.Helper()
	return FaultyPoolOver(tb, Build(tb))
}

// FaultyPoolOver is [OpenFaultyPool] over a pool file the caller built itself.
//
// **It exists for the budget sweep.** A Pool remembers its columns and its card
// lookups for as long as the file's stamp stands, so a sweep that reused one
// Pool would find its second pass spending no statements on the reads its first
// pass already learned — and every budget after the first would be measuring a
// different sequence. A fresh Pool per budget fixes that, and a fresh *database*
// per budget would make a twenty-step sweep pay for twenty DuckDB files. Build
// once, hand the path in, sweep.
func FaultyPoolOver(tb testing.TB, path string) (*pool.Pool, *Fault) {
	tb.Helper()
	fault := &Fault{}
	fault.Heal()
	p := pool.NewOver(path, nil,
		func(context.Context, string) (*sql.DB, error) {
			return faultyHandleWith(path, fault)
		})
	tb.Cleanup(p.Close)
	return p, fault
}

func faultyHandle(path string) (*sql.DB, *Fault, error) {
	fault := &Fault{}
	fault.Heal()
	db, err := faultyHandleWith(path, fault)
	if err != nil {
		return nil, nil, err
	}
	return db, fault, nil
}

// faultyHandleWith is [faultyHandle] over a [Fault] the caller already holds,
// so that a Pool re-opening its file lands on the same budget rather than on a
// fresh one.
func faultyHandleWith(path string, fault *Fault) (*sql.DB, error) {
	// The driver is borrowed from a handle rather than named: `sql.OpenDB`
	// wants a connector, and taking the registered driver off a throwaway
	// handle avoids registering a second name for what is the same driver.
	plain, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("faulty pool: %w", err)
	}
	inner := plain.Driver()
	if err := plain.Close(); err != nil {
		return nil, fmt.Errorf("faulty pool: %w", err)
	}
	db := sql.OpenDB(&faultyConnector{dsn: path, driver: inner, fault: fault})
	db.SetMaxOpenConns(1)
	return db, nil
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
	// This fixture wraps three calls and nothing else, and the check is made
	// here rather than guarded at every call site. A driver missing one of the
	// context forms would send `database/sql` down the prepared-statement road
	// instead, where nothing counts the budget -- so the fault would quietly
	// stop being injected and every test standing on it would go green for the
	// wrong reason. A driver missing the named-value check is worse than that:
	// `database/sql`'s default converter refuses a Go `[]string`, and every
	// query in this package that binds `?::VARCHAR[]` would fail for a reason
	// that has nothing to do with the test. DuckDB speaks all three; if that
	// ever changes, this fixture is to be rewritten rather than trusted.
	exec, execOK := real.(driver.ExecerContext)
	query, queryOK := real.(driver.QueryerContext)
	check, checkOK := real.(driver.NamedValueChecker)
	if !execOK || !queryOK || !checkOK {
		_ = real.Close()
		return nil, fmt.Errorf("pooltest: %T does not speak the driver calls "+
			"this fixture counts statements through", real)
	}
	return &faultyConn{real: real, exec: exec, query: query, check: check,
		fault: c.fault}, nil
}

func (c *faultyConnector) Driver() driver.Driver { return c.driver }

// faultyConn forwards the two calls that actually run something, spends one of
// the budget on each, and forwards the argument check that makes a list
// parameter bindable.
type faultyConn struct {
	real  driver.Conn
	exec  driver.ExecerContext
	query driver.QueryerContext
	check driver.NamedValueChecker
	fault *Fault
}

// Prepare and Begin are [driver.Conn]'s two required methods and neither is
// ever reached by the reads this fixture exists for: `database/sql` prefers
// the context forms above whenever a connection offers them, and nothing in
// `internal/pool`'s read paths opens a transaction ([pool.LoadOracle] opens
// one, but as a `BEGIN TRANSACTION` exec, which is counted like any other
// statement). They refuse rather than delegate so that a road this fixture
// does not count statements on cannot be taken silently.
func (c *faultyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("pooltest: this fixture prepares nothing; use the context forms")
}

func (c *faultyConn) Begin() (driver.Tx, error) {
	return nil, errors.New("pooltest: this fixture begins nothing; a counted BEGIN is an exec")
}

func (c *faultyConn) Close() error { return c.real.Close() }

// CheckNamedValue forwards to the real driver's own check, unchanged. It is
// what lets `?::VARCHAR[]` take a Go `[]string`, which is how every batched
// card lookup in this package binds its names.
func (c *faultyConn) CheckNamedValue(v *driver.NamedValue) error {
	return c.check.CheckNamedValue(v)
}

func (c *faultyConn) ExecContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Result, error) {
	if !c.fault.allow() {
		return nil, ErrPoolGoneAway
	}
	return c.exec.ExecContext(ctx, query, args)
}

func (c *faultyConn) QueryContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Rows, error) {
	if !c.fault.allow() {
		return nil, ErrPoolGoneAway
	}
	rows, err := c.query.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return &faultyRows{real: rows, fault: c.fault}, nil
}

// faultyRows is the result set under [Fault.RowsAfter]: it hands back the rows
// it was given until the budget runs out, and then fails the iteration rather
// than ending it. Failing is the point -- a result set that simply stopped
// early would be indistinguishable from a short answer, which is the very
// confusion `rows.Err()` exists to settle.
type faultyRows struct {
	real  driver.Rows
	fault *Fault
}

func (r *faultyRows) Columns() []string { return r.real.Columns() }
func (r *faultyRows) Close() error      { return r.real.Close() }

func (r *faultyRows) Next(dest []driver.Value) error {
	if !r.fault.allowRow() {
		return ErrPoolGoneAway
	}
	return r.real.Next(dest)
}
