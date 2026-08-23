package main

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The worker's door, driven over a real socket.
//
// A real `httptest.Server` rather than a recorder throughout, because half of
// what this file is about only exists on the wire: the ndjson framing, the
// flush that makes a tick arrive before the match ends, the Content-Type that
// tells an old app from a new one, and the bearer token that must be compared
// in constant time.

func shimServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(&shim{state: newShimState()})
	t.Cleanup(srv.Close)
	return srv
}

func ask(t *testing.T, srv *httptest.Server, method, path, body, token string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// TestHealthzIsWhatTheAppPollsAfterAMachineStart is the one endpoint the app
// calls before it has any work to send: the machine is up, the process is
// listening, the door answers.
func TestHealthzIsWhatTheAppPollsAfterAMachineStart(t *testing.T) {
	srv := shimServer(t)
	status, body := ask(t, srv, http.MethodGet, "/healthz", "", "")
	if status != 200 {
		t.Fatalf("healthz answered %d: %s", status, body)
	}
	var answer map[string]bool
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer["ok"] {
		t.Errorf("healthz said %s", body)
	}
}

// TestAnUnknownRouteIs404 keeps the door small: three endpoints, and anything
// else is a mistake worth naming.
func TestAnUnknownRouteIs404(t *testing.T) {
	srv := shimServer(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/match"},
		{http.MethodPost, "/anything"},
		{http.MethodDelete, "/healthz"},
	} {
		status, body := ask(t, srv, c.method, c.path, "{}", "")
		if status != 404 {
			t.Errorf("%s %s answered %d: %s", c.method, c.path, status, body)
		}
	}
}

// TestTheTokenGatesEveryRequest is the cheap second wall (ADR 35). The private
// network is already org-scoped; this is what makes a mistake in that scoping
// cost nothing.
//
// **Absent, the token gates nothing** — which is what a laptop running the
// shim by hand wants, and what every test above relies on.
func TestTheTokenGatesEveryRequest(t *testing.T) {
	t.Setenv("MTGLAB_FORGE_SHIM_TOKEN", "s3cret")
	srv := shimServer(t)
	for _, c := range []struct {
		note, method, path, token string
		want                      int
	}{
		{"no token at all", http.MethodGet, "/healthz", "", 401},
		{"a wrong token", http.MethodGet, "/healthz", "Bearer wrong", 401},
		{"the right token", http.MethodGet, "/healthz", "Bearer s3cret", 200},
		{"a prefix of the right token", http.MethodGet, "/healthz", "Bearer s3c", 401},
		{"the scheme alone", http.MethodGet, "/healthz", "Bearer ", 401},
		{"the token with no scheme", http.MethodGet, "/healthz", "s3cret", 401},
		// **Every POST too**, which is the half a guard on GET alone would
		// miss — and /match is where the JVM minutes are.
		{"a POST with no token", http.MethodPost, "/coverage", "", 401},
		{"a POST with a wrong token", http.MethodPost, "/match", "Bearer wrong", 401},
	} {
		t.Run(c.note, func(t *testing.T) {
			status, body := ask(t, srv, c.method, c.path, "{}", c.token)
			if status != c.want {
				t.Errorf("answered %d, want %d: %s", status, c.want, body)
			}
		})
	}
}

// TestAnUnreadableBodyIs400 keeps a malformed ask apart from a failed match:
// one is the caller's, the other is the JVM's.
func TestAnUnreadableBodyIs400(t *testing.T) {
	srv := shimServer(t)
	for _, body := range []string{"not json", "[1,2,3]", `{"decks": 7}`, ""} {
		status, answer := ask(t, srv, http.MethodPost, "/coverage", body, "")
		if status != 400 && status != 503 {
			// 503 is legitimate on a machine with no Forge: the decks parsed
			// and the index could not be read. Anything else is a bug.
			t.Errorf("a body of %q answered %d: %s", body, status, answer)
		}
	}
}

// TestTheWatchdogJudgesWorkRatherThanTime is what stops the machine, and the
// property that keeps it from stopping mid-match: work in flight is never
// idle, however long it takes.
//
// Fly's proxy-driven auto-stop never sees private-network traffic, so this
// counter is the only thing that turns a finished match into a stopped
// machine — and the only thing standing between a four-minute match and a
// machine that exits underneath it.
func TestTheWatchdogJudgesWorkRatherThanTime(t *testing.T) {
	state := newShimState()
	state.lastActivity = time.Now().Add(-time.Hour)
	if state.idleFor() < time.Hour {
		t.Errorf("an hour of quiet read as %s", state.idleFor())
	}
	state.begin()
	if got := state.idleFor(); got != 0 {
		t.Errorf("work in flight read as %s idle", got)
	}
	state.end()
	if got := state.idleFor(); got > time.Second {
		t.Errorf("the clock did not restart when the work finished: %s", got)
	}
	// Two overlapping requests: the first to finish must not make the machine
	// idle while the second is still running.
	state.begin()
	state.begin()
	state.end()
	if got := state.idleFor(); got != 0 {
		t.Errorf("one of two in-flight requests finishing read as %s idle", got)
	}
	state.end()
}

// TestOneMatchAtATime is the worker's own invariant, and it is deliberately
// not inherited from the app's FORGE lane: that lane is a property of one
// process on another machine, and the `.dck` directory this process hands to
// Forge is racy under two JVMs.
func TestOneMatchAtATime(t *testing.T) {
	state := newShimState()
	state.match.Lock()
	locked := make(chan struct{})
	go func() {
		state.match.Lock()
		close(locked)
		state.match.Unlock()
	}()
	select {
	case <-locked:
		t.Fatal("two matches held the lock at once")
	case <-time.After(50 * time.Millisecond):
	}
	state.match.Unlock()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("the second match never got the lock")
	}
}

// TestFailureTextNamesTheClass is what a job's error field carries.
//
// The sentence is the client's; the class name is the half a maintainer reads
// off a job row to know whether Forge was missing, the match timed out, or the
// results were untrustworthy — three very different mornings.
func TestFailureTextNamesTheClass(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{tier3.NotInstalled("no Forge card data at /opt/forge"),
			"ForgeNotInstalled: no Forge card data at /opt/forge"},
		{errOf("plain"), "RuntimeError: plain"},
	} {
		if got := failureText(c.err); got != c.want {
			t.Errorf("failureText = %q, want %q", got, c.want)
		}
	}
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }

func errOf(s string) error { return simpleErr(s) }

// ---------------------------------------------------------------- the live half

// TestTheGoShimPlaysARealMatchForTheGoClient is the hosted path rehearsed
// whole, on one machine: the app's worker client, over a real socket, to the
// shim that runs on the worker, in front of a real JVM.
//
// **This is the closest thing to the deployed gate that a laptop can hold.**
// Everything between the route and Forge is the code the instance runs; the
// only thing standing in for Fly is the socket. Opt-in, because CI has no
// distribution — see the live test in `internal/sim/tier3`.
func TestTheGoShimPlaysARealMatchForTheGoClient(t *testing.T) {
	if os.Getenv("MTGLAB_LIVE_FORGE") != "1" {
		t.Skip("set MTGLAB_LIVE_FORGE=1 to run a real Forge match")
	}
	if _, err := tier3.DesktopJar(""); err != nil {
		t.Skipf("no Forge distribution: %v", err)
	}

	srv := shimServer(t)
	t.Setenv("MTGLAB_FORGE_WORKER_URL", srv.URL)
	if !tier3.Configured() {
		t.Fatal("a worker URL did not configure the hosted path")
	}

	var decks []*deck.Deck
	for _, name := range []string{"mono-green-clean", "kaheera"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "gate", "testdata", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := deck.FromText(string(raw), name)
		if err != nil {
			t.Fatal(err)
		}
		decks = append(decks, d)
	}

	worker := &tier3.Worker{Boot: 30 * time.Second, Sleep: func(d time.Duration) { time.Sleep(d) }}

	// The pre-flight first, exactly as the route does it — on the machine
	// where the card scripts live.
	reports, err := worker.CheckCoverage(t.Context(), decks)
	if err != nil {
		t.Fatalf("the hosted pre-flight failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("the worker sent %d reports, want 2", len(reports))
	}
	for _, r := range reports {
		if !r.OK() {
			t.Fatalf("%s", r.Summary())
		}
		t.Logf("%s", r.Summary())
	}

	var ticks []int
	seated := 0
	seed := int64(7)
	run, err := worker.RunMatch(t.Context(), decks, 2, 300, bigOf(seed),
		func(finished int, game *tier3.GameResult) {
			ticks = append(ticks, finished)
			if game != nil {
				seated++
			}
		})
	if err != nil {
		t.Fatalf("the hosted match failed: %v", err)
	}
	t.Logf("hosted: %d games in %.1fs, Forge %s, seats %v",
		len(run.Games()), run.WallSeconds, run.ForgeVersion, run.Seats)

	if len(run.Games()) != 2 {
		t.Fatalf("the worker played %d games, want 2", len(run.Games()))
	}
	// The bar ticked per game and each tick carried its row: the two halves
	// of the match theater, over a real socket.
	if len(ticks) != 2 || seated != 2 {
		t.Errorf("ticked %v with %d rows seated, want two of each", ticks, seated)
	}
	// The seats crossed, which is what lets a winner be named as a deck —
	// and by the deck's OWN slug: `mono-green-clean.yaml` declares
	// `slug: mono-green`, and a file's own slug wins over the location's
	// name (`raw.get("slug") or slug`). A test that expected the filename
	// here would be asserting the wrong half of that rule.
	if run.Seats[1] != decks[0].Slug || run.Seats[2] != decks[1].Slug {
		t.Errorf("the seats did not cross: %v, want %s and %s",
			run.Seats, decks[0].Slug, decks[1].Slug)
	}
	if run.ForgeVersion == "" {
		t.Error("the worker reported no Forge version")
	}
}

// bigOf is the seed as the engine carries it: Python's integers are unbounded
// and the seed is echoed back, so it travels as a big.Int rather than an int64.
func bigOf(n int64) *big.Int { return big.NewInt(n) }
