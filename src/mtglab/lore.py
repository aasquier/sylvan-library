"""The shelves: what the library knows that no card states.

Reference prose, the third module of its kind (second punch list of
2026-08-15, item 7). `colors.py` teaches the 32 combinations, `glossary.py`
teaches the words, and this teaches everything a shelf of a Magic library
would actually hold — where the game came from, which rules used to be
different, whose brush painted the cards, how people talk about winning, and
the cards that exist mostly as stories. The Library page draws one fact at a
time from here; every fact carries a longer `more` for the reader who bites.

The same argument as the other two, and it was argued rather than assumed:
this is checked-in text and not a Claude surface because the set is finite
and written once, it answers with no card pool and no network and no key,
and bland prose is fixed by editing. What moves — the meta, the current
spoilers — is `research.py`'s job, and a fact in this file is deliberately
the kind that stopped moving years ago.

**Card facts still come from the pool.** A fact may *name* cards
(`Fact.cards`); the route resolves each through `get_cards` and ships the
card's own cost, type and text beside the prose, and a name that does not
resolve is dropped and counted — the same instrument `colors.py` uses for
champions. The full-pool test extends `test_colors.py`'s single
`needs_full_pool` test rather than adding a second marker, which would move
CI's skip gate off two. Prose in this file states things about *history and
people*; anything it says about what a card does is either trivially carried
by the resolved card itself or hedged as the legend it is.

Accuracy discipline for authors: dates and attributions here are the widely
documented ones. Where a story is folklore rather than record — the torn
Chaos Orb — the prose says "the story goes", because a library that launders
legend into fact teaches worse than one that shelves both honestly.
"""

from __future__ import annotations

from dataclasses import dataclass, field

#: The volumes, in shelf order.
VOLUMES = ("history", "mechanics", "artists", "table", "curiosities")

VOLUME_LABELS = {
    "history": "History",
    "mechanics": "Rules & mechanics",
    "artists": "The painters",
    "table": "At the table",
    "curiosities": "Curiosities",
}

VOLUME_BLURBS = {
    "history": "Where the game came from, and the turns it took on the way "
               "here.",
    "mechanics": "Rules that changed, keywords that retired, and the "
                 "machinery under the cards.",
    "artists": "The people whose paintings this app keeps hanging on every "
               "wall.",
    "table": "How people talk about building and winning — the ideas under "
             "the jargon.",
    "curiosities": "Cards that exist mostly as stories, and stories that "
                   "exist mostly as cards.",
}


@dataclass(frozen=True)
class Fact:
    """One thing from the shelves.

    `fact` is the card the Library shows — one or two sentences. `more` is
    the paragraph behind the "tell me more", and must add rather than
    restate, the same contract `glossary.py` holds between `short` and
    `long`. `cards` are names for the route to resolve through the pool;
    the prose should read whole even if every one of them drops. `learn`
    optionally points at a Learn-page anchor: `("colors", key)` or
    `("words", key)`.
    """

    key: str
    volume: str
    fact: str
    more: str
    cards: tuple[str, ...] = field(default_factory=tuple)
    learn: tuple[str, str] | None = None


FACTS: tuple[Fact, ...] = (

    # ----------------------------------------------------------- history
    Fact(
        "alpha-93", "history",
        "Magic's first printing — Limited Edition Alpha — reached shops in "
        "the summer of 1993 and sold out almost immediately.",
        "Wizards of the Coast expected the first print run to last a while; "
        "it lasted weeks. Alpha's 295 cards established almost everything "
        "the game still runs on — the five colours, lands, artifacts, "
        "instants — and a handful of things it later regretted, like a {0} "
        "artifact that makes three mana. The corners of Alpha cards are "
        "noticeably rounder than every printing since, which is how "
        "collectors tell them apart at a glance.",
        ("Black Lotus",)),
    Fact(
        "garfield", "history",
        "Richard Garfield designed Magic while finishing a PhD in "
        "combinatorial mathematics — and it was his second pitch.",
        "Garfield first brought Wizards of the Coast a board game, RoboRally. "
        "The company liked it but wanted something portable and quick to "
        "play between rounds at conventions, and Garfield came back with the "
        "idea of a game sold in pieces, where no two players owned the same "
        "cards. RoboRally did eventually get published — but only after the "
        "card game had paid for everything.",
    ),
    Fact(
        "the-gathering", "history",
        "The subtitle \"The Gathering\" exists substantially because "
        "\"Magic\" alone was too generic to protect as a trademark.",
        "The working title was simply Magic, and the team needed something "
        "distinctive enough to register. \"The Gathering\" was chosen from a "
        "list of candidates, and the plan was for later standalone sets to "
        "carry their own subtitles. The community never stopped calling the "
        "whole game Magic, and the subtitle quietly became permanent.",
    ),
    Fact(
        "power-nine", "history",
        "Nine cards from the game's first year are collectively called the "
        "Power Nine — and every one of them is banned or restricted "
        "everywhere it is legal at all.",
        "Black Lotus, the five Moxes, Ancestral Recall, Time Walk and "
        "Timetwister. Each breaks one of the costs the game was balanced "
        "around — free mana, free cards, free turns — and they were "
        "identified as a class within the game's first years. In Commander "
        "every one of the nine except Timetwister sits on the banned list; "
        "in Vintage they are restricted to one copy each and define the "
        "format anyway.",
        ("Black Lotus", "Ancestral Recall", "Time Walk", "Timetwister")),
    Fact(
        "reserved-list", "history",
        "In 1996 Wizards promised never to reprint certain cards — the "
        "Reserved List — and the promise has outlived every argument about "
        "it.",
        "The 1995 set Chronicles reprinted sought-after older cards in "
        "volume, and collectors who had paid scarcity prices were furious. "
        "The Reserved List was the company's answer: a published roster of "
        "cards that would never be functionally reprinted. Three decades "
        "on it is why the original dual lands cost what a used car costs, "
        "and why this app's decks each declare whether the Reserved List "
        "is allowed — a per-deck budget decision now, not a rules one.",
    ),
    Fact(
        "edh-origin", "history",
        "Commander began as a judges' after-hours format called Elder "
        "Dragon Highlander — one of everything, and a legendary dragon in "
        "charge.",
        "The name is literal: the original commanders were the five Elder "
        "Dragons of 1994's Legends set, Nicol Bolas the most famous among "
        "them, and \"Highlander\" is the one-copy rule, after the film's "
        "\"there can be only one\". Judge Sheldon Menery championed the "
        "format for decades and led the fan Rules Committee that governed "
        "it; Wizards printed the first official Commander decks in 2011, "
        "and stewardship of the format passed to Wizards itself in late "
        "2024. The spirit stayed the same throughout: a social format, "
        "built around a character.",
        ("Nicol Bolas",),
        ("words", "commander")),
    Fact(
        "combo-winter", "history",
        "The winter of 1998–99 is remembered as \"Combo Winter\": Urza's "
        "block combo decks won so fast that emergency bans came mid-season.",
        "Urza's Saga and its follow-ups printed free mana and free untaps in "
        "quantities the game has avoided ever since, and tournament decks "
        "began winning on the first or second turn. Memory Jar holds the "
        "record that era set: banned in an emergency announcement in March "
        "1999, weeks after release, rather than waiting for the scheduled "
        "update. The episode reshaped how the company plays it safe with "
        "\"free\" anything.",
        ("Memory Jar",)),
    Fact(
        "skullclamp", "history",
        "Skullclamp was printed costing one mana, dominated everything, and "
        "was banned within months — the textbook case of a stats change late "
        "in design.",
        "Late in development the Equipment's toughness bonus was changed to "
        "a penalty, turning it into a one-mana engine that converted any "
        "creature into two cards. It went into essentially every competitive "
        "deck of its Standard, and was banned there in June 2004. It "
        "remains legal in Commander, where it is one of the most-played "
        "Equipment cards in the format's history — power is relative to the "
        "table you bring it to.",
        ("Skullclamp",)),
    Fact(
        "planeswalker-type", "history",
        "Planeswalkers — the card type, not the story idea — did not exist "
        "until 2007, fourteen years into the game.",
        "The story had always cast players as planeswalkers, but Lorwyn was "
        "the first set to put the characters themselves on cards, with the "
        "loyalty-counter machinery invented for them. It was the first "
        "genuinely new card type since the game's beginnings — a "
        "distinction it held for sixteen years, until battles arrived in "
        "2023 — which is a measure of how rarely the frame of the game "
        "itself changes.",
    ),
    Fact(
        "ante", "history",
        "Magic originally assumed you were playing for keeps: the rules had "
        "an ante, and cards that manipulated it.",
        "Under the original rules each player set aside a random card from "
        "their deck at the start, and the winner kept both. A handful of "
        "cards — Contract from Below chief among them — were printed to be "
        "spectacular precisely because their drawback was losing cards you "
        "owned. Every ante card is banned in every sanctioned format, and "
        "they are the only cards banned for what they do *outside* the "
        "game.",
        ("Contract from Below",)),

    # --------------------------------------------------------- mechanics
    Fact(
        "banding", "mechanics",
        "Banding, from the very first set, is the keyword so notoriously "
        "hard to explain that it became the community's shorthand for "
        "over-complexity.",
        "It let creatures attack or block as a unit and, crucially, let the "
        "banding player decide how damage was assigned — inverting the "
        "usual rule and confusing everyone. It stopped appearing on new "
        "cards in the mid-1990s and has been affectionately mocked ever "
        "since, including by Wizards itself. If you ever meet it in a "
        "Commander game, someone at the table is being deliberate.",
    ),
    Fact(
        "mana-burn", "mechanics",
        "For the game's first sixteen years, unspent mana hurt you: it "
        "emptied from your pool as damage called mana burn.",
        "Mana burn was removed in the Magic 2010 rules overhaul of 2009. A "
        "few old cards were even designed around wanting the damage, and "
        "they read strangely now. The change is a clean example of the "
        "game's oldest tension: rules that model a world versus rules that "
        "play well, and playability winning.",
    ),
    Fact(
        "damage-stack", "mechanics",
        "Combat damage used to go on the stack — you could deal lethal "
        "damage and still sacrifice the creature for value before it "
        "resolved.",
        "From 1999's Sixth Edition rules until the 2009 overhaul, combat "
        "damage was an object on the stack like a spell, so a creature "
        "could \"deal its damage and then get out of the way\". The 2009 "
        "change made damage instantaneous and killed a whole family of "
        "tricks. Players still argue about this one, which after fifteen "
        "years tells you how good the tricks felt.",
    ),
    Fact(
        "the-stack", "mechanics",
        "The stack itself — the last-in-first-out rule for spells — was "
        "only formalised in 1999, six years into the game.",
        "Before Sixth Edition, timing was governed by \"batches\" and "
        "interrupt windows that even judges struggled to apply "
        "consistently. The stack replaced all of it with one rule a "
        "computer scientist would recognise immediately: spells resolve in "
        "reverse order of casting. It is the single most successful rules "
        "simplification the game has made, and every card since assumes "
        "it.",
    ),
    Fact(
        "layers", "mechanics",
        "When two continuous effects disagree, the rules resolve them in "
        "seven fixed layers — a system deep enough that judges study it "
        "like case law.",
        "Copy effects apply before control effects, control before text "
        "changes, and so on down to power and toughness, which get their "
        "own sub-layers. Nobody plays Commander thinking about layers "
        "until a stolen, animated, text-changed land is somehow still a "
        "land, and then only the layer system can say why. It exists so "
        "that any board state, however baroque, has exactly one right "
        "answer.",
    ),
    Fact(
        "commander-damage", "mechanics",
        "Twenty-one combat damage from a single commander kills you, "
        "whatever your life total — a number inherited from the format's "
        "duelling days.",
        "The original Elder Dragon Highlander rule was designed so that a "
        "voltron deck — one commander, many swords — could kill through "
        "the format's higher starting life. Twenty-one is three clean hits "
        "from a 7-power dragon, which is exactly what the five original "
        "Elder Dragons are. The rule counts each commander separately and "
        "only combat damage, both of which matter more at a four-player "
        "table than any other rule in the format.",
        (),
        ("words", "commander")),
    Fact(
        "phasing", "mechanics",
        "Phasing — a card blinking out of existence without leaving the "
        "battlefield — was a mid-90s idea that retired, then came back "
        "reformed.",
        "Mirage introduced it in 1996 and the original version confused "
        "everyone about triggers and auras. The modern game brought it "
        "back because it turned out to be the *cleanest* way to protect a "
        "permanent: a phased-out card is treated as though it does not "
        "exist, so nothing can touch it, but nothing triggers on it "
        "leaving either. Old mechanics do not die; they wait for the rules "
        "to catch up with them.",
    ),
    Fact(
        "dfc", "mechanics",
        "Cards with a second face on the back — no card back at all — were "
        "considered unthinkable until Innistrad tried it in 2011.",
        "Double-faced cards broke the oldest physical assumption the game "
        "had, that every card looks identical from behind, and required "
        "checklist cards and opaque sleeves as workarounds. They were also "
        "an immediate storytelling triumph: a werewolf that is printed as "
        "both the villager and the wolf is worth the sleeves. This app's "
        "own rule 2 exists partly because of them — a back face's colours "
        "count toward colour identity, which is exactly the kind of fact "
        "memory gets wrong.",
        (),
        ("words", "color-identity")),
    Fact(
        "hybrid-mana", "mechanics",
        "Hybrid mana — one symbol payable in either of two colours — "
        "arrived with the original Ravnica block in 2005.",
        "A {G/W} symbol is a small piece of rules design with a large "
        "consequence: it makes a card *belong* to two colours at once "
        "rather than require both. Colour identity treats every hybrid "
        "pip as both of its halves, wherever on the card it appears — a "
        "card can owe most of its identity to symbols that never show up "
        "in its mana cost — and that is one of the reasons this app never "
        "derives identity from the cost alone.",
        (),
        ("words", "color-identity")),
    Fact(
        "storm-scale", "mechanics",
        "Wizards rates the chance of a mechanic ever returning on the "
        "\"Storm Scale\" — named for Storm, the mechanic considered too "
        "dangerous to bring back.",
        "Storm copies a spell once for each spell cast before it in the "
        "turn, which turns any cheap spell into an engine payoff and warps "
        "every format it touches. Head designer Mark Rosewater's scale "
        "runs 1 (returns constantly) to 10 (expect it never to return), "
        "and Storm sits at the top as the unit of measure itself. The "
        "scale is a rare public look at how design thinks about its own "
        "mistakes.",
    ),

    # ----------------------------------------------------------- artists
    Fact(
        "christopher-rush", "artists",
        "Christopher Rush painted Black Lotus — arguably the most "
        "recognisable single painting in the history of card games.",
        "Rush was one of the twenty-five artists of the original Alpha "
        "set and stayed part of the game's look for decades; he also "
        "painted fan-favourite Lightning Bolt printings. The Lotus itself "
        "is a small, quiet still life — no battle, no wizard, just a "
        "flower — and its restraint is much of why it reads as an icon "
        "rather than an illustration. Rush passed away in 2016; the "
        "flower he painted appears on this app's shelves more than any "
        "monster does.",
        ("Black Lotus", "Lightning Bolt")),
    Fact(
        "dan-frazier", "artists",
        "Dan Frazier painted all five original Moxes — the jewelry of the "
        "Power Nine — as literal pieces of jewelry.",
        "Where other artists painted spells as events, Frazier painted "
        "the Moxes as objects you could hold: brooches and amulets on "
        "plain backgrounds. The choice aged perfectly — the cards became "
        "treasures, and the paintings already looked like treasure. "
        "Frazier has returned decades later to paint new cards in the "
        "same jewelled style, one of the game's longest visual "
        "continuities.",
        ("Mox Sapphire",)),
    Fact(
        "mark-poole", "artists",
        "Mark Poole painted Ancestral Recall, and the game's most famous "
        "library after this one: Alpha's own Library of Alexandria.",
        "Poole's airy blue style defined early blue magic — knowledge as "
        "light on water. Ancestral Recall, three cards for one mana, is "
        "the most efficient draw spell ever printed and its painting is "
        "almost abstract: a figure remembering. The app you are reading "
        "is named for a different library, but the lineage of \"a library "
        "as a place of power\" starts with Poole's.",
        ("Ancestral Recall",)),
    Fact(
        "rebecca-guay", "artists",
        "Rebecca Guay's watercolours — flowing hair, pre-Raphaelite "
        "light — were once judged \"too soft\" for the game and are now "
        "among its most collected art.",
        "Guay painted for Magic through the late 90s and 2000s in a style "
        "closer to book illustration than fantasy gaming, and for a while "
        "the art direction moved away from her. The players never did: "
        "her printings command premiums, and her Bitterblossom — a card "
        "as beautiful as it is miserable to play against — is the "
        "signature example of art and function pulling in opposite "
        "directions.",
        ("Bitterblossom",)),
    Fact(
        "john-avon", "artists",
        "John Avon has painted hundreds of basic lands — and for many "
        "players his skies, not any creature, are what Magic looks like.",
        "Basic lands are the cards players see most and think about "
        "least, and Avon turned them into the game's landscape painting "
        "tradition: luminous horizons, impossible light. His Unhinged "
        "basics — full-art, almost frameless — were so loved that "
        "full-art lands became a recurring product feature. A deck in "
        "this app can choose which printing's art it shows; Avon is "
        "usually on the shortlist.",
    ),
    Fact(
        "seb-mckinnon", "artists",
        "Seb McKinnon paints Magic like a dream having a nightmare — and "
        "funds short films from the proceeds of his own card art prints.",
        "McKinnon's work — flat, symbolist, closer to Klimt than to "
        "dungeon art — became some of the most sought-after of the "
        "modern game. He is also the clearest example of the modern "
        "artist-as-brand: his print runs and crowdfunded film projects "
        "are followed the way older collectors followed print sheets. "
        "The game's walls have never been one style; his proves they "
        "still are not.",
    ),
    Fact(
        "foglio", "artists",
        "Phil Foglio drew Magic cards as comics — panels, gags, motion "
        "lines — years before the game had an officially silly set.",
        "Foglio, already known for comics, brought cartooning into a "
        "frame that otherwise held oil paint, and his cards are "
        "immediately recognisable at across-the-table distance. The "
        "affection players hold for them helped make the case that the "
        "game could laugh at itself, which the Un-sets later proved at "
        "full length.",
    ),

    # ------------------------------------------------------------- table
    Fact(
        "card-advantage", "table",
        "\"Card advantage\" — winning by simply having more cards than "
        "your opponent — was the first deep strategic idea Magic players "
        "articulated.",
        "The insight sounds trivial and is not: if each of your cards "
        "trades for more than one of theirs, you eventually act while "
        "they cannot. Brian Weissman's mid-90s control deck, known "
        "simply as \"The Deck\", was built entirely on this arithmetic "
        "and shaped competitive Magic permanently. In Commander the "
        "arithmetic is steeper — one card that answers three players' "
        "threats is the format's holy grail, which is why board wipes "
        "and Cyclonic Rift hold the reputations they do.",
        ("Cyclonic Rift",)),
    Fact(
        "aggro-combo-control", "table",
        "Most deck talk descends from one old triangle: aggro beats "
        "control, control beats combo, combo beats aggro.",
        "Aggro spends cards fast to end the game before questions "
        "arise; control answers everything and wins late; combo ignores "
        "the fight and assembles its own ending. The triangle is a "
        "simplification and everyone knows it — but it survives because "
        "it names the three clocks a game can run on. A Commander pod "
        "usually seats all three at once, which is why the format's "
        "politics matter as much as its cards.",
    ),
    Fact(
        "ramp-etymology", "table",
        "\"Ramp\" — the whole category of accelerating your mana — is "
        "named after one specific two-mana sorcery: Rampant Growth.",
        "The card is not even the best at what it does any more, but it "
        "was emblematic enough that its name became the verb. Green "
        "ramp, artifact ramp, ritual ramp — the taxonomy this app's "
        "deck categories use descends from mid-90s shorthand. "
        "Etymologies like this are all over Magic slang: a category is "
        "usually named for the first card that made it matter.",
        ("Rampant Growth",),
        ("words", "ramp")),
    Fact(
        "goldfishing", "table",
        "Playing your deck against an imaginary opponent who does "
        "nothing is called goldfishing — and it is what this app's "
        "Tier 1 simulator does, thousands of times a second.",
        "The name is old players' humour: a goldfish is the opponent "
        "that offers no interaction whatsoever. A goldfish game answers "
        "real questions — how fast is the draw, does the mana hold up, "
        "when does the commander land — and is silent about "
        "interaction, which is exactly the caveat this app repeats "
        "every time it quotes a simulation number. The simulator is a "
        "very fast goldfish, never a very slow opponent.",
        (),
        ("words", "goldfish")),
    Fact(
        "london-mulligan", "table",
        "The modern mulligan is named after a city: the \"London\" rule "
        "was trialled at a 2019 professional tournament held there, and "
        "kept everywhere.",
        "Draw seven, keep what you like, put cards back equal to the "
        "mulligans you have taken. Earlier rules made you draw fewer "
        "each time, and bad opening hands compounded; London lets you "
        "dig for what the hand needs and give back what it does not. "
        "Commander plays it with a free first mulligan by common "
        "custom, and this app's simulator models keep rules "
        "explicitly, because the mulligan you simulate changes every "
        "number downstream.",
        (),
        ("words", "mulligan")),
    Fact(
        "politics", "table",
        "Commander is the only major format where the best play is "
        "sometimes a sentence, not a spell.",
        "With three opponents, every threat assessment is a "
        "negotiation: whose engine is scariest, who is defenceless, "
        "who benefits if you spend your removal now. Deals, threats "
        "and table talk are not outside the game — they are the "
        "format's fourth resource, after mana, cards and life. It is "
        "why a deck's power level matters less than its table manners, "
        "and why this app grades decks in brackets rather than on one "
        "axis.",
        (),
        ("words", "bracket")),
    Fact(
        "staples", "table",
        "A handful of cards appear in so many Commander decks that the "
        "format's data sites track them as \"staples\" — Sol Ring "
        "first among them.",
        "Sol Ring — one mana in, two out, every turn — is in a "
        "majority of all Commander decks ever registered, a statistic "
        "no other format tolerates for any card. Command Tower, a land "
        "with no downside in exactly this format, is close behind. A "
        "staple is worth knowing as a *baseline*: the interesting "
        "question about your 99 is rarely whether to play Sol Ring, "
        "and usually what the slots around it argue for.",
        ("Sol Ring", "Command Tower")),

    # ------------------------------------------------------- curiosities
    Fact(
        "chaos-orb", "curiosities",
        "Chaos Orb requires you to physically drop the card onto the "
        "table from a foot in the air — and the story goes that a "
        "tournament player once tore it into confetti first.",
        "The Orb destroys whatever it lands on, which made manual "
        "dexterity briefly a Magic skill. The legend — that a player "
        "ripped the Orb into pieces and scattered them across the "
        "opponent's whole board — is almost certainly folklore, and "
        "has been retold since the mid-90s anyway because it *should* "
        "be true. Dexterity cards were quietly retired; judges "
        "everywhere sleep better.",
        ("Chaos Orb",)),
    Fact(
        "shahrazad", "curiosities",
        "Shahrazad makes both players set their decks aside and play a "
        "whole separate sub-game of Magic — inside the game.",
        "The sub-game's loser loses half their life in the real one; "
        "nothing stops a second Shahrazad starting a sub-sub-game. It "
        "is banned in sanctioned play for the most honest reason on "
        "any ban list: time. Tournaments have rounds, and Scheherazade "
        "— the storyteller who survived by never finishing — is "
        "exactly the wrong patron saint for a fifty-minute clock.",
        ("Shahrazad",)),
    Fact(
        "proposal", "curiosities",
        "Richard Garfield proposed marriage with a custom Magic card — "
        "Proposal, played in a game against his fiancée.",
        "Only a handful of copies were printed, with help from the "
        "company; Garfield shuffled one into his deck and had to "
        "mulligan and dig for it while Lily Wu, allegedly, kept "
        "playing to win. She said yes. Wizards has printed a few more "
        "private family cards for the Garfields since, and they float "
        "around the collector world as the game's most personal "
        "artifacts.",
        ("Proposal",)),
    Fact(
        "one-with-nothing", "curiosities",
        "One with Nothing — discard your entire hand, get nothing — is "
        "the community's beloved shorthand for a bad card, and it is "
        "not even useless.",
        "Printed in 2005, it became the yardstick joke: \"still better "
        "than One with Nothing\". The joke is slightly unfair — "
        "against forced-discard decks or with madness cards it has "
        "genuine fringe uses, and dedicated players have built "
        "around it out of spite. Every game with fifty thousand cards "
        "needs a floor; Magic's floor has a fan club.",
        ("One with Nothing",)),
    Fact(
        "storm-crow", "curiosities",
        "Storm Crow, a two-mana 1/2 flying bird of no consequence "
        "whatsoever, is one of the most famous cards in the game.",
        "Nobody can fully explain why the community chose this one "
        "vanilla bird as its in-joke — a card so ordinary it became "
        "extraordinary, the subject of mock strategy articles and "
        "\"ban Storm Crow\" campaigns. It endures because Magic's "
        "culture runs on affectionate absurdity, and because every "
        "hobby eventually elects a mascot the founders never "
        "intended.",
        ("Storm Crow",)),
    Fact(
        "mindslaver", "curiosities",
        "Mindslaver lets you take an opponent's entire next turn — "
        "hand revealed, choices yours, regrets theirs.",
        "It is the purest expression of blue's control fantasy ever "
        "printed, and at a Commander table it is also a social "
        "grenade: piloting someone's beloved deck badly, on purpose, "
        "in front of them, is a memory the pod keeps. Locking one "
        "player out entirely by recurring it every turn is the kind "
        "of line that wins games and costs invitations. Rule 4 in "
        "this app asks every card to justify its slot; Mindslaver's "
        "why is usually a confession.",
        ("Mindslaver",)),
    Fact(
        "un-sets", "curiosities",
        "Magic has printed whole sets of jokes — silver-bordered "
        "cards where the rules themselves are the punchline.",
        "Unglued arrived in 1998 with cards requiring you to stand "
        "up, speak in rhyme, or rip a card in half; Unhinged and its "
        "successors followed. The silver border meant \"not legal "
        "anywhere\", which freed design to satirise its own game — "
        "and several ideas that debuted as jokes, like full-art "
        "lands, later crossed into the real card pool. The joke sets "
        "are where the game checks whether it can still laugh at "
        "itself.",
    ),
    Fact(
        "lotus-price", "curiosities",
        "Single copies of Black Lotus — one small 1993 still life of a "
        "flower — have sold at auction for the price of a house.",
        "Condition, printing and provenance drive the top end: an Alpha "
        "Lotus graded near-mint is a museum piece, and copies signed by "
        "artist Christopher Rush have set records. This is also why this "
        "repository's rule 5 exists — a public list of expensive cards "
        "tied to a real identity is a shopping list for somebody else. "
        "The app prices decks; it does not advertise collections.",
        ("Black Lotus",)),
)
