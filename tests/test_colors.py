"""The 32 colour combinations.

Two kinds of test here. Most are structural and run everywhere: there are
exactly 32, the keys are canonical, every tier is populated, and `of()` maps a
colour identity to the right slot.

The last one is different. `Combination.verified_by` names a real card whose
Scryfall `color_identity` is supposed to equal the combination's key, and that
test builds a tiny corpus to check the claim rather than trusting the table.
It is the same discipline as rule 1 applied to data that is not a card: the
guild, shard and wedge names are checkable facts, so they get checked.
"""

from __future__ import annotations

import pytest

from mtglab import colors


def test_there_are_exactly_thirty_two():
    """1 colourless + 5 mono + 10 pairs + 10 three + 5 four + 1 five.

    Not a round number chosen for tidiness -- it is the 32 Deck Challenge, and
    goal 8 treats the challenge and the colour diagrams as one dataset.
    """
    assert len(colors.COMBINATIONS) == 32
    counts = {t: len(colors.by_tier(t)) for t in colors.TIERS}
    assert counts == {"colorless": 1, "mono": 5, "guild": 10, "shard": 5,
                      "wedge": 5, "quad": 5, "five": 1}


def test_every_key_is_unique_and_canonical():
    """One spelling per identity, so set comparisons cannot depend on order."""
    keys = [c.key for c in colors.COMBINATIONS]
    assert len(set(keys)) == 32
    for c in colors.COMBINATIONS:
        assert c.key == colors.key_for(c.colors), c.name


def test_the_thirty_two_are_every_subset_of_wubrg():
    """Exhaustive, not merely 32 things. A missing combination would be a
    challenge slot nobody could ever fill."""
    from itertools import combinations as subsets
    expected = set()
    for n in range(6):
        for combo in subsets(colors.WUBRG, n):
            expected.add(colors.key_for(combo))
    assert {c.key for c in colors.COMBINATIONS} == expected


@pytest.mark.parametrize("identity,name", [
    (frozenset("GW"), "Selesnya"),
    (frozenset("WG"), "Selesnya"),          # order must not matter
    (frozenset(), "Colourless"),
    (frozenset("BRG"), "Jund"),
    (frozenset("WBG"), "Abzan"),
    (frozenset("WUBRG"), "Five-Colour"),
])
def test_of_maps_a_colour_identity_to_its_slot(identity, name):
    assert colors.of(identity).name == name


def test_of_accepts_what_the_corpus_actually_returns():
    """`CardRecord.color_identity` is a frozenset, and a deck's slot in the
    challenge is `of(deck_identity)` -- so this signature is load-bearing."""
    assert colors.of(frozenset({"G", "W"})).key == "WG"
    assert colors.of("WG").key == "WG"
    assert colors.of([]).key == "C"


def test_shards_are_allied_and_wedges_are_enemies():
    """The distinction the carousel is teaching, pinned so it cannot drift.

    A shard is a colour with its two neighbours on the wheel; a wedge is a
    colour with the two it is not adjacent to. Every three-colour identity is
    one or the other, so the two tiers must be disjoint and cover all ten.
    """
    ring = "WUBRG"

    def is_allied(key: str) -> bool:
        # Three consecutive colours on the WUBRG wheel, wrapping.
        return any({ring[i], ring[(i + 1) % 5], ring[(i + 2) % 5]} == set(key)
                   for i in range(5))

    shards = colors.by_tier("shard")
    wedges = colors.by_tier("wedge")
    assert all(is_allied(c.key) for c in shards), "a shard must be allied"
    assert not any(is_allied(c.key) for c in wedges), "a wedge must not be"
    assert len({c.key for c in shards} | {c.key for c in wedges}) == 10


def test_four_colour_combinations_carry_both_naming_conventions():
    """Scryfall's names are primary, EDHREC's Nephilim are aliases. Someone
    arriving from EDHREC has to be able to find the slot they came for."""
    quads = colors.by_tier("quad")
    assert {c.name for c in quads} == {
        "Artifice", "Chaos", "Aggression", "Altruism", "Growth"}
    assert {a for c in quads for a in c.aliases} == {
        "Yore-Tiller", "Glint-Eye", "Dune-Brood", "Ink-Treader", "Witch-Maw"}


def test_every_combination_has_prose_worth_showing():
    for c in colors.COMBINATIONS:
        assert c.tagline.strip(), c.key
        assert len(c.history.split()) >= 25, f"{c.name} history is too thin"


def test_the_five_colours_and_three_eras_are_present():
    assert [c.code for c in colors.COLORS] == list("WUBRG")
    assert {e.name for e in colors.ERAS} == {"Ravnica", "Alara", "Tarkir"}


def test_every_combination_cites_a_card_to_check_it_against():
    """Structural only -- that a citation exists, not that it is right.

    Deliberately separated from the test below, which is the one that actually
    verifies the claim. Building a fake corpus out of the table's own numbers
    would prove nothing at all: it would assert that `Selesnya Charm` is {G}{W}
    against data saying `Selesnya Charm` is {G}{W}. That circularity is the
    trap, so the real check needs the real corpus and is allowed to skip.
    """
    for c in colors.COMBINATIONS:
        assert c.verified_by.strip(), f"{c.name} cites no card"


def test_verified_by_names_match_the_real_corpus():
    """The claim that `Selesnya Charm` is {G}{W} is checkable. Check it.

    Skips without a corpus, which means it does not run in CI -- the suite is
    deliberately database-free (ADR 2). It is still worth having: this is the
    test that would catch a misremembered guild, a misspelled card, or a wedge
    filed as a shard, and it runs on the maintainer's machine where the corpus
    lives. Every one of the 32 was verified this way before the table landed.
    """
    pytest.importorskip("duckdb")
    from mtglab import config
    if not config.DB_PATH.exists():
        pytest.skip(f"no corpus at {config.DB_PATH} -- run `mtglab data refresh`")

    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    try:
        found = db.get_cards(con, [c.verified_by for c in colors.COMBINATIONS])
    finally:
        con.close()

    wrong = []
    for c in colors.COMBINATIONS:
        rec = found.get(c.verified_by)
        if rec is None:
            wrong.append(f"{c.name}: no card named {c.verified_by!r}")
        elif colors.key_for(rec.color_identity) != c.key:
            wrong.append(f"{c.name}: {c.verified_by} is "
                         f"{colors.key_for(rec.color_identity)}, not {c.key}")
    assert not wrong, "\n".join(wrong)
