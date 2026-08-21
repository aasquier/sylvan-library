"""The YAML fixture the Go module parses is what Python writes today.

`tests/go_fixtures.py` generates a deck that exercises every shape
`Deck.dump` can produce, plus PyYAML's reading of it as JSON; the Go test in
`go/internal/deckyaml` parses the YAML with goccy and must agree with the
JSON. This file holds the committed pair equal to a fresh render, so a change
to the dumper or to `rich_deck()` fails here with the regeneration command
rather than leaving the Go side proving equivalence with a stale text.
"""

from __future__ import annotations

import json

import yaml

import go_fixtures


def test_the_committed_yaml_is_what_the_dumper_writes_now():
    text, _ = go_fixtures.render()
    assert go_fixtures.YAML_PATH.read_text(encoding="utf-8") == text, (
        f"{go_fixtures.YAML_PATH} is stale; regenerate with "
        "`python tests/go_fixtures.py`")


def test_the_committed_json_is_pyyamls_reading_of_it():
    _, parsed = go_fixtures.render()
    assert go_fixtures.JSON_PATH.read_text(encoding="utf-8") == parsed, (
        f"{go_fixtures.JSON_PATH} is stale; regenerate with "
        "`python tests/go_fixtures.py`")
    # And the JSON is PyYAML's reading of the committed text, not of something
    # else: parse the committed YAML and compare structurally.
    committed = yaml.safe_load(go_fixtures.YAML_PATH.read_text(encoding="utf-8"))
    assert committed == json.loads(parsed)


def test_the_fixture_round_trips_through_the_deck_model():
    """The fixture is a real deck to the model, not only to the parser."""
    from mtglab.decks.model import Deck
    text, _ = go_fixtures.render()
    again = Deck.from_text(text).dump()
    assert again == text


def test_the_fixture_exercises_the_shapes_it_claims_to():
    """The whole point of the fixture is breadth; a refactor that simplified
    it would quietly narrow what the Go side is proven against."""
    text, _ = go_fixtures.render()
    # A single-quoted scalar folded across lines at width 100, with the
    # apostrophe doubled inside it.
    assert "why: 'Two mana on turn one, and it always has been: the single" in text
    assert "\n      -- which is why it is first.'" in text
    assert "It''s the card" in text
    # A plain (unquoted) scalar folded across lines, braces inside it.
    assert "why: cost {1}{W}{W} -- braces again" in text
    assert "\n      fold so both rules fire at once." in text
    # Quoted for an indicator, for a hash, for braces at the start.
    assert "'* starts with a star" in text
    assert "'#not a comment'" in text
    assert "mana_cost: '{1}{W}{W}'" in text
    # Strings that would otherwise read as a bool, a null, an int.
    assert "why: 'yes'" in text and "why: 'null'" in text and "why: '12'" in text
    # ...and one that stays plain because it only starts like a number.
    assert "why: 1.5 mana, in effect" in text
    # A newline inside a quoted scalar is a blank line in the text.
    assert "ship a hand\n\n    with no knight by turn three.'" in text
    assert "Æther" in text and "é" in text               # allow_unicode
    assert "shared: false" in text
    assert "archetype: midrange" in text                 # the legacy key, while unshadowed
