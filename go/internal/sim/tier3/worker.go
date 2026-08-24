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
	"os"
	"strconv"
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
// machine start (~1s measured shape), the shim's boot, and its first index
// warm. Generous rather than tight, because the refusal is a 503 somebody
// sees.
//
// The name carries no unit because the
// value carries one.
const BootBudget = 90 * time.Second

// Worker is the client. A value rather than package functions so a test can
// give it its own HTTP client and clock.
type Worker struct {
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

// Configured reports whether the hosted worker is the way to run Forge here —
// a fact about the environment.
//
// The gate calls this and answers `available: true` without any network: the
// same contract `/api/claude` set, where configuration is a fact of the
// environment and reachability is discovered when work is actually asked for.
func Configured() bool {
	if os.Getenv("MTGLAB_FORGE_WORKER_URL") != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("MTGLAB_FORGE_WORKER")) != "" &&
		os.Getenv("MTGLAB_FLY_API_TOKEN") != ""
}

func appName() (string, error) {
	// FLY_APP_NAME is injected by Fly into every machine; the override exists
	// for tests and for talking to the instance from a laptop.
	name := os.Getenv("MTGLAB_FLY_APP")
	if name == "" {
		name = os.Getenv("FLY_APP_NAME")
	}
	if name == "" {
		return "", NotInstalled("forge worker: no Fly app name in the " +
			"environment (FLY_APP_NAME or MTGLAB_FLY_APP)")
	}
	return name, nil
}

func machineName() string {
	if name := os.Getenv("MTGLAB_FORGE_MACHINE"); name != "" {
		return name
	}
	return "forge-worker"
}

func shimPort() int {
	if raw := os.Getenv("MTGLAB_FORGE_SHIM_PORT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return 8080
}

// api is one Machines API call. Returns [ErrForgeNotInstalled] with the API's
// own words on failure, because every caller here turns that into a 503.
func (w *Worker) api(ctx context.Context, method, path string,
	payload any, timeout time.Duration) (json.RawMessage, error) {
	token := os.Getenv("MTGLAB_FLY_API_TOKEN")
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
		return nil, NotInstalled("forge worker: Machines API unreachable: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// Truncated at 300 code points, not bytes -- the recorded cut is
		// by character, so a multibyte rune is never split.
		detail := truncate(string(raw), 300)
		return nil, NotInstalled("forge worker: Machines API %s %s answered "+
			"%d: %s", method, path, resp.StatusCode, detail)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

type machine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func (w *Worker) findMachine(ctx context.Context) (machine, string, error) {
	app, err := appName()
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
			return machine{}, "", NotInstalled(
				"forge worker: the Machines API listed machines this could not read: %v", err)
		}
	}
	for _, m := range machines {
		if m.Name == machineName() {
			return m, app, nil
		}
	}
	return machine{}, "", NotInstalled("forge worker: no machine named %q in "+
		"app %q — the deploy workflow creates it; has one run since the "+
		"worker landed?", machineName(), app)
}

func shimHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("MTGLAB_FORGE_SHIM_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// BaseURL is where the shim answers, waking the machine if it is asleep.
func (w *Worker) BaseURL(ctx context.Context) (string, error) {
	if direct := os.Getenv("MTGLAB_FORGE_WORKER_URL"); direct != "" {
		return strings.TrimRight(direct, "/"), nil
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
		if _, err := w.api(ctx, http.MethodGet,
			fmt.Sprintf("/apps/%s/machines/%s/wait?state=started&timeout=60", app, m.ID),
			nil, 70*time.Second); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("http://%s.vm.%s.internal:%d", m.ID, app, shimPort()), nil
}

// Ready is a base URL whose shim has answered.
func (w *Worker) Ready(ctx context.Context) (string, error) {
	base, err := w.BaseURL(ctx)
	if err != nil {
		return "", err
	}
	last := "no answer yet"
	deadline := time.Now().Add(w.boot())
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
	return "", NotInstalled("forge worker: machine started but the shim "+
		"never answered (%s)", last)
}

func (w *Worker) healthy(ctx context.Context, base string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false, err
	}
	shimHeaders(req)
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
	shimHeaders(req)
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

// RunMatch plays one match on the worker and returns it as if it had run here.
//
// The ask carries `stream: true`, so a current shim answers in
// newline-delimited JSON — `{"game": n, "row": ...}` as each game finishes
// (handed to `onGame`, the same callback [RunGames] takes, so the API cannot
// tell the two paths apart), then `{"result": ...}` or `{"error": ...}` as the
// last line. A shim from before the flag ignores it and answers one plain JSON
// body; the Content-Type is what says which conversation this is, and both are
// accepted so a deploy that updates the app a few minutes before its worker
// never breaks a match over it. The `row` is newer still (the match theater),
// so it is optional the same way: a pre-theater shim sends the count alone and
// `onGame` hears a nil row — the bar still ticks, the theater just has nothing
// to seat.
//
// The timeout is per read rather than per match, and `clock + 120` bounds
// every wait this request can make: the JVM boot before the first line, and
// one game before each line after it. The subprocess on the far side is
// already bounded whole, so this is the belt over that suspenders.
func (w *Worker) RunMatch(ctx context.Context, decks []*deck.Deck, games, clock int,
	seed *big.Int, onGame func(finished int, game *GameResult)) (*SimRun, error) {
	base, err := w.Ready(ctx)
	if err != nil {
		return nil, err
	}
	texts, err := DecksToWire(decks)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"decks": texts, "games": games,
		"clock": clock, "seed": seed, "stream": true}
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
	shimHeaders(req)

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

	return readStream(resp.Body, time.Duration(clock+120)*time.Second, cancel, onGame)
}

// streamLine is one newline-delimited line of the match stream. Exactly one of
// the three keys is set on any given line.
type streamLine struct {
	Game   *int      `json:"game"`
	Row    *WireGame `json:"row"`
	Error  *string   `json:"error"`
	Type   string    `json:"type"`
	Result *WireRun  `json:"result"`
}

func readStream(body io.Reader, perRead time.Duration, stall func(),
	onGame func(int, *GameResult)) (*SimRun, error) {
	// The stall timer: fired, it cancels the request, and the read below
	// fails rather than blocking forever on a worker that died mid-match.
	quiet := time.AfterFunc(perRead, stall)
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
				if onGame != nil {
					var game *GameResult
					if line.Row != nil {
						g := GameFromWire(*line.Row)
						game = &g
					}
					onGame(*line.Game, game)
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
