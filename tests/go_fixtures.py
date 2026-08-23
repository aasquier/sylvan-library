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

import base64
import dataclasses
import hashlib
import json
import math
import os
import random
import struct
import sys
import tempfile
import unicodedata
from datetime import date
from pathlib import Path
from typing import Any

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab import reference
from mtglab.cards.db import CardRecord
from mtglab.decks.decklist import MAX_LINE
from mtglab.decks.model import CATEGORIES, CardEntry, Deck
from mtglab.mana import ManaCost, ManaSource, parse_mana_cost
from mtglab.sim import curve, karsten
from mtglab.sim.tier1.engine import SimCard

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
#: The artifacts oracle: `render_all` over every fixture deck, beside the
#: exact markdown Python writes for it -- the five deliverables in the order
#: they are built, `swaps.md` when there is a baseline, the snapshot last, and
#: the refusal's own sentence for a draft.
ARTIFACTS_PATH = ROOT / "go" / "internal" / "artifacts" / "testdata" / "artifacts.json"
#: The activity log's oracle: every sentence `log.describe` writes, and
#: `app.db`'s schema as the ladder leaves it, so the Go log tests have a real
#: table to insert into in CI where there is no Python (Phase 4).
LOG_DIR = ROOT / "go" / "internal" / "decklog" / "testdata"
#: The pool's schema and the 21-card fixture, for the Go pool tests.
POOL_DIR = ROOT / "go" / "internal" / "pool"
SCHEMA_PATH = POOL_DIR / "schema.sql"
TINY_POOL_PATH = POOL_DIR / "pooltest" / "testdata" / "tiny_pool.json"
#: The Wheel's spins (Phase 8): seeded draws over the 21-card pool, recorded
#: as the marshalled payload text, because a spin is a promise -- a seed
#: somebody replays after the cutover must deal the same fate, face and card.
WHEEL_PATH = ROOT / "go" / "internal" / "wheel" / "testdata" / "spins.json"
#: The upcoming-sets filter (Phase 8): Scryfall payloads through the real
#: `service.upcoming_sets`, clock and network stubbed, rendered as the bytes
#: the route answers.
SETS_PATH = ROOT / "go" / "internal" / "api" / "testdata" / "sets.json"


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


# ---------------------------------------------------------- the Wheel's spins
#
# `decks/wheel.py` against the 21-card pool, seeded, and recorded as the
# **marshalled payload text** (the sim cache corpus's lesson): every way the
# two runtimes' JSON could differ presents as the same-looking dict, so the
# comparison is over the bytes the route would answer. Seventeen seeds sweep
# the four fates and both faces of each; the colourless deck exercises the
# `len(color_identity) = 0` branch and the no-candidate reason; 2**70 pins
# the unbounded seed echoed back through `pyrand`.


def _wire_dumps(payload: object) -> str:
    """`json.dumps` as Starlette writes a response body -- what
    `wire.Marshal` reproduces."""
    return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))


def wheel_cases() -> dict[str, object]:
    import tempfile

    import tiny_pool
    from mtglab.cards import db
    from mtglab.decks import wheel

    mono = tiny_pool.mono_green_deck(clean=True)
    bare = tiny_pool.mono_green_deck(clean=True)
    bare.commander = []

    cases: list[dict[str, object]] = []
    with tempfile.TemporaryDirectory() as tmp:
        con = db.connect(tiny_pool.build(Path(tmp) / "pool.duckdb"))
        try:
            commander = mono.commander[0]
            rec = db.get_cards(con, [commander]).get(commander)
            identity = rec.color_identity if rec else frozenset()
            for seed in [*range(16), 2**70]:
                cases.append({
                    "deck": "mono-green", "identity": sorted(identity),
                    "seed": seed,
                    "rendered": _wire_dumps(
                        wheel.spin(mono, identity, con, seed=seed)),
                })
            # A five-colour identity widens the candidate pool enough that
            # the card branch -- and the counted-offset draw inside it --
            # is exercised across fates, not once.
            for seed in [*range(12), 2**70 + 1]:
                cases.append({
                    "deck": "mono-green", "identity": list("WUBRG"),
                    "seed": seed,
                    "rendered": _wire_dumps(
                        wheel.spin(mono, frozenset("WUBRG"), con, seed=seed)),
                })
            for seed in (0, 3, 7):
                cases.append({
                    "deck": "bare", "identity": [], "seed": seed,
                    "rendered": _wire_dumps(
                        wheel.spin(bare, frozenset(), con, seed=seed)),
                })
        finally:
            con.close()
    return {
        "decks": {"mono-green": mono.dump(), "bare": bare.dump()},
        "cases": cases,
    }


def render_wheel_cases() -> str:
    return json.dumps(wheel_cases(), indent=1, ensure_ascii=False) + "\n"


# ------------------------------------------------------ the upcoming-sets feed
#
# `service.upcoming_sets` with the clock frozen and the network stubbed --
# the real function, so the strict `>` against today, the digital drop, the
# stable tie order and the six-key row all come from the code under test
# rather than from a description of it.


def sets_cases() -> list[dict[str, object]]:
    import io
    import urllib.request

    from mtglab.api import service

    frozen = "2026-08-23"
    payloads: dict[str, dict[str, object]] = {
        "mixed": {"data": [
            {"code": "old", "name": "Long Released", "released_at": "2020-01-01",
             "card_count": 300, "icon_svg_uri": "https://svgs.scryfall.io/sets/old.svg",
             "set_type": "expansion"},
            {"code": "tod", "name": "Releases Today", "released_at": frozen,
             "card_count": 50, "icon_svg_uri": "https://svgs.scryfall.io/sets/tod.svg",
             "set_type": "expansion"},
            {"code": "tw2", "name": "Twin Two", "released_at": "2026-11-20",
             "card_count": 250, "icon_svg_uri": "https://svgs.scryfall.io/sets/tw2.svg",
             "set_type": "expansion"},
            {"code": "dig", "name": "Arena Only", "released_at": "2026-10-01",
             "card_count": 100, "icon_svg_uri": "https://svgs.scryfall.io/sets/dig.svg",
             "set_type": "alchemy", "digital": True},
            {"code": "tw1", "name": "Twin One", "released_at": "2026-11-20",
             "card_count": 90, "set_type": "commander"},
            {"code": "mrc", "name": "Märchen der Welt — Föhn",
             "released_at": "2026-09-30", "card_count": 271,
             "icon_svg_uri": "https://svgs.scryfall.io/sets/mrc.svg",
             "set_type": "expansion"},
            {"code": "und", "name": "Undated"},
        ]},
        "empty": {"data": []},
        "keyless": {"object": "list"},
    }

    class FrozenDate:
        @staticmethod
        def today() -> date:
            return date(2026, 8, 23)

    out = []
    real_date, real_urlopen = service.date, urllib.request.urlopen
    saved_cache = dict(service._SETS_CACHE)
    try:
        service.date = FrozenDate  # type: ignore[assignment]
        for name, payload in payloads.items():
            body = json.dumps(payload).encode()

            def fake_urlopen(req: object, timeout: float = 0,
                             _body: bytes = body) -> io.BytesIO:
                del req, timeout
                return io.BytesIO(_body)

            urllib.request.urlopen = fake_urlopen  # type: ignore[assignment]
            answer = service.upcoming_sets(force=True)
            out.append({"name": name, "today": frozen, "payload": payload,
                        "rendered": _wire_dumps(answer)})
    finally:
        service.date = real_date  # type: ignore[assignment]
        urllib.request.urlopen = real_urlopen
        service._SETS_CACHE.clear()
        service._SETS_CACHE.update(saved_cache)
    return out


def render_sets_cases() -> str:
    return json.dumps(sets_cases(), indent=1, ensure_ascii=False) + "\n"


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
            "artifact-commander": artifact, "last-bit": last_bit_deck(),
            "rich": rich_deck()}


def last_bit_deck() -> Deck:
    """A deck at the shape where `analyze.opening_hand`'s `keepable` parts.

    Not a deck anybody would play, and not pretending to be: it is the ninth
    fixture and it exists for one number. `keepable` sums three hypergeometric
    probabilities, and **`sum()` over floats is not the same function on every
    interpreter** -- CPython 3.12 gave it compensated (Neumaier) accumulation
    where 3.11 adds left to right, and a Go `+=` loop reproduces 3.11 while the
    container runs 3.12. Swept over every deck size from 8 to 250 and every
    land count inside it, the two arithmetics disagree in 5,098 shapes.

    **The other eight fixtures are in none of them.** They sit at 99 cards on
    95 or 96 lands and at 106 on 96, and the arithmetic happens to agree there,
    so before this deck existed the corpus could not have caught the naive loop
    the port was written with -- a green test that had no way to go red. This
    is the same hole `sim/curve.py`'s `tie-breaker` deck was cut for, on the
    same day, from the opposite direction.

    99 cards on **91** lands is the nearest divergent shape reachable from
    `tiny_pool`, whose 21 cards cannot furnish the 65 nonlands a realistic land
    count would need. It is `mono_green_deck(clean=True)` with four more green
    spells and four fewer Forests, so it is gate-clean and reads as a sibling
    of the deck it came from. Measured: 3.11 answers 0.010640320706772594 and
    3.12 answers 0.010640320706772595, and the difference reaches the JSON.
    """
    import tiny_pool
    deck = tiny_pool.mono_green_deck(clean=True)
    deck.slug, deck.name = "last-bit", "Last Bit Fixture"
    forest = next(c for c in deck.cards if c.name == "Forest")
    extra = [
        ("Craterhoof Behemoth", "threat", "The finisher every green deck owes itself."),
        ("Terastodon", "interaction", "Green's answer to a permanent it cannot target."),
        ("Woodfall Primus", "interaction", "Persist, and the artifact goes twice."),
        ("Bag End Banquet", "ramp", "Colourless, so it costs the identity nothing."),
    ]
    deck.cards.extend(CardEntry(name=n, category=c, why=w) for n, c, w in extra)
    forest.qty -= len(extra)                     # still 99, now on 91 lands
    return deck


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


# --------------------------------------------------- the scorer's own corpus
#
# `suggest.score` had exactly one differential case: the Titan's four
# candidates out of the 21-card pool, which is four points in a space with
# four dimensions. That was enough to prove the *reasons* and the ordering and
# nothing at all about the arithmetic, and on 2026-08-22 the arithmetic turned
# out to be where the bug was -- the weighted sum was `sum` in Python (which
# CPython 3.12 compensates and 3.11 does not) and a chain of `+` in Go (which
# is 3.11's answer, and fusable into an FMA on arm64 besides).
#
# Real cards cannot be steered onto a knife edge. Synthetic ones can: each
# case here names the four component scores it wants and builds a pair of
# records that produce them, so the corpus covers the reachable range on
# purpose rather than by luck.

SCORE_PATH = ROOT / "go" / "internal" / "suggest" / "testdata" / "scores.json"


def _score_word(i: int) -> str:
    """A significant token: alphabetic (`_WORD` is `[a-z]+`), longer than two
    letters, and nowhere near the stopword list."""
    return f"zz{chr(97 + i // 26)}{chr(97 + i % 26)}"


def _score_pair(spec: dict[str, Any]) -> tuple[Any, Any]:
    """Two records built to produce the component scores `spec` asks for.

    `keywords` is `(target, candidate, shared)` counts, so the Jaccard the
    scorer computes is `shared / (target + candidate - shared)`; `tokens` is
    `(target, shared)`, so the asymmetric text score is `shared / target`.
    """
    from mtglab.cards.db import CardRecord
    kt, kc, ks = spec["keywords"]
    xt, xs = spec["tokens"]
    shared_kw = [f"shared{i:02d}" for i in range(ks)]
    target_kw = shared_kw + [f"onlytarget{i:02d}" for i in range(kt - ks)]
    cand_kw = shared_kw + [f"onlycand{i:02d}" for i in range(kc - ks)]
    target_words = [_score_word(i) for i in range(xt)]
    # The candidate repeats `xs` of the target's words and adds its own, so
    # the denominator stays the target's vocabulary.
    cand_words = target_words[:xs] + [_score_word(500 + i) for i in range(4)]

    def rec(name: str, type_line: str, cmc: float, kws: list[str],
            words: list[str], rank: int | None) -> Any:
        return CardRecord(
            name=name, mana_cost=None, cmc=cmc, type_line=type_line,
            oracle_text=" ".join(words), color_identity=frozenset(),
            produced_mana=(), legal_commander=False, reserved=False,
            edhrec_rank=rank, image_normal=None, keywords=tuple(kws))

    return (rec("Target Card", spec["target_type"], spec["target_cmc"],
                target_kw, target_words, None),
            rec("Candidate Card", spec["candidate_type"], spec["candidate_cmc"],
                cand_kw, cand_words, spec["rank"]))


def _score_spec(target_type: str, candidate_type: str, target_cmc: float,
                candidate_cmc: float, keywords: tuple[int, int, int],
                tokens: tuple[int, int], rank: int | None,
                note: str) -> dict[str, Any]:
    return {"target_type": target_type, "candidate_type": candidate_type,
            "target_cmc": target_cmc, "candidate_cmc": candidate_cmc,
            "keywords": keywords, "tokens": tokens, "rank": rank, "note": note}


def score_specs() -> list[dict[str, Any]]:
    """Every pair the corpus asks Python about.

    The first block walks the scorer's ordinary range -- both permanents, one
    permanent, an exact type match, every curve distance the score is nonzero
    over, no keywords at all on the target (which scores 0 rather than
    renormalising), no shared text, all shared text, and the popularity nudge
    at both ends. The second block is the point: quadruples measured to fall
    where a left-to-right sum and a correctly-rounded one disagree.
    """
    ordinary = [
        _score_spec("Creature — Elf", "Creature — Beast", 3.0, 3.0,
                    (3, 3, 3), (10, 10), 1, "identical everything, rank 1"),
        _score_spec("Creature — Elf", "Artifact", 4.0, 4.0,
                    (2, 2, 0), (12, 0), None, "permanent for permanent, nothing shared"),
        _score_spec("Instant", "Creature — Elf", 2.0, 5.0,
                    (0, 3, 0), (8, 4), 500, "no target keywords, curve three apart"),
        _score_spec("Sorcery", "Sorcery", 6.0, 5.0,
                    (4, 2, 1), (20, 7), 12345, "same type, one off the curve"),
        _score_spec("Land", "Land", 0.0, 0.0,
                    (1, 1, 1), (3, 3), 99999, "lands, and the rank floor"),
        _score_spec("Enchantment", "Instant", 5.0, 1.0,
                    (2, 2, 1), (15, 5), None, "curve four apart, so curve scores zero"),
        _score_spec("Planeswalker", "Battle", 4.0, 6.0,
                    (5, 5, 2), (30, 11), 3, "two permanents, two off the curve"),
        _score_spec("Creature — Elf", "", 3.0, 3.0,
                    (2, 2, 2), (6, 6), 250, "an empty type line scores zero"),
    ]
    # Measured on 2026-08-22: with these four components, adding
    # `0.30t + 0.20c + 0.15k + 0.35x` left to right and summing it correctly
    # give different doubles, and the first two differ at `round(., 4)` --
    # the value that is serialised and sorted on.
    knife_edge = [
        _score_spec("Instant", "Sorcery", 3.0, 3.0, (6, 7, 1), (40, 13), None,
                    "type 0, curve 1, keywords 1/12, text 13/40 -- differs at four places"),
        _score_spec("Instant", "Sorcery", 3.0, 3.0, (5, 6, 1), (40, 21), 620,
                    "keywords 1/10, text 21/40, with the popularity nudge on top"),
        _score_spec("Instant", "Sorcery", 2.0, 2.0, (6, 7, 1), (40, 19), None,
                    "keywords 1/12, text 19/40"),
    ]
    return ordinary + knife_edge


def score_cases() -> list[dict[str, Any]]:
    """Each pair, the records it builds, and what Python's `score` answers."""
    from mtglab.decks import suggest as sg

    def as_json(rec: Any) -> dict[str, Any]:
        return {"name": rec.name, "type_line": rec.type_line, "cmc": rec.cmc,
                "oracle_text": rec.oracle_text,
                "keywords": list(rec.keywords),
                "edhrec_rank": rec.edhrec_rank}

    out: list[dict[str, Any]] = []
    for spec in score_specs():
        target, candidate = _score_pair(spec)
        result = sg.score(target, candidate, why="")
        tokens = sg._tokens(target.oracle_text, "")
        out.append({
            "note": spec["note"],
            "target": as_json(target),
            "candidate": as_json(candidate),
            # The four components, recorded apart from the score they sum to,
            # so a failure says which half is wrong -- the same division
            # `pyrand`'s corpus makes between the word stream and its
            # consumers.
            "parts": [sg._type_score(target, candidate),
                      sg._curve_score(target, candidate),
                      sg._keyword_score(target, candidate),
                      sg._text_score(tokens, candidate)],
            "weights": [0.30, 0.20, 0.15, 0.35],
            "popularity": sg._popularity(candidate),
            "score": result.score,
            "reasons": list(result.reasons),
        })
    return out


def render_score_cases() -> str:
    return _rows_json({
        "note": "`suggest.score` over synthetic records, component by "
                "component. Written by `python tests/go_fixtures.py`.",
        "why": "The weighted sum is `math.fsum` and each product is rounded "
               "on its own. A chain of `+` is CPython 3.11's `sum` rather "
               "than 3.12's, and `a*b + c*d` is what arm64 fuses into one "
               "FMADDD -- and the value is rounded to four places and then "
               "sorted on, so either difference is a different shortlist.",
        "cases": score_cases(),
    }) + "\n"


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
        f"-- version {version}. Written by `python tests/go_fixtures.py`. Since",
        "-- Phase 8 the Go ladder (go/internal/auth/schema.go) owns the deployed",
        "-- file, and TestMigrateBuildsWhatPythonBuilt holds it to these bytes --",
        "-- the lockstep between the two ladders. Do not hand-edit.",
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


#: The notes fixture, and the one hand-written deck that exists for the
#: *dumper* rather than for the editor.
#:
#: `notes:` is the only mapping in a deck file whose keys are the author's
#: rather than the model's, and `sort_keys=False` means the file's order is
#: what a dump writes back. Nothing needed that until the artifacts snapshot,
#: which is `deck.dump()` of a **parsed** deck -- so this deck's notes are
#: deliberately out of alphabetical order (a map iterated at random passes an
#: in-order assertion far too often) and hold every value shape a person can
#: type: folded prose, a literal block that keeps its line breaks, a scalar
#: that has to be quoted to read back as itself, a nested mapping and a list.
#:
#: The exotic values are filed under keys the primers never ask for.
#: `generate._note` calls `.strip()` on whatever it finds, so a mapping under
#: `mulligan` would be a traceback rather than a fixture -- which is a true
#: fact about the Python and not one worth recording as an oracle case.
NOTED = """\
slug: noted
name: Written With Notes
status: built
stage: curated
commander:
  - Gyome, Master Chef
themes:
  - food
  - midrange

notes:
  wincons: >-
    Feed the table until somebody is the only one left who can afford to
    stop eating.
  pitfalls: >-
    The deck folds to graveyard hate it cannot rebuild through.
  mulligan: >-
    Keep any seven with two lands and a sacrifice outlet.
  gameplan: Cook, and keep cooking.
  curve_plan: |
    T2 outlet.
    T3 Gyome.
    T4 the table starts asking for food.
  commander_why: 'yes'
  shopping:
    cheapest: Bag End Banquet
    dearest: Sword of Feast and Famine
  sources:
    - The primer nobody wrote
    - A conversation at the kitchen table

cards:

  # ---- RAMP 1
  - name: Sol Ring
    category: ramp
    why: >-
      Two mana on turn one, and it always has been.

  # ---- LANDS 98
  - name: Forest
    category: land
    qty: 98
    why: >-
      Green, and there is nothing else to be.
"""


def handwritten_decks() -> dict[str, str]:
    """name -> deck text, written the way a person writes one."""
    return {"handwritten": HANDWRITTEN, "wide": WIDE, "tight": TIGHT,
            "noted": NOTED}


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
        # The notes, which `dump` could not write at all until the artifacts
        # flip: `sort_keys=False` makes the payload's order the file's order,
        # and a Go map has none. Deliberately not alphabetical, and holding a
        # value long enough to fold, one that must be quoted to read back as a
        # string, and one carrying its own line breaks.
        "notes": Deck(
            slug="notes", name="Noted", status="built", stage="curated",
            commander=["Gyome, Master Chef"],
            notes={
                "wincons": "Feed the table until somebody is the only one left "
                           "who can afford to stop eating, which takes longer "
                           "than a hundred characters to say.",
                "gameplan": "Cook.",
                "mulligan": "yes",
                "curve_plan": "T2 outlet.\nT3 Gyome.\nT4 the table asks for food.",
            },
            cards=[CardEntry(name="Forest", category="land", qty=99,
                             why="Green, and there is nothing else to be.")]),
    }


#: The dump cases whose *source* is a file a person typed rather than the
#: dumper's own output. The round trip the Go test drives -- parse this, dump
#: it, compare with what Python's parse dumped -- only proves anything about
#: reading a hand-written file when one is the thing being read: a recorded
#: dump fed back in has already been through PyYAML's choices once, so both
#: parsers see the tidied version and a disagreement about the untidied one
#: never surfaces.
def dump_from_text() -> dict[str, str]:
    """Hand-written deck text -> what Python's parse of it dumps to."""
    return handwritten_decks()


def render_dump_cases() -> str:
    """Each deck above, as the dumper writes it, beside what it was dumped from."""
    out: dict[str, dict[str, str]] = {}
    for name, deck in dump_cases().items():
        # A constructed deck has no source text of its own; its dump is both
        # sides, which is exactly the round trip this oracle has always run.
        text = deck.dump()
        out[name] = {"source": text, "want": text}
    for name, source in dump_from_text().items():
        # Loudly, because the two halves are named in different functions and
        # a collision would silently drop a case rather than fail.
        assert name not in out, f"{name} is both a constructed and a written case"
        out[name] = {"source": source,
                     "want": Deck.from_text(source, slug=name).dump()}
    return json.dumps(out, indent=1, ensure_ascii=False) + "\n"


# ----------------------------------------------------- the artifacts oracle
#
# The five deliverables, and the bytes are the product: a primer is markdown a
# person reads and `moxfield.txt` is pasted into a website, so the Go port has
# to reproduce `render_all` character for character rather than approximately.
#
# Two things this oracle does that the others do not. It records the **order**
# of the files as well as their contents, because `store` writes them in that
# order and relies on the snapshot being last. And it **freezes the date**:
# every deliverable ends in `_Generated <today>_`, so an oracle that asked the
# clock would be a fixture that failed at midnight. Go takes the same date
# through `Options.Today`.

#: The day the oracle was rendered on. Any date will do; what matters is that
#: it is written down rather than asked for.
ARTIFACTS_DATE = date(2026, 8, 22)


class _FrozenDate:
    """`generate.date`, with today pinned to ARTIFACTS_DATE."""

    @staticmethod
    def today() -> date:
        return ARTIFACTS_DATE


def _record(name: str, mana_cost: str | None) -> CardRecord:
    """A pool record with the one field the annotated list reads."""
    return CardRecord(name=name, mana_cost=mana_cost, cmc=0.0, type_line="",
                      oracle_text="", color_identity=frozenset(),
                      produced_mana=(), legal_commander=False, reserved=False,
                      edhrec_rank=None, image_normal=None)


#: The mana costs the annotated list prints, for the cards the fixtures use.
#: A card missing from here renders with no cost, which is the ordinary state
#: of a deck built on a machine with no pool.
ARTIFACT_CARDS = {"Sol Ring": "{1}", "Forest": None,
                  "Swords to Plowshares": "{W}",
                  "Cultivator Colossus": "{5}{G}{G}",
                  "Vorinclex, Voice of Hunger": "{6}{G}{G}"}


def artifact_decks() -> dict[str, Deck]:
    """Decks built for the renderer's own branches rather than the gate's."""
    every = Deck(
        slug="every-category", name="One Of Everything", status="built",
        stage="curated", commander=["Gyome, Master Chef"],
        companion="Kaheera, the Orphanguard", bracket=4,
        strategy="A card in every category, so every heading renders.",
        cards=[CardEntry(name=f"Card {i}", category=cat,
                         why=f"The {cat} slot.")
               for i, cat in enumerate(CATEGORIES)],
        swap_board=[CardEntry(name="Bag End Banquet", category="payoff",
                              why="Waiting on a slot."),
                    CardEntry(name="Aetherflux Reservoir", category="win-con",
                              why="")])
    # Two categories the model does not declare, so the "sorted, and after the
    # declared ones" branch has something to sort and `str.title()` has
    # something to capitalise -- and lands still end up last.
    every.cards += [
        CardEntry(name="Stax Piece", category="stax-piece", why="Invented."),
        CardEntry(name="Aggro Plan", category="aggro-plan", why="Invented too."),
        CardEntry(name="Forest", category="land", qty=30,
                  why="Green, and there is nothing else to be."),
        CardEntry(name="Sol Ring", category="ramp", qty=2, why=""),
    ]
    return {
        "every-category": every,
        # The shopping list's total, at the one place its two decimals can
        # show which interpreter added them up. `render_all`'s `total` was a
        # bare `sum` over floats until 2026-08-22 and the Go port was a `+=`
        # loop, which is CPython 3.11's `sum` and not 3.12's -- and 3.12 is
        # what the image runs. `every-category` prices four cards and could
        # not catch it: its 12.5, 0.25 and 1.995 are all exact halves or
        # quarters until the last addition, so the naive answer *is* the
        # correctly-rounded one and the byte-exact oracle passed either way.
        # A test that cannot fail is the thing worth fixing here, not the
        # line above it.
        #
        # Three cards at half-cent prices, the same shape as the 1.995 the
        # older case already carries. Their exact total is 902.405, dead on
        # the boundary `{:.2f}` rounds at: adding them left to right gives
        # 902.4050000000001 and renders **902.41**, and the correctly-rounded
        # sum gives 902.405 and renders **902.40**. Measured separately, and
        # worth writing down: over two million random all-two-decimal price
        # sets, *none* renders a different total, so real Scryfall prices
        # could never have exposed this. The guard has to be built, not
        # waited for.
        "half-cent": Deck(
            slug="half-cent", name="The Half Cent", status="built",
            stage="curated", commander=["Gyome, Master Chef"],
            strategy="Three cards whose prices land on the rounding boundary.",
            cards=[CardEntry(name="Forest", category="land", qty=96,
                             why="Green, and the rest of the deck."),
                   CardEntry(name="Card A", category="ramp",
                             why="Dearest of the three."),
                   CardEntry(name="Card B", category="ramp",
                             why="Priced so the order of addition matters."),
                   CardEntry(name="Card C", category="ramp",
                             why="The one that carries the last bit.")]),
        # Every note key both primers ask for, with the whitespace `_note`
        # strips still on them.
        "full-notes": Deck(
            slug="full-notes", name="Fully Noted", status="built",
            stage="curated", commander=["Trostani, Selesnya's Voice"],
            bracket=3,
            strategy="Make more creatures than anybody can answer.",
            notes={key: f"  What {key} has to say, with space around it.  "
                   for key in ("gameplan", "mulligan", "curve_plan", "pitfalls",
                               "engine_detail", "lines", "wincons", "manabase",
                               "matchups", "politics", "failure_modes",
                               "swap_philosophy", "rules_corners",
                               "commander_why")},
            cards=[CardEntry(name="Sol Ring", category="ramp", why="Fast."),
                   CardEntry(name="Forest", category="land", qty=40,
                             why="Green.")]),
        # No commander, no companion, no bracket: the header's other branch.
        # A bracket of nought is *falsy* in Python, so it is not written --
        # which is a real difference from "no bracket" and looks identical.
        "bare": Deck(slug="bare", name="Bare", status="theoretical",
                     stage="curated", commander=[], bracket=0,
                     cards=[CardEntry(name="Forest", category="land", qty=99,
                                      why="Green.")]),
        # A draft, refused -- with more than eight cards owing a rationale, so
        # the "and N more" tail renders.
        "draft-owing": Deck(
            slug="draft-owing", name="Owing", status="theoretical",
            stage="draft", commander=["Goreclaw, Terror of Qal Sisma"],
            cards=[CardEntry(name=f"Card {i}", category="utility", why="")
                   for i in range(11)]),
        # A draft that owes nothing, which is the other half of the refusal:
        # the way out is `stage: curated`, not a flag.
        "draft-settled": Deck(
            slug="draft-settled", name="Settled", status="built",
            stage="draft", commander=["Goreclaw, Terror of Qal Sisma"],
            cards=[CardEntry(name="Forest", category="land", qty=99,
                             why="Green.")]),
    }


def artifact_cases() -> list[dict[str, Any]]:
    """Every deck the renderer can be handed, and what Python renders for it."""
    from mtglab.artifacts import generate

    sources: dict[str, Deck] = dict(artifact_decks())
    sources.update(dump_cases())
    for name, text in handwritten_decks().items():
        sources[f"hand-{name}"] = Deck.from_text(text, slug=name)

    cards = {name: _record(name, cost) for name, cost in ARTIFACT_CARDS.items()}

    # The three kwargs `render_all` takes beyond the deck, each on one case:
    # a baseline (so `swaps.md` exists at all), prices (so it grows a shopping
    # list), and stats (so both primers grow a block). Neither the CLI nor the
    # API passes prices or stats today; the shape is ported, so the shape is
    # proven.
    previous = Deck(
        slug="every-category", name="One Of Everything", status="built",
        stage="curated", commander=["Gyome, Master Chef"],
        cards=[CardEntry(name="Card 0", category="land", why="The land slot."),
               CardEntry(name="Cut With A Reason", category="ramp",
                         why="It was here and now it is not."),
               CardEntry(name="Cut With None", category="ramp", why=""),
               CardEntry(name="Forest", category="land", qty=30, why="Green.")])
    # The baseline for `half-cent`: the same deck with the three priced cards
    # not yet in it, so `add` is exactly `["Card A", "Card B", "Card C"]` --
    # alphabetical, which is the order `swaps.md` adds them in. That order is
    # half of the point: a left-to-right sum is a function of it.
    before_half_cent = Deck(
        slug="half-cent", name="The Half Cent", status="built", stage="curated",
        commander=["Gyome, Master Chef"],
        cards=[CardEntry(name="Forest", category="land", qty=96,
                         why="Green, and the rest of the deck.")])
    extras: dict[str, dict[str, Any]] = {
        "every-category": {
            "previous": previous,
            # Two cards at the same price, so the dearest-first sort has a tie
            # to keep stable, and one with no price at all.
            "prices": {"Card 1": 12.5, "Card 2": 12.5, "Card 3": 0.25,
                       "Sol Ring": 1.995},
            "stats": {"mulligan rate": "12.4%", "spells through T8": "9.1"},
        },
        # 142.115 + 412.545 + 347.745 is exactly 902.405. Left to right that
        # is 902.4050000000001 and renders 902.41; correctly rounded it is
        # 902.405 and renders 902.40. See the deck's own note above.
        "half-cent": {
            "previous": before_half_cent,
            "prices": {"Card A": 142.115, "Card B": 412.545, "Card C": 347.745},
        },
        "full-notes": {"previous": previous},
    }

    out: list[dict[str, Any]] = []
    for name in sorted(sources):
        deck = sources[name]
        extra = extras.get(name, {})
        case: dict[str, Any] = {
            "name": name,
            "deck": deck.dump(),
            "previous": extra["previous"].dump() if "previous" in extra else None,
            "prices": extra.get("prices"),
            "stats": list((extra.get("stats") or {}).items()) or None,
        }
        try:
            rendered = generate.render_all(
                deck, cards=cards, previous=extra.get("previous"),
                prices=extra.get("prices"), stats=extra.get("stats"))
            case["ok"] = True
            case["files"] = [{"name": n, "text": t} for n, t in rendered.items()]
        except generate.DraftDeck as exc:
            # The refusal is half of what this module does, and its sentence
            # reaches the wire as the 422's `detail`.
            case["ok"] = False
            case["error"] = str(exc)
        out.append(case)
    return out


def render_artifact_cases() -> str:
    """Every case above, with the date pinned so the oracle does not expire."""
    from mtglab.artifacts import generate

    original = generate.date
    generate.date = _FrozenDate            # type: ignore[assignment]
    try:
        cases = artifact_cases()
    finally:
        generate.date = original           # type: ignore[assignment]
    return json.dumps({"today": ARTIFACTS_DATE.isoformat(),
                       "cards": ARTIFACT_CARDS, "cases": cases},
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


CRYPTO_PATH = ROOT / "go" / "internal" / "auth" / "testdata" / "crypto.json"

#: `app.db`'s DDL, embedded by `go/internal/auth/authtest` and handed to every
#: Go test that needs a real one. It lived beside `internal/decklog`'s tests
#: until 2026-08-22, when the accounts flip found that two *other* packages had
#: each transcribed the ladder by hand and frozen it at a different rung; the
#: package comment on `authtest` records what that cost.
APP_SCHEMA_PATH = ROOT / "go" / "internal" / "auth" / "authtest" / "app_schema.sql"

#: Passwords the Argon2 oracle records. Deliberately awkward: a short one the
#: strength check would refuse but the *verifier* must still handle (an account
#: whose hash predates the floor), an empty one, unicode above the BMP, a
#: passphrase with the spaces the floor is written to encourage, and one at the
#: 1024-byte ceiling. What is being proven is the encoding, and the encoding is
#: where a length or a code point turns into a different byte string.
_ARGON2_PASSWORDS: tuple[tuple[str, str], ...] = (
    ("an ordinary passphrase", "correct horse battery staple"),
    ("the shortest a route will store", "twelve chars"),
    ("below the floor, still verifiable", "short"),
    ("empty, which verify must not crash on", ""),
    ("unicode above the BMP", "🜁 quicksilver 🜃 the alchemist's word"),
    ("a colon and a dollar, which PHC delimits on", "a$b:c$d$e:f$gh"),
    ("at the byte ceiling", "x" * 1024),
)

#: Salts, fixed so the fixture is reproducible and the *encoding* is what is
#: under test rather than the entropy source. 16 bytes is argon2-cffi's default
#: and therefore the project's.
_ARGON2_SALTS: tuple[bytes, ...] = tuple(
    bytes(((i * 37 + j * 11) % 256) for j in range(16))
    for i in range(len(_ARGON2_PASSWORDS))
)


def crypto_cases() -> dict[str, Any]:
    """The Argon2id and SHA-256 oracle: what `mtglab.auth` writes, exactly.

    This is the fixture behind the migration's sharpest compatibility claim.
    ADR 38 promises "Argon2id PHC hashes verify as-is", which the read side
    proved in Phase 2 -- but Phase 4's accounts flip makes Go a *writer*, and a
    hash Go writes has to be one `argon2-cffi` verifies for the rest of time,
    including after a rollback to a Python-only door.

    A round trip in one direction would not settle it. So the record is
    stronger: each case pins the **exact PHC string** `argon2-cffi` produces
    for a given password *and a given salt*, which turns the question from "do
    both sides accept each other's output" into "are both sides the same
    function". Go recomputes the string from the recorded salt and must match
    it byte for byte; a Go-written hash then differs from a Python-written one
    only in the random salt, which travels inside the string.

    Two assertions run here rather than in a test, because a fixture that
    records something the project does not itself produce would prove the wrong
    thing: every encoded hash must verify under the project's own
    `PasswordHasher`, and none may be reported as needing a rehash -- which is
    what pins the parameters to `passwords.MEMORY_COST_KIB` and friends rather
    than to numbers written twice.

    The SHA-256 half is the token hash both `sessions.py` and `tokens.py`
    store. It is a one-liner on both sides and it is recorded anyway, because
    it is the other thing that would let a Go-minted session or invite be
    invisible to Python -- and a fixture is cheaper than finding out.
    """
    from argon2.low_level import Type, hash_secret

    from mtglab.auth import passwords, sessions, tokens

    cases: list[dict[str, Any]] = []
    for (note, password), salt in zip(_ARGON2_PASSWORDS, _ARGON2_SALTS,
                                      strict=True):
        encoded = hash_secret(
            password.encode("utf-8"), salt,
            time_cost=passwords.TIME_COST,
            memory_cost=passwords.MEMORY_COST_KIB,
            parallelism=passwords.PARALLELISM,
            hash_len=32,          # argon2-cffi's DEFAULT_HASH_LENGTH
            type=Type.ID,
        ).decode("ascii")
        # The fixture must be the project's own output, not merely valid
        # Argon2: verify it through the hasher the app actually holds.
        assert passwords.verify(encoded, password), note
        assert not passwords.needs_rehash(encoded), (
            f"{note}: the recorded parameters are not the ones "
            "`passwords.py` asks for")
        cases.append({
            "note": note,
            "password": password,
            "salt_b64": base64.b64encode(salt).decode("ascii"),
            "hash": encoded,
        })

    # Both modules hash a token the same way; recorded through both names so a
    # divergence between them would fail here rather than in production.
    token_inputs = ("", "a", "x" * 43, "🜂-a-token-with-unicode-in-it")
    digests = [{"input": raw,
                "digest": sessions._hash(raw)} for raw in token_inputs]
    for entry in digests:
        assert tokens._hash(entry["input"]) == entry["digest"]

    return {
        "argon2id": {
            "params": {
                "memory_cost_kib": passwords.MEMORY_COST_KIB,
                "time_cost": passwords.TIME_COST,
                "parallelism": passwords.PARALLELISM,
                "salt_bytes": 16,
                "hash_bytes": 32,
            },
            "min_password_length": passwords.MIN_PASSWORD_LENGTH,
            "max_password_bytes": passwords.MAX_PASSWORD_BYTES,
            "cases": cases,
        },
        "sha256_hex": digests,
    }


def render_crypto_cases() -> str:
    return json.dumps(crypto_cases(), indent=1, ensure_ascii=False) + "\n"



# ------------------------------------------------------------- the stance dial

#: Where `internal/claude`'s stance corpus lands.
SOURCES_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "sources.json"


def canonical_url_cases() -> list[dict[str, str]]:
    """`dossier.canonical_url`, which decides when two URLs are one page.

    The half a port gets wrong is the asymmetry: with a scheme, only the scheme
    and host are lowercased and THE PATH KEEPS ITS CASE; with no scheme, the
    whole string is lowercased. That is not a tidy rule and it is the rule.
    """
    from mtglab.claude.dossier import canonical_url
    raw = [
        "https://example.com/a", "https://example.com/a/", "HTTPS://EXAMPLE.COM/a",
        "https://EXAMPLE.com/A", "https://example.com/A/", "https://example.com",
        "https://example.com/", "  https://example.com/a  ",
        "http://example.com/a", "example.com/A", "EXAMPLE.COM/A", "",
        "   ", "/", "//", "https://example.com//a//",
        "https://example.com/a?b=C#D", "ftp://Example.COM/Path",
        "https://sub.Example.com/Deep/Path/Here",
    ]
    return [{"url": u, "canonical": canonical_url(u)} for u in raw]


def keep_sources_cases() -> list[dict[str, Any]]:
    """`dossier.keep_sources`: the intersection that puts something behind a URL.

    A response schema suppresses the API's own citations, so a URL in the
    payload is otherwise just a string the model typed.
    """
    from mtglab.claude.dossier import keep_sources
    searched = [
        {"url": "https://example.com/one", "title": "The First Page"},
        {"url": "https://example.com/two/", "title": "The Second Page"},
        {"url": "https://other.example/deep/Path", "title": ""},
        # Absurd on its face, and the only thing that separates the two str()
        # spellings for `url`. `str(get("url", ""))` turns an explicit null into
        # the four-letter string "None", which then MATCHES this page;
        # `str(get("url") or "")` turns it into "" and drops it. Every other
        # input drops either way, so without this page the spelling is
        # unobservable and a mutation harmonising them survives.
        {"url": "None", "title": "A Page Called None"},
    ]
    cases: list[dict[str, Any]] = []

    def add(note: str, claimed: Any) -> None:
        kept, dropped = keep_sources(claimed, searched)
        cases.append({"note": note, "claimed": claimed, "searched": searched,
                      "kept": kept, "dropped": dropped})

    add("a page the search returned",
        [{"id": "s1", "title": "the model's title", "url": "https://example.com/one"}])
    add("nothing claimed", [])
    add("a page the search never returned",
        [{"id": "s1", "title": "t", "url": "https://invented.example/page"}])
    add("a trailing slash is the same page",
        [{"id": "s1", "title": "t", "url": "https://example.com/two"}])
    add("the host is case-insensitive",
        [{"id": "s1", "title": "t", "url": "https://EXAMPLE.com/one"}])
    # The rule that keeps a citation meaning something.
    add("a different path on the same site is a different page",
        [{"id": "s1", "title": "t", "url": "https://example.com/three"}])
    add("no id falls back to the url",
        [{"title": "t", "url": "https://example.com/one"}])
    add("a blank id falls back to the url",
        [{"id": "   ", "title": "t", "url": "https://example.com/one"}])
    # The search's own title wins -- one is a fact about the page, the other a
    # description of it.
    add("the search's title beats the model's",
        [{"id": "s1", "title": "the model's title", "url": "https://example.com/two"}])
    add("an empty search title falls back to the model's",
        [{"id": "s1", "title": "the model's title", "url": "https://other.example/deep/Path"}])
    add("no url at all", [{"id": "s1", "title": "t"}])
    add("a blank url", [{"id": "s1", "title": "t", "url": "  "}])
    add("a non-object is dropped", ["https://example.com/one", 7, None])
    add("mixed", [{"id": "s1", "url": "https://example.com/one", "title": "t"},
                  {"id": "s2", "url": "https://nope.example/x", "title": "t"},
                  "not an object"])
    # **The two str() spellings, which differ on exactly one input.** `url` is
    # `str(get("url", ""))` and reaches `str(None)`; `id` and `title` are
    # `str(get(k) or "")` and never do. A port that harmonised them would put
    # the four-letter string "None" in an id, or lose it from a url.
    add("a null url is the string None",
        [{"id": "s1", "title": "t", "url": None}])
    add("a null id falls back to the url, not to None",
        [{"id": None, "title": "t", "url": "https://example.com/one"}])
    add("a null title falls back, not to None",
        [{"id": "s1", "title": None, "url": "https://other.example/deep/Path"}])
    add("a numeric id is rendered",
        [{"id": 7, "title": "t", "url": "https://example.com/one"}])
    return cases


def grounded_cases() -> list[dict[str, Any]]:
    """`dossier._section` beside `research.only_grounded`, the ADR 26 asymmetry.

    The SAME input goes to both, because the point is that they answer
    differently: a dossier passage may rest on its brief, so uncited prose
    survives; a research finding has no brief, so uncited prose is resting on
    the model's recall and is dropped and counted.
    """
    from mtglab.claude.dossier import _section
    from mtglab.claude.research import only_grounded
    allowed = {"s1", "s2", "3"}
    # **Each item carries BOTH keys**, and that is the whole point rather than
    # belt and braces: a dossier passage reads `prose` and a research finding
    # reads `claim`, so an item with only one of them exercises one function and
    # silently degenerates the other. The first draft of this corpus carried
    # `prose` alone -- every `only_grounded` row came back dropped for a missing
    # claim, the asymmetry looked real, and a mutation deleting the citation
    # check survived because the claim check was already dropping everything.
    inputs: list[tuple[str, Any]] = [
        ("cited, and the citation survived",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": ["s1"]}),
        ("cited, but every citation was dropped",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": ["gone"]}),
        ("some citations survived",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": ["s1", "gone", "s2"]}),
        ("no citations at all",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": []}),
        ("no source_ids key", {"prose": "A sentence.", "claim": "A sentence."}),
        ("empty text, good citation",
         {"prose": "   ", "claim": "   ", "source_ids": ["s1"]}),
        # A numeric id that IS allowed, so `str(i) in allowed` is exercised
        # rather than merely executed -- with `allowed` holding "3", an id of 3
        # survives only if it was stringified first.
        ("a numeric id that survives",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": [3]}),
        ("a numeric id that does not",
         {"prose": "A sentence.", "claim": "A sentence.", "source_ids": [1, 2]}),
        ("not an object", "a bare string"),
        # `_section` is `str(get("prose") or "")` and `only_grounded` is
        # `str(get("claim", ""))`, so a null prose is "" and a null claim is
        # the four-letter string "None" -- which is truthy, so the finding
        # survives on its claim being literally "None".
        ("a numeric text is rendered",
         {"prose": 42, "claim": 42, "source_ids": ["s1"]}),
        ("a null prose is empty, a null claim is None",
         {"prose": None, "claim": None, "source_ids": ["s1"]}),
        ("a boolean text", {"prose": True, "claim": True, "source_ids": ["s1"]}),
    ]
    cases = []
    for note, item in inputs:
        section = _section(item, allowed)
        kept, dropped = only_grounded([item], allowed)
        cases.append({
            "note": note, "item": item, "allowed": sorted(allowed),
            # The dossier keeps the prose either way.
            "section": section,
            # Research keeps it only if something survived.
            "grounded": kept, "grounded_dropped": dropped,
        })
    return cases


def pool_name_cases() -> list[dict[str, Any]]:
    """`dossier._competitors` beside `research.resolve_cards`, over one pool.

    The second ADR 26 asymmetry, and one REAL DIVERGENCE that is a Python bug
    rather than a design: `_competitors` indexes only the pool's own spelling,
    where `resolve_cards` (and `argue.resolve_alternatives`) index `asked_as`
    too. So a double-faced card named by its FRONT FACE resolves in research and
    is dropped as invented by the dossier. Measured, pinned, and raised -- not
    fixed here, because fixing one runtime would put the two out of step.
    """
    from mtglab.claude.dossier import _competitors
    from mtglab.claude.research import resolve_cards
    allowed = {"s1"}
    cases: list[dict[str, Any]] = []

    def add(note: str, name: Any) -> None:
        comp, comp_dropped = _competitors(
            [{"card": name, "prose": "p", "source_ids": ["s1"]}], allowed)
        cards, unresolved = resolve_cards([name])
        cases.append({
            "note": note, "name": name, "allowed": sorted(allowed),
            "competitors": comp, "competitors_dropped": comp_dropped,
            "research_cards": cards, "research_unresolved": unresolved,
        })

    add("an ordinary card", "Craterhoof Behemoth")
    add("a card the pool lacks", "Not A Real Card")
    add("a banned card still resolves", "Primeval Titan")
    # The divergence.
    add("a DFC by its front face", "Ajani, Nacatl Pariah")
    add("a DFC by its full name", "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger")
    add("case does not matter", "craterhoof behemoth")
    add("a numeric card name", 7)
    return cases


def sources_cases() -> dict[str, Any]:
    import tiny_pool
    from mtglab import config
    with tempfile.TemporaryDirectory() as tmp:
        tiny_pool.build(Path(tmp) / "mtg.duckdb")
        with config.use_paths(data_dir=Path(tmp)):
            pool_cases = pool_name_cases()
    return {
        "canonical": canonical_url_cases(),
        "keep": keep_sources_cases(),
        "grounded": grounded_cases(),
        "pool": pool_cases,
    }


def render_sources_cases() -> str:
    return _rows_json({
        "note": "The two hosted-search modes' shared instruments and their two "
                "deliberate asymmetries. Written by "
                "`python tests/go_fixtures.py`.",
        "why": "A response schema suppresses the API's own citations, so a URL "
               "in a payload has nothing behind it but the model's word; "
               "keep_sources is what puts something behind it. Past that the "
               "two modes diverge on purpose (a dossier passage may rest on "
               "its brief, a research finding may not; an unresolved rival is "
               "invented, an unresolved research card may be spoiled) and once "
               "BY ACCIDENT -- _competitors does not index `asked_as`, so a "
               "DFC named by its front face is dropped there and resolved in "
               "research. The three payload key ORDERS differ and all three "
               "are the wire.",
        **sources_cases(),
    }) + "\n"


ARGUE_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "argue.json"


def only_charges_cases() -> list[dict[str, Any]]:
    """`argue.only_charges` over what a model might actually return.

    The predicate here is not the interview's. Every item this mode returns is
    declarative by design, so "does it end in a question mark" would delete the
    feature; what separates an argument from an opinion is **whether it cites
    anything**, and a charge with no `fact` is not a charge.

    The enum handling is the half worth pinning: an unrecognised `ground` or
    `strength` FALLS BACK rather than dropping the charge, because a labelling
    miss is not a reason to throw away a cited argument -- and a port that
    dropped it instead would pass any test that only counted the good ones.
    """
    from mtglab.claude.argue import GROUNDS, STRENGTHS, only_charges
    cases: list[dict[str, Any]] = []

    def add(note: str, items: Any) -> None:
        kept, dropped = only_charges(items)
        cases.append({"note": note, "items": items,
                      "kept": kept, "dropped": dropped})

    good = {"claim": "Six other cards already do this.", "ground": "redundancy",
            "fact": "ramp holds 12 against a target of 8-12.",
            "strength": "serious"}
    add("a whole charge", [good])
    add("nothing at all", [])
    add("no fact is not a charge", [{"claim": "It is simply bad.",
                                     "ground": "cost", "strength": "minor"}])
    add("an empty fact is no fact", [dict(good, fact="   ")])
    add("no claim is not a charge", [dict(good, claim="")])
    add("an unknown ground falls back to ceiling", [dict(good, ground="vibes")])
    add("an unknown strength falls back to minor", [dict(good, strength="fatal")])
    add("a missing ground falls back", [{"claim": good["claim"], "fact": good["fact"]}])
    add("whitespace is stripped", [{"claim": "  spaced  ", "ground": " cost ",
                                    "fact": "  a fact  ", "strength": " minor "}])
    add("a non-object is dropped", ["a charge", 7, None, []])
    add("mixed: two kept, three dropped",
        [good, "nope", dict(good, fact=""), dict(good, claim="Another."), {}])
    # Every ground and every strength, so a transcription slip in either table
    # is a failure rather than a coin flip.
    for ground in GROUNDS:
        add(f"ground {ground}", [dict(good, ground=ground)])
    for strength in STRENGTHS:
        add(f"strength {strength}", [dict(good, strength=strength)])
    return cases


def resolve_alternative_cases() -> list[dict[str, Any]]:
    """`argue.resolve_alternatives` against the 21-card pool.

    Rule 2 made executable, and the reason the function exists: CLAUDE.md's
    first recorded error is *Ajani, Nacatl Pariah* proposed for a G/W deck,
    whose back face is R/W. `tiny_pool` carries that exact card, so the case is
    the real one rather than a stand-in -- and it is asked for **by its front
    face**, which is how a model would name it and the one spelling an index
    keyed only on the pool's `A // B` name would miss silently.

    The ORDER of the verdicts is pinned too. Primeval Titan is banned; put it
    in the deck as well and the answer must be `already_in_deck`, because that
    is the most specific true thing to say.
    """
    from mtglab.claude.argue import resolve_alternatives
    cases: list[dict[str, Any]] = []

    def add(note: str, names: Any, identity: list[str],
            in_deck: set[str] = frozenset()) -> None:
        kept, dropped = resolve_alternatives(names, identity=identity,
                                             in_deck=in_deck)
        cases.append({
            "note": note, "names": names, "identity": identity,
            "in_deck": sorted(in_deck),
            "kept": [c["name"] for c in kept], "dropped": dropped,
        })

    green = ["G"]
    add("a green card for a green deck", ["Craterhoof Behemoth"], green)
    add("nothing asked", [], green)
    add("a card nobody has heard of", ["Bolas's Citadel of Lies"], green)
    add("banned is banned", ["Primeval Titan"], green)
    # The whole reason this function exists.
    add("the DFC by its front face, off-colour",
        ["Ajani, Nacatl Pariah"], green)
    add("the DFC by its full name, off-colour",
        ["Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"], green)
    add("the same DFC is in identity for a Boros deck",
        ["Ajani, Nacatl Pariah"], ["R", "W"])
    add("colourless fits any identity", ["Sol Ring"], green)
    add("already in the deck beats banned",
        ["Primeval Titan"], green, {"primeval titan"})
    add("already in the deck beats off-colour",
        ["Ajani, Nacatl Pariah"], green, {"ajani, nacatl pariah // ajani, nacatl avenger"})
    add("both faces of one card are one card",
        ["Ajani, Nacatl Pariah", "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"],
        ["R", "W"])
    add("case and whitespace do not matter",
        ["  craterhoof behemoth  "], green)
    add("duplicates are asked once", ["Sol Ring", "sol ring", "SOL RING"], green)
    # And the case that makes the INPUT dedupe observable at all. Three
    # spellings of a card that resolves collapse anyway, on the pool's name,
    # further down -- so dropping the `seen` check changes nothing there and a
    # mutation of it survives. Three spellings of a card that does NOT resolve
    # have nothing to collapse them, so `not_in_pool` lists it once or three
    # times depending on whether the check is there.
    add("duplicate names that do not resolve are still counted once",
        ["Ghost Card", "ghost card", "GHOST CARD"], green)
    add("empty and blank names are skipped", ["", "   ", None, "Sol Ring"], green)
    add("a mixed bag", ["Craterhoof Behemoth", "Primeval Titan",
                        "Ajani, Nacatl Pariah", "Not A Real Card",
                        "Terastodon"], green)
    add("more than the cap", ["Craterhoof Behemoth", "Terastodon",
                              "Woodfall Primus", "Vorinclex, Voice of Hunger",
                              "Regal Behemoth", "Cultivator Colossus",
                              "Goreclaw, Terror of Qal Sisma"], green)
    add("an empty identity admits only colourless",
        ["Sol Ring", "Craterhoof Behemoth"], [])
    return cases


def argue_cases() -> dict[str, Any]:
    import tiny_pool
    from mtglab import config
    from mtglab.cards import db
    with tempfile.TemporaryDirectory() as tmp:
        pool_path = tiny_pool.build(Path(tmp) / "mtg.duckdb")
        con = db.connect_readonly(pool_path)
        try:
            with config.use_paths(data_dir=Path(tmp)):
                alternatives = resolve_alternative_cases()
        finally:
            con.close()
    # The no-pool answer is its own case and cannot be produced beside a pool:
    # every name comes back unresolved, and filing that under `not_in_pool`
    # would accuse the model of inventing six cards.
    from mtglab.claude.argue import resolve_alternatives
    with tempfile.TemporaryDirectory() as tmp:
        from mtglab import config as cfg
        with cfg.use_paths(data_dir=Path(tmp)):
            kept, dropped = resolve_alternatives(
                ["Sol Ring", "Craterhoof Behemoth"], identity=["G"])
            no_pool = {"note": "no pool at all", "names": ["Sol Ring", "Craterhoof Behemoth"],
                       "identity": ["G"], "in_deck": [],
                       "kept": [c["name"] for c in kept], "dropped": dropped}
    return {"charges": only_charges_cases(),
            "alternatives": [*alternatives, no_pool]}


def render_argue_cases() -> str:
    return _rows_json({
        "note": "`argue.only_charges` and `argue.resolve_alternatives`, ADR "
                "25's two Python-owned halves. Written by "
                "`python tests/go_fixtures.py`.",
        "why": "The charges half is the citation predicate and the two enum "
               "fallbacks. The alternatives half is rule 2 made executable: "
               "resolved through the pool, dropped and COUNTED SEPARATELY for "
               "each of four reasons, because `you invented that card` and "
               "`that card is off-colour` are different failures. The order "
               "of the verdicts is part of it -- already-in-deck is checked "
               "first because it is the most specific true thing to say.",
        **argue_cases(),
    }) + "\n"


DOSSIER_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "dossier.json"
RESEARCH_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "research.json"

#: Where the scan corpus lands (ADR 34, the seventh mode).
SCAN_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "scan.json"

#: One instant for every stamp in the two corpora below. `generated_at` and a
#: stored row's `created_at` are `datetime.now(UTC).isoformat()`; the Go tests
#: freeze their clock to this same string, so a whole report compares as bytes.
FROZEN_NOW = "2026-08-23T04:05:06.789012+00:00"

#: The dossier tests' deck: Gyome in front of ninety-nine Swamps, which is all
#: a brief needs and nothing a gate would pass. `tiny_pool` carries Gyome.
MINI_DECK = """\
slug: mini
name: Mini Deck
status: theoretical
stage: curated
commander:
  - Gyome, Master Chef
cards:
  - name: Swamp
    category: land
    qty: 99
    why: Black mana.
"""
HEADLESS_DECK = MINI_DECK.replace("commander:\n  - Gyome, Master Chef\n",
                                  "commander: []\n")


class _ClaudeScratch:
    """The tiny pool, a scratch `app.db`, frozen clocks, and no environment
    overrides -- everything the dossier and research corpora need held still.

    A context manager written out rather than decorated so the two module
    clocks are restored whatever the body does; `config.use_paths` puts the
    dossier store under the scratch data dir exactly as the dossier tests do.
    """

    _ENV = ("MTGLAB_CLAUDE_STANCE_CEILING", "MTGLAB_CLAUDE_MODEL")

    def __enter__(self) -> None:
        import tiny_pool
        from mtglab import config
        from mtglab.claude import dossier, research
        self._saved = {k: os.environ.pop(k, None) for k in self._ENV}
        self._clocks = (dossier._now, research._now)
        dossier._now = research._now = lambda: FROZEN_NOW
        self._tmp = tempfile.TemporaryDirectory()
        root = Path(self._tmp.name)
        tiny_pool.build(root / "mtg.duckdb")
        self._paths = config.use_paths(data_dir=root)
        self._paths.__enter__()

    def __exit__(self, *exc: Any) -> None:
        from mtglab.claude import dossier, research
        self._paths.__exit__(*exc)
        self._tmp.cleanup()
        dossier._now, research._now = self._clocks
        for key, value in self._saved.items():
            if value is not None:
                os.environ[key] = value


def _fake_turn(payload: Any = None, *, mode: str, text: str | None = None,
               stop_reason: str = "end_turn", refused: bool = False,
               searched: list[dict[str, str]] | None = None,
               search_errors: list[str] | None = None,
               model: str = "claude-sonnet-5", input_tokens: int = 1234,
               output_tokens: int = 567, cache_read_tokens: int = 89) -> Any:
    """A `Turn` as `converse` would return it, without the call."""
    from mtglab.claude.modes import Turn
    return Turn(mode=mode, model=model, stop_reason=stop_reason,
                text=json.dumps(payload) if text is None else text,
                tool_calls=[], input_tokens=input_tokens,
                output_tokens=output_tokens, searched=list(searched or []),
                search_errors=list(search_errors or []), refused=refused,
                cache_read_tokens=cache_read_tokens)


def _turn_record(turn: Any) -> dict[str, Any]:
    """What Go needs to rebuild the same Turn: everything the readers look at."""
    return {"model": turn.model, "stop_reason": turn.stop_reason,
            "text": turn.text, "refused": turn.refused,
            "input_tokens": turn.input_tokens,
            "output_tokens": turn.output_tokens,
            "cache_read_tokens": turn.cache_read_tokens,
            "searched": turn.searched, "search_errors": turn.search_errors}


_PAGES = [{"url": "https://edhrec.com/real", "title": "The Real Page"},
          {"url": "https://mtg.wiki/Gyome", "title": ""}]


def dossier_cases() -> dict[str, Any]:
    """`claude/dossier.py`'s Python-owned halves, held still for the Go port.

    Four tables. **`keys`** is `cache_key` over oracle ids and tiers, with the
    fingerprint's parts recorded apart so a failure says which half is wrong --
    and it matters more than any other key in the port, because a dossier
    Python wrote is served by Go under it after the cutover. **`brief`** is the
    whole opening message for the tiny pool's Gyome, bytes and all.
    **`cached_get`** is the free route's three shapes. **`reports`** is every
    outcome of a run, driven through a fake `converse` with the clock frozen,
    so the Go side can rebuild each Turn and compare the report it writes as
    marshalled bytes -- key order included, which is the wire.
    """
    from mtglab.api import dossierruns, service
    from mtglab.claude import client, dossier, tiers
    from mtglab.decks.model import Deck
    from mtglab.decks.source import MemoryDeckSource

    with _ClaudeScratch():
        keys = []
        for oracle_id in ("", "abc", "a6c1b2d3-0000-4000-8000-000000000042"):
            for tier in (None, *[t["key"] for t in tiers.roster()], "not-a-tier"):
                keys.append({"oracle_id": oracle_id, "tier": tier,
                             "key": dossier.cache_key(oracle_id, tier)})
        os.environ["MTGLAB_CLAUDE_MODEL"] = "claude-test-1"
        try:
            override = [{"oracle_id": "abc", "tier": tier,
                         "key": dossier.cache_key("abc", tier)}
                        for tier in (None, "opus")]
        finally:
            del os.environ["MTGLAB_CLAUDE_MODEL"]
        fingerprint = {
            "version": dossier.DOSSIER_VERSION,
            "instructions_sha256": hashlib.sha256(
                dossier.INSTRUCTIONS.encode()).hexdigest(),
            "schema_dumps": json.dumps(dossier.RESPONSE_SCHEMA, sort_keys=True),
            "model": client.model(None),
            "fingerprint": dossier._fingerprint(None),
        }

        mini = MemoryDeckSource([Deck.from_text(MINI_DECK, slug="mini")])
        headless = MemoryDeckSource([Deck.from_text(HEADLESS_DECK, slug="mini")])
        facts = dossier.brief("mini", source=mini)
        brief = {"slug": "mini", "commander": facts["card"]["name"],
                 "oracle_id": facts["card"]["oracle_id"], "facts": facts,
                 "opening": dossier._ask_for(facts),
                 "label": dossierruns.plan_dossier(
                     slug="mini", requested="off", source=mini).label}
        try:
            dossier.brief("mini", source=headless)
            raise AssertionError("a headless deck must refuse")
        except dossier.NoCommander as exc:
            brief["headless_refusal"] = str(exc)

        cached_get = [
            {"note": "no row yet",
             "payload": service.claude_dossier_cached(slug="mini", source=mini)},
            {"note": "no commander the pool knows",
             "payload": service.claude_dossier_cached(slug="mini", source=headless)},
        ]

        reports: list[dict[str, Any]] = []
        real_converse = dossier.converse

        def run(note: str, turn: Any, *, requested: Any = "consultant",
                refresh: bool = True) -> dict[str, Any]:
            def fake(*_a: Any, **_k: Any) -> Any:
                if turn is None:
                    raise AssertionError(f"{note}: no call may be made")
                return turn
            dossier.converse = fake
            try:
                report = dossier.ask("mini", requested=requested,
                                     refresh=refresh, source=mini)
            finally:
                dossier.converse = real_converse
            row = {"note": note, "requested": requested, "refresh": refresh,
                   "turn": _turn_record(turn) if turn is not None else None,
                   "report": report}
            reports.append(row)
            return report

        mode = dossier.COMMANDER_DOSSIER.name
        run("stance off, no call", None, requested="off")
        run("the model refused", _fake_turn(
            mode=mode, text="", stop_reason="refusal", refused=True))
        run("the answer did not parse", _fake_turn(
            mode=mode, text='{"who": {"prose": "trunc', stop_reason="max_tokens"))
        run("no source survived", _fake_turn({
            "who": {"prose": "A troll.", "source_ids": ["s1"]},
            "archetype": {"name": "Food", "prose": "Food.", "source_ids": ["s1"]},
            "competitors": [], "allies": {"prose": "", "source_ids": []},
            "rivals": {"prose": "None worth the name.", "source_ids": []},
            "standing": {"prose": "Niche.", "source_ids": ["s1"]},
            "sources": [{"id": "s1", "title": "Made up",
                         "url": "https://example.com/never-fetched"}],
        }, mode=mode, searched=_PAGES[:1]))
        run("no source, and the search itself failed", _fake_turn({
            "who": {"prose": "A troll.", "source_ids": []},
            "archetype": {"name": "", "prose": "", "source_ids": []},
            "competitors": [], "allies": {"prose": "", "source_ids": []},
            "rivals": {"prose": "", "source_ids": []},
            "standing": {"prose": "", "source_ids": []}, "sources": [],
        }, mode=mode, searched=[], search_errors=["max_uses_exceeded", "unavailable"]))
        whole = run("a whole dossier", _fake_turn({
            "who": {"prose": "  A troll chef of Kaldheim.  ",
                    "source_ids": ["s1", "s2", 3]},
            "archetype": {"name": " Food aristocrats ", "prose": "Sacrifice Food.",
                          "source_ids": ["s2", "s1"]},
            "competitors": [
                {"card": "Craterhoof Behemoth", "prose": "Goes wide.",
                 "source_ids": ["s1"]},
                # A double-faced card by its FRONT face -- resolves since
                # 2026-08-23, when `_competitors` learned `asked_as`.
                {"card": "Ajani, Nacatl Pariah", "prose": "Cats.",
                 "source_ids": ["s2"]},
                {"card": "Not A Real Card", "prose": "x", "source_ids": ["s1"]},
                {"card": "Primeval Titan", "prose": "Banned, still real.",
                 "source_ids": [3]},
                {"card": 7, "prose": "a number", "source_ids": ["s1"]},
                "not an object",
            ],
            "allies": {"prose": "The kitchen crew.", "source_ids": ["s1"]},
            "rivals": {"prose": "Nobody in particular.", "source_ids": []},
            "standing": {"prose": "A precon face.", "source_ids": ["3"]},
            "sources": [
                {"id": "s1", "title": "the model's title", "url": "https://edhrec.com/real"},
                {"id": "s2", "title": "t", "url": "https://example.com/invented"},
                {"id": 3, "title": "the model's title", "url": "https://mtg.wiki/Gyome/"},
            ],
        }, mode=mode, searched=_PAGES, input_tokens=4321, output_tokens=987,
            cache_read_tokens=2791))
        key = dossier.cache_key(brief["oracle_id"])
        stored = dossier.get(key)
        assert stored is not None and stored["created_at"] == FROZEN_NOW
        run("served from the store", None, requested="consultant", refresh=False)
        cached_get.append({"note": "a stored row",
                           "payload": service.claude_dossier_cached(slug="mini", source=mini)})
        assert whole["dossier"]["competitors"][1]["name"].startswith("Ajani")

    return {"keys": keys, "keys_with_model_override": override,
            "fingerprint": fingerprint, "brief": brief, "cached_get": cached_get,
            "stored": {"key": key, **stored}, "reports": reports}


def render_dossier_cases() -> str:
    return _rows_json({
        "note": "`claude/dossier.py`'s Python-owned halves: the cache key, the "
                "brief's opening message, the free GET's shapes, and every "
                "outcome of a run through a fake converse with the clock frozen "
                f"at {FROZEN_NOW}. Written by `python tests/go_fixtures.py`.",
        "why": "A dossier Python wrote is served by Go after the cutover under "
               "the SAME key, so the fingerprint is Python's byte for byte -- "
               "version, prompt, `json.dumps(schema, sort_keys=True)`, model id "
               "-- and its parts are recorded apart so a failure localises. The "
               "reports are compared as marshalled bytes, key order included, "
               "because the report is the wire and `tier1.Number` taught that a "
               "value can be right and still go out wrong.",
        **dossier_cases(),
    }) + "\n"


def research_cases() -> dict[str, Any]:
    """`claude/research.py`'s Python-owned halves, held still for the Go port.

    `check_question` over every shape a body can carry (a number, a list, an
    explicit null, control characters Python counts as whitespace), the
    in-flight key with its whitespace and case normalisation, the job label's
    sixty-character cut, the default stance and its clamp, and every outcome
    of a run through a fake `converse`. The one recorded GAP is `casefold`:
    Python folds and Go lowercases, and the `casefold_gap` row is the input
    where they differ, pinned so the difference is known rather than found --
    the key never leaves the process, so nothing can observe it.
    """
    from mtglab.api import researchruns
    from mtglab.claude import research
    from mtglab.claude import stance as st

    with _ClaudeScratch():
        questions: list[dict[str, Any]] = []
        for raw in [None, "", "   ", "  Is Goreclaw still played?  ", 7, True,
                    False, 0, "x" * research.MAX_QUESTION,
                    "x" * (research.MAX_QUESTION + 1),
                    "é" * research.MAX_QUESTION,
                    "é" * (research.MAX_QUESTION + 1),
                    "\x1c\x1d", "　tabs\tand nbsp　",
                    ["a", 1, None, True], "\x1fq\x1f"]:
            try:
                questions.append({"raw": raw,
                                  "question": research.check_question(raw)})
            except research.QuestionRejected as exc:
                questions.append({"raw": raw, "rejected": str(exc)})

        keys = [{"question": q, "key": research.question_key(q)} for q in (
            "Is Goreclaw still played?", "  is   GORECLAW   still played?  ",
            "is goreclaw\tstill\nplayed?", "Is Goreclaw still played",
            "ÉCLAIR ou éclair?", " spaced out\x1cwide",
            "x" * 50)]
        casefold_gap = {"question": "Straße?",
                        "key": research.question_key("Straße?"),
                        # What Go computes instead: the same normalisation
                        # with `lower()` where Python has `casefold()`.
                        "lowercased_key": "research:" + hashlib.sha256(
                            "Straße?".lower().encode()).hexdigest()[:16]}

        labels = []
        for q in ("Is Goreclaw still played?", "a" * 60, "a" * 61,
                  "a" * 58 + "  " + "b" * 10, "é" * 70,
                  "word " * 20):
            plan = researchruns.plan_research(question=q, requested="off")
            labels.append({"question": q, "label": plan.label})

        stance_for: list[dict[str, Any]] = []
        for ceiling in (None, "consultant"):
            if ceiling is None:
                os.environ.pop(st.CEILING_ENV, None)
            else:
                os.environ[st.CEILING_ENV] = ceiling
            try:
                for requested in (None, "off", "consultant", "collaborator",
                                  {"initiative": "on-request"}):
                    stance_for.append({
                        "ceiling": ceiling, "requested": requested,
                        "describe": st.describe(research.stance_for(requested))})
            finally:
                os.environ.pop(st.CEILING_ENV, None)

        ask_for = [{"question": q, "message": research._ask_for(q)}
                   for q in ("Is Goreclaw still played?", "two\nlines")]

        reports: list[dict[str, Any]] = []
        real_converse = research.converse

        def run(note: str, turn: Any, *, question: str = "Is Goreclaw still played?",
                requested: Any = None) -> dict[str, Any]:
            def fake(*_a: Any, **_k: Any) -> Any:
                if turn is None:
                    raise AssertionError(f"{note}: no call may be made")
                return turn
            research.converse = fake
            try:
                report = research.ask(question, requested=requested)
            finally:
                research.converse = real_converse
            reports.append({"note": note, "question": question,
                            "requested": requested,
                            "turn": _turn_record(turn) if turn is not None else None,
                            "report": report})
            return report

        mode = research.RESEARCH.name
        run("stance off, no call", None, requested="off")
        run("the model refused", _fake_turn(
            mode=mode, text="", stop_reason="refusal", refused=True))
        run("the answer did not parse", _fake_turn(
            mode=mode, text='{"answer": "trunc', stop_reason="max_tokens"))
        run("no source survived", _fake_turn({
            "answer": "Confidently wrong.",
            "findings": [{"claim": "Everyone says so.", "source_ids": ["s1"]}],
            "cards": [], "confidence": "settled",
            "sources": [{"id": "s1", "title": "Invented",
                         "url": "https://example.com/never-fetched"}],
        }, mode=mode, searched=_PAGES[:1]))
        run("no source, and the search itself failed", _fake_turn({
            "answer": "", "findings": [], "cards": [], "confidence": "thin",
            "sources": [],
        }, mode=mode, searched=[], search_errors=["max_uses_exceeded"]))
        run("a grounded answer", _fake_turn({
            "answer": "  It is still played in stompy lists.  ",
            "findings": [
                {"claim": "cEDH primers rate it a trap.", "source_ids": ["s1"]},
                {"claim": "Rests on nothing.", "source_ids": []},
                {"claim": "Rests on an invented page.", "source_ids": ["s2"]},
                {"claim": "A numeric id that survives.", "source_ids": [3]},
                {"claim": "Four.", "source_ids": ["s1"]},
                {"claim": "Five.", "source_ids": ["s1", "s2"]},
                {"claim": "Six.", "source_ids": ["3"]},
                {"claim": "Seven, past the cap.", "source_ids": ["s1"]},
                "not an object",
            ],
            "cards": ["Craterhoof Behemoth", "Ajani, Nacatl Pariah",
                      "Spoiled Card", "craterhoof behemoth", 7,
                      "Primeval Titan", None, ""],
            "confidence": "vibes",
            "sources": [
                {"id": "s1", "title": "the model's title", "url": "https://edhrec.com/real"},
                {"id": "s2", "title": "t", "url": "https://example.com/invented"},
                {"id": 3, "title": "the model's title", "url": "https://mtg.wiki/Gyome/"},
            ],
        }, mode=mode, searched=_PAGES, input_tokens=4321, output_tokens=987,
            cache_read_tokens=2062))
        run("a contested answer with nothing named", _fake_turn({
            "answer": "People disagree.", "findings": [
                {"claim": "Some say yes.", "source_ids": ["s1"]}],
            "cards": [], "confidence": "contested",
            "sources": [{"id": "s1", "title": "", "url": "https://edhrec.com/real"}],
        }, mode=mode, searched=_PAGES[:1]), question="  a   question  with   spaces ",
            requested={"initiative": "on-request", "scope": "rethink"})
        # More grounded findings than the cap. The grounded case above lands
        # on EXACTLY six survivors, so the cap never trims there and a port
        # that forgot it would pass -- a mutation run said so. Here nine
        # survive the citation check and six reach the wire.
        run("more findings than the cap", _fake_turn({
            "answer": "Plenty.",
            "findings": [{"claim": f"Finding {i}.", "source_ids": ["s1"]}
                         for i in range(1, 10)],
            "cards": [], "confidence": "settled",
            "sources": [{"id": "s1", "title": "t", "url": "https://edhrec.com/real"}],
        }, mode=mode, searched=_PAGES[:1]))

    return {"questions": questions, "keys": keys, "casefold_gap": casefold_gap,
            "labels": labels, "stance_for": stance_for, "ask_for": ask_for,
            "reports": reports}


def render_research_cases() -> str:
    return _rows_json({
        "note": "`claude/research.py`'s Python-owned halves: the question "
                "check, the in-flight key, the job label, the default stance, "
                "the opening message, and every outcome of a run through a "
                f"fake converse with the clock frozen at {FROZEN_NOW}. Written "
                "by `python tests/go_fixtures.py`.",
        "why": "ADR 26's mode cannot see a deck, and nothing here takes one. "
               "What can vary is what a person typed: `str(raw or '')` over a "
               "number, a list or a null, `len()` in code points, whitespace "
               "as Python counts it (the information separators included). "
               "The reports compare as marshalled bytes, key order included. "
               "The one recorded gap is casefold versus lowercase in the "
               "in-flight key, which never leaves the process.",
        **research_cases(),
    }) + "\n"


STANCE_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "stance.json"


def _all_stances() -> list[Any]:
    """Every stance there is: 4 x 3 x 3 = 36, in axis order.

    Enumerated rather than sampled, and the whole point is that it *can* be.
    `mana_oracle.py`'s 13,944 cases had to be drawn because the space is
    unbounded, and the hole that left -- no case offering a wide pip before a
    narrow one -- was invisible for exactly as long as nobody asked what the
    drawing rule excluded. This space is 36. Excluding nothing costs nothing,
    so nothing is excluded, and no future session has to reason about what a
    sampler skipped.
    """
    from mtglab.claude import stance as st
    return [st.Stance(i, s, w)
            for i in st.INITIATIVE for s in st.SCOPE for w in st.WRITE]


def stance_cases() -> dict[str, Any]:
    """The dial, exhaustively: every stance, every clamp pair, every readout.

    Four tables, because a failure that names its table localises itself:

    * `stances` -- all 36, each with the two properties everything asks
      (`allows_calls`, `may_write`) and its full `describe()` payload. That
      payload is the one that reaches `/api/claude`, so it is recorded as the
      *serialised* shape rather than as fields: a Go struct whose tags are
      right and whose field order is wrong marshals to different bytes, and
      only comparing bytes sees it. (The lesson `tier1.Number` taught -- a type
      bit-exact by `repr` still went onto the wire as a struct -- one level up.)
    * `clamps` -- all 36 x 36 = 1,296 pairs. Per-axis minimum is four lines of
      code and it is the line an operator's cap runs through, so it is checked
      at every pair rather than at a chosen few.
    * `parses` -- what `from_obj` accepts and what it refuses, refusal text
      included. Those strings reach a 422 body.
    * `ceilings` -- what each value of the environment variable resolves to,
      including the two that matter most: unset (uncapped) and unreadable
      (`OFF`, failing closed).
    """
    from mtglab.claude import stance as st

    stances = [{
        "stance": s.to_dict(),
        "allows_calls": s.allows_calls,
        "may_write": s.may_write,
        # Serialised, not structured: see the docstring. `sort_keys=False`
        # keeps Python's insertion order, which is the order the client reads.
        "describe": json.dumps(st.describe(s), ensure_ascii=False,
                               separators=(",", ":"), sort_keys=False),
    } for s in _all_stances()]

    clamps = [{
        "requested": a.to_dict(), "limit": b.to_dict(),
        "clamped": st.clamp(a, b).to_dict(),
    } for a in _all_stances() for b in _all_stances()]

    # Every shape `from_obj` can be handed, including the ones that must fail.
    # `partial` maps are the load-bearing group: an axis left out takes OFF's
    # value, so a half-written request is quieter and never louder.
    parse_inputs: list[Any] = [
        None,
        "off", "consultant", "second-opinion", "collaborator",
        "  Consultant  ", "COLLABORATOR", "Second-Opinion",
        "", "  ", "nope", "Off ",
        {}, {"initiative": "volunteers"}, {"scope": "rethink"},
        {"write": "applies"}, {"initiative": "interjects", "write": "applies"},
        {"initiative": "on-request", "scope": "adjacent", "write": "proposes"},
        {"initiative": "nope"}, {"scope": "nope"}, {"write": "nope"},
        {"nope": "off"}, {"initiative": "off", "nope": "x", "also": "y"},
        {"initiative": 3}, 7, 7.5, True, ["off"],
    ]
    parses = []
    for raw in parse_inputs:
        row: dict[str, Any] = {"input": raw}
        try:
            row["stance"] = st.Stance.from_obj(raw).to_dict()
        except ValueError as exc:
            row["error"] = str(exc)
        parses.append(row)

    ceilings = []
    for raw in (None, "", "off", "consultant", "second-opinion",
                "collaborator", "  COLLABORATOR ", "nope", "0", "none"):
        before = os.environ.get(st.CEILING_ENV)
        try:
            if raw is None:
                os.environ.pop(st.CEILING_ENV, None)
            else:
                os.environ[st.CEILING_ENV] = raw
            ceilings.append({"env": raw, "ceiling": st.ceiling().to_dict()})
        finally:
            if before is None:
                os.environ.pop(st.CEILING_ENV, None)
            else:
                os.environ[st.CEILING_ENV] = before

    # `default_for` reads one field and is the reason a theoretical deck opens
    # wider than a built one. Absent/blank/unknown all mean built, which is the
    # conservative direction and the one a typo must fall to.
    defaults = [{"status": status,
                 "stance": st.default_for(_Statused(status)).to_dict()}
                for status in (None, "", "built", "theoretical", "  Theoretical  ",
                               "THEORETICAL", "nonsense")]

    return {
        "_comment": ("The stance dial, exhaustive. Generated by "
                     "tests/go_fixtures.py; do not edit by hand. `describe` is "
                     "recorded as a SERIALISED string rather than as an object "
                     "because field order is part of the contract and only "
                     "bytes carry it."),
        "axes": list(st.AXES),
        "levels": {a: list(v) for a, v in st.LEVELS.items()},
        "preset_names": list(st.PRESETS),
        "presets": {n: s.to_dict() for n, s in st.PRESETS.items()},
        "preset_blurbs": dict(st.PRESET_BLURBS),
        "stances": stances,
        "clamps": clamps,
        "parses": parses,
        "ceilings": ceilings,
        "defaults": defaults,
        "dial": dial_cases(),
    }


#: The slug the dial corpus asks about. Any name; it exists only so the route's
#: `if slug:` branch is the one being exercised.
_DIAL_SLUG = "dial-probe"


def _dial_source(status: Any) -> Any:
    """A one-deck source whose deck has the status under test, or None.

    A **real** `DeckSource` rather than a patched payload: `claude_status`
    reads the deck itself (`Deck.from_text(decks.read_text(slug))`), so a
    corpus that computed the two deck-dependent fields on the side would be
    reimplementing the function it is checking -- and would stay green against
    a mutant of it. The tarot lane paid for that lesson once already.
    """
    if status is None:
        return None
    from mtglab.decks.model import Deck
    from mtglab.decks.source import MemoryDeckSource

    text = f"name: Dial Probe\nstatus: {status}\ncards: []\n"
    return MemoryDeckSource([Deck.from_text(text, slug=_DIAL_SLUG)])


def dial_cases() -> list[dict[str, Any]]:
    """`service.claude_status`, the whole payload `GET /api/claude` answers.

    Recorded as the **serialised** shape rather than field by field, for the
    reason the `stances` table above gives: a struct whose values are right
    and whose field order is wrong marshals to different bytes, and only
    comparing bytes sees it.

    Three environment facts are pinned rather than inherited, because all
    three are read at call time and would otherwise make the corpus a record
    of whichever machine rendered it: the stance ceiling, the credential (so
    `configured` is a decision rather than an accident of the shell), and the
    model override (so `model` is the house answer).

    **The deck is a status string, not a slug.** The route reads a deck only
    to get its `status`, so the corpus exercises that field directly and the
    handler's library resolution is left to the route tests, where a 404 can
    actually be asserted.

    Two rows were **warts until 2026-08-23** and are kept for it: for three
    months `?surface=scan` resolved `off` -- `_SURFACE_DEFAULTS` was never
    extended when ADR 34 landed (#180), while `scan.stance_for` sat there with
    a docstring saying it existed to prevent exactly that -- and the `modes`
    list was six of the seven for the same omission one layer along. Found by
    the Go port and ruled with Aaron; both fixed in both runtimes at once.
    Neither was reachable from the app, which is how they survived, so the
    rows stay here as the thing that would notice.
    """
    from mtglab.api import service

    cases = []
    for note, requested, status, surface, ceiling in [
        ("nothing at all", None, None, None, None),
        ("no deck, no surface", None, None, "", None),
        ("a theme surface with no deck", None, None, "theme", None),
        ("a research surface with no deck", None, None, "research", None),
        # `consultant` and not `second-opinion`: this is a transcription,
        # where volunteering is the failure mode rather than the feature. It
        # answered `off` until 2026-08-23.
        ("a scan surface with no deck", None, None, "scan", None),
        ("a surface nobody owns", None, None, "nonsense", None),
        ("a built deck", None, "built", None, None),
        ("a theoretical deck", None, "theoretical", None, None),
        ("a deck with an odd status", None, "sideways", None, None),
        # A deck present alongside a deckless surface: the deck wins, because
        # the branch is `surface in _SURFACE_DEFAULTS AND deck is None`.
        ("a theme surface WITH a deck", None, "built", "theme", None),
        ("a pin", "collaborator", None, None, None),
        ("a pin over a deck", "off", "theoretical", None, None),
        ("a pin over a deckless surface", "consultant", None, "theme", None),
        ("a custom stance", {"initiative": "volunteers", "scope": "rethink",
                             "write": "none"}, None, None, None),
        ("a malformed stance", {"initiative": 7}, None, None, None),
        ("a stance that is not a stance", 7, None, None, None),
        ("a preset that is not one", "nope", None, None, None),
        # Under a cap: every preset row's `available` flips, which is the
        # field a UI greys a control out on.
        ("everything, capped at off", None, None, None, "off"),
        ("a theme surface, capped at off", None, None, "theme", "off"),
        ("a pin over the cap", "collaborator", None, None, "consultant"),
    ]:
        saved_ceiling = os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
        saved_key = os.environ.pop("ANTHROPIC_API_KEY", None)
        saved_token = os.environ.pop("ANTHROPIC_AUTH_TOKEN", None)
        saved_model = os.environ.pop("MTGLAB_CLAUDE_MODEL", None)
        if ceiling:
            os.environ["MTGLAB_CLAUDE_STANCE_CEILING"] = ceiling
        try:
            row: dict[str, Any] = {"note": note, "requested": requested,
                                   "deck_status": status, "surface": surface,
                                   "ceiling": ceiling}
            try:
                row["payload"] = service.claude_status(
                    requested=requested, surface=surface,
                    slug=_DIAL_SLUG if status is not None else None,
                    source=_dial_source(status))
            except ValueError as exc:
                row["error"] = str(exc)
            cases.append(row)
        finally:
            os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
            for name, value in (("MTGLAB_CLAUDE_STANCE_CEILING", saved_ceiling),
                                ("ANTHROPIC_API_KEY", saved_key),
                                ("ANTHROPIC_AUTH_TOKEN", saved_token),
                                ("MTGLAB_CLAUDE_MODEL", saved_model)):
                if value is not None:
                    os.environ[name] = value
    return cases


class _Statused:
    """The one field `default_for` reads, and nothing else.

    A stand-in rather than a real `Deck`, because `default_for` uses `getattr`
    with a default and so accepts an object that has no `status` at all -- the
    `None` row above is that case, and a real Deck cannot produce it.
    """

    def __init__(self, status: Any) -> None:
        if status is not None:
            self.status = status


def render_stance_cases() -> str:
    return _rows_json(stance_cases()) + "\n"


# ------------------------------------------------------------- the seven voices

#: Where the persona roster lands. `data/`, not `testdata/`: Go EMBEDS this and
#: serves from it, exactly as `internal/reference/data` embeds the prose
#: modules. A voice is checked-in prose whose bytes reach a model, so
#: transcribing 350 lines of it by hand into a second language is the one
#: mistake this file exists to prevent.
PERSONA_PATH = ROOT / "go" / "internal" / "claude" / "data" / "personas.json"


def persona_payload() -> dict[str, Any]:
    """Every persona, voice included -- and the roster the door may serve.

    Two lists rather than one, and the split is the design. `voice` reaches the
    model and must never reach the client: not because a prompt in a public
    repository is a secret, but because a client that received one would
    eventually send one back, and "the persona is one of a fixed set" is worth
    keeping structural rather than polite. Go embeds `personas`; its route
    serves `roster`; a test asserts the second is the first with one field
    removed, so the two cannot drift into agreement by accident.
    """
    from mtglab.claude import persona as persona_mod
    return {
        "_comment": ("The seven voices. Generated by tests/go_fixtures.py from "
                     "src/mtglab/claude/persona.py; do not edit by hand. "
                     "`voice` is server-side only -- `roster` is what the route "
                     "may answer with."),
        "default": persona_mod.DEFAULT,
        "personas": [{"key": who.key, "label": who.label, "blurb": who.blurb,
                      "voice": who.voice, "deals": who.deals}
                     for who in persona_mod.PERSONAS.values()],
        "roster": persona_mod.as_dicts(),
    }


def render_persona_payload() -> str:
    return _rows_json(persona_payload()) + "\n"


# ------------------------------------------------------------------ the tarot

#: The deck itself: 136 cards, embedded and served. Data, like the personas.
TAROT_DATA_PATH = ROOT / "go" / "internal" / "tarot" / "data" / "deck.json"
#: The deal, held to CPython's own `random` through `pyrand`.
TAROT_PATH = ROOT / "go" / "internal" / "tarot" / "testdata" / "deals.json"


def tarot_deck_payload() -> dict[str, Any]:
    """Every card and every position, as the Go module embeds them.

    Rendered rather than transcribed for the persona reason doubled: 136 cards
    carrying names, artists, credits and `note` prose, where a mistyped `after`
    silently breaks the alignment paragraph and a mistyped weight silently
    changes which cards land. None of that fails loudly.

    `SPREAD` rides along because its three `slot` values ARE `theme.SLOT_KINDS`'
    first three, and that coupling is load-bearing: a card is dealt *for* a
    slot, so ADR 20's grounded-quote readiness works untouched. Its failure is
    silent -- drift, and the proposal button simply never lights up -- so the
    Go side gets the same three strings from the same place rather than a
    second copy of them.
    """
    from mtglab import tarot
    return {
        "_comment": ("The 1909 deck plus Magic's crossovers and echoes, and the "
                     "three-card spread. Generated by tests/go_fixtures.py from "
                     "src/mtglab/tarot.py; do not edit by hand."),
        "crossover_weight": tarot.CROSSOVER_WEIGHT,
        "echo_weight": tarot.ECHO_WEIGHT,
        "spread": [{"slot": pos.slot, "name": pos.name, "asks": pos.asks}
                   for pos in tarot.SPREAD],
        # FULL_DECK's ORDER is the sampler's search order, so it is part of the
        # answer and not a presentation detail: `_weighted_sample` walks this
        # list accumulating weight until it passes `mark`. Reorder it and every
        # seed deals differently.
        "cards": [{
            "key": c.key, "name": c.name, "arcana": c.arcana, "suit": c.suit,
            "number": c.number, "art_url": c.art_url, "artist": c.artist,
            "after": c.after, "echo": c.echo, "weight": c.weight,
            "note": c.note, "image": c.image, "face_name": c.face_name,
        } for c in tarot.FULL_DECK],
    }


def tarot_deal_cases() -> dict[str, Any]:
    """Seeded deals, recorded as the payload AND as the reader's prose.

    Both halves, because they fail differently. `as_dict` is what the browser
    renders and what a reload must reproduce; `describe()` is what reaches the
    model, and it carries the two Python-detected paragraphs -- the Magic-card
    omen and the trump landing twice -- that no card field states directly.

    The seeds are not arbitrary. Most are a plain sweep, but four are chosen by
    SEARCHING for the states the prose branches on: a spread containing a
    crossover, one containing an echo, one containing a reversed card, and --
    the rare one -- a trump landing twice at the same table. That last is what
    `describe` calls the rarest thing this spread can do, and a corpus of the
    first twenty seeds would never once contain it, so the branch that renders
    it would be covered by nothing at all.
    """
    from mtglab import tarot

    def has_after(r: Any) -> bool:
        return any(d.card.after for d in r.cards)

    def has_echo(r: Any) -> bool:
        return any(d.card.echo for d in r.cards)

    def has_reversed(r: Any) -> bool:
        return any(d.reversed for r_ in [r] for d in r_.cards)

    def has_alignment(r: Any) -> bool:
        seen: dict[tuple[str, Any, int], int] = {}
        for d in r.cards:
            k = (d.card.arcana, d.card.suit, d.card.number)
            seen[k] = seen.get(k, 0) + 1
        return any(v > 1 for v in seen.values())

    seeds = list(range(24))
    for want in (has_after, has_echo, has_reversed, has_alignment):
        for candidate in range(200_000):
            if candidate in seeds:
                continue
            if want(tarot.deal(candidate)):
                seeds.append(candidate)
                break
        else:  # pragma: no cover - a deck this search cannot satisfy is a bug
            raise AssertionError(f"no seed found for {want.__name__}")

    cases = []
    for seed in seeds:
        reading = tarot.deal(seed)
        cases.append({
            "seed": seed,
            # Serialised, not structured: field order is the contract, and only
            # bytes carry it. The stance corpus makes the same choice for the
            # same reason.
            "as_dict": json.dumps(reading.as_dict(), ensure_ascii=False,
                                  separators=(",", ":"), sort_keys=False),
            "describe": reading.describe(),
        })
    return {
        "_comment": ("Seeded tarot deals. Generated by tests/go_fixtures.py; do "
                     "not edit by hand. The last four seeds were SEARCHED for "
                     "-- a crossover, an echo, a reversal and a doubled trump "
                     "-- because a plain sweep reaches none of the prose "
                     "branches that make `describe` more than a card list."),
        "searched": {"crossover": seeds[24], "echo": seeds[25],
                     "reversed": seeds[26], "alignment": seeds[27]},
        "pool_totals": _tarot_pool_totals(),
        "seed_strings": _tarot_seed_strings(),
        "cases": cases,
    }


def _tarot_seed_strings() -> list[dict[str, Any]]:
    """What `?seed=` accepts, and what it refuses.

    `/api/tarot/reading` declares `seed: int | None`, so the query string is
    parsed by Pydantic before `deal` ever sees it -- and Pydantic's integer
    grammar is wider than `strconv.ParseInt`'s in three ways a port gets wrong
    by writing the obvious thing. Whitespace is stripped; a leading `+` is
    fine; and **single underscores between digits are digit separators**, so
    `1_0` is ten. Each of those is a 422 from a naive Go door where Python
    answered 200.

    **The oracle is `TypeAdapter(int)`, not `int()`, and that distinction was
    measured rather than assumed.** This function was first written against
    `int()` with a docstring asserting the two agreed; they do not. Python's
    `int()` accepts any Unicode decimal digit, so `int("７")` is 7, while
    Pydantic refuses the fullwidth form and the route answers 422. One row out
    of twenty-four, in the direction that would have made the Go door accept a
    seed the Python door rejects -- which is the direction a contract suite
    finds late, since no client sends fullwidth digits until one does.
    `TypeAdapter(int)` is what FastAPI actually calls, and driving the real
    route over every row below confirmed it agrees on all of them.

    The oversized values are the other load-bearing rows: `random.Random` seeds
    through arbitrary-precision integers, and `deal` echoes the seed it was
    given, so a Go side holding it in an int64 truncates 2**70 into a
    different reading AND a different number on the wire.
    """
    from pydantic import TypeAdapter, ValidationError
    adapter = TypeAdapter(int)
    raw = ["7", "0", "-5", "+7", "0007", "  7  ", "1_0", "1_000_000",
           "_7", "7_", "1__0", "", " ", "abc", "7.5", "0x10",
           # The two rows that separate Pydantic's grammar from Python's.
           "\uff17", "\u0667",
           str(2**70), str(-(2**70)), str(2**63), str(2**63 - 1),
           "-0", "00", "+0"]
    rows: list[dict[str, Any]] = []
    for text in raw:
        row: dict[str, Any] = {"text": text}
        try:
            value = adapter.validate_python(text)
        except ValidationError:
            row["ok"] = False
        else:
            row["ok"] = True
            # As a STRING: these outgrow every fixed-width integer type, and
            # the value is echoed back on the wire exactly as given.
            row["value"] = str(value)
        rows.append(row)
    return rows


def _tarot_pool_totals() -> list[dict[str, Any]]:
    """The three running totals a deal computes, as BITS -- and as both sums.

    This table exists because of a mutation that survives and cannot be made to
    die any other way. Replacing `fsum` with a naive running total changes no
    spread: `tarot.py` measured 200,000 seeds and every one deals the same
    three cards, because `mark` would have to land inside a 2.8e-14 window out
    of 90.2 to notice. A deal-level corpus therefore cannot tell the two
    arithmetics apart, at any size -- the odds are about 3e-16 per draw, so
    searching for a separating seed is not slow, it is hopeless.

    That is a reason to test the sum DIRECTLY, not a reason to leave it
    untested. `naive` is recorded beside `fsum` and asserted to DIFFER, so the
    corpus proves it can tell them apart before the Go side is asked to agree
    with one. Both are recorded as `Float64bits`, since the whole quantity in
    dispute is the last bit.

    The 134-card pool is the interesting one: the third draw, where the
    docstring's measurement found all 9,180 pools disagreeing across
    interpreters. 136 and 135 are recorded too, because a port that got the
    first two right and the third wrong would be a strange bug to debug from a
    single row.
    """
    from mtglab import tarot
    rows = []
    weights = [c.weight for c in tarot.FULL_DECK]
    for drop in (0, 1, 2):
        pool = weights[drop:]
        exact = math.fsum(pool)
        running = 0.0
        for w in pool:
            running += w
        rows.append({
            "cards": len(pool),
            "fsum_bits": struct.unpack("<Q", struct.pack("<d", exact))[0],
            "naive_bits": struct.unpack("<Q", struct.pack("<d", running))[0],
            "differ": exact != running,
        })
    return rows


def render_tarot_deck() -> str:
    return _rows_json(tarot_deck_payload()) + "\n"


def render_tarot_deals() -> str:
    return _rows_json(tarot_deal_cases()) + "\n"


# ------------------------------------------------------------- the Claude ledger

LEDGER_PATH = ROOT / "go" / "internal" / "claude" / "ledger" / "testdata" / "ledger.json"


def ledger_cases() -> dict[str, Any]:
    """Seeded usage rows and the roll-up Python computes over them.

    The SQL is short and the temptation is to call it obvious, which is what
    the artifacts corpus was called before nine decks of exact halves hid an
    `fsum` bug. Three things here are not obvious and each is recorded:

    * **Scan order depends on the axis.** The grouped column is SELECTed
      first, so `mode` and `model` swap positions between the two queries. A
      Go port that scans them in a fixed order gets the right numbers under
      the wrong names, which reads as a mislabelled panel rather than as a bug.
    * **Every row carries the marker in the column it did not group on.** A
      port that emitted SQLite's arbitrary winner would look right until two
      models shared a mode.
    * **The `since` bound is a TEXT comparison** against an ISO-8601 column,
      inclusive at `>=`. The boundary row is in the corpus for that reason.

    The totals are deliberately all distinct, so `ORDER BY` is fully
    determined. A tie's order is arbitrary in SQLite and pinning one would be
    pinning an implementation detail of whichever engine rendered the corpus.
    """
    import sqlite3

    # Fixed timestamps: the corpus must not expire, and `since` is compared
    # against these as text.
    rows = [
        ("2026-08-20T10:00:00.000000+00:00", "commander-dossier", "claude-opus-5", "end_turn", 3, 1000, 200, 500),
        ("2026-08-21T10:00:00.000000+00:00", "commander-dossier", "claude-opus-5", "end_turn", 2, 500, 100, 250),
        ("2026-08-21T11:00:00.000000+00:00", "commander-dossier", "claude-sonnet-5", "refusal", 1, 400, 0, 0),
        ("2026-08-22T10:00:00.000000+00:00", "rationale-interview", "claude-sonnet-5", "end_turn", 1, 90, 9, 0),
        ("2026-08-22T11:00:00.000000+00:00", "theme-proposal", "claude-sonnet-5", "exhausted", 6, 30, 3, 0),
        # A zero-token row: `sum` over it must be 0 and not NULL, and the row
        # must still count as a conversation.
        ("2026-08-22T12:00:00.000000+00:00", "scan", "claude-haiku-4-5", "end_turn", 1, 0, 0, 0),
    ]

    con = sqlite3.connect(":memory:")
    con.row_factory = sqlite3.Row
    con.execute(
        "CREATE TABLE claude_usage (id INTEGER PRIMARY KEY AUTOINCREMENT,"
        " created_at TEXT NOT NULL, mode TEXT NOT NULL, model TEXT NOT NULL,"
        " stop_reason TEXT NOT NULL, requests INTEGER NOT NULL,"
        " input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL,"
        " cache_read_tokens INTEGER NOT NULL)")
    con.executemany(
        "INSERT INTO claude_usage (created_at, mode, model, stop_reason,"
        " requests, input_tokens, output_tokens, cache_read_tokens)"
        " VALUES (?, ?, ?, ?, ?, ?, ?, ?)", rows)
    con.commit()

    def roll(by: str, since: str | None) -> list[dict[str, Any]]:
        other = "model" if by == "mode" else "mode"
        query = (f"SELECT {by}, '(various)' AS {other},"
                 " count(*) AS conversations, sum(requests) AS requests,"
                 " sum(input_tokens) AS input_tokens,"
                 " sum(output_tokens) AS output_tokens,"
                 " sum(cache_read_tokens) AS cache_read_tokens,"
                 " min(created_at) AS first_at, max(created_at) AS last_at"
                 " FROM claude_usage")
        args: tuple[Any, ...] = ()
        if since is not None:
            query += " WHERE created_at >= ?"
            args = (since,)
        query += f" GROUP BY {by} ORDER BY sum(input_tokens + output_tokens) DESC"
        return [dict(r) for r in con.execute(query, args).fetchall()]

    queries = []
    for by in ("mode", "model"):
        for since in (None,
                      "2026-08-21T10:00:00.000000+00:00",   # exactly a row: >= keeps it
                      "2026-08-21T10:00:00.000001+00:00",   # one microsecond later
                      "2999-01-01T00:00:00.000000+00:00"):  # everything dropped
            queries.append({"by": by, "since": since, "rows": roll(by, since)})

    return {
        "_comment": ("Claude usage rows and the roll-ups Python computes. "
                     "Generated by tests/go_fixtures.py; do not edit by hand. "
                     "Timestamps are fixed so the corpus cannot expire, and "
                     "every total is distinct so ORDER BY is fully determined "
                     "-- a tie's order is arbitrary and pinning one would pin "
                     "an implementation detail."),
        "columns": ["created_at", "mode", "model", "stop_reason", "requests",
                    "input_tokens", "output_tokens", "cache_read_tokens"],
        "rows": [list(r) for r in rows],
        "queries": queries,
    }


def render_ledger_cases() -> str:
    return _rows_json(ledger_cases()) + "\n"


# --------------------------------------------------------- the seven tools

#: Where the read-only tool schemas land. `data/`, embedded and served to the
#: model -- the same argument the persona voices make, doubled: a tool
#: description is prescriptive prose about *when to call*, and an
#: under-described tool is the most common reason a model answers from recall
#: instead. Which in this codebase is the exact failure rule 1 exists to
#: prevent, so these bytes are load-bearing and are not retyped by hand.
#: Where the mode definitions land. `data/`, embedded like the personas and the
#: tool schemas, and for the same reason at greater scale: a mode IS a system
#: prompt, and `theme.py`'s alone runs to thousands of words whose bytes reach
#: a model. Transcribing seven of them into a second language by hand is the
#: drift this file exists to prevent -- and unlike a voice, a mistyped
#: instruction here changes what the model is allowed to DO rather than how it
#: sounds.
#:
#: **All seven cross at once, including modes Go has no orchestration for yet.**
#: The definition is data; the code that assembles a brief and reads an answer
#: back is the mode's own and crosses when it crosses. Rendering only the ported
#: ones would make "has this mode crossed?" a question about two places.
DIGITS_PATH = ROOT / "go" / "internal" / "claude" / "data" / "digits.json"


def digits_payload() -> dict[str, Any]:
    """Where each of Unicode's decimal digit runs starts, swept from CPython.

    `int()` reads **any** Unicode decimal digit, so `int("７")` is seven and
    `int("٣")` is three -- and `theme._reading_for` spells its seed parse
    `int(seed)`, not the Pydantic grammar the `/api/tarot/reading` query
    string goes through. Two different functions, and the tarot lane already
    measured that they disagree.

    Go's `unicode.IsDigit` is category Nd exactly, but nothing in the standard
    library gives a digit's *value*. The obvious trick -- walk down while the
    neighbour is still a digit, and the distance is the value -- is **wrong
    for 36 code points**: the mathematical digit blocks at U+1D7CE onward are
    four runs of ten with no gap between them, so the walk crosses a boundary
    and reads a bold `4` as fourteen. Measured before it shipped.

    So the table is the run *starts*: every Nd run is exactly ten long,
    contiguous and zero-based (asserted below over the whole of Unicode), so
    68 numbers answer all 680. A code point's value is its distance from the
    greatest start not above it, when that distance is under ten.

    **This one really is interpreter-dependent, unlike the fold table beside
    it.** 3.11 ships Unicode 14 and knows 66 runs; 3.12 ships 15 and knows 68,
    having gained the Kawi digits at U+11F50 and the Nag Mundari at U+1E4F0.
    So the committed table is **3.12's**, because that is what the container
    runs and therefore what the deployed Python answers -- the same reasoning
    `math.fsum` got. `unicode_version` is recorded so the freshness check can
    say which sweep it is holding: it demands equality on a matching
    interpreter and a **subset** on an older one, since Unicode adds digit
    blocks and never removes them.
    """
    zeros = sorted({cp - int(chr(cp)) for cp in range(0x110000)
                    if unicodedata.category(chr(cp)) == "Nd"})
    for zero in zeros:
        assert [int(chr(zero + i)) for i in range(10)] == list(range(10)), zero
        # A run either starts a fresh block or butts straight onto the `9` of
        # the one before it -- never onto that run's middle. This is the
        # invariant the walk-down heuristic assumed and does not have.
        before = chr(zero - 1)
        assert (unicodedata.category(before) != "Nd"
                or int(before) == 9), zero
    return {
        "note": ("The first code point of every Unicode decimal-digit run, "
                 "swept from CPython by `python tests/go_fixtures.py`. Each "
                 "run is exactly ten long and zero-based, so a digit's value "
                 "is its distance from the greatest start not above it."),
        "unicode_version": unicodedata.unidata_version,
        "runs": len(zeros),
        "zeros": zeros,
    }


def render_digits_payload() -> str:
    return _rows_json(digits_payload()) + "\n"


CASEFOLD_PATH = ROOT / "go" / "internal" / "claude" / "data" / "casefold.json"


def casefold_payload() -> dict[str, Any]:
    """`str.casefold()` as a table, swept out of the interpreter itself.

    The fourth reproduction of somebody else's arithmetic, after `pyrand`,
    `pyyaml` and `pyfloat`, and the smallest argument for one. `theme.ground`
    folds the user's own turns and the model's claimed quote and asks whether
    one contains the other; `strings.ToLower` is **not** that function.
    `casefold()` applies Unicode *full* case folding, so `ß` becomes `ss`,
    `ſ` becomes `s` and `ς` becomes `σ` -- 211 code points where the two
    disagree, of which 104 fold to more than one character and cannot be a
    rune-to-rune mapping at all. A German answer quoted back at a
    fortune-teller's table would ground in Python and drop in Go, and what a
    dropped slot looks like from the other side of the screen is a readiness
    count that will not move.

    Swept rather than transcribed, and the whole of Unicode rather than the
    disagreements: a table of only the 211 differences would still rest on
    Go's `unicode` tables agreeing with CPython's everywhere else, which is a
    claim about two projects' Unicode versions and not one this port should
    be making. Every code point whose fold is not itself is recorded, and a
    rune absent from the table folds to itself. That is exact by
    construction and depends on nothing.

    Its own corpus, and no separate one: the table **is** the oracle, so
    `tests/test_go_fixtures.py` holding the committed file equal to a fresh
    sweep is the whole check.

    **It carries no Unicode version, and the absence is measured.** The first
    draft recorded `unicodedata.unidata_version` and went red on CI's 3.11
    leg, which was the right alarm about the wrong field: 3.11 ships Unicode
    14 and 3.12 ships 15, so the string differed -- and **nothing else did.**
    Both interpreters answer 1,530 folds, the same 1,530 code points, the
    same strings, 104 multi-character and 211 disagreeing with `lower()`;
    case folding has not moved between those two releases. Recording the
    version would have made a stable table look interpreter-dependent and
    left the freshness check unable to run on both legs, which is where a
    real drift would show. The counts below are the honest pin instead --
    they are what a changed fold would move. (`digits_payload` keeps its
    version, because that table really does differ; see there.)
    """
    folds = [{"cp": cp, "fold": chr(cp).casefold()}
             for cp in range(0x110000) if chr(cp).casefold() != chr(cp)]
    return {
        "note": ("`str.casefold()` for every code point that does not fold to "
                 "itself, swept from CPython by `python tests/go_fixtures.py`. "
                 "A code point absent here folds to itself. Identical under "
                 "3.11 and 3.12 -- verified, not assumed."),
        "multi_char": sum(1 for f in folds if len(f["fold"]) > 1),
        "differ_from_lower": sum(1 for f in folds
                                 if chr(f["cp"]).lower() != f["fold"]),
        "folds": folds,
    }


def render_casefold_payload() -> str:
    return _rows_json(casefold_payload()) + "\n"


MODES_PATH = ROOT / "go" / "internal" / "claude" / "data" / "modes.json"


def _every_mode() -> list[Any]:
    """Every `Mode` object anywhere under `mtglab.claude`, by discovery.

    **Not a hand-written list, and the first draft of this function was one.**
    It named six modes, gathered by grepping for `= Mode(` -- which silently
    missed `scan.py`, because that module spells it `modes.Mode(...)`. Seven
    became six with nothing anywhere looking wrong: Go would have loaded six,
    every test would have agreed six was the number, and ADR 34's deliberate
    absence of a card-name field would have crossed as the absence of the whole
    mode.

    That is the exact failure CLAUDE.md records four times over -- a
    completeness claim inherited rather than re-checked. So the modes are
    *found*: every module in the package is imported and every attribute that
    is a Mode is collected. Deduplicated by name, which also collapses
    `CONVERSATION_MODES["plain"]`, since that is THEME_CONVERSATION itself
    rather than a copy of it.
    """
    import importlib
    import pkgutil

    from mtglab.claude import modes as modes_mod

    found: dict[str, Any] = {}
    package = importlib.import_module("mtglab.claude")
    for info in pkgutil.iter_modules(package.__path__):
        module = importlib.import_module(f"mtglab.claude.{info.name}")
        for value in vars(module).values():
            if isinstance(value, modes_mod.Mode):
                found[value.name] = value
    return [found[name] for name in sorted(found)]


def modes_payload() -> dict[str, Any]:
    """Every Mode object Python defines, as the Go module embeds them.

    `response_schema` is the load-bearing field and the reason this is
    generated rather than transcribed. ADR 25's slot argument has no
    `defence`, `verdict` or `summary` property and sets
    `additionalProperties: false`, so a balanced answer -- the attractive one,
    and a rationale generator wearing a hat -- has nowhere to go. ADR 34's scan
    has no field for a card name. Those absences ARE the features, and an
    absence is exactly what a hand-copy drops with nothing looking wrong.

    `server_tools` crosses as the API's own dicts rather than as a Go type,
    because that is what they are on the wire. Go maps them onto the SDK's
    typed unions when it builds the request, and a mode declaring a type Go
    cannot build fails at startup rather than going out without its search.
    """
    rows = []
    for mode in _every_mode():
        rows.append({
            "name": mode.name,
            "purpose": mode.purpose,
            "instructions": mode.instructions,
            "tool_names": list(mode.tool_names),
            "server_tools": [dict(t) for t in mode.server_tools],
            "may_write": list(mode.may_write),
            "max_tokens": mode.max_tokens,
            "effort": mode.effort,
            "response_schema": mode.response_schema,
            "scope_notes": (dict(mode.scope_notes)
                            if mode.scope_notes is not None else None),
        })
    return {
        "_comment": ("Every Claude mode: prompt, tool set, and the response "
                     "schema whose ABSENCES are the feature (ADR 25 has no "
                     "field for a defence; ADR 34 has none for a card name). "
                     "Generated by tests/go_fixtures.py from "
                     "src/mtglab/claude/ BY DISCOVERY, not from a list -- see "
                     "_every_mode. Do not edit by hand. `may_write` is "
                     "rendered though it is always empty: ADR 15's claim is "
                     "that a mode is a capability declaration, and a field "
                     "asserted empty says so where a silence would not."),
        "modes": rows,
    }


def render_modes_payload() -> str:
    return _rows_json(modes_payload()) + "\n"


TOOLS_PATH = ROOT / "go" / "internal" / "claude" / "tools" / "data" / "tools.json"


def tools_payload() -> dict[str, Any]:
    """The seven read-only tools, as the model is told about them.

    Schemas only. The FUNCTION each one dispatches to is the Go registry's
    business and cannot cross as data -- which is the point of the split: the
    prose is generated so it cannot drift, and the dispatch is code so the
    boundary analysis can see it.

    `additional_properties` is recorded even though it is always false,
    because it is a *decision*: the API does not enforce it without `strict`,
    which is why `run()` checks the arguments again on the way in. Stated in
    the schema anyway, so the model is told the shape rather than corrected
    after the fact.
    """
    from mtglab.claude import tools as tools_mod
    rows = []
    for name in sorted(tools_mod.READ_ONLY):
        tool = tools_mod.READ_ONLY[name]
        schema = tool.schema()
        rows.append({
            "name": tool.name,
            "description": tool.description,
            "properties": schema["input_schema"]["properties"],
            "required": schema["input_schema"]["required"],
            "additional_properties": schema["input_schema"]["additionalProperties"],
            # Deck-facing tools take a DeckSource so nothing reads the
            # filesystem directly -- the same rule the routes follow.
            "takes_source": tool.takes_source,
        })
    return {
        "_comment": ("The read-only Claude tools, schemas only. Generated by "
                     "tests/go_fixtures.py from src/mtglab/claude/tools.py; do "
                     "not edit by hand. Sorted by name, which is also the order "
                     "`schemas()` renders -- an unstable tool order would "
                     "invalidate the prompt cache on every turn, for free and "
                     "invisibly, since tools render first in the prompt."),
        "tools": rows,
    }


def render_tools_payload() -> str:
    return _rows_json(tools_payload()) + "\n"


# ------------------------------------------------------------- the theme lane

THEME_PATH = ROOT / "go" / "internal" / "claude" / "testdata" / "theme.json"

#: The angle `theme._closing_for` draws for an opening turn. Python spells it
#: `random.choice` on the global RNG, so nothing reproducible rides on which
#: one comes out -- but a corpus that could not hold it still would be a
#: corpus of one opening question. Pinned to an index rather than the text, so
#: a reworded angle moves in both runtimes at once.
FROZEN_ANGLE_INDEX = 2

#: A transcript the readers can be driven over, and the quotes below are all
#: really in it. Deliberately mixed: an opening assistant turn (which is what
#: this mode's transcripts start with), short shy answers of the kind that
#: made the readiness count go backwards, and one turn carrying a character
#: `str.casefold()` and `str.lower()` disagree about.
THEME_TRANSCRIPT = [
    {"role": "assistant", "text": "Tell me something you keep coming back to."},
    {"role": "user", "text": "old horror films, the practical effects ones"},
    {"role": "assistant", "text": "And when a plan falls apart?"},
    {"role": "user", "text": "I improvise. STRASSE is where I grew up."},
    {"role": "assistant", "text": "At game night, then -- what are you like?"},
    {"role": "user", "text": "I make deals. quietly"},
]


def _theme_slot(kind: Any, value: Any, quote: Any) -> dict[str, Any]:
    return {"kind": kind, "value": value, "quote": quote}


def theme_ground_cases() -> list[dict[str, Any]]:
    """`ground()` over every way a slot can fail, and the two ways it passes.

    The casefold row is the one that could not have been written in Go: the
    model quotes `Straße` where the person typed `STRASSE`, which `casefold()`
    makes equal and `ToLower` does not. It is a real German spelling and the
    only reachable difference between the two functions in this module.
    """
    from mtglab.claude import theme
    cases = []
    for note, slots in [
        ("nothing at all", []),
        ("a slot that is not an object", ["taste", 7, None]),
        ("an unknown kind", [_theme_slot("vibe", "spooky", "old horror films")]),
        ("no value", [_theme_slot("taste", "", "old horror films")]),
        ("a quote below the floor", [_theme_slot("taste", "cats", "I")]),
        ("a quote at the floor", [_theme_slot("posture", "deals", "I m")]),
        ("a quote nobody said", [_theme_slot("taste", "blue decks", "I love blue")]),
        ("the interviewer's own words", [
            _theme_slot("temperament", "planner", "when a plan falls apart")]),
        ("a quote spanning two turns", [
            _theme_slot("taste", "both", "the practical effects ones I improvise")]),
        ("casefold, not lower", [
            _theme_slot("temperament", "grew up on the Strasse", "Straße")]),
        ("control characters in the quote", [
            _theme_slot("taste", "horror\x0cfilms", "old horror\tfilms")]),
        ("last reading of a kind wins", [
            _theme_slot("taste", "films", "old horror films"),
            _theme_slot("taste", "practical effects specifically",
                        "the practical effects ones")]),
        ("canonical order, not the model's", [
            _theme_slot("posture", "deals", "I make deals"),
            _theme_slot("taste", "films", "old horror films"),
            _theme_slot("temperament", "improviser", "I improvise")]),
        ("a non-string kind", [_theme_slot(7, "seven", "old horror films")]),
        ("a numeric value", [_theme_slot("taste", 7, "old horror films")]),
    ]:
        kept, dropped = theme.ground(list(slots), THEME_TRANSCRIPT)
        cases.append({"note": note, "slots": slots, "kept": kept,
                      "dropped": dropped,
                      "may_propose": theme.may_propose(kept)})
    return cases


def theme_carry_cases() -> list[dict[str, Any]]:
    """`carry()`: the floor a turn builds on, and it may not go backwards."""
    from mtglab.claude import theme
    taste = _theme_slot("taste", "films", "old horror films")
    temper = _theme_slot("temperament", "improviser", "I improvise")
    posture = _theme_slot("posture", "deals", "I make deals")
    sharper = _theme_slot("taste", "practical effects", "the practical effects ones")
    cases = []
    for note, previous, fresh in [
        ("nothing either way", [], []),
        ("a turn that heard nothing keeps what was known", [taste, temper], []),
        ("a turn that heard something new adds it", [taste], [temper]),
        ("a turn refining a kind replaces it", [taste], [sharper]),
        ("the floor is reached across turns", [taste, temper], [posture]),
        ("canonical order out, whatever went in", [posture], [temper, taste]),
    ]:
        carried = theme.carry(list(previous), list(fresh))
        cases.append({"note": note, "previous": previous, "fresh": fresh,
                      "carried": carried,
                      "may_propose": theme.may_propose(carried)})
    return cases


def theme_repeat_cases() -> list[dict[str, Any]]:
    """`repeats()`: the deterministic half of "never the same fact twice"."""
    from mtglab.claude import theme
    told = [
        "Pamela Colman Smith drew all 78 cards and was paid one flat fee.",
        "The Fool is the only card in the deck about to have an accident.",
    ]
    cases = []
    for note, text, against in [
        ("nothing told yet", "A brand new fact.", []),
        ("empty text", "", told),
        ("the same sentence", told[0], told),
        ("the same sentence, differently spaced", "  " + told[0].upper() + " ", told),
        ("reworded, same content words",
         "Smith drew all 78 cards of the deck and was paid a single flat fee.", told),
        ("a genuinely different fact about the same person",
         "Smith exhibited photographs with Alfred Stieglitz in 1907.", told),
        ("stop words only", "the a of and", told),
        ("a short fact that shares its whole vocabulary", "The Fool.", told),
        # Exactly on the threshold, which `>=` keeps and `>` drops. Ten
        # content words against a fact carrying seven of them: 7/10 is 0.7 in
        # both languages' arithmetic, so the boundary is reachable rather
        # than theoretical, and a mutation of the comparison dies here.
        ("exactly at the overlap threshold",
         "alpha bravo charlie delta echo foxtrot golf hotel india juliet",
         ["alpha bravo charlie delta echo foxtrot golf kilo lima mike november"]),
        ("one word below the threshold",
         "alpha bravo charlie delta echo foxtrot juliet hotel india golf",
         ["alpha bravo charlie delta echo foxtrot kilo lima mike november"]),
    ]:
        cases.append({"note": note, "text": text, "told": against,
                      "repeats": theme.repeats(text, tuple(against))})
    return cases


def theme_prose_cases() -> list[dict[str, Any]]:
    """`prose()`: control characters out, whitespace collapsed, `str(x or "")`."""
    from mtglab.claude import theme
    cases = []
    for note, value in [
        ("none", None),
        ("false is empty", False),
        ("zero is empty", 0),
        ("a number", 7),
        ("a float", 7.5),
        ("a list", ["a", "b"]),
        ("plain", "a plain sentence"),
        ("the form feed that ate a letter", "than policing the \x0cight"),
        ("DEL is not whitespace but goes anyway", "a\x7fb"),
        ("every C0 character", "".join(chr(c) for c in range(0x20)) + "x"),
        ("unicode whitespace collapses", "a\u00a0\u2003b\u3000c"),
        ("the information separators", "a\x1cb\x1dc\x1ed\x1fe"),
        ("leading and trailing", "   spaced   out   "),
        ("nothing but whitespace", " \t\n "),
    ]:
        cases.append({"note": note, "value": value, "prose": theme.prose(value)})
    return cases


def theme_told_cases() -> list[dict[str, Any]]:
    """`check_told()`: the door, and every refusal in its own words."""
    from mtglab.claude import theme
    cases = []
    for note, raw in [
        ("none", None),
        ("empty", []),
        ("plain strings", ["one", "two"]),
        ("blanks are dropped, not refused", ["one", "   ", ""]),
        ("stripped", ["  one  "]),
        ("not a list", "one"),
        ("not a list of strings", ["one", 2]),
        ("one too long", ["x" * (theme.MAX_FACT_CHARS + 1)]),
        ("one at the cap", ["x" * theme.MAX_FACT_CHARS]),
        ("counted in code points", ["é" * theme.MAX_FACT_CHARS]),
        ("more facts than exchanges", [f"fact {i}" for i in range(theme.MAX_EXCHANGES + 1)]),
    ]:
        row: dict[str, Any] = {"note": note, "raw": raw}
        try:
            row["told"] = list(theme.check_told(raw))
        except theme.TranscriptRejected as exc:
            row["error"] = str(exc)
        cases.append(row)
    return cases


def theme_transcript_cases() -> list[dict[str, Any]]:
    """`check_transcript()`: the door ADR 20 puts in front of client state."""
    from mtglab.claude import theme
    cases = []
    for note, raw in [
        ("none", None),
        ("empty", []),
        ("the fixture", THEME_TRANSCRIPT),
        ("not a list", {"role": "user", "text": "hi"}),
        ("a turn that is not an object", [["user", "hi"]]),
        ("an unknown role", [{"role": "system", "text": "hi"}]),
        ("a missing role", [{"text": "hi"}]),
        ("a role that is not a string", [{"role": 7, "text": "hi"}]),
        ("an empty turn", [{"role": "assistant", "text": "   "}]),
        ("a turn over the cap", [{"role": "assistant", "text": "x" * (theme.MAX_TURN_CHARS + 1)}]),
        ("a turn at the cap", [{"role": "assistant", "text": "x" * theme.MAX_TURN_CHARS}]),
        ("counted in code points", [{"role": "assistant", "text": "é" * theme.MAX_TURN_CHARS}]),
        ("the user speaking first", [{"role": "user", "text": "hi"}]),
        ("consecutive same-role turns are allowed", [
            {"role": "assistant", "text": "one"},
            {"role": "user", "text": "two"},
            {"role": "user", "text": "three"}]),
        ("a conversation past its ceiling",
         [{"role": "assistant", "text": "x"}] * (theme.MAX_EXCHANGES * 2 + 2)),
        ("an anthropic message block", [
            {"role": "user", "content": [{"type": "text", "text": "hi"}]}]),
    ]:
        row: dict[str, Any] = {"note": note, "raw": raw}
        try:
            row["turns"] = theme.check_transcript(raw)
        except theme.TranscriptRejected as exc:
            row["error"] = str(exc)
        cases.append(row)
    return cases


def theme_fact_cases() -> list[dict[str, Any]]:
    """`keep_fact()`: the three origins, and everything that is not one.

    The `tarot:` rows carry the whole point of ADR 21's corpus: the id is the
    ask, the model's `text` is discarded, and what the querent reads is the
    file's own sentence.
    """
    from mtglab import tarotlore
    from mtglab.claude import theme
    real = tarotlore.DECK_FACTS[0]
    cases = []
    for note, raw in [
        ("not an object", "a fact"),
        ("no text", {"text": "", "source": "taxonomy"}),
        ("no source", {"text": "A fact.", "source": ""}),
        ("taxonomy", {"text": "  White wants peace.  ", "source": "taxonomy"}),
        ("taxonomy, shouted", {"text": "White wants peace.", "source": "TAXONOMY"}),
        ("a tarot id, paraphrased away",
         {"text": "the model's own words", "source": f"tarot:{real.id}"}),
        ("a tarot id, shouted",
         {"text": "x", "source": f"TAROT:{real.id.upper()}"}),
        ("a tarot id nobody has", {"text": "x", "source": "tarot:not-a-fact"}),
        ("a page the search returned",
         {"text": "A thing on a page.", "source": "https://edhrec.com/real"}),
        ("the same page, differently spelled",
         {"text": "A thing on a page.", "source": "https://edhrec.com/real/"}),
        ("a page with no title",
         {"text": "A thing.", "source": "https://mtg.wiki/Gyome"}),
        ("a page nobody fetched",
         {"text": "A thing.", "source": "https://example.com/invented"}),
    ]:
        cases.append({"note": note, "raw": raw,
                      "fact": theme.keep_fact(raw, list(_PAGES))})
    return cases


def theme_seed_cases() -> list[dict[str, Any]]:
    """`int(seed)`, which is **not** the tarot route's Pydantic grammar.

    The two were measured apart in the tarot lane: `/api/tarot/reading` reads
    its seed off a query string through `seed: int | None`, which refuses the
    fullwidth `７` that `int()` reads as seven. This one is a JSON body going
    through the builtin, so the fullwidth digit lands, `5.9` truncates to
    five, and `True` is one.

    Recorded from `int()` directly rather than through `theme._reading_for`:
    what Go has to reproduce is the builtin, and routing the corpus through
    the caller would let a change of parser hide behind a matching change of
    expectation.
    """
    cases = []
    for note, raw in [
        ("none", None), ("zero", 0), ("an integer", 42), ("negative", -3),
        ("unbounded", 2 ** 70), ("a float truncates toward zero", 5.9),
        ("a negative float truncates toward zero", -5.9),
        ("a float in exponent form", 5e2),
        ("true is one", True), ("false is zero", False),
        ("a decimal string", "42"), ("a padded string", "  42  "),
        ("a signed string", "+42"), ("underscores between digits", "1_0"),
        ("a leading underscore", "_10"), ("a trailing underscore", "10_"),
        ("a doubled underscore", "1__0"),
        ("an underscore next to the sign", "+_10"),
        ("a fullwidth digit", "７"),
        ("arabic-indic digits", "٣٤"),
        ("a mathematical bold digit", "\U0001d7d0"),
        ("a float as a string", "5.9"),
        ("not a number at all", "soon"),
        ("empty", ""), ("whitespace only", "   "),
        ("a list", [1]), ("an object", {"seed": 1}),
    ]:
        row: dict[str, Any] = {"note": note, "raw": raw}
        try:
            row["seed"] = str(int(raw))
        except (TypeError, ValueError, OverflowError):
            row["error"] = f"not a usable reading seed: {raw!r}"
        cases.append(row)
    # And what `check_ask` does with each: a persona that does not deal drops
    # the seed rather than refusing it.
    return cases


def theme_budget_cases() -> list[dict[str, Any]]:
    """`f"{budget:g}"`, which Go's default `%g` is not."""
    cases = []
    for value in [50.0, 50.5, 0.5, 100.0, 1234567.0, 123456.0, 0.0001,
                  0.00001, 1e21, 1.5e-7, -50.0, 1/3, 2**53 + 1.0]:
        cases.append({"value": value, "formatted": f"{value:g}"})
    for note, value in [("inf", float("inf")), ("-inf", float("-inf")),
                        ("nan", float("nan"))]:
        cases.append({"note": note, "value": None, "formatted": f"{value:g}"})
    return cases


def theme_prompt_cases() -> dict[str, Any]:
    """The bytes that reach the model: the frame, the closing, and the ask.

    All three are assembled in Python from data Python owns, and all three are
    prompt rather than payload -- which is exactly why they are pinned as
    bytes. A frame that quietly lost the spread, or a closing instruction that
    stopped naming the missing slots, changes what the model was told and
    shows up nowhere in any report.
    """
    from mtglab.claude import persona as persona_mod
    from mtglab.claude import theme

    reader = persona_mod.PERSONAS["fortune-teller"]
    plain = persona_mod.PERSONAS[persona_mod.DEFAULT]
    told = ["Pamela Colman Smith drew all 78 cards and was paid one flat fee."]

    frames = []
    for note, who, seed, facts in [
        ("no reading at all", plain, None, ()),
        ("a reader with no seed", reader, None, ()),
        ("a plain voice ignores the seed", plain, 1909, ()),
        ("a reading, nothing told", reader, 1909, ()),
        ("a reading, one fact told", reader, 1909, tuple(told)),
        ("a reading, no corpus offered", reader, 1909, None),
    ]:
        dealt = theme._reading_for(who, seed)
        frames.append({"note": note, "persona": who.key, "seed": seed,
                       "told": None if facts is None else list(facts),
                       "frame": theme._frame_for(dealt, facts)})

    grounded, _ = theme.ground([
        _theme_slot("taste", "old horror films", "old horror films"),
        _theme_slot("temperament", "improviser", "I improvise"),
        _theme_slot("posture", "makes deals", "I make deals"),
    ], THEME_TRANSCRIPT)
    one, _ = theme.ground([
        _theme_slot("taste", "old horror films", "old horror films")],
        THEME_TRANSCRIPT)

    closings = []
    for note, slots, transcript, facts in [
        ("the opening turn", [], [], ()),
        ("the opening turn ignores what it was told", [], [], tuple(told)),
        ("two still missing", one, THEME_TRANSCRIPT, ()),
        ("two still missing, one fact told", one, THEME_TRANSCRIPT, tuple(told)),
        ("the floor is met", grounded, THEME_TRANSCRIPT, ()),
        ("the floor is met, facts told", grounded, THEME_TRANSCRIPT, tuple(told)),
    ]:
        closings.append({"note": note, "slots": slots, "told": list(facts),
                         "opening": not transcript,
                         "closing": theme._closing_for(slots, transcript, facts)})

    asks = []
    for note, budget, avoid in [
        ("nothing but the reading", None, ""),
        ("a budget", 50.0, ""),
        ("a zero budget is no budget", 0.0, ""),
        ("a budget that needs six significant digits", 1234567.0, ""),
        ("something to avoid", None, "  no blue please  "),
        ("whitespace is not something to avoid", None, "   "),
        ("both", 250.5, "nothing with infect"),
    ]:
        asks.append({"note": note, "budget": budget, "avoid": avoid,
                     "ask": theme._proposal_ask(grounded, budget, avoid)})

    return {"frames": frames, "closings": closings, "asks": asks,
            "grounded": grounded, "reading_seed": 1909,
            "opening_angles": list(theme.OPENING_ANGLES)}


def theme_float_cases() -> list[dict[str, Any]]:
    """`theme.read_budget`, whose `float()` is not `strconv.ParseFloat`.

    **Driven through the real function, not through `float()` by hand.** The
    tarot lane learnt that the expensive way: a test that reimplements the
    call it is checking passes against a mutant of the caller. So this asks
    `read_budget` and records what it answers.

    Two halves, and the first is the one a port can get wrong. The **grammar**
    is CPython's `float()` -- underscores between digits, any Unicode decimal
    digit, `inf`, `Infinity`, `NaN`, a leading `+`, surrounding whitespace,
    and *not* Go's `0x1p4` -- and every accepted value is recorded as `repr`
    so a one-ulp difference is a diff rather than a rounding nobody sees.

    The **refusal** is one sentence for every way the field can fail, which
    it was not until 2026-08-23: `float(budget)` sat in a `try` catching
    `ValueError` and not `TypeError`, so a list was an uncaught 500 and a bad
    string was a 422 quoting `float()` at the user. Ruled with Aaron and
    fixed in both runtimes at once, so `error_kind` is gone -- there is no
    longer a distinction to record, and that absence is the fix.
    """
    from mtglab.claude import theme

    cases = []
    for note, raw in [
        ("none is no budget", None), ("zero is no budget", 0),
        ("empty string is no budget", ""), ("false is no budget", False),
        ("an empty list is no budget", []),
        ("an integer", 50), ("a float", 50.5), ("a numeric string", "50"),
        ("a padded numeric string", "  50.5  "),
        ("a signed string", "-50"), ("an exponent", "5e2"),
        ("underscores between digits", "1_000.5"),
        ("a fullwidth digit", "５０"),
        ("inf", "inf"), ("Infinity", "Infinity"), ("-inf", "-inf"),
        ("nan", "NaN"),
        ("a hex float Go would take", "0x1p4"),
        ("not a number", "fifty"),
        ("an overflowing literal", "1e400"),
        ("true is one", True),
        ("a list refuses in the same words as a bad string", [1]),
        ("an object refuses in the same words", {"a": 1}),
        ("what somebody actually types", "about fifty quid"),
        ("a currency symbol", "$50"),
    ]:
        row: dict[str, Any] = {"note": note, "raw": raw}
        try:
            value = theme.read_budget(raw)
            row["budget"] = None if value is None else repr(value)
        except theme.BudgetRejected as exc:
            row["error"] = str(exc)
        cases.append(row)
    return cases


def theme_stance_cases() -> list[dict[str, Any]]:
    """`theme.stance_for`: the default is `second-opinion`, never `off`.

    The one every other mode gets from a deck's `status`, answered for a
    surface that runs before a deck exists. `/api/claude` reads it too, which
    is the reason it is public: a dial that reported `off` while the
    conversation was about to run at `second-opinion` is worse than no dial.
    """
    from mtglab.claude import stance as stance_mod
    from mtglab.claude import theme
    cases = []
    for note, requested, ceiling in [
        ("nothing asked for", None, None),
        ("nothing asked for, under a ceiling", None, "off"),
        ("nothing asked for, under a low ceiling", None, "sounding-board"),
        ("a preset", "consultant", None),
        ("a preset over the ceiling", "consultant", "sounding-board"),
        ("off is reachable", "off", None),
        ("a custom stance", {"initiative": "volunteers", "scope": "wide",
                             "write": "none"}, None),
        ("a malformed stance", {"initiative": 7}, None),
        ("a stance that is not a stance", 7, None),
    ]:
        saved = os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
        if ceiling:
            os.environ["MTGLAB_CLAUDE_STANCE_CEILING"] = ceiling
        try:
            row: dict[str, Any] = {"note": note, "requested": requested,
                                   "ceiling": ceiling}
            try:
                row["stance"] = stance_mod.describe(theme.stance_for(requested))
            except ValueError as exc:
                row["error"] = str(exc)
            cases.append(row)
        finally:
            os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
            if saved is not None:
                os.environ["MTGLAB_CLAUDE_STANCE_CEILING"] = saved
    return cases


def theme_report_cases() -> dict[str, Any]:
    """Every outcome of both halves, as marshalled bytes and key order.

    Driven through a fake `converse` so the Go side can rebuild the same Turn
    and compare the report it writes byte for byte -- which is the only way
    the **two shapes** each half has are pinned. A turn that reached the model
    carries `never` and one that did not does not; a proposal that resolved
    something carries five keys the other four shapes leave off, and they sit
    in the middle rather than at the end. One struct per half would put those
    keys on the wire in exactly the cases Python leaves them off.
    """
    from mtglab.claude import theme

    asks: list[dict[str, Any]] = []
    proposals: list[dict[str, Any]] = []
    real_converse = theme.converse

    grounded_in = [
        _theme_slot("taste", "old horror films", "old horror films"),
        _theme_slot("temperament", "improviser", "I improvise"),
        _theme_slot("posture", "makes deals", "I make deals"),
    ]

    def ask(note: str, turn: Any, *, transcript: Any = None, slots: Any = None,
            requested: Any = "second-opinion", persona: Any = None,
            seed: Any = None, facts: Any = None) -> None:
        def fake(*_a: Any, **_k: Any) -> Any:
            if turn is None:
                raise AssertionError(f"{note}: no call may be made")
            return turn
        theme.converse = fake
        try:
            report = theme.ask(THEME_TRANSCRIPT if transcript is None else transcript,
                               slots, requested=requested, persona=persona,
                               seed=seed, facts=facts)
        finally:
            theme.converse = real_converse
        asks.append({"note": note, "transcript": THEME_TRANSCRIPT if transcript is None else transcript,
                     "slots": slots, "requested": requested,
                     "persona": persona, "seed": seed, "facts": facts,
                     "turn": _turn_record(turn) if turn is not None else None,
                     "report": report})

    def propose(note: str, turn: Any, *, slots: Any = None,
                requested: Any = "second-opinion", budget: Any = None,
                avoid: str = "", persona: Any = None, seed: Any = None) -> None:
        def fake(*_a: Any, **_k: Any) -> Any:
            if turn is None:
                raise AssertionError(f"{note}: no call may be made")
            return turn
        theme.converse = fake
        try:
            report = theme.propose(THEME_TRANSCRIPT,
                                   grounded_in if slots is None else slots,
                                   requested=requested, budget=budget,
                                   avoid=avoid, persona=persona, seed=seed)
        finally:
            theme.converse = real_converse
        proposals.append({"note": note, "slots": grounded_in if slots is None else slots,
                          "requested": requested, "budget": budget,
                          "avoid": avoid, "persona": persona, "seed": seed,
                          "turn": _turn_record(turn) if turn is not None else None,
                          "report": report})

    conv = theme.THEME_CONVERSATION.name
    prop = theme.THEME_PROPOSAL.name

    # --- the conversation turn
    ask("stance off, no call", None, requested="off")
    ask("stance off keeps what the client carried", None, requested="off",
        slots=grounded_in)
    ask("past the ceiling, no call", None,
        transcript=[{"role": "assistant", "text": "q"},
                    {"role": "user", "text": "a"}] * theme.MAX_EXCHANGES,
        slots=grounded_in)
    ask("the model refused",
        _fake_turn(mode=conv, text="", stop_reason="refusal", refused=True))
    ask("the answer did not parse",
        _fake_turn(mode=conv, text='{"question": "trunc', stop_reason="max_tokens"))
    ask("a declarative sentence is not a question", _fake_turn(
        {"question": "You are clearly a green player.", "slots": []}, mode=conv))
    ask("a whole turn", _fake_turn({
        "question": "  What did the practical effects\x0cgive you?  ",
        "fact": {"text": "White wants peace.", "source": "taxonomy"},
        "slots": [
            _theme_slot("taste", "old horror films", "old horror films"),
            _theme_slot("temperament", "improviser", "I improvise"),
            _theme_slot("posture", "makes deals", "I make deals"),
            _theme_slot("anchor", "invented", "I love Sol Ring"),
            "not an object",
        ],
    }, mode=conv, searched=_PAGES, input_tokens=4900, output_tokens=310,
        cache_read_tokens=2791))
    ask("a turn that un-knew a slot", _fake_turn({
        "question": "And after that?",
        "slots": [_theme_slot("posture", "makes deals", "I make deals")],
    }, mode=conv), slots=grounded_in)
    ask("a fact already told is dropped and counted", _fake_turn({
        "question": "And after that?",
        "fact": {"text": "White wants peace.", "source": "taxonomy"},
        "slots": [],
    }, mode=conv), facts=["White wants peace."])
    ask("a fact from a page nobody fetched", _fake_turn({
        "question": "And after that?",
        "fact": {"text": "A thing.", "source": "https://example.com/invented"},
        "slots": [],
    }, mode=conv, searched=_PAGES))
    ask("the fortune-teller, reading", _fake_turn({
        "question": "The Fool steps off the cliff -- do you?",
        "fact": {"text": "the model's paraphrase", "source": "tarot:pixie-fee"},
        "slots": [_theme_slot("taste", "old horror films", "old horror films")],
    }, mode=conv), persona="fortune-teller", seed=1909)

    # --- the proposal
    propose("stance off, no call", None, requested="off")
    propose("the model refused",
            _fake_turn(mode=prop, text="", stop_reason="refusal", refused=True))
    propose("the answer did not parse",
            _fake_turn(mode=prop, text='{"combinations": [', stop_reason="max_tokens"))
    propose("nothing resolved against the pool", _fake_turn({
        "combinations": [
            {"key": "NOT-A-KEY", "reading": "x", "grounding": "y",
             "commanders": [{"card": "Gyome, Master Chef", "prose": "p"}]},
            {"key": "BG", "reading": "x", "grounding": "y",
             "commanders": [{"card": "Not A Real Card", "prose": "p"}]},
        ],
        "sources": [],
    }, mode=prop, searched=_PAGES))
    propose("a whole proposal", _fake_turn({
        "combinations": [
            {"key": "bg", "reading": "  Golgari, for the practical effects.  ",
             "grounding": "You said you improvise.",
             "source_ids": ["s1", "s2", "nope"],
             "commanders": [
                 {"card": "gyome, master chef", "prose": "A troll chef.",
                  "source_ids": ["s1"]},
                 {"card": "Not A Real Card", "prose": "invented",
                  "source_ids": ["s1"]},
                 {"card": "Craterhoof Behemoth", "prose": "mono-green, wrong slot",
                  "source_ids": ["s1"]},
                 {"card": 7, "prose": "a number"},
                 "not an object",
             ]},
            {"key": "G", "reading": "Mono-green, for the other reading.",
             "grounding": "You said you make deals.", "source_ids": [],
             "commanders": [{"card": "Craterhoof Behemoth", "prose": "Goes wide."}]},
            {"key": "W", "reading": "A third nobody asked for.",
             "grounding": "", "commanders": [
                 {"card": "Gyome, Master Chef", "prose": "wrong colours"}]},
        ],
        "sources": [
            {"id": "s1", "title": "the model's title", "url": "https://edhrec.com/real"},
            {"id": "s2", "title": "t", "url": "https://example.com/invented"},
        ],
    }, mode=prop, searched=_PAGES, input_tokens=8100, output_tokens=1450,
        cache_read_tokens=3200), budget=50.0, avoid="no blue")
    # Both caps, reached. Nothing above gets near them -- the third
    # combination there is dropped for having no confirmed commander, so the
    # two-combination cap is never exercised, and no combination above names
    # more than three legends that resolve. `tiny_pool` carries ten mono-green
    # cards, which is what makes a fourth commander possible at all.
    propose("more than the caps allow", _fake_turn({
        "combinations": [
            {"key": "G", "reading": "Mono-green.", "grounding": "You improvise.",
             "commanders": [
                 {"card": "Craterhoof Behemoth", "prose": "one"},
                 {"card": "Terastodon", "prose": "two"},
                 {"card": "Woodfall Primus", "prose": "three"},
                 {"card": "Vorinclex, Voice of Hunger", "prose": "four -- over the cap"},
             ]},
            {"key": "BG", "reading": "Golgari.", "grounding": "You make deals.",
             "commanders": [{"card": "Gyome, Master Chef", "prose": "a troll chef"}]},
            {"key": "W", "reading": "Mono-white.", "grounding": "You said films.",
             "commanders": [{"card": "Smothering Tithe", "prose": "third -- over the cap"}]},
        ],
        "sources": [{"id": "s1", "title": "t", "url": "https://edhrec.com/real"}],
    }, mode=prop, searched=_PAGES))

    return {"asks": asks, "proposals": proposals}


def theme_refusal_cases() -> list[dict[str, Any]]:
    """What `check_ask` and `check_proposal` refuse, in their own words.

    `NotReady` is the one with a status of its own -- 409, not 422 -- because
    nothing is malformed and nothing failed, there simply is not enough yet.
    """
    from mtglab.claude import theme
    cases = []
    one = [_theme_slot("taste", "old horror films", "old horror films")]
    three = [
        _theme_slot("taste", "old horror films", "old horror films"),
        _theme_slot("temperament", "improviser", "I improvise"),
        _theme_slot("posture", "makes deals", "I make deals"),
    ]
    for half, note, kwargs in [
        ("ask", "an empty conversation is fine", {"transcript": None}),
        ("ask", "a malformed transcript",
         {"transcript": [{"role": "system", "text": "hi"}]}),
        ("ask", "an unknown persona", {"persona": "not-a-voice"}),
        ("ask", "an unusable seed",
         {"persona": "fortune-teller", "seed": "soon"}),
        ("ask", "a seed a plain voice never deals", {"seed": "soon"}),
        ("ask", "a malformed stance", {"requested": {"initiative": 7}}),
        ("ask", "malformed facts", {"facts": ["one", 2]}),
        ("propose", "nothing known at all", {"slots": None}),
        ("propose", "one thing known", {"slots": one}),
        ("propose", "quotes nobody said",
         {"slots": [_theme_slot(s["kind"], s["value"], "never typed this")
                    for s in three]}),
        ("propose", "three things known", {"slots": three}),
        ("propose", "an unknown persona", {"slots": three, "persona": "not-a-voice"}),
        ("propose", "an unusable seed",
         {"slots": three, "persona": "fortune-teller", "seed": "soon"}),
    ]:
        row: dict[str, Any] = {"half": half, "note": note, **kwargs}
        try:
            if half == "ask":
                req = theme.check_ask(kwargs.get("transcript", THEME_TRANSCRIPT),
                                      kwargs.get("slots"),
                                      requested=kwargs.get("requested"),
                                      persona=kwargs.get("persona"),
                                      seed=kwargs.get("seed"),
                                      facts=kwargs.get("facts"))
                row["ok"] = {"persona": req.persona, "seed": req.seed,
                             "exchanges": req.exchanges,
                             "exhausted": req.exhausted,
                             "needs_call": req.needs_call,
                             "carried": req.carried, "told": list(req.told)}
            else:
                req = theme.check_proposal(THEME_TRANSCRIPT, kwargs.get("slots"),
                                           requested=kwargs.get("requested"),
                                           persona=kwargs.get("persona"),
                                           seed=kwargs.get("seed"))
                row["ok"] = {"persona": req.persona, "seed": req.seed,
                             "needs_call": req.needs_call,
                             "grounded": req.grounded, "dropped": req.dropped}
        except theme.NotReady as exc:
            row["error"] = str(exc)
            row["error_kind"] = "not-ready"
        except (theme.TranscriptRejected, ValueError) as exc:
            row["error"] = str(exc)
            row["error_kind"] = "rejected"
        cases.append(row)
    return cases


def theme_cases() -> dict[str, Any]:
    """`claude/theme.py`'s Python-owned halves, held still for the Go port."""
    import random as _random

    from mtglab.claude import theme

    with _ClaudeScratch():
        real_choice = _random.choice
        _random.choice = lambda seq: list(seq)[FROZEN_ANGLE_INDEX]
        try:
            prompts = theme_prompt_cases()
            reports = theme_report_cases()
        finally:
            _random.choice = real_choice
        return {
            "note": ("`claude/theme.py`'s Python-owned halves, written by "
                     "`python tests/go_fixtures.py` with the opening angle "
                     f"pinned to index {FROZEN_ANGLE_INDEX} and the clock "
                     f"frozen at {FROZEN_NOW}."),
            "floor": theme.FLOOR,
            "max_exchanges": theme.MAX_EXCHANGES,
            "max_turn_chars": theme.MAX_TURN_CHARS,
            "max_fact_chars": theme.MAX_FACT_CHARS,
            "min_quote_chars": theme.MIN_QUOTE_CHARS,
            "slot_kinds": list(theme.SLOT_KINDS),
            "slot_questions": dict(theme.SLOT_QUESTIONS),
            "frozen_angle_index": FROZEN_ANGLE_INDEX,
            "transcript": THEME_TRANSCRIPT,
            "pages": list(_PAGES),
            "prose": theme_prose_cases(),
            "ground": theme_ground_cases(),
            "carry": theme_carry_cases(),
            "repeats": theme_repeat_cases(),
            "told": theme_told_cases(),
            "transcripts": theme_transcript_cases(),
            "facts": theme_fact_cases(),
            "seeds": theme_seed_cases(),
            "budgets": theme_budget_cases(),
            "floats": theme_float_cases(),
            "stances": theme_stance_cases(),
            "prompts": prompts,
            "refusals": theme_refusal_cases(),
            **reports,
        }


def render_theme_cases() -> str:
    return _rows_json(theme_cases()) + "\n"


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
    WHEEL_PATH.parent.mkdir(parents=True, exist_ok=True)
    WHEEL_PATH.write_text(render_wheel_cases(), encoding="utf-8")
    print(f"wrote {WHEEL_PATH}")
    SETS_PATH.parent.mkdir(parents=True, exist_ok=True)
    SETS_PATH.write_text(render_sets_cases(), encoding="utf-8")
    print(f"wrote {SETS_PATH}")
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    (LOG_DIR / "describe.json").write_text(render_log_cases(), encoding="utf-8")
    print(f"wrote the log oracle into {LOG_DIR}")
    APP_SCHEMA_PATH.parent.mkdir(parents=True, exist_ok=True)
    APP_SCHEMA_PATH.write_text(render_app_schema(), encoding="utf-8")
    print(f"wrote app.db's schema into {APP_SCHEMA_PATH}")
    EDITS_PATH.parent.mkdir(parents=True, exist_ok=True)
    EDITS_PATH.write_text(render_edit_cases(), encoding="utf-8")
    print(f"wrote {EDITS_PATH}")
    DUMPS_PATH.parent.mkdir(parents=True, exist_ok=True)
    DUMPS_PATH.write_text(render_dump_cases(), encoding="utf-8")
    print(f"wrote {len(dump_cases()) + len(dump_from_text())} dump cases "
          f"into {DUMPS_PATH}")
    ARTIFACTS_PATH.parent.mkdir(parents=True, exist_ok=True)
    ARTIFACTS_PATH.write_text(render_artifact_cases(), encoding="utf-8")
    print(f"wrote {len(artifact_cases())} artifact cases into {ARTIFACTS_PATH}")
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
    SCORE_PATH.parent.mkdir(parents=True, exist_ok=True)
    SCORE_PATH.write_text(render_score_cases(), encoding="utf-8")
    print(f"wrote {len(score_specs())} scorer cases into {SCORE_PATH}")
    TAROT_DATA_PATH.parent.mkdir(parents=True, exist_ok=True)
    TAROT_DATA_PATH.write_text(render_tarot_deck(), encoding="utf-8")
    TAROT_PATH.parent.mkdir(parents=True, exist_ok=True)
    TAROT_PATH.write_text(render_tarot_deals(), encoding="utf-8")
    print(f"wrote {len(tarot_deck_payload()['cards'])} tarot cards and "
          f"{len(tarot_deal_cases()['cases'])} deals into {TAROT_PATH.parent.parent}")
    LEDGER_PATH.parent.mkdir(parents=True, exist_ok=True)
    LEDGER_PATH.write_text(render_ledger_cases(), encoding="utf-8")
    print(f"wrote {len(ledger_cases()['queries'])} ledger roll-ups into {LEDGER_PATH}")
    TOOLS_PATH.parent.mkdir(parents=True, exist_ok=True)
    TOOLS_PATH.write_text(render_tools_payload(), encoding="utf-8")
    print(f"wrote {len(tools_payload()['tools'])} tool schemas into {TOOLS_PATH}")
    PERSONA_PATH.parent.mkdir(parents=True, exist_ok=True)
    PERSONA_PATH.write_text(render_persona_payload(), encoding="utf-8")
    print(f"wrote {len(persona_payload()['personas'])} voices into {PERSONA_PATH}")
    CASEFOLD_PATH.parent.mkdir(parents=True, exist_ok=True)
    CASEFOLD_PATH.write_text(render_casefold_payload(), encoding="utf-8")
    DIGITS_PATH.write_text(render_digits_payload(), encoding="utf-8")
    print(f"wrote {digits_payload()['runs']} digit runs into {DIGITS_PATH}")
    print(f"wrote {len(casefold_payload()['folds'])} case foldings into {CASEFOLD_PATH}")
    MODES_PATH.parent.mkdir(parents=True, exist_ok=True)
    MODES_PATH.write_text(render_modes_payload(), encoding="utf-8")
    print(f"wrote {len(modes_payload()['modes'])} mode definitions into {MODES_PATH}")
    STANCE_PATH.parent.mkdir(parents=True, exist_ok=True)
    STANCE_PATH.write_text(render_stance_cases(), encoding="utf-8")
    SCAN_PATH.parent.mkdir(parents=True, exist_ok=True)
    SCAN_PATH.write_text(render_scan_cases(), encoding="utf-8")
    print(f"wrote the camera reader's corpus into {SCAN_PATH}")
    PYSTR_PATH.parent.mkdir(parents=True, exist_ok=True)
    PYSTR_PATH.write_text(render_pystr_cases(), encoding="utf-8")
    print(f"wrote {len(pystr_cases())} str()/repr() cases into {PYSTR_PATH}")
    PYTEXT_PATH.parent.mkdir(parents=True, exist_ok=True)
    PYTEXT_PATH.write_text(render_pytext_payload(), encoding="utf-8")
    _pytext = pytext_payload()
    print(f"wrote {len(_pytext['space_ranges'])} whitespace spans and "
          f"{len(_pytext['break_ranges'])} line-boundary spans into {PYTEXT_PATH}")
    FORGE_PATH.parent.mkdir(parents=True, exist_ok=True)
    FORGE_PATH.write_text(render_forge_cases(), encoding="utf-8")
    _forge = forge_cases()
    print(f"wrote {len(_forge['logs'])} Forge logs, {len(_forge['dck'])} dck "
          f"exports and {len(_forge['shape']['shapes'])} shaped matches "
          f"into {FORGE_PATH}")
    ARGUE_PATH.parent.mkdir(parents=True, exist_ok=True)
    ARGUE_PATH.write_text(render_argue_cases(), encoding="utf-8")
    SOURCES_PATH.write_text(render_sources_cases(), encoding="utf-8")
    DOSSIER_PATH.write_text(render_dossier_cases(), encoding="utf-8")
    RESEARCH_PATH.write_text(render_research_cases(), encoding="utf-8")
    THEME_PATH.write_text(render_theme_cases(), encoding="utf-8")
    print(f"wrote the dossier and research corpora into {DOSSIER_PATH.parent}")
    _stance = stance_cases()
    print(f"wrote {len(_stance['stances'])} stances and "
          f"{len(_stance['clamps'])} clamp pairs into {STANCE_PATH}")
    CRYPTO_PATH.parent.mkdir(parents=True, exist_ok=True)
    CRYPTO_PATH.write_text(render_crypto_cases(), encoding="utf-8")
    print(f"wrote the Argon2id and token-hash oracle into {CRYPTO_PATH}")
    PYRAND_PATH.parent.mkdir(parents=True, exist_ok=True)
    PYRAND_PATH.write_text(render_pyrand_cases(), encoding="utf-8")
    print(f"wrote the draw corpus for {len(PYRAND_SEEDS)} seeds into {PYRAND_PATH}")
    JOBS_PATH.parent.mkdir(parents=True, exist_ok=True)
    JOBS_PATH.write_text(render_jobs_cases(), encoding="utf-8")
    print(f"wrote the job registry oracle into {JOBS_PATH}")
    write_closed_forms()
    TIER1_PATH.parent.mkdir(parents=True, exist_ok=True)
    TIER1_PATH.write_text(render_tier1_cases(), encoding="utf-8")
    print(f"wrote the Tier 1 corpus into {TIER1_PATH}")
    MANA_PATH.parent.mkdir(parents=True, exist_ok=True)
    castability = mana_cases()
    MANA_PATH.write_text(_compact_json(castability) + "\n", encoding="utf-8")
    print(f"wrote {castability['cases']} castability cases into {MANA_PATH}")
    write_sim_engine()


# ------------------------------------------------------------------ pyrand
#
# CPython's `random.Random`, reproduced in Go (PLAN section 5 item 3), and the
# corpus that says the reproduction is exact rather than merely plausible.
#
# Three things make this corpus different from every other one in this file.
#
# **It records the generator, not only its consumers.** `getrandbits(32)` is
# one `genrand_uint32()` word and nothing else -- CPython's fast path is
# `genrand_uint32(self) >> (32 - k)` -- so `words` below is the raw Mersenne
# Twister stream, the only place a seeding or twist bug can be seen *as* a
# seeding or twist bug. Every other section consumes that stream through a
# method, and a mismatch there with `words` matching localises the fault to
# the consumer. That distinction is the whole reason this corpus has a layer
# nothing in the app calls directly.
#
# **The floats are compared as bits.** `random()` is
# `(a >> 5) * 67108864.0 + (b >> 6)) * (1.0 / 9007199254740992.0)` over two
# words, in that order; every value it can produce is exactly representable,
# so a tolerance would be hiding something rather than allowing for something.
# They are stored as `math.Float64bits`.
#
# **The seeds are strings.** `random.Random(n)` takes an int of any size and
# splits `abs(n)` into little-endian 32-bit words for `init_by_array`, so the
# number of words -- and therefore the whole stream -- changes at 2**32 and
# again at 2**64. A JSON number would not survive the round trip, and the
# seeds past 2**64 are exactly the ones worth having.
#
# Everything here is stdlib CPython and takes no pool, no network and no deck.

#: Where the Go module reads the draw corpus from.
PYRAND_PATH = ROOT / "go" / "internal" / "pyrand" / "testdata" / "draws.json"

#: The seeds the corpus is generated over, chosen so that every branch of
#: CPython's seeding path is taken by something: zero (`bits == 0`, so one
#: word of nothing), the small ints real callers pass, both sides of 2**32
#: and of 2**64 (where `keyused` grows), a seed past 2**96, and negatives --
#: which `random_seed` runs through `abs()`, so -7 and 7 are the same stream
#: and the corpus proves it rather than asserting it.
PYRAND_SEEDS: tuple[int, ...] = (
    0, 1, 2, 7, 42, 99, 4096, 20260810, 2147483647,
    4294967295, 4294967296, 4294967297,
    18446744073709551615, 18446744073709551616, 18446744073709551617,
    79228162514264337593543950337,
    -1, -7, -20260810, -4294967296,
)

#: The seeds that get the full-width sections. The rest get a narrower slice
#: of the same shapes: seeding breadth is what the wide set buys, and it is
#: bought by `words`, which every seed gets in full.
PYRAND_CORE_SEEDS: tuple[int, ...] = (0, 1, 20260810, 4294967296,
                                      18446744073709551616, -7)

#: How many raw words each seed records. 700 crosses MT19937's 624-word
#: regeneration boundary, which is where a wrong twist first shows and where a
#: generator that is merely *seeded* right stops being right.
PYRAND_WORDS = 700

#: Every bit width `getrandbits` is asked for. 1 through 64 covers the fast
#: path (k <= 32, one word), the two-word path, and the boundary at exactly
#: 32 and 64 where the last word's `r >>= (32 - k)` is and is not applied.
PYRAND_BITS = tuple(range(1, 65))

#: A width sequence drawn from one generator, so the corpus also says the
#: state threads correctly between calls of *different* widths -- a bug that
#: a per-width sweep from a fresh generator cannot see.
PYRAND_BITS_MIXED = (1, 3, 7, 8, 15, 16, 17, 31, 32, 33, 53, 64, 5, 2, 64, 32)

#: The bounds `_randbelow` is asked for. Powers of two and their neighbours,
#: because `k = n.bit_length()` makes the rejection rate jump there: at
#: n = 2**k the loop never rejects, and at n = 2**k + 1 it rejects almost
#: half the time. A port that gets the rejection wrong agrees on the first
#: draw and diverges on the tenth. 1 is in the list on purpose -- it always
#: returns 0 and is never free, because `getrandbits(1)` is redrawn until it
#: comes up 0.
PYRAND_BELOW = (
    1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65,
    99, 100, 255, 256, 257, 1000, 4096, 1000000,
    2147483647, 2147483648, 4294967295, 4294967296, 4294967297,
    9007199254740992, 4611686018427387904,
)

#: The `randrange` forms, as `(start, stop, step)` with `None` for absent.
#: Only the one-argument form has a caller in the served package (tarot's
#: `randrange(2**31)`, the wheel's three) -- the rest are here because they
#: are the same `_randbelow` reached by different arithmetic, and arithmetic
#: is cheap to get wrong in a language with different integer division.
PYRAND_RANGES: tuple[tuple[int, int | None, int | None], ...] = (
    (10, None, None), (2, None, None), (1, None, None),
    (10, 20, None), (-5, 5, None), (-20, -10, None), (1, 2, None),
    (0, 100, 7), (0, 10, 3), (5, 6, 1), (-100, -1, 3),
    (100, 0, -7), (10, -10, -3), (0, -1, -1),
)

#: Deck-sized and degenerate lengths for `shuffle`. 0 and 1 draw nothing at
#: all (the loop is `reversed(range(1, len(x)))`); 99 is a Commander deck,
#: which is the length Tier 1 actually shuffles.
PYRAND_SHUFFLES = (0, 1, 2, 3, 4, 5, 7, 13, 52, 60, 99, 100, 101, 250)

#: What a seed outside the core set still gets shuffled.
PYRAND_SHUFFLES_NARROW = (13, 99)


def _pyrand_seed_case(seed: int, core: bool) -> dict[str, Any]:
    """Every draw one seed produces, each section from its own generator.

    A fresh `random.Random` per section is deliberate: it makes each section
    independently checkable, so a Go test that fails one of them has not been
    put out of step by an earlier one.
    """
    case: dict[str, Any] = {"seed": str(seed)}

    #: The raw stream. `getrandbits(32)` is exactly `genrand_uint32()`.
    rng = random.Random(seed)
    case["words"] = [rng.getrandbits(32) for _ in range(PYRAND_WORDS)]

    rng = random.Random(seed)
    case["randoms"] = [_float_bits(rng.random())
                       for _ in range(120 if core else 40)]

    rng = random.Random(seed)
    case["bits_mixed"] = [
        {"k": k, "value": rng.getrandbits(k)}
        for _ in range(3) for k in PYRAND_BITS_MIXED
    ]

    #: `randrange(n)` is `_randbelow(n)` for n > 0 and reaches it through the
    #: public door, so the corpus never touches a private name.
    rng = random.Random(seed)
    case["below"] = [
        {"n": n, "value": rng.randrange(n)}
        for _ in range(3 if core else 1) for n in PYRAND_BELOW
    ]

    rng = random.Random(seed)
    ranges: list[dict[str, Any]] = []
    for _ in range(3 if core else 1):
        for start, stop, step in PYRAND_RANGES:
            if stop is None:
                value = rng.randrange(start)
            elif step is None:
                value = rng.randrange(start, stop)
            else:
                value = rng.randrange(start, stop, step)
            ranges.append({"start": start, "stop": stop, "step": step,
                           "value": value})
    case["ranges"] = ranges

    lengths = PYRAND_SHUFFLES if core else PYRAND_SHUFFLES_NARROW
    case["shuffles"] = []
    for n in lengths:
        rng = random.Random(seed)
        order = list(range(n))
        rng.shuffle(order)
        case["shuffles"].append({"n": n, "order": order})

    #: Ten shuffles from *one* generator, which is Tier 1's exact shape: one
    #: `random.Random(seed)` per run, one shuffle per mulligan, all games.
    rng = random.Random(seed)
    repeated: list[list[int]] = []
    for _ in range(10):
        order = list(range(99))
        rng.shuffle(order)
        repeated.append(order)
    case["repeated_99"] = repeated

    rng = random.Random(seed)
    case["choices"] = [rng.choice(range(7)) for _ in range(24)]

    return case


def _float_bits(value: float) -> int:
    """`math.Float64bits`, so a double is compared as the bits it is."""
    return int(struct.unpack("<Q", struct.pack("<d", value))[0])


def pyrand_bits_sweep() -> list[dict[str, Any]]:
    """`getrandbits(k)` for every k in 1..64, each from a fresh generator.

    Separated from the per-seed sections because what it isolates is the
    *width* arithmetic -- how many words k consumes and which one gets
    shifted -- and mixing that with state threading would leave a failure
    ambiguous between the two.
    """
    out: list[dict[str, Any]] = []
    for seed in PYRAND_CORE_SEEDS:
        for k in PYRAND_BITS:
            rng = random.Random(seed)
            out.append({"seed": str(seed), "k": k,
                        "values": [rng.getrandbits(k) for _ in range(4)]})
    return out


class _Recording(random.Random):
    """A `random.Random` that keeps a note of everything it was asked.

    Tier 1 is not ported here -- that is Phase 5's own job -- so the strongest
    honest claim this corpus can make about `REFERENCE_DIGEST` is that the Go
    generator produces the *stream the digest is computed over*. This is the
    instrument that reads that stream off a real reference run.

    It works because of a fact about the engine worth writing down: Tier 1
    consumes randomness through exactly one call, `rng.shuffle(deck)` in
    `simulate_game`, and through nothing else. So the whole entropy budget of
    a run is a sequence of shuffles of a known length, and a shuffle of length
    L is exactly `_randbelow(i + 1)` for i from L-1 down to 1. Record the
    lengths and the returns and you have described the run's randomness
    completely, without needing a single card fact.

    Both overrides delegate: `_randbelow` returns what CPython's
    `_randbelow_with_getrandbits` returns, so the instrument consumes nothing
    and changes nothing. `render_pyrand_cases` proves that rather than
    claiming it -- it re-runs the digest under instrumentation and refuses to
    write a corpus if `REFERENCE_DIGEST` moved.
    """

    def __init__(self, x: Any = None) -> None:
        super().__init__(x)
        self.pyrand_seed = x
        self.pyrand_lengths: list[int] = []
        self.pyrand_draws: list[tuple[int, int]] = []
        _RECORDED.append(self)

    def _randbelow(self, n: int) -> int:
        value = int(super()._randbelow(n))  # type: ignore[misc]
        self.pyrand_draws.append((n, value))
        return value

    def shuffle(self, x: Any) -> None:
        self.pyrand_lengths.append(len(x))
        super().shuffle(x)


#: Every `_Recording` built while the patch is in place, in construction
#: order -- which is the order `reference_outputs()` builds them in.
_RECORDED: list[_Recording] = []

#: How many draws either end of a generator's stream are recorded verbatim.
#: A digest is opaque, and this repository has already learned once that an
#: opaque golden can stay stable while the thing under it stops happening
#: (`test_the_reference_run_is_the_shape_the_digest_assumes`). These are the
#: shape beside the digest: a Go run that matches them and misses the digest
#: has diverged somewhere in the middle, which is a different bug from one
#: that never agreed at all.
PYRAND_TIER1_SAMPLE = 8


def _draw_digest(draws: list[tuple[int, int]]) -> str:
    """sha256 over a draw sequence, defined so Go can compute it too.

    One draw per line as `n:value`, decimal, `\\n`-separated with a trailing
    newline. `n` is in the hash as well as the return, so the digest pins the
    *order the shuffle loop asks in* -- `reversed(range(1, len(x)))` -- and
    not only what came back.
    """
    text = "".join(f"{n}:{value}\n" for n, value in draws)
    return hashlib.sha256(text.encode()).hexdigest()


def pyrand_tier1_stream() -> dict[str, Any]:
    """The draw sequence `REFERENCE_DIGEST` is computed over.

    Runs `determinism_probe.reference_outputs()` with `random.Random` patched
    to the recorder above, then reports, per generator the run built: the seed
    it was given, the lengths it was asked to shuffle in order, how many draws
    that came to, the first and last few verbatim, and a digest of all of
    them.

    The patch is global for the duration and restored in a `finally`, because
    `engine.run` builds its generator itself -- there is no seam to inject one
    through, and inventing one for a test would be changing the thing under
    measurement.
    """
    import determinism_probe as probe

    original = random.Random
    _RECORDED.clear()
    try:
        random.Random = _Recording  # type: ignore[misc]
        digest = probe.reference_digest()
    finally:
        random.Random = original  # type: ignore[misc]

    generators = []
    for rng in _RECORDED:
        draws = rng.pyrand_draws
        generators.append({
            "seed": str(rng.pyrand_seed),
            "lengths": list(rng.pyrand_lengths),
            "draws": len(draws),
            "first": [{"n": n, "value": v}
                      for n, v in draws[:PYRAND_TIER1_SAMPLE]],
            "last": [{"n": n, "value": v}
                     for n, v in draws[-PYRAND_TIER1_SAMPLE:]],
            "digest": _draw_digest(draws),
        })
    return {
        "reference_digest": digest,
        "seed": str(probe.SEED),
        "games": probe.GAMES,
        "turns": probe.TURNS,
        "sweep_counts": list(probe.SWEEP_COUNTS),
        "generators": generators,
    }


def _compact_json(obj: Any, indent: int = 0) -> str:
    """JSON that keeps a draw sequence on one line.

    `indent=1` -- what every other corpus in this file uses -- would put each
    of the 14,000 words on a line of its own. One line per *sequence* is both
    smaller and the granularity a diff wants: a changed stream shows as one
    changed line naming its seed.
    """
    pad = " " * indent
    if isinstance(obj, dict):
        if not obj:
            return "{}"
        if all(v is None or isinstance(v, (bool, int, float, str))
               for v in obj.values()):
            return json.dumps(obj, ensure_ascii=False)
        body = ",\n".join(f"{pad} {json.dumps(k)}: {_compact_json(v, indent + 1)}"
                          for k, v in obj.items())
        return "{\n" + body + "\n" + pad + "}"
    if isinstance(obj, list):
        if not obj:
            return "[]"
        if all(isinstance(v, int) and not isinstance(v, bool) for v in obj):
            return json.dumps(obj)
        body = ",\n".join(f"{pad} {_compact_json(v, indent + 1)}" for v in obj)
        return "[\n" + body + "\n" + pad + "]"
    return json.dumps(obj, ensure_ascii=False)


def pyrand_floor_div() -> list[dict[str, int]]:
    """Python's `//` over the numerators and divisors `randrange` builds.

    `randrange` counts its steps with floor division, and Go's `/` truncates
    towards zero. The two disagree only for a quotient that is negative and
    inexact -- and, worked through, every such case describes an empty range,
    which both implementations then refuse. So no *successful* `randrange`
    can tell the two apart, and the corpus above cannot pin the difference:
    a Go port that truncated would pass every case in it.

    That is an argument, and this file's standing rule is that an argument
    about equivalence is a thing to check rather than a thing to believe. So
    the division itself gets a corpus, taken from Python, covering the signs
    the argument turns on. It is the only section here that records something
    no caller can reach -- deliberately, because unreachable-by-argument is
    exactly the claim that rots.
    """
    out: list[dict[str, int]] = []
    for a in (-13, -12, -7, -3, -1, 0, 1, 3, 7, 12, 13, 100, -100):
        for b in (-7, -3, -2, -1, 1, 2, 3, 7):
            out.append({"a": a, "b": b, "want": a // b})
    return out


def pyrand_cases() -> dict[str, Any]:
    """The whole corpus, as the Go module reads it."""
    return {
        "note": ("Generated from CPython's own random.Random by "
                 "`python tests/go_fixtures.py`. Seeds are strings because "
                 "they outgrow a JSON number; floats are Float64bits."),
        "cross_version": ("These bytes must be identical on every CPython this "
                          "project supports. Nothing here records which one "
                          "wrote them, deliberately: the drift test in "
                          "tests/test_go_fixtures.py re-renders the corpus and "
                          "compares, and CI runs it on each version in the "
                          "matrix -- so cross-version stability is re-proven on "
                          "every push rather than asserted once in a comment."),
        "seeds": [_pyrand_seed_case(seed, seed in PYRAND_CORE_SEEDS)
                  for seed in PYRAND_SEEDS],
        "bits_sweep": pyrand_bits_sweep(),
        "floor_div": pyrand_floor_div(),
        "tier1": pyrand_tier1_stream(),
    }


def render_pyrand_cases() -> str:
    return _compact_json(pyrand_cases()) + "\n"

# --------------------------------------------------------- the closed forms
#
# `sim/karsten.py` and `sim/curve.py` -- Tier 1.5, ported in Phase 5 (PLAN
# section 5 item 4: "the closed forms match to float tolerance... Go must
# agree to within an epsilon pinned per function").
#
# The epsilon this corpus is written to support is **zero**, and that is a
# decision rather than an accident. Every integer these modules produce comes
# out of a `>=` against a float -- `required_sources` scans until the odds
# clear the target, `CardOdds.reliable_turn` scans against 0.90,
# `_slots_to_target` scans until `on_curve_odds` clears, and `curve`'s advice
# branches on `abs(per_land - per_ramp) < TOO_CLOSE`. One ulp of disagreement
# in any of those is not a rounding difference on a screen; it is a different
# land count, a different reliable turn, a different row order in the shelf,
# or a different recommendation. So Go reproduces the arithmetic exactly --
# `math/big` where Python has `math.comb`, Shewchuk's summation where Python
# has `math.fsum` -- and this corpus asserts bit equality rather than
# nearness. A tolerance is still pinned per function on the Go side, so that
# a future divergence names which function drifted.
#
# Floats travel as plain JSON numbers, which is exact in both directions:
# `json.dumps` writes a float through `repr`, which is the shortest string
# that round-trips, and Go's `encoding/json` parses it with `ParseFloat`,
# which is correctly rounded. No value here is NaN or infinite.
#
# Rows are lists rather than objects. A grid of 2,352 hypergeometrics is a
# table, and naming its four columns 2,352 times would quadruple the file to
# say nothing; the column order is written down beside each one.

#: The CPython float behaviours the Go `sim` package reproduces: `math.fsum`,
#: `round(x)` and `round(x, n)`. Their own corpus, because they are the floor
#: the two closed forms stand on and a failure there should not be reported as
#: a failure of arithmetic built on top of them.
#: Its own package since 2026-08-22 -- `go/internal/pyfloat`, beside `pyrand`
#: and `pyyaml` -- because `artifacts`, `analyze` and `suggest` need `Fsum`
#: too, and none of the three is the simulator.
PYFLOAT_PATH = ROOT / "go" / "internal" / "pyfloat" / "testdata" / "pyfloat.json"
#: Tier 1.5's closed form: the hypergeometrics, the requirement, the
#: castability heatmap, the regression land count, and whole shelves.
KARSTEN_PATH = ROOT / "go" / "internal" / "sim" / "karsten" / "testdata" / "karsten.json"
#: The mana curve: the two distributions, the two-dial question, the land-drop
#: truth, and whole curves with their advice.
CURVE_PATH = ROOT / "go" / "internal" / "sim" / "curve" / "testdata" / "curve.json"


#: Anything whose compact JSON fits in this many characters is written on one
#: line. A row of a table, a compiled card, one rung of a pip ladder -- each is
#: one thing, and the diff a reader wants names the thing rather than the
#: field inside it that moved. It is also most of the reason these three files
#: are a third of the size they were when every leaf got its own line.
_ONE_LINE = 200


def _rows_json(obj: Any, indent: int = 0) -> str:
    """JSON that keeps one row of a table on one line.

    `_compact_json` above keeps a list of *ints* on one line, which is what a
    draw stream wants. These tables are rows of mixed scalars -- three ints, a
    float, a bool -- and their leaves are small records, so the rule here is
    one line per flat list and one line per record that fits.
    """
    pad = " " * indent
    flat = json.dumps(obj, ensure_ascii=False, separators=(",", ":"))
    if len(flat) <= _ONE_LINE:
        return flat
    if isinstance(obj, dict):
        body = ",\n".join(f"{pad} {json.dumps(k)}: {_rows_json(v, indent + 1)}"
                          for k, v in obj.items())
        return "{\n" + body + "\n" + pad + "}"
    if isinstance(obj, list):
        body = ",\n".join(f"{pad} {_rows_json(v, indent + 1)}" for v in obj)
        return "[\n" + body + "\n" + pad + "]"
    return flat


# ------------------------------------------------------ the fixture decks

def _sim_land(name: str, *colors: str, tapped: bool = False) -> SimCard:
    return SimCard(name=name, is_land=True, enters_tapped=tapped,
                   category="land",
                   produces=(ManaSource(frozenset(colors)),))


def _sim_spell(name: str, cost: str, category: str = "utility") -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost), category=category)


def _sim_rock(name: str, cost: str, colors: str = "C", amount: int = 1,
              delay: int = 0) -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost), category="ramp",
                   produces=(ManaSource(frozenset(colors), amount),),
                   produce_delay=delay)


def _sim_fetch(name: str, cost: str, lands: int = 1) -> SimCard:
    return SimCard(name=name, cost=parse_mana_cost(cost), category="ramp",
                   fetches_lands=lands)


def _mono_green(lands: int, spells: int) -> list[SimCard]:
    return ([_sim_land(f"Forest {i}", "G") for i in range(lands)]
            + [_sim_spell(f"Bear {i}", "{1}{G}") for i in range(spells)])


def closed_form_decks() -> dict[str, dict[str, Any]]:
    """The decks both corpora are computed over, and why each one is here.

    A grid of numbers proves the arithmetic; a deck proves the *reading* of a
    deck, which is where the two modules do their real work. Each entry names
    the property it exists to exercise, because a corpus that loses one still
    passes every case left in it.
    """
    naya = (
        [_sim_land(f"Forest {i}", "G") for i in range(8)]
        + [_sim_land(f"Plains {i}", "W") for i in range(7)]
        + [_sim_land(f"Mountain {i}", "R") for i in range(6)]
        + [_sim_land(f"Temple {i}", "G", "W", tapped=True) for i in range(5)]
        + [_sim_land(f"Command Tower {i}", "W", "U", "B", "R", "G")
           for i in range(4)]
        + [_sim_land(f"Waste {i}", "C") for i in range(3)]
        + [_sim_rock("Sol Ring", "{1}", "C", amount=2)]
        + [_sim_rock("Birds of Paradise", "{G}", "WUBRG", delay=1)]
        + [_sim_rock("Selesnya Signet", "{2}", "GW")]
        + [_sim_fetch("Cultivate", "{2}{G}", lands=2)]
        + [_sim_fetch("Rampant Growth", "{1}{G}", lands=1)]
        + [_sim_spell("Naya Charm", "{R}{G}{W}")]
        + [_sim_spell("Hybrid Hero", "{G/W}{G/W}")]
        + [_sim_spell("Split Demand", "{G/W}{G}")]
        + [_sim_spell("Compleated One", "{2}{U/P}")]
        + [_sim_spell("Colourless Engine", "{4}")]
        + [_sim_spell("Ghalta", "{10}{G}{G}")]
        + [_sim_spell(f"Beast {i}", "{2}{G}") for i in range(20)]
        + [_sim_spell(f"Angel {i}", "{3}{W}{W}") for i in range(15)]
        + [_sim_spell(f"Dragon {i}", "{4}{R}") for i in range(10)]
    )
    esper = (
        [_sim_land(f"Island {i}", "U") for i in range(11)]
        + [_sim_land(f"Swamp {i}", "B") for i in range(11)]
        + [_sim_land(f"Plains {i}", "W") for i in range(11)]
        + [_sim_rock(f"Signet {i}", "{2}", "WUB") for i in range(5)]
        + [_sim_rock(f"Talisman {i}", "{2}", "UB", delay=0) for i in range(3)]
        + [_sim_rock("Mana Vault", "{1}", "C", amount=3)]
        + [_sim_spell(f"Counterspell {i}", "{U}{U}") for i in range(12)]
        + [_sim_spell(f"Removal {i}", "{1}{B}") for i in range(12)]
        + [_sim_spell(f"Wrath {i}", "{2}{W}{W}") for i in range(10)]
        + [_sim_spell(f"Esper Thing {i}", "{W}{U}{B}") for i in range(8)]
        + [_sim_spell(f"Filler {i}", "{3}") for i in range(15)]
    )
    ladder = [*_mono_green(34, 62),
              _sim_spell("Cheap", "{G}"),
              _sim_spell("Middling", "{1}{G}{G}"),
              _sim_spell("Greedy", "{G}{G}{G}")]
    hybrids = (_mono_green(30, 60)
               + [_sim_spell(f"Hybrid {i}", "{G/W}") for i in range(5)]
               + [_sim_spell(f"Both Ways {i}", "{G/W}{G}") for i in range(4)]
    )
    return {
        "mono-green": {
            "why": "the canonical fixture: 34 Forests, 65 two-drop bears",
            "library": _mono_green(34, 65), "commander": None,
        },
        "mono-green-rich": {
            "why": "44 lands, so castability rises and the regression's delta flips",
            "library": _mono_green(44, 55), "commander": None,
        },
        "mono-green-poor": {
            "why": "20 lands: an unmet requirement at every rung",
            "library": _mono_green(20, 79), "commander": None,
        },
        "pip-ladder": {
            "why": "one colour, three rungs, and only the triple pip failing",
            "library": ladder, "commander": None,
        },
        "hybrid-heavy": {
            "why": ("a hybrid pip is charged to both colours, and a card asking "
                    "{G/W} AND {G} lands in ('G', 1) twice -- the tier's card "
                    "list shows it, and tidying that away is a divergence"),
            "library": hybrids, "commander": None,
        },
        "naya": {
            "why": ("three colours with duals, a five-colour land, wastes, a "
                    "Sol Ring making two, a summoning-sick dork, two land "
                    "fetchers, a Phyrexian cost that must demand nothing, a "
                    "wholly generic cost, and a card past the horizon"),
            "library": naya, "commander": _sim_spell("Atla Palani", "{2}{R}{G}{W}"),
        },
        "esper-rocks": {
            "why": "rocks rather than lands carry the colours; a rock makes three",
            "library": esper, "commander": _sim_spell("Tivit", "{4}{W}{U}{B}"),
        },
        "commanded": {
            "why": ("the commander sets a white demand a mono-green library "
                    "cannot meet, and is not one of the 99"),
            "library": _mono_green(34, 65),
            "commander": _sim_spell("Legend", "{2}{W}{W}"),
        },
        "sixty": {
            "why": "Karsten's own fit size, where his published table is quoted",
            "library": ([_sim_land(f"Plains {i}", "W") for i in range(14)]
                        + [_sim_spell(f"Soldier {i}", "{W}") for i in range(46)]),
            "commander": None,
        },
        "all-lands": {
            "why": "no nonland cards: the regression has no curve to fit",
            "library": [_sim_land(f"Forest {i}", "G") for i in range(99)],
            "commander": None,
        },
        "no-lands": {
            "why": "no lands at all, and the whole 99 asking for colour",
            "library": [_sim_spell(f"Bear {i}", "{1}{G}") for i in range(99)],
            "commander": None,
        },
        "tiny": {
            "why": ("five cards, where a requirement is larger than the deck "
                    "and `required_sources` falls through to the deck size"),
            "library": [_sim_land("Forest", "G"), _sim_land("Plains", "W"),
                        _sim_spell("Bear", "{1}{G}"),
                        _sim_spell("Greedy", "{G}{G}{G}{G}"),
                        _sim_rock("Signet", "{2}", "GW")],
            "commander": None,
        },
        "empty": {
            "why": "the library nothing may be computed over, which must not raise",
            "library": [], "commander": None,
        },
        "tie-breaker": {
            "why": ("built so that every rounding in both modules lands on an "
                    "exact half, which is where Python's banker's rounding and "
                    "Go's away-from-zero rounding differ. 30 cards, 20 lands, "
                    "10 spells of total mana value 51 and exactly one cheap "
                    "accelerant put `regression_lands` at 14.5 (Python 14, a "
                    "naive port 15); the two accelerants average 2.5 mana, 2.5 "
                    "output and 0.5 delay, so `_typical_accelerant` rounds "
                    "three ties at once. Added because a mutation survived "
                    "without it -- twelve real decks and not one of them "
                    "landed on a half"),
            "library": (
                [_sim_land(f"Forest {i}", "G") for i in range(20)]
                + [_sim_rock("Sick Engine", "{2}", "C", amount=2, delay=1),
                   _sim_rock("Grand Battery", "{3}", "C", amount=3, delay=0)]
                + [_sim_spell(f"Titan {i}", "{6}") for i in range(6)]
                + [_sim_spell(f"Colossus {i}", "{5}") for i in range(2)]
            ),
            "commander": None,
        },
    }


def _source_json(src: ManaSource) -> dict[str, Any]:
    """A mana source. `amount` is written even when it is the default.

    Every other field below is omitted when it holds its default, because Go's
    zero value for an absent JSON field is the same default and 1,300 cards
    saying `"fetches_lands": 0` is a third of a megabyte saying nothing.
    `amount` is the one place that reasoning fails: Python's default is **1**
    and Go's zero value is **0**, so an omitted amount would silently make
    every mana source produce nothing.
    """
    return {"colors": sorted(src.colors), "amount": src.amount}


def _cost_json(cost: ManaCost) -> dict[str, Any]:
    out: dict[str, Any] = {}
    if cost.generic:
        out["generic"] = cost.generic
    if cost.pips:
        out["pips"] = [sorted(p) for p in cost.pips]
    if cost.phyrexian:
        out["phyrexian"] = [sorted(p) for p in cost.phyrexian]
    if cost.has_x:
        out["has_x"] = True
    return out


def _card_json(card: SimCard) -> dict[str, Any]:
    out: dict[str, Any] = {"name": card.name}
    cost = _cost_json(card.cost)
    if cost:
        out["cost"] = cost
    if card.category != "utility":
        out["category"] = card.category
    if card.is_land:
        out["is_land"] = True
    if card.enters_tapped:
        out["enters_tapped"] = True
    if card.produces:
        out["produces"] = [_source_json(s) for s in card.produces]
    if card.produce_delay:
        out["produce_delay"] = card.produce_delay
    if card.fetches_lands:
        out["fetches_lands"] = card.fetches_lands
    return out


def _decks_json() -> list[dict[str, Any]]:
    out = []
    for name, spec in closed_form_decks().items():
        commander = spec["commander"]
        out.append({
            "name": name,
            "why": spec["why"],
            "library": [_card_json(c) for c in spec["library"]],
            "commander": None if commander is None else _card_json(commander),
        })
    return out


# --------------------------------------------------- the CPython float floor

#: Sequences `math.fsum` is asked for, chosen so that a naive left-to-right
#: sum fails at least one of them. The last group is captured from the real
#: `castable_odds` term lists rather than invented -- see `_fsum_cases`.
_FSUM_SEEDS: list[list[float]] = [
    [],
    [0.0],
    [-0.0],
    [1.0],
    [0.1] * 10,
    [1e-16, 1.0, 1e16],
    [1e16, 1.0, 1e-16],
    [1e100, 1.0, -1e100, -1.0],
    [-1.0, 1e100, 1.0, -1e100],
    # No overflowing sequence here: CPython raises OverflowError on an
    # intermediate overflow of finite summands, which is an exception rather
    # than a value and so has nothing to compare against.
    [0.1, 0.2, 0.3, -0.6],
    [2.0 ** -1074, 2.0 ** -1074, 1.0],
    [1.0, 2.0 ** -1074, 2.0 ** -1074],
    [0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5],
    [1.0 / 3.0] * 3,
    [1.0 / 7.0] * 7,
]


def _fsum_cases() -> list[dict[str, Any]]:
    """Every sequence, with CPython's answer -- and the real ones, instrumented.

    The invented sequences are the classic traps. The captured ones are the
    point: `castable_odds` is the only caller, its term lists are a hundred
    products of probabilities, and a corpus of invented sequences would prove
    Shewchuk's algorithm without proving it on the shape this project actually
    sums. So `karsten.fsum` is replaced with a recorder that **delegates to
    CPython's own** `math.fsum` -- the same instrument-and-delegate technique
    the pyrand corpus uses on `random.shuffle` -- and a few real shelves are
    read with it in place.
    """
    cases = [{"values": list(v), "value": math.fsum(v)} for v in _FSUM_SEEDS]

    captured: list[list[float]] = []
    real = karsten.fsum

    def recorder(values: Any) -> float:
        seq = list(values)
        captured.append(seq)
        return real(seq)

    karsten.fsum = recorder          # type: ignore[assignment]
    try:
        decks = closed_form_decks()
        for name in ("naya", "esper-rocks", "mono-green"):
            spec = decks[name]
            karsten.shelf(spec["library"], spec["commander"])
    finally:
        karsten.fsum = real          # type: ignore[assignment]

    # Longest first, then the most terms per distinct value: a sum of a
    # hundred tiny products is the case the algorithm exists for, and a
    # handful of one-term lists would hide a broken partials loop.
    captured.sort(key=lambda seq: (-len(seq), seq))
    seen: set[tuple[float, ...]] = set()
    for seq in captured:
        key = tuple(seq)
        if key in seen:
            continue
        seen.add(key)
        cases.append({"values": seq, "value": math.fsum(seq)})
        if len(seen) >= 40:
            break
    return cases


#: Values whose rounding is the whole question: exact halves in both
#: directions, the double that is *just* under a half, and the land counts
#: `regression_lands` actually produces.
_ROUND_VALUES = [
    0.0, -0.0, 0.5, 1.5, 2.5, 3.5, -0.5, -1.5, -2.5,
    0.49999999999999994, 2.675, 33.5, 34.5, 35.5, 36.5,
    36.499999999999996, 36.500000000000004, 54.5, -54.5,
    19.59, 33.333333333333336, 1e15 + 0.5, 0.1, 0.9,
]

#: `(value, ndigits)` pairs. `regression_lands` rounds to 2 and `curve`'s
#: advice to 4, so those two are swept over real figures; the rest are the
#: cases where a decimal round and a binary one disagree.
_ROUND_TO_VALUES = [
    2.675, 1.005, 0.125, 0.135, 2.5, -2.5, 0.0, -0.0,
    0.1, 0.30000000000000004, 1.0 / 3.0, 2.0 / 3.0,
    0.86056, 0.9000000000000001, 0.8999999999999999,
    # These round to a *negative* zero, which CPython's dtoa/strtod
    # round trip keeps and an exact-rational implementation loses.
    -0.4, -0.0004, -0.049,
    3.4499999999999997, 3.45, 33.333333333333336, 19.59,
]


def _round_cases() -> list[dict[str, Any]]:
    """`round(x)` -- and the answer is an **int**, which is the point.

    CPython's one-argument round returns an integer, and an integer has no
    signed zero while Go's `math.Round(-0.5)` does. Recording the int is what
    makes `Round(-0.5)` a question with one answer.
    """
    return [{"x": v, "value": round(v)} for v in _ROUND_VALUES]


def _round_to_cases() -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for value in _ROUND_TO_VALUES:
        for ndigits in (0, 1, 2, 3, 4, 6):
            out.append({"x": value, "ndigits": ndigits,
                        "value": round(value, ndigits)})
    return out


def pyfloat_cases() -> dict[str, Any]:
    return {
        "note": ("CPython's `math.fsum`, `round(x)` and `round(x, n)`, which "
                 "`go/internal/sim` reproduces. Written by "
                 "`python tests/go_fixtures.py`."),
        "why": ("Go's `math.Round` breaks ties away from zero and Python's "
                "`round` breaks them to even, so a land count of 34.5 differs "
                "by one land between the two; and a naive sum of a hundred "
                "products is not `math.fsum`, which `castable_odds` calls "
                "deliberately. Both feed a `>=` comparison whose answer is an "
                "integer, so neither is cosmetic."),
        "fsum_columns": ["values", "value"],
        "fsum": _fsum_cases(),
        "round": _round_cases(),
        "round_to": _round_to_cases(),
    }


def render_pyfloat_cases() -> str:
    return _rows_json(pyfloat_cases()) + "\n"


# ------------------------------------------------------------- karsten

_HYPER_POPULATIONS = [0, 1, 5, 12, 40, 60, 99]
_HYPER_SUCCESSES = [0, 1, 2, 14, 30, 40, 99, 120]
_HYPER_DRAWS = [0, 1, 7, 10, 13, 40, 99]
_HYPER_WANTED = [0, 1, 2, 3, 5, 10]


def _hypergeometric_cases() -> list[list[Any]]:
    """The grid, plus the pair that drives each summation branch.

    `hypergeometric_at_least` sums the complement when it is the shorter sum,
    which is two code paths for one number and therefore two chances to be
    wrong. The grid crosses the branch many times over; the two rows appended
    at the end are the ones `tests/test_karsten.py` names, so a reader can see
    the branch being exercised without deriving it from the grid.
    """
    rows = []
    for population in _HYPER_POPULATIONS:
        for successes in _HYPER_SUCCESSES:
            for draws in _HYPER_DRAWS:
                for wanted in _HYPER_WANTED:
                    rows.append([
                        population, successes, draws, wanted,
                        karsten.hypergeometric_at_least(
                            population, successes, draws, wanted),
                    ])
    for pop, suc, dra, wan in [(12, 5, 4, 1), (12, 5, 4, 4), (60, 14, 7, 1),
                               (99, 36, 10, 2), (99, 1, 10, 2), (10, 10, 20, 5)]:
        rows.append([pop, suc, dra, wan,
                     karsten.hypergeometric_at_least(pop, suc, dra, wan)])
    return rows


def _exactly_cases() -> list[list[Any]]:
    rows = []
    for population in _HYPER_POPULATIONS:
        for successes in _HYPER_SUCCESSES:
            for draws in _HYPER_DRAWS:
                for count in _HYPER_WANTED:
                    rows.append([
                        population, successes, draws, count,
                        karsten._exactly(population, successes, draws, count),
                    ])
    return rows


def _cards_seen_cases() -> list[list[Any]]:
    return [[turn, otp, karsten.cards_seen(turn, on_the_play=otp)]
            for turn in range(-2, 15) for otp in (True, False)]


def _required_sources_cases() -> list[list[Any]]:
    rows = []
    for deck_size in (0, 1, 20, 40, 60, 99, 100):
        for pips in (0, 1, 2, 3, 4):
            for turn in (1, 2, 3, 4, 5, 10):
                for target in (0.5, 0.8, 0.9, 0.95, 0.99):
                    for otp in (True, False):
                        rows.append([
                            deck_size, pips, turn, target, otp,
                            karsten.required_sources(
                                deck_size=deck_size, pips=pips, turn=turn,
                                target=target, on_the_play=otp),
                        ])
    return rows


def _sources_for_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for colors in (["G"], ["W"], ["U"], ["C"], ["G", "W"], ["W", "U", "B"],
                       ["W", "U", "B", "R", "G"], []):
            rows.append({
                "deck": name,
                "colors": colors,
                "value": karsten.sources_for(spec["library"], frozenset(colors)),
            })
    return rows


def _probe_cards(library: list[SimCard], commander: SimCard | None) -> list[SimCard]:
    """A handful of distinct cards per deck, cheapest and dearest included."""
    seen: dict[str, SimCard] = {}
    for card in list(library) + ([commander] if commander is not None else []):
        if card.is_land:
            continue
        seen.setdefault(card.name, card)
    cards = sorted(seen.values(), key=lambda c: (c.mv, c.name))
    if len(cards) <= 8:
        return cards
    picks = [cards[0], cards[len(cards) // 4], cards[len(cards) // 2],
             cards[3 * len(cards) // 4], cards[-1]]
    # Anything with more than one distinct pip set is where the method stops
    # being exact, so make sure at least one is in the sample.
    for card in cards:
        if len(karsten._pip_demand(card.cost)) > 1 and card not in picks:
            picks.append(card)
            break
    out: list[SimCard] = []
    for card in picks:
        if card not in out:
            out.append(card)
    return out


def _castable_odds_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for card in _probe_cards(spec["library"], spec["commander"]):
            for otp in (True, False):
                rows.append({
                    "deck": name,
                    "card": card.name,
                    "on_the_play": otp,
                    "by_turn": [
                        karsten.castable_odds(card, spec["library"], turn=t,
                                              on_the_play=otp)
                        for t in range(1, karsten.HORIZON + 1)
                    ],
                })
    return rows


def _estimate_json(estimate: karsten.LandEstimate) -> dict[str, Any]:
    return {
        "lands_now": estimate.lands_now,
        "recommended": estimate.recommended,
        "delta": estimate.delta,
        "average_mana_value": estimate.average_mana_value,
        "cheap_accelerants": estimate.cheap_accelerants,
        "deck_size": estimate.deck_size,
        "caveats": list(estimate.caveats),
    }


def _regression_cases() -> list[dict[str, Any]]:
    return [{"deck": name,
             "value": _estimate_json(karsten.regression_lands(spec["library"]))}
            for name, spec in closed_form_decks().items()]


def _shelf_json(shelf: karsten.Shelf) -> dict[str, Any]:
    return {
        "deck_size": shelf.deck_size,
        "lands": shelf.lands,
        "target": shelf.target,
        "on_the_play": shelf.on_the_play,
        "colors": [
            {
                "color": req.color,
                "have": req.have,
                "have_lands": req.have_lands,
                "met": req.met,
                "shortfall": req.shortfall,
                "tiers": [
                    {
                        "pips": tier.pips,
                        "turn": tier.turn,
                        "need": tier.need,
                        "have": tier.have,
                        "met": tier.met,
                        "shortfall": tier.shortfall,
                        "odds_now": tier.odds_now,
                        "cards": list(tier.cards),
                    }
                    for tier in req.tiers
                ],
            }
            for req in shelf.colors
        ],
        "land_estimate": _estimate_json(shelf.land_estimate),
        "odds": [
            {
                "name": odds.name,
                "mv": odds.mv,
                "by_turn": list(odds.by_turn),
                "on_curve": odds.on_curve,
                "reliable_turn": odds.reliable_turn,
                "lag": odds.lag,
                "lateness": odds.lateness,
            }
            for odds in shelf.odds
        ],
        "approximated": list(shelf.approximated),
        "unmet": [req.color for req in shelf.unmet],
    }


def _shelf_cases() -> list[dict[str, Any]]:
    """Every deck at the default dials, and three of them turned.

    The whole structure rather than a summary, because the shelf's *order* is
    an output: `odds` is sorted by lateness so that a three-drop landing on
    turn six outranks an eight-drop that never arrives, and a port that got
    every probability right and the comparator wrong would look green against
    anything less.
    """
    decks = closed_form_decks()
    rows = []
    for name, spec in decks.items():
        rows.append({
            "deck": name, "target": karsten.TARGET, "on_the_play": True,
            "value": _shelf_json(karsten.shelf(spec["library"], spec["commander"])),
        })
    for name, target, otp in [("naya", 0.80, False), ("esper-rocks", 0.99, False),
                              ("pip-ladder", 0.60, True)]:
        spec = decks[name]
        rows.append({
            "deck": name, "target": target, "on_the_play": otp,
            "value": _shelf_json(karsten.shelf(
                spec["library"], spec["commander"], target=target,
                on_the_play=otp)),
        })
    return rows


def karsten_cases() -> dict[str, Any]:
    return {
        "note": ("`sim/karsten.py` -- Tier 1.5, the closed form -- rendered by "
                 "`python tests/go_fixtures.py`. Floats are plain JSON numbers "
                 "and round-trip exactly; `go/internal/sim/karsten` must "
                 "reproduce every one of them bit for bit."),
        "target": karsten.TARGET,
        "horizon": karsten.HORIZON,
        "hypergeometric_columns": ["population", "successes", "draws", "wanted", "value"],
        "hypergeometric": _hypergeometric_cases(),
        "exactly_columns": ["population", "successes", "draws", "count", "value"],
        "exactly": _exactly_cases(),
        "cards_seen_columns": ["turn", "on_the_play", "value"],
        "cards_seen": _cards_seen_cases(),
        "required_sources_columns": ["deck_size", "pips", "turn", "target",
                                     "on_the_play", "value"],
        "required_sources": _required_sources_cases(),
        "decks": _decks_json(),
        "sources_for": _sources_for_cases(),
        "castable_odds": _castable_odds_cases(),
        "regression_lands": _regression_cases(),
        "shelves": _shelf_cases(),
    }


def render_karsten_cases() -> str:
    return _rows_json(karsten_cases()) + "\n"


# --------------------------------------------------------------- the curve

def _expected_lands_cases() -> list[list[Any]]:
    rows = []
    for deck_size in (0, 1, 40, 60, 99):
        for lands in (0, 1, 17, 34, 40, 60, 99):
            for turn in (0, 1, 2, 4, 6, 10):
                for otp in (True, False):
                    rows.append([
                        deck_size, lands, turn, otp,
                        curve.expected_lands_in_play(deck_size, lands, turn,
                                                     on_the_play=otp),
                    ])
    return rows


def _land_distribution_cases() -> list[dict[str, Any]]:
    rows = []
    for deck_size in (0, 1, 40, 99):
        for lands in (0, 1, 17, 36, 60, 99):
            for turn in (0, 1, 3, 4, 7):
                for otp in (True, False):
                    rows.append({
                        "deck_size": deck_size, "lands": lands, "turn": turn,
                        "on_the_play": otp,
                        "value": curve._land_distribution(
                            deck_size, lands, turn, on_the_play=otp),
                    })
    return rows


def _expected_ramp_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for otp in (True, False):
            rows.append({
                "deck": name, "on_the_play": otp,
                "by_turn": [curve.expected_ramp(spec["library"], t,
                                                on_the_play=otp)
                            for t in range(1, curve.HORIZON + 1)],
            })
    return rows


def _ramp_distribution_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for turn in (1, 2, 4, 6, 10):
            for extra, count in ((None, 0), ((2, 1, 0), 1), ((1, 2, 1), 3),
                                 ((7, 5, 0), 2)):
                rows.append({
                    "deck": name, "turn": turn, "on_the_play": True,
                    "extra": None if extra is None else list(extra),
                    "extra_count": count,
                    "value": curve._ramp_distribution(
                        spec["library"], turn, extra=extra, extra_count=count),
                })
    return rows


def _on_curve_odds_cases() -> list[list[Any]]:
    """The two dials, and the hypotheticals the advice is built from.

    `need` is carried as a JSON null where Python passes `None`, because zero
    is a *different* question there -- `need=0` asks for no mana and answers
    1.0 -- and a port that collapsed the two would pass a corpus that never
    asked.
    """
    rows = []
    for name, spec in closed_form_decks().items():
        for turn in (1, 2, 4, 6):
            for need in (None, 0, 1, turn, turn + 1, turn + 2, 12):
                for extra in ({}, {"extra_lands": 1}, {"extra_lands": 5},
                              {"extra_ramp": (2, 1, 0), "extra_ramp_count": 1},
                              {"extra_ramp": (1, 2, 1), "extra_ramp_count": 4}):
                    for otp in (True, False):
                        piece = extra.get("extra_ramp")
                        rows.append([
                            name, turn, need, otp,
                            extra.get("extra_lands", 0),
                            None if piece is None else list(piece),
                            extra.get("extra_ramp_count", 0),
                            curve.on_curve_odds(
                                spec["library"], turn, need=need,
                                on_the_play=otp,
                                extra_lands=extra.get("extra_lands", 0),
                                extra_ramp=piece,
                                extra_ramp_count=extra.get("extra_ramp_count", 0)),
                        ])
    return rows


def _lands_for_every_drop_cases() -> list[list[Any]]:
    """Includes the pinned pair: 48 lands through turn 3, 54 through turn 4.

    That is the number the feature exists to talk somebody *out* of, so a
    regression that made it look reasonable would be the feature quietly
    reversing its own advice.
    """
    rows = []
    for deck_size in (5, 40, 60, 99, 120):
        for turn in range(1, 11):
            for target in (0.5, 0.8, 0.9, 0.99):
                for otp in (True, False):
                    rows.append([
                        deck_size, turn, target, otp,
                        curve.lands_for_every_drop(deck_size, turn,
                                                   target=target,
                                                   on_the_play=otp),
                    ])
    return rows


def _typical_accelerant_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for turn in (1, 2, 4, 6, 10):
            piece, generic = curve._typical_accelerant(spec["library"], turn)
            rows.append({"deck": name, "turn": turn,
                         "piece": list(piece), "generic": generic})
    return rows


def _curve_json(mc: curve.ManaCurve) -> dict[str, Any]:
    return {
        "deck_size": mc.deck_size,
        "lands": mc.lands,
        "accelerants": mc.accelerants,
        "target_turn": mc.target_turn,
        "target_mana": mc.target_mana,
        "target": mc.target,
        "on_the_play": mc.on_the_play,
        "turns": [
            {
                "turn": t.turn,
                "from_lands": t.from_lands,
                "from_ramp": t.from_ramp,
                "expected_mana": t.expected_mana,
                "land_drop_odds": t.land_drop_odds,
                "odds": t.odds,
            }
            for t in mc.turns
        ],
        "advice": {
            "target_turn": mc.advice.target_turn,
            "target_mana": mc.advice.target_mana,
            "target": mc.advice.target,
            "odds": mc.advice.odds,
            "odds_per_land": mc.advice.odds_per_land,
            "odds_per_ramp": mc.advice.odds_per_ramp,
            "recommend": mc.advice.recommend,
            "slots": mc.advice.slots,
            "ramp_is_generic": mc.advice.ramp_is_generic,
            "beyond_the_curve": mc.advice.beyond_the_curve,
            "lands_for_every_drop": mc.advice.lands_for_every_drop,
        },
    }


#: `(target_turn, target_mana, target, on_the_play)`. The clamps are here on
#: purpose: turn 99 must come back as the horizon and turn 0 as turn 1, and a
#: target of 0.999 must come back as 0.99.
_CURVE_DIALS = [
    (4, None, 0.90, True),
    (4, 6, 0.90, True),
    (2, 2, 0.60, True),
    (4, 4, 0.85, False),
    (2, 12, 0.90, True),
    (99, None, 0.999, True),
    (0, 0, 0.1, True),
    (6, 3, 0.95, False),
]


def _curve_cases() -> list[dict[str, Any]]:
    rows = []
    for name, spec in closed_form_decks().items():
        for target_turn, target_mana, target, otp in _CURVE_DIALS:
            rows.append({
                "deck": name,
                "target_turn": target_turn,
                "target_mana": target_mana,
                "target": target,
                "on_the_play": otp,
                "value": _curve_json(curve.curve(
                    spec["library"], target_turn=target_turn,
                    target_mana=target_mana, target=target, on_the_play=otp)),
            })
    return rows


def curve_cases() -> dict[str, Any]:
    return {
        "note": ("`sim/curve.py` -- P(N mana on turn T), decomposed, with the "
                 "lands-or-ramp advice -- rendered by "
                 "`python tests/go_fixtures.py`."),
        "horizon": curve.HORIZON,
        "default_target_turn": curve.DEFAULT_TARGET_TURN,
        "default_target": curve.DEFAULT_TARGET,
        "too_close": curve.TOO_CLOSE,
        "generic_rock": list(curve._GENERIC_ROCK),
        "decks": _decks_json(),
        "accelerants": [
            {"deck": name, "value": [list(p) for p in curve._accelerants(spec["library"])]}
            for name, spec in closed_form_decks().items()
        ],
        "expected_lands_columns": ["deck_size", "lands", "turn", "on_the_play", "value"],
        "expected_lands": _expected_lands_cases(),
        "land_distribution": _land_distribution_cases(),
        "expected_ramp": _expected_ramp_cases(),
        "ramp_distribution": _ramp_distribution_cases(),
        "on_curve_odds_columns": ["deck", "turn", "need", "on_the_play",
                                  "extra_lands", "extra_ramp", "extra_ramp_count",
                                  "value"],
        "on_curve_odds": _on_curve_odds_cases(),
        "lands_for_every_drop_columns": ["deck_size", "turn", "target",
                                         "on_the_play", "value"],
        "lands_for_every_drop": _lands_for_every_drop_cases(),
        "typical_accelerant": _typical_accelerant_cases(),
        "curves": _curve_cases(),
    }


def render_curve_cases() -> str:
    return _rows_json(curve_cases()) + "\n"


def write_closed_forms() -> None:
    """The three files the closed forms are held to. Called by `write`."""
    for path, body, what in (
        (PYFLOAT_PATH, render_pyfloat_cases(), "CPython's float floor"),
        (KARSTEN_PATH, render_karsten_cases(), "Tier 1.5's closed form"),
        (CURVE_PATH, render_curve_cases(), "the mana curve"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body, encoding="utf-8")
        print(f"wrote {what} into {path}")

# ---------------------------------------------------------- the job registry
#
# `api/jobs.py` ported to `go/internal/jobs` (Phase 5, the registry half).
# Most of that module is *behaviour* -- a lock held across two steps, a lane
# that runs one thing at a time -- which a corpus cannot hold and which the Go
# tests and the race detector do instead. What a corpus can hold is the part
# that is arithmetic and formatting, and that part is where the two runtimes
# disagree by default:
#
#   - **`percent` rounds half to even**, because Python's `round` does and
#     Go's `math.Round` rounds half away from zero. One job in eight lands on
#     a tie -- 1 of 8 is 12.5 -- and a progress bar a point out is exactly the
#     kind of wrongness nobody ever reports.
#   - **`created_at` is `datetime.isoformat()`**, whose fractional part
#     *vanishes* when the microsecond is zero and whose offset is spelled
#     `+00:00`, never `Z`. It is also the key `all_jobs` sorts on, as text, so
#     a differently-shaped stamp is a differently-ordered list.
#   - **`as_dict` has a field order**, and Starlette writes a dict in
#     insertion order, so the bytes are pinned rather than only the keys --
#     which is also how `owner` and `key` are held out of the payload.
#
# The lane refusal is recorded for the reason the crypto oracle records a PHC
# string: it is a sentence that reaches a caller today, and reproducing a
# sentence is not a thing to do from memory.

JOBS_PATH = ROOT / "go" / "internal" / "jobs" / "testdata" / "jobs.json"

#: `(done, total)` pairs. The first block is ties -- every eighth, and the
#: sixteenths and the halves, where half-to-even and half-away-from-zero part
#: company -- and the rest is ordinary traffic plus the two degenerate totals a
#: real job passes through: nothing reported yet, and a worker that reported a
#: count without a bound.
_PERCENT_CASES: tuple[tuple[int, int], ...] = (
    (1, 8), (3, 8), (5, 8), (7, 8), (1, 2), (3, 2), (1, 200), (3, 200),
    (5, 200), (7, 200), (1, 40), (1, 24), (5, 24), (7, 24), (11, 24),
    (0, 0), (5, 0), (0, 1), (1, 1), (1, 3), (2, 3), (1, 7), (6, 7),
    (1, 6), (5, 6), (33, 100), (99, 100), (100, 100), (1, 20000),
    (19999, 20000), (20000, 20000), (1, 33), (17, 33), (2, 16), (6, 16),
    (10, 16), (14, 16), (1, 80), (3, 80), (1, 800), (399, 800), (401, 800),
)

#: Instants as `(year, month, day, hour, minute, second, microsecond)`. The
#: interesting field is the last: zero (the whole fraction disappears), one
#: (six digits, five of them leading zeros), and a round number (whose
#: trailing zeros stay, and which a "strip the zeros" formatter would eat).
_STAMP_CASES: tuple[tuple[int, int, int, int, int, int, int], ...] = (
    (2026, 8, 22, 13, 10, 20, 936490),
    (2026, 8, 22, 13, 10, 20, 0),
    (2026, 8, 22, 13, 10, 20, 1),
    (2026, 8, 22, 13, 10, 20, 999999),
    (2026, 8, 22, 13, 10, 20, 100000),
    (2026, 1, 2, 3, 4, 5, 60),
    (2026, 12, 31, 23, 59, 59, 999999),
    (2027, 1, 1, 0, 0, 0, 0),
)


def _jobs_module() -> Any:
    """`mtglab.api.jobs`, imported here rather than at the top of this file.

    Every other oracle here imports its subject at module scope. This one does
    not, because importing `api.jobs` builds three real thread pools at import
    time -- three idle threads in a fixture generator that is otherwise pure,
    and three more in every test that imports this module for something else.
    """
    from mtglab.api import jobs as module
    return module


def job_payload_cases() -> list[dict[str, Any]]:
    """`Job.as_dict()` for the states a job actually passes through.

    Every case fixes `id` and `created_at`, because what this records is the
    shape *and* the bytes, and a random id makes the second unrecordable.
    `owner` and `key` are set on some of them deliberately: neither may appear
    in the output, and a port that serialised whatever struct it happened to
    have would put them there.
    """
    jobs = _jobs_module()
    stamp = "2026-08-22T13:10:20.936490+00:00"
    theater = [{"game": 1, "winner": "arahbo", "turns": 9},
               {"game": 2, "winner": None, "turns": 14}]
    cases: list[tuple[str, Any]] = [
        ("queued, nothing reported yet",
         jobs.Job(id="0f1e2d3c4b5a", kind="sim.mana", label="Arahbo — mana",
                  created_at=stamp)),
        ("queued for somebody, with a dedupe key -- neither may serialise",
         jobs.Job(id="a1b2c3d4e5f6", kind="claude.dossier",
                  label="Goreclaw, Terror of Qal Sisma", owner=7,
                  key="oracle:1234", created_at=stamp)),
        ("running, and on a tie: 1 of 8 is 12.5",
         jobs.Job(id="112233445566", kind="sim.lands", status="running",
                  done=1, total=8, label="Gyome — lands", created_at=stamp)),
        ("running, with the theater's rows before the result exists",
         jobs.Job(id="66554433221a", kind="sim.forge", status="running",
                  done=2, total=8, partial=theater, label="Cat vs Dino",
                  created_at=stamp)),
        ("done, with a result and the bar filled",
         jobs.Job(id="deadbeefcafe", kind="sim.mana", status="done",
                  done=20000, total=20000,
                  result={"games": 20000, "mulligan_rate": 0.1732,
                          "cached": False, "computed_at": None,
                          "caveat": "no opponents, no interaction"},
                  label="Trostani — mana", created_at=stamp)),
        ("born finished, which is what a cache hit is",
         jobs.Job(id="000000000001", kind="sim.mana", status="done",
                  done=1, total=1, result={"cached": True},
                  label="Tivit — mana", created_at=stamp)),
        ("errored, and the string is a class name and a message",
         jobs.Job(id="ffffffffffff", kind="sim.mana", status="error",
                  done=0, total=0,
                  error="DeckNotFound: no deck 'no-such-deck'",
                  label="no-such-deck — mana", created_at=stamp)),
        ("a label carrying the characters an HTML-escaping encoder would eat",
         jobs.Job(id="3c3e26273c3e", kind="claude.argue",
                  label="<Ajani> & \"Nacatl\" — 'why' — 100% <cut>",
                  created_at=stamp)),
        ("a label and a result in scripts the wire must not \\u-escape",
         jobs.Job(id="000000c0ffee", kind="claude.theme.proposal",
                  status="done", done=1, total=1,
                  result={"note": "Ünsere Vorschläge — 火の玉 🜁",
                          "score": 1.5},
                  label="Ünsere Vorschläge", created_at=stamp)),
        ("a count with no bound, which is nought per cent and not a crash",
         jobs.Job(id="0b0b0b0b0b0b", kind="sim.policy", status="running",
                  done=5, total=0, label="Atla — policy", created_at=stamp)),
        ("a stamp with no fraction at all",
         jobs.Job(id="0a0a0a0a0a0a", kind="sim.mana",
                  label="Goreclaw — mana",
                  created_at="2026-08-22T13:10:20+00:00")),
    ]
    def compact(value: Any) -> str | None:
        """The bytes Starlette would write for one nested value.

        Recorded apart from the payload because **Go cannot reproduce a
        Python dict's key order through a `map[string]any`** -- encoding/json
        sorts map keys, and `result` and `partial` are `Any`. So the corpus
        hands the Go side the nested bytes as they stand, and the constraint
        that finding puts on every family still to flip is stated in the
        package comment: a ported result is a struct with its fields in
        Python's order, or it is pre-encoded; it is never a map.
        """
        if value is None:
            return None
        return json.dumps(value, ensure_ascii=False, allow_nan=False,
                          separators=(",", ":"))

    return [
        {"name": name,
         "job": {"id": job.id, "kind": job.kind, "status": job.status,
                 "done": job.done, "total": job.total,
                 "result_json": compact(job.result),
                 "partial_json": compact(job.partial), "error": job.error,
                 "label": job.label, "owner": job.owner, "key": job.key,
                 "created_at": job.created_at},
         "want": job.as_dict(),
         "want_json": json.dumps(job.as_dict(), ensure_ascii=False,
                                 allow_nan=False, separators=(",", ":"))}
        for name, job in cases
    ]


def jobs_cases() -> dict[str, Any]:
    """The whole registry oracle, as the Go module reads it."""
    import uuid
    from datetime import UTC, datetime

    jobs = _jobs_module()
    messages: list[dict[str, str]] = []
    # The last two are not lanes anybody could pass; they are there because
    # the message quotes the lane with `repr`, and `repr` prefers single
    # quotes and switches to double ones only when the string holds a single
    # quote and no double. A port reaching for its own quoting would get the
    # ordinary cases right and those two wrong.
    for lane in ("nope", "", "CPU", "cpu ", "sim", "it's", 'say "cpu"'):
        try:
            jobs.submit("test", lambda _p: None, lane=lane)
        except ValueError as exc:
            messages.append({"lane": lane, "error": str(exc)})
        else:                                              # pragma: no cover
            raise AssertionError(f"lane {lane!r} was accepted")
    # A refused lane records nothing, which is the whole point of checking it
    # before the insert. Asserted here so the corpus cannot be generated from
    # a module that had started queueing them.
    assert jobs.all_jobs() == [], "a refused lane must record no job"
    return {
        "note": ("Generated from `mtglab.api.jobs` by "
                 "`python tests/go_fixtures.py`. `want_json` is the body "
                 "Starlette writes: compact separators, no ASCII escaping."),
        "lanes": {"cpu": jobs.CPU, "net": jobs.NET, "forge": jobs.FORGE},
        "live": list(jobs.LIVE),
        "max_jobs": jobs.MAX_JOBS,
        "id_length": len(uuid.uuid4().hex[:12]),
        "unknown_lane": messages,
        "percent": [{"done": d, "total": t,
                     "want": jobs.Job(id="x", kind="k", done=d,
                                      total=t).as_dict()["percent"]}
                    for d, t in _PERCENT_CASES],
        "stamps": [{"at": list(parts),
                    "want": datetime(*parts, tzinfo=UTC).isoformat()}
                   for parts in _STAMP_CASES],
        "payloads": job_payload_cases(),
    }


def render_jobs_cases() -> str:
    return _compact_json(jobs_cases()) + "\n"


# ------------------------------------------------------------------- tier 1
#
# `sim/tier1/engine.py` in Go (PLAN section 7, Phase 5), and the corpus that
# says the port is the same simulator rather than a similar one.
#
# The gate is `tests/test_determinism.py`'s REFERENCE_DIGEST -- a sha256 over
# `repr()` of one game, one 300-game run and a three-point land sweep. A
# digest is opaque by design, so this corpus records the *text* it is taken
# over as well: a Go run that diverges then reports which line and which
# character rather than "the hash moved".
#
# Four things this records that the digest alone cannot.
#
# **The decks, as data.** `build_golgari` is a test helper, and writing a
# second one in Go would put a deck builder between the two engines -- a
# builder disagreement would then present as an engine disagreement. So the
# libraries travel as compiled SimCards and neither side builds anything.
#
# **Cards that compare equal.** Every card in `build_golgari` has a distinct
# name, so nothing in the reference run exercises `list.remove` taking out a
# *different* card from the one it was handed -- which is what happens in
# every real deck, because `compile_deck` writes `[compiled] * qty`. The
# duplicates deck below is that hazard on purpose.
#
# **Castability, both ways round.** Tier 1 asks `mana.can_pay` inside
# `_pick_land` and `engine._consume` everywhere else, and the Go port answers
# both through its port of `_consume`. That is an equivalence argument, so
# each case here records what *both* Python functions said.
#
# **`repr(float)` at its boundaries.** The digest hashes text, so Python's
# number formatting is inside the gate: `100.0` is not `100` and `1e-05` is
# not `0.00001`. The float section is CPython's own answer for every value
# the reference run produces, plus the places the notation changes shape.

TIER1_PATH = ROOT / "go" / "internal" / "sim" / "tier1" / "testdata" / "tier1.json"


#: The card, cost and source encoders are the closed forms' own
#: (`_card_json` and friends, above): one encoder for one Go type, since
#: `sim.Card` is shared by every tier. They omit defaults, which is why a
#: `category` of "utility" and a `has_x` of False do not appear below -- Go's
#: zero value is the same default, and `amount` is the one field written
#: always because Python's default is 1 where Go's zero value is 0.
#:
#: A cost travels **parsed** rather than as the string it came from. Re-parsing
#: in Go would put `mana.Parse` inside the comparison, and a parser
#: disagreement would then present as an engine disagreement.


def _tier1_keep_json(rule: Any) -> dict[str, int]:
    return {
        "min_lands": rule.min_lands,
        "max_lands": rule.max_lands,
        "min_mana_pieces": rule.min_mana_pieces,
        "cheap_ramp_mv": rule.cheap_ramp_mv,
        "max_mulligans": rule.max_mulligans,
    }


def tier1_duplicate_deck() -> tuple[list[Any], Any]:
    """A deck of cards that compare equal, which `build_golgari` has none of.

    `compile_deck` writes `[compiled] * qty`, so a real deck's basics are one
    object repeated -- and `hand.remove(card)` takes out the first *equal*
    element, not the one it was handed. With every name distinct those are
    always the same card and the distinction is invisible; here they are not.
    It also carries the shapes the Golgari shell lacks: a two-mana source
    (unit expansion), a hybrid pip, a genuinely colourless pip, and a spell
    too expensive to cast inside most horizons, which is a never-cast timing
    row and therefore the head of the sort.
    """
    import test_sim_tier1 as t1
    from mtglab.mana import ManaSource

    forest = t1.land("Forest", t1.G)
    swamp = t1.land("Swamp", t1.B)
    tainted = t1.land("Tainted Wood", t1.BG, tapped=True)
    sol_ring = t1.spell("Sol Ring", "{1}", "ramp",
                        produces=[ManaSource(t1.C, 2)])
    signet = t1.spell("Golgari Signet", "{2}", "ramp",
                      produces=[ManaSource(t1.BG)])
    dork = t1.spell("Llanowar Elves", "{G}", "ramp",
                    produces=[ManaSource(t1.G)], delay=1)
    cultivate = t1.spell("Cultivate", "{2}{G}", "ramp", fetches=1)
    hybrid = t1.spell("Fire Covenant", "{2}{B/G}", "interaction")
    eldrazi = t1.spell("Kozilek's Chatter", "{4}{C}", "threat")
    unreachable = t1.spell("Impervious Greatwurm", "{6}{B}{G}", "threat")

    library = ([forest] * 18 + [swamp] * 9 + [tainted] * 6 + [sol_ring] * 4
               + [signet] * 8 + [dork] * 8 + [cultivate] * 7 + [hybrid] * 9
               + [eldrazi] * 10 + [unreachable] * 20)
    commander = t1.spell("Gyome, Master Chef", "{3}{B}{G}", "engine")
    return library[:99], commander


def tier1_decks() -> dict[str, tuple[list[Any], Any]]:
    """Every deck the cases below name, keyed by the name they name it by."""
    from test_sim_tier1 import build_golgari

    decks: dict[str, tuple[list[Any], Any]] = {
        f"golgari-{n}": build_golgari(n) for n in (30, 32, 34, 35, 38)
    }
    duplicates, commander = tier1_duplicate_deck()
    decks["duplicates"] = (duplicates, commander)
    decks["headless"] = (duplicates, None)
    return decks


#: (deck, seed, turns, keep rule overrides, on_the_play). The keep rules are
#: chosen to exercise both branches of the bottoming loop: a rule nothing can
#: satisfy mulligans to the cap and bottoms lands, and a hand held at
#: `min_lands` bottoms its most expensive spell instead.
TIER1_GAMES: tuple[tuple[str, int, int, dict[str, int], bool], ...] = (
    ("golgari-34", 17, 10, {}, False),
    ("golgari-34", 7, 12, {}, False),
    ("golgari-34", 20260810, 10, {}, True),
    ("golgari-30", 3, 8, {}, False),
    ("golgari-38", 99, 14, {}, False),
    ("duplicates", 1, 10, {}, False),
    ("duplicates", 2, 10, {}, True),
    ("duplicates", 3, 6, {"min_lands": 4, "max_lands": 4,
                          "min_mana_pieces": 6}, False),
    ("duplicates", 4, 10, {"min_lands": 0, "max_lands": 7,
                           "min_mana_pieces": 0, "max_mulligans": 0}, False),
    ("duplicates", 5, 10, {"min_lands": 6, "max_lands": 7,
                           "min_mana_pieces": 7, "max_mulligans": 3}, False),
    ("duplicates", 6, 10, {"cheap_ramp_mv": 4, "min_mana_pieces": 5}, False),
    # These two are here because a mutation survived without them. The first
    # bottoms LANDS from a hand of equal Forests -- `hand.remove` is handed
    # `lands_in_hand[-1]` and takes out the FIRST card equal to it, which is
    # the one place in the engine where "the same card" and "an equal card"
    # part company. The second mulligans past a seven-card hand, which is the
    # only way `min(mulligans, len(hand) - 1)` ever binds.
    ("duplicates", 7, 10, {"min_lands": 0, "max_lands": 7,
                           "min_mana_pieces": 8, "max_mulligans": 3}, False),
    ("duplicates", 8, 10, {"min_lands": 0, "max_lands": 0,
                           "min_mana_pieces": 0, "max_mulligans": 9}, False),
    ("golgari-34", 21, 10, {"min_lands": 0, "max_lands": 0,
                            "min_mana_pieces": 0, "max_mulligans": 9}, False),
    ("headless", 11, 10, {}, False),
    ("headless", 12, 4, {}, True),
    ("golgari-32", 0, 1, {}, False),
    ("golgari-32", 0, 0, {}, False),
)

#: (deck, games, turns, seed, keep rule overrides). Small on purpose -- the
#: reference run is the large one, and these vary the *shape* (no commander,
#: a horizon shorter than the commander's cost, a policy that mulligans
#: everything, a single game) rather than sampling harder.
TIER1_RUNS: tuple[tuple[str, int, int, int, dict[str, int]], ...] = (
    ("golgari-34", 40, 10, 42, {}),
    ("duplicates", 60, 10, 5, {}),
    ("duplicates", 25, 4, 8, {"min_lands": 3, "max_lands": 3,
                              "min_mana_pieces": 5}),
    ("headless", 30, 9, 13, {}),
    ("golgari-30", 1, 12, 1, {}),
    ("golgari-38", 12, 3, 2, {"max_mulligans": 0}),
    ("duplicates", 20, 8, 9, {"min_lands": 0, "max_lands": 0,
                              "min_mana_pieces": 0, "max_mulligans": 9}),
)


def _tier1_pool(spec: list[tuple[str, int]]) -> list[Any]:
    from mtglab.mana import ManaSource
    return [ManaSource(frozenset(colors), amount) for colors, amount in spec]


#: Pools for the castability cases, as (colours, amount) pairs.
TIER1_POOLS: tuple[tuple[str, list[tuple[str, int]]], ...] = (
    ("empty", []),
    ("one-green", [("G", 1)]),
    ("one-dual", [("BG", 1)]),
    ("two-duals", [("BG", 1), ("BG", 1)]),
    ("sol-ring", [("C", 2)]),
    ("sol-ring-and-forest", [("C", 2), ("G", 1)]),
    ("split", [("G", 1), ("B", 1)]),
    ("split-and-dual", [("G", 1), ("B", 1), ("BG", 1)]),
    ("any-colour", [("WUBRG", 1)]),
    ("any-colour-twice", [("WUBRG", 2)]),
    ("five-basics", [("W", 1), ("U", 1), ("B", 1), ("R", 1), ("G", 1)]),
    ("colourless-heap", [("C", 3)]),
    ("dual-and-colourless", [("BG", 1), ("C", 1)]),
    ("wide-and-narrow", [("BG", 1), ("G", 1), ("WUBRG", 1), ("C", 2)]),
    ("zero-amount", [("G", 0), ("B", 1)]),
)

#: Costs for the castability cases.
TIER1_COSTS: tuple[str, ...] = (
    "", "{0}", "{1}", "{2}", "{5}", "{G}", "{B}", "{C}",
    "{G}{G}", "{B}{G}", "{1}{G}", "{2}{G}", "{3}{B}{G}", "{2}{C}",
    "{B/G}", "{B/G}{B/G}", "{2}{B/G}", "{2/G}", "{G/P}", "{G/P}{G}",
    "{X}{G}", "{X}", "{W}{U}{B}{R}{G}", "{4}{C}", "{6}{B}{G}", "{C}{C}",
)


def tier1_consume_cases() -> list[dict[str, Any]]:
    """`engine._consume` and `mana.can_pay` over the same (cost, pool) pairs.

    Both, because the Go port answers `can_pay` through its port of
    `_consume`, and the equivalence of the two is an argument: the same
    matching over the same units, with `_consume`'s extra generic check
    implied by its own opening one. An argument about equivalence is a thing
    to check. If the two ever disagree on a case here, the Go side is
    answering the wrong question and this is where it says so.
    """
    from mtglab.mana import can_pay, parse_mana_cost
    from mtglab.sim.tier1.engine import _consume

    out: list[dict[str, Any]] = []
    for cost_text in TIER1_COSTS:
        cost = parse_mana_cost(cost_text)
        for pool_name, spec in TIER1_POOLS:
            sources = _tier1_pool(spec)
            remaining = _consume(cost, list(sources))
            out.append({
                "cost_text": cost_text,
                "cost": _cost_json(cost),
                "pool": pool_name,
                "sources": [_source_json(s) for s in sources],
                "can_pay": can_pay(cost, sources),
                "remaining": (None if remaining is None
                              else [_source_json(s) for s in remaining]),
            })
    return out


def _tier1_floats(obj: Any, seen: list[float]) -> None:
    """Every float reachable from a result object, in traversal order."""
    from dataclasses import fields, is_dataclass

    if isinstance(obj, bool) or obj is None:
        return
    if isinstance(obj, float):
        seen.append(obj)
    elif isinstance(obj, dict):
        for key, value in obj.items():
            _tier1_floats(key, seen)
            _tier1_floats(value, seen)
    elif isinstance(obj, (list, tuple)):
        for item in obj:
            _tier1_floats(item, seen)
    elif is_dataclass(obj) and not isinstance(obj, type):
        for f in fields(obj):
            _tier1_floats(getattr(obj, f.name), seen)


#: Floats chosen for where CPython's repr *changes shape* rather than for
#: their magnitude: both fixed/exponential boundaries, values needing all
#: seventeen significant digits, and the ones a naive renderer prints with no
#: decimal point at all.
TIER1_EDGE_FLOATS: tuple[float, ...] = (
    0.0, -0.0, 1.0, -1.0, 0.5, 2.0, 100.0, -100.0,
    1 / 3, 2 / 3, 1 / 7, 0.1, 0.2, 0.1 + 0.2, 1e-1,
    1e15, 1e16, 1e17, 9999999999999998.0, 1.2345678901234567e16,
    1e-3, 1e-4, 1e-5, 1.5e-5, 9.999999999999999e-5,
    1e100, 1e-100, 1e308, 1e-308, 5e-324, 2.2250738585072014e-308,
    1.7976931348623157e308, 2.0 ** 53, 2.0 ** 53 + 2, float(2 ** 63),
    123456789.0, 1234567890123456.0, 12345678901234567.0,
    float("inf"), float("-inf"), float("nan"),
)


def tier1_float_cases(extra: list[float]) -> list[dict[str, Any]]:
    """`repr(float)` from CPython, as (bits, text).

    Bits rather than a JSON number, for the same reason the draw corpus
    compares `Float64bits`: the thing under test is a *rendering*, so the
    input has to arrive as the exact double and not as a decimal that must be
    read back through the very code being checked.
    """
    values: list[float] = list(TIER1_EDGE_FLOATS)
    values += [n / 300 for n in range(302)]
    values += [n / 100 for n in range(101)]
    values += [n / 7 for n in range(40)]
    values += extra
    seen: set[int] = set()
    out: list[dict[str, Any]] = []
    for value in values:
        bits = _float_bits(value)
        if bits in seen:
            continue
        seen.add(bits)
        out.append({"bits": bits, "repr": repr(value)})
    return out


#: Strings whose `repr` a renderer can get wrong: the quote choice, the
#: escapes, and the printable non-ASCII CPython passes through untouched.
TIER1_STRINGS: tuple[str, ...] = (
    "", "Forest", "Gyome, Master Chef", "Kozilek's Chatter",
    'a "quoted" name', 'both \' and "', "back\\slash",
    "keep 2-5 lands AND lands + ramp(mv<=2) >= 3",
    "line\nbreak", "tab\there", "carriage\rreturn",
    "Jotun Grunt — em dash", "Júbilee", "æther Vial",
    "null\x00byte", "bell\x07", "delete\x7f", "no break",
    "\U0001f5e1 blade",
)


def tier1_string_cases() -> list[dict[str, str]]:
    return [{"value": s, "repr": repr(s)} for s in TIER1_STRINGS]


def tier1_cases() -> dict[str, Any]:
    """The whole Tier 1 corpus, as the Go module reads it."""
    import determinism_probe as probe
    from mtglab.sim.tier1.engine import KeepRule, run, simulate_game

    decks = tier1_decks()
    floats: list[float] = []

    games: list[dict[str, Any]] = []
    for name, seed, turns, overrides, on_play in TIER1_GAMES:
        library, commander = decks[name]
        rule = KeepRule(**overrides)
        result = simulate_game(library, commander, turns=turns,
                               keep_rule=rule, rng=random.Random(seed),
                               on_the_play=on_play)
        games.append({
            "deck": name,
            "seed": str(seed),
            "turns": turns,
            "keep_rule": _tier1_keep_json(rule),
            "on_the_play": on_play,
            "repr": repr(result),
        })

    runs: list[dict[str, Any]] = []
    for name, count, turns, seed, overrides in TIER1_RUNS:
        library, commander = decks[name]
        rule = KeepRule(**overrides)
        summary = run(library, commander, games=count, turns=turns,
                      keep_rule=rule, seed=seed)
        _tier1_floats(summary, floats)
        # `spells_through` is what a land sweep is read by, and it is a Python
        # *slice*: a negative turn drops from the end rather than meaning
        # nothing, and a turn past the horizon is the whole list. Recorded as
        # text because a sum of floats has one right answer and a tolerance
        # here would hide the summation rather than allow for it -- and it can
        # be recorded at all only because that sum is `fsum` now. It was
        # `sum`, which CPython compensates from 3.12 and not before, so these
        # values would have differed across the CI matrix and taken this
        # corpus's cross-version stability with them.
        through = [{"turn": t,
                    "spells": repr(summary.spells_through(t)),
                    "wasted": repr(summary.wasted_through(t))}
                   for t in (-1, 0, 1, 3, 8, 100)]
        runs.append({
            "deck": name,
            "through": through,
            "games": count,
            "turns": turns,
            "seed": str(seed),
            "keep_rule": _tier1_keep_json(rule),
            "repr": repr(summary),
        })

    outputs = probe.reference_outputs()
    digest = probe.reference_digest()
    library, commander = decks["golgari-34"]
    _tier1_floats(run(library, commander, games=probe.GAMES,
                      turns=probe.TURNS, seed=probe.SEED), floats)

    return {
        "note": ("Generated from the real engine by "
                 "`python tests/go_fixtures.py`. Seeds are strings because a "
                 "`random.Random` seed may outgrow a JSON number; floats are "
                 "Float64bits."),
        "reference": {
            "digest": digest,
            "seed": str(probe.SEED),
            "games": probe.GAMES,
            "turns": probe.TURNS,
            "sweep_counts": list(probe.SWEEP_COUNTS),
            "keep_rule": _tier1_keep_json(KeepRule()),
            "outputs": outputs,
        },
        "decks": {
            name: {
                "library": [_card_json(c) for c in library],
                "commander": (None if commander is None
                              else _card_json(commander)),
            }
            for name, (library, commander) in decks.items()
        },
        "games": games,
        "runs": runs,
        "consume": tier1_consume_cases(),
        "floats": tier1_float_cases(floats),
        "strings": tier1_string_cases(),
    }


def render_tier1_cases() -> str:
    return _compact_json(tier1_cases()) + "\n"


# -------------------------------------------------------------------- mana
#
# The castability solver's differential case set (PLAN section 5 item 2), and
# the one corpus in this file that was **written before the port needed it**.
# `tests/mana_oracle.py` has said since it was written that its cases yield
# "the same cases in the same order in any language, on any machine, forever
# -- which is what makes it usable as the differential case set for a compiled
# port". This is that sentence cashed in, and it is shaped by taking the claim
# literally: the corpus does not ship 13,944 rows, it ships the **enumeration**
# and the **answers**, because a port that cannot rebuild the case set itself
# has not honoured the claim -- it has only replayed a dump.
#
# So there are two digests rather than one, for the reason the draw corpus
# above records the raw word stream apart from its consumers. `enumeration`
# is a hash of the case *names* alone and says nothing about castability;
# `answers` is the project's own pinned golden, hashed over `name=answer`.
# A Go run that fails the first has an enumeration bug and its solver has not
# been tested at all; one that passes the first and fails the second has a
# solver bug and nothing else. Without the split, both present identically.
#
# `CASE_COSTS` and `CASE_POOLS` are the halves of `case_id`, in enumeration
# order, so a mismatch names the exact case in the oracle's own vocabulary --
# and comparing them is also what checks Go's `Cost.String()`, since the cost
# half of a case name is a rendered cost. `ANSWERS` is one row per cost and
# one column per pool, in that order, because the cross product is costs outer
# and pools inner: a changed row names the cost that changed.
#
# Nothing here needs a pool, a deck or a network. It is `math` and `itertools`.

#: Where the Go module reads the castability case set from.
MANA_PATH = ROOT / "go" / "internal" / "mana" / "testdata" / "castability.json"


def _mana_join(colors: frozenset[str]) -> str:
    """A colour set as `case_id` writes it: sorted and run together."""
    return "".join(sorted(colors))


def mana_cases() -> dict[str, Any]:
    """The enumeration, the answers, and both digests.

    Two things are re-checked here rather than assumed, and both would
    otherwise be able to write a corpus that is wrong in a way nothing
    downstream could see.

    **The three Python implementations are made to agree first.** `can_pay`,
    the brute-force search and Hall's condition are asserted equal across the
    whole case set before a byte is written, so a corpus can never be rendered
    from a Python that disagrees with itself -- which would hand the Go side a
    wrong answer to reproduce faithfully.

    **The pinned golden is compared, never re-pinned.**
    `CASES_ANSWER_DIGEST` in `tests/test_mana_properties.py` is the project's
    own record of what castability means, and this refuses to write if the
    digest it computes is not that one. Regenerating this corpus is therefore
    not a way to move the golden; moving the golden is a deliberate edit to
    that constant, with the reason in the commit message, exactly as its own
    comment says.
    """
    import mana_oracle as oracle
    import test_mana_properties as pinned
    from mtglab.mana import can_pay

    disagreements = [
        oracle.case_id(cost, pool)
        for cost, pool in oracle.all_cases()
        if not (can_pay(cost, pool)
                == oracle.brute_force_can_pay(cost, pool)
                == oracle.hall_can_pay(cost, pool))
    ]
    if disagreements:
        raise AssertionError(
            f"{len(disagreements)} case(s) disagree between can_pay and the "
            f"two oracles; first 10: {disagreements[:10]}. Refusing to write "
            f"a corpus Python does not itself agree on.")

    digest = oracle.cases_digest(can_pay)
    if digest != pinned.CASES_ANSWER_DIGEST:
        raise AssertionError(
            f"castability answers hash to {digest}, but "
            f"tests/test_mana_properties.py pins "
            f"{pinned.CASES_ANSWER_DIGEST}. Regenerating this corpus does not "
            f"move that golden -- decide whether the semantics really changed, "
            f"then edit the constant deliberately.")

    costs = list(oracle.case_costs())
    pools = list(oracle.case_pools())
    pool_ids = [
        " ".join(_mana_join(s.colors) + (f"x{s.amount}" if s.amount != 1 else "")
                 for s in pool)
        for pool in pools
    ]
    answers = ["".join(str(int(can_pay(cost, pool))) for pool in pools)
               for cost in costs]

    enumeration = hashlib.sha256()
    for cost, pool_id in ((c, p) for c in costs for p in pool_ids):
        enumeration.update(f"{cost} <- [{pool_id}]\n".encode())

    return {
        "note": ("The castability case set of tests/mana_oracle.py, written by "
                 "`python tests/go_fixtures.py`. Rebuild the enumeration rather "
                 "than trusting the lists: `costs` and `pools` are here to say "
                 "WHICH case disagreed, and to check the renderer that names "
                 "it."),
        "answers_digest": digest,
        "enumeration_digest": enumeration.hexdigest(),
        "max_pips": oracle.MAX_PIPS,
        "max_generic": oracle.MAX_GENERIC,
        "max_sources": oracle.MAX_SOURCES,
        "case_pips": [_mana_join(p) for p in oracle.CASE_PIPS],
        "case_units": [_mana_join(u) for u in oracle.CASE_UNITS],
        "costs": [str(cost) for cost in costs],
        "pools": pool_ids,
        "answers": answers,
        "cases": len(costs) * len(pools),
        "payable": sum(row.count("1") for row in answers),
    }


def render_mana_cases() -> str:
    return _compact_json(mana_cases()) + "\n"


# ---------------------------------------------------------- the compiler
#
# `sim/compile.py`, held to Go by two corpora in one file: the three text
# readers on their own, and whole decks compiled against the 21-card pool.
#
# The text cases matter more than they look. `mana_produced` is the function
# that made Sol Ring produce one mana for the life of this project, and it is
# read off prose -- so the corpus is prose, and **every string below marked
# with a card name was read out of the real pool on 2026-08-22 and pasted
# verbatim**, never typed from memory (rule 1 applies to test data; see
# `tiny_pool`'s own docstring making the same commitment). The generator needs
# no pool of its own, which is what lets it run in CI.

COMPILE_PATH = ROOT / "go" / "internal" / "sim" / "compile" / "testdata" / "compile.json"

#: `(card, oracle_text)` read from the pool. The cards are the ones
#: `mana_produced`'s docstring argues about, plus the land shapes
#: `enters_tapped` distinguishes and the ramp shapes `fetches_lands` does.
POOL_TEXTS: list[tuple[str, str]] = [
    ("Sol Ring", "{T}: Add {C}{C}."),
    ("Mana Vault",
     ("This artifact doesn't untap during your untap step.\n"
     "At the beginning of your upkeep, you may pay {4}. If you do, untap this "
     "artifact.\nAt the beginning of your draw step, if this artifact is "
     "tapped, it deals 1 damage to you.\n{T}: Add {C}{C}{C}.")),
    ("Gilded Lotus", "{T}: Add three mana of any one color."),
    ("Arcane Signet",
     "{T}: Add one mana of any color in your commander's color identity."),
    ("Talisman of Progress",
     "{T}: Add {C}.\n{T}: Add {W} or {U}. This artifact deals 1 damage to you."),
    ("Wooded Bastion", "{T}: Add {C}.\n{G/W}, {T}: Add {G}{G}, {G}{W}, or {W}{W}."),
    ("Ketria Triome",
     ("({T}: Add {G}, {U}, or {R}.)\nThis land enters tapped.\n"
     "Cycling {3} ({3}, Discard this card: Draw a card.)")),
    ("Grim Monolith",
     ("This artifact doesn't untap during your untap step.\n"
     "{T}: Add {C}{C}{C}.\n{4}: Untap this artifact.")),
    ("Nykthos, Shrine to Nyx",
     ("{T}: Add {C}.\n{2}, {T}: Choose a color. Add an amount of mana of that "
     "color equal to your devotion to that color. (Your devotion to a color is "
     "the number of mana symbols of that color in the mana costs of permanents "
     "you control.)")),
    ("Ashnod's Altar", "Sacrifice a creature: Add {C}{C}."),
    ("Phyrexian Tower", "{T}: Add {C}.\n{T}, Sacrifice a creature: Add {B}{B}."),
    ("Deadly Dispute",
     ("As an additional cost to cast this spell, sacrifice an artifact or "
     "creature.\nDraw two cards and create a Treasure token. (It's an artifact "
     "with \"{T}, Sacrifice this token: Add one mana of any color.\")")),
    ("Cultivate",
     ("Search your library for up to two basic land cards, reveal those cards, "
     "put one onto the battlefield tapped and the other into your hand, then "
     "shuffle.")),
    ("Nature's Lore",
     ("Search your library for a Forest card, put that card onto the "
     "battlefield, then shuffle.")),
    ("Three Visits",
     ("Search your library for a Forest card, put it onto the battlefield, then "
     "shuffle.")),
    ("Skyshroud Claim",
     ("Search your library for up to two Forest cards, put them onto the "
     "battlefield, then shuffle.")),
    ("Sakura-Tribe Elder",
     ("Sacrifice this creature: Search your library for a basic land card, put "
     "that card onto the battlefield tapped, then shuffle.")),
    ("Rampant Growth",
     ("Search your library for a basic land card, put that card onto the "
     "battlefield tapped, then shuffle.")),
    ("Temple of Plenty",
     ("This land enters tapped.\nWhen this land enters, scry 1. (Look at the "
     "top card of your library. You may put that card on the bottom.)\n"
     "{T}: Add {G} or {W}.")),
    ("Hallowed Fountain",
     ("({T}: Add {W} or {U}.)\nAs this land enters, you may pay 2 life. If you "
     "don't, it enters tapped.")),
    ("Rootbound Crag",
     ("This land enters tapped unless you control a Mountain or a Forest.\n"
     "{T}: Add {R} or {G}.")),
    ("Birds of Paradise", "Flying\n{T}: Add one mana of any color."),
    ("Llanowar Elves", "{T}: Add {G}."),
    ("Command Tower",
     "{T}: Add one mana of any color in your commander's color identity."),
    ("Wastes", "{T}: Add {C}."),
    ("Dark Ritual", "Add {B}{B}{B}."),
    ("Black Lotus", "{T}, Sacrifice this artifact: Add three mana of any one color."),
    ("Chromatic Lantern",
     ("Lands you control have \"{T}: Add one mana of any color.\"\n"
     "{T}: Add one mana of any color.")),
    ("Ancient Tomb", "{T}: Add {C}{C}. This land deals 2 damage to you."),
    ("City of Brass",
     ("Whenever this land becomes tapped, it deals 1 damage to you.\n"
     "{T}: Add one mana of any color.")),
    ("Sanctum of Eternity",
     ("{T}: Add {C}.\n{2}, {T}: Return target commander you own from the "
     "battlefield to your hand. Activate only during your turn.")),
]

#: Strings nobody printed on a card, each aimed at a branch the real texts
#: cannot reach. They are labelled `constructed` in the corpus so nothing here
#: can be mistaken for a claim about a card.
CONSTRUCTED_TEXTS: list[tuple[str, str]] = [
    ("empty", ""),
    ("no add at all", "Flying, trample"),
    ("add but not mana", "{T}: Add a +1/+1 counter to target creature."),
    ("add with no colon and no period", "Add {R}{R}"),
    ("colon at the very end", "Sacrifice this artifact:"),
    ("the cost is dearer than the mana", "{G}{G}{G}, {T}: Add {G}."),
    ("a second ability out-produces the first",
     "{T}: Add {W}.\n{T}: Add {W}{W}{W}{W}."),
    ("an amount word past the first token", "{T}: Add mana three of any color."),
    ("an amount word that is not in the table",
     "{T}: Add six mana of any one color."),
    ("the tapped clause with old templating",
     "Bloodstained Mire enters the battlefield tapped."),
    ("tapped, and a shock clause on another line",
     "This land enters tapped.\nYou may pay 2 life instead."),
    ("tapped unless, upper case",
     "THIS LAND ENTERS TAPPED UNLESS YOU CONTROL A SWAMP."),
    ("a fetch that names no land", ("Search your library for a card, put it "
                                  "onto the battlefield, then shuffle.")),
    ("a fetch to the hand only",
     "Search your library for a Forest card, put it into your hand, then shuffle."),
    ("a fetch that says up to two",
     ("Search your library for up to two Plains cards, put them onto the "
     "battlefield, then shuffle.")),
    ("a symbol that is not mana", "{T}, {Q}: Add {C}."),
    # The substring test, above the floor. A single such symbol is invisible
    # -- one mana and zero mana both floor to one -- so each of these has to
    # move a number that is already above it. A mutation survived without
    # them, which is the whole reason they are here.
    ("a hybrid half that is a substring of WUBRGC", "{T}: Add {WU}{WU}."),
    ("a hybrid half that is not", "{T}: Add {UW}{UW}."),
    ("a substring-ish symbol in the activation cost",
     "{WU}, {T}: Add {C}{C}{C}."),
    # `{}` matches nothing -- the pattern wants one character or more -- so
    # the only way to reach an EMPTY half is a symbol that is just the
    # separator, where `"" in "WUBRGC"` is Python's answer and `part == "W"`
    # would not be.
    ("a symbol that is only a separator", "{T}: Add {/}{/}{/}."),
    ("a separator symbol in the activation cost", "{/}, {T}: Add {C}{C}."),
    ("a braces pair with nothing in it", "{}, {T}: Add {C}{C}."),
    ("a numeric symbol in the add clause", "{T}: Add {2}."),
    ("a run with no comma beats the alternatives",
     "{T}: Add {C}{C}, {G}, or {W}."),
]


def compile_text_cases() -> list[dict[str, Any]]:
    """Every text reader's answer for every string above.

    All three in one record per string, because they read the same text and a
    corpus split three ways would let a case be dropped from one of them
    silently.
    """
    from mtglab.sim import compile as sim_compile
    out = []
    for source, texts in (("pool", POOL_TEXTS), ("constructed", CONSTRUCTED_TEXTS)):
        for label, text in texts:
            out.append({
                "source": source,
                "label": label,
                "text": text,
                "enters_tapped": sim_compile.enters_tapped(text),
                "mana_produced": sim_compile.mana_produced(text),
                "fetches_lands": sim_compile.fetches_lands(text),
            })
    return out


def _compiled_card_json(card: SimCard) -> dict[str, Any]:
    """A compiled card with **every** field written, defaults included.

    `_card_json` above omits a field holding its default, which is right for a
    corpus of decks somebody wrote by hand. This one is the compiler's own
    output, and the field that makes the difference is `category`: Python's
    `SimCard` defaults it to "utility" and `compile_one` never assigns it, so
    the natural Go port leaves the zero value "" -- invisible to every tier,
    and visible in the ADR 18 cache key, which serialises it. Writing it out
    is what makes the corpus able to say so.
    """
    return {
        "name": card.name,
        "cost": {
            "generic": card.cost.generic,
            "pips": [sorted(p) for p in card.cost.pips],
            "phyrexian": [sorted(p) for p in card.cost.phyrexian],
            "has_x": card.cost.has_x,
        },
        "category": card.category,
        "is_land": card.is_land,
        "enters_tapped": card.enters_tapped,
        "produces": [{"colors": sorted(s.colors), "amount": s.amount}
                     for s in card.produces],
        "produce_delay": card.produce_delay,
        "fetches_lands": card.fetches_lands,
    }


def compile_deck_cases() -> list[dict[str, Any]]:
    """Whole decks compiled against the 21-card pool, and the refusals.

    Each case is the deck's YAML text plus Python's report, so the Go test
    parses the same text, builds the same pool from `pooltest`, and must
    produce the same library in the same order -- the order being real input,
    since `qty` expansion and Tier 1's shuffle both read it.
    """
    import tiny_pool
    from mtglab.cards import db
    from mtglab.sim import compile as sim_compile

    def nothing_resolves() -> Deck:
        """Every card invented, and a commander the pool DOES know.

        The commander is what makes this `NothingToSimulate` rather than
        `PoolRequired`: `get_cards` returns only what it found, so a deck where
        not one single name resolves hands the compiler an **empty mapping**,
        and `if not cards` cannot tell that from "this machine has no pool".
        See `empty-lookup` below, which is that case and reports the other
        error. A wart, reproduced rather than fixed -- the two runtimes have to
        agree, and changing which exception a deck raises is a decision for the
        surface that renders it.
        """
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "nothing-resolves", "Nothing In The 99 Resolves"
        d.cards = [CardEntry(f"Invented Card {i}", "utility", "Nothing knows it.")
                   for i in range(5)]
        return d

    def empty_lookup() -> Deck:
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "empty-lookup", "Not One Name Resolves"
        d.commander = ["Not A Real Commander"]
        d.cards = [CardEntry(f"Invented Card {i}", "utility", "Nothing knows it.")
                   for i in range(5)]
        return d

    def half_unknown() -> Deck:
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "half-unknown", "Half Unknown"
        d.cards.insert(2, CardEntry("Invented Card A", "ramp", "Absent."))
        d.cards.insert(9, CardEntry("Invented Card B", "threat", "Absent too."))
        return d

    def unknown_commander() -> Deck:
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "unknown-commander", "Unknown Commander"
        d.commander = ["Not A Real Commander"]
        return d

    def no_commander() -> Deck:
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "no-commander", "No Commander At All"
        d.commander = []
        return d

    def commander_in_the_99() -> Deck:
        """The gate refuses this; the compiler has no opinion about it.

        It is here for the one thing no legal deck can show: the commander is
        compiled **again**, as its own object, even when a card of the same
        name is in the library. Tier 1 asks `chosen is commander`, so a port
        that reused the library card would silently stop the commander being
        castable from the command zone.
        """
        d = tiny_pool.mono_green_deck(clean=True)
        d.slug, d.name = "commander-in-the-99", "Commander In The 99"
        d.cards.insert(1, CardEntry(d.commander[0], "threat",
                                    "The commander, again."))
        return d

    cases: list[tuple[str, str, Deck, bool]] = [
        ("mono-green", ("the 21-card pool's own legal 99, every name resolving "
                       "and `qty` expanding the basics"),
         tiny_pool.mono_green_deck(clean=True), True),
        ("banned", ("the same deck with the banned card in it -- the compiler "
                   "has no opinion, which is the gate's job"),
         tiny_pool.mono_green_deck(), True),
        ("half-unknown", ("two names the pool lacks: the deck SHRINKS, and the "
                         "report is the only thing that says so"),
         half_unknown(), True),
        ("unknown-commander", ("the commander alone is missing, which is its "
                              "own field because every commander-speed figure "
                              "is then about nothing"),
         unknown_commander(), True),
        ("no-commander", ("a deck declaring no commander: `commander` is None "
                         "and `commander_unresolved` is FALSE, which is the "
                         "distinction a `!= nil` check loses"),
         no_commander(), True),
        ("commander-in-the-99", ("illegal, and the only shape that shows the "
                                "commander being compiled a second time "
                                "rather than aliased out of the library"),
         commander_in_the_99(), True),
        ("nothing-resolves", ("no card in the 99 resolves, so "
                             "`NothingToSimulate` -- named with the DECLARED "
                             "size, which is the number that makes it "
                             "diagnosable"),
         nothing_resolves(), True),
        ("empty-lookup", ("not one name resolves, so the pool mapping comes "
                         "back EMPTY and the compiler cannot tell that from "
                         "having no pool: `PoolRequired`, which is the wrong "
                         "word and the right behaviour to reproduce"),
         empty_lookup(), True),
        ("rich", "the hand-written fixture, whose names the pool mostly lacks",
         rich_deck(), True),
        ("no-pool", ("the same legal 99 with no pool at all: `PoolRequired`, "
                    "and it is checked BEFORE emptiness, because a broken "
                    "machine and an empty deck want different answers"),
         tiny_pool.mono_green_deck(clean=True), False),
    ]

    out = []
    with tempfile.TemporaryDirectory() as tmp:
        pool_path = tiny_pool.build(Path(tmp) / "mtg.duckdb")
        con = db.connect_readonly(pool_path)
        try:
            for name, why, deck, with_pool in cases:
                text = deck.dump()
                parsed = Deck.from_text(text, slug=deck.slug)
                names = parsed.commander + [c.name for c in parsed.cards]
                cards = db.get_cards(con, names) if with_pool else None
                case: dict[str, Any] = {"name": name, "why": why,
                                        "with_pool": with_pool, "yaml": text}
                try:
                    report = sim_compile.compile_report(parsed, cards)
                except sim_compile.NothingToSimulate as exc:
                    case["error"] = "nothing_to_simulate"
                    case["message"] = str(exc)
                except sim_compile.PoolRequired as exc:
                    case["error"] = "pool_required"
                    case["message"] = str(exc)
                else:
                    case["report"] = {
                        "library": [_compiled_card_json(c) for c in report.library],
                        "commander": (None if report.commander is None
                                      else _compiled_card_json(report.commander)),
                        "unresolved": list(report.unresolved),
                        "declared_size": report.declared_size,
                        "simulated_size": report.simulated_size,
                        "commander_unresolved": report.commander_unresolved,
                        "complete": report.complete,
                    }
                out.append(case)
        finally:
            con.close()
    return out


#: Card shapes the 21-card pool does not hold, each of which decides one line
#: of `compile_one` and none of which any deck fixture can reach. Written as
#: pool records rather than as decks, because what they exercise is the
#: *reading of a record* and building a deck around each would be four more
#: 99-card fixtures to say four things.
#:
#: Every one is here because a mutation survived without it. The type lines are
#: shaped like Scryfall's -- a `//` between faces -- and the oracle text is
#: from the real cards named in the label where there is one.
COMPILE_RECORDS: list[tuple[str, dict[str, Any]]] = [
    ("a creature on the BACK of a mana artifact",
     {"name": "Front Rock // Back Beast", "mana_cost": "{2}",
      "type_line": "Artifact // Creature — Beast", "layout": "transform",
      "oracle_text": "{T}: Add {C}{C}.", "produced_mana": ("C",)}),
    ("a mana creature, front and back",
     {"name": "Elf Front // Elf Back", "mana_cost": "{G}",
      "type_line": "Creature — Elf Druid // Creature — Elf Warrior",
      "layout": "transform", "oracle_text": "{T}: Add {G}.",
      "produced_mana": ("G",)}),
    ("a land that is also a creature (Dryad Arbor's shape)",
     {"name": "Arbor Shape", "mana_cost": None,
      "type_line": "Land Creature — Forest Dryad",
      "oracle_text": "{T}: Add {G}.", "produced_mana": ("G",)}),
    ("a fetchland, which is net ZERO lands",
     {"name": "Fetch Shape", "mana_cost": None, "type_line": "Land",
      "oracle_text": "{T}, Pay 1 life, Sacrifice this land: Search your "
                     "library for a Forest or Plains card, put it onto the "
                     "battlefield, then shuffle.",
      "produced_mana": ()}),
    ("a sorcery that makes Treasure, so Scryfall reports produced mana",
     {"name": "Treasure Maker", "mana_cost": "{1}{B}", "type_line": "Sorcery",
      "oracle_text": "Create a Treasure token.",
      "produced_mana": ("W", "U", "B", "R", "G")}),
    ("an instant that makes mana",
     {"name": "Ritual Shape", "mana_cost": "{B}", "type_line": "Instant",
      "oracle_text": "Add {B}{B}{B}.", "produced_mana": ("B",)}),
    ("an enchantment that makes mana, which IS a permanent",
     {"name": "Enchanted Source", "mana_cost": "{2}{G}",
      "type_line": "Enchantment", "oracle_text": "{T}: Add {G}{G}.",
      "produced_mana": ("G",)}),
    ("a modal DFC whose back is a land",
     {"name": "Spell Front // Land Back", "mana_cost": "{1}{U}",
      "type_line": "Instant // Land", "layout": "modal_dfc",
      "oracle_text": "Draw a card.\n{T}: Add {U}.", "produced_mana": ("U",)}),
    # `p in "WUBRGC"` is a SUBSTRING test in Python, not set membership, so an
    # empty string passes it and "X" does not. Scryfall never sends either;
    # this records what the filter actually is, because the natural Go port
    # (`p == "W" || ...`) would answer differently and no real card would say
    # so.
    ("a produced-mana list with an empty entry and an unproducible one",
     {"name": "Odd Producer", "mana_cost": "{3}", "type_line": "Artifact",
      "oracle_text": "{T}: Add {C}.", "produced_mana": ("C", "", "X")}),
]


def compile_record_cases() -> list[dict[str, Any]]:
    """`compile_one` over records the pool cannot supply.

    A one-card deck each, so the answer is the compiled card itself and the
    reader can see which field the label is about.
    """
    import dataclasses as _dc

    from mtglab.sim import compile as sim_compile

    out = []
    for label, fields in COMPILE_RECORDS:
        rec = _dc.replace(
            _record(fields["name"], fields.get("mana_cost")),
            type_line=fields["type_line"],
            oracle_text=fields["oracle_text"],
            produced_mana=tuple(fields.get("produced_mana", ())),
            layout=fields.get("layout", "normal"),
        )
        deck = Deck(slug="one-card", name="One Card", commander=[],
                    cards=[CardEntry(rec.name, "utility", "The only card.")])
        library, _ = sim_compile.compile_deck(deck, {rec.name: rec})
        out.append({
            "label": label,
            "record": {
                "name": rec.name,
                "mana_cost": rec.mana_cost,
                "type_line": rec.type_line,
                "oracle_text": rec.oracle_text,
                "produced_mana": list(rec.produced_mana),
                "layout": rec.layout,
                "is_land": rec.is_land,
            },
            "card": _compiled_card_json(library[0]),
        })
    return out


def compile_cases() -> dict[str, Any]:
    return {
        "note": ("`sim/compile.py`, written by `python tests/go_fixtures.py`. "
                 "Every `pool` text was read out of the real card pool and "
                 "pasted verbatim; every `constructed` one is a string nobody "
                 "printed, aimed at a branch the real ones cannot reach."),
        "texts": compile_text_cases(),
        "records": compile_record_cases(),
        "decks": compile_deck_cases(),
    }


def render_compile_cases() -> str:
    return _rows_json(compile_cases()) + "\n"


# ---------------------------------------------------------- the mulligan grid
#
# `sim/mulligan.py`. The grid itself is arithmetic and cheap to check; the
# sweeps are 33 seeded Tier 1 runs apiece, which is why `games` is small here
# -- the corpus is proving that Go ranks the same rules in the same order, not
# re-proving Tier 1, which `tier1.json` and REFERENCE_DIGEST already did.

MULLIGAN_PATH = ROOT / "go" / "internal" / "sim" / "mulligan" / "testdata" / "mulligan.json"


def _policy_row_json(row: Any) -> dict[str, Any]:
    """One `PolicyRow`, with every float as its bits.

    Bits rather than a tolerance for the reason `karsten.json` gives: the
    verdict is a `<` against `FLAT` and the ranking is a sort on these
    numbers, so one ulp is a different recommended rule rather than a
    different last decimal.
    """
    median = row.median_commander_turn
    return {
        "min_lands": row.min_lands,
        "max_lands": row.max_lands,
        "min_pieces": row.min_pieces,
        "spells_through_t8": _float_bits(row.spells_through_t8),
        "mulligan_rate": _float_bits(row.mulligan_rate),
        "avg_mulligans": _float_bits(row.avg_mulligans),
        # `statistics.median` answers an int for an odd count, so the type is
        # part of the answer -- `tier1.Number` is Go's word for that.
        "median_commander_turn": None if median is None else {
            "is_int": isinstance(median, int) and not isinstance(median, bool),
            "value": median if isinstance(median, int) and not isinstance(median, bool)
            else _float_bits(median),
        },
        "color_screw_rate": _float_bits(row.color_screw_rate),
        "stalled_turns": _float_bits(row.stalled_turns),
        "describe": row.describe,
    }


def mulligan_extra_decks() -> dict[str, dict[str, Any]]:
    """Decks built for this module's tie-breakers, not for the closed forms.

    Kept out of `closed_form_decks` deliberately: `karsten.json` and
    `curve.json` compute every function over every deck there, so adding two
    would grow both corpora to say nothing about either module.

    **Both exist because a mutation survived without them.** The grid's
    ordering rules -- `max` keeping the first extreme, the sort's secondary
    key, and `gentlest`'s half-open band -- can only be observed when two rules
    *tie* on deployment, and across ten real decks at seven sample sizes and
    six seeds, none ever did: the winner was always alone at the top. A
    corpus of real decks proves the arithmetic and cannot reach the
    comparators.
    """
    lands = [_sim_land(f"Forest {i}", "G") for i in range(40)]
    return {
        "uncastable": {
            "why": ("40 lands and 59 spells nobody can cast inside the "
                    "horizon, so EVERY rule deploys exactly 0.0 and the "
                    "ranking is decided entirely by the mulligan-rate "
                    "tie-break -- which is the only way to see which "
                    "direction it breaks in"),
            "library": lands + [_sim_spell(f"Colossus {i}", "{20}")
                                for i in range(59)],
            "commander": None,
        },
        "quarter-steps": {
            "why": ("34 lands and a handful of one-drops. Run at FOUR games, "
                    "so every deployment figure is a whole number of spells "
                    "over four and therefore an exact multiple of 0.25 -- "
                    "which puts rows exactly `FLAT` apart, where `< FLAT` and "
                    "`<= FLAT` choose different `gentlest` rules. The same "
                    "trick as `tie-breaker` in the closed forms' decks, aimed "
                    "at a different boundary"),
            "library": ([_sim_land(f"Forest {i}", "G") for i in range(34)]
                        + [_sim_spell(f"One Drop {i}", "{G}") for i in range(6)]
                        + [_sim_spell(f"Colossus {i}", "{20}")
                           for i in range(59)]),
            "commander": None,
        },
    }


def _mulligan_sweeps() -> list[dict[str, Any]]:
    from mtglab.sim import mulligan
    from mtglab.sim.tier1.engine import KeepRule

    decks = dict(closed_form_decks(), **mulligan_extra_decks())

    # A grid that deliberately EXCLUDES the default rule, so `search`'s
    # baseline fallback -- the branch that runs a thirty-fourth simulation --
    # is exercised. Nothing in the standard grid can reach it.
    off_grid = [KeepRule(min_lands=1, max_lands=6, min_mana_pieces=2),
                KeepRule(min_lands=3, max_lands=4, min_mana_pieces=4),
                KeepRule(min_lands=2, max_lands=6, min_mana_pieces=5)]

    plan: list[tuple[str, str, str, dict[str, Any]]] = [
        ("naya", "naya",
         ("a real three-colour deck with a commander, where colour screw and "
         "ramp both move the ranking"),
         {"games": 150, "turns": 10, "seed": 7}),
        ("esper-rocks", "esper-rocks",
         ("rocks rather than lands, so the `min_mana_pieces` axis is the one "
         "that separates the rules"),
         {"games": 150, "turns": 10, "seed": 7}),
        ("mono-green-poor", "mono-green-poor",
         ("a deck the grid cannot rescue: most rules land within `FLAT` of the "
         "default, which is the `flat` verdict and the `gentlest` answer"),
         {"games": 150, "turns": 8, "seed": 11}),
        ("all-lands", "all-lands",
         ("every hand keeps under every rule, so every row ties and `best` is "
         "decided purely by the grid's ORDER -- Python's `max` keeps the "
         "first, and a sort would keep the last"),
         {"games": 60, "turns": 6, "seed": 3}),
        ("no-lands", "no-lands",
         ("no hand is ever keepable, so every rule mulligans to the floor and "
         "the whole grid ties at the bottom"),
         {"games": 60, "turns": 6, "seed": 3}),
        ("naya-off-grid", "naya",
         ("an explicit rule list with the default NOT in it, which is the only "
         "way to reach the baseline fallback"),
         {"games": 120, "turns": 9, "seed": 5, "rules": off_grid}),
        ("uncastable", "uncastable",
         ("every rule deploys 0.0, so the whole ranking is the mulligan-rate "
         "tie-break and nothing else"),
         {"games": 120, "turns": 10, "seed": 7}),
        ("quarter-steps", "quarter-steps",
         ("four games, so every deployment is an exact multiple of 0.25 and "
         "rows land exactly `FLAT` apart. The seed is 2 and it was SEARCHED "
         "for: at that seed a row exactly `FLAT` below the best mulligans "
         "half as often as anything inside the band, so `< FLAT` and "
         "`<= FLAT` name different `gentlest` rules -- which is the only way "
         "that boundary is visible from outside"),
         {"games": 4, "turns": 10, "seed": 2}),
    ]

    out = []
    for label, deck_name, why, kwargs in plan:
        spec = decks[deck_name]
        sweep = mulligan.search(spec["library"], spec["commander"], **kwargs)
        rules = kwargs.get("rules")
        out.append({
            "label": label,
            "deck": deck_name,
            "why": why,
            "games": sweep.games,
            "turns": sweep.turns,
            "seed": sweep.seed,
            "rules": None if rules is None else [
                {"min_lands": r.min_lands, "max_lands": r.max_lands,
                 "min_mana_pieces": r.min_mana_pieces,
                 "cheap_ramp_mv": r.cheap_ramp_mv,
                 "max_mulligans": r.max_mulligans} for r in rules],
            "rows": [_policy_row_json(r) for r in sweep.rows],
            "best": _policy_row_json(sweep.best),
            "baseline": _policy_row_json(sweep.baseline),
            "spread": _float_bits(sweep.spread),
            "gain": _float_bits(sweep.gain),
            "flat": sweep.flat,
            "gentlest": _policy_row_json(sweep.gentlest),
        })
    return out


#: Which of `closed_form_decks` the sweeps below run over. Named here so the
#: corpus carries those decks and only those: the file is already 33 rows per
#: sweep, and fourteen decks nobody simulates would be most of its bytes.
MULLIGAN_DECKS = ("naya", "esper-rocks", "mono-green-poor", "all-lands",
                  "no-lands", "uncastable", "quarter-steps")


def mulligan_cases() -> dict[str, Any]:
    from mtglab.sim import mulligan
    decks = dict(closed_form_decks(), **mulligan_extra_decks())
    return {
        "note": ("`sim/mulligan.py`, written by `python tests/go_fixtures.py`. "
                 "Floats are Float64bits: the verdict is a `<` against FLAT "
                 "and the table is a sort, so one ulp is a different "
                 "recommendation."),
        "decks": [
            {"name": name,
             "why": decks[name]["why"],
             "library": [_card_json(c) for c in decks[name]["library"]],
             "commander": (None if decks[name]["commander"] is None
                           else _card_json(decks[name]["commander"]))}
            for name in MULLIGAN_DECKS],
        "through": mulligan.THROUGH,
        "flat": _float_bits(mulligan.FLAT),
        "min_lands": list(mulligan.MIN_LANDS),
        "max_lands": list(mulligan.MAX_LANDS),
        "min_pieces": list(mulligan.MIN_PIECES),
        "candidates": [
            {"min_lands": r.min_lands, "max_lands": r.max_lands,
             "min_mana_pieces": r.min_mana_pieces,
             "cheap_ramp_mv": r.cheap_ramp_mv, "max_mulligans": r.max_mulligans,
             "describe": r.describe()}
            for r in mulligan.candidates()],
        "sweeps": _mulligan_sweeps(),
    }


def render_mulligan_cases() -> str:
    return _rows_json(mulligan_cases()) + "\n"


# ------------------------------------------------------------- the sim cache
#
# ADR 18's key, and the one place in this port where "equivalent JSON" is not
# equivalent at all: the key is a sha256 over the serialised payload, so the
# bytes are the contract. The corpus records **the payload string itself**
# beside the key, because a mismatch in the hash alone says nothing about
# where, and every one of the three ways Go's `encoding/json` differs from
# Python's `json.dumps` would show up as the same opaque digest.
#
# It also records Python's live engine fingerprint. That makes this corpus
# regenerate whenever `engine.py` or `mana.py` moves, which is not a nuisance
# but the point: such an edit re-keys every stored row in `app.db`, and having
# to run the generator is the reminder.

CACHE_PATH = ROOT / "go" / "internal" / "sim" / "cache" / "testdata" / "cache.json"

#: Real card names carrying non-ASCII, read out of the pool on 2026-08-22.
#: `ensure_ascii=True` escapes each of these to `\uXXXX` and Go's encoder does
#: not, so a library holding one would key differently in the two runtimes --
#: which is the difference nobody would have gone looking for.
NON_ASCII_NAMES = ["Bösium Strip", "Déjà Vu", "Círdan the Shipwright", "Dandân"]


def _cache_inputs() -> list[dict[str, Any]]:
    """The `(kind, run arguments)` pairs the key is computed over."""
    from mtglab.mana import ManaCost, ManaSource
    from mtglab.sim.tier1.engine import KeepRule, SimCard

    plain = [
        SimCard(name="Forest", cost=ManaCost(), is_land=True,
                produces=(ManaSource(frozenset({"G"}), 1),)),
        SimCard(name="Sol Ring", cost=ManaCost(generic=1),
                produces=(ManaSource(frozenset({"C"}), 2),)),
        SimCard(name="Beast", cost=ManaCost(generic=2, pips=(frozenset({"G"}),))),
    ]
    commander = SimCard(name="Goreclaw, Terror of Qal Sisma",
                        cost=ManaCost(generic=3, pips=(frozenset({"G"}),)))
    exotic = [
        # Every escape `encode_basestring_ascii` knows, in one name.
        SimCard(name="quote \" backslash \\ tab \t newline \n bell \x07 del \x7f"),
        # Above the BMP, which Python writes as a surrogate PAIR.
        SimCard(name="astral \U0001f600 and \U0010ffff"),
        *[SimCard(name=n) for n in NON_ASCII_NAMES],
    ]
    shapes = [
        SimCard(name="Hybrid", cost=ManaCost(pips=(frozenset({"G", "W"}),))),
        SimCard(name="Phyrexian", cost=ManaCost(generic=2,
                                                phyrexian=(frozenset({"U"}),))),
        SimCard(name="X Spell", cost=ManaCost(generic=1, has_x=True)),
        SimCard(name="Rainbow", produces=(ManaSource(frozenset("WUBRG"), 1),
                                          ManaSource(frozenset({"C"}), 3))),
        SimCard(name="Tapland", is_land=True, enters_tapped=True,
                produces=(ManaSource(frozenset({"W", "U"}), 1),)),
        SimCard(name="Sickly Rock", produces=(ManaSource(frozenset({"G"}), 1),),
                produce_delay=1),
        SimCard(name="Cultivate", cost=ManaCost(generic=2,
                                                pips=(frozenset({"G"}),)),
                fetches_lands=2),
        SimCard(name="Categorised", category="ramp"),
    ]
    grid = sorted((r.min_lands, r.max_lands, r.min_mana_pieces)
                  for r in _mulligan_module().candidates())
    return [
        {"label": "the plain shape",
         "why": "three cards, a commander, and no extras",
         "kind": "sim.mana", "library": plain, "commander": commander,
         "games": 20000, "turns": 12, "keep_rule": KeepRule(), "seed": 4242,
         "extra": None},
        {"label": "no commander",
         "why": "`None` renders as `null`, not as an absent field",
         "kind": "sim.mana", "library": plain, "commander": None,
         "games": 2000, "turns": 10, "keep_rule": KeepRule(), "seed": 7,
         "extra": None},
        {"label": "an empty library",
         "why": "the compiler refuses this, and the key must still be a key",
         "kind": "sim.lands.count", "library": [], "commander": None,
         "games": 1, "turns": 1, "keep_rule": KeepRule(), "seed": 0,
         "extra": None},
        {"label": "every cost shape",
         "why": "hybrid, Phyrexian, X, several sources, a delay, a fetch, and "
                "the one card carrying a category the compiler never sets",
         "kind": "sim.mana", "library": shapes, "commander": None,
         "games": 2000, "turns": 10,
         "keep_rule": KeepRule(min_lands=1, max_lands=6, min_mana_pieces=5,
                               cheap_ramp_mv=3, max_mulligans=6),
         "seed": -1, "extra": None},
        {"label": "names JSON has to escape",
         "why": "the ensure_ascii difference, which is the one `SetEscapeHTML` "
                "does not cover and the one real card names actually hit",
         "kind": "sim.mana", "library": exotic, "commander": exotic[0],
         "games": 2000, "turns": 10, "keep_rule": KeepRule(), "seed": 1,
         "extra": None},
        {"label": "the policy sweep's extra",
         "why": "`sim.policy`'s real key, carrying the whole grid, an int and "
                "a FLOAT -- 0.25 renders through `float.__repr__`",
         "kind": "sim.policy", "library": plain, "commander": commander,
         "games": 2000, "turns": 10, "keep_rule": KeepRule(), "seed": 7,
         "extra": {"grid": [list(g) for g in grid],
                   "through": _mulligan_module().THROUGH,
                   "flat": _mulligan_module().FLAT}},
        {"label": "an extra whose keys are not in order",
         "why": "`sort_keys=True` sorts them, and a Go map walk is random",
         "kind": "sim.policy", "library": plain, "commander": None,
         "games": 2000, "turns": 10, "keep_rule": KeepRule(), "seed": 7,
         "extra": {"zebra": True, "alpha": None, "Middle": "text",
                   "nested": {"b": [1, 2, 3], "a": 0.5}}},
    ]


def _mulligan_module() -> Any:
    from mtglab.sim import mulligan
    return mulligan


def cache_cases() -> dict[str, Any]:
    """Python's payload and key for each input, plus its engine fingerprint.

    The Go test uses the recorded fingerprint to reproduce the payload byte for
    byte, which is what makes the *other* claim precise: the two runtimes'
    keys differ in exactly one field, deliberately, and not because two JSON
    encoders quietly disagreed.
    """
    import json as _json

    from mtglab.sim import cache

    engine = cache.fingerprint()
    if engine is None:
        raise AssertionError(
            "sim/cache.fingerprint() is None, so this corpus cannot record "
            "what Python keys on. That means engine.py or mana.py could not "
            "be read, which is a broken checkout rather than a fixture bug.")

    cases = []
    for spec in _cache_inputs():
        key = cache.key(spec["kind"], library=spec["library"],
                        commander=spec["commander"], games=spec["games"],
                        turns=spec["turns"], keep_rule=spec["keep_rule"],
                        seed=spec["seed"], extra=spec["extra"])
        # The payload itself, assembled exactly as `cache.key` assembles it.
        # Duplicated deliberately rather than exported from the module: the
        # corpus records what the key *is*, and a helper both sides called
        # would make a shared mistake invisible.
        payload = {
            "version": cache.SIM_VERSION,
            "engine": engine,
            "kind": spec["kind"],
            "library": [cache._card_form(c) for c in spec["library"]],
            "commander": (None if spec["commander"] is None
                          else cache._card_form(spec["commander"])),
            "games": spec["games"],
            "turns": spec["turns"],
            "keep_rule": dataclasses.asdict(spec["keep_rule"]),
            "seed": spec["seed"],
            **({"extra": dict(spec["extra"])} if spec["extra"] else {}),
        }
        blob = _json.dumps(payload, sort_keys=True, separators=(",", ":"),
                           ensure_ascii=True)
        assert hashlib.sha256(blob.encode("utf-8")).hexdigest() == key, (
            "this corpus reassembles the payload and no longer agrees with "
            "`cache.key`; the two have drifted")
        cases.append({
            "label": spec["label"],
            "why": spec["why"],
            "kind": spec["kind"],
            "library": [_compiled_card_json(c) for c in spec["library"]],
            "commander": (None if spec["commander"] is None
                          else _compiled_card_json(spec["commander"])),
            "games": spec["games"],
            "turns": spec["turns"],
            "keep_rule": dataclasses.asdict(spec["keep_rule"]),
            "seed": spec["seed"],
            "extra": spec["extra"],
            "payload": blob,
            "key": key,
        })
    return {
        "note": ("ADR 18's cache key, written by `python tests/go_fixtures.py`. "
                 "`payload` is the exact string sha256 is taken over -- "
                 "recorded because a digest mismatch alone cannot say WHICH of "
                 "the three encoder differences caused it."),
        "sim_version": cache.SIM_VERSION,
        "max_rows": cache.MAX_ROWS,
        "run_inputs": sorted(cache.RUN_INPUTS),
        "run_non_inputs": sorted(cache.RUN_NON_INPUTS),
        "engine_sources": list(cache._ENGINE_SOURCES),
        "python_fingerprint": engine,
        "cases": cases,
    }


def render_cache_cases() -> str:
    return _rows_json(cache_cases()) + "\n"


def write_sim_engine() -> None:
    """The three files Phase 5's engine tail is held to. Called by `write`."""
    for path, body, what in (
        (COMPILE_PATH, render_compile_cases(), "the compiler"),
        (MULLIGAN_PATH, render_mulligan_cases(), "the mulligan grid"),
        (CACHE_PATH, render_cache_cases(), "the sim cache's key"),
    ):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body, encoding="utf-8")
        print(f"wrote {what} into {path}")


# ------------------------------------------------------- the seventh mode

def scan_cases() -> dict[str, Any]:
    """`claude/scan.py`: the camera's reader, held to Python.

    Four tables, because a failure that names its table localises itself.

    * `media_types` -- every label the mode is handed, accepted or refused.
      The membership test is `media_type not in MEDIA_TYPES` over a frozenset
      of strings, so it is **exact**: no trimming, no case folding. The
      tolerant version is the one a port writes by reflex, so the uppercase
      and padded spellings are here to refuse.
    * `captures` -- what `_payload` does with a capture, and this is the
      table that matters. `base64.b64decode(s, validate=True)` is strict, and
      the strictness is load-bearing rather than fussy: the **default**
      `b64decode` silently discards characters outside the alphabet, so a
      capture carrying a stray newline would decode shorter and be sent as a
      corrupt image instead of refused. Whether Go's `StdEncoding` draws the
      same line is a question about two libraries, not one this port gets to
      answer by reading, so every edge is measured -- newlines, spaces,
      missing padding, over-padding, the URL-safe alphabet, and a non-zero
      trailing-bit encoding that the two could plausibly disagree about.
    * `sightings` -- a finished turn read back as the `Sighting` the pool's
      reader already takes. Every failure is "nothing legible": a refusal,
      unparseable text, a non-object, a non-string field, a whitespace-only
      field. A response schema makes the middle three nearly impossible and
      "nearly" is why the branches exist.
    * `stances` -- `scan.stance_for`, whose default is `consultant` and not
      `second-opinion`: this is a transcription, where volunteering is the
      failure mode rather than the feature.

    The `not_a_string` rows were a **wart** until 2026-08-23: a capture that
    is neither a string nor bytes reached `len()` and raised an uncaught
    `TypeError`, so a plainly malformed request was a 500. It was the theme
    proposal's `float(budget)` bug in a second module, found by the Go port on
    the day that one was ruled and ruled with it. They are ordinary refusals
    now, and they are here so the fix cannot silently come undone.
    """
    import base64 as b64

    from mtglab.claude import modes as modes_mod
    from mtglab.claude import scan
    from mtglab.claude import stance as stance_mod

    good = b64.b64encode(b"\x89PNG\r\n\x1a\n" + b"x" * 40).decode()

    media_types = []
    for label in ["image/jpeg", "image/png", "image/webp", "image/gif",
                  "image/tiff", "image/jpg", "IMAGE/JPEG", "Image/Png",
                  " image/jpeg ", "image/jpeg;charset=utf-8", "jpeg", "",
                  "image/jpeg\n", "image/svg+xml", "application/pdf"]:
        row: dict[str, Any] = {"media_type": label}
        try:
            scan.message(good, label)
            row["ok"] = True
        except scan.ScanRefused as exc:
            row["ok"] = False
            row["error"] = str(exc)
        media_types.append(row)

    captures: list[dict[str, Any]] = []
    for note, image in [
        ("a real capture", good),
        ("the shortest legal capture", b64.b64encode(b"x").decode()),
        ("two bytes", b64.b64encode(b"xy").decode()),
        ("three bytes, no padding needed", b64.b64encode(b"xyz").decode()),
        ("empty string", ""),
        ("base64 of a single space", b64.b64encode(b" ").decode()),
        ("not base64 at all", "!!!!"),
        ("a newline inside", good[:8] + "\n" + good[8:]),
        ("a space inside", good[:8] + " " + good[8:]),
        ("a trailing newline", good + "\n"),
        ("unpadded", "YWJjZA"),
        ("over-padded", "YWJjZA==="),
        ("one padding char where two are needed", "YWJjZA="),
        ("the url-safe alphabet", "-_-_"),
        ("a non-zero trailing bit", "YW=="),
        ("lone padding", "="),
        ("only padding", "===="),
        ("a unicode digit that is not base64", "YWJ０"),
        ("a nul byte in the string", "YWJj\x00"),
    ]:
        row = {"note": note, "image": image}
        try:
            message = scan.message(image, "image/jpeg")
            row["data"] = message["content"][0]["source"]["data"]
        except scan.ScanRefused as exc:
            row["error"] = str(exc)
        captures.append(row)

    # A capture that is not text at all. `payload.get("image") or b""` in the
    # route means a falsy value never reaches here, so these are the shapes
    # that DO: a list, an object, a number, a true. All one refusal since the
    # 2026-08-23 ruling; all an uncaught TypeError before it.
    not_a_string: list[dict[str, Any]] = []
    for note, raw in [("a list", [1, 2, 3]), ("an object", {"a": 1}),
                      ("a number", 7), ("a float", 7.5), ("true", True)]:
        row = {"note": note, "raw": raw}
        try:
            scan.message(raw, "image/jpeg")
            row["ok"] = True
        except scan.ScanRefused as exc:
            row["error"] = str(exc)
        not_a_string.append(row)

    # The size gate, at the three points that matter. Recorded by LENGTH
    # rather than by payload: a 4MB base64 string in a corpus would be 5.6MB
    # of JSON for one assertion.
    sizes: list[dict[str, Any]] = []
    for note, size in [("one under the cap", scan.MAX_BYTES - 1),
                       ("exactly the cap", scan.MAX_BYTES),
                       ("one over the cap", scan.MAX_BYTES + 1),
                       ("well over the cap", scan.MAX_BYTES * 2)]:
        row = {"note": note, "bytes": size}
        try:
            scan.message(b"x" * size, "image/jpeg")
            row["ok"] = True
        except scan.ScanRefused as exc:
            row["ok"] = False
            row["error"] = str(exc)
        sizes.append(row)

    sightings = []
    for note, refused, text in [
        ("both fields", False, '{"title":"Birds of Paradise","corner":"196/302 R\\nDOM \\u2022 EN"}'),
        ("a title only", False, '{"title":"Bayou","corner":""}'),
        ("a corner only", False, '{"title":"","corner":"63/249 M"}'),
        ("neither", False, '{"title":"","corner":""}'),
        ("both whitespace", False, '{"title":"   ","corner":"\\t\\n"}'),
        ("padded values are stripped", False, '{"title":"  Bayou  ","corner":" X "}'),
        ("a non-breaking space is whitespace to str.strip", False,
         '{"title":"\\u00a0","corner":"\\u2028"}'),
        # **The four information separators**, U+001C-U+001F: whitespace to
        # Python's `str.strip()` and NOT to Go's `unicode.IsSpace`, which is
        # the one place `strings.TrimSpace` and `str.strip()` part company
        # below U+0021. Without this row a port that reached for TrimSpace
        # passes the whole corpus -- measured, after that mutation survived.
        ("the information separators are whitespace to str.strip only", False,
         '{"title":"\\u001cBayou\\u001f","corner":"\\u001d\\u001e"}'),
        ("a refused turn", True, '{"title":"Bayou","corner":"X"}'),
        ("empty text", False, ""),
        ("not json", False, "sorry, I could not read that"),
        ("truncated json", False, '{"title":"Bay'),
        ("a list, not an object", False, '["Bayou"]'),
        ("a bare string", False, '"Bayou"'),
        ("null", False, "null"),
        ("a number where a string belongs", False, '{"title":7,"corner":"X"}'),
        ("a null field", False, '{"title":null,"corner":"X"}'),
        ("a list field", False, '{"title":["Bayou"],"corner":"X"}'),
        ("an extra field is ignored", False, '{"title":"Bayou","corner":"X","name":"Bayou"}'),
    ]:
        turn = modes_mod.Turn(mode="scan", model="m", stop_reason="end_turn",
                              text=text, tool_calls=[], input_tokens=0,
                              output_tokens=0, refused=refused)
        sightings.append({"note": note, "refused": refused, "text": text,
                          "sighting": scan.sighting(turn)})

    stances = []
    for note, requested, ceiling in [
        ("nothing asked for is consultant, never off", None, None),
        ("nothing asked for, under a ceiling", None, "off"),
        ("a preset", "second-opinion", None),
        ("a preset over the ceiling", "collaborator", "consultant"),
        ("off is reachable", "off", None),
        ("a malformed stance", {"initiative": 7}, None),
        ("a stance that is not a stance", 7, None),
    ]:
        saved = os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
        if ceiling:
            os.environ["MTGLAB_CLAUDE_STANCE_CEILING"] = ceiling
        try:
            row = {"note": note, "requested": requested, "ceiling": ceiling}
            try:
                row["stance"] = stance_mod.describe(scan.stance_for(requested))
            except ValueError as exc:
                row["error"] = str(exc)
            stances.append(row)
        finally:
            os.environ.pop("MTGLAB_CLAUDE_STANCE_CEILING", None)
            if saved is not None:
                os.environ["MTGLAB_CLAUDE_STANCE_CEILING"] = saved

    return {
        "max_bytes": scan.MAX_BYTES,
        "default_preset": scan.DEFAULT_PRESET,
        "media_types": sorted(scan.MEDIA_TYPES),
        "media_type_cases": media_types,
        "captures": captures,
        "not_a_string": not_a_string,
        "sizes": sizes,
        "sightings": sightings,
        "stances": stances,
        # The ask that rides beside the picture, and the ORDER of the two
        # blocks: the image goes first, which is the documented ordering for
        # vision requests and the order the instruction reads in.
        "message": _scan_message_shape(),
    }


def _scan_message_shape() -> dict[str, Any]:
    """The one user message, as a shape rather than as bytes.

    The image's own `data` is dropped: it is 64 characters of test PNG and
    carries nothing, while its *position* is the claim being made.
    """
    import base64 as b64

    from mtglab.claude import scan

    message = scan.message(b64.b64encode(b"x" * 12).decode(), "image/webp")
    blocks = []
    for block in message["content"]:
        if block["type"] == "image":
            blocks.append({"type": "image",
                           "source_type": block["source"]["type"],
                           "media_type": block["source"]["media_type"]})
        else:
            blocks.append({"type": block["type"], "text": block["text"]})
    return {"role": message["role"], "blocks": blocks}


def render_scan_cases() -> str:
    return _rows_json(scan_cases()) + "\n"


# ------------------------------------------------------------ the Forge bridge
#
# `sim/tier3` ported to `go/internal/sim/tier3` (Phase 7). Most of what that
# package does is a JVM and a private network, which no corpus can hold and
# which the Go tests and a real local match do instead. What a corpus CAN hold
# is every pure transformation between Forge's text and the client's payload:
# the log parser, the `.dck` exporter, the coverage reading, the wire codec,
# and the shaped result the deck page renders.
#
# Two decisions about the shape of it.
#
# **Decks travel as deck.yaml text.** Go re-parses with `deck.FromText`, which
# is already held to Python by 2,051 render cases and 514 edit steps, so a
# failure here is a failure of *this* lane rather than of the parser under it.
#
# **The log fixtures are Forge's real format strings**, read out of
# `forge.view.SimulateMatch` the way `parse.py`'s docstring says, plus the
# pathological lines that only ever appeared once — the unsupported-card
# complaint that plays on, the slow-match warning that arrives before the
# result it belongs to, and the deck that would not load.

PYSTR_PATH = ROOT / "go" / "internal" / "wire" / "testdata" / "pystr.json"


def pystr_cases() -> list[dict[str, Any]]:
    """`str()` and `repr()` over a JSON-decoded value.

    Four route families coerce a body field with `str(payload.get(k) or "")`
    before it becomes a slug, and for every value a real client sends — a
    string — `str` is the identity. For the values a client does not send it is
    not, and the answer lands in a 404's `detail`, which the deck page renders
    verbatim: `str(["x"])` is `"['x']"` and Go's `fmt.Sprint` of the same
    decoded list is `"[x]"`.

    **Found by diffing the pair, live since Phase 5.** The cases are JSON
    *documents* rather than Python literals, so both sides decode the same
    bytes with their own decoder — which is the only way this checks the
    coercion rather than two hand-written value trees.
    """
    import json as _json

    documents = [
        ("a string", '"mono-green"'),
        ("an empty string", '""'),
        ("a string with a quote", '"it\'s"'),
        ("a string with both quotes", '"it\'s a \\"deck\\""'),
        ("a non-ascii string", '"B\u00f6sium Strip"'),
        ("null", "null"),
        ("true", "true"),
        ("false", "false"),
        ("an integer", "7"),
        ("a negative integer", "-7"),
        ("a huge integer", "1180591620717411303424"),
        # `1.0` is `1.0` to Python and `1` to a naive Go renderer.
        ("a whole float", "1.0"),
        ("a float", "1.5"),
        ("an exponent", "1e16"),
        ("a small exponent", "1e-05"),
        ("a list of strings", '["x"]'),
        ("a longer list", '["x","y"]'),
        ("an empty list", "[]"),
        ("a nested list", '[["x"],2]'),
        ("a list with a null and a bool", "[null,true,false]"),
        ("a list of numbers", "[1,1.0,-2]"),
        ("an object", '{"a":"x"}'),
        ("an empty object", "{}"),
        # **The one row where Go cannot agree**, and it is recorded as such
        # rather than left out: a JSON object decodes to a Go map, whose
        # iteration order is randomised, so the port sorts the keys where
        # Python keeps the document's. `sorted` says which answer Go gives, and
        # the Go test asserts THAT — so the limit is checked rather than
        # merely written down, and the day somebody decodes bodies through an
        # ordered map this row tells them to update it.
        ("an object whose key order Go cannot keep", '{"b":2,"a":1}'),
        ("an object inside a list", '[{"a":"x"}]'),
        ("a string with a newline", '"a\\nb"'),
    ]
    rows = []
    for note, document in documents:
        value = _json.loads(document)
        row = {"note": note, "document": document,
               "str": str(value), "repr": repr(value)}
        if isinstance(value, dict) and len(value) > 1:
            # What Go answers instead: the same rendering with the keys
            # sorted. Computed here so the two spellings sit side by side and
            # a reader can see exactly how far apart they are.
            row["go_sorts_to"] = repr(dict(sorted(value.items())))
        rows.append(row)
    return rows


def render_pystr_cases() -> str:
    return _rows_json(pystr_cases()) + "\n"


PYTEXT_PATH = ROOT / "go" / "internal" / "pytext" / "testdata" / "pytext.json"


def pytext_payload() -> dict[str, Any]:
    """CPython's whitespace and line-boundary tables, swept whole.

    **The whole of Unicode rather than the differences**, for the reason
    `pycasefold`'s table gives: recording only the code points where Python
    and Go disagree would leave the rest resting on Go's `unicode` tables
    agreeing with CPython's, which is a claim about two projects' Unicode
    versions rather than one this port gets to make.

    Two tables, and they are NOT the same set — which is the near-miss this
    package exists for. U+001C..U+001E are both whitespace and line
    boundaries; **U+001F is whitespace and not a boundary**; U+2028 and
    U+2029 are both. Get either direction wrong and a Forge log line is one
    game or none.

    Recorded as run-length spans rather than code points, because the answer
    is False almost everywhere and 1.1 million rows of it would be a corpus
    nobody could read.
    """
    spaces: list[list[int]] = []
    breaks: list[list[int]] = []

    def sweep(predicate: Any, into: list[list[int]]) -> None:
        start: int | None = None
        for cp in range(0x110000):
            # Surrogates are not characters, and Go cannot put one in a
            # string at all — so they are excluded on both sides rather than
            # asked about.
            hit = False if 0xD800 <= cp <= 0xDFFF else predicate(chr(cp))
            if hit and start is None:
                start = cp
            elif not hit and start is not None:
                into.append([start, cp - 1])
                start = None
        if start is not None:
            into.append([start, 0x10FFFF])

    sweep(lambda c: c.isspace(), spaces)
    # A character is a line boundary exactly when splitting a string on it
    # yields two pieces. Asked of the function rather than of a table, so the
    # corpus records what `str.splitlines()` DOES rather than what a document
    # says it does.
    sweep(lambda c: len(("a" + c + "b").splitlines()) == 2, breaks)

    strips = []
    for note, raw in [
        ("nothing to strip", "plain"),
        ("ascii space", "  padded  "),
        ("a tab and a newline", "\t\nx\n\t"),
        ("the information separators", "\u001cx\u001f"),
        ("a no-break space", "\u00a0x\u00a0"),
        ("a line separator", "\u2028x\u2029"),
        ("an ideographic space", "\u3000x\u3000"),
        ("all whitespace", " \t\n\u001f"),
        ("empty", ""),
        ("a zero-width space is NOT whitespace", "\u200bx\u200b"),
        ("a Mongolian vowel separator is NOT whitespace", "\u180ex\u180e"),
    ]:
        strips.append({"note": note, "text": raw, "stripped": raw.strip(),
                       "rstripped": raw.rstrip(),
                       "split_join": " ".join(raw.split())})

    splits = []
    for note, raw in [
        ("no boundary at all", "one line"),
        ("a trailing newline yields no empty tail", "a\nb\n"),
        ("two trailing newlines do", "a\nb\n\n"),
        ("CRLF is one boundary", "a\r\nb\r\n"),
        ("a bare CR is a boundary", "a\rb"),
        ("a lone LF after text", "a\nb"),
        ("a vertical tab", "a\vb"),
        ("a form feed", "a\fb"),
        ("the file separator", "a\u001cb"),
        ("the group separator", "a\u001db"),
        ("the record separator", "a\u001eb"),
        ("the UNIT separator is not a boundary", "a\u001fb"),
        ("NEL", "a\u0085b"),
        ("a line separator", "a\u2028b"),
        ("a paragraph separator", "a\u2029b"),
        ("empty", ""),
        ("one newline alone", "\n"),
        ("a mix", "a\r\nb\rc\nd\ve\ff\u0085g\u2028h"),
        # A multi-byte character either side of a boundary: a byte-wise split
        # would cut one in half.
        ("multibyte around a boundary", "B\u00f6sium\nD\u00e9j\u00e0"),
    ]:
        splits.append({"note": note, "text": raw, "lines": raw.splitlines()})

    heads = []
    for note, raw, n in [
        ("ascii, under", "abc", 5),
        ("ascii, exact", "abcde", 5),
        ("ascii, over", "abcdefgh", 5),
        # `len()` and slicing are CODE POINTS, not bytes: five accented
        # characters are five to Python and ten in UTF-8.
        ("accents count as one each", "\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9", 5),
        ("an emoji is one code point", "\U0001f600\U0001f600\U0001f600", 2),
        ("empty", "", 3),
    ]:
        heads.append({"note": note, "text": raw, "n": n,
                      "len": len(raw), "head": raw[:n]})

    return {"space_ranges": spaces, "break_ranges": breaks,
            "strips": strips, "splits": splits, "heads": heads}


def render_pytext_payload() -> str:
    return _rows_json(pytext_payload()) + "\n"


FORGE_PATH = ROOT / "go" / "internal" / "sim" / "tier3" / "testdata" / "forge.json"


def forge_log_cases() -> list[dict[str, Any]]:
    """Every shape a `forge sim -q` stream can take, parsed.

    Recorded as the WHOLE parse rather than as a game count, because the
    fields that separate a real draw from a clock-out and a winner from a
    seat are exactly the ones a careless port folds together.
    """
    from mtglab.sim.tier3 import parse

    texts: list[tuple[str, str]] = [
        ("a clean win",
         "Game Result: Game 1 ended in 5421 ms. Ai(2)-Atla Palani has won!\n"),
        ("a turn count arrives first",
         (
          "Game Outcome: Turn 11\n"
          "Game Result: Game 1 ended in 5421 ms. Ai(1)-Arahbo has won!\n"
         )),
        ("a real draw",
         "Game Result: Game 3 ended in a Draw! Took 900 ms.\n"),
        # The slow-match warning is printed BEFORE the result line it belongs
        # to, so a parser that forgot to hold it would file the clock-out
        # against the next game instead of this one.
        ("a clock-out wears a winner",
         ("Stopping slow match as draw\n"
          "Game Result: Game 2 ended in 300000 ms. Ai(1)-Tivit has won!\n")),
        ("a clock-out with no winner",
         (
          "Stopping slow match as draw\n"
          "Game Result: Game 2 ended in a Draw! Took 300000 ms.\n"
         )),
        ("the pending state does not leak into the next game",
         (
          "Game Outcome: Turn 7\n"
          "Stopping slow match as draw\n"
          "Game Result: Game 1 ended in 300000 ms. Ai(1)-A has won!\n"
          "Game Result: Game 2 ended in 4000 ms. Ai(2)-B has won!\n"
         )),
        # The experiment CLAUDE.md records: three unimplemented cards, and
        # then Forge played the game anyway and reported a winner.
        ("an unsupported card, and the game plays on",
         ('An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".\n'
          "Forge could not find this card in the Database. Please report it.\n"
          "Game Result: Game 1 ended in 7212 ms. Ai(2)-Goreclaw has won!\n")),
        ("the same complaint repeated per copy is one name",
         (
          'An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".\n'
          'An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".\n'
          'An unsupported card was requested: "Other Card" from "[N.A.]".\n'
         )),
        ("a deck that would not load",
         "Could not load deck - cat-tribal, match cannot start\n"),
        ("a label with no seat",
         "Game Result: Game 1 ended in 100 ms. Somebody has won!\n"),
        # A deck name is under the user's control and rides inside the winner
        # label, so the label's own alphabet is part of the contract.
        ("a deck name with an em dash and an accent",
         "Game Result: Game 1 ended in 100 ms. Ai(2)-Bösium — Strip has won!\n"),
        ("noise between results is ignored",
         (
          "[main] WARN forge.ai — could not evaluate\n"
          "Game Result: Game 1 ended in 42 ms. Ai(1)-A has won!\n"
          "Loading card database...\n"
         )),
        ("nothing at all", ""),
        ("only noise", "Loading card database...\nDone.\n"),
        # `str.splitlines()` splits on eight boundaries `split("\n")` does
        # not. A form feed inside a deck name splits the winner line in
        # Python and leaves it whole in Go — one game against none.
        ("a form feed splits a line for str.splitlines",
         ("Game Result: Game 1 ended in 100 ms. Ai(1)-A has won!\x0c"
          "Game Result: Game 2 ended in 200 ms. Ai(2)-B has won!\n")),
        ("a NEL splits a line too",
         (
          "Game Result: Game 1 ended in 100 ms. Ai(1)-A has won!\u0085"
          "Game Outcome: Turn 3\n"
          "Game Result: Game 2 ended in 200 ms. Ai(2)-B has won!\n"
         )),
        # A carriage return alone is a line boundary; CRLF is ONE boundary.
        ("CRLF is one boundary",
         ("Game Result: Game 1 ended in 100 ms. Ai(1)-A has won!\r\n"
          "Game Result: Game 2 ended in 200 ms. Ai(2)-B has won!\r\n")),
        # U+001F is whitespace to `strip` and NOT a line boundary, which is
        # the near-miss `pytext` exists for: get it wrong in either direction
        # and this row moves.
        ("a unit separator is stripped, never split",
         "\u001fGame Result: Game 1 ended in 100 ms. Ai(1)-A has won!\u001f\n"),
    ]

    rows: list[dict[str, Any]] = []
    for note, text in texts:
        output = parse.parse(text)
        rows.append({
            "note": note,
            "text": text,
            "trustworthy": output.trustworthy,
            "unsupported": output.unsupported,
            "deck_load_failures": output.deck_load_failures,
            "games": [
                {"index": g.index, "milliseconds": g.milliseconds,
                 "draw": g.draw, "winner": g.winner,
                 "winner_seat": g.winner_seat, "turns": g.turns,
                 "timed_out": g.timed_out}
                for g in output.games
            ],
            # The stateless predicate, asked of every line, because it is a
            # separate seam older tests pin and a port could get it right in
            # one place and wrong in the other.
            "is_game_result": [parse.is_game_result(line)
                               for line in text.splitlines()],
        })
    return rows


def forge_decks() -> dict[str, Any]:
    """The decks the exporter and the pre-flight are asked about.

    Small and hand-built rather than real: what is being checked is section
    order, quantity lines, sorting, the companion's place, and how a `A // B`
    name resolves — none of which wants a 99-card file to read through.
    """
    from mtglab.decks.model import CardEntry as Card
    from mtglab.decks.model import Deck

    def card(name: str, qty: int = 1) -> Card:
        return Card(name=name, category="utility", why="because", qty=qty)

    plain = Deck(slug="atla-palani", name="Atla Palani, Nest Tender - Dinos",
                 commander=["Atla Palani, Nest Tender"],
                 cards=[card("Sol Ring"), card("Forest", 36),
                        card("Ancient Bronze Dragon"), card("Birds of Paradise")])
    companion = Deck(slug="arahbo", name="Arahbo, Roar of the World - Cats",
                     commander=["Arahbo, Roar of the World"],
                     companion="Kaheera, the Orphanguard",
                     cards=[card("Plains", 20), card("Sol Ring")])
    partners = Deck(slug="partners", name="Two Commanders",
                    commander=["Thrasios, Triton Hero", "Tymna the Weaver"],
                    cards=[card("Island", 30)])
    # A modal DFC: Scryfall's combined name is what deck.yaml holds, and the
    # face name is the only thing Forge's index has.
    dfc = Deck(slug="dfc", name="Faces",
               commander=["Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"],
               cards=[card("Barkchannel Pathway // Tidechannel Pathway"),
                      card("Alive // Well"), card("Forest", 30)])
    # No commander at all, and a card repeated under two entries — the deck
    # the exporter's sort and the pre-flight's dedupe are asked about.
    odd = Deck(slug="odd", name="Odd",
               cards=[card("Zuran Orb"), card("Ancestral Recall"),
                      card("Zuran Orb", 2), card("Bösium Strip")])
    empty = Deck(slug="empty", name="Empty")
    # **The faces are tried IN ORDER, and a later one can be the answer.**
    # Every `A // B` above resolves on its front, so a `resolve` that tried
    # only the front passed the whole corpus -- measured, after that mutation
    # survived. These three separate it: a front the index lacks and a back it
    # has, a name where neither face is known, and a pair where BOTH are known
    # so front-first is observable rather than assumed.
    faces = Deck(slug="faces", name="Faces Tried In Order",
                 commander=["Atla Palani, Nest Tender"],
                 cards=[card("Unprinted Front // Tidechannel Pathway"),
                        card("Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"),
                        card("Nothing Here // Nor Here"),
                        card("Forest", 30)])
    return {"plain": plain, "companion": companion, "partners": partners,
            "dfc": dfc, "odd": odd, "empty": empty, "faces": faces}


#: What a hand-built index knows, standing in for Forge's 34,532 names. Face
#: names only, which is the fact the exporter depends on: a modal DFC
#: contributes two entries and never the combined `A // B`.
FORGE_INDEX = [
    "Atla Palani, Nest Tender", "Sol Ring", "Forest", "Plains", "Island",
    "Ancient Bronze Dragon", "Birds of Paradise",
    "Arahbo, Roar of the World", "Kaheera, the Orphanguard",
    "Thrasios, Triton Hero", "Tymna the Weaver",
    "Ajani, Nacatl Pariah", "Ajani, Nacatl Avenger",
    "Barkchannel Pathway", "Tidechannel Pathway", "Alive", "Well",
    "Zuran Orb", "Bösium Strip",
]


def forge_dck_cases() -> list[dict[str, Any]]:
    """Each deck as `.dck` text, byte for byte, resolved and unresolved."""
    from mtglab.sim.tier3 import coverage, dck

    index = frozenset(FORGE_INDEX)
    rows: list[dict[str, Any]] = []
    for name, deck in forge_decks().items():
        report = coverage.check(deck, index)
        rows.append({
            "note": name,
            "deck": deck.dump(),
            # Unresolved: every name written as deck.yaml holds it, which is
            # what a caller with no Forge install gets.
            "bare": dck.to_dck(deck),
            # Resolved: exactly what the pre-flight verified, which is the
            # property that makes a clean report mean something.
            "resolved": dck.to_dck(deck, report.resolved),
        })
    return rows


def forge_coverage_cases() -> list[dict[str, Any]]:
    """The pre-flight's reading of each deck, and the sentence it says."""
    from mtglab.sim.tier3 import coverage
    from mtglab.sim.tier3.run import CoverageFailed, raise_unless_covered

    index = frozenset(FORGE_INDEX)
    rows: list[dict[str, Any]] = []
    for name, deck in forge_decks().items():
        report = coverage.check(deck, index)
        row = {
            "note": name,
            "deck": deck.dump(),
            "slug": report.slug,
            "checked": report.checked,
            "resolved": report.resolved,
            "missing": report.missing,
            "ok": report.ok,
            "renamed": [list(pair) for pair in report.renamed],
            "summary": report.summary(),
        }
        try:
            raise_unless_covered([report])
            row["refusal"] = None
        except CoverageFailed as exc:
            row["refusal"] = str(exc)
        rows.append(row)

    # A deck Forge cannot fully play, which is the whole reason the pre-flight
    # exists — and the two-deck refusal, because the message joins them.
    from mtglab.decks.model import CardEntry as Card
    from mtglab.decks.model import Deck

    broken = Deck(slug="broken", name="Broken",
                  commander=["Atla Palani, Nest Tender"],
                  cards=[Card(name="Nonexistent Card 1", category="utility",
                              why="x", qty=1),
                         Card(name="Sol Ring", category="ramp", why="x", qty=1),
                         Card(name="Nonexistent Card 2", category="utility",
                              why="x", qty=1)])
    alsobroken = Deck(slug="also-broken", name="Also Broken",
                      commander=["Nobody At All"],
                      cards=[Card(name="Forest", category="land", why="x", qty=1)])
    reports = [coverage.check(d, index) for d in (broken, alsobroken)]
    try:
        raise_unless_covered(reports)
        joined = None
    except CoverageFailed as exc:
        joined = str(exc)
    rows.append({
        "note": "two broken decks, one message",
        "deck": broken.dump(),
        "second_deck": alsobroken.dump(),
        "slug": reports[0].slug,
        "checked": reports[0].checked,
        "resolved": reports[0].resolved,
        "missing": reports[0].missing,
        "ok": reports[0].ok,
        "renamed": [list(p) for p in reports[0].renamed],
        "summary": reports[0].summary(),
        "second_summary": reports[1].summary(),
        "refusal": joined,
    })
    return rows


def forge_wire_cases() -> dict[str, Any]:
    """The seam, in both directions, as the bytes that actually cross."""
    from mtglab.sim.tier3 import coverage, parse, wire
    from mtglab.sim.tier3.run import SimRun

    index = frozenset(FORGE_INDEX)
    decks = forge_decks()
    pair = [decks["plain"], decks["companion"]]

    reports = [coverage.check(d, index) for d in pair]
    games = [
        parse.GameResult(index=1, milliseconds=5421, winner="Ai(1)-Atla",
                         winner_seat=1, turns=11),
        parse.GameResult(index=2, milliseconds=900, draw=True, turns=4),
        parse.GameResult(index=3, milliseconds=300000, winner="Ai(2)-Arahbo",
                         winner_seat=2, timed_out=True),
    ]
    run = SimRun(argv=["java", "-jar", "forge.jar"],
                 output=parse.SimOutput(games=games),
                 wall_seconds=61.5, seats={1: "atla-palani", 2: "arahbo"},
                 coverage=reports, forge_version="2.0.14")
    rebuilt = wire.run_from_wire(wire.run_to_wire(run))
    return {
        "decks": wire.decks_to_wire(pair),
        "reports": wire.reports_to_wire(reports),
        "games": [wire.game_to_wire(g) for g in games],
        "run": wire.run_to_wire(run),
        # The bytes, not just the shape: key order is the contract on a wire
        # a Python shim and a Go app may be on opposite ends of.
        "run_json": json.dumps(wire.run_to_wire(run)),
        "game_json": [json.dumps(wire.game_to_wire(g)) for g in games],
        # A run rebuilt from the wire computes the same derived numbers.
        "rebuilt_startup_seconds": rebuilt.startup_seconds,
        "rebuilt_seats": {str(k): v for k, v in rebuilt.seats.items()},
        "rebuilt_wall_seconds": rebuilt.wall_seconds,
        # Deploy skew: a shim from before the field omits it, and the app must
        # read that as "not reported" rather than as an error.
        "old_shim_run": wire.run_to_wire(
            SimRun(argv=[], output=parse.SimOutput(games=games[:1]),
                   wall_seconds=1.0, seats={1: "a", 2: "b"})),
        # **A whole-second wall clock**, recorded as bytes: Python writes
        # `1.0` and `encoding/json` writes `1` for the same float64, and a
        # round trip that only decodes would never see it. This row is what
        # `pyfloat.Float` on the wire exists for.
        "old_shim_run_json": json.dumps(wire.run_to_wire(
            SimRun(argv=[], output=parse.SimOutput(games=games[:1]),
                   wall_seconds=1.0, seats={1: "a", 2: "b"}))),
    }


def forge_shape_cases() -> dict[str, Any]:
    """The payload the deck page renders, and the two dials that make it."""
    from mtglab.api import forgeruns
    from mtglab.sim.tier3 import parse, wire
    from mtglab.sim.tier3.run import SimRun

    decks = forge_decks()
    pair = [decks["plain"], decks["companion"]]
    addresses = ["local/atla-palani", "local/arahbo"]

    def run_of(games: list[Any], wall: float,
               seats: dict[int, str] | None = None) -> SimRun:
        return SimRun(argv=[], output=parse.SimOutput(games=games),
                      wall_seconds=wall,
                      seats=seats or {1: "atla-palani", 2: "arahbo"},
                      forge_version="2.0.14")

    def game(index: int, ms: int, **kw: Any) -> parse.GameResult:
        return parse.GameResult(index=index, milliseconds=ms, **kw)

    shapes: list[dict[str, Any]] = []
    for note, games, wall, seats, decks_used, addr in [
        ("an ordinary match",
         [game(1, 5421, winner="Ai(1)-x", winner_seat=1, turns=11),
          game(2, 4000, winner="Ai(2)-y", winner_seat=2, turns=9),
          game(3, 6800, winner="Ai(1)-x", winner_seat=1, turns=12)],
         30.5, None, pair, addresses),
        # An even count: the median is the mean of the two middle values, not
        # a middle element — the arithmetic a port most often gets wrong.
        ("an even number of games",
         [game(1, 1000, winner="Ai(1)-x", winner_seat=1),
          game(2, 2000, winner="Ai(2)-y", winner_seat=2),
          game(3, 3000, draw=True),
          game(4, 4000, winner="Ai(1)-x", winner_seat=1)],
         20.0, None, pair, addresses),
        # A clock-out counts for NOBODY even though Forge printed a winner.
        ("a clock-out wearing a trophy",
         [game(1, 300000, winner="Ai(1)-x", winner_seat=1, timed_out=True),
          game(2, 5000, winner="Ai(2)-y", winner_seat=2)],
         310.0, None, pair, addresses),
        ("a real draw is not a clock-out",
         [game(1, 900, draw=True, turns=4)], 10.0, None, pair, addresses),
        ("no games at all", [], 3.0, None, pair, addresses),
        # Half-millisecond rounding: `round(x, 1)` is half-to-EVEN, so 1050ms
        # is 1.0 and 1150ms is 1.2. A port reaching for `math.Round` answers
        # 1.1 and 1.2, and only one of those rows separates them.
        ("rounding at the half",
         [game(1, 1050, winner="Ai(1)-x", winner_seat=1),
          game(2, 1150, winner="Ai(2)-y", winner_seat=2),
          game(3, 2250, draw=True),
          game(4, 2350, draw=True)],
         12.35, None, pair, addresses),
        # **The self-match wart**: `wins` is keyed on the slug, so one deck
        # played against itself collapses two seats into one counter and both
        # lines report the total.
        ("a deck played against itself",
         [game(1, 1000, winner="Ai(1)-x", winner_seat=1),
          game(2, 1000, winner="Ai(2)-x", winner_seat=2)],
         5.0, {1: "atla-palani", 2: "atla-palani"},
         [decks["plain"], decks["plain"]],
         ["local/atla-palani", "local/atla-palani"]),
        # A winner seat nothing maps: `seats.get` answers None and the row
        # reports no winner rather than raising.
        ("a seat the run does not name",
         [game(1, 1000, winner="Ai(9)-?", winner_seat=9)],
         5.0, None, pair, addresses),
    ]:
        run = run_of(games, wall, seats)
        shapes.append({
            "note": note,
            # The INPUT, in the wire codec both runtimes already agree on, so
            # the Go side rebuilds the exact run rather than reverse-
            # engineering one out of the answer. A test that has to invert its
            # own subject is a test that will agree with a wrong inversion.
            "run": wire.run_to_wire(run),
            "slugs": [d.slug for d in decks_used],
            "addresses": addr,
            "games_asked": 10,
            "seed": 7,
            "shape": forgeruns._shape(decks_used, addr, 10, 7, run),
        })

    # `_row` alone, at the moment a tick carries it — the same builder, and a
    # theater that showed one shape live and another in the tally would be the
    # drift the wire codec exists to prevent.
    rows = []
    for note, g, slug in [
        ("a win", game(1, 5421, winner="Ai(1)-x", winner_seat=1, turns=11), "atla-palani"),
        ("a draw", game(2, 900, draw=True, turns=4), None),
        ("a clock-out keeps its slug out of the row",
         game(3, 300000, winner="Ai(1)-x", winner_seat=1, timed_out=True), "atla-palani"),
        ("a clock-out that Forge called a draw",
         game(4, 300000, draw=True, timed_out=True), None),
        ("no turn count", game(5, 100, winner="Ai(2)-y", winner_seat=2), "arahbo"),
    ]:
        rows.append({"note": note, "game": wire.game_to_wire(g),
                     "slug": slug, "row": forgeruns._row(g, slug)})

    games_dial = []
    for note, payload in [
        ("absent", {}),
        ("a plain number", {"games": 3}),
        ("the default", {"games": forgeruns.GAMES_DEFAULT}),
        ("over the cap", {"games": 500}),
        ("exactly the cap", {"games": forgeruns.GAMES_MAX}),
        ("zero clamps up", {"games": 0}),
        ("negative clamps up", {"games": -7}),
        ("a string", {"games": "4"}),
        # Python's `int()` grammar, which is not `strconv.Atoi`.
        ("an underscore separator", {"games": "1_2"}),
        ("a fullwidth digit", {"games": "\uff15"}),
        ("padded", {"games": "  6  "}),
        ("a float truncates", {"games": 7.9}),
        ("a bool is an int", {"games": True}),
        ("null falls to the default", {"games": None}),
        ("not a number at all", {"games": "many"}),
        ("a list", {"games": [3]}),
    ]:
        row: dict[str, Any] = {"note": note, "payload": payload}
        try:
            row["games"] = forgeruns._games(payload)
        except (TypeError, ValueError) as exc:
            # **A wart, pinned**: `plan_forge` runs in the request with
            # nothing catching this, so it is an uncaught 500 rather than the
            # 422 it should be.
            row["raises"] = type(exc).__name__
        games_dial.append(row)

    seed_dial = []
    for note, payload in [
        ("absent", {}),
        ("null is the default", {"seed": None}),
        ("empty string is the default", {"seed": ""}),
        ("a number", {"seed": 12345}),
        ("a string", {"seed": "99"}),
        ("negative", {"seed": -1}),
        ("zero is a seed, not an absence", {"seed": 0}),
        # Python's integers are unbounded and the seed is echoed back, so a
        # value past 2**63 must not silently become a different number.
        ("past int64", {"seed": 1180591620717411303424}),
        ("a float truncates", {"seed": 7.9}),
        ("not a number", {"seed": "lucky"}),
    ]:
        row = {"note": note, "payload": payload}
        try:
            row["seed"] = str(forgeruns._seed(payload))
        except (TypeError, ValueError) as exc:
            row["raises"] = type(exc).__name__
        seed_dial.append(row)

    labels = []
    for slugs, games in [(["a", "b"], 10), (["a", "b"], 1),
                         (["atla-palani", "arahbo"], 20)]:
        labels.append({
            "slugs": slugs, "games": games,
            "label": (f"Forge: {' vs '.join(slugs)}, "
                      f"{games} game{'s' if games != 1 else ''}"),
        })

    keys = []
    for addr, games, seed in [(addresses, 10, 7),
                              (["alice/x", "bob/y"], 1, 0),
                              (addresses, 20, 1180591620717411303424)]:
        keys.append({"addresses": addr, "games": games, "seed": str(seed),
                     "key": "forge|" + "|".join(addr) + f"|{games}|{seed}"})

    return {
        "caveat": forgeruns.FORGE_CAVEAT,
        "clock": forgeruns.CLOCK,
        "games_default": forgeruns.GAMES_DEFAULT,
        "games_max": forgeruns.GAMES_MAX,
        "decks": {name: deck.dump() for name, deck in forge_decks().items()},
        "shapes": shapes,
        "rows": rows,
        "games_dial": games_dial,
        "seed_dial": seed_dial,
        "labels": labels,
        "keys": keys,
    }


def forge_cases() -> dict[str, Any]:
    return {
        "index": FORGE_INDEX,
        "logs": forge_log_cases(),
        "dck": forge_dck_cases(),
        "coverage": forge_coverage_cases(),
        "wire": forge_wire_cases(),
        "shape": forge_shape_cases(),
    }


def render_forge_cases() -> str:
    return _rows_json(forge_cases()) + "\n"


if __name__ == "__main__":
    write()
