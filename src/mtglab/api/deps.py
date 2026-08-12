"""Request scope: who is asking, and what they are allowed to see.

There is one accessor and everything user-scoped goes through it. That is not a
style preference — `docs/HOSTING.md` §1 and ADR 5 both say per-user isolation
fails when the `user_id` filter is sprinkled across handlers and one is
forgotten, and the fix is that no handler ever writes the filter itself.

Three dependencies, in a deliberate order:

    scope(request)     -> UserScope  who is asking
    admin(scope)       -> UserScope  ...and 403 unless they administer this box
    deck_source(scope) -> DeckSource what that person may see

`deck_source` was here first, returning the curated file-backed library and
taking no arguments — introduced before it had anything to isolate, which was
the only time it was cheap. It now derives from the scope, and the thirteen
handlers that depend on it did not change. That was the whole bet, and it paid.

The scope itself is resolved once per request by the middleware in
`api/auth.py`, not here: authentication has to happen *before* routing, so that
a route nobody remembered to protect is refused rather than served. This module
reads what the middleware left on `request.state`.

Three shapes of scope, and the difference between the first two matters:

- **LOCAL** — auth is switched off. `mtglab ui` on a laptop, one person, full
  access, no session. This is the default and the only way the app has ever run.
- **ANONYMOUS** — auth is on and the caller has no valid session. It reaches
  only the public routes; the middleware refuses everything else before a
  handler sees it.
- A real user, from a session cookie.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Annotated

from fastapi import Depends, HTTPException, Request

from mtglab.decks.source import DeckSource, FileDeckSource


@dataclass(frozen=True)
class UserScope:
    """Who this request is on behalf of.

    Carries an id and a name, never a password hash and never a session token.
    A scope is what handlers are allowed to know about the caller, so anything
    on it is something the whole API may see.
    """

    user_id: int | None = None
    username: str | None = None
    is_admin: bool = False
    authenticated: bool = False


# Auth off: one person, on their own machine, holding the file the app reads.
# `is_admin` is true because there is nobody else for it to be true relative to.
LOCAL = UserScope(is_admin=True, authenticated=False)

# Auth on, no session. Reaches the public routes and nothing else.
ANONYMOUS = UserScope()


def scope(request: Request) -> UserScope:
    """The caller, as resolved by the middleware.

    The `LOCAL` fallback covers an app assembled without the middleware — a
    unit test building routes directly. It is deliberately the permissive
    default: an app with no authentication layer installed is the local
    single-user tool, which is what this project is when nobody has deployed it.
    """
    return getattr(request.state, "scope", LOCAL)


Scope = Annotated[UserScope, Depends(scope)]


def admin(caller: Scope) -> UserScope:
    """The caller, if they administer this instance. 403 if they do not.

    **This is the second of two checks, not the only one.** `api/auth.py`
    refuses anything under `/api/admin` to a non-admin from the middleware,
    before routing, for the same reason authentication is enforced there: a
    dependency has to be remembered on each new route, and the route somebody
    adds in a year is the one that will not have it.

    So what is this for? The case the prefix rule cannot see — an admin route
    mounted somewhere else by mistake. Then this is the only thing between it
    and every logged-in account. It costs an attribute read. ADR 17.

    403 and not 404, which is a deliberate exception to ADR 5's rule and worth
    the sentence: that rule protects resources whose *existence* is the secret,
    and an admin route's existence is published in a public repository. Whether
    the caller is an admin is already on `/api/auth/me`. A 404 here would hide
    nothing and would cost every future debugging session the difference
    between "not logged in", "not an admin", and "typo in the path".
    """
    if not caller.is_admin:
        raise HTTPException(status_code=403, detail="admin only")
    return caller


Admin = Annotated[UserScope, Depends(admin)]


def deck_source(caller: Scope) -> DeckSource:
    """The decks this request may see.

    Still the curated file-backed library for everyone, because that is still
    the only tier that exists (ADR 1, and `docs/HOSTING.md` §6 step 6 is where
    the second one lands). What has changed is that the answer is now a
    function of *who is asking*, so adding `SqlDeckSource` for a user's own
    decks is an edit to this function and to nothing else.
    """
    del caller       # named, unused, and the signature is the point
    return FileDeckSource()
