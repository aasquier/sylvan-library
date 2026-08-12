"""The adversarial test: log in as B, ask for A's things, get nothing.

ADR 5 calls this the single highest-value test in the whole auth story, and is
specific about why the obvious way of writing it does not work. A hand-written
list of endpoints to check will miss the one somebody adds next year — which is
precisely the endpoint that will not have been thought about. So the suite is
**generated from the route table**: every `/api` route the app declares must
appear in one of the three classifications below, and one that appears in none
fails `test_every_route_is_classified` with instructions.

The three classifications, and what each one is checked to actually do:

- **public** — reachable with no session. Read straight out of
  `api/auth.py:PUBLIC_PATHS`, so the test and the middleware cannot disagree
  about what is public; a route added to one is a route added to both.
- **shared** — needs a session, and then shows the same thing to everyone. The
  curated six are the library, not anybody's property (ADR 1). Each entry
  carries the reason it is shared, so the classification is an argument rather
  than a place to put routes that are inconvenient to isolate.
- **user-scoped** — needs a session and belongs to one person. Checked
  adversarially: A creates it, B asks for it, B must get **404 and not 403**.
  403 confirms the resource exists, which is the leak.

Every non-public route is also requested with no cookie at all and must answer
401. That check is what catches an endpoint declared without protection, and it
needs no per-route knowledge: the middleware refuses before routing, so the
route does not have to know it is being protected.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient  # noqa: E402

from mtglab import config  # noqa: E402
from mtglab.api import jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.api.auth import PUBLIC_PATHS  # noqa: E402
from mtglab.auth import db, users  # noqa: E402

PASSWORD_A = "correct-horse-battery-staple"
PASSWORD_B = "a-different-long-passphrase"

# Needs a session; shows everyone the same thing once you have one. The value
# is the justification -- if it does not read as a reason, the route is
# probably user-scoped and mis-filed.
SHARED = {
    "/api/decks": "the curated library (ADR 1); the same six for everyone",
    "/api/decks/import": "writes into the curated library, which is shared",
    "/api/decks/{slug}": "one deck out of that same shared library",
    "/api/decks/{slug}/validate": "the gate's verdict on a shared deck",
    "/api/decks/{slug}/stats": "counts over a shared deck",
    "/api/decks/{slug}/swap": "edits a shared deck",
    "/api/decks/{slug}/cards": "edits a shared deck",
    "/api/decks/{slug}/cards/{name}": "edits a shared deck",
    "/api/decks/{slug}/notes/{key}": "edits a shared deck",
    "/api/decks/{slug}/suggestions": "derived from a shared deck and the corpus",
    "/api/decks/{slug}/interview": "questions about a shared deck's card",
    "/api/cards/search": "the public Scryfall corpus",
    "/api/sets/upcoming": "Scryfall's own release calendar",
    "/api/colors": "a fixed taxonomy, no data at all",
    "/api/colors/progress": "scored over the shared library",
    "/api/claude": "whether the surface is configured on this instance",
    "/api/sim/mana": "submits a job against a shared deck; the job is scoped",
    "/api/sim/lands": "submits a job too, and the job it returns is scoped",
}

# Belongs to one person. Each entry says how to make one as user A and where to
# find it, and the test then tries to reach it as user B.
USER_SCOPED = {
    "/api/jobs": "list",
    "/api/jobs/{job_id}": "item",
}


@pytest.fixture
def instance(tmp_path):
    """An app with auth required, on a scratch `app.db`, holding A and B."""
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        try:
            alice = users.create(con, "alice", password=PASSWORD_A)
            bob = users.create(con, "bob", password=PASSWORD_B)
        finally:
            con.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as client:
            yield client, alice, bob


def login(client, username: str, password: str) -> None:
    response = client.post("/api/auth/login",
                           json={"username": username, "password": password})
    assert response.status_code == 200, response.text


def api_routes(client) -> list:
    """Every declared `/api` route, from the app itself."""
    return [r for r in client.app.routes
            if getattr(r, "path", "").startswith("/api")]


# ------------------------------------------------------- the generated sweep

def test_every_route_is_classified(instance):
    """A new endpoint fails the suite until somebody says what it is.

    This is the check that makes the rest of the file hold in a year. It does
    not test behaviour -- it tests that nobody has quietly added a route the
    isolation tests are not looking at.
    """
    client, _, _ = instance
    known = PUBLIC_PATHS | set(SHARED) | set(USER_SCOPED)
    declared = {r.path for r in api_routes(client)}
    unclassified = declared - known
    assert not unclassified, (
        "these routes are not classified in tests/test_isolation.py:\n  "
        + "\n  ".join(sorted(unclassified))
        + "\n\nAdd each to PUBLIC_PATHS (in api/auth.py), to SHARED with the "
          "reason it is the same for everyone, or to USER_SCOPED -- which "
          "requires it to answer 404 for another user.")


def test_classification_has_no_ghosts(instance):
    """The other direction: a classified route that no longer exists.

    Without this, a renamed endpoint leaves its old entry behind and the sweep
    goes on reporting a pass for a route nobody serves.
    """
    client, _, _ = instance
    declared = {r.path for r in api_routes(client)}
    stale = (set(SHARED) | set(USER_SCOPED) | {p for p in PUBLIC_PATHS}) - declared
    assert not stale, f"classified but not declared: {sorted(stale)}"


@pytest.mark.parametrize("path", sorted(set(SHARED) | set(USER_SCOPED)))
def test_no_session_is_refused(instance, path):
    """Every non-public route answers 401 without a cookie.

    Generated from the classification, and it is the check that catches an
    endpoint added without protection: the middleware refuses before routing,
    so this passes for routes that have never heard of authentication and fails
    the moment one is added to the public list by accident.

    A GET is enough. The middleware runs ahead of method matching, so a route
    that only accepts POST still answers 401 rather than 405 -- which is itself
    the desired behaviour, since 405 would confirm the path exists.
    """
    client, _, _ = instance
    response = client.get(path.replace("{slug}", "arahbo-cats")
                                .replace("{name}", "Forest")
                                .replace("{key}", "mulligan")
                                .replace("{job_id}", "deadbeef"))
    assert response.status_code == 401, (
        f"{path} answered {response.status_code} to a request with no session")
    assert "detail" in response.json()


@pytest.mark.parametrize("path", sorted(PUBLIC_PATHS))
def test_public_routes_need_no_session(instance, path):
    """The allowlist is checked too -- in the other direction.

    A route that is meant to be public and answers 401 breaks the login page,
    and nothing else in this file would notice.

    The assertion is on the middleware's own message rather than on the status,
    because `/api/auth/login` answers 401 for bad credentials and that 401 is
    the endpoint working. What must not appear is the refusal that happens
    before the handler runs.
    """
    client, _, _ = instance
    response = client.get(path) if path != "/api/auth/login" \
        else client.post(path, json={"username": "nobody", "password": "x"})
    body = response.json() if response.headers.get("content-type", "") \
        .startswith("application/json") else {}
    assert body.get("detail") != "authentication required", \
        f"{path} is on the public list but the middleware refused it"


# --------------------------------------------------- the adversarial part

def test_bs_request_for_as_job_is_a_404(instance):
    """The whole point of the file. A owns a job; B asks for it by id.

    404 and not 403: a 403 would confirm that the id names something real,
    which is the fact being protected. ADR 5 is explicit about this and it is
    the one assertion here that would still matter if everything else were
    deleted.
    """
    client, alice, _ = instance
    job = jobs.submit("test", lambda progress: "alice's result",
                      label="alice's deck", owner=alice.id)

    login(client, "bob", PASSWORD_B)
    response = client.get(f"/api/jobs/{job.id}")
    assert response.status_code == 404
    assert job.id not in response.text or "no job" in response.text


def test_a_can_see_their_own_job(instance):
    """The control. Without it, a handler that 404s everything passes above."""
    client, alice, _ = instance
    job = jobs.submit("test", lambda progress: "alice's result",
                      label="alice's deck", owner=alice.id)

    login(client, "alice", PASSWORD_A)
    response = client.get(f"/api/jobs/{job.id}")
    assert response.status_code == 200
    assert response.json()["id"] == job.id


def test_the_job_list_is_one_persons(instance):
    """Listing leaks as readily as fetching, and is easier to forget."""
    client, alice, bob = instance
    mine = jobs.submit("test", lambda progress: 1, label="alice", owner=alice.id)
    theirs = jobs.submit("test", lambda progress: 2, label="bob", owner=bob.id)

    login(client, "bob", PASSWORD_B)
    body = client.get("/api/jobs").json()
    ids = {j["id"] for j in body}
    assert theirs.id in ids
    assert mine.id not in ids
    # The label names a deck. A leaked list is a leaked list of what somebody
    # is working on, not just of opaque identifiers.
    assert "alice" not in client.get("/api/jobs").text


def test_a_job_submitted_over_http_is_owned_by_the_caller(instance):
    """The ownership stamp has to happen at submission, not at read time.

    Reached through the route rather than through `jobs.submit` directly,
    because the thing being checked is that the *handler* passes the scope --
    a handler that forgot would leave `owner=None` and the job would be
    visible to nobody, which is a bug that reads as working isolation.
    """
    client, alice, _ = instance
    login(client, "alice", PASSWORD_A)
    submitted = client.post("/api/sim/mana",
                            json={"slug": "no-such-deck", "games": 1})
    assert submitted.status_code == 200
    job_id = submitted.json()["id"]

    assert client.get(f"/api/jobs/{job_id}").status_code == 200
    client.post("/api/auth/logout")
    login(client, "bob", PASSWORD_B)
    assert client.get(f"/api/jobs/{job_id}").status_code == 404


# ----------------------------------------------------------- shared routes

def test_shared_routes_are_reachable_by_any_account(instance):
    """The other half of the classification: shared really is shared.

    Checked on one representative rather than all of them -- the sweep above
    already proves every shared route needs a session, and this proves the
    session is enough. What would be wrong is a route filed as shared that
    quietly returns different data per user; that is a claim about handlers
    which do not read the scope at all.
    """
    client, _, _ = instance
    login(client, "alice", PASSWORD_A)
    as_alice = client.get("/api/decks").json()
    client.post("/api/auth/logout")
    login(client, "bob", PASSWORD_B)
    as_bob = client.get("/api/decks").json()
    assert as_alice == as_bob


def test_shared_entries_carry_a_reason():
    """The classification is meant to be an argument, not a bucket."""
    for path, reason in SHARED.items():
        assert len(reason) > 15, f"{path} is filed as shared without a reason"


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
