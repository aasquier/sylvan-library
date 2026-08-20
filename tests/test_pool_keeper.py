"""The pool keeper's lease must expire between two health checks.

`service._KEEPER` holds a read-only DuckDB handle so a burst of requests --
a page load is four of them -- does not pay 17.5ms to reopen the pool each
time. A held handle is a held **shared lock**, and `mtglab data refresh`
needs the **exclusive** one, so the keeper is a lease rather than a claim:
`_reap_keeper` lets go once nobody has wanted the pool for `_KEEPER_IDLE`.

That reasoning was right and the number under it was not. `_KEEPER_IDLE` was
30.0 while `fly.toml` calls `/api/health` every 30s -- and `service.health`
opens the pool, counting both tables and asking `pool_stale`. So the lease
was renewed by the one caller that never stops asking, exactly as often as it
expired, and the app held the shared lock forever. On 2026-08-19 a refresh on
the instance was refused **forty times over five minutes**, always by the same
holder, always at `connect`, before `load_printings` could touch a row. The
runbook's one-line step 6 had never worked on a populated volume; the only
time it succeeded there was no pool file to lock.

**This is a truth table, not a string match** -- the lesson `test_packaging`
paid for in #86 and #188. Restating "the interval is 30s" would pass against
the exact bug it was written for the day somebody sets it to 10s. So the
ceiling is *derived* from `fly.toml`, and moving the check fails here.
"""

from __future__ import annotations

import tomllib
from pathlib import Path

import pytest

from mtglab.api import service

ROOT = Path(__file__).resolve().parents[1]
FLY = ROOT / "fly.toml"


def _seconds(value: str) -> float:
    """Fly durations, in the two shapes this file actually uses."""
    text = value.strip()
    for suffix, scale in (("ms", 0.001), ("s", 1.0), ("m", 60.0), ("h", 3600.0)):
        if text.endswith(suffix):
            return float(text[: -len(suffix)]) * scale
    raise ValueError(f"unrecognised fly duration: {value!r}")


def health_check_intervals() -> list[float]:
    """Every interval at which the platform calls a health endpoint."""
    config = tomllib.loads(FLY.read_text())
    checks = config.get("http_service", {}).get("checks", [])
    return [_seconds(c["interval"]) for c in checks if "interval" in c]


def test_the_health_check_is_still_declared() -> None:
    """A derived ceiling over an empty set is not a ceiling.

    If the checks block is renamed or removed, every assertion below passes
    vacuously -- which is the failure mode this whole file exists to refuse.
    """
    intervals = health_check_intervals()
    assert intervals, "no http_service check found in fly.toml"
    assert all(i > 0 for i in intervals)


def test_the_keeper_lets_go_between_two_health_checks() -> None:
    """The lease must be strictly shorter than the shortest check interval.

    Equality is the bug: at 30.0 against 30s the lease came due exactly as
    the next call arrived, and the call won every time for five minutes.
    """
    shortest = min(health_check_intervals())
    assert shortest > service._KEEPER_IDLE, (
        f"the pool keeper holds its lease for {service._KEEPER_IDLE}s while "
        f"the platform calls a health endpoint every {shortest}s -- the lease "
        f"can never expire, and `mtglab data refresh` can never take the "
        f"write lock on the instance")


def test_the_lease_leaves_a_real_window_not_a_hairline() -> None:
    """Strictly-shorter is necessary and not sufficient.

    29.9s against 30s satisfies the test above and loses the same race on
    scheduler jitter. The window a refresh has to squeeze through should be a
    meaningful fraction of the cycle, so the reaper -- which wakes every
    `_KEEPER_IDLE / 3` -- gets several looks at an idle pool inside it.
    """
    shortest = min(health_check_intervals())
    window = shortest - service._KEEPER_IDLE
    assert window >= shortest / 2, (
        f"only {window}s of each {shortest}s cycle leaves the pool free; a "
        f"refresh has to win a race rather than walk through a door")


def test_the_health_endpoint_is_what_makes_this_load_bearing() -> None:
    """Pin *why* the ceiling exists, so a later reader cannot mistake it.

    If `health` ever stopped opening the pool, the collision would disappear
    and this file would be guarding nothing -- worth knowing deliberately
    rather than discovering. It is the pool-reading that makes the interval
    a ceiling at all.
    """
    source = Path(service.__file__).read_text()
    body = source.split("def health(")[1].split("\ndef ")[0]
    assert "_connect()" in body, (
        "service.health no longer opens the pool -- the keeper lease and the "
        "health-check interval may no longer be coupled; re-derive the "
        "ceiling in this file rather than deleting it")


@pytest.mark.parametrize("burst", [1, 4, 8])
def test_the_lease_still_covers_a_page_load(burst: int) -> None:
    """The lease has a floor too, and it is the reason it exists.

    A page load is about four requests. Shrinking `_KEEPER_IDLE` to buy the
    refresh its window must not shrink it past the burst it was bought for --
    that would trade a 17.5ms reopen onto every request to solve a problem
    that happens monthly.
    """
    # A generous upper bound on how long a burst of `burst` requests takes to
    # arrive: even a slow client is well inside a second per request.
    assert burst * 1.0 <= service._KEEPER_IDLE
