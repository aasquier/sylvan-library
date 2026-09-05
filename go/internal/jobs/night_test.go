package jobs

import "testing"

// The two seams the Coliseum's night runner leans on (ADR 46): a lane that
// can be asked whether anyone is in it, and an owner no request can ever
// arrive as.

func TestLaneBusyReadsTheQueueAsWellAsTheRunner(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	if r.LaneBusy(FORGE) {
		t.Fatal("an idle lane reads busy")
	}
	started := make(chan string, 8)
	release := make(chan struct{})
	defer close(release)
	if _, err := r.Submit("sim.forge", held(started, release, "a"),
		Options{Lane: FORGE}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)
	if !r.LaneBusy(FORGE) {
		t.Fatal("a running match reads idle")
	}
	// A second match queued behind the first is a person already waiting —
	// exactly who the night must not queue behind (ADR 46 decision 4).
	if _, err := r.Submit("sim.forge", held(started, release, "b"),
		Options{Lane: FORGE}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !r.LaneBusy(FORGE) {
		t.Fatal("a queued match reads idle")
	}
	// The lanes do not bleed into each other.
	if r.LaneBusy(NET) || r.LaneBusy(CPU) {
		t.Fatal("the arena's work reads busy on another lane")
	}

	// The queued half alone. The assertion above cannot isolate it — job
	// "a" keeps the lane busy whether or not the queue is counted — and no
	// outside driving can: a queued job with nothing running is promoted the
	// instant the token frees, so the state lasts only scheduling latency.
	// Built by hand instead, the way the registry's own seams are used: a
	// job filed on the lane, still queued, nothing running. This is the
	// person the night must not submit ahead of.
	solo := quietRegistry(t, Config{})
	solo.mu.Lock()
	job := solo.newJobLocked("sim.forge", "waiting", 0, "")
	job.lane = FORGE
	solo.fileLocked(job)
	solo.mu.Unlock()
	if !solo.LaneBusy(FORGE) {
		t.Fatal("a queued match with nothing running reads idle")
	}
}

func TestLaneBusyClearsWhenTheWorkIsDone(t *testing.T) {
	t.Parallel()
	r := quietRegistry(t, Config{})
	started := make(chan string, 8)
	release := make(chan struct{})
	if _, err := r.Submit("sim.forge", held(started, release, "a"),
		Options{Lane: FORGE}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, started, 1)
	close(release)
	r.Wait()
	if r.LaneBusy(FORGE) {
		t.Fatal("a finished lane still reads busy")
	}
	// And a job born finished never occupied one at all.
	r.Completed("sim.mana", map[string]any{}, "cached", 1)
	for _, lane := range Lanes {
		if r.LaneBusy(lane) {
			t.Fatalf("a job born finished reads busy on %s", lane)
		}
	}
}

func TestTheHousesJobsAppearInNobodysListing(t *testing.T) {
	t.Parallel()
	// ADR 5, aimed at the night: a bout's job names two decks in its label,
	// and neither an account nor the no-auth local user may list it. The
	// arithmetic is the guarantee — rowids begin at one, the anonymous scope
	// is zero, so nobody can arrive owning -1.
	r := quietRegistry(t, Config{})
	job := r.Completed("night.forge", map[string]any{}, "Night: a vs b", HouseOwner)
	for _, owner := range []int64{0, 1, 2} {
		if got := r.All(owner); len(got) != 0 {
			t.Errorf("owner %d lists %d of the house's jobs", owner, len(got))
		}
		if r.Get(job.ID, owner) != nil {
			t.Errorf("owner %d can fetch the house's job by id", owner)
		}
	}
	// The census still counts it — counts cannot carry a name, and the admin
	// dashboard should see the load.
	if got := r.Census()["done"]; got != 1 {
		t.Errorf("the census counts %d done, want the house's 1", got)
	}
}
