"""Deterministic deck analysis.

These are the numbers the app shows instead of me working them out in chat,
so they need to be pinned. No DuckDB here -- the pool is a plain dict, the
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
    pool = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Bear": Rec("Bear", "{1}{G}")}
    out = curve(deck, pool)
    assert out["nonland_cards"] == 1
    assert out["average_mv"] == 2.0


def test_curve_weights_by_qty():
    deck = deck_of(CardEntry(name="Bear", category="threat", qty=3, why="x"))
    out = curve(deck, {"Bear": Rec("Bear", "{1}{G}")})
    assert out["nonland_cards"] == 3
    assert out["buckets"][0].mv == 2 and out["buckets"][0].count == 3


def test_curve_uses_deck_override_when_card_is_too_new():
    """CardEntry.mana_cost exists for cards not yet in the pool."""
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
    pool = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Swamp": Rec("Swamp", None, "Basic Land — Swamp", ("B",))}
    got = color_sources(deck, pool)
    assert got["G"] == 8 and got["B"] == 6 and got["W"] == 0


def test_dual_counts_for_both_colors():
    deck = deck_of(CardEntry(name="Overgrown Tomb", category="land", why="x"))
    pool = {"Overgrown Tomb": Rec("Overgrown Tomb", None,
                                    "Land — Swamp Forest", ("B", "G"))}
    got = color_sources(deck, pool)
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
    pool = {"Tower": Rec("Tower", None, "Land", ("B", "G"))}
    assert all(n.pips == 0 for n in pip_requirements(deck, pool))


def test_any_color_sources_do_not_invent_offidentity_colors():
    """A Golgari deck full of Command Towers must not report white sources.

    "Add one mana of any color" lists all five in produced_mana, which is true
    of the permanent and meaningless for the deck.
    """
    deck = deck_of(
        CardEntry(name="Command Tower", category="land", qty=10, why="x"),
        CardEntry(name="Doubler", category="threat", why="x"),
    )
    pool = {
        "Gyome, Master Chef": Rec("Gyome, Master Chef", "{2}{B}{G}",
                                  "Legendary Creature — Troll Warlock",
                                  color_identity=frozenset("BG")),
        "Command Tower": Rec("Command Tower", None, "Land",
                             ("W", "U", "B", "R", "G")),
        "Doubler": Rec("Doubler", "{B}{B}"),
    }
    reported = {n.color for n in pip_requirements(deck, pool)}
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
    pool = {"Doubler": Rec("Doubler", "{B}{B}"),
              "Swamp": Rec("Swamp", None, "Basic Land — Swamp", ("B",))}
    needs = {n.color: n for n in pip_requirements(deck, pool)}
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
    pool = {"Path": Rec("Path", None, "Land // Land"),
              "Gyome": Rec("Gyome", "{2}{B}{G}",
                           "Legendary Creature — Troll Warlock")}
    got = type_breakdown(deck, pool)
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
    pool = {"Forest": Rec("Forest", None, "Basic Land — Forest", ("G",)),
              "Bear": Rec("Bear", "{1}{G}")}
    stats = deck_stats(deck, pool)
    assert stats["total_cards"] == 9
    assert stats["land_count"] == 8
    assert {"curve", "categories", "colors", "types"} <= set(stats)

    import json
    json.dumps({k: v for k, v in stats.items() if k != "curve"})


def test_deck_stats_survives_a_missing_pool():
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


# ---------------------------------------------------------- game changers
#
# The one bracket rule that can be *counted* rather than estimated: Scryfall
# flags the official Game Changers list per card, so a declared bracket finally
# has something to be checked against. Reported, not enforced -- `validate.py`
# is the gate, and which Game Changer to cut is a decision rather than a fix.

def _Rec(game_changer=False, *, type_line="Artifact", cmc=2.0,
         mana_cost="{2}"):
    """A real `CardRecord`, not a stub.

    `deck_stats` reads the curve, the pips and the type line as well as this
    flag, and a stub thin enough to make one test pass is a stub that breaks
    the next one. Importing `db` costs nothing here -- duckdb is imported
    lazily inside `connect()`.
    """
    from mtglab.cards.db import CardRecord
    return CardRecord(
        name="stub", mana_cost=mana_cost, cmc=cmc, type_line=type_line,
        oracle_text="", color_identity=frozenset("G"), produced_mana=(),
        legal_commander=True, reserved=False, edhrec_rank=None,
        image_normal=None, game_changer=game_changer)


def _deck_with(names, bracket=3):
    from mtglab.decks.model import Deck
    return Deck.from_text(
        f"slug: gc\nname: GC\nbracket: {bracket}\n"
        "commander:\n  - Gyome, Master Chef\n"
        "cards:\n" + "".join(
            f"  - name: {n}\n    category: ramp\n    why: x\n" for n in names),
        slug="gc")


def test_counts_the_game_changers_a_deck_runs():
    from mtglab.decks.analyze import game_changers
    deck = _deck_with(["Smothering Tithe", "Llanowar Elves"])
    pool = {"Smothering Tithe": _Rec(True), "Llanowar Elves": _Rec(False),
              "Gyome, Master Chef": _Rec(False)}
    report = game_changers(deck, pool)
    assert report["cards"] == ["Smothering Tithe"]
    assert report["count"] == 1
    assert report["allowed"] == 3
    assert report["verdict"] == "ok"


def test_a_bracket_three_deck_over_the_cap_is_flagged():
    from mtglab.decks.analyze import game_changers
    names = ["A", "B", "C", "D"]
    deck = _deck_with(names, bracket=3)
    pool = {n: _Rec(True) for n in names} | {"Gyome, Master Chef": _Rec(False)}
    report = game_changers(deck, pool)
    assert report["count"] == 4
    assert report["verdict"] == "over", "bracket 3 permits three"


def test_a_high_bracket_has_no_limit():
    from mtglab.decks.analyze import game_changers
    names = ["A", "B", "C", "D", "E"]
    deck = _deck_with(names, bracket=5)
    pool = {n: _Rec(True) for n in names} | {"Gyome, Master Chef": _Rec(False)}
    report = game_changers(deck, pool)
    assert report["allowed"] is None
    assert report["verdict"] == "ok", "cEDH is where these belong"


def test_the_commander_counts_too():
    """A Game Changer in the command zone is available every game, which is
    rather the point of the list."""
    from mtglab.decks.analyze import game_changers
    deck = _deck_with(["Llanowar Elves"])
    pool = {"Llanowar Elves": _Rec(False), "Gyome, Master Chef": _Rec(True)}
    assert game_changers(deck, pool)["count"] == 1


def test_no_pool_is_unknown_rather_than_zero():
    """An absent count is not a count of zero. A deck reporting "0 Game
    Changers" because nobody looked is the quiet wrong answer this whole
    column set exists to prevent."""
    from mtglab.decks.analyze import game_changers
    report = game_changers(_deck_with(["Smothering Tithe"]), {})
    assert report["verdict"] == "unknown"
    assert report["count"] == 0


def test_no_declared_bracket_is_unknown_rather_than_ok():
    from mtglab.decks.analyze import game_changers
    deck = _deck_with(["Smothering Tithe"], bracket="")
    pool = {"Smothering Tithe": _Rec(True), "Gyome, Master Chef": _Rec(False)}
    assert game_changers(deck, pool)["verdict"] == "unknown"


def test_deck_stats_carries_the_report():
    from mtglab.decks.analyze import deck_stats
    stats = deck_stats(_deck_with(["Smothering Tithe"]),
                       {"Smothering Tithe": _Rec(True),
                        "Gyome, Master Chef": _Rec(False)})
    assert stats["game_changers"]["count"] == 1


# ---------------------------------------------------------- opening hands

def _big_deck(lands=34, threats=20):
    """A 99 with a known composition: `lands` Forests, `threats` Bears,
    and ramp filling the rest."""
    rest = 99 - lands - threats
    return deck_of(
        CardEntry(name="Forest", category="land", qty=lands, why="x"),
        CardEntry(name="Bear", category="threat", qty=threats, why="x"),
        CardEntry(name="Rock", category="ramp", qty=rest, why="x"),
    )


def test_opening_hand_land_distribution_sums_to_one():
    from mtglab.decks.analyze import opening_hand
    out = opening_hand(_big_deck())
    total = sum(row["chance"] for row in out["lands"]["distribution"])
    assert abs(total - 1.0) < 1e-9
    assert out["deck_size"] == 99
    assert out["hand_size"] == 7


def test_opening_hand_keepable_is_the_two_to_four_band():
    from mtglab.decks.analyze import opening_hand
    out = opening_hand(_big_deck())
    band = sum(row["chance"] for row in out["lands"]["distribution"]
               if 2 <= row["lands"] <= 4)
    assert out["lands"]["keepable"] == band
    assert 0.0 < band < 1.0


def test_opening_hand_zero_lands_matches_the_hand_computation():
    """One value pinned against arithmetic done by hand, not by the same
    formula: P(0 lands in 7 of 99, 34 lands) = C(65,7)/C(99,7), and the
    first factor form of that is 65/99 * 64/98 * ... * 59/93."""
    from mtglab.decks.analyze import opening_hand
    out = opening_hand(_big_deck(lands=34))
    expected = 1.0
    for i in range(7):
        expected *= (65 - i) / (99 - i)
    zero = out["lands"]["distribution"][0]
    assert zero["lands"] == 0
    assert abs(zero["chance"] - expected) < 1e-12


def test_opening_hand_certain_category_is_certain():
    """A category that is the whole deck is in every hand."""
    from mtglab.decks.analyze import opening_hand
    deck = deck_of(CardEntry(name="Forest", category="land", qty=99, why="x"))
    out = opening_hand(deck)
    row = next(r for r in out["categories"] if r["category"] == "land")
    assert row["in_opening_hand"] == 1.0


def test_opening_hand_singleton_is_seen_over_deck_size():
    """On the draw, by end of turn t you have seen 7 + t cards, and a
    singleton's odds are exactly that over the deck size."""
    from mtglab.decks.analyze import opening_hand
    out = opening_hand(_big_deck())
    by_turn = {row["turn"]: row for row in out["singleton"]}
    assert by_turn[4]["cards_seen"] == 11
    assert abs(by_turn[4]["chance"] - 11 / 99) < 1e-12


def test_deck_stats_carries_opening_hand():
    from mtglab.decks.analyze import deck_stats
    stats = deck_stats(_big_deck())
    assert stats["opening"]["lands"]["count"] == 34
