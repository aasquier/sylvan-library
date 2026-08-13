"""The words, for somebody who does not have them yet.

Reference data, like `colors.py` and for the same reason: no amount of card
data teaches somebody what a mulligan is, and the app is full of screens that
assume they already know. It is committed, it needs no corpus and no network,
and it is checked rather than asserted wherever a claim happens to be
checkable.

**Two kinds of term, deliberately in one table.** Half of these are Magic and
Commander vocabulary -- commander, colour identity, ramp, goldfish. The other
half are *this tool's own* controls and measures: what "Min mana pieces" means,
what a seed is for, why "spells deployed through T8" is the number the land
sweep is decided on. Both are words and numbers divorced from meaning to a
newcomer, and both want the same treatment, so they live in one table with a
`section` rather than in two modules that would drift.

Keeping the simulator's half here rather than in the React component is what
makes it checkable: the meaning of `min_pieces` belongs to `sim.tier1.engine`'s
`KeepRule`, not to a form, and `tests/test_glossary.py` fails if a control on
the screen has no entry.

`short` is one sentence and is what a tooltip shows. `long` is a paragraph and
is what the Learn page shows. Both are rendered as **plain text** -- an
asterisk meant as emphasis reaches the screen as an asterisk -- except that a
`{G}` is drawn as a mana symbol by the same `ManaText` the deck files use.
"""

from __future__ import annotations

from dataclasses import dataclass, field

#: The sections, in the order somebody should meet them.
SECTIONS = ("format", "building", "simulator")

SECTION_LABELS = {
    "format": "The format",
    "building": "Building a deck",
    "simulator": "Reading a simulation",
}

SECTION_BLURBS = {
    "format": "Commander's own rules, and the vocabulary every other screen "
              "here assumes you already have.",
    "building": "What the 99 is made of, and the words people use for the "
                "jobs a card can be doing.",
    "simulator": "Every control on the simulator and every number it reports "
                 "back, in the terms the engine actually computes them in.",
}


@dataclass(frozen=True)
class Term:
    """One word, defined twice at two lengths.

    `key` is stable and is what the UI asks for -- the simulator's controls
    look themselves up by key, so renaming one breaks a test rather than
    quietly emptying a tooltip. Keys in the `simulator` section are prefixed
    `sim.` for a parameter the caller sets and `stat.` for a number the engine
    reports, because those are two different kinds of thing to be confused by.
    """

    key: str
    term: str
    short: str
    long: str
    section: str
    #: Other keys worth reading next. Checked for orphans.
    see_also: tuple[str, ...] = field(default_factory=tuple)


TERMS: tuple[Term, ...] = (

    # ------------------------------------------------------------- format
    Term(
        "commander", "Commander",
        "The one legendary creature your deck is built around, which starts "
        "outside the deck and can be cast again every time it dies.",
        "A Commander deck is 100 cards: one commander and 99 others. The "
        "commander starts in the command zone rather than the library, so it "
        "is the only card you are guaranteed to have access to every game — "
        "which is why decks are built around what it does rather than around "
        "a card they hope to draw. When it dies or is exiled you may put it "
        "back in the command zone and cast it again.",
        "format", ("command-zone", "commander-tax", "color-identity")),
    Term(
        "command-zone", "Command zone",
        "The place your commander waits: not in the deck, not in play, and "
        "always available.",
        "A game area that exists outside the library, the hand and the "
        "battlefield. Your commander begins there and returns there whenever "
        "it would go to the graveyard or exile, if you want it to. Everything "
        "that makes Commander feel different from other formats comes from "
        "this one rule — a deck has a card it can always reach, so it can be "
        "built as an argument for that card.",
        "format", ("commander", "commander-tax")),
    Term(
        "commander-tax", "Commander tax",
        "Each time you recast your commander from the command zone it costs "
        "{2} more.",
        "The cost is cumulative and it does not reset: the second casting "
        "costs {2} extra, the third {4}, and so on for the rest of the game. "
        "It is the format's answer to a card you can never run out of, and it "
        "is why a deck that leans hard on its commander cares about protecting "
        "it rather than about recasting it.",
        "format", ("commander", "command-zone")),
    Term(
        "color-identity", "Colour identity",
        "Every coloured symbol anywhere on a card — not just its mana cost — "
        "and the rule that decides what is legal in your deck.",
        "A card's colour identity is every mana symbol in its cost and every "
        "one in its rules text, plus any colour indicator. A card may only go "
        "in your "
        "deck if its identity fits inside your commander's. This is why it can "
        "never be worked out from the mana cost alone: Alesha, Who Smiles at "
        "Death costs {2}{R} and is red-white-black, because her ability asks "
        "for two hybrid {W/B} pips. This tool always takes identity from "
        "Scryfall's own field and never derives it.",
        "format", ("commander", "hybrid", "pip")),
    Term(
        "singleton", "Singleton",
        "One copy of each card, except basic lands.",
        "No two cards in a Commander deck may share a name, however many "
        "copies you own. Basic lands are the exception and may be repeated "
        "freely. It is the rule that makes the format's decks feel different "
        "from each other game to game, and it is the reason a deck's "
        "consistency is a question worth simulating rather than assuming.",
        "format", ("mulligan",)),
    Term(
        "mulligan", "Mulligan",
        "Taking a new opening hand, then putting one card from it back on the "
        "bottom for each mulligan you took.",
        "Commander uses the London mulligan: you draw a fresh seven every "
        "time, and once you keep, you put that many cards from the hand on the "
        "bottom of your library. So a hand kept after two mulligans is a seven "
        "you choose five cards out of. The first one is free in most Commander "
        "play. What counts as keepable is a real strategic choice, which is "
        "why the simulator makes it a parameter rather than a default.",
        "format", ("sim.min_lands", "sim.min_pieces", "stat.mulligan_rate")),
    Term(
        "bracket", "Bracket",
        "A one-to-five scale for how hard a Commander deck is trying, so a "
        "table can agree on a game before it starts.",
        "Brackets are a shared vocabulary for power level: roughly, 1 is a "
        "theme deck, 3 is an upgraded precon, 4 is optimised, and 5 is cEDH, "
        "where the deck is built to win as fast as the format allows. The "
        "point is the conversation rather than the number — two decks in the "
        "same bracket should produce a game both players wanted.",
        "format", ("game-changer",)),
    Term(
        "game-changer", "Game Changer",
        "A card on an official list of the most bracket-defining cards in the "
        "format.",
        "A maintained list of cards powerful enough that their presence says "
        "something about which bracket a deck belongs in. It is not a ban "
        "list and it is not a judgement about whether a card is fun; it is a "
        "signal, and the number of them in a deck is one of the things a table "
        "can usefully compare.",
        "format", ("bracket",)),
    Term(
        "companion", "Companion",
        "A card kept outside the deck that may be brought in once, if the "
        "whole deck obeys a restriction it names.",
        "A companion sits beside the command zone and can be moved to your "
        "hand once for {3}, but only if your deck satisfied its condition when "
        "you started — Kaheera, the Orphanguard asks that every creature in "
        "the deck be one of a short list of types. It is a real deckbuilding "
        "cost paid in advance, and this tool checks it as part of the gate.",
        "format", ("partner", "commander")),
    Term(
        "partner", "Partner",
        "Two commanders instead of one, when both cards allow it.",
        "Some legends say 'Partner', which lets a deck run two commanders and "
        "combine their colour identities. Related mechanics do the same thing "
        "in different clothes: Partner With names a specific other card, "
        "Background pairs a legend with an enchantment, and a Doctor pairs with "
        "a companion character. The combined identity is what the deck is "
        "built to.",
        "format", ("commander", "color-identity", "companion")),

    # ----------------------------------------------------------- building
    Term(
        "ramp", "Ramp",
        "Cards that give you more mana earlier than your land drops would.",
        "Extra lands (Cultivate), creatures that tap for mana (Llanowar "
        "Elves) and artifacts that do the same (a signet) all count. Commander "
        "starts at 40 life and games run long, so casting your deck ahead of "
        "schedule is worth more here than in almost any other format — which "
        "is also why a deck's early ramp is what the simulator's keep rule is "
        "most sensitive to.",
        "building", ("sim.min_pieces", "mana-base")),
    Term(
        "mana-base", "Mana base",
        "The lands, and whatever else is there to make the colours you need "
        "on the turn you need them.",
        "Two jobs, and they are not the same job: producing enough mana, and "
        "producing the right colours. A deck can have plenty of lands "
        "and still be unable to cast the card in its hand. That second failure "
        "is what the simulator reports separately as a colour-only block, "
        "because it is fixed by different cards from the first.",
        "building", ("ramp", "stat.color_screw_rate", "color-identity")),
    Term(
        "pip", "Pip",
        "One coloured mana symbol in a cost. {1}{G}{W} has two pips and one "
        "generic.",
        "The coloured symbols are the demanding part of a cost: generic mana "
        "can be paid with anything, a pip cannot. A card with heavy pips — "
        "{U}{U}{U} — is a real constraint on a mana base in a way its total "
        "cost does not show, and it is the difference the castability solver "
        "in this tool exists to model.",
        "building", ("mana-value", "hybrid", "mana-base")),
    Term(
        "mana-value", "Mana value",
        "The total cost of a card as a single number. {2}{G}{W} is 4.",
        "Formerly 'converted mana cost'. It counts every symbol, coloured or "
        "not, and it is what a curve is drawn against. It says nothing about "
        "which colours a card needs, which is why a deck's mana value curve "
        "and its colour requirements are two separate problems.",
        "building", ("pip", "curve")),
    Term(
        "curve", "Curve",
        "The spread of mana values across a deck — how many cheap cards, how "
        "many expensive ones.",
        "A deck with everything at five is a deck that does nothing for four "
        "turns. A deck with everything at one runs out of things to do. The "
        "curve is the shape in between, and the simulator's board development "
        "chart is the curve seen from the other end: what actually got cast, "
        "rather than what was affordable in principle.",
        "building", ("mana-value", "stat.spells_through_t8")),
    Term(
        "hybrid", "Hybrid mana",
        "A symbol payable with either of two colours, written {G/W}.",
        "A hybrid pip can be paid with either colour, which makes it flexible "
        "to cast — but it still counts as both colours for colour identity. "
        "That is the trap: a card that is easy to cast in one colour can still "
        "be illegal in a deck of that colour. Phyrexian mana ({G/P}) has the "
        "same shape, paid with two life instead of a second colour.",
        "building", ("pip", "color-identity")),
    Term(
        "tutor", "Tutor",
        "A card that searches your library for another card.",
        "Named after Demonic Tutor. In a singleton format a tutor is how a "
        "deck reaches the one copy of the card it wants, so tutors are what "
        "turn a 99-card deck into something that can be relied on to do a "
        "specific thing. This tool's simulation does not model them, which is "
        "part of why its numbers are a floor rather than a forecast.",
        "building", ("singleton", "goldfish")),
    Term(
        "goldfish", "Goldfishing",
        "Playing a deck against nobody, to see what it does when nothing "
        "goes wrong.",
        "The name comes from the joke that you could be playing against a "
        "goldfish. It is genuinely useful — how fast the deck deploys, how "
        "often it stumbles on mana — and genuinely limited, because nobody is "
        "removing your commander or countering your ramp. The simulator here "
        "is a goldfish, at 20,000 games instead of one.",
        "building", ("stat.spells_through_t8", "sim.games")),
    Term(
        "flood", "Flood and screw",
        "Flood is drawing too many lands; screw is drawing too few.",
        "The two ways a mana base fails, and they pull in opposite "
        "directions: every land you add makes screw less likely and flood more "
        "likely. That trade is exactly what the land sweep measures, and it is "
        "why the answer is not simply 'more lands' — flooding costs you the "
        "spells you would rather have drawn.",
        "building", ("mana-base", "stat.deployment_spread",
                     "stat.wasted_through_t8")),
    Term(
        "wincon", "Win condition",
        "The specific way the deck expects to end the game.",
        "Combat damage, commander damage, a combo, or milling everyone out. "
        "Naming it matters more in Commander than elsewhere: 40 life across "
        "three opponents is a lot of damage, and a deck that generates value "
        "beautifully and has no way to convert it is a deck that loses slowly. "
        "Every card in a deck here should be able to say which one it serves.",
        "building", ("bracket",)),
    Term(
        "why", "The `why`",
        "The reason a card is in the deck, written by the person whose deck "
        "it is.",
        "Every card in a deck file here carries a rationale, and validation "
        "refuses a deck that has a blank one. It is the rule the rest of the "
        "tool is arranged around: a card that cannot justify its slot is a "
        "card to cut. Nothing in this app will write one for you — not the "
        "editor, and not Claude. It will argue with one, which is the useful "
        "part.",
        "building", ("wincon",)),

    # ---------------------------------------------------------- simulator
    Term(
        "sim.games", "Games",
        "How many games to shuffle and play. More games, less noise.",
        "Every number the simulator reports is an average over this many "
        "games. A few hundred is enough to see a big difference and not enough "
        "to trust a small one; 20,000 is the default because it makes the "
        "run-to-run wobble smaller than any difference worth acting on. It "
        "costs seconds, not minutes.",
        "simulator", ("sim.seed", "goldfish")),
    Term(
        "sim.seed", "Seed",
        "The number the shuffler starts from. The same seed gives the same "
        "games, every time.",
        "Runs are seeded so that looking at a deck twice gives the same "
        "answer and costs nothing the second time — results are cached against "
        "the seed and the compiled deck. Wanting a fresh sample is legitimate, "
        "and that is what New sample is: it picks a seed nobody would pick "
        "twice. A number quoted from a cached run says so.",
        "simulator", ("sim.games",)),
    Term(
        "sim.min_lands", "Keep min lands",
        "Mulligan any opening hand with fewer lands than this.",
        "Half of the mulligan policy: the fewest lands you are willing to "
        "keep seven cards on. This is a real strategic lever rather than a "
        "default — a deck full of cheap ramp can keep hands a land-hungry "
        "deck cannot — and moving it changes the mulligan rate and every "
        "number downstream of it.",
        "simulator", ("mulligan", "sim.max_lands", "sim.min_pieces")),
    Term(
        "sim.max_lands", "Keep max lands",
        "Mulligan any opening hand with more lands than this.",
        "The other half of the land bound, and the one people forget. A hand "
        "of six lands and a spell is a keep by every rule about having enough "
        "mana and is still a hand that does nothing. Setting a ceiling is how "
        "the model is told that flood in the opener is a reason to shuffle.",
        "simulator", ("mulligan", "sim.min_lands", "flood")),
    Term(
        "sim.min_pieces", "Min mana pieces",
        "Lands plus cheap ramp must reach this number, or the hand is "
        "mulliganed.",
        "A 'piece' is a land or a non-land ramp card of mana value 2 or less, "
        "counted together — so a hand of two lands and a Llanowar Elves has "
        "three. This is the parameter most worth tuning per deck, because it "
        "is the one that knows the difference between a deck that can keep two "
        "lands and a deck that cannot.",
        "simulator", ("ramp", "mulligan", "sim.min_lands")),
    Term(
        "sim.land_range", "From lands / To lands",
        "The range of land counts to try, one simulation each.",
        "The sweep rebuilds the deck at every land count in this range and "
        "runs a full simulation for each, then compares them. A range of 30 to "
        "40 is eleven simulations. Results are cached per count, so widening a "
        "range you have already run only pays for the new rows.",
        "simulator", ("stat.deployment_spread", "stat.argmax_lands")),
    Term(
        "stat.mulligan_rate", "Mulligan rate",
        "The share of games in which at least one hand was thrown back.",
        "Counted against the keep rule you set, so it measures the policy as "
        "much as the deck. A high rate is not automatically bad — a strict "
        "policy that mulligans often may still deploy faster than a loose one "
        "that keeps everything — which is why it is read next to the "
        "deployment numbers rather than on its own.",
        "simulator", ("mulligan", "sim.min_pieces")),
    Term(
        "stat.median_commander_turn", "Median commander turn",
        "The turn by which half of all games had the commander on the "
        "battlefield.",
        "A median rather than an average, because the games where the "
        "commander never arrives would drag a mean somewhere meaningless. It "
        "rises and falls with ramp and with the commander's own cost, and it "
        "always improves with more lands — which is exactly why it is the "
        "wrong number to choose a land count on.",
        "simulator", ("stat.never_cast_commander",
                      "stat.spells_through_t8")),
    Term(
        "stat.never_cast_commander", "Never cast by T12",
        "The share of games where the commander was still uncast when the "
        "simulation stopped.",
        "The tail the median hides. A deck can have a comfortable median "
        "commander turn and still whiff outright in one game in ten, and that "
        "is the number a mana base is usually fixed against. It counts only "
        "mana: nothing in this model removes or counters anything.",
        "simulator", ("stat.median_commander_turn", "goldfish")),
    Term(
        "stat.color_screw_rate", "Colour-only blocks",
        "Turns per game where a card was affordable by total mana but not by "
        "colour.",
        "The measurement that separates the two jobs of a mana base. It "
        "counts a turn where something in hand had a mana value the board "
        "could pay and still could not be cast, because the colours were "
        "wrong. That failure is fixed by fixing, not by more lands, so it is "
        "reported on its own rather than folded into the rest.",
        "simulator", ("mana-base", "color-identity", "pip")),
    Term(
        "stat.spells_through_t8", "Spells deployed through T8",
        "The total number of spells cast in the first eight turns. This is "
        "the number a land count is chosen on.",
        "Commander speed rises with every land you add, so optimising it "
        "alone always recommends more lands. Deployment does not: it climbs, "
        "peaks, and then falls as flood sets in. That peak is the answer, and "
        "it is the only measure here that prices both failures at once.",
        "simulator", ("flood", "stat.argmax_lands",
                      "stat.deployment_spread")),
    Term(
        "stat.wasted_through_t8", "Wasted mana",
        "Mana that was available and never spent, summed over the first eight "
        "turns.",
        "The cost of flooding, made into a number. Without it a model has no "
        "reason not to recommend sixty lands — every extra land makes the "
        "commander land sooner and the mulligan rate fall, and only the mana "
        "sitting unused says what that bought.",
        "simulator", ("flood", "stat.spells_through_t8")),
    Term(
        "stat.deployment_spread", "Deployment spread",
        "Best minus worst deployment across the whole land range. A small "
        "spread means the choice does not matter much.",
        "If every land count in the range deploys within a fraction of a "
        "spell of every other, there is no peak to find and the arithmetic "
        "maximum is noise. The simulator says so in those words rather than "
        "reporting a recommendation it cannot support, and points at mulligan "
        "rate and wasted mana instead.",
        "simulator", ("stat.spells_through_t8", "stat.argmax_lands")),
    Term(
        "stat.argmax_lands", "Highest deployment",
        "The land count in the range that deployed the most spells through "
        "turn 8.",
        "The sweep's answer, and only as good as the spread it sits in. Read "
        "it together with the deployment spread: a clear peak is a "
        "recommendation, and a flat range means this is simply the largest of "
        "a set of numbers that are all the same.",
        "simulator", ("stat.deployment_spread", "sim.land_range")),
    Term(
        "tier-1", "Tier 1",
        "This simulator: shuffle, draw, pay costs, repeat. No opponents.",
        "Tier 1 models mana and nothing else. It does not know about "
        "opponents, interaction, tutors, cost reduction, or any card text "
        "beyond producing mana and fetching lands. That narrowness is what "
        "makes its numbers trustworthy for the questions it does answer, and "
        "it is why every result it gives you carries its caveat.",
        "simulator", ("goldfish", "tier-3")),
    Term(
        "tier-3", "Tier 3",
        "Forge — a real rules engine playing real games, with a real AI.",
        "Where Tier 1 stops, Forge starts: it plays the whole game, with "
        "opponents and interaction. The catch is that its AI is good with "
        "aggro and midrange, poor with control and bad with most combo, so its "
        "results are reported per archetype and never as a single ranking. It "
        "runs from the command line rather than from this app.",
        "simulator", ("tier-1",)),
)

BY_KEY = {t.key: t for t in TERMS}


def by_section(section: str) -> list[Term]:
    return [t for t in TERMS if t.section == section]


def get(key: str) -> Term | None:
    return BY_KEY.get(key)
