"""The match ledger (ADR 36): every Forge match, recorded as it finishes.

The Simulator's next phase is measurement over accumulated games — rating
boards with honest uncertainty, a win-probability regression, game-length
survival curves, and eventually Tier 2's calibration anchor. All of it drinks
from one dataset, and this module is that dataset's only writer: `record` is
called from the two places a match finishes (the API job in
`api/forgeruns.py`, and `mtglab sim forge`), the same single-call-site
argument the deck activity log made in ADR 28. The worker machine never
records — its rootfs is ephemeral scratch and the app's `app.db` is the only
ledger there is; results cross the wire first and are recorded where they
land.

Three rules, inherited deliberately:

- **`record` never raises.** The match has already been played and paid for;
  a ledger failure is a logged warning, exactly as in `decks/log.py`,
  `sim/cache.py` and `claude/ledger.py`. Reading raises, for the opposite
  reason: an empty answer for a ledger that has rows would be worse than an
  error.
- **Games are stored as parsed, and readers apply the rules.** A clocked-out
  game keeps the winner line Forge printed after giving up; `wins` below is
  the reference reading — a clock-out counts for nobody — and quoting the raw
  rows without that rule is quoting the measurement's surrender as a trophy.
- **Seats snapshot the deck's labels.** `archetype` and `themes` are copied
  from the deck at match time, never joined from the live deck at read time:
  the boards group by the class a deck wore when it played, and relabelling
  a deck must not rewrite its history. Since ADR 37 the archetype copied is
  a *reading* of the declared themes (worst-piloted declared class word
  wins) rather than a second declaration; the snapshot rule is unchanged,
  and rows written before the rule moved mean exactly what they did.

This imports `auth/db.py` for the same single reason `sim/cache.py` does:
that module is the `app.db` connection helper and migration ladder, and a
second ladder for the same file would be worse than the import.
"""

from __future__ import annotations

import json
import logging
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from mtglab.decks.model import Deck
    from mtglab.sim.tier3.run import SimRun

_LOG = logging.getLogger("mtglab.sim.tier3.ledger")

#: How many matches a reader gets when it does not say. The ledger is read as
#: a recent-history panel first; the analysis passes that want everything ask
#: for everything.
DEFAULT_LIMIT = 20


def _now() -> str:
    return datetime.now(UTC).isoformat()


def record(run: SimRun, decks: list[Deck], *, seed: int | None, clock: int,
           games_requested: int, hosted: bool,
           owner_ids: list[int | None] | None = None,
           path: Path | str | None = None) -> int | None:
    """Write one finished match. Never raises; returns the match id, or None.

    `decks` arrive in seat order — the order they were handed to Forge, which
    is what `run.seats` and every `winner_seat` already index. `owner_ids`
    matches them positionally and defaults to all-`None`, the file-backed
    curated library, which is what the CLI always records (it only ever loads
    file decks — the same fact that keeps `mtglab decks log` honest).

    `seed=None` is recorded as NULL: an unseeded CLI run is not reproducible,
    and the ledger says so rather than inventing a number.
    """
    owners = owner_ids if owner_ids is not None else [None] * len(decks)
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            cur = con.execute(
                "INSERT INTO forge_matches (created_at, seed, clock,"
                " games_requested, forge_version, hosted, wall_seconds)"
                " VALUES (?, ?, ?, ?, ?, ?, ?)",
                (_now(), seed, clock, games_requested, run.forge_version,
                 1 if hosted else 0, run.wall_seconds))
            match_id = cur.lastrowid
            for seat, (deck, owner) in enumerate(
                    zip(decks, owners, strict=True), 1):
                con.execute(
                    "INSERT INTO forge_seats (match_id, seat, owner_id, slug,"
                    " commander, archetype, themes)"
                    " VALUES (?, ?, ?, ?, ?, ?, ?)",
                    (match_id, seat, owner, deck.slug,
                     json.dumps(deck.commander), deck.archetype,
                     json.dumps(deck.themes)))
            for game in run.games:
                con.execute(
                    "INSERT INTO forge_games (match_id, game_index,"
                    " winner_seat, milliseconds, turns, draw, timed_out)"
                    " VALUES (?, ?, ?, ?, ?, ?, ?)",
                    (match_id, game.index, game.winner_seat,
                     game.milliseconds, game.turns,
                     1 if game.draw else 0, 1 if game.timed_out else 0))
            return int(match_id) if match_id is not None else None
    except Exception as exc:                                        # noqa: BLE001
        # Broader than `decks/log.py`'s catch, deliberately: that writer is
        # handed strings, while this one is handed a run object — and a
        # malformed one must cost a warning, not the minutes of JVM work the
        # caller is holding the results of.
        _LOG.warning("match ledger record failed (%s: %s)",
                     type(exc).__name__, exc)
        return None


def recent(*, limit: int = DEFAULT_LIMIT,
           path: Path | str | None = None) -> list[dict[str, Any]]:
    """The latest matches, newest first, each with its seats and tallies.

    The reference reading of the raw rows: `wins` here applies the clock-out
    rule — a game that hit the clock counts for nobody, even when Forge
    printed a winner line after giving up — so a caller rendering this never
    has to know the rule exists. `draws` counts real draws only; `timed_out`
    is reported apart, because a clock-out is the measurement giving up and
    CLAUDE.md requires the two never be folded together.
    """
    from mtglab.auth import db
    with db.connection(path) as con:
        matches = con.execute(
            "SELECT id, created_at, seed, clock, games_requested,"
            " forge_version, hosted, wall_seconds"
            " FROM forge_matches ORDER BY id DESC LIMIT ?",
            (int(limit),)).fetchall()
        out: list[dict[str, Any]] = []
        for row in matches:
            games = con.execute(
                "SELECT game_index, winner_seat, milliseconds, turns, draw,"
                " timed_out FROM forge_games WHERE match_id = ?"
                " ORDER BY game_index", (row["id"],)).fetchall()
            wins: dict[int, int] = {}
            for game in games:
                if game["winner_seat"] is not None and not game["timed_out"]:
                    wins[game["winner_seat"]] = \
                        wins.get(game["winner_seat"], 0) + 1
            seats = [
                {"seat": seat["seat"], "owner_id": seat["owner_id"],
                 "slug": seat["slug"],
                 "commander": json.loads(seat["commander"]),
                 "archetype": seat["archetype"],
                 "themes": json.loads(seat["themes"]),
                 "wins": wins.get(seat["seat"], 0)}
                for seat in con.execute(
                    "SELECT seat, owner_id, slug, commander, archetype,"
                    " themes FROM forge_seats WHERE match_id = ?"
                    " ORDER BY seat", (row["id"],)).fetchall()
            ]
            out.append({
                "id": row["id"],
                "created_at": row["created_at"],
                "seed": row["seed"],
                "clock": row["clock"],
                "games_requested": row["games_requested"],
                "forge_version": row["forge_version"],
                "hosted": bool(row["hosted"]),
                "wall_seconds": row["wall_seconds"],
                "played": len(games),
                "draws": sum(1 for g in games
                             if g["draw"] and not g["timed_out"]),
                "timed_out": sum(1 for g in games if g["timed_out"]),
                "seats": seats,
            })
        return out
