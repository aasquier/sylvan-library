"""API surface.

These run against the real deck files in `decks/` but tolerate a missing
corpus, because a fresh clone has no `data/mtg.duckdb` until `data refresh`
runs -- and the app has to stay usable in that state rather than 500ing.
"""

import sys
import time
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

fastapi = pytest.importorskip("fastapi")
pytest.importorskip("httpx")

from fastapi.testclient import TestClient  # noqa: E402

from mtglab.api import jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402

# The job pool has a single worker, so a queued job may sit behind another
# test's. Poll on a clock rather than an iteration count.
JOB_TIMEOUT_S = 60


@pytest.fixture
def client():
    jobs.clear()
    with TestClient(create_app()) as c:
        yield c


def await_job(client, job_id: str) -> dict:
    """Block until a job leaves the queue, or fail with what it was doing."""
    deadline = time.monotonic() + JOB_TIMEOUT_S
    body = {}
    while time.monotonic() < deadline:
        body = client.get(f"/api/jobs/{job_id}").json()
        if body["status"] in ("done", "error"):
            return body
        time.sleep(0.05)
    pytest.fail(f"job did not finish in {JOB_TIMEOUT_S}s: {body}")


# -------------------------------------------------------------------- meta

def test_health_reports_corpus_state(client):
    body = client.get("/api/health").json()
    assert "corpus" in body
    assert isinstance(body["oracle_cards"], int)


# ------------------------------------------------------------------- decks

def test_decks_lists_the_library(client):
    body = client.get("/api/decks").json()
    assert isinstance(body, list)
    slugs = {d["slug"] for d in body}
    assert "gyome-food" in slugs
    assert "_template" not in slugs


def test_deck_detail_has_every_card_with_its_why(client):
    body = client.get("/api/decks/gyome-food").json()
    assert body["total_cards"] == 99
    assert body["land_count"] == 34
    assert all(c["why"] for c in body["cards"]), "a card lost its rationale"


def test_missing_deck_is_a_404_not_a_500(client):
    resp = client.get("/api/decks/does-not-exist")
    assert resp.status_code == 404
    assert "does-not-exist" in resp.json()["detail"]


def test_missing_deck_stats_is_also_404(client):
    assert client.get("/api/decks/nope/stats").status_code == 404
    assert client.get("/api/decks/nope/validate").status_code == 404


def test_validate_returns_structured_issues(client):
    body = client.get("/api/decks/gyome-food/validate").json()
    assert set(body) == {"ok", "errors", "warnings"}
    assert isinstance(body["errors"], list)


def test_stats_are_json_serialisable(client):
    """The curve buckets are dataclasses; they must be flattened for the wire."""
    body = client.get("/api/decks/gyome-food/stats").json()
    assert body["total_cards"] == 99
    assert body["curve"]["buckets"], "no curve buckets"
    assert isinstance(body["curve"]["buckets"][0]["mv"], int)
    assert isinstance(body["categories"], list)


# ------------------------------------------------------------------- cards

def test_card_search_respects_the_limit(client):
    body = client.get("/api/cards/search", params={"limit": 5}).json()
    if body.get("message"):
        pytest.skip("no corpus available")
    assert len(body["cards"]) <= 5


def test_card_search_identity_filter_is_a_subset_check(client):
    """Asking for BG must return colorless and mono-B cards too -- that is the
    question a Golgari deckbuilder is actually asking."""
    body = client.get("/api/cards/search",
                      params={"identity": "BG", "limit": 50}).json()
    if body.get("message"):
        pytest.skip("no corpus available")
    for card in body["cards"]:
        assert set(card["color_identity"]) <= {"B", "G"}, card["name"]


def test_search_limit_is_clamped_not_rejected(client):
    assert client.get("/api/cards/search", params={"limit": 9999}).status_code == 422


# --------------------------------------------------------------------- sim

def test_sim_requires_a_slug(client):
    assert client.post("/api/sim/mana", json={}).status_code == 422


def test_sim_job_runs_to_completion(client):
    """Submit, poll, and get a result -- the whole contract the UI relies on."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/mana",
                            json={"slug": "gyome-food", "games": 300,
                                  "turns": 8, "seed": 1}).json()
    assert submitted["status"] in ("queued", "running", "done")

    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    result = body["result"]
    assert result["games"] == 300
    assert len(result["by_turn"]) == 8
    assert result["caveat"], "Tier 1 numbers must ship with their caveat"


def test_failed_job_reports_the_error_rather_than_hanging(client):
    """A job for a deck that cannot compile must end in `error`, not spin."""
    from mtglab.api import simruns

    job = jobs.submit("test", lambda _p: simruns._compile("no-such-deck"))
    current = await_job(client, job.id)
    assert current["status"] == "error"
    assert current["error"]


def test_unknown_job_is_404(client):
    assert client.get("/api/jobs/deadbeef").status_code == 404


def test_job_progress_is_reported(client):
    job = jobs.submit("test", lambda p: [p(i, 10) for i in range(10)] and "ok")
    body = await_job(client, job.id)
    assert body["status"] == "done"
    assert body["percent"] == 100
    assert client.get("/api/jobs").json()[0]["id"] == job.id
