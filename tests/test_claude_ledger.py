"""The usage ledger: one row per conversation, and the roll-up over them.

Everything here runs against a scratch `app.db` named explicitly by path --
never the config-derived default, which in a test process would be the
developer's real data directory. The autouse fixture in conftest.py protects
the rest of the suite from that; these tests protect themselves by always
passing `path=`.
"""

import logging
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.claude import ledger


def _record(db, mode="argue", **overrides):
    row = {"mode": mode, "model": "claude-sonnet-5", "stop_reason": "end_turn",
           "requests": 2, "input_tokens": 100, "output_tokens": 50,
           "cache_read_tokens": 25}
    row.update(overrides)
    ledger.record(path=db, **row)


def test_record_then_summary_roundtrip(tmp_path):
    db = tmp_path / "app.db"
    _record(db)
    _record(db, input_tokens=200, output_tokens=10, cache_read_tokens=0)
    _record(db, mode="commander-dossier", requests=8,
            input_tokens=5000, output_tokens=800)

    rows = ledger.summary(path=db)
    # Most expensive mode first, which is the order a cost question reads in.
    assert [r["mode"] for r in rows] == ["commander-dossier", "argue"]

    argue = rows[1]
    assert argue["conversations"] == 2
    assert argue["requests"] == 4
    assert argue["input_tokens"] == 300
    assert argue["output_tokens"] == 60
    assert argue["cache_read_tokens"] == 25
    assert argue["first_at"] <= argue["last_at"]


def test_summary_since_cuts_on_created_at(tmp_path):
    """`since` is a string compared against ISO-8601 UTC timestamps, which is
    date comparison for free -- pinned so a future format change to
    `created_at` fails here rather than silently filtering nothing."""
    from mtglab.auth import db as auth_db

    db = tmp_path / "app.db"
    _record(db, mode="old")
    _record(db, mode="new")
    with auth_db.connection(db) as con, con:
        con.execute("UPDATE claude_usage SET created_at = "
                    "'2020-01-01T00:00:00+00:00' WHERE mode = 'old'")

    rows = ledger.summary(since="2025-01-01T00:00:00+00:00", path=db)
    assert [r["mode"] for r in rows] == ["new"]
    assert len(ledger.summary(path=db)) == 2, "no filter means everything"


def test_an_empty_ledger_summarises_to_nothing(tmp_path):
    assert ledger.summary(path=tmp_path / "app.db") == []


def test_record_never_raises(tmp_path, caplog):
    """Accounting that can fail a paid four-minute dossier run is worse than
    no accounting -- a broken database is a logged warning, not an exception
    riding up through `converse`."""
    unopenable = tmp_path / "a-directory"
    unopenable.mkdir()
    with caplog.at_level(logging.WARNING, logger="mtglab.claude.ledger"):
        _record(unopenable)
    assert "claude usage record failed" in caplog.text
