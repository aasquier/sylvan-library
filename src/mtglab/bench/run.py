"""Sample the declared suite, cold or warm, and report it as a table.

Two numbers per target, because 2026-08-19 left the ledger unable to record
what it had learned: the deck shelf went 201ms to 16ms **warm** and did not
move at all **cold**, and a ledger row with one slot for a millisecond can
hold neither honestly. So cold and warm are separate runs of this module, and
`--cold` means what it says -- every registered cache emptied between samples,
which is only possible because the bench runs in-process.

Medians, never means. Game length taught this repo that lesson and response
times deserve it too: one 300ms sample from a GC pause moves a mean and does
not move a median, and the tail is reported separately rather than smeared
into the headline.
"""

from __future__ import annotations

import statistics
import time
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from mtglab import caches
from mtglab.bench.profile import Profile, profile_target
from mtglab.bench.targets import PROFILE_OVER_MS, Target


@dataclass(frozen=True)
class Sample:
    """One target's timings, or the reason it did not run."""

    target: Target
    median_ms: float = 0.0
    p95_ms: float = 0.0
    best_ms: float = 0.0
    runs: int = 0
    skipped: str = ""
    failed: str = ""
    profile: Profile | None = None

    @property
    def ran(self) -> bool:
        return self.runs > 0


def run_suite(targets: list[Target], *, runs: int = 12, cold: bool = False,
              profile_over_ms: float = PROFILE_OVER_MS,
              ) -> list[Sample]:
    """Time every target. Unavailable ones come back as rows, never as gaps."""
    return [_sample(t, runs=runs, cold=cold, profile_over_ms=profile_over_ms)
            for t in targets]


def _sample(target: Target, *, runs: int, cold: bool,
            profile_over_ms: float) -> Sample:
    if target.unavailable:
        return Sample(target=target, skipped=target.unavailable)

    try:
        if cold:
            caches.clear_all()
        else:
            # One unmeasured call, so a warm run measures the warm path rather
            # than the first-request price averaged into eleven cheap ones.
            target.call()
    except Exception as exc:                                        # noqa: BLE001
        return Sample(target=target, failed=f"{type(exc).__name__}: {exc}")

    times = []
    try:
        for _ in range(runs):
            if cold:
                caches.clear_all()
            start = time.perf_counter()
            target.call()
            times.append((time.perf_counter() - start) * 1000.0)
    except Exception as exc:                                        # noqa: BLE001
        return Sample(target=target, failed=f"{type(exc).__name__}: {exc}")

    times.sort()
    median = statistics.median(times)
    sample = Sample(
        target=target, median_ms=median,
        p95_ms=times[min(int(len(times) * 0.95), len(times) - 1)],
        best_ms=times[0], runs=len(times))

    # Gap 1 of the 2026-08-19 refinement, made mechanical: a number over the
    # threshold is a question, and the tool asks it rather than trusting a run
    # to remember that it should.
    if median >= profile_over_ms:
        try:
            prof = profile_target(target.name, _as_measured(target, cold=cold))
        except Exception:                                           # noqa: BLE001
            prof = None
        sample = Sample(**{**sample.__dict__, "profile": prof})
    return sample


def _as_measured(target: Target, *, cold: bool) -> Callable[[], Any]:
    """The target, called the way the run measured it.

    Load-bearing, and it was wrong once. A cold run used to hand the bare
    callable to the profiler, so the table said 38.3ms cold and the profile
    beneath it said 7.2ms — a *warm* breakdown printed under a cold heading,
    attributing a cold request's cost to whatever the warm one happened to do.
    A profile that does not describe the number it sits under is worse than no
    profile, because it is read as the explanation.
    """
    if not cold:
        return target.call

    def measured() -> Any:
        caches.clear_all()
        return target.call()

    return measured


def as_markdown(samples: list[Sample], *, cold: bool) -> str:
    """The table, ready to paste into `docs/polish/LEDGER.md`.

    Markdown rather than JSON because the ledger is prose that people read,
    and a measurement nobody can read next to last quarter's is a measurement
    that will not be compared.
    """
    state = "cold" if cold else "warm"
    lines = [f"| target | {state} median | p95 | best | database | statements |",
             "|---|---:|---:|---:|---:|---:|"]
    for s in samples:
        if s.skipped:
            lines.append(f"| `{s.target.name}` | — | — | — | — | "
                         f"_skipped: {s.skipped}_ |")
            continue
        if s.failed:
            lines.append(f"| `{s.target.name}` | — | — | — | — | "
                         f"**failed: {s.failed}** |")
            continue
        if s.profile is not None:
            database = f"{s.profile.db_s * 1000:.1f}ms"
            stmts = str(s.profile.queries.count)
        else:
            database = stmts = "—"
        lines.append(f"| `{s.target.name}` | {s.median_ms:.1f}ms | "
                     f"{s.p95_ms:.1f}ms | {s.best_ms:.1f}ms | {database} | "
                     f"{stmts} |")
    return "\n".join(lines)
