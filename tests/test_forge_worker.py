"""The hosted half of Tier 3 (ADR 35): the wire, the shim, and the client.

No JVM, no Forge install, no Fly. The shim runs in-process on a loopback
port, so the client half of these tests exercises the **real HTTP path** —
serialisation, token, status codes — with only `run_games` and the coverage
index faked at the same module seams `test_sim_tier3.py` fakes them. The
Machines API is faked at `worker._api`, which is the single door every
control-plane call goes through.

The drift test is the one to keep honest: a match that crossed the wire and
a match that ran locally must shape identically in `forgeruns._shape`,
because `wire.py`'s whole promise is that downstream code cannot tell.
"""

from __future__ import annotations

import json
import threading
import urllib.request

import pytest

from mtglab.api import forgeruns
from mtglab.decks.model import CardEntry, Deck
from mtglab.sim.tier3 import parse, shim, wire, worker
from mtglab.sim.tier3 import run as forge_run
from mtglab.sim.tier3.coverage import ForgeNotInstalled
from mtglab.sim.tier3.run import CoverageFailed, SimRun


def make_deck(slug: str = "test-deck") -> Deck:
    return Deck(slug=slug, name=f"Deck {slug}",
                commander=["Gyome, Master Chef"],
                cards=[CardEntry(name="Sol Ring", category="ramp", why="x")])


INDEX = frozenset({"Gyome, Master Chef", "Sol Ring"})


def fake_run(games: int = 3) -> SimRun:
    results = [
        parse.GameResult(index=1, milliseconds=134_500, winner="Ai(1)-x",
                         winner_seat=1, timed_out=True),
        parse.GameResult(index=2, milliseconds=8_000, draw=True),
        parse.GameResult(index=3, milliseconds=5_100, winner="Ai(2)-x",
                         winner_seat=2, turns=9),
    ]
    return SimRun(argv=["java"], output=parse.SimOutput(games=results[:games]),
                  wall_seconds=70.0, seats={1: "deck-a", 2: "deck-b"},
                  forge_version="2.0.14")


# ---------------------------------------------------------------- the wire

def test_a_deck_survives_the_wire_intact():
    deck = make_deck()
    [back] = wire.decks_from_wire(wire.decks_to_wire([deck]))
    assert back.slug == deck.slug
    assert back.commander == deck.commander
    assert [(c.name, c.qty) for c in back.cards] == \
        [(c.name, c.qty) for c in deck.cards]


def test_a_run_survives_the_wire_with_its_edges():
    """The clock-out, the draw, the seats and the derived startup number all
    have to arrive — these are exactly the fields `_shape` reports apart."""
    run = fake_run()
    back = wire.run_from_wire(json.loads(json.dumps(wire.run_to_wire(run))))
    assert [(g.index, g.milliseconds, g.draw, g.winner_seat, g.turns,
             g.timed_out) for g in back.games] == \
        [(g.index, g.milliseconds, g.draw, g.winner_seat, g.turns,
          g.timed_out) for g in run.games]
    assert back.seats == run.seats
    assert back.startup_seconds == run.startup_seconds
    assert back.winner_slug(back.games[2]) == "deck-b"
    # The match ledger's provenance column rides the wire (ADR 36) — and an
    # old shim's payload without it rebuilds as "not reported", not an error.
    assert back.forge_version == "2.0.14"
    skewed = wire.run_to_wire(run)
    del skewed["forge_version"]
    assert wire.run_from_wire(skewed).forge_version is None


def test_a_remote_run_shapes_identically_to_a_local_one():
    """The drift test. `forgeruns._shape` is the consumer both halves feed."""
    decks = [make_deck("deck-a"), make_deck("deck-b")]
    local = fake_run()
    remote = wire.run_from_wire(wire.run_to_wire(local))
    addresses = ["local/deck-a", "local/deck-b"]
    assert forgeruns._shape(decks, addresses, 3, 42, local) == \
        forgeruns._shape(decks, addresses, 3, 42, remote)


def test_coverage_reports_survive_the_wire():
    from mtglab.sim.tier3.coverage import check
    report = check(make_deck(), INDEX)
    [back] = wire.reports_from_wire(wire.reports_to_wire([report]))
    assert (back.slug, back.checked, back.resolved, back.missing) == \
        (report.slug, report.checked, report.resolved, report.missing)


# ---------------------------------------------------------------- the shim

def _fake_run_games(decks, *, games, clock, seed, memory_mb, on_game=None):
    """What the shim's seam fakes: a match that ticks like the real one."""
    played = min(games, 3)
    for n in range(1, played + 1):
        if on_game is not None:
            on_game(n)
    return fake_run(played)


@pytest.fixture
def shim_url(monkeypatch):
    """A real shim on a loopback port, with the JVM-shaped seams faked."""
    monkeypatch.setattr(shim, "implemented_names", lambda: INDEX)
    monkeypatch.setattr(forge_run, "run_games", _fake_run_games)
    monkeypatch.delenv("MTGLAB_FORGE_SHIM_TOKEN", raising=False)
    server = shim.serve("127.0.0.1", 0)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_address[1]}"
    finally:
        server.shutdown()


def fetch(url: str, payload: dict | None = None,
          token: str | None = None) -> tuple[int, dict]:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(
        url, headers=headers,
        data=None if payload is None else json.dumps(payload).encode(),
        method="GET" if payload is None else "POST")
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return response.status, json.loads(response.read())
    except urllib.error.HTTPError as exc:
        return exc.code, json.loads(exc.read())


def test_the_shim_answers_health(shim_url):
    assert fetch(f"{shim_url}/healthz") == (200, {"ok": True})


def test_the_shim_checks_coverage_without_a_jvm(shim_url):
    code, body = fetch(f"{shim_url}/coverage",
                       {"decks": wire.decks_to_wire([make_deck()])})
    assert code == 200
    [report] = wire.reports_from_wire(body["reports"])
    assert report.missing == []
    assert report.checked == 2


def test_the_shim_names_a_missing_card(shim_url, monkeypatch):
    deck = make_deck()
    deck.cards.append(CardEntry(name="Nonexistent Card 1", category="x",
                                why="x"))
    code, body = fetch(f"{shim_url}/coverage",
                       {"decks": wire.decks_to_wire([deck])})
    assert code == 200
    [report] = wire.reports_from_wire(body["reports"])
    assert report.missing == ["Nonexistent Card 1"]


def test_the_shim_plays_a_match_and_returns_the_run(shim_url):
    code, body = fetch(
        f"{shim_url}/match",
        {"decks": wire.decks_to_wire([make_deck("a"), make_deck("b")]),
         "games": 3, "clock": 300, "seed": 42})
    assert code == 200
    run = wire.run_from_wire(body)
    assert len(run.games) == 3
    assert run.seats == {1: "deck-a", 2: "deck-b"}


def test_a_failing_run_is_a_500_with_the_reason(shim_url, monkeypatch):
    def explode(decks, *, games, clock, seed, memory_mb):
        raise forge_run.ResultsUntrustworthy("dropped card: X")
    monkeypatch.setattr(forge_run, "run_games", explode)
    code, body = fetch(f"{shim_url}/match",
                       {"decks": wire.decks_to_wire([make_deck()]),
                        "games": 1})
    assert code == 500
    assert "dropped card: X" in body["error"]


def stream(url: str, payload: dict) -> list[dict]:
    """POST /match with `stream: true` and read the NDJSON lines back."""
    request = urllib.request.Request(
        f"{url}/match", headers={"Content-Type": "application/json"},
        data=json.dumps(payload).encode(), method="POST")
    with urllib.request.urlopen(request, timeout=10) as response:
        assert "ndjson" in response.headers.get("Content-Type", "")
        return [json.loads(line) for line in response]


def test_the_shim_streams_a_tick_per_game_then_the_result(shim_url):
    lines = stream(
        shim_url,
        {"decks": wire.decks_to_wire([make_deck("a"), make_deck("b")]),
         "games": 3, "clock": 300, "seed": 42, "stream": True})
    assert [line.get("game") for line in lines[:-1]] == [1, 2, 3]
    run = wire.run_from_wire(lines[-1]["result"])
    assert len(run.games) == 3
    assert run.seats == {1: "deck-a", 2: "deck-b"}


def test_a_streamed_failure_is_an_error_line_naming_its_type(shim_url,
                                                             monkeypatch):
    """200 is already spent when a streamed match fails, so the refusal is a
    line — and it carries the exception type, which is how the client keeps
    `ForgeNotInstalled` 503-shaped across the wire."""
    def explode(decks, *, games, clock, seed, memory_mb, on_game=None):
        raise ForgeNotInstalled("no jar on this machine")
    monkeypatch.setattr(forge_run, "run_games", explode)
    lines = stream(shim_url,
                   {"decks": wire.decks_to_wire([make_deck()]),
                    "games": 1, "stream": True})
    assert lines[-1]["type"] == "ForgeNotInstalled"
    assert "no jar" in lines[-1]["error"]


def test_the_token_gates_every_route_when_set(shim_url, monkeypatch):
    monkeypatch.setenv("MTGLAB_FORGE_SHIM_TOKEN", "sesame")
    assert fetch(f"{shim_url}/healthz")[0] == 401
    assert fetch(f"{shim_url}/coverage", {"decks": []})[0] == 401
    assert fetch(f"{shim_url}/healthz", token="wrong")[0] == 401
    assert fetch(f"{shim_url}/healthz", token="sesame")[0] == 200


def test_idle_is_zero_while_work_is_in_flight():
    state = shim._State()
    state.begin()
    assert state.idle_for() == 0.0
    state.end()
    assert state.idle_for() >= 0.0


# -------------------------------------------------------------- the client

@pytest.fixture
def worker_env(shim_url, monkeypatch):
    monkeypatch.setenv("MTGLAB_FORGE_WORKER_URL", shim_url)
    yield


def test_configured_is_a_truth_table(monkeypatch):
    for name in ("MTGLAB_FORGE_WORKER_URL", "MTGLAB_FORGE_WORKER",
                 "MTGLAB_FLY_API_TOKEN"):
        monkeypatch.delenv(name, raising=False)
    assert worker.configured() is False
    monkeypatch.setenv("MTGLAB_FORGE_WORKER", "1")
    assert worker.configured() is False, "the dial alone must not flip it"
    monkeypatch.setenv("MTGLAB_FLY_API_TOKEN", "t")
    assert worker.configured() is True
    monkeypatch.delenv("MTGLAB_FORGE_WORKER")
    monkeypatch.delenv("MTGLAB_FLY_API_TOKEN")
    monkeypatch.setenv("MTGLAB_FORGE_WORKER_URL", "http://[::1]:1")
    assert worker.configured() is True


def test_the_client_runs_a_match_over_real_http(worker_env):
    run = worker.run_match([make_deck("a"), make_deck("b")],
                           games=3, clock=300, seed=42)
    assert run.seats == {1: "deck-a", 2: "deck-b"}
    assert len(run.games) == 3


def test_the_client_relays_per_game_ticks(worker_env):
    """The whole point of the stream: `on_game` fires here, on the app's
    machine, once per game the worker finished."""
    ticks: list[int] = []
    run = worker.run_match([make_deck("a"), make_deck("b")],
                           games=3, clock=300, seed=42,
                           on_game=ticks.append)
    assert ticks == [1, 2, 3]
    assert len(run.games) == 3


def test_a_streamed_forge_not_installed_arrives_as_itself(worker_env,
                                                          monkeypatch):
    def explode(decks, *, games, clock, seed, memory_mb, on_game=None):
        raise ForgeNotInstalled("no jar on this machine")
    monkeypatch.setattr(forge_run, "run_games", explode)
    with pytest.raises(ForgeNotInstalled, match="no jar"):
        worker.run_match([make_deck()], games=1, clock=300, seed=None)


def test_the_client_accepts_a_plain_answer_from_an_older_shim(monkeypatch):
    """Deploy skew: the app can update minutes before its worker, so a shim
    that ignores `stream: true` and answers one flat JSON body must still
    make a match — with no ticks, which is the honest degradation."""
    from http.server import BaseHTTPRequestHandler, HTTPServer

    answer = json.dumps(wire.run_to_wire(fake_run())).encode()

    class OldShim(BaseHTTPRequestHandler):
        def do_GET(self):
            body = b'{"ok": true}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            self.rfile.read(int(self.headers.get("Content-Length") or 0))
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(answer)))
            self.end_headers()
            self.wfile.write(answer)

        def log_message(self, *args):
            pass

    server = HTTPServer(("127.0.0.1", 0), OldShim)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    monkeypatch.setenv("MTGLAB_FORGE_WORKER_URL",
                       f"http://127.0.0.1:{server.server_address[1]}")
    try:
        ticks: list[int] = []
        run = worker.run_match([make_deck("a"), make_deck("b")],
                               games=3, clock=300, seed=1,
                               on_game=ticks.append)
        assert len(run.games) == 3
        assert ticks == []
    finally:
        server.shutdown()


def test_the_client_raises_coverage_failures_with_the_cards_named(worker_env):
    deck = make_deck()
    deck.cards.append(CardEntry(name="Nonexistent Card 1", category="x",
                                why="x"))
    with pytest.raises(CoverageFailed) as exc:
        worker.check_coverage([deck])
    assert "Nonexistent Card 1" in str(exc.value)


def test_a_clean_deck_passes_the_client_preflight(worker_env):
    reports = worker.check_coverage([make_deck()])
    assert [r.ok for r in reports] == [True]


def test_the_gate_answers_yes_on_configuration_alone(worker_env):
    """No machine is woken to answer `/api/forge` — the `/api/claude`
    contract. The shim fixture is running, but `status` must not need it."""
    assert forgeruns.status() == {"available": True, "why": None}


# ------------------------------------------------------- the machines door

def test_a_missing_machine_is_a_503_shaped_refusal(monkeypatch):
    monkeypatch.delenv("MTGLAB_FORGE_WORKER_URL", raising=False)
    monkeypatch.setenv("MTGLAB_FLY_APP", "sylvan-library")
    monkeypatch.setattr(worker, "_api", lambda *a, **k: [])
    with pytest.raises(ForgeNotInstalled) as exc:
        worker._base_url()
    assert "forge-worker" in str(exc.value)


def test_a_stopped_machine_is_started_and_addressed_by_id(monkeypatch):
    monkeypatch.delenv("MTGLAB_FORGE_WORKER_URL", raising=False)
    monkeypatch.setenv("MTGLAB_FLY_APP", "sylvan-library")
    calls = []

    def api(method, path, payload=None, timeout=30):
        calls.append((method, path))
        if path == "/apps/sylvan-library/machines":
            return [{"id": "m1", "name": "forge-worker", "state": "stopped"}]
        return {}

    monkeypatch.setattr(worker, "_api", api)
    base = worker._base_url()
    assert base == "http://m1.vm.sylvan-library.internal:8080"
    assert ("POST", "/apps/sylvan-library/machines/m1/start") in calls
    assert any("wait?state=started" in path for _, path in calls)


def test_a_started_machine_is_not_restarted(monkeypatch):
    monkeypatch.delenv("MTGLAB_FORGE_WORKER_URL", raising=False)
    monkeypatch.setenv("MTGLAB_FLY_APP", "sylvan-library")

    def api(method, path, payload=None, timeout=30):
        assert method == "GET" and path.endswith("/machines"), \
            f"unexpected control-plane call: {method} {path}"
        return [{"id": "m1", "name": "forge-worker", "state": "started"}]

    monkeypatch.setattr(worker, "_api", api)
    assert worker._base_url().startswith("http://m1.vm.")
