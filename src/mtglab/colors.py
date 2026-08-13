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

**The depth is checked in rather than generated, and that was a decision.**
The obvious alternative was a Claude surface: a guild is exactly the sort of
thing ADR 19's dossier already answers well. Four things argued the other way.
`/api/colors` is the one page in the app that works with no corpus and no
network, which a model call would spend. The set is finite -- ten guilds, five
shards, five wedges, written once, ever -- so a per-view call pays repeatedly
for content with no variance in it. ADR 20 already classed this module as a
fourth source alongside the dossier's three: checked in, carrying
`verified_by`, and free. And the complaint that produced `lore` was that the
prose was bland; bland is fixed by editing, and only checked-in text can be
edited. What Claude answers is the unbounded per-deck question about a
*commander*, which is ADR 19 and stays there.

**Card facts inside that prose still come from the corpus.** `champions` and
`signature` hold *names*; the page resolves them through `get_cards` and shows
the real card. `signature` carries no prose at all, so there is no sentence in
it for a card fact to be wrong in -- what it asserts is a checkable property,
that the card's identity is exactly this combination. `Champion.role` is the
one editorial sentence attached to a card here, it is about the character
rather than the card, and the card's own text renders beside it.

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

from collections.abc import Iterable
from dataclasses import dataclass, field

# Canonical order. Every key in this module is a subset of WUBRG written in
# this order, so a colour identity has exactly one spelling and set comparisons
# never depend on how a caller sorted it.
WUBRG = "WUBRG"


def key_for(colors: Iterable[str]) -> str:
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

#: What a tier *is*, as opposed to what any one of its members is.
#:
#: This table exists because its absence had a cost. Three combinations —
#: Bant, Abzan and Artifice, the first entry in the shard, wedge and quad
#: tiers — each opened by explaining their whole tier, because there was
#: nowhere else for that sentence to live. Someone arrowing straight to Naya
#: therefore never learned what a shard was, and someone reading Bant was told
#: about Alara twice, once here and once in the era paragraph above it.
#:
#: Distinct from `ERAS`, and deliberately so. A blurb is definitional and every
#: tier has one; an era is the block of Magic whose story supplied the names,
#: and only three tiers have that. Colourless is not from anywhere.
#:
#: The split is also a rule about what goes where, and it has to be kept or the
#: two paragraphs say the same thing one after the other. **A blurb is
#: mechanical and never names a plane; an era is a setting and never restates
#: the mechanics.** Both of these are rendered as plain text, so no markdown:
#: an asterisk here reaches the screen as an asterisk.
TIER_BLURBS = {
    "colorless": "The absence of colour rather than a choice among colours — "
                 "and the only identity that can play any land in the game "
                 "without a cost.",
    "mono": "One colour, and everything it cannot do left undone. The "
            "cheapest mana base in the format and the sharpest set of "
            "weaknesses, which is what makes these the decks people recommend "
            "building first.",
    "guild": "Two colours, and there are exactly ten ways to pick two: five "
             "pairs that sit next to each other on the colour wheel and five "
             "that sit opposite. Neighbours cooperate; opposites have to be "
             "talked into it, and are usually the more interesting deck.",
    "shard": "Three colours: one colour and both of the two it neighbours, so "
             "the whole identity is one unbroken arc of the wheel. A shard "
             "agrees with itself, which makes it the more comfortable of the "
             "two three-colour shapes and the less pointed one.",
    "wedge": "Three colours: one colour and the two it sits opposite. That is "
             "the whole difference from a shard, and it is why wedges feel "
             "less comfortable and more distinctive — the colours in them "
             "disagree, and the deck has to be the argument that settles it.",
    "quad": "Four colours, best understood by the one they refuse. Each of "
            "these is all five minus one, and the missing colour says more "
            "about the deck than the four present ones do.",
    "five": "Every colour, and the mana base that bill comes with.",
}


@dataclass(frozen=True)
class Champion:
    """A character who is the face of a faction, and the card that proves it.

    `role` is the one thing the card cannot say about itself -- who this is in
    the story. It is deliberately the only editorial sentence attached to a
    card anywhere in this module, and it stops at the character: what the card
    *does* comes from the corpus, next to the sentence, so a note that drifted
    from the card is visible rather than authoritative.

    `card` is a real card name and is checked against the corpus, twice. Its
    colour identity must be a *subset* of the combination rather than equal to
    it -- Alesha is a Mardu warrior on a {2}{R} card whose {W/B} hybrids make
    her identity Mardu, and the khans are exact, but a faction is a story and
    the story does not owe the colour pie an exact match.
    """

    card: str
    role: str


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
    #: What happened. The story beat this faction is known for, and the field
    #: this module grew for the complaint that started it: "the guilds of
    #: Ravnica are pretty famous, we can do better".
    #:
    #: Only the twenty combinations that are an actual faction have one -- the
    #: ten guilds, the five shards, the five wedges. Mono-Red is not from
    #: anywhere and does not get a paragraph pretending it is.
    lore: str = ""
    #: The faces of the faction. Empty for the twelve slots that are not one.
    champions: tuple[Champion, ...] = field(default_factory=tuple)
    #: Cards whose colour identity is **exactly** this combination.
    #:
    #: Names only, and that is the point. The page renders the real card next
    #: to the name -- cost, type, oracle text, art, all from the corpus -- so
    #: there is no sentence here for a card fact to be wrong in. What the list
    #: asserts is a checkable property rather than an opinion: these cards can
    #: exist in this combination and in no smaller one, which is the most
    #: direct answer available to "what are these colours *for*".
    signature: tuple[str, ...] = field(default_factory=tuple)

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
        verified_by="Kozilek, Butcher of Truth",
        signature=("Wastes", "Ugin, the Spirit Dragon",
                   "Ulamog, the Ceaseless Hunger")),

    Combination(
        "W", "Mono-White", "mono",
        "The group, protected by rules everyone agreed to.",
        "White alone is the purest version of the argument: that structure is "
        "kindness. It builds wide boards of small creatures, protects them "
        "collectively, and answers threats with rules rather than force — "
        "exile, taxation, and effects that hit everyone equally, which White "
        "considers the fairest kind of answer.",
        verified_by="Wrath of God",
        signature=("Swords to Plowshares", "Smothering Tithe",
                   "Ghostly Prison")),
    Combination(
        "U", "Mono-Blue", "mono",
        "Patience, and the confidence that knowing more is winning.",
        "Blue alone plays a longer game than anyone at the table finds "
        "comfortable. It draws, it counters, it takes extra turns, and it "
        "wins with something that was never in doubt once it had enough "
        "information. Its weakness is the one it admits to: it cannot deal "
        "with a permanent it failed to stop on the way in.",
        verified_by="Counterspell",
        signature=("Rhystic Study", "Cyclonic Rift", "Brainstorm")),
    Combination(
        "B", "Mono-Black", "mono",
        "Any price, as long as it is worth paying.",
        "Black alone has the format's most flexible removal and the fewest "
        "moral objections. It pays life to draw, sacrifices its own creatures "
        "to fuel engines, and reanimates whatever died on the way. Its real "
        "restriction is not power but blind spots: enchantments and artifacts "
        "are the things it has the hardest time answering.",
        verified_by="Damnation",
        signature=("Demonic Tutor", "Reanimate", "Phyrexian Arena")),
    Combination(
        "R", "Mono-Red", "mono",
        "Do it now, and find out whether it worked afterwards.",
        "Red alone is speed, damage and the willingness to be wrong loudly. "
        "It rummages rather than draws, generates treasure and impulsive "
        "advantage that expires if unused, and burns whatever it can reach. "
        "In Commander its weakness is arithmetic: forty life across three "
        "opponents is a lot of damage.",
        verified_by="Lightning Bolt",
        signature=("Chaos Warp", "Faithless Looting", "Blasphemous Act")),
    Combination(
        "G", "Mono-Green", "mono",
        "The biggest creatures, and the mana to cast them early.",
        "Green alone accelerates. It puts lands into play, casts things ahead "
        "of schedule, and wins by presenting a board nobody can profitably "
        "attack into. It answers artifacts and enchantments comfortably and "
        "fliers poorly, and it historically could not draw cards — a gap "
        "modern sets have largely closed.",
        verified_by="Llanowar Elves",
        signature=("Cultivate", "Beast Within", "Eternal Witness")),

    # ---- the ten guilds of Ravnica, allied and enemy pairs alike ----
    Combination(
        "WU", "Azorius", "guild",
        "Law as a weapon. Nothing resolves that was not approved.",
        "The Azorius Senate writes Ravnica's laws and enforces them with "
        "detain, taxation and counterspells. Its promise is that a perfectly "
        "ordered society is a safe one; its critics point out that a system "
        "which never lets anything happen is indistinguishable from one that "
        "has stopped working.",
        verified_by="Azorius Charm",
        lore="The Guildpact that binds all ten guilds was written by an "
             "Azorius sphinx, Azor, who then left Ravnica entirely and never "
             "came back. The Senate has spent ten thousand years enforcing a "
             "document whose author walked away from it, which is either the "
             "purest expression of the guild or its standing indictment, "
             "depending which guild you ask.",
        champions=(
            Champion("Azor, the Lawbringer",
                     "The parun: the sphinx who wrote the Guildpact, then "
                     "abandoned the plane it bound."),
            Champion("Isperia, Supreme Judge",
                     "The sphinx who led the Senate after him, and answered "
                     "petitioners with riddles."),
            Champion("Lavinia of the Tenth",
                     "An arrester of the Tenth District — the Senate at "
                     "street level, where detain is a person with a warrant."),
        ),
        signature=("Supreme Verdict", "Dovin's Veto",
                   "Grand Arbiter Augustin IV")),
    Combination(
        "UB", "Dimir", "guild",
        "Information as power, and a guild that officially does not exist.",
        "House Dimir keeps Ravnica's secrets, including the secret of itself — "
        "for most of the city's history its existence was a rumour. It mills, "
        "it steals, it reads hands and it wins with knowledge nobody knew it "
        "had. Blue's patience with black's willingness gives it the format's "
        "most comfortable relationship with doing what is necessary.",
        verified_by="Dimir Charm",
        lore="Dimir's parun broke the Guildpact. Szadek had been feeding on "
             "Ravnica for centuries, and when he was finally caught the "
             "magically binding treaty that held the city together turned out "
             "to be breakable after all — which is the event the rest of "
             "Ravnica's modern history proceeds from. The guild's own "
             "existence was a rumour for most of that time.",
        champions=(
            Champion("Szadek, Lord of Secrets",
                     "The parun, and the one who broke the Guildpact by "
                     "feeding on the city itself."),
            Champion("Lazav, Dimir Mastermind",
                     "Guildmaster of a guild that officially does not exist, "
                     "and a shapeshifter with no face of his own."),
            Champion("Etrata, the Silencer",
                     "An assassin whose contracts leave nothing behind for "
                     "anyone to investigate."),
        ),
        signature=("Baleful Strix", "Drown in the Loch", "The Scarab God")),
    Combination(
        "BR", "Rakdos", "guild",
        "Cruelty performed for an audience.",
        "The Cult of Rakdos is Ravnica's entertainment industry, which on "
        "Ravnica means blood sport. It is black's appetite with red's "
        "showmanship, and its decks are correspondingly direct: sacrifice, "
        "damage, and a refusal to play a long game it does not need.",
        verified_by="Rakdos Charm",
        lore="The guild is named after a demon who is asleep for most of "
             "Ravnica's history, and whose waking is treated by every other "
             "guild as a civic emergency. What the cult does in the meantime "
             "is put on shows. That is not a euphemism and it is not a cover: "
             "the performances are the guild, and the fact that people keep "
             "coming to them is the point Rakdos is making about everyone "
             "else.",
        champions=(
            Champion("Rakdos, Lord of Riots",
                     "The parun, a demon, and the only guild leader whose "
                     "absence is the normal state of affairs."),
            Champion("Judith, the Scourge Diva",
                     "The cult's impresario — she books the show, and the "
                     "show is the killing."),
            Champion("Exava, Rakdos Blood Witch",
                     "A blood witch of the outer circles, and the guild's "
                     "recruiting sergeant."),
        ),
        signature=("Terminate", "Bedevil", "Mayhem Devil")),
    Combination(
        "RG", "Gruul", "guild",
        "Tear the city down and let something honest grow back.",
        "The Gruul Clans are what Ravnica pushed to the margins, and they want "
        "the whole edifice gone. Red's fury and green's conviction that the "
        "natural order is right produce the format's least complicated plan: "
        "very large creatures, attacking immediately.",
        verified_by="Gruul Charm",
        lore="The Gruul are what the city displaced. When Ravnica grew to "
             "cover the world it grew over their land, and the Guildpact gave "
             "them a seat at the table in exchange for their range — a deal "
             "the clans have regarded ever since as the theft it was. They "
             "are the one guild whose stated goal is the end of the "
             "arrangement that keeps all ten of them alive.",
        champions=(
            Champion("Borborygmos",
                     "Cyclops chieftain of the largest clan, and the loudest "
                     "argument for tearing the city down."),
            Champion("Ruric Thar, the Unbowed",
                     "An ogre who leads by being the last one standing, and "
                     "hates a spell more than he hates a soldier."),
            Champion("Domri Rade",
                     "A Ravnican who left, became a planeswalker, and came "
                     "back to give the clans someone to follow."),
        ),
        signature=("Decimate", "Xenagos, God of Revels",
                   "Rhythm of the Wild")),
    Combination(
        "WG", "Selesnya", "guild",
        "The community is the creature.",
        "The Selesnya Conclave believes the individual matters less than the "
        "whole, which is unsettling from the outside and genuinely powerful in "
        "play: tokens, anthems, and boards that get stronger for every "
        "additional body. White's structure and green's growth agree on "
        "almost everything, which makes this the most harmonious pair on the "
        "wheel and the least tolerant of dissent.",
        verified_by="Selesnya Charm",
        lore="Selesnya's founder is not a person. Mat'Selesnya is a shared "
             "consciousness the guild's members join, and its leadership "
             "speaks through Trostani — three dryads fused into one voice "
             "that says 'we'. Every other guild is run by somebody. This one "
             "is run by an agreement, which is exactly as reassuring and as "
             "unsettling as it sounds.",
        champions=(
            Champion("Chorus of the Conclave",
                     "Mat'Selesnya given a body — the guild's founder is a "
                     "consciousness rather than anyone in particular."),
            Champion("Trostani, Selesnya's Voice",
                     "Three dryads who speak as one; the guild's leadership "
                     "is a chorus rather than a chair."),
            Champion("Emmara, Soul of the Accord",
                     "The Conclave at human scale, and the elf who kept "
                     "negotiating after the other guilds stopped."),
        ),
        signature=("Mirari's Wake", "Aura Shards", "Eladamri's Call")),
    Combination(
        "WB", "Orzhov", "guild",
        "Debt, collected forever, including after death.",
        "The Orzhov Syndicate is a church and a bank, and it sees no tension "
        "between the two. White's obligation plus black's leverage becomes "
        "drain, taxation and a workforce of spirits who died still owing. It "
        "grinds rather than races, and it is very hard to kill.",
        verified_by="Orzhov Charm",
        lore="The Orzhov are run by ten ghosts. The Obzedat are the "
             "syndicate's founding patriarchs, who declined to stop being in "
             "charge when they died and have held the seats ever since — "
             "which tells you what the guild thinks a contract is. Its "
             "workforce is largely made of people who died still owing, and "
             "it considers that an accounting matter.",
        champions=(
            Champion("Obzedat, Ghost Council",
                     "Ten spirits who refused to vacate, and still hold the "
                     "syndicate's seats."),
            Champion("Teysa Karlov",
                     "The advokist who runs the legal machinery, and the "
                     "afterlife workforce it produces."),
            Champion("Kaya, Orzhov Usurper",
                     "A planeswalker hired to kill the Ghost Council, who "
                     "then took their chair."),
        ),
        signature=("Anguished Unmaking", "Vindicate", "Cruel Celebrant")),
    Combination(
        "UR", "Izzet", "guild",
        "Genius with no impulse control.",
        "The Izzet League invents constantly and asks about consequences "
        "afterwards. Blue supplies the intelligence and red supplies the "
        "refusal to wait, producing the format's spellslinger decks: cheap "
        "instants, effects that copy them, and payoffs that reward casting a "
        "great many of them in one turn.",
        verified_by="Izzet Charm",
        lore="Izzet is the one guild still run by its founder. Niv-Mizzet is "
             "a dragon, the most intelligent being on Ravnica, and he has "
             "been personally in charge for ten thousand years — a guild "
             "whose entire culture is 'try it and see' answering to somebody "
             "who has already worked out the answer. When the maze that "
             "reordered Ravnica's politics was finally run, the Guildpact was "
             "rebuilt inside him.",
        champions=(
            Champion("Niv-Mizzet, Parun",
                     "The founder, still running it, and the smartest thing "
                     "on the plane by his own confident assessment."),
            Champion("Ral Zarek",
                     "A storm mage who worked for Niv-Mizzet, and then for "
                     "Nicol Bolas, in that order."),
            Champion("Melek, Izzet Paragon",
                     "A weird — a creature of raw elemental magic — and the "
                     "guild's proof that the experiments work."),
        ),
        signature=("Thousand-Year Storm", "Prismari Command",
                   "Goblin Electromancer")),
    Combination(
        "BG", "Golgari", "guild",
        "Death is a stage of growth, not the end of one.",
        "The Golgari Swarm runs Ravnica's undercity and its decomposition, and "
        "sees no difference between the two jobs. Black's comfort with the "
        "graveyard and green's cycles of growth make the yard a second hand: "
        "creatures that want to die, engines that recur them, and value that "
        "compounds every time something is lost.",
        verified_by="Golgari Charm",
        lore="The Swarm holds the undercity and Ravnica's whole "
             "decomposition contract, which is the leverage nobody else "
             "thinks about until it is withdrawn. Its succession is done the "
             "Golgari way: Savra ruled, then her brother Jarad, then Vraska, "
             "a gorgon planeswalker who took the guild by killing her way to "
             "the top of it. Nothing there is wasted either.",
        champions=(
            Champion("Savra, Queen of the Golgari",
                     "The Devkarin queen, and Jarad's sister — succession "
                     "here is a sacrifice outcome."),
            Champion("Jarad, Golgari Lich Lord",
                     "Guildmaster of the undercity, and dead for most of his "
                     "tenure without that slowing him down."),
            Champion("Vraska the Unseen",
                     "A gorgon planeswalker who took the Swarm by killing "
                     "her way to the top of it."),
        ),
        signature=("Assassin's Trophy", "Deathrite Shaman",
                   "The Gitrog Monster")),
    Combination(
        "WR", "Boros", "guild",
        "Righteous anger, applied immediately.",
        "The Boros Legion is white's conviction moving at red's speed. It "
        "attacks, it goes wide, it doubles its own damage, and it treats "
        "hesitation as a moral failure. Long the weakest pair in Commander "
        "for lack of card advantage, it has been the biggest beneficiary of "
        "impulse-draw and treasure effects.",
        verified_by="Boros Charm",
        lore="Boros is an army of angels that believes in due process, and "
             "the tension between those two things is its whole story. Razia "
             "founded the Legion; Aurelia leads it now, and did not inherit "
             "it quietly. A guild whose answer to injustice is a charge is "
             "always one commander away from being the thing it fights, and "
             "it knows that about itself.",
        champions=(
            Champion("Razia, Boros Archangel",
                     "The parun, and the Legion's founder — Aurelia holds "
                     "the command now."),
            Champion("Aurelia, the Warleader",
                     "The angel who leads the Legion, and did not come by it "
                     "gently."),
            Champion("Feather, the Redeemed",
                     "An angel exiled from the Legion for insubordination, "
                     "and beloved on the streets for exactly that."),
        ),
        signature=("Deflecting Palm", "Gisela, Blade of Goldnight",
                   "Warleader's Call")),
    Combination(
        "UG", "Simic", "guild",
        "Improve the organism. Then improve it again.",
        "The Simic Combine splices what works onto what nearly works. Blue's "
        "refinement plus green's growth produces ramp into card draw into "
        "enormous, carefully specified creatures — and a reputation at "
        "Commander tables for decks that do not interact until they win.",
        verified_by="Simic Charm",
        lore="The Combine began as Ravnica's research guild and stayed one "
             "long after the other nine turned political, which is why it is "
             "the guild the others notice last. Its signature product is the "
             "krasis — organisms assembled from whatever worked — and its "
             "signature failing is that it will not stop at the version that "
             "already works.",
        champions=(
            Champion("Momir Vig, Simic Visionary",
                     "The elf who ran the Combine as a laboratory, back when "
                     "that was all it was."),
            Champion("Prime Speaker Zegana",
                     "A merfolk of the Zonots, and the Combine's speaker "
                     "after it turned outward."),
            Champion("Vorel of the Hull Clade",
                     "A biomancer, and the guild's standing argument that "
                     "anything worth having is worth doubling."),
        ),
        signature=("Growth Spiral", "Coiling Oracle", "Hydroid Krasis")),

    # ---- the five shards of Alara: a colour with both its allies ----
    Combination(
        "WUG", "Bant", "shard",
        "Order, chivalry, and a hierarchy everyone accepts.",
        "Bant is white's shard. It kept blue and green and lost black and red "
        "entirely — so it has no ambition and no passion, only duty. Angels, "
        "knights, and a rigid caste system nobody questions because nobody has "
        "anything to be angry about.",
        verified_by="Bant Charm",
        lore="Bant is what a society looks like when nobody has anything to "
             "be angry about. Angels sit at the top of a caste system, every "
             "caste knows its sigil and its duty, and the arrangement holds "
             "because the shard lost the two colours that would have "
             "questioned it. When the shards were forced back together, the "
             "first thing through Bant's border was Grixis's undead — a world "
             "with no experience of an enemy, meeting one.",
        champions=(
            Champion("Rafiq of the Many",
                     "A knight of the Order of the Skyward Eye, and Bant's "
                     "own idea of what a hero is."),
            Champion("Jenara, Asura of War",
                     "An angel raised into the caste that fights, on a world "
                     "where caste is not a matter of opinion."),
        ),
        signature=("Chulane, Teller of Tales", "Derevi, Empyrial Tactician",
                   "Noble Hierarch")),
    Combination(
        "WUB", "Esper", "shard",
        "Flesh is a rough draft. Etherium is the correction.",
        "Esper is blue's shard, and it replaced its own biology with metal. "
        "White's order and black's ruthlessness serve blue's perfectionism, "
        "producing artifice, control and the format's most calculating decks. "
        "Losing red and green cost it spontaneity and growth, which Esper "
        "regards as an upgrade.",
        verified_by="Esper Charm",
        lore="Esper replaced itself. Etherium — a metal that improves "
             "whatever it is grafted to — was invented there, and the shard "
             "took that literally, until having flesh left was a "
             "statement about your standing. The catch was arithmetic: there "
             "was only ever so much etherium, and a society that measures "
             "worth in a finite substance has decided in advance who loses.",
        champions=(
            Champion("Sharuum the Hegemon",
                     "The sphinx who rules Esper, and the reason its rulers "
                     "are partly made of metal."),
            Champion("Sen Triplets",
                     "Three vedalken siblings who took the shard's appetite "
                     "for control to its logical end."),
        ),
        signature=("Zur the Enchanter", "Void Rend", "Raffine, Scheming Seer")),
    Combination(
        "UBR", "Grixis", "shard",
        "Everything is already dead. Use it anyway.",
        "Grixis is black's shard, stripped of white's compassion and green's "
        "renewal, and it ran out of living resources long ago. What is left is "
        "necromancy, scarcity and a politics of pure leverage — reanimation, "
        "theft, and the cheerful assumption that a corpse is a resource.",
        verified_by="Grixis Charm",
        lore="Grixis is Nicol Bolas's shard, and the Conflux that put Alara "
             "back together is his doing: an elder dragon stripped of most of "
             "his power spent a very long time arranging for the five "
             "fragments to collide, because what that released was what he "
             "wanted back. The place had run out of living things long "
             "before. What is left rules by necromancy, because a corpse is "
             "the only resource that has not run out.",
        champions=(
            Champion("Nicol Bolas, Planeswalker",
                     "The elder dragon whose shard this is, and who arranged "
                     "the Conflux to get his own power back."),
            Champion("Thraximundar",
                     "Bolas's general — a zombie who has to keep killing to "
                     "keep himself running."),
            Champion("Sedris, the Traitor King",
                     "A king who died and kept ruling, which on Grixis is "
                     "not a contradiction."),
        ),
        signature=("Kess, Dissident Mage", "Marchesa, the Black Rose",
                   "Nekusar, the Mindrazer")),
    Combination(
        "BRG", "Jund", "shard",
        "A food chain with no top and no rules.",
        "Jund is red's shard: black's appetite and green's ferocity with no "
        "white to organise it and no blue to think it through. Dragons at the "
        "top, everything else eating each other underneath. In play it is "
        "aggression backed by sacrifice value — nothing is wasted, because "
        "everything gets eaten eventually.",
        verified_by="Jund Charm",
        lore="Jund has no government, only a food chain, and the dragons are "
             "at the top of it. Everything else is arranged by what eats "
             "what: goblins scavenge, viashino raid, human warchiefs hold "
             "territory exactly as long as they can hold it. It is the shard "
             "that lost white and blue, so nothing on it has ever proposed a "
             "rule or thought a plan through.",
        champions=(
            Champion("Karrthus, Tyrant of Jund",
                     "The dragon at the top, which on Jund is the only "
                     "office there is."),
            Champion("Kresh the Bloodbraided",
                     "A human warchief who measures his standing by what he "
                     "has killed, and grows on it."),
        ),
        signature=("Korvold, Fae-Cursed King", "Ziatora, the Incinerator",
                   "Lord Windgrace")),
    Combination(
        "WRG", "Naya", "shard",
        "Nature at a scale that makes worship the sensible response.",
        "Naya is green's shard, keeping white's reverence and red's passion "
        "and losing blue's caution and black's self-interest. Its inhabitants "
        "venerate gargantuan beasts, and its decks do roughly that: ramp into "
        "enormous creatures and swing.",
        verified_by="Naya Charm",
        lore="On Naya everything grew. The behemoths are the size of "
             "landmarks, the jungle is the size of the world, and the nacatl "
             "built a religion around calling them rather than a strategy for "
             "fighting them — which, at that scale, is the rational response. "
             "The shard's own peace was broken from inside by Marisi, and it "
             "has been arguing about whether he was right ever since.",
        champions=(
            Champion("Mayael the Anima",
                     "The Anima, whose line calls the behemoths the nacatl "
                     "build their religion around."),
            Champion("Marisi, Breaker of the Coil",
                     "The nacatl who broke Naya's peace on purpose, and "
                     "started the argument the shard is still having."),
        ),
        signature=("Gishath, Sun's Avatar", "Zacama, Primal Calamity",
                   "Rith, the Awakener")),

    # ---- the five wedges of Tarkir: a colour with both its enemies ----
    Combination(
        "WBG", "Abzan", "wedge",
        "The family endures. The individual is how it does that.",
        "The Abzan Houses are white's endurance flanked by black and green: "
        "fortress walls, ancestors who keep contributing after death, and "
        "creatures that get harder to kill the longer the game runs. Outlast "
        "everything, and count the dead as still on the roster.",
        verified_by="Abzan Charm",
        lore="The Abzan keep their dead on the roster. The kin-tree at the "
             "centre of a house is not a memorial, it is the household — "
             "ancestors are consulted, counted and expected to contribute, "
             "and a warrior who dies has changed department rather than left. "
             "In the timeline where the dragons won, the clan is gone and "
             "Dromoka's brood holds the same desert.",
        champions=(
            Champion("Anafenza, the Foremost",
                     "Khan of the Abzan Houses, and the one who decides "
                     "which ancestors are still contributing."),
            Champion("Daghatar the Adamant",
                     "The warrior who held the houses after her, by moving "
                     "the line rather than yielding it."),
        ),
        signature=("Doran, the Siege Tower", "Ghave, Guru of Spores",
                   "Eerie Ultimatum")),
    Combination(
        "WUR", "Jeskai", "wedge",
        "Cunning, discipline, and the strike that ends it.",
        "The Jeskai Way is blue's enlightenment with white's discipline and "
        "red's decisiveness — monks who study for decades in order to act "
        "instantly. In play it is tempo: cheap interaction, prowess, and a "
        "clock that closes while the opponent is still setting up.",
        verified_by="Jeskai Charm",
        lore="The Jeskai are archivists who fight. Their monasteries kept "
             "Tarkir's records through a thousand years in which the plane's "
             "own history had been comprehensively rewritten by the people "
             "who won, and Narset is the one who read them and worked out "
             "what was missing. Decades of study, spent in a single strike, "
             "is the Way stated as a training regime.",
        champions=(
            Champion("Narset, Enlightened Master",
                     "Khan of the Jeskai, and the first in a thousand years "
                     "to read what Tarkir's history left out."),
            Champion("Shu Yun, the Silent Tempest",
                     "A monk of the Way: decades of preparation, spent on "
                     "one strike."),
        ),
        signature=("Jeskai Ascendancy", "Kykar, Wind's Fury",
                   "Whirlwind of Thought")),
    Combination(
        "UBG", "Sultai", "wedge",
        "Whatever you have is not enough. Take more.",
        "The Sultai Brood is black's ambition with blue's cunning and green's "
        "abundance, and it is the most decadent faction Magic has printed: "
        "wealth, necromancy and a total absence of restraint. Its decks fill "
        "the graveyard on purpose and treat it as a resource pool.",
        verified_by="Sultai Charm",
        lore="The Sultai solved labour by declining to let it retire. The "
             "Brood's naga royalty raise the dead to work the rice paddies "
             "and hold the courts, and spend the surplus on a level of "
             "luxury that is itself the point — the clan's ambition is not "
             "power for its own sake but appetite, publicly satisfied. Under "
             "the dragons, Silumgar keeps the court and the decadence.",
        champions=(
            Champion("Sidisi, Brood Tyrant",
                     "Khan of the Sultai, who keeps the dead on staff and "
                     "the living on notice."),
            Champion("Tasigur, the Golden Fang",
                     "A prince who bought his way onto the throne and ate "
                     "his way through it."),
        ),
        signature=("Muldrotha, the Gravetide", "Villainous Wealth",
                   "Yarok, the Desecrated")),
    Combination(
        "WBR", "Mardu", "wedge",
        "Speed is the only virtue. Stopping is how you die.",
        "The Mardu Horde is red's momentum with white's numbers and black's "
        "disregard, a raiding culture that measures worth in motion. Its decks "
        "go wide and fast, sacrifice freely, and are the wedge least "
        "interested in the late game.",
        verified_by="Mardu Charm",
        lore="The Mardu measure worth in motion, and a horde that stops is a "
             "horde that has lost. It is also the clan with the format's "
             "best-loved moment of nerve: Alesha took her name in front of "
             "the whole horde and invited anyone who objected to say so, and "
             "then led them. Her card is a good lesson in rule 2 as well as "
             "in courage — she costs {2}{R}, and her identity is all three "
             "Mardu colours, because of two hybrid pips in her ability.",
        champions=(
            Champion("Zurgo Helmsmasher",
                     "Khan of the Horde, and its longest-surviving warlord "
                     "by the only metric the Mardu keep."),
            Champion("Alesha, Who Smiles at Death",
                     "A khan who announced her name to the horde and dared "
                     "anyone to argue; nobody did, and she led them."),
        ),
        signature=("Crackling Doom", "Kaalia of the Vast",
                   "Isshin, Two Heavens as One")),
    Combination(
        "URG", "Temur", "wedge",
        "Survive the frontier, and become large enough that it survives you.",
        "The Temur Frontier is green's savagery with blue's cunning and red's "
        "ferocity — a clan that reads the land and then overwhelms it. Ramp, "
        "big creatures, and enough card selection to find the right one.",
        verified_by="Temur Charm",
        lore="The Temur live where the ice is, and their shamans' job is to "
             "read what the land is about to do before it does it. That is "
             "the whole clan in one sentence: green's endurance and red's "
             "ferocity, with just enough blue to see the avalanche coming. "
             "They do not out-think the frontier, they out-last it, and then "
             "grow to a size it stops being able to argue with.",
        champions=(
            Champion("Surrak Dragonclaw",
                     "Khan of the Temur, who leads from the front of the "
                     "hunt because there is nowhere else to lead from."),
            Champion("Yasova Dragonclaw",
                     "A Temur elder and shaman-hunter, of the generation "
                     "that read the ice for a living."),
        ),
        signature=("Animar, Soul of Elements", "Maelstrom Wanderer",
                   "Temur Ascendancy")),

    # ---- four colours: each defined by the one it refuses ----
    Combination(
        "WUBR", "Artifice", "quad",
        "Everything except green — the made over the grown.",
        "Artifice rejects green: nothing here trusts the natural order, and "
        "the answer to every problem is a better artifact. Commander 2016 "
        "named this deck Artifice and gave it Breya; EDHREC calls it "
        "Yore-Tiller after the Guildpact Nephilim with the same identity.",
        aliases=("Yore-Tiller",), verified_by="Breya, Etherium Shaper",
        signature=("Breya, Etherium Shaper", "Yore-Tiller Nephilim")),
    Combination(
        "UBRG", "Chaos", "quad",
        "Everything except white — no rules, and no one to enforce them.",
        "Chaos rejects white, and with it structure, fairness and restraint. "
        "What remains is appetite, cunning, fury and growth with nothing "
        "moderating any of it — cascade, randomness and effects that hand the "
        "outcome to chance on purpose. Yidris led the Commander 2016 deck; "
        "EDHREC calls it Glint-Eye.",
        aliases=("Glint-Eye",), verified_by="Yidris, Maelstrom Wielder",
        signature=("Yidris, Maelstrom Wielder", "Glint-Eye Nephilim")),
    Combination(
        "WBRG", "Aggression", "quad",
        "Everything except blue — act, and do not deliberate.",
        "Aggression rejects blue, which means it rejects patience, counterplay "
        "and the idea that more information would help. It is the four-colour "
        "identity that attacks. Saskia fronted the Commander 2016 deck; "
        "EDHREC calls it Dune-Brood.",
        aliases=("Dune-Brood",), verified_by="Saskia the Unyielding",
        signature=("Saskia the Unyielding", "Dune-Brood Nephilim")),
    Combination(
        "WURG", "Altruism", "quad",
        "Everything except black — generosity as a strategy.",
        "Altruism rejects black, and therefore rejects self-interest as an "
        "organising principle. It is the home of group hug: shared draw, "
        "shared ramp, symmetrical generosity, and the wager that you will use "
        "the gift better than the people you gave it to. Kynaios and Tiro led "
        "the Commander 2016 deck; EDHREC calls it Ink-Treader.",
        aliases=("Ink-Treader",), verified_by="Kynaios and Tiro of Meletis",
        signature=("Kynaios and Tiro of Meletis", "Aragorn, the Uniter",
                   "Ink-Treader Nephilim")),
    Combination(
        "WUBG", "Growth", "quad",
        "Everything except red — patience compounding into inevitability.",
        "Growth rejects red, and with it impulse, haste and the need for "
        "anything to happen right now. It accumulates: counters, permanents, "
        "value, until the board is simply better than everyone else's. Atraxa "
        "led the Commander 2016 deck and went on to become one of the "
        "format's most-built commanders; EDHREC calls this identity "
        "Witch-Maw.",
        aliases=("Witch-Maw",), verified_by="Atraxa, Praetors' Voice",
        signature=("Atraxa, Praetors' Voice", "Atraxa, Grand Unifier",
                   "Witch-Maw Nephilim")),

    Combination(
        "WUBRG", "Five-Colour", "five",
        "All of it, if the mana base can be paid for.",
        "Five-colour decks give up nothing in card choice and everything in "
        "consistency — the whole card pool is legal, and the cost is a mana "
        "base that has to produce any colour on any turn. It is the most "
        "expensive way to build a deck and the only one with no answers "
        "unavailable to it.",
        verified_by="Sliver Overlord",
        signature=("Progenitus", "Child of Alara", "The Ur-Dragon")),
)

BY_KEY = {c.key: c for c in COMBINATIONS}


def of(colors: Iterable[str]) -> Combination:
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
    """A block that named a set of colour combinations, for the carousel.

    The setting and why the names stuck — not what the combination *is*, which
    is `TIER_BLURBS` and is rendered directly above this. The two used to
    overlap badly enough that a reader met the definition of a wedge twice in
    consecutive paragraphs.
    """

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
        "Alara broke into five fragments and each fragment lost two colours "
        "outright — not suppressed, gone, with nothing left to argue against "
        "what remained. A world with no ambition; a world with no mercy. The "
        "plane was eventually put back together, and the names stayed."),
    Era("Tarkir", "A world of dragons, and then of clans, and then of dragons",
        "the five wedges",
        "Tarkir's five clans each venerated one aspect of dragonkind, and "
        "each was built around a colour flanked by the two it least agrees "
        "with — an ancestor-cult, a monastery, a raiding horde. Then someone "
        "went back through time and changed the outcome, and the dragons won "
        "instead. Tarkir has two histories, and cards printed from both."),
)
