"""What crosses the private network between the app and the Forge worker.

ADR 35's hosted shape splits Tier 3 across two machines: the app plans a
match and shapes its results; the worker holds the distribution, the JVM and
nothing else. This module is the seam — every byte that crosses is defined
here, in both directions, so `shim.py` (the worker's door) and `worker.py`
(the app's client) cannot drift apart without a test noticing.

Two deliberate choices:

* **A deck travels as `deck.yaml` text.** `Deck.from_text` is the one parser
  (ADR 4 relies on that property), so inventing a second JSON encoding of a
  deck would be a second parser to keep honest. The worker re-parses with the
  same code the app used to load it.
* **Results come back as JSON and are rebuilt into a real `SimRun`.** The
  shape `forgeruns._shape` consumes is `SimRun`'s — games, seats, wall clock —
  so the client reconstructs `parse.GameResult` objects rather than teaching
  the API a parallel result type. A remote match and a local one are the same
  thing to everything downstream, which is the point.
"""

from __future__ import annotations

from typing import Any

from mtglab.decks.model import Deck
from mtglab.sim.tier3 import parse
from mtglab.sim.tier3.coverage import CoverageReport
from mtglab.sim.tier3.run import SimRun


def decks_to_wire(decks: list[Deck]) -> list[str]:
    """Each deck as its own `deck.yaml` text, in seat order."""
    return [deck.dump() for deck in decks]


def decks_from_wire(texts: list[str]) -> list[Deck]:
    return [Deck.from_text(text) for text in texts]


def reports_to_wire(reports: list[CoverageReport]) -> list[dict[str, Any]]:
    return [
        {"slug": r.slug, "checked": r.checked, "resolved": r.resolved,
         "missing": r.missing}
        for r in reports
    ]


def reports_from_wire(payload: list[dict[str, Any]]) -> list[CoverageReport]:
    return [
        CoverageReport(slug=str(r["slug"]), checked=int(r["checked"]),
                       resolved=dict(r.get("resolved") or {}),
                       missing=list(r.get("missing") or []))
        for r in payload
    ]


def game_to_wire(game: parse.GameResult) -> dict[str, Any]:
    """One game, encoded once for both journeys it makes.

    A finished run's games cross inside `run_to_wire` below; since the match
    theater, each game also crosses *alone*, on the stream's `{"game": n}`
    line, the moment it ends. One codec for both is what keeps a streamed row
    and the same row in the final tally byte-identical — the drift this
    module exists to make impossible.
    """
    return {"index": game.index, "milliseconds": game.milliseconds,
            "draw": game.draw, "winner": game.winner,
            "winner_seat": game.winner_seat, "turns": game.turns,
            "timed_out": game.timed_out}


def game_from_wire(payload: dict[str, Any]) -> parse.GameResult:
    return parse.GameResult(
        index=int(payload["index"]), milliseconds=int(payload["milliseconds"]),
        draw=bool(payload.get("draw")), winner=payload.get("winner"),
        winner_seat=(None if payload.get("winner_seat") is None
                     else int(payload["winner_seat"])),
        turns=None if payload.get("turns") is None else int(payload["turns"]),
        timed_out=bool(payload.get("timed_out")))


def run_to_wire(run: SimRun) -> dict[str, Any]:
    """A finished `SimRun`, minus what does not travel.

    `argv` stays on the worker (a path on another machine is noise here) and
    `coverage` is not repeated — the pre-flight already crossed on its own.
    `startup_seconds` is a property derived from games and wall clock, so it
    is not serialised; the rebuilt run computes the same number.
    """
    return {
        "games": [game_to_wire(g) for g in run.games],
        # JSON keys are strings; the seat numbers come back in `run_from_wire`.
        "seats": {str(seat): slug for seat, slug in run.seats.items()},
        "wall_seconds": run.wall_seconds,
        # For the match ledger (ADR 36). Optional in both directions on
        # purpose: an old shim omits it and an old app ignores it, so deploy
        # skew degrades to "not reported" rather than to an error.
        "forge_version": run.forge_version,
    }


def run_from_wire(payload: dict[str, Any]) -> SimRun:
    games = [game_from_wire(g) for g in payload.get("games", [])]
    version = payload.get("forge_version")
    return SimRun(
        argv=[], output=parse.SimOutput(games=games),
        wall_seconds=float(payload.get("wall_seconds", 0.0)),
        seats={int(seat): str(slug)
               for seat, slug in (payload.get("seats") or {}).items()},
        forge_version=None if version is None else str(version))
