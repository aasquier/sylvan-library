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

import hashlib
import json
import math
import unicodedata
from datetime import date
from pathlib import Path

import pytest
import yaml

import go_fixtures
from mtglab.sim import curve, karsten


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
    ("the scorer", "SCORE_PATH", "render_score_cases"),
    ("the draw corpus", "PYRAND_PATH", "render_pyrand_cases"),
    ("the job registry", "JOBS_PATH", "render_jobs_cases"),
    ("CPython's float floor", "PYFLOAT_PATH", "render_pyfloat_cases"),
    ("the closed form", "KARSTEN_PATH", "render_karsten_cases"),
    ("the mana curve", "CURVE_PATH", "render_curve_cases"),
    ("Tier 1", "TIER1_PATH", "render_tier1_cases"),
    ("the castability cases", "MANA_PATH", "render_mana_cases"),
    ("the compiler", "COMPILE_PATH", "render_compile_cases"),
    ("the mulligan grid", "MULLIGAN_PATH", "render_mulligan_cases"),
    ("the sim cache's key", "CACHE_PATH", "render_cache_cases"),
    ("the stance dial", "STANCE_PATH", "render_stance_cases"),
    ("the seven voices", "PERSONA_PATH", "render_persona_payload"),
    ("the tarot deck", "TAROT_DATA_PATH", "render_tarot_deck"),
    ("the tarot deals", "TAROT_PATH", "render_tarot_deals"),
    ("the Claude ledger", "LEDGER_PATH", "render_ledger_cases"),
    ("the seven tools", "TOOLS_PATH", "render_tools_payload"),
    ("the seven mode definitions", "MODES_PATH", "render_modes_payload"),
    ("the slot argument", "ARGUE_PATH", "render_argue_cases"),
    ("the shared sources", "SOURCES_PATH", "render_sources_cases"),
    ("the commander dossier", "DOSSIER_PATH", "render_dossier_cases"),
    ("research", "RESEARCH_PATH", "render_research_cases"),
    ("the theme interview", "THEME_PATH", "render_theme_cases"),
    ("CPython's case folding", "CASEFOLD_PATH", "render_casefold_payload"),
    ("the camera's reader", "SCAN_PATH", "render_scan_cases"),
    ("the Forge bridge", "FORGE_PATH", "render_forge_cases"),
    ("CPython's whitespace tables", "PYTEXT_PATH", "render_pytext_payload"),
    ("CPython's str() and repr()", "PYSTR_PATH", "render_pystr_cases"),
    # `DIGITS_PATH` is deliberately NOT here: it is the one oracle in this
    # file whose fresh render legitimately differs between the two supported
    # interpreters, so exact equality is the wrong rule. See
    # `test_the_digit_runs_are_the_containers_unicode` below.
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
        "YAML_PATH", "JSON_PATH", "RENDER_PATH", "SCHEMA_PATH", "TINY_POOL_PATH",
        "APP_SCHEMA_PATH", "CRYPTO_PATH",
        # Checked by `test_the_digit_runs_are_the_containers_unicode`, which
        # cannot be an exact-equality row: it is the one oracle whose fresh
        # render legitimately differs between the two supported interpreters.
        "DIGITS_PATH"}
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
    committed = go_fixtures.APP_SCHEMA_PATH.read_text(encoding="utf-8")
    assert committed == fresh, (
        f"{go_fixtures.APP_SCHEMA_PATH} is stale; regenerate with "
        "`python tests/go_fixtures.py`")
    from mtglab.auth import db
    assert f"PRAGMA user_version = {db.SCHEMA_VERSION};" in committed
    assert "CREATE TABLE deck_log" in committed
    # The columns three Go packages had each transcribed by hand, and the one
    # that broke two of them: `model_tier` arrives at rung 10, and a fixture
    # frozen at rung 1 reports `no such column` rather than being obviously
    # eight rungs behind. Named here so a rebuild of `users` that dropped it
    # fails as a sentence rather than as somebody else's test.
    assert "model_tier" in committed
    # Schema only. This file is committed to a public repository, and rule 5
    # is about what a tracked file may contain, not only about what is easy to
    # forget.
    assert "INSERT" not in committed.upper()


def test_the_committed_crypto_oracle_is_what_argon2_writes_now():
    """The migration's sharpest compatibility claim, and its one moving part.

    Phase 4 makes Go a *writer* of password hashes, so a hash it stores has to
    be one `argon2-cffi` verifies for the rest of the file's life -- after a
    rollback to a Python-only door, and for `mtglab users` on the machine. The
    Go test proves that by reproducing each recorded PHC string byte for byte
    from the recorded salt; this holds the record itself to what Python writes
    today, so an argon2-cffi upgrade that changed the encoding fails here with
    the regeneration command rather than in production six weeks later.
    """
    fresh = go_fixtures.render_crypto_cases()
    committed = go_fixtures.CRYPTO_PATH.read_text(encoding="utf-8")
    assert committed == fresh, (
        f"{go_fixtures.CRYPTO_PATH} is stale; regenerate with "
        "`python tests/go_fixtures.py`")


def test_the_crypto_oracle_records_this_builds_own_parameters():
    """A corpus recorded under other parameters would prove the wrong thing.

    `crypto_cases` already asserts every hash verifies and none needs a
    rehash, which is the strong form; this is the cheap readable form, so a
    change to `passwords.py` fails with the parameter's name in the message.
    """
    from mtglab.auth import passwords
    recorded = json.loads(go_fixtures.render_crypto_cases())["argon2id"]
    assert recorded["params"]["memory_cost_kib"] == passwords.MEMORY_COST_KIB
    assert recorded["params"]["time_cost"] == passwords.TIME_COST
    assert recorded["params"]["parallelism"] == passwords.PARALLELISM
    assert recorded["min_password_length"] == passwords.MIN_PASSWORD_LENGTH
    assert recorded["max_password_bytes"] == passwords.MAX_PASSWORD_BYTES
    # The corpus is only as good as its awkward cases: an empty password, one
    # under the storage floor that must still *verify*, and one carrying the
    # `$` and `:` that PHC delimits on.
    passwords_recorded = {c["password"] for c in recorded["cases"]}
    assert "" in passwords_recorded
    assert any(len(p) < passwords.MIN_PASSWORD_LENGTH and p
               for p in passwords_recorded)
    assert any("$" in p for p in passwords_recorded)
    # And a salt whose base64 uses both characters the URL alphabet renames,
    # because that substitution is the mistake that would produce a string
    # Python accepts for some salts and rejects for others.
    salts = "".join(c["salt_b64"] for c in recorded["cases"])
    assert "+" in salts and "/" in salts


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


# ------------------------------------------------------------------ pyrand

def test_the_draw_corpus_covers_every_shape_it_claims_to():
    """A corpus that quietly lost a section still passes every case left in it.

    Each assertion below names a way `random.Random` can be reproduced
    wrongly, and the seeding breadth is the load-bearing half: the key
    `init_by_array` is given grows a 32-bit word at 2**32 and again at 2**64,
    so a corpus of small seeds would prove a port correct for exactly the
    seeds it was shown.
    """
    cases = go_fixtures.pyrand_cases()

    seeds = {int(c["seed"]) for c in cases["seeds"]}
    assert 0 in seeds, "the zero seed takes its own branch (bits == 0)"
    assert any(s < 0 for s in seeds), "a negative seed is what proves the abs()"
    assert any(2**32 < s < 2**64 for s in seeds), "no two-word key"
    assert any(s > 2**64 for s in seeds), "no key past what an int64 can hold"
    for pair in ((-7, 7), (-20260810, 20260810)):
        assert set(pair) <= seeds, f"{pair} must both be present to compare"

    first = cases["seeds"][0]
    assert len(first["words"]) > 624, (
        "the raw stream must cross MT19937's 624-word regeneration boundary, "
        "or a wrong twist is invisible")
    for section in ("words", "randoms", "bits_mixed", "below", "ranges",
                    "shuffles", "repeated_99", "choices"):
        assert first[section], f"the {section} section is empty"

    widths = {c["k"] for c in cases["bits_sweep"]}
    assert widths == set(range(1, 65)), "the getrandbits sweep has a hole"

    # The bounds that make `_randbelow` reject at very different rates: at a
    # power of two it never rejects, one past it rejects almost half the time.
    bounds = {d["n"] for d in first["below"]}
    assert {1, 2, 3, 2**32, 2**32 + 1} <= bounds

    # Every `randrange` form, including a negative step -- the only place the
    # count is a floor division of a negative quotient.
    steps = {c["step"] for c in first["ranges"]}
    assert None in steps and any(s is not None and s < 0 for s in steps)

    # And the division itself, which no *successful* `randrange` can pin.
    assert any(c["want"] != int(c["a"] / c["b"]) for c in cases["floor_div"]), (
        "no floor-division case disagrees with truncation, so the Go test "
        "over them proves nothing")


def test_the_recorded_tier1_stream_is_the_reference_runs_own():
    """The instrument that reads Tier 1's draws off a real run is inert.

    `pyrand_tier1_stream` patches `random.Random` to a recording subclass that
    delegates every draw to CPython, so the run under it should be the same
    run. "Should be" is why this test exists: the recorded digest is checked
    against the pin in `tests/test_determinism.py`, so an instrument that
    consumed a single extra draw would be caught here rather than baked into a
    corpus the Go side then matches perfectly and wrongly.
    """
    from test_determinism import REFERENCE_DIGEST

    tier1 = go_fixtures.pyrand_tier1_stream()
    assert tier1["reference_digest"] == REFERENCE_DIGEST, (
        "recording the reference run changed it; the corpus would pin a "
        "stream Tier 1 does not actually consume")

    assert tier1["generators"], "the reference run built no generators"
    for generator in tier1["generators"]:
        assert generator["draws"] > 0
        assert generator["lengths"], "a generator shuffled nothing"
        assert len(generator["digest"]) == 64
    total = sum(g["draws"] for g in tier1["generators"])
    assert total > 10_000, f"the whole reference run drew only {total} times"


# --------------------------------------------------------- the job registry


def test_the_jobs_oracle_records_what_it_claims_to():
    """A corpus is worth what it catches, and this one has three claims.

    Each names a way `api/jobs.py` can be reproduced wrongly *and plausibly*:
    a percentage rounded the other way at a tie, a timestamp that always
    carries six digits, and a quoting rule taken from the wrong language. Drop
    the cases that turn on them and the Go tests over the rest still pass,
    which is the failure this file exists to stop.
    """
    cases = go_fixtures.jobs_cases()

    # A tie is where Python's round-half-to-even and Go's `math.Round` part
    # company. Without one, the Go rounding test proves only that division
    # works -- asserted the same way `floor_div` is: at least one case where
    # the naive implementation would answer differently.
    ties = [c for c in cases["percent"]
            if c["total"] and 200 * c["done"] % c["total"] == 0
            and 100 * c["done"] % c["total"] != 0]
    assert len(ties) >= 4, (
        "no exact percentage ties in the corpus, so a port that rounded half "
        "away from zero would match every case in it")

    # `isoformat` drops the fraction entirely when the microsecond is zero and
    # keeps six digits otherwise, trailing zeros included.
    fractions = {c["want"].split("+")[0].partition(".")[2]
                 for c in cases["stamps"]}
    assert "" in fractions, "no stamp without a fraction"
    assert "100000" in fractions, "no stamp whose trailing zeros must survive"
    assert all(len(f) in (0, 6) for f in fractions), fractions

    # `repr` prefers single quotes and switches to double only when the string
    # holds a single quote and no double.
    refusals = [c["error"] for c in cases["unknown_lane"]]
    assert any(e.startswith('unknown job lane "') for e in refusals), (
        "no lane forces repr's double-quote branch")
    assert any(e.startswith("unknown job lane '") for e in refusals)

    # Every status a job can be in, and both nested values -- otherwise the
    # payload test is only exercising the empty shapes.
    statuses = {c["job"]["status"] for c in cases["payloads"]}
    assert statuses == {"queued", "running", "done", "error"}, statuses
    assert any(c["job"]["result_json"] for c in cases["payloads"])
    assert any(c["job"]["partial_json"] for c in cases["payloads"])
    assert any(c["job"]["owner"] and c["job"]["key"]
               for c in cases["payloads"]), (
        "no payload case carries an owner and a key, so nothing proves the "
        "two never serialise")


def test_the_jobs_oracle_never_leaks_the_owner_or_the_key():
    """`as_dict` publishes eleven fields and neither of those is among them.

    Checked on Python's own side as well as Go's, because the claim is about
    the payload rather than about either runtime: a caller who can see a job
    already knows whose it is, and one who cannot must not learn it exists
    (ADR 5).
    """
    for case in go_fixtures.job_payload_cases():
        assert set(case["want"]) == {
            "id", "kind", "status", "done", "total", "percent", "partial",
            "label", "result", "error", "created_at"}
        assert "owner" not in case["want_json"]
        key = case["job"]["key"]
        assert not key or key not in case["want_json"]
# ------------------------------------------------------- the closed forms

def test_the_closed_form_corpus_covers_every_deck_it_claims_to():
    """A corpus that quietly lost a deck still passes every case left in it.

    Each fixture deck exists for a property -- a hybrid pip charged to both
    colours, a Phyrexian symbol that must demand nothing, a fetcher that is not
    a rock, a library with nothing in it -- and the `why` beside each is what
    makes dropping one visible rather than merely smaller.
    """
    decks = go_fixtures.closed_form_decks()
    assert set(decks) == {
        "mono-green", "mono-green-rich", "mono-green-poor", "pip-ladder",
        "hybrid-heavy", "naya", "esper-rocks", "commanded", "sixty",
        "all-lands", "no-lands", "tiny", "empty", "tie-breaker"}
    for name, spec in decks.items():
        assert spec["why"].strip(), f"{name} does not say what it is for"

    # And the edges are really edges, not decks that merely have odd names.
    assert decks["empty"]["library"] == []
    assert all(c.is_land for c in decks["all-lands"]["library"])
    assert not any(c.is_land for c in decks["no-lands"]["library"])
    assert decks["commanded"]["commander"] is not None
    naya = decks["naya"]["library"]
    assert any(c.cost.phyrexian for c in naya), "no Phyrexian cost left"
    assert any(c.fetches_lands for c in naya), "no land fetcher left"
    assert any(c.produce_delay for c in naya), "no summoning-sick dork left"
    assert any(len(s.colors) > 1 for c in naya for s in c.produces), "no dual"
    assert any(len(p) > 1 for c in naya for p in c.cost.pips), "no hybrid pip"
    assert any(c.mv > karsten.HORIZON for c in naya), "nothing past the horizon"

    # And the deck that exists only so a rounding tie has somewhere to happen.
    # Asserted as the halves themselves rather than as the deck's contents,
    # because what matters is the property, not the list that produces it.
    tie = decks["tie-breaker"]["library"]
    estimate_scaled = karsten.regression_lands(tie)
    assert estimate_scaled.recommended == 14, (
        "the tie-breaker deck no longer lands on 14.5; a port rounding away "
        "from zero would answer 15 and nothing here would notice")
    piece, generic = curve._typical_accelerant(tie, 4)
    assert not generic and piece == (2, 2, 0), (
        "the tie-breaker deck no longer averages 2.5 / 2.5 / 0.5, so the "
        "three ties in `_typical_accelerant` are no longer exercised")


def test_the_closed_form_corpus_is_wide_enough_to_be_worth_running():
    """Breadth, asserted, because a grid is the easiest thing to quietly trim.

    The numbers are floors rather than exact counts: adding a case must not
    fail a test, and losing a thousand must.
    """
    karsten_cases = go_fixtures.karsten_cases()
    assert len(karsten_cases["hypergeometric"]) >= 2000
    assert len(karsten_cases["exactly"]) >= 2000
    assert len(karsten_cases["required_sources"]) >= 2000
    assert len(karsten_cases["shelves"]) >= 13
    curve_cases = go_fixtures.curve_cases()
    assert len(curve_cases["on_curve_odds"]) >= 3000
    assert len(curve_cases["lands_for_every_drop"]) >= 300
    assert len(curve_cases["curves"]) >= 50

    # Both summation branches of `hypergeometric_at_least`, and both answers of
    # `lands_for_every_drop`. A grid that only ever took one path would prove
    # half of what it looks like it proves.
    assert any(row[4] is None for row in curve_cases["lands_for_every_drop"]), (
        "no row reaches the unreachable case, which is a real answer")
    assert any(row[2] is None for row in curve_cases["on_curve_odds"]), (
        "no row asks the `need=None` question, which is the surface's default")


def test_the_corpus_would_notice_a_sum_that_is_not_an_fsum():
    """The discovery that produced `curve.fsum`, kept as a live assertion.

    `expected_lands_in_play` and `on_curve_odds` summed with the builtin `sum`
    until 2026-08-22, and CPython 3.12 gave `sum()` over floats compensated
    accumulation where 3.11 adds left to right -- so those two lines answered
    differently depending on the interpreter, on a project that supports both
    and ships 3.12. The fix was `math.fsum`, which is correctly rounded and
    therefore the same on every interpreter.

    What this asserts is that the *corpus* can tell the difference. If every
    recorded value happened to agree with a naive running total, the Go side
    could sum however it liked and pass, and the next interpreter change would
    walk straight back in.
    """
    rows = [row for row in go_fixtures.curve_cases()["expected_lands"]
            if row[4] != 0.0]
    assert rows, "the expectation grid recorded nothing but zeroes"

    differs = 0
    for deck_size, lands, turn, on_the_play, value in rows:
        seen = min(curve.cards_seen(turn, on_the_play=on_the_play), deck_size)
        terms = [min(turn, k) * curve._exactly(deck_size, lands, seen, k)
                 for k in range(min(seen, lands) + 1)]
        running = 0.0
        for term in terms:
            running += term
        assert value == math.fsum(terms), (
            f"expected_lands_in_play({deck_size}, {lands}, {turn}, "
            f"{on_the_play}) is not the correctly-rounded sum of its terms")
        if running != value:
            differs += 1
    assert differs, (
        "no row in the grid separates fsum from a running total, so the Go "
        "test over them could not tell the two apart")
# -------------------------------------------------------------------- mana

def test_the_castability_corpus_carries_the_pinned_golden_and_not_a_new_one():
    """Regenerating the case set must never be a way to move the golden.

    `render_mana_cases` refuses to write when the answers it computes hash to
    something other than `CASES_ANSWER_DIGEST`, which is the whole reason this
    corpus is safe to regenerate on a whim. That refusal is the load-bearing
    line in the generator and it is worth an assertion of its own, because its
    failure mode is silent in exactly the wrong direction: a corpus that
    quietly recorded new semantics would then be matched, perfectly, by a Go
    solver reproducing a change nobody decided to make.
    """
    from test_mana_properties import CASES_ANSWER_DIGEST

    committed = json.loads(go_fixtures.MANA_PATH.read_text(encoding="utf-8"))
    assert committed["answers_digest"] == CASES_ANSWER_DIGEST
    assert committed["enumeration_digest"] != committed["answers_digest"], (
        "the two digests are equal, so one of them is not hashing what it says")


def test_the_castability_corpus_is_the_case_set_it_claims():
    cases = go_fixtures.mana_cases()

    assert cases["cases"] == 13_944 == len(cases["costs"]) * len(cases["pools"])
    assert len(cases["answers"]) == len(cases["costs"])
    assert {len(row) for row in cases["answers"]} == {len(cases["pools"])}
    assert set("".join(cases["answers"])) == {"0", "1"}

    # Both answers have to be well represented, or a solver that always
    # refused would match a corpus of refusals and look correct.
    assert 0 < cases["payable"] < cases["cases"]
    assert len(set(cases["answers"])) > 20, "the answer rows are nearly all alike"


def test_the_enumerated_case_set_never_puts_a_wide_pip_before_a_narrow_one():
    """The case set's blind spot, written down where it can be checked.

    Found 2026-08-22, by mutation-testing the Go port of the solver: deleting
    the `seen` reset between pips in Kuhn's algorithm -- one line, and the
    classic way to get that algorithm wrong -- passes **all 13,944 cases**,
    both oracles across all 13,944, and every hand-pinned trap in
    `tests/test_mana.py`. It answers `{W/U}{W} <- [W U]` wrongly, and that
    case cannot appear here: `case_costs` draws pips with
    `itertools.combinations_with_replacement`, whose tuples are non-decreasing
    in alphabet index, and `CASE_PIPS` puts its only hybrid last. So every
    cost in the set has its pips in non-decreasing width order, and a matching
    that is wrong only when a wide pip is offered first is invisible to it.

    Nothing is broken. `mana.py` is correct, Hypothesis covers this on the
    Python side (`test_pip_order_does_not_change_the_answer` states exactly
    the property that fails), and `go/internal/mana/fuzz_test.go` covers it on
    the Go side. What was missing was anybody *saying* so: the enumeration is
    described as the differential case set for a port, and a reader has no way
    to see this hole by looking at it.

    So this test asserts the limit rather than removing it. Widening
    `CASE_PIPS`, or reordering it so a hybrid comes first, would change
    `CASES_ANSWER_DIGEST` -- a deliberate act, argued in a commit message --
    and this failing is how that reader is told the hole has closed and this
    docstring is stale.
    """
    import mana_oracle as oracle

    wide_first = [
        oracle.case_id(cost, ())
        for cost in oracle.case_costs()
        if any(len(later) < len(earlier)
               for i, earlier in enumerate(cost.pips)
               for later in cost.pips[i + 1:])
    ]
    assert not wide_first, (
        f"{len(wide_first)} cost(s) now present a wider pip before a narrower "
        f"one, e.g. {wide_first[0]}. That is a widening of the case set, not a "
        f"bug -- update this test and its docstring, which tell the next "
        f"reader the hole is still open.")

    # And the property that hole hides, asserted here so the claim above is a
    # demonstration rather than an assertion about a mutation nobody can see.
    from mtglab.mana import ManaCost, ManaSource, can_pay

    w, u = frozenset({"W"}), frozenset({"U"})
    hand = (ManaSource(w), ManaSource(u))
    assert can_pay(ManaCost(pips=(frozenset({"W", "U"}), w)), hand)
    assert can_pay(ManaCost(pips=(w, frozenset({"W", "U"}))), hand)


# ------------------------------------------------- Phase 5's engine tail
#
# Three corpora, and claims about them the drift test above cannot make.
# Drift says "this file is what Python answers now"; these say "this file
# still contains the case it was written for", which is the failure a corpus
# has when somebody trims it.


def test_the_compile_oracle_still_loses_cards_to_the_pool():
    """A corpus with no unresolved name proves nothing about the report.

    `CompileReport.unresolved` is the whole reason that type exists -- a
    99-card deck whose pool was missing six cards simulated as a 93-card deck
    with no signal at all -- so a case set where every name resolves would
    leave the Go side asserting an empty list against an empty list.
    """
    decks = {c["name"]: c for c in go_fixtures.compile_deck_cases()}
    half = decks["half-unknown"]["report"]
    assert half["unresolved"], "no case loses a card to the pool any more"
    assert half["simulated_size"] < half["declared_size"]
    assert not half["complete"]
    assert not half["commander_unresolved"], (
        "half-unknown must lose cards and keep its commander, or the two "
        "fields are only ever exercised together")
    # The other half of the pair: a commander that did not resolve, with every
    # other name intact.
    commander = decks["unknown-commander"]["report"]
    assert commander["commander"] is None
    assert commander["commander_unresolved"]
    assert commander["unresolved"] == []
    # A deck with no commander declared is NOT an unresolved commander, which
    # is the distinction a bare `commander is None` check loses.
    none_declared = decks["no-commander"]["report"]
    assert none_declared["commander"] is None
    assert not none_declared["commander_unresolved"]
    # Both refusals, each with the message Go must reproduce -- and the third
    # case, which is the one nobody would have thought to write: a deck where
    # not one name resolves hands the compiler an EMPTY mapping, which reads
    # as "no pool" rather than as "no cards". Wrong word, right behaviour, and
    # pinned in both runtimes so it can only change deliberately.
    assert decks["nothing-resolves"]["error"] == "nothing_to_simulate"
    assert "declares" in decks["nothing-resolves"]["message"]
    assert decks["empty-lookup"]["error"] == "pool_required"
    assert decks["no-pool"]["error"] == "pool_required"


def test_the_compile_oracle_reads_a_real_multi_mana_source():
    """`mana_produced` is the function that made Sol Ring produce one mana.

    The corpus is prose, and prose is easy to trim. This names the answers the
    docstring's argument rests on -- a run summed, alternatives maxed, a
    written-out amount, and the shape that correctly falls through to one --
    so a case set that quietly lost them fails here rather than passing
    smaller.
    """
    texts = {c["label"]: c for c in go_fixtures.compile_text_cases()}
    assert texts["Sol Ring"]["mana_produced"] == 2
    assert texts["Gilded Lotus"]["mana_produced"] == 3
    assert texts["Talisman of Progress"]["mana_produced"] == 1
    assert texts["Wooded Bastion"]["mana_produced"] == 1
    assert texts["Grim Monolith"]["mana_produced"] == 3
    assert texts["Nykthos, Shrine to Nyx"]["mana_produced"] == 1
    # The land shapes `enters_tapped` exists to tell apart.
    assert texts["Temple of Plenty"]["enters_tapped"] is True
    assert texts["Hallowed Fountain"]["enters_tapped"] is False
    assert texts["Rootbound Crag"]["enters_tapped"] is False
    # A fetch that moves two, one that moves one, and one that moves none.
    assert texts["Skyshroud Claim"]["fetches_lands"] == 2
    assert texts["Nature's Lore"]["fetches_lands"] == 1
    assert texts["Sol Ring"]["fetches_lands"] == 0
    # Every `pool` text is a card read out of the real pool; every
    # `constructed` one is labelled as nobody's card.
    assert {c["source"] for c in go_fixtures.compile_text_cases()} == {
        "pool", "constructed"}


def test_the_mulligan_oracle_reaches_the_baseline_fallback():
    """`search` runs a thirty-fourth simulation when the default is not in the grid.

    Nothing in the standard 33 can reach that branch -- the default rule is a
    cell of it -- so without an explicit rule list the fallback is dead code on
    both sides, and a Go port that dropped it would stay green.
    """
    sweeps = {s["label"]: s for s in json.loads(
        go_fixtures.MULLIGAN_PATH.read_text(encoding="utf-8"))["sweeps"]}
    off = sweeps["naya-off-grid"]
    assert off["rules"] is not None
    triples = {(r["min_lands"], r["max_lands"], r["min_mana_pieces"])
               for r in off["rules"]}
    assert (2, 5, 3) not in triples, (
        "the off-grid sweep now contains the default rule, so the baseline "
        "fallback is unreachable again")
    baseline = off["baseline"]
    assert (baseline["min_lands"], baseline["max_lands"],
            baseline["min_pieces"]) == (2, 5, 3)
    # Every other sweep finds its baseline inside the grid, which is the branch
    # that runs 33 simulations rather than 34.
    for label, sweep in sweeps.items():
        if label == "naya-off-grid":
            continue
        assert sweep["rules"] is None
        assert len(sweep["rows"]) == 33


def test_the_mulligan_oracle_keeps_a_flat_sweep_and_a_decided_one():
    """`flat` is measured against the default, never against the spread.

    Goreclaw is the case that found that: a grid spanning 1.62 spells whose
    best rule beats its default by 0.01. The corpus has to hold a sweep of
    each verdict or the Go side proves only one branch of it.
    """
    sweeps = json.loads(
        go_fixtures.MULLIGAN_PATH.read_text(encoding="utf-8"))["sweeps"]
    verdicts = {s["flat"] for s in sweeps}
    assert verdicts == {True, False}, (
        f"every sweep now reports flat={verdicts}; one branch of the verdict "
        f"is untested")
    # A tied grid is where `best` is decided by the grid's ORDER rather than by
    # the numbers, which is what Python's first-wins `max` settles.
    tied = [s for s in sweeps
            if len({r["spells_through_t8"] for r in s["rows"]}) == 1]
    assert tied, "no sweep ties across the whole grid any more"


def test_the_cache_oracle_records_the_engine_it_was_written_under():
    """The fingerprint in the file is this checkout's, and that is the point.

    ADR 18 makes the engine's own source part of the key, so this corpus goes
    stale the moment `engine.py` or `mana.py` moves -- and having to regenerate
    it is the reminder that every stored row in `app.db` just re-keyed. If that
    ever feels like a nuisance, the alternative was a constant somebody
    remembered to bump, which is the thing the ADR rejected.
    """
    from mtglab.sim import cache

    recorded = json.loads(go_fixtures.CACHE_PATH.read_text(encoding="utf-8"))
    assert recorded["python_fingerprint"] == cache.fingerprint()
    assert recorded["engine_sources"] == list(cache._ENGINE_SOURCES)
    assert recorded["sim_version"] == cache.SIM_VERSION
    # Recorded so the Go side checks its own list against Python's rather than
    # against a transcription.
    assert set(recorded["run_inputs"]) == cache.RUN_INPUTS
    assert set(recorded["run_non_inputs"]) == cache.RUN_NON_INPUTS


def test_the_cache_oracle_carries_a_name_json_has_to_escape():
    r"""`ensure_ascii=True` is the encoder difference no Go flag turns on.

    Go writes raw UTF-8 where Python writes `\uXXXX`, and a real library holds
    Bösium Strip and Déjà Vu -- so without a non-ASCII name in the corpus the
    two runtimes would agree on every case here and disagree on a real deck.
    """
    recorded = json.loads(go_fixtures.CACHE_PATH.read_text(encoding="utf-8"))
    payloads = "".join(c["payload"] for c in recorded["cases"])
    assert "\\u00f6" in payloads, "no name escapes to \\uXXXX any more"
    # Above the BMP, which Python writes as a surrogate pair.
    assert "\\ud83d\\ude00" in payloads, "no astral character survives"
    # The short escapes and the numeric form for a control character.
    for escape in ('\\"', "\\\\", "\\t", "\\n", "\\u0007", "\\u007f"):
        assert escape in payloads, f"nothing exercises {escape} any more"
    # A float in `extra`, which renders through `float.__repr__`.
    assert any(c["extra"] and any(isinstance(v, float) for v in c["extra"].values())
               for c in recorded["cases"]), "no `extra` carries a float"
    # And every payload really is what its key hashes.
    for case in recorded["cases"]:
        assert hashlib.sha256(
            case["payload"].encode("utf-8")).hexdigest() == case["key"]


# ------------------------------------------------------- the stance and voices

def test_the_stance_corpus_is_exhaustive_rather_than_sampled():
    """36 stances and all 1,296 clamp pairs, which is the whole space.

    Asserted rather than trusted, because "exhaustive" is a claim that decays
    the moment somebody adds a level to an axis: the generator would keep
    producing a corpus, the Go tests would keep passing, and the new level
    would be covered nowhere. The arithmetic is recomputed from the axes here,
    so widening one fails this test until the corpus is regenerated.
    """
    payload = json.loads(go_fixtures.render_stance_cases())
    expected = 1
    for axis in payload["axes"]:
        expected *= len(payload["levels"][axis])
    assert len(payload["stances"]) == expected
    assert len(payload["clamps"]) == expected * expected


def test_the_stance_corpus_records_the_refusals_and_not_only_the_answers():
    """The refusal text is the half that caught two real porting bugs.

    Python repr-quotes with single quotes where Go's `%q` uses double, and
    Python's json tells `7` from `7.5` by the literal rather than by the value
    -- so `cannot read a stance from int` and `...from float` are two
    sentences that a plain float64 port collapses into one. Neither divergence
    changes a stance; both change what a person reads in a 422. If these rows
    ever stop being recorded, the Go side can drift back with every structural
    test still green.
    """
    payload = json.loads(go_fixtures.render_stance_cases())
    errors = [row["error"] for row in payload["parses"] if "error" in row]
    assert any(e.startswith("'nope' is not a stance preset") for e in errors), \
        "no single-quoted refusal in the corpus"
    assert "cannot read a stance from int" in errors
    assert "cannot read a stance from float" in errors, \
        "int and float must be told apart, or the Go port cannot be checked"
    assert any(e.startswith("['also', 'nope'] are not stance axes")
               for e in errors), "no Python list-repr refusal in the corpus"


def test_the_embedded_roster_never_carries_a_voice():
    """The served half of the persona payload has no `voice`, in the file the
    Go module embeds -- so the guard holds at the data as well as at the type.

    A client that received a prompt would eventually send one back, and "the
    persona is one of a fixed set" is worth keeping structural. Two guards
    rather than one because they fail differently: the Go test catches a
    `RosterEntry` that grows a field, and this catches a generator that starts
    writing one into the roster it renders.
    """
    payload = json.loads(go_fixtures.render_persona_payload())
    assert payload["roster"], "an empty roster would satisfy any leak check"
    for entry in payload["roster"]:
        assert set(entry) == {"key", "label", "blurb", "deals"}, entry
    # And the other direction: the server-side half must still HAVE the voices,
    # or the Go module would build every persona's prompt out of nothing.
    voiced = [p for p in payload["personas"] if p["voice"]]
    assert len(voiced) == len(payload["personas"]) - 1, \
        "every persona but `plain` carries a voice"


def test_the_tarot_corpus_reaches_the_branches_a_sweep_never_would():
    """Four of the deal corpus's seeds were searched for, not swept.

    `describe()` grows two paragraphs that no card field states directly -- the
    Magic-card omen, and a trump landing twice at one table, which the prose
    itself calls the rarest thing this spread can do. A plain range of seeds
    reaches neither reliably and the doubled trump not at all, so those
    branches would be exercised by nothing while the corpus looked thorough.

    Asserted on the rendered payload rather than on the search, because the
    search would happily "succeed" by returning a seed inside the sweep it was
    supposed to reach past.
    """
    payload = json.loads(go_fixtures.render_tarot_deals())
    searched = payload["searched"]
    assert set(searched) == {"crossover", "echo", "reversed", "alignment"}
    prose = {case["seed"]: case["describe"] for case in payload["cases"]}
    for want in searched.values():
        assert want in prose, f"seed {want} was searched for but not recorded"
    assert "The stars have aligned" in prose[searched["alignment"]]
    assert "an omen in its own right" in prose[searched["crossover"]]
    # The doubled trump is genuinely rare: if it ever turns up inside the plain
    # sweep, the search stopped being the thing this test is defending.
    assert searched["alignment"] >= 24, (
        "the alignment seed is inside the swept range; either the deck changed "
        "or the search is no longer reaching past it")


def test_the_tarot_pool_totals_separate_the_two_arithmetics():
    """The corpus can tell `fsum` from a running total before Go is asked to.

    Swapping one for the other changes no spread -- 200,000 seeds deal
    identically, measured -- so the deal cannot test it and the Go mutation
    survives every seeded case. The pool totals are recorded as bits, both
    ways, precisely so the arithmetic can be checked where it is visible. If
    these ever stop differing, the Go test that reads them passes vacuously.
    """
    payload = json.loads(go_fixtures.render_tarot_deals())
    totals = payload["pool_totals"]
    assert len(totals) == 3, "one total per draw in the spread"
    for row in totals:
        assert row["differ"], (
            f"the {row['cards']}-card pool no longer separates fsum from a "
            f"naive sum; the Go fsum test would pass against either")
        assert row["fsum_bits"] != row["naive_bits"]


def test_the_casefold_table_is_the_whole_of_unicode_not_the_differences():
    """The table Go folds with is swept, and a sweep is what makes it exact.

    Recording only the 211 code points where `casefold()` differs from
    `lower()` would leave the other 1,319 resting on Go's `unicode` tables
    agreeing with CPython's -- a claim about two projects' Unicode versions
    rather than one this port gets to make. So the assertion is that the file
    holds *every* non-identity fold, and that the two headline counts it
    reports are really what is in it.
    """
    committed = json.loads(go_fixtures.CASEFOLD_PATH.read_text(encoding="utf-8"))
    assert "unicode_version" not in committed, (
        "the fold table records a Unicode version again. It was removed "
        "deliberately: 3.11 and 3.12 answer the same 1,530 folds and differ "
        "only in that string, so recording it made a stable table look "
        "interpreter-dependent and took the freshness check off one CI leg.")
    folds = {row["cp"]: row["fold"] for row in committed["folds"]}
    assert len(folds) == len(committed["folds"]), "a code point is in there twice"
    assert committed["multi_char"] == sum(1 for f in folds.values() if len(f) > 1)
    assert committed["differ_from_lower"] == sum(
        1 for cp, f in folds.items() if chr(cp).lower() != f)
    # Every fold in the file is real, and every real fold is in the file.
    assert all(chr(cp).casefold() == f for cp, f in folds.items())
    missing = [cp for cp in range(0x110000)
               if chr(cp).casefold() != chr(cp) and cp not in folds]
    assert not missing, f"{len(missing)} code points fold and are not in the table"
    # And the one that made it worth building: ß is not what lower() says.
    assert folds[0xDF] == "ss" and chr(0xDF).lower() == "ß"


def test_the_digit_runs_are_the_containers_unicode():
    """The committed digit table is 3.12's, and an older leg may only lack rows.

    **The one oracle here that is interpreter-dependent, and it was found by
    CI rather than reasoned out.** 3.11 ships Unicode 14 and knows 66 decimal
    digit runs; 3.12 ships 15 and knows 68, having gained the Kawi digits at
    U+11F50 and the Nag Mundari at U+1E4F0. The container runs 3.12, so the
    committed sweep is 3.12's — the same call `math.fsum` forces for the same
    reason.

    A skip on the other leg would be a finding filed as an absence, so the
    rule is a real assertion on both: **equality when the interpreter's
    Unicode matches the file's, a subset when it is older.** Unicode adds
    digit blocks and never removes them, so a superset on any leg means the
    table was regenerated on the wrong interpreter, and a row this
    interpreter knows and the file does not means it is stale.
    """
    committed = json.loads(go_fixtures.DIGITS_PATH.read_text(encoding="utf-8"))
    fresh = go_fixtures.digits_payload()
    if fresh["unicode_version"] == committed["unicode_version"]:
        assert go_fixtures.DIGITS_PATH.read_text(encoding="utf-8") == \
            go_fixtures.render_digits_payload(), (
                f"{go_fixtures.DIGITS_PATH} is stale; regenerate with "
                "`python tests/go_fixtures.py`")
        return
    here, there = set(fresh["zeros"]), set(committed["zeros"])
    assert here < there, (
        f"this interpreter's Unicode ({fresh['unicode_version']}) knows "
        f"{sorted(hex(z) for z in here - there)} which the committed table "
        f"({committed['unicode_version']}) does not — either it is stale, or "
        f"it was regenerated on the wrong interpreter. The container runs "
        f"3.12; regenerate there.")


def test_the_digit_runs_answer_every_unicode_decimal_digit():
    """68 run starts answer all 680 digits, including the adjacent ones.

    The walk-down heuristic Go would otherwise use -- step back while the
    neighbour is still a digit -- is wrong for the 36 mathematical digits from
    U+1D7CE, where four runs of ten sit with no gap between them. This asserts
    the table gets all 680 right, and names those 36 so the reason survives.
    """
    committed = json.loads(go_fixtures.DIGITS_PATH.read_text(encoding="utf-8"))
    zeros = sorted(committed["zeros"])
    assert committed["runs"] == len(zeros) == len(set(zeros))

    def value(cp):
        below = [z for z in zeros if z <= cp]
        return cp - below[-1] if below and cp - below[-1] < 10 else None

    # Every digit *this* interpreter knows. The committed table may know more
    # (it is 3.12's, and 3.11 has two fewer blocks); it may never know fewer,
    # which the test above is what checks.
    digits = [cp for cp in range(0x110000)
              if unicodedata.category(chr(cp)) == "Nd"]
    wrong = [cp for cp in digits if value(cp) != int(chr(cp))]
    assert not wrong, f"{len(wrong)} digits read as the wrong value"
    adjacent = [cp for cp in digits if 0x1D7CE <= cp <= 0x1D7FF]
    assert len(adjacent) == 50 and all(value(cp) == int(chr(cp)) for cp in adjacent)
