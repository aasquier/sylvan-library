package pool_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// staleOn builds the fixture pool, applies each doctoring statement, and
// asks `Stale` — the five verdicts its doc comment documents.
func staleOn(t *testing.T, verdict bool, doctoring ...string) {
	t.Helper()
	path := pooltest.Build(t)
	if len(doctoring) > 0 {
		db, err := pooltest.Writer(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range doctoring {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		_ = db.Close()
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	err := p.Use(context.Background(), func(c *pool.Conn) error {
		got, err := pool.Stale(context.Background(), c)
		if err != nil {
			return err
		}
		if got != verdict {
			t.Fatalf("Stale = %v, want %v", got, verdict)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestACurrentPoolIsNotStale(t *testing.T) { staleOn(t, false) }

func TestAnEmptyPoolIsNotStale(t *testing.T) {
	// Nothing to be wrong about: `health` reports the pool as missing.
	staleOn(t, false, "DELETE FROM printings", "DELETE FROM oracle_cards")
}

func TestAPoolWithoutThePrintedStatsIsStale(t *testing.T) {
	staleOn(t, true, "UPDATE oracle_cards SET power = NULL")
	// The column entirely absent -- a pool loaded before it existed. DuckDB
	// refuses to alter a table an index depends on, so the index goes first.
	staleOn(t, true, "DROP INDEX idx_oracle_name",
		"ALTER TABLE oracle_cards DROP COLUMN power")
}

func TestAnOracleOnlyRefreshIsNotStale(t *testing.T) {
	// `--oracle-only` is a supported refresh: an empty `printings` is a
	// deliberate state, not an old one.
	staleOn(t, false, "DELETE FROM printings")
}

func TestAPoolWithUnsignedPaintingsIsStale(t *testing.T) {
	staleOn(t, true, "UPDATE printings SET artist = NULL")
	staleOn(t, true, "DROP INDEX idx_printings_oracle",
		"ALTER TABLE printings DROP COLUMN artist")
}

// The verdict is a property of the pool FILE — the pool is opened read-only
// and a refresh re-opens it — so the walk above runs once per open and every
// later ask reads the answer. It matters because of *who asks*: `/api/health`
// asks on every call, the platform's health check is one of its callers, and
// the walk is four probes plus two `Columns` lookups. Eight statements were
// answering a question that needs two.
//
// The instrument is the memo's own hit count rather than a stopwatch, for the
// reason the whole counter exists: a cache can be correct, tested and never
// once used, and only a counter tells the difference.
func TestTheStalenessVerdictIsWalkedOncePerOpen(t *testing.T) {
	ctx := context.Background()
	p := pool.New(pooltest.Build(t), nil)
	t.Cleanup(p.Close)
	ask := func() {
		t.Helper()
		if err := p.Use(ctx, func(c *pool.Conn) error {
			_, err := pool.Stale(ctx, c)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for range 5 {
		ask()
	}
	if hits, misses := p.Memo(pool.MemoStale); hits != 4 || misses != 1 {
		t.Fatalf("staleness memo %d hits / %d misses after five asks, want 4/1",
			hits, misses)
	}
	// `Columns` is asked twice inside the walk and never again, which is the
	// same claim about the memo underneath this one.
	if hits, misses := p.Memo(pool.MemoColumns); hits != 0 || misses != 2 {
		t.Fatalf("columns memo %d hits / %d misses, want 0/2", hits, misses)
	}

	// Per open, and the counters are too: a pool handed back and re-opened
	// re-asks, because the file it re-opens may not be the file it closed.
	p.Close()
	ask()
	if hits, misses := p.Memo(pool.MemoStale); hits != 0 || misses != 1 {
		t.Fatalf("after a re-open the staleness memo reads %d/%d, want 0 hits / 1 miss",
			hits, misses)
	}
}
