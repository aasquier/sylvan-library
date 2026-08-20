"""The match ledger (ADR 36): the writer, the readers, and the rules.

Everything here runs against a scratch `app.db` handed in as an explicit
`path`, the way `test_deck_log.py` exercises its writer — the conftest
detector watches the developer's real database either way. What is under
test is the contract the ADR states: `record` never raises, games are stored
as parsed, `recent` is the reference reading that applies the clock-out
rule, and seats snapshot the labels the deck wore when it played.
"""

import logging
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab.decks.model import CardEntry, Deck
from mtglab.sim.tier3 import ledger, parse
from mtglab.sim.tier3.run import SimRun


def _deck(slug: str, archetype: str = "", themes: list[str] | None = None) -> Deck:
    return Deck(slug=slug, name=f"Deck {slug}",
                commander=["Gyome, Master Chef"],
                archetype=archetype, themes=themes or [],
                cards=[CardEntry(name="Sol Ring", category="ramp", why="x")])


def _run() -> SimRun:
    """Three games exercising every edge the rows must keep apart.

    Game 1 is the parse edge that keeps mattering: a slow-match warning
    followed by a winner line, so `timed_out` and `winner_seat` are both set
    and any tally that counts it for seat 1 has quoted the measurement's
    surrender as a trophy.
    """
    games = [
        parse.GameResult(index=1, milliseconds=300_000, winner="Ai(1)-x",
                         winner_seat=1, timed_out=True),
        parse.GameResult(index=2, milliseconds=8_000, draw=True, turns=14),
        parse.GameResult(index=3, milliseconds=5_100, winner="Ai(2)-x",
                         winner_seat=2, turns=9),
    ]
    return SimRun(argv=["java"], output=parse.SimOutput(games=games),
                  wall_seconds=70.0, seats={1: "deck-a", 2: "deck-b"},
                  forge_version="2.0.14")


def test_the_migration_builds_the_three_tables(tmp_path):
    from mtglab.auth import db

    with db.connection(tmp_path / "app.db") as con:
        assert con.execute("PRAGMA user_version").fetchone()[0] == \
            db.SCHEMA_VERSION
        names = {r[0] for r in con.execute(
            "SELECT name FROM sqlite_master WHERE type = 'table'").fetchall()}
    assert {"forge_matches", "forge_seats", "forge_games"} <= names


def test_a_match_is_recorded_and_read_back(tmp_path):
    db = tmp_path / "app.db"
    decks = [_deck("deck-a", "midrange", ["food", "aristocrats"]),
             _deck("deck-b", "aggro", ["cats"])]
    match_id = ledger.record(_run(), decks, seed=42, clock=300,
                             games_requested=3, hosted=True, path=db)
    assert match_id is not None

    [match] = ledger.recent(path=db)
    assert match["id"] == match_id
    assert match["seed"] == 42
    assert match["clock"] == 300
    assert match["games_requested"] == 3
    assert match["forge_version"] == "2.0.14"
    assert match["hosted"] is True
    assert match["played"] == 3

    a, b = match["seats"]
    assert (a["slug"], a["archetype"], a["themes"]) == \
        ("deck-a", "midrange", ["food", "aristocrats"])
    assert (b["slug"], b["archetype"], b["themes"]) == \
        ("deck-b", "aggro", ["cats"])
    assert a["commander"] == ["Gyome, Master Chef"]
    assert a["owner_id"] is None and b["owner_id"] is None


def test_the_reference_reading_applies_the_clock_out_rule(tmp_path):
    """Game 1 timed out with a winner line: it counts for nobody, it is not a
    real draw, and it is reported apart -- the tally `forgeruns._shape`
    already computes, now promised by the ledger's own reader."""
    db = tmp_path / "app.db"
    ledger.record(_run(), [_deck("deck-a"), _deck("deck-b")], seed=1,
                  clock=300, games_requested=3, hosted=False, path=db)

    [match] = ledger.recent(path=db)
    a, b = match["seats"]
    assert a["wins"] == 0          # the clocked-out "win" counts for nobody
    assert b["wins"] == 1
    assert match["draws"] == 1     # the real draw only
    assert match["timed_out"] == 1


def test_games_store_facts_as_parsed_not_verdicts(tmp_path):
    """The raw row keeps the winner line Forge printed after giving up --
    readers apply the rule, so the rule can be re-examined without the data
    having pre-judged it (ADR 36 decision 3)."""
    from mtglab.auth import db as auth_db

    db = tmp_path / "app.db"
    ledger.record(_run(), [_deck("deck-a"), _deck("deck-b")], seed=1,
                  clock=300, games_requested=3, hosted=False, path=db)
    with auth_db.connection(db) as con:
        row = con.execute(
            "SELECT winner_seat, timed_out FROM forge_games"
            " WHERE game_index = 1").fetchone()
    assert (row["winner_seat"], row["timed_out"]) == (1, 1)


def test_an_unseeded_match_records_null_not_a_number(tmp_path):
    db = tmp_path / "app.db"
    ledger.record(_run(), [_deck("deck-a"), _deck("deck-b")], seed=None,
                  clock=300, games_requested=3, hosted=False, path=db)
    [match] = ledger.recent(path=db)
    assert match["seed"] is None


def test_record_never_raises(tmp_path, caplog):
    """The match is already played and paid for; a ledger failure is a
    warning, exactly as in `decks/log.py`."""
    broken = tmp_path / "not-a-database"
    broken.write_text("this is not SQLite", encoding="utf-8")
    with caplog.at_level(logging.WARNING):
        result = ledger.record(_run(), [_deck("a"), _deck("b")], seed=1,
                               clock=300, games_requested=3, hosted=False,
                               path=broken)
    assert result is None
    assert "match ledger record failed" in caplog.text


def test_a_malformed_run_is_a_warning_not_a_crash(tmp_path, caplog):
    """Broader than the deck log's catch, deliberately: this writer is handed
    a run object, and a caller holding minutes of finished JVM work must not
    lose it to a shape mismatch."""
    with caplog.at_level(logging.WARNING):
        result = ledger.record(object(), [_deck("a"), _deck("b")],  # type: ignore[arg-type]
                               seed=1, clock=300, games_requested=3,
                               hosted=False, path=tmp_path / "app.db")
    assert result is None
    assert "match ledger record failed" in caplog.text


def test_reading_does_raise(tmp_path):
    """Unlike `record`: a silently empty history for a ledger that has rows
    would be worse than an error."""
    import sqlite3

    broken = tmp_path / "not-a-database"
    broken.write_text("this is not SQLite", encoding="utf-8")
    with pytest.raises(sqlite3.Error):
        ledger.recent(path=broken)


def test_deleting_an_account_takes_its_seats(tmp_path):
    """`owner_id` cascades like the activity log's: a deleted account's slugs
    are its content, and the match drops out of per-deck queries because the
    JOIN finds nothing -- the intended afterlife, checked so it stays one."""
    from mtglab.auth import db as auth_db

    db = tmp_path / "app.db"
    with auth_db.connection(db) as con, con:
        con.execute("INSERT INTO users (id, username, created_at)"
                    " VALUES (7, 'bob', '2026-01-01T00:00:00+00:00')")
    ledger.record(_run(), [_deck("deck-a"), _deck("deck-b")], seed=1,
                  clock=300, games_requested=3, hosted=False,
                  owner_ids=[None, 7], path=db)

    with auth_db.connection(db) as con, con:
        con.execute("DELETE FROM users WHERE id = 7")

    [match] = ledger.recent(path=db)
    assert [s["slug"] for s in match["seats"]] == ["deck-a"]
    assert match["played"] == 3    # the games remain; the seat is gone
