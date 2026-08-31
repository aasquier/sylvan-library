package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// match serialises the JVM: one token, held for the length of a match.
	//
	// **A channel rather than a `sync.Mutex`, so waiting for it can be given
	// up on.** A mutex can only be waited on unconditionally, which meant a
	// queued match played out even when whoever asked for it had long since
	// gone — and a second bout behind a first one had no way to notice its own
	// caller hanging up. Waiting on a channel can be raced against a cancelled
	// request, which is the whole difference.
	match chan struct{}
}

func newShimState() *shimState {
	s := &shimState{lastActivity: time.Now(), match: make(chan struct{}, 1)}
	s.match <- struct{}{}
	return s
}

// takeMatch waits for the one match slot, giving up if `ctx` dies first.
//
// Reports whether the slot is held; a false answer means the caller left and
// there is nothing left to play for.
func (s *shimState) takeMatch(ctx context.Context) bool {
	select {
	case <-s.match:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *shimState) releaseMatch() { s.match <- struct{}{} }

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
	// play is how a match is run, and it exists so this door can be driven
	// without a JVM. Nil means [tier3.Settings.RunGames], which is every
	// deployment.
	//
	// **A seam rather than a mock of the whole door.** What has to be tested
	// here is what happens to a match when its listener hangs up, and that is a
	// property of this handler — the abort channel it builds, the writes it
	// watches, and how quickly it lets go of the machine afterwards. Testing it
	// against a real Forge would mean a 470MB distribution and minutes per
	// case; testing it without one meant, until this field, not testing it at
	// all. The zombie that closed a deployed arena for an hour lived in exactly
	// that gap.
	play func(decks []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error)
}

func (s *shim) runGames(decks []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
	if s.play != nil {
		return s.play(decks, opt)
	}
	return s.forge.RunGames(decks, opt)
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
	// **Read to the end, which is what makes a hang-up visible at all.** Go's
	// server only starts watching a connection for the client going away once
	// the request body has been consumed — a decoder stops at the closing
	// brace, so until this line the handler could sit on `r.Context()` for an
	// hour and never be told the caller had left. Measured: with the body
	// drained the cancellation arrives in about 200ms, and without it, never.
	//
	// It matters for exactly one moment, and it is the moment that mattered: a
	// match waiting for the arena's one slot has written nothing yet, so this
	// is its only way to learn there is no longer anybody to play for. Once
	// the streamed answer starts, a failed write says the same thing.
	_, _ = io.Copy(io.Discard, r.Body)

	switch r.URL.Path {
	case "/coverage":
		s.coverage(w, body)
	case "/match":
		s.match(w, r, body)
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

func (s *shim) match(w http.ResponseWriter, r *http.Request,
	body map[string]json.RawMessage) {
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

	// One JVM at a time, whatever the caller believes about its own lane —
	// and never for a caller who has already gone. A match queued behind
	// another one used to be played out regardless of whether anybody was
	// still waiting for it, which is how one abandoned bout became an hour of
	// a machine playing to an empty room.
	if !s.state.takeMatch(r.Context()) {
		return
	}
	defer s.state.releaseMatch()

	if ask.Stream {
		s.matchStreamed(w, r, decks, ask)
		return
	}
	run, err := s.runGames(decks, tier3.RunOptions{Games: ask.Games,
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
func (s *shim) matchStreamed(w http.ResponseWriter, r *http.Request,
	decks []*deck.Deck, ask matchAsk) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	// **Flushed before the JVM starts, so the 200 means "the arena is yours".**
	// The match slot is taken above this line, and the headers sit in the
	// server's buffer until something pushes them — so a caller waiting for
	// this response was really waiting for the first *game*, minutes later,
	// with no way to tell a queue from a slow match. Flushing here makes the
	// answer arrive the moment the slot is won, which is what lets the app
	// bound getting in separately from playing (`tier3.ArenaBudget`).
	if flusher != nil {
		flusher.Flush()
	}

	// **The match ends when nobody is listening.** This used to swallow the
	// write error and play on, deliberately — "a vanished listener must not
	// kill the JVM mid-game". That reasoning had the cost backwards: the
	// request stays in flight for the whole abandoned match, `inFlight` never
	// falls to zero, and the idle watchdog therefore cannot stop the machine.
	// On 2026-08-30 that left a worker playing a twenty-game bout to nobody
	// while every later bout queued behind it, until somebody stopped the
	// machine by hand. A listener that has gone is the one unambiguous sign
	// that a match is worth nothing to anybody, so it is now the signal to
	// stop.
	abort := make(chan struct{})
	var once sync.Once
	gone := func() { once.Do(func() { close(abort) }) }

	emit := func(payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err := w.Write(append(raw, '\n')); err != nil {
			gone()
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Two ways to learn the same thing, because neither is reliable alone: a
	// write only fails once the peer's close has been noticed, and a request
	// context is only cancelled once the server's background read sees it. The
	// watcher is wound up with the handler.
	watching := make(chan struct{})
	defer close(watching)
	go func() {
		select {
		case <-r.Context().Done():
			gone()
		case <-watching:
		}
	}()

	// The row rides the tick (the match theater): the game the parser just
	// completed crosses beside its count, in the same encoding the final
	// result will carry it. The beats ride their own line, and only when the
	// caller asked to hear them — an [tier3.EventLog] crosses as itself, for
	// the reason `wire.go` gives.
	run, err := s.runGames(decks, tier3.RunOptions{Games: ask.Games,
		Clock: ask.Clock, Seed: ask.Seed,
		Memory:  s.forge.MemoryMB,
		Narrate: ask.Narrate,
		Abort:   abort,
		OnEvents: func(log tier3.EventLog) {
			emit(map[string]any{"events": log})
		},
		OnGame: func(n int, g tier3.GameResult) {
			emit(map[string]any{"game": n, "row": tier3.GameToWire(g)})
		}})
	if err != nil {
		// An abandoned match has no audience to tell, and saying so on a
		// socket nobody is reading would only be another failed write. It goes
		// to the log, where the machine's own operator reads.
		if errors.Is(err, tier3.ErrAbandoned) {
			s.log.Printf("forge shim: the match was abandoned by its caller, "+
				"stopping after %d game(s) asked for", ask.Games)
			return
		}
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
