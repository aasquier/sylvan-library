package pooltest

import (
	"context"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// The faulty pool's own guards.
//
// **A fixture whose fault stops being injected passes every test standing on
// it.** That is the whole reason these exist: three of the four things checked
// here are refusals nothing in the suite ever trips, and each of them is the
// difference between a sweep that proves a read refuses a half-answer and a
// sweep that quietly ran against a healthy database and called it green.

// A list parameter binds. This is the load-bearing one and it looks like
// nothing: `database/sql` hands a Go `[]string` to the driver only if the
// connection speaks [driver.NamedValueChecker], and every batched card lookup
// in `internal/pool` binds `?::VARCHAR[]` that way. A wrapper that forgot to
// forward the check would refuse the argument, every faulty-pool test would
// fail for a reason of its own, and the honest-looking fix would be to stop
// binding lists in the tests.
func TestTheFaultyHandleStillBindsAListParameter(t *testing.T) {
	t.Parallel()
	db, fault := OpenFaulty(t)
	fault.Heal()
	var n int64
	err := db.QueryRowContext(context.Background(),
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[]))
		 SELECT count(*) FROM oracle_cards WHERE lower(name) IN (SELECT w FROM wanted)`,
		[]string{"sol ring", "black lotus"}).Scan(&n)
	if err != nil {
		t.Fatalf("a list parameter was refused by the faulty handle: %v", err)
	}
	if n != 2 {
		t.Errorf("the fixture answered %d of the two cards asked for", n)
	}
}

// Spending the budget refuses, healing restores, and the error is this
// fixture's own rather than DuckDB's.
func TestTheBudgetRefusesAndHealsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault := OpenFaulty(t)

	fault.After(0)
	if _, err := db.ExecContext(ctx, "SELECT 1"); !errors.Is(err, ErrPoolGoneAway) {
		t.Fatalf("a spent budget answered %v", err)
	}
	fault.Heal()
	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("a healed handle still refused: %v", err)
	}

	// The row half, separately: a result set that dies mid-iteration must fail
	// the iteration rather than end it, or a short answer is indistinguishable
	// from a short table.
	fault.RowsAfter(0)
	rows, err := db.QueryContext(ctx, "SELECT name FROM oracle_cards")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Error("a result set with no row budget handed a row over")
	}
	if !errors.Is(rows.Err(), ErrPoolGoneAway) {
		t.Errorf("an iteration that ran out of budget ended with %v rather "+
			"than failing", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	fault.Heal()
}

// A query that is simply wrong comes back as DuckDB's own complaint, not as
// this fixture's. The budget is untouched, so nothing about the error says
// "the pool went away" — which matters because every sweep standing on this
// handle reads a refusal as evidence of the fault it injected.
func TestARealSQLErrorIsNotTheFixturesFault(t *testing.T) {
	t.Parallel()
	db, fault := OpenFaulty(t)
	fault.Heal()
	_, err := db.QueryContext(context.Background(), "SELECT no_such_column FROM oracle_cards")
	if err == nil {
		t.Fatal("a column that does not exist was queried successfully")
	}
	if errors.Is(err, ErrPoolGoneAway) {
		t.Errorf("a SQL error was dressed as the injected fault: %v", err)
	}
}

// A path DuckDB cannot open at all is refused where it is opened, and the
// refusal names the fixture so nobody reads it as the pool being broken.
// `sql.Open("duckdb", …)` fails eagerly on a directory that is not there,
// which is what makes this reachable at all.
func TestAHandleOverAPathThatCannotBeOpenedIsRefused(t *testing.T) {
	t.Parallel()
	db, fault, err := faultyHandle(filepath.Join(t.TempDir(), "no-such-dir", "x.duckdb"))
	if err == nil {
		_ = db.Close()
		t.Fatal("a pool under a directory that does not exist was opened")
	}
	if fault != nil || db != nil {
		t.Error("a refused open handed back a handle or a fault to arm")
	}
	if !strings.Contains(err.Error(), "faulty pool") {
		t.Errorf("the refusal does not name the fixture: %v", err)
	}
}

// The two roads this fixture does not count statements on refuse rather than
// delegate. `database/sql` prefers the context forms whenever a connection
// offers them, so neither is reached in practice — and that is exactly why they
// must not silently work: a change that sent a query down the
// prepared-statement road would stop spending the budget, and every sweep would
// go green against a healthy pool.
func TestTheUncountedRoadsRefuse(t *testing.T) {
	t.Parallel()
	c := &faultyConn{fault: &Fault{}}
	if _, err := c.Prepare("SELECT 1"); err == nil {
		t.Error("the fixture prepared a statement it does not count")
	}
	if _, err := c.Begin(); err == nil {
		t.Error("the fixture opened a transaction it does not count")
	}
}

// A driver that does not speak all three of the calls this fixture forwards is
// refused at connect time, by name. Without this the budget would stop being
// spent — or a list parameter would stop binding — with nothing saying so.
func TestAConnectorRefusesADriverItCannotCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		halfA halfADriver
	}{
		{"one that cannot exec", halfADriver{query: true, check: true}},
		{"one that cannot query", halfADriver{exec: true, check: true}},
		{"one that cannot check a named value", halfADriver{exec: true, query: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &faultyConnector{driver: oneConnDriver{tc.halfA}, fault: &Fault{}}
			_, err := c.Connect(context.Background())
			if err == nil {
				t.Fatal("a driver this fixture cannot count statements " +
					"through was accepted")
			}
			if !strings.Contains(err.Error(), "does not speak") {
				t.Errorf("the refusal does not say what is wrong: %v", err)
			}
		})
	}
}

// A driver that refuses to open at all is passed straight back, unwrapped: a
// handle that never opened is not a handle with a spent budget, and dressing the
// first as the second would hide a missing pool file behind this fixture's own
// sentence.
func TestAConnectorPassesOnADriverThatWillNotOpen(t *testing.T) {
	t.Parallel()
	refusal := errors.New("pooltest: no pool file here")
	c := &faultyConnector{driver: refusingDriver{refusal}, fault: &Fault{}}
	if _, err := c.Connect(context.Background()); !errors.Is(err, refusal) {
		t.Errorf("the connector answered %v rather than the driver's own refusal", err)
	}
}

// The connector hands back the driver it was built over, which is what
// `database/sql` asks it for when a caller wants `db.Driver()`.
func TestTheConnectorNamesItsDriver(t *testing.T) {
	t.Parallel()
	d := oneConnDriver{halfADriver{}}
	c := &faultyConnector{driver: d, fault: &Fault{}}
	if c.Driver() != driver.Driver(d) {
		t.Error("the connector answered with a driver it was not built over")
	}
}

// halfADriver is a connection that speaks whichever of the three forwarded
// calls the test switched on and no others. The flags become real method sets
// through [wrapped]: a Go type cannot gain a method conditionally, so each
// switched-on call is one embedding layer.
type halfADriver struct {
	exec, query, check bool
}

func (halfADriver) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("pooltest: half a driver prepares nothing")
}
func (halfADriver) Close() error { return nil }
func (halfADriver) Begin() (driver.Tx, error) {
	return nil, errors.New("pooltest: half a driver begins nothing")
}

func wrapped(h halfADriver) driver.Conn {
	var conn driver.Conn = h
	if h.exec {
		conn = withExec{conn}
	}
	if h.query {
		conn = withQuery{conn}
	}
	if h.check {
		conn = withCheck{conn}
	}
	return conn
}

type withExec struct{ driver.Conn }

func (withExec) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errors.New("pooltest: half a driver execs nothing")
}

type withQuery struct{ driver.Conn }

func (withQuery) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, errors.New("pooltest: half a driver queries nothing")
}

type withCheck struct{ driver.Conn }

func (withCheck) CheckNamedValue(*driver.NamedValue) error { return nil }

// oneConnDriver always hands back the one connection it was built with.
type oneConnDriver struct{ half halfADriver }

func (d oneConnDriver) Open(string) (driver.Conn, error) { return wrapped(d.half), nil }

// refusingDriver never opens.
type refusingDriver struct{ err error }

func (d refusingDriver) Open(string) (driver.Conn, error) { return nil, d.err }
