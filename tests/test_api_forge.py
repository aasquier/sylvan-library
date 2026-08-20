"""The Forge surface (ADR 35): the gate, the refusals, and the match job.

Forge itself is never run here — the JVM is faked at the module seams
`sim/tier3/run.py` exposes, the same places `test_sim_tier3.py` fakes them.
What is under test is the division the route promises: everything refusable
refused in the request with its own status code, and a job that holds nothing
but the subprocess call.
"""

import sys
import threading
import time
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

fastapi = pytest.importorskip("fastapi")
pytest.importorskip("httpx")

from fastapi.testclient import TestClient  # noqa: E402

import tiny_pool  # noqa: E402
from mtglab import config  # noqa: E402
from mtglab.api import forgeruns, jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.sim.tier3 import parse  # noqa: E402
from mtglab.sim.tier3 import run as forge_run  # noqa: E402
from mtglab.sim.tier3.coverage import ForgeNotInstalled  # noqa: E402

JOB_TIMEOUT_S = 60

A, B = "mono-green", "mono-green-b"


@pytest.fixture
def client(tmp_path):
    """The app over a scratch library holding two decks — a match needs two."""
    jobs.clear()
    root = tmp_path / "decks"
    for slug, name in ((A, "Mono-Green Fixture"), (B, "Mono-Green Sparring")):
        deck = tiny_pool.mono_green_deck()
        deck.slug, deck.name = slug, name
        (root / slug).mkdir(parents=True)
        (root / slug / "deck.yaml").write_text(deck.dump(), encoding="utf-8")
    with config.use_paths(decks_dir=root), TestClient(create_app()) as c:
        yield c


@pytest.fixture
def forge_present(monkeypatch):
    """The environment the deployed instance does not have: Forge installed."""
    monkeypatch.setattr(forgeruns, "status",
                        lambda: {"available": True, "why": None})
    monkeypatch.setattr(forge_run, "check_coverage", lambda decks, *a: [])


def fake_match(games):
    """A SimRun the shape `run_games` really returns, seats and all."""
    results = []
    for i in range(1, games + 1):
        if i == 1:
            # The parse edge: a slow-match warning followed by a win line
            # attaches `timed_out` to a game that names a winner. The shape
            # must count it for nobody.
            results.append(parse.GameResult(index=i, milliseconds=134_500,
                                            winner="Ai(1)-x", winner_seat=1,
                                            timed_out=True))
        elif i == 2:
            results.append(parse.GameResult(index=i, milliseconds=8_000,
                                            draw=True))
        else:
            seat = 1 if i % 2 else 2
            results.append(parse.GameResult(
                index=i, milliseconds=5_000 + i, winner=f"Ai({seat})-x",
                winner_seat=seat, turns=9))
    return forge_run.SimRun(
        argv=["java"], output=parse.SimOutput(games=results),
        wall_seconds=70.0, seats={1: A, 2: B})


def await_job(client, job_id):
    deadline = time.monotonic() + JOB_TIMEOUT_S
    body = {}
    while time.monotonic() < deadline:
        body = client.get(f"/api/jobs/{job_id}").json()
        if body["status"] in ("done", "error"):
            return body
        time.sleep(0.05)
    pytest.fail(f"job did not finish in {JOB_TIMEOUT_S}s: {body}")


def post(client, **overrides):
    payload = {"a_slug": A, "b_slug": B, "games": 4}
    payload.update(overrides)
    return client.post("/api/sim/forge", json=payload)


# ---------------------------------------------------------------- the gate

def test_the_gate_reports_absence_as_a_fact_not_an_error(client, monkeypatch):
    monkeypatch.setattr(forgeruns, "status",
                        lambda: {"available": False, "why": "no jar"})
    body = client.get("/api/forge")
    assert body.status_code == 200
    assert body.json() == {"available": False, "why": "no jar"}


def test_the_gate_probes_without_booting_a_jvm(monkeypatch):
    """`status` may look for the jar and the JVM; it may not run Forge."""
    monkeypatch.setattr(forge_run, "desktop_jar",
                        lambda *a: (_ for _ in ()).throw(
                            ForgeNotInstalled("no Forge distribution here")))
    body = forgeruns.status()
    assert body["available"] is False
    assert "no Forge distribution" in body["why"]


# ------------------------------------------------------------ the refusals

def test_a_missing_slug_is_422(client, forge_present):
    assert post(client, a_slug="").status_code == 422


def test_an_absent_forge_is_503_and_no_job_exists(client, monkeypatch):
    monkeypatch.setattr(forgeruns, "status",
                        lambda: {"available": False, "why": "no jar"})
    assert post(client).status_code == 503
    assert client.get("/api/jobs").json() == []


def test_an_unknown_deck_is_404(client, forge_present):
    assert post(client, b_slug="no-such-deck").status_code == 404


def test_a_coverage_failure_is_422_naming_the_cards(client, monkeypatch):
    monkeypatch.setattr(forgeruns, "status",
                        lambda: {"available": True, "why": None})
    monkeypatch.setattr(
        forge_run, "check_coverage",
        lambda decks, *a: (_ for _ in ()).throw(
            forge_run.CoverageFailed("missing: Nonexistent Card 1")))
    refused = post(client)
    assert refused.status_code == 422
    assert "Nonexistent Card 1" in refused.json()["detail"]
    assert client.get("/api/jobs").json() == []


# ------------------------------------------------------------- the match

def test_a_match_runs_as_a_job_and_reports_honestly(client, forge_present,
                                                    monkeypatch):
    seen = {}

    def run_games(decks, *, games, clock, seed):
        seen.update(games=games, clock=clock, seed=seed,
                    slugs=[d.slug for d in decks])
        return fake_match(games)

    monkeypatch.setattr(forge_run, "run_games", run_games)
    job = post(client).json()
    result = await_job(client, job["id"])
    assert result["status"] == "done"

    # The run was asked for exactly what the request said, seeded by default.
    assert seen == {"games": 4, "clock": forgeruns.CLOCK,
                    "seed": forgeruns.DEFAULT_SEED, "slugs": [A, B]}

    body = result["result"]
    # Game 1 clocked out, game 2 really drew, games 3 and 4 split by seat.
    assert body["timed_out"] == 1
    assert body["draws"] == 1
    assert {d["slug"]: d["wins"] for d in body["decks"]} == {A: 1, B: 1}
    assert body["decks"][0]["address"] == f"local/{A}"
    assert body["median_seconds"] is not None
    assert body["caveat"] == forgeruns.FORGE_CAVEAT
    # A clock-out is never a draw and never a win.
    clocked = [r for r in body["rows"] if r["timed_out"]]
    assert clocked and all(not r["draw"] and r["winner"] is None
                           for r in clocked)


def test_games_are_clamped_before_the_label_is_written(client, forge_present,
                                                       monkeypatch):
    monkeypatch.setattr(forge_run, "run_games",
                        lambda decks, *, games, clock, seed: fake_match(games))
    job = post(client, games=999).json()
    assert f"{forgeruns.GAMES_MAX} games" in job["label"]
    assert await_job(client, job["id"])["result"]["games"] == forgeruns.GAMES_MAX


def test_asking_twice_in_flight_is_one_job(client, forge_present, monkeypatch):
    """The dossier's money-bug rule, inherited: a second identical click while
    the JVM is working joins the live job rather than queueing a second one."""
    release = threading.Event()

    def slow_run(decks, *, games, clock, seed):
        release.wait(JOB_TIMEOUT_S)
        return fake_match(games)

    monkeypatch.setattr(forge_run, "run_games", slow_run)
    try:
        first = post(client).json()
        second = post(client).json()
        different = post(client, games=3).json()
        assert first["id"] == second["id"]
        assert different["id"] != first["id"]
    finally:
        release.set()
    await_job(client, first["id"])
    await_job(client, different["id"])


def test_the_lane_is_forge_not_cpu():
    """A match must not park behind a Tier 1 sweep or beside a second JVM."""
    deck = tiny_pool.mono_green_deck()
    plan = forgeruns.plan_forge([deck, deck], ["local/a", "local/b"], {})
    assert plan.lane == jobs.FORGE
    assert plan.key is not None and "local/a" in plan.key
