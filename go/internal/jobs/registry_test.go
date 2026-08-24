package jobs

import (
	"errors"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The behaviour half of the registry, which no corpus can hold: what a lock
// held across two steps buys, what a lane's width means, and what a job born
// finished must never have touched.
//
// **Every test here holds its own registry.** A value per test is what lets
// `go test -race`
// run these in parallel and mean something by it.

// quietRegistry is a registry whose logger goes nowhere -- a failing job logs
// its error and a panicking one logs a stack, and both are expected in this
// file.
func quietRegistry(t *testing.T, cfg Config) *Registry {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard,
			&slog.HandlerOptions{Level: slog.LevelError + 1}))
	}
	r := New(cfg)
	t.Cleanup(func() {
		// A test that leaves a worker blocked hangs here rather than leaking
		// it into the next one, and the panic message names the file.
		done := make(chan struct{})
		go func() { r.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("a worker is still running at the end of the test")
		}
	})
	return r
}

// tick makes the registry's clock a counter, one second per job, so orderings
// are stated rather than raced for.
func tick(r *Registry, step time.Duration) {
	base := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	var n int64
	r.now = func() time.Time {
		at := base.Add(time.Duration(n) * step)
		n++
		return at
	}
}

// frozen makes every job share one stamp, which is what puts the tie-break
// under test.
func frozen(r *Registry) {
	at := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return at }
}

func laneRuns(r *Registry, l Lane) int64 { return r.lanes[l].runs.Load() }

// held is a worker that reports it has started and then waits to be let go.
func held(started chan<- string, release <-chan struct{}, name string) Runner {
	return func(Progress) (any, error) {
		started <- name
		<-release
		return nil, nil
	}
}

func waitFor(t *testing.T, ch <-chan string, want int) {
	t.Helper()
	got := make([]string, 0, want)
	for range want {
		select {
		case name := <-ch:
			got = append(got, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d workers started", len(got), want)
		}
	}
}

func noneMore(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case name := <-ch:
		t.Fatalf("worker %q started, and the lane should not have let it", name)
	case <-time.After(100 * time.Millisecond):
	}
}

// ------------------------------------------------------------ the lanes

func TestTheCPULaneIsAsWideAsTheMachineAndTheOthersAreNot(t *testing.T) {
	t.Parallel()
	// The width, said as a number: the CPU lane is a semaphore over
	// goroutines and its width is a fact about the machine -- GOMAXPROCS,
	// which reads the container's quota rather than the host's core count.
	//
	// The other two are deliberately *not* sized from the machine: NET is
	// two because a Claude call costs money per run, FORGE is one because two
	// JVMs would race the shared `.dck` directory. A port that scaled all
	// three together would look consistent and be wrong twice.
	r := quietRegistry(t, Config{})
	if got, want := r.Width(CPU), runtime.GOMAXPROCS(0); got != want {
		t.Errorf("the CPU lane is %d wide, the machine allows %d", got, want)
	}
	if got := r.Width(NET); got != netWorkers {
		t.Errorf("the NET lane is %d wide, want %d", got, netWorkers)
	}
	if got := r.Width(FORGE); got != forgeWorkers {
		t.Errorf("the FORGE lane is %d wide, want %d", got, forgeWorkers)
	}
	if got := r.Width("nope"); got != 0 {
		t.Errorf("a lane that does not exist is %d wide", got)
	}
}

func TestAConfiguredCPUWidthWins(t *testing.T) {
	t.Parallel()
	// The width is a knob because the right answer is a fact about a
	// deployment rather than about this code.
	r := quietRegistry(t, Config{CPUWorkers: 3})
	if got := r.Width(CPU); got != 3 {
		t.Errorf("the CPU lane is %d wide, want the configured 3", got)
	}
}

func TestTheForgeLaneRunsOneMatchAtATime(t *testing.T) {
	t.Parallel()
	// ADR 35's rule, and it is by construction rather than by care: two
	// matches at once race the shared `.dck` directory `ensure_profile` hands
	// out, and saturate the machine besides.
	r := quietRegistry(t, Config{})
	started := make(chan string, 8)
	release := make(chan struct{})
	defer close(release)

	for _, name := range []string{"a", "b", "c"} {
		if _, err := r.Submit("sim.forge", held(started, release, name),
			Options{Lane: FORGE}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	waitFor(t, started, 1)
	noneMore(t, started)
}

func TestTheNetLaneRunsTwoAtOnceAndNeverThree(t *testing.T) {
	t.Parallel()
	// Two, and the bound is about money rather than throughput: a proposal
	// costs real money per run, and a queue is a cheaper way to say "not four
	// at once" than a rate limiter nobody has written.
	r := quietRegistry(t, Config{})
	started := make(chan string, 8)
	release := make(chan struct{})
	defer close(release)

	for _, name := range []string{"a", "b", "c", "d"} {
		if _, err := r.Submit("claude.theme.proposal",
			held(started, release, name), Options{Lane: NET}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	waitFor(t, started, 2)
	noneMore(t, started)
}

func TestTheCPULaneRunsSeveralAtOnce(t *testing.T) {
	t.Parallel()
	// Stated positively, because every other lane test here asserts a
	// *ceiling* and a semaphore that never let anything through would pass
	// all of them.
	r := quietRegistry(t, Config{CPUWorkers: 4})
	started := make(chan string, 8)
	release := make(chan struct{})
	defer close(release)

	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if _, err := r.Submit("sim.mana", held(started, release, name),
			Options{}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	waitFor(t, started, 4)
	noneMore(t, started)
}

func TestTheDefaultLaneIsCPU(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	if _, err := r.Submit("sim.mana", func(Progress) (any, error) {
		return nil, nil
	}, Options{}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	if got := laneRuns(r, CPU); got != 1 {
		t.Errorf("the CPU lane ran %d jobs, want 1", got)
	}
	if got := laneRuns(r, NET) + laneRuns(r, FORGE); got != 0 {
		t.Errorf("%d jobs went to a lane nobody asked for", got)
	}
}

// -------------------------------------------------- asking twice at once

func TestASecondAskJoinsTheRunAlreadyGoing(t *testing.T) {
	t.Parallel()
	// The money bug. A dossier takes about four minutes and pays for a web
	// search; on 2026-08-13 two ran concurrently on the deployed instance
	// because a second click inside that window had nothing to collide with.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	var calls atomic.Int64

	run := func(Progress) (any, error) {
		calls.Add(1)
		started <- "dossier"
		<-release
		return "written", nil
	}
	opt := Options{Owner: 7, Lane: NET, Key: "oracle:goreclaw"}
	first, err := r.Submit("claude.dossier", run, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	second, err := r.Submit("claude.dossier", run, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if second != first {
		t.Fatalf("a second ask started job %s instead of joining %s",
			second.ID, first.ID)
	}
	close(release)
	r.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("the work ran %d times; one run, one bill", got)
	}
	if got := r.All(7); len(got) != 1 {
		t.Errorf("%d jobs in the list, want one", len(got))
	}
}

func TestAQueuedJobIsJoinedToo(t *testing.T) {
	t.Parallel()
	// `LIVE` is queued *and* running, which matters because the window the
	// bug lived in is mostly spent waiting for a lane rather than in one.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	opt := Options{Lane: FORGE, Key: "cat-vs-dino"}
	blocker, err := r.Submit("sim.forge", held(started, release, "blocker"),
		Options{Lane: FORGE})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	queued, err := r.Submit("sim.forge", held(started, release, "queued"), opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if queued.Status() != Queued {
		t.Fatalf("the second job is %q, want queued behind the first",
			queued.Status())
	}
	joined, err := r.Submit("sim.forge", held(started, release, "joined"), opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if joined != queued {
		t.Errorf("a queued job was not joined")
	}
	if blocker == queued {
		t.Errorf("a job with no key was joined")
	}
}

func TestAFinishedRunIsNotJoined(t *testing.T) {
	t.Parallel()
	// Only a *live* job is reused. What covers "somebody asked before" is the
	// cache (ADR 19), one step earlier in time and with a stored answer to
	// hand back; joining a finished job would hand over a result whose
	// freshness nothing had checked.
	r := quietRegistry(t, Config{})
	opt := Options{Lane: NET, Key: "oracle:goreclaw"}
	done := func(Progress) (any, error) { return "written", nil }

	first, err := r.Submit("claude.dossier", done, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	if first.Status() != Done {
		t.Fatalf("the first job is %q, want done", first.Status())
	}
	second, err := r.Submit("claude.dossier", done, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if second == first {
		t.Errorf("a finished job was joined")
	}
}

func TestAFailedRunStaysRetryable(t *testing.T) {
	t.Parallel()
	// Long documented and never asserted: a failed job must not be
	// joined, or a transient Anthropic error would be permanent until the
	// process restarted.
	r := quietRegistry(t, Config{})
	opt := Options{Lane: NET, Key: "oracle:goreclaw"}
	broken := func(Progress) (any, error) { return nil, errors.New("the search fell over") }

	first, err := r.Submit("claude.dossier", broken, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	if first.Status() != Errored {
		t.Fatalf("the first job is %q, want error", first.Status())
	}
	second, err := r.Submit("claude.dossier", broken, opt)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if second == first {
		t.Errorf("a failed job was joined, and so could never be retried")
	}
}

func TestTwoAccountsAskingAtOnceDoNotShareAJob(t *testing.T) {
	t.Parallel()
	// Matching is per owner as well as per key, and that is not caution. A
	// job belongs to a person (ADR 5) and Get reports somebody else's as
	// absent -- so handing two accounts one id would give the second a 404
	// for a job it had just been told about.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	mine, err := r.Submit("claude.dossier", held(started, release, "mine"),
		Options{Owner: 1, Lane: NET, Key: "k"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	theirs, err := r.Submit("claude.dossier", held(started, release, "theirs"),
		Options{Owner: 2, Lane: NET, Key: "k"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if mine.ID == theirs.ID {
		t.Fatalf("two accounts were handed one job")
	}
	if r.Get(theirs.ID, 1) != nil {
		t.Errorf("one account can see the other's job")
	}
}

func TestWorkWithNoKeyIsNeverDeduplicated(t *testing.T) {
	t.Parallel()
	// The default, and right for anything not reproducible. A theme proposal
	// is a conversation nobody else is having; two at once are two proposals.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	one, err := r.Submit("claude.theme.proposal",
		held(started, release, "one"), Options{Lane: NET})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	two, err := r.Submit("claude.theme.proposal",
		held(started, release, "two"), Options{Lane: NET})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if one.ID == two.ID {
		t.Errorf("two conversations were collapsed into one job")
	}
}

func TestTheSameKeyUnderADifferentKindIsADifferentJob(t *testing.T) {
	t.Parallel()
	// Kind is part of the identity, and it has to be: two features can
	// perfectly well key on the same commander.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	first, err := r.Submit("claude.dossier", held(started, release, "d"),
		Options{Lane: NET, Key: "goreclaw"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	second, err := r.Submit("claude.argue", held(started, release, "a"),
		Options{Lane: NET, Key: "goreclaw"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if first.ID == second.ID {
		t.Errorf("two kinds sharing a key were collapsed into one job")
	}
}

// ---------------------------------------------------- born finished

func TestAJobBornFinishedNeverReachesALane(t *testing.T) {
	t.Parallel()
	// **`status == "done"` cannot tell a short-circuit from a worker that was
	// simply quick**, which is why this asserts the lane was never entered
	// rather than asserting the status. `Wait` is what makes it sound: Submit
	// adds to the wait group before it returns, so a registry with a queued
	// job does not return from it.
	r := quietRegistry(t, Config{})
	plan := Plan{Kind: "sim.mana", Label: "Tivit — mana",
		Result: map[string]any{"cached": true},
		Run: func(Progress) (any, error) {
			t.Error("the plan's work ran, and its answer was already known")
			return nil, nil
		}}
	job, err := r.FromPlan(plan, 3)
	if err != nil {
		t.Fatalf("from plan: %v", err)
	}
	r.Wait()

	for _, l := range Lanes {
		if got := laneRuns(r, l); got != 0 {
			t.Errorf("the %s lane ran %d jobs for an answer already known", l, got)
		}
	}
	if job.Status() != Done {
		t.Errorf("a job born finished is %q", job.Status())
	}
	payload := job.Payload()
	if payload.Done != 1 || payload.Total != 1 || payload.Percent != 100 {
		t.Errorf("a job born finished reads %d/%d (%d%%), want 1/1 at 100",
			payload.Done, payload.Total, payload.Percent)
	}
	if job.Owner != 3 {
		t.Errorf("a job born finished belongs to %d, want 3", job.Owner)
	}
}

func TestAPlanWithNoAnswerDoesReachItsLane(t *testing.T) {
	t.Parallel()
	// The contrapositive, because the test above would pass just as happily
	// against a registry that never ran anything at all.
	r := quietRegistry(t, Config{})
	job, err := r.FromPlan(Plan{Kind: "claude.dossier", Lane: NET,
		Run: func(Progress) (any, error) { return "written", nil }}, 3)
	if err != nil {
		t.Fatalf("from plan: %v", err)
	}
	r.Wait()
	if got := laneRuns(r, NET); got != 1 {
		t.Errorf("the NET lane ran %d jobs, want 1", got)
	}
	if got := job.Result(); got != "written" {
		t.Errorf("the result is %v, want the work's answer", got)
	}
}

func TestAPlanCarriesItsOwnLane(t *testing.T) {
	t.Parallel()
	// Which pool the work belongs in is a property of the work, so it is
	// decided where the work is planned rather than at the route, where
	// somebody would eventually forget to pass it.
	r := quietRegistry(t, Config{})
	if _, err := r.FromPlan(Plan{Kind: "sim.forge", Lane: FORGE,
		Run: func(Progress) (any, error) { return nil, nil }}, 0); err != nil {
		t.Fatalf("from plan: %v", err)
	}
	r.Wait()
	if got := laneRuns(r, FORGE); got != 1 {
		t.Errorf("the FORGE lane ran %d jobs, want 1", got)
	}
}

// ------------------------------------------------------ running a job

func TestProgressIsReportedAndThePartialIsClearedByTheResult(t *testing.T) {
	t.Parallel()
	// The theater's rows are renderable before the result exists, and they
	// are cleared the moment it does -- the result is the whole answer, and a
	// leftover partial is a second copy of part of it that would go stale.
	r := quietRegistry(t, Config{})
	seen := make(chan Payload, 4)
	release := make(chan struct{})

	job, err := r.Submit("sim.forge", func(p Progress) (any, error) {
		p.Report(1, 8)
		p.ReportPartial(2, 8, []any{"game one"})
		<-release
		return map[string]any{"games": 8}, nil
	}, Options{Lane: FORGE})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	// Poll until the partial lands, which is what a client does.
	go func() {
		for range 200 {
			if p := job.Payload(); p.Partial != nil {
				seen <- p
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		close(seen)
	}()
	select {
	case mid, ok := <-seen:
		if !ok {
			t.Fatal("the partial never appeared")
		}
		if mid.Done != 2 || mid.Total != 8 || mid.Percent != 25 {
			t.Errorf("mid-run reads %d/%d (%d%%), want 2/8 at 25",
				mid.Done, mid.Total, mid.Percent)
		}
		if mid.Status != Running {
			t.Errorf("mid-run status is %q, want running", mid.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the partial never appeared")
	}
	close(release)
	r.Wait()

	end := job.Payload()
	if end.Partial != nil {
		t.Errorf("the partial survived the result: %v", end.Partial)
	}
	if end.Done != 8 || end.Percent != 100 {
		t.Errorf("a finished job reads %d/%d (%d%%), want the bar filled",
			end.Done, end.Total, end.Percent)
	}
	if end.Error != nil {
		t.Errorf("a finished job carries an error: %v", *end.Error)
	}
}

func TestReportWithNoPartialLeavesTheOneThatIsThere(t *testing.T) {
	t.Parallel()
	// An omitted partial means
	// "leave what is there" rather than "clear it" -- which is why Progress
	// has two methods instead of one call with a nil.
	r := quietRegistry(t, Config{})
	job, err := r.Submit("sim.forge", func(p Progress) (any, error) {
		p.ReportPartial(1, 4, []any{"game one"})
		p.Report(2, 4)
		return nil, errors.New("stop here so the result does not clear it")
	}, Options{Lane: FORGE})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	got := job.Payload()
	if got.Partial == nil {
		t.Errorf("a two-argument report cleared the partial")
	}
	if got.Done != 2 || got.Total != 4 {
		t.Errorf("the counts read %d/%d, want 2/4", got.Done, got.Total)
	}
}

func TestAFailureRecordsTheMessageAndLeavesTheResultEmpty(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	job, err := r.Submit("sim.mana", func(p Progress) (any, error) {
		p.Report(3, 10)
		return nil, errors.New("no deck 'no-such-deck'")
	}, Options{})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()

	got := job.Payload()
	if got.Status != Errored {
		t.Errorf("a failed job is %q", got.Status)
	}
	if got.Error == nil || *got.Error != "no deck 'no-such-deck'" {
		t.Errorf("the error reads %v", got.Error)
	}
	if got.Result != nil {
		t.Errorf("a failed job carries a result: %v", got.Result)
	}
	// The bar is left where the worker stopped. A failure is not a finished
	// bar, and the registry only fills it on the success path.
	if got.Done != 3 || got.Total != 10 {
		t.Errorf("a failed job reads %d/%d, want the counts it stopped at",
			got.Done, got.Total)
	}
}

func TestAPanickingWorkerIsAFailedJobAndNotADeadProcess(t *testing.T) {
	t.Parallel()
	// One job's bug must cost one job's error string.
	// Go's default is the opposite -- an unrecovered panic in any goroutine
	// takes the process with it -- and this door is also serving every other
	// request, so the recover is the containment the registry promises
	// rather than defensive habit.
	r := quietRegistry(t, Config{})
	job, err := r.Submit("sim.mana", func(p Progress) (any, error) {
		var rows []int
		p.Report(len(rows), 4)
		// An index nobody checked, which is how it actually happens -- taken
		// from a value so nothing can fold it away before it runs.
		return rows[len(rows)+4], nil
	}, Options{})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()

	got := job.Payload()
	if got.Status != Errored {
		t.Fatalf("a panicking job is %q, want error", got.Status)
	}
	if got.Error == nil {
		t.Fatal("a panicking job carries no error string")
	}
	if !strings.HasPrefix(*got.Error, "panic: ") {
		t.Errorf("a panic reads as %q, and a reader cannot tell it from a "+
			"refusal the worker made on purpose", *got.Error)
	}
	if !strings.Contains(*got.Error, "index out of range") {
		t.Errorf("the panic's own words were lost: %q", *got.Error)
	}
	// And the next job on the same lane still runs, which is the property
	// that matters: a panic must not poison the semaphore.
	next, err := r.Submit("sim.mana", func(Progress) (any, error) {
		return "fine", nil
	}, Options{})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	if next.Status() != Done {
		t.Errorf("the lane did not survive a panic: the next job is %q",
			next.Status())
	}
}

// ------------------------------------------------------ who may see one

func TestGetReportsSomebodyElsesJobAsAbsent(t *testing.T) {
	t.Parallel()
	// Nil covers both "no such job" and "not yours", which is the point --
	// the caller cannot tell them apart, and neither can whoever is probing
	// ids. The route turns nil into a 404, never a 403 (ADR 5).
	r := quietRegistry(t, Config{})
	job := r.Completed("sim.mana", map[string]any{"cached": true}, "theirs", 9)

	if r.Get(job.ID, 9) == nil {
		t.Errorf("an owner cannot see their own job")
	}
	if r.Get(job.ID, 8) != nil {
		t.Errorf("one account can see another's job")
	}
	if r.Get(job.ID, 0) != nil {
		t.Errorf("the local user can see an account's job")
	}
	if r.Get("deadbeefcafe", 9) != nil {
		t.Errorf("a job that does not exist was found")
	}
}

func TestTheLocalUserIsOwnerZero(t *testing.T) {
	t.Parallel()
	// When auth is off -- every local run -- there is
	// one person, every job theirs. Owner zero is how that is spelled, and
	// `auth.Scope` already spells an unauthenticated caller the same way.
	r := quietRegistry(t, Config{})
	job := r.Completed("sim.mana", nil, "mine", 0)
	if r.Get(job.ID, 0) == nil {
		t.Errorf("the local user cannot see their own job")
	}
	if got := r.All(0); len(got) != 1 {
		t.Errorf("the local user has %d jobs, want one", len(got))
	}
	if got := r.All(1); len(got) != 0 {
		t.Errorf("an account inherited the local user's %d jobs", len(got))
	}
}

func TestJobsAreListedNewestFirstAndTiesKeepTheirBirthOrder(t *testing.T) {
	t.Parallel()
	// The recorded sort is stable and descending, so two jobs stamped in
	// the same microsecond keep the order they were made in rather than
	// having it reversed with everything else. A Go map has no order at all,
	// so the tie-break has to be carried explicitly or the list is a
	// different list every run.
	r := quietRegistry(t, Config{})
	frozen(r)
	first := r.Completed("sim.mana", nil, "first", 0)
	second := r.Completed("sim.mana", nil, "second", 0)
	third := r.Completed("sim.mana", nil, "third", 0)

	for range 20 {
		got := r.All(0)
		if len(got) != 3 {
			t.Fatalf("%d jobs listed, want 3", len(got))
		}
		if got[0] != first || got[1] != second || got[2] != third {
			t.Fatalf("ties were reordered: %s, %s, %s",
				got[0].Label, got[1].Label, got[2].Label)
		}
	}
}

func TestJobsAreListedNewestFirst(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	tick(r, time.Second)
	old := r.Completed("sim.mana", nil, "old", 0)
	mid := r.Completed("sim.mana", nil, "mid", 0)
	recent := r.Completed("sim.mana", nil, "recent", 0)

	got := r.All(0)
	if len(got) != 3 || got[0] != recent || got[1] != mid || got[2] != old {
		t.Fatalf("the list is not newest first: %v", got)
	}
}

// ------------------------------------------------------------- the bound

func TestTheOldestFinishedJobIsEvictedAndALiveOneNever(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{Max: 3})
	tick(r, time.Second)
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	running, err := r.Submit("sim.forge", held(started, release, "running"),
		Options{Lane: FORGE})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	oldest := r.Completed("sim.mana", nil, "oldest", 0)
	middle := r.Completed("sim.mana", nil, "middle", 0)
	newest := r.Completed("sim.mana", nil, "newest", 0)

	if r.Get(oldest.ID, 0) != nil {
		t.Errorf("the oldest finished job survived the bound")
	}
	for _, job := range []*Job{running, middle, newest} {
		if r.Get(job.ID, 0) == nil {
			t.Errorf("%s was evicted and should not have been", job.Label)
		}
	}
}

func TestNothingIsEvictedWhenEverythingIsStillLive(t *testing.T) {
	t.Parallel()
	// Eviction takes the oldest of the *finished* jobs, of which there may
	// be none, so a registry full of running jobs simply goes over its bound.
	// That is the right trade -- dropping a job somebody is polling to make
	// room for one they just asked for is worse -- but it means the bound is
	// a target rather than a ceiling, and that is worth saying out loud.
	r := quietRegistry(t, Config{Max: 1})
	started := make(chan string, 8)
	release := make(chan struct{})
	defer close(release)

	var made []*Job
	for _, name := range []string{"a", "b", "c"} {
		job, err := r.Submit("sim.mana", held(started, release, name),
			Options{})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		made = append(made, job)
	}
	for _, job := range made {
		if r.Get(job.ID, 0) == nil {
			t.Errorf("a live job was evicted to honour the bound")
		}
	}
}

func TestABornFinishedJobCanEvictItself(t *testing.T) {
	t.Parallel()
	// Reproduced rather than chosen. The whole finished job is built and
	// then filed, so it is already finished when the bound is
	// checked -- and if it is the only finished job there, it is the one that
	// goes. Unreachable at MaxJobs=200, and pinned here because an
	// implementation that
	// filed the job *before* finishing it would quietly behave differently
	// the day somebody dialled the bound down.
	r := quietRegistry(t, Config{Max: 1})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	if _, err := r.Submit("sim.forge", held(started, release, "running"),
		Options{Lane: FORGE}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	hit := r.Completed("sim.mana", map[string]any{"cached": true}, "hit", 0)
	if r.Get(hit.ID, 0) != nil {
		t.Errorf("the born-finished job stayed, and the recorded discipline " +
			"evicts it")
	}
	if hit.Status() != Done {
		t.Errorf("the caller's own handle changed: %q", hit.Status())
	}
}

// --------------------------------------------------- deleting an account

func TestForgetOwnerDropsThatAccountsJobsAndCountsThem(t *testing.T) {
	t.Parallel()
	// The failure this prevents is not a missing filter. `users.id` is
	// `INTEGER PRIMARY KEY` without `AUTOINCREMENT`, so SQLite re-issues a
	// deleted account's rowid, and jobs left keyed on that integer would be
	// handed to whoever is created next -- results, and the deck names in
	// their labels.
	r := quietRegistry(t, Config{})
	r.Completed("sim.mana", nil, "friend's deck", 4)
	r.Completed("sim.lands", nil, "friend's other deck", 4)
	mine := r.Completed("sim.mana", nil, "my deck", 5)
	local := r.Completed("sim.mana", nil, "the local user's", 0)

	if got := r.ForgetOwner(4); got != 2 {
		t.Errorf("forgot %d jobs, want 2", got)
	}
	if got := r.All(4); len(got) != 0 {
		t.Errorf("%d of the deleted account's jobs survived", len(got))
	}
	if r.Get(mine.ID, 5) == nil || r.Get(local.ID, 0) == nil {
		t.Errorf("somebody else's jobs went with the deleted account")
	}
	if got := r.ForgetOwner(4); got != 0 {
		t.Errorf("forgetting twice found %d jobs", got)
	}
}

func TestForgetOwnerRefusesTheLocalUser(t *testing.T) {
	t.Parallel()
	// The owner here is never the local user -- zero is the
	// no-auth local case, where there is one person and no account to delete.
	// Nothing can produce it, so
	// the guard costs nothing; without it a caller that reached here with a
	// zero would wipe the local user's jobs instead of a deleted account's.
	r := quietRegistry(t, Config{})
	local := r.Completed("sim.mana", nil, "the local user's", 0)
	if got := r.ForgetOwner(0); got != 0 {
		t.Errorf("forgetting owner zero claimed %d jobs", got)
	}
	if r.Get(local.ID, 0) == nil {
		t.Errorf("the local user's jobs were dropped as an account's")
	}
}

func TestARunningJobIsDroppedButNotCancelled(t *testing.T) {
	t.Parallel()
	// The honest trade, long documented at ForgetOwner: the lanes have
	// no cancellation, so a dropped job's goroutine finishes into a record
	// nothing points at. Inventing a cancellation here would be a worse lie
	// than a few seconds of orphaned CPU.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	var finished atomic.Bool

	job, err := r.Submit("sim.forge", func(Progress) (any, error) {
		started <- "match"
		<-release
		finished.Store(true)
		return nil, nil
	}, Options{Owner: 6, Lane: FORGE})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	if got := r.ForgetOwner(6); got != 1 {
		t.Errorf("forgot %d jobs, want 1", got)
	}
	if r.Get(job.ID, 6) != nil {
		t.Errorf("a dropped job is still in the registry")
	}
	close(release)
	r.Wait()
	if !finished.Load() {
		t.Errorf("the work stopped, and nothing in this registry can stop it")
	}
}

// -------------------------------------------------------------- counting

func TestCensusCountsByStatusAcrossOwnersAndCarriesNoName(t *testing.T) {
	t.Parallel()
	// The admin dashboard's view, and deliberately the only cross-owner one:
	// a job's label can name another person's deck, and administering the
	// instance (ADR 17) is about load, not about reading anybody's work.
	// Counts cannot carry a name.
	r := quietRegistry(t, Config{})
	started := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)

	r.Completed("sim.mana", nil, "one account's deck", 1)
	r.Completed("sim.mana", nil, "another account's deck", 2)
	if _, err := r.Submit("sim.mana", func(Progress) (any, error) {
		return nil, errors.New("broken")
	}, Options{Owner: 3}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	r.Wait()
	if _, err := r.Submit("sim.forge", held(started, release, "running"),
		Options{Owner: 4, Lane: FORGE}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)

	got := r.Census()
	if got[Done] != 2 || got[Errored] != 1 || got[Running] != 1 {
		t.Errorf("the census reads %v, want two done, one error, one running", got)
	}
	if len(got) != 3 {
		t.Errorf("the census has %d keys: %v", len(got), got)
	}
}

func TestClearDropsEveryJob(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	r.Completed("sim.mana", nil, "one", 0)
	r.Completed("sim.mana", nil, "two", 1)
	r.Clear()
	if got := len(r.All(0)) + len(r.All(1)); got != 0 {
		t.Errorf("%d jobs survived a clear", got)
	}
	if got := r.Census(); len(got) != 0 {
		t.Errorf("the census still reads %v", got)
	}
}
