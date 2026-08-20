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
    """Did this line just finish a game? A single-line predicate.

    `run.py` used to count ticks with this and tally with `parse` -- two
    readers of the same stream, kept honest only by sharing regexes. Both now
    ride one `StreamParser`, so this survives as the cheap question a caller
    with no state wants answered (and as the seam older tests pin).
    """
    stripped = line.strip()
    return bool(_WON.match(stripped) or _DRAW.match(stripped))


class StreamParser:
    """The parser, fed one line at a time as Forge speaks.

    `feed` returns a `GameResult` at the moment a result line completes one,
    and accumulates everything into `output` exactly as `parse` would --
    because `parse` *is* this machine fed a whole text. One parser for the
    tick and the tally is the property #203 bought by sharing regexes, made
    structural: they cannot drift because they are the same pass. The match
    theater rides it one step further -- the row a tick carries is the row
    the final tally holds, by identity.
    """

    def __init__(self) -> None:
        self.output = SimOutput()
        # "Game Outcome: Turn N" and the slow-match warning are printed
        # before the "Game Result" line they belong to, so both are held
        # until the result arrives.
        self._pending_turn: int | None = None
        self._pending_timeout = False

    def feed(self, raw: str) -> GameResult | None:
        """Read one line; hand back the game it completed, if it did.

        Forge interleaves game logs, AI warnings and card-database complaints
        on the same stream, so this matches lines it recognises and ignores
        the rest rather than trying to model the whole log.
        """
        line = raw.strip()

        if _SLOW in line:
            self._pending_timeout = True
            return None

        m = _UNSUPPORTED.match(line)
        if m:
            # Forge repeats the complaint per copy; a name is a name.
            if m.group(1) not in self.output.unsupported:
                self.output.unsupported.append(m.group(1))
            return None

        m = _DECK_FAILED.match(line)
        if m:
            self.output.deck_load_failures.append(m.group(1))
            return None

        m = _TURN.match(line)
        if m:
            self._pending_turn = int(m.group(1))
            return None

        m = _WON.match(line)
        if m:
            label = m.group(3)
            seat = PLAYER.match(label)
            game = GameResult(
                index=int(m.group(1)),
                milliseconds=int(m.group(2)),
                winner=label,
                winner_seat=int(seat.group(1)) if seat else None,
                turns=self._pending_turn,
                timed_out=self._pending_timeout,
            )
            self.output.games.append(game)
            self._pending_turn, self._pending_timeout = None, False
            return game

        m = _DRAW.match(line)
        if m:
            game = GameResult(
                index=int(m.group(1)),
                milliseconds=int(m.group(2)),
                draw=True,
                turns=self._pending_turn,
                timed_out=self._pending_timeout,
            )
            self.output.games.append(game)
            self._pending_turn, self._pending_timeout = None, False
            return game

        return None


def parse(text: str) -> SimOutput:
    """Parse a whole `sim` run: `StreamParser` fed every line at once."""
    parser = StreamParser()
    for raw in text.splitlines():
        parser.feed(raw)
    return parser.output
