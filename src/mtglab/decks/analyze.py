"""Deterministic deck analysis.

Everything here is a pure function of a deck plus a card pool. No DuckDB, no
network, no judgement calls that vary between runs -- the same deck always
produces the same report, and the API layer is a thin wrapper over these.

That is the point. Curve, pip demand and category coverage were previously
worked out by hand in conversation, which meant nobody running the app could
reproduce them and nothing tested them. Logic that lives here gets tests.

The pool is passed in as a `{name: CardRecord}` dict, matching
`decks/validate.py`, so these test without a database.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from mtglab.decks.model import CATEGORIES, Deck
from mtglab.mana import COLORS, parse_mana_cost

if TYPE_CHECKING:
    from mtglab.cards.db import CardRecord

# Rough coverage targets by bracket. These are conventional Commander
# deckbuilding guidance, not computed truth, and are reported as guidance
# rather than enforced -- `validate.py` is the gate, this is the advice.
CATEGORY_TARGETS: dict[str, tuple[int, int]] = {
    "land": (33, 38),
    "ramp": (8, 12),
    "card-advantage": (8, 12),
    "interaction": (6, 12),
    "tutor": (0, 10),
}

#: How many cards from the official Game Changers list each bracket permits.
#: `None` means no limit.
#:
#: Unlike `CATEGORY_TARGETS` above, which is conventional guidance, this is the
#: published rule for the bracket a deck declares — which is why it can be
#: *counted* rather than estimated. Scryfall carries the flag per card, so the
#: count is a card pool fact.
#:
#: It is still reported rather than enforced. `validate.py` is the gate and
#: this module is the advice, and the same reasoning that keeps a banned card a
#: matter for the user to resolve applies here: which Game Changer to cut, or
#: whether to move the deck up a bracket, is a decision rather than a fix.
GAME_CHANGER_LIMITS: dict[int, int | None] = {
    1: 0, 2: 0, 3: 3, 4: None, 5: None,
}


def game_changers(deck: Deck, cards: dict[str, CardRecord] | None = None) -> dict[str, Any]:
    """Which Game Changers the deck runs, and what its bracket allows.

    Returns `allowed: None` for an unlimited bracket and `verdict: "unknown"`
    when the deck declares no bracket or the pool is missing — an absent
    count is not a count of zero, and a deck that reports "0 Game Changers"
    because nobody looked is the quiet wrong answer this project keeps finding.
    """
    cards = cards or {}
    names = [c.name for c in deck.cards] + list(deck.commander)
    if deck.companion:
        names.append(deck.companion)

    found = [n for n in names
             if getattr(cards.get(n), "game_changer", False)]
    allowed = GAME_CHANGER_LIMITS.get(deck.bracket or 0)

    if not cards or deck.bracket is None:
        verdict = "unknown"
    elif allowed is None or len(found) <= allowed:
        verdict = "ok"
    else:
        verdict = "over"

    return {
        "cards": sorted(found),
        "count": len(found),
        "allowed": allowed,
        "bracket": deck.bracket,
        "verdict": verdict,
    }


@dataclass
class CurveBucket:
    mv: int
    count: int
    names: list[str] = field(default_factory=list)


@dataclass
class ColorNeed:
    """Demand for one color, and the supply available to meet it."""

    color: str
    pips: int          # colored pips this deck's spells require, weighted by qty
    sources: int       # permanents/lands that can produce this color
    cards: int         # cards requiring at least one pip of this color

    @property
    def sources_per_pip(self) -> float:
        return round(self.sources / self.pips, 2) if self.pips else 0.0


def _is_land(rec: Any) -> bool:
    return bool(rec) and "Land" in (getattr(rec, "type_line", "") or "")


def curve(deck: Deck, cards: dict[str, CardRecord] | None = None) -> dict[str, Any]:
    """MV histogram over the nonland 99, plus the average nonland MV.

    Lands are excluded: a land has no mana value worth curving, and including
    them drags the average toward zero and makes decks incomparable.
    """
    buckets: dict[int, CurveBucket] = {}
    total_mv = 0
    counted = 0

    for entry in deck.cards:
        rec = (cards or {}).get(entry.name)
        if entry.category == "land" or _is_land(rec):
            continue
        cost = entry.mana_cost or (getattr(rec, "mana_cost", None) if rec else None)
        mv = parse_mana_cost(cost).mana_value
        bucket = buckets.setdefault(mv, CurveBucket(mv=mv, count=0))
        bucket.count += entry.qty
        bucket.names.append(entry.name)
        total_mv += mv * entry.qty
        counted += entry.qty

    return {
        "buckets": [buckets[mv] for mv in sorted(buckets)],
        "nonland_cards": counted,
        "average_mv": round(total_mv / counted, 2) if counted else 0.0,
    }


def color_sources(deck: Deck, cards: dict[str, CardRecord]) -> dict[str, int]:
    """How many permanents can produce each color.

    Uses Scryfall's `produced_mana`, which already accounts for lands that make
    mana through a type line (a Forest produces {G} without saying so) and for
    "add one mana of any color" effects.
    """
    counts = dict.fromkeys(COLORS, 0)
    for entry in deck.cards:
        rec = cards.get(entry.name)
        if rec is None:
            continue
        for color in getattr(rec, "produced_mana", ()) or ():
            if color in counts:
                counts[color] += entry.qty
    return counts


def commander_identity(deck: Deck, cards: dict[str, CardRecord]) -> frozenset[str]:
    """The deck's legal colors, taken from the commander's Scryfall identity.

    Never derived from mana costs -- identity already accounts for back faces,
    reminder text and land types, which a cost does not.
    """
    identity: set[str] = set()
    for name in deck.commander:
        rec = cards.get(name)
        if rec is not None:
            identity |= set(getattr(rec, "color_identity", ()) or ())
    return frozenset(identity)


def pip_requirements(deck: Deck, cards: dict[str, CardRecord]) -> list[ColorNeed]:
    """Colored pip demand against colored source supply, per color.

    This is the number that explains color screw and nothing else computes it.
    A deck can pass every identity check and still be unable to cast its own
    spells, because identity says "this card is legal here" while pips say
    "you need double black on turn three".

    Hybrid pips count toward every color that can pay them, since either will
    do. That deliberately overstates supply-side pressure rather than
    understating it.

    Only colors inside the commander's identity are reported. "Add one mana of
    any color" permanents list all five in `produced_mana`, so counting them
    naively claims a Golgari deck has ten white sources -- true of the
    permanent, meaningless for the deck, and pure noise in the chart.
    """
    pips = dict.fromkeys(COLORS, 0)
    carders = dict.fromkeys(COLORS, 0)

    for entry in deck.cards:
        rec = cards.get(entry.name)
        if entry.category == "land" or _is_land(rec):
            continue
        cost = entry.mana_cost or (getattr(rec, "mana_cost", None) if rec else None)
        seen: set[str] = set()
        for pip in parse_mana_cost(cost).pips:
            for color in pip:
                if color in pips:
                    pips[color] += entry.qty
                    seen.add(color)
        for color in seen:
            carders[color] += entry.qty

    sources = color_sources(deck, cards)
    identity = commander_identity(deck, cards)
    # Without a commander in the pool there is no identity to filter by, so
    # fall back to whatever the spells actually demand.
    relevant = identity or {c for c in COLORS if pips[c]}
    return [
        ColorNeed(color=c, pips=pips[c], sources=sources.get(c, 0), cards=carders[c])
        for c in COLORS
        if c in relevant
    ]


def category_report(deck: Deck) -> list[dict[str, Any]]:
    """Category counts against conventional targets.

    Answers "is ramp covered" with a number and a range rather than an
    opinion. Categories with no target are reported with `target: None` so the
    UI can show the count without implying a verdict.
    """
    counts = deck.category_counts
    out = []
    for category in CATEGORIES:
        count = counts.get(category, 0)
        target = CATEGORY_TARGETS.get(category)
        status = "ok"
        if target:
            low, high = target
            status = "low" if count < low else "high" if count > high else "ok"
        out.append({
            "category": category,
            "count": count,
            "target_low": target[0] if target else None,
            "target_high": target[1] if target else None,
            "status": status if target else None,
        })
    return out


def type_breakdown(deck: Deck, cards: dict[str, CardRecord]) -> dict[str, int]:
    """Counts by primary card type, taking the front face of split cards."""
    out: dict[str, int] = {}
    for entry in deck.cards:
        rec = cards.get(entry.name)
        if rec is None:
            out["Unknown"] = out.get("Unknown", 0) + entry.qty
            continue
        front = (rec.type_line or "").split(" // ")[0]
        # "Legendary Creature — Troll Warlock" -> "Creature"
        left = front.split("—")[0].strip()
        primary = left.split()[-1] if left.split() else "Unknown"
        out[primary] = out.get(primary, 0) + entry.qty
    return dict(sorted(out.items(), key=lambda kv: -kv[1]))


def deck_stats(deck: Deck, cards: dict[str, CardRecord] | None = None) -> dict[str, Any]:
    """The whole deterministic report, as plain data ready to serialise."""
    cards = cards or {}
    return {
        "slug": deck.slug,
        "name": deck.name,
        "commander": deck.commander,
        "bracket": deck.bracket,
        "total_cards": deck.total_cards,
        "land_count": deck.land_count,
        "curve": curve(deck, cards),
        "categories": category_report(deck),
        "game_changers": game_changers(deck, cards),
        "colors": [
            {
                "color": n.color,
                "pips": n.pips,
                "sources": n.sources,
                "cards": n.cards,
                "sources_per_pip": n.sources_per_pip,
            }
            for n in pip_requirements(deck, cards)
        ],
        "types": type_breakdown(deck, cards),
    }
