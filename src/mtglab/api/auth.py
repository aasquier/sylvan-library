"""Login, logout, and the middleware that makes everything else need one.

**Deny by default, from a middleware rather than a per-route dependency**, and
that choice is the whole security argument of this module. A dependency has to
be remembered on each new route; the route somebody adds in a year is exactly
the one that will not have it, and it will look completely normal in review.
Middleware runs before routing, so an endpoint nobody protected is refused
because nobody listed it — the failure mode is a 401 on a route that should
have been public, which is loud and gets fixed in a minute.

The allowlist is `PUBLIC_PATHS` below. It is short, it is data, and
`tests/test_isolation.py` reads it: every `/api` route the app declares which is
*not* in it is requested without a cookie and must answer 401. That test is
generated from the route table, so a new unprotected endpoint fails the suite
rather than shipping.

What the checklist in `docs/HOSTING.md` §1 asks for, and where each item is:

- one generic failure for unknown-user and wrong-password ... `users.authenticate`
- a hash computed even for an unknown user ................... `passwords.verify_dummy`
- rate limit by account *and* address ....................... `auth/ratelimit.py`
- regenerate the session id on login ........................ `login`, below
- `HttpOnly; Secure; SameSite=Lax` .......................... `_set_cookie`, below
- log auth failures with time and address, never secrets ..... `loggable`, below

CSRF is handled by `SameSite=Lax`: it stops the cross-site form post, and the
only state-changing routes here take a JSON body, which a cross-origin caller
cannot send without a CORS preflight the app does not answer. If the cookie
policy is ever relaxed to `None`, a double-submit token becomes required — the
note is here because that change would look innocuous.

With auth off these three routes still exist and still work — `login` will open
`app.db`, verify a password and hand back a cookie that nothing then checks.
That is deliberate rather than an oversight: it is how the flow gets exercised
against a real browser on a laptop. What it must never become is a route that
*grants* something locally, and it does not: with auth off every caller already
has full access, so a session confers nothing it did not have.
"""

from __future__ import annotations

import logging
import posixpath
import sqlite3
from collections.abc import Awaitable, Callable
from typing import Any

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import JSONResponse

from mtglab import config
from mtglab.api.deps import ANONYMOUS, LOCAL, Scope, UserScope
from mtglab.auth import db, ratelimit, sessions, users

_LOG = logging.getLogger("mtglab.auth")

COOKIE_NAME = "sid"

# Reachable without a session when auth is on. Everything else is refused
# before routing.
#
# `/api/auth/me` is here so a browser can ask "am I logged in?" without having
# to be. It answers about the caller and nobody else, so an anonymous request
# learns only that it is anonymous.
#
# `/api/health` is here so a platform health check is a health check rather
# than a 401. It reports corpus size and the count of curated decks, which is
# what a public git repository already says.
PUBLIC_PATHS = frozenset({
    "/api/health",
    "/api/auth/login",
    "/api/auth/logout",
    "/api/auth/me",
})


def normalise_path(path: str) -> str:
    """Collapse a request path to the one form the allowlist is checked against.

    This exists because the check must never be *more permissive* than the
    router. Starlette matches the raw path, so `//api/decks` does not match the
    `/api/decks` route — but a naive `startswith("/api")` reads it as being
    outside the API and therefore public. Today that combination lands on the
    SPA and leaks nothing, which is luck rather than design. Normalising first
    means the middleware's idea of a path is at least as strict as the router's.

    `posixpath.normpath` collapses repeated slashes and resolves `.` and `..`;
    the leading-slash guard is because it turns `//x` into `//x` by POSIX rule.
    """
    collapsed = posixpath.normpath("/" + path.lstrip("/"))
    return collapsed.rstrip("/") or "/"


def is_public(path: str) -> bool:
    """Whether this path may be served without a session.

    Anything outside `/api` is the built frontend: the SPA shell, its assets,
    and the client router's own paths. It has to load for a login form to
    exist, and it contains no data — every fact it displays is fetched from an
    endpoint that is *not* public.
    """
    normalised = normalise_path(path)
    if not normalised.startswith("/api"):
        return True
    return normalised in PUBLIC_PATHS


def loggable(username: str) -> str:
    """A failed login's principal, safe to write to a log line.

    Usernames cannot contain `@` (`users.USERNAME_RE`), so anything that does
    is somebody typing their email address into the username box — and ADR 16
    is unconditional that an address must never reach a log line. The domain is
    kept because it is the part that helps and the part that is not personal.

    Redacting rather than dropping the field: "who is failing to log in" is the
    question these lines exist to answer, and `<redacted>@example.com` still
    answers "is this one person or a script working through a list".
    """
    if "@" not in username:
        return username
    _, _, domain = username.partition("@")
    return f"<redacted>@{domain}"


def client_address(request: Request) -> str:
    """The caller's address, for rate limiting and the auth log.

    Reads a header only when `MTGLAB_CLIENT_IP_HEADER` names one, because a
    header any client can set is a rate limit any client can opt out of. See
    `config.client_ip_header`.
    """
    header = config.client_ip_header()
    if header:
        raw = request.headers.get(header, "")
        if raw:
            # `X-Forwarded-For` is a chain; the client is the first entry.
            return raw.split(",")[0].strip()
    return request.client.host if request.client else "unknown"


def scope_for_token(con: sqlite3.Connection, token: str) -> UserScope:
    """Resolve a session token to a caller. `ANONYMOUS` if it resolves to none.

    Re-checks `disabled` even though disabling an account already deletes its
    sessions. The redundancy is cheap and the failure it covers — a session
    surviving a disable by some path added later — is one where the account
    holder keeps working access after somebody believed they had removed it.
    """
    session = sessions.lookup(con, token)
    if session is None:
        return ANONYMOUS
    user = users.get_by_id(con, session.user_id)
    if user is None or user.disabled:
        return ANONYMOUS
    return UserScope(user_id=user.id, username=user.username,
                     is_admin=user.is_admin, authenticated=True)


def install(app: FastAPI, *, require: bool, secure_cookies: bool) -> None:
    """Add the authentication middleware and the three auth routes."""

    @app.middleware("http")
    async def authenticate(
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        """Resolve the caller, then refuse anything not on the allowlist.

        The SQLite lookup is a primary-key hit on a local file — tens of
        microseconds — so it runs inline rather than through a threadpool hop
        that would cost more than the query.
        """
        if not require:
            request.state.scope = LOCAL
            return await call_next(request)

        token = request.cookies.get(COOKIE_NAME, "")
        if token:
            with db.connection() as con:
                request.state.scope = scope_for_token(con, token)
        else:
            request.state.scope = ANONYMOUS

        caller: UserScope = request.state.scope
        if not caller.authenticated and not is_public(request.url.path):
            return JSONResponse(status_code=401,
                                content={"detail": "authentication required"})
        return await call_next(request)

    def _set_cookie(response: Response, token: str) -> None:
        response.set_cookie(
            COOKIE_NAME, token,
            max_age=int(sessions.LIFETIME.total_seconds()),
            httponly=True,                 # unreadable from JavaScript
            secure=secure_cookies,         # HTTPS only, once deployed
            samesite="lax",                # the CSRF defence; see the docstring
            path="/",
        )

    @app.post("/api/auth/login")
    def login(payload: dict[str, Any], request: Request,
              response: Response) -> dict[str, Any]:
        """Exchange a username and password for a session cookie.

        Returns 401 for every kind of refusal with one message, and 429 when
        either budget is spent. It never reports which of the two limits was
        hit, for the same reason it never reports which half of the credentials
        was wrong.
        """
        username = str(payload.get("username") or "").strip()
        password = str(payload.get("password") or "")
        address = client_address(request)
        if not username or not password:
            raise HTTPException(status_code=422,
                                detail="username and password are required")

        keys = ((ratelimit.account_key(username), ratelimit.PER_ACCOUNT),
                (ratelimit.address_key(address), ratelimit.PER_ADDRESS))

        with db.connection() as con:
            for key, limit in keys:
                if ratelimit.exhausted(con, key, limit):
                    wait = ratelimit.retry_after(con, key, limit)
                    _LOG.warning("login throttled for %r from %s",
                                 loggable(username), address)
                    raise HTTPException(
                        status_code=429,
                        detail="too many attempts -- wait and try again",
                        headers={"Retry-After": str(wait)})

            user = users.authenticate(con, username, password)
            if user is None:
                for key, limit in keys:
                    ratelimit.record_failure(con, key, limit)
                # Username and address, never the password and never a full
                # email address -- see `loggable`.
                _LOG.warning("failed login for %r from %s",
                             loggable(username), address)
                raise HTTPException(status_code=401,
                                    detail="invalid username or password")

            for key, _ in keys:
                ratelimit.clear(con, key)
            # Session fixation: whatever token arrived is destroyed, and the
            # one that leaves is new. A token an attacker planted before the
            # login is not the token that is valid after it.
            stale = request.cookies.get(COOKIE_NAME, "")
            if stale:
                sessions.delete(con, stale)
            token = sessions.create(con, user.id)

        _set_cookie(response, token)
        _LOG.info("login for %r from %s", user.username, address)
        return {"user": user.as_dict()}      # no email; see `User.as_dict`

    @app.post("/api/auth/logout")
    def logout(request: Request, response: Response) -> dict[str, Any]:
        """End this session. Public, and a no-op when there is none.

        Public because a 401 on logout is a confusing answer to "get me out of
        here" — the honest response to logging out of nothing is that you are
        logged out.
        """
        token = request.cookies.get(COOKIE_NAME, "")
        if token and require:
            with db.connection() as con:
                sessions.delete(con, token)
        response.delete_cookie(COOKIE_NAME, path="/")
        return {"authenticated": False}

    @app.get("/api/auth/me")
    def me(caller: Scope) -> dict[str, Any]:
        """Who the caller is, and whether this instance requires anyone to be.

        Public, and the two fields are separate on purpose: a frontend needs to
        tell "you are logged out of an instance that wants a login" from "this
        instance has no login", and one collapsed boolean makes the local app
        render a sign-in form it has no server for.
        """
        return {
            "auth_required": require,
            "authenticated": caller.authenticated,
            "user": {"id": caller.user_id, "username": caller.username,
                     "is_admin": caller.is_admin}
            if caller.authenticated else None,
        }
