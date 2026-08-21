"""The middleware properties, generated from `routes.json` -- over the wire.

What `tests/test_isolation.py` proves against the FastAPI route table
in-process, this proves against whatever is listening: every protected
route answers 401 with no session, every admin route answers 403 to a
signed-in non-admin, no public route is refused, and every response carries
the hardening headers. The Go front door has to pass this file before a
single route family moves behind it (docs/go-migration/PLAN.md, Phase 2).
"""

from __future__ import annotations

import pytest

from contract import checks
from contract.harness import DECK_OWNER, DECK_SLUG
from contract.routes import ROUTES


@pytest.mark.parametrize("path", sorted(ROUTES.protected))
def test_every_protected_route_refuses_without_a_session(instance, path):
    client = instance.as_user(None)
    checks.check_refused_without_session(
        client.get(ROUTES.concrete(path)), path)


@pytest.mark.parametrize("path", sorted(ROUTES.public))
def test_public_routes_are_not_refused(instance, path):
    client = instance.as_user(None)
    if path == "/api/auth/login":
        response = client.post(path, json={"username": "nobody", "password": "x"})
    elif path.startswith("/api/auth/") and path not in ("/api/auth/me",):
        # The other public routes take a body; an empty one is refused by
        # the handler, which is still not the middleware refusing it.
        response = client.post(path, json={})
    else:
        response = client.get(path)
    checks.check_public_not_refused(response, path)


@pytest.mark.parametrize("path", sorted(ROUTES.admin))
def test_admin_routes_refuse_a_signed_in_non_admin(instance, path):
    client = instance.as_user("bob")
    checks.check_admin_refused(client.get(ROUTES.concrete(path)), path)


def test_an_admin_reaches_the_prefix(instance):
    """The control: a middleware that 403s everybody passes the sweep above."""
    client = instance.as_user("alice")
    response = client.get("/api/admin/users")
    assert response.status_code == 200, response.text
    names = {u["username"] for u in response.json()["users"]}
    assert {"alice", "bob"} <= names


def test_the_admin_prefix_covers_paths_no_route_claims(instance):
    """The refusal must not depend on the route table, or a non-admin could
    learn which admin routes exist by which ones answered differently."""
    bob = instance.as_user("bob")
    checks.check_admin_refused(bob.get("/api/admin/nothing-here"),
                               "/api/admin/nothing-here")
    alice = instance.as_user("alice")
    assert alice.get("/api/admin/nothing-here").status_code != 403


def test_a_dotted_path_does_not_slip_past_the_prefix(instance):
    """`/api/./admin/users` normalises to an admin path here and at the
    router; a check less strict than the router is a check with a way round."""
    bob = instance.as_user("bob")
    for path in ("/api/./admin/users", "/api//admin/users"):
        checks.check_admin_refused(bob.get(path), path)


@pytest.mark.parametrize("user, method, path", [
    (None, "GET", "/api/decks"),                      # the middleware's 401
    (None, "GET", "/api/health"),                     # a routed JSON 200
    (None, "GET", "/"),                               # the shell
    (None, "GET", "/tarot/00-fool.webp"),             # a packaged asset
    ("alice", "GET", "/api/no-such-route"),           # a routed JSON 404
    ("bob", "GET", "/api/admin/users"),               # the middleware's 403
    ("alice", "GET", f"/api/decks/{DECK_OWNER}/{DECK_SLUG}"),
], ids=lambda v: str(v))
def test_every_response_carries_the_security_headers(instance, user, method,
                                                     path):
    client = instance.as_user(user)
    response = client.request(method, path)
    checks.check_security_headers(response, f"{method} {path} as {user}")
