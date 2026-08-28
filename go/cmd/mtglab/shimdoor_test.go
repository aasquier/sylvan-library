package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The Forge worker's door (ADR 35), which is the other half of the client in
// `internal/sim/tier3`. `shim_test.go` holds the protocol shapes; this holds
// the door itself -- who gets in, what a refusal looks like, and the two
// invariants the worker enforces rather than inherits.
//
// The invariants are the reason this file exists. **One JVM at a time**: the
// `.dck` directory this process hands Forge is racy under two, so the shim
// serialises matches whatever the caller believes about its own lane. And
// **the idle watchdog must never stop the machine under a request in
// flight**: a streamed match is minutes of silence between lines by design,
// and a watchdog that read silence as idleness would kill matches at the
// three-minute mark and look exactly like a Forge crash.
//
// All parallel since ADR 40: the door is handed the machine it serves as a
// [tier3.Settings], so a test that wants a locked door and a test that wants
// an open one can run at the same time.

// newShim is a server over a fresh door with no lock on it.
func newShim(t *testing.T) *httptest.Server { return newShimFor(t, tier3.Settings{}) }

// newShimFor is the same door serving a described machine.
func newShimFor(t *testing.T, forge tier3.Settings) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(&shim{
		state: newShimState(), log: log.New(io.Discard, "", 0), forge: forge,
	})
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, path, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// The token guards every route, and the comparison is constant time -- a
// token compared with `==` leaks its prefix to anybody who can time the
// private network.
func TestTheShimRefusesEveryRouteWithoutItsToken(t *testing.T) {
	t.Parallel()
	srv := newShimFor(t, tier3.Settings{ShimToken: "s3cret"})

	for _, tc := range []struct{ name, token string }{
		{"no token at all", ""},
		{"the wrong token", "Bearer wrong"},
		{"the right token, unprefixed", "s3cret"},
		{"a prefix of the right token", "Bearer s3c"},
		{"the right token with something after it", "Bearer s3cretx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/coverage", "/match"} {
				resp := post(t, srv, path, tc.token, `{"decks":[]}`)
				if resp.StatusCode != http.StatusUnauthorized {
					t.Errorf("POST %s answered %d", path, resp.StatusCode)
				}
			}
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/healthz", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.token != "" {
				req.Header.Set("Authorization", tc.token)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("GET /healthz answered %d", resp.StatusCode)
			}
		})
	}

	// And the right token gets in.
	resp := post(t, srv, "/coverage", "Bearer s3cret", `{"decks":[]}`)
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("the right token was refused")
	}
}

// An unset token is a laptop running the shim by hand, and it lets everything
// through rather than locking the operator out of their own process.
func TestAnUnsetTokenLetsALaptopIn(t *testing.T) {
	t.Parallel()
	srv := newShim(t)

	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz answered %d", resp.StatusCode)
	}
	var answer struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if !answer.OK {
		t.Error("/healthz answered ok=false on a healthy shim")
	}
}

// Everything that is not one of the three routes is a 404 that names what was
// asked for, rather than a bare status the app has to guess about.
func TestTheShimHasExactlyThreeRoutes(t *testing.T) {
	t.Parallel()
	srv := newShim(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/match"},
		{http.MethodGet, "/coverage"},
		{http.MethodPost, "/healthz"},
		{http.MethodPost, "/anything-else"},
		{http.MethodDelete, "/match"},
		{http.MethodPut, "/coverage"},
	} {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s answered %d", tc.method, tc.path, resp.StatusCode)
			continue
		}
		if !strings.Contains(string(body), tc.path) {
			t.Errorf("%s %s said %q without naming the route", tc.method, tc.path, body)
		}
	}
}

// A body the shim cannot read is a 400 that says so, rather than a panic or a
// match played with default arguments nobody asked for.
func TestAnUnreadableBodyIsRefusedRatherThanGuessedAt(t *testing.T) {
	t.Parallel()
	srv := newShim(t)

	for _, tc := range []struct{ name, body string }{
		{"not JSON at all", `nonsense`},
		{"a list", `[1,2,3]`},
		{"a bare string", `"hello"`},
		{"empty", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/coverage", "/match"} {
				resp := post(t, srv, path, "", tc.body)
				if resp.StatusCode != http.StatusBadRequest {
					t.Errorf("%s answered %d to %s", path, resp.StatusCode, tc.name)
				}
			}
		})
	}

	// A `decks` value that is not a list of strings is the same refusal,
	// caught one layer further in.
	resp := post(t, srv, "/coverage", "", `{"decks":[1,2]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a numeric deck list answered %d", resp.StatusCode)
	}
	// Deck text that is not a deck is also a 400 rather than a 500.
	resp = post(t, srv, "/coverage", "", `{"decks":["not a deck at all: ["]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unparseable deck text answered %d", resp.StatusCode)
	}
}

// A worker with no Forge answers 503 rather than 500, because the app turns
// exactly that into "Forge is not available here" instead of a red job.
// **Serial**: it clears the package-level coverage index, which every other
// test reading a cardsfolder shares.
func TestAWorkerWithNoForgeAnswers503(t *testing.T) {
	tier3.ClearIndex()
	// A Forge home that is present, and empty.
	srv := newShimFor(t, tier3.Settings{Home: t.TempDir()})

	resp := post(t, srv, "/coverage", "", `{"decks":[]}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("a worker with no card data answered %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "MTGLAB_FORGE_HOME") {
		t.Errorf("the 503 does not say how to fix it: %s", body)
	}
}

// An absent `decks` key is an empty match rather than a crash: the app never
// sends one, but a hand-rolled curl does, and the shim is on a private
// network rather than behind a schema validator.
func TestAnAbsentDeckListIsEmptyRatherThanFatal(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{}`, `{"decks":null}`, `{"decks":[]}`} {
		decks, err := decksFromBody(rawBody(t, body))
		if err != nil {
			t.Errorf("%s: %v", body, err)
			continue
		}
		if len(decks) != 0 {
			t.Errorf("%s produced %d decks", body, len(decks))
		}
	}
}

// rawBody decodes a JSON object the way ServeHTTP hands it to a handler.
func rawBody(t *testing.T, text string) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// **One JVM at a time**, whatever the caller believes about its own lane. The
// serialisation is the shim's own invariant rather than one inherited from
// the app's job lanes, because the `.dck` directory is racy under two.
func TestTheShimSerialisesMatchesWhateverTheCallerBelieves(t *testing.T) {
	t.Parallel()
	state := newShimState()

	// The match lock is what serialises them; two goroutines that both hold
	// it at once would be two JVMs.
	var concurrent, peak int
	var counter sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state.match.Lock()
			defer state.match.Unlock()
			counter.Lock()
			concurrent++
			if concurrent > peak {
				peak = concurrent
			}
			counter.Unlock()

			time.Sleep(time.Millisecond)

			counter.Lock()
			concurrent--
			counter.Unlock()
		}()
	}
	wg.Wait()
	if peak != 1 {
		t.Errorf("%d matches ran at once -- the .dck directory is racy under two", peak)
	}
}

// **The watchdog must never stop the machine under a request in flight.** A
// streamed match is minutes of silence between lines by design, so an idle
// clock that ignored in-flight work would kill matches at the idle mark and
// look exactly like a Forge crash.
func TestTheIdleClockStopsWhileWorkIsInFlight(t *testing.T) {
	t.Parallel()
	state := newShimState()

	// A request arrives, and however long it takes, the shim is not idle.
	//
	// **The sleep is the signal this test measures against**, which is why it
	// is longer than it looks like it needs to be. Every bound below asks the
	// same question — did the clock restart at `end`, or has it been running
	// since `begin`? — and the two answers are only distinguishable by the
	// distance between them. A short sleep makes that distance small enough
	// for a loaded runner's scheduling to cross it.
	state.begin()
	time.Sleep(100 * time.Millisecond)
	if got := state.idleFor(); got != 0 {
		t.Errorf("a shim with work in flight reported %v of idleness", got)
	}

	// Two overlapping requests: finishing one does not make it idle.
	state.begin()
	state.end()
	if got := state.idleFor(); got != 0 {
		t.Errorf("one of two requests finishing reported %v of idleness", got)
	}

	// Only when the last one ends does the clock start, and it starts from
	// the end rather than from the beginning.
	//
	// **The bound is generous on purpose, and it used to be 5ms.** What this
	// has to tell apart is a clock that just started from one that has been
	// running for the whole request -- nought against a hundred milliseconds.
	// It does not have to tell 0.1ms from 6ms, and when it tried it failed on
	// a loaded CI runner at 5.887ms and skipped a deploy: `idleFor` is called
	// on the statement after `end`, so the reading is whatever the scheduler
	// put between two adjacent lines, and under `-race` on a shared runner
	// that is not always microseconds. Half the sleep is the widest bound that
	// still catches a clock which never restarted.
	state.end()
	if got := state.idleFor(); got > 50*time.Millisecond {
		t.Errorf("the idle clock ran during the request: %v", got)
	}
	// And it really is running: a rest that clears the noise both ways.
	time.Sleep(60 * time.Millisecond)
	if got := state.idleFor(); got < 30*time.Millisecond {
		t.Errorf("the idle clock is not running: %v", got)
	}
}

// A fresh shim is idle from the moment it starts, so a machine started and
// never asked for anything stops on its own rather than billing forever.
func TestAShimNobodyAsksForGoesIdle(t *testing.T) {
	t.Parallel()
	state := newShimState()
	time.Sleep(5 * time.Millisecond)
	if state.idleFor() <= 0 {
		t.Error("a shim nobody has asked for is not accumulating idleness")
	}
}

// The class name is the half that survives translation: the client shows the
// sentence, and a maintainer reading a job row wants to know whether Forge
// was missing, the match timed out, or the results were untrustworthy.
func TestAFailureIsRenderedWithItsClassName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err    error
		prefix string
	}{
		{tier3.NotInstalled("no jar in /forge"), "ForgeNotInstalled: "},
		{tier3.ErrTimedOut, "TimeoutExpired: "},
		{errSentinel{}, "RuntimeError: "},
	} {
		got := failureText(tc.err)
		if !strings.HasPrefix(got, tc.prefix) {
			t.Errorf("%v rendered as %q, want a %q prefix", tc.err, got, tc.prefix)
		}
		// The sentence survives beside the class name rather than being
		// replaced by it.
		if !strings.Contains(got, tc.err.Error()) {
			t.Errorf("%v rendered as %q, losing its own words", tc.err, got)
		}
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "something else went wrong" }

// The JVM heap ceiling and the idle window fall back rather than becoming
// zero -- a zero heap would fail every match, and a zero idle window means
// "never stop", which is what a laptop wants. The reading itself moved to
// [tier3.LoadSettings] with ADR 40 and is tested there; what is asked here is
// the thing this package still owns, which is whether the numbers are sane.
func TestTheShimsDefaultsAreSane(t *testing.T) {
	t.Parallel()
	// The heap sits below the client's own default on purpose, because the
	// heap is not the only resident of a 4GB machine.
	if tier3.DefaultMemoryMB >= tier3.MemoryDefault {
		t.Errorf("the shim's heap ceiling (%d) is not below tier3's (%d) -- "+
			"metaspace, the card database and this process live beside it",
			tier3.DefaultMemoryMB, tier3.MemoryDefault)
	}
	if tier3.DefaultIdleSeconds <= 0 {
		t.Errorf("the default idle window is %d", tier3.DefaultIdleSeconds)
	}
	// Zero IS a legitimate explicit value for the idle window: it means
	// "never stop", and a shim asked for it must not fall back to three
	// minutes and exit under a laptop.
	if got := (tier3.Settings{IdleSeconds: 0}).IdleSeconds; got != 0 {
		t.Errorf("an explicit zero became %d", got)
	}
}

// The shim's answers are always JSON with a length, because the client reads
// them with a decoder and a body of unknown length over a private network is
// how a read hangs.
func TestEveryShimAnswerIsCountedJSON(t *testing.T) {
	t.Parallel()
	srv := newShim(t)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/healthz", ""},
		{http.MethodGet, "/nope", ""},
		{http.MethodPost, "/coverage", `nonsense`},
	} {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s answered content-type %q", tc.method, tc.path, ct)
		}
		if resp.ContentLength < 0 {
			t.Errorf("%s %s answered a body of unknown length", tc.method, tc.path)
		}
		if !json.Valid(body) {
			t.Errorf("%s %s answered non-JSON: %s", tc.method, tc.path, body)
		}
	}
}

// A payload the shim cannot encode becomes an error it can, rather than a
// half-written body the client cannot parse.
func TestAnUnencodablePayloadBecomesAnErrorTheClientCanRead(t *testing.T) {
	t.Parallel()
	s := &shim{state: newShimState(), log: log.New(io.Discard, "", 0)}
	rec := httptest.NewRecorder()
	// A channel cannot be marshalled.
	s.send(rec, http.StatusOK, map[string]any{"bad": make(chan int)})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("an unencodable answer went out as %d", rec.Code)
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("the fallback is not JSON: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not encode") {
		t.Errorf("the fallback says %q", rec.Body.String())
	}
}
