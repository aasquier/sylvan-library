"""The register of what this process memoises, and whether it is working.

Every cache in this package is a claim -- *this question repeats, so stop
asking it* -- and a claim nobody measures is a claim that can rot in silence.
The 2026-08-19 performance pass caught one doing exactly that: an
`oracle_columns` cache keyed on the connection object was correct, tested, and
**never once hit**, because every endpoint opens one handle, asks one question
and closes it. No test could have found that; only a counter could.

So each cache registers here with three things: a name, a way to empty it, and
a pair of counters it bumps at its own hit and miss branches. That buys three
powers, and the registry exists because they are one mechanism:

* `mtglab bench caches` reports the hit rate, so a cache that never hits shows
  up as the dead weight it is rather than as a confident paragraph of comment.
* `mtglab bench --cold` empties every cache between samples, which is what
  makes a cold number honest -- the alternative measures the second request
  forever and calls it the first.
* `tests/test_caches.py` asserts a real workload hits each one, and sweeps the
  package for module-level state that never registered. A cache written next
  year is in the report the day it lands, or the sweep fails.

Stdlib only, and it stays that way: `cards/db.py` imports it, and that module
is the boundary DuckDB lives behind.

The counters are plain integers bumped without a lock. `hits += 1` is not
atomic across threads, so a busy process can undercount by a handful. That is
deliberate -- what is lost is a statistic, never a cached value, and a lock on
the read path of a cache that exists to be fast would cost more than the
number is worth.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any, Protocol


@dataclass
class CacheStats:
    """One cache's hit/miss counters. Held by the module that owns the cache."""

    name: str
    hits: int = 0
    misses: int = 0

    def hit(self) -> None:
        self.hits += 1

    def miss(self) -> None:
        self.misses += 1

    def reset(self) -> None:
        self.hits = self.misses = 0

    @property
    def asked(self) -> int:
        return self.hits + self.misses

    @property
    def rate(self) -> float | None:
        """Hits over questions, or None when nobody has asked yet.

        None rather than 0.0 on purpose: a cache nobody consulted and a cache
        that missed every time are different findings, and a report rendering
        both as `0%` hides the more interesting one.
        """
        return self.hits / self.asked if self.asked else None


class _Cached(Protocol):
    """The shape `functools.cache` and `functools.lru_cache` leave behind."""

    def cache_info(self) -> Any: ...
    def cache_clear(self) -> None: ...


@dataclass(frozen=True)
class Registration:
    """A cache as the register knows it."""

    stats: CacheStats
    clear: Callable[[], None]
    #: Entries currently held, when the owner can say cheaply. A cache that
    #: cannot answer without walking itself reports None instead.
    size: Callable[[], int] | None = None
    #: Why this cache exists, in one line, for the report.
    note: str = ""
    #: Set for a `functools.cache`, which keeps its own counters; the register
    #: copies them in at report time rather than asking the owner to bump two.
    refresh: Callable[[CacheStats], None] | None = None
    #: True when what is held is an operating-system resource rather than a
    #: value -- an open file handle, a connection, a lock. Exactly one entry
    #: sets it, and the distinction earns its place: a stale *value* costs
    #: memory, while a stale *handle* can refuse somebody else's work.
    holds_handle: bool = False
    #: How such a cache zeroes its counters *without* being emptied. Clearing
    #: is the obvious way and the wrong one -- `auth.passwords._dummy_hash`
    #: holds an Argon2 hash whose whole design is that it is computed once, so
    #: a run that resets its statistics must not also make it pay again.
    rebase: Callable[[], None] | None = None


_REGISTRY: dict[str, Registration] = {}


def register(name: str, *, clear: Callable[[], None],
             size: Callable[[], int] | None = None,
             note: str = "", holds_handle: bool = False) -> CacheStats:
    """Enrol a cache and hand back the counters it should bump.

    Called at import time by the module that owns the cache, so the register
    is complete for whatever this process has loaded and no more -- which is
    why the report says how many caches it saw.

    Re-registering a name replaces the entry rather than raising: a module
    reloaded under a test would otherwise leave a dead `clear` behind, pointed
    at a cache object nobody uses any more.
    """
    stats = CacheStats(name)
    _REGISTRY[name] = Registration(stats=stats, clear=clear, size=size,
                                   note=note, holds_handle=holds_handle)
    return stats


def register_lru(name: str, fn: _Cached, *, note: str = "") -> None:
    """Enrol a `functools.cache`/`lru_cache` function, which counts itself.

    Nothing to bump and nothing to hand back: `cache_info()` already holds the
    hits and misses, so the register reads them at report time. This is the
    one-line way in, and the reason a memoised helper has no excuse to sit
    outside the register.
    """
    zero = [0, 0]

    def _refresh(stats: CacheStats) -> None:
        info = fn.cache_info()
        stats.hits = info.hits - zero[0]
        stats.misses = info.misses - zero[1]

    def _rebase() -> None:
        info = fn.cache_info()
        zero[0], zero[1] = info.hits, info.misses

    _REGISTRY[name] = Registration(
        stats=CacheStats(name), clear=fn.cache_clear,
        size=lambda: int(fn.cache_info().currsize), note=note,
        refresh=_refresh, rebase=_rebase)


def registered() -> dict[str, Registration]:
    """Every cache the currently-imported modules declared."""
    return dict(_REGISTRY)


def clear_all() -> None:
    """Empty every registered cache. What `bench --cold` calls between samples.

    A cache whose `clear` raises does not stop the others: the caller wants a
    cold process, and one stubborn entry is better skipped than allowed to
    abort the sweep.
    """
    for reg in list(_REGISTRY.values()):
        try:
            reg.clear()
        except Exception:                                           # noqa: BLE001
            continue


def release_handles() -> None:
    """Let go of anything holding an operating-system resource.

    A narrower `clear_all` for callers that want the *handles* back without
    throwing away every memoised value. `tests/conftest.py` calls it after
    every test, and the reason is a real failure: `api/service.py`'s pool
    keeper holds a **read-only** DuckDB handle for a thirty-second lease, and
    DuckDB refuses a second connection to the same file with a different
    configuration -- so one test that loaded a page could make the next test's
    read-write `connect()` fail outright, in a different file, with an error
    about configuration. Deployed that cannot happen (the app never writes the
    pool) and across processes it degrades to waiting out the lease, which is
    what `_reap_keeper` argues about. In one process it is a hard error, and
    it arrives as an unrelated test failing.
    """
    for reg in list(_REGISTRY.values()):
        if not reg.holds_handle:
            continue
        try:
            reg.clear()
        except Exception:                                           # noqa: BLE001
            continue


def reset_stats() -> None:
    """Zero every counter without emptying anything. A warm run starts here."""
    for reg in _REGISTRY.values():
        if reg.rebase is not None:
            reg.rebase()
        reg.stats.reset()


@dataclass(frozen=True)
class Row:
    """One line of the hit-rate report."""

    name: str
    hits: int
    misses: int
    rate: float | None
    size: int | None
    note: str


def report() -> list[Row]:
    """The hit-rate table, worst first -- so the finding is the top line.

    A cache nobody asked sorts above everything, because "this never ran"
    is a louder result than any percentage.
    """
    rows = []
    for name, reg in _REGISTRY.items():
        if reg.refresh is not None:
            reg.refresh(reg.stats)
        rows.append(Row(name=name, hits=reg.stats.hits,
                        misses=reg.stats.misses, rate=reg.stats.rate,
                        size=_size_of(reg), note=reg.note))
    return sorted(rows, key=lambda r: (r.rate is not None, r.rate or 0.0,
                                       r.name))


def _size_of(reg: Registration) -> int | None:
    if reg.size is None:
        return None
    try:
        return reg.size()
    except Exception:                                               # noqa: BLE001
        return None


#: Module-level mutable state that is deliberately **not** a cache, so the
#: sweep in `tests/test_caches.py` tells the two apart by name rather than by
#: guessing from shape. Every entry is a decision and the reason is the point
#: of it: state that is neither listed here nor registered above is drift, and
#: the sweep fails on it the day it appears.
NOT_CACHES: dict[str, str] = {
    "mtglab.api.app.app":
        "the application object, built once at import",
    "mtglab.api.jobs._JOBS":
        "the job table -- state a client polls, not a memo of an answer",
    "mtglab.api.traffic._BUFFER":
        "a write buffer flushed into app.db; clearing it LOSES visits, "
        "which is the opposite of what clearing a cache means",
    "mtglab.api.traffic._last_flush":
        "when that buffer last drained",
    "mtglab.api.service._KEEPER":
        "one held read-only handle, never queried -- a lease on DuckDB's "
        "loaded instance. Registered as `pool.keeper` for its stamp check, "
        "which is the part that hits or misses",
    "mtglab.api.service._KEEPER_STAMP":
        "the keeper's file stamp",
    "mtglab.api.service._KEEPER_USED":
        "when the keeper was last wanted, for the idle lease",
    "mtglab.api.service._REAPER":
        "the thread that reaps the keeper",
    "mtglab.cards.db._QUERY_PROBE":
        "the bench's statement timer -- a hook, and None in every process "
        "that is not measuring",
    "mtglab.cards.db._CONNECTION_POOL_PATH":
        "a WeakKeyDictionary remembering which file a handle opened; it "
        "answers a question about identity, never about content",
    "mtglab.caches._REGISTRY":
        "this register",
    "mtglab.config.DATA_DIR": "a configured path, rebound by use_paths()",
    "mtglab.config.DB_PATH": "a configured path, rebound by use_paths()",
    "mtglab.config.DECKS_DIR": "a configured path, rebound by use_paths()",
    "mtglab.config.APP_DB_PATH": "a configured path, rebound by use_paths()",
    "mtglab.config.SCRYFALL_DIR": "a configured path, rebound by use_paths()",
    "mtglab.ocr._refused":
        "a negative set -- files this process failed to fetch, so it stops "
        "asking. Emptying it would restore the retry it exists to prevent",
    "mtglab.symbols._missing":
        "the same negative set, one shelf over",
    "mtglab.sim.cache._fingerprint_done":
        "the latch beside `sim.fingerprint`, which is the registered half",
}
