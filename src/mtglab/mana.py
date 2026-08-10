"""Mana costs, mana sources, and an exact castability solver.

The castability question -- "given these untapped sources, can I pay this cost?"
-- is a bipartite matching problem, not a counting problem. Counting colored
sources gives wrong answers on hands with restrictive duals and hybrid costs.
We solve it exactly with augmenting-path matching (Kuhn's algorithm).

Vocabulary
----------
Pip      : one colored mana requirement, expressed as the SET of colors that
           can pay it. {G} -> {"G"}.  {G/W} -> {"G","W"}.  {G/P} -> satisfiable
           by G or by 2 life, so we treat it as always satisfiable.
Unit     : one mana of output. A source producing N mana expands into N units,
           each carrying the set of colors it can produce. Sol Ring -> two
           units of {"C"}. This expansion is what makes the matching correct
           for multi-mana sources.
"""

from __future__ import annotations

import re
from collections.abc import Iterable, Sequence
from dataclasses import dataclass

COLORS = ("W", "U", "B", "R", "G")
COLORLESS = "C"
ALL_PRODUCIBLE = frozenset(COLORS) | {COLORLESS}

_SYMBOL_RE = re.compile(r"\{([^}]+)\}")


@dataclass(frozen=True)
class ManaCost:
    """A parsed mana cost.

    generic  : number of generic mana required ({2} -> 2). X counts as 0 here;
               callers decide what to pay into X.
    pips     : tuple of frozensets, one per colored requirement.
    has_x    : whether the cost contains {X}.
    """

    generic: int = 0
    pips: tuple[frozenset[str], ...] = ()
    has_x: bool = False

    @property
    def mana_value(self) -> int:
        return self.generic + len(self.pips)

    @property
    def colors(self) -> frozenset[str]:
        """Every color that appears in any pip (used for identity checks)."""
        out: set[str] = set()
        for pip in self.pips:
            out |= {c for c in pip if c in COLORS}
        return frozenset(out)

    def __str__(self) -> str:
        parts = []
        if self.has_x:
            parts.append("{X}")
        if self.generic:
            parts.append("{%d}" % self.generic)
        for pip in self.pips:
            parts.append("{%s}" % "/".join(sorted(pip)))
        return "".join(parts) or "{0}"


def parse_mana_cost(cost: str | None) -> ManaCost:
    """Parse a Scryfall-style mana cost string, e.g. '{2}{G}{W}' or '{X}{U/R}'.

    Handles generic, colored, colorless {C}, hybrid {G/W}, monocolor hybrid
    {2/G}, and Phyrexian {G/P}.
    """
    if not cost:
        return ManaCost()

    generic = 0
    pips: list[frozenset[str]] = []
    has_x = False

    for raw in _SYMBOL_RE.findall(cost):
        sym = raw.upper().strip()

        if sym == "X":
            has_x = True
            continue
        if sym.isdigit():
            generic += int(sym)
            continue
        if sym == COLORLESS:
            # {C} demands genuinely colorless mana. Treated as a pip payable
            # only by colorless sources.
            pips.append(frozenset({COLORLESS}))
            continue
        if sym in COLORS:
            pips.append(frozenset({sym}))
            continue

        if "/" in sym:
            halves = sym.split("/")
            # Phyrexian: payable with 2 life, so never a constraint on mana.
            if "P" in halves:
                continue
            # Monocolor hybrid {2/G}: payable by the color, or by 2 generic.
            # The cheaper branch for castability is the colored one, but if we
            # lack the color it costs 2 generic. We model it as a pip that any
            # source can satisfy, plus 1 extra generic -- an approximation that
            # never claims a hand is castable when it is not.
            numeric = [h for h in halves if h.isdigit()]
            if numeric:
                generic += int(numeric[0])
                continue
            colorset = {h for h in halves if h in ALL_PRODUCIBLE}
            if colorset:
                pips.append(frozenset(colorset))
            continue

        # Unknown symbol (e.g. snow {S}); treat as generic so we never
        # overstate castability.
        generic += 1

    return ManaCost(generic=generic, pips=tuple(pips), has_x=has_x)


@dataclass(frozen=True)
class ManaSource:
    """Something that produces mana.

    colors : which colors this source can produce. Use {"C"} for colorless.
             A source producing "any color" gets the full WUBRG set.
    amount : how many mana it makes per activation (Sol Ring -> 2).
    """

    colors: frozenset[str]
    amount: int = 1

    def units(self) -> list[frozenset[str]]:
        return [self.colors] * self.amount


def expand_units(sources: Iterable[ManaSource]) -> list[frozenset[str]]:
    """Flatten sources into individual mana units for the matching solver."""
    units: list[frozenset[str]] = []
    for src in sources:
        units.extend(src.units())
    return units


def _max_matching(pips: Sequence[frozenset[str]], units: Sequence[frozenset[str]]) -> int:
    """Maximum bipartite matching between pips and mana units (Kuhn's).

    A pip may be matched to a unit iff the unit can produce a color the pip
    accepts. Returns the size of the maximum matching.
    """
    # adjacency: for each pip, the indices of units that can pay it
    adj: list[list[int]] = []
    for pip in pips:
        adj.append([j for j, unit in enumerate(units) if unit & pip])

    match_unit: list[int] = [-1] * len(units)  # unit index -> pip index

    def try_assign(pip_idx: int, seen: list[bool]) -> bool:
        for unit_idx in adj[pip_idx]:
            if seen[unit_idx]:
                continue
            seen[unit_idx] = True
            if match_unit[unit_idx] == -1 or try_assign(match_unit[unit_idx], seen):
                match_unit[unit_idx] = pip_idx
                return True
        return False

    matched = 0
    for pip_idx in range(len(pips)):
        if try_assign(pip_idx, [False] * len(units)):
            matched += 1
    return matched


def can_pay(cost: ManaCost, sources: Iterable[ManaSource], *, x_value: int = 0) -> bool:
    """Exact test: can this set of sources pay this cost?

    Two conditions must both hold:
      1. every colored pip can be simultaneously assigned a distinct source
         that produces an acceptable color (maximum matching == len(pips));
      2. the sources left over cover the generic portion.

    Condition 2 is safe because any source can pay generic mana.
    """
    units = expand_units(sources)
    total_needed = cost.generic + len(cost.pips) + (x_value if cost.has_x else 0)
    if len(units) < total_needed:
        return False
    if not cost.pips:
        return True
    return _max_matching(cost.pips, units) == len(cost.pips)


def color_identity_of(*, mana_cost: str | None = None,
                      oracle_text: str | None = None,
                      explicit: Iterable[str] | None = None) -> frozenset[str]:
    """Best-effort color identity.

    Prefer `explicit` (Scryfall's `color_identity`, which is authoritative and
    already accounts for back faces, reminder text, and land types). The
    derivation from cost + text exists only as a fallback for cards not yet in
    the local database -- e.g. freshly spoiled previews.
    """
    if explicit is not None:
        return frozenset(c.upper() for c in explicit if c.upper() in COLORS)

    identity: set[str] = set(parse_mana_cost(mana_cost).colors)
    if oracle_text:
        # Mana symbols anywhere in rules text contribute to identity.
        for raw in _SYMBOL_RE.findall(oracle_text):
            for ch in raw.upper():
                if ch in COLORS:
                    identity.add(ch)
    return frozenset(identity)
