package pool_test

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The door a Pool opens its file through, and both sides of it.
//
// `pool.Connect` argues why the door exists: every reader in this tree reaches
// the pool through a `*Pool` rather than a handle, and a Pool opened its own
// file — so `pooltest.OpenFaulty`, which is a real library behind a connector
// that refuses after a budget, could not be given to any of them. Ten refusals
// in the commander dossier alone had never been entered.
//
// **A seam whose default no test runs is a new uncovered line**, so both arms
// are driven here: the app's arm, which is [pool.Open] read-only with a handful
// of connections, and the handed-in arm. The default is asserted by what it
// *does* rather than by which function it called — a read-only handle refuses a
// write, which is the property the app actually depends on (a running refresh
// wants DuckDB's writer lock and must not be shut out by the door holding it).

// The default: nothing is handed in, the pool opens its own file read-only, and
// it answers.
func TestAPoolWithNoConnectorOpensItsOwnFileReadOnly(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)

	var name string
	var writeErr error
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		found, err := c.GetCards(t.Context(), []string{"Sol Ring"})
		if err != nil {
			return err
		}
		if rec := found["Sol Ring"]; rec != nil {
			name = rec.Name
		}
		// The property the default buys: this handle cannot write. A pool held
		// read-write would be a pool a refresh could not take the lock on.
		_, writeErr = c.DB().ExecContext(t.Context(),
			"DELETE FROM oracle_cards WHERE name = 'Sol Ring'")
		return nil
	}); err != nil {
		t.Fatalf("leasing a pool with no connector: %v", err)
	}
	if name != "Sol Ring" {
		t.Errorf("the default open answered %q for Sol Ring", name)
	}
	if writeErr == nil {
		t.Error("the default open accepted a write -- the served pool is " +
			"opened read-only so that a refresh can take the writer lock")
	}
}

// A connector handed in is the one that is used, and it is handed the pool's own
// path rather than one of its own.
func TestAPoolHandedAConnectorUsesItAndTellsItThePath(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	asked := make(chan string, 4)
	p := pool.NewOver(path, nil, func(ctx context.Context, p string) (*sql.DB, error) {
		asked <- p
		return pool.Open(ctx, p)
	})
	t.Cleanup(p.Close)

	var name string
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		found, err := c.GetCards(t.Context(), []string{"Sol Ring"})
		if err != nil {
			return err
		}
		if rec := found["Sol Ring"]; rec != nil {
			name = rec.Name
		}
		return nil
	}); err != nil {
		t.Fatalf("leasing a pool over a handed-in connector: %v", err)
	}
	if name != "Sol Ring" {
		t.Errorf("a handed-in connector answered %q for Sol Ring", name)
	}
	select {
	case got := <-asked:
		if got != path {
			t.Errorf("the connector was asked for %q, want the pool's own %q", got, path)
		}
	default:
		t.Error("the pool opened its file without asking the connector it was given")
	}
}

// A connector that refuses degrades the pool rather than failing the request,
// which is the same answer a missing file gets and the one every read path in
// the app already copes with (ADR 6).
func TestAPoolWhoseConnectorRefusesDegradesLikeAnAbsentPool(t *testing.T) {
	t.Parallel()
	refused := errors.New("the shelf is not there")
	p := pool.NewOver(pooltest.Build(t), nil,
		func(context.Context, string) (*sql.DB, error) { return nil, refused })
	t.Cleanup(p.Close)

	err := p.Use(t.Context(), func(*pool.Conn) error {
		t.Error("the lease was handed out over a connector that refused")
		return nil
	})
	if !errors.Is(err, pool.ErrNoPool) {
		t.Errorf("a refused open answered %v, want the absent-pool answer", err)
	}
	if p.Held() {
		t.Error("a refused open left the pool holding something")
	}
}

// The faulty pool reaches a reader as a `*pool.Conn`, which is the whole point
// of the seam: a library that worked a moment ago and does not any more.
func TestTheFaultyPoolStopsAnsweringThroughTheLease(t *testing.T) {
	t.Parallel()
	// Healthy first, so the refusal below is the budget's doing. The budget is
	// left alone here: what is being proved is that the pool answers at all
	// through the seam.
	p, _ := pooltest.OpenFaultyPool(t)

	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		_, err := c.GetCards(t.Context(), []string{"Sol Ring"})
		return err
	}); err != nil {
		t.Fatalf("the healthy faulty pool: %v", err)
	}

	// A second Pool over the same file, because the first one now remembers
	// both the columns and the lookup.
	armed, fault := pooltest.FaultyPoolOver(t, pooltest.Build(t))
	fault.After(0)
	err := armed.Use(t.Context(), func(c *pool.Conn) error {
		_, e := c.GetCards(t.Context(), []string{"Sol Ring"})
		return e
	})
	fault.Heal()
	if err == nil {
		t.Error("a pool whose next statement refuses answered a card lookup")
	}
	if errors.Is(err, pool.ErrNoPool) {
		t.Error("a pool that opened and then refused a query was reported as an " +
			"absent pool -- that reads as a machine with no library at all")
	}
}

// A file that is not a database is refused at the open rather than handed back
// as a handle nobody can use.
//
// The app leans on that: `Pool.acquire` treats a refused open as an absent pool
// and answers in degraded form, which it could not do if a corrupt file opened
// "successfully" and then failed one request at a time.
//
// **Which step says so is the driver's business and this does not assert it.**
// `Open` pings after `sql.Open` for the usual reason — a DSN parse is not a
// connection — and this driver turns out to connect eagerly at `sql.Open`
// (`pooltest`'s own refusal arrives from there too), so the ping is never the
// first failure. The contract is the refusal, not the line it comes from.
func TestOpeningSomethingThatIsNotAPoolFailsAtOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "not-a-pool.duckdb")
	if err := os.WriteFile(path, []byte("this is a shopping list\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := pool.Open(t.Context(), path)
	if err == nil {
		_ = db.Close()
		t.Fatal("a shopping list opened as a card pool")
	}
	if db != nil {
		t.Error("the refusal came back with a handle beside it")
	}
}

// A refresh that finds the door shut and then finds it open takes the file,
// rather than waiting out the rest of its budget.
//
// `writerlock_test.go` holds the two ends of this — the door that never opens,
// and the door that was never shut. The middle is the ordinary case on the
// instance: the serving process lets go of its lease about ten seconds after
// the last page, and a refresh started during a page load should be standing
// there when it does.
//
// The holder is released **from the wait's own announcement**, which is called
// exactly once and only after an open has already been refused. A timer would
// be a race between this test and the machine.
func TestARefreshTakesTheFileTheMomentTheHolderLetsGo(t *testing.T) {
	t.Parallel()
	path, release := heldUntilReleased(t)

	said := 0
	start := time.Now()
	db, err := pool.OpenWriterWaiting(t.Context(), path, 30*time.Second, func() {
		said++
		release()
	})
	if err != nil {
		t.Fatalf("the writer never got the file the holder let go of: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("closing the writer: %v", err)
	}
	if said != 1 {
		t.Errorf("the wait was announced %d times, want exactly once -- the "+
			"door has to have been found shut first or this test proves nothing",
			said)
	}
	// It took the file rather than sitting out its budget.
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("the writer waited %s for a door that opened at once", took)
	}
}

// heldUntilReleased is `writerlock_test.go`'s second process with a handle on
// it: the pool file held read-only by a child, and the function that lets it go.
//
// A copy rather than a shared helper because the other one's child is torn down
// by its own cleanup and nothing else may reach it; what this test needs is the
// release as a value it can call at a moment of its choosing.
func heldUntilReleased(t *testing.T) (string, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "handover.duckdb")
	built, err := os.ReadFile(pooltest.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, built, 0o600); err != nil {
		t.Fatal(err)
	}
	held := exec.Command(os.Args[0], "-test.run", "TestAHolderIsASecondProcess")
	held.Env = append(os.Environ(), helperEnv+"="+path)
	out, err := held.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	release := func() {
		_ = held.Process.Kill()
		_ = held.Wait()
		close(done)
	}
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = held.Process.Kill()
			_ = held.Wait()
		}
	})
	// Wait for the holder to say it has the file rather than sleeping at it.
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil {
		t.Fatalf("the holder never took the file: %v (%q)", err, line)
	}
	return path, release
}
