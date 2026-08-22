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
from datetime import date
from pathlib import Path

import pytest
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


def test_the_committed_render_oracle_is_what_pyyaml_writes_now():
    """The Go emitter's whole gate is byte equality against this file."""
    fresh = go_fixtures.render_render_cases()
    assert go_fixtures.RENDER_PATH.read_text(encoding="utf-8") == fresh, (
        f"{go_fixtures.RENDER_PATH} is stale; regenerate with "
        "`python tests/go_fixtures.py`")


def test_the_render_oracle_covers_every_group_it_claims_to():
    """A corpus that quietly lost a group still passes every case left in it.

    The groups are the argument for the corpus: each names a way PyYAML's
    emitter can be reproduced wrongly, and dropping one would leave the Go
    side proving less while reporting the same green.
    """
    cases = go_fixtures.render_cases()
    groups = {c["group"] for c in cases}
    assert groups == {
        "resolver-lookalikes", "resolver-near-misses", "indicators",
        "whitespace", "prose", "names", "control", "width-sweep",
        "unicode-width", "int", "list", "bool"}
    # Breadth, not just presence: the sweep is what catches a fold point one
    # column out, and it is the group most likely to be trimmed for size.
    assert sum(c["group"] == "width-sweep" for c in cases) >= 500
    assert len(cases) >= 2000


def test_the_render_oracle_records_both_sides_of_the_resolver():
    """Quoting everything would pass the look-alikes and fail the near misses.

    `why: 'yes'` and `why: 1e3` are both correct and they differ only in what
    PyYAML's resolver says about the value; a corpus holding only the first
    kind would accept a port that never asked.
    """
    rendered = {(c["key"], c["value"], c["fold"]): c["want"]
                for c in go_fixtures.render_cases() if c["kind"] == "str"}
    assert rendered[("why", "yes", False)] == ["      why: 'yes'"]
    assert rendered[("why", "1e3", False)] == ["      why: 1e3"]
    assert rendered[("why", "12", False)] == ["      why: '12'"]
    assert rendered[("why", "12a", False)] == ["      why: 12a"]


# The oracles that had no drift test at all until 2026-08-22 -- `edits.json`
# among them, which is Phase 4's own gate and the largest corpus here. A stale
# one is the worst kind of green: the Go side keeps agreeing with a Python that
# no longer exists, and nothing anywhere says so. Parametrised rather than four
# copies, because the fifth is the one somebody adds next year.
#
# Each is `(name, path, render)`. `render` is called, so a generator that
# stopped being deterministic fails here rather than in a diff.
_ORACLES = [
    ("the edit operations", "EDITS_PATH", "render_edit_cases"),
    ("the whole-file dumps", "DUMPS_PATH", "render_dump_cases"),
    ("the decklist grammar", "DECKLIST_PATH", "render_decklist_cases"),
    ("the importer", "IMPORT_PATH", "render_import_cases"),
    ("the five deliverables", "ARTIFACTS_PATH", "render_artifact_cases"),
]


@pytest.mark.parametrize(("what", "path_name", "render_name"), _ORACLES,
                         ids=[o[0] for o in _ORACLES])
def test_the_committed_oracle_is_what_python_answers_now(what, path_name, render_name):
    path = getattr(go_fixtures, path_name)
    fresh = getattr(go_fixtures, render_name)()
    assert path.read_text(encoding="utf-8") == fresh, (
        f"{path} ({what}) is stale; regenerate with "
        f"`python tests/go_fixtures.py`")


def test_the_artifacts_oracle_does_not_expire_at_midnight():
    """The five deliverables each end in `_Generated <today>_`.

    An oracle that asked the clock would be a fixture that passed all day and
    failed overnight -- the sort of red build that gets rerun rather than read,
    and the sort of green one that means nothing. So `render_artifact_cases`
    pins `generate.date`, and this is the assertion that it did: every
    generated line names the recorded day rather than this one.

    It also checks the pin was *undone*. The freeze is a module attribute
    swapped in place, so a `finally` that ever stopped running would leave
    every later test in the session rendering artifacts dated 2026-08-22.
    """
    from mtglab.artifacts import generate

    committed = json.loads(go_fixtures.ARTIFACTS_PATH.read_text(encoding="utf-8"))
    frozen = go_fixtures.ARTIFACTS_DATE.isoformat()
    assert committed["today"] == frozen
    generated = [line for case in committed["cases"] for file in case.get("files", [])
                 for line in file["text"].splitlines() if line.startswith("_Generated ")]
    assert generated, "no deliverable carries a generated-on line any more"
    assert all(frozen in line for line in generated), (
        "a deliverable is dated something other than the oracle's own day")
    assert generate.date.today() == date.today(), (
        "the frozen date outlived the render that installed it")


def test_every_oracle_this_module_writes_is_checked_for_drift():
    """The list above is complete, and stays complete.

    A corpus with no drift test is a corpus that can rot silently, which is
    what `edits.json` did from the day it was written until this test existed.
    So the check is on the *set*: every `*_PATH` the generator writes is either
    checked by name above or by a test of its own.
    """
    named = {name for _what, name, _render in _ORACLES} | {
        "YAML_PATH", "JSON_PATH", "RENDER_PATH", "SCHEMA_PATH", "TINY_POOL_PATH"}
    writes = {name for name in dir(go_fixtures)
              if name.endswith("_PATH") and isinstance(getattr(go_fixtures, name), Path)}
    assert writes - named == set(), (
        f"these fixtures have no drift test: {sorted(writes - named)}. Add "
        f"them to _ORACLES, or give them one of their own.")


def test_the_committed_log_oracle_is_what_describe_writes_now():
    """The log's sentence is rendered once, at write time (ADR 28)."""
    fresh = go_fixtures.render_log_cases()
    committed = (go_fixtures.LOG_DIR / "describe.json").read_text(encoding="utf-8")
    assert committed == fresh, (
        f"{go_fixtures.LOG_DIR / 'describe.json'} is stale; regenerate with "
        "`python tests/go_fixtures.py`")


def test_the_log_oracle_keeps_a_rationale_in_it():
    """The Go test that proves no `why` reaches a log line needs one to try.

    A corpus that quietly stopped passing a rationale to `describe` would leave
    that assertion passing against nothing, which is the exact shape of a guard
    enforced by nothing.
    """
    cases = go_fixtures.log_cases()
    carrying = [c for c in cases if "why" in c["extra"]]
    assert carrying, "no case passes a `why` to describe; the Go guard is idle"
    for case in carrying:
        assert case["extra"]["why"] not in case["summary"]


def test_the_committed_app_schema_is_what_the_ladder_leaves():
    """Python owns `app.db`'s ladder until Phase 8; the Go tests read it.

    The file is what `sqlite_master` held after `auth/db.py` migrated a fresh
    database, so a new migration moves it here rather than leaving the Go log
    tests inserting into a table that no longer looks like that.
    """
    fresh = go_fixtures.render_app_schema()
    committed = (go_fixtures.LOG_DIR / "app_schema.sql").read_text(encoding="utf-8")
    assert committed == fresh, (
        f"{go_fixtures.LOG_DIR / 'app_schema.sql'} is stale; regenerate with "
        "`python tests/go_fixtures.py`")
    from mtglab.auth import db
    assert f"PRAGMA user_version = {db.SCHEMA_VERSION};" in committed
    assert "CREATE TABLE deck_log" in committed
    # Schema only. This file is committed to a public repository, and rule 5
    # is about what a tracked file may contain, not only about what is easy to
    # forget.
    assert "INSERT" not in committed.upper()


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


# ------------------------------------------------------------- the gate

def test_the_committed_gate_cases_are_pythons_reports_now():
    """`go/internal/gate/testdata/*` is each fixture deck's text and Python's
    own validate report over it; the Go gate is held to them case for case,
    so a change to the gate here must be regenerated there or the Go test
    proves equivalence with a stale answer."""
    rendered = go_fixtures.render_gate_cases()
    for name, body in rendered.items():
        path = go_fixtures.GATE_DIR / name
        assert path.exists(), (
            f"{path} is missing; generate it with `python tests/go_fixtures.py`")
        assert path.read_text(encoding="utf-8") == body, (
            f"{path} is stale; regenerate with `python tests/go_fixtures.py`")
    on_disk = {p.name for p in go_fixtures.GATE_DIR.iterdir()}
    assert on_disk == set(rendered), f"stray gate fixtures: {on_disk - set(rendered)}"


def test_the_gate_cases_exercise_what_they_claim():
    """Breadth is the point: every code the gate can emit should appear in
    at least one case, or the Go gate is proven only against the easy ones."""
    rendered = go_fixtures.render_gate_cases()
    codes = set()
    for name, body in rendered.items():
        if name.endswith(".report.json"):
            for report in json.loads(body).values():
                codes.update(i["code"] for i in report)
    # `companion-restriction` is absent on purpose: `tiny_pool` holds no
    # companion, so the restriction checkers are held by unit tests over
    # synthetic records in `go/internal/gate` rather than by a case here.
    for code in ("banned", "color-identity", "unknown-card", "singleton",
                 "commander-in-99", "not-a-commander", "illegal-pairing",
                 "category-mismatch", "unknown-category", "missing-rationale",
                 "draft-incomplete", "unknown-theme", "legacy-archetype",
                 "deck-status", "deck-size", "not-a-companion", "unverified"):
        assert code in codes, f"no gate case emits {code}"
    assert any(n.endswith(".stats.json") for n in rendered)
    assert "mono-green.suggestions.json" in rendered
