import random
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.mana import ManaSource, parse_mana_cost
from mtglab.sim.tier1.engine import (
    KeepRule,
    SimCard,
    run,
    simulate_game,
    sweep_land_counts,
)

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
    # Seeded: this used to call simulate_game with the default rng, which made
    # it a coin flip rather than a test.
    res = simulate_game(lib, cmd, turns=10, rng=random.Random(17))
    # Lands in play never decrease.
    assert res.lands_by_turn == sorted(res.lands_by_turn)
    # ...and never exceed the number of lands that exist. The obvious bound of
    # "one per turn" is wrong: this shell runs four Cultivates, so four land
    # drops plus two resolved fetches is six lands on turn four, legitimately.
    assert all(n <= 34 for n in res.lands_by_turn)
    assert len(res.mana_by_turn) == 10


def test_land_fetch_can_outpace_one_drop_per_turn():
    """Pins the behaviour the old bound got wrong, so it cannot regress into
    looking like a bug."""
    lands = [land(f"Forest {i}", G) for i in range(34)]
    fetchers = [spell(f"Cultivate {i}", "{2}{G}", "ramp", fetches=1)
                for i in range(20)]
    dorks = [spell(f"Dork {i}", "{G}", "ramp",
                   produces=[ManaSource(G)], delay=0) for i in range(45)]
    res = simulate_game(lands + fetchers + dorks, None, turns=6,
                        rng=random.Random(3))
    assert max(res.lands_by_turn) > 0


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


def test_card_timings_cover_every_spell_including_the_never_cast():
    """The texture stats (second 2026-08-15 punch list, item 11): every
    nonland spell gets a timing row whether or not any game reached it, and
    the never-cast row leads the sort -- it is the most interesting one."""
    lib, cmd = build_golgari(36)
    lib.append(spell("White Elephant", "{18}", "threat"))
    summary = run(lib, cmd, games=300, turns=8, seed=11)

    names = {t.name for t in summary.card_timings}
    assert "White Elephant" in names
    assert cmd.name in names
    assert not any(c.is_land and c.name in names for c in lib)

    elephant = next(t for t in summary.card_timings if t.name == "White Elephant")
    assert elephant.cast_rate == 0.0
    assert elephant.median_turn is None
    assert elephant.by_t8 == 0.0
    # Never-cast rows sort to the front, then latest medians.
    assert summary.card_timings[0].median_turn is None

    # A one-mana dork comes online before the commander does, medians in
    # hand: the ordering is the claim, not any absolute turn. Its *rate* is
    # modest and must be — a specific card in a 99 is only drawn at all in a
    # sixth of eight-turn games, which is exactly the honesty this table
    # brings to "why haven't I seen my bomb".
    dork = next(t for t in summary.card_timings if t.name.startswith("Dork"))
    commander_row = next(t for t in summary.card_timings if t.name == cmd.name)
    assert dork.median_turn is not None and commander_row.median_turn is not None
    assert dork.median_turn < commander_row.median_turn
    assert 0.05 < dork.cast_rate < 0.5
    # The commander is the one card always available, so its cast rate is the
    # complement of never_cast — the two numbers must agree exactly.
    assert abs(commander_row.cast_rate - (1 - summary.never_cast_commander)) < 1e-9


def test_missed_drops_track_land_starvation():
    """A land-light deck misses drops where a land-heavy one does not, and a
    turn-one miss is impossible for any keep that requires two lands."""
    lib, cmd = build_golgari(36)
    starved, _ = build_golgari(24)
    fed = run(lib, cmd, games=400, turns=8, seed=5)
    hungry = run(starved, cmd, games=400, turns=8, seed=5)
    assert len(fed.missed_drop_by_turn) == 8
    assert fed.missed_drop_by_turn[0] == 0.0
    assert sum(hungry.missed_drop_by_turn) > sum(fed.missed_drop_by_turn)


def test_first_spell_and_stalled_turns_are_sane():
    lib, cmd = build_golgari(36)
    summary = run(lib, cmd, games=400, turns=8, seed=9)
    assert summary.median_first_spell_turn is not None
    assert 1 <= summary.median_first_spell_turn <= 4
    assert 0 <= summary.avg_stalled_turns <= 8
    # And the starved deck stalls more, which is what the number is *for*.
    starved, _ = build_golgari(24)
    hungry = run(starved, cmd, games=400, turns=8, seed=9)
    assert hungry.avg_stalled_turns > summary.avg_stalled_turns


def test_median_commander_turn_is_an_int_for_an_odd_count():
    """`statistics.median` answers two types, and both are in the digest.

    An odd-length list gives `data[n // 2]` -- the element itself, an **int**
    here, because commander turns are turn numbers. An even one gives the mean
    of the middle pair, a float. The annotation said `float | None` until
    2026-08-22 and mypy never caught it, because `statistics.median`'s stub is
    loose enough to accept the claim.

    Nothing asserted the runtime type until this test, which is how it went
    unnoticed -- and the type matters rather than being pedantry:
    `SimSummary.__repr__` renders it and `tests/test_determinism`'s
    REFERENCE_DIGEST hashes that text, so `4` and `4.0` are different digests.
    Coercing the value to match the old annotation would have been a change to
    Tier 1's output disguised as a type fix. `go/internal/sim/tier1`'s
    `Number` is the same duality, and it reproduces the digest with it.

    The two counts are chosen by *how many games cast the commander at all*,
    not by `games`, so each is asserted rather than assumed.
    """
    lib, cmd = build_golgari(36)
    parities = {}
    for games in range(20, 60):
        summary = run(lib, cmd, games=games, turns=10, seed=3)
        assert summary.median_commander_turn is not None
        cast = round(games * (1 - summary.never_cast_commander))
        parities.setdefault(cast % 2, summary.median_commander_turn)
        if len(parities) == 2:
            break
    assert set(parities) == {0, 1}, (
        "no run reached both parities, so only one branch of `median` was "
        "exercised")
    assert isinstance(parities[1], int) and not isinstance(parities[1], bool), (
        f"an odd count gave {parities[1]!r}, which is not an int")
    assert isinstance(parities[0], float), (
        f"an even count gave {parities[0]!r}, which is not a float")
    # And a deck that never casts its commander reports None rather than 0.
    starved, _ = build_golgari(24)
    empty = run(starved, SimCard(name="Uncastable", cost=parse_mana_cost("{20}")),
                games=20, turns=3, seed=3)
    assert empty.median_commander_turn is None


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
