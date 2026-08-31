package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// What happens to a match when nobody is listening any more.
//
// **This is the zombie, and it closed a deployed arena for over an hour.** On
// 2026-08-30 the app cancelled a twenty-game bout mid-match. The shim went on
// playing it: the write failures were swallowed on purpose — *"a vanished
// listener must not kill the JVM mid-game"* — the request therefore stayed in
// flight, `inFlight` never fell to zero, and the idle watchdog cannot stop a
// machine with work on it. So the worker sat "started" playing to an empty
// room until its subprocess ran out of its own clock, and every later bout
// queued behind a match that had no audience and no result. It took a machine
// stopped by hand to clear it.
//
// The reasoning behind swallowing the error had the cost backwards. A listener
// that has gone is the one unambiguous sign that a match is worth nothing to
// anybody, and it is now the signal to stop.

// abandonShim is a door whose matches are played by a stand-in, so the
// handler's own behaviour can be driven over a real socket without a JVM.
type abandonShim struct {
	*httptest.Server
	// started closes when a match begins, so a test can hang up at a moment
	// when there is really something to abandon.
	started chan struct{}
	// aborted carries the match's own view: closed when the runner saw its
	// abort channel close, which is the thing under test.
	aborted chan struct{}
	// release lets the stand-in match end at teardown. A test whose client
	// never hangs up would otherwise hold `httptest.Server.Close` for as long
	// as the match pretends to run.
	release chan struct{}
	state   *shimState
}

// matchBody is a real `/match` ask: two decks in the wire's own encoding,
// because the door decodes them before it does anything else and a
// placeholder would be refused at the wrong line.
func matchBody(t *testing.T, games int) string {
	t.Helper()
	decks := []*deck.Deck{
		{Slug: "gyome", Name: "gyome", Commander: []string{"Sol Ring"},
			Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}},
		{Slug: "trostani", Name: "trostani", Commander: []string{"Sol Ring"},
			Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}},
	}
	texts, err := tier3.DecksToWire(decks)
	if err != nil {
		t.Fatalf("the decks would not cross the wire: %v", err)
	}
	raw, err := json.Marshal(map[string]any{
		"decks": texts, "games": games, "clock": 300, "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func newAbandonShim(t *testing.T) *abandonShim {
	t.Helper()
	s := &abandonShim{
		started: make(chan struct{}),
		aborted: make(chan struct{}),
		release: make(chan struct{}),
		state:   newShimState(),
	}
	handler := &shim{
		state: s.state,
		log:   log.New(io.Discard, "", 0),
		play: func(_ []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
			close(s.started)
			select {
			case <-opt.Abort:
				close(s.aborted)
			case <-s.release:
			}
			// Exactly what a killed subprocess produces, so the handler meets
			// the real error rather than a convenient one.
			return nil, tier3.ErrAbandoned
		},
	}
	s.Server = httptest.NewServer(handler)
	// Registered before the release below, so teardown runs them the other way
	// round: the match is let go first and only then is the server closed.
	t.Cleanup(s.Close)
	t.Cleanup(func() { close(s.release) })
	return s
}

// The whole fault, driven rather than described: a real client on a real
// socket asks for a match, waits for it to start, and hangs up.
func TestAMatchWhoseListenerHangsUpIsStoppedRatherThanPlayedOut(t *testing.T) {
	t.Parallel()
	arena := newAbandonShim(t)

	ctx, hangUp := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		arena.URL+"/match", strings.NewReader(matchBody(t, 20)))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("the arena would not take the match: %v", err)
	}
	defer resp.Body.Close()

	// The 200 arrives before the match does, which is what lets the app bound
	// getting in separately from playing.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the arena answered %d before the match began", resp.StatusCode)
	}

	select {
	case <-arena.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the match never started, so there was nothing to abandon")
	}

	// The listener goes away, exactly as an app whose bout was cancelled does.
	hangUp()

	select {
	case <-arena.aborted:
	case <-time.After(20 * time.Second):
		t.Fatal("the match played on for a listener that had gone — this is " +
			"the zombie that held a deployed arena for an hour")
	}

	// **And the machine is free again.** The abort is only half the fix: what
	// made it a zombie rather than a waste was that the request stayed in
	// flight, so the idle watchdog could never stop the machine under it.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if arena.state.idleFor() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the door still counts itself busy after the match was " +
		"abandoned, so the machine can never go to sleep")
}

// A second bout queued behind the first is not played for a caller who has
// already gone — the wait for the match slot is given up on, rather than
// spending a JVM on a result nobody will ever read.
func TestAQueuedMatchIsDroppedWhenItsCallerLeaves(t *testing.T) {
	t.Parallel()
	state := newShimState()
	if !state.takeMatch(context.Background()) {
		t.Fatal("the first match could not take a free slot")
	}
	defer state.releaseMatch()

	ctx, gone := context.WithCancel(context.Background())
	queued := make(chan bool, 1)
	go func() { queued <- state.takeMatch(ctx) }()

	select {
	case <-queued:
		t.Fatal("a second match took the slot while the first still held it")
	case <-time.After(50 * time.Millisecond):
	}

	gone()
	select {
	case got := <-queued:
		if got {
			t.Error("a match whose caller had left was seated anyway")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the queued match never gave up, so it would have played to " +
			"nobody the moment the slot came free")
	}
}

// The headers go out when the slot is won, not when the first game ends.
//
// That ordering is what makes the app's acquire clock mean anything: the shim
// takes its one match slot before it answers, so a 200 is the arena saying
// *you are in*. Without the flush the answer sat in the server's buffer until
// something else pushed it, and a caller could not tell a queue from a slow
// game — which is why waiting to get in had no bound worth the name.
func TestTheArenaAnswersWhenItSeatsTheMatchNotWhenItFinishes(t *testing.T) {
	t.Parallel()
	arena := newAbandonShim(t)

	resp, err := http.Post(arena.URL+"/match", "application/json",
		strings.NewReader(matchBody(t, 20)))
	if err != nil {
		t.Fatalf("the arena would not take the match: %v", err)
	}
	defer resp.Body.Close()

	// `Post` returning at all is the assertion: the stand-in match never
	// finishes on its own, so a buffered header would still be unwritten here.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the arena answered %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Errorf("the answer is %q, which is not the streamed conversation", ct)
	}
	select {
	case <-arena.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the arena answered before it had even seated the match")
	}
}

// An abandoned match says nothing on the wire, and it must not: the socket it
// would say it on is the one that just closed. It is not a failure either —
// nobody is left for it to be news to.
func TestAnAbandonedMatchIsNotDressedUpAsAFailure(t *testing.T) {
	t.Parallel()
	var wrote []byte
	rec := httptest.NewRecorder()
	handler := &shim{
		state: newShimState(),
		log:   log.New(io.Discard, "", 0),
		play: func(_ []*deck.Deck, _ tier3.RunOptions) (*tier3.SimRun, error) {
			return nil, tier3.ErrAbandoned
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/match",
		strings.NewReader(matchBody(t, 2)))
	handler.ServeHTTP(rec, req)
	wrote = rec.Body.Bytes()

	if len(wrote) == 0 {
		return
	}
	// Whatever came out, it must not be an error line: a room that had gone
	// would not read it, and a room still there would be told its own hang-up
	// was the arena falling over.
	for _, line := range strings.Split(strings.TrimSpace(string(wrote)), "\n") {
		var answer struct {
			Error *string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &answer); err != nil {
			continue
		}
		if answer.Error != nil {
			t.Errorf("an abandoned match reported %q as a failure", *answer.Error)
		}
	}
}
