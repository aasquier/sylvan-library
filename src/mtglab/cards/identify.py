"""Name the card in front of the camera, or offer what it might be.

The reader for the photographed deck (ROADMAP item 14). A capture hands this
module what it thought it saw -- a set code and a collector number off the
bottom-left corner, a title off the top, or both -- and gets back either a
card or a shortlist. What it never gets back is a guess.

**Two tiers, and the line between them is measured rather than assumed.**

A set code plus a collector number is a *lookup*. It picks one row out of
`printings` by what amounts to a compound key, there is no judgement in it,
and so a hit resolves outright.

A title is a *similarity*, and the numbers say a similarity must never
resolve anything. Simulating OCR damage over the 400 most-played cards, with
the character confusions a real engine makes (l/1, O/0, rn/m, cl/d):

    1 error : top-1 correct 120/120 | lowest CORRECT 0.875 | highest WRONG -
    2 errors: top-1 correct 116/120 | lowest CORRECT 0.801 | highest WRONG 0.917
    3 errors: top-1 correct 108/120 | lowest CORRECT 0.780 | highest WRONG 0.933
    5 errors: top-1 correct  72/120 | lowest CORRECT 0.740 | highest WRONG 0.942

The two distributions overlap, and they overlap *badly*: at three errors a
wrong card scores higher (0.933) than a right one does (0.780). No threshold
can separate them, so a threshold would only decide how often the app
asserts the wrong card confidently. It does not get to assert one at all --
`by_title` ranks, and a human picks. That is rule 4's ethic reached from a
measurement instead of a principle, and it is why `Reading.resolved` is
`None` for every title-only sighting no matter how good the score looks.

`CANDIDATES` is five for the same reason it is not eight: correct-answer-in-
the-list runs 99% / 95% / 80% at two, three and five errors, and is already
99% / 95% / 78% by the third. Past three the list stops paying.

**Front faces are matched too.** 889 names in the pool carry a `//`, and a
*flawlessly* read title of one loses without this: `Delver of Secrets` ranks
`Stealer of Secrets` (0.905) above `Delver of Secrets // Insectile
Aberration` (0.883). Scoring against the front face as well takes it to
1.000, because the camera can only ever see one side of the card.

**Banned cards are identifiable and non-cards are not.** The filter is
commander legality `IN ('legal', 'banned')`, which keeps Primeval Titan and
Emrakul -- two of this library's own decks run them -- while dropping the 87
emblems, 102 schemes, 207 planes and 107 vanguards that are in `oracle_cards`
and can never be in a deck. Refusing a banned card is the gate's job; the
reader's job is to say what is on the table.

Nothing here writes, and nothing here reaches a network.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

#: How many names a title sighting offers. The knee is three; see the module
#: docstring for the curve this is read off.
CANDIDATES = 5

#: The most sightings one request may carry. A photo of a fanned spread holds
#: ten or so cards, and each title costs a scan of ~32k names (~16ms), so this
#: is a bound on work rather than a statement about how people use the camera.
MAX_SIGHTINGS = 40

#: Longest title text considered. The longest card name ever printed is 141
#: characters (the Unhinged elemental whose name is a joke about long card
#: names); past this it is not a misread title, it is a misread card.
MAX_TITLE = 200

#: Cards a Commander deck could contain, banned ones included. Interpolated
#: rather than parameterised because it carries no caller input.
_IS_CARD = "json_extract_string(legalities, 'commander') IN ('legal', 'banned')"

#: A collector number as the *card face* prints it: `284/281`, sometimes
#: zero-padded. The pool stores only the first half, so the denominator is
#: dropped before the lookup.
_FACE_NUMBER = re.compile(r"^\s*(?P<number>[^/\s]+)")

#: A set code is two to six alphanumerics, as `decklist.py` already has it.
#: Stored lowercase in the pool, which is why every comparison folds case.
_SET_CODE = re.compile(r"^[A-Za-z0-9]{2,6}$")


@dataclass(frozen=True)
class Sighting:
    """What one capture thought it saw.

    Every field is optional and independently unreliable, which is the whole
    reason this is three fields rather than a resolved card: the corner may
    be legible on a card whose title is glare, and a pre-2015 frame prints no
    collector number at all.
    """

    set_code: str | None = None
    collector_number: str | None = None
    title: str | None = None


@dataclass(frozen=True)
class Candidate:
    """A name the title *might* be, and how alike the two strings are.

    The score is a string distance and nothing more. It is carried so a
    client can order and shade a list, never so anything can threshold on it.
    """

    name: str
    score: float


@dataclass
class Reading:
    """What the pool made of one sighting."""

    #: A card name, and only ever from a printing lookup. `None` means the
    #: user still has to choose -- including when `candidates` looks certain.
    resolved: str | None = None
    #: How `resolved` was arrived at, or why nothing was: `printing`,
    #: `title` (offered, not decided), or `nothing`.
    via: str = "nothing"
    candidates: list[Candidate] = field(default_factory=list)


def face_number(text: str | None) -> str | None:
    """`284/281` -> `284`. The pool stores the numerator alone."""
    if not text:
        return None
    found = _FACE_NUMBER.match(text)
    return found["number"] if found else None


def by_printing(con: Any, set_code: str | None,
                collector_number: str | None) -> str | None:
    """The corner tier: one row out of `printings`, or nothing.

    Zero-padding is tolerated on both sides because the face pads and the
    pool does not (three of 12,650 stored numbers have a leading zero, and
    plenty of cards print `0284`).

    A `(set, number)` pair that somehow names two different cards resolves to
    neither -- that is an ambiguity, and this tier's whole claim is that it
    has no judgement in it.
    """
    number = face_number(collector_number)
    if not set_code or not number or not _SET_CODE.match(set_code):
        return None
    rows = con.execute(
        "SELECT DISTINCT name FROM printings "
        "WHERE lower(set_code) = lower(?) "
        "  AND (collector_number = ? "
        "       OR ltrim(collector_number, '0') = ltrim(?, '0')) "
        "LIMIT 2",
        [set_code, number, number],
    ).fetchall()
    return str(rows[0][0]) if len(rows) == 1 else None


def by_title(con: Any, title: str | None, *,
             limit: int = CANDIDATES) -> list[Candidate]:
    """The title tier: a ranked shortlist, and never an answer.

    Scored against the whole name *and* the front face, taking the better of
    the two, because the camera sees one side of a double-faced card.
    """
    if not title:
        return []
    text = title.strip()[:MAX_TITLE]
    if not text:
        return []
    rows = con.execute(
        "SELECT name, greatest("
        "         jaro_winkler_similarity(lower(name), lower(?)),"
        "         jaro_winkler_similarity("
        "           lower(split_part(name, ' // ', 1)), lower(?))) AS score "
        f"FROM oracle_cards WHERE {_IS_CARD} "
        "ORDER BY score DESC, edhrec_rank NULLS LAST "
        "LIMIT ?",
        [text, text, max(1, int(limit))],
    ).fetchall()
    return [Candidate(name=str(r[0]), score=float(r[1])) for r in rows]


def read(con: Any, sightings: list[Sighting]) -> list[Reading]:
    """Read a batch of captures, in the order they were taken.

    One `Reading` per `Sighting`, always, so a client can hold the two lists
    side by side; a sighting nothing could be made of comes back as
    `via="nothing"` rather than being dropped.
    """
    out: list[Reading] = []
    for sighting in sightings[:MAX_SIGHTINGS]:
        name = by_printing(con, sighting.set_code, sighting.collector_number)
        if name is not None:
            out.append(Reading(resolved=name, via="printing"))
            continue
        candidates = by_title(con, sighting.title)
        out.append(Reading(
            via="title" if candidates else "nothing",
            candidates=candidates,
        ))
    return out
