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
name-pattern search agreed). The **echoes** are fifty-five real cards
whose name, art and rules genuinely carry a tarot card: every one of the
22 trumps is answered (the Alpha Wheel of Fortune above all, which is also
the painting this site's own Wheel spins), and the tier reaches into the
minors -- all four aces, the coins, the courts, and after two rounds of
Aaron's verdicts (nine minors on 2026-08-17, then seventeen more the same
day) thirty-six of the fifty-six. Each was chosen by looking at the art
beside the 1909 scans AND
by its `note`: original imagery outranks every other classifier (Aaron's
rule), and a card that cannot justify its slot in checkable facts is cut;
the rejects (a comic-styled Justice, a playtest-doodle Moon, Thor and Loki
wearing a Pacifism frame) are why both rules exist.

**The rubric widened on 2026-08-17**, at Aaron's "expand your associations":
art still outranks, but a slot may also be won on the card's *name*, its
*rules text*, or its place in the game -- Command Tower answers The Tower
on all four at once, and Murder holds the Ten of Swords on three of them
with art that merely does not embarrass it. What did not widen is the
evidence: a `note` states facts checked against the pool row, and a
resonance a reader can verify is a wink while one they cannot is a lie with
candles on it. That session also settled the **printing** question -- the
pool's default printing is not always the right painting, so Command Tower,
Young Pyromancer, Thassa, Murder and -- since 2026-08-17 -- Murderous Rider
each hotlink a chosen one. The Rider is the clearest case the rule has: Josh
Hass's default is a colour painting of a green-faced Zombie Knight, while
Ravenna Tran's Eldraine showcase is pen and ink, and under the ageing filter
it reads as though it came off the same press as the 1909 plate. Every one is
a `Card` like the
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

**Suit colour is a tiebreaker and not a law** (Aaron, 2026-08-17). The
correspondence is real -- wands/fire to red, cups/water to blue,
swords/air to white, pentacles/earth to green, which leaves black to the
trumps -- and it breaks a tie between two candidates that are otherwise
level. It never outranks the picture or the fact, and the roster is the
evidence: round two seated four white cards in the fire suit and no white
card at all in the suit of air, because in every one of those slots the
painting and the rules text pointed somewhere the colour pie did not. Do
not "correct" a card into its suit's colour -- if the colour is the best
argument a candidate has, it is not a good enough candidate.
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

    @property
    def face_name(self) -> str:
        """What is hand-set under the picture, which is not always `name`.

        Three echoes are double-faced cards, and `name` is the pool's name
        for the whole card -- "Murderous Rider // Swift End". That form is
        right in `describe`, where the reader is being told what the card
        *is* and the second half is half the resonance, and wrong on the
        card face, where it is two names and a piece of punctuation in
        12px small caps on a 1909 plate. The front face is the picture, so
        the front face is the caption.
        """
        return self.name.split(" // ")[0]

    def as_dict(self) -> dict[str, object]:
        return {"key": self.key, "name": self.name, "arcana": self.arcana,
                "suit": self.suit, "number": self.number, "image": self.image,
                "artist": self.artist, "after": self.after, "note": self.note,
                "face_name": self.face_name}


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
#: -- fifty-five of them supply the presence now, every trump covered and
#: the suits opened, so each member's weight drops as the roster grows to
#: keep the whole tier landing about every third reading. It came down
#: again from 0.25 with Aaron's 2026-08-17 verdicts, which added nine
#: minors: the landing rate is the constant, the weight is what moves.
#: Round two of the minors took the tier to fifty-five, and the weight came
#: down again to hold that constant -- 38 echoes at 0.20 put a Magic card in
#: 35.5% of spreads, and 55 at 0.14 put one in 35.7%, measured over 40,000
#: seeded deals rather than reasoned about. Re-measure it whenever the
#: roster grows; the arithmetic is not linear and guessing has been wrong.
CROSSOVER_WEIGHT = 1.5
ECHO_WEIGHT = 0.14

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
#: tarot card, found in three deep dives (2026-08-16 twice, at Aaron's "get
#: weird" and "leave no stone unturned"; 2026-08-17 at his verdicts, which
#: swapped eight trumps and opened nine more minors). Chosen by eye against
#: the 1909 scans AND by note: every entry justifies its slot with
#: checkable resonances or it was cut -- the rejects (a comic-book Justice,
#: a playtest-doodle Moon, a Cait Sith too cute to sit beside 1909
#: cardboard) are why both rules exist. Art and artists come from the
#: pool's rows *unless a printing was chosen deliberately* -- the Wheel uses
#: Daniel Gelon's Alpha painting, the same one `components/wheel.tsx` spins,
#: so the card on the table and the wheel on the deck page are one image,
#: and five more name their printing because the pool's default was the
#: wrong picture. Thirty-six echoes now reach into the minor arcana, which is
#: why the tier is keyed on suit as well as number.
ECHOES: tuple[Card, ...] = (
    Card("mtg-willow-priestess", "Willow Priestess", "major", None, 2,
         art_url="https://cards.scryfall.io/art_crop/front/3/1/"
                 "31479296-5ea7-470c-9d3f-257e67844fbc.jpg",
         artist="Susan Van Camp", after="The High Priestess", echo=True,
         weight=ECHO_WEIGHT,
         note="The High Priestess keeps what she knows behind a veil, and "
              "Homelands printed a priestess who does the same — she puts "
              "Faerie permanents onto the battlefield straight out of her "
              "hand, the hidden thing brought into the world without "
              "paying for it. Her flavor text is scripture quoted from her "
              "own goddess: \"Those of faith are those of strength.\" "
              "—Autumn Willow"),
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
    Card("mtg-true-loves-kiss", "True Love's Kiss", "major", None, 6,
         art_url="https://cards.scryfall.io/art_crop/front/2/3/"
                 "23a4bac2-f6cb-4712-8510-a63657c43a5c.jpg",
         artist="Donato Giancola", after="The Lovers", echo=True,
         weight=ECHO_WEIGHT,
         note="The Lovers stand under a blessing with a serpent in the "
              "tree behind them, and Eldraine printed the fairy tale's own "
              "answer to a curse: true love's kiss exiles an enchantment "
              "outright. The trump's warning survives in the flavor text — "
              "\"Be careful, dear. Some people deserve their curses.\""),
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
    Card("mtg-suspension-field", "Suspension Field", "major", None, 12,
         art_url="https://cards.scryfall.io/art_crop/front/b/a/"
                 "ba5c9628-1801-43d9-8bb4-4cca168510b2.jpg",
         artist="Seb McKinnon", after="The Hanged Man", echo=True,
         weight=ECHO_WEIGHT,
         note="The Hanged Man hangs serene between sky and earth, and Seb "
              "McKinnon painted a figure held in the air with its arms "
              "open inside a ring of light. The card exiles a creature "
              "until the field leaves the battlefield and then hands it "
              "back to its owner — time stopped, and nothing lost."),
    Card("mtg-murderous-rider", "Murderous Rider // Swift End", "major",
         None, 13,
         art_url="https://cards.scryfall.io/art_crop/front/4/9/"
                 "49c98e70-e8fe-4fea-b1d0-e7560780fda9.jpg",
         artist="Ravenna Tran", after="Death", echo=True,
         weight=ECHO_WEIGHT,
         note="Trump XIII is an armoured skeleton riding at a walk, and "
              "this is a Zombie Knight on a warhorse whose other half is "
              "an instant called Swift End: destroy target creature or "
              "planeswalker, and pay two life for it. It has lifelink — "
              "what it takes, it gives you — and when it dies it goes to "
              "the bottom of the library rather than the graveyard. Death "
              "goes back into the deck and comes round again."),
    Card("mtg-chalice-of-life", "Chalice of Life // Chalice of Death",
         "major", None, 14,
         art_url="https://cards.scryfall.io/art_crop/front/e/4/"
                 "e432b156-baf0-48a1-b8fb-1aa18bfbf7de.jpg",
         # Settled 2026-08-17: Aaron kept the Chalice over Angel of
         # Serenity, which had been offered as the more striking picture.
         # The Chalice has the better fact and its monochrome suits the
         # 1909 stock; the report that it read as a grey blob was a
         # screenshot of the folded 96px strip, not of the card. Do not
         # reopen this on the strength of another thumbnail.
         artist="Ryan Yee", after="Temperance", echo=True,
         weight=ECHO_WEIGHT,
         note="Temperance pours between two cups, and this is two cups on "
              "one card. The Chalice of Life gives a life at a time — and "
              "the moment you hold ten more than you started with, it "
              "transforms into the Chalice of Death. Excess tips the cup, "
              "which is the trump's entire warning printed as a trigger."),
    Card("mtg-asmodeus-the-archfiend", "Asmodeus the Archfiend", "major",
         None, 15,
         art_url="https://cards.scryfall.io/art_crop/front/a/5/"
                 "a5e6b864-58e7-43b9-9d79-1d0361340960.jpg",
         artist="Aleksi Briclot", after="The Devil", echo=True,
         weight=ECHO_WEIGHT,
         note="The Devil sits enthroned above two captives whose chains "
              "are loose enough to lift off. Magic's answer has the type "
              "line Devil God, costs six mana for a 6/6, and its ability "
              "is called Binding Contract: every card you would draw is "
              "taken and held face down, and you buy them all back by "
              "paying a life for each one."),
    Card("mtg-command-tower", "Command Tower", "major", None, 16,
         art_url="https://cards.scryfall.io/art_crop/front/e/c/"
                 "ec1f1041-f667-4b73-b1f2-e5bcae84095e.jpg",
         artist="Evan Shipard", after="The Tower", echo=True,
         weight=ECHO_WEIGHT,
         note="The Tower is a crown struck from a turret by fire out of "
              "the sky, and Evan Shipard painted precisely that — forked "
              "lightning around the battlements, the sky behind it "
              "burning. It is the land written for this format — it taps "
              "for any colour in your commander's identity — and it has "
              "been reprinted in more than fifty different sets, which is "
              "its own kind of monument."),
    Card("mtg-ephara", "Ephara, God of the Polis", "major", None, 17,
         art_url="https://cards.scryfall.io/art_crop/front/6/8/"
                 "6832e495-7ee9-43e0-94ea-03c88344080e.jpg",
         artist="Eric Deschamps", after="The Star", echo=True,
         weight=ECHO_WEIGHT,
         note="The Star is the water-bearer's card, a figure kneeling to "
              "pour under eight stars — and Ephara stands over her city "
              "tipping an urn of starlit water into it. She is "
              "indestructible and yet not a creature at all until seven "
              "of her people stand with her: hope you can see and cannot "
              "touch until the city gathers."),
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
    Card("mtg-expedition-map", "Expedition Map", "minor", "wands", 2,
         art_url="https://cards.scryfall.io/art_crop/front/0/8/"
                 "08e66835-c228-48fa-bcaa-eb96edbd4f5a.jpg",
         artist="Franz Vohwinkel", after="the Two of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Two of Wands holds a globe on the battlements and "
              "plans the voyage. This is a map whose whole text is "
              "reaching ground you have not stood on: sacrifice it, search "
              "your library, and the place you were looking at is in your "
              "hand."),
    Card("mtg-goblin-gathering", "Goblin Gathering", "minor", "wands", 5,
         art_url="https://cards.scryfall.io/art_crop/front/1/4/"
                 "147bef05-4497-44d5-9dd6-fb5dc08e78f7.jpg",
         artist="Svetlin Velinov", after="the Five of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Five of Wands is five youths swinging staves at each "
              "other with nobody much hurt. This one makes two goblins, "
              "and one more for every copy of itself already in the "
              "graveyard — the same brawl, bigger each time it happens. "
              "\"Two's a party. Three's a felony.\""),
    Card("mtg-darling-of-the-masses", "Darling of the Masses", "minor",
         "wands", 6,
         art_url="https://cards.scryfall.io/art_crop/front/5/2/"
                 "52aa9815-6b47-4c80-87c7-4277166ea0df.jpg",
         artist="Mila Pesic", after="the Six of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Six of Wands is the victory procession — the rider "
              "crowned with laurel and the crowd pressed in around the "
              "horse. Mila Pesic painted the same parade: a woman riding "
              "through falling petals over a crowd of silhouettes. The "
              "flavor text names it outright — \"The people flocked to the "
              "glittering parade, turning their backs on the threats in "
              "the shadows\" — and every attack she makes puts one more "
              "Citizen in behind her."),
    Card("mtg-high-ground", "High Ground", "minor", "wands", 7,
         art_url="https://cards.scryfall.io/art_crop/front/b/7/"
                 "b79b3153-8874-458d-8e7e-cf97c6c1887c.jpg",
         artist="rk post", after="the Seven of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Seven of Wands is one figure on higher ground holding "
              "off six staves coming up at him, and this is that hill "
              "painted — the defenders above, the attack below. Its whole "
              "rules text is the advantage of standing where you stand: "
              "each creature you control can block an additional creature. "
              "The flavor text is the trump in one line — \"In war, as in "
              "society, position is everything.\""),
    Card("mtg-hail-of-arrows", "Hail of Arrows", "minor", "wands", 8,
         art_url="https://cards.scryfall.io/art_crop/front/7/9/"
                 "797b0779-8a2c-4f24-9447-f039ac9d6aa5.jpg",
         artist="Anthony S. Waters", after="the Eight of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Eight of Wands is the one card in the deck with nobody "
              "in it: eight staves in flight over open country, already "
              "loosed and not yet landed. This one deals X damage divided "
              "as you choose among any number of attacking creatures — "
              "many shafts, many marks, one release — and General Takeno "
              "counts them the same way: \"Do not let a single shaft loose "
              "until my word.\""),
    Card("mtg-burdened-stoneback", "Burdened Stoneback", "minor", "wands", 10,
         art_url="https://cards.scryfall.io/art_crop/front/3/2/"
                 "3278b8d0-3d2b-4d3d-bbf1-fd9b714b53ed.jpg",
         artist="Carl Critchlow", after="the Ten of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ten of Wands is a man bent double under ten staves he "
              "insisted on carrying by himself. This Giant is painted "
              "under a load of stone spires and is printed a 4/4 that "
              "*enters with two -1/-1 counters* — it arrives already "
              "smaller for what it is carrying. Spend one of those "
              "counters and another creature gains indestructible: the "
              "weight comes off only to spare somebody else."),
    Card("mtg-young-pyromancer", "Young Pyromancer", "minor", "wands", 11,
         art_url="https://cards.scryfall.io/art_crop/front/e/3/"
                 "e349c204-3a93-4bf7-b79a-5f5f261ea2d3.jpg",
         artist="Cynthia Sheppard", after="the Page of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Page of Wands is a youth looking up at a staff that "
              "has just come into leaf. Cynthia Sheppard painted the same "
              "moment in fire — a young shaman turning a flame over in an "
              "open hand — and every instant or sorcery cast leaves "
              "another 1/1 Elemental standing there."),
    Card("mtg-hellrider", "Hellrider", "minor", "wands", 12,
         art_url="https://cards.scryfall.io/art_crop/front/7/b/"
                 "7bbfd905-8c71-4389-9174-6e84bcbcf05c.jpg",
         artist="Svetlin Velinov", after="the Knight of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Knight of Wands charges on a rearing horse in a "
              "salamander-figured tabard. Magic's is a Devil on one, with "
              "haste, and every creature that attacks alongside it burns "
              "the defender for one more — the cavalry charge written as "
              "a trigger."),
    Card("mtg-queen-marchesa", "Queen Marchesa", "minor", "wands", 13,
         art_url="https://cards.scryfall.io/art_crop/front/0/f/"
                 "0fdae05f-7bdc-45fb-b9b9-e5ec3766f965.jpg",
         artist="Kieran Yanner", after="the Queen of Wands", echo=True,
         weight=ECHO_WEIGHT,
         note="The Queen of Wands sits facing you, staff in hand, her "
              "throne carved with lions. Marchesa is painted the same way "
              "— enthroned, crowned, straight to camera — and she is the "
              "card that hands out a throne: when she enters, you become "
              "the monarch. Deathtouch and haste, and an Assassin every "
              "upkeep the crown sits on somebody else's head."),
    Card("mtg-everflowing-chalice", "Everflowing Chalice", "minor", "cups", 1,
         art_url="https://cards.scryfall.io/art_crop/front/e/4/"
                 "e4ed0052-d6dd-4f69-8313-10863baefac9.jpg",
         artist="Steve Argyle", after="the Ace of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ace of Cups overflows — and this chalice is named for "
              "it, kicked any number of times, filling without limit "
              "from the moment it costs nothing at all to place on the "
              "table."),
    Card("mtg-wedding-announcement", "Wedding Announcement // "
         "Wedding Festivity", "minor", "cups", 2,
         art_url="https://cards.scryfall.io/art_crop/front/4/e/"
                 "4e6f365d-c5c4-4fd6-94cb-833b89239d73.jpg",
         artist="Caroline Gariba", after="the Two of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Two of Cups is a betrothal: two people, two cups, a "
              "pledge held between them. Magic printed the invitation — a "
              "couple leaning over a wedding announcement — and the card "
              "itself is two cards, gathering invitation counters at each "
              "end step until it transforms into Wedding Festivity. Two "
              "faces for two cups, and the second one is the party."),
    Card("mtg-rite-of-harmony", "Rite of Harmony", "minor", "cups", 3,
         art_url="https://cards.scryfall.io/art_crop/front/e/b/"
                 "eb1a16f9-13d4-4188-8e1e-0f2394349c7a.jpg",
         artist="Rovina Cai", after="the Three of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Three of Cups is three figures in a ring with their "
              "cups raised. Rovina Cai painted three in a ring with their "
              "hands joined, and the card pays you a card for every "
              "creature or enchantment that joins them that turn."),
    Card("mtg-forsake-the-worldly", "Forsake the Worldly", "minor", "cups", 8,
         art_url="https://cards.scryfall.io/art_crop/front/c/c/"
                 "cca4e95e-f14e-4cfa-918a-cfb15f912293.jpg",
         artist="Steve Argyle", after="the Eight of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Eight of Cups is the one where somebody stacks up eight "
              "cups, looks at them, and walks away under an eclipsed moon. "
              "This spell exiles an artifact or enchantment — it takes a "
              "possession out of the world — and its flavor text is the "
              "trump's own argument: \"Why cling to these trappings? They "
              "are but tools and affectations.\""),
    Card("mtg-golden-wish", "Golden Wish", "minor", "cups", 9,
         art_url="https://cards.scryfall.io/art_crop/front/d/c/"
                 "dc409ded-41f3-4f14-8199-72a9fe98bac0.jpg",
         artist="Alan Pollack", after="the Nine of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Nine of Cups is the wish card, and this is the only "
              "kind of wish Magic ever printed: reveal an artifact or "
              "enchantment you own from *outside the game* and put it in "
              "your hand. What you wanted arrives from beyond the table. "
              "The flavor text keeps the trump's small print — \"She "
              "wished for nobility, but not for a nation to honor it.\""),
    Card("mtg-happily-ever-after", "Happily Ever After", "minor", "cups", 10,
         art_url="https://cards.scryfall.io/art_crop/front/d/3/"
                 "d32d85d5-a6f0-4cc5-9fd6-6b329aae2e5b.jpg",
         artist="Matt Stewart", after="the Ten of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ten of Cups is the rainbow over the settled house — "
              "the one card in the deck that is simply happy. This one "
              "hands every player five life and a card, and if you ever "
              "hold all five colours, six card types and the life you "
              "started with, you win the game outright."),
    Card("mtg-thassa", "Thassa, God of the Sea", "minor", "cups", 13,
         art_url="https://cards.scryfall.io/art_crop/front/d/6/"
                 "d6876c7a-8bbe-484e-b733-70229fa336cd.jpg",
         artist="Jason Chan", after="the Queen of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The Queen of Cups sits at the water's edge holding a "
              "covered cup that only she looks into. Thassa is the sea "
              "itself, indestructible, and not a creature at all until "
              "five of her people stand with her — and every upkeep she "
              "scries: she looks first, then decides what comes."),
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
    Card("mtg-svyelun", "Svyelun of Sea and Sky", "minor", "cups", 14,
         art_url="https://cards.scryfall.io/art_crop/front/0/1/"
                 "01f5ab00-5305-4ac4-915d-feeb591f9389.jpg",
         artist="Seb McKinnon", after="the King of Cups", echo=True,
         weight=ECHO_WEIGHT,
         note="The King of Cups keeps his throne on a slab in open water "
              "while the sea heaves around it — mastery of the element he "
              "sits on. Seb McKinnon painted a Merfolk God rising out of a "
              "whirlpool, still in the middle of it, and the rules say the "
              "same thing: indestructible while two other Merfolk stand "
              "with her, ward {1} over every one of them, and a card drawn "
              "each time she attacks."),
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
    Card("mtg-curse-of-the-pierced-heart", "Curse of the Pierced Heart",
         "minor", "swords", 3,
         art_url="https://cards.scryfall.io/art_crop/front/7/1/"
                 "71010182-c004-4d18-adab-80319cd1e625.jpg",
         artist="E. M. Gist", after="the Three of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Three of Swords is a heart run through by three blades "
              "under rain. This is a Curse — that is the card type — and "
              "it pierces the player it enchants for one damage at the "
              "beginning of every one of their upkeeps. Grief that comes "
              "back each morning."),
    Card("mtg-winters-rest", "Winter's Rest", "minor", "swords", 4,
         art_url="https://cards.scryfall.io/art_crop/front/1/f/"
                 "1fdaf6b6-aca7-49e5-a4da-bf8c08b4a055.jpg",
         artist="Mila Pesic", after="the Four of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Four of Swords is a knight lying in effigy on his own "
              "tomb, hands together, three swords on the wall above him "
              "and one beneath — rest, not death, and the only quiet card "
              "in a bad suit. Mila Pesic laid a figure out the same way "
              "under frost and roses, and the Aura does what the picture "
              "does: it taps the creature, and while a snow permanent is "
              "yours it does not untap."),
    Card("mtg-rescue-from-the-underworld", "Rescue from the Underworld",
         "minor", "swords", 6,
         art_url="https://cards.scryfall.io/art_crop/front/2/e/"
                 "2e46aa9c-7a29-4eb4-bc44-a201c22824d2.jpg",
         artist="Raymond Swanland", after="the Six of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Six of Swords is the ferryman poling away from choppy "
              "water, six swords standing in the bow and his passengers "
              "under a shroud. Swanland painted the ferry itself — a "
              "hooded figure taking a punt across a dark river by "
              "lamplight — and the spell is that crossing in both "
              "directions: sacrifice a creature, choose one lying in the "
              "graveyard, and both of them come back at your next upkeep."),
    Card("mtg-startled-awake", "Startled Awake // Persistent Nightmare",
         "minor", "swords", 9,
         art_url="https://cards.scryfall.io/art_crop/front/e/6/"
                 "e630fc56-a96f-4abf-be8d-f0f40d5a9edf.jpg",
         artist="Sean Sevestre", after="the Nine of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Nine of Swords is a figure sitting up in bed with "
              "their face in their hands at the worst hour of the night, "
              "and Magic painted that exact picture and gave it that "
              "exact name. It mills thirteen, then climbs back out of the "
              "graveyard as Persistent Nightmare — and returns to hand "
              "every time it connects, so it can do it again."),
    Card("mtg-murder", "Murder", "minor", "swords", 10,
         art_url="https://cards.scryfall.io/art_crop/front/c/8/"
                 "c8676f02-cf1e-4d40-a0c5-6e5a97417898.jpg",
         artist="Allen Williams", after="the Ten of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ten of Swords is a body face down with ten blades in "
              "its back — the plainest ending the deck has. Magic's "
              "plainest ending is three mana and three words: destroy "
              "target creature. Allen Williams painted it as the blade "
              "already through the chest."),
    Card("mtg-pale-rider-of-trostad", "Pale Rider of Trostad", "minor",
         "swords", 12,
         art_url="https://cards.scryfall.io/art_crop/front/8/d/"
                 "8dad70b8-0dec-4634-a31a-b78438a313a2.jpg",
         artist="Seb McKinnon", after="the Knight of Swords", echo=True,
         weight=ECHO_WEIGHT,
         note="The Knight of Swords is the hardest charge in the deck: a "
              "knight on a galloping white horse with his cloak straight "
              "out behind him, going at the thing head-on. Seb McKinnon "
              "painted a pale horse at full gallop through birches with "
              "its rider barely there. Skulk means nothing bigger than him "
              "can stop him, and he takes a card out of your hand the "
              "moment he arrives — the charge that goes through, and "
              "costs."),
    Card("mtg-chromatic-star", "Chromatic Star", "minor", "pentacles", 1,
         art_url="https://cards.scryfall.io/art_crop/front/c/2/"
                 "c2e8d492-2c67-410b-b556-c157a14c4cec.jpg",
         artist="Daniel Ljunggren", after="the Ace of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ace of Pentacles is a hand out of a cloud holding one "
              "gold disc with a five-pointed star cut into it. This is "
              "that disc: a five-pointed star, an artifact for one mana "
              "that becomes any colour you name and replaces itself with a "
              "card on the way out. Every ace in this deck is its suit's "
              "object, and the pentacle's object is a star."),
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
    Card("mtg-harvest-season", "Harvest Season", "minor", "pentacles", 7,
         art_url="https://cards.scryfall.io/art_crop/front/3/2/"
                 "326b5fad-bb8d-4019-84a8-1a319a14962e.jpg",
         artist="Shreya Shetty", after="the Seven of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Seven of Pentacles leans on his staff and looks at a "
              "bush he has been growing, counting what is on it and not "
              "picking any of it yet. Shreya Shetty painted the same "
              "pause, a gardener over terraced beds with a seedling still "
              "in hand — and the spell pays exactly what was worked for: a "
              "basic land for every *tapped* creature you control. The "
              "yield is measured in the labour."),
    Card("mtg-argivian-blacksmith", "Argivian Blacksmith", "minor",
         "pentacles", 8,
         art_url="https://cards.scryfall.io/art_crop/front/f/1/"
                 "f1fb4d0b-fa3f-4794-9285-89ddb9ac21c3.jpg",
         artist="Kerstin Kaman", after="the Eight of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Eight of Pentacles is an apprentice at his bench "
              "hammering one coin at a time, the finished ones nailed up "
              "beside him. This is a smith at the anvil mid-swing, and the "
              "flavor text states the trump's whole lesson as history: "
              "\"Through years of study and training, the Blacksmiths of "
              "Argive became adept at reassembling the mangled remains\" "
              "of the machines around them. Repetition is the craft, and "
              "repair is what it buys."),
    Card("mtg-soraya-the-falconer", "Soraya the Falconer", "minor",
         "pentacles", 9,
         art_url="https://cards.scryfall.io/art_crop/front/1/9/"
                 "19fb3ce2-a660-4829-9af4-330cfd612f06.jpg",
         artist="Dennis Detwiller", after="the Nine of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Nine of Pentacles is a woman alone in her own walled "
              "garden with a hooded falcon on her glove — comfort she "
              "built and now enjoys by herself. Homelands printed her "
              "standing in her own field with the bird on her fist, every "
              "Bird stronger for her being there. Her flavor text is "
              "spoken by Autumn Willow, the same voice quoted on Willow "
              "Priestess: \"Soraya speaks with the hunters of the air, as "
              "do all of her family line.\""),
    Card("mtg-inheritance", "Inheritance", "minor", "pentacles", 10,
         art_url="https://cards.scryfall.io/art_crop/front/f/6/"
                 "f660a337-09f7-4ae9-b61b-f5ecbda4a4ca.jpg",
         artist="Kaja Foglio", after="the Ten of Pentacles", echo=True,
         weight=ECHO_WEIGHT,
         note="The Ten of Pentacles is three generations under one arch — "
              "the old man and his dogs, the couple, the child — with ten "
              "coins laid out in the pattern of the Tree of Life. Kaja "
              "Foglio painted two of those generations in one frame, an "
              "elder standing behind a young woman in the same collar. The "
              "card is what passes between them: whenever a creature dies, "
              "pay {3} and draw. \"More than lessons may be gained from "
              "the past.\""),
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

#: What actually gets shuffled: 136 cards -- every trump answered by a
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
        without replacement across all 136, so the two Fools really can
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
