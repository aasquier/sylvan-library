package tier3

import (
	"errors"
	"os/exec"
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

// sleeper is a stand-in for the JVM as the deployed worker actually starts it:
// **not one process but a small tree**. The image points `MTGLAB_JAVA` at a
// `/bin/sh` wrapper around `xvfb-run`, which is itself a `/bin/sh` script that
// forks the command it was given, so the thing playing the games is a
// grandchild of the process this package holds a handle to.
//
// The trailing `:` is the whole of that shape, and it is load-bearing: a shell
// handed a single command *execs* it and disappears, leaving one process where
// the deployed worker has three. macOS's `/bin/sh` does exactly that, which is
// how a kill that could never reach a grandchild passed on the laptop for as
// long as it did and only ever failed on Linux. Give the shell something to do
// afterwards and it must stay, and both platforms then ask the same question.
func sleeper() []string { return []string{"/bin/sh", "-c", "sleep 120; :"} }

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
	// The pause is also what makes the tree a tree: it puts the fork safely
	// before the abort, so this is never a test of killing a lone shell.
	time.Sleep(50 * time.Millisecond)
	close(abort)

	// **That `spawn` returns at all is the assertion.** It reads the match's
	// output to EOF, and a pipe reaches EOF only once every process holding
	// its write end is gone — so a grandchild that survived the abort would
	// hold this open for the rest of its two minutes, which is the deployed
	// fault in miniature. Waiting on the error is waiting on the whole tree.
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

// **The fallback in [endGroup], which nothing above can reach.** `spawn` always
// asks for a group, and [exec.Cmd.Start] returns only once the child has got
// one, so the group kill is what fires in every test above this line. This
// drives the other branch by hand: a child deliberately left in *this*
// process's group, which is the shape a failed `Setpgid` would leave behind.
//
// Two claims, and the second is why the order in `endGroup` is written down.
// The child must die — a signal that missed and no fallback would be a kill
// path that silently does nothing. And this process must live: the child's pid
// leads no group, so `-pid` can only come back as "no such process", where a
// helper that reached for our own group id instead would take the test binary,
// the suite, and on a deployed machine the app, down with it.
func TestKillingAMatchThatNeverGotItsOwnGroupStillKillsIt(t *testing.T) {
	t.Parallel()
	// No shell and no children: the tree is not the question here, the missing
	// group is, and a lone process leaves nothing orphaned when it goes.
	cmd := exec.Command("sleep", "120")
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start the stand-in: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	endGroup(cmd.Process)

	reaped := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(30 * time.Second):
		t.Fatal("a match with no group of its own outlived endGroup — the " +
			"group kill missed it and nothing else was aimed at it")
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
	if want := 19 * GameBudget(300); twenty-one != want {
		t.Errorf("nineteen more games bought %v rather than %v", twenty-one, want)
	}
	// **And a subprocess is never cut before the games inside it are.** The
	// per-game ceiling is the bound with news in it — one game recorded as a
	// clock-out, the bout played on — where this one has nothing to hand back,
	// so it must stay the outer of the two for every ask.
	for _, games := range []int{1, 5, 20} {
		if floor := time.Duration(games) * GameBudget(300); SubprocessBudget(
			games, 300) <= floor {
			t.Errorf("over %d games the subprocess is cut at %v, inside the %v "+
				"its own games may take", games,
				SubprocessBudget(games, 300), floor)
		}
	}
	// A caller that named neither is a caller using the defaults, not one
	// asking for a match with no time in it.
	if SubprocessBudget(0, 0) != SubprocessBudget(1, ClockDefault) {
		t.Error("an unnamed games count or clock did not fall back")
	}
}
