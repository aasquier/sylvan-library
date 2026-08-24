package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// netWorkers and forgeWorkers are fixed, and neither is sized from the
// machine on purpose -- see the package comment. Two, because a Claude call
// costs money per run; one, because two JVMs would race the `.dck` directory.
const (
	netWorkers   = 2
	forgeWorkers = 1
)

// Config is what a registry needs. Every field has a working zero value, so
// `jobs.New(jobs.Config{})` is a real registry.
type Config struct {
	// Logger receives a failed job's error and stack. Nil takes
	// `slog.Default()`.
	Logger *slog.Logger

	// CPUWorkers is how many CPU-lane jobs may run at once. **Zero asks the
	// machine**, which is [runtime.GOMAXPROCS] rather than [runtime.NumCPU]:
	// GOMAXPROCS is what this process may actually *use*, so it honours a
	// `GOMAXPROCS=` in the environment and, since Go 1.25, a cgroup v2 CPU
	// quota -- the two ways a deployment grants fewer cores than it owns.
	//
	// **Neither of them fires on the instance, and this comment used to say
	// one did.** It argued that GOMAXPROCS reads the container's quota, so on
	// the 1-vCPU `shared-cpu-1x` it answers what the machine really grants
	// rather than what the host happens to have, and that NumCPU "would count
	// cores nobody gave us". That is the reasoning for a cgroup-limited
	// container sharing a host, and a Fly machine is not one: it is a
	// Firecracker microVM whose guest kernel boots with exactly the vCPUs the
	// size grants. Measured on the instance 2026-08-22 -- `nproc` answers 1,
	// and `/sys/fs/cgroup/cpu.max` does not exist, because the machine runs
	// cgroup **v1** with every controller mounted at the root, so Go's reader
	// has no file to find and falls back to NumCPU, which is also 1. The two
	// agree here; NumCPU would not over-count, it would answer the same. The
	// code is unchanged and still right -- GOMAXPROCS is what this process may
	// use, wherever it runs -- but it is not rescuing this deployment from a
	// wrong number, and it is not buying width on the instance either; this
	// package's own comment says what is banked and what is realised.
	//
	// It is a knob rather than a constant because the right answer is a fact
	// about a deployment: eight here, **one on the instance, measured** rather
	// than expected, and dialable without a code change if a measurement
	// disagrees. Nothing is reserved for the door's own request handling --
	// Go's scheduler is preemptive, so a saturated CPU lane slows the door
	// rather than silencing it, and a reservation would floor the pool at one
	// on exactly the machine where the width was supposed to help.
	CPUWorkers int

	// Max bounds the registry. Zero takes [MaxJobs].
	Max int
}

// Registry is the job table and the three lanes that feed it,
// made a value so a test can hold
// its own and the race detector has something to be right about.
//
// The table is deliberately **not** a cache -- it is
// "state a client polls, not a memo of an
// answer". A process still holds exactly one of these; it is passed in rather
// than reached for.
type Registry struct {
	mu   sync.Mutex
	jobs map[string]*Job
	seq  uint64

	lanes map[Lane]*lane
	max   int
	log   *slog.Logger

	// now and newID are seams the tests set; a real registry takes the
	// clock and crypto/rand.
	now   func() time.Time
	newID func() string

	// running tracks workers so a test can join them. Production never
	// waits -- see [Registry.Wait].
	running sync.WaitGroup
}

// lane is a semaphore over goroutines. A buffered channel rather than a
// worker pool with a queue, because submission must queue
// without bound and never block the submitter, and a bounded queue would.
// Blocked senders on a full channel are woken in the order they arrived, so
// FIFO service order survives.
type lane struct {
	tokens chan struct{}
	// runs counts workers that actually reached the work. Tests read it: a
	// job born finished must never have reached one, and `status == "done"`
	// cannot tell a short-circuit from a fast worker.
	runs atomic.Int64
}

func newLane(width int) *lane { return &lane{tokens: make(chan struct{}, width)} }

// New builds a registry with its three lanes.
func New(cfg Config) *Registry {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.CPUWorkers <= 0 {
		cfg.CPUWorkers = defaultCPUWorkers()
	}
	if cfg.Max <= 0 {
		cfg.Max = MaxJobs
	}
	return &Registry{
		jobs: map[string]*Job{},
		lanes: map[Lane]*lane{
			CPU:   newLane(cfg.CPUWorkers),
			NET:   newLane(netWorkers),
			FORGE: newLane(forgeWorkers),
		},
		max:   cfg.Max,
		log:   cfg.Logger,
		now:   time.Now,
		newID: randomID,
	}
}

// defaultCPUWorkers is the machine's answer, floored at one so a runtime that
// reported nonsense still runs work.
func defaultCPUWorkers() int {
	if n := runtime.GOMAXPROCS(0); n > 0 {
		return n
	}
	return 1
}

// Width is how many jobs a lane runs at once. Exported because it is the
// number this phase was about, and a number nobody can read is a number
// nobody checks.
func (r *Registry) Width(l Lane) int {
	if ln, ok := r.lanes[l]; ok {
		return cap(ln.tokens)
	}
	return 0
}

// randomID is 48 uniformly random bits as twelve
// lowercase hex digits. The recorded ids are the first twelve hex characters
// of a random (version 4) UUID, which are all random --
// the version nibble sits at index 12 and the variant at 16 -- so six bytes
// from crypto/rand is the same distribution rather than merely a similar one.
func randomID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// crypto/rand.Read does not fail on any platform this runs on, and
		// the alternative to a panic here is a registry that silently hands
		// out one id to two jobs.
		panic("jobs: no randomness for a job id: " + err.Error())
	}
	return hex.EncodeToString(raw[:])
}

// Job is one unit of slow work and everything a poll may see of it.
//
// The immutable half is set once at birth and read without a lock. The
// mutable half -- what a worker writes and a poll reads -- lives behind mu,
// because a worker goroutine and a polling request really do touch it at
// once.
type Job struct {
	ID    string
	Kind  string
	Label string
	// Owner is whose job this is. **Zero means nobody in particular**: the
	// local
	// user, which is everybody when auth is off. It is safe as a sentinel
	// because `users.id` is a SQLite rowid and rowids begin at one, and
	// because `auth.Scope` already spells an unauthenticated caller that way.
	Owner int64
	// Key is what this job *is*, for work where asking twice at once is a
	// mistake rather than a request. Empty opts out and is the default.
	Key string
	// CreatedAt is a string, not an instant -- see [stamp] for why the
	// text is the sort key.
	CreatedAt string

	// seq is birth order, the tie-break an insertion-ordered table would
	// have carried for free: two jobs stamped in the same microsecond
	// must keep the order they were made in, both when the newest are listed
	// and when the oldest are evicted.
	seq uint64

	mu      sync.Mutex
	status  string
	done    int
	total   int
	result  any
	partial any
	err     *string
}

// Status is where the job is now.
func (j *Job) Status() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// Result is the answer, or nil while there is not one.
func (j *Job) Result() any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.result
}

// Payload is `Job.as_dict()`: one consistent moment of this job.
func (j *Job) Payload() Payload {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Payload{
		ID: j.ID, Kind: j.Kind, Status: j.status,
		Done: j.done, Total: j.total, Percent: percent(j.done, j.total),
		Partial: j.partial, Label: j.Label, Result: j.result,
		Error: j.err, CreatedAt: j.CreatedAt,
	}
}

// report is the callback a worker is handed.
type report struct{ job *Job }

func (p report) Report(done, total int) {
	p.job.mu.Lock()
	defer p.job.mu.Unlock()
	p.job.done, p.job.total = done, total
}

func (p report) ReportPartial(done, total int, partial any) {
	p.job.mu.Lock()
	defer p.job.mu.Unlock()
	p.job.done, p.job.total = done, total
	if partial != nil {
		p.job.partial = partial
	}
}

// Options are Submit's levers.
type Options struct {
	Label string
	Owner int64
	// Lane defaults to CPU when empty.
	Lane Lane
	Key  string
}

// Completed is a job that is already finished, because its answer was already
// known.
//
// This is what a simulation cache hit returns. The
// alternative was a second response shape saying "no job, here is the
// result", and it was rejected: every client would need a branch, a hit would
// leave no trace in `/api/jobs`, and the claim being made is not "this is not
// a job" but "this job took no time". A job born done is the honest shape and
// the response contract does not fork.
//
// It still costs a registry slot, which is the point of sharing [Registry.record]
// with [Registry.Submit] -- fifty instant hits evict fifty finished jobs, not
// the running one. And it reaches no lane at all, which is the property a
// test has to assert directly: `status == "done"` cannot tell a
// short-circuit from a worker that was simply quick.
func (r *Registry) Completed(kind string, result any, label string, owner int64) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.newJobLocked(kind, label, owner, "")
	// Fully formed *before* it is filed -- the recorded discipline: build
	// the whole finished job, then file it. The order is
	// visible: eviction runs on the filed table, so a born-finished job is
	// already finished when the bound is checked and can evict itself if it
	// is the only finished one there. No lock is needed for these three --
	// nothing can hold this pointer until it is filed, and filing under r.mu
	// is what publishes them.
	job.status = Done
	job.done, job.total = 1, 1
	job.result = result
	r.fileLocked(job)
	return job
}

// Submit queues fn, handing it a [Progress] to report with.
//
// Lane picks the pool. It is checked before the job is recorded, so a typo is
// an error out of the route rather than a job that sits `queued` forever
// because nothing was ever going to run it.
//
// **Key makes asking twice at once return the same job, and that is a money
// bug rather than a tidiness one.** A dossier takes about four minutes and
// pays for a web search; nothing stopped a second click, a second tab or a
// second device inside that window from starting a *second* paid run for the
// same commander, and on 2026-08-13 two ran concurrently on the deployed
// instance for exactly that reason. The client-side reattach built first only
// ever covered one tab, because it lives in that tab's localStorage; this
// covers every case at once by making the server the thing that knows a run
// is already going.
//
// Matching is per **owner** as well as per key, which is deliberate rather
// than cautious: a job belongs to a person (ADR 5) and [Registry.Get] refuses
// one that is not yours, so handing two accounts the same id would give the
// second a 404 for a job it had just been told about. Every case the bug
// actually covers -- one person, two tabs -- is one owner.
//
// Only a *live* job is reused. A finished one is what a cache is for, and the
// dossier has one (ADR 19); a failed one must be retryable or a transient
// Anthropic error would be permanent until the process restarted.
//
// **The lookup and the insert are one locked step**, because the window this
// closes is exactly the window two concurrent requests race in. The goroutine
// is started after the lock is dropped, so a worker that finishes instantly
// cannot deadlock against the submit that made it.
func (r *Registry) Submit(kind string, fn Runner, opt Options) (*Job, error) {
	if opt.Lane == "" {
		opt.Lane = CPU
	}
	ln, ok := r.lanes[opt.Lane]
	if !ok {
		return nil, &UnknownLaneError{Lane: opt.Lane}
	}

	r.mu.Lock()
	if opt.Key != "" {
		if already := r.joinableLocked(kind, opt.Key, opt.Owner); already != nil {
			r.mu.Unlock()
			return already, nil
		}
	}
	job := r.newJobLocked(kind, opt.Label, opt.Owner, opt.Key)
	r.fileLocked(job)
	r.mu.Unlock()

	r.running.Add(1)
	go func() {
		defer r.running.Done()
		ln.tokens <- struct{}{}
		defer func() { <-ln.tokens }()
		ln.runs.Add(1)
		r.run(job, fn)
	}()
	return job, nil
}

// FromPlan is a finished job when the answer was
// already known, a queued one otherwise.
//
// Every producer of a [Plan] comes through here and no route decides which
// pool the work belongs in. The plan carries its own lane because that is a
// property of the work rather than of the route, and a route is exactly the
// place somebody would eventually forget to pass it. Key rides along for the
// same reason: what counts as "the same work" is the planner's to know.
func (r *Registry) FromPlan(p Plan, owner int64) (*Job, error) {
	if p.Result != nil {
		return r.Completed(p.Kind, p.Result, p.Label, owner), nil
	}
	return r.Submit(p.Kind, p.Run, Options{
		Label: p.Label, Owner: owner, Lane: p.Lane, Key: p.Key})
}

// joinableLocked finds a live job with this identity. r.mu is held.
//
// At most one can exist -- Submit is the only path that sets a key and it
// takes the lock across both the lookup and the insert -- but it answers the
// lowest seq rather than whatever the map iterated to first, so the function
// is total rather than merely correct-today.
func (r *Registry) joinableLocked(kind, key string, owner int64) *Job {
	var found *Job
	for _, job := range r.jobs {
		if job.Key != key || job.Kind != kind || job.Owner != owner {
			continue
		}
		if !live(job.Status()) {
			continue
		}
		if found == nil || job.seq < found.seq {
			found = job
		}
	}
	return found
}

// newJobLocked makes a job without filing it. r.mu is held, because the id
// is checked against the table and the sequence is the table's.
func (r *Registry) newJobLocked(kind, label string, owner int64, key string) *Job {
	r.seq++
	id := r.newID()
	// A collision check 48 bits wide over at most two hundred rows would
	// likely never fire. Two lines buy the guarantee outright, and
	// they cannot diverge observably: the only behaviour they change is one
	// job silently overwriting another.
	for _, taken := r.jobs[id]; taken; _, taken = r.jobs[id] {
		id = r.newID()
	}
	return &Job{
		ID: id, Kind: kind, Label: label, Owner: owner, Key: key,
		CreatedAt: stamp(r.now()), seq: r.seq, status: Queued,
	}
}

// fileLocked is `_record`: register a job, evicting finished ones if the
// registry is over its bound. r.mu is held.
func (r *Registry) fileLocked(job *Job) {
	r.jobs[job.ID] = job
	r.keepLocked()
}

// keepLocked evicts finished jobs if the registry is over its bound. r.mu is
// held.
func (r *Registry) keepLocked() {
	over := len(r.jobs) - r.max
	if over <= 0 {
		return
	}
	finished := make([]*Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		if !live(job.Status()) {
			finished = append(finished, job)
		}
	}
	sort.Slice(finished, func(i, j int) bool {
		if finished[i].CreatedAt != finished[j].CreatedAt {
			return finished[i].CreatedAt < finished[j].CreatedAt
		}
		return finished[i].seq < finished[j].seq
	})
	if over > len(finished) {
		over = len(finished)
	}
	for _, old := range finished[:over] {
		delete(r.jobs, old.ID)
	}
}

// run is the worker wrapper around one job's Runner.
func (r *Registry) run(job *Job, fn Runner) {
	job.mu.Lock()
	job.status = Running
	job.mu.Unlock()

	result, err := r.invoke(job, fn)

	job.mu.Lock()
	defer job.mu.Unlock()
	if err != nil {
		// Surface the message; a local tool is more useful when it says what
		// broke than when it returns a bare 500. The result is left nil and
		// `done` is left where the worker stopped -- a failure is not a
		// finished bar.
		text := err.Error()
		job.err = &text
		job.status = Errored
		return
	}
	job.result = result
	job.status = Done
	if job.total != 0 {
		job.done = job.total
	}
	// The result is the whole answer; a partial left behind is a stale second
	// copy of part of it.
	job.partial = nil
}

// invoke calls the worker and turns a panic into a failed job.
//
// One job's bug must cost one job's error string. An unrecovered panic
// would take the process down instead, and this door is also serving every
// other request -- so the recover is not defensive habit, it is the
// containment the registry promises.
func (r *Registry) invoke(job *Job, fn Runner) (result any, err error) {
	defer func() {
		if p := recover(); p != nil {
			stack := string(debug.Stack())
			r.log.Error("job panicked", "job", job.ID, "kind", job.Kind,
				"panic", p, "stack", stack)
			// Named as a panic so a reader can tell a crash from a refusal
			// the worker made on purpose.
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	result, err = fn(report{job: job})
	if err != nil {
		r.log.Error("job failed", "job", job.ID, "kind", job.Kind, "err", err)
	}
	return result, err
}

// Get answers one job, if it is this owner's. Nil covers both "no such job"
// and "not yours", which is the point -- the caller cannot tell them apart,
// and neither can whoever is probing ids (ADR 5).
func (r *Registry) Get(id string, owner int64) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[id]
	if job == nil || job.Owner != owner {
		return nil
	}
	return job
}

// All is this owner's jobs, newest first.
//
// The order is the recorded one exactly, ties included: the sort is
// stable and descending, so two jobs stamped in the same microsecond keep
// the order they
// were created in rather than having it reversed with everything else. That
// is what seq is for.
func (r *Registry) All(owner int64) []*Job {
	r.mu.Lock()
	mine := make([]*Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		if job.Owner == owner {
			mine = append(mine, job)
		}
	}
	r.mu.Unlock()

	sort.Slice(mine, func(i, j int) bool {
		if mine[i].CreatedAt != mine[j].CreatedAt {
			return mine[i].CreatedAt > mine[j].CreatedAt
		}
		return mine[i].seq < mine[j].seq
	})
	return mine
}

// Census is how many jobs the registry holds, by status. Counts and nothing
// else.
//
// The admin dashboard's view of this registry, and deliberately the only
// cross-owner one: Get and All scope to an owner because a job's label can
// name another person's deck, and administering the instance (ADR 17) is
// about load, not about reading anybody's work. Counts cannot carry a name.
func (r *Registry) Census() map[string]int {
	r.mu.Lock()
	held := make([]*Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		held = append(held, job)
	}
	r.mu.Unlock()

	counts := map[string]int{}
	for _, job := range held {
		counts[job.Status()]++
	}
	return counts
}

// ForgetOwner drops every job belonging to one account and returns how many
// went.
//
// Called when an account is deleted, and the reason is `users.id`: it is
// `INTEGER PRIMARY KEY` without `AUTOINCREMENT`, so SQLite is free to
// re-issue a deleted account's rowid to the next account created. Jobs are
// keyed on that integer and held in memory, so without this the next holder
// of the id would inherit the dead account's jobs -- results, and the deck
// names in their labels. That is the isolation Get and All are written to
// enforce, defeated by arithmetic rather than by a missing filter.
//
// A running job is dropped from the registry but not cancelled; its goroutine
// finishes into a record nothing points at. That is the honest trade -- the
// lanes have no cancellation and inventing one here would be a worse lie than
// a few seconds of orphaned CPU.
//
// **Zero is refused.** The owner here is never "nobody" --
// zero is the no-auth local case, where there is one person and no account
// to
// delete -- so a caller that reached
// here with one would wipe the local user's jobs rather than a deleted
// account's. Nothing can produce it (`user.id` is a rowid), which is why the
// guard costs nothing and why it is here anyway.
func (r *Registry) ForgetOwner(owner int64) int {
	if owner == 0 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	doomed := 0
	for id, job := range r.jobs {
		if job.Owner == owner {
			delete(r.jobs, id)
			doomed++
		}
	}
	return doomed
}

// Clear drops every recorded job. A test helper: a test wanting isolation
// should hold its own registry instead.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = map[string]*Job{}
}

// Wait blocks until every worker this registry started has finished. **For
// tests only** -- production never calls it, because there is nothing to wait
// for and a shutdown that waited would hold the door open for the length of
// a Forge match. It is what makes an assertion about a lane sound rather than
// racy: Submit adds to the group before it returns, so a registry with
// nothing outstanding returns immediately and one with a queued job does not.
func (r *Registry) Wait() { r.running.Wait() }
