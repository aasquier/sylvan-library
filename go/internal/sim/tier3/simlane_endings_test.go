package tier3

import (
	"sync"
	"testing"
)

// **A match that has been reaped is never signalled again**, and the window that
// rule protects is microseconds wide in a real bout.
//
// Three timers and an abort watcher race [exec.Cmd.Wait] by construction: the
// whole-subprocess clock, the per-game ceiling, and whoever stopped watching.
// Each of them ends the match by killing its process *group*, which names a
// group by a number — and once Wait has returned, that number belongs to
// whichever process the kernel hands it to next. So a timer that fires a hair
// late must do nothing at all. The fault it would otherwise cause has no symptom
// anywhere on this side of it: a SIGKILL lands in somebody else's work, on a
// shared machine, and this process goes on to report a perfectly normal bout.
//
// Driven here rather than through a bout because a bout cannot aim at that
// window. `gameclock_test.go` and `runabort_test.go` prove each timer *does* kill
// a match that is still running; this proves the one case none of them can
// arrange.

func TestNothingIsKilledAfterTheMatchHasBeenReaped(t *testing.T) {
	t.Parallel()
	var killed int
	ends := &endings{kill: func() { killed++ }}

	// Before the reap, every way of ending a match ends it.
	ends.stop(&ends.clocked)
	if killed != 1 {
		t.Fatalf("a game that outran its ceiling killed %d times", killed)
	}

	killedBy, quit, cut := ends.reap()
	if killedBy || quit || !cut {
		t.Errorf("the reap reported killed=%v abandoned=%v cut=%v", killedBy, quit, cut)
	}

	// After it, nothing is signalled and no flag is raised — a bout that
	// finished must not be relabelled as one that was abandoned.
	ends.stop(&ends.abandoned)
	ends.stop(&ends.expired)
	if killed != 1 {
		t.Errorf("a reaped match was signalled %d times altogether, so a kill "+
			"went to whatever holds that pid now", killed)
	}
	if ends.abandoned || ends.expired {
		t.Errorf("a reaped match was marked abandoned=%v expired=%v",
			ends.abandoned, ends.expired)
	}
}

// And the three ways race each other as well as the reap, which is why one lock
// covers all of them: whichever arrives first is the one the bout is reported as.
func TestEveryWayOfEndingAMatchRacesTheOthersSafely(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	kills := 0
	ends := &endings{kill: func() {
		mu.Lock()
		defer mu.Unlock()
		kills++
	}}

	var wg sync.WaitGroup
	for _, mark := range []*bool{&ends.expired, &ends.abandoned, &ends.clocked} {
		wg.Add(1)
		go func(m *bool) {
			defer wg.Done()
			ends.stop(m)
		}(mark)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ends.reap()
	}()
	wg.Wait()

	// Whatever order they arrived in, every flag that was raised was raised
	// before the reap — so the bout is reported as whichever ending happened,
	// and never as more endings than there were kills.
	raised := 0
	for _, flag := range []bool{ends.expired, ends.abandoned, ends.clocked} {
		if flag {
			raised++
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if raised != kills {
		t.Errorf("%d endings were recorded against %d kills", raised, kills)
	}
}
