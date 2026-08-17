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

**Magic cards are in the deck too**, in two tiers (punch list 2026-08-15
item 13; the echoes tier added 2026-08-16 with Aaron). The **crossovers**
are the three cards Magic has printed that *are* tarot cards -- Flubs, the
Fool; Homer, the Hermit; Massimo, the Magician -- and a deep scan of the
pool on 2026-08-16 confirmed the cycle is exactly those three (a Scryfall
name-pattern search agreed). The **echoes** are seven real cards whose name
and art genuinely carry a trump -- the Alpha Wheel of Fortune above all,
which is also the painting this site's own Wheel spins -- each chosen by
looking at the art beside the 1909 scans, never from the name alone; the
rejects (a comic-styled Justice, a playtest-doodle Moon) are why that rule
exists. Every one is a `Card` like the 78 with extra facts: the Scryfall
art crop it shows (hotlinked with the artist credited, never committed;
rule 5 and the persona tiles' precedent), who painted it, which trump it
answers `after`, and its `weight` in the shuffle. `after` states a fact
about the card's design or name, not a meaning: this module still
interprets nothing, and the reader is told what kind of thing landed and
left to read it. A crossover is weighted to land a little more often than
a natural card -- a wink is worthless if nobody ever sees it -- and with
the echoes aboard, roughly every third reading now holds some Magic card,
which `describe` tells the reader to treat as an omen with precedence.
Rarest of all, one trump can land twice -- the 1909 printing and Magic's
own in one spread -- and `describe` names that alignment outright.
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
    #: The Magic-card fields, all None for the 78: a Scryfall art crop
    #: (the art, not the full card -- the frame would make it a Magic card
    #: on a tarot table, and the wink works the other way around), the
    #: artist owed a credit line, and the trump the card answers after.
    art_url: str | None = None
    artist: str | None = None
    after: str | None = None
    #: True for the echoes tier: a real card whose name and art carry a
    #: trump, as opposed to the three printed *as* tarot cards. The reader
    #: is told which kind landed, because "printed after The Fool" is a
    #: design fact and "answers to The Tower" is an editorial one.
    echo: bool = False
    #: How often it lands, against a natural card's 1.0.
    weight: float = 1.0

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

#: How often each Magic tier lands, against a natural card's 1.0. The three
#: printed tarot cards stay the most favoured; an echo is exactly as likely
#: as any natural card, so the tier thickens the winks without drowning the
#: 78 under them. The crossover weight came down from 2.0 when the echoes
#: joined (Aaron, 2026-08-16): ten Magic cards supply the presence the
#: weighting used to, so the thumb rests lighter on the scale.
CROSSOVER_WEIGHT = 1.5
ECHO_WEIGHT = 1.0

#: The three Magic cards that are tarot cards, each carrying its trump's
#: number so a reader who cares about such things sees it sit where its
#: original sits. Names, artists and art verified on Scryfall 2026-08-15,
#: and the cycle confirmed complete at three on 2026-08-16.
CROSSOVERS: tuple[Card, ...] = (
    Card("mtg-flubs-the-fool", "Flubs, the Fool", "major", None, 0,
         art_url="https://cards.scryfall.io/art_crop/front/4/1/"
                 "41e58eb8-e5b9-4ef6-be1f-00e28cebb998.jpg",
         artist="Adam Rex", after="The Fool", weight=CROSSOVER_WEIGHT),
    Card("mtg-massimo-the-magician", "Massimo, the Magician", "major", None, 1,
         art_url="https://cards.scryfall.io/art_crop/front/b/e/"
                 "bed6b0c6-6e25-4129-9d11-3e80157ddb42.jpg",
         artist="Jodie Muir", after="The Magician", weight=CROSSOVER_WEIGHT),
    Card("mtg-homer-the-hermit", "Homer, the Hermit", "major", None, 9,
         art_url="https://cards.scryfall.io/art_crop/front/d/9/"
                 "d9934129-11b4-4e91-b81c-9d8e5bb14523.jpg",
         artist="Christina Kraus", after="The Hermit",
         weight=CROSSOVER_WEIGHT),
)

#: The echoes: real Magic cards whose name and art carry a trump, chosen by
#: eye against the 1909 scans (deep scan with Aaron, 2026-08-16). Art and
#: artists come from the pool's rows; the Wheel deliberately uses Daniel
#: Gelon's Alpha painting -- the same one `components/wheel.tsx` spins, so
#: the card on the table and the wheel on the deck page are one image.
ECHOES: tuple[Card, ...] = (
    Card("mtg-empress-galina", "Empress Galina", "major", None, 3,
         art_url="https://cards.scryfall.io/art_crop/front/6/8/"
                 "6851dbc7-f072-41e7-a899-897445d99425.jpg",
         artist="Matt Cavotta", after="The Empress", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-emperor-apatzec", "Emperor Apatzec Intli IV", "major", None, 4,
         art_url="https://cards.scryfall.io/art_crop/front/b/6/"
                 "b6987c56-8bd5-40b6-8e4f-44d2dab801d6.jpg",
         artist="Johan Grenier", after="The Emperor", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-chariot-of-victory", "Chariot of Victory", "major", None, 7,
         art_url="https://cards.scryfall.io/art_crop/front/c/2/"
                 "c2dd03d2-05ef-4929-b973-3dbe49fc7592.jpg",
         artist="John Stanko", after="The Chariot", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-wheel-of-fortune", "Wheel of Fortune", "major", None, 10,
         art_url="https://cards.scryfall.io/art_crop/front/6/7/"
                 "67b369c4-faa8-45c8-a1b9-98f228b69682.jpg",
         artist="Daniel Gelon", after="Wheel of Fortune", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-tower-of-calamities", "Tower of Calamities", "major", None, 16,
         art_url="https://cards.scryfall.io/art_crop/front/8/a/"
                 "8a77391b-5727-4408-bb50-970f7a13a83c.jpg",
         artist="Aleksi Briclot", after="The Tower", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-imprisoned-in-the-moon", "Imprisoned in the Moon", "major",
         None, 18,
         art_url="https://cards.scryfall.io/art_crop/front/0/d/"
                 "0d150547-09f5-45ce-a825-89944b066bd4.jpg",
         artist="Ryan Alexander Lee", after="The Moon", echo=True,
         weight=ECHO_WEIGHT),
    Card("mtg-the-world-tree", "The World Tree", "major", None, 21,
         art_url="https://cards.scryfall.io/art_crop/front/a/7/"
                 "a70cb6d9-3955-4064-917b-11dec26440c5.jpg",
         artist="Anastasia Ovchinnikova", after="The World", echo=True,
         weight=ECHO_WEIGHT),
)

#: What actually gets shuffled: 88 cards. The Magic tiers are dealt at their
#: weights against every natural card's 1 -- present often enough to be
#: found, rare enough to stay a surprise.
FULL_DECK: tuple[Card, ...] = DECK + CROSSOVERS + ECHOES

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

        Two paragraphs can follow the cards, and both are Python-detected
        facts rather than interpretations (Aaron, 2026-08-16). A Magic card
        in the spread is called out as an omen the reader should give
        precedence -- the game the reading serves has walked into it. And a
        trump landing twice, once as the 1909 printing and once as Magic's
        own card, is named as the alignment it is: the sampler draws
        without replacement across all 88, so the two Fools really can
        share a table, and a reader who is not told how rare that is will
        read past it.
        """
        lines = []
        for d in self.cards:
            line = f"- {d.position.name} ({d.position.asks}): {d.card.name}"
            if d.card.after and d.card.echo:
                line += (f" — a real Magic card whose art and name answer to "
                         f"{d.card.after}; read it as {d.card.after} wearing "
                         f"Magic's own painting")
            elif d.card.after:
                line += (f" — a Magic card printed after {d.card.after}, "
                         f"which you may read as you would read "
                         f"{d.card.after}")
            if d.reversed:
                line += ", reversed"
            lines.append(line)
        text = "\n".join(lines)

        if any(d.card.after for d in self.cards):
            text += (
                "\n\nA Magic card on this table is an omen in its own right: "
                "the game this reading is in service of has walked into the "
                "spread. Give it precedence — let it colour the reading more "
                "than a natural card would, say so plainly, and make sure "
                "they feel how uncommon a visit it is.")

        by_trump: dict[int, list[str]] = {}
        for d in self.cards:
            if d.card.arcana == "major":
                by_trump.setdefault(d.card.number, []).append(d.card.name)
        for names in by_trump.values():
            if len(names) > 1:
                text += (
                    f"\n\nThe stars have aligned: one trump has landed twice "
                    f"at this table, as {' and '.join(names)}. That is the "
                    f"rarest thing this spread can do. Make it the centre of "
                    f"the reading, and tell them exactly what they are "
                    f"looking at.")
        return text


def _weighted_sample(rng: random.Random, k: int) -> list[Card]:
    """`k` distinct cards from `FULL_DECK`, crossovers at their weight.

    `random.sample` has no weights and `random.choices` has no "without
    replacement", so this is the classic successive draw: pick against the
    remaining total, remove, repeat. Deterministic under a seeded `rng`,
    which is the property everything else here is built on.
    """
    pool = [(c, c.weight) for c in FULL_DECK]
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
