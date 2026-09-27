package tools

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What a tool answers when the card pool **fails a query** rather than being
// absent.
//
// The two faults look the same from a handler and are not the same thing, and
// this file exists because only one of them had ever been driven here. A
// machine with no pool is a fresh checkout before `mtglab data refresh`: the
// deck tools answer anyway and `pool_available` says so, and the card tools
// say the pool is not there. A pool file DuckDB opens happily and then cannot
// answer -- a half-written refresh, a truncated restore, a schema older than
// the binary -- is a real error, on a path where every handler has an
// `if err != nil` nobody had ever reached.
//
// What is asked of each tool is not the wording, which is DuckDB's rather
// than ours. It is that the failure **comes back as a failure**: a handler
// that swallowed a catalog error and answered an empty card list would be
// telling the model "there is no such card", which is a sentence about Magic
// rather than about the machine, and the model would go on to reason from it.

// schemalessConn leases a connection to a real DuckDB file with none of the
// pool's tables in it: `Use` succeeds, every `SELECT` fails.
//
// The lease is held for the length of the test the way a Claude conversation
// holds one for the length of a turn -- the same shape `leasedConn` uses, and
// the same reason.
func schemalessConn(t *testing.T) *pool.Conn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	// One table, so the file is a database rather than an empty stub -- and
	// deliberately not one of ours, so nothing resolves.
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
		err := p.Use(context.Background(), func(c *pool.Conn) error {
			held = c
			close(ready)
			<-done
			return nil
		})
		if err != nil {
			panic(err)
		}
	}()
	<-ready
	t.Cleanup(func() { close(done) })
	return held
}

// The sanity check the rest of this file rests on: the fault really is a
// query error and not the degraded answer a missing pool gives. If the two
// ever fold into one, every test below would keep passing while testing the
// other thing.
func TestASchemalessPoolFailsAToolsQueryRatherThanItsLease(t *testing.T) {
	t.Parallel()
	c := schemalessConn(t)
	if c == nil {
		t.Fatal("leasing a schemaless pool answered no connection at all")
	}
	_, err := c.GetCards(context.Background(), []string{"Sol Ring"})
	if err == nil {
		t.Fatal("a schemaless pool answered a card lookup; the fault this file " +
			"is about has stopped happening")
	}
}

// Every tool that reads the pool reports the failure. The deck tools have a
// documented answer for a pool that is *absent* and none for a pool that is
// *broken*, and answering the absent-pool shape here would say "this deck has
// no card data" where the truth is "I cannot read the card data".
func TestEveryToolReportsAFailingPoolRatherThanAnEmptyAnswer(t *testing.T) {
	t.Parallel()
	broken := deps(t, false, "gyome")
	broken.Pool = schemalessConn(t)

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"list_decks", map[string]any{}},
		{"get_deck", map[string]any{"slug": "gyome"}},
		{"validate_deck", map[string]any{"slug": "gyome"}},
		{"deck_stats", map[string]any{"slug": "gyome"}},
		{"suggest_replacements", map[string]any{"slug": "gyome"}},
		{"get_cards", map[string]any{"names": []any{"Sol Ring"}}},
		{"search_cards", map[string]any{"q": "Forest"}},
	} {
		out, err := run(t, tc.name, tc.args, broken)
		if err == nil {
			t.Errorf("%s answered %v over a pool that cannot answer a query",
				tc.name, out)
		}
	}
}

// Every filter `search_cards` advertises is carried into the query rather
// than being read and dropped.
//
// Nine arguments, nine reads, and every one of them was a line nothing had
// ever driven -- a schema advertises a filter to the model, so a filter the
// handler quietly ignores is a promise the model believes and acts on. The
// assertion is that the narrow search answers a *subset* of the wide one:
// asking what the rows are would be asking the fixture pool's contents, and
// what is in question here is whether the argument arrived at all.
func TestSearchCardsCarriesEveryFilterItAdvertises(t *testing.T) {
	t.Parallel()
	d := deps(t, true)
	ctx := context.Background()

	wide, err := run(t, "search_cards", map[string]any{"q": ""}, d)
	if err != nil {
		t.Fatalf("the unfiltered search: %v", err)
	}
	wideTotal := searchTotal(t, wide)
	if wideTotal == 0 {
		t.Fatal("the unfiltered search found nothing; the fixture pool is empty " +
			"and every comparison below would be vacuous")
	}

	// Every argument at once, so a read that was never wired shows up as the
	// filter having no effect.
	narrow, err := Run(ctx, "search_cards", map[string]any{
		"q":               "a",
		"identity":        "G",
		"type_line":       "Creature",
		"sort":            "name",
		"identity_exact":  true,
		"commanders_only": true,
		"cmc_max":         float64(3),
		"price_max":       float64(1),
		"limit":           float64(4),
	}, d, nil)
	if err != nil {
		t.Fatalf("the filtered search: %v", err)
	}
	narrowTotal := searchTotal(t, narrow)
	if narrowTotal >= wideTotal {
		t.Errorf("nine filters at once found %d of %d cards; at least one "+
			"argument is being read and dropped", narrowTotal, wideTotal)
	}

	// The limit is capped here as well as in the schema, because the schema is
	// advice to the model and this is the rule. A model that asks for a
	// thousand gets two hundred, not a thousand.
	big, err := Run(ctx, "search_cards", map[string]any{"limit": float64(1000)}, d, nil)
	if err != nil {
		t.Fatalf("an over-large limit: %v", err)
	}
	if n := searchTotal(t, big); n > 200 {
		t.Errorf("a limit of 1000 answered %d cards; the handler's cap is 200", n)
	}
}

// searchTotal reads the `total` the search tool reports, failing the test
// rather than the assertion when the shape is not the one the model is told
// to expect.
func searchTotal(t *testing.T, out any) int {
	t.Helper()
	payload, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("search_cards answered %T, not the object its schema promises", out)
	}
	total, ok := payload["total"].(int)
	if !ok {
		t.Fatalf("search_cards answered total %#v", payload["total"])
	}
	if _, ok := payload["cards"]; !ok {
		t.Error("search_cards answered no `cards` key -- the model iterates it")
	}
	return total
}

// A library the process cannot READ is not a library with no decks in it.
//
// `list_decks` answering an empty list over an unreadable directory would
// tell the model the shelf is bare, and the model would then reason about a
// person's collection from the premise that they own nothing -- which is the
// same lie an absent pool would tell about cards, told about the half of the
// app that is actually theirs.
func TestAnUnreadableLibraryIsReportedRatherThanListedAsEmpty(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0o000 directory, so the fault cannot be staged here")
	}
	d := deps(t, false, "gyome")
	root := t.TempDir()
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	// Put it back, or the temp-dir cleanup cannot remove what is inside it.
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
	d.Source = library.NewFileSource(root, false)

	out, err := run(t, "list_decks", map[string]any{}, d)
	if err == nil {
		t.Errorf("listing an unreadable library answered %v rather than "+
			"reporting that it could not be read", out)
	}
}

// The refusal sentences reach the model as tool-result text, so how a name is
// quoted in one is recorded rather than incidental: single quotes, unless the
// name itself holds a single quote and no double quote.
func TestARefusalQuotesAToolNameTheRecordedWay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, want string }{
		{"set_card_field", `'set_card_field'`},
		{"it's_a_tool", `"it's_a_tool"`},
		{`it's_a_"tool"`, `'it\'s_a_"tool"'`},
	} {
		_, err := run(t, tc.name, map[string]any{}, Deps{})
		if err == nil {
			t.Errorf("%q was dispatched", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("refusing %q reads %q; the recorded quoting is %s",
				tc.name, err, tc.want)
		}
	}
}
