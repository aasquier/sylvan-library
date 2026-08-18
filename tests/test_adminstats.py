"""The admin dashboard's numbers: what the four stats views actually report.

`tests/test_isolation.py` owns *who may reach* these routes — they are
classified `ADMIN` there, so the 403-for-non-admins sweep covers them without
this file repeating it. This file owns what they say once reached, and the
properties are mostly about honesty on a fresh box:

- an absent store is `null`, never `0` — a fresh instance has no pool, and
  "present and empty" is a different fact from "not there yet";
- the Claude view's caveat rides in the payload, because the token totals
  are a floor on the bill and any renderer of the numbers needs the sentence
  next to them;
- the activity view is counts all the way down — nothing in it names a deck,
  a question, or another person's job.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient

from mtglab import config
from mtglab.api import jobs
from mtglab.api.app import create_app
from mtglab.auth import db, users
from mtglab.claude import ledger

PASSWORD = "correct-horse-battery-staple"
OTHER = "a-different-long-passphrase"


@pytest.fixture
def client(tmp_path, monkeypatch):
    """Auth on, logged in as an admin, with a second ordinary account."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    # `decks_dir` is scoped as well as `data_dir`, and the first run of this
    # file is why: left at the default, the storage endpoint counted the nine
    # real deck directories of the machine the suite ran on (ADR 30 — decks
    # are live app data, and a checkout has none).
    with config.use_paths(data_dir=tmp_path / "data",
                          decks_dir=tmp_path / "decks"):
        connection = db.connect()
        try:
            users.create(connection, "root", password=PASSWORD,
                         email="root@example.com", is_admin=True)
            users.create(connection, "friend", password=OTHER,
                         email="friend@example.com")
        finally:
            connection.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as test_client:
            assert test_client.post(
                "/api/auth/login",
                json={"username": "root", "password": PASSWORD}
            ).status_code == 200
            yield test_client


def test_system_reports_the_process_and_the_volume(client):
    body = client.get("/api/admin/stats/system").json()

    assert body["process"]["bytes"] > 0
    # The label the page needs to render the number honestly: /proc gives a
    # current figure, the getrusage fallback a peak.
    assert body["process"]["kind"] in ("current", "peak")
    assert body["disk"]["total_bytes"] > 0
    assert body["disk"]["used_bytes"] + body["disk"]["free_bytes"] <= \
        body["disk"]["total_bytes"]
    # The volume the number describes is named, so the page never has to
    # guess which mount it is looking at.
    assert body["disk"]["path"] == str(config.DATA_DIR)


def test_storage_says_null_for_absent_and_sized_for_present(client):
    """The fresh-box distinction this endpoint exists to make."""
    body = client.get("/api/admin/stats/storage").json()

    # The scratch app.db genuinely exists — logging in wrote to it.
    assert body["app_db_bytes"] > 0
    # There is no card pool in a scratch data dir, and that is `null`, not
    # zero: a zero would claim an empty pool file exists.
    assert body["pool_bytes"] is None
    assert body["scryfall_bulk_bytes"] is None
    assert body["decks"]["count"] == 0
    assert body["decks"]["trashed"] == 0


def test_claude_totals_carry_their_caveat(client):
    """The numbers and the sentence travel together, Tier 1's rule."""
    empty = client.get("/api/admin/stats/claude").json()
    assert empty["windows"]["week"]["by_mode"] == []
    assert empty["windows"]["all"]["by_model"] == []
    assert empty["windows"]["all"]["estimated_usd"]["usd"] == 0
    assert "floor" in empty["caveat"]
    # The date a person last read the pricing page rides with the figure. A
    # number whose age is invisible is the wrong invoice the old no-price-table
    # comment warned about; one a reader can discount is not.
    assert empty["prices"]["checked"]

    ledger.record(mode="dossier", model="claude-sonnet-5",
                  stop_reason="end_turn", requests=3,
                  input_tokens=1000, output_tokens=200,
                  cache_read_tokens=400)

    body = client.get("/api/admin/stats/claude").json()
    week = {row["mode"]: row for row in body["windows"]["week"]["by_mode"]}
    assert week["dossier"]["conversations"] == 1
    assert week["dossier"]["input_tokens"] == 1000
    # A row recorded now is inside every window.
    assert {row["mode"] for row in body["windows"]["all"]["by_mode"]} == {"dossier"}


def test_the_two_axes_sum_to_the_same_totals(client):
    """Each is its own `GROUP BY`, not a pivot of the other — so a disagreement
    here means one of the two queries lost rows."""
    for mode in ("dossier", "research"):
        ledger.record(mode=mode, model="claude-sonnet-5",
                      stop_reason="end_turn", requests=2,
                      input_tokens=500, output_tokens=100,
                      cache_read_tokens=50)

    pane = client.get("/api/admin/stats/claude").json()["windows"]["all"]
    for field in ("conversations", "requests", "input_tokens",
                  "output_tokens", "cache_read_tokens"):
        assert (sum(r[field] for r in pane["by_mode"])
                == sum(r[field] for r in pane["by_model"])), field


def test_a_per_mode_row_says_various_rather_than_naming_one_model(client):
    """Grouping by mode aggregates the model, so the column cannot name one
    truthfully. `(various)` is the honest answer; whichever id SQLite happened
    to keep would look like a fact and be an accident."""
    for model in ("claude-sonnet-5", "claude-opus-5"):
        ledger.record(mode="dossier", model=model, stop_reason="end_turn",
                      requests=1, input_tokens=10, output_tokens=10,
                      cache_read_tokens=0)

    pane = client.get("/api/admin/stats/claude").json()["windows"]["all"]
    assert [r["model"] for r in pane["by_mode"]] == ["(various)"]
    assert {r["model"] for r in pane["by_model"]} == {"claude-sonnet-5",
                                                     "claude-opus-5"}
    assert {r["mode"] for r in pane["by_model"]} == {"(various)"}


def test_a_model_with_no_rate_is_counted_rather_than_priced_at_zero(client):
    """The ledger stores the served-by id, which can be one this build has
    never heard of. Charging it nothing without saying so would make the total
    read low and reassuring — which is worse than refusing to price it."""
    ledger.record(mode="dossier", model="claude-archaeopteryx-9",
                  stop_reason="end_turn", requests=1,
                  input_tokens=9_000_000, output_tokens=9_000_000,
                  cache_read_tokens=0)

    spend = (client.get("/api/admin/stats/claude").json()
             ["windows"]["all"]["estimated_usd"])
    assert spend["usd"] == 0
    assert spend["unpriced"] == 1
    assert spend["unpriced_models"] == ["claude-archaeopteryx-9"]
    assert spend["complete"] is False


def test_activity_counts_accounts_sessions_and_the_registry(client):
    body = client.get("/api/admin/stats/activity").json()

    # Two claimed accounts, both able to sign in.
    assert body["accounts"] == {"active": 2}
    # Exactly one session exists (root's own), and it has been seen today —
    # this very request authenticated with it.
    assert body["sessions"]["total"] == 1
    assert body["sessions"]["seen_day"] == 1
    assert body["sessions"]["seen_week"] >= body["sessions"]["seen_day"]
    assert body["deck_edits_by_day"] == []
    assert body["sim_cache_rows"] == 0
    assert body["jobs"] == {}


def test_activity_sees_the_job_registry_as_counts_only(client):
    """The census is statuses and integers — no label, no owner, no result.

    `jobs.get`/`all_jobs` scope to an owner because labels can name another
    person's deck; the dashboard's cross-owner view must stay too coarse to
    leak. This pins the shape, so a helpful field added later fails a test
    instead of an audit.
    """
    jobs.completed("sim", result={"answer": 42}, label="somebody's deck",
                   owner=99)
    try:
        body = client.get("/api/admin/stats/activity").json()
        assert body["jobs"] == {"done": 1}
        assert "somebody's deck" not in str(body)
    finally:
        jobs.clear()
