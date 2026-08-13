"""The adversarial test: log in as B, ask for A's things, get nothing.

ADR 5 calls this the single highest-value test in the whole auth story, and is
specific about why the obvious way of writing it does not work. A hand-written
list of endpoints to check will miss the one somebody adds next year — which is
precisely the endpoint that will not have been thought about. So the suite is
**generated from the route table**: every `/api` route the app declares must
appear in one of the three classifications below, and one that appears in none
fails `test_every_route_is_classified` with instructions.

The four classifications, and what each one is checked to actually do:

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
- **admin** — needs a session belonging to somebody who administers this
  instance. Added by [ADR 17](../docs/adr/0017-the-maintainer-is-named-in-the-environment.md),
  and checked from both ends: a logged-in non-admin must get **403**, and the
  classification must agree with `api/auth.py:ADMIN_PREFIX` in both directions,
  so neither an admin route filed as shared nor an admin route mounted outside
  the prefix can pass.

Every non-public route is also requested with no cookie at all and must answer
401. That check is what catches an endpoint declared without protection, and it
needs no per-route knowledge: the middleware refuses before routing, so the
route does not have to know it is being protected.

**A is an admin and B is not**, which is why the adversarial tests below say
more than they used to: every one of them now also establishes that
administering the instance is not the same thing as reaching another account's
work. `test_an_admin_still_cannot_see_another_persons_job` is that stated
directly, and it is the assertion that would catch somebody "fixing" the
isolation filter by exempting admins from it.
"""

import ast
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
from mtglab.api.auth import ADMIN_PREFIX, PUBLIC_PATHS  # noqa: E402
from mtglab.auth import db, users  # noqa: E402

PASSWORD_A = "correct-horse-battery-staple"
PASSWORD_B = "a-different-long-passphrase"

SRC = Path(__file__).resolve().parents[1] / "src" / "mtglab"

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
    "/api/decks/{slug}/commander": "corpus facts about a shared deck's commander",
    "/api/decks/{slug}/printings": "which arts a shared deck's commander has",
    "/api/decks/{slug}/interview": "questions about a shared deck's card",
    # Both verbs, one entry: the dossier is about the commander of a deck
    # everybody can already see, and ADR 19 keys it on the card's oracle id
    # precisely so it is shared rather than per-person.
    "/api/decks/{slug}/dossier": "who a shared deck's commander is",
    "/api/cards/search": "the public Scryfall corpus",
    "/api/sets/upcoming": "Scryfall's own release calendar",
    "/api/colors": "a fixed taxonomy, no data at all",
    "/api/colors/progress": "scored over the shared library",
    "/api/colors/{key}": "a fixed taxonomy plus public corpus cards",
    "/api/glossary": "fixed reference prose, no data at all",
    "/api/claude": "whether the surface is configured on this instance",
    # ADR 20's two. Neither takes a deck or a slug — the conversation is about
    # the person and is held by their browser, so there is nothing here
    # belonging to one account for another to reach. What comes back is a
    # suggestion; making the deck goes through the shared create route.
    "/api/claude/theme": "a conversation the client holds; no stored state",
    # Submits a job rather than answering — 226 seconds is longer than a hosted
    # proxy will hold a POST. Filed here for the same reason the two sim routes
    # are: the *submission* is shared (there is no deck and nothing personal on
    # the server to reach), and the job it hands back is scoped by `jobs.get`.
    "/api/claude/theme/proposal": "submits a job; colours and commanders out of "
                                  "the shared corpus, and the job is scoped",
    "/api/sim/mana": "submits a job against a shared deck; the job is scoped",
    "/api/sim/lands": "submits a job too, and the job it returns is scoped",
}

# Belongs to one person. Each entry says how to make one as user A and where to
# find it, and the test then tries to reach it as user B.
USER_SCOPED = {
    "/api/jobs": "list",
    "/api/jobs/{job_id}": "item",
}

# Needs a session belonging to an admin (ADR 17). The value is the
# justification, same as SHARED -- an admin route is a route that administers
# *the instance*, and anything that merely feels sensitive belongs in one of the
# other three.
#
# Every entry must live under ADMIN_PREFIX and every route under that prefix
# must appear here; `test_admin_classification_matches_the_prefix` checks both
# directions. That is what makes the middleware and this file one statement
# rather than two that can drift.
ADMIN = {
    "/api/admin/users": "every account on the instance, addresses included",
    "/api/admin/users/{username}": "grants admin, disables and re-enables, "
                                   "and deletes an account outright",
    "/api/admin/users/{username}/reset": "mails somebody a password link",
    "/api/admin/users/{username}/sessions": "signs an account out everywhere",
}

# The one place `include_email=True` is allowed to appear, now that ADR 17 has
# widened it from "the CLI only". See `test_addresses_are_serialised_in_two_places`.
EMAIL_CALLERS = frozenset({"cli.py", "api/admin.py"})


@pytest.fixture
def instance(tmp_path):
    """Auth on, a scratch `app.db`, and two accounts: A administers, B does not.

    A being the admin is deliberate and it is what makes the adversarial tests
    below stronger than their names suggest -- see the module docstring. It also
    means every admin route can be checked from both sides with the accounts
    that are already here.
    """
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        try:
            alice = users.create(con, "alice", password=PASSWORD_A,
                                 is_admin=True)
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


def concrete(path: str) -> str:
    """A templated route path with something plausible in every placeholder.

    The values do not have to resolve to anything. Every check that uses this
    asserts on a refusal that happens *before* the handler runs, so a real slug
    and a nonsense one produce the same answer -- and one that produced a
    different answer would be the bug.
    """
    return (path.replace("{slug}", "arahbo-cats")
                .replace("{name}", "Forest")
                .replace("{key}", "mulligan")
                .replace("{job_id}", "deadbeef")
                .replace("{username}", "alice"))


# ------------------------------------------------------- the generated sweep

def test_every_route_is_classified(instance):
    """A new endpoint fails the suite until somebody says what it is.

    This is the check that makes the rest of the file hold in a year. It does
    not test behaviour -- it tests that nobody has quietly added a route the
    isolation tests are not looking at.
    """
    client, _, _ = instance
    known = PUBLIC_PATHS | set(SHARED) | set(USER_SCOPED) | set(ADMIN)
    declared = {r.path for r in api_routes(client)}
    unclassified = declared - known
    assert not unclassified, (
        "these routes are not classified in tests/test_isolation.py:\n  "
        + "\n  ".join(sorted(unclassified))
        + "\n\nAdd each to PUBLIC_PATHS (in api/auth.py), to SHARED with the "
          "reason it is the same for everyone, to USER_SCOPED -- which "
          "requires it to answer 404 for another user -- or to ADMIN, which "
          "requires it to live under ADMIN_PREFIX and answer 403 to anybody "
          "else.")


def test_classification_has_no_ghosts(instance):
    """The other direction: a classified route that no longer exists.

    Without this, a renamed endpoint leaves its old entry behind and the sweep
    goes on reporting a pass for a route nobody serves.
    """
    client, _, _ = instance
    declared = {r.path for r in api_routes(client)}
    classified = set(SHARED) | set(USER_SCOPED) | set(ADMIN) | set(PUBLIC_PATHS)
    stale = classified - declared
    assert not stale, f"classified but not declared: {sorted(stale)}"


def test_admin_classification_matches_the_prefix(instance):
    """The two ways an admin route can be wrong, checked in both directions.

    An admin route filed as SHARED is one every logged-in account can reach. An
    admin route mounted outside `ADMIN_PREFIX` is one the middleware never looks
    at, and its only protection is a `deps.Admin` somebody remembered -- which
    is exactly the thing ADR 17 refuses to rely on.

    Neither is catchable by testing behaviour route by route, because both
    failures look like a route that works.
    """
    client, _, _ = instance
    misfiled = {p for p in ADMIN if not p.startswith(ADMIN_PREFIX)}
    assert not misfiled, (
        f"classified ADMIN but not under {ADMIN_PREFIX}, so the middleware "
        f"does not enforce them: {sorted(misfiled)}")

    under_prefix = {r.path for r in api_routes(client)
                    if r.path.startswith(ADMIN_PREFIX)}
    unclassified = under_prefix - set(ADMIN)
    assert not unclassified, (
        f"declared under {ADMIN_PREFIX} but not classified ADMIN: "
        f"{sorted(unclassified)}")


def test_admin_entries_carry_a_reason():
    """As with SHARED: the classification is an argument, not a bucket."""
    for path, reason in ADMIN.items():
        assert len(reason) > 15, f"{path} is filed as admin without a reason"


@pytest.mark.parametrize("path",
                         sorted(set(SHARED) | set(USER_SCOPED) | set(ADMIN)))
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
    response = client.get(concrete(path))
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


# ------------------------------------------------------------ admin routes

@pytest.mark.parametrize("path", sorted(ADMIN))
def test_an_admin_route_refuses_a_logged_in_non_admin(instance, path):
    """The generated sweep ADR 17 asked for: B has a session and is not an admin.

    A GET is enough for the same reason it is enough above -- the middleware
    runs ahead of method matching, so a POST-only route answers 403 rather than
    405, and 405 would itself be a small confirmation that the path exists.

    403 and not 404 is checked deliberately, because it is a considered
    exception to ADR 5 rather than an oversight, and a future reader
    "correcting" it should have to change a test that says so.
    """
    client, _, _ = instance
    login(client, "bob", PASSWORD_B)
    response = client.get(concrete(path))
    assert response.status_code == 403, (
        f"{path} answered {response.status_code} to a logged-in non-admin")
    assert response.json()["detail"] == "admin only"


def test_an_admin_reaches_them(instance):
    """The control. Without it, a middleware that 403s everybody passes above."""
    client, _, _ = instance
    login(client, "alice", PASSWORD_A)
    response = client.get("/api/admin/users")
    assert response.status_code == 200
    assert {u["username"] for u in response.json()["users"]} == {"alice", "bob"}


def test_an_admin_still_cannot_see_another_persons_job(instance):
    """Administering the instance is not access to another account's work.

    The direction nobody writes by accident. ADR 5's isolation is per-user and
    admin is orthogonal to it: A can disable B's account, and cannot read B's
    jobs. This is the test that fails if somebody ever "fixes" a support request
    by exempting admins from the ownership filter.
    """
    client, _, bob = instance
    theirs = jobs.submit("test", lambda progress: "bob's result",
                         label="bob's deck", owner=bob.id)

    login(client, "alice", PASSWORD_A)
    assert client.get(f"/api/jobs/{theirs.id}").status_code == 404
    assert theirs.id not in client.get("/api/jobs").text


def test_the_admin_prefix_covers_paths_no_route_claims(instance):
    """A path under the prefix that matches no route is still not for B.

    The refusal must not depend on the route table. If it did, somebody who is
    not an admin could learn which admin routes exist by seeing which ones
    answered differently -- so the check runs before routing and answers the
    same for a real path and an invented one.

    Alice's half only asserts that she is *not* refused. What she actually gets
    is the SPA shell: the catch-all in `api/app.py` serves `index.html` for any
    unmatched path, `/api/...` included. That is a separate wart and not this
    file's subject; asserting 404 here would be asserting something the app has
    never done.
    """
    client, _, _ = instance
    login(client, "bob", PASSWORD_B)
    assert client.get("/api/admin/nothing-here").status_code == 403

    login(client, "alice", PASSWORD_A)
    assert client.get("/api/admin/nothing-here").status_code != 403


def test_a_dotted_path_does_not_slip_past_the_prefix(instance):
    """`/api/./admin/users` normalises to an admin path here and at the router.

    `is_public` documents this trap for the allowlist and `is_admin_path`
    inherits it: a check that is less strict than the router is a check with a
    way around it.
    """
    client, _, _ = instance
    login(client, "bob", PASSWORD_B)
    assert client.get("/api/./admin/users").status_code == 403
    assert client.get("/api//admin/users").status_code == 403


def test_addresses_are_serialised_in_two_places(instance):
    """ADR 17 widened the email rule from one caller to two. Pin the two.

    CLAUDE.md rule 5 and ADR 16 keep addresses out of logs, artifacts and tool
    results, and `User.as_dict` makes that opt-in. Until ADR 17 the only opt-in
    was `mtglab users list`; now it is that plus the admin routes, and the rule
    is "an address may be serialised only into a response an admin
    authenticated for".

    A grep rather than a behavioural check, for the reason
    `test_claude_boundary.py` gives about its own subject: the failure being
    guarded against is the caller somebody adds later, and a test that only
    exercises today's call paths passes straight through it.
    """
    del instance
    serialisers = {path.relative_to(SRC).as_posix()
                   for path in SRC.rglob("*.py")
                   if "include_email=True" in path.read_text(encoding="utf-8")}
    assert serialisers == {"api/admin.py"}, (
        f"`as_dict(include_email=True)` is called from {sorted(serialisers)}; "
        "the only response an address belongs in is one an admin "
        "authenticated for (ADR 17).")

    # `mtglab users list` formats columns rather than serialising a dict, so it
    # reaches the column directly and the check above cannot see it. This one
    # can: any module outside the package that owns the column, naming an
    # account's address at all.
    readers = set()
    for path in SRC.rglob("*.py"):
        name = path.relative_to(SRC).as_posix()
        if name.startswith("auth/"):
            continue
        tree = ast.parse(path.read_text(encoding="utf-8"))
        if any(isinstance(node, ast.Attribute) and node.attr == "email"
               for node in ast.walk(tree)):
            readers.add(name)
    assert readers == EMAIL_CALLERS, (
        "the set of modules that read an account's email address has changed.\n"
        f"  expected: {sorted(EMAIL_CALLERS)}\n"
        f"  found:    {sorted(readers)}\n"
        "An address may be serialised only into a response an admin "
        "authenticated for, or into the maintainer's own terminal (ADR 17). "
        "If that is genuinely what the new caller does, add it here with the "
        "argument; if not, it should not be touching the field.")


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
