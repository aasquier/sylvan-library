"""In-process job registry for work too slow to answer inside a request.

A Tier 1 sweep at 20,000 games takes around 30 seconds and a land sweep across
eleven counts takes minutes. Neither can run inside a request handler, and
neither is worth a real task queue for a single-user local app -- that would
add a broker, a worker process, and a whole class of "is redis running?"
failures to something a friend is supposed to be able to clone and run.

So: a bounded thread pool plus a dict. Jobs are ephemeral by design. Restart
the server and they are gone, which is correct for a local tool where the
inputs are cheap to resubmit.

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
from typing import Any

# One worker. The simulation is CPU-bound pure Python, so extra threads would
# contend on the GIL and make every job slower rather than adding throughput.
# Queueing is the honest behaviour here.
_EXECUTOR = ThreadPoolExecutor(max_workers=1, thread_name_prefix="mtglab-job")
_LOCK = threading.Lock()
_JOBS: dict[str, Job] = {}

# Keep the registry from growing without bound in a long-lived session.
MAX_JOBS = 50


@dataclass
class Job:
    id: str
    kind: str
    status: str = "queued"        # queued | running | done | error
    done: int = 0
    total: int = 0
    result: Any = None
    error: str | None = None
    label: str = ""
    # Whose job this is. `None` means "the local user", which is everybody when
    # auth is off. Never serialised: a caller who can see a job already knows
    # whose it is, and one who cannot must not learn it exists.
    owner: int | None = None
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
            "label": self.label,
            "result": self.result,
            "error": self.error,
            "created_at": self.created_at,
        }


def _record(job: Job) -> Job:
    """Register a job, evicting finished ones if the registry is over its bound."""
    with _LOCK:
        _JOBS[job.id] = job
        if len(_JOBS) > MAX_JOBS:
            finished = [j for j in _JOBS.values() if j.status in ("done", "error")]
            for old in sorted(finished, key=lambda j: j.created_at)[:len(_JOBS) - MAX_JOBS]:
                _JOBS.pop(old.id, None)
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


def submit(kind: str, fn: Callable[[Callable[[int, int], None]], Any],
           *, label: str = "", owner: int | None = None) -> Job:
    """Queue `fn`, handing it a `progress(done, total)` callback to report with."""
    job = _record(Job(id=uuid.uuid4().hex[:12], kind=kind, label=label,
                      owner=owner))

    def progress(done: int, total: int) -> None:
        job.done, job.total = done, total

    def wrapped() -> None:
        job.status = "running"
        try:
            job.result = fn(progress)
            job.status = "done"
            if job.total:
                job.done = job.total
        except Exception as exc:                                    # noqa: BLE001
            # Surface the type and message; a local tool is more useful when it
            # says what broke than when it returns a bare 500.
            job.error = f"{type(exc).__name__}: {exc}"
            job.status = "error"
            traceback.print_exc()

    _EXECUTOR.submit(wrapped)
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


def clear() -> None:
    """Test helper -- drops every recorded job."""
    with _LOCK:
        _JOBS.clear()
