package wheel_test

// The two things the wheel owes a card pool that answers the first question and
// not the second.
//
// A spin is two statements with a decision between them: count the legal cards
// in the deck's colours that answer to the fate, then fetch the one the seed
// landed on. `exclusions_test.go` covers the spin that is abandoned before the
// first of those -- a cancelled context, and the count never comes back. What
// nothing had driven is the gap *between* them, and the gap has two faults in
// it:
//
//   - the library stops answering after the count, and the spin must come back
//     as a failure rather than as "the pool holds no legal card in these
//     colours", which is a sentence about somebody's deck and would be a lie
//     told because a volume detached (ADR 23);
//   - the count says there are cards and the fetch hands back none, which is
//     the one state where `picked[0]` would be an index into an empty list --
//     a panic on a novelty page, where the honest answer is that the card
//     slipped off the wheel.
//
// Both are reached rather than argued: the first with `pooltest.FaultyPoolOver`,
// the second with a pool whose rows go away between the two statements.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wheel"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// monoGreen is the fixture deck both tests below spin for, and the one the
// frozen corpus uses: mono-green, so `{G}` is an identity the fixture pool has
// legal cards in and the count is above zero.
func monoGreen(t *testing.T) *deck.Deck {
	t.Helper()
	fx := loadSpins(t)
	d, err := deck.FromText(fx.Decks["mono-green"], "mono-green")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// reasonOf reads the wheel's own sentence out of a spin, or "" when it handed a
// card over.
func reasonOf(t *testing.T, spun wire.OrderedMap) string {
	t.Helper()
	raw, err := wire.MarshalOrdered(spun)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Reason
}

// A spin either hands over a card or comes back as a failure, at every statement
// the pool can stop answering.
//
// The wheel's own empty answer -- "the pool holds no legal card in these colours
// that answers to this fate" -- is a true and useful sentence when the count
// really is zero, and a lie about somebody's colours when the library has gone
// away. The two must never be confused, which is why the fetch's refusal is an
// error rather than a second route to that sentence.
func TestASpinRefusesAtEveryStatementTheCardPoolCanStopAnswering(t *testing.T) {
	t.Parallel()
	d := monoGreen(t)
	path := pooltest.Build(t)
	ctx := context.Background()

	// Above the longest a spin is: the count, the columns read the fetch makes
	// first, and the fetch itself.
	const widest = 6
	refused, handed := 0, 0
	for budget := 0; budget <= widest; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		var spun wire.OrderedMap
		err := p.Use(ctx, func(c *pool.Conn) error {
			got, e := wheel.Spin(ctx, d, map[string]bool{"G": true}, c, big.NewInt(7))
			spun = got
			return e
		})
		fault.Heal()
		p.Close()
		if err != nil {
			if spun != nil {
				t.Errorf("at budget %d the spin refused and handed back a payload "+
					"beside the refusal: %v", budget, spun)
			}
			refused++
			continue
		}
		if name := cardNameOf(t, spun); name == "" {
			t.Errorf("at budget %d the wheel said it had nothing to hand over, "+
				"which is a sentence about this deck's colours and not about a "+
				"library that stopped answering: %q", budget, reasonOf(t, spun))
		}
		handed++
	}
	// Both halves have to happen or the sweep measured one state seven times.
	if refused == 0 || handed == 0 {
		t.Errorf("%d budgets refused and %d handed a card over -- the sweep never "+
			"crossed from one to the other", refused, handed)
	}
}

// The count says there are cards and the fetch finds none.
//
// It is the one state where the wheel holds a number it can no longer trust: the
// seeded offset was drawn against a count, and the list the offset indexes into
// came back empty. Without the guard `picked[0]` is an index into nothing, so the
// failure is not a wrong answer but a panic -- on the one page in this app whose
// whole job is to be a bit of fun.
//
// Reached with a pool whose rows go away between the two statements. The fixture
// is a connector of this test's own (`pool.NewOver` is the door, and
// `pool.Connect` argues it) that empties `oracle_cards` before one chosen
// statement; the position is swept rather than counted, because the number of
// statements a fetch makes is not this test's business and changes the day the
// pool learns another column.
func TestTheWheelSaysTheCardSlippedWhenTheCountAndTheFetchDisagree(t *testing.T) {
	t.Parallel()
	d := monoGreen(t)
	ctx := context.Background()

	slipped := 0
	for at := 1; at <= 6; at++ {
		// One pool file per position: DuckDB keys a loaded database on its path
		// and refuses a second handle on one it already holds, and this fixture
		// deletes rows, so no two positions may share a file.
		path := pooltest.Build(t)
		p := pool.NewOver(path, nil, vanishingConnect(t, path, at))
		var spun wire.OrderedMap
		err := p.Use(ctx, func(c *pool.Conn) error {
			got, e := wheel.Spin(ctx, d, map[string]bool{"G": true}, c, big.NewInt(7))
			spun = got
			return e
		})
		p.Close()
		if err != nil {
			// Emptying the table before the count's own statement, or before the
			// columns read, is a different fault and not this test's subject.
			continue
		}
		if name := cardNameOf(t, spun); name != "" {
			// The rows vanished after the fetch had already read them.
			continue
		}
		reason := reasonOf(t, spun)
		if reason == "" {
			t.Fatalf("at position %d the wheel handed back neither a card, a "+
				"reason, nor an error: %v", at, spun)
		}
		if strings.Contains(reason, "slipped") {
			slipped++
		}
	}
	if slipped == 0 {
		t.Error("no position emptied the pool between the count and the fetch, so " +
			"the one state where the seeded offset indexes into nothing was never " +
			"driven -- the guard that keeps it from being a panic is unproven")
	}
}

// vanishingConnect is a [pool.Connect] whose handle empties `oracle_cards` just
// before its `at`th statement, and answers everything else from the real driver.
//
// It is the smallest fixture that can put a change *between* two statements of
// one read: a DuckDB handle gives each statement its own view of the data, so a
// count and a fetch are two questions about two moments. The file is the test's
// own temp copy and the handle is read-write, as `pooltest.OpenFaulty`'s is and
// for the same reason: the subject is the disagreement, not the app's
// read-only-ness, which is proved where it is written.
func vanishingConnect(t *testing.T, path string, at int) pool.Connect {
	t.Helper()
	return func(context.Context, string) (*sql.DB, error) {
		plain, err := sql.Open("duckdb", path)
		if err != nil {
			return nil, err
		}
		inner := plain.Driver()
		if err := plain.Close(); err != nil {
			return nil, err
		}
		db := sql.OpenDB(&vanishingConnector{dsn: path, driver: inner, at: at})
		// Single connection, because a position counted across a pool of them is
		// a position nobody could predict.
		db.SetMaxOpenConns(1)
		return db, nil
	}
}

type vanishingConnector struct {
	dsn    string
	driver driver.Driver
	at     int
	seen   atomic.Int64
	gone   atomic.Bool
}

func (c *vanishingConnector) Driver() driver.Driver { return c.driver }

func (c *vanishingConnector) Connect(context.Context) (driver.Conn, error) {
	real, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	// The same three calls `pooltest`'s own fixture forwards, checked here for
	// the same reason it checks them: a driver missing a context form would send
	// `database/sql` down the prepared-statement road where nothing counts, so
	// the fixture would quietly stop injecting anything and this test would go
	// green for the wrong reason.
	exec, execOK := real.(driver.ExecerContext)
	query, queryOK := real.(driver.QueryerContext)
	check, checkOK := real.(driver.NamedValueChecker)
	if !execOK || !queryOK || !checkOK {
		_ = real.Close()
		return nil, fmt.Errorf("the vanishing fixture cannot count statements "+
			"through %T", real)
	}
	return &vanishingConn{real: real, exec: exec, query: query, check: check,
		owner: c}, nil
}

type vanishingConn struct {
	real  driver.Conn
	exec  driver.ExecerContext
	query driver.QueryerContext
	check driver.NamedValueChecker
	owner *vanishingConnector
}

// Prepare and Begin refuse rather than delegate, so a road this fixture does not
// count statements on cannot be taken silently.
func (c *vanishingConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("the vanishing fixture prepares nothing; use the context forms")
}

func (c *vanishingConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("the vanishing fixture begins nothing")
}

func (c *vanishingConn) Close() error { return c.real.Close() }

func (c *vanishingConn) CheckNamedValue(v *driver.NamedValue) error {
	return c.check.CheckNamedValue(v)
}

// step is called before each counted statement. When the count reaches the
// position this fixture was armed at, the table is emptied first -- once.
func (c *vanishingConn) step(ctx context.Context) {
	if int(c.owner.seen.Add(1)) != c.owner.at || c.owner.gone.Swap(true) {
		return
	}
	// Not counted: the deletion is the fixture's own move, not one of the reads
	// whose order is being swept.
	_, _ = c.exec.ExecContext(ctx, "DELETE FROM oracle_cards", nil)
}

func (c *vanishingConn) ExecContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Result, error) {
	c.step(ctx)
	return c.exec.ExecContext(ctx, query, args)
}

func (c *vanishingConn) QueryContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Rows, error) {
	c.step(ctx)
	return c.query.QueryContext(ctx, query, args)
}
