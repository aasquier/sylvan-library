"""The mana curve: do you have T mana on turn T, and what fixes it if not.

Aaron asked for a land count that guarantees a land drop every turn. **The
answer is 54 lands, and that is the finding rather than the feature.** You
need T lands among the 6+T cards you have seen, and that ratio climbs toward
100%, so the requirement grows without bound: 48 lands to make every drop
through turn 3 at 90% confidence, 54 through turn 4, 59 through turn 5. At a
real 34-36 lands every deck in this library makes all its drops through turn 4
about half the time. "A land drop every turn" is only ever answerable as
"through turn *what*", and the honest answer to the question as asked is that
no deck you would build gets there.

So the question is reframed, and the reframing is the useful part: **do you
have T mana available on turn T?** A land drop is only one way to get it, and
for these decks it is not the main one -- ramp is worth between 28 and 58
percentage points by turn six, which is most of the answer rather than a
correction to it.

## The formula

    E[mana on turn T] = E[min(T, lands drawn)] + Σ (accelerants online at T)

Two details carry it, and both were found by being wrong first.

**The one-land-per-turn cap goes inside the expectation.** `E[min(T, lands)]`
is not `min(T, E[lands])`; the second is optimistic in exactly the region
where flooding begins, because it lets a hand of nine lands on turn four count
as nine.

**A land-fetch spell is not a mana rock.** Cultivate compiles with no
`produces` at all and a `fetches_lands` of two, and it does not use your land
drop -- so it adds *on top of* the cap rather than through it. Leaving it out
biased the whole formula by -0.54 mana, and the shape of the error is what
named the missing term: Esper Tivit was accurate to +0.1 while every green
deck sat about 0.9 low. **A systematic bias that varies by deck colour is a
missing term, not noise.**

Validated against 3,000 Tier 1 games per deck on 2026-08-21, six decks by six
turns: **mean error -0.06 mana, mean absolute error 0.25.** In closed form, in
microseconds, against a simulation that takes eighteen seconds.

## Lands or ramp, and the rule that decides it

Recommending either is Aaron's ruling of 2026-08-21. It is a different *kind*
of advice from anything else here -- every other recommendation in this
project is about which card, and this one is about which kind of card -- so it
was built and then measured, and the measurement changed the design.

**Asked for "T mana on turn T", ramp never wins.** Six decks by five target
turns, thirty comparisons, and a land was ahead or level in every one. That is
not a bug and it is not ramp being bad; it is the objective. A land gives one
mana per turn and you may play one a turn, so at exactly T mana on turn T a
land is the most reliable route there and nothing can beat it.

The parameter that makes the question real is **how much mana you want**, not
just when:

    up to the curve, lands.  past the curve, ramp.

A fifth land does **nothing at all** for turn four -- you cannot play it --
while an accelerant can push you past the cap. Measured at turn four: at four
mana lands win or tie in all six decks; at five, six and seven mana **ramp
wins in all six, at every level.** That is why `on_curve_odds` takes `need`
and why the surface asks for a mana target rather than assuming one.

Where the two are within `TOO_CLOSE` the advice says "either" rather than
resolving a tie with a coin.

Stdlib only, and no model. Every number here has a right answer.
"""

from __future__ import annotations

from dataclasses import dataclass

from mtglab.sim.karsten import _exactly, cards_seen, hypergeometric_at_least
from mtglab.sim.tier1.engine import SimCard

#: Turns reported. Matches `karsten.HORIZON` deliberately: two shelves on one
#: screen that stopped at different turns would invite a comparison neither
#: supports.
HORIZON = 10

#: The turn the advice is aimed at unless the caller says otherwise. Four,
#: because it is where most Commander decks are trying to deploy something
#: that matters and where the land-only answer has already fallen to a coin
#: flip. Exposed as a control -- Aaron's ruling -- because a cEDH deck cares
#: about turn two and a battlecruiser deck about turn six.
DEFAULT_TARGET_TURN = 4

#: The consistency bar the advice measures against unless the caller sets one.
#: The same figure `karsten.TARGET` uses, and on the Simulator the same control
#: drives both -- one "how often do you want this to work" dial rather than two
#: that could disagree.
DEFAULT_TARGET = 0.90

#: Odds. Below this difference the advice refuses to choose between a land and
#: an accelerant, because the closed form's own agreement with Tier 1 is
#: several points and a recommendation finer than the instrument is a guess
#: wearing a confident face.
TOO_CLOSE = 0.01

#: What a generic two-mana rock is worth, for a deck that runs no accelerants
#: at all and so has no profile to average. A Signet: costs two, makes one,
#: usable the turn after it lands.
_GENERIC_ROCK = (2, 1, 0)   # (cost, output, delay)


def _accelerants(library: list[SimCard]) -> list[tuple[int, int, int]]:
    """Every nonland mana source, as `(cost, output, delay)`.

    Both kinds are here and they are not the same thing. A permanent that
    produces mana carries its `produce_delay` -- a mana creature is summoning
    sick and pays for itself a turn late. A spell that *fetches lands* has no
    `produces` at all; it resolves into lands that are usable immediately, so
    its delay is zero, and crucially it does not consume the land drop.
    """
    out: list[tuple[int, int, int]] = []
    for card in library:
        if card.is_land:
            continue
        if card.produces:
            out.append((card.mv, sum(u.amount for u in card.produces),
                        card.produce_delay))
        elif card.fetches_lands:
            out.append((card.mv, card.fetches_lands, 0))
    return out


def expected_lands_in_play(deck_size: int, lands: int, turn: int, *,
                           on_the_play: bool = True) -> float:
    """E[min(turn, lands drawn)] -- lands actually on the battlefield.

    The cap is applied inside the expectation because you may play one land a
    turn. Outside it, `min(turn, mean)` counts a seven-land hand on turn three
    as seven, which is precisely the flooding case the number exists to price.
    """
    if deck_size <= 0 or lands <= 0 or turn <= 0:
        return 0.0
    seen = min(cards_seen(turn, on_the_play=on_the_play), deck_size)
    return sum(min(turn, k) * _exactly(deck_size, lands, seen, k)
               for k in range(min(seen, lands) + 1))


def expected_ramp(library: list[SimCard], turn: int, *,
                  on_the_play: bool = True) -> float:
    """Mana from accelerants that are online on `turn`.

    "Online" under Aaron's own framing -- everything played on the most
    reasonable turn -- means drawn early enough to have been cast and to have
    shed any summoning sickness: drawn by turn `turn - delay`, and cheap
    enough that `cost + delay` has arrived at all.

    Each accelerant is a singleton, so the chance of having drawn it by turn X
    is simply the fraction of the deck seen by then.
    """
    deck_size = len(library)
    if deck_size <= 0:
        return 0.0
    total = 0.0
    for cost, output, delay in _accelerants(library):
        if cost + delay > turn:
            continue
        seen = cards_seen(turn - delay, on_the_play=on_the_play)
        total += output * min(1.0, seen / deck_size)
    return total


def _land_distribution(deck_size: int, lands: int, turn: int, *,
                       on_the_play: bool = True) -> list[float]:
    """P(exactly t lands in play on `turn`), for t in 0..turn.

    Everything at or above the cap piles into the last bucket, because a
    seventh land on turn four is not a seventh mana -- it is a card you did
    not get to play.
    """
    dist = [0.0] * (turn + 1)
    if deck_size <= 0 or turn <= 0:
        return dist
    seen = min(cards_seen(turn, on_the_play=on_the_play), deck_size)
    for k in range(min(seen, lands) + 1):
        dist[min(turn, k)] += _exactly(deck_size, lands, seen, k)
    return dist


def _ramp_distribution(library: list[SimCard], turn: int, *,
                       on_the_play: bool = True,
                       extra: tuple[int, int, int] | None = None,
                       extra_count: int = 0) -> list[float]:
    """P(exactly m mana from accelerants on `turn`), for m in 0..total.

    A small dynamic program over the accelerants, each an independent
    Bernoulli: drawn by the turn it needs to be, or not. Independence between
    *accelerants* is a mild approximation -- they are drawn from one deck
    without replacement, so they are weakly negatively correlated -- and it is
    a much safer one than the independence this module refuses elsewhere.
    `karsten.castable_odds` has to condition on the draw because "four lands"
    and "one green source" are very nearly the same event; two different rocks
    are not.

    `extra` adds hypothetical copies, which is how the advice below asks
    "what would one more do" without rebuilding a deck to find out.
    """
    deck_size = len(library)
    pieces = [(c, m, d) for c, m, d in _accelerants(library) if c + d <= turn]
    if extra is not None and extra_count > 0 and extra[0] + extra[2] <= turn:
        pieces += [extra] * extra_count
    dist = [1.0]
    if deck_size <= 0:
        return dist
    for _cost, output, delay in pieces:
        seen = cards_seen(turn - delay, on_the_play=on_the_play)
        p = min(1.0, seen / deck_size)
        nxt = [0.0] * (len(dist) + output)
        for total, weight in enumerate(dist):
            if weight == 0.0:
                continue
            nxt[total] += weight * (1.0 - p)
            nxt[total + output] += weight * p
        dist = nxt
    return dist


def on_curve_odds(library: list[SimCard], turn: int, *,
                  need: int | None = None,
                  on_the_play: bool = True,
                  extra_lands: int = 0,
                  extra_ramp: tuple[int, int, int] | None = None,
                  extra_ramp_count: int = 0) -> float:
    """P(at least `need` mana available on `turn`; `need` defaults to `turn`).

    **`need` is the parameter that makes "lands or ramp" a real question**, and
    it was added because without it the answer was always "lands". See the
    module docstring: a land is capped at one mana per turn, so at `need ==
    turn` it is the most reliable way to get there and ramp cannot beat it;
    at `need > turn` a land is worth *nothing at all* and ramp is the only
    thing that can help.

    **This is the number the advice is keyed on, and the reason expectation
    was not enough.** Every deck in this library expects four or more mana on
    turn four, and every one of them still misses it a third of the time or
    worse -- an average is exactly the statistic that hides a coin flip.

    Lands and ramp are convolved as independent. They are drawn from one deck,
    so that is an approximation; it is a defensible one because they are
    different cards competing only for slots, and the alternative -- a joint
    distribution over the whole 99 -- is not something anybody could read off
    a screen even if it were cheap.
    """
    deck_size = len(library)
    if deck_size <= 0 or turn <= 0:
        return 0.0
    want = turn if need is None else need
    lands = sum(1 for c in library if c.is_land) + extra_lands
    land_dist = _land_distribution(deck_size, lands, turn,
                                   on_the_play=on_the_play)
    ramp_dist = _ramp_distribution(library, turn, on_the_play=on_the_play,
                                   extra=extra_ramp,
                                   extra_count=extra_ramp_count)
    total = 0.0
    for land_count, lw in enumerate(land_dist):
        if lw == 0.0:
            continue
        short = want - land_count
        if short <= 0:
            total += lw
        elif short < len(ramp_dist):
            total += lw * sum(ramp_dist[short:])
    return min(1.0, total)


def lands_for_every_drop(deck_size: int, turn: int, *, target: float = 0.90,
                         on_the_play: bool = True) -> int | None:
    """Lands needed to make **every** drop through `turn`, or None if never.

    The binding constraint is the last turn, not a product over all of them:
    lands seen only grows, so holding `turn` lands among the cards seen by
    `turn` means you held enough at every earlier turn too. That makes this
    one hypergeometric rather than a dependent chain, which is worth knowing
    because the chain is the thing people compute by mistake.

    Returns `None` when no land count in the deck reaches the target, which is
    a real answer and happens sooner than anybody expects.
    """
    seen = cards_seen(turn, on_the_play=on_the_play)
    for count in range(turn, deck_size + 1):
        if hypergeometric_at_least(deck_size, count, seen, turn) >= target:
            return count
    return None


def _typical_accelerant(library: list[SimCard],
                        turn: int) -> tuple[tuple[int, int, int], bool]:
    """One more accelerant *like the ones this deck already plays*.

    Averaged over the deck's own pieces rather than over an idealised one,
    because "add more ramp" means adding the kind of ramp this deck runs — a
    deck of three-mana sorceries does not acquire a Sol Ring by being told to
    accelerate. Only pieces that could be online by the target turn are
    averaged; a six-mana rock is not ramp for turn four.

    Returns `(piece, is_generic)`. A deck with no accelerants has no profile to
    average, so a plain Signet stands in and the flag says so — advice built on
    a stand-in has to admit that it is.
    """
    usable = [(c, m, d) for c, m, d in _accelerants(library) if c + d <= turn]
    if not usable:
        return _GENERIC_ROCK, True
    cost = max(0, round(sum(c for c, _, _ in usable) / len(usable)))
    output = max(1, round(sum(m for _, m, _ in usable) / len(usable)))
    delay = max(0, round(sum(d for _, _, d in usable) / len(usable)))
    return (cost, output, delay), False


@dataclass(frozen=True)
class CurveTurn:
    """One turn: what you can expect, where it came from, and the odds.

    `odds` and `expected_mana` answer different questions, and carrying both
    is the point rather than redundancy. **Every deck in this library expects
    four or more mana on turn four, and every one of them still misses it a
    quarter of the time or worse** — an average is exactly the statistic that
    hides a coin flip. So the advice is keyed on the odds, and the expectation
    is kept for the one thing it gives that the odds cannot: how much of the
    mana came from lands and how much from ramp.
    """

    turn: int
    from_lands: float
    from_ramp: float
    #: P(a land available every turn up to and including this one).
    land_drop_odds: float
    #: P(at least `turn` mana available on `turn`). The headline figure.
    odds: float

    @property
    def expected_mana(self) -> float:
        return self.from_lands + self.from_ramp


@dataclass(frozen=True)
class Advice:
    """What to add to hit `target_mana` on `target_turn`, and how sure it is.

    `recommend` is one of `lands`, `ramp`, `either` or `none`.
    """

    target_turn: int
    target_mana: int
    #: The consistency bar this is measured against.
    target: float
    #: P(target_mana available on target_turn), as things stand.
    odds: float
    #: Odds after one more land / one more accelerant, so the reader sees the
    #: comparison rather than being handed its conclusion.
    odds_per_land: float
    odds_per_ramp: float
    recommend: str
    #: Slots of the recommended kind needed to reach `target`, or None when
    #: twenty of them still would not.
    slots: int | None
    #: True when the deck runs no accelerants and `odds_per_ramp` stands in.
    ramp_is_generic: bool
    #: True when `target_mana` exceeds `target_turn` -- the region where a
    #: land is worth nothing and only ramp can help. The reason this feature
    #: has two controls rather than one.
    beyond_the_curve: bool
    #: Lands that would make every land drop through the target turn, or None.
    #: Almost always absurd, and carried for exactly that reason.
    lands_for_every_drop: int | None


@dataclass(frozen=True)
class ManaCurve:
    deck_size: int
    lands: int
    accelerants: int
    target_turn: int
    target_mana: int
    target: float
    on_the_play: bool
    turns: tuple[CurveTurn, ...]
    advice: Advice


def _slots_to_target(library: list[SimCard], turn: int, need: int,
                     target: float, *, on_the_play: bool, kind: str,
                     piece: tuple[int, int, int]) -> int | None:
    """How many added lands or accelerants reach `target`, or None.

    A search rather than a division, because odds are not linear in slots: the
    tenth land buys far less than the first, and dividing a shortfall by a
    marginal rate quietly assumes otherwise. Capped at twenty, past which the
    honest answer is that this deck does not get there by adding one kind of
    card.
    """
    for count in range(1, 21):
        odds = (on_curve_odds(library, turn, need=need, on_the_play=on_the_play,
                              extra_lands=count)
                if kind == "lands"
                else on_curve_odds(library, turn, need=need,
                                   on_the_play=on_the_play,
                                   extra_ramp=piece, extra_ramp_count=count))
        if odds >= target:
            return count
    return None


def curve(library: list[SimCard], *,
          target_turn: int = DEFAULT_TARGET_TURN,
          target_mana: int | None = None,
          target: float = DEFAULT_TARGET,
          on_the_play: bool = True) -> ManaCurve:
    """The whole mana curve for one compiled deck, and what to do about it.

    `target_mana` defaults to `target_turn` -- the on-curve question. Asking
    for more is what turns this into a question about ramp; see the module
    docstring for why that is not a preference but arithmetic.
    """
    deck_size = len(library)
    lands = sum(1 for c in library if c.is_land)
    target_turn = max(1, min(target_turn, HORIZON))
    want = target_turn if target_mana is None else max(1, min(target_mana, 20))
    target = max(0.5, min(target, 0.99))

    turns = tuple(
        CurveTurn(
            turn=t,
            from_lands=expected_lands_in_play(deck_size, lands, t,
                                              on_the_play=on_the_play),
            from_ramp=expected_ramp(library, t, on_the_play=on_the_play),
            land_drop_odds=hypergeometric_at_least(
                deck_size, lands, cards_seen(t, on_the_play=on_the_play), t),
            odds=on_curve_odds(library, t, on_the_play=on_the_play),
        )
        for t in range(1, HORIZON + 1)
    )

    odds = on_curve_odds(library, target_turn, need=want,
                         on_the_play=on_the_play)
    piece, generic = _typical_accelerant(library, target_turn)
    per_land = on_curve_odds(library, target_turn, need=want,
                             on_the_play=on_the_play, extra_lands=1)
    per_ramp = on_curve_odds(library, target_turn, need=want,
                             on_the_play=on_the_play, extra_ramp=piece,
                             extra_ramp_count=1)

    # Annotated because the two branches genuinely differ: a met target has
    # zero slots to add, and an unmet one may have no reachable count at all.
    recommend: str
    slots: int | None
    if odds >= target:
        recommend, slots = "none", 0
    else:
        if abs(per_land - per_ramp) < TOO_CLOSE:
            recommend = "either"
            kind = "lands" if per_land >= per_ramp else "ramp"
        elif per_ramp > per_land:
            recommend = kind = "ramp"
        else:
            recommend = kind = "lands"
        slots = _slots_to_target(library, target_turn, want, target,
                                 on_the_play=on_the_play, kind=kind,
                                 piece=piece)

    return ManaCurve(
        deck_size=deck_size,
        lands=lands,
        accelerants=len(_accelerants(library)),
        target_turn=target_turn,
        target_mana=want,
        target=target,
        on_the_play=on_the_play,
        turns=turns,
        advice=Advice(
            target_turn=target_turn,
            target_mana=want,
            target=target,
            odds=round(odds, 4),
            odds_per_land=round(per_land, 4),
            odds_per_ramp=round(per_ramp, 4),
            recommend=recommend,
            slots=slots,
            ramp_is_generic=generic,
            beyond_the_curve=want > target_turn,
            lands_for_every_drop=lands_for_every_drop(
                deck_size, target_turn, target=target,
                on_the_play=on_the_play),
        ),
    )
