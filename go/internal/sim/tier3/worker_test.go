package tier3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The app's side of the hosted Forge (ADR 35), driven against a stub shim
// over real HTTP.
//
// `MTGLAB_FORGE_WORKER_URL` exists precisely so this is possible: pointed at
// a running shim, the client skips the Machines API and talks straight to it.
// So these tests exercise the real request building, the real streaming
// reader and the real error classification -- everything except Fly's control
// plane, which is asked for separately below with a stubbed transport.
//
// **These run in parallel now, and one of them could not have existed before.**
// Until ADR 40 the client read `MTGLAB_FORGE_WORKER_URL` from the process, so
// a stub shim published its address into the single global slot the process
// has -- which made every test here serial, and made *two* stub shims in one
// test impossible. [TestTheShimTokenRidesOnEveryRequest] wants exactly that:
// one shim that demands a token and one that does not, running at once. It
// now gets it, because a [Worker] carries the [Settings] naming its own shim.
//
// The distinction that matters most here is **503 versus everything else**: a
// shim saying "no distribution" has to stay [ErrForgeNotInstalled] all the way
// up so the route answers 503, while a shim that failed mid-match is a job
// error the caller records. Collapsing the two would turn "Forge is not
// installed" into a red job, or a real crash into a soothing "not available".

// stubShim is a worker shim: `/healthz`, `/coverage` and `/match`, each
// answering whatever the test installs.
type stubShim struct {
	*httptest.Server
	mu       sync.Mutex
	health   func(w http.ResponseWriter, r *http.Request)
	coverage func(w http.ResponseWriter, r *http.Request)
	match    func(w http.ResponseWriter, r *http.Request)
	seen     []string // the Authorization headers the shim was sent
}

func newStubShim(t *testing.T) *stubShim {
	t.Helper()
	s := &stubShim{}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		s.mu.Lock()
		h := s.health
		s.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("/coverage", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		s.mu.Lock()
		h := s.coverage
		s.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"reports":[]}`)
	})
	mux.HandleFunc("/match", func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		s.mu.Lock()
		h := s.match
		s.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// worker is a client pointed at this shim and nothing else, which does not
// actually sleep between health polls -- so a test of the retry loop costs
// wall-clock nothing. `token` is what the shim will demand; empty is a door
// with no lock.
func (s *stubShim) worker(boot time.Duration, token string) *Worker {
	return &Worker{
		Settings: Settings{WorkerURL: s.URL, ShimToken: token},
		Boot:     boot,
		Sleep:    func(time.Duration) {},
	}
}

func (s *stubShim) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, r.Header.Get("Authorization"))
}

func (s *stubShim) headers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *stubShim) on(which string, h func(http.ResponseWriter, *http.Request)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch which {
	case "healthz":
		s.health = h
	case "coverage":
		s.coverage = h
	case "match":
		s.match = h
	}
}

// flyWorker talks to Fly's control plane through a stub transport, with the
// deployment described rather than exported.
func flyWorker(fly *httptest.Server, s Settings) *Worker {
	w := &Worker{Settings: s, HTTP: fly.Client(), Sleep: func(time.Duration) {}}
	w.HTTP.Transport = rewriteTo(fly.URL)
	return w
}

func testDeck(slug string) *deck.Deck {
	return &deck.Deck{Slug: slug, Name: slug, Commander: []string{"Sol Ring"},
		Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}}
}

// The dial is a fact about the environment, answered without any network --
// the same contract `/api/claude` set, where reachability is discovered when
// work is actually asked for.
func TestTheWorkerIsConfiguredByTheEnvironmentAlone(t *testing.T) {
	for _, c := range []struct {
		what string
		s    Settings
		want bool
	}{
		{"an unconfigured environment", Settings{}, false},
		// The dial alone is not enough: without a token there is nothing to
		// start the machine with.
		{"the dial without a token", Settings{WorkerEnabled: true}, false},
		{"a token without the dial", Settings{FlyAPIToken: "tok"}, false},
		{"the dial and a token", Settings{WorkerEnabled: true, FlyAPIToken: "tok"}, true},
		// A direct URL skips the Machines API entirely, so it is sufficient on
		// its own -- this is how a laptop talks to a hand-started shim.
		{"a direct worker URL", Settings{WorkerURL: "http://127.0.0.1:9999"}, true},
	} {
		if got := c.s.Configured(); got != c.want {
			t.Errorf("%s reported configured=%v, want %v", c.what, got, c.want)
		}
	}

	// Whitespace is not a dial: an empty-looking value must not read as on.
	// That is a fact about the *reader*, which is what keeps this one serial.
	t.Setenv("MTGLAB_FORGE_WORKER", "   ")
	t.Setenv("MTGLAB_FLY_API_TOKEN", "tok")
	t.Setenv("MTGLAB_FORGE_WORKER_URL", "")
	if LoadSettings().Configured() {
		t.Error("a whitespace dial read as on")
	}
}

// A direct URL is used as given, minus a trailing slash -- otherwise every
// path would be requested with a double slash.
func TestADirectWorkerURLSkipsTheMachinesAPI(t *testing.T) {
	t.Parallel()
	// No transport at all: if this reached the Machines API it would fail
	// rather than quietly succeed, which is the assertion. Trimming the
	// trailing slash is [LoadSettings]'s job and is asked of the reader, in
	// `TestTheOverridesWinAndTheFallbacksAgreeOnTheLayout`.
	base, err := (&Worker{
		Settings: Settings{WorkerURL: "http://shim.internal:8080"},
	}).BaseURL(context.Background())
	if err != nil {
		t.Fatalf("a direct URL was refused: %v", err)
	}
	if base != "http://shim.internal:8080" {
		t.Errorf("the base URL is %q", base)
	}
}

// The shim's token rides on every request when it is set, and the header is
// simply absent when it is not.
func TestTheShimTokenRidesOnEveryRequest(t *testing.T) {
	t.Parallel()
	// Two shims, running at the same time, in one test. Before ADR 40 the
	// client read the shim's address out of the process, so there was exactly
	// one slot for it and this had to be two sequential halves that each
	// rewrote the environment. Now each worker carries its own.
	locked, open := newStubShim(t), newStubShim(t)

	if _, err := locked.worker(time.Second, "s3cret").Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := open.worker(time.Second, "").Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, got := range locked.headers() {
		if got != "Bearer s3cret" {
			t.Errorf("a request to the locked shim carried %q", got)
		}
	}
	for _, got := range open.headers() {
		if got != "" {
			t.Errorf("an unset token still sent %q", got)
		}
	}
}

// Ready polls until the shim answers. A shim that is still booting says so
// and the poll waits; a shim that never answers becomes a 503 that carries
// the last thing it said.
func TestReadyWaitsForTheShimToComeUp(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	var calls int
	shim.on("healthz", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			_, _ = io.WriteString(w, `{"ok":false}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	base, err := shim.worker(10*time.Second, "").Ready(context.Background())
	if err != nil {
		t.Fatalf("a shim that came up late was refused: %v", err)
	}
	if base != shim.URL {
		t.Errorf("Ready answered %q", base)
	}
	if calls != 3 {
		t.Errorf("the poll gave up after %d tries", calls)
	}
}

func TestAShimThatNeverAnswersBecomesANotInstalledRefusal(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("healthz", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "still unpacking", http.StatusServiceUnavailable)
	})

	_, err := shim.worker(50*time.Millisecond, "").Ready(context.Background())
	if err == nil {
		t.Fatal("a dead shim was reported ready")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("the refusal is %T, want ErrForgeNotInstalled so the route answers 503", err)
	}
	if !strings.Contains(err.Error(), "never answered") {
		t.Errorf("the refusal said %q", err)
	}
	// The last thing the shim said rides along, because it is the only clue
	// anyone gets.
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("the refusal dropped the shim's own words: %q", err)
	}
}

// A health endpoint that answers something other than the agreed shape is
// not healthy, rather than being read as healthy by default.
func TestAnUnreadableHealthAnswerIsNotHealthy(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `this is not json`)
	})
	if _, err := shim.worker(50*time.Millisecond, "").Ready(context.Background()); err == nil {
		t.Fatal("a shim answering garbage was reported ready")
	}
}

// The pre-flight is computed where the card scripts live and re-raised here,
// so the route's 422 does not care which machine read the zip.
func TestThePreFlightCrossesTheWireAndComesBackTheSameShape(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("coverage", func(w http.ResponseWriter, r *http.Request) {
		// The deck crossed as `deck.yaml` text, because `deck.FromText` is
		// the one parser (ADR 4).
		var payload struct {
			Decks []string `json:"decks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(payload.Decks) != 1 || !strings.Contains(payload.Decks[0], "slug: gyome") {
			http.Error(w, "the deck did not cross as its own yaml", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"reports":[{"slug":"gyome","checked":2,`+
			`"resolved":{"Sol Ring":"Sol Ring"},"missing":[]}]}`)
	})

	reports, err := shim.worker(time.Second, "").CheckCoverage(context.Background(),
		[]*deck.Deck{testDeck("gyome")})
	if err != nil {
		t.Fatalf("a covered deck failed the remote pre-flight: %v", err)
	}
	if len(reports) != 1 || reports[0].Slug != "gyome" || reports[0].Checked != 2 {
		t.Fatalf("the report came back as %+v", reports)
	}
}

// A card the worker's Forge lacks fails the same way a local pre-flight
// fails, because coverage is checked before and after precisely so a dropped
// card is never silent.
func TestARemotePreFlightFailureIsStillACoverageFailure(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("coverage", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"reports":[{"slug":"gyome","checked":2,`+
			`"resolved":{},"missing":["Nonexistent Card"]}]}`)
	})

	_, err := shim.worker(time.Second, "").CheckCoverage(context.Background(),
		[]*deck.Deck{testDeck("gyome")})
	if !errors.Is(err, ErrCoverageFailed) {
		t.Fatalf("a missing card failed as %v, want ErrCoverageFailed", err)
	}
	if !strings.Contains(err.Error(), "Nonexistent Card") {
		t.Errorf("the failure did not name the card: %q", err)
	}
}

// **503 versus everything else.** A shim saying "no distribution" must stay
// ErrForgeNotInstalled all the way up so the route answers 503; anything else
// is a runtime failure the job records.
func TestAShimRefusalKeepsItsClassAllTheWayUp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		code         int
		body         string
		notInstalled bool
		wants        string
	}{
		{"no distribution", 503, `{"error":"no Forge desktop jar in /forge"}`,
			true, "no Forge desktop jar"},
		{"a crash", 500, `{"error":"the JVM died"}`, false, "the JVM died"},
		{"a bad ask", 400, `{"error":"two decks minimum"}`, false, "two decks minimum"},
		{"a refusal with no words", 500, `not json at all`, false, "shim answered 500"},
		{"a 503 with no words", 503, ``, true, "shim answered 503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shim := newStubShim(t)
			shim.on("coverage", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := shim.worker(time.Second, "").CheckCoverage(context.Background(),
				[]*deck.Deck{testDeck("gyome")})
			if err == nil {
				t.Fatal("the refusal was not raised")
			}
			if got := errors.Is(err, ErrForgeNotInstalled); got != tc.notInstalled {
				t.Errorf("ErrForgeNotInstalled=%v, want %v -- the route's status "+
					"turns on this", got, tc.notInstalled)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("the refusal said %q, want something containing %q", err, tc.wants)
			}
		})
	}
}

// A streaming match: a tick per game as it finishes, then the result. The
// callback is the same one a local run takes, so the API cannot tell the two
// paths apart.
func TestAStreamedMatchTicksPerGameAndRebuildsTheRun(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Games  int    `json:"games"`
			Clock  int    `json:"clock"`
			Seed   *int64 `json:"seed"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !payload.Stream {
			http.Error(w, "the ask did not carry stream:true", http.StatusBadRequest)
			return
		}
		if payload.Seed == nil || *payload.Seed != 42 {
			http.Error(w, "the seed did not cross", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		lines := []string{
			`{"game":1,"row":{"index":0,"milliseconds":1200,"draw":false,` +
				`"winner":"gyome","winner_seat":1,"turns":9,"timed_out":false}}`,
			// A pre-theater shim sends the count alone and no row.
			`{"game":2}`,
			`{"result":{"games":[` +
				`{"index":0,"milliseconds":1200,"draw":false,"winner":"gyome",` +
				`"winner_seat":1,"turns":9,"timed_out":false},` +
				`{"index":1,"milliseconds":800,"draw":true,"winner":null,` +
				`"winner_seat":null,"turns":12,"timed_out":false}],` +
				`"seats":{"1":"gyome","2":"trostani"},"wall_seconds":4.5,` +
				`"forge_version":"1.6.50"}}`,
		}
		for _, line := range lines {
			_, _ = io.WriteString(w, line+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	type tick struct {
		finished int
		hasRow   bool
	}
	var ticks []tick
	seed := big.NewInt(42)
	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome"), testDeck("trostani")},
		MatchAsk{Games: 2, Clock: 300, Seed: seed,
			OnGame: func(finished int, g *GameResult) {
				ticks = append(ticks, tick{finished, g != nil})
			}})
	if err != nil {
		t.Fatalf("the match failed: %v", err)
	}

	if len(ticks) != 2 {
		t.Fatalf("the bar ticked %d times, want 2: %v", len(ticks), ticks)
	}
	if !ticks[0].hasRow {
		t.Error("the first tick carried no row -- the theater has nothing to seat")
	}
	// A pre-theater shim's row-less tick still ticks the bar.
	if ticks[1].hasRow {
		t.Error("a row-less line invented a row")
	}

	if len(run.Games()) != 2 {
		t.Fatalf("the rebuilt run has %d games", len(run.Games()))
	}
	if run.WallSeconds != 4.5 {
		t.Errorf("wall seconds came back as %v", run.WallSeconds)
	}
	if run.ForgeVersion != "1.6.50" {
		t.Errorf("the Forge version is %q -- the ledger records it (ADR 36)", run.ForgeVersion)
	}
	// Seats map back to slugs, which is how a winner gets a name.
	if got := run.WinnerSlug(run.Games()[0]); got != "gyome" {
		t.Errorf("the winner is %q", got)
	}
	if got := run.WinnerSlug(run.Games()[1]); got != "" {
		t.Errorf("a draw named %q as the winner", got)
	}
	// Startup is wall clock minus what was spent inside games.
	if got := run.StartupSeconds(); got != 2.5 {
		t.Errorf("startup is %v, want 4.5 - 2.0", got)
	}
}

// A shim from before the streaming flag ignores it and answers one plain JSON
// body. The Content-Type is what says which conversation this is, and both
// are accepted so a deploy that updates the app before its worker never
// breaks a match over it.
func TestAPreStreamingShimIsStillUnderstood(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"games":[{"index":0,"milliseconds":1000,`+
			`"draw":false,"winner":"gyome","winner_seat":1,"turns":7,`+
			`"timed_out":false}],"seats":{"1":"gyome"},"wall_seconds":2.0}`)
	})

	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome")},
		MatchAsk{Games: 1, Clock: 300, Seed: big.NewInt(1)})
	if err != nil {
		t.Fatalf("an old shim's answer was refused: %v", err)
	}
	if len(run.Games()) != 1 || run.Seats[1] != "gyome" {
		t.Fatalf("the flat answer rebuilt as %+v", run)
	}
	// An old shim omits the version, and that degrades to "not reported"
	// rather than to an error.
	if run.ForgeVersion != "" {
		t.Errorf("an old shim reported version %q", run.ForgeVersion)
	}
}

// A flat answer that is not a run at all is refused rather than rebuilt into
// an empty match that would look like a legitimate zero-game result.
func TestAnUnreadableFlatAnswerIsRefused(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `["not", "a", "run"]`)
	})
	_, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome")},
		MatchAsk{Games: 1, Clock: 300, Seed: big.NewInt(1)})
	if err == nil {
		t.Fatal("a nonsense answer rebuilt into a run")
	}
	if !strings.Contains(err.Error(), "unexpected answer") {
		t.Errorf("the refusal said %q", err)
	}
}

// The stream's own failure modes: an error line, a not-installed error line,
// an unreadable line, and a stream that simply stops.
func TestTheMatchStreamsFailureModes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		lines        string
		notInstalled bool
		wants        string
	}{
		{"an error line", `{"game":1}` + "\n" + `{"error":"the JVM died"}` + "\n",
			false, "the JVM died"},
		{"a not-installed error line",
			`{"error":"no Forge desktop jar","type":"ForgeNotInstalled"}` + "\n",
			true, "no Forge desktop jar"},
		{"an unreadable line", "{not json}\n", false, "unreadable line"},
		{"no result at all", `{"game":1}` + "\n", false, "ended without a result"},
		{"nothing at all", "", false, "ended without a result"},
		{"blank lines only", "\n\n\n", false, "ended without a result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shim := newStubShim(t)
			shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, tc.lines)
			})
			_, err := shim.worker(time.Second, "").RunMatch(context.Background(),
				[]*deck.Deck{testDeck("gyome")},
				MatchAsk{Games: 1, Clock: 300, Seed: big.NewInt(1)})
			if err == nil {
				t.Fatal("the stream's failure was not raised")
			}
			if got := errors.Is(err, ErrForgeNotInstalled); got != tc.notInstalled {
				t.Errorf("ErrForgeNotInstalled=%v, want %v", got, tc.notInstalled)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("the failure said %q, want something containing %q", err, tc.wants)
			}
		})
	}
}

// A stream that goes quiet for longer than one game plus the JVM's boot is
// cancelled, rather than blocking forever on a worker that died mid-match.
func TestAStreamThatGoesQuietIsCancelledRatherThanWaitedOnForever(t *testing.T) {
	t.Parallel()
	// The reader is driven directly: a pipe that is never written to is a
	// worker that stopped talking, and the stall callback is what the real
	// path uses to cancel the request.
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	stalled := make(chan struct{})
	var once sync.Once
	_, err := readStream(pr, 20*time.Millisecond, func() {
		once.Do(func() { close(stalled) })
		_ = pr.CloseWithError(errors.New("stalled"))
	}, MatchAsk{})
	if err == nil {
		t.Fatal("a silent stream was waited on forever")
	}
	select {
	case <-stalled:
	default:
		t.Error("the stall callback never fired")
	}
}

// handTimer is [stallTimer] on a clock the test turns, and the stall it fires
// is the real one — the callback [readStreamOn] was handed.
type handTimer struct {
	now      time.Duration
	deadline time.Duration
	stall    func()
	fired    bool
	stopped  bool
	resets   int
}

// arm is the constructor [readStreamOn] takes.
func (h *handTimer) arm(d time.Duration, stall func()) stallTimer {
	h.deadline, h.stall = h.now+d, stall
	return h
}

// pass moves the clock forward and fires the stall if the deadline went by,
// which is the whole of what `time.AfterFunc` would have done for us.
func (h *handTimer) pass(d time.Duration) {
	h.now += d
	if !h.fired && !h.stopped && h.now >= h.deadline {
		h.fired = true
		h.stall()
	}
}

func (h *handTimer) Reset(d time.Duration) bool {
	h.resets++
	h.deadline = h.now + d
	return true
}

func (h *handTimer) Stop() bool { h.stopped = true; return true }

// pacedReader hands back one line per read and lets `gap` pass before each,
// so **the stream's own pacing is the clock** and there is nothing running
// beside it to race. A fired stall reads as the closed pipe the real
// cancellation produces.
type pacedReader struct {
	lines []string
	gap   time.Duration
	clock *handTimer
	i     int
}

func (p *pacedReader) Read(b []byte) (int, error) {
	if p.i >= len(p.lines) {
		return 0, io.EOF
	}
	p.clock.pass(p.gap)
	if p.clock.fired {
		return 0, errors.New("io: read/write on closed pipe")
	}
	n := copy(b, p.lines[p.i])
	p.i++
	return n, nil
}

// The stall timer is reset on every line, so a stream that keeps talking runs
// as long as it likes -- the property that makes this a per-read budget
// rather than a second, shorter copy of the far side's whole-match ceiling.
//
// **The clock is turned by hand, because the assertion is about a budget.**
// This used to pace the lines with `time.Sleep(15 * time.Millisecond)` against
// a 60ms budget, the sum deliberately larger than the budget while no single
// gap was. That reads as an assertion about the reset and is really an
// assertion about the machine's scheduler: a starved one stretches a single
// 15ms sleep past 60ms unaided, the timer fires, and the test reports **"a
// chatty stream was cancelled"** about a stream that was perfectly chatty.
// Measured under load it failed 6 runs in 20, and it had failed on several
// unrelated changes in one day. Nothing sleeps here now — the same numbers
// prove the same thing on any machine, in no time at all.
func TestTheStallBudgetIsPerReadRatherThanPerMatch(t *testing.T) {
	t.Parallel()
	const budget, gap = 60 * time.Millisecond, 15 * time.Millisecond

	lines := make([]string, 0, 6)
	for i := 1; i <= 5; i++ {
		lines = append(lines, fmt.Sprintf(`{"game":%d}`+"\n", i))
	}
	lines = append(lines, `{"result":{"games":[],"seats":{},"wall_seconds":1.0}}`+"\n")

	clock := &handTimer{}
	body := &pacedReader{lines: lines, gap: gap, clock: clock}
	ticks := 0
	run, err := readStreamOn(body, budget, func() {},
		MatchAsk{OnGame: func(int, *GameResult) { ticks++ }}, clock.arm)
	if err != nil {
		t.Fatalf("a chatty stream was cancelled: %v", err)
	}
	if ticks != 5 {
		t.Errorf("the bar ticked %d times, want 5", ticks)
	}
	if run == nil {
		t.Fatal("no run came back")
	}
	if clock.fired {
		t.Error("the stall fired on a stream that never stopped talking")
	}
	// **The test is only saying anything while this holds.** The point is a
	// total beyond the budget with no single gap near it; a later edit that
	// trims a line or widens the budget could leave every assertion above
	// passing and nothing being proved.
	if clock.now <= budget {
		t.Errorf("the stream ran %v against a %v budget, so nothing here "+
			"distinguishes a per-read budget from a per-match one",
			clock.now, budget)
	}
	if clock.resets != len(lines) {
		t.Errorf("the deadline was re-armed %d times for %d lines",
			clock.resets, len(lines))
	}
}

// Without a Fly token there is nothing to start a machine with, and that is a
// 503 rather than a panic on an empty header.
func TestTheMachinesAPINeedsATokenAndAnAppName(t *testing.T) {
	t.Parallel()
	// The app name is asked for first, because there is no URL to call
	// without one.
	_, err := (&Worker{Settings: Settings{FlyAPIToken: "tok"}}).BaseURL(context.Background())
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("no app name failed as %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "FLY_APP_NAME") {
		t.Errorf("no app name said %q", err)
	}

	_, err = (&Worker{Settings: Settings{FlyApp: "mtglab"}}).BaseURL(context.Background())
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("no token failed as %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "MTGLAB_FLY_API_TOKEN") {
		t.Errorf("the refusal said %q", err)
	}
}

// Fly injects FLY_APP_NAME into every machine; MTGLAB_FLY_APP overrides it,
// for tests and for talking to the instance from a laptop. Which of the two
// won is a question about the reader, so it is asked of the reader.
func TestTheFlyAppOverrideWinsOverFlysOwnInjection(t *testing.T) {
	t.Setenv("FLY_APP_NAME", "mtglab")
	t.Setenv("MTGLAB_FLY_APP", "")
	if got := LoadSettings().FlyApp; got != "mtglab" {
		t.Errorf("Fly's own injection lost: %q", got)
	}
	t.Setenv("MTGLAB_FLY_APP", "somewhere-else")
	if got := LoadSettings().FlyApp; got != "somewhere-else" {
		t.Errorf("the override lost: %q", got)
	}
}

// Creation is deliberately not the client's job: pulling a ~1GB image is
// minutes, and the deploy workflow does it. A missing machine says so rather
// than provisioning infrastructure from a request thread.
func TestAMissingMachineIsReportedRatherThanCreated(t *testing.T) {
	t.Parallel()
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("the client tried to %s %s -- creation is not its job", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"id":"abc","name":"something-else","state":"stopped"}]`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "tok", FlyApp: "mtglab",
		Machine: DefaultMachine})
	_, err := w.BaseURL(context.Background())
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("a missing machine failed as %v", err)
	}
	for _, want := range []string{"forge-worker", "mtglab", "deploy workflow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %q", want, err)
		}
	}
}

// A stopped machine is started and waited for, and the base URL is built
// from the machine's own id on the private network.
func TestAStoppedMachineIsStartedAndWaitedFor(t *testing.T) {
	t.Parallel()
	var paths []string
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/machines"):
			_, _ = io.WriteString(w, `[{"id":"d891","name":"forge-worker","state":"stopped"}]`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "tok", FlyApp: "mtglab",
		Machine: DefaultMachine, ShimPort: DefaultShimPort})
	base, err := w.BaseURL(context.Background())
	if err != nil {
		t.Fatalf("starting the machine: %v", err)
	}
	if base != "http://d891.vm.mtglab.internal:8080" {
		t.Errorf("the base URL is %q", base)
	}
	joined := strings.Join(paths, " | ")
	if !strings.Contains(joined, "POST /v1/apps/mtglab/machines/d891/start") {
		t.Errorf("the machine was never started: %s", joined)
	}
	if !strings.Contains(joined, "GET /v1/apps/mtglab/machines/d891/wait") {
		t.Errorf("the client never waited for the state: %s", joined)
	}
}

// An already-started machine is used as-is: no start, no wait.
func TestAStartedMachineIsUsedAsIs(t *testing.T) {
	t.Parallel()
	var paths []string
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = io.WriteString(w, `[{"id":"d891","name":"forge-worker","state":"started"}]`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "tok", FlyApp: "mtglab",
		Machine: DefaultMachine, ShimPort: DefaultShimPort})
	base, err := w.BaseURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The default port, since the override was cleared.
	if base != "http://d891.vm.mtglab.internal:8080" {
		t.Errorf("the base URL is %q", base)
	}
	for _, p := range paths {
		if strings.Contains(p, "/start") || strings.Contains(p, "/wait") {
			t.Errorf("a started machine was started again: %v", paths)
		}
	}
}

// The Machines API's own words reach the caller, because they are the only
// diagnosis available -- truncated at 300 code points so a multibyte rune is
// never split.
func TestTheMachinesAPIsOwnWordsReachTheCaller(t *testing.T) {
	t.Parallel()
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"token expired"}`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "tok", FlyApp: "mtglab",
		Machine: DefaultMachine})
	_, err := w.BaseURL(context.Background())
	if err == nil {
		t.Fatal("a 401 was not raised")
	}
	if !strings.Contains(err.Error(), "token expired") || !strings.Contains(err.Error(), "401") {
		t.Errorf("the refusal said %q", err)
	}
}

// A machine list this client cannot read is a refusal rather than an empty
// list, which would present as "no machine named forge-worker" and send
// somebody looking at the deploy workflow.
func TestAnUnreadableMachineListIsSaidPlainly(t *testing.T) {
	t.Parallel()
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"machines": "not a list"}`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "tok", FlyApp: "mtglab",
		Machine: DefaultMachine})
	_, err := w.BaseURL(context.Background())
	if err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Errorf("an unreadable list said %q", err)
	}
}

// The truncation is by code point rather than by byte, so a multibyte rune is
// never split down the middle into replacement characters.
func TestTruncateCutsByCodePointRatherThanByByte(t *testing.T) {
	t.Parallel()
	if got := truncate("hello", 300); got != "hello" {
		t.Errorf("a short string became %q", got)
	}
	if got := truncate("hello", 2); got != "he" {
		t.Errorf("truncate(hello, 2) = %q", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("the empty string became %q", got)
	}
	// Three runes, nine bytes: cutting at two must give two whole runes.
	got := truncate("日本語", 2)
	if got != "日本" {
		t.Errorf("truncate by code point gave %q", got)
	}
	if len([]rune(got)) != 2 {
		t.Errorf("%q is %d runes, want 2", got, len([]rune(got)))
	}
}

// The worker's own defaults, which are what a zero-value Worker runs with.
func TestAZeroWorkerHasWorkingDefaults(t *testing.T) {
	t.Parallel()
	w := &Worker{}
	if w.client() != http.DefaultClient {
		t.Error("a zero worker has no HTTP client")
	}
	if w.boot() != BootBudget {
		t.Errorf("the default boot budget is %v", w.boot())
	}
	// Sleep on a zero worker really sleeps, so it is asked for something
	// small enough not to matter.
	start := time.Now()
	w.sleep(time.Millisecond)
	if time.Since(start) < time.Millisecond {
		t.Error("the default sleep did not sleep")
	}

	custom := &Worker{HTTP: &http.Client{}, Boot: time.Second}
	if custom.client() == http.DefaultClient {
		t.Error("an injected client was ignored")
	}
	if custom.boot() != time.Second {
		t.Errorf("an injected budget came back as %v", custom.boot())
	}
}

// errorField reads the shim's own word for what went wrong, and answers ""
// for anything it cannot read -- which `refused` turns into a sentence
// naming the status instead.
func TestErrorFieldReadsTheShimsWordOrNothing(t *testing.T) {
	t.Parallel()
	if got := errorField([]byte(`{"error":"boom"}`)); got != "boom" {
		t.Errorf("read %q", got)
	}
	if got := errorField([]byte(`{"detail":"boom"}`)); got != "" {
		t.Errorf("a different key read as %q", got)
	}
	if got := errorField([]byte(`not json`)); got != "" {
		t.Errorf("garbage read as %q", got)
	}
	if got := errorField(nil); got != "" {
		t.Errorf("no body read as %q", got)
	}
}

// rewriteTo points every request at a local test server, whatever host it
// names -- the Machines API's URL is a constant, so this is how it is stubbed.
func rewriteTo(base string) http.RoundTripper {
	return rewriter{base: base}
}

type rewriter struct{ base string }

func (r rewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Clone(req.Context())
	parsed, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	target.URL.Scheme, target.URL.Host = parsed.Scheme, parsed.Host
	return http.DefaultTransport.RoundTrip(target)
}
