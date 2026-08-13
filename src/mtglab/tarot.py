"""The 78-card Rider-Waite-Smith deck, and the deal.

**Python decides, Claude advises** ([ADR 14](../../docs/adr/0014-python-decides-claude-advises.md)),
and a tarot reading splits along that line more cleanly than it looks. The
deck, the shuffle, the reversals and the spread are facts with right answers:
they are here, they are seeded, and they are tested without a network. What a
spread *means* has no right answer at all, which is exactly the half Claude is
for -- see `claude/persona.py`.

So this module knows every card and no card's meaning. There is deliberately no
`meaning` field: writing 78 upright-and-reversed interpretations into a Python
file would be inventing an authority this project does not have, and it would
also freeze the reading into something the same every time. The reader
interprets; this deals.

**The spread is three cards, and the positions are not decorative.** They are
`SLOT_KINDS`' first three, from `claude/theme.py` -- taste, temperament,
posture -- which is what lets the tarot door reuse ADR 20's readiness
instrument unchanged. A card is dealt *for* a slot, the reader asks about that
slot while it is face up, and the answer is what grounds it. The cards colour
the questions; the querent's own words remain the only evidence. `anchor` gets
no card because it is the optional fourth slot and `FLOOR` is three.

The art is the original 1909 Rider "Roses & Lilies" printing, public domain in
both the US and the UK. `assets/tarot/PROVENANCE.md` is the argument; the short
version is that the 1971 US Games recolouring everybody pictures is *not* it,
and every filename here carries `RWS1909` upstream for that reason.
"""

from __future__ import annotations

import random
from dataclasses import dataclass

#: The trumps, in order. Keys match the asset filenames exactly, which is what
#: `tests/test_tarot.py` checks -- a renamed card with no picture is a broken
#: reading, and it should fail in a test rather than in front of somebody.
#:
#: Strength is VIII and Justice is XI. That is Waite's swap, not a mistake:
#: he exchanged them from the Marseille order for reasons of astrological
#: correspondence, and this deck is his.
MAJOR_ARCANA: tuple[tuple[str, str], ...] = (
    ("00-fool", "The Fool"),
    ("01-magician", "The Magician"),
    ("02-high-priestess", "The High Priestess"),
    ("03-empress", "The Empress"),
    ("04-emperor", "The Emperor"),
    ("05-hierophant", "The Hierophant"),
    ("06-lovers", "The Lovers"),
    ("07-chariot", "The Chariot"),
    ("08-strength", "Strength"),
    ("09-hermit", "The Hermit"),
    ("10-wheel-of-fortune", "Wheel of Fortune"),
    ("11-justice", "Justice"),
    ("12-hanged-man", "The Hanged Man"),
    ("13-death", "Death"),
    ("14-temperance", "Temperance"),
    ("15-devil", "The Devil"),
    ("16-tower", "The Tower"),
    ("17-star", "The Star"),
    ("18-moon", "The Moon"),
    ("19-sun", "The Sun"),
    ("20-judgement", "Judgement"),
    ("21-world", "The World"),
)

#: Wands, Cups, Swords, Pentacles -- the order Waite lists them in.
SUITS: tuple[str, ...] = ("wands", "cups", "swords", "pentacles")

#: 1-10 then the four court cards, which is how the upstream files are
#: numbered (`RWS1909 - Cups 14` is the King).
RANKS: dict[int, str] = {
    1: "Ace", 2: "Two", 3: "Three", 4: "Four", 5: "Five", 6: "Six", 7: "Seven",
    8: "Eight", 9: "Nine", 10: "Ten",
    11: "Page", 12: "Knight", 13: "Queen", 14: "King",
}


@dataclass(frozen=True)
class Card:
    """One card. `key` is both its identity and its picture's filename."""

    key: str
    name: str
    #: "major" or "minor". The reader is told which, because a spread of all
    #: trumps means something to a reader and nothing to a shuffler.
    arcana: str
    suit: str | None
    #: 0-21 for a trump, 1-14 within a suit.
    number: int

    @property
    def image(self) -> str:
        return f"/tarot/{self.key}.webp"

    def as_dict(self) -> dict[str, object]:
        return {"key": self.key, "name": self.name, "arcana": self.arcana,
                "suit": self.suit, "number": self.number, "image": self.image}


def _build_deck() -> tuple[Card, ...]:
    cards = [Card(key, name, "major", None, i)
             for i, (key, name) in enumerate(MAJOR_ARCANA)]
    for suit in SUITS:
        for n, rank in RANKS.items():
            cards.append(Card(f"{suit}-{n:02d}", f"{rank} of {suit.title()}",
                              "minor", suit, n))
    return tuple(cards)


#: All 78, majors first. Built rather than typed out: 56 minor cards written by
#: hand is 56 chances to typo a filename nobody notices until a card is dealt.
DECK: tuple[Card, ...] = _build_deck()

BY_KEY: dict[str, Card] = {c.key: c for c in DECK}


@dataclass(frozen=True)
class Position:
    """A place in the spread, and the slot the reader is fishing for there."""

    #: The slot kind from `claude/theme.py`. This is the load-bearing field.
    slot: str
    #: What the reader calls it out loud.
    name: str
    #: What the position is asking, in the reader's own frame. Goes into the
    #: prompt; it is not shown as a label, because a spread that explains
    #: itself in advance is a form with candles on it.
    asks: str


#: Three cards, three required slots, in the order a reading wants them: what
#: you already are, what happens when it goes wrong, who you are with others.
SPREAD: tuple[Position, ...] = (
    Position("taste", "The Root",
             "what they already love, from long before any of this"),
    Position("temperament", "The Turning",
             "how they are when the plan comes apart in their hands"),
    Position("posture", "The Table",
             "who they become when there are other people in the room"),
)


@dataclass(frozen=True)
class Drawn:
    """A card, where it landed, and which way up."""

    card: Card
    position: Position
    #: Reversed cards are traditional and they are free: the picture is
    #: rotated in CSS. They give the reader somewhere to go when a card's
    #: upright reading does not fit the person in front of them.
    reversed: bool

    def as_dict(self) -> dict[str, object]:
        return {**self.card.as_dict(), "reversed": self.reversed,
                "slot": self.position.slot, "position": self.position.name}


@dataclass(frozen=True)
class Reading:
    """A dealt spread. Seeded, so a reload shows the same cards."""

    seed: int
    cards: tuple[Drawn, ...]

    def as_dict(self) -> dict[str, object]:
        return {"seed": self.seed, "cards": [d.as_dict() for d in self.cards]}

    def describe(self) -> str:
        """The spread as a line per card, for the reader's prompt.

        Prose rather than JSON on purpose: this is read by a model that is
        being asked to sound like a person, and handing it a data structure
        invites it to answer with one.
        """
        return "\n".join(
            f"- {d.position.name} ({d.position.asks}): {d.card.name}"
            f"{', reversed' if d.reversed else ''}"
            for d in self.cards)


def deal(seed: int | None = None) -> Reading:
    """Shuffle, cut, and lay three cards out.

    Seeded and returned with its seed, for the reason every long-running
    surface here is seeded (ADR 18): the reading outlives the request that
    produced it, and a spread that changed on reload would be a different
    reading of the same person.

    `random` rather than `secrets`: this is a shuffle, not a credential, and
    being able to reproduce it from an integer is the whole point.
    """
    rng = random.Random(seed) if seed is not None else random.Random()
    if seed is None:
        seed = rng.randrange(2**31)
        rng = random.Random(seed)
    drawn = rng.sample(DECK, len(SPREAD))
    return Reading(seed=seed, cards=tuple(
        Drawn(card=card, position=pos, reversed=rng.random() < 0.5)
        for card, pos in zip(drawn, SPREAD, strict=True)))
