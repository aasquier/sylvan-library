"""Forge's `sim -q` output is line-oriented text. This turns it into results.

Not JSON, and not a stable API -- the formats below are the literal format
strings in `forge.view.SimulateMatch`, read out of the shipped jar so that the
parser matches what the code prints rather than what a wiki page says:

    Game Result: Game %d ended in %d ms. %s has won!
    Game Result: Game %d ended in a Draw! Took %d ms.

Everything else here exists because of one experiment. A deck containing three
card names Forge does not implement produced this, and then **played the game
anyway**:

    An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".
    Forge could not find this card in the Database. ...
    Game Result: Game 1 ended in 7212 ms. Ai(2)-... has won!

A 96-card deck, a clean winner, and a result line that says nothing is wrong.
That is the failure CLAUDE.md's pre-flight requirement is aimed at, and it is
why `unsupported` is collected here as well: `coverage.py` catches this before
a JVM starts, and this catches it again from Forge's own mouth if a name ever
slips past the index. Two independent checks, because the thing they prevent is
silent.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

# `%s has won!` interpolates the player label, which Forge builds as
# "Ai(<seat>)-<deck name>". Seat is what identifies a deck: it is the position
# in the `-d` argument list, and unlike the name it cannot collide or contain
# an em dash.
PLAYER = re.compile(r"^Ai\((\d+)\)-(.*)$")

_WON = re.compile(r"^Game Result: Game (\d+) ended in (\d+) ms\. (.+) has won!$")
_DRAW = re.compile(r"^Game Result: Game (\d+) ended in a Draw! Took (\d+) ms\.")
_TURN = re.compile(r"^Game Outcome: Turn (\d+)$")
# The line that must never appear. `[N.A.]` is the edition Forge substitutes
# when it cannot place the card at all.
_UNSUPPORTED = re.compile(r'^An unsupported card was requested: "(.+?)" from ')
_DECK_FAILED = re.compile(r"^Could not load deck - (.+), match cannot start$")
_SLOW = "Stopping slow match as draw"


@dataclass
class GameResult:
    """One completed game."""

    index: int
    milliseconds: int
    draw: bool = False
    # The raw "Ai(2)-Atla Palani..." label, and the seat parsed out of it.
    winner: str | None = None
    winner_seat: int | None = None
    turns: int | None = None
    # True when the game hit `-c` and was called a draw rather than finishing.
    # Folded into `draw` by Forge, separated here because a clock-out is a
    # measurement problem and a real draw is a game outcome.
    timed_out: bool = False


@dataclass
class SimOutput:
    """Everything the run said, parsed.

    `unsupported` being non-empty invalidates every result in `games`. Callers
    do not get to decide that; `run.py` raises.
    """

    games: list[GameResult] = field(default_factory=list)
    unsupported: list[str] = field(default_factory=list)
    deck_load_failures: list[str] = field(default_factory=list)

    @property
    def trustworthy(self) -> bool:
        return not self.unsupported and not self.deck_load_failures


def is_game_result(line: str) -> bool:
    """Did this line just finish a game? The streaming tick's one question.

    `run.py` streams Forge's output live and reports progress per game; this
    is the same pair of patterns the full parse recognises results by, shared
    so the tick and the tally cannot drift apart — a progress bar that counts
    lines the parser will later reject would tick past its own total.
    """
    stripped = line.strip()
    return bool(_WON.match(stripped) or _DRAW.match(stripped))


def parse(text: str) -> SimOutput:
    """Parse a whole `sim` run.

    Forge interleaves game logs, AI warnings and card-database complaints on
    the same stream, so this matches lines it recognises and ignores the rest
    rather than trying to model the whole log.
    """
    out = SimOutput()
    # "Game Outcome: Turn N" is printed before the "Game Result" line it
    # belongs to, so it is held until the result arrives.
    pending_turn: int | None = None
    pending_timeout = False

    for raw in text.splitlines():
        line = raw.strip()

        if _SLOW in line:
            pending_timeout = True
            continue

        m = _UNSUPPORTED.match(line)
        if m:
            # Forge repeats the complaint per copy; a name is a name.
            if m.group(1) not in out.unsupported:
                out.unsupported.append(m.group(1))
            continue

        m = _DECK_FAILED.match(line)
        if m:
            out.deck_load_failures.append(m.group(1))
            continue

        m = _TURN.match(line)
        if m:
            pending_turn = int(m.group(1))
            continue

        m = _WON.match(line)
        if m:
            label = m.group(3)
            seat = PLAYER.match(label)
            out.games.append(GameResult(
                index=int(m.group(1)),
                milliseconds=int(m.group(2)),
                winner=label,
                winner_seat=int(seat.group(1)) if seat else None,
                turns=pending_turn,
                timed_out=pending_timeout,
            ))
            pending_turn, pending_timeout = None, False
            continue

        m = _DRAW.match(line)
        if m:
            out.games.append(GameResult(
                index=int(m.group(1)),
                milliseconds=int(m.group(2)),
                draw=True,
                turns=pending_turn,
                timed_out=pending_timeout,
            ))
            pending_turn, pending_timeout = None, False

    return out
