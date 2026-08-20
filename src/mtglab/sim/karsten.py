"""Tier 1.5: the closed form. What arithmetic can answer without shuffling.

Tier 1 shuffles a deck twenty thousand times and counts what happened. This
module asks the same questions of the *same compiled 99* and answers them with
a hypergeometric distribution instead -- exactly, in milliseconds, with no
sampling error and no seed.

That is not a replacement for Tier 1 and it must never be rendered as one. The
two disagree, and **where they disagree is the whole point**:

    the closed form is exact about a simplified game
    the simulation is approximate about a fuller one

The closed form here assumes you keep your opening seven, play a land every
turn, and that every mana source you have drawn is a mana source you have in
play. Tier 1 assumes none of that -- it mulligans, it plays tapped lands
tapped, it makes a creature wait a turn for summoning sickness, and it spends
mana on the wrong thing because a real deck has more than one thing to cast.
It also *ramps*, which this cannot: a compiled Cultivate produces no mana and
so is invisible here.

**Which way that cuts is a fact about your deck, not about the method**, and
it was worth measuring rather than assuming. On the commander -- the one card
both models answer the same question about, since it is always available --
the closed form read against 6,000 Tier 1 games on 2026-08-21:

    Arahbo    44.9% vs 60.0%   (+15.1 to the simulation)
    Goreclaw  68.7% vs 81.9%   (+13.2)
    Tivit     25.5% vs 32.9%   ( +7.4)
    Trostani  64.5% vs 61.2%   ( -3.4)
    Gyome     80.1% vs 69.7%   (-10.4)
    Atla      54.0% vs 41.6%   (-12.3)

Two forces, pulling opposite ways: ramp the arithmetic cannot see, against
tapped lands, colour screw, mulligans and every turn the mana went somewhere
else. Mono-green Goreclaw is dominated by its dorks; three-colour Atla Palani
is dominated by its tapped lands. **The gap is a reading of which force runs
your deck**, and that is the number worth putting on a screen -- far more so
than either figure alone, and the reason this renders beside Tier 1 rather
than instead of it.

One caution that governs every other card: `castable_odds` asks *"if this card
were in your hand, could you pay for it?"* -- not *"will you have cast it?"*.
The commander is the only card where those coincide. Sylvan Safekeeper is 96.7%
castable on turn 1 and cast in 4.3% of games, because the other 92% is simply
not having drawn it. Both numbers are correct and they are answers to different
questions; a surface that stacks them in one column is lying with true figures.

Named for Frank Karsten, whose hypergeometric analysis of coloured mana
requirements is the method this implements. **The numbers are not his table.**
Nothing here is copied from a published figure: `required_sources` recomputes
the requirement from this deck's real size, this card's real pips and this
turn's real card count, which is why it answers for a 99-card singleton deck
without anybody having to find a 99-card row.

They also do not match his table, and the difference is the assumption above
rather than an error in either. He models the London mulligan; this does not,
because Tier 1 sitting beside it models mulligans properly and a second,
worse mulligan model would be the confusing thing to add. Digging for a
keepable hand is worth roughly two sources: at 14 white sources in 60 cards
his table reads 90% for a turn-one single pip and the arithmetic here reads
86.1%, so **every requirement this module reports is a shade stricter than the
published one**. Stricter in a known direction is a usable number; silently
different is not, and `tests/test_karsten.py` pins the gap so it stays known.

Stdlib only, per CLAUDE.md's rule for the simulation core: `math.comb` is the
entire dependency. No model is consulted here and none ever should be -- every
number in this module has a right answer.
"""

from __future__ import annotations

from dataclasses import dataclass
from functools import lru_cache
from math import comb, fsum

from mtglab.mana import ManaCost
from mtglab.sim.tier1.engine import SimCard

#: The consistency bar every requirement here is measured against. Karsten's
#: choice, and it is a choice rather than a fact: 90% means one game in ten
#: goes wrong on colour, which is the most a deck can reasonably ask for
#: without spending its whole list on lands. Exposed so a cEDH deck can ask
#: for more and a kitchen-table deck can ask for less.
TARGET = 0.90

#: Turns the shelf reports on. Past this a Commander deck has drawn a quarter
#: of itself and the arithmetic stops being interesting -- everything is
#: castable, which is a statement about turn 12 rather than about the deck.
HORIZON = 10


# ------------------------------------------------------------ the arithmetic

@lru_cache(maxsize=100_000)
def hypergeometric_at_least(population: int, successes: int,
                            draws: int, wanted: int) -> float:
    """P(at least `wanted` successes) when drawing `draws` from `population`.

    The one piece of mathematics this module is built on. Cached because the
    heatmap asks for the same (deck, sources, cards seen, pips) tuple once per
    card that shares a cost, and a 99-card deck asks a few thousand times.

    Exact rational arithmetic via `math.comb`, then one division -- not a
    normal approximation, which is wrong in exactly the tail this cares about.
    """
    if wanted <= 0:
        return 1.0
    if successes < wanted or draws < wanted:
        return 0.0
    draws = min(draws, population)
    total = comb(population, draws)
    if total == 0:
        return 0.0
    # Sum the complement when it is the shorter sum: P(X >= k) needs
    # min(successes, draws) - k + 1 terms and P(X < k) needs k. For a
    # single pip on turn 1 the complement is one term against twenty.
    if wanted <= (min(successes, draws) - wanted + 1):
        below = sum(
            comb(successes, i) * comb(population - successes, draws - i)
            for i in range(wanted)
            if 0 <= draws - i <= population - successes
        )
        return max(0.0, 1.0 - below / total)
    hits = sum(
        comb(successes, i) * comb(population - successes, draws - i)
        for i in range(wanted, min(successes, draws) + 1)
        if 0 <= draws - i <= population - successes
    )
    return min(1.0, hits / total)


@lru_cache(maxsize=100_000)
def _exactly(population: int, successes: int, draws: int, count: int) -> float:
    """P(exactly `count` successes). The conditioning step's weight."""
    draws = min(draws, population)
    if count > successes or count > draws or draws - count > population - successes:
        return 0.0
    total = comb(population, draws)
    if total == 0:
        return 0.0
    return comb(successes, count) * comb(population - successes, draws - count) / total


def cards_seen(turn: int, *, on_the_play: bool = True) -> int:
    """How many cards you have looked at by turn `turn`.

    Seven plus a draw step per turn, minus the one the starting player skips.
    On the play is the default because it is the harder case, and a mana base
    that holds on the play holds on the draw.
    """
    return 7 + max(0, turn - 1) if on_the_play else 7 + max(0, turn)


def required_sources(*, deck_size: int, pips: int, turn: int,
                     target: float = TARGET,
                     on_the_play: bool = True) -> int:
    """The fewest sources of one colour that cast a `pips`-pip spell on curve.

    This is the number the shelf is for. It is recomputed rather than looked
    up: pass a 99-card deck and it answers for 99, which is why there is no
    table in this file to go stale.

    Monotone in the source count, so a linear scan from zero is both correct
    and the clearest thing to read; the deck sizes involved make anything
    cleverer a micro-optimisation of a loop that runs at most 99 times.
    """
    if pips <= 0:
        return 0
    seen = cards_seen(turn, on_the_play=on_the_play)
    for count in range(pips, deck_size + 1):
        if hypergeometric_at_least(deck_size, count, seen, pips) >= target:
            return count
    return deck_size


# ------------------------------------------------------- reading the deck

def _pip_demand(cost: ManaCost) -> dict[frozenset[str], int]:
    """The card's coloured demand, grouped by what may pay it.

    Keyed on the pip's own colour *set*, so a hybrid {G/W} lands in its own
    bucket rather than being charged to green or to white. Phyrexian is absent
    by construction -- `ManaCost` keeps it apart precisely because two life
    always pays it, so it places no demand on a mana base.
    """
    demand: dict[frozenset[str], int] = {}
    for pip in cost.pips:
        demand[pip] = demand.get(pip, 0) + 1
    return demand


def sources_for(library: list[SimCard], colors: frozenset[str]) -> int:
    """How many cards in the deck can produce a colour in `colors`.

    Counts every mana-producing permanent, not only lands: a Signet and a dual
    land both answer the question "can I make white this turn", and a shelf
    that counted lands alone would tell an artifact deck it was short of
    colours it has plenty of. Lands are reported separately by `shelf` so the
    split stays visible.
    """
    return sum(1 for c in library
               if any(src.colors & colors for src in c.produces))


@dataclass(frozen=True)
class PipTier:
    """One colour at one pip count: what that demand needs, and who makes it.

    The shelf reports a colour as a *ladder* of these rather than as a single
    verdict, and that is a commandment 2 decision before it is a modelling one.
    A mono-green deck with 37 green sources meets every single- and double-pip
    demand it has and fails only its one triple-pip card; collapsing that to
    "green: short by 11" tells a beginner their mana base is broken when what
    is true is that one card is greedy. The ladder says which rung broke.
    """

    pips: int
    #: The turn this is judged on: the cheapest card in the deck making this
    #: demand, since that is the earliest the deck asks the question.
    turn: int
    #: Sources this demand wants at `TARGET`, and what the deck has.
    need: int
    have: int
    #: P(making this demand on `turn`) at the count the deck actually has.
    odds_now: float
    #: The cards making this demand, cheapest first. Named, because "which
    #: card is doing this to me" is the actionable half of the answer.
    cards: tuple[str, ...]

    @property
    def met(self) -> bool:
        return self.have >= self.need

    @property
    def shortfall(self) -> int:
        return max(0, self.need - self.have)


@dataclass(frozen=True)
class ColorRequirement:
    """One colour, what the deck holds, and what its own cards demand.

    `tiers` is the ladder. `met` is true only when every rung is, but a deck
    that fails one rung is not the same as a deck that fails all of them, and
    the surface renders the rungs rather than the verdict.
    """

    color: str
    #: Every producer of this colour in the 99.
    have: int
    #: Of those, the ones that are lands.
    have_lands: int
    tiers: tuple[PipTier, ...]

    @property
    def met(self) -> bool:
        return all(t.met for t in self.tiers)

    @property
    def worst(self) -> PipTier | None:
        """The rung that fails hardest, or the most demanding one if all pass."""
        if not self.tiers:
            return None
        return max(self.tiers, key=lambda t: (t.shortfall, t.pips))

    @property
    def shortfall(self) -> int:
        worst = self.worst
        return worst.shortfall if worst else 0


@dataclass(frozen=True)
class CardOdds:
    """One card's chance of being castable on each turn, turns 1..HORIZON.

    `on_curve` is the entry for the card's own mana value -- the turn it was
    designed for -- and is the number worth sorting a deck by.
    """

    name: str
    mv: int
    #: Index 0 is turn 1. Length is always `HORIZON`.
    by_turn: tuple[float, ...]

    @property
    def on_curve(self) -> float | None:
        """P(castable on the turn it costs), or None past the horizon.

        None rather than zero for a card costing more than `HORIZON`. Zero
        would be a claim -- that a twelve-drop is uncastable -- and what is
        true is that this shelf was not asked about turn 12. A number that
        cannot tell "no" from "not asked" is how a sorted list ends up leading
        with every expensive card in the deck.
        """
        if not 1 <= self.mv <= HORIZON:
            return None
        return self.by_turn[self.mv - 1]

    @property
    def reliable_turn(self) -> int | None:
        """The first turn this card is castable at `TARGET`, if ever by then."""
        for index, odds in enumerate(self.by_turn):
            if odds >= TARGET:
                return index + 1
        return None

    @property
    def lag(self) -> int | None:
        """Turns between what the card costs and when you can rely on it.

        The number this whole table exists to produce, and the closed-form
        twin of `CardTiming.median_turn`'s "fun number". A three-drop you
        cannot rely on until turn six is three turns late, and a deck's worst
        lags are its real mana problems -- unlike raw castability, which just
        ranks the deck by mana value and tells you the expensive cards are
        expensive.
        """
        reliable = self.reliable_turn
        return None if reliable is None else reliable - self.mv

    @property
    def lateness(self) -> int:
        """`lag`, with a floor for cards that never get there. Sort on this.

        A card that never reaches `TARGET` inside the horizon has no lag, and
        the ranking still has to place it. Charging it *at least* the lag it
        would have if it arrived one turn past the horizon is what makes the
        list read correctly, because it normalises by what the card costs: a
        three-drop that never becomes reliable is eight turns late, and an
        eight-drop that never becomes reliable is three. The three-drop is the
        real complaint, and this is the arithmetic that says so.

        It also puts anything costing more than the horizon *last*, at a
        negative lateness -- which is right, because the shelf was never asked
        about turn 12 and a list led by twelve-drops answers no question
        anybody had.
        """
        lag = self.lag
        return (HORIZON + 1 - self.mv) if lag is None else lag


def castable_odds(card: SimCard, library: list[SimCard], *, turn: int,
                  on_the_play: bool = True) -> float:
    """P(the mana for this card is there on `turn`), given the deck it lives in.

    **Assuming the card is in your hand.** This is a question about the mana
    base and only about the mana base: it does not ask whether you drew the
    card, and for anything except the commander those are very different
    numbers -- see the module docstring, where a 96.7% turn-one card is cast in
    4.3% of games. Every surface rendering this must say which one it is
    showing.

    The model, stated plainly because the number is only as honest as it is:

    1. You must have reached the turn at all -- nothing with mana value 4 is
       castable on turn 3, whatever you drew.
    2. You must have drawn at least `mana_value` mana sources, and they must
       all be in play. That is the optimistic half: it plays tapped lands as
       though untapped and casts a mana creature the turn it lands.
    3. Given exactly `n` sources drawn, the coloured requirements are checked
       *within those n* rather than against the deck at large.

    Step 3 is what makes this worth writing instead of multiplying two
    unconditional probabilities together. In a mono-green deck "I have three
    lands" and "I have two green sources" are very nearly the same event, and
    multiplying them prices the deck as though they were independent -- which
    reports the most consistent mana base in Magic as a coin flip. Conditioning
    on the draw fixes that, and makes the answer **exact** for any card whose
    coloured demand is a single colour, which is most of them.

    Where a card demands two or more different colours the conditional terms
    are still multiplied, so those remain an approximation. It errs low --
    duals and any-colour sources satisfy two demands at once and this counts
    them once each -- and `shelf` reports which cards are affected rather than
    hiding it.
    """
    mv = card.mv
    if mv > turn:
        return 0.0
    deck_size = len(library)
    if deck_size == 0:
        return 0.0

    seen = cards_seen(turn, on_the_play=on_the_play)
    pool = sum(1 for c in library if c.produces)
    if pool < mv:
        return 0.0

    demand = _pip_demand(card.cost)
    # Free spells are a land-count question and nothing else.
    if not demand:
        return hypergeometric_at_least(deck_size, pool, seen, mv)

    # How many of the deck's sources answer each distinct pip set.
    answering = {colors: sources_for(library, colors) for colors in demand}

    # Condition on the number of sources drawn. `drawn` cannot exceed the turn
    # for lands, but a deck's sources are not all lands and a rock cast on turn
    # two is producing on turn two, so the ceiling here is the draw itself.
    terms = []
    for drawn in range(mv, min(seen, pool) + 1):
        weight = _exactly(deck_size, pool, seen, drawn)
        if weight == 0.0:
            continue
        odds = 1.0
        for colors, pips in demand.items():
            odds *= hypergeometric_at_least(pool, answering[colors], drawn, pips)
            if odds == 0.0:
                break
        terms.append(weight * odds)
    # fsum rather than sum: a hundred small products of probabilities is
    # exactly the shape that accumulates float error worth avoiding when the
    # answer is rendered as a percentage to one decimal place.
    return min(1.0, fsum(terms))


# ------------------------------------------------------- the land count

#: Frank Karsten's regression over winning 60-card tournament decklists
#: ("How Many Lands Do You Need?", ChannelFireball). An intercept, a slope on
#: average mana value, and a discount per cheap accelerant.
#:
#: These three numbers are the only figures in this module that were *read*
#: rather than derived, and they are kept together, named, and cited here so
#: that stays obvious. Everything they are multiplied by -- the curve, the
#: accelerant count, the deck's size -- is measured off the compiled 99.
_FIT_INTERCEPT = 19.59
_FIT_PER_MANA_VALUE = 1.90
_FIT_PER_CHEAP_ACCELERANT = 0.28

#: The size the fit was made at. A 99-card singleton deck draws the same
#: fraction of itself per turn as a 60-card deck draws of itself, so the
#: recommendation scales with deck size rather than surviving unchanged.
_FIT_DECK_SIZE = 60


@dataclass(frozen=True)
class LandEstimate:
    """A land count from the deck's curve, and everything it did not know.

    `caveats` is not decoration. This is a regression fit to 60-card
    tournament decks and rescaled, so it is a starting point rather than an
    answer, and the three inputs it cannot see from a compiled deck are named
    in it every time. Read it against the land sweep, which simulates this
    actual deck and prices flood, and against `lands_now`.
    """

    lands_now: int
    recommended: int
    average_mana_value: float
    cheap_accelerants: int
    deck_size: int
    caveats: tuple[str, ...]

    @property
    def delta(self) -> int:
        return self.recommended - self.lands_now


def regression_lands(library: list[SimCard]) -> LandEstimate:
    """A land count for this deck's curve, in arithmetic rather than games.

    Three inputs, all measured here: the average mana value of the nonland
    cards, the number of cheap accelerants, and the deck's size. The published
    fit carries three more terms this cannot supply, and they are returned as
    caveats rather than silently treated as zero -- each one would lower the
    recommendation, so the number errs high and says so.
    """
    deck_size = len(library)
    lands_now = sum(1 for c in library if c.is_land)
    spells = [c for c in library if not c.is_land]
    if not spells or deck_size == 0:
        return LandEstimate(lands_now, lands_now, 0.0, 0, deck_size,
                            ("this deck has no nonland cards to fit a curve to",))

    average = sum(c.mv for c in spells) / len(spells)
    # Karsten's discount is for cheap card draw *and* cheap ramp. A compiled
    # card knows what it produces and knows nothing about drawing cards, so
    # only the ramp half is counted and the other half is declared below.
    accelerants = sum(1 for c in spells if c.is_ramp and c.mv <= 2)

    fitted = (_FIT_INTERCEPT
              + _FIT_PER_MANA_VALUE * average
              - _FIT_PER_CHEAP_ACCELERANT * accelerants)
    scaled = fitted * deck_size / _FIT_DECK_SIZE

    caveats = (
        ("fitted to 60-card tournament decks and scaled to this deck's size, "
         "not fitted to Commander"),
        ("cheap card draw is not counted -- a compiled card knows what it "
         "produces, not what it draws -- so this reads high for a deck that "
         "cantrips"),
        ("fast mana counts once here however much it makes, because the pool "
         "reports which colours a card produces and not how many"),
    )
    return LandEstimate(
        lands_now=lands_now,
        recommended=max(0, round(scaled)),
        average_mana_value=round(average, 2),
        cheap_accelerants=accelerants,
        deck_size=deck_size,
        caveats=caveats,
    )


# ------------------------------------------------------------- the shelf

@dataclass(frozen=True)
class Shelf:
    """Everything the closed form has to say about one deck.

    The name is Aaron's: a shelf of reference numbers you can take down beside
    the simulation rather than instead of it.
    """

    deck_size: int
    lands: int
    target: float
    on_the_play: bool
    colors: tuple[ColorRequirement, ...]
    land_estimate: LandEstimate
    odds: tuple[CardOdds, ...]
    #: Cards whose coloured demand spans two or more colours, for which
    #: `castable_odds` is an approximation rather than exact. Named rather
    #: than counted, because "which of my cards is this softest about" is a
    #: question with a useful answer.
    approximated: tuple[str, ...]

    @property
    def unmet(self) -> tuple[ColorRequirement, ...]:
        return tuple(c for c in self.colors if not c.met)


def shelf(library: list[SimCard], commander: SimCard | None = None, *,
          target: float = TARGET, on_the_play: bool = True) -> Shelf:
    """Read the whole closed form off one compiled deck.

    The commander is included in the card-by-card odds and in what sets each
    colour's requirement -- it is the one card you are guaranteed to be casting
    every game, so a mana base that cannot support it is the first thing to
    know -- but it is not part of the 99 the probabilities are drawn from,
    because it is not in the library.
    """
    deck_size = len(library)
    lands = sum(1 for c in library if c.is_land)
    considered = list(library) + ([commander] if commander is not None else [])

    # ---- colour requirements: a ladder per colour ------------------------
    # Every (colour, pip count) the deck actually asks for, and the cheapest
    # card asking it. The cheapest is the right one to judge on: it is the
    # earliest turn the deck puts the question, so a rung that passes there
    # passes everywhere later.
    asked: dict[tuple[str, int], list[SimCard]] = {}
    for card in considered:
        for colors, pips in _pip_demand(card.cost).items():
            # A hybrid pip is charged to every colour that can pay it: each of
            # those colours could be the one carrying the card, so each has to
            # be able to. It is the honest reading of "how many white sources
            # do I need" for a deck holding {G/W} cards.
            for color in colors:
                asked.setdefault((color, pips), []).append(card)

    requirements = []
    for color in sorted({c for c, _ in asked}):
        wanted = frozenset({color})
        have = sources_for(library, wanted)
        have_lands = sum(1 for c in library
                         if c.is_land and any(s.colors & wanted for s in c.produces))
        tiers = []
        for pips in sorted(p for c, p in asked if c == color):
            cards = sorted(asked[(color, pips)], key=lambda c: (c.mv, c.name))
            turn = max(1, cards[0].mv)
            tiers.append(PipTier(
                pips=pips,
                turn=turn,
                need=required_sources(deck_size=deck_size, pips=pips, turn=turn,
                                      target=target, on_the_play=on_the_play),
                have=have,
                odds_now=hypergeometric_at_least(
                    deck_size, have, cards_seen(turn, on_the_play=on_the_play), pips),
                cards=tuple(c.name for c in cards),
            ))
        requirements.append(ColorRequirement(
            color=color, have=have, have_lands=have_lands, tiers=tuple(tiers)))

    # ---- per card, per turn ----------------------------------------------
    odds = []
    approximated = []
    seen_names: set[str] = set()
    for card in considered:
        if card.is_land or card.name in seen_names:
            continue
        seen_names.add(card.name)
        odds.append(CardOdds(
            name=card.name,
            mv=card.mv,
            by_turn=tuple(
                castable_odds(card, library, turn=t, on_the_play=on_the_play)
                for t in range(1, HORIZON + 1)
            ),
        ))
        colors_demanded = {c for pip in _pip_demand(card.cost) for c in pip}
        if len({frozenset(p) for p in _pip_demand(card.cost)}) > 1 and len(colors_demanded) > 1:
            approximated.append(card.name)

    # Latest first, by how far a card runs behind its own cost. Sorting by raw
    # castability instead just ranks the deck by mana value -- it leads with
    # every twelve-drop and reports that expensive cards are expensive, which
    # is the one thing the reader already knew. Lag leads with the three-drop
    # that lands on turn six, which is the row worth acting on.
    #
    # `CardOdds.lateness` handles the never-arrives case and the past-the-
    # horizon case; see its docstring for why both are keyed the way they are.
    odds.sort(key=lambda o: (-o.lateness, o.mv, o.name))

    return Shelf(
        deck_size=deck_size,
        lands=lands,
        target=target,
        on_the_play=on_the_play,
        colors=tuple(requirements),
        land_estimate=regression_lands(library),
        odds=tuple(odds),
        approximated=tuple(sorted(approximated)),
    )
