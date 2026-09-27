package tier3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The two conversations the worker holds, asked about what goes wrong in them.
//
// One is Fly's Machines API — the control plane that starts the machine — and
// the other is the shim's own door once it answers. `worker_test.go` drives
// both when they work; this drives them when they do not, because the whole
// design of ADR 35 turns on which *sentence* a failure produces: a shim saying
// "no distribution" has to stay [ErrForgeNotInstalled] all the way up so the
// route answers 503, a control plane that is merely slow has to stay
// [ErrWorkerNotReady] so the room says come back in a minute, and a match that
// broke mid-play is a job error the caller records. Collapsing any two of those
// turns "Forge is not installed here" into a red job, or a real crash into a
// soothing "not available".
//
// **Nothing underneath the word Forge is ever said to a player** (CLAUDE.md
// names this exactly): the statuses, the payloads and the variable names below
// go to the log, and `api.forgeTrouble` is the one place any of it becomes
// words in a room. These assertions are therefore about what the *log* gets,
// which is where the diagnosis has to be.

// machinesAPI is a stub Fly control plane, with the client's transport
// rewritten onto it so the real URL building and the real headers run.
func machinesAPI(t *testing.T, h http.HandlerFunc) *Worker {
	t.Helper()
	fly := httptest.NewServer(h)
	t.Cleanup(fly.Close)
	return flyWorker(fly, Settings{FlyAPIToken: "a-deploy-token", FlyApp: "mtglab",
		Machine: DefaultMachine})
}

// Every way one Machines API call can fail, and the words each produces.
func TestEveryMachinesAPIFailureSaysWhichKindItIs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// No token at all: nothing to authenticate with, so the machine can never
	// be started here -- that is not a transient fault and must not read as one.
	nothing := &Worker{Settings: Settings{FlyApp: "mtglab"}}
	_, err := nothing.api(ctx, http.MethodGet, "/apps/mtglab/machines", nil, time.Second)
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("a missing token failed as %v", err)
	}
	if errors.Is(err, ErrWorkerNotReady) {
		t.Error("a missing token read as `come back in a minute`")
	}
	if !strings.Contains(err.Error(), "MTGLAB_FLY_API_TOKEN") {
		t.Errorf("the refusal does not name what would fix it: %v", err)
	}

	// A payload that reaches the far side, which is the arm a GET never takes.
	var sent string
	posted := machinesAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		sent = string(body)
		if got := r.Header.Get("Authorization"); got != "Bearer a-deploy-token" {
			t.Errorf("the control plane was called with %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	raw, err := posted.api(ctx, http.MethodPost, "/apps/mtglab/machines/m1/start",
		map[string]any{"signal": "SIGTERM"}, time.Second)
	if err != nil {
		t.Fatalf("a POST with a body failed: %v", err)
	}
	if !strings.Contains(sent, `"signal":"SIGTERM"`) {
		t.Errorf("the body arrived as %q", sent)
	}
	// An empty answer is not an answer to parse: a 200 with nothing in it is
	// how Fly says "done", and returning an empty `json.RawMessage` would make
	// every caller check the length before unmarshalling.
	if raw != nil {
		t.Errorf("an empty 200 came back as %q rather than nothing", raw)
	}

	// A payload with no JSON form fails before anything is sent, rather than
	// putting a half-written body on the wire.
	if _, err := posted.api(ctx, http.MethodPost, "/apps/mtglab/machines",
		map[string]any{"handler": func() {}}, time.Second); err == nil {
		t.Error("a payload with no JSON form was sent anyway")
	}

	// A refusal carries the control plane's own words, truncated -- and a 408
	// is the one answer worth asking again, because Fly ends a long poll that
	// reached its own ceiling with one.
	longWinded := machinesAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestTimeout)
		_, _ = w.Write([]byte(`{"error":"` + strings.Repeat("é", 400) + `"}`))
	})
	_, err = longWinded.api(ctx, http.MethodGet, "/apps/mtglab/machines", nil, time.Second)
	if !stillComing(err) {
		t.Errorf("a 408 did not read as `not yet`: %v", err)
	}
	if !errors.Is(err, ErrWorkerNotReady) || !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("a long-poll timeout answered to neither sentinel: %v", err)
	}
	// Truncated by code point rather than by byte, so a multibyte rune is
	// never split in half on its way into a log line.
	if n := strings.Count(err.Error(), "é"); n > 300 {
		t.Errorf("the refusal carried %d accented characters, want at most 300", n)
	}

	// Every other refusal is a refusal rather than a retry.
	refused := machinesAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err = refused.api(ctx, http.MethodGet, "/apps/mtglab/machines", nil, time.Second)
	if stillComing(err) {
		t.Errorf("a 401 read as something to ask again: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("the refusal does not carry the status: %v", err)
	}
}

// A control plane that does not answer at all is transient news: the machine
// may be perfectly fine and the network between here and Fly may not be.
func TestAnUnreachableControlPlaneIsNotYetRatherThanNever(t *testing.T) {
	t.Parallel()
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	worker := flyWorker(dead, Settings{FlyAPIToken: "tok", FlyApp: "mtglab"})
	dead.Close()

	_, err := worker.api(context.Background(), http.MethodGet,
		"/apps/mtglab/machines", nil, time.Second)
	if !errors.Is(err, ErrWorkerNotReady) {
		t.Errorf("an unreachable control plane failed as %v", err)
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("the diagnosis reads %q", err)
	}
}

// A match asked of a worker that never becomes ready is refused before a deck
// is ever serialised -- the machine is the thing that is wrong, and saying so
// is what keeps the room's sentence about the arena rather than about a deck.
func TestAMatchAskedOfAWorkerThatNeverAnswersIsRefusedEarly(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	_, err := shim.worker(50*time.Millisecond, "").RunMatch(context.Background(),
		twoDecks(), MatchAsk{Games: 1})
	if err == nil {
		t.Fatal("a match ran on a worker that never answered its health check")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("a worker that never came up failed as %T: %v", err, err)
	}
}

// A shim that refuses the match itself keeps its status's meaning: a 503 is
// "there is no Forge here" and stays [ErrForgeNotInstalled] so the route
// answers 503 in turn, while anything else is a job error somebody records.
func TestAShimThatRefusesTheMatchKeepsTheMeaningOfItsStatus(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		what       string
		status     int
		body       string
		installed  bool
		wantPhrase string
	}{
		{"no distribution on the worker", http.StatusServiceUnavailable,
			`{"error":"no Forge card data at /opt/forge"}`, true,
			"no Forge card data"},
		{"a match that broke", http.StatusInternalServerError,
			`{"error":"Forge fell over"}`, false, "Forge fell over"},
		{"a refusal with nothing said", http.StatusBadGateway, "", false,
			"shim answered 502"},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			shim := newStubShim(t)
			shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			})

			_, err := shim.worker(time.Second, "").RunMatch(context.Background(),
				twoDecks(), MatchAsk{Games: 1})
			if err == nil {
				t.Fatalf("a shim answering %d produced a run", c.status)
			}
			if got := errors.Is(err, ErrForgeNotInstalled); got != c.installed {
				t.Errorf("%s answered to ErrForgeNotInstalled=%v, want %v",
					c.what, got, c.installed)
			}
			if !strings.Contains(err.Error(), c.wantPhrase) {
				t.Errorf("%s said %q, want a mention of %q", c.what, err, c.wantPhrase)
			}
		})
	}
}

// The pre-flight runs on the request thread, so a machine that will not come
// up is a refusal rather than a hang -- and it is a refusal rather than a
// clean report, because "every card is implemented" from a worker that never
// answered would be the one sentence this whole package exists to prevent.
func TestAPreFlightOnAWorkerThatIsNotThereRefusesRatherThanReportsCoverage(t *testing.T) {
	t.Parallel()
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	worker := &Worker{
		Settings: Settings{WorkerURL: dead.URL},
		Boot:     50 * time.Millisecond,
		Sleep:    func(time.Duration) {},
	}
	dead.Close()

	reports, err := worker.CheckCoverage(context.Background(), twoDecks())
	if err == nil {
		t.Fatalf("a worker that is not there pre-flighted %d decks", len(reports))
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("an absent worker failed as %T: %v", err, err)
	}
}

// A shim that hangs up during the pre-flight names the call that broke, which
// is what tells a reader of the log that the machine answered its health check
// and then went -- a different fault from one that never came up at all.
func TestAShimThatHangsUpDuringThePreFlightNamesTheCall(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("coverage", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("the stub could not hijack: %v", err)
			return
		}
		_ = conn.Close()
	})

	_, err := shim.worker(time.Second, "").CheckCoverage(context.Background(), twoDecks())
	if err == nil {
		t.Fatal("a shim that hung up produced a coverage report")
	}
	if !strings.Contains(err.Error(), "/coverage failed") {
		t.Errorf("the diagnosis reads %q", err)
	}
}

// A busy arena is its own news: the machine is working, on somebody else's
// match, and the whole of what anybody needs to do is come back. It answers to
// the transient sentinel as well, so a caller that only sorts "come back" from
// "never" keeps working without knowing this class exists.
func TestABusyArenaIsItsOwnNewsAndAlsoTransientNews(t *testing.T) {
	t.Parallel()
	err := ArenaBusy("the arena did not take the match within %s", 30*time.Second)
	if got := err.Error(); got != "the arena did not take the match within 30s" {
		t.Errorf("the reason reads %q", got)
	}
	for _, sentinel := range []error{ErrArenaBusy, ErrWorkerNotReady, ErrForgeNotInstalled} {
		if !errors.Is(err, sentinel) {
			t.Errorf("a busy arena does not answer to %v", sentinel)
		}
	}
	// And a bout's whole ceiling grows with the games asked for, while a caller
	// who named neither a count nor a clock gets the one-game default rather
	// than a match with no time in it.
	if MatchBudget(0, 0) != MatchBudget(1, ClockDefault) {
		t.Error("an unnamed games count or clock did not fall back")
	}
	if MatchBudget(10, 300) <= MatchBudget(1, 300) {
		t.Error("ten games are given no more rope than one")
	}
}

// A shim that hangs up mid-answer is a match that broke off, and it is named
// as one: the match is the thing that failed, and a caller reading this in a
// job's error has to be able to tell it from a refusal.
func TestAShimThatHangsUpMidAnswerIsAMatchThatBrokeOff(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
		// Hijack and close: the request is accepted and then the connection
		// goes, which is what a worker being taken out from under a match does.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("the stub could not hijack: %v", err)
			return
		}
		_ = conn.Close()
	})

	_, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		twoDecks(), MatchAsk{Games: 1})
	if err == nil {
		t.Fatal("a shim that hung up produced a run")
	}
	if !strings.Contains(err.Error(), "/match failed") {
		t.Errorf("a broken-off match reads as %q", err)
	}
	// And it is not a refusal: nothing here says the arena is unavailable, so
	// nothing downstream may turn it into "Forge is not available".
	if errors.Is(err, ErrForgeNotInstalled) {
		t.Error("a match that broke off read as a worker with no Forge on it")
	}
}
