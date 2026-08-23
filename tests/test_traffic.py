"""The visitor ledger: what it records, and — the real test — what it cannot.

The design is the privacy stance (schema v9, `api/traffic.py`), so most of
this file pins absences:

- the table has exactly four columns, so a helpful fifth one fails a test
  instead of an audit;
- a matched route lands as its **template**, never the concrete path a
  person typed — a path can carry a slug and a slug can carry a person;
- everything no route matched shares one bucket, pre-routing refusals
  included, because recording which door was tried is recording the path;
- the flush never raises and a broken database costs counts, not requests.
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
from mtglab.api import jobs, traffic
from mtglab.api.app import create_app
from mtglab.auth import db, users

PASSWORD = "correct-horse-battery-staple"


@pytest.fixture
def real_ledger(monkeypatch):
    """Undo the suite's global stub — this file is about the real thing.

    `conftest._no_request_counts` no-ops `record` and `flush` for every
    test, because the counter was the third writer found resolving its
    database from ambient `config`. Here the ambient config *is* a scratch
    directory, so the real implementations run.
    """
    monkeypatch.setattr(traffic, "record", traffic._REAL_RECORD)
    monkeypatch.setattr(traffic, "flush", traffic._REAL_FLUSH)


@pytest.fixture
def client(tmp_path, monkeypatch, real_ledger):
    """Auth on, an admin signed in, and an empty buffer either side."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    traffic.reset()
    with config.use_paths(data_dir=tmp_path / "data",
                          decks_dir=tmp_path / "decks"):
        connection = db.connect()
        try:
            users.create(connection, "root", password=PASSWORD,
                         email="root@example.com", is_admin=True)
        finally:
            connection.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as test_client:
            assert test_client.post(
                "/api/auth/login",
                json={"username": "root", "password": PASSWORD}
            ).status_code == 200
            yield test_client
    traffic.reset()


def rows(con) -> list[tuple]:
    return con.execute(
        "SELECT day, route, status_class, count FROM request_log"
        " ORDER BY route, status_class").fetchall()


def test_the_table_has_four_columns_and_no_helpful_fifth(client):
    """The audit, as a test. IP, user agent, username, concrete path — the
    way any of them arrives is a new column, and this fails on it."""
    with db.connection() as con:
        names = {row[1] for row in
                 con.execute("PRAGMA table_info(request_log)").fetchall()}
    assert names == {"day", "route", "status_class", "count"}


def test_a_matched_route_is_recorded_as_its_template(client):
    # A deck that does not exist: the route matches, the handler 404s, and
    # what lands in the ledger must be the template — the concrete path
    # carries an owner and a slug, which is exactly what is not kept.
    assert client.get("/api/decks/root/somebodys-deck").status_code == 404
    traffic.flush()

    with db.connection() as con:
        recorded = {route for _, route, _, _ in rows(con)}
    assert "/api/decks/{owner}/{slug}" in recorded
    assert not any("somebodys-deck" in route for route in recorded)


def test_a_refusal_before_routing_shares_the_unrouted_bucket(client):
    fresh = TestClient(client.app)
    # No session: the middleware refuses before routing (ADR 17), so there
    # is no template to record and the concrete path must not stand in.
    assert fresh.get("/api/decks/alice/private-deck").status_code == 401
    traffic.flush()

    with db.connection() as con:
        recorded = rows(con)
    unrouted = [r for r in recorded if r[1] == traffic.UNROUTED]
    assert unrouted and unrouted[0][2] == "4xx"
    assert not any("private-deck" in route for _, route, _, _ in recorded)


def test_counts_accumulate_across_flushes(client):
    assert client.get("/api/health").status_code == 200
    traffic.flush()
    assert client.get("/api/health").status_code == 200
    traffic.flush()

    with db.connection() as con:
        health = [r for r in rows(con) if r[1] == "/api/health"]
    assert len(health) == 1
    assert health[0][2] == "2xx" and health[0][3] >= 2


def test_the_traffic_view_reports_days_and_top_routes(client):
    for _ in range(3):
        assert client.get("/api/health").status_code == 200

    body = client.get("/api/admin/stats/traffic").json()

    assert body["days"], "the ledger saw requests and reported no days"
    today = body["days"][-1]
    assert today["total"] >= today.get("2xx", 0) > 0
    top = {row["route"] for row in body["top_routes"]}
    assert "/api/health" in top
    # The sentence travels with the numbers, the Claude view's rule.
    assert "never records" in body["note"]


def test_every_day_carries_every_bucket_even_at_zero(client):
    """A day with no failures still reports `4xx: 0` and `5xx: 0`.

    The keys used to appear only when the class did, so a quiet day simply had
    no `5xx` and the chart's series vanished for it. **Found by the Go
    migration**: flipping `/api/sim/forge` removed the last route that made
    this process answer a 5xx during a contract run, and the recorded wire
    shape lost a key -- which is the honest reading of a payload whose keys
    depend on its data. Nothing here fails 4xx or 5xx, which is exactly the
    case that used to be short.
    """
    for _ in range(3):
        assert client.get("/api/health").status_code == 200

    body = client.get("/api/admin/stats/traffic").json()
    assert body["days"], "the ledger saw requests and reported no days"
    for day in body["days"]:
        assert set(day) == {"day", "total", *traffic.STATUS_CLASSES}, day
        assert day["total"] == sum(day[cls] for cls in traffic.STATUS_CLASSES)


def test_a_refused_request_lands_in_its_own_bucket(client):
    """And the buckets are still counted, not merely present."""
    assert client.get("/api/decks/local/nope-not-a-deck").status_code == 404
    body = client.get("/api/admin/stats/traffic").json()
    today = body["days"][-1]
    assert today["4xx"] > 0, today
    assert today["5xx"] == 0, today


def test_a_broken_database_costs_counts_not_requests(client, tmp_path):
    """`_write` swallows and warns; the request that triggered the flush
    (and every one after it) must keep answering."""
    with config.use_paths(data_dir=tmp_path / "somewhere-else"):
        # `app.db` here is a directory, so opening it as a database fails —
        # and it exists, so the never-mint skip does not save the write.
        (config.APP_DB_PATH).mkdir(parents=True)
        traffic.record("/api/health", 200)
        traffic.flush()   # must not raise
    assert client.get("/api/health").status_code == 200


def test_the_ledger_never_mints_a_database(real_ledger, tmp_path):
    """A bare instance counting requests must not acquire an `app.db`.

    `tests/test_library_write_gate.py` pins this property for the app as a
    whole; this is the counter's own share of it, exercised directly: the
    deferred write drops counts for a database nothing else has created,
    because a request counter is not the thing that gets to run the
    migration ladder on a fresh laptop.
    """
    traffic.reset()
    with config.use_paths(data_dir=tmp_path / "bare"):
        traffic.record("/api/health", 200)
        traffic.flush()
        assert not config.APP_DB_PATH.exists()
    traffic.reset()
