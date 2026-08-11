"""Bulk-file ingest: URL selection and streaming parse.

These pin the Scryfall format change that broke `mtglab data refresh`: the
index stopped publishing `download_uri` (a plain JSON array) and started
publishing `jsonl_download_uri` (gzipped JSONL). Both shapes must keep
loading, because a cached file downloaded before the change is still a
perfectly good corpus.

Nothing here touches DuckDB or the network -- the parser is deliberately
separable from the database.
"""

import gzip
import json
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab.cards.db import (
    SCHEMA,
    CardRecord,
    _iter_cards,
    bulk_download_url,
    get_cards,
)

CARDS = [
    {"name": "Gyome, Master Chef", "color_identity": ["B", "G"],
     "legalities": {"commander": "legal"}},
    # Nested braces and a quoted brace, to exercise the array scanner's
    # depth tracking and string handling.
    {"name": "Krark-Clan Ironworks", "oracle_text": "Sacrifice an artifact: Add {C}{C}.",
     "card_faces": [{"name": "front"}]},
]


def _write(tmp: Path, name: str, text: str, *, gz: bool = False) -> Path:
    path = tmp / (name + (".gz" if gz else ""))
    data = text.encode("utf-8")
    path.write_bytes(gzip.compress(data) if gz else data)
    return path


# ------------------------------------------------------------------ url choice

def test_prefers_jsonl_download_uri():
    entry = {"type": "oracle_cards",
             "jsonl_download_uri": "https://data.scryfall.io/o.jsonl.gz",
             "download_uri": "https://data.scryfall.io/o.json"}
    assert bulk_download_url(entry) == "https://data.scryfall.io/o.jsonl.gz"


def test_falls_back_to_legacy_download_uri():
    entry = {"type": "oracle_cards", "download_uri": "https://data.scryfall.io/o.json"}
    assert bulk_download_url(entry) == "https://data.scryfall.io/o.json"


def test_missing_url_raises_something_actionable():
    """The original failure was a bare KeyError, which said nothing useful."""
    with pytest.raises(ValueError) as exc:
        bulk_download_url({"type": "oracle_cards", "uri": "https://api/x"})
    assert "oracle_cards" in str(exc.value)


# --------------------------------------------------------------------- parsing

def test_reads_plain_jsonl():
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.jsonl",
                      "\n".join(json.dumps(c) for c in CARDS) + "\n")
        assert [c["name"] for c in _iter_cards(path)] == [c["name"] for c in CARDS]


def test_reads_gzipped_jsonl():
    """The current Scryfall format."""
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.jsonl",
                      "\n".join(json.dumps(c) for c in CARDS) + "\n", gz=True)
        got = list(_iter_cards(path))
        assert [c["name"] for c in got] == [c["name"] for c in CARDS]
        assert got[0]["color_identity"] == ["B", "G"]


def test_reads_legacy_json_array():
    """A corpus downloaded before the format change must still load."""
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.json", json.dumps(CARDS))
        assert [c["name"] for c in _iter_cards(path)] == [c["name"] for c in CARDS]


def test_reads_gzipped_json_array():
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.json", json.dumps(CARDS), gz=True)
        assert [c["name"] for c in _iter_cards(path)] == [c["name"] for c in CARDS]


def test_array_scanner_survives_braces_inside_strings():
    """Mana symbols in oracle text are literal braces. Naive depth counting
    would treat '{C}' as an object and desynchronise the whole file."""
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.json", json.dumps(CARDS))
        got = list(_iter_cards(path))
    assert len(got) == 2
    assert got[1]["oracle_text"].endswith("Add {C}{C}.")
    assert got[1]["card_faces"] == [{"name": "front"}]


def test_jsonl_ignores_blank_lines_and_trailing_newline():
    with tempfile.TemporaryDirectory() as tmp:
        body = json.dumps(CARDS[0]) + "\n\n" + json.dumps(CARDS[1]) + "\n\n"
        path = _write(Path(tmp), "o.jsonl", body)
        assert len(list(_iter_cards(path))) == 2


def test_leading_whitespace_does_not_confuse_format_detection():
    with tempfile.TemporaryDirectory() as tmp:
        path = _write(Path(tmp), "o.json", "\n  " + json.dumps(CARDS))
        assert len(list(_iter_cards(path))) == 2


def test_empty_file_yields_nothing_rather_than_raising():
    with tempfile.TemporaryDirectory() as tmp:
        assert list(_iter_cards(_write(Path(tmp), "o.jsonl", ""))) == []


# --------------------------------------------------------------- name lookup

# Real shapes, because the bug was entirely about how Scryfall names faces.
# An in-memory database keeps this independent of the downloaded corpus.
ROWS = [
    ("id-gyome", "Gyome, Master Chef", "{2}{B}{G}", "Legendary Creature — Troll Warlock",
     ["B", "G"]),
    ("id-path", "Darkbore Pathway // Slitherbore Pathway", None, "Land // Land",
     ["B", "G"]),
    ("id-virtue", "Virtue of Persistence // Locthwain Scorn", "{6}{B}{B}",
     "Enchantment // Sorcery — Adventure", ["B"]),
    # The card CLAUDE.md names: white front, red back, so identity is R/W.
    ("id-ajani", "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger", "{1}{W}",
     "Legendary Creature — Cat Warrior // Legendary Planeswalker — Ajani", ["R", "W"]),
]


@pytest.fixture
def con():
    duckdb = pytest.importorskip("duckdb")
    c = duckdb.connect(":memory:")
    c.execute(SCHEMA)
    for oid, name, cost, type_line, identity in ROWS:
        c.execute(
            "INSERT INTO oracle_cards (oracle_id, name, mana_cost, cmc, type_line, "
            "oracle_text, color_identity, legalities, reserved) "
            "VALUES (?, ?, ?, 0, ?, '', ?, ?, false)",
            [oid, name, cost, type_line, identity, json.dumps({"commander": "legal"})])
    yield c
    c.close()


def test_exact_full_name_still_resolves(con):
    got = get_cards(con, ["Gyome, Master Chef"])
    assert got["Gyome, Master Chef"].color_identity == frozenset("BG")


def test_modal_dfc_resolves_by_front_face(con):
    """Moxfield writes 'Darkbore Pathway'; Scryfall stores 'A // B'."""
    got = get_cards(con, ["Darkbore Pathway"])
    assert "Darkbore Pathway" in got
    assert got["Darkbore Pathway"].is_land


def test_adventure_resolves_by_front_face(con):
    got = get_cards(con, ["Virtue of Persistence"])
    assert got["Virtue of Persistence"].color_identity == frozenset({"B"})


def test_back_face_also_resolves(con):
    got = get_cards(con, ["Slitherbore Pathway"])
    assert "Slitherbore Pathway" in got


def test_face_lookup_reports_the_whole_card_identity(con):
    """The Ajani case. Looking the card up by its white front face must still
    report {R}{W}, or the gate would wave an illegal card into a G/W deck."""
    got = get_cards(con, ["Ajani, Nacatl Pariah"])
    assert got["Ajani, Nacatl Pariah"].color_identity == frozenset({"R", "W"})


def test_unknown_name_is_still_absent(con):
    """Resolving faces must not turn a typo into a silent match."""
    assert get_cards(con, ["Darkbore Pathwya"]) == {}


def test_mixed_batch_keeps_every_requested_key(con):
    got = get_cards(con, ["Gyome, Master Chef", "Darkbore Pathway", "Nonesuch"])
    assert set(got) == {"Gyome, Master Chef", "Darkbore Pathway"}


# ------------------------------------------------------------------ is_land
#
# Scryfall reports one combined type line for a double-faced card, so
# `"Land" in type_line` was true for every card whose BACK face is a land.
# Tier 1 uses is_land to decide what a land is, so Trostani simulated with 37
# lands instead of 35 and Atla with 37 instead of 36 -- the same class of
# silent, confident, wrong answer as the qty and tapland bugs.

def _rec(type_line, layout="normal"):
    return CardRecord(
        name="x", mana_cost=None, cmc=0.0, type_line=type_line, oracle_text="",
        color_identity=frozenset(), produced_mana=(), legal_commander=True,
        reserved=False, edhrec_rank=None, image_normal=None, layout=layout,
    )


@pytest.mark.parametrize("type_line", [
    "Legendary Land",                          # Boseiju, Who Endures
    "Basic Land — Forest",
    "Land Creature — Forest Dryad",            # Dryad Arbor
    "Land // Land",                            # Branchloft Pathway
])
def test_front_face_land_is_a_land(type_line):
    assert _rec(type_line).is_land


@pytest.mark.parametrize("type_line", [
    "Legendary Creature — God // Land",             # Ojer Taq
    "Legendary Enchantment // Legendary Land",      # Growing Rites of Itlimoc
    "Enchantment — Saga // Legendary Land",         # Welcome to . . .
])
def test_transforming_permanent_with_a_land_back_is_not_a_land(type_line):
    """You cast the front face. The back only ever arrives by flipping
    something already on the battlefield, so it is never a land drop."""
    assert not _rec(type_line, layout="transform").is_land


@pytest.mark.parametrize("type_line", [
    "Instant // Land",     # Sink into Stupor, Kabira Takedown
    "Sorcery // Land",     # Stump Stomp
])
def test_modal_dfc_with_a_land_back_is_a_land(type_line):
    """A modal DFC lets you choose which face to play, so the land back face
    is a real land drop and Tier 1 should count it."""
    assert _rec(type_line, layout="modal_dfc").is_land


def test_battle_with_a_creature_back_is_not_a_land():
    """Invasion of Ikoria -- neither face is a land, and it must not become one."""
    assert not _rec(
        "Battle — Siege // Legendary Creature — Dinosaur", layout="transform").is_land


# ------------------------------------------------------------- corpus load
#
# `load_oracle` and `load_printings` build the corpus every other number in
# the project is derived from, and they need no network -- they take a file.
# Leaving them untested meant a silent ingest bug would surface as wrong
# simulation output rather than a failing test.

ORACLE_FIXTURE = [
    {"oracle_id": "id-1", "name": "Ley Weaver", "mana_cost": "{3}{G}", "cmc": 4,
     "type_line": "Creature — Human Druid", "oracle_text": "Partner with Lore Weaver",
     "colors": ["G"], "color_identity": ["G"], "keywords": [],
     "produced_mana": [], "legalities": {"commander": "legal"},
     "layout": "normal", "reserved": False, "edhrec_rank": 900,
     "released_at": "2018-06-08", "set": "bbd",
     "image_uris": {"normal": "https://img/normal.jpg",
                    "art_crop": "https://img/art.jpg"}},
    {"oracle_id": "id-2", "name": "Ojer Taq, Deepest Foundation // Temple",
     "mana_cost": "{3}{W}{W}", "cmc": 5,
     "type_line": "Legendary Creature — God // Land", "oracle_text": "",
     "colors": ["W"], "color_identity": ["W"], "keywords": [],
     "produced_mana": [], "legalities": {"commander": "legal"},
     "layout": "transform", "reserved": True, "edhrec_rank": None,
     "released_at": "2023-11-17", "set": "lci"},
]


def _jsonl(tmp: Path, rows) -> Path:
    path = tmp / "cards.jsonl"
    path.write_text("\n".join(json.dumps(r) for r in rows), encoding="utf-8")
    return path


def test_connect_creates_the_schema_on_a_fresh_file(tmp_path):
    """A first run has no database at all; connect() must build one rather
    than fail on a missing table."""
    pytest.importorskip("duckdb")
    from mtglab.cards.db import connect
    con = connect(tmp_path / "nested" / "mtg.duckdb")
    try:
        tables = {r[0] for r in con.execute("SHOW TABLES").fetchall()}
        assert {"oracle_cards", "printings", "price_history"} <= tables
    finally:
        con.close()


def test_load_oracle_ingests_and_is_queryable(tmp_path):
    pytest.importorskip("duckdb")
    from mtglab.cards.db import connect, load_oracle
    con = connect(tmp_path / "mtg.duckdb")
    try:
        n = load_oracle(con, _jsonl(tmp_path, ORACLE_FIXTURE))
        assert n == 2
        got = get_cards(con, ["Ley Weaver"])
        rec = got["Ley Weaver"]
        assert rec.color_identity == frozenset({"G"})
        assert rec.legal_commander
        assert rec.image_art_crop == "https://img/art.jpg"
    finally:
        con.close()


def test_load_oracle_preserves_layout_so_is_land_stays_correct(tmp_path):
    """The regression path for the 37-vs-35 land bug: if layout is dropped on
    ingest, a transforming permanent with a land back reads as a land again."""
    pytest.importorskip("duckdb")
    from mtglab.cards.db import connect, load_oracle
    con = connect(tmp_path / "mtg.duckdb")
    try:
        load_oracle(con, _jsonl(tmp_path, ORACLE_FIXTURE))
        rec = get_cards(con, ["Ojer Taq, Deepest Foundation"])[
            "Ojer Taq, Deepest Foundation"]
        assert rec.layout == "transform"
        assert not rec.is_land
        assert rec.reserved is True
    finally:
        con.close()


def test_load_oracle_is_idempotent(tmp_path):
    """`data refresh` is run repeatedly; loading twice must not double rows."""
    pytest.importorskip("duckdb")
    from mtglab.cards.db import connect, load_oracle
    con = connect(tmp_path / "mtg.duckdb")
    try:
        path = _jsonl(tmp_path, ORACLE_FIXTURE)
        load_oracle(con, path)
        load_oracle(con, path)
        total = con.execute("SELECT count(*) FROM oracle_cards").fetchone()[0]
        assert total == 2, "a second refresh duplicated the corpus"
    finally:
        con.close()


def test_layout_round_trips_from_the_database(con):
    """is_land depends on layout, so layout has to survive the query -- a NULL
    column must fall back to 'normal' rather than crashing or reading falsey."""
    con.execute(
        "INSERT INTO oracle_cards (oracle_id, name, mana_cost, cmc, type_line, "
        "oracle_text, color_identity, legalities, reserved, layout) "
        "VALUES ('ot', 'Ojer Taq, Deepest Foundation', '{3}{W}{W}', 5, "
        "'Legendary Creature — God // Land', '', ['W'], ?, false, 'transform')",
        [json.dumps({"commander": "legal"})])
    got = get_cards(con, ["Ojer Taq, Deepest Foundation"])
    rec = got["Ojer Taq, Deepest Foundation"]
    assert rec.layout == "transform"
    assert not rec.is_land
    # A row loaded before this column was selected still reports sanely.
    assert get_cards(con, ["Gyome, Master Chef"])["Gyome, Master Chef"].layout == "normal"


if __name__ == "__main__":
    # No standalone runner here, unlike the older test modules. The lookup
    # tests take a pytest fixture, so calling them bare would report noise
    # rather than results.
    sys.exit("run these under pytest:  pytest tests/test_cards_db.py")
