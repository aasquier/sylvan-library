"""Bulk-file ingest: URL selection and streaming parse.

These pin the Scryfall format change that broke `mtglab data refresh`: the
index stopped publishing `download_uri` (a plain JSON array) and started
publishing `jsonl_download_uri` (gzipped JSONL). Both shapes must keep
loading, because a cached file downloaded before the change is still a
perfectly good pool.

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
    """A card pool downloaded before the format change must still load."""
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
# An in-memory database keeps this independent of the downloaded pool.
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
    import duckdb
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


# ------------------------------------------------------------- pool load
#
# `load_oracle` and `load_printings` build the pool every other number in
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
    from mtglab.cards.db import connect
    con = connect(tmp_path / "nested" / "mtg.duckdb")
    try:
        tables = {r[0] for r in con.execute("SHOW TABLES").fetchall()}
        assert {"oracle_cards", "printings", "price_history"} <= tables
    finally:
        con.close()


def test_load_oracle_ingests_and_is_queryable(tmp_path):
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
    from mtglab.cards.db import connect, load_oracle
    con = connect(tmp_path / "mtg.duckdb")
    try:
        path = _jsonl(tmp_path, ORACLE_FIXTURE)
        load_oracle(con, path)
        load_oracle(con, path)
        total = con.execute("SELECT count(*) FROM oracle_cards").fetchone()[0]
        assert total == 2, "a second refresh duplicated the pool"
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


# ------------------------------------------------- printed stats and faces
#
# Added with the power/toughness columns. The bug that motivated them is the
# one in `test_a_double_faced_card_is_not_a_free_spell`: Scryfall puts
# `mana_cost`, `power` and `toughness` on the *faces* of a double-faced card
# and not on the card, so reading only the top level recorded all 501 of them
# with a NULL cost -- which `parse_mana_cost` turns into `{0}`, and the Tier 1
# compiler then simulates as a free spell.

DFC_FIXTURE = [
    {"oracle_id": "id-etali",
     "name": "Etali, Primal Conqueror // Etali, Primal Sickness",
     "cmc": 7, "layout": "transform",
     "type_line": "Legendary Creature — Elder Dinosaur // Legendary Creature "
                  "— Phyrexian Elder Dinosaur",
     "color_identity": ["G", "R"], "legalities": {"commander": "legal"},
     "card_faces": [
         {"name": "Etali, Primal Conqueror", "mana_cost": "{5}{R}{R}",
          "type_line": "Legendary Creature — Elder Dinosaur",
          "oracle_text": "Exile cards from the top of each library.",
          "power": "7", "toughness": "7", "artist": "Ryan Pancoast"},
         {"name": "Etali, Primal Sickness", "mana_cost": "",
          "type_line": "Legendary Creature — Phyrexian Elder Dinosaur",
          "oracle_text": "Toxic 10.", "power": "11", "toughness": "11"},
     ]},
    {"oracle_id": "id-tithe", "name": "Smothering Tithe", "mana_cost": "{3}{W}",
     "cmc": 4, "type_line": "Enchantment", "oracle_text": "Pay {2} or else.",
     "color_identity": ["W"], "legalities": {"commander": "legal"},
     "layout": "normal", "game_changer": True, "artist": "Mark Behm"},
    {"oracle_id": "id-tarmo", "name": "Tarmogoyf", "mana_cost": "{1}{G}",
     "cmc": 2, "type_line": "Creature — Lhurgoyf", "oracle_text": "It grows.",
     "color_identity": ["G"], "legalities": {"commander": "legal"},
     "layout": "normal", "power": "*", "toughness": "1+*"},
]


@pytest.fixture
def stats_con(tmp_path):
    from mtglab.cards.db import connect, load_oracle
    con = connect(tmp_path / "mtg.duckdb")
    load_oracle(con, _jsonl(tmp_path, DFC_FIXTURE))
    yield con
    con.close()


def test_printed_stats_survive_the_ingest(stats_con):
    rec = get_cards(stats_con, ["Smothering Tithe"])["Smothering Tithe"]
    assert rec.power is None, "not a creature; absent, not zero"
    assert rec.artist == "Mark Behm"


def test_stats_are_strings_because_star_is_a_real_value(stats_con):
    """Coercing to an integer either throws on Tarmogoyf or invents a number.
    The pool reports what is printed on the card."""
    rec = get_cards(stats_con, ["Tarmogoyf"])["Tarmogoyf"]
    assert rec.power == "*"
    assert rec.toughness == "1+*"


def test_a_double_faced_card_is_not_a_free_spell(stats_con):
    """The regression path for the bug this column set was added alongside.

    Etali is a seven-drop whose cost lives on its front face. Before the
    fallback the pool stored NULL, `parse_mana_cost(None)` returned `{0}`,
    and Tier 1 cast it on turn one -- in two of the six real decks.
    """
    from mtglab.mana import parse_mana_cost
    rec = get_cards(stats_con, ["Etali, Primal Conqueror"])[
        "Etali, Primal Conqueror"]
    assert rec.mana_cost == "{5}{R}{R}"
    assert str(parse_mana_cost(rec.mana_cost)) != "{0}"


def test_a_double_faced_card_reports_its_front_faces_stats(stats_con):
    """The front face is the one you cast from hand. The back is still in
    `card_faces`, which is stored unchanged."""
    rec = get_cards(stats_con, ["Etali, Primal Conqueror"])[
        "Etali, Primal Conqueror"]
    assert (rec.power, rec.toughness) == ("7", "7"), "front, not the 11/11 back"
    assert rec.artist == "Ryan Pancoast"


def test_game_changer_is_read_off_the_card(stats_con):
    assert get_cards(stats_con, ["Smothering Tithe"])["Smothering Tithe"].game_changer
    assert not get_cards(stats_con, ["Tarmogoyf"])["Tarmogoyf"].game_changer


def test_an_absent_game_changer_flag_is_false_not_none(stats_con):
    """Scryfall omits it on some rows rather than sending false, and a missing
    flag must not read as a missing *answer* -- `bool(None)` is the right
    reading here and the ingest makes it explicit."""
    rec = get_cards(stats_con, ["Etali, Primal Conqueror"])[
        "Etali, Primal Conqueror"]
    assert rec.game_changer is False


# ------------------------------------------------------ old-schema tolerance

#: `oracle_cards` exactly as it was before the printed-stat columns landed.
#: Spelled out rather than produced by dropping columns from the current
#: schema, because that is what an existing database on somebody's disk
#: actually looks like -- and because DuckDB will not drop a column an index
#: depends on, which is itself a hint that migrating in place is not free.
OLD_SCHEMA = """
CREATE TABLE oracle_cards (
    oracle_id VARCHAR PRIMARY KEY, name VARCHAR, mana_cost VARCHAR,
    cmc DOUBLE, type_line VARCHAR, oracle_text VARCHAR, colors VARCHAR[],
    color_identity VARCHAR[], keywords VARCHAR[], produced_mana VARCHAR[],
    legalities JSON, layout VARCHAR, card_faces JSON, reserved BOOLEAN,
    edhrec_rank INTEGER, released_at DATE, set_code VARCHAR,
    scryfall_uri VARCHAR, image_normal VARCHAR, image_art_crop VARCHAR
);
"""


@pytest.fixture
def old_con():
    import duckdb
    c = duckdb.connect(":memory:")
    c.execute(OLD_SCHEMA)
    c.execute(
        "INSERT INTO oracle_cards (oracle_id, name, mana_cost, cmc, type_line, "
        "oracle_text, color_identity, legalities, reserved) "
        "VALUES ('id-gyome', 'Gyome, Master Chef', '{2}{B}{G}', 4, "
        "'Legendary Creature — Troll', '', ['B','G'], ?, false)",
        [json.dumps({"commander": "legal"})])
    yield c
    c.close()


def test_a_pool_without_the_new_columns_still_answers(old_con):
    """The API opens the pool read-only, so it cannot migrate itself. A
    fixed column list would turn this schema change into an immediate outage
    for every existing database rather than a prompt to re-ingest."""
    from mtglab.cards.db import oracle_columns
    assert "power" not in oracle_columns(old_con)

    got = get_cards(old_con, ["Gyome, Master Chef"])
    assert got["Gyome, Master Chef"].color_identity == frozenset("BG")
    assert got["Gyome, Master Chef"].power is None
    assert got["Gyome, Master Chef"].game_changer is False


def test_an_old_pool_reports_itself_stale(old_con):
    """So the app can say "re-ingest" rather than showing every creature as
    statless -- an all-NULL column reads exactly like "no card has power",
    which is the quiet wrong answer the column was added to prevent."""
    from mtglab.cards.db import pool_is_stale
    assert pool_is_stale(old_con) is True


def test_connect_migrates_an_old_database_in_place(tmp_path):
    """A writable handle can fix itself, and must: `CREATE TABLE IF NOT EXISTS`
    does nothing to a table that already exists."""
    import duckdb

    from mtglab.cards.db import connect, oracle_columns
    path = tmp_path / "old.duckdb"
    c = duckdb.connect(str(path))
    c.execute(OLD_SCHEMA)
    c.close()

    con = connect(path)
    try:
        assert {"power", "toughness", "game_changer", "artist"} <= oracle_columns(con)
    finally:
        con.close()


def test_a_current_pool_is_not_stale(stats_con):
    from mtglab.cards.db import pool_is_stale
    assert pool_is_stale(stats_con) is False


def test_an_empty_pool_is_not_stale(tmp_path):
    """Nothing to be wrong about, and `health()` already says it is missing."""
    from mtglab.cards.db import connect, pool_is_stale
    con = connect(tmp_path / "empty.duckdb")
    try:
        assert pool_is_stale(con) is False
    finally:
        con.close()


# -------------------------------------------------------- ingest, end to end

def _bulk(tmp: Path, cards, name="oracle.jsonl.gz") -> Path:
    text = "\n".join(json.dumps(c) for c in cards)
    return _write(tmp, name, text, gz=name.endswith(".gz"))


def test_load_oracle_skips_tokens_and_batches(tmp_path):
    from mtglab.cards import db
    con = db.connect(tmp_path / "t.duckdb")
    path = _bulk(tmp_path, [
        {"name": "Sol Ring", "oracle_id": "o1", "layout": "normal",
         "type_line": "Artifact", "legalities": {"commander": "legal"}},
        {"name": "Food", "layout": "token"},
        {"name": "Gyome, Master Chef", "oracle_id": "o2", "layout": "normal",
         "type_line": "Legendary Creature — Troll Chef",
         "legalities": {"commander": "legal"}},
    ])
    # batch=1 drives the mid-loop flush as well as the tail one.
    assert db.load_oracle(con, path, batch=1) == 2
    row = con.execute("SELECT count(*) FROM oracle_cards").fetchone()
    assert row[0] == 2
    con.close()


def test_load_printings_skips_digital_and_snapshots_prices(tmp_path):
    from mtglab.cards import db
    con = db.connect(tmp_path / "t.duckdb")
    path = _bulk(tmp_path, [
        {"id": "p1", "oracle_id": "o1", "name": "Sol Ring", "set": "c21",
         "prices": {"usd": "1.50", "usd_foil": None},
         "image_uris": {"normal": "https://img/normal/p1.jpg"}},
        {"id": "p2", "oracle_id": "o1", "name": "Sol Ring", "set": "arena",
         "digital": True},
        # A double-faced printing carries its images on the faces.
        {"id": "p3", "oracle_id": "o2", "name": "A // B", "set": "neo",
         "prices": {"usd": None},
         "card_faces": [{"image_uris": {"normal": "https://img/normal/p3.jpg"}}]},
    ], name="printings.jsonl")
    assert db.load_printings(con, path, batch=1) == 2

    img = con.execute("SELECT image_normal FROM printings WHERE id = 'p3'"
                      ).fetchone()
    assert img[0] == "https://img/normal/p3.jpg", \
        "a DFC's image comes off its front face"

    # The daily price snapshot: only priced printings land in history.
    assert db.snapshot_prices(con, on_date="2026-08-14") == 1
    con.close()


def test_a_snapshot_date_is_bound_and_not_interpolated(tmp_path):
    """`on_date` reaches DuckDB as a parameter, so a value cannot be syntax.

    Unreachable from a request today -- the only production caller passes no
    date -- but that is a property of the callers, and the next caller inherits
    whatever this function does. Interpolated, the payload below would close the
    date literal and drop the table; bound, it is a bad date and nothing more.
    """
    from mtglab.cards import db

    con = db.connect(tmp_path / "pool.duckdb")
    try:
        con.execute("INSERT INTO printings (id, oracle_id, name, price_usd) "
                    "VALUES ('p1', 'o1', 'Sol Ring', 1.5)")

        # Note what this does *not* assert: that the payload is rejected.
        # DuckDB's DATE cast reads the leading `2026-08-14` and ignores the rest,
        # so the call succeeds. That is the point -- the string is a value being
        # parsed, not syntax being executed. Interpolated, the same payload would
        # have closed the date literal and dropped the table.
        assert db.snapshot_prices(
            con, on_date="2026-08-14'); DROP TABLE printings; --") == 1
        assert con.execute("SELECT count(*) FROM printings").fetchone()[0] == 1
        assert con.execute(
            "SELECT count(*) FROM price_history WHERE snapshot_date = DATE "
            "'2026-08-14'").fetchone()[0] == 1

        # And the ordinary path still works, so the guard did not cost the
        # feature: a real date still writes a row under that date.
        assert db.snapshot_prices(con, on_date="2026-08-14") == 1
    finally:
        con.close()


def test_connect_readonly_opens_without_creating_or_migrating(tmp_path):
    """The API's handle: it must not create the file, and must not run DDL.

    Both halves matter. Creating it would mean a missing pool looks present and
    empty rather than absent, and `ALTER TABLE` on a read-only handle raises --
    which is why `oracle_columns()` exists and why `connect()` cannot serve the
    app. This is the function that let `api/service.py` stop importing `duckdb`
    itself, closing the one live exception to "DuckDB stays behind `cards/db.py`".
    """
    from mtglab.cards import db

    missing = tmp_path / "absent.duckdb"
    with pytest.raises(Exception):  # noqa: B017 -- duckdb's own IO error
        db.connect_readonly(missing)
    assert not missing.exists(), "a read-only open must not create the pool"

    db.connect(tmp_path / "pool.duckdb").close()
    con = db.connect_readonly(tmp_path / "pool.duckdb")
    try:
        assert con.execute("SELECT count(*) FROM oracle_cards").fetchone()[0] == 0
        with pytest.raises(Exception):  # noqa: B017 -- read-only refusal
            con.execute("ALTER TABLE oracle_cards ADD COLUMN x INTEGER")
    finally:
        con.close()


def test_download_bulk_writes_once_and_reuses_the_copy(tmp_path, monkeypatch):
    import urllib.request

    from mtglab.cards import db

    monkeypatch.setattr(db, "_fetch_json", lambda url: {"data": [
        {"type": "oracle_cards", "updated_at": "2026-08-14T09:00:00Z",
         "jsonl_download_uri": "https://data.scryfall.io/o.jsonl.gz"}]})

    class FakeResp:
        def __init__(self):
            self.chunks = [b"payload-bytes", b""]

        def read(self, n):
            return self.chunks.pop(0)

        def __enter__(self):
            return self

        def __exit__(self, *exc):
            return False

    fetched = []
    monkeypatch.setattr(urllib.request, "urlopen",
                        lambda req, timeout: fetched.append(1) or FakeResp())

    target = db.download_bulk("oracle_cards", dest_dir=tmp_path)
    assert target.name == "oracle_cards-2026-08-14.jsonl.gz"
    assert target.read_bytes() == b"payload-bytes"
    assert not list(tmp_path.glob("*.part")), "the temp file must be renamed"

    # Same stamp, second ask: the cached copy answers and nothing is fetched.
    again = db.download_bulk("oracle_cards", dest_dir=tmp_path)
    assert again == target
    assert len(fetched) == 1


def test_download_bulk_refuses_an_unknown_kind(monkeypatch):
    from mtglab.cards import db
    monkeypatch.setattr(db, "_fetch_json", lambda url: {"data": []})
    with pytest.raises(ValueError, match="unknown bulk type"):
        db.download_bulk("no_such_kind")


# --------------------------------------------------------------- the keeper


def test_the_keeper_makes_repeat_connections_cheap_and_lets_go(tmp_path,
                                                               monkeypatch):
    """`service._pin` holds the pool open, and stops holding it when idle.

    Both halves matter and only the first is an optimisation. DuckDB frees a
    database instance when the last connection to it closes, so an app that
    opens and closes per request reloads the pool every time -- but a keeper
    held forever takes a shared lock that `mtglab data refresh` can never get
    past. The lease is what makes the speed safe, so the release is pinned
    here rather than left to be noticed by a refresh that will not start.
    """
    import time

    import tiny_pool
    from mtglab import config
    from mtglab.api import service
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    with config.use_paths(data_dir=tmp_path, decks_dir=tmp_path / "decks"):
        # A read-write connection is available before anything has pinned.
        db.connect(pool).close()

        con = service._connect()
        assert con is not None
        con.close()
        assert service._KEEPER is not None, "a request should leave a keeper"

        # The keeper is what a second connection is cheap against; it is also
        # a claim on the file, so the read-write open a refresh needs is
        # refused while it is held. DuckDB words that refusal differently
        # depending on where the other handle is -- a lock message across
        # processes (`mtglab data refresh` beside a running `mtglab ui`), a
        # configuration-mismatch message within one -- so this matches the
        # fact rather than the sentence.
        with pytest.raises(Exception) as refused:
            db.connect(pool)
        assert "lock" in str(refused.value).lower() or \
               "configuration" in str(refused.value).lower()

        # A keeper still inside its lease is left alone -- the reaper must not
        # be handing the pool back between the four requests of one page load.
        assert service._reap_once() is False
        assert service._KEEPER is not None

        # Idle past the lease, and the reaper is what hands the pool back.
        # Driven through `_reap_once` rather than by calling the release
        # directly, so this fails if the lease stops firing and not merely if
        # the release stops working.
        monkeypatch.setattr(service, "_KEEPER_USED",
                            time.monotonic() - service._KEEPER_IDLE - 1)
        assert service._reap_once() is True
        assert service._KEEPER is None
        db.connect(pool).close()          # the refresh can proceed again


def test_the_keeper_is_dropped_when_the_pool_file_changes(tmp_path):
    """A re-ingested pool must not be served from a stale cached instance."""
    import tiny_pool
    from mtglab import config
    from mtglab.api import service

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    with config.use_paths(data_dir=tmp_path, decks_dir=tmp_path / "decks"):
        service._connect().close()
        first = service._KEEPER_STAMP
        assert first is not None

        with service._KEEPER_LOCK:        # release so the file can be rebuilt
            service._release_keeper()
        pool.unlink()
        tiny_pool.build(pool)

        service._connect().close()
        assert first != service._KEEPER_STAMP, "a rebuilt pool needs a new keeper"


def test_the_column_cache_survives_a_closed_connection(tmp_path):
    """`oracle_columns` asks the catalogue once per pool, not once per request.

    The first version of this cache was keyed on the connection object. It was
    correct and it never once hit: every endpoint in this app opens a handle,
    asks its card question and closes it, so the entry was written and then
    thrown away with the connection that owned it. Keyed on the pool file, the
    answer outlives the handle -- which is the only way a cache helps a
    per-request connection at all.

    Asserted by **identity**: a connection that re-read the catalogue would
    build a fresh `set`, so the same object coming back three times over three
    separate handles is the cache being used rather than merely present.
    """
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db._COLUMN_CACHE.clear()

    answers = []
    for _ in range(3):
        con = db.connect_readonly(pool)
        answers.append(db.oracle_columns(con))
        con.close()

    assert answers[0] is answers[1] is answers[2], (
        "each request re-read the catalogue; the cache is keyed on something "
        "that dies with the connection")
    assert len(db._COLUMN_CACHE) == 1


def test_a_rebuilt_pool_gets_its_columns_read_again(tmp_path):
    """The stamp is what makes caching the file safe. A `data refresh` adds
    columns, and a cache that answered from the old file would report a schema
    the database no longer has."""
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db._COLUMN_CACHE.clear()

    con = db.connect_readonly(pool)
    db.oracle_columns(con)
    con.close()
    assert len(db._COLUMN_CACHE) == 1

    pool.unlink()
    tiny_pool.build(pool)
    import os
    os.utime(pool, ns=(0, 0))

    con = db.connect_readonly(pool)
    db.oracle_columns(con)
    con.close()
    assert len(db._COLUMN_CACHE) == 2, "a rebuilt pool reused the old answer"


# ------------------------------------------------------- the lookup memo


def test_the_same_names_against_the_same_pool_are_asked_once(tmp_path):
    """`get_cards` memoises on the pool's stamp and the exact names.

    Asserted by identity of the records rather than by wall clock: a second
    query would build fresh `CardRecord`s, so the same objects coming back is
    the cache being used. The dict itself must *not* be the same object -- see
    the isolation test below.
    """
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    con = db.connect_readonly(pool)
    first = db.get_cards(con, ["Sol Ring", "Llanowar Elves"])
    second = db.get_cards(con, ["Sol Ring", "Llanowar Elves"])

    assert first == second
    assert first["Sol Ring"] is second["Sol Ring"], "the pool was asked twice"
    # Two results that were *both* served from the cache still must not be the
    # same dict, which is the comparison that catches handing the entry out.
    third = db.get_cards(con, ["Sol Ring", "Llanowar Elves"])
    assert second is not third, "callers must not share one dict"
    con.close()


def test_a_caller_cannot_corrupt_the_memo_through_its_own_result(tmp_path):
    """The dict is copied out; the records are frozen. Both halves matter."""
    import dataclasses

    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    con = db.connect_readonly(pool)
    names = ["Sol Ring", "Llanowar Elves"]

    # The **second** call is the one served from the cache, and it is the one
    # that has to be a copy. Popping from the first proves nothing: that result
    # was built by the query and was never the cached object. (Written the
    # weak way first, and a mutation that returned the cached dict straight out
    # passed it.)
    db.get_cards(con, names)
    served = db.get_cards(con, names)
    served.pop("Sol Ring")

    assert "Sol Ring" in db.get_cards(con, names)

    # And the record itself cannot be written to at all, which is what makes
    # handing the same object to two requests safe.
    with pytest.raises(dataclasses.FrozenInstanceError):
        db.get_cards(con, ["Sol Ring"])["Sol Ring"].name = "Not Sol Ring"
    con.close()


def test_a_rebuilt_pool_is_not_answered_from_the_old_one(tmp_path):
    """The stamp is the whole safety argument. A `data refresh` rewrites the
    file, and an answer cached against the old one must not survive it."""
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    con = db.connect_readonly(pool)
    before = db.get_cards(con, ["Sol Ring"])
    con.close()

    pool.unlink()
    tiny_pool.build(pool)
    import os
    os.utime(pool, ns=(0, 0))

    con = db.connect_readonly(pool)
    after = db.get_cards(con, ["Sol Ring"])
    con.close()

    assert after == before
    assert after["Sol Ring"] is not before["Sol Ring"], \
        "a rebuilt pool was answered from the previous file's cache"


def test_different_names_are_different_entries(tmp_path):
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    con = db.connect_readonly(pool)
    db.get_cards(con, ["Sol Ring"])
    db.get_cards(con, ["Sol Ring", "Llanowar Elves"])
    assert len(db._CARD_CACHE) == 2
    con.close()


def test_the_memo_is_bounded_and_evicts_the_least_recently_used(tmp_path):
    """A cache on a long-running instance has to have a ceiling."""
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    con = db.connect_readonly(pool)
    # One distinct name set per ask, more of them than the cache may hold.
    for i in range(db._CARD_CACHE_MAX + 6):
        db.get_cards(con, ["Sol Ring"] * (i + 1))
    assert len(db._CARD_CACHE) == db._CARD_CACHE_MAX

    # The most recent survives; the first is long gone.
    keys = list(db._CARD_CACHE)
    assert keys[-1][-1] == ("Sol Ring",) * (db._CARD_CACHE_MAX + 6)
    assert ("Sol Ring",) not in [k[-1] for k in keys]
    con.close()


def test_a_write_handle_is_never_memoised(tmp_path):
    """Only read-only handles carry a stamp.

    A stamp claims the file's contents follow from its mtime and size, which
    holds only for a handle that cannot write: an ingest inserts rows for as
    long as its transaction runs while the file on disk still looks untouched.
    Caching against a writer would serve rows from before its own inserts.
    """
    import tiny_pool
    from mtglab.cards import db

    pool = tmp_path / "mtg.duckdb"
    tiny_pool.build(pool)
    db.cache_clear()

    writer = db.connect(pool)
    db.get_cards(writer, ["Sol Ring"])
    db.get_cards(writer, ["Sol Ring"])
    assert len(db._CARD_CACHE) == 0, "a read-write handle was memoised"
    assert db._pool_stamp(writer) is None
    writer.close()
