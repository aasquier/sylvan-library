package tier3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The clocks a bout runs against, and the live fault that made them three.
//
// On 2026-08-30 a twenty-game match on the deployed instance was killed by the
// app that asked for it, nineteen minutes and forty-eight seconds in:
//
//	ERROR the Forge match failed error="forge worker: /match failed: context canceled"
//	job failed job=386131489002 kind=sim.forge err="the match broke off before it reached a result…"
//
// "Canceled" rather than "deadline exceeded" says something *called* cancel,
// and there was exactly one caller in the path: the stall timer in
// [readStreamOn], armed at `clock + 120` seconds. That is a budget describing
// one game, and it was the only thing bounding a bout of twenty — while the
// far side, playing the same match, had [SubprocessBudget]'s hour and a
// half. The belt was tighter than the suspenders it was written to back up,
// and the longer the bout the likelier it was to give way first.
//
// Each test below is one of the three questions that used to be answered by
// that single constant.

// The reproduction, kept: a stream that goes quiet mid-bout produces the exact
// sentence the instance logged, through the real HTTP path rather than by
// calling the reader directly.
//
// [Worker.Stall] is the seam that makes this runnable. The boundary is minutes
// wide in production, and the only way this fault was reproduced in the first
// place was by editing the budget by hand until it landed in seconds — which
// is a thing a test should be able to do without editing the code under it.
func TestAQuietStreamIsTheSentenceTheInstanceLogged(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		// Two games land, and then the worker stops talking the way one that
		// has died mid-match does.
		for i := 1; i <= 2; i++ {
			_, _ = io.WriteString(w, fmt.Sprintf(`{"game":%d}`, i)+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		<-r.Context().Done()
	})

	worker := shim.worker(time.Second, "")
	worker.Stall = 150 * time.Millisecond
	_, err := worker.RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome"), testDeck("trostani")},
		MatchAsk{Games: 20, Clock: 300, Seed: big.NewInt(42)})
	if err == nil {
		t.Fatal("a worker that stopped talking was waited on for the whole bout")
	}
	// The sentence matters as much as the failure: it is what the instance's
	// log said, and matching it is what makes this the same fault rather than
	// a lookalike.
	if err.Error() != "forge worker: /match failed: context canceled" {
		t.Fatalf("the match failed with %q, which is not what the deployed "+
			"instance logged", err.Error())
	}
	// A worker that went quiet is not a busy arena, and telling a person to
	// come back in a moment would be wrong about both.
	if errors.Is(err, ErrArenaBusy) {
		t.Error("a dead stream was reported as an arena somebody else is using")
	}
}

// **The budget that fired must no longer be reachable at the same silence.**
// This is the boundary test: the gap that killed the live bout — the old
// `clock + 120` — is now inside what one silence may be, and the whole bout is
// bounded by something that grows with the ask instead.
func TestTheBoutsClocksAreBoundAgainstTheAskRatherThanAConstant(t *testing.T) {
	t.Parallel()
	const clock = ForgeClockForTests

	// The exact expression that cancelled the live match.
	fired := time.Duration(clock+120) * time.Second
	if got := StallBudget(clock); got <= fired {
		t.Errorf("one silence may last %v, and the silence that killed a real "+
			"bout was %v — the boundary has not moved", got, fired)
	}

	// The whole bout scales with the games asked for, which is the house
	// principle: a clock is bound against the question it asks.
	small, large := MatchBudget(1, clock), MatchBudget(20, clock)
	if large <= small {
		t.Errorf("a twenty-game bout is given %v and a one-game bout %v — the "+
			"budget does not grow with the ask", large, small)
	}
	if want := 19 * clock * int(time.Second); large-small != time.Duration(want) {
		t.Errorf("nineteen more games bought %v rather than %v of clock",
			large-small, time.Duration(want))
	}

	// **The belt must outlast the suspenders.** The far side kills its own
	// subprocess at [SubprocessBudget]; a client that gave up first would cut
	// off the very answer it was waiting for, which is the shape of the fault
	// this file is about.
	for _, games := range []int{1, 5, 20} {
		if MatchBudget(games, clock) <= SubprocessBudget(games, clock) {
			t.Errorf("over %d games the app gives up at %v while the worker "+
				"plays until %v", games, MatchBudget(games, clock),
				SubprocessBudget(games, clock))
		}
	}

	// A zero clock is a caller that named none, not a bout with no time in it.
	if StallBudget(0) != StallBudget(ClockDefault) {
		t.Error("an unnamed clock did not fall back to the default")
	}
}

// **Waiting to get in is bounded apart from playing**, and it is the wait that
// had no bound at all: the shim takes its one match slot before it answers, so
// a bout behind a wedged one sat on `Do` with no client timeout and no request
// deadline while the room drew a progress bar at it.
func TestABoutThatCannotBeSeatedIsToldSoRatherThanWaitingForever(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	// An arena mid-match: the handler never answers, exactly as the real one
	// does not until the match slot is free.
	//
	// **The handler is released by the test rather than by the hang-up**, and
	// that is a fact about `net/http` worth writing down. A handler that has
	// written nothing and not read its request body to the end is never told
	// its client left — measured here as "never", against about 200ms once the
	// body is drained. The real shim drains it (that is what lets a queued
	// match give up); this stand-in does not, so without a door of its own it
	// would hold `httptest.Server.Close` open until the suite timed out. What
	// is under test is the *app's* clock, which owes nothing to the far side
	// noticing anything.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	shim.on("match", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})

	worker := shim.worker(time.Second, "")
	worker.Arena = 150 * time.Millisecond
	// Deliberately far longer than the arena budget: this must be the *acquire*
	// clock that ends the wait, not the silence one.
	worker.Stall = time.Minute

	started := time.Now()
	_, err := worker.RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome"), testDeck("trostani")},
		MatchAsk{Games: 20, Clock: 300, Seed: big.NewInt(42)})
	if err == nil {
		t.Fatal("an arena that never answered came back without an error")
	}
	if !errors.Is(err, ErrArenaBusy) {
		t.Fatalf("a bout that could not be seated failed as %v", err)
	}
	// It answers to the transient class too, so every caller that only sorts
	// "come back" from "never going to work" keeps working unchanged.
	if !errors.Is(err, ErrWorkerNotReady) {
		t.Error("a busy arena did not read as a transient fault")
	}
	if waited := time.Since(started); waited > 10*time.Second {
		t.Errorf("the room waited %v for an arena that was never going to "+
			"answer", waited)
	}
}

// A slow callback is this side's own work, and charging it to the far side is
// how a busy moment here shortens the rope a worker hangs from.
//
// The clock is turned by hand for the reason [handTimer] gives: this is an
// assertion about a budget, and pacing it with real sleeps would make it an
// assertion about the machine.
func TestTheSilenceBudgetIsNotSpentOnThisSidesOwnWork(t *testing.T) {
	t.Parallel()
	const budget, gap = 60 * time.Millisecond, 15 * time.Millisecond
	// Longer than the whole budget: resolving a board's paintings on the first
	// game of a match is a pool lease and a query for every card on it.
	const callback = 5 * budget

	lines := []string{
		`{"game":1}` + "\n",
		`{"result":{"games":[],"seats":{},"wall_seconds":1.0}}` + "\n",
	}
	clock := &handTimer{}
	body := &pacedReader{lines: lines, gap: gap, clock: clock}

	run, err := readStreamOn(body, budget, func() {},
		MatchAsk{OnGame: func(int, *GameResult) {
			// The room goes away and works for longer than the far side is
			// ever allowed to be silent.
			clock.pass(callback)
		}}, clock.arm)
	if err != nil {
		t.Fatalf("this side's own slowness cancelled the worker: %v", err)
	}
	if run == nil {
		t.Fatal("no run came back")
	}
	if clock.fired {
		t.Error("the stall fired while the far side was not silent at all — " +
			"the budget is being spent on work this side did")
	}
}

// ForgeClockForTests is the clock the arena really runs at, written here
// rather than imported: `internal/api` owns the number and importing it back
// would be a cycle. A drift between the two is caught where it matters, in
// `internal/api`'s own test of the clocks it hands over.
const ForgeClockForTests = 300
