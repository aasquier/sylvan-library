"""Where a slow call actually spends its time, measured three ways.

**One millisecond figure cannot route a finding**, and both failures of
2026-08-19 prove it from opposite directions. The card search was 80ms with
almost no Python running: the query itself was the cost, and no loop in this
repo could have been rewritten to help. The deck shelf was 200ms of which
162ms was failed `import pandas` -- Python, in a codebase that imports nothing
at request time, coming from DuckDB probing once per bound parameter. A
checklist that says "record the response time" records 80 and 200 and cannot
tell you that those are two entirely different investigations.

So a profile here reports three budgets rather than one:

* **database** -- exact, from the probe in `cards/db.py`, because cProfile
  raises no event for an extension method and folds its time into the caller.
  Its statement **count** rides along: an n+1 is a number here, not a pattern
  to grep for.
* **everything else** -- wall minus database. A subtraction, but a legitimate
  one: both halves are measured on the same unprofiled run, so nothing here
  is inferred from a profiler's inflated clock. On the deck shelf it reads
  16.6ms, which is the YAML and aggregation the ledger says is left.
* **imports** -- a *count* of calls into `importlib`, and their share of the
  traced run. The count is the honest half: reproducing the pandas storm on
  this suite moves the share from 0.1% to 4.9%, which is easy to read past,
  and the call count from a handful to thousands, which is not, and a count
  cannot be deflated by a fast machine the way a percentage can. **Read it
  against `IMPORT_CALLS_SUSPECT`, never against zero** -- this paragraph said
  "a warm request that imports anything at all is a bug" until 2026-08-19, and
  the number beside it has never once been zero.

One number is deliberately **not** reported as milliseconds: cProfile's own
totals. Its overhead is charged per call, so a target making many cheap calls
is inflated wildly -- the deck shelf profiles at 188ms against a 19ms wall.
The frame table below is a **ranking**, trustworthy for *which* line is
hottest and worthless as a budget. The first draft of this module printed
those totals next to the real ones; that would have been the misattribution
this whole tool exists to prevent, committed by the tool itself.
"""

from __future__ import annotations

import cProfile
import pstats
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from mtglab.cards import db

#: Calls into `importlib` above which a target is asking a question rather
#: than reporting a datum. Measured rather than picked: the warm suite runs
#: 0-31, a cold one ~900, and the pandas storm 92,057. So 200 separates
#: warm-healthy from anything worth reading, and the storm clears it 400x over.
#:
#: **What the cold ~900 is, since this line used to guess.** It said "a fresh
#: DuckDB connect does real path work", which is wrong and was checkable: cold
#: `db.oracle_columns` connects and reports **1** call. Traced 2026-08-19 by
#: recording every `__import__` during a cold `/api/decks`: 912 calls, 892 of
#: them `import pandas` from `db.py`'s parameter binding -- the same probe #181
#: acted on, still asked twice per bound value and now answered by the
#: `sys.modules` sentinel instead of by a walk of `sys.path`. Timed: 892
#: defused probes cost **0.96ms**, against 86.7ms undefused. So a cold figure
#: near 900 on a pool-reading target is the storm *staying* defused, and the
#: number to be alarmed by is a warm one.
IMPORT_CALLS_SUSPECT = 200


@dataclass(frozen=True)
class Frame:
    """One row of the profile: where cProfile says the time went."""

    where: str
    tottime: float
    cumtime: float
    calls: int
    native: bool


@dataclass
class QueryLog:
    """Every statement one run of a target sent to the pool."""

    count: int = 0
    total_s: float = 0.0
    slowest_s: float = 0.0
    slowest_sql: str = ""
    #: Statements seen more than once, and how often. An n+1 is the top row.
    repeats: dict[str, int] = field(default_factory=dict)

    def record(self, sql: str, seconds: float) -> None:
        self.count += 1
        self.total_s += seconds
        if seconds > self.slowest_s:
            self.slowest_s, self.slowest_sql = seconds, _one_line(sql)
        key = _one_line(sql)[:120]
        self.repeats[key] = self.repeats.get(key, 0) + 1

    def worst_repeat(self) -> tuple[str, int] | None:
        """The statement run most often, when that is more than once.

        `get_cards` binding 441 parameters was one statement; a genuine n+1 is
        441 statements. This is what tells them apart without reading any code.
        """
        if not self.repeats:
            return None
        sql, n = max(self.repeats.items(), key=lambda kv: kv[1])
        return (sql, n) if n > 1 else None


@dataclass(frozen=True)
class Profile:
    """One target, measured. Every field is a separate budget."""

    name: str
    wall_s: float
    #: Exact, from the query probe -- never `wall` minus something.
    db_s: float
    queries: QueryLog
    #: Calls into `importlib` per run of the target, judged against
    #: `IMPORT_CALLS_SUSPECT` and **never against zero**.
    #:
    #: This line used to say zero was the only right answer, which is not
    #: reachable and never was. #181 did not remove DuckDB's per-parameter
    #: `import pandas` probe -- it *answered* it, with a `None` sentinel in
    #: `sys.modules` -- so the probe still enters the import machinery and
    #: still counts, it just no longer walks `sys.path`. Measured 2026-08-19:
    #: a fully-defused three-parameter bind reports exactly **6**, two per
    #: bound value, deterministically; the warm suite runs 7-31 across its
    #: twelve targets, a cold one ~900, and the storm 92,057.
    #:
    #: The distinction is the whole point. A run holding the old sentence
    #: would file a false finding on every single warm profile, which is how
    #: an instrument gets ignored -- and an instrument nobody reads is worse
    #: than an absent one, because its numbers are still in the ledger.
    import_calls: int
    #: Their share of the traced run. Read second -- see the module docstring
    #: on why the count is the sturdier of the two.
    import_share: float
    #: A ranking, never a budget. See the module docstring.
    frames: list[Frame]

    @property
    def other_s(self) -> float:
        """Wall minus database: this repo's own code, plus serialisation."""
        return max(self.wall_s - self.db_s, 0.0)

    @property
    def db_share(self) -> float:
        return self.db_s / self.wall_s if self.wall_s else 0.0

    def verdict(self) -> str:
        """One sentence routing the finding, because two numbers get read as
        two numbers and a run needs to know which lever to reach for."""
        if self.wall_s <= 0:
            return "too fast to attribute"
        if self.import_calls > IMPORT_CALLS_SUSPECT:
            return (f"**{self.import_calls} calls into the import machinery** "
                    f"({self.import_share:.1%} of the traced run) — a served "
                    f"request should import nothing at all; find what asks")
        if self.db_share > 0.60:
            worst = self.queries.worst_repeat()
            if worst is not None and worst[1] >= 20:
                return (f"database-bound, and one statement ran {worst[1]} "
                        f"times — an n+1, not a slow query")
            return ("database-bound — the lever is the query or an index, "
                    "not the Python around it")
        if self.queries.count == 0:
            return ("no pool involved at all — every millisecond is this "
                    "repo's own code, and the frame table ranks it")
        return (f"mixed: {self.db_share:.0%} database, the rest this repo's "
                f"— read both before choosing a lever")


def profile_target(name: str, call: Callable[[], Any], *, repeat: int = 1,
                   top: int = 15) -> Profile:
    """Measure `call` three ways, then hand back all three.

    Timed unprofiled and profiled separately on purpose: cProfile's overhead
    is charged per call, so a target making a million cheap calls is inflated
    far more than one making a thousand expensive ones. Reading the profiled
    total as the wall clock would blame Python for the profiler.
    """
    call()                                   # warm the imports, not the caches

    log = QueryLog()
    db.set_query_probe(log.record)
    try:
        start = time.perf_counter()
        for _ in range(repeat):
            call()
        wall = (time.perf_counter() - start) / repeat
    finally:
        db.set_query_probe(None)

    prof = cProfile.Profile()
    prof.enable()
    for _ in range(repeat):
        call()
    prof.disable()

    frames, traced_s, import_s, import_calls = [], 0.0, 0.0, 0
    for key, value in pstats.Stats(prof).stats.items():              # type: ignore[attr-defined]
        filename, lineno, funcname = key
        calls, tottime, cumtime = value[0], value[2], value[3]
        # cProfile writes `~` for anything with no Python source: builtins,
        # extension methods, and the interpreter's own entry points.
        is_native = filename == "~"
        traced_s += tottime
        if not is_native and "importlib" in filename:
            import_s += tottime
            import_calls += calls
        frames.append(Frame(
            where=f"{_short(filename)}:{lineno}({funcname})",
            tottime=tottime / repeat, cumtime=cumtime / repeat,
            calls=calls, native=is_native))

    frames.sort(key=lambda f: f.tottime, reverse=True)
    return Profile(
        name=name, wall_s=wall, db_s=log.total_s / repeat, queries=log,
        import_calls=import_calls // repeat,
        import_share=import_s / traced_s if traced_s else 0.0,
        frames=frames[:top])


def _one_line(sql: str) -> str:
    return " ".join(sql.split())


def _short(filename: str) -> str:
    """Enough path to identify the file, not enough to wrap the table."""
    if filename == "~":
        return "<native>"
    parts = filename.replace("\\", "/").split("/")
    for marker in ("mtglab", "site-packages"):
        if marker in parts:
            return "/".join(parts[parts.index(marker):])
    return "/".join(parts[-2:])
