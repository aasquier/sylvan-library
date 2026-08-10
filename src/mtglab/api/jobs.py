"""In-process job registry for work too slow to answer inside a request.

A Tier 1 sweep at 20,000 games takes around 30 seconds and a land sweep across
eleven counts takes minutes. Neither can run inside a request handler, and
neither is worth a real task queue for a single-user local app -- that would
add a broker, a worker process, and a whole class of "is redis running?"
failures to something a friend is supposed to be able to clone and run.

So: a bounded thread pool plus a dict. Jobs are ephemeral by design. Restart
the server and they are gone, which is correct for a local tool where the
inputs are cheap to resubmit.
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


def submit(kind: str, fn: Callable[[Callable[[int, int], None]], Any],
           *, label: str = "") -> Job:
    """Queue `fn`, handing it a `progress(done, total)` callback to report with."""
    job = Job(id=uuid.uuid4().hex[:12], kind=kind, label=label)
    with _LOCK:
        _JOBS[job.id] = job
        if len(_JOBS) > MAX_JOBS:
            finished = [j for j in _JOBS.values() if j.status in ("done", "error")]
            for old in sorted(finished, key=lambda j: j.created_at)[:len(_JOBS) - MAX_JOBS]:
                _JOBS.pop(old.id, None)

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


def get(job_id: str) -> Job | None:
    with _LOCK:
        return _JOBS.get(job_id)


def all_jobs() -> list[Job]:
    with _LOCK:
        return sorted(_JOBS.values(), key=lambda j: j.created_at, reverse=True)


def clear() -> None:
    """Test helper -- drops every recorded job."""
    with _LOCK:
        _JOBS.clear()
