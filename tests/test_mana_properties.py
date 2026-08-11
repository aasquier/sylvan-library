"""Property-based tests for `mana.py`, against the oracles in `mana_oracle.py`.

`tests/test_mana.py` pins the specific cases where naive source-counting gives
the wrong answer. Those are examples, and examples only cover what someone
thought of. This file covers what nobody thought of: Hypothesis generates costs
and pools, and every one of them is checked against a brute-force search and
against Hall's condition. A disagreement is a real bug in the most
correctness-critical function in the project.

The enumerated corpus at the bottom does the same job deterministically, so the
suite has a fixed backbone that does not depend on which examples Hypothesis
happened to roll -- and so a compiled port has something concrete to be
differentially tested against (docs/ENGINEERING.md 1).

`hypothesis` is imported unconditionally on purpose. tests/test_api.py opens
with `pytest.importorskip`, and CI has an explicit guard against it because
that once silently disabled the whole HTTP layer behind a green check. Same
lesson: a missing dependency here should be a loud import error.
"""

import sys
from dataclasses import replace
from pathlib import Path

from hypothesis import assume, given
from hypothesis import strategies as st

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mana_oracle import (
    brute_force_can_pay,
    case_id,
    corpus_cases,
    corpus_digest,
    hall_can_pay,
)
from mtglab.mana import ManaCost, ManaSource, can_pay, expand_units, parse_mana_cost
from mtglab.sim.tier1.engine import _consume

# ------------------------------------------------------------- strategies
#
# Sizes are small deliberately. The brute-force oracle is factorial in the pip
# count, and the interesting failures -- a dual that two pips both want -- show
# up at three pips and four sources, not at ten. Bigger cases would cost
# seconds per example and find nothing new.

PALETTE = ("W", "U", "G", "C")

colour_sets = st.frozensets(st.sampled_from(PALETTE), min_size=1, max_size=3)

mana_sources = st.builds(ManaSource, colors=colour_sets, amount=st.integers(1, 2))

pools = st.lists(mana_sources, max_size=4)

costs = st.builds(
    ManaCost,
    generic=st.integers(0, 3),
    pips=st.lists(colour_sets, max_size=4).map(tuple),
    has_x=st.booleans(),
    phyrexian=st.lists(colour_sets, max_size=2).map(tuple),
)

costs_with_pips = costs.filter(lambda c: bool(c.pips))

x_values = st.integers(0, 3)


# ------------------------------------------------------- the two oracles

@given(costs, pools, x_values)
def test_solver_agrees_with_brute_force_search(cost, pool, x_value):
    assert can_pay(cost, pool, x_value=x_value) == brute_force_can_pay(
        cost, pool, x_value=x_value
    ), case_id(cost, pool)


@given(costs, pools, x_values)
def test_solver_agrees_with_halls_condition(cost, pool, x_value):
    assert can_pay(cost, pool, x_value=x_value) == hall_can_pay(
        cost, pool, x_value=x_value
    ), case_id(cost, pool)


# ------------------------------------------------------------ monotonicity
#
# Properties that must hold for any correct solver, stated without reference to
# how it works. These are the ones that would survive a rewrite in another
# language unchanged.

@given(costs, pools, mana_sources)
def test_an_extra_source_never_makes_a_payable_cost_unpayable(cost, pool, extra):
    assume(can_pay(cost, pool))
    assert can_pay(cost, [*pool, extra])


@given(costs_with_pips, pools, st.integers(0, 3))
def test_dropping_a_pip_never_makes_a_payable_cost_unpayable(cost, pool, index):
    assume(can_pay(cost, pool))
    index %= len(cost.pips)
    smaller = replace(cost, pips=cost.pips[:index] + cost.pips[index + 1:])
    assert can_pay(smaller, pool)


@given(costs_with_pips, pools, st.integers(0, 3), st.sampled_from(PALETTE))
def test_widening_a_pip_never_makes_a_payable_cost_unpayable(cost, pool, index, colour):
    """A hybrid is strictly easier to pay than either of its halves."""
    assume(can_pay(cost, pool))
    index %= len(cost.pips)
    widened = list(cost.pips)
    widened[index] |= {colour}
    assert can_pay(replace(cost, pips=tuple(widened)), pool)


@given(costs, pools, st.integers(1, 4))
def test_more_generic_is_never_easier_to_pay(cost, pool, extra_generic):
    """The other direction: a strictly more expensive cost is never easier."""
    dearer = replace(cost, generic=cost.generic + extra_generic)
    if can_pay(dearer, pool):
        assert can_pay(cost, pool)


# --------------------------------------------------------------- invariants

@given(costs, pools, x_values)
def test_payable_implies_the_pool_holds_enough_mana(cost, pool, x_value):
    """Nothing taps twice, so a payable cost can never exceed the pool size.

    Note this is not `cost.mana_value`: Phyrexian symbols raise the mana value
    without demanding a source, because 2 life pays them.
    """
    if can_pay(cost, pool, x_value=x_value):
        needed = cost.generic + len(cost.pips) + (x_value if cost.has_x else 0)
        assert len(expand_units(pool)) >= needed


@given(costs, pools, st.data())
def test_source_order_does_not_change_the_answer(cost, pool, data):
    """Guards the class of bug that makes a seeded simulation irreproducible:
    an answer that depends on the order sources happen to arrive in."""
    shuffled = data.draw(st.permutations(pool))
    assert can_pay(cost, pool) == can_pay(cost, shuffled)


@given(costs, pools, st.data())
def test_pip_order_does_not_change_the_answer(cost, pool, data):
    reordered = replace(cost, pips=tuple(data.draw(st.permutations(cost.pips))))
    assert can_pay(cost, pool) == can_pay(reordered, pool)


@given(costs, colour_sets, st.integers(1, 3))
def test_one_source_of_n_equals_n_sources_of_one(cost, colours, amount):
    """Sol Ring is two colorless mana, not one mana that is worth two."""
    assert can_pay(cost, [ManaSource(colours, amount)]) == can_pay(
        cost, [ManaSource(colours, 1)] * amount
    )


@given(costs, x_values)
def test_a_pool_built_from_the_cost_always_pays_it(cost, x_value):
    """The trivially sufficient pool: one exactly-matching source per pip, plus
    an any-color source for each point of generic and X."""
    anything = frozenset({"W", "U", "B", "R", "G", "C"})
    generic_count = cost.generic + (x_value if cost.has_x else 0)
    pool = [ManaSource(pip) for pip in cost.pips]
    pool += [ManaSource(anything)] * generic_count
    assert can_pay(cost, pool, x_value=x_value)


# ------------------------------------------------- the engine's own solver
#
# `engine._consume` re-solves the same problem, because it has to return the
# leftovers rather than a yes/no. Two implementations of one predicate is
# exactly the shape that drifts apart silently, so pin them together.

@given(costs, pools)
def test_consume_agrees_with_can_pay(cost, pool):
    assert (_consume(cost, list(pool)) is not None) == can_pay(cost, pool)


@given(costs, pools)
def test_consume_spends_exactly_what_the_cost_demands(cost, pool):
    remaining = _consume(cost, list(pool))
    assume(remaining is not None)
    spent = cost.generic + len(cost.pips)   # again, Phyrexian costs life
    assert len(expand_units(remaining)) == len(expand_units(pool)) - spent


@given(costs, pools)
def test_consume_leaves_a_subset_of_what_it_was_given(cost, pool):
    """The leftovers must be units that were actually there -- never a source
    invented by the payment step."""
    remaining = _consume(cost, list(pool))
    assume(remaining is not None)
    before = sorted("".join(sorted(u)) for u in expand_units(pool))
    after = sorted("".join(sorted(u)) for u in expand_units(remaining))
    for unit in after:
        assert unit in before
        before.remove(unit)


# ------------------------------------------------------------- the parser
#
# Mana costs arrive from Scryfall, so they are well-formed in practice. They are
# still parsed input, and docs/ENGINEERING.md 5 asks that parsed input never
# crash and never silently mis-parse.

@given(st.text())
def test_the_parser_never_raises_on_arbitrary_text(text):
    parse_mana_cost(text)


@given(st.lists(st.sampled_from(["{", "}", "/", "P", "X", "2", "W", "S", " "]),
                max_size=12).map("".join))
def test_the_parser_never_raises_on_manglings_of_real_symbols(text):
    cost = parse_mana_cost(text)
    assert cost.generic >= 0


@given(costs)
def test_a_cost_round_trips_through_its_own_string_form(cost):
    assert parse_mana_cost(str(cost)) == cost


# Mana value per rule 202.3: a hybrid symbol counts as its highest possible
# value, so {2/W} is 2 and {W/U} is 1; Phyrexian counts as its color, so {U/P}
# is 1; {X} is 0 anywhere except on the stack. Written out as a table rather
# than derived, so it is checkable against the rules by reading it.
SYMBOL_MANA_VALUES = {
    "{0}": 0, "{1}": 1, "{2}": 2, "{7}": 7,
    "{X}": 0,
    "{W}": 1, "{U}": 1, "{B}": 1, "{R}": 1, "{G}": 1,
    "{C}": 1,
    "{W/U}": 1, "{B/G}": 1, "{2/W}": 2,
    "{W/P}": 1, "{U/P}": 1,
    "{S}": 1,
}


@given(st.lists(st.sampled_from(sorted(SYMBOL_MANA_VALUES)), max_size=8))
def test_mana_value_is_the_sum_of_its_symbols(symbols):
    """The curve in `decks/analyze.py` is built from this number, and a card in
    the wrong bucket is a wrong number in a generated primer."""
    expected = sum(SYMBOL_MANA_VALUES[s] for s in symbols)
    assert parse_mana_cost("".join(symbols)).mana_value == expected


@given(st.lists(st.sampled_from(sorted(SYMBOL_MANA_VALUES)), max_size=8))
def test_phyrexian_is_never_a_colour_requirement(symbols):
    """{U/P} is payable with 2 life, so it constrains the mana base not at all.
    It still counts toward mana value, and toward color identity."""
    phyrexian_only = [s for s in symbols if s.endswith("/P}")]
    cost = parse_mana_cost("".join(phyrexian_only))
    assert cost.pips == ()
    assert can_pay(cost, []) is (cost.generic == 0)


def test_phyrexian_mana_value_matches_scryfall():
    """The two Phyrexian cards in the curated decks, by name. Both live in the
    Tivit list; their Scryfall `cmc` is 1 and 4."""
    assert parse_mana_cost("{U/P}").mana_value == 1            # Mental Misstep
    assert parse_mana_cost("{3}{U/P}").mana_value == 4         # Phyrexian Metamorph
    assert parse_mana_cost("{U/P}").colors == frozenset({"U"})


# ---------------------------------------------------------------- corpus
#
# Deterministic coverage, so the suite does not depend on which examples
# Hypothesis rolled this run.

def test_the_corpus_is_the_size_it_claims():
    """Pins the enumeration itself. If someone widens an alphabet, the digest
    below changes for a reason that has nothing to do with the solver, and this
    assertion says which of the two happened."""
    assert sum(1 for _ in corpus_cases()) == 13_944


def test_solver_agrees_with_both_oracles_across_the_whole_corpus():
    disagreements = []
    for cost, pool in corpus_cases():
        got = can_pay(cost, pool)
        if got != brute_force_can_pay(cost, pool) or got != hall_can_pay(cost, pool):
            disagreements.append(case_id(cost, pool))
    assert not disagreements, (
        f"{len(disagreements)} case(s) disagree, first 10: {disagreements[:10]}"
    )


# The answers `mana.py` gives across the whole corpus, as one hash. This is a
# golden value: it does not verify correctness -- the oracles above do that --
# it makes a change in castability semantics impossible to make by accident.
# Regenerate deliberately, and say in the commit message what changed and why:
#     python tests/mana_oracle.py --digest
CORPUS_ANSWER_DIGEST = "ffa656572471da2a29177d3b195ffbe7684c2e0e9c2ec1f500934dd9d6fed36c"


def test_corpus_answers_have_not_changed():
    assert corpus_digest(can_pay) == CORPUS_ANSWER_DIGEST, (
        "castability semantics changed. If that was deliberate, update "
        "CORPUS_ANSWER_DIGEST and explain the change in the commit message."
    )


if __name__ == "__main__":
    failures = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"  PASS  {name}")
            except AssertionError as exc:
                failures += 1
                print(f"  FAIL  {name}: {exc}")
    print(f"\n{failures} failure(s)")
    sys.exit(1 if failures else 0)
