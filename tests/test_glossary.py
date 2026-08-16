"""The vocabulary.

Structural, and every test here runs everywhere -- there is nothing in this
module that needs a card pool, which is the property the Learn page depends on.

The one test worth reading twice is the last: the simulator's controls look
their help text up by key, so a renamed key would empty a tooltip silently.
Pinning the keys here is what turns that into a failure.
"""

from __future__ import annotations

import pytest

from mtglab import glossary


def test_every_term_is_defined_at_both_lengths():
    """`short` is a tooltip and `long` is a paragraph. A term with only one of
    them is a term that works on exactly one of the two surfaces."""
    for t in glossary.TERMS:
        assert t.term.strip(), t.key
        assert t.short.strip(), t.key
        assert t.long.strip(), t.key
        assert len(t.short.split()) <= 40, f"{t.key}: short is a paragraph"
        assert len(t.long.split()) >= 30, f"{t.key}: long is a tooltip"


def test_keys_are_unique_and_sections_are_known():
    keys = [t.key for t in glossary.TERMS]
    assert len(set(keys)) == len(keys), "a duplicate key shadows a definition"
    for t in glossary.TERMS:
        assert t.section in glossary.SECTIONS, f"{t.key}: {t.section}"
    assert {t.section for t in glossary.TERMS} == set(glossary.SECTIONS), \
        "an empty section renders as a heading with nothing under it"


def test_every_section_has_a_label_and_a_blurb():
    for section in glossary.SECTIONS:
        assert glossary.SECTION_LABELS[section]
        assert glossary.SECTION_BLURBS[section]
    assert set(glossary.SECTION_LABELS) == set(glossary.SECTIONS)
    assert set(glossary.SECTION_BLURBS) == set(glossary.SECTIONS)


def test_see_also_never_points_at_nothing():
    """A cross-reference is rendered as a link. One that resolves to no term
    is a dead link on a page whose whole job is explaining things."""
    dangling = [(t.key, ref) for t in glossary.TERMS for ref in t.see_also
                if ref not in glossary.BY_KEY]
    assert not dangling, dangling


def test_no_term_cross_references_itself():
    for t in glossary.TERMS:
        assert t.key not in t.see_also, t.key


def test_no_markdown_in_the_prose():
    """Rendered as plain text, exactly like `colors.py`'s blurbs -- an
    asterisk meant as emphasis reaches the screen as an asterisk. Braces are
    the one exception: `{G}` is drawn as a mana symbol by `ManaText`."""
    for t in glossary.TERMS:
        for text in (t.short, t.long):
            assert "*" not in text, f"{t.key}: {text[:60]}"
            assert "_" not in text, f"{t.key}: {text[:60]}"


def test_simulator_terms_are_prefixed_by_what_they_are():
    """A parameter you set and a number you are given are different kinds of
    thing to be confused by, and the prefix is how the UI tells them apart."""
    for t in glossary.by_section("simulator"):
        assert t.key.startswith(("sim.", "stat.", "tier-")), t.key


#: Every control and every reported figure on the simulator screen.
#:
#: Duplicated here on purpose. `Simulator.tsx` passes these strings to
#: `useGlossary`, and TypeScript cannot check a string against a Python table,
#: so this is the assertion that the two agree. A missing key is a control
#: whose help quietly does not open.
SIMULATOR_KEYS = (
    "sim.games", "sim.seed", "sim.min_lands", "sim.max_lands",
    "sim.min_pieces", "sim.land_range",
    "stat.mulligan_rate", "stat.median_commander_turn",
    "stat.never_cast_commander", "stat.color_screw_rate",
    "stat.spells_through_t8", "stat.wasted_through_t8",
    "stat.deployment_spread", "stat.argmax_lands",
    # The texture stats (second 2026-08-15 punch list, item 11).
    "stat.median_first_spell", "stat.stalled_turns",
    "stat.missed_drop", "stat.card_timing",
)


@pytest.mark.parametrize("key", SIMULATOR_KEYS)
def test_every_simulator_control_has_help(key):
    assert glossary.get(key) is not None, f"{key} has no glossary entry"


def test_get_returns_none_rather_than_raising():
    """The UI asks for a key and renders nothing when there is no answer; a
    KeyError here would be a blank screen instead of a missing tooltip."""
    assert glossary.get("no-such-term") is None
