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
from mtglab.decks.model import CardEntry, Deck

ROOT = Path(__file__).resolve().parents[1]
TESTDATA = ROOT / "go" / "internal" / "deckyaml" / "testdata"
YAML_PATH = TESTDATA / "rich-deck.yaml"
JSON_PATH = TESTDATA / "rich-deck.parsed.json"
#: Where the Go module embeds the reference prose from (`go:embed data/*`).
REFERENCE_DIR = ROOT / "go" / "internal" / "reference" / "data"
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
    GATE_DIR.mkdir(parents=True, exist_ok=True)
    for name, body in render_gate_cases().items():
        (GATE_DIR / name).write_text(body, encoding="utf-8")
    print(f"wrote {len(render_gate_cases())} gate cases into {GATE_DIR}")


if __name__ == "__main__":
    write()
