"""Login, logout, reset, claim, and the middleware that makes the rest need one.

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

The middleware makes one further distinction, added by ADR 17: everything under
`ADMIN_PREFIX` is refused to a caller who is not an admin, also before routing.
Same argument, one level up — an admin route is protected by *where it is
mounted*, so nobody has to remember a decorator for it to be safe.

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

With auth off these routes still exist and still work — `login` will open
`app.db`, verify a password and hand back a cookie that nothing then checks.
That is deliberate rather than an oversight: it is how the flow gets exercised
against a real browser on a laptop. What it must never become is a route that
*grants* something locally, and it does not: with auth off every caller already
has full access, so a session confers nothing it did not have.

**`reset` and `claim` are ADR 16's email half**, and two properties of them are
worth finding here rather than in a review:

- `reset` answers the same thing in the same time whether or not the address
  exists. The response is fixed text, and the lookup and the send both happen
  in a background task *after* it has gone out — so "no such address" is not
  merely un-said, it is un-timeable. `invites.send_reset` returns nothing in
  every case, which is what stops a future edit from branching on the result.
- `claim` resolves the token before it hashes anything. It is unauthenticated
  and Argon2 costs 19 MiB a call, so an endpoint that hashed first would be a
  denial of service with a password field on it.
"""

from __future__ import annotations

import logging
import posixpath
import sqlite3
from collections.abc import Awaitable, Callable
from typing import Any

from fastapi import BackgroundTasks, FastAPI, HTTPException, Request, Response
from fastapi.responses import JSONResponse

from mtglab import config
from mtglab.api.deps import ANONYMOUS, LOCAL, Scope, UserScope
from mtglab.auth import (
    db,
    invites,
    mail,
    passwords,
    ratelimit,
    sessions,
    tokens,
    users,
)

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
#
# `/api/auth/reset` and `/api/auth/claim` are here because a person who cannot
# log in is the only person who ever needs them. They are the two routes on
# this list that *change* something, so both are rate limited, and `claim`
# proves possession of a 256-bit token before it does any work.
PUBLIC_PATHS = frozenset({
    "/api/health",
    "/api/auth/login",
    "/api/auth/logout",
    "/api/auth/me",
    "/api/auth/reset",
    "/api/auth/claim",
    # Public for the same reason `claim` is: its caller is holding an emailed
    # link and by definition has no session. It reads a token and spends
    # nothing.
    "/api/auth/claim/preview",
})

# Everything an admin does lives under one prefix, and the middleware refuses it
# to anybody else *before routing* — the same mechanism as `PUBLIC_PATHS` and
# for the same reason. An admin route is then protected by where it is mounted
# rather than by what its author remembered, so the one somebody adds in a year
# is covered by having a path rather than by having a decorator.
#
# `deps.admin` is the second check, on the route functions themselves. It is
# what would catch an admin route mounted outside this prefix, which the
# structural rule by construction cannot see. ADR 17.
ADMIN_PREFIX = "/api/admin"

# The only thing `POST /api/auth/reset` ever says. A constant rather than a
# literal in the handler, because the one way this endpoint leaks is by
# answering two different things, and a single name is harder to fork than a
# string typed twice.
RESET_ANSWER = ("if that address has an account, a link is on its way -- "
                "check your mail, and the spam folder")


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


def is_admin_path(path: str) -> bool:
    """Whether this path may be served only to an admin.

    Normalised through the same function as `is_public`, and for the same
    reason: the check must never be *less* strict than the router. A prefix test
    on the raw path would let `/api/./admin/users` through here and straight
    into the route that matches it.

    The `/` in the second test is what stops `/api/administrators` — a route
    nobody has written — from being silently swept in. It is cheap to be exact
    now and confusing to discover later.
    """
    normalised = normalise_path(path)
    return (normalised == ADMIN_PREFIX
            or normalised.startswith(ADMIN_PREFIX + "/"))


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


def install(app: FastAPI, *, require: bool, secure_cookies: bool,
            email_sender: mail.EmailSender | None = None) -> None:
    """Add the authentication middleware and the five auth routes.

    `email_sender` is the ADR 16 seam reaching the edge. `None` means "decide
    from the environment, when a message is actually being sent" — which is
    what a real process wants, and which is also why no test in this suite
    sends mail: the tests pass a recorder instead.
    """

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
        # After the 401 and not before it, so an anonymous request for an admin
        # path is told it needs a session rather than that it needs to be
        # somebody else. 403 rather than 404 -- see `deps.admin` for why ADR 5's
        # rule does not reach this case.
        if not caller.is_admin and is_admin_path(request.url.path):
            _LOG.warning("refused %s to %r, who is not an admin",
                         normalise_path(request.url.path), caller.username)
            return JSONResponse(status_code=403,
                                content={"detail": "admin only"})
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

    def _sender() -> mail.EmailSender:
        return email_sender if email_sender is not None else mail.sender_from_env()

    def _deliver_reset(email: str) -> None:
        """Look the address up and send, after the response has already gone.

        Everything about this function is downstream of one requirement: the
        caller must not be able to tell a hit from a miss. Doing the lookup
        here rather than in the handler means the *response time* carries no
        signal either — a hit costs a database read and an HTTPS round trip to
        the mail provider, a miss costs neither, and neither of them happens
        while anybody is waiting.

        It also means a mail outage cannot become a 500 that says "this
        address exists, and something went wrong sending to it". The failure
        is a log line for the maintainer, which is who can act on it.
        """
        try:
            with db.connection() as con:
                invites.send_reset(con, email, sender=_sender())
        except (mail.EmailNotSent, mail.EmailNotConfigured, OSError) as exc:
            # Every one of these carries a status code, a configuration
            # complaint or a socket error -- and none of them carries a
            # recipient, which is what makes this line safe to write. ADR 16,
            # and see `mail._describe` for how that is kept true.
            _LOG.error("password reset could not be delivered: %s", exc)

    @app.post("/api/auth/reset", status_code=202)
    def request_reset(payload: dict[str, Any], request: Request,
                      background: BackgroundTasks) -> dict[str, Any]:
        """Ask for a password-reset link. Always the same answer (ADR 16).

        202 rather than 200, and the code is the honest one: the request has
        been accepted and nothing about the outcome is being reported. That is
        the whole design, not a limitation of it.

        The 429 is not an exception to "always the same answer". Every request
        is counted, hit or miss — a reset has no success to clear the budget
        with — so being throttled says something about how often *this* client
        and *this* mailbox have been asked about, and nothing about whether an
        account is behind it.
        """
        email = str(payload.get("email") or "").strip()
        if not email:
            raise HTTPException(status_code=422, detail="an email is required")

        address = client_address(request)
        keys = ((ratelimit.email_key(email), ratelimit.RESET_PER_MAILBOX),
                (ratelimit.address_key(address, scope="reset"),
                 ratelimit.RESET_PER_ADDRESS))

        with db.connection() as con:
            for key, limit in keys:
                if ratelimit.exhausted(con, key, limit):
                    wait = ratelimit.retry_after(con, key, limit)
                    _LOG.warning("password reset throttled from %s", address)
                    raise HTTPException(
                        status_code=429,
                        detail="too many requests -- wait and try again",
                        headers={"Retry-After": str(wait)})
            for key, limit in keys:
                ratelimit.record_failure(con, key, limit)

        background.add_task(_deliver_reset, email)
        # No address, and no "for ada@example.com". ADR 16 keeps them out of
        # logs; this line is where the temptation to be helpful would put one.
        _LOG.info("password reset requested from %s", address)
        return {"detail": RESET_ANSWER}

    @app.post("/api/auth/claim")
    def claim(payload: dict[str, Any], request: Request) -> dict[str, Any]:
        """Redeem an invite or reset link by choosing a password.

        **It does not log you in.** A cookie here would make an emailed link
        into a session-minting endpoint, and the thing it would save is one
        trip through a login form that has to work anyway. What comes back is
        the username, so that form can be filled in.

        Every session for the account ends inside `tokens.redeem`, in the same
        transaction that sets the password.

        An **invite** may also carry a `username`, which is the holder naming
        themselves rather than living with the one derived from their email
        address. A reset may not; `redeem` gates that on the token's own
        purpose, read from the database, so this handler never has to be the
        thing that remembers.
        """
        token = str(payload.get("token") or "").strip()
        password = str(payload.get("password") or "")
        chosen = str(payload.get("username") or "").strip() or None
        if not token or not password:
            raise HTTPException(status_code=422,
                                detail="a token and a password are required")

        address = client_address(request)
        key = ratelimit.address_key(address, scope="claim")

        with db.connection() as con:
            if ratelimit.exhausted(con, key, ratelimit.CLAIM_PER_ADDRESS):
                wait = ratelimit.retry_after(con, key,
                                             ratelimit.CLAIM_PER_ADDRESS)
                raise HTTPException(
                    status_code=429,
                    detail="too many attempts -- wait and try again",
                    headers={"Retry-After": str(wait)})
            try:
                user = tokens.redeem(con, token, password, username=chosen)
            except (passwords.WeakPassword, users.InvalidUsername,
                    tokens.WrongPurpose) as exc:
                # Not counted and not a refusal of the link: the token is
                # intact and the sensible next move is a longer password, or a
                # different handle.
                raise HTTPException(status_code=422, detail=str(exc)) from exc
            except users.UserExists as exc:
                # 409, and the link is still live -- `redeem` rolls the token
                # spend back with the failed rename, so "that name is taken"
                # is a retryable answer rather than a dead invite.
                raise HTTPException(status_code=409, detail=str(exc)) from exc
            except tokens.TokenError as exc:
                ratelimit.record_failure(con, key,
                                         ratelimit.CLAIM_PER_ADDRESS)
                _LOG.warning("rejected a claim from %s: %s", address, exc)
                raise HTTPException(status_code=400, detail=str(exc)) from exc
            ratelimit.clear(con, key)

        _LOG.info("password set for %r from %s", user.username, address)
        return {"detail": "password set -- you can sign in now",
                "username": user.username}

    @app.post("/api/auth/claim/preview")
    def claim_preview(payload: dict[str, Any],
                      request: Request) -> dict[str, Any]:
        """What kind of link this is, and the name it currently carries.

        **A POST, and the token is in the body.** A GET would put it in a query
        string, and the entire reason `invites.claim_link` hides it in a URL
        *fragment* is that a fragment reaches no server's access log. Reading a
        token back over the query string would undo that at the first hop, for
        the convenience of a verb.

        It exists because the claim screen has a token and nothing else, and
        two things now depend on which kind of link arrived: an invite offers a
        username field, a reset must not (`tokens.redeem` refuses it either
        way — this is so the form does not offer what the server will refuse).

        **It spends nothing and changes nothing.** A valid link previewed ten
        times is still a valid link; only `claim` consumes one.

        What comes back is the purpose and the username, and deliberately not
        the email address. The holder of the token knows their own address, but
        ADR 16 does not put one in a response that nobody authenticated for,
        and a preview is exactly that.
        """
        token = str(payload.get("token") or "").strip()
        if not token:
            raise HTTPException(status_code=422, detail="a token is required")

        address = client_address(request)
        # Its own bucket rather than `claim`'s. Sharing one would let a page
        # that previews on mount spend the budget a person needs to actually
        # redeem, and the two failures mean different things.
        key = ratelimit.address_key(address, scope="claim-preview")

        with db.connection() as con:
            if ratelimit.exhausted(con, key, ratelimit.CLAIM_PER_ADDRESS):
                wait = ratelimit.retry_after(con, key,
                                             ratelimit.CLAIM_PER_ADDRESS)
                raise HTTPException(
                    status_code=429,
                    detail="too many attempts -- wait and try again",
                    headers={"Retry-After": str(wait)})
            try:
                resolved = tokens.lookup(con, token)
            except tokens.TokenError as exc:
                ratelimit.record_failure(con, key,
                                         ratelimit.CLAIM_PER_ADDRESS)
                raise HTTPException(status_code=400, detail=str(exc)) from exc

            account = users.get_by_id(con, resolved.user_id)
            if account is None or account.disabled:
                # The same sentence `redeem` gives, for the same reason: a
                # disabled account's link is not a link whose holder is owed
                # an explanation of why.
                raise HTTPException(status_code=400,
                                    detail="that link is not valid")
            ratelimit.clear(con, key)

        return {"purpose": str(resolved.purpose), "username": account.username}

    @app.get("/api/auth/me")
    def me(caller: Scope) -> dict[str, Any]:
        """Who the caller is, and whether this instance requires anyone to be.

        Public, and the three flags are separate on purpose: a frontend needs to
        tell "you are logged out of an instance that wants a login" from "this
        instance has no login", and one collapsed boolean makes the local app
        render a sign-in form it has no server for.

        `is_admin` is reported at the top level as well as inside `user`, and
        the difference is exactly the case `user` cannot express: with auth off
        the caller is `LOCAL` — nobody in particular, and an admin, because
        there is nobody else for it to be true relative to. A client deriving
        that from `auth_required` would be reimplementing `deps.LOCAL` in
        TypeScript, so the server answers it instead. It is never a grant: the
        middleware reads the scope, not this response.
        """
        return {
            "auth_required": require,
            "authenticated": caller.authenticated,
            "is_admin": caller.is_admin,
            "user": {"id": caller.user_id, "username": caller.username,
                     "is_admin": caller.is_admin}
            if caller.authenticated else None,
        }
