package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/spf13/cobra"
)

// The worker machine's door — three endpoints in front of a Forge run.
//
// This is what `Dockerfile.forge` runs (ADR 35). It is deliberately the
// smallest server that can hold a JVM: JSON in and out, no framework — the
// worker image already carries ~470MB of Forge and a JRE, and a web stack
// would be surface area on a machine whose whole job is one subprocess. The
// app talks to it over Fly's private network; nothing routes here from the
// internet.
//
//	GET  /healthz    am I up? (also what the app polls after a machine start)
//	POST /coverage   deck.yaml texts -> coverage reports, no JVM booted
//	POST /match      deck.yaml texts + games/clock/seed -> a finished run
//
// **It is a subcommand of `mtglab` rather than a second binary**, and that
// shape is a decision, not an accident. The worker image already has to
// carry the binary; `mtglab forge-shim` is that binary wearing a second
// hat, so the two images ship the same artefact and an artefact skew
// between them is impossible rather than merely unlikely.
//
// Three properties worth stating, all of them load-bearing:
//
//   - **The pre-flight still runs twice.** `/coverage` answers the request
//     thread's refusal check, and `/match` calls [tier3.RunGames], which
//     re-checks before and after. The worker never trusts that the app asked
//     first.
//   - **A bearer token gates every request when `MTGLAB_FORGE_SHIM_TOKEN` is
//     set.** The private network is already org-scoped; the token is the cheap
//     second wall, and the deployed app shares it automatically because Fly
//     injects one app's secrets into every machine of that app.
//   - **The shim stops its own machine.** Fly's proxy-driven auto-stop never
//     sees private-network traffic, so idleness is judged here: after
//     `MTGLAB_FORGE_IDLE_SECONDS` with no request and no match in flight, the
//     process exits cleanly and the machine's `restart: no` policy turns that
//     into `stopped` — the state ADR 35 prices.
//
// One match at a time, enforced with a lock. The app's FORGE lane already
// serialises matches, but that is a property of one process on another
// machine; the `.dck` directory this process hands to Forge is racy under two
// JVMs, so the worker enforces its own invariant rather than inheriting one.

// The two settings this door used to own -- how long it stays up when nobody
// asks, and how much heap a match gets -- now live on [tier3.Settings] as
// [tier3.DefaultIdleSeconds] and [tier3.DefaultMemoryMB], beside the rest of
// the Forge environment. They were read here through a local `envInt`, which
// meant the shim tests could describe an idle timeout only by setting a
// variable on the process; ADR 40 moved the read to one place and left the
// argument for the numbers where the numbers are.

func shimCommand(forge tier3.Settings) *cobra.Command {
	return &cobra.Command{
		Use:   "forge-shim",
		Short: "Serve Forge matches to the app over the private network (ADR 35)",
		Long: "The Forge worker's door. Runs on the forge-worker machine, " +
			"holds the JVM, and stops its own machine when idle.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serveShim(cmd.Context(), forge)
		},
	}
}

// shimState is what the idle watchdog judges: when work last happened, and
// whether any is happening right now.
type shimState struct {
	mu           sync.Mutex
	lastActivity time.Time
	inFlight     int
	// match serialises the JVM. Separate from mu, which is only ever held for
	// the length of a counter update.
	match sync.Mutex
}

func newShimState() *shimState {
	return &shimState{lastActivity: time.Now()}
}

func (s *shimState) begin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
	s.inFlight++
}

func (s *shimState) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
	s.inFlight--
}

func (s *shimState) idleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight > 0 {
		return 0
	}
	return time.Since(s.lastActivity)
}

type shim struct {
	state *shimState
	log   *log.Logger
	// forge is the distribution this door serves and the token it demands.
	forge tier3.Settings
}

func (s *shim) authorised(r *http.Request) bool {
	token := s.forge.ShimToken
	if token == "" {
		return true
	}
	supplied := r.Header.Get("Authorization")
	// Constant time, as `hmac.compare_digest` is — a token compared with `==`
	// leaks its prefix to anybody who can time the private network.
	return subtle.ConstantTimeCompare([]byte(supplied), []byte("Bearer "+token)) == 1
}

func (s *shim) send(w http.ResponseWriter, code int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"error":"the shim could not encode its own answer"}`)
		code = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func (s *shim) fail(w http.ResponseWriter, code int, format string, args ...any) {
	s.send(w, code, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func (s *shim) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.state.begin()
	defer s.state.end()

	if r.Method == http.MethodGet {
		if r.URL.Path != "/healthz" {
			s.fail(w, http.StatusNotFound, "no route %s", r.URL.Path)
			return
		}
		if !s.authorised(r) {
			s.fail(w, http.StatusUnauthorized, "bad or missing token")
			return
		}
		s.send(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method != http.MethodPost {
		s.fail(w, http.StatusNotFound, "no route %s", r.URL.Path)
		return
	}
	if !s.authorised(r) {
		s.fail(w, http.StatusUnauthorized, "bad or missing token")
		return
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, http.StatusBadRequest, "unreadable request: %v", err)
		return
	}

	switch r.URL.Path {
	case "/coverage":
		s.coverage(w, body)
	case "/match":
		s.match(w, body)
	default:
		s.fail(w, http.StatusNotFound, "no route %s", r.URL.Path)
	}
}

func decksFromBody(body map[string]json.RawMessage) ([]*deck.Deck, error) {
	var texts []string
	if raw, ok := body["decks"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &texts); err != nil {
			return nil, err
		}
	}
	return tier3.DecksFromWire(texts)
}

func (s *shim) coverage(w http.ResponseWriter, body map[string]json.RawMessage) {
	decks, err := decksFromBody(body)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "ValueError: %v", err)
		return
	}
	index, err := s.forge.ImplementedNames()
	if err != nil {
		s.fail(w, http.StatusServiceUnavailable, "%v", err)
		return
	}
	reports := make([]tier3.CoverageReport, 0, len(decks))
	for _, d := range decks {
		reports = append(reports, tier3.Check(d, index))
	}
	s.send(w, http.StatusOK, map[string]any{
		"reports": tier3.ReportsToWire(reports)})
}

type matchAsk struct {
	Games  int      `json:"games"`
	Clock  int      `json:"clock"`
	Seed   *big.Int `json:"seed"`
	Stream bool     `json:"stream"`
	// Narrate drops Forge's `-q`, so the subprocess tells the whole game and
	// [tier3.EventParser] reads it. Asked for rather than assumed, and only
	// ever on the streamed path: the beats are worth a hundred lines a game
	// and they exist to be *watched*, so a caller that is not listening live
	// has no use for them. `events.go` carries the measurement.
	Narrate bool `json:"narrate"`
}

func (s *shim) match(w http.ResponseWriter, body map[string]json.RawMessage) {
	decks, err := decksFromBody(body)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "unreadable request: %v", err)
		return
	}
	ask := matchAsk{Games: 1, Clock: 300}
	for key, into := range map[string]any{
		"games": &ask.Games, "clock": &ask.Clock,
		"seed": &ask.Seed, "stream": &ask.Stream, "narrate": &ask.Narrate,
	} {
		raw, ok := body[key]
		if !ok || string(raw) == "null" {
			continue
		}
		if err := json.Unmarshal(raw, into); err != nil {
			s.fail(w, http.StatusBadRequest, "unreadable request: %v", err)
			return
		}
	}

	// One JVM at a time, whatever the caller believes about its own lane.
	s.state.match.Lock()
	defer s.state.match.Unlock()

	if ask.Stream {
		s.matchStreamed(w, decks, ask)
		return
	}
	run, err := s.forge.RunGames(decks, tier3.RunOptions{Games: ask.Games,
		Clock: ask.Clock, Seed: ask.Seed, Memory: s.forge.MemoryMB})
	if err != nil {
		if errors.Is(err, tier3.ErrForgeNotInstalled) {
			s.fail(w, http.StatusServiceUnavailable, "%v", err)
			return
		}
		s.fail(w, http.StatusInternalServerError, "%s", failureText(err))
		return
	}
	s.send(w, http.StatusOK, tier3.RunToWire(run))
}

// matchStreamed is the same match, reported one game at a time.
//
// A match takes minutes and a job on the far side wants to tick per game, so a
// `{"stream": true}` request answers 200 up front and then speaks
// newline-delimited JSON as the match runs: `{"events": ...}` and then
// `{"game": n, "row": ...}` per finished game, then exactly one of
// `{"result": ...}` or `{"error": ...}`.
// The headers go out before the run does, which is why failure is a line
// rather than a status code here — 200 was already spent buying the right to
// speak early.
//
// **The beats come out before the row that closes their game**, which is not
// an ordering worth guarding but is worth knowing: both readers ride one pass
// over the subprocess (`tier3.spawn`), and the event parser is fed first. A
// far side that stashes the beats and publishes them with the row therefore
// never publishes a row whose game it has not already heard.
//
// A caller that never asks to stream gets the plain answer, and one that never
// asks to narrate gets no `events` lines, so an app deployed a few minutes
// apart from its worker keeps working in both directions and across both
// flags.
func (s *shim) matchStreamed(w http.ResponseWriter, decks []*deck.Deck, ask matchAsk) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	emit := func(payload any) {
		// A vanished listener must not kill the JVM mid-game: the write fails
		// quietly and the match plays out. The idle watchdog cannot stop the
		// machine under it — this request is still in flight.
		raw, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err := w.Write(append(raw, '\n')); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	// The row rides the tick (the match theater): the game the parser just
	// completed crosses beside its count, in the same encoding the final
	// result will carry it. The beats ride their own line, and only when the
	// caller asked to hear them — an [tier3.EventLog] crosses as itself, for
	// the reason `wire.go` gives.
	run, err := s.forge.RunGames(decks, tier3.RunOptions{Games: ask.Games,
		Clock: ask.Clock, Seed: ask.Seed,
		Memory:  s.forge.MemoryMB,
		Narrate: ask.Narrate,
		OnEvents: func(log tier3.EventLog) {
			emit(map[string]any{"events": log})
		},
		OnGame: func(n int, g tier3.GameResult) {
			emit(map[string]any{"game": n, "row": tier3.GameToWire(g)})
		}})
	if err != nil {
		kind := "RuntimeError"
		if errors.Is(err, tier3.ErrForgeNotInstalled) {
			kind = "ForgeNotInstalled"
		}
		emit(map[string]any{"error": failureText(err), "type": kind})
		return
	}
	emit(map[string]any{"result": tier3.RunToWire(run)})
}

// failureText renders an error as `<ClassName>: <sentence>`, which is what
// the app renders as a job's error. The class name is the half that survives
// translation: the client shows the sentence, and a maintainer reading a job
// row wants to know whether Forge was missing, the match timed out, or the
// results were untrustworthy.
func failureText(err error) string {
	switch {
	case errors.Is(err, tier3.ErrForgeNotInstalled):
		return "ForgeNotInstalled: " + err.Error()
	case errors.Is(err, tier3.ErrCoverageFailed):
		return "CoverageFailed: " + err.Error()
	case errors.Is(err, tier3.ErrResultsUntrustworthy):
		return "ResultsUntrustworthy: " + err.Error()
	case errors.Is(err, tier3.ErrTimedOut):
		return "TimeoutExpired: " + err.Error()
	}
	return "RuntimeError: " + err.Error()
}

func serveShim(ctx context.Context, forge tier3.Settings) error {
	addr := net.JoinHostPort(forge.ShimHost, strconv.Itoa(forge.ShimPort))
	return serveShimOn(ctx, forge, func() (net.Listener, error) {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("forge shim: %w", err)
		}
		return l, nil
	})
}

// serveShimOn is [serveShim] with the listener supplied rather than named, for
// the same reason [serveOn] takes one: an address is not a reservation, and a
// caller that hands over a port number cannot hold it while this boots. The
// argument is made in full on [serveOn].
func serveShimOn(ctx context.Context, forge tier3.Settings,
	listen func() (net.Listener, error)) error {
	limit := forge.IdleSeconds

	state := newShimState()
	handler := &shim{state: state, log: log.Default(), forge: forge}

	if limit > 0 {
		go watchdog(state, time.Duration(limit)*time.Second)
	}
	// Warm the coverage index while nobody is waiting: the first `/coverage`
	// after a cold boot would otherwise pay the ~2s zip scan inside somebody's
	// request thread.
	if _, err := forge.ImplementedNames(); err != nil {
		// A worker image without Forge is misbuilt; say so and keep serving,
		// because /healthz answering is what lets the app read the 503s.
		fmt.Printf("forge shim: %v\n", err)
	}

	server := &http.Server{
		Handler: handler,
		// No write timeout: a streamed match is minutes of silence between
		// lines by design, and a deadline here would cut it off mid-JVM. The
		// far side bounds every wait it makes, and the subprocess is bounded
		// whole.
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	listener, err := listen()
	if err != nil {
		return err
	}
	fmt.Printf("forge shim: listening on %s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func watchdog(state *shimState, limit time.Duration) {
	for {
		time.Sleep(15 * time.Second)
		if state.idleFor() > limit {
			fmt.Printf("forge shim: idle for %ds, stopping the machine\n",
				int(limit.Seconds()))
			// Abrupt on purpose: there is nothing to flush, no state to keep,
			// and a clean exit is what turns `restart: no` into `stopped`.
			os.Exit(0)
		}
	}
}
