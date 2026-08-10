import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.mana import ManaSource, parse_mana_cost
from mtglab.sim.tier1.engine import KeepRule, SimCard, run, simulate_game, sweep_land_counts

B = frozenset({"B"})
G = frozenset({"G"})
BG = frozenset({"B", "G"})
C = frozenset({"C"})


def land(name, colors, tapped=False):
    return SimCard(name=name, category="land", is_land=True,
                   enters_tapped=tapped, produces=(ManaSource(colors),))


def spell(name, cost, category="threat", produces=(), delay=0, fetches=0):
    return SimCard(name=name, cost=parse_mana_cost(cost), category=category,
                   produces=tuple(produces), produce_delay=delay,
                   fetches_lands=fetches)


def build_golgari(n_lands: int):
    """A stand-in for the Gyome shell: Golgari, 5-drop commander, ramp-forward.

    Land split mirrors a real two-color base: roughly a third duals, the rest
    split between the two basics.
    """
    duals = n_lands // 3
    swamps = (n_lands - duals) // 2
    forests = n_lands - duals - swamps

    library = []
    library += [land(f"Dual {i}", BG, tapped=(i % 2 == 0)) for i in range(duals)]
    library += [land(f"Swamp {i}", B) for i in range(swamps)]
    library += [land(f"Forest {i}", G) for i in range(forests)]

    # 11 ramp: signets/rocks (no summoning sickness), dorks (delayed), land ramp
    library += [spell(f"Signet {i}", "{2}", "ramp", produces=[ManaSource(BG)])
                for i in range(3)]
    library += [spell(f"Dork {i}", "{G}", "ramp", produces=[ManaSource(G)], delay=1)
                for i in range(4)]
    library += [spell(f"Cultivate {i}", "{2}{G}", "ramp", fetches=1) for i in range(4)]

    filler = [
        ("{1}{B}", "interaction"), ("{2}{G}", "card-advantage"),
        ("{1}{G}", "engine"), ("{2}{B}", "engine"), ("{3}{G}", "threat"),
        ("{B}", "sac-outlet"), ("{1}", "sac-outlet"), ("{4}{B}{G}", "threat"),
        ("{2}{B}{G}", "payoff"), ("{3}{B}", "card-advantage"),
    ]
    i = 0
    while len(library) < 99:
        cost, cat = filler[i % len(filler)]
        library.append(spell(f"Spell {i}", cost, cat))
        i += 1

    commander = spell("Gyome, Master Chef", "{3}{B}{G}", "engine")
    return library[:99], commander


# ------------------------------------------------------------------ tests

def test_deck_builder_is_the_right_size():
    lib, cmd = build_golgari(34)
    assert len(lib) == 99
    assert sum(1 for c in lib if c.is_land) == 34
    assert cmd.mv == 5


def test_single_game_is_internally_consistent():
    lib, cmd = build_golgari(34)
    res = simulate_game(lib, cmd, turns=10)
    # Lands in play never decrease and never exceed the turn number.
    assert res.lands_by_turn == sorted(res.lands_by_turn)
    assert all(n <= t + 1 for t, n in enumerate(res.lands_by_turn, start=1))
    assert len(res.mana_by_turn) == 10


def test_commander_lands_at_a_plausible_turn():
    """A 5-drop commander in a ramp deck should median around turn 5.

    This is the cross-check against the previously computed Gyome result
    (34 lands, commander down by T5 in roughly three quarters of games).
    """
    lib, cmd = build_golgari(34)
    summary = run(lib, cmd, games=3_000, turns=12, seed=42)
    assert summary.median_commander_turn is not None
    assert 4 <= summary.median_commander_turn <= 6, summary.median_commander_turn
    assert 0.55 <= summary.commander_by_turn[5] <= 0.90, summary.commander_by_turn[5]
    assert summary.never_cast_commander < 0.05


def test_more_lands_means_faster_commander_up_to_a_point():
    """Sanity: the curve must rise then flatten. If it rises forever the model
    is broken (it would recommend 60 lands)."""
    results = sweep_land_counts(build_golgari, [28, 31, 34, 37, 40],
                                games=1_500, turns=10, seed=11)
    p5 = [s.commander_by_turn[5] for _, s in results]
    assert p5[0] < p5[2], f"28 lands should be worse than 34: {p5}"
    # Flattening: the gain from 37->40 should be smaller than 28->31.
    assert (p5[4] - p5[3]) < (p5[1] - p5[0]), p5


def test_strict_keep_rule_raises_mulligan_rate():
    lib, cmd = build_golgari(34)
    loose = run(lib, cmd, games=1_500, seed=5,
                keep_rule=KeepRule(min_lands=2, max_lands=6, min_mana_pieces=2))
    strict = run(lib, cmd, games=1_500, seed=5,
                 keep_rule=KeepRule(min_lands=3, max_lands=4, min_mana_pieces=5))
    assert strict.mulligan_rate > loose.mulligan_rate


def test_offcolor_base_produces_color_screw():
    """A deck whose lands cannot make its colors must show color-only blocks."""
    lib, cmd = build_golgari(34)
    broken = [land(f"Wastes {i}", C) if c.is_land else c for i, c in enumerate(lib)]
    good = run(lib, cmd, games=600, turns=8, seed=3)
    bad = run(broken, cmd, games=600, turns=8, seed=3)
    assert bad.color_screw_rate > 5 * good.color_screw_rate
    # Not ~100%: the Signets cost {2} and are castable off colorless mana, so
    # they still fix colors in a minority of games. That is correct behaviour,
    # and a useful reminder that colorless rocks are real fixing insurance.
    assert 0.5 < bad.never_cast_commander < 0.95
    assert bad.never_cast_commander > 5 * good.never_cast_commander


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
