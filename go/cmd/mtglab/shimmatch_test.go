package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// A match that **finishes**, which is the one thing this door had never been
// asked to do.
//
// `shimdoor_test.go` holds who gets in, `shimabandon_test.go` holds what
// happens when the room empties, and between them every question about
// `/match` was about a match going wrong. The ordinary answer — 200, a row
// per game, the run at the end — was written by nothing, which is how the
// whole streamed conversation came to be listed in `ci.yml` as needing a JVM.
// It does not: the door plays its match through [shim.play] precisely so the
// handler can be driven without one, and what it says on the wire is the
// handler's own doing.
//
// **The answers are read as the app reads them**, over a real socket, one
// newline-delimited line at a time, and decoded with the same [tier3] wire
// codec the client uses. A test that asserted on the bytes would be pinning
// this end's spelling; decoding proves the far end could.

// finishedRun is what a bout looks like coming back: two games, one each, with
// a version and a wall clock on it.
func finishedRun() *tier3.SimRun {
	first, second := 1, 2
	firstLabel, secondLabel := "Ai(1)-gyome", "Ai(2)-trostani"
	turns := 9
	return &tier3.SimRun{
		Output: tier3.SimOutput{Games: []tier3.GameResult{
			{Index: 1, Milliseconds: 8055, Winner: &firstLabel,
				WinnerSeat: &first, Turns: &turns},
			{Index: 2, Milliseconds: 9100, Winner: &secondLabel,
				WinnerSeat: &second, Turns: &turns},
		}},
		Seats:        map[int]string{1: "gyome", 2: "trostani"},
		WallSeconds:  20.5,
		ForgeVersion: "1.6.50",
	}
}

// playingShim is a door whose matches are played by `play`, over a real
// socket.
func playingShim(t *testing.T, play func([]*deck.Deck, tier3.RunOptions) (*tier3.SimRun, error)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(&shim{
		state: newShimState(), log: log.New(io.Discard, "", 0), play: play,
	})
	t.Cleanup(srv.Close)
	return srv
}

// ndjson reads a streamed answer into the lines it is made of, decoded.
func ndjson(t *testing.T, body io.Reader) []map[string]json.RawMessage {
	t.Helper()
	var lines []map[string]json.RawMessage
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var line map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("the shim said something that is not a line of JSON: %q", scanner.Text())
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading the streamed answer: %v", err)
	}
	return lines
}

// The plain answer: no stream asked for, so the whole run crosses once, as
// JSON, with a status code that can still carry a refusal.
func TestAMatchNobodyAskedToWatchComesBackWhole(t *testing.T) {
	t.Parallel()
	var asked tier3.RunOptions
	srv := playingShim(t, func(decks []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
		asked = opt
		if len(decks) != 2 {
			t.Errorf("the door handed the match %d decks", len(decks))
		}
		return finishedRun(), nil
	})

	resp := post(t, srv, "/match", "", matchBodyPlain(t, 2, 4242))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("a finished match answered %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("the answer is %q", ct)
	}

	var wire tier3.WireRun
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		t.Fatalf("the app could not read the shim's answer: %v", err)
	}
	run := tier3.RunFromWire(wire)
	if len(run.Games()) != 2 {
		t.Fatalf("the match crossed with %d games", len(run.Games()))
	}
	// The seats crossed, which is what lets a winner be named as a deck
	// rather than as a chair.
	if run.SeatSlug(1) != "gyome" || run.SeatSlug(2) != "trostani" {
		t.Errorf("the seats are %v", run.Seats)
	}
	if run.ForgeVersion != "1.6.50" {
		t.Errorf("the run crossed as Forge %q", run.ForgeVersion)
	}
	// And the ask crossed the other way: the games, the clock and the seed
	// the caller named, with the machine's own heap ceiling added by the door.
	if asked.Games != 2 || asked.Clock != 300 {
		t.Errorf("the match was asked for as %+v", asked)
	}
	if asked.Seed == nil || asked.Seed.Int64() != 4242 {
		t.Errorf("the seed crossed as %v", asked.Seed)
	}
	if asked.Narrate {
		t.Error("a caller who never asked to watch was narrated to anyway")
	}
}

// The streamed conversation, whole: 200 up front, a beats line and a row per
// game as they finish, and exactly one closing line carrying the run.
//
// **The beats come out before the row that closes their game**, which is not
// an ordering anything guards but is the reason a far side may stash them and
// publish them together: both readers ride one pass over the subprocess, and
// the event parser is fed first.
func TestAStreamedMatchTicksPerGameAndClosesWithTheRun(t *testing.T) {
	t.Parallel()
	srv := playingShim(t, func(_ []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
		run := finishedRun()
		for i, game := range run.Games() {
			if opt.OnEvents != nil {
				opt.OnEvents(tier3.EventLog{Game: i + 1, Events: []tier3.GameEvent{
					{Kind: tier3.EventTurn, Seat: 1, Turn: 1},
					{Kind: tier3.EventLand, Seat: 1, Card: "Forest"},
				}})
			}
			opt.OnGame(i+1, game)
		}
		return run, nil
	})

	resp := post(t, srv, "/match", "", matchBody(t, 2))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a streamed match answered %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Errorf("the answer is %q, which is not the streamed conversation", ct)
	}

	lines := ndjson(t, resp.Body)
	var games, results int
	var lastGame int
	for _, line := range lines {
		switch {
		case line["game"] != nil:
			games++
			var n int
			if err := json.Unmarshal(line["game"], &n); err != nil {
				t.Fatalf("a tick crossed unreadable: %v", err)
			}
			if n != lastGame+1 {
				t.Errorf("the ticks ran %d then %d", lastGame, n)
			}
			lastGame = n
			// The row rides the tick, in the same encoding the final result
			// will carry it — which is what lets the far side seat a game the
			// moment it is over rather than at the end of the bout.
			var row tier3.WireGame
			if err := json.Unmarshal(line["row"], &row); err != nil {
				t.Fatalf("game %d crossed without a readable row: %v", n, err)
			}
			if got := tier3.GameFromWire(row); got.Index != n {
				t.Errorf("the row beside tick %d is game %d", n, got.Index)
			}
		case line["result"] != nil:
			results++
			if games != 2 {
				t.Errorf("the run closed after %d ticks", games)
			}
			var wire tier3.WireRun
			if err := json.Unmarshal(line["result"], &wire); err != nil {
				t.Fatalf("the closing run crossed unreadable: %v", err)
			}
			if len(tier3.RunFromWire(wire).Games()) != 2 {
				t.Error("the closing run had lost its games")
			}
		case line["error"] != nil:
			t.Errorf("a match that finished reported %s", line["error"])
		}
	}
	if games != 2 {
		t.Errorf("%d games ticked, want 2", games)
	}
	if results != 1 {
		t.Errorf("the conversation carried %d closing lines, want exactly one", results)
	}
}

// A caller that asked to hear the beats hears them, and one that did not gets
// no `events` lines at all — which is the whole of why narration is asked for
// rather than assumed: a nine-turn game is about a hundred beats and a
// twenty-game bout is not a thing anybody wants who is not watching.
//
// **The ask is what carries it, not the callback.** The door always hands the
// runner somewhere to put beats; what it passes on is the caller's own
// `narrate`, and a runner that was not asked to narrate never says a word. So
// the stand-in below narrates exactly when it was asked to, as Forge's reader
// does, and the assertion is that the flag crossed the wire and the lines
// followed it.
func TestTheBeatsCrossOnlyForACallerWhoAskedToHearThem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ narrate bool }{{true}, {false}} {
		srv := playingShim(t, func(_ []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
			if opt.Narrate != tc.narrate {
				t.Errorf("the door asked for narrate=%v, want %v", opt.Narrate, tc.narrate)
			}
			if opt.Narrate && opt.OnEvents != nil {
				opt.OnEvents(tier3.EventLog{Game: 1, Events: []tier3.GameEvent{
					{Kind: tier3.EventTurn, Seat: 1, Turn: 1}}})
			}
			return finishedRun(), nil
		})

		body, err := json.Marshal(map[string]any{
			"decks": wireDecks(t), "games": 1, "clock": 300,
			"stream": true, "narrate": tc.narrate})
		if err != nil {
			t.Fatal(err)
		}
		resp := post(t, srv, "/match", "", string(body))
		lines := ndjson(t, resp.Body)
		_ = resp.Body.Close()

		heard := 0
		for _, line := range lines {
			if line["events"] != nil {
				heard++
				var log tier3.EventLog
				if err := json.Unmarshal(line["events"], &log); err != nil {
					t.Fatalf("the beats crossed unreadable: %v", err)
				}
				if len(log.Events) == 0 {
					t.Error("an events line crossed with no beats in it")
				}
			}
		}
		if tc.narrate && heard == 0 {
			t.Error("a caller who asked to watch was told nothing")
		}
		if !tc.narrate && heard > 0 {
			t.Errorf("a caller who never asked to watch was sent %d beat lines", heard)
		}
	}
}

// A match that fell over on the streamed path says so **as a line**, not as a
// status code: the 200 was already spent buying the right to speak early. And
// the class name crosses beside the sentence, because a maintainer reading a
// job row wants to know whether Forge was missing or the run merely broke.
func TestAStreamedFailureIsALineWithItsClassOnIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		err  error
		kind string
	}{
		{"a worker with no distribution",
			tier3.NotInstalled("no Forge card data at /nowhere"), "ForgeNotInstalled"},
		{"a match that simply broke",
			errors.New("the JVM went away"), "RuntimeError"},
	} {
		srv := playingShim(t, func(_ []*deck.Deck, _ tier3.RunOptions) (*tier3.SimRun, error) {
			return nil, tc.err
		})
		resp := post(t, srv, "/match", "", matchBody(t, 1))
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s answered %d rather than speaking on the stream",
				tc.what, resp.StatusCode)
		}
		lines := ndjson(t, resp.Body)
		_ = resp.Body.Close()

		if len(lines) != 1 {
			t.Fatalf("%s said %d lines, want exactly one", tc.what, len(lines))
		}
		var kind, sentence string
		if err := json.Unmarshal(lines[0]["type"], &kind); err != nil {
			t.Fatalf("%s crossed without a class: %v", tc.what, err)
		}
		if err := json.Unmarshal(lines[0]["error"], &sentence); err != nil {
			t.Fatalf("%s crossed without a sentence: %v", tc.what, err)
		}
		if kind != tc.kind {
			t.Errorf("%s crossed as %q, want %q", tc.what, kind, tc.kind)
		}
		if sentence == "" {
			t.Errorf("%s crossed with an empty sentence", tc.what)
		}
	}
}

// A match that failed on the **plain** path answers with a status code,
// because nothing has been spent yet — and the two failures answer differently
// on purpose: a worker with no Forge is a 503 the app reads as "this machine
// is not ready", and anything else is a 500.
func TestAPlainMatchFailsWithAStatusRatherThanALine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		err  error
		code int
	}{
		{"a worker with no distribution",
			tier3.NotInstalled("no Forge card data at /nowhere"),
			http.StatusServiceUnavailable},
		{"a match that simply broke", errors.New("the JVM went away"),
			http.StatusInternalServerError},
	} {
		srv := playingShim(t, func(_ []*deck.Deck, _ tier3.RunOptions) (*tier3.SimRun, error) {
			return nil, tc.err
		})
		resp := post(t, srv, "/match", "", matchBodyPlain(t, 1, 0))
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.code {
			t.Errorf("%s answered %d, want %d: %s", tc.what, resp.StatusCode,
				tc.code, body)
		}
		if !strings.Contains(string(body), "error") {
			t.Errorf("%s answered %s", tc.what, body)
		}
	}
}

// The coverage pre-flight over the wire: deck texts in, one report out per
// deck, in the order they were sent — and no JVM booted to do it, which is the
// whole reason this route exists beside `/match`.
func TestTheCoverageRouteReportsOnEveryDeckItWasSent(t *testing.T) {
	t.Parallel()
	srv := newShimFor(t, tier3.Settings{
		Home: fakeForgeHome(t, "1.6.50", "Sol Ring", "Forest")})

	resp := post(t, srv, "/coverage", "", `{"decks":`+string(mustJSON(t, wireDecks(t)))+`}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("the pre-flight answered %d: %s", resp.StatusCode, body)
	}
	var answer struct {
		Reports []tier3.WireReport `json:"reports"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		t.Fatalf("the app could not read the reports: %v", err)
	}
	reports := tier3.ReportsFromWire(answer.Reports)
	if len(reports) != 2 {
		t.Fatalf("%d reports came back for 2 decks", len(reports))
	}
	for i, r := range reports {
		if !r.OK() {
			t.Errorf("deck %d was reported short: %s", i+1, r.Summary())
		}
		if r.Checked == 0 {
			t.Errorf("deck %d was reported on without a card being checked", i+1)
		}
	}
}

// A match asked for in a body the door cannot read is refused by name rather
// than guessed at: a deck list that is not a list of deck texts, and a games
// count that is not a number.
func TestAMatchAskedForInNonsenseIsRefusedRatherThanPlayed(t *testing.T) {
	t.Parallel()
	played := false
	srv := playingShim(t, func([]*deck.Deck, tier3.RunOptions) (*tier3.SimRun, error) {
		played = true
		return finishedRun(), nil
	})

	for _, tc := range []struct{ what, body string }{
		{"decks that are not deck texts", `{"decks":[17],"games":1}`},
		{"a deck text that is not a deck", `{"decks":["not a deck at all"],"games":1}`},
		{"a games count that is not a number", `{"decks":[],"games":"lots"}`},
		{"a seed that is not an integer", `{"decks":[],"seed":"seven"}`},
	} {
		resp := post(t, srv, "/match", "", tc.body)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400: %s", tc.what, resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "unreadable request") &&
			!strings.Contains(string(body), "ValueError") {
			t.Errorf("%s was refused with %s", tc.what, body)
		}
	}
	if played {
		t.Error("a match the door could not read was played anyway")
	}
}

// A match queued behind another one is **dropped** when its caller leaves,
// rather than seated the moment the slot comes free: the wait for the arena is
// raced against the request going away, which is the difference between a
// mutex and the channel this door uses.
// **Driven at the handler rather than over a socket**, and deliberately: the
// socket version is a stopwatch. A hang-up reaches a handler about 200ms after
// the peer closes (measured, and only because the body is drained first), so a
// test that hung up and then freed the arena would be asking which of two
// clocks won. What is under test is not the 200ms — `shimabandon_test.go`
// proves that end to end — it is the line the handler runs when the wait comes
// back false: a caller whose request is already over never reaches the arena,
// and the slot it did not take is still there for whoever comes next.
func TestAQueuedMatchIsNeverSeatedForACallerWhoHasGone(t *testing.T) {
	t.Parallel()
	played := 0
	handler := &shim{
		state: newShimState(),
		log:   log.New(io.Discard, "", 0),
		play: func(_ []*deck.Deck, _ tier3.RunOptions) (*tier3.SimRun, error) {
			played++
			return finishedRun(), nil
		},
	}
	// The arena is taken, as it is for the whole of somebody else's match.
	if !handler.state.takeMatch(context.Background()) {
		t.Fatal("a fresh door had no slot to give")
	}

	gone, hangUp := context.WithCancel(context.Background())
	hangUp()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/match",
		strings.NewReader(matchBodyPlain(t, 1, 0))).WithContext(gone)
	handler.ServeHTTP(rec, req)

	if played != 0 {
		t.Error("a match whose caller had already gone was played for nobody")
	}
	// And nothing was said on a socket that is not there: writing a refusal
	// to a listener that has hung up is another failed write, not news.
	if rec.Body.Len() != 0 {
		t.Errorf("the door answered an absent caller with %q", rec.Body.String())
	}
	// The slot it did not take is still free, which is the half that made the
	// old shape a zombie rather than merely a waste.
	handler.state.releaseMatch()
	if !handler.state.takeMatch(context.Background()) {
		t.Error("the arena was left held by a match nobody played")
	}
}

// `mtglab forge-shim` is a real subcommand and it refuses by name when the
// port is taken — which on the worker is the difference between a machine that
// says why it did not come up and one that simply is not there.
func TestTheShimSubcommandRefusesAPortItCannotHave(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	held := heldPort(t)
	d.Forge = tier3.Settings{
		ShimHost: "127.0.0.1", ShimPort: atoi(t, portOf(t, held)),
		Home: t.TempDir(),
	}
	_, err := d.run(t, "forge-shim")
	if err == nil {
		t.Fatal("`mtglab forge-shim` bound a port somebody else was holding")
	}
	if !strings.Contains(err.Error(), "forge shim") {
		t.Errorf("the refusal said %q without naming what would not start", err)
	}
}

// The idle watch is what stops the worker machine, and it is the one verdict
// nothing could ever watch: it ends the process.
//
// So the three things that make it a process ender are values — how often it
// looks, where it says so, and what it does about a machine that has gone
// quiet — and what is left is the judgement. A machine past its limit is
// stopped, the line names the limit it gave up on, and it is said exactly once
// rather than once per look.
func TestAQuietMachineIsStoppedAndSaysWhichLimitItPassed(t *testing.T) {
	t.Parallel()
	state := newShimState()
	said := &syncWriter{}
	stopped := make(chan int, 4)
	go idleWatch{
		state: state,
		limit: time.Millisecond,
		every: 5 * time.Millisecond,
		say:   said,
		exit:  func(code int) { stopped <- code },
	}.run()

	select {
	case code := <-stopped:
		// Clean, which is what turns `restart: no` into `stopped` rather than
		// into a machine Fly restarts.
		if code != 0 {
			t.Errorf("the machine was stopped with %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a machine that had been quiet the whole time was never stopped")
	}
	if got := said.String(); !strings.Contains(got, "idle for 0s, stopping the machine") {
		t.Errorf("the last line an operator reads is %q", got)
	}
}

// And the other half, which is the one that matters on a real worker: a
// streamed match is minutes of silence between lines by design, so a watch
// that read silence as idleness would kill matches at the three-minute mark
// and look exactly like a Forge crash.
func TestAMachineWithWorkOnItIsNeverStopped(t *testing.T) {
	t.Parallel()
	state := newShimState()
	state.begin() // a match in flight, saying nothing
	stopped := make(chan int, 1)
	go idleWatch{
		state: state,
		limit: time.Millisecond,
		every: time.Millisecond,
		say:   io.Discard,
		exit:  func(code int) { stopped <- code },
	}.run()

	select {
	case <-stopped:
		t.Fatal("the machine was stopped under a match that was still being played")
	case <-time.After(150 * time.Millisecond):
	}
	// And once the work is done it goes, which proves the watch was looking
	// the whole time rather than simply broken.
	state.end()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Error("the machine never stopped once its work was finished")
	}
}

// A watch is only armed when the machine was given a limit: zero idle seconds
// means never stop, which is what a laptop running the shim by hand wants.
func TestAShimWithNoIdleLimitArmsNoWatchAtAll(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A limit small enough that a watch, if one were armed, would have fired
	// long before this test finished -- and this process is still here.
	l := heldPort(t)
	forge := tier3.Settings{
		ShimHost: "127.0.0.1", ShimPort: atoi(t, portOf(t, l)),
		Home: t.TempDir(), IdleSeconds: 0,
	}
	done := make(chan error, 1)
	go func() {
		done <- serveShimOn(ctx, forge, func() (net.Listener, error) { return l, nil })
	}()
	base := "http://127.0.0.1:" + portOf(t, l)
	resp, err := waitForHealth(t, base+"/healthz", done)
	if err != nil {
		t.Fatalf("the shim never answered: %v", err)
	}
	_ = resp.Body.Close()
}

// syncWriter is an [io.Writer] a test can read while a goroutine writes to it.
type syncWriter struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// wireDecks is two decks in the encoding `/match` and `/coverage` take, both
// of them covered by [fakeForgeHome]'s cardsfolder.
func wireDecks(t *testing.T) []string {
	t.Helper()
	texts, err := tier3.DecksToWire([]*deck.Deck{
		{Slug: "gyome", Name: "gyome", Commander: []string{"Sol Ring"},
			Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}},
		{Slug: "trostani", Name: "trostani", Commander: []string{"Sol Ring"},
			Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}},
	})
	if err != nil {
		t.Fatalf("the decks would not cross the wire: %v", err)
	}
	return texts
}

// matchBodyPlain is a `/match` ask with no stream: the shape a caller that
// only wants the result uses. A zero seed is an unseeded ask.
func matchBodyPlain(t *testing.T, games int, seed int64) string {
	t.Helper()
	ask := map[string]any{"decks": wireDecks(t), "games": games, "clock": 300}
	if seed != 0 {
		ask["seed"] = seed
	}
	return string(mustJSON(t, ask))
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
