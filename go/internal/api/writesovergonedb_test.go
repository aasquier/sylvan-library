package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The write half of `closeddb_test.go`'s question, and the half where a
// swallowed error costs somebody a deck rather than a page of zeroes.
//
// A read that eats its database error answers "you have no accounts" and is
// merely false. A **write** that eats one answers "created" and is false about
// something that does not exist: the caller closes the tab believing a deck is
// on their shelf. Every route here resolves the caller's library before it
// writes anything, and that resolution is a query -- so a handle that has gone
// is the one fault that reaches all of them at once.
//
// Two arrangements, because they fail at different depths and only the second
// reaches the crypt:
//
//   - a maintainer configured, so the library is resolved by a lookup that
//     fails outright;
//   - **no maintainer configured** -- the `local` arrangement a laptop and a
//     fresh instance both run -- where the lookup is skipped, the library
//     resolves fine, and the failure arrives one layer in, when the shared
//     shelf is read. That is the one the crypt's three routes and the two
//     master switches take, and none of them had ever taken it.

// noMaintainerGoneDB is `goneDB` with no maintainer address: the library
// resolves, and every route that asks it what is *visible* runs into the
// closed handle instead.
func noMaintainerGoneDB(t *testing.T) *API {
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
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AppDB: db, AppWriteDB: writeDB, AppDBPath: path})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeDB.Close(); err != nil {
		t.Fatal(err)
	}
	return a
}

// Every write route, against a database that is not there any more. The
// question is the one `closeddb_test.go` asks of the reads, sharpened: not
// "did it 500" but **"did it claim to have written something"**.
func TestNoWriteClaimsSuccessWhenTheDatabaseHasGone(t *testing.T) {
	t.Parallel()
	a := goneDB(t)

	all := append(append([]deckRoute{}, writes()...), topLevelWrites...)
	swept := 0
	for _, route := range all {
		target := route.suffix
		switch {
		case !strings.HasPrefix(target, "/api/"):
			target = route.path(t, "alice", "mono-green-clean")
		case strings.Contains(target, "{id}"):
			target = strings.Replace(target, "{id}", "no-such-handle", 1)
		}
		swept++
		status, payload, raw := a.hit(t, alice, route.method, target, route.payload)
		if status == http.StatusOK {
			t.Errorf("%s %s answered 200 over a database that has gone: %s",
				route.method, target, raw)
			continue
		}
		if len(raw) == 0 {
			t.Errorf("%s %s answered %d with no body at all", route.method, target, status)
			continue
		}
		if !json.Valid(raw) {
			t.Errorf("%s %s answered non-JSON: %s", route.method, target, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s %s answered %d and said nothing a person could read: %s",
				route.method, target, status, raw)
		}
	}
	if swept < 20 {
		t.Fatalf("only %d write routes were swept -- the table this reads has "+
			"shrunk and the sweep is no longer worth its green", swept)
	}
}

// The crypt and the two master switches, on an instance with no maintainer
// address, when the shelf cannot be read.
//
// These five resolve the caller's *whole* library rather than one deck, so
// they are the routes where "I cannot read your shelf" and "your shelf is
// empty" are the easiest two sentences to confuse -- and the crypt is the one
// room where the second sentence, wrongly given, reads as "the decks you
// buried are destroyed".
func TestTheCryptSaysItCannotReadTheShelfRatherThanThatItIsEmpty(t *testing.T) {
	t.Parallel()
	a := noMaintainerGoneDB(t)

	for _, row := range []struct{ method, target, body string }{
		{"GET", "/api/decks/entombed", ""},
		{"POST", "/api/decks/entombed/no-such-handle/return", ""},
		{"DELETE", "/api/decks/entombed?confirm=exile", ""},
		{"PUT", "/api/decks/shared", `{"shared":true}`},
		{"PUT", "/api/decks/coliseum-at-night", `{"coliseum_at_night":true}`},
	} {
		status, payload, raw := a.hit(t, alice, row.method, row.target, row.body)
		if status == http.StatusOK {
			t.Errorf("%s %s answered 200 over a shelf it could not read: %s",
				row.method, row.target, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s %s answered %d with nothing a person could read: %s",
				row.method, row.target, status, raw)
		}
	}
}

// hit is `callAs` against a bare API rather than a rig, so a fixture that is
// not a `writeRig` can still drive a route.
func (a *API) hit(t *testing.T, scope auth.Scope, method, target, body string) (
	int, map[string]any, []byte) {
	t.Helper()
	return callAs(t, a, scope, method, target, body)
}

// saysSomething is the one thing every refusal owes the person reading it: a
// sentence. `fmtDetail` renders either shape the wire uses -- a string on the
// routes' own refusals, a validation list on the body checks -- and renders an
// absent one as `null`, which is exactly the answer this is looking for.
func saysSomething(payload map[string]any) bool {
	detail := strings.TrimSpace(fmtDetail(payload))
	return detail != "" && detail != "null"
}
