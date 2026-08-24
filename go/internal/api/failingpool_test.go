package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What every route does when the card pool answers an **error** rather than
// answering nothing.
//
// These are two different faults and the code treats them differently, which
// is the whole reason this file is separate from the degraded-mode tests. A
// machine with no pool at all is a fresh checkout before `mtglab data
// refresh`: [pool.ErrNoPool], a documented degraded answer, and every surface
// has a sentence for it. A pool file that opens and then *fails a query* is a
// half-written refresh, a truncated restore, or a schema older than the
// binary reading it — a real error, carrying DuckDB's own words, on a path
// nothing had ever driven.
//
// The distinction is easy to lose. A corrupt file -- bytes that are not a
// database at all -- is **not** this fault: [pool.Pool.Use] cannot open it and
// returns `ErrNoPool`, exactly as an absent file does, so it re-drives the
// degraded path rather than this one. It takes a file DuckDB is happy to open
// and a `SELECT` it cannot answer, which is what `schemalessPool` builds.
//
// Two things are asked of every route, and they are the same two the
// unreadable-library sweep asks, for the same reason. It must **say
// something** in the shape the client reads -- a 500 with a sentence is
// honest, a panic is not, and neither is an empty body. And it must **not
// answer 200**: a route that swallowed the error and served a card list built
// from nothing would be telling somebody their pool is fine.

// schemalessPool is a real DuckDB file with none of the pool's tables in it:
// every query fails with a catalog error, and the failure arrives from inside
// `Use` rather than from `acquire`.
func schemalessPool(t *testing.T) *pool.Pool {
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
	return p
}

// failingPoolAPI is an instance whose library is fine and whose pool is not.
func failingPoolAPI(t *testing.T) *API {
	t.Helper()
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(Config{
		Pool: schemalessPool(t), DecksDir: decksDir(t),
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: db,
	})
}

// The sanity check the rest of this file rests on: the fault really is a query
// error and not the degraded answer. If [pool.Pool.Use] ever starts folding
// this into `ErrNoPool`, every test below would still pass while testing the
// degraded path a second time -- so the distinction is asserted directly
// rather than assumed.
func TestASchemalessPoolFailsTheQueryRatherThanTheOpen(t *testing.T) {
	t.Parallel()
	err := schemalessPool(t).Use(t.Context(), func(c *pool.Conn) error {
		_, e := c.GetCards(t.Context(), []string{"Sol Ring"})
		return e
	})
	if err == nil {
		t.Fatal("a pool with no tables answered a card lookup")
	}
	if err == pool.ErrNoPool { //nolint:errorlint // the sentinel identity is the point
		t.Fatal("a pool that opens and fails its query was reported as an absent " +
			"pool -- this whole file would then be re-testing the degraded path")
	}
}

// Every GET the API serves, against a pool that will not answer.
//
// **The list is discovered, not written here.** A hand-kept list of routes is
// a list that loses the one added last, which is exactly the route nobody has
// driven against a broken pool -- so this asks [API.Routes] and fills the
// parameters in. A route added tomorrow is swept tomorrow, with nothing to
// remember.
func TestEveryPoolRouteSaysSomethingWhenTheQueryFails(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	swept := 0
	for _, route := range a.Routes() {
		if route.Method != http.MethodGet {
			continue
		}
		target := fillPattern(route.Pattern)
		if target == "" {
			continue // a parameter this sweep has no sensible value for
		}
		swept++
		status, payload, raw := callAs(t, a, alice, http.MethodGet, target, "")
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s answered %d with no body", target, status)
			continue
		}
		if status >= 400 && !json.Valid(raw) {
			t.Errorf("%s answered non-JSON: %s", target, raw)
			continue
		}
		// A 200 is allowed and often right: `health` reports the pool's state
		// rather than failing with it, and the deck views degrade on purpose
		// (an invalid deck is diagnosed, never refused). What no route may do
		// is answer 500 with nothing to read.
		if status >= 400 {
			if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Errorf("%s answered %d with no detail: %s", target, status, raw)
			}
		}
	}
	// A sweep that swept nothing passes silently, which is the failure mode
	// every table-driven test has.
	if swept < 15 {
		t.Errorf("only %d routes were swept -- the pattern filler has stopped "+
			"matching the route table", swept)
	}
}

// fillPattern turns a route pattern into a request path, or "" for one this
// sweep cannot ask sensibly.
//
// The deck parameters resolve to a real deck in a real library, so the route
// reaches the pool rather than stopping at a 404 -- which would sweep the
// refusal path a second time instead of the one this file is about.
func fillPattern(pattern string) string {
	switch {
	case strings.Contains(pattern, "{owner}"):
		pattern = strings.ReplaceAll(pattern, "{owner}", "alice")
		pattern = strings.ReplaceAll(pattern, "{slug}", "gyome")
		pattern = strings.ReplaceAll(pattern, "{name}", "primer-quick.md")
	case strings.Contains(pattern, "{key}"):
		pattern = strings.ReplaceAll(pattern, "{key}", "G")
	case strings.Contains(pattern, "{code}"):
		pattern = strings.ReplaceAll(pattern, "{code}", "G")
	}
	if strings.Contains(pattern, "{") {
		return "" // an OCR name, an oracle id, an effect: nothing to fill
	}
	if pattern == "/api/cards/search" {
		return pattern + "?q=sol"
	}
	return pattern
}

// A write that needs the pool must not report success over one that cannot
// answer: an edit reported as landed, against a pool that could not resolve
// the card, is the answer somebody acts on.
func TestNoWriteReportsSuccessOverAPoolThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	for _, route := range writeRoutes {
		target := "/api/decks/alice/gyome" + route.suffix
		status, _, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusOK && needsThePool(route.suffix) {
			t.Errorf("%s %s reported success over a pool that will not answer: %s",
				route.method, target, raw)
		}
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s %s answered %d with no body", route.method, target, status)
		}
	}
}

// needsThePool is the half of the write family whose success depends on a card
// resolving. The rest -- a note, a stage, a share flag, a delete -- are facts
// about the file and are right to succeed without a pool, which is why this
// sweep asks about them separately rather than failing them all.
func needsThePool(suffix string) bool {
	switch suffix {
	case "/swap", "/cards", "/entomb":
		return true
	}
	return false
}

// Creating and importing reach the pool from a different direction: both
// resolve a commander before they will write anything, and a commander that
// cannot be resolved is a refusal rather than a deck with no identity.
func TestCreateAndImportRefuseWhenTheCommanderCannotBeResolved(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	for _, tc := range []struct{ target, body string }{
		{"/api/decks", `{"slug":"new-deck","commander":"Sol Ring"}`},
		{"/api/decks/import", `{"slug":"imported","text":"1 Sol Ring"}`},
	} {
		status, _, raw := callAs(t, a, alice, http.MethodPost, tc.target, tc.body)
		if status == http.StatusOK || status == http.StatusCreated {
			t.Errorf("%s created a deck against a pool that will not answer: %s",
				tc.target, raw)
		}
		if len(raw) == 0 {
			t.Errorf("%s answered %d with no body", tc.target, status)
		}
	}
}
