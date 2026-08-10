"""Deterministic deck analysis.

These are the numbers the app shows instead of me working them out in chat,
so they need to be pinned. No DuckDB here -- the corpus is a plain dict, the
same shape `decks/validate.py` accepts.
"""

import sys
from dataclasses import dataclass, field
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.decks.analyze import (
    category_report,
    color_sources,
    curve,
    deck_stats,
    pip_requirements,
    type_breakdown,
)
from mtglab.decks.model import CardEntry, Deck


@dataclass
class Rec:
    name: str
    mana_cost: str | None = "{1}{G}"
    type_line: str = "Creature — Troll"
    produced_mana: tuple = ()
    color_identity: frozenset = field(default_factory=frozenset)
    oracle_text: str = ""

    @property
    def is_land(self):
        return "Land" in self.type_line


def deck_of(*entries):
    return Deck(slug="t", name="T", commander=["Gyome, Master Chef"],
                cards=list(entries))


# ------------------------------------------------------------------- curve

def test_curve_excludes_lands():
    deck = deck_of(
        CardEntry(name="Forest", category="land", qty=8, why="x"),
        CardEntry(name="Bear", category="threat", why="x"),
    )
    corpus = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Bear": Rec("Bear", "{1}{G}")}
    out = curve(deck, corpus)
    assert out["nonland_cards"] == 1
    assert out["average_mv"] == 2.0


def test_curve_weights_by_qty():
    deck = deck_of(CardEntry(name="Bear", category="threat", qty=3, why="x"))
    out = curve(deck, {"Bear": Rec("Bear", "{1}{G}")})
    assert out["nonland_cards"] == 3
    assert out["buckets"][0].mv == 2 and out["buckets"][0].count == 3


def test_curve_uses_deck_override_when_card_is_too_new():
    """CardEntry.mana_cost exists for cards not yet in the corpus."""
    deck = deck_of(CardEntry(name="Spoiler", category="threat",
                             mana_cost="{4}{B}{G}", why="x"))
    assert curve(deck, {})["average_mv"] == 6.0


def test_empty_deck_does_not_divide_by_zero():
    assert curve(deck_of(), {})["average_mv"] == 0.0


# ------------------------------------------------------------------- colors

def test_color_sources_counts_lands_by_qty():
    deck = deck_of(
        CardEntry(name="Forest", category="land", qty=8, why="x"),
        CardEntry(name="Swamp", category="land", qty=6, why="x"),
    )
    corpus = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Swamp": Rec("Swamp", None, "Basic Land — Swamp", ("B",))}
    got = color_sources(deck, corpus)
    assert got["G"] == 8 and got["B"] == 6 and got["W"] == 0


def test_dual_counts_for_both_colors():
    deck = deck_of(CardEntry(name="Overgrown Tomb", category="land", why="x"))
    corpus = {"Overgrown Tomb": Rec("Overgrown Tomb", None,
                                    "Land — Swamp Forest", ("B", "G"))}
    got = color_sources(deck, corpus)
    assert got["B"] == 1 and got["G"] == 1


def test_pip_demand_counts_double_pips_twice():
    """{B}{B} is two pips of demand, not one. Getting this wrong is exactly
    how a deck passes identity checks and still cannot cast its own spells."""
    deck = deck_of(CardEntry(name="Doubler", category="threat", why="x"))
    needs = {n.color: n for n in
             pip_requirements(deck, {"Doubler": Rec("Doubler", "{1}{B}{B}")})}
    assert needs["B"].pips == 2
    assert needs["B"].cards == 1


def test_hybrid_pip_counts_for_every_payable_color():
    deck = deck_of(CardEntry(name="Hybrid", category="threat", why="x"))
    needs = {n.color: n for n in
             pip_requirements(deck, {"Hybrid": Rec("Hybrid", "{B/G}")})}
    assert needs["B"].pips == 1 and needs["G"].pips == 1


def test_lands_do_not_contribute_pip_demand():
    deck = deck_of(CardEntry(name="Tower", category="land", why="x"))
    corpus = {"Tower": Rec("Tower", None, "Land", ("B", "G"))}
    assert all(n.pips == 0 for n in pip_requirements(deck, corpus))


def test_any_color_sources_do_not_invent_offidentity_colors():
    """A Golgari deck full of Command Towers must not report white sources.

    "Add one mana of any color" lists all five in produced_mana, which is true
    of the permanent and meaningless for the deck.
    """
    deck = deck_of(
        CardEntry(name="Command Tower", category="land", qty=10, why="x"),
        CardEntry(name="Doubler", category="threat", why="x"),
    )
    corpus = {
        "Gyome, Master Chef": Rec("Gyome, Master Chef", "{2}{B}{G}",
                                  "Legendary Creature — Troll Warlock",
                                  color_identity=frozenset("BG")),
        "Command Tower": Rec("Command Tower", None, "Land",
                             ("W", "U", "B", "R", "G")),
        "Doubler": Rec("Doubler", "{B}{B}"),
    }
    reported = {n.color for n in pip_requirements(deck, corpus)}
    assert reported == {"B", "G"}, reported


def test_without_a_commander_falls_back_to_demanded_colors():
    deck = deck_of(CardEntry(name="Doubler", category="threat", why="x"))
    reported = {n.color for n in pip_requirements(deck, {"Doubler": Rec("Doubler", "{B}{B}")})}
    assert reported == {"B"}


def test_sources_per_pip_is_reported():
    deck = deck_of(
        CardEntry(name="Doubler", category="threat", why="x"),
        CardEntry(name="Swamp", category="land", qty=4, why="x"),
    )
    corpus = {"Doubler": Rec("Doubler", "{B}{B}"),
              "Swamp": Rec("Swamp", None, "Basic Land — Swamp", ("B",))}
    needs = {n.color: n for n in pip_requirements(deck, corpus)}
    assert needs["B"].sources == 4 and needs["B"].pips == 2
    assert needs["B"].sources_per_pip == 2.0


# --------------------------------------------------------------- categories

def test_category_report_flags_low_and_high():
    deck = deck_of(
        *[CardEntry(name=f"L{i}", category="land", why="x") for i in range(20)],
        *[CardEntry(name=f"R{i}", category="ramp", why="x") for i in range(2)],
    )
    by = {r["category"]: r for r in category_report(deck)}
    assert by["land"]["status"] == "low"      # 20 vs target 33-38
    assert by["ramp"]["status"] == "low"      # 2 vs target 8-12
    assert by["tutor"]["count"] == 0


def test_categories_without_a_target_report_no_status():
    by = {r["category"]: r for r in category_report(deck_of())}
    assert by["payoff"]["status"] is None
    assert by["land"]["status"] is not None


# -------------------------------------------------------------------- types

def test_type_breakdown_takes_the_front_face():
    deck = deck_of(
        CardEntry(name="Path", category="land", why="x"),
        CardEntry(name="Gyome", category="threat", why="x"),
    )
    corpus = {"Path": Rec("Path", None, "Land // Land"),
              "Gyome": Rec("Gyome", "{2}{B}{G}",
                           "Legendary Creature — Troll Warlock")}
    got = type_breakdown(deck, corpus)
    assert got["Land"] == 1
    assert got["Creature"] == 1


def test_unknown_cards_are_surfaced_not_dropped():
    deck = deck_of(CardEntry(name="Nonesuch", category="threat", why="x"))
    assert type_breakdown(deck, {})["Unknown"] == 1


# ------------------------------------------------------------------- report

def test_deck_stats_is_serialisable_and_complete():
    deck = deck_of(
        CardEntry(name="Forest", category="land", qty=8, why="x"),
        CardEntry(name="Bear", category="threat", why="x"),
    )
    corpus = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Bear": Rec("Bear", "{1}{G}")}
    stats = deck_stats(deck, corpus)
    assert stats["total_cards"] == 9
    assert stats["land_count"] == 8
    assert {"curve", "categories", "colors", "types"} <= set(stats)

    import json
    json.dumps({k: v for k, v in stats.items() if k != "curve"})


def test_deck_stats_survives_a_missing_corpus():
    """The app must still render something before `data refresh` has run."""
    stats = deck_stats(deck_of(CardEntry(name="Bear", category="threat", why="x")))
    assert stats["total_cards"] == 1


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
