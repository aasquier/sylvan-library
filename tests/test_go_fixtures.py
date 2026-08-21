"""The YAML fixture the Go module parses is what Python writes today.

`tests/go_fixtures.py` generates a deck that exercises every shape
`Deck.dump` can produce, plus PyYAML's reading of it as JSON; the Go test in
`go/internal/deckyaml` parses the YAML with goccy and must agree with the
JSON. This file holds the committed pair equal to a fresh render, so a change
to the dumper or to `rich_deck()` fails here with the regeneration command
rather than leaving the Go side proving equivalence with a stale text.

The reference prose travels the same road (Phase 3): `mtglab.reference`
renders the five JSON files the Go module embeds and serves, the same
script writes them, and the tests at the bottom hold the committed files to
a fresh render -- and hold the payloads to the routes they stand in for, so
the JSON cannot say one thing while `/api/colors` says another.
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


# ------------------------------------------------------- the reference prose

import pytest  # noqa: E402

from mtglab import lore, reference, tarotlore  # noqa: E402
from mtglab.decks.model import ARCHETYPES, THEMES  # noqa: E402


@pytest.mark.parametrize("name", reference.FILES)
def test_the_committed_reference_json_is_what_python_renders_now(name):
    """`go/internal/reference/data/<name>` is the Go module's copy of the
    prose, embedded and served; it must be byte-for-byte what the Python
    modules say today, or the two runtimes answer different words."""
    path = go_fixtures.REFERENCE_DIR / name
    assert path.exists(), (
        f"{path} is missing; generate it with `python tests/go_fixtures.py`")
    assert path.read_text(encoding="utf-8") == reference.render()[name], (
        f"{path} is stale; regenerate with `python tests/go_fixtures.py`")


def test_every_reference_file_is_named():
    """`FILES` is the Go side's embed list; a payload written under a name
    not in it would be written and never served."""
    assert set(reference.FILES) == set(reference.payloads())
    assert set(reference.FILES) == set(reference.render())


def test_the_reference_payloads_are_the_routes_payloads():
    """The JSON serves the same routes Python serves today, so each payload
    must be exactly what `api/service.py` renders -- a second copy of the
    taxonomy that agreed with the first only in spirit would be the drift
    this file exists to refuse."""
    from mtglab.api import service
    assert reference.colors_payload() == service.color_taxonomy()
    assert reference.glossary_payload() == service.glossary()
    assert reference.themes_payload() == {"themes": list(THEMES),
                                          "archetypes": list(ARCHETYPES)}


def test_the_lore_payload_carries_names_and_the_routes_prose():
    """`lore_shelves` resolves cards through the pool; the JSON carries the
    names it will resolve and the prose it will render around them, exactly."""
    payload = reference.lore_payload()
    assert [f["key"] for f in payload["facts"]] == [f.key for f in lore.FACTS]
    for rendered, fact in zip(payload["facts"], lore.FACTS, strict=True):
        assert rendered["cards"] == list(fact.cards)
        assert (rendered["fact"], rendered["more"], rendered["volume"]) == \
            (fact.fact, fact.more, fact.volume)
        assert rendered["learn"] == (
            {"tab": fact.learn[0], "key": fact.learn[1]} if fact.learn else None)
    assert [v["key"] for v in payload["volumes"]] == list(lore.VOLUMES)


def test_the_tarot_payload_is_every_fact_deck_tier_first():
    payload = reference.tarotlore_payload()
    assert [f["id"] for f in payload["facts"]] == [f.id for f in tarotlore.ALL]
    assert payload["facts"][0]["card"] == ""          # the deck tier leads
    assert all(f["source"] for f in payload["facts"])  # no fact without one


# ------------------------------------------------------------- the pool

def test_the_committed_schema_is_the_pools_schema():
    """`go/internal/pool/schema.sql` is what `cards/db.py` runs to create a
    pool; the Go tests build theirs from it and the Go `data refresh` will."""
    assert go_fixtures.SCHEMA_PATH.read_text(encoding="utf-8") == \
        go_fixtures.render_schema(), (
            f"{go_fixtures.SCHEMA_PATH} is stale; regenerate with "
            "`python tests/go_fixtures.py`")


def test_the_committed_tiny_pool_is_the_fixture_as_loaded():
    """The 21 cards and their printings, as the rows the loaders insert."""
    assert go_fixtures.TINY_POOL_PATH.read_text(encoding="utf-8") == \
        go_fixtures.render_tiny_pool(), (
            f"{go_fixtures.TINY_POOL_PATH} is stale; regenerate with "
            "`python tests/go_fixtures.py`")
    payload = json.loads(go_fixtures.render_tiny_pool())
    assert len(payload["oracle_columns"]) == len(payload["oracle_cards"][0])
    assert len(payload["printing_columns"]) == len(payload["printings"][0])
    names = {row[payload["oracle_columns"].index("name")]
             for row in payload["oracle_cards"]}
    # The two cards the gate and the reader exist for are in it.
    assert "Primeval Titan" in names
    assert "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger" in names
