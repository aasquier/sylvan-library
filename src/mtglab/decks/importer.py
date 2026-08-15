"""Turn a pasted decklist into a draft `deck.yaml`.

The parser (`decklist.py`) reads lines. This resolves what those lines *mean*
against the pool and writes a deck file. Split that way because resolution is
the half with an opinion, and the opinion is short:

**Nothing is guessed.** Rule 1 says never evaluate a card from memory, and the
same discipline applies to a name: a line that does not resolve is reported
with what was written, kept in the deck verbatim so the list stays the size the
user pasted, and left for the gate to flag as `unknown-card`. Dropping it would
quietly hand back a 96-card deck.

**Nothing is invented.** Every card arrives with an empty `why`, and the deck is
written as `stage: draft` so the gate reports those as warnings and counts them
(ADR 13). A rationale written by the tool is precisely the empty justification
rule 4 exists to prevent, and doing it 99 times at once is worse than doing it
once ([ADR 11](../../../docs/adr/0011-the-api-may-apply-a-swap.md)).

**One thing is inferred, and only because it is a card pool fact.** A card is filed
under `land` when `CardRecord.is_land` says so -- which is right about the
double-faced cards a type line is wrong about -- and everything else takes the
model's `utility` default for a human to file. Guessing that a card is "ramp"
would put an invented claim into a generated primer, which is the failure this
project was built to stop.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import date
from typing import TYPE_CHECKING

from mtglab.decks.decklist import ParsedCard, ParsedList
from mtglab.decks.model import CardEntry, Deck

if TYPE_CHECKING:
    from mtglab.cards.db import CardRecord

HEADER = """\
# Imported {today} from a pasted decklist, and NOT yet reasoned about.
#
# `stage: draft` means the gate reports every missing `why` as a warning
# instead of an error, so the deck's *facts* -- legality, colour identity,
# singleton, size -- are checked from day one while the thinking is still
# owed. Artifacts stay blocked until that work is done.
#
# To promote it: write a `why` on every card, then set `stage: curated`. The
# gate refuses the promotion while any card is still blank, and nothing here
# will fill one in for you -- a rationale written by the tool is exactly the
# empty justification the rule exists to prevent.
"""


class ImportRefused(Exception):
    """The list could not be turned into a deck, and nothing was written."""


@dataclass
class ImportReport:
    """What the import did, and what it could not do.

    Deliberately verbose. An import that half-worked and said nothing is how a
    deck ends up with three cards spelled wrong and a commander in the 99.
    """

    deck: Deck
    yaml_text: str
    # Names the pool does not have. Kept in the deck; reported here and by
    # the gate as `unknown-card`.
    unknown: list[str] = field(default_factory=list)
    # Lines the parser could not read at all, as (line number, text).
    unreadable: list[tuple[int, str]] = field(default_factory=list)
    # Lines under a section that is not part of the deck, e.g. Tokens.
    skipped: list[tuple[int, str]] = field(default_factory=list)
    # Things that were changed rather than rejected, each worth saying out loud.
    notes: list[str] = field(default_factory=list)

    @property
    def needs_rationale(self) -> int:
        return len(self.deck.unjustified)


def names_in(parsed: ParsedList, *, commander: list[str] | None = None,
             companion: str | None = None) -> list[str]:
    """Every name that needs a card pool lookup, so the caller can fetch once."""
    names = [c.name for c in parsed.cards]
    names += list(commander or [])
    if companion:
        names.append(companion)
    return sorted({n for n in names if n})


def _canonical(written: str,
               cards: dict[str, CardRecord]) -> tuple[str, CardRecord | None]:
    """The name as the pool spells it, and the record behind it.

    Casing is corrected, but a double-faced card written by its front face
    stays written that way. `db.get_cards` resolves both, the curated decks all
    use face names ("Branchloft Pathway"), and expanding one to
    "Branchloft Pathway // Boulderloft Pathway" on import would make the
    library inconsistent for no gain.
    """
    rec = cards.get(written)
    if rec is None:
        return written, None
    for face in rec.name.split(" // "):
        if face.lower() == written.strip().lower():
            return face, rec
    return rec.name, rec


def build_deck(parsed: ParsedList, cards: dict[str, CardRecord], *, slug: str,
               name: str | None = None, commander: list[str] | None = None,
               companion: str | None = None, bracket: int | None = None,
               status: str = "theoretical") -> ImportReport:
    """Resolve a parsed list into a draft deck. Raises `ImportRefused`.

    `cards` is a name -> CardRecord mapping, as returned by `db.get_cards` over
    `names_in(parsed)`. Passing an empty mapping is legal and means every name
    is reported as unknown, which is what a fresh clone with no card pool gets --
    honestly useless rather than silently wrong.
    """
    report_notes: list[str] = []
    unknown: list[str] = []

    def resolve(written: str) -> tuple[str, CardRecord | None]:
        canonical, rec = _canonical(written, cards)
        if rec is None and canonical not in unknown:
            unknown.append(canonical)
        return canonical, rec

    # ---- the command zone ------------------------------------------------
    # An explicit commander wins over whatever the list claimed: the caller
    # knows which deck this is and the exporter only knows what it wrote.
    wanted = [c for c in (commander or []) if c.strip()] or parsed.commander
    if not wanted:
        raise ImportRefused(_no_commander_message(parsed))
    if len(wanted) > 2:
        raise ImportRefused(
            f"{len(wanted)} commanders listed ({', '.join(wanted)}); Commander "
            "allows at most two, and only with a pairing ability")

    commanders = [resolve(c)[0] for c in wanted]
    companion_name = None
    if companion and companion.strip():
        companion_name = resolve(companion.strip())[0]
    elif parsed.companion:
        companion_name = resolve(parsed.companion)[0]

    outside = {n.lower() for n in commanders}
    if companion_name:
        outside.add(companion_name.lower())

    # ---- the 99 ----------------------------------------------------------
    # A list can nominate a commander or companion that the caller then
    # overrides. Those cards are still cards, and dropping them because of a
    # section header they were filed under would quietly hand back a 98-card
    # deck -- so anything the command zone did not take falls into the 99.
    demoted = [line for line in parsed.section("commander") + parsed.section("companion")
               if _canonical(line.name, cards)[0].lower() not in outside]
    if demoted:
        report_notes.append(
            f"{len(demoted)} card(s) the list nominated for the command zone "
            f"were not chosen, and went into the 99: "
            f"{', '.join(sorted({line.name for line in demoted}))}")

    entries, moved = _entries(parsed.section("deck") + demoted, resolve,
                              outside, report_notes)
    swaps, also_moved = _entries(parsed.section("swap_board"), resolve,
                                 outside, report_notes)

    # Lists that mark the commander inline really do have 100 lines, and a
    # commander given with `--commander` is usually still sitting in the
    # sideboard section -- that is where our own moxfield.txt artifact puts it.
    lifted = moved + [n for n in also_moved if n not in moved]
    if lifted:
        report_notes.append(
            f"{len(lifted)} card(s) sit outside the 99 and were removed from "
            f"it: {', '.join(lifted)}")

    if companion_name is None:
        report_notes += _companion_hints(swaps, cards)

    deck = Deck(
        slug=slug,
        name=name.strip() if name and name.strip() else slug,
        status=status,
        stage="draft",
        commander=commanders,
        companion=companion_name,
        bracket=bracket,
        cards=entries,
        swap_board=swaps,
    )
    text = HEADER.format(today=date.today().isoformat()) + "\n" + deck.dump()

    return ImportReport(deck=deck, yaml_text=text, unknown=unknown,
                        unreadable=list(parsed.unreadable),
                        skipped=list(parsed.skipped), notes=report_notes)


def _entries(lines: list[ParsedCard],
             resolve: Callable[[str], tuple[str, CardRecord | None]],
             outside: set[str],
             notes: list[str]) -> tuple[list[CardEntry], list[str]]:
    """Resolve parsed lines into card entries, merging repeated names."""
    by_key: dict[str, CardEntry] = {}
    removed: list[str] = []

    for line in lines:
        canonical, rec = resolve(line.name)
        key = canonical.lower()
        if key in outside:
            # The card is in the command zone, so it is not in the 99. Lists
            # that mark the commander inline really do have 100 lines.
            if canonical not in removed:
                removed.append(canonical)
            continue
        if key in by_key:
            by_key[key].qty += line.qty
            notes.append(f"{canonical} appeared on more than one line; "
                         f"merged to qty {by_key[key].qty}")
            continue
        by_key[key] = CardEntry(
            name=canonical,
            # The only inference, and only because `is_land` is a card pool fact.
            category="land" if rec is not None and rec.is_land else "utility",
            why="",
            qty=line.qty,
        )
    return list(by_key.values()), removed


def _companion_hints(swaps: list[CardEntry],
                     cards: dict[str, CardRecord]) -> list[str]:
    """Point out a companion sitting on the swap board, without assuming one.

    Our own moxfield.txt puts the commander *and* the companion under
    `SIDEBOARD:`, so re-importing an exported deck strands the companion there.
    Having a Companion ability is a card pool fact and worth reporting; concluding
    that this deck runs it as its companion is a judgement, and the card is
    perfectly capable of being an ordinary creature in the 99.
    """
    from mtglab.decks import companion as companion_rules

    found = [e.name for e in swaps
             if (rec := cards.get(e.name)) is not None
             and companion_rules.is_companion(rec)]
    if not found:
        return []
    return [(f"{', '.join(found)} has a Companion ability and is on the swap "
             f"board. Pass a companion explicitly if this deck runs one -- it "
             f"changes what the gate checks, so it is not assumed.")]


def _no_commander_message(parsed: ParsedList) -> str:
    """Say what to do next, with the candidates the list actually contains."""
    message = ("no commander in this list, and none was given. Add a "
               "`Commander` section to the list, or name one explicitly")
    # Moxfield's Commander import reads the commander out of `SIDEBOARD:`, and
    # our own moxfield.txt artifact writes it there -- so the swap board is
    # where a re-imported deck's commander actually is. Say so rather than
    # picking one, which would be a guess between two legendary creatures.
    board = [c.name for c in parsed.section("swap_board")]
    if board:
        message += (f". This list has a sideboard section containing "
                    f"{', '.join(board)} -- Moxfield uses `SIDEBOARD:` to "
                    f"carry the commander, so that may be what you want")
    return message + "."
