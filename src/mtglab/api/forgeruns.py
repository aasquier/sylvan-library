"""Forge matches, shaped for the UI (ADR 35).

Tier 3 reaches the app the way every slow thing does — a job — with the
division `api/themeruns.py` argued and this module inherits deliberately:
**everything refusable is refused in the request**, and only the JVM run is
queued. A missing deck is a 404, a bad games count is a 422, an absent Forge
is a 503, and a deck Forge cannot fully play is a 422 that names the cards —
`check_coverage` reads a zip on the request thread and was designed to
(`sim/tier3/run.py` says so in as many words). What the job holds is exactly
the part with no fast answer: one subprocess, minutes long.

Unlike `simruns`, planning failures here are *not* deferred into the job.
That deferral exists over there for compatibility — those errors reached
clients as job-state `error` before the cache moved compilation into the
request, and changing the shape would have broken the UI. This surface is
new, so it gets the honest shape from day one: distinct refusals with status
codes, and a job that only ever fails for runtime reasons.

**The lane is `FORGE`, and one worker is load-bearing.** The thread sleeps in
`subprocess.run` while the JVM works, so `CPU` (it would block Tier 1 for
minutes) and `NET` (two JVMs at once race the shared `.dck` directory) are
both wrong. Serialising the lane also makes the dedup story simple: a second
identical request joins the live job via `Plan.key`, and a *different* match
queues honestly behind the first.

Nothing is cached. Forge is seeded here the way Tier 1 is (same default, same
doctrine — an unseeded sample is not reproducible), but a cache needs a key
that names the engine's behaviour and `forge_home` can be upgraded under us;
until someone measures that a repeat ask is common, in-flight dedup is the
whole memory. Results carry the archetype caveat because CLAUDE.md requires
it quoted with the numbers, not near them.
"""

from __future__ import annotations

from statistics import median
from typing import Any

from mtglab.api.jobs import FORGE, Plan, Progress
from mtglab.api.simruns import DEFAULT_SEED
from mtglab.decks.model import Deck

FORGE_CAVEAT = (
    "Forge's AI is best with aggro and midrange, poor with control, and bad "
    "with most combo — read results per archetype, never as a single "
    "ranking. Games that hit the clock are reported apart from draws: a "
    "clock-out is the measurement giving up, not a game outcome.")

#: Forge's `-c`: seconds before a game is called a draw. 300 rather than
#: Forge's 120, because CLAUDE.md says so and a measured Trostani game ran
#: 134 seconds — a shorter clock turns real games into fake draws.
CLOCK = 300

GAMES_DEFAULT = 10
#: Twenty games of the slowest measured heads-up pairing is ~10 minutes of
#: wall clock. The lane is serial, so the cap is what keeps one enthusiastic
#: request from parking the Forge for an hour.
GAMES_MAX = 20


def status() -> dict[str, Any]:
    """Is the Forge reachable from this process? A fact about the environment.

    Two environments, one contract. With the hosted worker configured
    (ADR 35's second half), the answer is yes on configuration alone — no
    network, no machine woken to ask, exactly as `/api/claude` answers on the
    presence of a key rather than by calling Anthropic. Otherwise this probes
    the two things a local run needs — the distribution's jar and a JVM new
    enough — without booting either. `why` is maintainer-facing prose (it
    names paths and version floors); the client renders its own words, which
    is commandment 10 doing its usual work.
    """
    from mtglab.sim.tier3 import run as forge
    from mtglab.sim.tier3 import worker
    from mtglab.sim.tier3.coverage import ForgeNotInstalled
    if worker.configured():
        return {"available": True, "why": None}
    try:
        forge.desktop_jar()
        forge.java_binary()
    except ForgeNotInstalled as exc:
        return {"available": False, "why": str(exc)}
    return {"available": True, "why": None}


def _games(payload: dict[str, Any]) -> int:
    return max(1, min(int(payload.get("games", GAMES_DEFAULT)), GAMES_MAX))


def _seed(payload: dict[str, Any]) -> int:
    raw = payload.get("seed")
    if raw in (None, ""):
        return DEFAULT_SEED
    return int(raw)


def _shape(decks: list[Deck], addresses: list[str], games: int, seed: int,
           result: Any) -> dict[str, Any]:
    """The payload a match becomes. Medians and tails, never a mean alone.

    `wins` is counted per seat and reported per deck; real draws and
    clock-outs are separate columns because they are separate facts
    (`parse.GameResult` keeps them apart and this must not fold them back).
    """
    wins = {deck.slug: 0 for deck in decks}
    rows: list[dict[str, Any]] = []
    for game in result.games:
        slug = result.winner_slug(game)
        # A clocked-out game counts for nobody even when Forge printed a
        # winner line after the slow-match warning — `parse` attaches the
        # pending timeout to whatever result line follows, and a "win"
        # awarded because the other AI ran out of thinking time is the
        # measurement giving up wearing a trophy.
        if slug is not None and not game.timed_out:
            wins[slug] = wins.get(slug, 0) + 1
        rows.append({
            "game": game.index,
            "winner": None if game.timed_out else slug,
            "seconds": round(game.milliseconds / 1000, 1),
            "turns": game.turns,
            "draw": game.draw and not game.timed_out,
            "timed_out": game.timed_out,
        })
    seconds = sorted(r["seconds"] for r in rows)
    return {
        "decks": [
            {"slug": deck.slug, "name": deck.name, "address": address,
             "wins": wins.get(deck.slug, 0)}
            for deck, address in zip(decks, addresses, strict=True)
        ],
        "games": games,
        "played": len(rows),
        "draws": sum(1 for r in rows if r["draw"]),
        "timed_out": sum(1 for r in rows if r["timed_out"]),
        "median_seconds": round(median(seconds), 1) if seconds else None,
        "max_seconds": seconds[-1] if seconds else None,
        "startup_seconds": round(result.startup_seconds, 1),
        "wall_seconds": round(result.wall_seconds, 1),
        "clock": CLOCK,
        "seed": seed,
        "rows": rows,
        "caveat": FORGE_CAVEAT,
    }


def plan_forge(decks: list[Deck], addresses: list[str],
               payload: dict[str, Any]) -> Plan:
    """One heads-up match, planned. Refusals happened at the route already.

    `decks` arrive resolved because resolving them is the route's job — it
    holds the `DeckSource` per owner and the 404-versus-422 vocabulary. What
    this decides is the work: coverage has passed, so the closure is exactly
    one `run_games` call. `addresses` are the `owner/slug` pairs the client
    asked with, echoed back so the result can say whose decks played without
    the job inventing a second naming scheme.

    Heads-up only, and that is ADR 35 rather than a limitation to lift
    casually: measured on this hardware, 40% of four-player games hit the
    clock, and a mode whose results are mostly clock is not honest enough to
    ship. The CLI still plays pods for whoever wants to watch one.
    """
    games = _games(payload)
    seed = _seed(payload)
    label = (f"Forge: {' vs '.join(deck.slug for deck in decks)}, "
             f"{games} game{'s' if games != 1 else ''}")
    key = "forge|" + "|".join(addresses) + f"|{games}|{seed}"

    def compute(progress: Progress) -> dict[str, Any]:
        from mtglab.sim.tier3 import run as forge
        from mtglab.sim.tier3 import worker
        progress(0, games)

        # Forge's output is streamed, so the job ticks once per finished
        # game and the client's clock can be a bar that moves. Clamped
        # because a tick is a progress report, not a result — the shaped
        # answer below is the only tally anybody should quote.
        def tick(finished: int) -> None:
            progress(min(finished, games), games)

        # Same match, two places it can run (ADR 35): the worker when the
        # environment names one, a local subprocess otherwise. `run_match`
        # hands back a `SimRun` rebuilt from the wire and relays the same
        # per-game ticks, so `_shape` and the bar cannot tell the difference
        # — that is `wire.py`'s whole promise.
        if worker.configured():
            result: Any = worker.run_match(decks, games=games, clock=CLOCK,
                                           seed=seed, on_game=tick)
        else:
            result = forge.run_games(decks, games=games, clock=CLOCK,
                                     seed=seed, on_game=tick)
        progress(games, games)
        return _shape(decks, addresses, games, seed, result)

    return Plan("sim.forge", label, None, compute, lane=FORGE, key=key)
