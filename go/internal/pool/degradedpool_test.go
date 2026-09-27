package pool_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The two ways a pool declines to be a pool, and the line between them.
//
// **A corrupt file is not a failing pool.** It reads like one, and a whole
// pass of coverage work was once planned around the belief that pointing the
// app at a broken file would fire every query's error branch. It does not:
// DuckDB cannot open it at all, so the answer is [pool.ErrNoPool] --
// byte-identical to the answer an absent file gives -- and the app degrades
// exactly as it does on a fresh checkout. The fault that reaches a *query* is
// a file DuckDB opens happily and then cannot answer, which is what the
// schemaless fixtures elsewhere in this tree are for.
//
// It is worth a test rather than a note because the two answers are one `if`
// apart, and if `Use` ever folded them together the degraded path would start
// swallowing real faults without a single test going red.

func TestAFileThatIsNotADatabaseIsNoPoolRatherThanAFailingOne(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	if err := os.WriteFile(path, []byte("this was never a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)

	reached := false
	err := p.Use(context.Background(), func(*pool.Conn) error {
		reached = true
		return nil
	})
	if !errors.Is(err, pool.ErrNoPool) {
		t.Fatalf("a file that is not a database answered %v, want the same "+
			"degraded answer an absent one gives", err)
	}
	if reached {
		t.Error("a caller was handed a lease on a file that never opened")
	}
	if p.Held() {
		t.Error("the pool is standing open over a file it could not open")
	}
}

// And the open itself refuses a path that is not there, rather than minting an
// empty database at it -- read-only is what makes that true, and it is the
// only reason a running app cannot quietly create a pool of its own beside the
// real one.
func TestOpeningAPoolThatIsNotThereIsRefusedRatherThanCreated(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-volume", "mtg.duckdb")
	db, err := pool.Open(context.Background(), missing)
	if err == nil {
		_ = db.Close()
		t.Fatal("a pool was opened on a path with no directory under it")
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Error("the refused open left a file behind")
	}
}

// The fixture's own two conveniences, which nothing had asked of it: doctoring
// in no extra cards is the plain pool, and a card with no colours named is
// colourless rather than a nil that binds as NULL.
func TestTheFixtureDoctorsInNothingWhenAskedForNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plain := pooltest.BuildWith(t)
	p := pool.New(plain, nil)
	t.Cleanup(p.Close)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		got, err := c.GetCards(ctx, []string{"Sol Ring"})
		if err != nil {
			return err
		}
		if got["Sol Ring"] == nil {
			t.Error("a fixture built with no extras lost the recorded cards")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestADoctoredCardWithNoColoursNamedIsColourless(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pooltest.OpenWith(t, pooltest.Card{
		Name: "Fixture Sundial", ManaCost: "{2}", CMC: 2,
		TypeLine: "Artifact", OracleText: "Fixture text.",
	})
	if err := p.Use(ctx, func(c *pool.Conn) error {
		got, err := c.GetCards(ctx, []string{"Fixture Sundial"})
		if err != nil {
			return err
		}
		rec := got["Fixture Sundial"]
		if rec == nil {
			t.Fatal("the doctored card did not go in")
		}
		if rec.ColorIdentity == nil || len(rec.ColorIdentity) != 0 {
			t.Errorf("its identity reads %+v, want an empty list -- a nil "+
				"there binds as NULL and every colour check downstream reads "+
				"NULL as 'we do not know'", rec.ColorIdentity)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
