package pool_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// staleOn builds the fixture pool, applies each doctoring statement, and
// asks `Stale` — the five verdicts `cards/db.py:pool_is_stale` documents.
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
