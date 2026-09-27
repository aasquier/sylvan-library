package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// `failingpool_test.go`'s question, asked of a deck that is actually there.
//
// That file fills its route patterns with `alice/gyome`, and **there is no
// deck called gyome in the fixture library** -- the file tier a test builds is
// the gate's nine testdata decks, and none of them is named that. So every
// deck-scoped route in both of its sweeps stops at ADR 5's 404 and never
// reaches the pool at all: the sweeps are honest about what they assert and
// were re-driving the refusal path instead of the query-failure path. The
// non-deck routes (the card doors, the colour rooms) do reach it, which is
// why the file reads as working.
//
// This is the same lever aimed at `kaheera` and `mono-green-clean`, which the
// library does hold. What it reaches is the whole family of
// `if err != nil { return err }` lines inside a `usePool` closure -- the ones
// a working database never goes near, on every route that looks a card up
// before it answers.
//
// The two questions are the ones that file argues for, and the second is the
// one that matters: a route may degrade, and a route may refuse, but no route
// may **answer as though the pool had spoken**. A commander dossier assembled
// from a failed query is a page of confident blanks.

// A real deck, and a pool that opens and then cannot answer a single query.
func TestEveryDeckReadReachesThePoolAndSaysSomethingWhenItFails(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	swept := 0
	for _, route := range reads() {
		target := route.path(t, "alice", "kaheera")
		swept++
		status, payload, raw := callAs(t, a, alice, route.method, target, "")
		if status == http.StatusNotFound {
			t.Errorf("%s answered 404 -- this sweep is aimed at a deck that "+
				"exists, and a 404 means it never reached the pool: %s", target, raw)
			continue
		}
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s answered %d with no body", target, status)
			continue
		}
		if !json.Valid(raw) && len(raw) > 0 {
			// The artifacts route serves a file rather than JSON, which is
			// fine: it is not a pool route.
			if !strings.Contains(target, "/artifacts/") {
				t.Errorf("%s answered non-JSON: %s", target, raw)
			}
			continue
		}
		if status >= 400 && !saysSomething(payload) {
			t.Errorf("%s answered %d with nothing a person could read: %s",
				target, status, raw)
		}
	}
	if swept < 8 {
		t.Fatalf("only %d deck reads were swept -- the route table this reads "+
			"has shrunk", swept)
	}
}

// The write half, likewise aimed at a deck that is there.
//
// A write is where a swallowed query error stops being a bad page and starts
// being a bad file: `playableCard` is the check that makes rule 1 true of
// every card that goes in, and it runs entirely inside a `usePool` closure. A
// pool that answers an error rather than a card must never leave that check
// reporting "fine".
func TestNoDeckWriteTreatsAFailedQueryAsAnAnsweredOne(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	swept := 0
	for _, route := range writes() {
		target := route.path(t, "alice", "mono-green-clean")
		swept++
		status, payload, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusNotFound {
			t.Errorf("%s %s answered 404 against a deck that exists -- it never "+
				"reached the pool: %s", route.method, target, raw)
			continue
		}
		if status == http.StatusOK && needsThePool(route.suffix) {
			t.Errorf("%s %s reported success over a pool that will not answer: %s",
				route.method, target, raw)
			continue
		}
		if len(raw) == 0 && status != http.StatusNoContent {
			t.Errorf("%s %s answered %d with no body", route.method, target, status)
			continue
		}
		if status >= 400 && json.Valid(raw) && !saysSomething(payload) {
			t.Errorf("%s %s answered %d with nothing a person could read: %s",
				route.method, target, status, raw)
		}
	}
	if swept < 15 {
		t.Fatalf("only %d deck writes were swept", swept)
	}
}

// The one refusal in this family with a sentence of its own, read as the
// person reading it reads it.
//
// A card that cannot be resolved is not a card that is not in the pool, and
// the two must not answer alike: "Sol Ring is not a card the pool knows" sends
// somebody to check their spelling, and the spelling is not the problem.
func TestAddingACardOverAFailedQueryDoesNotBlameTheCardsName(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	status, payload, raw := callAs(t, a, alice, "POST",
		"/api/decks/alice/mono-green-clean/cards",
		`{"name":"Sol Ring","category":"ramp","why":"Two mana on turn one."}`)
	if status == http.StatusOK {
		t.Fatalf("a card was added over a pool that answered an error: %s", raw)
	}
	detail := fmtDetail(payload)
	if strings.Contains(detail, "is not a card the pool knows") {
		t.Errorf("a failed query was reported as an unknown card, which sends "+
			"somebody to check a spelling that is fine: %q", detail)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries no sentence at all: %s", raw)
	}
}
