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
item 13; the echoes tier added 2026-08-16 with Aaron, then widened twice at
his "get weird" and "leave no stone unturned"). The **crossovers** are the
three cards Magic has printed that *are* tarot cards -- Flubs, the Fool;
Homer, the Hermit; Massimo, the Magician -- and a deep scan of the pool on
2026-08-16 confirmed the cycle is exactly those three (a Scryfall
name-pattern search agreed). The **echoes** are twenty-nine real cards
whose name, art and rules genuinely carry a tarot card: every one of the
22 trumps is answered (the Alpha Wheel of Fortune above all, which is also
the painting this site's own Wheel spins), and the tier reaches into the
minors -- the aces, the coins, a page, a king. Each was chosen by looking
at the art beside the 1909 scans AND by its `note`: original imagery
outranks every other classifier (Aaron's rule -- Stone Rain's tower
mid-ruin beat a better-numbered tower that merely loomed), and a card that
cannot justify its slot in checkable facts is cut; the rejects (a
comic-styled Justice, a playtest-doodle Moon, Thor and Loki wearing a
Pacifism frame) are why both rules exist. Every one is a `Card` like the
78 with extra facts: the Scryfall
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
    #: Why this card holds its place — the resonance with its original,
    #: stated as checkable facts (a power of 0, a fourth of his name, a
    #: cost and a strike that sum to sixteen). Every Magic card carries
    #: one or it is cut, which is rule 4's discipline applied to this
    #: deck's own slots (Aaron, 2026-08-16). Rendered under the card and
    #: handed to the reader; still not a meaning.
    note: str | None = None

    @property
    def image(self) -> str:
        return self.art_url or f"/tarot/{self.key}.webp"

    def as_dict(self) -> dict[str, object]:
        return {"key": self.key, "name": self.name, "arcana": self.arcana,
                "suit": self.suit, "number": self.number, "image": self.image,
                "artist": self.artist, "after": self.after, "note": self.note}


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
#: printed tarot cards stay the most favoured; an echo is *rarer* than a
#: natural card since the deep dives widened the tier (Aaron, 2026-08-16)
#: -- twenty-nine of them supply the presence now, every trump covered and
#: the suits opened, so each member's weight drops as the roster grows to
#: keep the whole tier landing about every third reading.
CROSSOVER_WEIGHT = 1.5
ECHO_WEIGHT = 0.25

#: The three Magic cards that are tarot cards, each carrying its trump's
#: number so a reader who cares about such things sees it sit where its
#: original sits. Names, artists and art verified on Scryfall 2026-08-15,
#: and the cycle confirmed complete at three on 2026-08-16. Every `note`
#: below, on every tier, states only facts checked against the pool row on
#: 2026-08-16 -- a resonance a reader can verify is a wink; one they cannot
#: is a lie with candles on it.
CROSSOVERS: tuple[Card, ...] = (
    Card("mtg-flubs-the-fool", "Flubs, the Fool", "major", None, 0,
         art_url="https://cards.scryfall.io/art_crop/front/4/1/"
                 "41e58eb8-e5b9-4ef6-be1f-00e28cebb998.jpg",
         artist="Adam Rex", after="The Fool", weight=CROSSOVER_WEIGHT,
         note="The Fool is trump 0, and Flubs has exactly 0 power — he "
              "walks his cliff edge drawing best with empty hands, and his "
              "flavor text knows exactly where he's going and exactly how "
              "he isn't getting there."),
    Card("mtg-massimo-the-magician", "Massimo, the Magician", "major", None, 1,
         art_url="https://cards.scryfall.io/art_crop/front/b/e/"
                 "bed6b0c6-6e25-4129-9d11-3e80157ddb42.jpg",
         artist="Jodie Muir", after="The Magician", weight=CROSSOVER_WEIGHT,
         note="The Magician is trump I, and Massimo conjures only spells "
              "of mana value exactly 1 — raising them from the graveyard "
              "the way the trump draws power from below, wand high over "
              "his table of cup and sword."),
    Card("mtg-homer-the-hermit", "Homer, the Hermit", "major", None, 9,
         art_url="https://cards.scryfall.io/art_crop/front/d/9/"
                 "d9934129-11b4-4e91-b81c-9d8e5bb14523.jpg",
         artist="Christina Kraus", after="The Hermit",
         weight=CROSSOVER_WEIGHT,
         note="The Hermit is trump IX, and Homer is a 0/9 — no power, "
              "all withdrawal — a hermit crab by type, carrying a lantern "
              "of pearls, whose flavor text requires no destination, just "
              "a direction."),
)

#: The echoes: real Magic cards whose name, art and rules genuinely carry a
#: tarot card, found in two deep dives (2026-08-16, the second at Aaron's
#: "get weird"). Chosen by eye against the 1909 scans AND by note: every
#: entry justifies its slot with checkable resonances or it was cut -- the
#: rejects (a comic-book Justice, a playtest-doodle Moon, a Cait Sith too
#: cute to sit beside 1909 cardboard) are why both rules exist. Art and
#: artists come from the pool's rows; the Wheel deliberately uses Daniel
#: Gelon's Alpha painting -- the same one `components/wheel.tsx` spins, so
#: the card on the table and the wheel on the deck page are one image. Two
#: echoes reach into the minor arcana (the aces), which is why the tier is
#: keyed on suit as well as number.
ECHOES: tuple[Card, ...] = (
    Card("mtg-blind-seer", "Blind Seer", "major", None, 2,
         art_url="https://cards.scryfall.io/art_crop/front/5/c/"
                 "5c54ec26-c7f1-4258-9cc9-1709987f293c.jpg",
         artist="Dave Dorman", after="The High Priestess", echo=True,
         weight=ECHO_WEIGHT,
         note="The High Priestess keeps knowledge behind a veil, and the "
              "Blind Seer is Urza himself behind blindfold and disguise — "
              "his flavor text suspects he sees more than he lets on."),
    Card("mtg-empress-galina", "Empress Galina", "major", None, 3,
         art_url="https://cards.scryfall.io/art_crop/front/6/8/"
                 "6851dbc7-f072-41e7-a899-897445d99425.jpg",
         artist="Matt Cavotta", after="The Empress", echo=True,
         weight=ECHO_WEIGHT,
         note="The Empress is trump III, and Galina is a 1/3 with a 3 in "
              "her cost — enthroned with her scepter, claiming every "
              "crowned legend who strays into her waters."),
    Card("mtg-emperor-apatzec", "Emperor Apatzec Intli IV", "major", None, 4,
         art_url="https://cards.scryfall.io/art_crop/front/b/6/"
                 "b6987c56-8bd5-40b6-8e4f-44d2dab801d6.jpg",
         artist="Johan Grenier", after="The Emperor", echo=True,
         weight=ECHO_WEIGHT,
         note="The Emperor is trump IV, and Apatzec is the fourth of his "
              "name — his rules text says the number 4 four times over: "
              "power 4, toughness 4, 4 life, mana value 4."),
    Card("mtg-orzhov-pontiff", "Orzhov Pontiff", "major", None, 5,
         art_url="https://cards.scryfall.io/art_crop/front/3/e/"
                 "3e36323c-75d0-475e-a5a7-9a1567ff2b62.jpg",
         artist="Adam Rex", after="The Hierophant", echo=True,
         weight=ECHO_WEIGHT,
         note="The Hierophant blesses from his throne, and the Pontiff "
              "blesses one congregation or damns the other — and when he "
              "dies his office passes on, haunting a successor. Painted "
              "by Adam Rex, who also painted Flubs."),
    Card("mtg-kynaios-and-tiro", "Kynaios and Tiro of Meletis", "major",
         None, 6,
         art_url="https://cards.scryfall.io/art_crop/front/9/7/"
                 "97fa8615-2b6c-445a-bcaf-44a7e847bf65.jpg",
         artist="Willian Murai", after="The Lovers", echo=True,
         weight=ECHO_WEIGHT,
         note="The Lovers are two figures under a blessing light, and "
              "Theros printed them: two kings, one card, a light between "
              "colossi above their city, whose text gives gifts to the "
              "whole table — look what we fought for, look what we built "
              "together."),
    Card("mtg-esikas-chariot", "Esika's Chariot", "major", None, 7,
         art_url="https://cards.scryfall.io/art_crop/front/a/8/"
                 "a87606cc-fbf0-4e2c-9798-f1c935d0573d.jpg",
         artist="Raoul Vitale", after="The Chariot", echo=True,
         weight=ECHO_WEIGHT,
         note="The Chariot is drawn by two sphinxes — a feline pair, one "
              "light, one dark — and Kaldheim printed the goddess "
              "Freyja's own answer: a chariot drawn by great cats, whose "
              "rules text creates exactly two Cats the moment it "
              "arrives."),
    Card("mtg-lion-umbra", "Lion Umbra", "major", None, 8,
         art_url="https://cards.scryfall.io/art_crop/front/8/9/"
                 "89d58e9b-b1d9-4174-a30d-426d2e0ace07.jpg",
         artist="Julia Metzger", after="Strength", echo=True,
         weight=ECHO_WEIGHT,
         note="Strength is a figure closing a lion's mouth with bare "
              "gentle hands, and this umbra is the lion itself wrapped "
              "willing around its person — umbra armor takes the blow "
              "that would have destroyed them, which is strength as "
              "protection rather than force."),
    Card("mtg-wheel-of-fortune", "Wheel of Fortune", "major", None, 10,
         art_url="https://cards.scryfall.io/art_crop/front/6/7/"
                 "67b369c4-faa8-45c8-a1b9-98f228b69682.jpg",
         artist="Daniel Gelon", after="Wheel of Fortune", echo=True,
         weight=ECHO_WEIGHT,
         note="Magic printed the trump outright in its very first set: one "
              "spin and every player's fortune — rich hand or poor — is "
              "thrown over for a fresh seven. Gelon's painted wheel is "
              "the same one that spins on this site's deck pages."),
    Card("mtg-balance", "Balance", "major", None, 11,
         art_url="https://cards.scryfall.io/art_crop/front/c/e/"
                 "ce648aa3-098b-4af0-a433-fd290bc85904.jpg",
         artist="Kev Walker", after="Justice", echo=True,
         weight=ECHO_WEIGHT,
         note="Justice holds a sword in one hand and scales in the other, "
              "and Kev Walker painted exactly that figure — a card that "
              "levels every player's lands, hand and creatures down to "
              "equal, which is the scales made rules text."),
    Card("mtg-hanged-executioner", "Hanged Executioner", "major", None, 12,
         art_url="https://cards.scryfall.io/art_crop/front/7/6/"
                 "7676deeb-b80d-49c9-b810-5ed0a68fa618.jpg",
         artist="Johann Bodin", after="The Hanged Man", echo=True,
         weight=ECHO_WEIGHT,
         note="The Hanged Man hangs serene between sky and earth, and "
              "this spirit hangs at its own gallows — flying, suspended, "
              "and willing to give itself up to pass judgment."),
    Card("mtg-pale-rider", "Pale Rider of Trostad", "major", None, 13,
         art_url="https://cards.scryfall.io/art_crop/front/8/d/"
                 "8dad70b8-0dec-4634-a31a-b78438a313a2.jpg",
         artist="Seb McKinnon", after="Death", echo=True,
         weight=ECHO_WEIGHT,
         note="Trump XIII rides a pale horse, as Death has since "
              "Revelation named the rider — and skulk means no greater "
              "power can block this one, which is the trump's whole "
              "sermon: kings fall before it too."),
    Card("mtg-alchemists-apprentice", "Alchemist's Apprentice", "major",
         None, 14,
         art_url="https://cards.scryfall.io/art_crop/front/3/1/"
                 "31abba67-1241-4fb3-88b5-4c4668ec5f25.jpg",
         artist="David Palumbo", after="Temperance", echo=True,
         weight=ECHO_WEIGHT,
         note="Temperance pours one vessel into another, blending what "
              "should not mix — and this apprentice is caught mid-pour, "
              "one measure falling into the flask, the whole art of "
              "moderation one drop from going wrong."),
    Card("mtg-master-of-cruelties", "Master of Cruelties", "major", None, 15,
         art_url="https://cards.scryfall.io/art_crop/front/f/f/"
                 "ffd68fe1-5cfc-44cf-8dfe-3488278cdcef.jpg",
         artist="Chase Stone", after="The Devil", echo=True,
         weight=ECHO_WEIGHT,
         note="The Devil keeps his captives chained but alive, and this "
              "one attacks alone and leaves its victim at exactly 1 life "
              "— bondage, never release — horned above the twisted "
              "figures crawling at his steps."),
    Card("mtg-stone-rain", "Stone Rain", "major", None, 16,
         art_url="https://cards.scryfall.io/art_crop/front/d/2/"
                 "d2334c10-fa96-4f8e-8187-c7ecc00cbac8.jpg",
         artist="John Matson", after="The Tower", echo=True,
         weight=ECHO_WEIGHT,
         note="The Tower is a crown struck from a turret by fire out of "
              "the sky, and Magic's oldest land-destruction spell paints "
              "the very act — a castle tower under a burning rain, walls "
              "already going. It has been knocking towers down since the "
              "game's first set."),
    Card("mtg-starfield-of-nyx", "Starfield of Nyx", "major", None, 17,
         art_url="https://cards.scryfall.io/art_crop/front/b/f/"
                 "bfe4e1cb-ceee-46aa-96d1-f3f26516cd77.jpg",
         artist="Tyler Jacobson", after="The Star", echo=True,
         weight=ECHO_WEIGHT,
         note="The Star is the hope that follows the Tower — trump XVII "
              "after XVI — and this sky returns what was lost from the "
              "graveyard, until the constellations themselves step down "
              "and walk."),
    Card("mtg-imprisoned-in-the-moon", "Imprisoned in the Moon", "major",
         None, 18,
         art_url="https://cards.scryfall.io/art_crop/front/0/d/"
                 "0d150547-09f5-45ce-a825-89944b066bd4.jpg",
         artist="Ryan Alexander Lee", after="The Moon", echo=True,
         weight=ECHO_WEIGHT,
         note="The Moon is the trump of illusion and unease, and "
              "Innistrad's silver moon is literally a prison — only one "
              "vault was great enough to hold Emrakul, says the flavor "
              "text, and there she waits."),
    Card("mtg-approach-second-sun", "Approach of the Second Sun", "major",
         None, 19,
         art_url="https://cards.scryfall.io/art_crop/front/f/d/"
                 "fdf59a6e-7708-45a1-884d-d12e9f7b9ed9.jpg",
         artist="Noah Bradley", after="The Sun", echo=True,
         weight=ECHO_WEIGHT,
         note="The Sun is the deck's one unclouded yes, and no Magic card "
              "is more optimistic: gain 7 life, set it seventh from the "
              "top, and when the sun comes round a second time you simply "
              "win the game."),
    Card("mtg-angelic-renewal", "Angelic Renewal", "major", None, 20,
         art_url="https://cards.scryfall.io/art_crop/front/a/0/"
                 "a03fc1f1-31f6-4e52-87d0-e3d18ea60d3b.jpg",
         artist="Rebecca Guay", after="Judgement", echo=True,
         weight=ECHO_WEIGHT,
         note="Judgement is the call that raises the dead, and Rebecca "
              "Guay — no Magic artist ever painted closer to the tarot — "
              "gave it an angel lifting a mortal into the stars: when "
              "death comes, the angel refuses it, and the fallen return "
              "to the field."),
    Card("mtg-the-world-tree", "The World Tree", "major", None, 21,
         art_url="https://cards.scryfall.io/art_crop/front/a/7/"
                 "a70cb6d9-3955-4064-917b-11dec26440c5.jpg",
         artist="Anastasia Ovchinnikova", after="The World", echo=True,
         weight=ECHO_WEIGHT,
         note="The World is the last trump, the journey complete — and "
              "when the whole world is assembled, this tree turns every "
              "land into every color and calls all the gods home at "
              "once."),
    Card("mtg-wand-of-the-worldsoul", "Wand of the Worldsoul", "minor",
         "wands", 1,
         art_url="https://cards.scryfall.io/art_crop/front/3/e/"
                 "3ef9fb7d-1fd5-4dce-afda-6743dec3bbc1.jpg",
         artist="Alessandra Pisano", after="the Ace of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ace of Wands is a hand raising a branch still in leaf "
              "— and this wand is painted mid-blossom in a lifted hand, "
              "named for the soul of the world, gathering the whole "
              "community into every spell it kindles."),
    Card("mtg-chart-a-course", "Chart a Course", "minor", "wands", 2,
         art_url="https://cards.scryfall.io/art_crop/front/7/5/"
                 "7599d459-fe71-48f4-8c65-0f519bb63a68.jpg",
         artist="James Ryman", after="the Two of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Two of Wands holds a globe and plans the voyage — and "
              "this captain stands over his charts and orrery doing "
              "exactly that. The card even draws exactly two."),
    Card("mtg-everflowing-chalice", "Everflowing Chalice", "minor", "cups", 1,
         art_url="https://cards.scryfall.io/art_crop/front/e/4/"
                 "e4ed0052-d6dd-4f69-8313-10863baefac9.jpg",
         artist="Steve Argyle", after="the Ace of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ace of Cups overflows — and this chalice is named for "
              "it, kicked any number of times, filling without limit "
              "from the moment it costs nothing at all to place on the "
              "table."),
    Card("mtg-tragic-poet", "Tragic Poet", "minor", "cups", 11,
         art_url="https://cards.scryfall.io/art_crop/front/f/9/"
                 "f957b353-7765-4c16-9645-d41000154130.jpg",
         artist="Anthony Palumbo", after="the Page of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Page of Cups is the suit's young dreamer, and this "
              "poet writes under her tree with a golden quill — the same "
              "instrument this reading is written with — in a healing "
              "world, as her flavor text says, that will never heal "
              "her."),
    Card("mtg-sword-truth-justice", "Sword of Truth and Justice", "minor",
         "swords", 1,
         art_url="https://cards.scryfall.io/art_crop/front/2/b/"
                 "2be8d24e-1370-4e85-90f2-66b6d6e9c4a4.jpg",
         artist="Chris Rahn", after="the Ace of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ace of Swords is a hand from the clouds raising one "
              "blade — truth's first cut — and Chris Rahn painted that "
              "exact gauntlet, on a sword named for the ace's own "
              "meanings, whose counters grow from one into many."),
    Card("mtg-sram-senior-edificer", "Sram, Senior Edificer", "minor",
         "pentacles", 3,
         art_url="https://cards.scryfall.io/art_crop/front/8/f/"
                 "8fbd18ce-0ac3-4b52-9cd0-0af7e0244207.jpg",
         artist="Chris Rahn", after="the Three of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Three of Pentacles is the master craftsman at his "
              "work, admired — and Sram leans over a golden model of his "
              "city, a senior edificer by title, rewarded for every "
              "made thing that passes through his hands."),
    Card("mtg-dragons-hoard", "Dragon's Hoard", "minor", "pentacles", 4,
         art_url="https://cards.scryfall.io/art_crop/front/3/e/"
                 "3ee80876-6e87-46c4-a6fd-ff6752db3032.jpg",
         artist="Adam Paquette", after="the Four of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Four of Pentacles clutches its coins and will not "
              "spend them — and here is a mountain of gold in the dark, "
              "gathering a counter for every dragon that comes home to "
              "sit on it."),
    Card("mtg-smothering-tithe", "Smothering Tithe", "minor", "pentacles", 5,
         art_url="https://cards.scryfall.io/art_crop/front/8/6/"
                 "861b5889-0183-4bee-afeb-a4b2aa700a8e.jpg",
         artist="Mark Behm", after="the Five of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Five of Pentacles is want at the church door, and "
              "this man in the Orzhov cathedral's shadow bleeds gold he "
              "cannot keep — the church's priest awaits your donation, "
              "says the flavor text, and the tithe takes its coin from "
              "every hand."),
    Card("mtg-alms-collector", "Alms Collector", "minor", "pentacles", 6,
         art_url="https://cards.scryfall.io/art_crop/front/3/6/"
                 "367ac07a-a30f-4ebf-8877-27cd9ebe2f71.jpg",
         artist="Bram Sels", after="the Six of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Six of Pentacles weighs out wealth so everyone gets a "
              "share — and this cleric's own rules text does it: where "
              "one player would draw two, both draw one instead. There "
              "is no justice, his flavor text says, when some profit and "
              "others go without."),
    Card("mtg-king-macar", "King Macar, the Gold-Cursed", "minor",
         "pentacles", 14,
         art_url="https://cards.scryfall.io/art_crop/front/f/f/"
                 "ff5987ab-570a-426c-ae4a-a270fac6b346.jpg",
         artist="Greg Staples", after="the King of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The King of Pentacles sits enthroned amid his riches, and "
              "Theros printed Midas himself: everything King Macar loved "
              "is gold now — the golden figure behind his throne "
              "included — and his touch still turns whatever it exiles "
              "to gold."),
)

#: What actually gets shuffled: 110 cards -- every trump answered by a
#: Magic card, and the suits opening one number at a time. The Magic tiers
#: are dealt at their weights against every natural card's 1 -- present
#: often enough to be found, rare enough to stay a surprise.
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

        # Keyed on suit as well as number since the echoes reached into the
        # minors: the natural Ace of Swords and its Magic answer aligning is
        # every bit the event the two Fools are.
        by_trump: dict[tuple[str, str | None, int], list[str]] = {}
        for d in self.cards:
            key = (d.card.arcana, d.card.suit, d.card.number)
            by_trump.setdefault(key, []).append(d.card.name)
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
