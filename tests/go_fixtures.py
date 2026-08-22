"""The fixtures the Go module reads, written by Python -- and the proof they match.

The Go migration's YAML spike (docs/go-migration/PLAN.md section 6) is a
parser-equivalence question: `decks/edit.py` is text surgery and Go never
*serialises* a deck, but it must *parse* one -- to validate, and to check an
edit against a parse-mutate-dump oracle -- exactly as PyYAML does. So Python
writes a deck that exercises every YAML shape `Deck.dump` can produce (folded
long plain scalars at width 100, quoted scalars with `: ` and `#` and braces,
apostrophes, unicode, multi-line notes, nested lists of maps, booleans,
integers, nulls never -- dump omits absent fields) and, beside it, PyYAML's own
reading of that text as canonical JSON. The Go test parses the YAML with
goccy and must produce the same JSON.

`tests/test_go_fixtures.py` holds the committed pair equal to what this
module generates now, so neither side can drift quietly; regenerate with

    python tests/go_fixtures.py

from the repository root after changing `rich_deck()` or the dumper.

**Since Phase 3 this also writes the reference prose** -- the 32 colour
combinations, the glossary, the lore shelves, the tarot deck's facts and the
labelling vocabulary -- as the JSON the Go module embeds and serves
(`go/internal/reference/data/`, rendered by `mtglab.reference`). Same
command, same drift test, same rule: the committed bytes are what Python
renders now, or the suite says so and names this command.

**Since Phase 4 it also writes the render oracle.** `decks/edit.py` writes
its lines through PyYAML's emitter, at a width, with PyYAML's own choices
about quoting and folding -- and `swaps.md` is a diff of the file those lines
land in, so *which* legal YAML gets written is part of an edit's correctness.
`render_cases()` asks `edit._render` for every scalar shape a deck file can
hold (the resolver's look-alikes, every indicator, leading and trailing
whitespace, real deck prose, card names with unicode and apostrophes, a
one-column sweep of the fold width, ints and lists) and records the exact
lines back; `go/internal/pyyaml` must reproduce all of them byte for byte.

**And the pool's shape.** `go/internal/pool/schema.sql` is `cards/db.py`'s
`SCHEMA` verbatim, embedded by the Go module so its tests can build a pool
of their own and so the Go `data refresh` (Phase 8) creates the file Python
creates; and `go/internal/pool/pooltest/testdata/tiny_pool.json` is the 21-card
`tiny_pool` -- `CARDS` and `PRINTINGS` as the rows `load_oracle` and
`load_printings` would insert, column names beside them -- so the Go pool
tests read real cards in CI, where there is no Python and no DuckDB file.
This is the same fixture `tiny_pool.py` already commits, in a second
encoding; it is not a card pool redistribution (ADR 6, rule 5).
"""

from __future__ import annotations

import json
import sys
import tempfile
from pathlib import Path
from typing import Any

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab import reference
from mtglab.decks.decklist import MAX_LINE
from mtglab.decks.model import CardEntry, Deck

ROOT = Path(__file__).resolve().parents[1]
TESTDATA = ROOT / "go" / "internal" / "deckyaml" / "testdata"
YAML_PATH = TESTDATA / "rich-deck.yaml"
JSON_PATH = TESTDATA / "rich-deck.parsed.json"
#: Where the Go module embeds the reference prose from (`go:embed data/*`).
REFERENCE_DIR = ROOT / "go" / "internal" / "reference" / "data"
#: The render oracle: every scalar shape `edit._render` writes, and the bytes
#: PyYAML gives for it (Phase 4).
RENDER_PATH = ROOT / "go" / "internal" / "pyyaml" / "testdata" / "render.json"
#: The edit-equivalence oracle: every operation applied over fixture decks,
#: beside the exact bytes Python's operation yields (Phase 4's gate).
EDITS_PATH = ROOT / "go" / "internal" / "deckedit" / "testdata" / "edits.json"
#: The decklist grammar's oracle: every dialect and every edge, beside the
#: structure Python's parser gives back. `go/internal/decklist` must agree
#: line for line, including which lines it could not read.
DECKLIST_PATH = ROOT / "go" / "internal" / "decklist" / "testdata" / "lists.json"
#: The importer's oracle: a paste, resolved against the 21-card pool, beside
#: the draft `deck.yaml` Python writes for it.
IMPORT_PATH = ROOT / "go" / "internal" / "deckimport" / "testdata" / "imports.json"
#: The whole-file dump oracle: `Deck.dump` over every field combination the
#: deck lifecycle can produce, beside the exact bytes PyYAML writes for it.
#: Reached by `create` and `import` only -- every other write is surgical.
DUMPS_PATH = ROOT / "go" / "internal" / "deck" / "testdata" / "dumps.json"
#: The activity log's oracle: every sentence `log.describe` writes, and
#: `app.db`'s schema as the ladder leaves it, so the Go log tests have a real
#: table to insert into in CI where there is no Python (Phase 4).
LOG_DIR = ROOT / "go" / "internal" / "decklog" / "testdata"
#: The pool's schema and the 21-card fixture, for the Go pool tests.
POOL_DIR = ROOT / "go" / "internal" / "pool"
SCHEMA_PATH = POOL_DIR / "schema.sql"
TINY_POOL_PATH = POOL_DIR / "pooltest" / "testdata" / "tiny_pool.json"


def rich_deck() -> Deck:
    """A deck built to trip a second parser, not to be played."""
    long_why = ("Two mana on turn one, and it always has been: the single most "
                "played card in the format, the first thing every primer tells a "
                "newcomer to find, and the card this line exists to make room for "
                "-- which is why it is first.")
    return Deck(
        slug="rich-fixture",
        name="Rich Fixture: Every Shape the Dumper Writes",
        status="built",
        stage="curated",
        shared=False,
        pilot="Mark's wife",
        commander=["Syr Gwyn, Hero of Ashvale"],
        commander_art="0aae2e33-0000-4000-8000-000000000000",
        companion="Kaheera, the Orphanguard",
        bracket=3,
        legacy_archetype="midrange",
        themes=["knights", "equipment", "voltron"],
        strategy=("Suit up a knight and swing: Syr Gwyn's trigger draws a card and "
                  "the equipment costs nothing, so every turn after the third is "
                  "a cantrip with a sword attached -- panache *and* martial prowess."),
        notes={
            "mulligan": "Keep any seven with two lands and an equipment; ship a hand\n"
                        "with no knight by turn three.",
            "politics": "The table forgets Ashvale until it is too late: say nothing.",
            "weird": "colon: and # hash, plus braces {G}{W} and a trailing space ",
        },
        cards=[
            CardEntry("Sol Ring", "ramp", long_why),
            CardEntry("Embercleave", "equipment",
                      "It's the card with the apostrophe -- and the one that ends "
                      "games: flash, trample, double strike.",
                      scryfall_id="c3a4b2d1-1111-4222-8333-444455556666",
                      tags=["finisher", "flash"]),
            CardEntry("Æther Vial", "utility", "Unicode in the name: Æ — é — ü.",
                      mana_cost="{1}", art="deadbeef-0000-4000-8000-000000000001"),
            CardEntry("Forest", "land", "* starts with a star, which must be quoted",
                      qty=12),
            CardEntry("Plains", "land", "#not a comment", qty=13),
            CardEntry("Lightning Greaves", "equipment", "yes"),
            CardEntry("Null Rod", "hate", "null"),
            CardEntry("Counterspell", "interaction", "12"),
            CardEntry("Swords to Plowshares", "interaction", "1.5 mana, in effect"),
            CardEntry("Knight of the White Orchid", "ramp",
                      "cost {1}{W}{W} -- braces again, inside a longer sentence that "
                      "also runs past the hundred-character fold so both rules fire "
                      "at once.",
                      mana_cost="{1}{W}{W}"),
        ],
        swap_board=[CardEntry("Sword of Feast and Famine", "equipment",
                              "Waiting on a slot.")],
        graveyard=[CardEntry("Gilded Lotus", "ramp", "Entombed: too slow for a "
                             "deck that wants its mana on turn two.")],
    )


def render() -> tuple[str, str]:
    """The YAML text and PyYAML's reading of it as canonical JSON."""
    text = rich_deck().dump()
    parsed = yaml.safe_load(text)
    return text, json.dumps(parsed, sort_keys=True, indent=2,
                            ensure_ascii=False) + "\n"


def render_schema() -> str:
    """`cards/db.py:SCHEMA`, exactly -- the text Python runs to create a pool."""
    from mtglab.cards import db
    return db.SCHEMA.strip("\n") + "\n"


def render_tiny_pool() -> str:
    """`tiny_pool`'s cards and printings as the rows the loaders insert.

    The same filters `load_oracle` and `load_printings` apply (no tokens or
    art-series rows; no digital printings), the same row builders, and the
    column names beside the rows so the Go loader binds by name rather than
    by a position counted in two languages.
    """
    import tiny_pool
    from mtglab.cards import db
    cards = [db._oracle_row(c) for c in tiny_pool.CARDS
             if c.get("layout") not in {"art_series", "token", "double_faced_token"}]
    printings = [db._printing_row(p) for p in tiny_pool.PRINTINGS
                 if not p.get("digital")]
    return json.dumps({
        "oracle_columns": list(db._ORACLE_COLUMNS),
        "oracle_cards": cards,
        "printing_columns": list(db._PRINTING_COLUMNS),
        "printings": printings,
    }, indent=1, ensure_ascii=False) + "\n"


# ------------------------------------------------------------ the gate's cases
#
# The decks the Go gate is held to, case for case: each is written as the
# YAML text `Deck.dump` produces and, beside it, Python's own report over the
# 21-card pool (and, for the first, with no pool at all) -- so
# `go/internal/gate`'s test parses the same text, builds the same pool from
# `pooltest`, and must produce the same issues in the same order with the
# same sentences. The cases are chosen for what they exercise, not for being
# decks anybody would play.

GATE_DIR = ROOT / "go" / "internal" / "gate" / "testdata"


def gate_cases() -> dict[str, Deck]:
    """Name -> deck, each built to trip a different part of the gate."""
    import tiny_pool
    mono = tiny_pool.mono_green_deck()                    # one banned card
    clean = tiny_pool.mono_green_deck(clean=True)          # passes
    draft = tiny_pool.mono_green_deck(stage="draft")
    for c in draft.cards[:4]:
        c.why = ""                                         # the counted warning
    draft.themes = ["stompy", "not-a-theme"]
    draft.legacy_archetype = "midrange"
    # Everything the card-level checks can say, at once: off-identity cards,
    # a not-legal card that is not banned, a second commander that cannot
    # pair, a land filed under ramp and a spell filed under land, a
    # companion with no such ability, a duplicate, a commander in the 99,
    # an unknown category and a card the pool lacks.
    messy = tiny_pool.mono_green_deck(clean=True)
    messy.slug, messy.name = "messy", "Messy Fixture"
    messy.commander = ["Goreclaw, Terror of Qal Sisma", "Gyome, Master Chef"]
    messy.companion = "Sol Ring"
    messy.status, messy.stage = "shelved", "curated"
    messy.cards[0].category = "land"                       # Sol Ring as a land
    messy.cards[1].why = ""                                # curated: blocks
    messy.cards.append(CardEntry("Rhystic Study", "card-advantage", "Blue, in a green deck."))
    messy.cards.append(CardEntry("Swords to Plowshares", "interaction", "White, in a green deck."))
    messy.cards.append(CardEntry("Black Lotus", "ramp", "Not legal, not banned."))
    messy.cards.append(CardEntry("Llanowar Reborn", "ramp", "A land under ramp."))
    messy.cards.append(CardEntry("Goreclaw, Terror of Qal Sisma", "threat", "The commander, again."))
    messy.cards.append(CardEntry("Sol Ring", "utility", "A second Sol Ring."))
    messy.cards.append(CardEntry("Not A Real Card", "mystery", "Unknown to the pool."))
    messy.swap_board.append(CardEntry("Ajani, Nacatl Pariah", "threat", "Red-white, on the board."))
    # Kaheera's restriction against a deck with non-Cats; a Background as a
    # lone commander; and the rich fixture, which the pool mostly lacks.
    kaheera = tiny_pool.mono_green_deck(clean=True)
    kaheera.slug, kaheera.name = "kaheera", "Kaheera Fixture"
    kaheera.companion = "Kaheera, the Orphanguard"
    pair = tiny_pool.mono_green_deck(clean=True)
    pair.slug, pair.name = "pair", "Pair Fixture"
    pair.commander = ["Gyome, Master Chef", "Ajani, Nacatl Pariah"]
    artifact = tiny_pool.mono_green_deck(clean=True)
    artifact.slug, artifact.name = "artifact-commander", "Artifact Commander Fixture"
    artifact.commander = ["Sol Ring"]
    artifact.cards = [c for c in artifact.cards if c.name != "Sol Ring"]
    artifact.cards[-1].qty += 1                             # keep the 99
    return {"mono-green": mono, "mono-green-clean": clean, "draft": draft,
            "messy": messy, "kaheera": kaheera, "pair": pair,
            "artifact-commander": artifact, "rich": rich_deck()}


def render_gate_cases() -> dict[str, str]:
    """name -> the deck text and Python's answers over it: the gate's report
    (`<name>.report.json`, with and without the pool), the analysis
    (`<name>.stats.json`, exactly what `GET .../stats` serves) and, where the
    gate found something a different card would fix, the suggestions
    (`<name>.suggestions.json`, what `GET .../suggestions` serves)."""
    import tiny_pool
    from mtglab.api import service
    from mtglab.cards import db
    from mtglab.decks.source import MemoryDeckSource
    from mtglab.decks.validate import validate
    out: dict[str, str] = {}
    with tempfile.TemporaryDirectory() as tmp:
        # Named as `config.DB_PATH` names it, so `service` finds it through
        # `use_paths(data_dir=tmp)` below and answers *with* the pool.
        pool_path = tiny_pool.build(Path(tmp) / "mtg.duckdb")
        con = db.connect_readonly(pool_path)
        try:
            for name, deck in gate_cases().items():
                text = deck.dump()
                parsed = Deck.from_text(text, slug=deck.slug)
                names = parsed.commander + [c.name for c in parsed.cards] + \
                    [c.name for c in parsed.swap_board] + [c.name for c in parsed.graveyard]
                if parsed.companion:
                    names.append(parsed.companion)
                cards = db.get_cards(con, names)
                reports = {
                    "with_pool": [issue_json(i) for i in validate(parsed, cards).issues],
                    "without_pool": [issue_json(i) for i in validate(parsed, None).issues],
                }
                out[f"{name}.yaml"] = text
                out[f"{name}.report.json"] = json.dumps(reports, indent=1,
                                                        ensure_ascii=False) + "\n"
                # The service's own answers, through the same source the
                # routes use, against the same pool: what the wire carries.
                from mtglab import config
                with config.use_paths(data_dir=Path(tmp)):
                    source = MemoryDeckSource([parsed])
                    stats = service.stats_for(parsed.slug, source=source)
                    out[f"{name}.stats.json"] = json.dumps(
                        stats, indent=1, ensure_ascii=False) + "\n"
                    suggestions = service.suggestions_for(parsed.slug, source=source)
                    if suggestions["targets"]:
                        out[f"{name}.suggestions.json"] = json.dumps(
                            suggestions, indent=1, ensure_ascii=False) + "\n"
        finally:
            con.close()
    return out


def issue_json(issue: Any) -> dict[str, Any]:
    return {"level": issue.level, "code": issue.code, "message": issue.message,
            "card": issue.card}


# --------------------------------------------------------- the render oracle

def render_strings() -> list[tuple[str, str]]:
    """Every string worth asking the emitter about, grouped by what it tests."""
    out: list[tuple[str, str]] = []

    def add(group: str, *values: str) -> None:
        out.extend((group, v) for v in values)

    # A plain rendering of any of these reads back as something that is not a
    # string, so PyYAML quotes each -- YAML 1.1 rules, which is what PyYAML
    # implements and therefore what the port has to implement.
    add("resolver-lookalikes",
        "yes", "Yes", "YES", "no", "No", "NO", "true", "True", "TRUE",
        "false", "False", "FALSE", "on", "On", "ON", "off", "Off", "OFF",
        "null", "Null", "NULL", "~", "", "12", "0", "-3", "+7", "0x1f",
        "0b101", "0o17", "017", "1_000", "1.5", ".5", "-.inf", ".nan",
        "1:30", "1:30:00", "2026-08-21", "2026-8-1", "<<", "=", "!", "&", "*")
    # ... and the near misses, which must stay plain. Both halves matter: a
    # port that quoted everything would pass the list above and fail here.
    add("resolver-near-misses",
        "yess", "nope", "onward", "nullify", "1e3", "1.2.3", "12a", "0x",
        "2026-13-99x", "truthy", "==", "a=b", "- dash", "12:99")

    # Indicators, leading and interior. `#` after a space starts a comment;
    # `:` before one ends a key.
    add("indicators",
        "#not a comment", "* starts with a star", "- leads with a dash",
        "-dash-no-space", "? question", "?nospace", ": colon", ":nospace",
        "| pipe", "> angle", "! bang", "& amp", "@ at", "` tick", "% pct",
        "'quoted'", '"quoted"', "[bracket]", "{brace}", ",comma",
        "--- doc start", "... doc end", "---nospace",
        "colon: in the middle", "colon:nospace", "hash # in the middle",
        "hash#nospace", "trailing colon:", "braces {G}{W} inline")

    # Whitespace decides between the block, single and double styles, and it
    # is the only input that can make a fold lose information.
    add("whitespace",
        " leading space", "trailing space ", "  both  ",
        "a\nb", "a\n\nb", "a \nb", "a\n b", "a\n", "a\n\n",
        "\nleading break", "one\ntwo\nthree", "tab\there", "line\r\nbreak")

    # Real deck prose, at the lengths that make the folder wrap.
    add("prose",
        "Two mana on turn one, and it always has been: the single most played "
        "card in the format, the first thing every primer tells a newcomer to "
        "find, and the card this line exists to make room for -- which is why "
        "it is first.",
        "It's the card with the apostrophe -- and the one that ends games: "
        "flash, trample, double strike.",
        "cost {1}{W}{W} -- braces again, inside a longer sentence that also "
        "runs past the hundred-character fold so both rules fire at once.",
        "Keep any seven with two lands and an equipment; ship a hand\n"
        "with no knight by turn three.",
        "colon: and # hash, plus braces {G}{W} and a trailing space ",
        "The table forgets Ashvale until it is too late: say nothing.",
        "Suit up a knight and swing: Syr Gwyn's trigger draws a card and the "
        "equipment costs nothing, so every turn after the third is a cantrip "
        "with a sword attached -- panache *and* martial prowess.",
        "supercalifragilistic" * 12,
        " ".join(["word"] * 60),
        "a " * 50,
        "one two three " * 9 + "and-a-very-long-unbreakable-token-at-the-end")

    # Card names: where the unicode and the apostrophes live. Every one of
    # these is a real card except the last three.
    add("names",
        "Sol Ring", "\u00c6ther Vial", "Ajani, Nacatl Pariah",
        "Yawgmoth, Thran Physician", "J\u00f6tun Grunt", "Lim-D\u00fbl's Vault",
        "Ach! Hans, Run!", 'Kongming, "Sleeping Dragon"',
        "Question Elemental?", "_____", "Rakdos, Lord of Riots",
        "Look at Me, I'm the DCI", "Hazezon Tamar", "S\u00e9ance",
        "na\u00efve r\u00e9sum\u00e9", "em \u2014 dash", "en \u2013 dash",
        "ellipsis \u2026", "emoji \U0001f600")

    # The characters that force double quotes -- and the two Unicode line
    # separators, which are the port's one documented divergence
    # (`TestTheSeparatorDivergence` in go/internal/pyyaml).
    add("control", "bell\x07here", "nul\x00here", "esc\x1bhere", "del\x7fhere",
        "nbsp\xa0here", "bom\ufeffhere", "\u2028 sep", "\u2029 par",
        "back\\slash", 'double"quote', "single'quote")

    return out


def render_cases() -> list[dict[str, Any]]:
    """Every `_render` call the Go emitter is held to, with PyYAML's answer."""
    from mtglab.decks import edit

    out: list[dict[str, Any]] = []
    seen: set[tuple[Any, ...]] = set()

    def kind_of(value: Any) -> str:
        if isinstance(value, bool):
            return "bool"
        if isinstance(value, int):
            return "int"
        if isinstance(value, list):
            return "list"
        return "str"

    def add(group: str, key: str, value: Any, indent: int, width: int,
            fold: bool) -> None:
        signature = (key, repr(value), indent, width, fold)
        if signature in seen:
            return
        seen.add(signature)
        out.append({
            "group": group, "key": key, "kind": kind_of(value),
            "value": value, "indent": indent, "width": width, "fold": fold,
            "want": edit._render(key, value, indent, width=width, fold=fold),
        })

    # The keys, indents and widths the edit operations actually use: a card's
    # `why` sits at 6, a category at 4, the deck's own fields at 0, and a note
    # at 2 with the narrower prose width.
    shapes = (("why", 6, 96, True), ("why", 6, 96, False),
              ("name", 6, 96, False), ("category", 4, 96, False),
              ("strategy", 0, 96, True),
              ("mulligan", 2, edit._PROSE_WIDTH, True),
              ("pilot", 0, 96, False))
    for group, value in render_strings():
        for key, indent, width, fold in shapes:
            add(group, key, value, indent, width, fold)

    # The fold point swept a column at a time, because being one column out is
    # the failure a handful of examples would never show.
    long_prose = ("Two mana on turn one, and it always has been: the single "
                  "most played card in the format, the first thing every "
                  "primer tells a newcomer to find, and the card this line "
                  "exists to make room for -- which is why it is first.")
    for width in range(20, 121):
        for indent in (0, 2, 4, 6, 8):
            add("width-sweep", "why", long_prose, indent, width, True)
            add("width-sweep", "why", long_prose, indent, width, False)

    # The same sweep over prose whose characters are wider in UTF-8 than they
    # are in columns: a port counting bytes wraps early and only here.
    unicode_prose = ("\u00c6ther \u2014 a rationale with \u00e9 and \u00fc and "
                     "\u2014 em dashes \u2014 written long enough that the folder "
                     "has to choose a break, and every one of those characters is "
                     "more than a byte wide in UTF-8 while being exactly one "
                     "column to Python.")
    for width in range(24, 101, 4):
        add("unicode-width", "why", unicode_prose, 6, width, True)

    # The other two value shapes an edit writes.
    for qty in (1, 2, 12, 13, 99, 100):
        add("int", "qty", qty, 6, 96, False)
    for bracket in range(1, 6):
        add("int", "bracket", bracket, 0, 96, False)
    for themes in ([], ["knights"], ["knights", "equipment", "voltron"],
                   ["tokens", "lifegain"], ["combo"], ["aggro", "midrange"],
                   ["no"], ["yes", "off"]):
        add("list", "themes", themes, 0, 96, False)
        add("list", "themes", themes, 2, 96, False)
    for flag in (True, False):
        add("bool", "shared", flag, 0, 96, False)

    return out


def render_render_cases() -> str:
    return json.dumps(render_cases(), ensure_ascii=False, indent=1) + "\n"


# --------------------------------------------------------- the activity log
#
# ADR 28's sentences. `describe` renders them once, at write time, so the CLI
# and the deck panel cannot become two renderers of the same row in two
# languages -- which means the Go port has to write the same sentence or the
# panel's history changes wording the day a route flips.
#
# The schema travels with them: `app.db`'s DDL as the migration ladder leaves
# it, so `go/internal/decklog`'s tests can build a real database in CI. Python
# owns the ladder until Phase 8; this is a *reading* of what it produced, the
# way `pool/schema.sql` is a reading of `cards/db.py`'s SCHEMA.

def log_cases() -> list[dict[str, Any]]:
    """Every shape `_commit` hands `describe`, and the sentence it gets back."""
    from mtglab.decks import log

    extras: list[dict[str, Any]] = [
        {"added": "Sol Ring", "category": "ramp", "into": "cards"},
        {"added": "Sol Ring", "category": "ramp", "into": "swap_board"},
        {"added": "Sol Ring", "into": "cards"},
        {"added": "Sol Ring"},
        {"entombed": "Primeval Titan"},
        {"entombed": ["Primeval Titan"]},
        {"entombed": ["Primeval Titan", "Sol Ring"]},
        {"entombed": ["a", "b", "c", "d", "e", "f"]},
        {"entombed": ["a", "b", "c", "d", "e", "f", "g"]},
        {"entombed": ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"]},
        {"removed": "Rhystic Study"},
        {"returned": "Gilded Lotus"},
        {"exiled": "Gilded Lotus"},
        {"swapped_out": "Primeval Titan", "swapped_in": "Cultivator Colossus"},
        {"swapped_out": "Primeval Titan"},
        # The rationale rides along and must never reach the sentence.
        {"swapped_out": "Primeval Titan", "swapped_in": "Sol Ring",
         "why": "A rationale that must not appear in any log line."},
        {"note": "mulligan"},
        {"card": "Sol Ring", "field": "why"},
        {"card": "Sol Ring", "field": "qty"},
        {"card": "Sol Ring", "field": "category"},
        {"card": "Sol Ring", "field": "art"},
        {"card": "Sol Ring", "field": ""},
        {"card": "Sol Ring"},
        {"field": "status", "value": "built"},
        {"field": "stage", "value": "curated"},
        {"field": "bracket", "value": 4},
        {"field": "pilot", "value": "Aaron"},
        {"field": "pilot", "value": ""},
        {"field": "pilot", "value": None},
        {"field": "pilot", "value": "   "},
        {"field": "commander_art", "value": "0aae2e33-0000-4000-8000-000000000000"},
        {"field": "commander_art", "value": ""},
        {"field": "art", "value": "0aae2e33-0000-4000-8000-000000000000"},
        {"field": "themes", "value": ["knights", "equipment", "voltron"]},
        {"field": "themes", "value": []},
        {"field": "themes", "value": ("aggro", "midrange")},
        {"field": "newfangled", "value": "something"},
        # Past forty characters the value is not printed, only named.
        {"field": "pilot", "value": "x" * 41},
        {"field": "pilot", "value": "x" * 40},
        # The load-bearing fallback: an operation nobody taught it about.
        {},
        {"unheard_of": "yes"},
    ]
    out = []
    for extra in extras:
        action, summary = log.describe(extra)
        # Tuples are a Python-only shape; the wire and the Go side see a list.
        shown = {k: (list(v) if isinstance(v, tuple) else v)
                 for k, v in extra.items()}
        out.append({"extra": shown, "action": action, "summary": summary})
    return out


def render_log_cases() -> str:
    return json.dumps(log_cases(), indent=1, ensure_ascii=False) + "\n"


def render_app_schema() -> str:
    """`app.db`'s DDL, as the migration ladder leaves it.

    Read out of `sqlite_master` rather than transcribed, so it cannot drift
    from the ladder that made it, and sorted so the file is stable.
    `sqlite_%` names are excluded: `sqlite_sequence` is SQLite's own
    bookkeeping for AUTOINCREMENT and it refuses to let anybody else create it.
    """
    from mtglab import config
    from mtglab.auth import db
    with tempfile.TemporaryDirectory() as tmp, \
            config.use_paths(data_dir=Path(tmp)), db.connection() as con:
        version = con.execute("PRAGMA user_version").fetchone()[0]
        rows = con.execute(
            "SELECT type, name, sql FROM sqlite_master"
            " WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'"
            " ORDER BY type DESC, name").fetchall()
    lines = [
        "-- app.db, as `auth/db.py`'s migration ladder leaves it at schema",
        f"-- version {version}. Written by `python tests/go_fixtures.py`; Python",
        "-- owns the ladder until Phase 8 and this is a reading of what it made,",
        "-- for the Go tests that need a real table in CI. Do not hand-edit.",
        f"PRAGMA user_version = {version};",
    ]
    for row in rows:
        lines.append(f"{row['sql']};")
    return "\n".join(lines) + "\n"


# ------------------------------------------------------- hand-written decks
#
# Every fixture above is what `Deck.dump` writes, and the six real decks are
# not: they were typed by a person, and `edit.py` exists precisely because a
# load-and-dump would reflow them. So the edit oracle also runs over two decks
# written the way a person writes one -- section banners with blank lines
# around them, a trailing comment on a scalar, `[]` for the empty lists, no
# `stage:` key at all, a legacy `archetype:` waiting to be shadowed, and card
# entries at an indent nobody assumed.
#
# This is not decoration. A mutation that made an entry stop owning the blank
# line after it passed every dumped-deck case in the corpus and failed only
# here: `Deck.dump` writes cards back to back, so the rule that keeps
# Goreclaw's `# ---- RAMP 14` banner off the land above it had nothing to act
# on until these existed.

HANDWRITTEN = """\
slug: handwritten
name: Written By Hand
status: built  # the cards are sleeved up
archetype: midrange
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 4
strategy: >-
  Cast the biggest thing that will still resolve, and then cast a bigger one.
  The commander is the discount; everything else is the payoff.

notes:
  mulligan: >-
    Keep any seven with two lands and a way to spend the third turn. Ship a
    hand that cannot cast the commander by turn five.
  pitfalls: >-
    The deck folds to a wrath it cannot rebuild through, so hold one threat
    back once the board is ahead.

cards:

  # ---- RAMP 2
  - name: Sol Ring
    category: ramp
    why: >-
      Two mana on turn one, and it always has been.

  - name: Cultivator Colossus
    category: ramp
    why: >-
      Draws the land drops the curve is built on.

  # ---- THREATS 2
  - name: Regal Behemoth
    category: threat
    why: >-
      A monarch that also doubles the mana.

  - name: Vorinclex, Voice of Hunger
    category: threat
    why: >-
      The mana denial the deck wins through.

  # ---- LANDS 95
  - name: Forest
    category: land
    qty: 95
    why: >-
      Green, and there is nothing else to be.

swap_board: []

graveyard: []
"""

#: The same deck at a wider indent, with a card carrying overrides, so the key
#: indent is read off the entry rather than assumed and `replace_card`'s two
#: dropped keys have something to drop.
#:
#: The dash sits two columns left of the keys, which is the only spacing either
#: editor can write: `_card_lines` re-attaches the dash by hand and lays the
#: rest at the entry's key indent, so a deck typed `-   name:` -- three spaces
#: -- produces invalid YAML from its own editor. Both runtimes catch that in
#: verification and refuse, quoting their own parser as they do, which is the
#: one refusal whose sentence the port cannot match; `TestABrokenParseRefuses`
#: pins the behaviour and the reason.
WIDE = """\
slug: wide
name: Wide Indentation
status: theoretical
stage: draft
commander:
    - Goreclaw, Terror of Qal Sisma
themes:
    - stompy
    - big-mana
cards:
    - name: Sol Ring
      category: ramp
      scryfall_id: c3a4b2d1-1111-4222-8333-444455556666
      mana_cost: "{1}"
      why: >-
        An override on each side, so a swap has both to drop.
    - name: Regal Behemoth
      category: threat
      why: ''
    - name: Forest
      category: land
      qty: 97
      why: >-
        Green, and there is nothing else to be.
swap_board:
    - name: Rhystic Study
      category: card-advantage
      why: >-
        Waiting on a slot it will never get in mono-green.
graveyard:
    - name: Gilded Lotus
      category: ramp
      why: >-
        Entombed: too slow for a deck that wants its mana on turn two.
"""


#: A deck whose keys do *not* sit two columns right of the dash, which is the
#: only shape that proves the key indent is read off the entry rather than
#: computed from it. `_card_lines` re-attaches the dash by hand and lays the
#: rest at the key indent, so the operations that *write* a card entry cannot
#: serve this deck at all -- they build invalid YAML, verification catches it,
#: and both runtimes refuse. The ones that only rewrite a field or remove an
#: entry serve it correctly, and they are what this fixture is here for.
TIGHT = """\
slug: tight
name: Tight Dash
status: built
stage: curated
commander:
  - Goreclaw, Terror of Qal Sisma
cards:
  -  name: Sol Ring
     category: ramp
     why: >-
       Two mana on turn one, and it always has been.
  -  name: Regal Behemoth
     category: threat
     why: >-
       A monarch that also doubles the mana.
  -  name: Forest
     category: land
     qty: 97
     why: >-
       Green, and there is nothing else to be.
"""


def handwritten_decks() -> dict[str, str]:
    """name -> deck text, written the way a person writes one."""
    return {"handwritten": HANDWRITTEN, "wide": WIDE, "tight": TIGHT}


# ------------------------------------------------------- the edit operations
#
# Phase 4's gate, in the plan's own words: "every operation applied by Go over
# fixture decks yields byte-output Python's operation also yields". So this
# runs the nine operations over the same fixture decks the gate uses, records
# the resulting text -- or the refusal, which is half of what an edit
# operation is -- and the Go tests must reproduce both.
#
# Steps chain: each applies to the previous step's output, which is how the
# entomb/return round trip, the second burial, and the emptied-list cases get
# reached at all. A refused step leaves the text where it was, so a refusal in
# the middle of a chain is a real assertion rather than an abandoned case.

def edit_steps(name: str, deck: Deck) -> list[list[dict[str, Any]]]:
    """The chains to run over one fixture deck, as (op, kwargs) lists."""
    cards = [c.name for c in deck.cards]
    first, last = cards[0], cards[-1]
    middle = cards[len(cards) // 2]
    category = deck.cards[0].category
    chains: list[list[dict[str, Any]]] = [
        # Adding: into an existing category, into a new one, onto the swap
        # board (which may be `[]` and have to be reopened), and with a
        # quantity, which is the one key `_card_lines` writes conditionally.
        [{"op": "add_card", "name": "Llanowar Reborn", "category": category,
          "why": "Filed beside the cards it belongs with."},
         {"op": "add_card", "name": "Rhystic Study", "category": "card-advantage",
          "why": "A category this deck did not have, so it lands at the end."},
         {"op": "add_card", "name": "Black Lotus", "category": "ramp",
          "why": "Onto the board, not into the 99.", "list_key": "swap_board"},
         {"op": "add_card", "name": "Snow-Covered Forest", "category": "land",
          "why": "Several of them.", "qty": 4}],
        # ... and every way adding is refused.
        [{"op": "add_card", "name": first, "category": category, "why": "Twice."},
         {"op": "add_card", "name": deck.commander[0], "category": "threat",
          "why": "The commander is outside the 99."},
         {"op": "add_card", "name": "", "category": "ramp", "why": "No name."},
         {"op": "add_card", "name": "Mox Emerald", "category": "", "why": "No category."},
         {"op": "add_card", "name": "Mox Jet", "category": "ramp", "why": "", },
         {"op": "add_card", "name": "Mox Ruby", "category": "ramp", "why": "Zero.",
          "qty": 0},
         {"op": "add_card", "name": "Mox Pearl", "category": "ramp", "why": "Elsewhere.",
          "list_key": "sideboard"}],
        # Removing: the first entry, a middle one, the last one -- the three
        # places the blank-line rule reads differently -- and a name that is
        # not there.
        [{"op": "remove_card", "name": first},
         {"op": "remove_card", "name": last},
         {"op": "remove_card", "name": middle},
         {"op": "remove_card", "name": "Not In This Deck"}],
        # The graveyard round trip: entomb, return, entomb twice, exile.
        [{"op": "entomb_card", "name": first},
         {"op": "return_card", "name": first},
         {"op": "entomb_card", "name": first},
         {"op": "entomb_card", "name": last},
         {"op": "exile_card", "name": first},
         {"op": "return_card", "name": last},
         {"op": "return_card", "name": last},
         {"op": "exile_card", "name": last}],
        # A card's own fields, including the two that drop a key.
        [{"op": "set_card_field", "name": first, "field": "category", "value": "utility"},
         {"op": "set_card_field", "name": first, "field": "qty", "value": 3},
         {"op": "set_card_field", "name": first, "field": "qty", "value": 1},
         {"op": "set_card_field", "name": first, "field": "why",
          "value": "A rewritten rationale, long enough that PyYAML has to fold "
                   "it across more than one line and pick its own quoting."},
         {"op": "set_card_field", "name": first, "field": "why", "value": "yes"},
         {"op": "set_card_field", "name": first, "field": "art",
          "value": "0aae2e33-0000-4000-8000-000000000000"},
         {"op": "set_card_field", "name": first, "field": "art", "value": ""},
         {"op": "set_card_field", "name": first, "field": "art", "value": "not-a-uuid"},
         {"op": "set_card_field", "name": first, "field": "qty", "value": 0},
         {"op": "set_card_field", "name": first, "field": "name", "value": "Nope"},
         {"op": "set_card_field", "name": first, "field": "category", "value": "  "}],
        # A swap: the operation the whole module was written for.
        [{"op": "replace_card", "old_name": first, "new_name": "Mana Crypt",
          "why": "Faster, and the damage is a cost this deck can pay."},
         {"op": "replace_card", "old_name": "Mana Crypt", "new_name": "Mana Vault",
          "why": "Moved as well as swapped.", "category": "utility"},
         {"op": "replace_card", "old_name": "Mana Vault", "new_name": "Sol Ring",
          "why": "   "},
         {"op": "replace_card", "old_name": "Not Here", "new_name": "Sol Ring",
          "why": "Missing."}],
        # The deck's own fields.
        [{"op": "set_deck_field", "field": "status", "value": "built"},
         {"op": "set_deck_field", "field": "bracket", "value": 4},
         {"op": "set_deck_field", "field": "pilot", "value": "Aaron"},
         {"op": "set_deck_field", "field": "pilot", "value": ""},
         {"op": "set_deck_field", "field": "commander_art",
          "value": "0aae2e33-0000-4000-8000-000000000000"},
         {"op": "set_deck_field", "field": "themes", "value": ["stompy", "ramp"]},
         {"op": "set_deck_field", "field": "themes", "value": "stompy, midrange, stompy"},
         {"op": "set_deck_field", "field": "themes", "value": []},
         {"op": "set_deck_field", "field": "stage", "value": "draft"},
         {"op": "set_deck_field", "field": "stage", "value": "curated"}],
        # ... and every way a deck field is refused.
        [{"op": "set_deck_field", "field": "archetype", "value": "combo"},
         {"op": "set_deck_field", "field": "commander", "value": "Somebody Else"},
         {"op": "set_deck_field", "field": "bracket", "value": 9},
         {"op": "set_deck_field", "field": "bracket", "value": "four"},
         {"op": "set_deck_field", "field": "status", "value": "shelved"},
         {"op": "set_deck_field", "field": "pilot", "value": "x" * 41},
         {"op": "set_deck_field", "field": "commander_art", "value": "nope"},
         {"op": "set_deck_field", "field": "themes", "value": ["not-a-theme"]}],
        # Notes, which are the only folded write and the only block the
        # operation may have to create.
        [{"op": "set_note", "key": "mulligan",
          "value": "Keep any seven with two lands and something to do with them; "
                   "ship a hand that cannot cast the commander by turn five."},
         {"op": "set_note", "key": "mulligan", "value": "Shorter now."},
         {"op": "set_note", "key": "politics",
          "value": "Say nothing about the commander until it is already resolved."},
         {"op": "set_note", "key": "weird",
          "value": "colon: and # hash, plus braces {G}{W}"},
         {"op": "set_note", "key": "", "value": "No key."},
         {"op": "set_note", "key": "not a key", "value": "Spaces."},
         {"op": "set_note", "key": "empty", "value": "   "}],
        # Sharing, the tenth operation (2026-08-22): insert, no-op on a value
        # already written, removal, and no-op on a key that is not there.
        # Every one of these was a whole-file rewrite until the day this chain
        # was added, which is what the hand-written decks in this corpus are
        # for -- a reflow passes on a dumped deck and shows up only here.
        [{"op": "set_shared", "shared": False},
         {"op": "set_shared", "shared": False},
         {"op": "set_shared", "shared": True},
         {"op": "set_shared", "shared": True}],
    ]
    if deck.swap_board:
        chains.append([
            {"op": "entomb_card", "name": deck.swap_board[0].name},
            {"op": "remove_card", "name": deck.swap_board[0].name},
            {"op": "add_card", "name": deck.swap_board[0].name,
             "category": "equipment", "why": "Back on the board.",
             "list_key": "swap_board"}])
    if deck.graveyard:
        buried = deck.graveyard[0].name
        chains.append([
            {"op": "return_card", "name": buried},
            {"op": "entomb_card", "name": buried},
            {"op": "exile_card", "name": buried},
            {"op": "exile_card", "name": buried}])
    return chains


# ----------------------------------------------------- the decklist oracle
#
# The parser is pure text in, structure out, which is what makes an exhaustive
# corpus cheap: no pool, no filesystem, no database. What it is really pinning
# is the three places Go's regexp is not Python's `re` -- `\s` (Python's
# includes U+00A0 and the separators), `splitlines()` (Python breaks on eleven
# characters), and `\d` (Python matches any Unicode decimal digit and `int()`
# reads it). Each of those arrives in a real paste: a non-breaking space from a
# web page, a lone `\r` from an old export, a U+2028 from a browser.

def decklist_cases() -> dict[str, str]:
    """Every dialect, and every edge the grammar has an opinion about."""
    return {
        "moxfield": "1 Arahbo, Roar of the World (C17) 27 *CMDR*\n"
                    "1 Kaheera, the Orphanguard (IKO) 197\n"
                    "1 Sol Ring (LTC) 284\n"
                    "1 Branchloft Pathway // Boulderloft Pathway (ZNR) 258\n"
                    "36 Forest (UNF) 235 *F*\n",
        "archidekt": "1x Atla Palani, Nest Tender (M20) 191 [Commander{top}]\n"
                     "1x Sol Ring (C21) 263 [Ramp]\n"
                     "1x Cultivate (M21) 177 [Ramp{noDeck}]\n",
        "arena": "Deck\n1 Gyome, Master Chef (CLB) 265\n30 Swamp (UNF) 239\n"
                 "\nSideboard\n1 Reliquary Tower (M19) 254\n",
        "tappedout": "Creatures (2)\n1x Goreclaw, Terror of Qal Sisma\n"
                     "1x Craterhoof Behemoth\n\nLands (1)\n1x Ancient Tomb\n",
        "deckstats": "//Commander\n1 [C17] Arahbo, Roar of the World\n"
                     "//Creatures\n1 [M20] Craterhoof Behemoth\n"
                     "# a real comment\n",
        "headers": "COMMANDER (1)\n1 Gyome, Master Chef\nCommander:\n"
                   "Commander: (1)\nCommander (1):\nMaybeboard\n1 Sol Ring\n"
                   "Tokens\n1 Beast\nDeck--\nOther\n1 Skullclamp\n",
        "quantities": "1 Sol Ring\n1x Sol Ring\n1 x Sol Ring\n1X Sol Ring\n"
                      "36 Forest\n999 Forest\n1996 World Champion\n"
                      "3 Steps Ahead\n1xSol Ring\n12\n0 Sol Ring\n",
        "printings": "1 Sol Ring (2X2) 297 *F*\n1 Sol Ring (c21)\n"
                     "1 Erase (Not the Urza's Legacy One)\n"
                     "1 B.F.M. (Big Furry Monster)\n1 Ratchet Bomb (WAR) 25\u2605\n"
                     "1 Sol Ring (NEO) 123s\n",
        "unreadable": "(LTC) 284\n*CMDR*\n[Ramp]\n1 \n   \n1 [C17]\n",
        "the-bound": "1 " + "A" * (MAX_LINE - 2) + "\n1 " + "A" * MAX_LINE + "\n",
        # The three divergences, each in the form a paste actually delivers.
        "unicode-space": "1\u00a0Sol Ring\n1 Sol\u00a0Ring\n\u00a0\n"
                         "1 Sol Ring\u00a0\n1 Sol Ring (LTC)\u00a0284\n",
        "line-breaks": "1 Sol Ring\r1 Mana Crypt\r\n1 Mana Vault\u2028"
                       "1 Black Lotus\x0b1 Mox Jet\x0c1 Mox Pearl\x851 Mox Ruby",
        "unicode-digits": "\u0663 Forest\n\u0669\u0669 Island\n"
                          "\uff11 Sol Ring\n",
        "empty": "",
        "blank": "\n\n   \n\t\n",
    }


def render_decklist_cases() -> str:
    """Each paste above, as Python's parser reads it."""
    from mtglab.decks.decklist import parse

    out: dict[str, Any] = {}
    for name, text in decklist_cases().items():
        parsed = parse(text)
        out[name] = {
            "text": text,
            "cards": [{"name": c.name, "qty": c.qty, "section": c.section,
                       "line_no": c.line_no} for c in parsed.cards],
            "unreadable": [{"line_no": n, "text": s} for n, s in parsed.unreadable],
            "skipped": [{"line_no": n, "text": s} for n, s in parsed.skipped],
            "commander": parsed.commander,
            "companion": parsed.companion,
        }
    return json.dumps(out, indent=1, ensure_ascii=False) + "\n"


# ----------------------------------------------------- the importer's oracle
#
# One layer above the grammar: what the lines *mean* once the pool has been
# asked. Run over `tiny_pool`, which is a real 21-card DuckDB on both sides, so
# the two runtimes resolve the same names against the same records -- including
# the two double-faced cards, which is where "the name as the pool spells it"
# has an opinion.
#
# The header carries the day it was written, so the recorded YAML has that date
# replaced by the literal `DATE` and the Go test does the same to its own
# output before comparing. A Go side that formatted the date differently would
# fail to substitute and the comparison would say so, which is the point.

def import_cases() -> list[dict[str, Any]]:
    """Every shape `build_deck` has an opinion about."""
    return [
        {"name": "explicit-commander", "slug": "explicit",
         "commander": ["Goreclaw, Terror of Qal Sisma"], "bracket": 4,
         "text": "1 Sol Ring (LTC) 284\n1 Cultivator Colossus\n38 Forest\n"
                 "1 Rhystic Study\n"},
        # The commander marked inline, Moxfield's plain export.
        {"name": "inline-commander", "slug": "inline", "commander": [],
         "text": "1 Goreclaw, Terror of Qal Sisma (M19) 176 *CMDR*\n"
                 "1 Sol Ring (LTC) 284\n30 Forest\n"},
        # A commander the list nominated and the caller overrode: the demoted
        # card goes into the 99 rather than vanishing.
        {"name": "demoted", "slug": "demoted",
         "commander": ["Gyome, Master Chef"],
         "text": "Commander\n1 Goreclaw, Terror of Qal Sisma\n"
                 "Deck\n1 Sol Ring\n20 Swamp\n"},
        # The commander sitting in the sideboard, which is where our own
        # moxfield.txt artifact puts it.
        {"name": "lifted-from-the-board", "slug": "lifted",
         "commander": ["Goreclaw, Terror of Qal Sisma"],
         "text": "Deck\n1 Sol Ring\n30 Forest\n"
                 "Sideboard\n1 Goreclaw, Terror of Qal Sisma\n1 Black Lotus\n"},
        # Casing corrected, a double-faced card written by its front face, and
        # a name the pool has never heard of -- kept, not dropped.
        {"name": "resolution", "slug": "resolution",
         "commander": ["goreclaw, terror of qal sisma"],
         "text": "1 sol ring\n1 Ajani, Nacatl Pariah\n"
                 "1 Etali, Primal Conqueror // Etali, Primal Sickness\n"
                 "1 Not A Real Card\n1 llanowar reborn\n"},
        # The same name on more than one line.
        {"name": "merged", "slug": "merged",
         "commander": ["Gyome, Master Chef"],
         "text": "1 Sol Ring\n2 Sol Ring\n10 Swamp\n20 Swamp\n"},
        # A companion, given explicitly and taken from the list.
        {"name": "companion", "slug": "companion",
         "commander": ["Goreclaw, Terror of Qal Sisma"],
         "companion": "Black Lotus",
         "text": "1 Sol Ring\n10 Forest\n"},
        # Every refusal.
        {"name": "no-commander", "slug": "nobody", "commander": [],
         "text": "1 Sol Ring\n10 Forest\n"},
        {"name": "no-commander-with-a-board", "slug": "nobody2", "commander": [],
         "text": "Deck\n1 Sol Ring\nSideboard\n1 Goreclaw, Terror of Qal Sisma\n"
                 "1 Gyome, Master Chef\n"},
        {"name": "three-commanders", "slug": "three",
         "commander": ["Goreclaw, Terror of Qal Sisma", "Gyome, Master Chef",
                       "Regal Behemoth"],
         "text": "1 Sol Ring\n"},
        # The empty paste: legal here, and refused one layer up in the service.
        {"name": "no-cards", "slug": "bare",
         "commander": ["Gyome, Master Chef"], "text": ""},
        # Unreadable and skipped lines ride into the report unchanged.
        {"name": "reported-lines", "slug": "reported",
         "commander": ["Gyome, Master Chef"],
         "text": "1 Sol Ring\nTokens\n1 Beast\n(LTC) 284\n*CMDR*\n"},
    ]


def render_import_cases() -> str:
    """Each paste above, resolved against the 21-card pool by Python."""
    import datetime

    from mtglab.cards import db
    from mtglab.decks import decklist, importer

    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import tiny_pool

    today = datetime.date.today().isoformat()
    out: list[dict[str, Any]] = []
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "tiny.duckdb"
        tiny_pool.build(path)
        con = db.connect(path)
        try:
            for case in import_cases():
                parsed = decklist.parse(case["text"])
                commander = case.get("commander") or []
                companion = case.get("companion") or ""
                cards = db.get_cards(con, importer.names_in(
                    parsed, commander=commander, companion=companion or None))
                record: dict[str, Any] = {
                    "name": case["name"], "slug": case["slug"],
                    "text": case["text"], "commander": commander,
                    "companion": companion, "bracket": case.get("bracket"),
                    "deck_name": case.get("deck_name", ""),
                    "status": case.get("status", "theoretical"),
                }
                try:
                    report = importer.build_deck(
                        parsed, cards, slug=case["slug"],
                        name=record["deck_name"] or None, commander=commander,
                        companion=companion or None, bracket=case.get("bracket"),
                        status=record["status"])
                except importer.ImportRefused as exc:
                    record["refused"] = str(exc)
                    out.append(record)
                    continue
                record["yaml"] = report.yaml_text.replace(today, "DATE", 1)
                record["unknown"] = report.unknown
                record["notes"] = report.notes
                record["unreadable"] = [{"line_no": n, "text": s}
                                        for n, s in report.unreadable]
                record["skipped"] = [{"line_no": n, "text": s}
                                     for n, s in report.skipped]
                record["needs_rationale"] = report.needs_rationale
                out.append(record)
        finally:
            con.close()
    return json.dumps(out, indent=1, ensure_ascii=False) + "\n"


# --------------------------------------------------------- the dump oracle
#
# `Deck.dump` is the *other* way deck YAML gets written, and the only one that
# writes a whole file. Two callers reach it and both are creating a file that
# does not exist yet -- `create_deck` and `import_deck` -- which is what makes
# a second writer defensible beside `edit.py`'s surgery: a deck being born has
# no comments to destroy and no diff to keep small.
#
# The cases below are field combinations rather than decks anybody would play.
# What they exercise is the payload's *order* (`sort_keys=False`, so the order
# these keys are built in is the order the file has), the omissions that keep a
# curated file from growing lines asserting defaults, the draft rule that
# appends a blank `why:` **after** `qty` and `art` because Python's
# `setdefault` appends, and the scalar shapes PyYAML has to choose quoting and
# folding for at width 100.

def dump_cases() -> dict[str, Deck]:
    """Every shape the two lifecycle writers can hand the dumper."""
    return {
        # What `create_deck` writes: a commander and nothing else.
        "created": Deck(slug="created", name="Gyome, Master Chef",
                        status="theoretical", stage="draft",
                        commander=["Gyome, Master Chef"]),
        "created-paired": Deck(
            slug="created-paired", name="Ishai and Kraum", status="built",
            stage="draft", commander=["Ishai, Ojutai Dragonspeaker",
                                      "Kraum, Ludevic's Opus"],
            companion="Kaheera, the Orphanguard", bracket=5),
        # What `import_deck` writes: a draft whose cards have no rationale,
        # which is where the appended `why: ''` shows up -- after `qty`.
        "imported": Deck(
            slug="imported", name="Imported", status="theoretical",
            stage="draft", commander=["Goreclaw, Terror of Qal Sisma"],
            bracket=4,
            cards=[CardEntry(name="Forest", category="land", qty=38, why=""),
                   CardEntry(name="Sol Ring", category="ramp", why=""),
                   CardEntry(name="Cultivator Colossus", category="ramp",
                             why="", art="0aae2e33-0000-4000-8000-000000000000")],
            swap_board=[CardEntry(name="Kaheera, the Orphanguard",
                                  category="utility", why="")]),
        # A curated deck omits the blank `why:` instead of pre-typing it, and
        # every optional field is written here in the order `dump` builds it.
        "curated-full": Deck(
            slug="curated-full", name="Everything At Once", status="built",
            stage="curated", shared=False, pilot="Aaron's sister",
            commander_art="0aae2e33-1111-4000-8000-000000000000",
            commander=["Trostani, Selesnya's Voice"],
            companion="Kaheera, the Orphanguard", bracket=4,
            themes=["tokens", "lifegain", "midrange"],
            strategy="Make more creatures than anybody can answer, and then "
                     "make each of them worth answering twice.",
            cards=[CardEntry(name="Forest", category="land", qty=12,
                             why="Green mana, and a great deal of it."),
                   CardEntry(name="Sol Ring", category="ramp",
                             why="Two mana on turn one, and it always has been.",
                             scryfall_id="0aae2e33-2222-4000-8000-000000000000",
                             mana_cost="{1}", tags=["fast", "colourless"])],
            graveyard=[CardEntry(name="Primeval Titan", category="ramp",
                                 why="Banned, and buried until it is not.")]),
        # The legacy `archetype:` key: written back while it is load-bearing,
        # and dropped the moment the themes name a class word.
        "legacy-archetype": Deck(
            slug="legacy-archetype", name="Pre-ADR-37", status="built",
            stage="curated", commander=["Atla Palani, Nest Tender"],
            legacy_archetype="midrange", themes=["dinosaurs", "sacrifice"]),
        "legacy-shadowed": Deck(
            slug="legacy-shadowed", name="Shadowed", status="built",
            stage="curated", commander=["Atla Palani, Nest Tender"],
            legacy_archetype="midrange",
            themes=["dinosaurs", "sacrifice", "midrange"]),
        # The empty shapes: `check_empty_sequence` sends both to the flow
        # writer, so these are `[]` rather than a block with nothing under it.
        "empty": Deck(slug="empty", name="empty", status="theoretical",
                      stage="draft", commander=[]),
        # The scalar shapes, at the width `dump` uses. Every one of these is a
        # different branch of `choose_scalar_style`: a look-alike that must be
        # quoted, an indicator that must be, prose long enough to fold, a name
        # with an apostrophe, one with unicode, and one that would read back as
        # a number.
        "scalars": Deck(
            slug="scalars", name="yes", status="built", stage="curated",
            commander=["Ich-Tekik, Salvage Splicer"],
            pilot="1.0e+3",
            strategy="A strategy long enough that PyYAML has to fold it across "
                     "more than one line at width one hundred, which is where "
                     "the fold rule and the indent rule meet.",
            cards=[CardEntry(name="Ætherling", category="threat",
                             why="colon: and # hash, plus braces {G}{W} and a "
                                 "trailing space "),
                   CardEntry(name="Sword of Feast and Famine", category="threat",
                             why="no"),
                   CardEntry(name="Borrowing 100,000 Arrows", category="threat",
                             why="- a leading dash, and *an asterisk*"),
                   CardEntry(name="Erase (Not the Urza's Legacy One)",
                             category="interaction", why="1996")]),
    }


def render_dump_cases() -> str:
    """Each deck above, as the dumper writes it."""
    return json.dumps(
        {name: deck.dump() for name, deck in dump_cases().items()},
        indent=1, ensure_ascii=False) + "\n"


def render_edit_cases() -> str:
    """Every operation over every fixture deck, with Python's exact answer."""
    from mtglab.decks import edit

    ops = {"replace_card": edit.replace_card, "add_card": edit.add_card,
           "remove_card": edit.remove_card, "entomb_card": edit.entomb_card,
           "return_card": edit.return_card, "exile_card": edit.exile_card,
           "set_card_field": edit.set_card_field,
           "set_deck_field": edit.set_deck_field, "set_note": edit.set_note,
           "set_shared": edit.set_shared}

    sources: dict[str, str] = {name: deck.dump()
                              for name, deck in gate_cases().items()}
    sources.update(handwritten_decks())

    decks: dict[str, str] = {}
    cases: list[dict[str, Any]] = []
    for name, text in sources.items():
        deck = Deck.from_text(text, slug=name)
        decks[name] = text
        for n, chain in enumerate(edit_steps(name, deck)):
            steps: list[dict[str, Any]] = []
            current = text
            for step in chain:
                kwargs = {k: v for k, v in step.items() if k != "op"}
                record: dict[str, Any] = {"op": step["op"], "args": kwargs}
                try:
                    current = ops[step["op"]](current, **kwargs)
                    record["ok"] = True
                    record["want"] = current
                except edit.EditFailed as exc:
                    record["ok"] = False
                    record["error"] = str(exc)
                steps.append(record)
            cases.append({"deck": name, "chain": n, "steps": steps})
    return json.dumps({"decks": decks, "cases": cases},
                      indent=1, ensure_ascii=False) + "\n"


def write() -> None:
    TESTDATA.mkdir(parents=True, exist_ok=True)
    text, parsed = render()
    YAML_PATH.write_text(text, encoding="utf-8")
    JSON_PATH.write_text(parsed, encoding="utf-8")
    print(f"wrote {YAML_PATH}\nwrote {JSON_PATH}")
    REFERENCE_DIR.mkdir(parents=True, exist_ok=True)
    for name, body in reference.render().items():
        (REFERENCE_DIR / name).write_text(body, encoding="utf-8")
        print(f"wrote {REFERENCE_DIR / name}")
    TINY_POOL_PATH.parent.mkdir(parents=True, exist_ok=True)
    SCHEMA_PATH.write_text(render_schema(), encoding="utf-8")
    TINY_POOL_PATH.write_text(render_tiny_pool(), encoding="utf-8")
    print(f"wrote {SCHEMA_PATH}\nwrote {TINY_POOL_PATH}")
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    (LOG_DIR / "describe.json").write_text(render_log_cases(), encoding="utf-8")
    (LOG_DIR / "app_schema.sql").write_text(render_app_schema(), encoding="utf-8")
    print(f"wrote the log oracle and app.db's schema into {LOG_DIR}")
    EDITS_PATH.parent.mkdir(parents=True, exist_ok=True)
    EDITS_PATH.write_text(render_edit_cases(), encoding="utf-8")
    print(f"wrote {EDITS_PATH}")
    DUMPS_PATH.parent.mkdir(parents=True, exist_ok=True)
    DUMPS_PATH.write_text(render_dump_cases(), encoding="utf-8")
    print(f"wrote {len(dump_cases())} dump cases into {DUMPS_PATH}")
    DECKLIST_PATH.parent.mkdir(parents=True, exist_ok=True)
    DECKLIST_PATH.write_text(render_decklist_cases(), encoding="utf-8")
    print(f"wrote {len(decklist_cases())} decklist cases into {DECKLIST_PATH}")
    IMPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    IMPORT_PATH.write_text(render_import_cases(), encoding="utf-8")
    print(f"wrote {len(import_cases())} import cases into {IMPORT_PATH}")
    RENDER_PATH.parent.mkdir(parents=True, exist_ok=True)
    RENDER_PATH.write_text(render_render_cases(), encoding="utf-8")
    print(f"wrote {len(render_cases())} render cases into {RENDER_PATH}")
    GATE_DIR.mkdir(parents=True, exist_ok=True)
    for name, body in render_gate_cases().items():
        (GATE_DIR / name).write_text(body, encoding="utf-8")
    print(f"wrote {len(render_gate_cases())} gate cases into {GATE_DIR}")


if __name__ == "__main__":
    write()
