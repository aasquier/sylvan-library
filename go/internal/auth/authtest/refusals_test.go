package authtest

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture's own refusals.
//
// `authtest_test.go` proves the fixture does what it says when it works. This
// file is the other half: every road the fixture refuses to travel, taken, and
// held to its refusal.
//
// It matters more than a fixture's coverage usually does, because the roads
// here are not tidiness. `faultyConn.Prepare` and `Begin` exist to refuse:
// `database/sql` reaches for a connection's prepared-statement path whenever
// the context forms are missing, and nothing on that path spends the budget —
// so a driver that stopped speaking one of the three would make the fault
// quietly stop being injected, and **every test standing on this fixture would
// go green for the wrong reason**. `Connect`'s check is the same guard one
// level up. A guard nothing has ever tripped is a guard nobody knows still
// works.

// A faulty handle over a path with no database at it fails at the connection
// rather than handing back something that looks like a handle.
//
// `OpenFaulty` builds the database first, so this is the road its sibling
// takes: a connector pointed at a file that is not there and `mode=rw`, which
// refuses to create one.
func TestAFaultyHandleOverNothingFailsAtTheConnectionRatherThanLater(t *testing.T) {
	t.Parallel()
	// Building the handle cannot fail -- it is a DSN and a connector, and
	// `faultyHandle`'s own comment argues why it no longer pretends otherwise
	// -- so the refusal has to arrive at the first connection, and that is
	// what this asks for.
	db, _ := faultyHandle(filepath.Join(t.TempDir(), "absent.db"), "rw")
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("a handle over a file that does not exist answered a ping")
	}
}

// `OpenFaulty` cannot build its database, and says so rather than returning a
// fault nobody can arm.
func TestOpenFaultyReportsADatabaseItCouldNotBuild(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, fault, err := OpenFaulty(filepath.Join(blocker, "deeper", "app.db"))
	if err == nil {
		_ = db.Close()
		t.Fatal("a faulty handle was built under a plain file")
	}
	if db != nil || fault != nil {
		t.Error("the refusal came back with a handle or a fault beside it")
	}
}

// The two roads the fixture will not travel, taken directly.
//
// `database/sql` prefers the context forms and `Connect` refuses a driver that
// lacks them, so neither of these is ever called in practice — which is
// exactly why they must refuse rather than delegate. A delegating `Prepare`
// would send statements down a path where nothing counts the budget, and the
// fault would silently stop injecting.
func TestTheFixtureRefusesThePreparedStatementRoadRatherThanTakingItQuietly(t *testing.T) {
	t.Parallel()
	conn := &faultyConn{}

	stmt, err := conn.Prepare("SELECT 1")
	if err == nil {
		t.Error("the fixture prepared a statement on a road it does not count")
	}
	if stmt != nil {
		t.Error("a refused Prepare came back with a statement")
	}
	if !strings.Contains(err.Error(), "prepares nothing") {
		t.Errorf("the refusal does not say why: %v", err)
	}

	tx, err := conn.Begin() //nolint:staticcheck // the deprecated road is the point
	if err == nil {
		t.Error("the fixture began a transaction on a road it does not count")
	}
	if tx != nil {
		t.Error("a refused Begin came back with a transaction")
	}
	if !strings.Contains(err.Error(), "begins nothing") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// halfADriver speaks `driver.Conn` and nothing else: no `ExecerContext`, no
// `QueryerContext`, no `ConnBeginTx`. It is the driver the fixture's own
// comment says must be refused rather than trusted.
type halfADriver struct{}

func (halfADriver) Open(string) (driver.Conn, error) { return halfAConn{}, nil }

type halfAConn struct{}

func (halfAConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (halfAConn) Close() error                        { return nil }
func (halfAConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

// A driver that does not speak all three context forms is refused at the
// connection, by name.
//
// Without this the fixture would hand back a handle whose budget is never
// spent — every refusal test standing on it green, and none of them refusing
// anything. The message names the type so the next person is told which driver
// stopped speaking which form rather than being left to find out that a
// hundred tests have gone vacuous.
func TestADriverThatCannotCountItsStatementsIsRefusedByName(t *testing.T) {
	t.Parallel()
	c := &faultyConnector{dsn: "irrelevant", driver: halfADriver{}, fault: &Fault{}}

	conn, err := c.Connect(context.Background())
	if err == nil {
		_ = conn.Close()
		t.Fatal("a driver with no context forms was accepted, and every budget " +
			"standing on this fixture would be a budget nothing spends")
	}
	if !strings.Contains(err.Error(), "halfAConn") {
		t.Errorf("the refusal does not name the driver: %v", err)
	}
	if c.Driver() == nil {
		t.Error("the connector does not report the driver it wraps")
	}
}

// A statement the budget allowed and the database refused comes back as the
// database's own error, not as the fixture's.
//
// The distinction is the whole reason [ErrGoneAway]'s wording is obviously not
// SQLite's: a test that cannot tell "the fault fired" from "the SQL was wrong"
// is a test that passes on a typo.
func TestAStatementTheBudgetAllowedFailsWithTheDatabasesOwnError(t *testing.T) {
	t.Parallel()
	db, fault, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fault.Heal()

	ctx := context.Background()
	if _, err := db.QueryContext(ctx, "SELECT * FROM no_such_table"); err == nil {
		t.Fatal("a query against a table that is not there succeeded")
	} else if strings.Contains(err.Error(), "went away mid-statement") {
		t.Errorf("a SQL error was reported as the fixture's fault: %v", err)
	}
	if _, err := db.ExecContext(ctx, "NOT SQL AT ALL"); err == nil {
		t.Fatal("nonsense was executed")
	} else if strings.Contains(err.Error(), "went away mid-statement") {
		t.Errorf("a SQL error was reported as the fixture's fault: %v", err)
	}
	// And the handle is still the fixture's: the budget still bites.
	fault.After(0)
	var one int
	if err := db.QueryRowContext(ctx,
		"SELECT 1").Scan(&one); err == nil || !strings.Contains(
		err.Error(), "went away mid-statement") {
		t.Errorf("the budget stopped biting after a SQL error: %v", err)
	}
	fault.Heal()
	var alive int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&alive); err != nil {
		t.Errorf("the healed handle does not answer: %v", err)
	}
	if alive != 1 {
		t.Errorf("the healed handle answered %d", alive)
	}
}
