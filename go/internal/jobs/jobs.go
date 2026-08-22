// Package jobs is `src/mtglab/api/jobs.py`: the in-process registry for work
// too slow to answer inside a request.
//
// Python's argument for a dict and a thread pool -- rather than a broker, a
// worker process and a whole class of "is redis running?" failures -- carries
// over unchanged, and so does the consequence: **jobs are ephemeral by
// design.** Restart the process and they are gone, which is correct for
// inputs that are cheap to resubmit.
//
// # What the port changes, and it is the phase's whole point
//
// Python runs **one** CPU worker because Tier 1 is pure Python and a second
// thread would contend on the GIL -- honest queueing rather than throughput.
// `fly.toml` records the other half of that: the registry is a module-level
// dict, so a second uvicorn worker would answer 404 for a job the first one
// holds. Neither reason survives the crossing. Here the CPU lane is a
// **semaphore over goroutines**, sized from the machine (see [Lane] and
// [Config.CPUWorkers]), so two sweeps genuinely run at once on a machine with
// two cores to run them on. ADR 38 named this as what Go buys; it is bought
// here.
//
// Three things that widening it changes downstream, written here because the
// next session will meet them and not here:
//
//   - **The sim family has no dedupe key.** `api/simruns.py` passes none: it
//     leans on ADR 18's cache instead, which covers "somebody asked before"
//     but not "somebody is asking right now". Two identical sweeps submitted
//     together used to queue; now they compute side by side and both store.
//     That is a wasted core, never a wrong answer -- and `sim/cache.py` opens
//     `app.db` in WAL with a 5s busy timeout and never raises, so a collision
//     is a short wait and, at worst, a logged warning.
//   - **Completion order is no longer submission order.** Nothing on the wire
//     promises it; `GET /api/jobs` sorts by `created_at`, which is a
//     submission stamp and unaffected.
//   - **A queued job is a parked goroutine, where Python queued a callable in
//     a deque.** Roughly 4KB of stack each against roughly 200 bytes, and the
//     live count is bounded by nothing but the request rate ([MaxJobs] evicts
//     only *finished* jobs). At the concurrency `fly.toml` admits -- 40 hard
//     -- that is under a megabyte, so the simpler structure wins; a bounded
//     queue would have to block the submitting request, which Python's
//     unbounded one never does.
//
// # What the port does not change
//
// **The lanes are still three, and two of them are not about the machine.**
// `NET` stays at two because a Claude call costs real money per run and a
// queue is a cheaper way to say "not four at once" than a rate limiter
// nobody has written; `FORGE` stays at one because two JVMs would race the
// shared `.dck` directory `ensure_profile` hands out. Sizing either from
// `GOMAXPROCS` would be reading the wrong fact off the wrong machine.
//
// **Every job carries an owner, and lookups take one.** A job is the first
// thing in this app that belongs to a person rather than to the library: its
// label names a deck and its result is a simulation somebody paid for. [Registry.Get]
// and [Registry.All] filter rather than trust the caller, and a job belonging
// to somebody else is reported as *absent* -- the route turns that into a
// 404, never a 403, so ids cannot be probed (ADR 5).
//
// **There is no cancellation, and inventing one here would be a worse lie
// than a few seconds of orphaned CPU.** Python says so at `forget_owner` and
// it is right: a dropped job's goroutine finishes into a record nothing
// points at. No `context.Context` reaches a worker for that reason -- a
// context that is never cancelled is a promise the caller would believe.
//
// # Two differences that are Go's, not the design's
//
// **The mutable half of a [Job] is guarded.** Python's worker writes
// `job.status`, `job.done` and `job.result` with no lock while a request
// thread reads them in `as_dict`; the GIL makes that survivable and the race
// detector makes it a hard failure. So the fields a worker touches live
// behind the registry's mutex and [Job.Payload] takes it. Nothing about the
// answers changes -- a poll still sees a torn-free snapshot of one moment.
//
// **A panicking worker is a failed job, not a dead process.** Python catches
// `Exception` around the call, so a bug in one job is one job's `error`
// string. Go's default is the opposite -- an unrecovered panic in any
// goroutine takes the process with it -- and for a door that is also serving
// every other request, that would be strictly worse than what it replaces.
// The runner recovers, records it as `panic: <what panicked>` so a reader can
// tell a crash from a refusal the worker made on purpose, and logs the stack
// exactly where Python's `traceback.print_exc()` goes.
//
// # The seam the flip has to close
//
// Python records a failure as `f"{type(exc).__name__}: {exc}"` -- a class
// name and a message, e.g. `DeckNotFound: no deck 'nope'` -- and that string
// reaches the browser (`tests/contract/golden/jobs.json`, the
// `sim-mana-missing-deck-done` shape). Go records `err.Error()`, which has
// no class name in it. **Whether the ported families reproduce the prefix is
// each family's decision, taken when it flips**, not something this package
// can decide for them; it is recorded here because the alternative is that
// nobody notices the strings changed. (There is a second question underneath
// it, for Aaron: a Python exception class name on a user-visible surface sits
// oddly beside commandment 10, and the flip is the moment to settle it.)
package jobs

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Lane is which pool work belongs in, and it is a property of the work rather
// than of the route -- which is why a [Plan] carries one and a handler never
// picks.
type Lane string

const (
	// CPU is work that burns CPU: Tier 1, the land sweep, the mulligan grid.
	// Wide as the machine allows here, where Python ran one worker to keep
	// two simulations off one GIL.
	CPU Lane = "cpu"

	// NET is work that waits on a socket: anything that calls Anthropic. Two
	// at once, and the bound is about money rather than throughput -- see the
	// package comment.
	NET Lane = "net"

	// FORGE is work that waits on a Forge subprocess (ADR 35). It fits
	// neither of the others: in CPU it would block Tier 1 for the minutes a
	// match takes, and at NET's width two matches could run at once, which
	// races the shared `.dck` directory and saturates the machine besides.
	// One worker makes both impossible by construction rather than by care.
	FORGE Lane = "forge"
)

// Lanes are the three, in the order Python's error message lists them --
// `sorted(_LANES)`, which is alphabetical and happens to read cpu, forge,
// net.
var Lanes = []Lane{CPU, FORGE, NET}

// Status is where a job is. The four are Python's strings verbatim, because
// `web/src/lib/api.ts` compares against them.
const (
	Queued  = "queued"
	Running = "running"
	Done    = "done"
	Errored = "error"
)

// live reports whether a status is one a second ask may join. A finished job
// is a cache's problem, not this module's, and a failed one must stay
// retryable or a transient Anthropic error would be permanent until the
// process restarted.
func live(status string) bool { return status == Queued || status == Running }

// MaxJobs bounds the registry, so a long-lived session does not grow without
// one. The bound is global rather than per owner, and eviction takes the
// oldest *finished* job -- which on a shared instance is very likely one
// somebody's tab is still polling, since a job born `done` (a cache hit) is
// finished the moment it is handed out. Fifty was sized for one laptop; two
// hundred costs memory only when occupied.
const MaxJobs = 200

// Progress is what a worker is handed to report with.
//
// Two methods rather than one variadic call, because Python's third argument
// means something different when it is omitted than when it is passed:
// `partial=None` leaves whatever was there alone, and only a real payload
// replaces it. An interface says that where a `nil` would have to be
// explained.
//
// Partial is the match theater's: rows of a match still being played,
// renderable before the result exists. It is serialised on the job while the
// job runs and **cleared the moment the job finishes**, because the result is
// the whole answer and a leftover partial is a second copy that would go
// stale.
type Progress interface {
	// Report is `progress(done, total)`: the two-argument call every worker
	// written before the theater still makes.
	Report(done, total int)
	// ReportPartial is `progress(done, total, partial)`.
	ReportPartial(done, total int, partial any)
}

// Runner is the work itself: `Callable[[Progress], Any]`, plus Go's second
// return value where Python raised.
type Runner func(Progress) (any, error)

// Plan is what a slow request turns into: an answer, or the work to produce
// one.
//
// Result is set when the answer was already available -- a cached simulation,
// or a Claude mode whose stance is `off` and so made no call. In that case
// Run is never called. Exactly one of them is used, and [Registry.FromPlan]
// turns that into a job born finished or a job that was queued.
//
// It lives here rather than beside any caller because it is the shape of a
// job's *input*, and there are several producers of one. Lane is the second
// reason: which pool the work belongs in is a property of the work, so it is
// decided where the work is planned rather than at the route, where somebody
// would eventually forget.
type Plan struct {
	Kind  string
	Label string
	// Result short-circuits: non-nil means the answer is already known.
	Result any
	Run    Runner
	// Lane defaults to CPU when empty, matching Python's `lane: str = CPU`.
	Lane Lane
	// Key is what this work *is*, when two simultaneous requests for it are
	// the same request. Decided by the planner, because only the planner
	// knows what identity means for its own work -- for the dossier it is the
	// commander, not the deck. Empty means "start a second one", which is
	// right for anything not reproducible; see [Registry.Submit].
	Key string
}

// UnknownLaneError is what [Registry.Submit] answers for a lane nothing will
// ever run, checked before the job is recorded so a typo is an error out of
// the route rather than a job that sits `queued` forever.
//
// Its message reproduces Python's `ValueError` byte for byte, list repr
// included, because that string is what a 500 carries today and the contract
// is the whole point of the exercise.
type UnknownLaneError struct{ Lane Lane }

func (e *UnknownLaneError) Error() string {
	names := make([]string, 0, len(Lanes))
	for _, lane := range Lanes {
		names = append(names, pyRepr(string(lane)))
	}
	return fmt.Sprintf("unknown job lane %s; expected one of [%s]",
		pyRepr(string(e.Lane)), strings.Join(names, ", "))
}

// pyRepr is `repr()` for a string, which is not `strconv.Quote`: **Python
// prefers single quotes**, and switches to double ones only when the string
// holds a single quote and no double. That is the whole of the rule for
// anything a lane can be -- a short identifier out of a planner -- and the
// corpus carries a quoted case in each direction so the preference is
// checked rather than assumed. Escapes beyond the quote, the backslash and
// the three whitespace ones are out of reach here and are not reproduced.
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var out strings.Builder
	out.WriteByte(quote)
	for _, r := range s {
		switch r {
		case rune(quote):
			out.WriteByte('\\')
			out.WriteByte(quote)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte(quote)
	return out.String()
}

// Payload is `Job.as_dict()`: what a poll gets back. The field order is
// Python's insertion order, so the bytes match as well as the keys.
//
// Owner and Key are deliberately absent. Owner because a caller who can see a
// job already knows whose it is, and one who cannot must not learn it exists;
// Key because it is an internal identity -- and for the dossier it is the
// cache key, which names a card.
type Payload struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	Done      int     `json:"done"`
	Total     int     `json:"total"`
	Percent   int     `json:"percent"`
	Partial   any     `json:"partial"`
	Label     string  `json:"label"`
	Result    any     `json:"result"`
	Error     *string `json:"error"`
	CreatedAt string  `json:"created_at"`
}

// percent is `round(100 * done / total) if total else 0`, and the only
// interesting word in it is `round`.
//
// **Python rounds half to even and Go's `math.Round` rounds half away from
// zero**, so one job in eight disagrees: at done=1, total=8 the true value is
// 12.5, and Python answers 12 where `math.Round` answers 13. The corpus in
// `testdata/jobs.json` is what caught it, and it is checked rather than
// argued because a one-point difference in a progress bar is exactly the kind
// of wrongness nobody reports.
//
// The arithmetic is done in float64 for the same reason: Python computes
// `100 * done / total` as a true division, so the value being rounded is a
// float with a float's representation, and integer arithmetic here would
// disagree on its own schedule.
func percent(done, total int) int {
	if total == 0 {
		return 0
	}
	return roundHalfToEven(float64(100*done) / float64(total))
}

// roundHalfToEven is Python's built-in `round(x)` for a float with no digit
// count: nearest integer, ties to the even one. CPython does it as C's
// `round` -- half away from zero -- corrected back to even when the distance
// is exactly a half, and this is that, the other way round.
func roundHalfToEven(x float64) int {
	nearest := math.Floor(x)
	frac := x - nearest
	switch {
	case frac > 0.5:
		nearest++
	case frac == 0.5 && math.Mod(nearest, 2) != 0:
		nearest++
	}
	return int(nearest)
}

// stamp is `datetime.now(UTC).isoformat()`.
//
// Two details, and both are load-bearing rather than cosmetic. **The
// fractional part vanishes entirely when there is none** -- Python writes
// `...:20+00:00`, not `...:20.000000+00:00` -- which happens once in a
// million and would otherwise be a shape no client had seen. And **the offset
// is spelled `+00:00`, never `Z`**, which is what `web/src/lib/api.ts` has
// always been handed.
//
// Both matter more than they look, because this string is also the **sort
// key**: Python sorts `all_jobs` on the text, not on a datetime. That is
// sound -- every field is fixed width, and within one second the elided form
// begins with `+` (0x2B), which sorts below every digit the six-digit form
// can start with, so the microsecond-zero job correctly comes first. Storing
// the string rather than the instant is therefore the faithful choice, not a
// shortcut.
func stamp(t time.Time) string {
	t = t.UTC().Truncate(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}
