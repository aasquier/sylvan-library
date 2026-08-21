"""The mana curve, and the rule that decides lands against ramp.

Most of these are properties rather than figures. The one thing this module
must never get wrong is the direction of its own advice, and a direction is
exactly what a spot-checked number cannot defend.
"""

from __future__ import annotations

from math import isclose

from mtglab.mana import ManaSource, parse_mana_cost
from mtglab.sim import curve as C
from mtglab.sim.tier1.engine import SimCard


def _land(name: str) -> SimCard:
    return SimCard(name=name, is_land=True,
                   produces=(ManaSource(frozenset("G")),))


def _spell(name: str, cost: str = "{1}{G}") -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost))


def _rock(name: str, cost: str, output: int = 1, delay: int = 0) -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost),
                   produces=(ManaSource(frozenset("C"), output),),
                   produce_delay=delay)


def _fetch(name: str, cost: str = "{2}{G}", lands: int = 1) -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost), fetches_lands=lands)


def _deck(lands: int = 36, rocks: int = 0) -> list[SimCard]:
    out = [_land(f"Forest {i}") for i in range(lands)]
    out += [_rock(f"Rock {i}", "{2}") for i in range(rocks)]
    out += [_spell(f"Bear {i}") for i in range(99 - lands - rocks)]
    return out


# ------------------------------------------------------- the distributions

def test_the_land_distribution_sums_to_one_and_respects_the_cap():
    """You may play one land a turn, so turn 4 tops out at four in play.

    The cap living inside the distribution is what stops a nine-land hand on
    turn four being counted as nine mana -- the flooding case the whole
    formula exists to price honestly.
    """
    dist = C._land_distribution(99, 36, 4)
    assert len(dist) == 5, "buckets are 0..turn"
    assert isclose(sum(dist), 1.0, abs_tol=1e-9)
    assert dist[4] > 0.3, "four-or-more is the fat bucket at 36 lands"


def test_the_ramp_distribution_sums_to_one():
    dist = C._ramp_distribution(_deck(rocks=8), 4)
    assert isclose(sum(dist), 1.0, abs_tol=1e-9)


def test_a_deck_with_no_accelerants_has_all_its_ramp_mass_at_zero():
    dist = C._ramp_distribution(_deck(rocks=0), 4)
    assert isclose(dist[0], 1.0, abs_tol=1e-9)


def test_a_rock_too_expensive_for_the_turn_is_not_counted():
    """A six-mana rock is not ramp for turn four."""
    late = [*_deck(lands=36), _rock("Big", "{6}")]
    assert isclose(sum(C._ramp_distribution(late, 4)[1:]), 0.0, abs_tol=1e-9)


def test_summoning_sickness_delays_a_mana_creature():
    """A dork cast on turn one pays on turn two, so its odds at a given turn
    are the odds of having drawn it a turn earlier."""
    dork = _rock("Elf", "{G}", delay=1)
    library = [*_deck(lands=36), dork]
    early = C._ramp_distribution(library, 1)
    assert isclose(sum(early[1:]), 0.0, abs_tol=1e-9), "sick on the turn it lands"
    assert sum(C._ramp_distribution(library, 2)[1:]) > 0


# ------------------------------------------------------------ land fetch

def test_a_land_fetch_spell_counts_as_ramp():
    """Cultivate has no `produces` at all and is still acceleration.

    Omitting it was a -0.54 mana bias across the whole formula, and the error
    only showed up as a pattern: Esper decks accurate, every green deck low.
    """
    plain = _deck(lands=36)
    fetched = [*_deck(lands=35), _fetch("Cultivate", "{2}{G}", lands=2)]
    assert C.expected_ramp(plain, 5) == 0.0
    assert C.expected_ramp(fetched, 5) > 0.0


def test_a_land_fetch_spell_is_not_capped_by_the_land_drop():
    """It puts lands onto the battlefield, so it adds on top of the cap.

    This is the property that makes it different from playing another land,
    and getting it wrong is what made the first draft of the formula
    systematically pessimistic for green decks.
    """
    library = [*_deck(lands=35), _fetch("Skyshroud Claim", "{3}{G}", lands=2)]
    # Turn 4 caps lands at 4, so anything above 4 must be coming from the
    # fetch rather than from a fifth land being played.
    assert C.on_curve_odds(library, 4, need=6) > 0.0


# ------------------------------------------------- lands against the curve

def test_a_land_is_worth_nothing_past_the_turn_it_is_played_on():
    """The rule the whole feature turns on.

    You may play one land a turn, so on turn four no number of lands gets you
    to five mana. This is why "T mana on turn T" could never recommend ramp,
    and why the surface asks how much mana rather than assuming.
    """
    lands_only = _deck(lands=60, rocks=0)
    assert C.on_curve_odds(lands_only, 4, need=4) > 0.5
    assert isclose(C.on_curve_odds(lands_only, 4, need=5), 0.0, abs_tol=1e-9), (
        "sixty lands and still no fifth mana on turn four")


def test_ramp_is_the_only_thing_that_beats_the_cap():
    with_rocks = _deck(lands=36, rocks=10)
    assert C.on_curve_odds(with_rocks, 4, need=5) > 0.0


def test_at_the_curve_a_land_is_at_least_as_good_as_an_accelerant():
    """Measured, and it is the finding that reshaped this module.

    Six decks by five target turns, and a land was ahead or level in all
    thirty. A land is one mana with no cast cost and no sickness; at exactly
    T mana on turn T nothing beats that.
    """
    library = _deck(lands=36, rocks=8)
    piece, _ = C._typical_accelerant(library, 4)
    per_land = C.on_curve_odds(library, 4, need=4, extra_lands=1)
    per_ramp = C.on_curve_odds(library, 4, need=4, extra_ramp=piece,
                               extra_ramp_count=1)
    assert per_land >= per_ramp - C.TOO_CLOSE


def test_past_the_curve_an_accelerant_beats_a_land_outright():
    """The other half, and the reason `recommend == "ramp"` is not dead code."""
    library = _deck(lands=36, rocks=8)
    piece, _ = C._typical_accelerant(library, 4)
    per_land = C.on_curve_odds(library, 4, need=6, extra_lands=1)
    per_ramp = C.on_curve_odds(library, 4, need=6, extra_ramp=piece,
                               extra_ramp_count=1)
    assert per_ramp > per_land


# ------------------------------------------------------------- the advice

def test_the_advice_recommends_ramp_past_the_curve():
    mc = C.curve(_deck(lands=36, rocks=8), target_turn=4, target_mana=6)
    assert mc.advice.recommend == "ramp"
    assert mc.advice.beyond_the_curve is True


def test_the_advice_stays_quiet_when_the_target_is_already_met():
    mc = C.curve(_deck(lands=40, rocks=10), target_turn=2, target_mana=2,
                 target=0.60)
    assert mc.advice.recommend == "none"
    assert mc.advice.slots == 0


def test_a_deck_with_no_ramp_says_its_comparison_is_a_stand_in():
    """Advice built on a hypothetical Signet has to admit that it is."""
    mc = C.curve(_deck(lands=36, rocks=0), target_turn=4, target_mana=6)
    assert mc.advice.ramp_is_generic is True
    piece, generic = C._typical_accelerant(_deck(rocks=0), 4)
    assert generic and piece == C._GENERIC_ROCK


def test_a_deck_with_ramp_averages_its_own_pieces():
    """"Add more ramp" means more of the ramp this deck actually plays."""
    library = [*_deck(lands=36, rocks=0)[:90],
               *[_rock(f"Signet {i}", "{2}") for i in range(9)]]
    piece, generic = C._typical_accelerant(library, 4)
    assert not generic
    assert piece[0] == 2 and piece[1] == 1


def test_slots_is_a_search_not_a_division():
    """Odds are not linear in slots, so the count must be found by trying.

    Dividing a shortfall by a marginal rate assumes the tenth land buys what
    the first did. It does not, and the error compounds exactly where the
    advice matters most.
    """
    mc = C.curve(_deck(lands=30, rocks=4), target_turn=4, target=0.85)
    slots = mc.advice.slots
    if slots is not None:
        reached = C.on_curve_odds(_deck(lands=30, rocks=4), 4,
                                  extra_lands=slots)
        one_fewer = C.on_curve_odds(_deck(lands=30, rocks=4), 4,
                                    extra_lands=slots - 1) if slots > 1 else 0.0
        assert reached >= 0.85
        assert one_fewer < 0.85, "slots must be the smallest count that works"


def test_an_unreachable_target_reports_none_rather_than_a_number():
    mc = C.curve(_deck(lands=36, rocks=2), target_turn=2, target_mana=12,
                 target=0.90)
    assert mc.advice.slots is None


def test_the_target_turn_is_clamped_to_the_horizon():
    assert C.curve(_deck(), target_turn=99).target_turn == C.HORIZON
    assert C.curve(_deck(), target_turn=0).target_turn == 1


def test_an_empty_library_does_not_raise():
    mc = C.curve([])
    assert mc.deck_size == 0
    assert mc.advice.odds == 0.0


# ----------------------------------------------------- the land drop truth

def test_a_land_drop_every_turn_is_unaffordable_and_says_so():
    """The answer to the question as originally asked.

    Fifty-four lands to make every drop through turn four at 90%. This is
    pinned because it is the number the feature exists to talk somebody *out*
    of, and a regression that made it look reasonable would be the feature
    quietly reversing its own advice.
    """
    assert C.lands_for_every_drop(99, 4, target=0.90) == 54
    assert C.lands_for_every_drop(99, 3, target=0.90) == 48
    # And it only gets worse, which is the shape that makes it hopeless.
    assert (C.lands_for_every_drop(99, 5, target=0.90)
            > C.lands_for_every_drop(99, 4, target=0.90))


def test_an_impossible_drop_requirement_returns_none():
    """`None` means no land count reaches it, not "a big number".

    Harder to provoke than it looks, and the first version of this test was
    wrong about it: 86 lands really does make every drop through turn ten at
    99.9%, because you see 6+T cards for T drops and an almost-all-lands deck
    gets there. The genuinely unreachable case is a deck with fewer cards than
    the turn asks for.
    """
    assert C.lands_for_every_drop(5, 10, target=0.90) is None


def test_land_drop_odds_fall_as_the_turns_go_on():
    """More turns means more drops to have made, and the requirement outruns
    the draws. A curve that rose would mean the cap was being ignored."""
    mc = C.curve(_deck(lands=36))
    odds = [t.land_drop_odds for t in mc.turns]
    assert odds == sorted(odds, reverse=True)
