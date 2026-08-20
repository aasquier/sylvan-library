"""The app's side of the hosted Forge: wake the worker, ask it, let it sleep.

ADR 35's fifth decision, as client code. The deployed app machine holds no
JVM and no distribution; what it holds is a token for Fly's Machines API and
the name of a machine that does. A match becomes: start that machine (it is
kept `stopped` between asks), wait for its shim to answer, POST the work over
the private network, and rebuild the answer into the same `SimRun` a local
run produces. The machine stops itself when idle (`shim.py` owns that), so
nothing here has to remember to turn the lights off.

**Creation is deliberately not here.** Creating the machine means pulling a
~1GB image onto a host, which is minutes the first time — the deploy workflow
does it (`.github/workflows/ci.yml`, the worker steps) with the same image it
just pushed, and keeps the machine's image current on every deploy. If the
machine is missing, this module says so as a 503-shaped `ForgeNotInstalled`
rather than provisioning infrastructure from a request thread.

Configuration, all environment (`.env.example` documents them):

* `MTGLAB_FORGE_WORKER=1` — the dial. Off, the app probes for a local Forge
  and this module is never consulted.
* `MTGLAB_FLY_API_TOKEN` — a deploy token (`fly tokens create deploy`),
  stored with `fly secrets set`. Never in `fly.toml`; the repo is public.
* `MTGLAB_FORGE_SHIM_TOKEN` — shared with the shim automatically, because
  one app's secrets reach every machine in it.
* `MTGLAB_FORGE_WORKER_URL` — point straight at a running shim and skip the
  Machines API entirely. This is how tests drive the real HTTP path, and how
  a laptop can talk to a hand-started shim.
"""

from __future__ import annotations

import contextlib
import json
import os
import time
import urllib.error
import urllib.request
from typing import Any

from mtglab.decks.model import Deck
from mtglab.sim.tier3 import wire
from mtglab.sim.tier3.coverage import CoverageReport, ForgeNotInstalled
from mtglab.sim.tier3.run import SimRun, raise_unless_covered

MACHINES_API = "https://api.machines.dev/v1"

#: How long a cold start may take before the request is refused: machine
#: start (~1s measured shape), the shim's Python boot, and its first index
#: warm. Generous rather than tight, because the refusal is a 503 somebody
#: sees.
BOOT_SECONDS = 90


def configured() -> bool:
    """Is the hosted worker the way to run Forge here? A fact about env.

    The gate (`forgeruns.status`) calls this and answers `available: true`
    without any network — the same contract `/api/claude` set: configuration
    is a fact of the environment, reachability is discovered when work is
    actually asked for.
    """
    if os.environ.get("MTGLAB_FORGE_WORKER_URL"):
        return True
    return bool(os.environ.get("MTGLAB_FORGE_WORKER", "").strip()
                and os.environ.get("MTGLAB_FLY_API_TOKEN"))


def _app_name() -> str:
    # FLY_APP_NAME is injected by Fly into every machine; the override exists
    # for tests and for talking to the instance from a laptop.
    name = (os.environ.get("MTGLAB_FLY_APP")
            or os.environ.get("FLY_APP_NAME", ""))
    if not name:
        raise ForgeNotInstalled(
            "forge worker: no Fly app name in the environment "
            "(FLY_APP_NAME or MTGLAB_FLY_APP)")
    return name


def _machine_name() -> str:
    return os.environ.get("MTGLAB_FORGE_MACHINE", "forge-worker")


def _shim_port() -> int:
    return int(os.environ.get("MTGLAB_FORGE_SHIM_PORT", "8080"))


def _api(method: str, path: str, payload: dict[str, Any] | None = None,
         timeout: float = 30) -> Any:
    """One Machines API call. Raises `ForgeNotInstalled` with the API's own
    words on failure, because every caller here turns that into a 503."""
    token = os.environ.get("MTGLAB_FLY_API_TOKEN", "")
    if not token:
        raise ForgeNotInstalled("forge worker: MTGLAB_FLY_API_TOKEN is not set")
    request = urllib.request.Request(
        f"{MACHINES_API}{path}", method=method,
        data=None if payload is None else json.dumps(payload).encode("utf-8"),
        headers={"Authorization": f"Bearer {token}",
                 "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read()
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", "replace")[:300]
        raise ForgeNotInstalled(
            f"forge worker: Machines API {method} {path} answered "
            f"{exc.code}: {detail}") from exc
    except OSError as exc:
        raise ForgeNotInstalled(
            f"forge worker: Machines API unreachable: {exc}") from exc
    return json.loads(body) if body else None


def _find_machine() -> dict[str, Any]:
    app = _app_name()
    machines = _api("GET", f"/apps/{app}/machines")
    for machine in machines or []:
        if machine.get("name") == _machine_name():
            return dict(machine)
    raise ForgeNotInstalled(
        f"forge worker: no machine named {_machine_name()!r} in app "
        f"{app!r} — the deploy workflow creates it; has one run since the "
        f"worker landed?")


def _shim_headers() -> dict[str, str]:
    token = os.environ.get("MTGLAB_FORGE_SHIM_TOKEN", "")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def _shim(method: str, url: str, payload: dict[str, Any] | None = None,
          timeout: float = 30) -> dict[str, Any]:
    request = urllib.request.Request(
        url, method=method,
        data=None if payload is None else json.dumps(payload).encode("utf-8"),
        headers=_shim_headers())
    with urllib.request.urlopen(request, timeout=timeout) as response:
        answer = json.loads(response.read())
    if not isinstance(answer, dict):
        raise RuntimeError(f"forge worker: unexpected answer from {url}")
    return answer


def _base_url() -> str:
    """Where the shim answers, waking the machine if it is asleep."""
    direct = os.environ.get("MTGLAB_FORGE_WORKER_URL")
    if direct:
        return direct.rstrip("/")

    machine = _find_machine()
    app = _app_name()
    machine_id = machine["id"]
    if machine.get("state") != "started":
        # Starting an already-started machine races here are benign: the API
        # answers an error the wait below will absorb, because what matters
        # is the state, not who set it.
        with contextlib.suppress(ForgeNotInstalled):
            _api("POST", f"/apps/{app}/machines/{machine_id}/start")
        _api("GET",
             f"/apps/{app}/machines/{machine_id}/wait?state=started&timeout=60",
             timeout=70)
    return f"http://{machine_id}.vm.{app}.internal:{_shim_port()}"


def _await_healthy(base: str, deadline: float) -> None:
    last = "no answer yet"
    while time.monotonic() < deadline:
        try:
            if _shim("GET", f"{base}/healthz", timeout=5).get("ok"):
                return
        except (OSError, ValueError, RuntimeError) as exc:
            last = str(exc)
        time.sleep(1)
    raise ForgeNotInstalled(
        f"forge worker: machine started but the shim never answered "
        f"({last})")


def _ready() -> str:
    base = _base_url()
    _await_healthy(base, time.monotonic() + BOOT_SECONDS)
    return base


def _refused(answer: dict[str, Any], code: int) -> Exception:
    message = str(answer.get("error") or f"shim answered {code}")
    if code == 503:
        return ForgeNotInstalled(message)
    return RuntimeError(message)


def _post(base: str, path: str, payload: dict[str, Any],
          timeout: float) -> dict[str, Any]:
    try:
        return _shim("POST", f"{base}{path}", payload, timeout=timeout)
    except urllib.error.HTTPError as exc:
        try:
            answer = json.loads(exc.read())
        except (ValueError, OSError):
            answer = {}
        raise _refused(answer if isinstance(answer, dict) else {},
                       exc.code) from exc
    except OSError as exc:
        raise RuntimeError(f"forge worker: {path} failed: {exc}") from exc


def check_coverage(decks: list[Deck]) -> list[CoverageReport]:
    """The pre-flight, computed where the card scripts live.

    Same contract as `run.check_coverage`: reports back, or `CoverageFailed`
    naming the cards — the route's 422 does not care which machine read the
    zip. Runs on the request thread, so the boot budget is bounded and a
    machine that will not come up is a 503, not a hang.
    """
    base = _ready()
    answer = _post(base, "/coverage",
                   {"decks": wire.decks_to_wire(decks)}, timeout=60)
    reports = wire.reports_from_wire(list(answer.get("reports") or []))
    raise_unless_covered(reports)
    return reports


def run_match(decks: list[Deck], *, games: int, clock: int,
              seed: int | None) -> SimRun:
    """One match on the worker, returned as if it had run here.

    The timeout mirrors `run_games`'s own derivation (`60 + games * clock`)
    plus slack for the wake and the JVM — the subprocess on the far side is
    already bounded, so this is the belt over that suspenders.
    """
    base = _ready()
    answer = _post(base, "/match",
                   {"decks": wire.decks_to_wire(decks), "games": games,
                    "clock": clock, "seed": seed},
                   timeout=180 + games * clock)
    return wire.run_from_wire(answer)
