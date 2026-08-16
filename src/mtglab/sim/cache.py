"""Memoised Tier 1 results, keyed on the simulation's own input.

`docs/ENGINEERING.md` §1 and `docs/HOSTING.md` §3 both reach the same
conclusion from different directions: a 20,000-game `sim mana` is ~18 CPU
seconds, deck files change rarely, and everybody views the same numbers
repeatedly, so the common path should cost nothing. That is a bigger win than
making the cold path thirty times faster, and it needs no second language.

The whole design turns on one requirement, and it is the one CLAUDE.md is
written against: **a cached number must never be a stale number.** Everything
below is in service of that.

## The key is the input, not a name for the input

`run()` takes exactly `(library, commander, games, turns, keep_rule, seed)` and
reads nothing else. `library` and `commander` come out of `compile_deck`, which
reads `mana_cost`, `type_line`, `oracle_text` and `produced_mana` **from the
pool**. So the obvious key -- a hash of `deck.yaml` -- is not sufficient: a
`data refresh` can change what a card does while the deck file is byte
identical. That is not hypothetical. Scryfall retemplated "enters the
battlefield tapped" to "enters tapped", and `sim/compile.py` carries the
comment about it, because matching only the old wording silently treated every
modern tapland as untapped and overstated early mana for every deck. A
deck-hash cache would have served the pre-refresh numbers forever.

So the key is a hash of the **compiled** deck: the `SimCard` list the engine is
actually handed. That is exact in both directions.

- A card's pool facts change in a way that matters -> different SimCards ->
  miss, and the numbers get recomputed.
- A rationale is edited, or a comment is reflowed, or the pool is refreshed
  without touching anything this deck runs -> identical SimCards -> hit.

The cost is that a hit is not quite free: it needs a deck parse and one indexed
`get_cards` over ~100 names, a few milliseconds, instead of eighteen seconds.
That is the right trade. The alternative -- hashing the YAML text plus some
proxy for the pool, like the DuckDB file's mtime -- is cheaper on the hit
path and asks you to trust that the proxy never lies. This one does not have to
be trusted; it is the input.

## The engine is part of the input too

The compiled cards do not capture the *code* that consumes them. A change to
`engine.py` or to `mana.py` changes the answer while every other input stays
put, so both are fingerprinted by source and mixed into the key. Editing a
comment in either invalidates the cache, which costs one recomputation and buys
the property that no engine change can serve a pre-change number -- not even
one nobody remembered to declare. If the sources cannot be read at all
(a frozen or zipped install), `fingerprint()` returns `None` and **caching is
switched off** rather than falling back to something weaker.

`SIM_VERSION` sits alongside for changes those two files do not cover -- the
serialisation below, or a fix in `compile.py` that must invalidate results
compiled before it. `tests/test_sim_cache.py` pins it against the determinism
digest as a pair, so a deliberate change to Tier 1's output cannot be recorded
without this being looked at.

## Where it lives

`app.db`, the SQLite half of ADR 4, in the table `docs/HOSTING.md` §2 already
sketched. It is on the volume, so it survives a deploy exactly the way the
decks it is keyed to do, and it reuses one connect-and-migrate implementation
for one file. The import of `mtglab.auth.db` from inside `sim/` is that module
being the `app.db` helper rather than an authentication dependency -- its own
docstring says so -- and a second implementation of the pragmas and the
migration ladder for the same file would be worse than the import. Nothing
here imports FastAPI, DuckDB or anything outside the standard library, so the
dependency rule CLAUDE.md sets for `sim/` still holds.

Rows are derived data in a file that is otherwise irreplaceable, which is worth
one sentence: this table can be dropped at any moment without losing anything a
re-run cannot produce, `mtglab sim cache --clear` does exactly that, and the
rows are ~1-2 kB each against a bound of `MAX_ROWS`.

## What it deliberately does not do

**It never raises.** A cache is an optimisation, and an optimisation that can
turn a working simulation into a failed one is a bad trade. Every entry point
here swallows its own errors, logs, and behaves as a miss.

**It does not deduplicate concurrent work.** Two identical requests arriving
together both miss and the second recomputes. The job pool has one worker, so
they serialise rather than compete, and a lock across a job that runs for
thirty seconds would be a worse problem than the one it solves.

**It does not cache an unseeded run.** Callers guard on that rather than this
module doing it silently; see `api/simruns.py`, where an absent seed now
resolves to a default so that the common path is reproducible *and* cacheable.
"""

from __future__ import annotations

import hashlib
import json
import logging
import sqlite3
from dataclasses import asdict, dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from mtglab.sim.tier1.engine import KeepRule, SimCard

_LOG = logging.getLogger("mtglab.sim.cache")

#: Bumped when something changes what a stored result *means* and neither
#: `engine.py` nor `mana.py` moved -- the serialisation below, or a `compile.py`
#: fix that must invalidate cards compiled before it. Pinned against
#: `tests/test_determinism.REFERENCE_DIGEST` as a pair, so a deliberate change
#: to Tier 1's output forces a decision here rather than passing unnoticed.
SIM_VERSION = 2  # 2: GameResult/SimSummary grew the texture fields (2026-08-15 item 11)

#: How many rows to keep. A mana result is ~1.5 kB and a land-sweep row a few
#: hundred bytes, so this is a couple of megabytes at worst -- bounded because
#: an unbounded cache on a 3 GB volume is a slow-motion outage, not because the
#: rows are expensive.
MAX_ROWS = 2_000

#: Every parameter of `engine.run` that changes its output, and the one that
#: does not. `tests/test_sim_cache.py` checks this against the real signature,
#: so a new parameter fails the suite until somebody decides which list it
#: belongs in -- which is the difference between this staying correct and this
#: quietly serving pre-parameter numbers.
RUN_INPUTS = frozenset({"library", "commander", "games", "turns",
                        "keep_rule", "seed"})
RUN_NON_INPUTS = frozenset({"progress"})

# The modules whose *code* decides the answer. `compile.py` is deliberately
# absent: its output is the SimCards, which are hashed directly, so hashing its
# source as well would invalidate on changes that provably cannot matter.
_ENGINE_SOURCES = ("mtglab.sim.tier1.engine", "mtglab.mana")

_fingerprint_cache: str | None = None
_fingerprint_done = False


def fingerprint() -> str | None:
    """A hash of the code that turns SimCards into numbers.

    `None` means the sources could not be read, and a `None` fingerprint
    disables caching entirely. That is deliberate: the fallback for "I cannot
    tell which engine this is" must be to compute rather than to guess, because
    guessing here is the silent-wrong-answer failure this module exists to
    avoid.

    Computed once per process. The files cannot change under a running
    interpreter in any way that also changes the loaded code.
    """
    global _fingerprint_cache, _fingerprint_done
    if _fingerprint_done:
        return _fingerprint_cache
    _fingerprint_done = True
    digest = hashlib.sha256()
    try:
        import importlib
        for name in _ENGINE_SOURCES:
            module = importlib.import_module(name)
            source = getattr(module, "__file__", None)
            if not source:
                raise OSError(f"{name} has no source file")
            digest.update(Path(source).read_bytes())
    except Exception as exc:                                        # noqa: BLE001
        _LOG.warning("simulation cache disabled: cannot fingerprint the "
                     "engine (%s: %s)", type(exc).__name__, exc)
        _fingerprint_cache = None
        return None
    _fingerprint_cache = digest.hexdigest()
    return _fingerprint_cache


# ------------------------------------------------------------------- the key

def _card_form(card: SimCard) -> list[Any]:
    """One SimCard, as something JSON can serialise the same way every time.

    Sets are sorted on the way out. `frozenset` iteration order varies with
    `PYTHONHASHSEED`, so serialising one directly would produce a key that
    changes between processes -- the cache would miss constantly and nobody
    would notice it was broken rather than merely cold.

    Tuple order is *preserved*, in `pips` and in `produces`, because it is real
    input: `_consume` matches pips in order and the leftovers it returns depend
    on that order.

    `category` is carried even though the engine never reads it. Excluding a
    field is a claim about engine behaviour that can go stale; including one
    costs a handful of bytes.
    """
    return [
        card.name,
        card.category,
        card.cost.generic,
        [sorted(p) for p in card.cost.pips],
        [sorted(p) for p in card.cost.phyrexian],
        card.cost.has_x,
        card.is_land,
        card.enters_tapped,
        [[sorted(s.colors), s.amount] for s in card.produces],
        card.produce_delay,
        card.fetches_lands,
    ]


def key(kind: str, *, library: list[SimCard], commander: SimCard | None,
        games: int, turns: int, keep_rule: KeepRule, seed: int) -> str | None:
    """The cache key for one `run()`, or `None` if caching is unavailable.

    Every argument here is an argument of `run`, which is the property
    `RUN_INPUTS` exists to keep true. `keep_rule` goes in through `asdict`
    rather than field by field, so a new mulligan lever is in the key the day
    it is added instead of the day somebody remembers it.
    """
    engine = fingerprint()
    if engine is None:
        return None
    payload = {
        "version": SIM_VERSION,
        "engine": engine,
        "kind": kind,
        "library": [_card_form(c) for c in library],
        "commander": _card_form(commander) if commander is not None else None,
        "games": games,
        "turns": turns,
        "keep_rule": asdict(keep_rule),
        "seed": seed,
    }
    blob = json.dumps(payload, sort_keys=True, separators=(",", ":"),
                      ensure_ascii=True)
    return hashlib.sha256(blob.encode("utf-8")).hexdigest()


# ------------------------------------------------------------------- storage

@dataclass(frozen=True)
class Hit:
    """A stored result, and when it was computed.

    The timestamp is not decoration: a cached figure that cannot say how old it
    is cannot be reported honestly, and the app shows it.
    """

    result: dict[str, Any]
    created_at: str


def _now() -> str:
    return datetime.now(UTC).isoformat()


def get(cache_key: str | None, *, path: Path | str | None = None) -> Hit | None:
    """The stored result for `cache_key`, or `None` for a miss.

    A `None` key is a miss without touching the database, which is what makes
    "caching is off" a one-line concern for callers.
    """
    if not cache_key:
        return None
    try:
        from mtglab.auth import db
        with db.connection(path) as con:
            row = con.execute(
                "SELECT result_json, created_at FROM sim_cache WHERE key = ?",
                (cache_key,)).fetchone()
            if row is None:
                return None
            # Touched on read so eviction is least-recently-*used* rather than
            # oldest-computed. The numbers everybody looks at are exactly the
            # ones worth keeping.
            con.execute("UPDATE sim_cache SET last_used_at = ? WHERE key = ?",
                        (_now(), cache_key))
            con.commit()
            return Hit(json.loads(row["result_json"]), row["created_at"])
    except (sqlite3.Error, OSError, ValueError) as exc:
        _LOG.warning("simulation cache read failed (%s: %s)",
                     type(exc).__name__, exc)
        return None


def put(cache_key: str | None, kind: str, result: dict[str, Any],
        *, path: Path | str | None = None) -> None:
    """Store `result`, evicting the least recently used rows if over `MAX_ROWS`.

    Never raises. A cache that can fail a simulation is worse than no cache.
    """
    if not cache_key:
        return
    try:
        blob = json.dumps(result, separators=(",", ":"))
    except (TypeError, ValueError) as exc:
        _LOG.warning("simulation result is not storable (%s: %s)",
                     type(exc).__name__, exc)
        return
    try:
        from mtglab.auth import db
        now = _now()
        with db.connection(path) as con, con:
            con.execute(
                "INSERT INTO sim_cache (key, kind, result_json, created_at,"
                " last_used_at) VALUES (?, ?, ?, ?, ?) "
                "ON CONFLICT(key) DO UPDATE SET "
                "result_json = excluded.result_json, "
                "created_at = excluded.created_at, "
                "last_used_at = excluded.last_used_at",
                (cache_key, kind, blob, now, now))
            total = con.execute("SELECT count(*) FROM sim_cache").fetchone()[0]
            if total > MAX_ROWS:
                con.execute(
                    "DELETE FROM sim_cache WHERE key IN ("
                    "SELECT key FROM sim_cache ORDER BY last_used_at LIMIT ?)",
                    (total - MAX_ROWS,))
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("simulation cache write failed (%s: %s)",
                     type(exc).__name__, exc)


def stats(*, path: Path | str | None = None) -> dict[str, Any]:
    """What is in there, for `mtglab sim cache`.

    Reports whether caching is even switched on, because "the cache is empty"
    and "the cache is disabled" look identical from a row count and want
    completely different responses.
    """
    out: dict[str, Any] = {"enabled": fingerprint() is not None,
                           "rows": 0, "by_kind": {}, "oldest": None,
                           "newest": None, "bytes": 0}
    try:
        from mtglab.auth import db
        with db.connection(path) as con:
            rows = con.execute(
                "SELECT kind, count(*) AS n, sum(length(result_json)) AS b, "
                "min(created_at) AS oldest, max(created_at) AS newest "
                "FROM sim_cache GROUP BY kind ORDER BY kind").fetchall()
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("simulation cache stats failed (%s: %s)",
                     type(exc).__name__, exc)
        return out
    for row in rows:
        out["by_kind"][row["kind"]] = row["n"]
        out["rows"] += row["n"]
        out["bytes"] += row["b"] or 0
        for field, pick in (("oldest", min), ("newest", max)):
            current = out[field]
            out[field] = row[field] if current is None else pick(current, row[field])
    return out


def clear(*, path: Path | str | None = None) -> int:
    """Drop every row. Returns how many went."""
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            total = con.execute("SELECT count(*) FROM sim_cache").fetchone()[0]
            con.execute("DELETE FROM sim_cache")
        return int(total)
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("simulation cache clear failed (%s: %s)",
                     type(exc).__name__, exc)
        return 0
