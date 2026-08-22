"""In-process job registry for work too slow to answer inside a request.

A Tier 1 sweep at 20,000 games takes around 30 seconds and a land sweep across
eleven counts takes minutes. Neither can run inside a request handler, and
neither is worth a real task queue for a single-user local app -- that would
add a broker, a worker process, and a whole class of "is redis running?"
failures to something a friend is supposed to be able to clone and run.

So: a bounded thread pool plus a dict. Jobs are ephemeral by design. Restart
the server and they are gone, which is correct for a local tool where the
inputs are cheap to resubmit.

**There are two pools, and the split is about what the work waits on.** A Tier 1
sweep is CPU-bound pure Python, so a second simulation thread would contend on
the GIL and make both slower -- one worker, honest queueing. A Claude call is
the opposite shape: it is a socket wait that releases the GIL for minutes at a
time (the theme proposal was measured at 226 seconds), so a single shared queue
would stall a thirty-second sweep behind four minutes of somebody else's
conversation while the CPU sat idle. Hence `CPU` and `NET`. The `NET` lane is
bounded at two for a reason that is not throughput: a proposal costs real money
per run, and a queue is a cheaper way to say "not four at once" than a
rate limiter nobody has written yet.

**Every job carries an owner, and lookups take one.** A job is the first thing
in this app that belongs to a person rather than to the library: its label
names a deck and its result is a simulation somebody paid thirty seconds of CPU
for. `get()` and `all_jobs()` therefore filter rather than trust the caller to,
and a job belonging to somebody else is reported as *absent* — the route turns
`None` into a 404, never a 403, so ids cannot be probed (ADR 5).

The owner is `None` when auth is off, which is every local run: one person,
every job theirs.
"""

from __future__ import annotations

import threading
import traceback
import uuid
from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any, Protocol

#: Work that burns CPU: Tier 1, the land sweep. One worker, because the
#: simulation is pure Python and extra threads would contend on the GIL and
#: make every job slower rather than adding throughput.
CPU = "cpu"

#: Work that waits on a socket: anything that calls Anthropic. Two workers,
#: because such a job holds no GIL and blocking a sweep behind it would be a
#: four-minute stall for nothing. See the module docstring for why two.
NET = "net"

#: Work that waits on a Forge subprocess (ADR 35). The thread sleeps in
#: `subprocess.run` while the JVM burns the CPU, so it fits neither lane:
#: in `CPU` it would block Tier 1 sweeps for the minutes a match takes, and
#: in `NET` two matches could run at once — which races the shared `.dck`
#: directory `ensure_profile` hands out and saturates the machine besides.
#: One worker makes both impossible by construction rather than by care.
FORGE = "forge"

_LANES: dict[str, ThreadPoolExecutor] = {
    CPU: ThreadPoolExecutor(max_workers=1, thread_name_prefix="mtglab-job"),
    NET: ThreadPoolExecutor(max_workers=2, thread_name_prefix="mtglab-net"),
    FORGE: ThreadPoolExecutor(max_workers=1, thread_name_prefix="mtglab-forge"),
}
_LOCK = threading.Lock()
_JOBS: dict[str, Job] = {}

# Keep the registry from growing without bound in a long-lived session. The
# bound is global, not per owner, and eviction takes the oldest *finished*
# job — which on a shared instance is very likely one somebody's tab is still
# polling, since a job born `done` (a cache hit) is finished the moment it is
# handed out. Fifty was sized for one laptop; a hundred accounts sharing fifty
# slots would evict results mid-poll under entirely ordinary use. Two hundred
# costs memory only when occupied (a Tier 1 result is a few kB of curves), and
# the LRU-by-`created_at` order is unchanged.
MAX_JOBS = 200

class Progress(Protocol):
    """What a worker is handed to report with: `progress(done, total)`.

    `partial` is the third, optional argument (the match theater): a payload
    the client may render before the result exists — the rows of a match
    still being played. It replaces what was there, is serialised on the job
    while the job runs, and is cleared the moment the job finishes, because
    the result is the whole answer and a leftover partial is a second copy
    that would go stale. Progress was a plain two-argument Callable until
    then; every existing worker still calls it that way, which is why the
    third argument defaults rather than demands.
    """

    # `done` and `total` are positional-only: workers name those parameters
    # freely (`simruns` calls one `_total`), and a Protocol would otherwise
    # bind the names as part of the contract.
    def __call__(self, done: int, total: int, /,
                 partial: Any | None = None) -> None: ...


@dataclass
class Job:
    id: str
    kind: str
    status: str = "queued"        # queued | running | done | error
    done: int = 0
    total: int = 0
    result: Any = None
    # What is known before the result is: the match theater's rows-so-far.
    # Live only while the job is — `submit` clears it on completion.
    partial: Any = None
    error: str | None = None
    label: str = ""
    # Whose job this is. `None` means "the local user", which is everybody when
    # auth is off. Never serialised: a caller who can see a job already knows
    # whose it is, and one who cannot must not learn it exists.
    owner: int | None = None
    #: What this job *is*, for work where asking twice at once is a mistake
    #: rather than a request. `None` opts out and is the default; see `submit`.
    #: Not serialised — it is an internal identity, and for the dossier it is
    #: the cache key, which names a card the caller already knows they asked
    #: about.
    key: str | None = None
    created_at: str = field(
        default_factory=lambda: datetime.now(UTC).isoformat())

    def as_dict(self) -> dict[str, Any]:
        pct = round(100 * self.done / self.total) if self.total else 0
        return {
            "id": self.id,
            "kind": self.kind,
            "status": self.status,
            "done": self.done,
            "total": self.total,
            "percent": pct,
            "partial": self.partial,
            "label": self.label,
            "result": self.result,
            "error": self.error,
            "created_at": self.created_at,
        }


@dataclass
class Plan:
    """What a slow request turns into: an answer, or the work to produce one.

    `result` is set when the answer was already available -- a cached
    simulation, or a Claude mode whose stance is `off` and so made no call. In
    that case `run` is never called. Exactly one of them is used, and the route
    turns that into a job born finished or a job that was queued.

    It lives here rather than beside either caller because it is the shape of a
    job's *input*, and there are now two producers of one: `api/simruns.py` and
    `api/themeruns.py`. `lane` is the second reason -- which pool the work
    belongs in is a property of the work, so it is decided where the work is
    planned rather than at the route, where somebody would eventually forget.
    """

    kind: str
    label: str
    result: dict[str, Any] | None
    run: Callable[[Progress], dict[str, Any]]
    lane: str = CPU
    #: What this work *is*, when two simultaneous requests for it are the same
    #: request. Decided by the planner because only the planner knows what
    #: identity means for its own work — for the dossier it is the commander,
    #: not the deck. `None` means "start a second one", which is right for
    #: anything not reproducible; see `submit`.
    key: str | None = None


#: A job that has not finished. Two of these for the same `key` is the bug
#: `submit` exists to prevent; a finished one is a cache's problem, not this
#: module's.
LIVE = ("queued", "running")


def _keep_locked() -> None:
    """Evict finished jobs if the registry is over its bound. `_LOCK` held."""
    if len(_JOBS) <= MAX_JOBS:
        return
    finished = [j for j in _JOBS.values() if j.status not in LIVE]
    for old in sorted(finished, key=lambda j: j.created_at)[:len(_JOBS) - MAX_JOBS]:
        _JOBS.pop(old.id, None)


def _record(job: Job) -> Job:
    """Register a job, evicting finished ones if the registry is over its bound."""
    with _LOCK:
        _JOBS[job.id] = job
        _keep_locked()
    return job


def completed(kind: str, *, result: Any, label: str = "",
              owner: int | None = None) -> Job:
    """A job that is already finished, because its answer was already known.

    This is what a simulation cache hit returns (`sim/cache.py`). The
    alternative was a second response shape saying "no job, here is the
    result", and it was rejected: every client would need a branch, a hit would
    leave no trace in `/api/jobs`, and the claim being made is not "this is not
    a job" but "this job took no time". A job born done is the honest shape and
    the response contract does not fork.

    It still costs a registry slot, which is the point of `_record` being
    shared -- fifty instant hits evict fifty finished jobs, not the running one.
    """
    return _record(Job(id=uuid.uuid4().hex[:12], kind=kind, status="done",
                       done=1, total=1, result=result, label=label,
                       owner=owner))


def submit(kind: str, fn: Callable[[Progress], Any],
           *, label: str = "", owner: int | None = None,
           lane: str = CPU, key: str | None = None) -> Job:
    """Queue `fn`, handing it a `progress(done, total)` callback to report with.

    `lane` picks the pool -- `CPU` for work that computes, `NET` for work that
    waits on Anthropic. Checked before the job is recorded, so a typo is an
    exception out of the route rather than a job that sits `queued` forever
    because nothing was ever going to run it.

    **`key` makes asking twice at once return the same job, and that is a
    money bug rather than a tidiness one.** A dossier takes about four minutes
    and pays for a web search; nothing stopped a second click, a second tab or
    a second device inside that window from starting a *second* paid run for
    the same commander, and on 2026-08-13 two ran concurrently on the deployed
    instance for exactly that reason. The client-side reattach that was built
    first only ever covered one tab, because it lives in that tab's
    localStorage; this covers every case at once by making the server the
    thing that knows a run is already going.

    Matching is per **owner** as well as per key, which is deliberate rather
    than cautious: a job belongs to a person (ADR 5) and `get` refuses one that
    is not yours, so handing two accounts the same id would give the second a
    404 for a job it had just been told about. Every case the bug actually
    covers -- one person, two tabs -- is one owner.

    Only a *live* job is reused. A finished one is what a cache is for, and the
    dossier has one (ADR 19); a failed one must be retryable or a transient
    Anthropic error would be permanent until the process restarted.

    The lookup and the insert are one locked step, because the window this
    closes is exactly the window two concurrent requests race in.
    """
    pool = _LANES.get(lane)
    if pool is None:
        raise ValueError(
            f"unknown job lane {lane!r}; expected one of {sorted(_LANES)}")

    with _LOCK:
        if key:
            already = next(
                (j for j in _JOBS.values()
                 if j.key == key and j.kind == kind and j.owner == owner
                 and j.status in LIVE),
                None)
            if already is not None:
                return already
        job = Job(id=uuid.uuid4().hex[:12], kind=kind, label=label,
                  owner=owner, key=key)
        _JOBS[job.id] = job
        _keep_locked()

    def progress(done: int, total: int, partial: Any | None = None) -> None:
        job.done, job.total = done, total
        if partial is not None:
            job.partial = partial

    def wrapped() -> None:
        job.status = "running"
        try:
            job.result = fn(progress)
            job.status = "done"
            if job.total:
                job.done = job.total
            # The result is the whole answer; a partial left behind is a
            # stale second copy of part of it.
            job.partial = None
        except Exception as exc:                                    # noqa: BLE001
            # Surface the message and NOT the exception's class name.
            #
            # This read `f"{type(exc).__name__}: {exc}"` until 2026-08-22, so a
            # failed job rendered `DeckNotFound: no deck 'nope'` in the
            # browser -- `job.error` becomes a JS `Error` in `lib/api.ts` and
            # the screen shows it. A Python class name is a technology a user
            # can see, which commandment 10 does not allow; Aaron ruled on it
            # when the Go port surfaced the seam, since `internal/jobs`
            # records `err.Error()` and has no class name to offer. Correcting
            # Python rather than teaching Go to imitate it is what makes the
            # two agree *now*, instead of at whichever flip moves this family.
            #
            # Nothing is lost. Every exception a job worker can raise carries
            # a message -- checked, there are no bare raises -- so the prefix
            # was never the load-bearing half, and `traceback.print_exc()`
            # still puts the class name in the log, where a maintainer reads
            # it and a visitor does not.
            job.error = str(exc)
            job.status = "error"
            traceback.print_exc()

    pool.submit(wrapped)
    return job


def get(job_id: str, *, owner: int | None = None) -> Job | None:
    """One job, if it is this owner's. `None` covers both "no such job" and
    "not yours", which is the point -- the caller cannot tell them apart, and
    neither can whoever is probing ids."""
    with _LOCK:
        job = _JOBS.get(job_id)
    return job if job is not None and job.owner == owner else None


def all_jobs(*, owner: int | None = None) -> list[Job]:
    """This owner's jobs, newest first."""
    with _LOCK:
        mine = [j for j in _JOBS.values() if j.owner == owner]
    return sorted(mine, key=lambda j: j.created_at, reverse=True)


def census() -> dict[str, int]:
    """How many jobs the registry holds, by status. Counts and nothing else.

    The admin dashboard's view of this registry, and deliberately the only
    cross-owner one: `get` and `all_jobs` scope to an owner because a job's
    label can name another person's deck, and administering the instance
    (ADR 17) is about load, not about reading anybody's work. Counts cannot
    carry a name.
    """
    with _LOCK:
        counts: dict[str, int] = {}
        for job in _JOBS.values():
            counts[job.status] = counts.get(job.status, 0) + 1
    return counts


def forget_owner(owner: int) -> int:
    """Drop every job belonging to one account. Returns how many went.

    Called when an account is deleted, and the reason is `users.id`: it is
    `INTEGER PRIMARY KEY` without `AUTOINCREMENT`, so SQLite is free to re-issue
    a deleted account's rowid to the next account created. Jobs are keyed on
    that integer and held in memory, so without this the next holder of the id
    would inherit the dead account's jobs -- results, and the deck names in
    their labels. That is the isolation `get` and `all_jobs` are written to
    enforce, defeated by arithmetic rather than by a missing filter.

    A running job is dropped from the registry but not cancelled; its thread
    finishes into a record nothing points at. That is the honest trade -- the
    pools have no cancellation and inventing one here would be a worse lie than
    a few seconds of orphaned CPU.

    `owner` is never `None` here: that is the no-auth local case, where there is
    one person and no account to delete.
    """
    with _LOCK:
        doomed = [k for k, j in _JOBS.items() if j.owner == owner]
        for key in doomed:
            del _JOBS[key]
    return len(doomed)


def clear() -> None:
    """Test helper -- drops every recorded job."""
    with _LOCK:
        _JOBS.clear()
