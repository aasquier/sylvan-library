package suggest_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
)

// schemalessConn leases a connection to a real card pool file with none of the
// pool's tables in it: the lease succeeds, every `SELECT` fails.
//
// A damaged file would not do — `pool.Pool.Use` cannot open one, so it answers
// the "no pool at all" path, which is a documented degraded answer rather than
// the fault under test. What this needs is a file the engine is happy to open
// and a query it cannot answer: a refresh that died halfway, a restore that
// came back short, a file older than the binary reading it.
//
// Copied in shape from `internal/claude`'s fixture of the same name rather than
// shared, because a helper reaching across package boundaries for a fault is
// a dependency neither package wants.
func schemalessConn(t *testing.T) *pool.Conn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE half_a_refresh (name VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)

	var held *pool.Conn
	ready, done := make(chan struct{}), make(chan struct{})
	go func() {
		if err := p.Use(context.Background(), func(c *pool.Conn) error {
			held = c
			close(ready)
			<-done
			return nil
		}); err != nil {
			panic(err)
		}
	}()
	<-ready
	t.Cleanup(func() { close(done) })
	return held
}

// Replacements for one slot are planned from records the caller already holds
// and then widened by a query of their own. The second half failing is the
// shape this drives: a pool that answered the deck's cards a moment ago and
// cannot answer the search.
//
// What must not happen is an empty list. Nothing in the suggestion payload
// distinguishes "the pool has nothing better for this slot" from "the pool
// could not be asked", so a swallowed failure reads to a player as a verdict on
// their card — the strongest possible statement about a slot, made by an
// outage.
func TestReplacementsRefuseAPoolThatCannotAnswerTheSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := &deck.Deck{
		Commander: []string{"Gyome, Master Chef"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Why: "ramp"},
			{Name: "Primeval Titan", Why: "lands"},
		},
	}
	names := []string{"Gyome, Master Chef", "Sol Ring", "Primeval Titan"}

	// The records come off a pool that works, exactly as the route's earlier
	// read does.
	var cards map[string]*pool.CardRecord
	working := pooltest.Open(t)
	if err := working.Use(ctx, func(c *pool.Conn) error {
		got, err := c.GetCards(ctx, names)
		cards = got
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if cards["Primeval Titan"] == nil {
		t.Fatal("the fixture pool did not answer the card this test is about")
	}

	// The same slot, asked of a pool that has since stopped answering.
	got, err := suggest.ReplacementsFor(ctx, schemalessConn(t), d, cards, "Primeval Titan", 5)
	if err == nil {
		t.Fatalf("a pool that cannot answer a query produced %d suggestions "+
			"instead of a failure", len(got))
	}
	if got != nil {
		t.Errorf("the refusal came back with %d candidates beside it", len(got))
	}

	// The premise: a card the records do not hold is still an empty list rather
	// than an error, so the refusal above is about the pool and not about a
	// lookup miss.
	empty, err := suggest.ReplacementsFor(ctx, schemalessConn(t), d, cards, "A Card Nobody Printed", 5)
	if err != nil {
		t.Errorf("a name the records do not hold refused: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("an unknown name produced %d suggestions", len(empty))
	}
}
