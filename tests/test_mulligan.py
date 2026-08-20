"""The mulligan policy search, and the honesty it is required to keep.

Most of these are about the *verdict* rather than the arithmetic. The search
itself is a loop over `engine.run`, which `tests/test_tier1.py` already
defends; what is new here is a module that makes a recommendation, and every
way a recommendation can be dishonest is a case below.
"""

from __future__ import annotations

import pytest

from mtglab.mana import ManaSource, parse_mana_cost
from mtglab.sim import mulligan
from mtglab.sim.tier1.engine import KeepRule, SimCard


def _deck(lands: int = 36) -> list[SimCard]:
    """A plain mono-green 99 with a curve worth deploying."""
    forests = [SimCard(name=f"Forest {i}", is_land=True,
                       produces=(ManaSource(frozenset("G")),))
               for i in range(lands)]
    spells = [SimCard(name=f"Spell {i}",
                      cost=parse_mana_cost("{1}{G}" if i % 2 else "{2}{G}"))
              for i in range(99 - lands)]
    return forests + spells


def _commander() -> SimCard:
    return SimCard(name="Goreclaw", cost=parse_mana_cost("{2}{G}{G}"))


@pytest.fixture(scope="module")
def sweep() -> mulligan.PolicySweep:
    """One full-grid search, shared by every test that reads a verdict.

    Six tests interrogate the same sweep from different angles, and running
    the whole 33-rule grid once each took this file from four seconds to
    sixty. The sweep is seeded and its inputs are constants, so one run is as
    good as six -- and a frozen dataclass cannot be mutated by a test that
    reads it, which is what makes sharing it safe rather than merely fast.
    """
    return mulligan.search(_deck(), _commander(), games=250, seed=7)


# ------------------------------------------------------------- the grid

def test_the_grid_never_offers_a_rule_that_cannot_keep_anything():
    """`min_lands > max_lands` is not a policy, it is an error.

    Running one would report a rule that mulligans to its floor every game as
    though it were a candidate that lost fairly.
    """
    for rule in mulligan.candidates():
        assert rule.min_lands <= rule.max_lands
        assert rule.min_mana_pieces >= rule.min_lands


def test_the_grid_only_offers_rules_the_app_can_actually_set():
    """A recommendation with no control behind it is a curiosity.

    The three swept fields are the three the simulator's own mulligan controls
    expose, so every answer this can give is an answer somebody can then dial
    in by hand and re-run. If a fourth field is ever swept, a fourth control
    has to arrive with it.
    """
    settable = {"min_lands", "max_lands", "min_mana_pieces"}
    default = KeepRule()
    for rule in mulligan.candidates():
        for field, value in vars(rule).items():
            if field not in settable:
                assert value == getattr(default, field), (
                    f"{field} varies in the grid but has no control")


def test_every_rule_runs_on_the_same_seed():
    """The comparison is between policies, not between shuffles.

    Two rules judged on different samples are ranked partly by luck, and at
    these sample sizes luck is worth more than the effect being measured. The
    check drives the search twice with one rule and asserts the run is
    reproducible, which is the property the seed is there to give.
    """
    deck, commander = _deck(), _commander()
    rule = KeepRule(min_lands=2, max_lands=5, min_mana_pieces=3)
    first = mulligan.search(deck, commander, games=300, rules=[rule], seed=11)
    second = mulligan.search(deck, commander, games=300, rules=[rule], seed=11)
    assert first.rows[0].spells_through_t8 == second.rows[0].spells_through_t8
    assert first.rows[0].mulligan_rate == second.rows[0].mulligan_rate


# ---------------------------------------------------------- the verdict

def test_flatness_is_measured_against_the_default_not_against_the_grid(sweep):
    """The bug this property was written against.

    The grid deliberately contains rules nobody would play, so its spread is
    always wide; a `flat` keyed on the spread would never fire, and a deck
    whose best rule beats its default by 0.01 spells would be handed a
    recommendation to go and change a setting for nothing.
    """
    # Whatever this particular deck does, the two must not be the same test.
    assert sweep.flat == (sweep.gain < mulligan.FLAT)
    assert sweep.spread >= sweep.gain, (
        "the grid's range must be at least the improvement over one member")


def test_the_baseline_is_the_rule_the_simulator_actually_defaults_to(sweep):
    """The answer is read as a change from where the deck stands.

    If `KeepRule()`'s own defaults ever move outside the grid, the baseline
    must still be that rule -- measured by running it -- rather than the
    nearest grid member, or the reported gain is against something the user
    has never been using.
    """
    default = KeepRule()
    assert sweep.baseline.min_lands == default.min_lands
    assert sweep.baseline.max_lands == default.max_lands
    assert sweep.baseline.min_pieces == default.min_mana_pieces


def test_the_baseline_is_run_even_when_the_grid_excludes_it():
    """A grid that does not contain the default still has to price it."""
    deck, commander = _deck(), _commander()
    odd = KeepRule(min_lands=3, max_lands=6, min_mana_pieces=4)
    sweep = mulligan.search(deck, commander, games=300, rules=[odd], seed=7)
    assert sweep.rows == (sweep.rows[0],), "only the one rule was swept"
    assert sweep.baseline.min_lands == KeepRule().min_lands
    assert sweep.baseline is not sweep.rows[0]


def test_the_best_rule_is_the_one_that_deploys_most(sweep):
    """Deployment is the objective, and nothing else may outrank it."""
    assert sweep.best.spells_through_t8 == max(
        r.spells_through_t8 for r in sweep.rows)


def test_ties_break_toward_keeping_more_hands(sweep):
    """Two policies that deploy identically are not equally good to play.

    The one that keeps more hands spends less of the game a card down, so a
    tie on the objective is broken on the mulligan rate rather than on
    whichever row the loop reached first.
    """
    tied = [r for r in sweep.rows
            if r.spells_through_t8 == sweep.best.spells_through_t8]
    assert sweep.best.mulligan_rate == min(r.mulligan_rate for r in tied)


def test_the_gentlest_rule_never_costs_deployment_beyond_the_flat_band(sweep):
    """The answer for a flat sweep, and it must not become a second objective.

    Only rules within `FLAT` of the winner are eligible -- exactly the band the
    objective cannot tell apart anyway -- so this can never recommend a
    materially worse rule on the grounds that it mulligans less.
    """
    gap = sweep.best.spells_through_t8 - sweep.gentlest.spells_through_t8
    assert gap < mulligan.FLAT
    assert sweep.gentlest.mulligan_rate <= sweep.best.mulligan_rate


def test_rows_come_back_ranked_best_first(sweep):
    rows = sweep.rows
    assert [r.spells_through_t8 for r in rows] == sorted(
        (r.spells_through_t8 for r in rows), reverse=True)


def test_an_absurd_rule_really_does_lose():
    """A sanity anchor: demanding five mana pieces in an opening seven
    mulligans nearly every hand, and must deploy worse than the default."""
    deck, commander = _deck(), _commander()
    harsh = KeepRule(min_lands=3, max_lands=4, min_mana_pieces=5)
    sweep = mulligan.search(deck, commander, games=600, rules=[harsh], seed=7)
    assert sweep.rows[0].mulligan_rate > 0.5
    assert sweep.rows[0].spells_through_t8 < sweep.baseline.spells_through_t8


def test_an_empty_grid_is_refused_rather_than_answered():
    deck, commander = _deck(), _commander()
    try:
        mulligan.search(deck, commander, games=100, rules=[])
    except ValueError:
        return
    raise AssertionError("an empty grid must raise, not report a winner")


def test_progress_is_reported_and_finishes_at_the_total():
    """A UI bar that never reaches its end is a job that looks hung."""
    seen: list[tuple[int, int]] = []
    deck, commander = _deck(), _commander()
    rules = [KeepRule(min_lands=2, max_lands=5, min_mana_pieces=3),
             KeepRule(min_lands=1, max_lands=5, min_mana_pieces=2)]
    mulligan.search(deck, commander, games=200, rules=rules, seed=7,
                    progress=lambda done, total: seen.append((done, total)))
    assert seen[-1] == (len(rules), len(rules))
    assert [d for d, _ in seen] == sorted(d for d, _ in seen)
