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

#: A collector number anywhere inside a token: `U0284` carries `0284`, since
#: the rarity letter runs into the number as often as not.
_FACE_NUMBER_IN = re.compile(r"(?P<number>\d{1,4}[a-z]?)")

#: Four digits that are a plausible year. The copyright line sits directly
#: below the crop and Magic has never printed a collector number this high.
_YEAR = re.compile(r"(19|20)\d\d")


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
    #: The bottom-left block exactly as the reader saw it, newlines and all.
    #: Preferred over the two fields above when present, because picking the
    #: set code out of it needs the 986 real ones -- see `from_corner`.
    corner: str | None = None


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



def set_codes(con: Any) -> frozenset[str]:
    """Every set code the pool knows, upper-cased.

    Read once per `read` and handed down, rather than memoised on the
    module. A module memo would be faster and wrong: tests build a pool
    apiece and the app can be pointed at another with `config.use_paths`, so
    a cached table outlives the database it came from. One scan of 107,338
    rows is ~16ms against a batch that already costs a scan per title.
    """
    rows = con.execute(
        "SELECT DISTINCT upper(set_code) FROM printings "
        "WHERE set_code IS NOT NULL").fetchall()
    return frozenset(str(r[0]) for r in rows)


def from_corner(con: Any, text: str | None, *,
                codes: frozenset[str] | None = None) -> Sighting:
    """Read a set code and a collector number out of the raw corner block.

    **This is server-side because the answer needs the pool.** A card's
    bottom-left prints the set code, the language and the artist on one line,
    and a real reader returns them run together -- an actual capture of a
    Lord of the Rings Sol Ring came back as::

        U0284
        LTCENLIK

    `LTC` is in there and no amount of client-side string work can find it,
    because "is this a set code" is a question only the 986 real ones answer.
    A browser would have to be handed that table to ask it.

    Two rules keep the match honest, and both were measured:

    * **Longest real prefix of a line's *first* token.** The set code is the
      leftmost thing on its line, so the artist that follows never gets a
      vote -- which matters, because `CHRISRAHN` has `CHR` (Chronicles) as a
      prefix and 12 of 13 real artist names tested have no match at all.
    * **A token starting with a digit is never a set code.** It is the
      collector-number line, and `U0284` must yield a number and no set.
    """
    if not text:
        return Sighting(corner=text)
    if codes is None:
        codes = set_codes(con)
    number: str | None = None
    code: str | None = None

    for line in text.split("\n")[:8]:
        tokens = [t for t in re.split(r"[^0-9A-Za-z]+", line) if t]
        if not tokens:
            continue
        if number is None:
            for token in tokens:
                digits = _FACE_NUMBER_IN.search(token)
                # A four-digit year is the copyright line, which sits just
                # below and bleeds into the crop constantly.
                if digits and not _YEAR.fullmatch(digits["number"]):
                    number = digits["number"]
                    break
        if code is None:
            first = tokens[0].upper()
            if not first[:1].isdigit():
                for size in range(6, 1, -1):
                    if len(first) >= size and first[:size] in codes:
                        code = first[:size]
                        break

    return Sighting(set_code=code, collector_number=number, corner=text)


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
    batch = sightings[:MAX_SIGHTINGS]
    # One scan for the whole batch. `from_corner` would otherwise repeat it
    # per card, which is forty scans for one photographed deck.
    codes = (set_codes(con)
             if any(s.corner and not s.set_code for s in batch)
             else frozenset())

    out: list[Reading] = []
    for raw in batch:
        # A raw corner block is read here rather than in the browser, because
        # picking a set code out of it needs the pool's own table.
        sighting = (from_corner(con, raw.corner, codes=codes)
                    if raw.corner and not raw.set_code else raw)
        name = by_printing(con, sighting.set_code, sighting.collector_number)
        if name is not None:
            out.append(Reading(resolved=name, via="printing"))
            continue
        candidates = by_title(con, raw.title)
        out.append(Reading(
            via="title" if candidates else "nothing",
            candidates=candidates,
        ))
    return out
