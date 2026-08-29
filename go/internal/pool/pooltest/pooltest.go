// Package pooltest builds the 21-card tiny pool as a real DuckDB file, for
// any test that wants a pool: the recorded rows in testdata/ --
// every value read out of the real pool, never typed from memory --
// loaded through the
// driver into the schema `pool.Schema` embeds. CI carries
// no DuckDB file; this is how its jobs read real cards anyway. It is the
// same fixture in a second encoding, not a card pool redistribution (ADR 6,
// CLAUDE.md rule 5).
package pooltest

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

//go:embed testdata/tiny_pool.json
var tinyPool []byte

type fixture struct {
	OracleColumns   []string `json:"oracle_columns"`
	OracleCards     [][]any  `json:"oracle_cards"`
	PrintingColumns []string `json:"printing_columns"`
	Printings       [][]any  `json:"printings"`
}

// Build writes the fixture into a DuckDB file under the test's temp dir and
// returns its path, so a test opens it the way the app opens the real one:
// read-only, on a lease.
func Build(tb testing.TB) string {
	tb.Helper()
	var fx fixture
	dec := json.NewDecoder(strings.NewReader(string(tinyPool)))
	dec.UseNumber()
	if err := dec.Decode(&fx); err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(tb.TempDir(), "tiny.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		tb.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), pool.Schema); err != nil {
		tb.Fatalf("schema: %v", err)
	}
	insert(tb, db, "oracle_cards", fx.OracleColumns, fx.OracleCards)
	insert(tb, db, "printings", fx.PrintingColumns, fx.Printings)
	return path
}

// Open is Build plus a Pool over it, closed when the test ends.
func Open(tb testing.TB) *pool.Pool {
	tb.Helper()
	p := pool.New(Build(tb), nil)
	tb.Cleanup(p.Close)
	return p
}

// Writer opens a pool file read-write, for a test that wants to change one
// under a Pool.
func Writer(path string) (*sql.DB, error) {
	return sql.Open("duckdb", path)
}

// insert binds each row by column, casting where the driver's VARCHAR would
// otherwise meet a typed column: lists, JSON, dates.
//
// **`?::JSON` is not how the refresh writes, and that gap has cost a live
// outage.** The cast *parses* the text into a document; the Appender that
// `pool.LoadOracle` uses takes a value and would store this same string as a
// JSON string, which `json_extract_string` reads NULL out of. So every
// fixture built here is correct by a route production does not take, and no
// test standing on this one can see an encoding fault in the loader. The
// fixture that can is `cards.TestAPoolWrittenByTheRealLoaderAnswersTheCardQueries`,
// which loads a bulk file through [pool.LoadOracle] itself; send a question
// about *how the pool is written* there, and keep this for questions about
// what the pool contains.
func insert(tb testing.TB, db *sql.DB, table string, cols []string, rows [][]any) {
	tb.Helper()
	holders := make([]string, len(cols))
	for i, c := range cols {
		switch c {
		case "colors", "color_identity", "keywords", "produced_mana", "finishes":
			holders[i] = "?::VARCHAR[]"
		case "legalities", "card_faces", "all_parts":
			holders[i] = "?::JSON"
		case "released_at":
			holders[i] = "?::DATE"
		default:
			holders[i] = "?"
		}
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table,
		strings.Join(cols, ", "), strings.Join(holders, ", "))
	for _, row := range rows {
		args := make([]any, len(row))
		for i, v := range row {
			args[i] = bindable(v)
		}
		if _, err := db.Exec(stmt, args...); err != nil {
			tb.Fatalf("insert into %s: %v\nrow: %v", table, err, row)
		}
	}
}

// bindable turns the JSON reading of a value into what the driver binds: an
// integral number as int64, any other as float64, a list as []string.
func bindable(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64()
		return f
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return v
}
