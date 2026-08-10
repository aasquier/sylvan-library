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

from mtglab.cards.db import SCHEMA, _iter_cards, bulk_download_url, get_cards

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


if __name__ == "__main__":
    # No standalone runner here, unlike the older test modules. The lookup
    # tests take a pytest fixture, so calling them bare would report noise
    # rather than results.
    sys.exit("run these under pytest:  pytest tests/test_cards_db.py")
