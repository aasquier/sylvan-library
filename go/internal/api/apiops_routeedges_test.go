package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Route edges: a body nobody could parse, a deck nobody holds, a feed that
// stopped mid-sentence, and a combination whose cards the pool does know.

// **A delete asks for a body and has to refuse a broken one.** Every other
// admin route's malformed-body arm is held by the sweep in
// `bodyshapes_test.go`; this one was missed because a DELETE with a body reads
// like a contradiction and is not — the route needs the `confirm` field, so the
// body is where the request is.
func TestForgettingAnAccountRefusesABodyNobodyCanRead(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, adminScope, "DELETE", "/api/admin/users/bob",
		`{"confirm": tru`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a half-written body answered %d: %s", rec.Code, rec.Body)
	}
	// And bob is still there: a refusal at the body must happen before
	// anything is looked up, let alone removed.
	after := rig.call(t, adminScope, "GET", "/api/admin/users", "", "")
	if after.Code != http.StatusOK {
		t.Fatalf("listing the accounts answered %d", after.Code)
	}
	if !strings.Contains(after.Body.String(), `"bob"`) {
		t.Error("bob is gone after a request whose body could not be read")
	}
}

// **A `deck=` that names nothing annotates nothing, and says nothing about it.**
// The suggestion route takes an optional deck so each offer can say whether the
// commander allows it; a reference to a deck the caller cannot see — one that
// does not exist, one belonging to somebody else — must come back as a plain
// unannotated list rather than as a 404, because a 404 here is an oracle for
// whether a stranger's deck exists (ADR 5).
func TestASuggestionAgainstADeckNobodyHoldsIsPlainRatherThanRefused(t *testing.T) {
	t.Parallel()
	// A real `app.db`, so the library assembles and alice's own shelf resolves:
	// the only thing that must fail is the deck lookup itself, which is the arm
	// this is about.
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
		DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: db})

	status, _, raw := callAs(t, a, alice, "GET",
		"/api/cards/suggest?q=Sol&deck=alice/no-such-deck-here", "")
	if status != http.StatusOK {
		t.Fatalf("a suggestion naming a deck nobody holds answered %d: %s",
			status, raw)
	}
	var answer struct {
		Cards []map[string]any `json:"cards"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("the offers did not decode: %v\n%s", err, raw)
	}
	offers := answer.Cards
	if len(offers) == 0 {
		t.Fatalf("no offers came back at all: %s", raw)
	}
	for _, offer := range offers {
		if _, annotated := offer["playable"]; annotated {
			t.Fatalf("an offer was annotated against a deck that does not "+
				"exist, which is a yes-or-no answer about somebody else's "+
				"library: %s", raw)
		}
	}
}

// A combination whose signature cards the pool holds resolves them, with the
// painting and the credit a prose card carries.
//
// Mono-white is the one the fixture can answer: two of its three signature
// cards are in the tiny pool, and the third is the `dropped` count working.
func TestACombinationResolvesTheSignatureCardsThePoolHolds(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "GET", "/api/colors/W", "")
	if status != http.StatusOK {
		t.Fatalf("%d %s", status, raw)
	}
	signature, _ := body["signature"].([]any)
	if len(signature) != 2 {
		t.Fatalf("%d signature cards resolved, want the 2 the fixture holds: %s",
			len(signature), raw)
	}
	for _, entry := range signature {
		card, _ := entry.(map[string]any)
		if name, _ := card["name"].(string); name == "" {
			t.Errorf("a resolved signature card crossed with no name: %v", card)
		}
	}
	// The one the pool does not have is counted rather than silently missing.
	if dropped, _ := body["dropped"].(float64); dropped != 1 {
		t.Errorf("dropped is %v, want 1 -- the third signature card is not in "+
			"the fixture and the count is how a reader knows", body["dropped"])
	}
}

// ---- the upcoming-sets feed ------------------------------------------------

// An entry in the feed that is not an object is **skipped**, not read.
//
// Scryfall has never sent one, which is the point: this is the arm that keeps a
// feed change from becoming a panic in a route the set page asks on every load.
func TestAFeedEntryThatIsNotASetIsSkipped(t *testing.T) {
	t.Parallel()
	got := upcomingFrom(map[string]any{"data": []any{
		"a string where a set should be",
		nil,
		map[string]any{"code": "fin", "name": "Final Fantasy",
			"released_at": "2099-01-01"},
	}}, "2026-10-03")
	var sets []wire.OrderedMap
	for _, kv := range got {
		if kv.Key == "sets" {
			sets, _ = kv.Value.([]wire.OrderedMap)
		}
	}
	if len(sets) != 1 {
		t.Fatalf("%d sets came out of a feed with one real entry in it: %v",
			len(sets), got)
	}
	for _, kv := range sets[0] {
		if kv.Key == "code" && kv.Value != "fin" {
			t.Errorf("the one real entry shaped to %v", sets[0])
		}
	}
}

// **A feed that stops mid-sentence is "could not reach", not a 200.**
// A connection that dies between the headers and the last byte is the ordinary
// shape of a flaky network, and a route that served whatever arrived would
// publish a truncated set list as the whole truth.
func TestAFeedThatStopsMidBodyIsReportedAsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			return
		}
		// A promise of a long body and a tenth of it, then the socket goes.
		_, _ = fmt.Fprint(buffered, "HTTP/1.1 200 OK\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: 4096\r\n\r\n"+
			`{"data":[{"code":"fin"`)
		_ = buffered.Flush()
		closeHard(conn)
	}))
	defer srv.Close()

	a := New(Config{Logger: quietLogger(), SetsFeed: srv.URL})
	status, payload, raw := call(t, a, "GET", "/api/sets/upcoming", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("a truncated feed answered %d: %s", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if detail == "" {
		t.Fatalf("the refusal said nothing: %s", raw)
	}
}

// closeHard drops the connection without the polite close, so the client's read
// ends short rather than cleanly.
func closeHard(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetLinger(0)
	}
	_ = conn.Close()
}
