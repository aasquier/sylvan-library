"""The worker machine's door: three endpoints in front of `run_games`.

This is what `Dockerfile.forge` runs (ADR 35). It is deliberately the
smallest server that can hold a JVM: stdlib `http.server`, JSON in and out,
no framework — the worker image already carries ~470MB of Forge and a JRE,
and a web stack would be surface area on a machine whose whole job is one
subprocess. The app talks to it over Fly's private network; nothing routes
here from the internet.

    GET  /healthz    am I up? (also what the app polls after a machine start)
    POST /coverage   deck.yaml texts -> coverage reports, no JVM booted
    POST /match      deck.yaml texts + games/clock/seed -> a finished run

Three properties worth stating:

* **The pre-flight still runs twice.** `/coverage` answers the request
  thread's refusal check, and `/match` calls `run_games`, which re-checks
  before and after (the docstring there says why a caller that could skip it
  would). The worker never trusts that the app asked first.
* **A bearer token gates every request when `MTGLAB_FORGE_SHIM_TOKEN` is
  set.** The private network is already org-scoped; the token is the cheap
  second wall, and the deployed app shares it automatically because Fly
  injects one app's secrets into every machine of that app.
* **The shim stops its own machine.** Fly's proxy-driven auto-stop never
  sees private-network traffic, so idleness is judged here: after
  `MTGLAB_FORGE_IDLE_SECONDS` with no request and no match in flight, the
  process exits cleanly and the machine's `restart: no` policy turns that
  into `stopped` — which is the state ADR 35 prices ("a stopped machine
  costs only its rootfs storage").

One match at a time, enforced with a lock. The app's `FORGE` lane already
serialises matches, but that is a property of one process on another machine;
the `.dck` directory this process hands to Forge is racy under two JVMs, so
the worker enforces its own invariant rather than inheriting one.
"""

from __future__ import annotations

import contextlib
import hmac
import json
import os
import socket
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

from mtglab.decks.model import Deck
from mtglab.sim.tier3 import run as forge
from mtglab.sim.tier3 import wire
from mtglab.sim.tier3.coverage import ForgeNotInstalled, check, implemented_names

#: Seconds of quiet before the process exits and the machine stops. Three
#: minutes keeps a follow-up match on a warm machine (the JVM restarts per
#: match either way; this saves the machine start) at a cost of well under a
#: cent, and `0` means never — which is what a laptop running the shim by
#: hand would want.
IDLE_DEFAULT = 180

#: The JVM heap ceiling, below `run_games`'s 4096 default on purpose: the
#: worker machine has 4GB total and the heap is not the only resident —
#: metaspace, the card database's off-heap share, and this process all live
#: beside it. Measured heads-up games run well inside this.
MEMORY_MB_DEFAULT = 3072


def _idle_seconds() -> int:
    return int(os.environ.get("MTGLAB_FORGE_IDLE_SECONDS", str(IDLE_DEFAULT)))


def _memory_mb() -> int:
    return int(os.environ.get("MTGLAB_FORGE_MEMORY_MB",
                              str(MEMORY_MB_DEFAULT)))


def _token() -> str:
    return os.environ.get("MTGLAB_FORGE_SHIM_TOKEN", "")


class _State:
    """What the idle watchdog judges: when work last happened, and whether
    any is happening right now."""

    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.match_lock = threading.Lock()
        self.last_activity = time.monotonic()
        self.in_flight = 0

    def begin(self) -> None:
        with self.lock:
            self.last_activity = time.monotonic()
            self.in_flight += 1

    def end(self) -> None:
        with self.lock:
            self.last_activity = time.monotonic()
            self.in_flight -= 1

    def idle_for(self) -> float:
        with self.lock:
            if self.in_flight:
                return 0.0
            return time.monotonic() - self.last_activity


STATE = _State()


class Handler(BaseHTTPRequestHandler):
    # HTTP/1.1 so the app's client can keep a connection open across the
    # health poll and the work that follows it.
    protocol_version = "HTTP/1.1"

    def _send(self, code: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _authorised(self) -> bool:
        token = _token()
        if not token:
            return True
        supplied = self.headers.get("Authorization", "")
        return hmac.compare_digest(supplied, f"Bearer {token}")

    def _body(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b"{}"
        parsed = json.loads(raw.decode("utf-8"))
        if not isinstance(parsed, dict):
            raise ValueError("the request body must be a JSON object")
        return parsed

    def do_GET(self) -> None:
        STATE.begin()
        try:
            if self.path != "/healthz":
                self._send(404, {"error": f"no route {self.path}"})
                return
            if not self._authorised():
                self._send(401, {"error": "bad or missing token"})
                return
            self._send(200, {"ok": True})
        finally:
            STATE.end()

    def do_POST(self) -> None:
        STATE.begin()
        try:
            if not self._authorised():
                self._send(401, {"error": "bad or missing token"})
                return
            try:
                body = self._body()
            except (ValueError, json.JSONDecodeError) as exc:
                self._send(400, {"error": f"unreadable request: {exc}"})
                return
            if self.path == "/coverage":
                self._coverage(body)
            elif self.path == "/match":
                self._match(body)
            else:
                self._send(404, {"error": f"no route {self.path}"})
        finally:
            STATE.end()

    def _coverage(self, body: dict[str, Any]) -> None:
        try:
            decks = wire.decks_from_wire(list(body.get("decks") or []))
            index = implemented_names()
            reports = [check(deck, index) for deck in decks]
        except ForgeNotInstalled as exc:
            self._send(503, {"error": str(exc)})
            return
        except Exception as exc:  # noqa: BLE001 - the app renders the message
            self._send(400, {"error": f"{type(exc).__name__}: {exc}"})
            return
        self._send(200, {"reports": wire.reports_to_wire(reports)})

    def _match(self, body: dict[str, Any]) -> None:
        try:
            decks = wire.decks_from_wire(list(body.get("decks") or []))
            games = int(body.get("games", 1))
            clock = int(body.get("clock", 300))
            seed = None if body.get("seed") is None else int(body["seed"])
            stream = bool(body.get("stream"))
        except Exception as exc:  # noqa: BLE001
            self._send(400, {"error": f"unreadable request: {exc}"})
            return
        # One JVM at a time, whatever the caller believes about its own lane.
        with STATE.match_lock:
            if stream:
                self._match_streamed(decks, games, clock, seed)
                return
            try:
                result = forge.run_games(decks, games=games, clock=clock,
                                         seed=seed, memory_mb=_memory_mb())
            except ForgeNotInstalled as exc:
                self._send(503, {"error": str(exc)})
                return
            except Exception as exc:  # noqa: BLE001 - becomes the job's error
                self._send(500, {"error": f"{type(exc).__name__}: {exc}"})
                return
        self._send(200, wire.run_to_wire(result))

    def _match_streamed(self, decks: list[Deck], games: int, clock: int,
                        seed: int | None) -> None:
        """The same match, reported one game at a time.

        A match takes minutes and a job on the far side wants to tick per
        game, so a `{"stream": true}` request answers 200 up front and then
        speaks newline-delimited JSON as the match runs: `{"game": n}` per
        finished game, then exactly one of `{"result": ...}` or
        `{"error": ...}`. The headers go out before `run_games` does, which
        is why failure is a line rather than a status code here -- 200 was
        already spent buying the right to speak early. Chunked framing is
        written by hand because `BaseHTTPRequestHandler` offers none, and
        `protocol_version = "HTTP/1.1"` (declared above for keep-alive)
        requires the framing once Content-Length is unknowable.

        A caller that never asks to stream gets the plain answer, so an app
        deployed a few minutes apart from its worker keeps working in both
        directions -- an old shim ignores the flag, and `worker.py` reads the
        Content-Type before deciding how to listen.
        """
        self.send_response(200)
        self.send_header("Content-Type", "application/x-ndjson")
        self.send_header("Transfer-Encoding", "chunked")
        self.end_headers()

        def emit(payload: dict[str, Any]) -> None:
            # A vanished listener must not kill the JVM mid-game: the write
            # fails quietly and the match plays out. The idle watchdog cannot
            # stop the machine under it -- this request is still in flight.
            data = json.dumps(payload).encode("utf-8") + b"\n"
            try:
                self.wfile.write(f"{len(data):X}\r\n".encode("ascii")
                                 + data + b"\r\n")
                self.wfile.flush()
            except OSError:
                pass

        try:
            # The row rides the tick (the match theater): the game the parser
            # just completed crosses beside its count, in the same encoding
            # the final result will carry it. An app from before the theater
            # ignores the extra key, so the skew story is unchanged.
            result = forge.run_games(
                decks, games=games, clock=clock, seed=seed,
                memory_mb=_memory_mb(),
                on_game=lambda n, g: emit(
                    {"game": n, "row": wire.game_to_wire(g)}))
        except Exception as exc:  # noqa: BLE001 - becomes the job's error
            emit({"error": f"{type(exc).__name__}: {exc}",
                  "type": type(exc).__name__})
        else:
            emit({"result": wire.run_to_wire(result)})
        with contextlib.suppress(OSError):
            self.wfile.write(b"0\r\n\r\n")


def serve(host: str = "::", port: int = 8080) -> ThreadingHTTPServer:
    """A bound, unstarted server. Tests drive it on a loopback port; `main`
    runs it on the machine's private address."""
    class Server(ThreadingHTTPServer):
        address_family = (socket.AF_INET6 if ":" in host else socket.AF_INET)
        daemon_threads = True

    return Server((host, port), Handler)


def _watchdog(limit: int) -> None:
    while True:
        time.sleep(15)
        if STATE.idle_for() > limit:
            print(f"forge shim: idle for {limit}s, stopping the machine",
                  flush=True)
            # Abrupt on purpose: there is nothing to flush, no state to keep,
            # and a clean exit is what turns `restart: no` into `stopped`.
            os._exit(0)


def main() -> None:
    host = os.environ.get("MTGLAB_FORGE_SHIM_HOST", "::")
    port = int(os.environ.get("MTGLAB_FORGE_SHIM_PORT", "8080"))
    limit = _idle_seconds()
    if limit > 0:
        threading.Thread(target=_watchdog, args=(limit,), daemon=True).start()
    # Warm the coverage index while nobody is waiting: the first `/coverage`
    # after a cold boot would otherwise pay the ~2s zip scan inside somebody's
    # request thread.
    try:
        implemented_names()
    except ForgeNotInstalled as exc:
        # A worker image without Forge is misbuilt; say so and keep serving,
        # because /healthz answering is what lets the app read the 503s.
        print(f"forge shim: {exc}", flush=True)
    server = serve(host, port)
    print(f"forge shim: listening on [{host}]:{port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
