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

**Three Magic cards are in the deck too** (punch list 2026-08-15 item 13),
because Magic has now printed three cards that *are* tarot cards: Flubs, the
Fool; Homer, the Hermit; and Massimo, the Magician. Each is a `Card` like
the 78 with three extra facts -- the Scryfall art crop it shows (hotlinked
with the artist credited, never committed; rule 5 and the persona tiles'
precedent), who painted it, and which trump it is printed `after`. That last
field is a fact about the card's design, not a meaning: this module still
interprets nothing, and the reader is simply told "a Magic card printed
after The Fool" and left to read it. They are weighted to land a little
more often than a natural card -- a wink is worthless if nobody ever sees
it -- which is why `deal` samples by weight now. All three were verified on
Scryfall 2026-08-15 (Bloomburrow Commander; Mystery Booster Commander
Edition, releasing 2026-11-09 -- spoiled cards are real cards, ADR 26 says
so), so none of this waits on the pool.
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
    """One card. `key` is both its identity and its picture's filename --
    except for a Magic crossover, whose picture is a hotlinked art crop and
    whose extra fields say so."""

    key: str
    name: str
    #: "major" or "minor". The reader is told which, because a spread of all
    #: trumps means something to a reader and nothing to a shuffler.
    arcana: str
    suit: str | None
    #: 0-21 for a trump, 1-14 within a suit.
    number: int
    #: The three crossover fields, all None for the 78: a Scryfall art crop
    #: (the art, not the full card -- the frame would make it a Magic card
    #: on a tarot table, and the wink works the other way around), the
    #: artist owed a credit line, and the trump the card is printed after.
    art_url: str | None = None
    artist: str | None = None
    after: str | None = None

    @property
    def image(self) -> str:
        return self.art_url or f"/tarot/{self.key}.webp"

    def as_dict(self) -> dict[str, object]:
        return {"key": self.key, "name": self.name, "arcana": self.arcana,
                "suit": self.suit, "number": self.number, "image": self.image,
                "artist": self.artist, "after": self.after}


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

#: The three Magic cards that are tarot cards, each carrying its trump's
#: number so a reader who cares about such things sees it sit where its
#: original sits. Names, artists and art verified on Scryfall 2026-08-15.
CROSSOVERS: tuple[Card, ...] = (
    Card("mtg-flubs-the-fool", "Flubs, the Fool", "major", None, 0,
         art_url="https://cards.scryfall.io/art_crop/front/4/1/"
                 "41e58eb8-e5b9-4ef6-be1f-00e28cebb998.jpg",
         artist="Adam Rex", after="The Fool"),
    Card("mtg-massimo-the-magician", "Massimo, the Magician", "major", None, 1,
         art_url="https://cards.scryfall.io/art_crop/front/b/e/"
                 "bed6b0c6-6e25-4129-9d11-3e80157ddb42.jpg",
         artist="Jodie Muir", after="The Magician"),
    Card("mtg-homer-the-hermit", "Homer, the Hermit", "major", None, 9,
         art_url="https://cards.scryfall.io/art_crop/front/d/9/"
                 "d9934129-11b4-4e91-b81c-9d8e5bb14523.jpg",
         artist="Christina Kraus", after="The Hermit"),
)

#: What actually gets shuffled: 81 cards. The crossovers are dealt at
#: `CROSSOVER_WEIGHT` against every natural card's 1 -- present often enough
#: to be found, rare enough to stay a surprise.
FULL_DECK: tuple[Card, ...] = DECK + CROSSOVERS

CROSSOVER_WEIGHT = 2.0

BY_KEY: dict[str, Card] = {c.key: c for c in FULL_DECK}


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
            + (f" — a Magic card printed after {d.card.after}, which you may "
               f"read as you would read {d.card.after}"
               if d.card.after else "")
            + (", reversed" if d.reversed else "")
            for d in self.cards)


def _weighted_sample(rng: random.Random, k: int) -> list[Card]:
    """`k` distinct cards from `FULL_DECK`, crossovers at their weight.

    `random.sample` has no weights and `random.choices` has no "without
    replacement", so this is the classic successive draw: pick against the
    remaining total, remove, repeat. Deterministic under a seeded `rng`,
    which is the property everything else here is built on.
    """
    pool = [(c, CROSSOVER_WEIGHT if c.after else 1.0) for c in FULL_DECK]
    out: list[Card] = []
    for _ in range(k):
        total = sum(w for _, w in pool)
        mark = rng.random() * total
        acc = 0.0
        for i, (_card, weight) in enumerate(pool):
            acc += weight
            if mark < acc:
                out.append(pool.pop(i)[0])
                break
        else:
            # Float summation can leave `mark` a hair past `acc`; the last
            # card was the answer.
            out.append(pool.pop()[0])
    return out


def deal(seed: int | None = None) -> Reading:
    """Shuffle, cut, and lay three cards out.

    Seeded and returned with its seed, for the reason every long-running
    surface here is seeded (ADR 18): the reading outlives the request that
    produced it, and a spread that changed on reload would be a different
    reading of the same person. (One deliberate break in that promise: the
    deal grew weights when the Magic crossovers joined the deck, so a seed
    stashed before that change re-deals differently once, after this
    deploys. A reading is of one person on one evening; an evening that
    spans a deploy gets a fresh one.)

    `random` rather than `secrets`: this is a shuffle, not a credential, and
    being able to reproduce it from an integer is the whole point.
    """
    rng = random.Random(seed) if seed is not None else random.Random()
    if seed is None:
        seed = rng.randrange(2**31)
        rng = random.Random(seed)
    drawn = _weighted_sample(rng, len(SPREAD))
    return Reading(seed=seed, cards=tuple(
        Drawn(card=card, position=pos, reversed=rng.random() < 0.5)
        for card, pos in zip(drawn, SPREAD, strict=True)))
