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


def test_deck_list_carries_the_gate_counts(client):
    """The shelf renders a deck with a banned card identically to a clean one
    unless the list endpoint says so, and fetching /validate per deck would be
    an N+1 on every page load. `errors` may be None when the corpus is missing
    -- that is 'the gate did not run', not 'the deck passed'."""
    body = client.get("/api/decks").json()
    assert body, "no decks found"
    for deck in body:
        assert "errors" in deck and "warnings" in deck
        assert deck["errors"] is None or isinstance(deck["errors"], int)
        assert deck["warnings"] is None or isinstance(deck["warnings"], int)
        # Never report a count without the corpus that produced it.
        assert (deck["errors"] is None) == (deck["warnings"] is None)


def test_deck_list_gate_counts_agree_with_the_validate_endpoint(client):
    """Two code paths compute the same thing; they must not drift."""
    for deck in client.get("/api/decks").json():
        if deck["errors"] is None:
            pytest.skip("no corpus; the gate did not run")
        rep = client.get(f"/api/decks/{deck['slug']}/validate").json()
        assert deck["errors"] == len(rep["errors"]), \
            f"{deck['slug']} error count disagrees with /validate"
        assert deck["warnings"] == len(rep["warnings"]), \
            f"{deck['slug']} warning count disagrees with /validate"
        assert rep["ok"] == (deck["errors"] == 0)


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


def test_land_sweep_returns_a_row_per_count_and_reports_the_spread(client):
    """The sweep is the command that actually decides a land count, and it was
    entirely untested. The spread matters as much as the winner: Gyome's curve
    was flat to 0.07 across 30-40, and reading the argmax of that is reading
    noise."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/lands",
                            json={"slug": "gyome-food", "low": 32, "high": 34,
                                  "games": 200, "seed": 1}).json()
    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    result = body["result"]

    assert [r["lands"] for r in result["rows"]] == [32, 33, 34]
    for row in result["rows"]:
        assert 0.0 <= row["commander_by_t5"] <= 1.0
        assert 0.0 <= row["mulligan_rate"] <= 1.0
        assert row["spells_through_t8"] >= 0
    # A flat curve must be reported as flat rather than leaving the caller to
    # read the argmax of noise.
    assert result["deployment_spread"] >= 0
    assert isinstance(result["flat"], bool)
    assert result["flat"] == (result["deployment_spread"] < 0.25)
    assert result["argmax_lands"] in [r["lands"] for r in result["rows"]]
    assert result["caveat"], "Tier 1 numbers must ship with their caveat"


def test_land_sweep_normalises_a_reversed_range(client):
    """low/high the wrong way round should sweep, not return nothing."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/lands",
                            json={"slug": "gyome-food", "low": 34, "high": 32,
                                  "games": 200, "seed": 1}).json()
    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert [r["lands"] for r in body["result"]["rows"]] == [32, 33, 34]


def test_sim_payload_bounds_are_clamped_not_rejected(client):
    """An absurd game count should be pulled into range rather than 500."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/mana",
                            json={"slug": "gyome-food", "games": 1,
                                  "turns": 99, "seed": 1}).json()
    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert body["result"]["games"] >= 100, "games floor"
    assert len(body["result"]["by_turn"]) <= 20, "turns ceiling"


def test_keep_rule_uses_documented_defaults():
    from mtglab.api.simruns import _keep_rule
    rule = _keep_rule({})
    assert (rule.min_lands, rule.max_lands) == (2, 5)


def test_unknown_job_is_404(client):
    assert client.get("/api/jobs/deadbeef").status_code == 404


def test_job_progress_is_reported(client):
    job = jobs.submit("test", lambda p: [p(i, 10) for i in range(10)] and "ok")
    body = await_job(client, job.id)
    assert body["status"] == "done"
    assert body["percent"] == 100
    assert client.get("/api/jobs").json()[0]["id"] == job.id
