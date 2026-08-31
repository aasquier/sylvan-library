package tier3

import (
	"errors"
	"testing"
	"time"
)

// Stopping a match that is already running.
//
// **Nothing could, and that is how a worker became a zombie.** Once `/match`
// had started a JVM, the only things that ever ended it were the subprocess
// finishing or [RunOptions.Timeout] killing it — and on a twenty-game bout
// that ceiling is over an hour. So an app that gave up mid-match left a
// machine playing to nobody for the rest of it, counting itself busy the whole
// time, with every later bout queued behind the one with no audience
// (2026-08-30, deployed; cleared by stopping the machine by hand).
//
// These drive the real trigger: a real subprocess, killed by the real abort
// path, with `/bin/sh` standing in for the JVM because what is being tested is
// the killing rather than the playing.

// sleeper is a stand-in for the JVM: a process that starts, says nothing, and
// would outlive any test left to itself.
func sleeper() []string { return []string{"/bin/sh", "-c", "sleep 120"} }

func TestAnAbortedMatchKillsItsSubprocess(t *testing.T) {
	t.Parallel()
	abort := make(chan struct{})
	// Far beyond anything this test will wait for, so a pass can only mean the
	// abort did it and never that the clock ran out.
	opt := RunOptions{Timeout: time.Hour, Abort: abort}

	done := make(chan error, 1)
	go func() {
		_, err := spawn(sleeper(), t.TempDir(), opt, newProseTelling(false))
		done <- err
	}()

	// The process is up and reading nothing; now nobody is left to want it.
	time.Sleep(50 * time.Millisecond)
	close(abort)

	select {
	case err := <-done:
		if !errors.Is(err, ErrAbandoned) {
			t.Fatalf("an abandoned match came back as %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the subprocess outlived the abort — this is the match that " +
			"held a deployed machine for an hour")
	}
}

// **An abandoned match is not a match that ran out of time.** Both end with a
// killed process and the difference is only visible in the flag, so it is
// worth a test of its own: one is news for whoever asked, and the other is
// news for nobody because the asker has gone.
func TestAnAbandonedMatchIsNotReportedAsATimeout(t *testing.T) {
	t.Parallel()
	abort := make(chan struct{})
	close(abort)

	_, err := spawn(sleeper(), t.TempDir(),
		RunOptions{Timeout: time.Hour, Abort: abort}, newProseTelling(false))
	if errors.Is(err, ErrTimedOut) {
		t.Error("a match nobody was waiting for was blamed on the clock")
	}
	if !errors.Is(err, ErrAbandoned) {
		t.Fatalf("an abandoned match came back as %v", err)
	}
}

// A match nobody aborts is untouched by any of this, and the watcher it never
// needed does not outlive it. `-race` and the leak in a `go test` run are what
// actually police the second half; the assertion here is the first.
func TestAMatchNobodyAbandonsRunsToItsOwnEnd(t *testing.T) {
	t.Parallel()
	// `true` exits at once: a subprocess that finishes on its own, which is
	// every match that works.
	run, err := spawn([]string{"/bin/sh", "-c", "exit 0"}, t.TempDir(),
		RunOptions{Timeout: time.Hour}, newProseTelling(false))
	if err != nil {
		t.Fatalf("a match with no abort channel failed: %v", err)
	}
	if run == nil {
		t.Fatal("no run came back")
	}
}

// The whole-subprocess ceiling is the far side's own, and the app's
// [MatchBudget] is written against it. A change to either without the other is
// the fault this package spent an evening on, so they are held together here.
func TestTheSubprocessCeilingGrowsWithTheGamesAsked(t *testing.T) {
	t.Parallel()
	one, twenty := SubprocessBudget(1, 300), SubprocessBudget(20, 300)
	if twenty <= one {
		t.Errorf("twenty games are given %v and one game %v", twenty, one)
	}
	if want := 19 * 300 * time.Second; twenty-one != want {
		t.Errorf("nineteen more games bought %v rather than %v", twenty-one, want)
	}
	// A caller that named neither is a caller using the defaults, not one
	// asking for a match with no time in it.
	if SubprocessBudget(0, 0) != SubprocessBudget(1, ClockDefault) {
		t.Error("an unnamed games count or clock did not fall back")
	}
}
