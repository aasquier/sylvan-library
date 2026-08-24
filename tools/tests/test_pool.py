"""The extracted pool reader: read-only, refuses absence, derives the crop.

`cardmotion/pool.py` is the one module in the toolbox that did not cross
verbatim from the app -- it is a stance (read-only, never create, never
migrate) carved out of a module that owned the whole pool lifecycle -- so
its promises get their own tests rather than riding on the derivation's.
"""

from __future__ import annotations

from pathlib import Path

import pytest

import tiny_pool
from cardmotion import pool


def test_connect_reads_but_never_writes(tmp_path: Path) -> None:
    """The whole contract: a picture pipeline may read card facts and may
    not acquire, grow, or repair a pool."""
    db_path = tiny_pool.build(tmp_path / "mtg.duckdb")
    con = pool.connect(db_path)
    try:
        row = con.execute(
            "SELECT name, artist FROM oracle_cards WHERE oracle_id = ?",
            [tiny_pool.GYOME[0]]).fetchone()
        assert row == ("Gyome, Master Chef", "Steve Prescott")
        with pytest.raises(Exception, match="read-only"):
            con.execute("INSERT INTO printings VALUES ('x', 'X', NULL, NULL)")
    finally:
        con.close()


def test_connect_refuses_an_absent_pool_by_name(tmp_path: Path) -> None:
    """The app's `connect()` would have *created* the file here, which for
    this toolbox turns a path typo into an empty pool; the refusal names the
    path and where a real pool comes from."""
    with pytest.raises(FileNotFoundError) as excinfo:
        pool.connect(tmp_path / "nowhere" / "mtg.duckdb")
    message = str(excinfo.value)
    assert "nowhere" in message
    assert "data refresh" in message


def test_printing_columns_answers_what_is_there(tmp_path: Path) -> None:
    con = pool.connect(tiny_pool.build(tmp_path / "mtg.duckdb"))
    try:
        columns = pool.printing_columns(con)
        assert "artist" in columns          # what resolve_subject asks
        assert "image_normal" in columns
        assert "no_such_column" not in columns
    finally:
        con.close()


def test_art_crop_from_derives_or_declines() -> None:
    # The shape the derivation relies on: one size segment swapped, once.
    assert pool.art_crop_from(
        "https://cards.scryfall.io/normal/front/9/9/p1.jpg?1") == \
        "https://cards.scryfall.io/art_crop/front/9/9/p1.jpg?1"
    # Anything else is None, never a guess -- None renders as the card.
    assert pool.art_crop_from(None) is None
    assert pool.art_crop_from("") is None
    assert pool.art_crop_from("https://cards.scryfall.io/large/x.jpg") is None
