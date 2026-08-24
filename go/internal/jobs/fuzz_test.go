package jobs

import (
	"fmt"
	"sync"
	"testing"
)

// The registry is the first thing in this port whose correctness is about
// *simultaneity*, so the instruments are different from the last four phases'.
// A corpus proves what one call answers; nothing in one proves that two calls
// arriving together answer once. These do, and they are written to be run
// under `-race`, which is where the interesting failures actually surface.
//
// The assertion throughout is **the count of jobs created**, never that the
// handles agree. Two goroutines handed the same id agree with each other
// perfectly well while a third run is quietly billing somebody.

// hold is a gate every worker parks behind, so no job can finish while
// submissions are still arriving -- which is what makes "only a live job is
// joined" a fact about the test rather than a hope about its timing.
type hold struct {
	release chan struct{}
	runs    sync.Map // job id -> struct{}
	once    sync.Once
}

func newHold() *hold { return &hold{release: make(chan struct{})} }

func (h *hold) work(id string) Runner {
	return func(Progress) (any, error) {
		h.runs.Store(id, struct{}{})
		<-h.release
		return id, nil
	}
}

func (h *hold) let() { h.once.Do(func() { close(h.release) }) }

func (h *hold) count() int {
	n := 0
	h.runs.Range(func(any, any) bool { n++; return true })
	return n
}

func TestManyGoroutinesAskingAtOnceGetExactlyOneJob(t *testing.T) {
	// The stress form of the money bug: sixty-four tabs, one commander. The
	// window `submit` closes is exactly the one two concurrent requests race
	// in, and the lookup and the insert being one locked step is the whole
	// mechanism.
	const askers = 64
	r := quietRegistry(t, Config{})
	gate := newHold()
	defer gate.let()

	ids := make([]string, askers)
	var wg, ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(askers)
	wg.Add(askers)
	for i := range askers {
		go func() {
			defer wg.Done()
			ready.Done()
			<-start // every goroutine arrives at once, or as near as makes no odds
			job, err := r.Submit("claude.dossier", gate.work(fmt.Sprint(i)),
				Options{Owner: 3, Lane: NET, Key: "oracle:goreclaw"})
			if err != nil {
				t.Errorf("submit: %v", err)
				return
			}
			ids[i] = job.ID
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()

	distinct := map[string]int{}
	for _, id := range ids {
		distinct[id]++
	}
	if len(distinct) != 1 {
		t.Fatalf("%d asks produced %d jobs, want one", askers, len(distinct))
	}
	if got := len(r.All(3)); got != 1 {
		t.Errorf("the registry holds %d jobs, want one", got)
	}
	gate.let()
	r.Wait()
	if got := gate.count(); got != 1 {
		t.Errorf("the work ran %d times; one run, one bill", got)
	}
	if got := laneRuns(r, NET); got != 1 {
		t.Errorf("the NET lane started %d jobs, want one", got)
	}
}

func TestConcurrentSubmitsAcrossKeysAndOwnersDoNotBleed(t *testing.T) {
	// The same pressure with the identity varying, which is what would catch
	// a dedupe that matched on too little: a key alone would collapse two
	// accounts' jobs into one and hand the second a 404 for a job it had just
	// been told about (ADR 5).
	const askers = 96
	r := quietRegistry(t, Config{})
	gate := newHold()
	defer gate.let()

	type identity struct {
		kind  string
		key   string
		owner int64
	}
	kinds := []string{"claude.dossier", "claude.argue"}
	keys := []string{"goreclaw", "arahbo", "tivit"}
	owners := []int64{0, 1, 2}

	ids := make([]string, askers)
	want := map[identity]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(askers)
	for i := range askers {
		// **The three vary independently, and that is not decoration.**
		// Written the obvious way -- `keys[i%3]` beside `owners[i%3]` -- key
		// and owner move together, every key belongs to exactly one account,
		// and a dedupe that had forgotten the owner entirely would produce
		// the same counts and pass. Found by mutating that check out and
		// watching this test stay green.
		who := identity{
			kind:  kinds[i%len(kinds)],
			key:   keys[i%len(keys)],
			owner: owners[(i/len(keys))%len(owners)],
		}
		mu.Lock()
		want[who] = true
		mu.Unlock()
		go func() {
			defer wg.Done()
			<-start
			job, err := r.Submit(who.kind, gate.work(fmt.Sprint(i)),
				Options{Owner: who.owner, Lane: CPU, Key: who.key})
			if err != nil {
				t.Errorf("submit: %v", err)
				return
			}
			ids[i] = job.ID
		}()
	}
	close(start)
	wg.Wait()

	distinct := map[string]bool{}
	for _, id := range ids {
		distinct[id] = true
	}
	if len(distinct) != len(want) {
		t.Fatalf("%d distinct identities produced %d jobs", len(want), len(distinct))
	}
	for _, owner := range owners {
		mine := r.All(owner)
		expected := 0
		for who := range want {
			if who.owner == owner {
				expected++
			}
		}
		if len(mine) != expected {
			t.Errorf("owner %d holds %d jobs, want %d", owner, len(mine), expected)
		}
		for _, job := range mine {
			if job.Owner != owner {
				t.Errorf("owner %d was handed job %s, which belongs to %d",
					owner, job.ID, job.Owner)
			}
		}
	}
}

func TestPollingWhileAJobRunsIsNotARace(t *testing.T) {
	// What the race detector is here for. A worker writes `status`,
	// `done` and `result` while request goroutines read
	// them for polls; unguarded, `-race` makes that a
	// hard failure, so the mutable half of a Job is guarded and this is
	// the test that would have found it if it were not.
	r := quietRegistry(t, Config{CPUWorkers: 4})
	const jobs, pollers = 8, 8

	made := make([]*Job, 0, jobs)
	for i := range jobs {
		job, err := r.Submit("sim.mana", func(p Progress) (any, error) {
			for n := range 200 {
				p.Report(n, 200)
				if n%20 == 0 {
					p.ReportPartial(n, 200, []any{n})
				}
			}
			return map[string]any{"games": i}, nil
		}, Options{Owner: int64(i % 3)})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		made = append(made, job)
	}

	var wg sync.WaitGroup
	wg.Add(pollers)
	for range pollers {
		go func() {
			defer wg.Done()
			for range 300 {
				for _, job := range made {
					_ = job.Payload()
					_ = r.Get(job.ID, job.Owner)
				}
				_ = r.All(0)
				_ = r.Census()
			}
		}()
	}
	wg.Wait()
	r.Wait()

	for _, job := range made {
		got := job.Payload()
		if got.Status != Done {
			t.Errorf("job %s is %q", job.ID, got.Status)
		}
		if got.Percent != 100 || got.Done != got.Total {
			t.Errorf("job %s ended at %d/%d (%d%%)", job.ID, got.Done,
				got.Total, got.Percent)
		}
		if got.Partial != nil {
			t.Errorf("job %s kept a partial past its result", job.ID)
		}
	}
}

func TestForgettingAnOwnerWhileWorkIsInFlightIsNotARace(t *testing.T) {
	// Deleting an account happens while that account's jobs are running, by
	// construction: `forget_owner` is called from the delete route and the
	// lanes have no cancellation, so the two genuinely overlap.
	r := quietRegistry(t, Config{CPUWorkers: 4})
	gate := newHold()
	defer gate.let()

	for i := range 24 {
		if _, err := r.Submit("sim.mana", gate.work(fmt.Sprint(i)),
			Options{Owner: int64(1 + i%3)}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	var wg sync.WaitGroup
	wg.Add(4)
	for range 3 {
		go func() { defer wg.Done(); r.ForgetOwner(2) }()
	}
	go func() {
		defer wg.Done()
		for range 200 {
			_ = r.Census()
			_ = r.All(1)
		}
	}()
	wg.Wait()
	gate.let()
	r.Wait()

	if got := r.All(2); len(got) != 0 {
		t.Errorf("%d of the deleted account's jobs survived", len(got))
	}
	if got := len(r.All(1)) + len(r.All(3)); got != 16 {
		t.Errorf("%d jobs left for the other two accounts, want 16", got)
	}
}

// FuzzSubmitDedupe drives submit/poll/dedupe with the shape of the traffic
// varied: how many callers arrive at once, over how many distinct identities,
// spread across how many accounts.
//
// The invariant is arithmetic and does not depend on the numbers: **the count
// of jobs created equals the count of distinct (kind, key, owner) triples
// asked for**, and every caller who asked for one identity holds the one job
// that identity produced. A registry that deduplicated too eagerly fails the
// first half; one that deduplicated on too little fails the second.
func FuzzSubmitDedupe(f *testing.F) {
	f.Add(uint8(8), uint8(1), uint8(1), uint8(1))
	f.Add(uint8(32), uint8(3), uint8(2), uint8(2))
	f.Add(uint8(64), uint8(5), uint8(3), uint8(1))
	f.Add(uint8(17), uint8(7), uint8(1), uint8(3))
	f.Add(uint8(1), uint8(1), uint8(1), uint8(1))

	f.Fuzz(func(t *testing.T, rawAskers, rawKeys, rawOwners, rawKinds uint8) {
		askers := 1 + int(rawAskers)%96
		keys := 1 + int(rawKeys)%8
		owners := 1 + int(rawOwners)%4
		kinds := 1 + int(rawKinds)%3

		r := quietRegistry(t, Config{CPUWorkers: 4})
		gate := newHold()
		defer gate.let()

		type identity struct {
			kind  string
			key   string
			owner int64
		}
		asked := make([]identity, askers)
		handles := make([]string, askers)
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(askers)
		for i := range askers {
			// Independent, for the reason spelled out in the test above:
			// `i%keys` beside `i%owners` correlates the two whenever the
			// counts share a factor, and a dedupe blind to the owner would
			// then pass the whole corpus.
			who := identity{
				kind:  fmt.Sprintf("kind.%d", i%kinds),
				key:   fmt.Sprintf("key.%d", i%keys),
				owner: int64((i / keys) % owners),
			}
			asked[i] = who
			go func() {
				defer wg.Done()
				<-start
				job, err := r.Submit(who.kind, gate.work(fmt.Sprint(i)),
					Options{Owner: who.owner, Key: who.key})
				if err != nil {
					t.Errorf("submit: %v", err)
					return
				}
				handles[i] = job.ID
			}()
		}
		close(start)
		wg.Wait()

		want := map[identity]string{}
		for i, who := range asked {
			if handles[i] == "" {
				t.Fatalf("caller %d was handed no job", i)
			}
			if held, seen := want[who]; seen {
				if held != handles[i] {
					t.Fatalf("identity %+v produced two jobs, %s and %s",
						who, held, handles[i])
				}
				continue
			}
			want[who] = handles[i]
		}
		distinct := map[string]bool{}
		for _, id := range handles {
			distinct[id] = true
		}
		if len(distinct) != len(want) {
			t.Fatalf("%d distinct identities produced %d jobs",
				len(want), len(distinct))
		}
		for owner := range int64(owners) {
			mine := len(r.All(owner))
			expected := 0
			for who := range want {
				if who.owner == owner {
					expected++
				}
			}
			if mine != expected {
				t.Fatalf("owner %d holds %d jobs, want %d", owner, mine, expected)
			}
		}
		gate.let()
		r.Wait()
		if got := gate.count(); got != len(want) {
			t.Fatalf("%d runs for %d identities", got, len(want))
		}
	})
}
