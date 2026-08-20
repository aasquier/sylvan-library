"""The vocabulary.

Structural, and every test here runs everywhere -- there is nothing in this
module that needs a card pool, which is the property the Learn page depends on.

The two worth reading twice are the seam: the client looks its help text up by
key, and a renamed key empties a tooltip in silence -- TypeScript cannot check
a string against a Python table, and neither can Vitest. `SIMULATOR_KEYS` pins
one screen's controls by hand; the derived sweep below reads every key the
client names, so the two together cover both directions of the drift.
"""

from __future__ import annotations

import re
from pathlib import Path

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
    # Tier 1.5, the closed form (the Karsten shelf).
    "tier-1.5", "sim.target", "stat.sources_needed", "stat.card_lag",
    "stat.regression_lands", "stat.policy_gain",
    # The Forge mode (ADR 35).
    "sim.forge_games", "stat.forge_wins", "stat.forge_length",
    "stat.forge_timed_out",
)


@pytest.mark.parametrize("key", SIMULATOR_KEYS)
def test_every_simulator_control_has_help(key):
    assert glossary.get(key) is not None, f"{key} has no glossary entry"


def test_get_returns_none_rather_than_raising():
    """The UI asks for a key and renders nothing when there is no answer; a
    KeyError here would be a blank screen instead of a missing tooltip."""
    assert glossary.get("no-such-term") is None


# --------------------------------------------- the other direction, derived

WEB_SRC = Path(__file__).resolve().parents[1] / "web" / "src"

#: Where a glossary key can appear in the client.
#:
#: Two shapes, because there are two. `<Term name="goldfish">` marks up a word
#: inline; `Simulator.tsx` funnels its seventeen through a one-line `help()`
#: alias, so the key is a bare string by the time it is written down. The
#: prefix pattern is the same rule `test_simulator_terms_are_prefixed_by_what_
#: _they_are` enforces on the table's own side, used here as a way to
#: recognise a key on sight rather than as a second definition of one.
KEY_SITES = (
    re.compile(r"<(?:Term|HelpTip)\b[^>]*?\bname=\{?[\"']([^\"']+)[\"']\}?",
               re.DOTALL),
    re.compile(r"[\"']((?:sim|stat)\.[a-z0-9_]+|tier-\d)[\"']"),
)


def keys_named_in_the_client() -> dict[str, str]:
    """Every glossary key the client names, and the file that names it."""
    found: dict[str, str] = {}
    for path in sorted(WEB_SRC.rglob("*.ts*")):
        if ".test." in path.name:
            continue
        text = path.read_text(encoding="utf-8")
        for pattern in KEY_SITES:
            for key in pattern.findall(text):
                found.setdefault(key, path.name)
    return found


def test_the_sweep_finds_the_keys_it_is_meant_to_check():
    """A sweep that matches nothing passes forever and measures nothing.

    Floored well under the current count rather than pinned to it -- the point
    is to catch the day a refactor moves the call shape and this file starts
    silently checking an empty set, not to fail whenever a tooltip is added.
    """
    assert len(keys_named_in_the_client()) >= 15


def test_every_key_the_client_names_resolves():
    """`web/README.md` states that `Term`/`HelpTip` names must exist in
    `glossary.py`. Until now the only thing enforcing that was `SIMULATOR_KEYS`
    above -- a list kept by hand, covering one screen.

    That is the same shape as the animist recipes (2 of 12 pinned by name until
    2026-08-18) and the no-non-null-assertion rule (prose for months, four
    violations): **a convention this repo states absolutely and enforces with
    nothing will drift.** Derived from the client rather than restated, so a
    tooltip added tomorrow is covered the day it lands.

    One-directional on purpose, and `SIMULATOR_KEYS` stays for the other way
    round. This asks "does every key named resolve"; that asks "is every
    control still offering its help", which no sweep of the client can see --
    a deleted `HelpTip` simply stops being found. `stat.missed_drop` is why
    both are needed: it is a real reported figure with a real entry, rendered
    as a table column that carries no affordance, so it appears in the hand-
    kept list and can never appear here.
    """
    unresolved = {key: where for key, where in keys_named_in_the_client().items()
                  if glossary.get(key) is None}
    assert not unresolved, (
        f"the client names glossary keys that do not exist: {unresolved}. "
        "Each one renders as a word with no affordance, or a control whose "
        "help quietly does not open -- neither of which fails a Vitest run."
    )
