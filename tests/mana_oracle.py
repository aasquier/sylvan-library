"""Reference implementations of castability, and the case set they answer.

`mtglab.mana.can_pay` decides "can these sources pay this cost?" with Kuhn's
augmenting-path matching. It is fast, and it is subtle enough that a reader has
to take the algorithm on trust. This module removes the trust by answering the
same question two other ways, both obviously correct and both far too slow to
ship:

  * `brute_force_can_pay` enumerates every injective assignment of pips to mana
    units. No algorithm, just search.
  * `hall_can_pay` applies Hall's marriage theorem -- a matching covering every
    pip exists iff every subset of pips has at least as many usable units as it
    has pips. A different theorem, not a rewrite of the same one.

Two routes agreeing with the implementation is worth more than one, because the
failure they are guarding against is "the clever algorithm is subtly wrong" and
a second clever algorithm can be wrong the same way. Neither reference shares
code with `mana.py`; `_units` below is reimplemented rather than imported for
exactly that reason.

The case set is enumerated, not sampled. `all_cases()` yields the same cases in
the same order in any language, on any machine, forever -- which is what makes
it usable as the differential case set for a compiled port (docs/ENGINEERING.md
1). Run this module as a script to dump it as JSON Lines.
"""

from __future__ import annotations

import hashlib
import itertools
import sys
from collections.abc import Callable, Iterable, Iterator, Sequence
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.mana import ManaCost, ManaSource

Pool = tuple[ManaSource, ...]


def _units(sources: Iterable[ManaSource]) -> list[frozenset[str]]:
    """Flatten sources into one entry per mana produced.

    Deliberately not `mana.expand_units`: an oracle that shares code with the
    implementation cannot catch a bug in the shared part.
    """
    out: list[frozenset[str]] = []
    for source in sources:
        for _ in range(source.amount):
            out.append(source.colors)
    return out


def _required(cost: ManaCost, x_value: int) -> int:
    """Total mana the cost demands. Every point needs its own source -- nothing
    taps twice -- so this is a hard lower bound on the pool size."""
    return cost.generic + len(cost.pips) + (x_value if cost.has_x else 0)


def brute_force_can_pay(cost: ManaCost, sources: Iterable[ManaSource],
                        *, x_value: int = 0) -> bool:
    """Exhaustive search over assignments of pips to distinct mana units.

    Factorial in the number of pips. That is the point: there is nothing here
    to get wrong beyond the definition itself.
    """
    units = _units(sources)
    if len(units) < _required(cost, x_value):
        return False
    if not cost.pips:
        return True
    for choice in itertools.permutations(range(len(units)), len(cost.pips)):
        if all(units[j] & pip for j, pip in zip(choice, cost.pips, strict=True)):
            return True
    return False


def hall_can_pay(cost: ManaCost, sources: Iterable[ManaSource],
                 *, x_value: int = 0) -> bool:
    """Hall's condition: for every subset S of pips, at least |S| units can pay
    into S. Exponential in the pip count, and derived from a theorem rather
    than from a search, so it fails differently than the brute force above."""
    units = _units(sources)
    if len(units) < _required(cost, x_value):
        return False
    for size in range(1, len(cost.pips) + 1):
        for subset in itertools.combinations(cost.pips, size):
            usable = sum(1 for u in units if any(u & pip for pip in subset))
            if usable < size:
                return False
    return True


# -------------------------------------------------------------------- cases
#
# Small alphabets chosen for structure, not realism: a mono pip, a colorless
# pip that only {C} can pay, and a hybrid whose two colors are both reachable,
# against a pool alphabet that includes a restrictive dual and an any-color
# source. That is enough to produce Hall violations -- two pips whose only
# shared payer is one dual -- which is the family of cases naive source
# counting gets wrong.

CASE_PIPS: tuple[frozenset[str], ...] = (
    frozenset({"W"}),
    frozenset({"U"}),
    frozenset({"G"}),
    frozenset({"C"}),
    frozenset({"W", "U"}),
)

CASE_UNITS: tuple[frozenset[str], ...] = (
    frozenset({"W"}),
    frozenset({"U"}),
    frozenset({"G"}),
    frozenset({"C"}),
    frozenset({"W", "U"}),
    frozenset({"W", "U", "B", "R", "G"}),
)

MAX_PIPS = 3
MAX_GENERIC = 2
MAX_SOURCES = 3


def case_costs() -> Iterator[ManaCost]:
    """Every cost of up to MAX_PIPS pips over CASE_PIPS, at each generic
    count. Pips are drawn with replacement, so {W}{W} is included."""
    for size in range(MAX_PIPS + 1):
        for pips in itertools.combinations_with_replacement(CASE_PIPS, size):
            for generic in range(MAX_GENERIC + 1):
                yield ManaCost(generic=generic, pips=pips)


def case_pools() -> Iterator[Pool]:
    """Every pool of 1..MAX_SOURCES single-mana sources over CASE_UNITS."""
    for size in range(1, MAX_SOURCES + 1):
        for colors in itertools.combinations_with_replacement(CASE_UNITS, size):
            yield tuple(ManaSource(c) for c in colors)


def all_cases() -> Iterator[tuple[ManaCost, Pool]]:
    """The full cross product, costs outer and pools inner.

    Order is part of the contract: a port reimplementing this enumeration must
    produce the same sequence for the digest below to mean anything.
    """
    for cost in case_costs():
        for pool in case_pools():
            yield cost, pool


def case_id(cost: ManaCost, pool: Sequence[ManaSource]) -> str:
    """A stable one-line name for a case, for digests and failure messages."""
    pool_text = " ".join(
        "".join(sorted(s.colors)) + (f"x{s.amount}" if s.amount != 1 else "")
        for s in pool
    )
    return f"{cost} <- [{pool_text}]"


def cases_digest(answer: Callable[[ManaCost, Pool], bool]) -> str:
    """sha256 over `case=answer` lines for the whole case set, in order."""
    digest = hashlib.sha256()
    for cost, pool in all_cases():
        digest.update(f"{case_id(cost, pool)}={int(answer(cost, pool))}\n".encode())
    return digest.hexdigest()


if __name__ == "__main__":
    import json

    from mtglab.mana import can_pay

    # `--digest` prints the golden pinned by tests/test_mana_properties.py;
    # bare, it dumps the whole case set as JSON Lines for a port to consume.
    if "--digest" in sys.argv[1:]:
        print(cases_digest(can_pay))
        raise SystemExit(0)

    for case_cost, case_pool in all_cases():
        print(json.dumps({
            "cost": str(case_cost),
            "generic": case_cost.generic,
            "pips": ["".join(sorted(p)) for p in case_cost.pips],
            "pool": ["".join(sorted(s.colors)) for s in case_pool],
            "payable": can_pay(case_cost, case_pool),
        }))
