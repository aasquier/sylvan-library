"""The 32 colour combinations, and what each one means.

Reference data, not corpus data. Every other module here refuses to know
anything about a card that Scryfall did not say; this one is deliberately the
opposite -- it is the vocabulary a new player does not have yet, and no amount
of card data teaches it. `CLAUDE.md` rule 5 is about card corpora and
collections, and none of this is either, so it is committed.

**Why 32.** One colourless + five mono + ten pairs + ten three-colour (five
shards, five wedges) + five four-colour + one five-colour. That is also exactly
the 32 Deck Challenge, which is why goal 8 in ROADMAP treats the challenge and
the colour diagrams as one dataset rather than two features: a deck's slot in
the challenge is `Combination.of(deck.color_identity)`.

**Every mapping here was verified against the corpus**, not recalled -- the
guild, shard and wedge charms, the Nephilim, and the Commander 2016 commanders
are all real cards whose `color_identity` Scryfall reports, so the table below
is checkable rather than asserted. Rule 1 is about cards; this is the same
habit applied to something that is not one.

**Four-colour naming has two conventions and this module carries both.**
Scryfall uses the Commander 2016 deck names (Artifice, Chaos, Aggression,
Altruism, Growth); EDHREC uses the Nephilim from Guildpact (Yore-Tiller,
Glint-Eye, Dune-Brood, Ink-Treader, Witch-Maw). They describe identical colour
sets -- Breya and Yore-Tiller Nephilim are both {W}{U}{B}{R}. Scryfall's names
are primary here for the same reason Scryfall decides colour identity: one
authority, consistently. The Nephilim names are aliases so that someone
arriving from EDHREC finds what they are looking for.
"""

from __future__ import annotations

from dataclasses import dataclass, field

# Canonical order. Every key in this module is a subset of WUBRG written in
# this order, so a colour identity has exactly one spelling and set comparisons
# never depend on how a caller sorted it.
WUBRG = "WUBRG"


def key_for(colors) -> str:
    """Canonical key for a set of colours. `frozenset({'G','W'})` -> `'WG'`."""
    return "".join(c for c in WUBRG if c in set(colors)) or "C"


@dataclass(frozen=True)
class Color:
    """One of the five, and what it wants."""

    code: str
    name: str
    wants: str
    fears: str


COLORS: tuple[Color, ...] = (
    Color("W", "White",
          "Peace through structure. White believes suffering is a problem of "
          "organisation, and that a group that agrees on rules protects "
          "everyone in it.",
          "That the rules stop serving the people and start serving "
          "themselves."),
    Color("U", "Blue",
          "Perfection through knowledge. Blue believes anything can be "
          "improved given enough information and patience, including itself.",
          "That it acts before it understands, and cannot take the move back."),
    Color("B", "Black",
          "Power through self-interest. Black is the only colour honest about "
          "wanting to win, and it will pay any price it decides is worth "
          "paying.",
          "That someone else is willing to pay more."),
    Color("R", "Red",
          "Freedom through action. Red believes the feeling is the truth, and "
          "that a life spent hesitating is a life not lived.",
          "Being told to wait."),
    Color("G", "Green",
          "Growth through acceptance. Green believes the natural order is "
          "already right, and that the work is to find your place in it "
          "rather than to redesign it.",
          "Change imposed from outside."),
)

COLOR_BY_CODE = {c.code: c for c in COLORS}

# The tiers, in the order a newcomer should meet them.
TIERS = ("colorless", "mono", "guild", "shard", "wedge", "quad", "five")

TIER_LABELS = {
    "colorless": "Colourless",
    "mono": "Mono-colour",
    "guild": "Guild — two colours",
    "shard": "Shard — three allied colours",
    "wedge": "Wedge — a colour and its two enemies",
    "quad": "Four colours",
    "five": "All five",
}


@dataclass(frozen=True)
class Combination:
    """One of the 32, with the history that gives it a name."""

    key: str
    name: str
    tier: str
    tagline: str
    history: str
    aliases: tuple[str, ...] = field(default_factory=tuple)
    # The card whose colour identity was used to verify this row, where one
    # exists. Kept so the claim stays checkable: `mtglab` can look it up.
    verified_by: str = ""

    @property
    def colors(self) -> tuple[str, ...]:
        return tuple(self.key) if self.key != "C" else ()

    @property
    def size(self) -> int:
        return len(self.colors)


# --------------------------------------------------------------------- the 32

COMBINATIONS: tuple[Combination, ...] = (
    Combination(
        "C", "Colourless", "colorless",
        "No colour at all — and a deck that asks what colour is even for.",
        "Every other slot on this list is a set of colours. This one is the "
        "absence of them, and it is a real place to build: Eldrazi, artifacts, "
        "and lands that ask nothing of you. Colourless decks trade the "
        "identity and the answers a colour brings for a mana base that never "
        "betrays them.",
        verified_by="Kozilek, Butcher of Truth"),

    Combination(
        "W", "Mono-White", "mono",
        "The group, protected by rules everyone agreed to.",
        "White alone is the purest version of the argument: that structure is "
        "kindness. It builds wide boards of small creatures, protects them "
        "collectively, and answers threats with rules rather than force — "
        "exile, taxation, and effects that hit everyone equally, which White "
        "considers the fairest kind of answer.",
        verified_by="Wrath of God"),
    Combination(
        "U", "Mono-Blue", "mono",
        "Patience, and the confidence that knowing more is winning.",
        "Blue alone plays a longer game than anyone at the table finds "
        "comfortable. It draws, it counters, it takes extra turns, and it "
        "wins with something that was never in doubt once it had enough "
        "information. Its weakness is the one it admits to: it cannot deal "
        "with a permanent it failed to stop on the way in.",
        verified_by="Counterspell"),
    Combination(
        "B", "Mono-Black", "mono",
        "Any price, as long as it is worth paying.",
        "Black alone has the format's most flexible removal and the fewest "
        "moral objections. It pays life to draw, sacrifices its own creatures "
        "to fuel engines, and reanimates whatever died on the way. Its real "
        "restriction is not power but blind spots: enchantments and artifacts "
        "are the things it has the hardest time answering.",
        verified_by="Damnation"),
    Combination(
        "R", "Mono-Red", "mono",
        "Do it now, and find out whether it worked afterwards.",
        "Red alone is speed, damage and the willingness to be wrong loudly. "
        "It rummages rather than draws, generates treasure and impulsive "
        "advantage that expires if unused, and burns whatever it can reach. "
        "In Commander its weakness is arithmetic: forty life across three "
        "opponents is a lot of damage.",
        verified_by="Lightning Bolt"),
    Combination(
        "G", "Mono-Green", "mono",
        "The biggest creatures, and the mana to cast them early.",
        "Green alone accelerates. It puts lands into play, casts things ahead "
        "of schedule, and wins by presenting a board nobody can profitably "
        "attack into. It answers artifacts and enchantments comfortably and "
        "fliers poorly, and it historically could not draw cards — a gap "
        "modern sets have largely closed.",
        verified_by="Llanowar Elves"),

    # ---- the ten guilds of Ravnica, allied and enemy pairs alike ----
    Combination(
        "WU", "Azorius", "guild",
        "Law as a weapon. Nothing resolves that was not approved.",
        "The Azorius Senate writes Ravnica's laws and enforces them with "
        "detain, taxation and counterspells. Its promise is that a perfectly "
        "ordered society is a safe one; its critics point out that a system "
        "which never lets anything happen is indistinguishable from one that "
        "has stopped working.",
        verified_by="Azorius Charm"),
    Combination(
        "UB", "Dimir", "guild",
        "Information as power, and a guild that officially does not exist.",
        "House Dimir keeps Ravnica's secrets, including the secret of itself — "
        "for most of the city's history its existence was a rumour. It mills, "
        "it steals, it reads hands and it wins with knowledge nobody knew it "
        "had. Blue's patience with black's willingness gives it the format's "
        "most comfortable relationship with doing what is necessary.",
        verified_by="Dimir Charm"),
    Combination(
        "BR", "Rakdos", "guild",
        "Cruelty performed for an audience.",
        "The Cult of Rakdos is Ravnica's entertainment industry, which on "
        "Ravnica means blood sport. It is black's appetite with red's "
        "showmanship, and its decks are correspondingly direct: sacrifice, "
        "damage, and a refusal to play a long game it does not need.",
        verified_by="Rakdos Charm"),
    Combination(
        "RG", "Gruul", "guild",
        "Tear the city down and let something honest grow back.",
        "The Gruul Clans are what Ravnica pushed to the margins, and they want "
        "the whole edifice gone. Red's fury and green's conviction that the "
        "natural order is right produce the format's least complicated plan: "
        "very large creatures, attacking immediately.",
        verified_by="Gruul Charm"),
    Combination(
        "WG", "Selesnya", "guild",
        "The community is the creature.",
        "The Selesnya Conclave believes the individual matters less than the "
        "whole, which is unsettling from the outside and genuinely powerful in "
        "play: tokens, anthems, and boards that get stronger for every "
        "additional body. White's structure and green's growth agree on "
        "almost everything, which makes this the most harmonious pair on the "
        "wheel and the least tolerant of dissent.",
        verified_by="Selesnya Charm"),
    Combination(
        "WB", "Orzhov", "guild",
        "Debt, collected forever, including after death.",
        "The Orzhov Syndicate is a church and a bank, and it sees no tension "
        "between the two. White's obligation plus black's leverage becomes "
        "drain, taxation and a workforce of spirits who died still owing. It "
        "grinds rather than races, and it is very hard to kill.",
        verified_by="Orzhov Charm"),
    Combination(
        "UR", "Izzet", "guild",
        "Genius with no impulse control.",
        "The Izzet League invents constantly and asks about consequences "
        "afterwards. Blue supplies the intelligence and red supplies the "
        "refusal to wait, producing the format's spellslinger decks: cheap "
        "instants, effects that copy them, and payoffs that reward casting a "
        "great many of them in one turn.",
        verified_by="Izzet Charm"),
    Combination(
        "BG", "Golgari", "guild",
        "Death is a stage of growth, not the end of one.",
        "The Golgari Swarm runs Ravnica's undercity and its decomposition, and "
        "sees no difference between the two jobs. Black's comfort with the "
        "graveyard and green's cycles of growth make the yard a second hand: "
        "creatures that want to die, engines that recur them, and value that "
        "compounds every time something is lost.",
        verified_by="Golgari Charm"),
    Combination(
        "WR", "Boros", "guild",
        "Righteous anger, applied immediately.",
        "The Boros Legion is white's conviction moving at red's speed. It "
        "attacks, it goes wide, it doubles its own damage, and it treats "
        "hesitation as a moral failure. Long the weakest pair in Commander "
        "for lack of card advantage, it has been the biggest beneficiary of "
        "impulse-draw and treasure effects.",
        verified_by="Boros Charm"),
    Combination(
        "UG", "Simic", "guild",
        "Improve the organism. Then improve it again.",
        "The Simic Combine splices what works onto what nearly works. Blue's "
        "refinement plus green's growth produces ramp into card draw into "
        "enormous, carefully specified creatures — and a reputation at "
        "Commander tables for decks that do not interact until they win.",
        verified_by="Simic Charm"),

    # ---- the five shards of Alara: a colour with both its allies ----
    Combination(
        "WUG", "Bant", "shard",
        "Order, chivalry, and a hierarchy everyone accepts.",
        "Alara shattered into five planes, each missing two colours, and each "
        "became a caricature of what remained. Bant kept white with blue and "
        "green and lost black and red entirely — so it has no ambition and no "
        "passion, only duty. Angels, knights, and a rigid caste system nobody "
        "questions because nobody has anything to be angry about.",
        verified_by="Bant Charm"),
    Combination(
        "WUB", "Esper", "shard",
        "Flesh is a rough draft. Etherium is the correction.",
        "Esper is blue's shard, and it replaced its own biology with metal. "
        "White's order and black's ruthlessness serve blue's perfectionism, "
        "producing artifice, control and the format's most calculating decks. "
        "Losing red and green cost it spontaneity and growth, which Esper "
        "regards as an upgrade.",
        verified_by="Esper Charm"),
    Combination(
        "UBR", "Grixis", "shard",
        "Everything is already dead. Use it anyway.",
        "Grixis is black's shard, stripped of white's compassion and green's "
        "renewal, and it ran out of living resources long ago. What is left is "
        "necromancy, scarcity and a politics of pure leverage — reanimation, "
        "theft, and the cheerful assumption that a corpse is a resource.",
        verified_by="Grixis Charm"),
    Combination(
        "BRG", "Jund", "shard",
        "A food chain with no top and no rules.",
        "Jund is red's shard: black's appetite and green's ferocity with no "
        "white to organise it and no blue to think it through. Dragons at the "
        "top, everything else eating each other underneath. In play it is "
        "aggression backed by sacrifice value — nothing is wasted, because "
        "everything gets eaten eventually.",
        verified_by="Jund Charm"),
    Combination(
        "WRG", "Naya", "shard",
        "Nature at a scale that makes worship the sensible response.",
        "Naya is green's shard, keeping white's reverence and red's passion "
        "and losing blue's caution and black's self-interest. Its inhabitants "
        "venerate gargantuan beasts, and its decks do roughly that: ramp into "
        "enormous creatures and swing.",
        verified_by="Naya Charm"),

    # ---- the five wedges of Tarkir: a colour with both its enemies ----
    Combination(
        "WBG", "Abzan", "wedge",
        "The family endures. The individual is how it does that.",
        "Where a shard is a colour with its two friends, a wedge is a colour "
        "with its two enemies — which makes wedges harder to hold together and "
        "more interesting when they cohere. Tarkir's five clans each took one. "
        "Abzan is white's endurance flanked by black and green: outlast "
        "everything, and count the dead as still contributing.",
        verified_by="Abzan Charm"),
    Combination(
        "WUR", "Jeskai", "wedge",
        "Cunning, discipline, and the strike that ends it.",
        "The Jeskai Way is blue's enlightenment with white's discipline and "
        "red's decisiveness — monks who study for decades in order to act "
        "instantly. In play it is tempo: cheap interaction, prowess, and a "
        "clock that closes while the opponent is still setting up.",
        verified_by="Jeskai Charm"),
    Combination(
        "UBG", "Sultai", "wedge",
        "Whatever you have is not enough. Take more.",
        "The Sultai Brood is black's ambition with blue's cunning and green's "
        "abundance, and it is the most decadent faction Magic has printed: "
        "wealth, necromancy and a total absence of restraint. Its decks fill "
        "the graveyard on purpose and treat it as a resource pool.",
        verified_by="Sultai Charm"),
    Combination(
        "WBR", "Mardu", "wedge",
        "Speed is the only virtue. Stopping is how you die.",
        "The Mardu Horde is red's momentum with white's numbers and black's "
        "disregard, a raiding culture that measures worth in motion. Its decks "
        "go wide and fast, sacrifice freely, and are the wedge least "
        "interested in the late game.",
        verified_by="Mardu Charm"),
    Combination(
        "URG", "Temur", "wedge",
        "Survive the frontier, and become large enough that it survives you.",
        "The Temur Frontier is green's savagery with blue's cunning and red's "
        "ferocity — a clan that reads the land and then overwhelms it. Ramp, "
        "big creatures, and enough card selection to find the right one.",
        verified_by="Temur Charm"),

    # ---- four colours: each defined by the one it refuses ----
    Combination(
        "WUBR", "Artifice", "quad",
        "Everything except green — the made over the grown.",
        "Four-colour identities are best understood by what they exclude, "
        "since each is all five minus one. Artifice rejects green: nothing "
        "here trusts the natural order, and the answer to every problem is a "
        "better artifact. Commander 2016 named this deck Artifice and gave it "
        "Breya; EDHREC calls it Yore-Tiller after the Guildpact Nephilim with "
        "the same identity.",
        aliases=("Yore-Tiller",), verified_by="Breya, Etherium Shaper"),
    Combination(
        "UBRG", "Chaos", "quad",
        "Everything except white — no rules, and no one to enforce them.",
        "Chaos rejects white, and with it structure, fairness and restraint. "
        "What remains is appetite, cunning, fury and growth with nothing "
        "moderating any of it — cascade, randomness and effects that hand the "
        "outcome to chance on purpose. Yidris led the Commander 2016 deck; "
        "EDHREC calls it Glint-Eye.",
        aliases=("Glint-Eye",), verified_by="Yidris, Maelstrom Wielder"),
    Combination(
        "WBRG", "Aggression", "quad",
        "Everything except blue — act, and do not deliberate.",
        "Aggression rejects blue, which means it rejects patience, counterplay "
        "and the idea that more information would help. It is the four-colour "
        "identity that attacks. Saskia fronted the Commander 2016 deck; "
        "EDHREC calls it Dune-Brood.",
        aliases=("Dune-Brood",), verified_by="Saskia the Unyielding"),
    Combination(
        "WURG", "Altruism", "quad",
        "Everything except black — generosity as a strategy.",
        "Altruism rejects black, and therefore rejects self-interest as an "
        "organising principle. It is the home of group hug: shared draw, "
        "shared ramp, symmetrical generosity, and the wager that you will use "
        "the gift better than the people you gave it to. Kynaios and Tiro led "
        "the Commander 2016 deck; EDHREC calls it Ink-Treader.",
        aliases=("Ink-Treader",), verified_by="Kynaios and Tiro of Meletis"),
    Combination(
        "WUBG", "Growth", "quad",
        "Everything except red — patience compounding into inevitability.",
        "Growth rejects red, and with it impulse, haste and the need for "
        "anything to happen right now. It accumulates: counters, permanents, "
        "value, until the board is simply better than everyone else's. Atraxa "
        "led the Commander 2016 deck and went on to become one of the "
        "format's most-built commanders; EDHREC calls this identity "
        "Witch-Maw.",
        aliases=("Witch-Maw",), verified_by="Atraxa, Praetors' Voice"),

    Combination(
        "WUBRG", "Five-Colour", "five",
        "All of it, if the mana base can be paid for.",
        "Five-colour decks give up nothing in card choice and everything in "
        "consistency — the whole card pool is legal, and the cost is a mana "
        "base that has to produce any colour on any turn. It is the most "
        "expensive way to build a deck and the only one with no answers "
        "unavailable to it.",
        verified_by="Sliver Overlord"),
)

BY_KEY = {c.key: c for c in COMBINATIONS}


def of(colors) -> Combination:
    """The combination for a colour identity. Accepts any iterable of codes.

    `of(deck_color_identity)` is a deck's slot in the 32 Deck Challenge, which
    is why this takes exactly what `CardRecord.color_identity` provides.
    """
    return BY_KEY[key_for(colors)]


def by_tier(tier: str) -> list[Combination]:
    return [c for c in COMBINATIONS if c.tier == tier]


# ------------------------------------------------------------------ the eras

@dataclass(frozen=True)
class Era:
    """A block that named a set of colour combinations, for the carousel."""

    name: str
    setting: str
    named: str
    story: str


ERAS: tuple[Era, ...] = (
    Era("Ravnica", "A city that covers an entire world",
        "the ten guilds",
        "Ravnica is one city, everywhere, run by ten guilds under a magically "
        "binding treaty. Each guild is one pair of colours, and between them "
        "they cover all ten pairs exactly once — which is why the guild names "
        "became the standard vocabulary for two-colour decks. Ask a Commander "
        "player what their deck is and 'Golgari' is a complete answer."),
    Era("Alara", "One plane shattered into five",
        "the five shards",
        "Alara broke into five fragments, and each fragment lost two colours "
        "entirely. What survived was a colour and both of its allies, taken to "
        "an extreme with nothing to balance it — a world with no ambition, a "
        "world with no mercy. The five shards give three-colour allied "
        "identities their names."),
    Era("Tarkir", "A world of dragons, and then of clans, and then of dragons",
        "the five wedges",
        "Tarkir's five clans each venerated one aspect of dragonkind, and each "
        "took a colour with both of its *enemies* rather than its friends. "
        "That is the difference between a shard and a wedge, and it is why "
        "wedges feel less comfortable and more distinctive: the colours in "
        "them disagree, and the deck has to be the argument that settles it."),
)
