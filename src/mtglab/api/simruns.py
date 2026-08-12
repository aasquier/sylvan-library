"""Simulation jobs, shaped for the UI.

These reuse the exact compilation path the CLI uses (`sim.compile`), so a
number shown in the app is the same number `mtglab sim mana` prints. Every
result carries its own caveat text, because Tier 1 answers mana questions
only and a chart with no caption invites over-reading.

**Results are cached** (`sim/cache.py`), which shows up here in three places.

*Every run is seeded.* An absent seed used to mean `random.Random(None)`, and
the app never sent one -- so the numbers on the deck everybody looks at were a
fresh sample every time, reproducible by nobody and cacheable not at all. It
now resolves to `DEFAULT_SEED`, the same 7 the land sweep has always used, and
the seed is reported in the result so which sample you are looking at is a fact
on the page rather than an assumption. Asking for a different sample is an
explicit act: send another seed.

*Planning happens in the request, running happens in the job.* The key is a
hash of the compiled deck, so knowing whether this is a hit means compiling
first -- a deck parse and one indexed corpus query, milliseconds against the
eighteen seconds it saves. `plan_mana` and `plan_lands` do that and hand back
either a finished result or the closure that would produce one. On a miss the
compiled cards are passed *into* the closure, so nothing is compiled twice.

*A planning failure is not a new failure mode.* Compiling in the request means
a missing deck or an absent corpus now fails earlier than it used to, and it
must not fail *differently*: the exception is carried into the job and thrown
from the worker, so the caller still gets a 200 and then a job in state
`error`, which is the shape the UI already knows. An optimisation that turns a
reported error into a different kind of reported error is an optimisation that
broke something.

The land sweep caches **per land count**, not per sweep. Each count is an
independent seeded `run()` over a deck derived deterministically from the same
compiled cards, so the rows compose; a sweep of 30-40 followed by one of 32-42
reuses nine of its eleven rows, where a whole-sweep key would have reused none.
A partial hit is still a job, and simulates only the counts it is missing.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from mtglab.api.service import _connect, _corpus_for, _source
from mtglab.decks.model import Deck
from mtglab.decks.source import DeckSource
from mtglab.sim import cache
from mtglab.sim.compile import compile_deck
from mtglab.sim.tier1.engine import KeepRule, SimCard, run, simulate_game

TIER1_CAVEAT = (
    "Tier 1 shuffles, draws and pays costs. It does not model opponents, "
    "interaction, tutors, cost reduction, or card text beyond mana production."
)

LAND_SWEEP_CAVEAT = (
    "Read 'spells through T8', not commander speed: commander speed rises "
    "monotonically with land count, so optimising it alone always recommends "
    "more lands. Resizing cycles the existing land pool, preserving the colour "
    "mix but not specific utility lands."
)

#: The seed a run gets when the caller does not choose one. Seven, because that
#: is what `sweep_land_counts` has always defaulted to and two different
#: "unspecified" seeds in one module would be a trap. See the module docstring
#: for why an unseeded default was worth replacing.
DEFAULT_SEED = 7

Progress = Callable[[int, int], None]


def _compile(slug: str, *, source: DeckSource | None = None
             ) -> tuple[Deck, list[SimCard], SimCard | None]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        cards = _corpus_for(deck, con)
    finally:
        if con is not None:
            con.close()
    if not cards:
        raise RuntimeError(
            "simulation needs the card corpus -- run `mtglab data refresh`")
    library, commander = compile_deck(deck, cards)
    return deck, library, commander


def _keep_rule(payload: dict[str, Any]) -> KeepRule:
    return KeepRule(
        min_lands=int(payload.get("min_lands", 2)),
        max_lands=int(payload.get("max_lands", 5)),
        min_mana_pieces=int(payload.get("min_pieces", 3)),
    )


def _seed(payload: dict[str, Any]) -> int:
    """The seed for this run, always a real number.

    Clamped to nothing and validated barely: any integer is a valid seed, and
    an unparseable one is a caller error rather than something to paper over.
    """
    raw = payload.get("seed")
    if raw in (None, ""):
        return DEFAULT_SEED
    return int(raw)


@dataclass
class Plan:
    """What a sim request turns into: an answer, or the work to produce one.

    `result` is set when the cache already held it, in which case `run` is
    never called. Exactly one of them is used, and the route decides whether
    that becomes a finished job or a queued one.
    """

    kind: str
    label: str
    result: dict[str, Any] | None
    run: Callable[[Progress], dict[str, Any]]


def _deferred_failure(exc: BaseException) -> Callable[[Progress], dict[str, Any]]:
    """Re-raise a planning failure inside the job it would have been.

    Compilation moved into the request when the cache arrived, so a deck that
    is missing, has no corpus or has no lands now fails *earlier* than it used
    to. It must not fail differently: before the cache, that error reached the
    caller as a job in state `error`, and the UI knows how to show one. So the
    exception is caught, carried, and thrown again from the worker -- same
    type, same message, same traceback, same 200-then-error shape.

    Carrying the exception rather than re-running the failing compile inside
    the job is deliberate. An earlier draft re-ran it, which meant `plan_lands`
    called `run_lands` and `run_lands` called `plan_lands`: mutual recursion on
    exactly the input that cannot work, reported as a RecursionError instead of
    "no such deck".
    """
    def fail(_progress: Progress) -> dict[str, Any]:
        raise exc
    return fail


def _stamp(result: dict[str, Any], *, cached: bool,
           computed_at: str | None = None) -> dict[str, Any]:
    """Say whether this number was computed just now or read back.

    Not cosmetic. A figure served from a previous run is honest only if it says
    so -- the same reason every Tier 1 result carries its caveat.
    """
    out = dict(result)
    out["cached"] = cached
    out["computed_at"] = computed_at
    return out


# --------------------------------------------------------------------- mana

def _mana_params(payload: dict[str, Any]) -> tuple[int, int, KeepRule, int]:
    """The clamped parameters, resolved once.

    Clamping *before* the key is computed matters: `games=10**9` and
    `games=200000` run the identical simulation, and keying on the raw request
    would store that result twice under different names.
    """
    games = max(100, min(int(payload.get("games", 20_000)), 200_000))
    turns = max(4, min(int(payload.get("turns", 12)), 20))
    return games, turns, _keep_rule(payload), _seed(payload)


def _mana_result(slug: str, deck: Deck, library: list[SimCard],
                 commander: SimCard | None, games: int, turns: int,
                 keep: KeepRule, seed: int, progress: Progress) -> dict[str, Any]:
    summary = run(library, commander, games=games, turns=turns,
                  keep_rule=keep, seed=seed, progress=progress)
    return {
        "slug": slug,
        "deck_name": deck.name,
        "games": games,
        "turns": turns,
        "seed": seed,
        "mulligan_rate": summary.mulligan_rate,
        "avg_mulligans": summary.avg_mulligans,
        "median_commander_turn": summary.median_commander_turn,
        "never_cast_commander": summary.never_cast_commander,
        "color_screw_rate": summary.color_screw_rate,
        "by_turn": [
            {
                "turn": t + 1,
                "lands": round(summary.avg_lands_by_turn[t], 2),
                "mana": round(summary.avg_mana_by_turn[t], 2),
                "unused": round(summary.avg_unused_by_turn[t], 2),
                "spells": round(summary.avg_spells_by_turn[t], 2),
                "commander_down": summary.commander_by_turn.get(t + 1, 0.0),
            }
            for t in range(turns)
        ],
        "caveat": TIER1_CAVEAT,
    }


def _mana_key(library: list[SimCard], commander: SimCard | None,
              games: int, turns: int, keep: KeepRule, seed: int) -> str | None:
    return cache.key("sim.mana", library=library, commander=commander,
                     games=games, turns=turns, keep_rule=keep, seed=seed)


def plan_mana(slug: str, payload: dict[str, Any],
              *, source: DeckSource | None = None) -> Plan:
    """Decide whether this mana run is already answered."""
    games, turns, keep, seed = _mana_params(payload)
    label = f"{slug}: mana, {games:,} games"

    try:
        deck, library, commander = _compile(slug, source=source)
    except Exception as exc:                                        # noqa: BLE001
        # Planning is an optimisation, and a deck that cannot compile has to
        # fail the way it failed before this existed: inside the job, reported
        # as a job error rather than as an exception out of the route.
        return Plan("sim.mana", label, None, _deferred_failure(exc))

    key = _mana_key(library, commander, games, turns, keep, seed)
    hit = cache.get(key)
    if hit is not None:
        answer = _stamp(hit.result, cached=True, computed_at=hit.created_at)
        return Plan("sim.mana", label, answer, lambda _progress: answer)

    def compute(progress: Progress) -> dict[str, Any]:
        result = _mana_result(slug, deck, library, commander, games, turns,
                              keep, seed, progress)
        cache.put(key, "sim.mana", result)
        return _stamp(result, cached=False)

    return Plan("sim.mana", label, None, compute)


# -------------------------------------------------------------------- lands

def _land_params(payload: dict[str, Any]) -> tuple[list[int], int, int, KeepRule, int]:
    low = max(20, int(payload.get("low", 30)))
    high = min(60, int(payload.get("high", 40)))
    if high < low:
        low, high = high, low
    games = max(100, min(int(payload.get("games", 5_000)), 100_000))
    turns = max(8, min(int(payload.get("turns", 10)), 20))
    return list(range(low, high + 1)), games, turns, _keep_rule(payload), _seed(payload)


def _resize(library: list[SimCard], count: int) -> list[SimCard]:
    """The deck at `count` lands, by cycling the existing pool.

    That preserves the colour mix, which is what the sweep is measuring, and
    holds the deck at 99 so mulligan rates stay comparable across counts. It is
    a pure function of the compiled library and the count, which is what makes
    a per-count cache key sound.
    """
    lands = [c for c in library if c.is_land]
    spells = [c for c in library if not c.is_land]
    if not lands:
        raise RuntimeError("deck has no lands to sweep")
    pool = [lands[i % len(lands)] for i in range(count)]
    return pool + spells[: max(0, 99 - count)]


def _land_row(count: int, deck_cards: list[SimCard], commander: SimCard | None,
              games: int, turns: int, keep: KeepRule, seed: int,
              progress: Progress) -> dict[str, Any]:
    summary = run(deck_cards, commander, games=games, turns=turns,
                  keep_rule=keep, seed=seed, progress=progress)
    return {
        "lands": count,
        "commander_by_t5": summary.commander_by_turn.get(5, 0.0),
        "spells_through_t8": round(summary.spells_through(8), 2),
        "wasted_through_t8": round(summary.wasted_through(8), 2),
        "mulligan_rate": summary.mulligan_rate,
    }


def _land_summary(slug: str, deck: Deck, rows: list[dict[str, Any]], games: int,
                  seed: int) -> dict[str, Any]:
    # Report the spread so a flat curve is visible as flat rather than being
    # read as a peak. The Gyome sweep was flat to 0.07 across 30-40, and
    # picking the argmax of that would have been reading noise.
    spread = (max(r["spells_through_t8"] for r in rows)
              - min(r["spells_through_t8"] for r in rows))
    best = max(rows, key=lambda r: r["spells_through_t8"])
    return {
        "slug": slug,
        "deck_name": deck.name,
        "games": games,
        "seed": seed,
        "rows": rows,
        "deployment_spread": round(spread, 2),
        "argmax_lands": best["lands"],
        "flat": spread < 0.25,
        "caveat": LAND_SWEEP_CAVEAT,
    }


def _land_keys(resized: dict[int, list[SimCard]], commander: SimCard | None,
               games: int, turns: int, keep: KeepRule,
               seed: int) -> dict[int, str | None]:
    return {
        count: cache.key("sim.lands.count", library=cards, commander=commander,
                         games=games, turns=turns, keep_rule=keep, seed=seed)
        for count, cards in resized.items()
    }


def _sweep(slug: str, deck: Deck, commander: SimCard | None,
           resized: dict[int, list[SimCard]], games: int, turns: int,
           keep: KeepRule, seed: int, progress: Progress) -> dict[str, Any]:
    """Run the counts that are not cached, reuse the ones that are.

    The cache is consulted here as well as in `plan_lands`, which costs one
    extra SELECT per count. That is not redundancy: the plan runs in the
    request and this runs in the worker, minutes apart on a queued job, and in
    between another request can have filled in the counts this one was going to
    simulate. Re-reading means the work is skipped rather than repeated.
    """
    counts = sorted(resized)
    keys = _land_keys(resized, commander, games, turns, keep, seed)
    hits = {count: cache.get(keys[count]) for count in counts}
    missing = [count for count in counts if hits[count] is None]

    # Progress counts only the counts actually being simulated, so a sweep that
    # is nine-elevenths cached shows a bar that finishes in the time the
    # remaining two take rather than sitting at 82% from the first tick.
    total_steps = max(1, len(missing)) * games
    rows: list[dict[str, Any]] = []
    simulated = 0
    for count in counts:
        hit = hits[count]
        if hit is not None:
            rows.append(hit.result)
            continue

        def step(done: int, _total: int, _base: int = simulated * games) -> None:
            progress(_base + done, total_steps)

        row = _land_row(count, resized[count], commander, games, turns, keep,
                        seed, step)
        cache.put(keys[count], "sim.lands.count", row)
        rows.append(row)
        simulated += 1

    progress(total_steps, total_steps)
    return _stamp(_land_summary(slug, deck, rows, games, seed), cached=False)


def plan_lands(slug: str, payload: dict[str, Any],
               *, source: DeckSource | None = None) -> Plan:
    """Decide which land counts still need simulating, if any."""
    counts, games, turns, keep, seed = _land_params(payload)
    label = f"{slug}: land sweep {counts[0]}-{counts[-1]}"

    try:
        deck, library, commander = _compile(slug, source=source)
        resized = {count: _resize(library, count) for count in counts}
    except Exception as exc:                                        # noqa: BLE001
        # As in `plan_mana`, plus one failure of its own: `_resize` refuses a
        # deck with no lands to sweep, and that has to reach the caller the
        # same way a missing deck does.
        return Plan("sim.lands", label, None, _deferred_failure(exc))

    keys = _land_keys(resized, commander, games, turns, keep, seed)
    hits = {count: cache.get(keys[count]) for count in counts}
    if all(hit is not None for hit in hits.values()):
        # Ordered by `counts` rather than by whatever the lookups returned:
        # the rows are a curve, and a sweep whose rows are out of order is a
        # chart that lies.
        found = [hits[count] for count in counts]
        answer = _stamp(
            _land_summary(slug, deck,
                          [hit.result for hit in found if hit is not None],
                          games, seed),
            cached=True,
            computed_at=min(hit.created_at for hit in found if hit is not None))
        return Plan("sim.lands", label, answer, lambda _progress: answer)

    return Plan("sim.lands", label, None,
                lambda progress: _sweep(slug, deck, commander, resized, games,
                                        turns, keep, seed, progress))


def goldfish_one(slug: str, seed: int | None = None,
                 *, source: DeckSource | None = None) -> dict[str, Any]:
    """A single game, for the playtest screen. Cheap enough to run inline.

    Deliberately uncached: one game is a millisecond, and the screen's whole
    purpose is a fresh draw each time somebody presses the button.
    """
    import random
    _deck, library, commander = _compile(slug, source=source)
    res = simulate_game(library, commander, turns=12,
                        rng=random.Random(seed))
    return {
        "commander_turn": res.commander_turn,
        "mulligans": res.mulligans,
        "lands_by_turn": res.lands_by_turn,
        "mana_by_turn": res.mana_by_turn,
        "spells_by_turn": res.spells_by_turn,
    }
