"""The 32 colour combinations.

Two kinds of test here. Most are structural and run everywhere: there are
exactly 32, the keys are canonical, every tier is populated, and `of()` maps a
colour identity to the right slot.

The last one is different. `Combination.verified_by` names a real card whose
Scryfall `color_identity` is supposed to equal the combination's key, and that
test builds a tiny pool to check the claim rather than trusting the table.
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


def test_of_accepts_what_the_pool_actually_returns():
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
    verifies the claim. Building a fake pool out of the table's own numbers
    would prove nothing at all: it would assert that `Selesnya Charm` is {G}{W}
    against data saying `Selesnya Charm` is {G}{W}. That circularity is the
    trap, so the real check needs the real pool and is allowed to skip.
    """
    for c in colors.COMBINATIONS:
        assert c.verified_by.strip(), f"{c.name} cites no card"


@pytest.mark.needs_full_pool
def test_every_named_card_matches_the_real_pool():
    """The claim that `Selesnya Charm` is {G}{W} is checkable. Check it.

    One of the two tests `tiny_pool` cannot carry, and marked rather than
    silently skipped so the gap is countable. The point is to check all 32
    combinations at once -- every guild, shard and wedge -- against cards
    nobody chose for the fixture. Putting those cards in the fixture would
    make the test verify the fixture rather than the table, which is the one
    thing it must not do: this is what would catch a misremembered guild, a
    misspelled card, or a wedge filed as a shard.

    It covers all three lists rather than just `verified_by`, and deliberately
    stays **one** test: a second `needs_full_pool` marker would move CI's
    skip count off the two it is pinned to, and the argument for running here
    is identical for all three. Three rules, and they are not the same rule:

    - `verified_by` and every `signature` card must be **exactly** this
      identity. Signature carries no prose, so that equality is the entire
      claim the list makes, and it is what makes the list worth showing.
    - a `champion` need only be a **subset** -- Alesha is a Mardu warrior on a
      card whose identity is Mardu via two hybrid pips, but a faction is a
      story and the story owes the colour pie no exact match.
    """
    from mtglab import config
    if not config.DB_PATH.exists():
        pytest.skip(f"needs the full pool at {config.DB_PATH}")

    from mtglab.cards import db
    exact = {c.key: [c.verified_by, *c.signature] for c in colors.COMBINATIONS}
    within = {c.key: [ch.card for ch in c.champions]
              for c in colors.COMBINATIONS}
    # The lore shelves ride in the same test rather than adding a second
    # `needs_full_pool` marker (see the docstring above): their claim is
    # weaker -- the named card must simply exist, since the facts assert
    # history rather than identity -- but the argument for running against
    # the real pool is the same, and a misspelled name would otherwise
    # silently drop on every page load.
    from mtglab import lore
    lore_names = sorted({n for f in lore.FACTS for n in f.cards})

    names = sorted({n for v in exact.values() for n in v}
                   | {n for v in within.values() for n in v}
                   | set(lore_names))

    con = db.connect(config.DB_PATH)
    try:
        found = db.get_cards(con, names)
    finally:
        con.close()

    wrong = []
    for name in lore_names:
        if name not in found:
            wrong.append(f"lore: no card named {name!r}")
    for c in colors.COMBINATIONS:
        for name in exact[c.key]:
            rec = found.get(name)
            if rec is None:
                wrong.append(f"{c.name}: no card named {name!r}")
            elif colors.key_for(rec.color_identity) != c.key:
                wrong.append(f"{c.name}: {name} is "
                             f"{colors.key_for(rec.color_identity)}, "
                             f"not exactly {c.key}")
        for name in within[c.key]:
            rec = found.get(name)
            if rec is None:
                wrong.append(f"{c.name}: no champion named {name!r}")
            elif not set(rec.color_identity) <= set(c.colors):
                wrong.append(f"{c.name}: champion {name} is "
                             f"{colors.key_for(rec.color_identity)}, "
                             f"outside {c.key}")
    assert not wrong, "\n".join(wrong)


# ------------------------------------------------------- the teaching depth

#: The tiers that are an actual faction, with characters and a story.
#:
#: Mono-Red is not from anywhere, and neither is Colourless -- the four tiers
#: left out here get signature cards and the pool-derived "most-built
#: commanders" the builder already showed, which is the honest answer where
#: there is no faction to write about.
FACTION_TIERS = ("guild", "shard", "wedge")


def test_exactly_the_factions_have_lore_and_champions():
    """Twenty of the 32, and the other twelve have neither.

    Both directions matter. A faction with no lore is a blank panel on the
    page; a non-faction with lore is a paragraph inventing a story for
    Mono-Blue, which is the failure mode of writing to fill a field.
    """
    factions = {c.key for c in colors.COMBINATIONS if c.tier in FACTION_TIERS}
    assert len(factions) == 20
    for c in colors.COMBINATIONS:
        has = c.key in factions
        assert bool(c.lore) is has, f"{c.name}: lore should be {has}"
        assert bool(c.champions) is has, f"{c.name}: champions should be {has}"


def test_every_combination_has_signature_cards():
    """Including the four-colour slots, where there are barely any cards to
    choose from -- which is the point rather than a gap."""
    for c in colors.COMBINATIONS:
        assert c.signature, f"{c.name} lists no signature card"
        assert len(set(c.signature)) == len(c.signature), \
            f"{c.name} lists a signature card twice"


def test_every_champion_says_who_they_are():
    for c in colors.COMBINATIONS:
        cards = [ch.card for ch in c.champions]
        assert len(set(cards)) == len(cards), f"{c.name} repeats a champion"
        for ch in c.champions:
            assert ch.card.strip(), c.name
            assert len(ch.role.split()) >= 6, f"{c.name}: {ch.card} role is thin"


def test_the_lore_is_a_paragraph_and_not_a_restatement_of_the_tier():
    """`TIER_BLURBS` says what a guild is; `lore` says what happened to this
    one. The failure this guards is the same one that put the definition of a
    shard inside Bant."""
    for c in colors.COMBINATIONS:
        if not c.lore:
            continue
        assert len(c.lore.split()) >= 40, f"{c.name} lore is thin"
        assert "three allied colours" not in c.lore
        assert "two enemies" not in c.lore


def test_no_teaching_prose_uses_markdown():
    """Same rule as the blurbs above, extended to the fields added for the
    teaching pass -- all of it renders as plain text."""
    prose = [c.lore for c in colors.COMBINATIONS]
    prose += [ch.role for c in colors.COMBINATIONS for ch in c.champions]
    for text in prose:
        assert "*" not in text and "_" not in text, text[:60]


def test_every_tier_has_a_label_and_a_blurb():
    """A tier with no blurb is a tier whose definition has to live somewhere,
    and the somewhere it chose was the first combination in it."""
    for tier in colors.TIERS:
        assert colors.TIER_LABELS[tier], tier
        assert colors.TIER_BLURBS[tier], tier
    assert set(colors.TIER_BLURBS) == set(colors.TIERS), "no orphan blurbs"


@pytest.mark.parametrize("key,stolen", [
    ("WUG", "Alara shattered"),      # Bant opened by explaining shards
    ("WBG", "Where a shard is"),     # Abzan opened by explaining wedges
    ("WUBR", "Four-colour identities are"),   # Artifice, the same again
])
def test_a_combination_does_not_explain_its_own_tier(key, stolen):
    """The regression this table was added for.

    Each of these three is the first entry in its tier, and each used to open
    with a paragraph about the tier as a whole -- so the definition of a shard
    was only visible to somebody who happened to land on Bant, and somebody
    who arrowed straight to Naya never met it. `TIER_BLURBS` is where that
    sentence lives now; a combination's own history is about the combination.
    """
    assert stolen not in colors.BY_KEY[key].history


def test_no_reference_prose_uses_markdown():
    """Blurbs, taglines, histories and era stories are rendered as plain text.
    An asterisk meant as emphasis reaches the screen as an asterisk, which is
    what `*enemies*` did in the wedge blurb."""
    prose = [*colors.TIER_BLURBS.values()]
    prose += [e.story for e in colors.ERAS]
    prose += [c.tagline for c in colors.COMBINATIONS]
    prose += [c.history for c in colors.COMBINATIONS]
    for text in prose:
        assert "*" not in text and "_" not in text, text[:60]


# ----------------------------------------------------------- the colour wheel

# The pentagram in `web/src/components/pentagram.tsx` draws five vertices in
# WUBRG order and claims that its perimeter is the allied guilds and its five
# chords the enemy ones. That is a claim about `colors.py`, made in TypeScript,
# where nothing here can see it -- so it is checked from this side instead,
# against the same adjacency rule the component computes its lines from.
#
# The component copies no names and no membership: it looks every label up in
# the taxonomy `/api/colors` already served it. What these pin is the property
# that makes the *shape* honest, which is the one thing a diagram can get wrong
# silently.

def _pairs(step: int) -> list[str]:
    """Keys for the five pairs `step` apart on a WUBRG wheel."""
    w = colors.WUBRG
    return [colors.key_for({w[i], w[(i + step) % 5]}) for i in range(5)]


def test_the_wheels_perimeter_and_chords_are_exactly_the_ten_guilds():
    """Ten lines, ten guilds, nothing left over and nothing drawn twice.

    If this fails the diagram is drawing a line to a combination that is not a
    guild, or two lines to the same one -- either of which looks entirely
    plausible on screen.
    """
    perimeter, chords = _pairs(1), _pairs(2)
    assert not set(perimeter) & set(chords), "a pair cannot be both"
    assert len(set(perimeter + chords)) == 10, "each guild drawn once"
    assert set(perimeter + chords) == {c.key for c in colors.by_tier("guild")}


def test_adjacent_on_the_wheel_is_allied_and_two_apart_is_enemy():
    """The rule the component classifies solid against dashed with.

    Anchored on the shards and wedges rather than asserted: a shard is a colour
    with both its neighbours, so every allied pair sits inside some shard; a
    wedge is a colour with both its opposites, so every enemy pair sits inside
    some wedge. That makes this checkable from the taxonomy alone.
    """
    shards = [set(c.colors) for c in colors.by_tier("shard")]
    wedges = [set(c.colors) for c in colors.by_tier("wedge")]

    for key in _pairs(1):
        pair = set(colors.BY_KEY[key].colors)
        assert any(pair <= s for s in shards), f"{key} is not allied"
    for key in _pairs(2):
        pair = set(colors.BY_KEY[key].colors)
        assert any(pair <= w for w in wedges), f"{key} is not an enemy pair"


def test_the_tier_badges_draw_the_shape_their_blurb_describes():
    """`TIER_SHAPE` in the component picks one member per tier to light up.

    Those index sets are duplicated here deliberately -- they are the badge's
    geometry, and this is the assertion that each one really is a member of the
    tier it stands for. The pair that matters is shard against wedge: same
    three dots, opposite textures, and the badges are only worth drawing if
    that distinction is real.
    """
    w = colors.WUBRG
    shapes = {
        "colorless": [], "mono": [0], "guild": [0, 1], "shard": [4, 0, 1],
        "wedge": [0, 2, 3], "quad": [0, 1, 2, 3], "five": [0, 1, 2, 3, 4],
    }
    assert set(shapes) == set(colors.TIERS), "a badge for every tier"
    for tier, idx in shapes.items():
        combo = colors.of({w[i] for i in idx})
        assert combo.tier == tier, f"{tier} badge draws a {combo.tier}"
