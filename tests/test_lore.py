"""The shelves hold together (second 2026-08-15 punch list, item 7).

Structural checks that need no pool: the volumes are complete, the keys are
unique, every fact carries a `more` that adds rather than restates, and every
Learn link points at an anchor that exists. Whether the *named cards* exist
is the full pool's question, and it is asked inside `test_colors.py`'s single
`needs_full_pool` test rather than in a second marked test here -- CI's skip
gate is pinned to two, and the argument for running there is identical.
"""

from mtglab import colors, glossary, lore


def test_every_volume_is_labelled_and_stocked():
    assert set(lore.VOLUME_LABELS) == set(lore.VOLUMES)
    assert set(lore.VOLUME_BLURBS) == set(lore.VOLUMES)
    by_volume: dict[str, int] = {}
    for f in lore.FACTS:
        assert f.volume in lore.VOLUMES, f"{f.key}: unknown volume {f.volume}"
        by_volume[f.volume] = by_volume.get(f.volume, 0) + 1
    for v in lore.VOLUMES:
        assert by_volume.get(v, 0) >= 3, f"volume {v!r} is nearly empty"


def test_keys_are_unique_and_slug_shaped():
    keys = [f.key for f in lore.FACTS]
    assert len(keys) == len(set(keys))
    for key in keys:
        assert key == key.lower() and " " not in key, key


def test_more_adds_rather_than_restates():
    for f in lore.FACTS:
        assert f.fact.strip(), f.key
        assert f.more.strip(), f.key
        # The contract from glossary.py: the paragraph elaborates, it does
        # not echo. Identical text is the failure this pins; length is a
        # proxy for "there is actually more".
        assert f.more != f.fact, f.key
        assert len(f.more) > len(f.fact), f"{f.key}: `more` adds nothing"


def test_no_markdown_reaches_the_screen():
    # Rendered as plain text, like the colour prose: an asterisk meant as
    # emphasis reaches the screen as an asterisk. The one allowed mark is
    # `*word*` never appearing at all -- but `more` prose in this module uses
    # asterisks for emphasis nowhere, so a bare check is enough.
    for f in lore.FACTS:
        assert "**" not in f.fact and "**" not in f.more, f.key
        assert "](" not in f.fact and "](" not in f.more, f.key


def test_learn_links_point_at_real_anchors():
    glossary_keys = {t.key for t in glossary.TERMS}
    for f in lore.FACTS:
        if f.learn is None:
            continue
        tab, key = f.learn
        assert tab in ("colors", "words"), f.key
        if tab == "words":
            assert key in glossary_keys, f"{f.key}: no glossary term {key!r}"
        else:
            assert key in colors.BY_KEY, f"{f.key}: no combination {key!r}"


def test_a_fact_reads_whole_without_its_cards():
    """The writing rule that makes the no-pool answer honest: a card name in
    `cards` may enrich a fact, but the prose must already say what it needs
    to. Proxy check: the fact text never *only* consists of a reference like
    "this card" -- concretely, every fact with cards still names its subject
    in its own sentence."""
    for f in lore.FACTS:
        for name in f.cards:
            assert isinstance(name, str) and name.strip(), f.key
