package tier3

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The worker's conversations, asked about the half of each that goes wrong
// *after* the first byte.
//
// `workerfaults_test.go` drives a control plane that refuses and a shim that
// hangs up before answering. What was left was the narrower band either side of
// that: a request this side cannot even build, a body that is announced and then
// cut short, and a wait that is asked again until the budget runs out. All three
// have the same shape of failure — something came back, and treating it as an
// answer would be worse than refusing.
//
// None of these assert a sentence from the operating system or from Fly. What
// they assert is which *kind* of refusal came back, that nothing claimed
// success, and — where the point of a branch is restraint — how many times the
// far side was asked.

// shortBody answers with a Content-Length it does not honour and hangs up:
// the headers arrive, the read fails. A half-written answer is the shape a
// worker taken out from under a running match produces, and it is the one
// failure a status code cannot describe.
func shortBody(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", "512")
		_, _ = io.WriteString(w, `{"reports`)
	}
}

// A pre-flight whose answer is cut off mid-body names the call that broke rather
// than reporting coverage nobody read.
func TestAPreFlightAnswerThatIsCutShortIsNotCoverage(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("coverage", shortBody("application/json"))

	reports, err := shim.worker(time.Second, "").CheckCoverage(
		context.Background(), twoDecks())
	if err == nil {
		t.Fatalf("a half-written answer became %d coverage reports", len(reports))
	}
	if !strings.Contains(err.Error(), "/coverage failed") {
		t.Errorf("the diagnosis reads %q", err)
	}
	if reports != nil {
		t.Error("a refused pre-flight still handed back reports")
	}
}

// And a flat match answer cut off mid-body is a match that broke off, not an
// unreadable one: the distinction matters because an unreadable answer sends
// somebody looking at the shim's version and a broken-off one at the machine.
func TestAFlatMatchAnswerThatIsCutShortIsAMatchThatBrokeOff(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", shortBody("application/json"))

	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		twoDecks(), MatchAsk{Games: 1})
	if err == nil {
		t.Fatalf("a half-written answer became a run of %d games", len(run.Games()))
	}
	if !strings.Contains(err.Error(), "/match failed") {
		t.Errorf("a cut-off answer reads as %q", err)
	}
	if errors.Is(err, ErrForgeNotInstalled) {
		t.Error("a cut-off answer read as a worker with no Forge on it")
	}
}

// A health check whose answer is cut short is not a healthy shim, and a base URL
// this side cannot even turn into a request is not one either. Both are asked of
// the health check directly, because [Worker.Ready] spends its whole boot budget
// polling and what is under test is one answer rather than the waiting.
func TestAShimIsUnhealthyWhenItsAnswerCannotBeRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shim := newStubShim(t)
	shim.on("healthz", shortBody("application/json"))

	w := shim.worker(time.Second, "")
	if ok, err := w.healthy(ctx, shim.URL); ok || err == nil {
		t.Errorf("a half-written health answer read as healthy=%v, %v", ok, err)
	}

	// A base URL with a control character in it cannot become a request at all.
	if ok, err := w.healthy(ctx, "http://worker\n.internal:8080"); ok || err == nil {
		t.Errorf("an unusable base URL read as healthy=%v, %v", ok, err)
	}
}

// The shim call that every post goes through refuses two things before it
// reaches the wire: a payload that cannot be written down, and a base URL that
// cannot be asked. Neither may be sent half-built, because the far side would
// then answer a question nobody asked.
func TestAShimCallRefusesWhatItCannotSend(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shim := newStubShim(t)
	w := shim.worker(time.Second, "")

	var answer struct {
		Reports []WireReport `json:"reports"`
	}
	// A channel has no rendering, so the payload cannot be written down.
	if err := w.post(ctx, shim.URL, "/coverage", make(chan int),
		time.Second, &answer); err == nil {
		t.Error("a payload with no rendering was sent anyway")
	}
	if answer.Reports != nil {
		t.Error("a call that was never made filled in an answer")
	}
	// And a base URL that cannot be a request.
	if err := w.post(ctx, "http://worker\n.internal:8080", "/coverage",
		map[string]any{"decks": []string{}}, time.Second, &answer); err == nil {
		t.Error("an unusable base URL was asked anyway")
	}
	if got := shim.headers(); len(got) != 0 {
		t.Errorf("the shim was reached %d times by calls that could not be "+
			"built", len(got))
	}
}

// **A machine the control plane cannot be asked about is refused before any
// request leaves.** The app name comes out of the environment, so a value with a
// line break in it is a misconfigured deployment rather than a hypothetical —
// and the refusal has to be the class every caller maps onto a 503, not a panic
// from the URL builder.
func TestAnAppNameThatCannotBeAskedAboutNeverReachesTheControlPlane(t *testing.T) {
	t.Parallel()
	var asked atomic.Int64
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = io.WriteString(w, `[]`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "a-deploy-token",
		FlyApp: "mtglab\nforge", Machine: DefaultMachine})
	_, err := w.BaseURL(context.Background(), time.Now().Add(time.Second))
	if err == nil {
		t.Fatal("an app name with a line break in it was asked about")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("the refusal is %v, which no caller maps onto a refusal", err)
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("the control plane was reached %d times by a request that "+
			"could not be built", n)
	}
}

// **A machine that never comes up is reported in Fly's own last words.** The
// wait is a long poll that answers 408 while the image is still being unpacked,
// so a budget spent entirely on those must end with the newest of them rather
// than with this side's generic sentence — which is what a reader of the log
// needs to tell "the image is enormous" from "the machine does not exist".
func TestAWaitThatRunsOutCarriesTheLastAnswerRatherThanAGenericOne(t *testing.T) {
	t.Parallel()
	var waits atomic.Int64
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/wait") {
			waits.Add(1)
			w.WriteHeader(http.StatusRequestTimeout)
			_, _ = io.WriteString(w, `{"error":"deadline_exceeded: machine `+
				`failed to reach desired state, started, currently stopped"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "a-deploy-token", FlyApp: "mtglab",
		Machine: DefaultMachine})
	err := w.waitStarted(context.Background(), "mtglab", "d891",
		time.Now().Add(20*time.Millisecond))
	if err == nil {
		t.Fatal("a machine that never started was reported as started")
	}
	if !errors.Is(err, ErrWorkerNotReady) {
		t.Errorf("a slow boot is %v rather than something to come back to", err)
	}
	if !strings.Contains(err.Error(), "currently stopped") {
		t.Errorf("the last answer was replaced by this side's own words: %v", err)
	}
	if n := waits.Load(); n < 1 {
		t.Error("the wait was never asked at all")
	}
}

// **A caller who has gone is not waited for.** A cancelled context ends the wait
// at once instead of asking again until the boot budget is spent: the room that
// wanted the match has closed, and three more minutes of polling is three
// minutes of a machine's time spent on nobody.
func TestACancelledWaitIsNotAskedAgain(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var waits atomic.Int64
	fly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/wait") {
			waits.Add(1)
			// The caller goes while the long poll is still open, which is the
			// live shape: a browser closed, or the request's own deadline.
			cancel()
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer fly.Close()

	w := flyWorker(fly, Settings{FlyAPIToken: "a-deploy-token", FlyApp: "mtglab",
		Machine: DefaultMachine})
	err := w.waitStarted(ctx, "mtglab", "d891", time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("a cancelled wait reported a started machine")
	}
	if n := waits.Load(); n != 1 {
		t.Errorf("a cancelled caller was asked about %d times, want 1", n)
	}
}

// A worker with nowhere to ask is refused at the first step, and the refusal is
// the control plane's rather than the shim's — nothing was ever asked to be
// healthy, because there was no address to ask.
func TestAWorkerWithNoAddressIsRefusedBeforeAnyHealthCheck(t *testing.T) {
	t.Parallel()
	w := &Worker{Settings: Settings{Machine: DefaultMachine},
		Boot: time.Second, Sleep: func(time.Duration) {}}
	base, err := w.Ready(context.Background())
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("a worker with no app name answered %q, %v", base, err)
	}
	if base != "" {
		t.Errorf("a refused worker handed back the base URL %q", base)
	}
	if strings.Contains(err.Error(), "never answered") {
		t.Errorf("a machine that was never found was reported as a shim that "+
			"never answered: %v", err)
	}
}
