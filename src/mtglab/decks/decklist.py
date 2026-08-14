"""Read a decklist the way people actually paste one.

Pure text in, structured lines out. No card pool, no filesystem, no judgement
about whether a card is real -- that is `importer.py`'s job, and keeping the
two apart is what lets the grammar be tested exhaustively without a database.

There is no decklist standard, so this parses the union of what the exports
people use actually emit:

    1 Sol Ring                          plain
    1x Sol Ring                         Archidekt, TappedOut
    36 Forest                           basics in one line
    Sol Ring                            bare name, quantity implied
    1 Sol Ring (LTC) 284                Moxfield, MTGA -- set code, collector no.
    1 Sol Ring (2X2) 297 *F*            printing markers: foil, etched
    1 Arahbo, Roar of the World *CMDR*  Moxfield's commander marker
    1x Sol Ring (c21) 263 [Ramp]        Archidekt category annotation
    1 Darkbore Pathway // Slitherbore Pathway
    # a comment                         and MTGO's `//` at the start of a line

    Commander                           section headers, with or without a
    Commander: / Commander (1)          colon, count, or both
    Deck / Sideboard / Maybeboard

**Two things this deliberately does not do.** It never consults a card list to
disambiguate, and it never silently drops a line it could not read. Lines it
cannot turn into a name land in `unreadable` with their line number, so the
importer can report them. Everything it *can* read is handed on verbatim to be
resolved against the pool, which is the only thing entitled to say whether a
name is a card (rule 1).

The one ambiguity that cannot be resolved here: a bare `3 Steps Ahead` is
either three copies of "Steps Ahead" or one copy of the blue instant. Without a
pool both readings are equally good, so the leading-number reading wins --
and the resulting unknown name gets reported rather than guessed at. Writing
the quantity, as every real export does, removes the ambiguity.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

# Where a line ends up. `swap_board` is the model's name for the maybeboard:
# Commander has no sideboard, and a list of cards you are considering is
# exactly what the deck file's swap board already holds. `ignored` is a section
# whose lines are not deck cards at all.
_SECTION_WORDS = {
    "commander": "commander",
    "commanders": "commander",
    "companion": "companion",
    "companions": "companion",
    "deck": "deck",
    "decklist": "deck",
    "main": "deck",
    "mainboard": "deck",
    "main deck": "deck",
    "maindeck": "deck",
    "sideboard": "swap_board",
    "maybeboard": "swap_board",
    "considering": "swap_board",
    # Tokens are not cards in the 99. Recognised so they are reported as
    # skipped rather than resolved and then reported as unknown cards.
    "token": "ignored",
    "tokens": "ignored",
}

# TappedOut and Deckstats group the deck under card-type headings. They are
# still deck cards, so they all map to the same section -- and note that a
# `Lands` heading does NOT set the land category. That comes from the pool's
# `is_land`, which is right about the double-faced cards a heading is wrong
# about.
_SECTION_WORDS.update({
    word: "deck"
    for stem in ("creature", "instant", "artifact", "enchantment",
                 "planeswalker", "land", "battle", "spell", "other")
    for word in (stem, stem + "s")
})
_SECTION_WORDS.update({"sorcery": "deck", "sorceries": "deck"})

# `Commander`, `Commander:`, `COMMANDER (1)`, `Sideboard - 0`. The count is
# whatever the exporter thought; it is not trusted, only tolerated.
_HEADER = re.compile(
    r"^\s*(?P<word>[A-Za-z][A-Za-z ]*?)\s*[:\-]?\s*(?:\(\s*\d+\s*\))?\s*$")

# A leading quantity. Capped at three digits so that `1996 World Champion`
# parses as a name rather than as 1,996 copies of "World Champion".
_QTY = re.compile(r"^(?P<qty>\d{1,3})\s*[xX]?\s+(?=\S)")

# Trailing annotations, stripped right to left. Each is anchored to the end so
# that a card name containing brackets or parentheses mid-string survives.
_MARKER = re.compile(r"\s*\*(?P<marker>[^*]*)\*\s*$")
_BRACKET = re.compile(r"\s*\[(?P<label>[^\]]*)\]\s*$")
# `(LTC) 284`, `(NEO) 123s`, `(WAR) 25★`, or a bare `(c21)`. The set code is
# 2-6 alphanumerics, which is short enough not to swallow `B.F.M. (Big Furry
# Monster)` or `Erase (Not the Urza's Legacy One)`.
_PRINTING = re.compile(
    r"\s*\((?P<set>[A-Za-z0-9]{2,6})\)(?:\s+[A-Za-z0-9\-★]+)?\s*$")

# Deckstats puts the set code in front: `1 [C17] Arahbo, Roar of the World`.
# Safe to strip unconditionally because no card name begins with a bracket.
_LEADING_SET = re.compile(r"^\[[A-Za-z0-9]{2,6}\]\s*")


@dataclass(frozen=True)
class ParsedCard:
    """One readable line. The name is verbatim -- nothing has resolved it yet."""

    name: str
    qty: int
    section: str
    line_no: int


@dataclass
class ParsedList:
    cards: list[ParsedCard] = field(default_factory=list)
    # Lines that are not blank, not comments and not headers, but left no name
    # behind once the annotations were stripped.
    unreadable: list[tuple[int, str]] = field(default_factory=list)
    # Lines under a section that is not part of the deck, e.g. Tokens.
    skipped: list[tuple[int, str]] = field(default_factory=list)

    def section(self, name: str) -> list[ParsedCard]:
        return [c for c in self.cards if c.section == name]

    @property
    def commander(self) -> list[str]:
        return [c.name for c in self.section("commander")]

    @property
    def companion(self) -> str | None:
        found = self.section("companion")
        return found[0].name if found else None


def _strip_annotations(text: str) -> tuple[str, set[str]]:
    """Peel printing and category annotations off the end of a line.

    Returns the remaining text and any `*...*` markers found, lowercased.
    Applied in a loop because they combine: `(2X2) 297 *F* *CMDR*`.
    """
    markers: set[str] = set()
    while True:
        marker = _MARKER.search(text)
        if marker:
            markers.add(marker["marker"].strip().lower())
            text = text[: marker.start()]
            continue
        bracket = _BRACKET.search(text)
        if bracket:
            # Archidekt writes the deck's own category here, e.g. `[Ramp]` or
            # `[Commander{top}]`. Only the commander label is kept, and only
            # because it is a fact the exporter stated rather than something
            # inferred from the card -- ADR 13 leaves every other category to a
            # human, on the grounds that a guessed category ends up asserted in
            # a generated primer.
            label = bracket["label"].split("{")[0].strip().lower()
            if label in ("commander", "commanders"):
                markers.add("cmdr")
            text = text[: bracket.start()]
            continue
        printing = _PRINTING.search(text)
        if printing:
            text = text[: printing.start()]
            continue
        return text.strip(), markers


def _header_section(line: str) -> str | None:
    match = _HEADER.match(line)
    if not match:
        return None
    return _SECTION_WORDS.get(" ".join(match["word"].lower().split()))


def parse(text: str) -> ParsedList:
    """Parse a pasted decklist. Never raises: unreadable lines are reported."""
    out = ParsedList()
    section = "deck"

    for line_no, raw in enumerate(text.splitlines(), start=1):
        line = raw.rstrip()
        if not line.strip():
            continue

        # MTGO writes comments as `//`, which is also the double-faced card
        # separator -- so only a line that *starts* with it is a comment.
        # Deckstats writes its section headers that way (`//Commander`), so the
        # word is checked before the line is discarded: mistaking that header
        # for a comment puts the commander in the 99, which is a wrong deck
        # rather than a missing one.
        commented = line.lstrip().lstrip("#/").strip()
        if line.lstrip().startswith(("#", "//")):
            header = _header_section(commented)
            if header is not None:
                section = header
            continue

        header = _header_section(line)
        if header is not None:
            section = header
            continue

        body, markers = _strip_annotations(line)
        if not body:
            out.unreadable.append((line_no, line.strip()))
            continue

        qty = 1
        quantity = _QTY.match(body)
        if quantity:
            qty = int(quantity["qty"])
            body = body[quantity.end():].strip()
        body = _LEADING_SET.sub("", body).strip()
        if not body:
            out.unreadable.append((line_no, line.strip()))
            continue

        if section == "ignored":
            out.skipped.append((line_no, body))
            continue

        # A `*CMDR*` marker overrides the section it was found in: Moxfield's
        # plain export has no headers at all and marks the commander inline.
        where = "commander" if "cmdr" in markers else section
        out.cards.append(ParsedCard(name=body, qty=qty, section=where,
                                    line_no=line_no))

    return out
