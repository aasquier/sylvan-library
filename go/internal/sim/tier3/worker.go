package tier3

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The worker client: the app's side of the hosted Forge — wake the worker,
// ask it, let it sleep.
//
// ADR 35's fifth decision, as client code. The deployed app machine holds no
// JVM and no distribution; what it holds is a token for Fly's Machines API and
// the name of a machine that does. A match becomes: start that machine (it is
// kept `stopped` between asks), wait for its shim to answer, POST the work
// over the private network, and rebuild the answer into the same [SimRun] a
// local run produces. The machine stops itself when idle (the shim owns that),
// so nothing here has to remember to turn the lights off.
//
// **Creation is deliberately not here.** Creating the machine means pulling a
// ~1GB image onto a host, which is minutes the first time — the deploy
// workflow does it, with the same image it just pushed, and keeps the
// machine's image current on every deploy. If the machine is missing, this
// says so as a 503-shaped [ErrForgeNotInstalled] rather than provisioning
// infrastructure from a request thread.
//
// Configuration, all environment (`.env.example` documents them):
//
//   - `MTGLAB_FORGE_WORKER=1` — the dial. Off, the app probes for a local
//     Forge and this is never consulted.
//   - `MTGLAB_FLY_API_TOKEN` — a deploy token, stored with `fly secrets set`.
//     Never in `fly.toml`; the repo is public.
//   - `MTGLAB_FORGE_SHIM_TOKEN` — shared with the shim automatically, because
//     one app's secrets reach every machine in it.
//   - `MTGLAB_FORGE_WORKER_URL` — point straight at a running shim and skip
//     the Machines API entirely. This is how tests drive the real HTTP path,
//     and how a laptop can talk to a hand-started shim.

// MachinesAPI is Fly's control plane.
const MachinesAPI = "https://api.machines.dev/v1"

// BootBudget is how long a cold start may take before the request is refused:
// machine start, the shim's boot, and its first index warm. Generous rather
// than tight, because the refusal is a 503 somebody sees.
//
// The name carries no unit because the
// value carries one.
//
// **It covers the machine start now, and it did not — which is the whole of a
// fault that made the first match after every deploy fail.** This comment has
// always said "machine start (~1s measured shape)", and the code never applied
// the budget there: [Worker.BaseURL] asked Fly to wait sixty seconds for the
// machine and treated the answer as final, while [Worker.Ready] spent this
// number on the shim afterwards. One second is the measured shape of a *warm*
// image. Merging deploys (ADR 23) swaps the worker's image, and a cold one has
// to be pulled and unpacked before anything in it runs — which does not finish
// in sixty seconds. So the sequence a person met was: a change lands, the first
// person to send two decks in is told the match failed, and the second attempt
// works because the first one warmed the machine (2026-08-25, found by playing
// a real match on the deployed instance; Aaron asked for it fixed 2026-08-28).
//
// **A 408 from Fly's wait is "not yet", not "no".** The machine is still
// coming up and asking again costs nothing — so `BaseURL` now asks repeatedly
// until this budget is spent, and a warm machine still returns on the first
// call in about a second. Nothing is kept awake and nothing is pre-pulled; the
// only thing that changed is how long the room is willing to wait before
// calling a boot a failure.
//
// **Three minutes is a dial, and it is Aaron's.** It is the honest cost of a
// cold image on this shape, and it is also a long time to watch a spinner. The
// alternative is to warm the worker as part of the deploy so nobody ever pays
// it — which spends machine time on every merge, and that is a judgement about
// somebody's money rather than about this package.
const BootBudget = 180 * time.Second

// machineWait is how long a single Fly `wait?state=started` call is asked to
// block. Fly's own ceiling for that parameter is sixty seconds, so a longer
// budget is spent as a series of these rather than as one request.
const machineWait = 60 * time.Second

// Worker is the client. A value rather than package functions so a test can
// give it its own HTTP client and clock.
type Worker struct {
	// Settings is the resolved Forge and Fly environment: which app holds the
	// machine, the tokens, and where the shim answers. The zero value is a
	// worker nothing is configured for, which every method refuses in the
	// same words an unset variable used to produce.
	Settings Settings
	// HTTP is the client every call goes through. Nil means a default with no
	// timeout of its own — every call below carries its own context deadline.
	HTTP *http.Client
	// Boot is the cold-start budget; zero means [BootBudget].
	Boot time.Duration
	// Sleep is how the health poll waits, so a test need not.
	Sleep func(time.Duration)
}

func (w *Worker) client() *http.Client {
	if w.HTTP != nil {
		return w.HTTP
	}
	return http.DefaultClient
}

func (w *Worker) boot() time.Duration {
	if w.Boot != 0 {
		return w.Boot
	}
	return BootBudget
}

func (w *Worker) sleep(d time.Duration) {
	if w.Sleep != nil {
		w.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (w *Worker) appName() (string, error) {
	if w.Settings.FlyApp == "" {
		return "", NotInstalled("forge worker: no Fly app name in the " +
			"environment (FLY_APP_NAME or MTGLAB_FLY_APP)")
	}
	return w.Settings.FlyApp, nil
}

// api is one Machines API call. Returns [ErrForgeNotInstalled] with the API's
// own words on failure, because every caller here turns that into a 503.
func (w *Worker) api(ctx context.Context, method, path string,
	payload any, timeout time.Duration) (json.RawMessage, error) {
	token := w.Settings.FlyAPIToken
	if token == "" {
		return nil, NotInstalled("forge worker: MTGLAB_FLY_API_TOKEN is not set")
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, MachinesAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client().Do(req)
	if err != nil {
		return nil, NotReady("forge worker: Machines API unreachable: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// Truncated at 300 code points, not bytes -- the recorded cut is
		// by character, so a multibyte rune is never split.
		detail := truncate(string(raw), 300)
		return nil, &apiRefusal{code: resp.StatusCode,
			err: NotReady("forge worker: Machines API %s %s answered "+
				"%d: %s", method, path, resp.StatusCode, detail)}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// apiRefusal is a Machines API answer with its status still attached, so a
// caller can ask *which* refusal this was without reading the sentence.
//
// It exists for one question: [Worker.waitStarted] needs to know a 408 from
// everything else. Fly answers a long poll that reached its own ceiling with
// one, and that is the only answer worth asking again — a 401 asked again is a
// 401 three minutes later.
//
// Wrapping rather than replacing, so `errors.Is(err, ErrWorkerNotReady)` and
// `errors.Is(err, ErrForgeNotInstalled)` both still hold and every caller that
// maps this onto a 503 is untouched.
type apiRefusal struct {
	code int
	err  error
}

func (e *apiRefusal) Error() string { return e.err.Error() }
func (e *apiRefusal) Unwrap() error { return e.err }

// stillComing reports whether an answer means "not yet" rather than "no".
func stillComing(err error) bool {
	var refusal *apiRefusal
	return errors.As(err, &refusal) && refusal.code == http.StatusRequestTimeout
}

type machine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func (w *Worker) findMachine(ctx context.Context) (machine, string, error) {
	app, err := w.appName()
	if err != nil {
		return machine{}, "", err
	}
	raw, err := w.api(ctx, http.MethodGet, "/apps/"+app+"/machines", nil, 30*time.Second)
	if err != nil {
		return machine{}, "", err
	}
	var machines []machine
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &machines); err != nil {
			return machine{}, "", NotReady(
				"forge worker: the Machines API listed machines this could not read: %v", err)
		}
	}
	for _, m := range machines {
		if m.Name == w.Settings.Machine {
			return m, app, nil
		}
	}
	return machine{}, "", NotInstalled("forge worker: no machine named %q in "+
		"app %q — the deploy workflow creates it; has one run since the "+
		"worker landed?", w.Settings.Machine, app)
}

func (w *Worker) shimHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if w.Settings.ShimToken != "" {
		req.Header.Set("Authorization", "Bearer "+w.Settings.ShimToken)
	}
}

// BaseURL is where the shim answers, waking the machine if it is asleep.
//
// The deadline is the caller's, so the machine coming up and the shim opening
// its port are spent out of **one** budget rather than two — see [BootBudget]
// for the fault that came of them being separate.
func (w *Worker) BaseURL(ctx context.Context, by time.Time) (string, error) {
	if w.Settings.WorkerURL != "" {
		return w.Settings.WorkerURL, nil
	}

	m, app, err := w.findMachine(ctx)
	if err != nil {
		return "", err
	}
	if m.State != "started" {
		// Starting an already-started machine races benignly here: the API
		// answers an error the wait below absorbs, because what matters is the
		// state, not who set it.
		_, _ = w.api(ctx, http.MethodPost,
			fmt.Sprintf("/apps/%s/machines/%s/start", app, m.ID), nil, 30*time.Second)
		if err := w.waitStarted(ctx, app, m.ID, by); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("http://%s.vm.%s.internal:%d", m.ID, app, w.Settings.ShimPort), nil
}

// waitStarted blocks until the machine reports `started`, or until the
// deadline.
//
// **A wait that runs out is asked again**, and that is the fix rather than the
// retry-for-luck it looks like. Fly's `wait` is a long poll with a ceiling of
// its own, and a 408 from it says the machine has not reached the state *yet* —
// the start is still running. Treating that one answer as the whole verdict is
// what made a cold image, which cannot be pulled and unpacked inside a single
// poll, report as a failed match to whoever asked first.
//
// A warm machine leaves on the first pass in about a second, so nothing that
// worked before waits any longer than it did.
func (w *Worker) waitStarted(ctx context.Context, app, id string,
	by time.Time) error {
	var last error
	for {
		left := time.Until(by)
		if left <= 0 {
			if last != nil {
				return last
			}
			return NotReady("forge worker: the machine did not start in time")
		}
		block := machineWait
		if left < block {
			block = left
		}
		_, err := w.api(ctx, http.MethodGet,
			fmt.Sprintf("/apps/%s/machines/%s/wait?state=started&timeout=%d",
				app, id, int(block.Seconds())),
			nil, block+10*time.Second)
		if err == nil {
			return nil
		}
		// A context the caller cancelled is not a machine that is slow, and
		// asking again would spin until the deadline for nothing.
		if ctx.Err() != nil {
			return err
		}
		// **Only "not yet" is asked again.** A wait that came back 408 is a
		// machine still on its way; anything else — a token that expired, a
		// machine that went away — is an answer, and asking it repeatedly for
		// three minutes would turn one clear refusal into a long silence.
		if !stillComing(err) {
			return err
		}
		last = err
	}
}

// Ready is a base URL whose shim has answered.
func (w *Worker) Ready(ctx context.Context) (string, error) {
	deadline := time.Now().Add(w.boot())
	base, err := w.BaseURL(ctx, deadline)
	if err != nil {
		return "", err
	}
	last := "no answer yet"
	for time.Now().Before(deadline) {
		ok, err := w.healthy(ctx, base)
		if ok {
			return base, nil
		}
		if err != nil {
			last = err.Error()
		}
		w.sleep(time.Second)
	}
	return "", NotReady("forge worker: machine started but the shim "+
		"never answered (%s)", last)
}

func (w *Worker) healthy(ctx context.Context, base string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false, err
	}
	w.shimHeaders(req)
	resp, err := w.client().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	var answer struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return false, err
	}
	return answer.OK, nil
}

// refused turns a shim's refusal into the error class the caller expects: a
// 503 is the distribution's absence and becomes a 503 upstream; anything else
// is a runtime failure the job records.
func refused(message string, code int) error {
	if message == "" {
		message = fmt.Sprintf("shim answered %d", code)
	}
	if code == 503 {
		return NotInstalled("%s", message)
	}
	return errors.New(message)
}

// CheckCoverage is the pre-flight, computed where the card scripts live.
//
// Same contract as [CheckCoverage]: reports back, or [ErrCoverageFailed]
// naming the cards — the route's 422 does not care which machine read the zip.
// Runs on the request thread, so the boot budget is bounded and a machine that
// will not come up is a 503, not a hang.
func (w *Worker) CheckCoverage(ctx context.Context, decks []*deck.Deck) ([]CoverageReport, error) {
	base, err := w.Ready(ctx)
	if err != nil {
		return nil, err
	}
	texts, err := DecksToWire(decks)
	if err != nil {
		return nil, err
	}
	var answer struct {
		Reports []WireReport `json:"reports"`
	}
	if err := w.post(ctx, base, "/coverage",
		map[string]any{"decks": texts}, 60*time.Second, &answer); err != nil {
		return nil, err
	}
	reports := ReportsFromWire(answer.Reports)
	if err := RaiseUnlessCovered(reports); err != nil {
		return nil, err
	}
	return reports, nil
}

func (w *Worker) post(ctx context.Context, base, path string, payload any,
	timeout time.Duration, into any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path,
		bytes.NewReader(raw))
	if err != nil {
		return err
	}
	w.shimHeaders(req)
	resp, err := w.client().Do(req)
	if err != nil {
		return fmt.Errorf("forge worker: %s failed: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("forge worker: %s failed: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return refused(errorField(body), resp.StatusCode)
	}
	return json.Unmarshal(body, into)
}

func errorField(body []byte) string {
	var answer struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return ""
	}
	return answer.Error
}

// MatchAsk is one match asked of the worker, and who hears it as it happens.
//
// A struct rather than a hand of positional arguments, because this is the
// app's half of the shim's own `matchAsk` and the two grow together: a flag
// added on one side is a field added on the other, and a call site that reads
// `RunMatch(ctx, decks, 10, 300, seed, true, false, tick, nil)` tells nobody
// which `true` was which.
//
// The two callbacks are optional and independent. [MatchAsk.OnEvents] is only
// ever called when [MatchAsk.Narrate] asked for beats, and only by a shim new
// enough to send them.
type MatchAsk struct {
	Games int
	Clock int
	Seed  *big.Int
	// Narrate asks the far side to drop Forge's `-q` and tell the game. It
	// costs nothing in time and about a hundred beats a game in volume, so it
	// is asked for by whoever is watching and by nobody who is measuring —
	// the argument is in `events.go`, where the measuring was done.
	Narrate bool
	// OnGame hears each game as it finishes, with the row the far side just
	// parsed. The row is a pointer because a shim from before the match
	// theater streams the count alone: the bar still ticks and the theater
	// has nothing to seat, which is a legible state rather than a failure.
	OnGame func(finished int, game *GameResult)
	// OnEvents hears one game's beats, whole, at the moment the game closed.
	OnEvents func(log EventLog)
}

// RunMatch plays one match on the worker and returns it as if it had run here.
//
// The ask carries `stream: true`, so a current shim answers in
// newline-delimited JSON — `{"events": ...}` and then `{"game": n, "row": ...}`
// as each game finishes (handed to [MatchAsk.OnEvents] and [MatchAsk.OnGame],
// the same callbacks [RunGames] takes, so the API cannot tell the two paths
// apart), then `{"result": ...}` or `{"error": ...}` as the last line. A shim
// from before the flag ignores it and answers one plain JSON
// body; the Content-Type is what says which conversation this is, and both are
// accepted so a deploy that updates the app a few minutes before its worker
// never breaks a match over it. The `row` is newer still (the match theater),
// so it is optional the same way: a pre-theater shim sends the count alone and
// `OnGame` hears a nil row — the bar still ticks, the theater just has nothing
// to seat. `events` is newer again and degrades the same way: an older shim
// simply never sends the line, and a room that was going to narrate shows the
// rows it does have.
//
// The timeout is per read rather than per match, and `clock + 120` bounds
// every wait this request can make: the JVM boot before the first line, and
// one game before each line after it. The subprocess on the far side is
// already bounded whole, so this is the belt over that suspenders.
func (w *Worker) RunMatch(ctx context.Context, decks []*deck.Deck,
	ask MatchAsk) (*SimRun, error) {
	base, err := w.Ready(ctx)
	if err != nil {
		return nil, err
	}
	texts, err := DecksToWire(decks)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"decks": texts, "games": ask.Games,
		"clock": ask.Clock, "seed": ask.Seed, "stream": true,
		"narrate": ask.Narrate}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// **The budget is per read, not per match**, and the distinction is the
	// contract: a per-read (socket-style) timeout
	// applies to each read, while `http.Client.Timeout` bounds the whole
	// exchange including the body — on a twenty-game match that would be a
	// second, worse copy of the far side's own ceiling, and a shorter one.
	// So the request carries a cancellable context and a timer that is reset
	// on every line: a stream that goes quiet for longer than one game plus
	// the JVM's boot is cancelled, and a stream that keeps talking runs as
	// long as it likes.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/match",
		bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	w.shimHeaders(req)

	client := *w.client()
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forge worker: /match failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, refused(errorField(body), resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "ndjson") {
		// An older shim played the whole match and answered it flat.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("forge worker: /match failed: %w", err)
		}
		var run WireRun
		if err := json.Unmarshal(body, &run); err != nil {
			return nil, errors.New("forge worker: unexpected answer from /match")
		}
		return RunFromWire(run), nil
	}

	return readStream(resp.Body, time.Duration(ask.Clock+120)*time.Second,
		cancel, ask)
}

// streamLine is one newline-delimited line of the match stream. Exactly one of
// `game`, `events`, `error` and `result` is set on any given line; `row` rides
// with `game`.
type streamLine struct {
	Game   *int      `json:"game"`
	Row    *WireGame `json:"row"`
	Events *EventLog `json:"events"`
	Error  *string   `json:"error"`
	Type   string    `json:"type"`
	Result *WireRun  `json:"result"`
}

// stallTimer is the part of *time.Timer [readStream] needs: armed once,
// re-armed after every read, stopped at the end.
type stallTimer interface {
	Reset(time.Duration) bool
	Stop() bool
}

func readStream(body io.Reader, perRead time.Duration, stall func(),
	ask MatchAsk) (*SimRun, error) {
	return readStreamOn(body, perRead, stall, ask,
		func(d time.Duration, f func()) stallTimer { return time.AfterFunc(d, f) })
}

// readStreamOn is [readStream] with the stall timer supplied rather than
// built, so a test about the *budget* can be about the budget.
//
// **A duration compared against real elapsed time is an assertion about the
// machine.** The property here is that the deadline is re-armed on every read,
// and the only way to show it with `time.AfterFunc` is to let more wall clock
// pass in total than the budget allows while no single gap does — which stops
// being true the moment a loaded machine stretches one of those gaps. That is
// not a hypothetical: pacing five lines with 15ms sleeps against a 60ms budget
// failed 6 runs in 20 under load, reporting a chatty stream as a cancelled
// one. Handing the timer in lets the test turn the clock by hand and assert
// the reset exactly, on any machine, without sleeping at all.
func readStreamOn(body io.Reader, perRead time.Duration, stall func(),
	ask MatchAsk, arm func(time.Duration, func()) stallTimer) (*SimRun, error) {
	// The stall timer: fired, it cancels the request, and the read below
	// fails rather than blocking forever on a worker that died mid-match.
	quiet := arm(perRead, stall)
	defer quiet.Stop()

	reader := bufio.NewReaderSize(body, 64*1024)
	for {
		raw, err := reader.ReadBytes('\n')
		quiet.Reset(perRead)
		if len(bytes.TrimSpace(raw)) > 0 {
			var line streamLine
			if err := json.Unmarshal(raw, &line); err != nil {
				return nil, errors.New("forge worker: unreadable line from /match")
			}
			switch {
			case line.Game != nil:
				if ask.OnGame != nil {
					var game *GameResult
					if line.Row != nil {
						g := GameFromWire(*line.Row)
						game = &g
					}
					ask.OnGame(*line.Game, game)
				}
			case line.Events != nil:
				// Before the `game` line that closes the same game, because
				// that is the order one pass over Forge's output produces
				// them in — so a listener that stashes beats and publishes
				// them with the row never has a row waiting on beats.
				if ask.OnEvents != nil {
					ask.OnEvents(*line.Events)
				}
			case line.Error != nil:
				if line.Type == "ForgeNotInstalled" {
					return nil, NotInstalled("%s", *line.Error)
				}
				return nil, errors.New(*line.Error)
			case line.Result != nil:
				return RunFromWire(*line.Result), nil
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("forge worker: /match failed: %w", err)
		}
	}
	return nil, errors.New("forge worker: the match stream ended without a result")
}

func truncate(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
