package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// Every route that reads `app.db`, against a handle that has gone.
//
// `app.db` lives on the instance's volume. A handle whose file went away
// mid-request — a volume detached, a restore in progress, a database moved
// under a running process — is a real fault and not a contrived one, and it is
// the fault whose branches nothing had ever taken: almost every query in this
// package ends `if err != nil { … 500 }`, and a working database never goes
// there.
//
// That is the branch worth taking, because a swallowed error here does not
// crash — it **succeeds wrongly**. An admin roster that ate its query error
// and rendered zero accounts says "nobody is registered" to the one person who
// could tell that it is false. A stats view that did the same reports an
// instance nobody uses.
//
// What is asserted is not the message — SQLite's wording is not ours — but
// that the answer is **readable and not a lie**: it must not be a panic, it
// must not be an empty body, and it must not be a 200 carrying an empty
// collection. `internal/auth/errorpaths_test.go` uses the same lever one layer
// down; this is that lever at the route.

// goneDB is an API over a real migrated `app.db` whose handle has been closed
// underneath it. Both handles go: the read one and the write one.
func goneDB(t *testing.T) *API {
	t.Helper()
	path := appDB(t)
	db, err := auth.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	writeDB, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{
		Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: writeDB,
		AppDBPath: path,
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeDB.Close(); err != nil {
		t.Fatal(err)
	}
	return a
}

// Every GET, against a database that is not there any more.
func TestNoRouteLiesWhenTheDatabaseHasGone(t *testing.T) {
	t.Parallel()
	a := goneDB(t)

	swept := 0
	for _, route := range a.Routes() {
		if route.Method != http.MethodGet {
			continue
		}
		target := fillPattern(route.Pattern)
		if target == "" {
			continue
		}
		swept++
		status, payload, raw := callAs(t, a, alice, http.MethodGet, target, "")
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s answered %d with no body", target, status)
			continue
		}
		if status >= 400 {
			if !json.Valid(raw) {
				t.Errorf("%s answered non-JSON: %s", target, raw)
				continue
			}
			if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Errorf("%s answered %d with no detail: %s", target, status, raw)
			}
			continue
		}
		// A 200 is allowed where the route's answer does not come from the
		// database — the colour taxonomy, the glossary, a tarot deal. What it
		// may not be is a *collection* that came back empty because the query
		// failed: "you have no accounts" and "I cannot read your accounts" are
		// different sentences and only one of them is true.
		if isEmptyCollection(raw) && readsTheDatabase(route.Pattern) {
			t.Errorf("%s answered 200 with an empty collection over a database "+
				"that has gone -- that reads as 'there is nothing there': %s",
				target, raw)
		}
	}
	if swept < 15 {
		t.Errorf("only %d routes were swept -- the pattern filler has stopped "+
			"matching the route table", swept)
	}
}

// Every admin route, which is where the lie would cost most: the roster, the
// stats and the ledger are what one person reads to find out whether the
// instance is healthy.
func TestNoAdminRouteLiesWhenTheDatabaseHasGone(t *testing.T) {
	t.Parallel()
	a := goneDB(t)

	// Discovered, not listed: every admin and account route the API serves,
	// with a body that would be valid if the database were there. A hand-kept
	// list here would lose the route added last, which is the one nobody has
	// driven against a gone database.
	swept := 0
	for _, route := range a.Routes() {
		if !strings.HasPrefix(route.Pattern, "/api/admin") &&
			!strings.HasPrefix(route.Pattern, "/api/account") {
			continue
		}
		target := strings.ReplaceAll(route.Pattern, "{username}", "alice")
		if strings.Contains(target, "{") {
			continue
		}
		swept++
		tc := struct{ method, target, body string }{route.Method, target, bodyFor(route.Pattern)}
		status, _, raw := callAs(t, a, alice, tc.method, tc.target, tc.body)
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s %s answered %d with no body", tc.method, tc.target, status)
			continue
		}
		if status == http.StatusOK && isEmptyCollection(raw) {
			t.Errorf("%s %s answered 200 with an empty collection over a database "+
				"that has gone: %s", tc.method, tc.target, raw)
		}
	}
	if swept < 8 {
		t.Errorf("only %d admin routes were swept -- the filter has stopped "+
			"matching the route table", swept)
	}
}

// bodyFor is a request body that would be valid if the database were there,
// so a route refuses for the reason this sweep is about rather than at its
// own grammar check.
func bodyFor(pattern string) string {
	switch {
	case strings.HasSuffix(pattern, "/api/admin/users"):
		return `{"email":"friend@example.com","username":"friend"}`
	case strings.HasSuffix(pattern, "/login"):
		return `{"username":"alice","password":"a-password-long-enough"}`
	case strings.Contains(pattern, "/users/{username}"):
		return `{"is_admin":true}`
	case strings.HasSuffix(pattern, "/password"):
		return `{"current":"old-password-here","new":"new-password-here"}`
	}
	return "{}"
}

// isEmptyCollection reports whether a body is `[]`, `{}` or an object whose
// only list is empty -- the shapes that read as "there is nothing there".
func isEmptyCollection(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "[]" || trimmed == "{}"
}

// readsTheDatabase names the route families whose answer comes out of
// `app.db`. The reference prose, the colour taxonomy and the tarot deal do
// not, so an empty-looking answer from one of those is not a lie.
//
// **`/api/jobs` is not one of them**, which this sweep found the hard way: the
// registry is in-memory, so a process with no jobs running answers `[]`
// honestly whatever the database is doing. Listing it here made the sweep
// fail on correct behaviour, which is the failure mode a hand-kept "these read
// the database" list has.
func readsTheDatabase(pattern string) bool {
	for _, prefix := range []string{"/api/admin", "/api/account"} {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	return strings.HasSuffix(pattern, "/log")
}
