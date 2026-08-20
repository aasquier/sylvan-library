"""The closed form, checked against arithmetic that does not come from it.

A module whose whole output is numbers is a module whose tests have to be
suspicious of those numbers. Nothing here asserts a figure this module
produced and somebody wrote down; every probability is checked either against
an independent brute-force count, against a value with a closed form of its
own, or against a property that must hold whatever the implementation does.
"""

from __future__ import annotations

from fractions import Fraction
from itertools import combinations
from math import comb, isclose

import pytest

from mtglab.mana import ManaSource, parse_mana_cost
from mtglab.sim import karsten
from mtglab.sim.tier1.engine import SimCard


def _brute_force_at_least(population: int, successes: int,
                          draws: int, wanted: int) -> Fraction:
    """The same probability by enumerating every hand, in exact rationals.

    Deliberately the stupidest correct implementation: label the population,
    look at every subset of size `draws`, count the ones that qualify. It is
    unusable past about twenty cards and it cannot be wrong, which is the
    trade a test wants.
    """
    universe = range(population)
    hits = sum(1 for hand in combinations(universe, draws)
               if sum(1 for c in hand if c < successes) >= wanted)
    return Fraction(hits, comb(population, draws))


@pytest.mark.parametrize("population,successes,draws,wanted", [
    (12, 5, 4, 1), (12, 5, 4, 2), (12, 5, 4, 3), (12, 5, 4, 5),
    (15, 6, 7, 2), (15, 6, 7, 0), (10, 10, 3, 3), (10, 0, 3, 1),
    (14, 3, 9, 3), (14, 3, 9, 4),
])
def test_hypergeometric_matches_exhaustive_enumeration(
        population, successes, draws, wanted):
    """The one piece of mathematics everything else rests on."""
    expected = _brute_force_at_least(population, successes, draws, wanted)
    got = karsten.hypergeometric_at_least(population, successes, draws, wanted)
    assert isclose(got, float(expected), abs_tol=1e-12)


def test_both_summation_branches_are_exercised_and_agree():
    """`hypergeometric_at_least` sums the complement when it is shorter.

    Two code paths for one number is two chances to be wrong, so the test
    drives a case on each side of the branch and checks both against the
    enumeration. Without this, a bug in the rarely-taken branch would show up
    only as a slightly wrong percentage on somebody's screen.
    """
    # wanted=1 of 5 in 4 draws: complement is one term, direct sum is four.
    low = karsten.hypergeometric_at_least(12, 5, 4, 1)
    # wanted=4: direct sum is two terms, complement is four.
    high = karsten.hypergeometric_at_least(12, 5, 4, 4)
    assert isclose(low, float(_brute_force_at_least(12, 5, 4, 1)), abs_tol=1e-12)
    assert isclose(high, float(_brute_force_at_least(12, 5, 4, 4)), abs_tol=1e-12)
    assert low > high


def test_the_probability_mass_function_sums_to_one():
    total = sum(karsten._exactly(99, 36, 9, k) for k in range(10))
    assert isclose(total, 1.0, abs_tol=1e-12)


def test_at_least_is_monotone_in_every_argument_that_should_move_it():
    """Properties that hold whatever the arithmetic is.

    More sources cannot hurt, more draws cannot hurt, and asking for more pips
    cannot help. A sign error survives a spot-check of one number and does not
    survive this.
    """
    base = karsten.hypergeometric_at_least(99, 30, 10, 2)
    assert karsten.hypergeometric_at_least(99, 31, 10, 2) >= base
    assert karsten.hypergeometric_at_least(99, 30, 11, 2) >= base
    assert karsten.hypergeometric_at_least(99, 30, 10, 3) <= base
    assert karsten.hypergeometric_at_least(99, 30, 10, 0) == 1.0
    assert karsten.hypergeometric_at_least(99, 1, 10, 2) == 0.0


# --------------------------------------------------------- cards seen

def test_cards_seen_counts_the_skipped_draw_step():
    """On the play you skip the first draw; on the draw you do not."""
    assert karsten.cards_seen(1, on_the_play=True) == 7
    assert karsten.cards_seen(1, on_the_play=False) == 8
    assert karsten.cards_seen(4, on_the_play=True) == 10
    assert karsten.cards_seen(4, on_the_play=False) == 11


# ------------------------------------------------- the requirement itself

def test_required_sources_is_the_smallest_count_that_clears_the_bar():
    """The definition, checked as a definition rather than as a number.

    Whatever `required_sources` returns must clear the target, and one fewer
    must not. That is the entire contract, and it cannot be satisfied by a
    lookup table that has drifted.
    """
    for pips in (1, 2, 3):
        for turn in (max(1, pips), pips + 2):
            need = karsten.required_sources(deck_size=99, pips=pips, turn=turn)
            seen = karsten.cards_seen(turn)
            assert karsten.hypergeometric_at_least(99, need, seen, pips) >= karsten.TARGET
            assert karsten.hypergeometric_at_least(99, need - 1, seen, pips) < karsten.TARGET


def test_more_pips_and_earlier_turns_both_cost_more_sources():
    single = karsten.required_sources(deck_size=99, pips=1, turn=3)
    double = karsten.required_sources(deck_size=99, pips=2, turn=3)
    assert double > single
    assert (karsten.required_sources(deck_size=99, pips=2, turn=2)
            > karsten.required_sources(deck_size=99, pips=2, turn=5))


def test_a_single_pip_wants_about_a_fifth_of_a_commander_deck():
    """The landmark, as a band rather than as a figure.

    Karsten's result everybody quotes is that one coloured pip wants roughly a
    fifth of the deck making that colour. Asserted as a range because this
    module does not model mulligans and his table does, so the numbers are
    close rather than equal -- see the module docstring. A range that this
    falls outside means the arithmetic has moved, which is what the test is
    for; a range it can never leave would test nothing.
    """
    need = karsten.required_sources(deck_size=99, pips=1, turn=4)
    assert 18 <= need <= 22, need
    assert 0.18 <= need / 99 <= 0.23


def test_this_module_is_stricter_than_the_published_table_and_by_how_much():
    """Pins the documented gap, in the documented direction.

    The module docstring claims 86.1% where Karsten's table reads 90%, at 14
    sources in 60 cards for a turn-one single pip, and explains the difference
    as the London mulligan this does not model. That is a load-bearing claim --
    it is why our requirements are allowed to disagree with a published table
    without either being wrong -- so it is pinned. If this fails, the docstring
    is now wrong and must be edited rather than the assertion loosened.
    """
    odds = karsten.hypergeometric_at_least(60, 14, karsten.cards_seen(1), 1)
    assert isclose(odds, 0.861, abs_tol=0.001), odds
    assert odds < 0.90, "we must be stricter than the table, never looser"


# ------------------------------------------------------- reading a deck

def _land(name: str, *colors: str) -> SimCard:
    return SimCard(name=name, is_land=True,
                   produces=(ManaSource(frozenset(colors)),))


def _spell(name: str, cost: str) -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost))


def _mono_green(lands: int = 34, spells: int = 65) -> list[SimCard]:
    return ([_land(f"Forest {i}", "G") for i in range(lands)]
            + [_spell(f"Bear {i}", "{1}{G}") for i in range(spells)])


def test_hybrid_pips_are_charged_to_both_colours():
    """A {G/W} card needs green sources *or* white ones, so both are asked.

    The alternative -- charging it to neither, or picking one -- is how a deck
    full of hybrid cards reports a mana base it does not have.
    """
    library = [*_mono_green(lands=30, spells=68), _spell("Hybrid", "{G/W}")]
    shelf = karsten.shelf(library)
    colors = {c.color for c in shelf.colors}
    assert "G" in colors and "W" in colors


def test_phyrexian_pips_place_no_demand_on_the_mana_base():
    """Two life always pays, so a Phyrexian symbol asks the mana base nothing.

    Checked here as well as in `test_mana.py` because this module reads
    `ManaCost` directly, and the property it depends on is one somebody could
    reasonably "fix" in the parser.
    """
    library = _mono_green()
    plain = karsten.shelf(library)
    with_phyrexian = karsten.shelf([*library, _spell("Compleated", "{2}{U/P}")])
    assert [c.color for c in plain.colors] == [c.color for c in with_phyrexian.colors]


def test_the_pip_ladder_reports_each_demand_separately():
    """A deck short on one rung is not a deck short on all of them.

    This is the commandment 2 shape of the feature: mono-green Goreclaw meets
    every single-pip demand it has and fails only its triple-pip card, and a
    shelf that collapsed that into "green: short" would tell a beginner their
    mana base was broken when one card is greedy.
    """
    library = [*_mono_green(lands=34, spells=62),
               _spell("Cheap", "{G}"), _spell("Middling", "{1}{G}{G}"),
               _spell("Greedy", "{G}{G}{G}")]
    shelf = karsten.shelf(library)
    green = next(c for c in shelf.colors if c.color == "G")
    tiers = {t.pips: t for t in green.tiers}
    # A rung exists only where the deck actually makes that demand: the ladder
    # is read off the 99, not laid out 1-2-3 and filled in.
    assert set(tiers) == {1, 2, 3}
    assert tiers[1].met, "34 green sources must satisfy a single pip"
    assert not tiers[3].met, "a triple pip on turn 3 wants far more than 34"
    assert tiers[1].need < tiers[2].need < tiers[3].need
    assert "Greedy" in tiers[3].cards


def test_a_tier_is_judged_on_the_cheapest_card_that_asks():
    """The earliest turn the deck puts the question is the turn that matters."""
    library = [*_mono_green(lands=34, spells=63),
               _spell("Early", "{G}{G}"), _spell("Late", "{5}{G}{G}")]
    green = next(c for c in karsten.shelf(library).colors if c.color == "G")
    two = next(t for t in green.tiers if t.pips == 2)
    assert two.turn == 2, "judged on the two-drop, not the seven-drop"
    assert two.cards[0] == "Early"


# ------------------------------------------------------- castability

def test_a_card_is_never_castable_before_its_own_mana_value():
    library = _mono_green()
    card = _spell("Four Drop", "{3}{G}")
    for turn in (1, 2, 3):
        assert karsten.castable_odds(card, library, turn=turn) == 0.0
    assert karsten.castable_odds(card, library, turn=4) > 0.0


def test_castability_rises_with_the_turn_and_with_the_mana_base():
    library = _mono_green(lands=34, spells=65)
    richer = _mono_green(lands=44, spells=55)
    card = _spell("Four Drop", "{3}{G}")
    by_turn = [karsten.castable_odds(card, library, turn=t) for t in range(4, 9)]
    assert by_turn == sorted(by_turn), "more turns cannot make a card harder"
    assert (karsten.castable_odds(card, richer, turn=5)
            > karsten.castable_odds(card, library, turn=5))


def test_conditioning_on_the_draw_beats_multiplying_two_probabilities():
    """The reason `castable_odds` is not two hypergeometrics multiplied.

    In a mono-green deck "I have four lands" and "I have one green source" are
    very nearly the same event. Multiplying them unconditionally prices the
    most consistent mana base in Magic as though the two were independent. The
    conditional form must come out close to the pure land probability, because
    for this deck the colour is nearly free once the lands are there.
    """
    library = _mono_green(lands=40, spells=59)
    card = _spell("Four Drop", "{3}{G}")
    conditional = karsten.castable_odds(card, library, turn=4)
    lands_only = karsten.hypergeometric_at_least(99, 40, karsten.cards_seen(4), 4)
    naive = lands_only * karsten.hypergeometric_at_least(
        99, 40, karsten.cards_seen(4), 1)
    assert isclose(conditional, lands_only, abs_tol=0.001), (
        "every source is green, so the colour must cost nothing")
    assert conditional > naive


def test_a_colourless_cost_is_a_land_count_question_only():
    library = _mono_green()
    card = SimCard(name="Rock", cost=parse_mana_cost("{2}"))
    assert isclose(
        karsten.castable_odds(card, library, turn=2),
        karsten.hypergeometric_at_least(99, 34, karsten.cards_seen(2), 2),
        abs_tol=1e-9)


# --------------------------------------------------------------- lateness

def test_lateness_ranks_a_cheap_card_that_never_arrives_above_an_expensive_one():
    """The ranking's whole job, and the bug it was written against.

    Sorting by raw castability leads with every twelve-drop and reports that
    expensive cards are expensive. Lateness normalises by what the card costs,
    so a three-drop that never becomes reliable outranks an eight-drop that
    never becomes reliable -- which is the row somebody can actually act on.
    """
    cheap = karsten.CardOdds("Cheap", 3, (0.0,) * karsten.HORIZON)
    dear = karsten.CardOdds("Dear", 8, (0.0,) * karsten.HORIZON)
    assert cheap.lateness > dear.lateness
    assert cheap.lag is None and dear.lag is None


def test_a_card_past_the_horizon_reports_not_asked_rather_than_never():
    """None, not zero. Zero would be a claim the shelf did not make."""
    huge = karsten.CardOdds("Ghalta", 12, (0.0,) * karsten.HORIZON)
    assert huge.on_curve is None
    assert huge.lateness < 0, "must sort last, not first"


def test_lag_is_the_gap_between_cost_and_reliability():
    odds = karsten.CardOdds("Three Drop", 3,
                            (0.0, 0.0, 0.5, 0.7, 0.95, 1.0, 1.0, 1.0, 1.0, 1.0))
    assert odds.reliable_turn == 5
    assert odds.lag == 2
    assert odds.lateness == 2
    assert odds.on_curve == 0.5


# ------------------------------------------------------------ land estimate

def test_the_regression_scales_with_deck_size_rather_than_ignoring_it():
    """A 60-card fit applied to 99 cards without scaling recommends a 60-card
    mana base for a Commander deck, which is the whole trap."""
    library = _mono_green()
    estimate = karsten.regression_lands(library)
    assert estimate.deck_size == 99
    # The fit's intercept alone is 19.59 at 60 cards; scaled it must clear 30.
    assert estimate.recommended > 30


def test_the_regression_names_what_it_could_not_see():
    """Three terms of the published fit cannot be read off a compiled card.

    Each one would *lower* the recommendation, so the number errs high, and a
    caveat list that quietly emptied would turn a known overestimate into an
    unmarked one.
    """
    estimate = karsten.regression_lands(_mono_green())
    assert estimate.caveats
    joined = " ".join(estimate.caveats).lower()
    assert "draw" in joined and "60-card" in joined


def test_a_steeper_curve_asks_for_more_lands():
    cheap = ([_land(f"F{i}", "G") for i in range(34)]
             + [_spell(f"One {i}", "{G}") for i in range(65)])
    dear = ([_land(f"F{i}", "G") for i in range(34)]
            + [_spell(f"Six {i}", "{5}{G}") for i in range(65)])
    assert (karsten.regression_lands(dear).recommended
            > karsten.regression_lands(cheap).recommended)


def test_a_deck_with_no_spells_does_not_divide_by_zero():
    lands = [_land(f"F{i}", "G") for i in range(99)]
    estimate = karsten.regression_lands(lands)
    assert estimate.recommended == estimate.lands_now == 99


# ------------------------------------------------------------------ shelf

def test_the_commander_sets_requirements_but_is_not_drawn_from_the_library():
    """It is the one card you always have, and it is not in the 99.

    Both halves matter: a mana base that cannot cast the commander is the first
    thing to know, and counting it as a hundredth card would make every
    probability on the shelf slightly wrong.
    """
    library = _mono_green(lands=34, spells=65)
    commander = _spell("Legend", "{2}{W}{W}")
    shelf = karsten.shelf(library, commander)
    assert shelf.deck_size == 99, "the commander is not part of the library"
    assert "W" in {c.color for c in shelf.colors}, "but it still sets a demand"
    assert any(o.name == "Legend" for o in shelf.odds)


def test_an_empty_library_does_not_raise():
    shelf = karsten.shelf([])
    assert shelf.deck_size == 0
    assert shelf.colors == ()


def test_multi_colour_cards_are_named_as_approximated():
    """The one place the method stops being exact, reported rather than hidden."""
    library = [*_mono_green(lands=30, spells=68), _spell("Gold", "{G}{W}")]
    shelf = karsten.shelf(library)
    assert "Gold" in shelf.approximated
    assert "Bear 0" not in shelf.approximated
