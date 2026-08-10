import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.mana import ManaSource, can_pay, color_identity_of, parse_mana_cost

W, U, B, R, G, C = (frozenset({x}) for x in "WUBRGC")
WU = frozenset({"W", "U"})
GW = frozenset({"G", "W"})
ANY = frozenset({"W", "U", "B", "R", "G"})


def src(colors, amount=1):
    return ManaSource(colors, amount)


# ---------------------------------------------------------------- parsing

def test_parse_basic():
    c = parse_mana_cost("{2}{G}{W}")
    assert c.generic == 2
    assert len(c.pips) == 2
    assert c.mana_value == 4
    assert c.colors == GW


def test_parse_hybrid():
    c = parse_mana_cost("{U/R}")
    assert c.pips == (frozenset({"U", "R"}),)
    assert c.mana_value == 1


def test_parse_phyrexian_is_never_a_mana_constraint():
    # {G/P} can always be paid with life, so it must not gate castability.
    c = parse_mana_cost("{1}{G/P}")
    assert c.pips == ()
    assert c.generic == 1


def test_parse_monocolor_hybrid_is_costed_conservatively():
    # {2/G} is at worst 2 generic; never claim it is cheaper than it may be.
    c = parse_mana_cost("{2/G}")
    assert c.generic == 2 and c.pips == ()


def test_parse_x_and_colorless():
    c = parse_mana_cost("{X}{C}{R}")
    assert c.has_x
    assert frozenset({"C"}) in c.pips
    assert c.mana_value == 2  # X counts as 0


# ------------------------------------------------------------ castability

def test_single_dual_cannot_pay_two_different_pips():
    """The case naive counting gets wrong.

    A W/U dual plus a Forest: 'I have a white source' is true and 'I have a
    blue source' is true and I have 2 total mana -- but the dual cannot be
    tapped twice, so {W}{U} is NOT castable.
    """
    cost = parse_mana_cost("{W}{U}")
    assert can_pay(cost, [src(WU), src(G)]) is False
    assert can_pay(cost, [src(WU), src(U)]) is True


def test_duals_cover_double_pips():
    cost = parse_mana_cost("{G}{G}")
    assert can_pay(cost, [src(GW), src(GW)]) is True
    assert can_pay(cost, [src(GW), src(W)]) is False


def test_generic_paid_by_leftovers():
    cost = parse_mana_cost("{2}{G}")
    assert can_pay(cost, [src(G), src(W), src(U)]) is True
    assert can_pay(cost, [src(G), src(W)]) is False  # only 2 mana for a 3-drop


def test_multi_mana_source_expands():
    # Sol Ring: two colorless. Pays {2} but never a colored pip.
    cost = parse_mana_cost("{2}")
    assert can_pay(cost, [src(C, 2)]) is True
    assert can_pay(parse_mana_cost("{G}"), [src(C, 2)]) is False


def test_colorless_pip_needs_colorless():
    cost = parse_mana_cost("{C}")
    assert can_pay(cost, [src(ANY)]) is False   # "any color" is not colorless
    assert can_pay(cost, [src(C)]) is True


def test_five_color_pip_spread():
    cost = parse_mana_cost("{W}{U}{B}{R}{G}")
    assert can_pay(cost, [src(ANY) for _ in range(5)]) is True
    assert can_pay(cost, [src(ANY) for _ in range(4)]) is False


def test_x_value_consumes_extra_mana():
    cost = parse_mana_cost("{X}{G}")
    assert can_pay(cost, [src(G), src(G), src(G)], x_value=2) is True
    assert can_pay(cost, [src(G), src(G)], x_value=2) is False


# -------------------------------------------------------- color identity

def test_identity_prefers_explicit():
    assert color_identity_of(explicit=["R", "W"]) == frozenset({"R", "W"})


def test_identity_falls_back_to_cost_and_text():
    ident = color_identity_of(mana_cost="{2}{G}", oracle_text="Add {W}.")
    assert ident == frozenset({"G", "W"})


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
