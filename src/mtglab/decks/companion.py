"""Companion deckbuilding restrictions.

This module exists because the gate happily accepted
`companion: Kaheera, the Orphanguard` on the Arahbo list while checking
nothing about it. It confirmed the card *had* a Companion ability and stopped
there -- so a deck with a single non-Cat creature would have passed validation
and been declared legal, and the artifacts would have said so in writing.

Two rules shape the code:

1. **The condition text comes from the pool, never from memory.** Kaheera's
   allowed creature types are parsed out of Kaheera's own oracle text rather
   than typed in here, so a future errata changes the check automatically.
   Reading the pool while writing this also turned up three companions that
   are not the well-known ten -- Lutri, Pauper Otter; Treizeci, Sun of Serra;
   and The Companion of the Wilds -- whose conditions reference expansion
   symbols, frame treatments and set codes. None of those are properties of an
   oracle card, so they cannot be checked here at all.

2. **An unknown condition is reported, never passed.** A companion this module
   does not recognise produces a loud "not checked" warning. Silently
   returning "no violations" for a rule we never evaluated is exactly the
   failure this project exists to avoid.

Scope note: "your starting deck" includes your commander. A Kaheera deck whose
commander is a creature needs that commander to be one of her types too --
Arahbo, Roar of the World is a Cat Avatar, so the cats list is legal.
"""

from __future__ import annotations

import re
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from mtglab.cards.db import CardRecord

#: The starting deck as (card name, its pool record) pairs -- the 99 and the
#: commander, never the companion itself. Every rule below takes the same
#: shape, which is what lets `check` dispatch through one table.
Entries = Sequence[tuple[str, "CardRecord"]]

# Card types that make a card a permanent card. A card's types are those of
# its front face, which is why the MDFC split matters here.
PERMANENT_TYPES = ("Artifact", "Creature", "Enchantment", "Land",
                   "Planeswalker", "Battle")

# Keyword abilities that ARE activated abilities but are printed without a
# colon when Scryfall omits reminder text. Without these, Zirda would flag an
# Equipment whose only activated ability is its equip cost.
ACTIVATED_KEYWORDS = (
    "equip", "cycling", "level up", "crew", "unearth", "channel", "forecast",
    "fortify", "reconfigure", "outlast", "monstrosity", "adapt", "scavenge",
    "transfigure", "transmute", "morph", "megamorph", "disguise", "prototype",
    "boast", "exhaust",
)


@dataclass
class CompanionCheck:
    """The result of checking one companion's deckbuilding restriction."""

    condition: str
    violations: list[str] = field(default_factory=list)
    # False when the check is a heuristic rather than an exact reading of the
    # card data. Callers should report these as warnings, not errors.
    exact: bool = True
    # Set when the condition cannot be evaluated from oracle data at all.
    unsupported: str = ""

    @property
    def ok(self) -> bool:
        return not self.violations and not self.unsupported


def _front(type_line: str) -> str:
    return (type_line or "").split(" // ")[0]


def _is_permanent(rec: Any) -> bool:
    return any(t in _front(rec.type_line) for t in PERMANENT_TYPES)


def _is_land(rec: Any) -> bool:
    # Deliberately the card's own front face, not CardRecord.is_land. A modal
    # DFC with a land back is a nonland *card*; you only get a land by
    # choosing that face on the way down.
    return "Land" in _front(rec.type_line)


def _mana_symbols(mana_cost: str | None) -> list[str]:
    """Colour, hybrid and colourless symbols in a cost. Generic and {X} are
    excluded: Jegantha cares about repeated *mana symbols*, and {3} is a
    single generic symbol rather than three of one kind."""
    out = []
    for sym in re.findall(r"\{([^}]+)\}", mana_cost or ""):
        if sym.isdigit() or sym.upper() == "X":
            continue
        out.append(sym.upper())
    return out


def _has_activated_ability(rec: Any) -> bool:
    """Heuristic. An activated ability is written `cost: effect`, so a colon
    is a strong signal -- but keywords like `Equip {2}` are activated abilities
    printed without one whenever reminder text is absent."""
    text = (rec.oracle_text or "")
    for line in text.split("\n"):
        # Strip reminder text before looking for the colon, so a triggered
        # ability whose reminder happens to contain one is not miscounted.
        bare = re.sub(r"\([^)]*\)", "", line)
        if ":" in bare:
            return True
        if bare.strip().lower().startswith(ACTIVATED_KEYWORDS):
            return True
    # Planeswalker loyalty abilities are activated abilities.
    return "Planeswalker" in _front(rec.type_line)


# --------------------------------------------------------------- conditions
#
# Each takes the cards making up the starting deck (commander included,
# companion excluded) and returns the names that break the restriction.

def _even_mana_values(entries: Entries, rec: CardRecord) -> list[str]:
    # Gyruda has no land exception -- but lands are mana value 0, which is
    # even, so they pass on their own merits.
    return [n for n, r in entries if int(r.cmc) % 2 != 0]


def _odd_mana_values_or_land(entries: Entries, rec: CardRecord) -> list[str]:
    return [n for n, r in entries if not _is_land(r) and int(r.cmc) % 2 == 0]


def _mv_three_or_greater_or_land(entries: Entries,
                                 rec: CardRecord) -> list[str]:
    return [n for n, r in entries if not _is_land(r) and r.cmc < 3]


def _permanent_mv_two_or_less(entries: Entries, rec: CardRecord) -> list[str]:
    return [n for n, r in entries if _is_permanent(r) and r.cmc > 2]


def _creature_types(entries: Entries, rec: CardRecord) -> list[str]:
    """Kaheera. The allowed types are read out of her own oracle text."""
    m = re.search(r"is an? (.+?) card", rec.oracle_text or "")
    if not m:
        raise ValueError("could not parse the allowed creature types")
    allowed = [t.strip() for t in re.split(r",\s*|\s+or\s+", m.group(1)) if t.strip()]
    return [n for n, r in entries
            if "Creature" in _front(r.type_line)
            and not any(t in _front(r.type_line) for t in allowed)]


def _no_repeated_mana_symbol(entries: Entries, rec: CardRecord) -> list[str]:
    out = []
    for n, r in entries:
        syms = _mana_symbols(r.mana_cost)
        if len(syms) != len(set(syms)):
            out.append(n)
    return out


def _distinct_nonland_names(entries: Entries, rec: CardRecord) -> list[str]:
    seen: dict[str, int] = {}
    for n, r in entries:
        if not _is_land(r):
            seen[n] = seen.get(n, 0) + 1
    return sorted(n for n, c in seen.items() if c > 1)


def _nonland_shares_a_type(entries: Entries, rec: CardRecord) -> list[str]:
    nonland = [(n, r) for n, r in entries if not _is_land(r)]
    if not nonland:
        return []
    shared = set(PERMANENT_TYPES) | {"Instant", "Sorcery"}
    for _, r in nonland:
        shared &= {t for t in shared if t in _front(r.type_line)}
    if shared:
        return []
    # No single type is common to all of them. Report the minority so the
    # message is actionable rather than listing the whole deck.
    counts: dict[str, int] = {}
    for _, r in nonland:
        for t in set(PERMANENT_TYPES) | {"Instant", "Sorcery"}:
            if t in _front(r.type_line):
                counts[t] = counts.get(t, 0) + 1
    if not counts:
        return [n for n, _ in nonland]
    best = max(counts, key=lambda k: counts[k])
    return [n for n, r in nonland if best not in _front(r.type_line)]


def _permanents_have_activated_abilities(entries: Entries,
                                         rec: CardRecord) -> list[str]:
    return [n for n, r in entries
            if _is_permanent(r) and not _has_activated_ability(r)]


# name -> (checker, exact). `exact=False` marks a heuristic, which callers
# should surface as a warning rather than a hard failure.
_CHECKS: dict[str, tuple[Callable[[Any, Any], list[str]], bool]] = {
    "gyruda, doom of depths": (_even_mana_values, True),
    "obosh, the preypiercer": (_odd_mana_values_or_land, True),
    "keruga, the macrosage": (_mv_three_or_greater_or_land, True),
    "lurrus of the dream-den": (_permanent_mv_two_or_less, True),
    "kaheera, the orphanguard": (_creature_types, True),
    "jegantha, the wellspring": (_no_repeated_mana_symbol, True),
    "lutri, the spellchaser": (_distinct_nonland_names, True),
    "umori, the collector": (_nonland_shares_a_type, True),
    "zirda, the dawnwaker": (_permanents_have_activated_abilities, False),
    # Yorion restricts deck SIZE, not card contents, so it is handled by the
    # caller's size check rather than by a per-card scan. See DECK_SIZE_BONUS.
    "yorion, sky nomad": (lambda entries, rec: [], True),
}

# Yorion is the only companion that changes how big the deck must be:
# "at least twenty cards more than the minimum deck size".
DECK_SIZE_BONUS = {"yorion, sky nomad": 20}

# Conditions that reference something an oracle card does not carry -- set
# membership, expansion symbols, frame treatments. Named explicitly so they
# report "cannot check" rather than falling through to "unknown companion".
_UNCHECKABLE = {
    "lutri, pauper otter": "the condition is about expansion symbols, which are "
                           "a property of a printing rather than an oracle card",
    "treizeci, sun of serra": "the condition is about retro frames and other "
                              "'nostalgic' treatments, which are per-printing",
    "the companion of the wilds": "the condition names specific sets, which "
                                  "oracle data does not carry",
}


def condition_text(rec: Any) -> str:
    """The Companion sentence from a card's oracle text, reminder stripped."""
    for line in (rec.oracle_text or "").split("\n"):
        if re.match(r"^(old )?companion\s*—", line.strip(), re.IGNORECASE):
            return re.sub(r"\s*\([^)]*\)\s*$", "", line).strip()
    return ""


def is_companion(rec: Any) -> bool:
    """Does this card actually have a Companion ability?

    Tested against the `Companion —` ability marker rather than the mere
    presence of the word, which appears in flavour and rules text elsewhere.
    """
    return bool(condition_text(rec))


def check(companion_name: str, entries: Entries,
          cards: dict[str, CardRecord]) -> CompanionCheck:
    """Check a companion's restriction against the starting deck.

    `entries` is a sequence of (card_name, CardRecord) covering the whole
    starting deck -- the 99 and the commander, but not the companion itself.
    """
    rec = cards.get(companion_name)
    if rec is None:
        return CompanionCheck(condition="", unsupported="card not in the pool")

    condition = condition_text(rec)
    if not condition:
        return CompanionCheck(condition="", unsupported="card has no Companion ability")

    key = companion_name.lower()
    if key in _UNCHECKABLE:
        return CompanionCheck(condition=condition, unsupported=_UNCHECKABLE[key])

    entry = _CHECKS.get(key)
    if entry is None:
        return CompanionCheck(
            condition=condition,
            unsupported="no checker is implemented for this companion")

    fn, exact = entry
    try:
        violations = fn(entries, rec)
    except ValueError as exc:                     # condition text did not parse
        return CompanionCheck(condition=condition, unsupported=str(exc))
    return CompanionCheck(condition=condition, violations=violations, exact=exact)
