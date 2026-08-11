"""Simulation jobs, shaped for the UI.

These reuse the exact compilation path the CLI uses (`cli._sim_cards`), so a
number shown in the app is the same number `mtglab sim mana` prints. Every
result carries its own caveat text, because Tier 1 answers mana questions
only and a chart with no caption invites over-reading.
"""

from __future__ import annotations

from collections.abc import Callable
from typing import Any

from mtglab.api.service import _connect, _corpus_for, _source
from mtglab.decks.source import DeckSource
from mtglab.sim.compile import compile_deck
from mtglab.sim.tier1.engine import KeepRule, run, simulate_game

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


def _compile(slug: str, *, source: DeckSource | None = None):
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


def run_mana(slug: str, payload: dict[str, Any],
             progress: Callable[[int, int], None],
             *, source: DeckSource | None = None) -> dict[str, Any]:
    deck, library, commander = _compile(slug, source=source)
    games = max(100, min(int(payload.get("games", 20_000)), 200_000))
    turns = max(4, min(int(payload.get("turns", 12)), 20))
    seed = payload.get("seed")

    summary = run(library, commander, games=games, turns=turns,
                  keep_rule=_keep_rule(payload),
                  seed=int(seed) if seed not in (None, "") else None,
                  progress=progress)

    return {
        "slug": slug,
        "deck_name": deck.name,
        "games": games,
        "turns": turns,
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


def run_lands(slug: str, payload: dict[str, Any],
              progress: Callable[[int, int], None],
              *, source: DeckSource | None = None) -> dict[str, Any]:
    """Sweep land counts, rebuilding the deck at each size.

    The engine's own `sweep_land_counts` takes a builder function; here the
    deck is fixed, so lands are added or removed by cycling the existing pool.
    That preserves the colour mix, which is what the sweep is measuring.
    """
    deck, library, commander = _compile(slug, source=source)
    low = max(20, int(payload.get("low", 30)))
    high = min(60, int(payload.get("high", 40)))
    if high < low:
        low, high = high, low
    games = max(100, min(int(payload.get("games", 5_000)), 100_000))
    turns = max(8, min(int(payload.get("turns", 10)), 20))
    keep = _keep_rule(payload)
    seed = payload.get("seed")
    seed = int(seed) if seed not in (None, "") else 7

    lands = [c for c in library if c.is_land]
    spells = [c for c in library if not c.is_land]
    if not lands:
        raise RuntimeError("deck has no lands to sweep")

    counts = list(range(low, high + 1))
    total_steps = len(counts) * games
    rows = []

    for index, count in enumerate(counts):
        pool = [lands[i % len(lands)] for i in range(count)]
        # Hold the deck at 99 so mulligan rates stay comparable across counts.
        trimmed = spells[: max(0, 99 - count)]
        deck_cards = pool + trimmed

        def step(done: int, _total: int, _i=index) -> None:
            progress(_i * games + done, total_steps)

        summary = run(deck_cards, commander, games=games, turns=turns,
                      keep_rule=keep, seed=seed, progress=step)

        rows.append({
            "lands": count,
            "commander_by_t5": summary.commander_by_turn.get(5, 0.0),
            "spells_through_t8": round(summary.spells_through(8), 2),
            "wasted_through_t8": round(summary.wasted_through(8), 2),
            "mulligan_rate": summary.mulligan_rate,
        })

    progress(total_steps, total_steps)

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
        "rows": rows,
        "deployment_spread": round(spread, 2),
        "argmax_lands": best["lands"],
        "flat": spread < 0.25,
        "caveat": LAND_SWEEP_CAVEAT,
    }


def goldfish_one(slug: str, seed: int | None = None,
                 *, source: DeckSource | None = None) -> dict[str, Any]:
    """A single game, for the playtest screen. Cheap enough to run inline."""
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
