"""The rig: one seeded instance of the app, reached three different ways.

The contract suite asks the same questions of the served app however the app
happens to be running, and this module is what makes "however" true:

- **in-process** -- `TestClient` over `create_app(...)`, the way every other
  HTTP test in the suite works. The default, and what the ordinary `pytest`
  run uses; fast, and it keeps the contract tests inside the coverage run.
- **live** (`pytest tests/contract --live`) -- the harness seeds a scratch
  directory, starts `mtglab ui` on it in a child process, exactly the command
  the container's `CMD` runs, and drives it over TCP with `httpx`. This is
  the mode that proves the rig: nothing is mocked, stubbed or monkeypatched
  on the far side of the socket.
- **external** (`--base-url URL --data-dir DIR --decks-dir DIR`) -- a server
  somebody else started against `DIR`, seeded by the same function. This is
  the coexistence mode the Go migration needs from Phase 2: the Go front
  door on one port proxying uvicorn on another, both reading the scratch the
  harness seeded (docs/go-migration/PLAN.md section 5, instrument 1).

One seeding function serves all three, and it is **idempotent**, so an
external server can be re-run against without a rebuild: two accounts (alice
administers, bob does not -- the `test_isolation.py` arrangement, kept so
every adversarial check also says something about admin), a throwaway
account the rate-limit test may lock out, two file-tier decks built from
`tiny_pool`, and the 21-card pool itself at `DB_PATH`. Seeding uses the
app's own Python helpers pointed at the scratch, which is what the plan meant
by "the harness needs no Go-side fixtures": the Go binary reads `app.db`
and `deck.yaml` files Python wrote, which is also exactly what a cutover is.

**Nothing here spends anything.** The child server and the in-process app
both run with the Anthropic key, the mail key, the Fly token and the Forge
home blanked, so a contract case that reaches a Claude route records the
503 the key's absence produces, and a `sim/forge` case records the 503 an
uninstalled Forge produces -- on this Mac, where Forge *is* installed, as
much as in CI. A contract case that could cost money or touch the network
would record a different shape on every machine that ran it, which is the
opposite of a contract.

The mutation hook at the bottom is the suite's proof of itself. Pointed at a
deliberately broken shape (`MTGLAB_CONTRACT_MUTATE=envelope`, say), the
in-process app is wrapped so that every error body says `message` where the
frontend reads `detail`, and `tests/test_contract_harness.py` asserts the
suite fails. A contract suite that cannot be shown to fail is a contract
suite that may already be passing against nothing -- the lesson this
repository has learned about guards often enough to write down.
"""

from __future__ import annotations

import contextlib
import gzip
import json
import os
import socket
import sqlite3
import subprocess
import sys
import time
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import httpx

import tiny_pool
from mtglab import config
from mtglab.api import jobs
from mtglab.api.app import create_app
from mtglab.auth import db, users

# -------------------------------------------------------------- the seeding

#: The accounts every mode starts with. Alice administers, bob does not; the
#: third exists to be locked out by the rate-limit case, which a shared
#: account could not survive.
PASSWORDS = {
    "alice": "correct-horse-battery-staple",
    "bob": "a-different-long-passphrase",
    "throttle": "this-account-gets-locked-out",
}
ADMINS = frozenset({"alice"})

#: The file-tier decks, from `tiny_pool`, under the owner segment the file
#: tier answers to when auth is on.
DECK_OWNER = "local"
DECK_SLUG = "mono-green"
BUILT_DECK_SLUG = "mono-green-built"

#: What a contract run blanks in the app's environment, in-process and in
#: the child server alike. `MTGLAB_REQUIRE_AUTH` is *set*: the mail sender
#: reads it from the environment rather than from the app object, and the two
#: modes must agree about whether an invite can be delivered.
SERVER_ENV = {
    "MTGLAB_REQUIRE_AUTH": "1",
    "MTGLAB_SECURE_COOKIES": "0",
    "MTGLAB_ADMIN_EMAIL": "",
    "MTGLAB_ADMIN_USERNAME": "",
    "ANTHROPIC_API_KEY": "",
    "ANTHROPIC_AUTH_TOKEN": "",
    "RESEND_API_KEY": "",
    "FLY_METRICS_TOKEN": "",
    "MTGLAB_FORGE_WORKER": "",
    "MTGLAB_FLY_API_TOKEN": "",
    "MTGLAB_CLIENT_IP_HEADER": "",
    "MTGLAB_BASE_URL": "",
    "PYTHONUNBUFFERED": "1",
}


@dataclass(frozen=True)
class Scratch:
    """Where one contract run's app state lives."""

    root: Path
    data_dir: Path
    decks_dir: Path

    @property
    def forge_home(self) -> Path:
        """A Forge home that does not exist, so the gate says so everywhere."""
        return self.root / "no-forge-here"

    @property
    def server_log(self) -> Path:
        return self.root / "server.log"


def make_scratch(root: Path, *, data_dir: Path | None = None,
                 decks_dir: Path | None = None) -> Scratch:
    root.mkdir(parents=True, exist_ok=True)
    return Scratch(root=root,
                   data_dir=(data_dir or root / "data").resolve(),
                   decks_dir=(decks_dir or root / "decks").resolve())


def seed(scratch: Scratch) -> None:
    """Put the contract fixtures in place. Safe to call again."""
    scratch.decks_dir.mkdir(parents=True, exist_ok=True)
    scratch.data_dir.mkdir(parents=True, exist_ok=True)

    def write_deck(slug: str, deck: Any) -> None:
        folder = scratch.decks_dir / slug
        if (folder / "deck.yaml").exists():
            return
        folder.mkdir(parents=True, exist_ok=True)
        (folder / "deck.yaml").write_text(deck.dump(), encoding="utf-8")

    write_deck(DECK_SLUG, tiny_pool.mono_green_deck())
    built = tiny_pool.mono_green_deck()
    built.slug, built.status, built.name = (BUILT_DECK_SLUG, "built",
                                            "Mono-Green Fixture, Built")
    write_deck(BUILT_DECK_SLUG, built)

    with config.use_paths(data_dir=scratch.data_dir,
                          decks_dir=scratch.decks_dir):
        if not config.DB_PATH.exists():
            tiny_pool.build(config.DB_PATH)
        con = db.connect()
        try:
            for name, password in PASSWORDS.items():
                if users.get(con, name) is None:
                    users.create(con, name, password=password,
                                 is_admin=name in ADMINS)
        finally:
            con.close()
    reset_rate_limits(scratch)


def reset_rate_limits(scratch: Scratch) -> None:
    """Forget every counted login failure, so a run never inherits a lockout.

    The limiter lives in `app.db` (`auth/ratelimit.py`), which the harness
    owns in every mode, so this is a plain delete rather than a wait. Called
    at seeding and again by the rate-limit case after it has done its worst.
    """
    with config.use_paths(data_dir=scratch.data_dir,
                          decks_dir=scratch.decks_dir):
        con = db.connect()
        try:
            with contextlib.suppress(sqlite3.OperationalError):
                con.execute("DELETE FROM login_attempts")
                con.commit()
        finally:
            con.close()


# ------------------------------------------------------------ the instance

@dataclass
class Instance:
    """A seeded app and a client pointed at it, however it is running."""

    mode: str                    # "in-process" | "live" | "external"
    client: Any                  # `TestClient` or `httpx.Client`; same surface
    scratch: Scratch
    base_url: str
    _make_client: Callable[[], Any]

    def login(self, user: str) -> Any:
        """Sign `user` in on the instance's client, replacing any session."""
        response = self.client.post("/api/auth/login", json={
            "username": user, "password": PASSWORDS[user]})
        assert response.status_code == 200, (
            f"could not sign in as {user}: {response.status_code} "
            f"{response.text}")
        return response

    def logout(self) -> None:
        self.client.post("/api/auth/logout")
        self.client.cookies.clear()

    def as_user(self, user: str | None) -> Any:
        """The client, signed in as `user` -- or signed out when `None`."""
        if user is None:
            self.logout()
        else:
            self.login(user)
        return self.client

    def new_client(self) -> Any:
        """A second client with its own cookie jar, for two-session checks."""
        return self._make_client()


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _server_env(scratch: Scratch) -> dict[str, str]:
    env = dict(os.environ)
    env.update(SERVER_ENV)
    env["MTGLAB_DATA_DIR"] = str(scratch.data_dir)
    env["MTGLAB_DECKS_DIR"] = str(scratch.decks_dir)
    env["MTGLAB_FORGE_HOME"] = str(scratch.forge_home)
    return env


def _wait_for_health(base_url: str, proc: subprocess.Popen[bytes] | None,
                     log: Path, *, timeout: float = 90.0) -> None:
    deadline = time.monotonic() + timeout
    last = "no answer yet"
    while time.monotonic() < deadline:
        if proc is not None and proc.poll() is not None:
            raise RuntimeError(
                f"the server exited with {proc.returncode} before answering "
                f"/api/health:\n{_tail(log)}")
        try:
            response = httpx.get(f"{base_url}/api/health", timeout=5.0)
            if response.status_code == 200:
                return
            last = f"{response.status_code} {response.text[:200]}"
        except httpx.HTTPError as exc:
            last = repr(exc)
        time.sleep(0.25)
    raise RuntimeError(
        f"{base_url}/api/health did not answer 200 within {timeout:.0f}s "
        f"(last: {last}):\n{_tail(log)}")


def _tail(log: Path, lines: int = 40) -> str:
    if not log.exists():
        return "(no server log)"
    text = log.read_text(encoding="utf-8", errors="replace").splitlines()
    return "\n".join(text[-lines:])


@contextlib.contextmanager
def live_server(scratch: Scratch) -> Iterator[str]:
    """`mtglab ui` on the scratch, as the container runs it; yields its URL."""
    port = _free_port()
    base_url = f"http://127.0.0.1:{port}"
    with scratch.server_log.open("wb") as log:
        proc = subprocess.Popen(
            [sys.executable, "-m", "mtglab.cli", "ui", "--no-open",
             "--host", "127.0.0.1", "--port", str(port)],
            env=_server_env(scratch), cwd=scratch.root,
            stdout=log, stderr=subprocess.STDOUT)
        try:
            _wait_for_health(base_url, proc, scratch.server_log)
            yield base_url
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=15)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=15)


@contextlib.contextmanager
def open_instance(mode: str, scratch: Scratch, *, base_url: str | None = None,
                  mutation: str | None = None) -> Iterator[Instance]:
    """A seeded, reachable instance in the requested mode."""
    seed(scratch)
    if mode == "in-process":
        with _in_process(scratch, mutation) as instance:
            yield instance
        return
    if mutation:
        raise ValueError("a mutation can only be applied in-process")
    if mode == "live":
        with live_server(scratch) as url:
            yield _remote(mode, scratch, url)
        return
    if mode == "external":
        if not base_url:
            raise ValueError("external mode needs --base-url")
        _wait_for_health(base_url, None, scratch.server_log, timeout=10.0)
        yield _remote(mode, scratch, base_url)
        return
    raise ValueError(f"unknown contract mode {mode!r}")


def _remote(mode: str, scratch: Scratch, base_url: str) -> Instance:
    def make() -> httpx.Client:
        return httpx.Client(base_url=base_url, follow_redirects=False,
                            timeout=httpx.Timeout(120.0))
    return Instance(mode=mode, client=make(), scratch=scratch,
                    base_url=base_url, _make_client=make)


@contextlib.contextmanager
def _in_process(scratch: Scratch, mutation: str | None) -> Iterator[Instance]:
    import pytest
    from fastapi.testclient import TestClient

    # The child-server environment, applied to this process for the life of
    # the instance: the key blanked so no Claude route can spend, the mail
    # key blanked so an invite is the same 503 it is over the socket, the
    # maintainer unset so the lifespan does not create an account the seeding
    # did not, and Forge pointed at nothing. `MonkeyPatch.context` rather
    # than the fixture, because this may outlive one test.
    with pytest.MonkeyPatch.context() as patch:
        for name, value in _server_env(scratch).items():
            if name in SERVER_ENV or name.startswith("MTGLAB_"):
                if value == "":
                    patch.delenv(name, raising=False)
                else:
                    patch.setenv(name, value)
        with config.use_paths(data_dir=scratch.data_dir,
                              decks_dir=scratch.decks_dir):
            jobs.clear()
            app = apply_mutation(mutation, create_app(
                require_auth=True, secure_cookies=False))

            def make() -> TestClient:
                return TestClient(app, raise_server_exceptions=False,
                                  follow_redirects=False)

            with make() as client:
                yield Instance(mode="in-process", client=client,
                               scratch=scratch, base_url="http://testserver",
                               _make_client=make)


# ------------------------------------------------------------ the mutations
#
# Each takes the built ASGI app and returns one that is wrong in exactly one
# way the contract suite must notice. They exist to be run by
# `tests/test_contract_harness.py`, which is the Phase 1 exit gate: the suite
# fails loudly when pointed at a deliberately broken shape. A mutation is
# applied once, in-process, and never in a mode that reaches a real server.

Rewrite = Callable[[str, int, list[tuple[bytes, bytes]], bytes],
                   tuple[int, list[tuple[bytes, bytes]], bytes] | None]


class _Rewriter:
    """An ASGI wrapper that buffers each response and lets `fn` replace it."""

    def __init__(self, app: Any, fn: Rewrite) -> None:
        self.app = app
        self.fn = fn

    async def __call__(self, scope: Any, receive: Any, send: Any) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        started: dict[str, Any] = {}
        chunks: list[bytes] = []

        async def capture(message: Any) -> None:
            if message["type"] == "http.response.start":
                started.update(message)
                return
            if message["type"] != "http.response.body":
                await send(message)
                return
            chunks.append(message.get("body", b""))
            if message.get("more_body"):
                return
            status = int(started["status"])
            headers = [(k, v) for k, v in started.get("headers", [])]
            body = b"".join(chunks)
            replaced = self.fn(scope["path"], status, headers, body)
            if replaced is not None:
                status, headers, body = replaced
                headers = [(k, v) for k, v in headers
                           if k.lower() not in (b"content-length",
                                                b"content-encoding")]
                headers.append((b"content-length", str(len(body)).encode()))
            await send({"type": "http.response.start", "status": status,
                        "headers": headers})
            await send({"type": "http.response.body", "body": body})

        await self.app(scope, receive, capture)


def _json_of(headers: list[tuple[bytes, bytes]], body: bytes) -> Any | None:
    """The decoded JSON body, or `None` when the response is not JSON."""
    kind = dict(headers).get(b"content-type", b"")
    if not kind.startswith(b"application/json"):
        return None
    if dict(headers).get(b"content-encoding", b"") == b"gzip":
        body = gzip.decompress(body)
    try:
        return json.loads(body)
    except ValueError:
        return None


def _rewrite_json(decide: Callable[[str, int, Any], tuple[int, Any] | None]
                  ) -> Rewrite:
    def fn(path: str, status: int, headers: list[tuple[bytes, bytes]],
           body: bytes) -> tuple[int, list[tuple[bytes, bytes]], bytes] | None:
        parsed = _json_of(headers, body)
        if parsed is None:
            return None
        outcome = decide(path, status, parsed)
        if outcome is None:
            return None
        new_status, new_body = outcome
        return new_status, headers, json.dumps(new_body).encode()
    return fn


def _envelope(path: str, status: int, body: Any) -> tuple[int, Any] | None:
    if status >= 400 and isinstance(body, dict) and "detail" in body:
        return status, {"message": body["detail"]}
    return None


def _admin_404(path: str, status: int, body: Any) -> tuple[int, Any] | None:
    if status == 403 and path.startswith("/api/admin"):
        return 404, body
    return None


def _leak_403(path: str, status: int, body: Any) -> tuple[int, Any] | None:
    if status == 404 and path.startswith("/api/decks/"):
        return 403, body
    return None


def _drop_field(path: str, status: int, body: Any) -> tuple[int, Any] | None:
    if path == "/api/decks" and status == 200 and isinstance(body, list):
        return status, [{k: v for k, v in d.items() if k != "writable"}
                        for d in body]
    return None


def _status_drift(path: str, status: int, body: Any) -> tuple[int, Any] | None:
    if path == "/api/health" and status == 200:
        return 203, body
    return None


def _strip_header(name: bytes) -> Rewrite:
    def fn(path: str, status: int, headers: list[tuple[bytes, bytes]],
           body: bytes) -> tuple[int, list[tuple[bytes, bytes]], bytes] | None:
        kept = [(k, v) for k, v in headers if k.lower() != name]
        if len(kept) == len(headers):
            return None
        return status, kept, body
    return fn


def _open_route(app: Any) -> Any:
    """The allowlist grows a route it must not have; the sweep has to notice."""
    from mtglab.api import auth
    auth.PUBLIC_PATHS = auth.PUBLIC_PATHS | {"/api/decks"}  # type: ignore[misc]
    return app


MUTATIONS: dict[str, Callable[[Any], Any]] = {
    "envelope": lambda app: _Rewriter(app, _rewrite_json(_envelope)),
    "open-route": _open_route,
    "admin-404": lambda app: _Rewriter(app, _rewrite_json(_admin_404)),
    "leak-403": lambda app: _Rewriter(app, _rewrite_json(_leak_403)),
    "drop-field": lambda app: _Rewriter(app, _rewrite_json(_drop_field)),
    "status-drift": lambda app: _Rewriter(app, _rewrite_json(_status_drift)),
    "header-drop": lambda app: _Rewriter(app, _strip_header(b"x-frame-options")),
    "retry-after-drop": lambda app: _Rewriter(app, _strip_header(b"retry-after")),
}


def apply_mutation(name: str | None, app: Any) -> Any:
    if not name:
        return app
    try:
        return MUTATIONS[name](app)
    except KeyError:
        raise ValueError(
            f"unknown contract mutation {name!r}; one of "
            f"{sorted(MUTATIONS)}") from None


def undo_mutations() -> None:
    """Put back what `open-route` moved. The others wrap an app and leave no trace."""
    from mtglab.api import auth
    auth.PUBLIC_PATHS = frozenset(  # type: ignore[misc]
        p for p in auth.PUBLIC_PATHS if p != "/api/decks")


def ensure_account(scratch: Scratch, name: str, *, password: str | None = None
                   ) -> None:
    """An account that exists for one scenario to act on -- the admin
    sequence deletes `doomed` -- created again if a previous run used it up."""
    with config.use_paths(data_dir=scratch.data_dir,
                          decks_dir=scratch.decks_dir):
        con = db.connect()
        try:
            if users.get(con, name) is None:
                users.create(con, name,
                             password=password or "a-throwaway-passphrase")
        finally:
            con.close()
