package pool

import (
	"context"
	"testing"
)

// The staleness walk against pools that cannot answer it, and the two places
// the pool's memory declines to learn something.
//
// **Staleness is the app's guard against the quiet wrong answer** -- a pool
// loaded before `power` existed answers every question about it with NULL,
// which reads exactly like "this creature has no power". So a walk that cannot
// be completed must come back as a failure: "not stale" from a pool nobody
// could read is the same false reassurance one rung up.

func TestTheStalenessWalkRefusesAPoolItCannotRead(t *testing.T) {
	t.Parallel()
	stale, err := Stale(context.Background(), onAClosedPool(t))
	if err == nil {
		t.Fatalf("a closed pool was walked and declared stale=%v", stale)
	}
	if stale {
		t.Error("a refusal came back carrying a verdict as well")
	}
}

// The printings half of the walk, on a pool that has an oracle table and no
// printings table at all -- a rebuild that died between its two loads. An
// *empty* printings table is a deliberate state (`--oracle-only` produces one)
// and reads as not stale; a printings table that is not there is a fault, and
// the two must not fold into one answer.
func TestTheStalenessWalkTellsAnEmptyPrintingsTableFromAMissingOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const oracle = `CREATE TABLE oracle_cards (name VARCHAR, power VARCHAR)`
	const aCreature = `INSERT INTO oracle_cards VALUES ('Fixture Chef', '3')`

	// No printings table: the walk cannot finish and says so.
	if _, err := Stale(ctx, onTables(t, oracle, aCreature)); err == nil {
		t.Error("a pool with no printings table was walked to a verdict")
	}

	// An empty one, which `--oracle-only` leaves behind deliberately.
	verdict, err := Stale(ctx, onTables(t, oracle, aCreature,
		`CREATE TABLE printings (artist VARCHAR)`))
	if err != nil {
		t.Fatal(err)
	}
	if verdict {
		t.Error("an oracle-only refresh's empty printings table read as a " +
			"stale pool; nothing is wrong with it and the page would tell " +
			"somebody to run a refresh that just ran")
	}
}

// A verdict learned through a Conn whose open the pool has already handed back
// is **not** filed. The memory is keyed on the file standing still, and a Conn
// that outlived its open may describe a pool that is no longer there.
func TestAVerdictFromAnOpenThePoolNoLongerHoldsIsNotRemembered(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, power VARCHAR)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Chef', '3')`,
		`CREATE TABLE printings (artist VARCHAR)`)
	if _, err := Stale(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.pool.stale != nil {
		t.Error("a Conn taught the pool a verdict about a file the pool is " +
			"not holding")
	}
}

// A pool that has never been opened does not keep counters. A memo answer with
// no memo behind it is not a hit rate, and counting it would make a pool
// nobody has asked anything of look busy.
func TestAPoolThatHasNeverStoodKeepsNoCounters(t *testing.T) {
	t.Parallel()
	p := New("never-opened.duckdb", nil)
	p.note(MemoColumns, true)
	p.note(MemoCards, false)
	if hits, misses := p.Memo(MemoColumns); hits != 0 || misses != 0 {
		t.Errorf("an unopened pool reports %d hits and %d misses", hits, misses)
	}
}
