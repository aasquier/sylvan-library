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
import subprocess
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient

import tiny_pool
from mtglab import config
from mtglab.api import jobs
from mtglab.api.app import create_app
from mtglab.api.auth import ADMIN_PREFIX, PUBLIC_PATHS
from mtglab.auth import db, users

PASSWORD_A = "correct-horse-battery-staple"
PASSWORD_B = "a-different-long-passphrase"

SRC = Path(__file__).resolve().parents[1] / "src" / "mtglab"

# Needs a session; shows everyone the same thing once you have one. The value
# is the justification -- if it does not read as a reason, the route is
# probably user-scoped and mis-filed.
SHARED = {
    # The three collection routes. They stay shared because every account may
    # *call* them -- what differs is what comes back, and a route that filters
    # its own answer per caller cannot be checked by asking for somebody
    # else's copy of it. The per-deck routes below are where ADR 22's 404 is
    # actually enforced, and they are user-scoped now.
    "/api/decks": "every caller may list; the list is filtered to what they "
                  "may see, and carries an owner per deck (ADR 22)",
    "/api/decks/import": "writes into the caller's OWN library, never into an "
                         "owner named in the path",
    "/api/sim/mana": "submits a job against a deck the caller may see; the "
                     "owner is resolved through Library and the job is scoped",
    "/api/sim/lands": "submits a job too, same resolution, and the job is scoped",
    # ADR 35's two. The gate is the same class of thing as `/api/claude`: a
    # fact about the environment, identical for every caller. The match route
    # resolves both decks through Library exactly as the two sim routes above
    # resolve one, so somebody else's private deck is a 404 here too, and the
    # job it hands back is scoped by `jobs.get` like every other.
    "/api/forge": "whether the Forge is installed on this instance",
    "/api/sim/forge": "submits a job against two decks the caller may see; "
                      "both resolved through Library, and the job is scoped",
    "/api/cards/search": "the public Scryfall pool",
    "/api/cards/identify": "the same pool, asked the other way round -- a set\n                            code and a collector number in, a card name out. It\n                            reads no deck and writes nothing, and the photograph\n                            it came from never leaves the browser",
    "/api/sets/upcoming": "Scryfall's own release calendar",
    "/api/colors": "a fixed taxonomy, no data at all",
    "/api/colors/progress": "scored over the shared library",
    "/api/colors/{key}": "a fixed taxonomy plus public pool cards",
    "/api/glossary": "fixed reference prose, no data at all",
    "/api/lore": "fixed reference prose plus public pool cards",
    "/api/claude": "whether the surface is configured on this instance",
    # The persona roster and the deal. Both are the same class of thing as
    # `/api/colors`: a checked-in table and a shuffle, computed from nothing
    # anybody owns. The reading is seeded and stateless — the client carries
    # the seed, so there is no dealt spread on the server for anybody to reach,
    # belonging to them or to anyone else.
    "/api/claude/personas": "a fixed roster of voices, no data at all",
    "/api/tarot/reading": "a seeded shuffle of a public-domain deck; no state",
    # ADR 20's two. Neither takes a deck or a slug — the conversation is about
    # the person and is held by their browser, so there is nothing here
    # belonging to one account for another to reach. What comes back is a
    # suggestion; making the deck goes through the shared create route.
    "/api/claude/theme": "submits a job; the conversation is the client's and "
                         "nothing is stored, and the job is scoped",
    # Submits a job rather than answering — 226 seconds is longer than a hosted
    # proxy will hold a POST. Filed here for the same reason the two sim routes
    # are: the *submission* is shared (there is no deck and nothing personal on
    # the server to reach), and the job it hands back is scoped by `jobs.get`.
    "/api/claude/theme/proposal": "submits a job; colours and commanders out of "
                                  "the shared pool, and the job is scoped",
    # ADR 26. Shared for the strongest version of the theme routes' reason:
    # this one *cannot* reach a deck. It takes no owner, no slug and no
    # `DeckSource`, so there is nothing belonging to one account for another to
    # find — the question is in the request body and the answer comes back on a
    # job `jobs.get` already scopes. Note the dedupe key is per owner too, so
    # two accounts asking the same question get two jobs.
    "/api/claude/research": "submits a job; the question is the caller's and "
                            "no deck is reachable, and the job is scoped",
    # ADR 34. The one route that receives a photograph. Shared for the same
    # reason research is: no deck is reachable, the capture belongs to whoever
    # sent it and is never stored, and the resulting job is scoped by
    # `jobs.get` like every other. The dedupe key is a digest of the image and
    # is matched per owner, so two accounts photographing the same card get
    # two jobs.
    "/api/claude/scan": "submits a job; the capture is the caller's, no deck "
                        "is reachable, and the job is scoped",
    # ADR 32. A derivative is keyed on a public painting's oracle_id and a
    # fixed effect name -- the same class of thing as `/api/colors/{key}`:
    # nothing here belongs to an account, and every session sees the same
    # cache. Behind auth (not PUBLIC_PATHS) because deck pages are, and a
    # backdrop should not outlive the door it decorates.
    "/api/art/motion/{oracle_id}/{effect}": "a shared cache over public "
                                            "card art; nothing per-account",
    "/api/art/motion/{oracle_id}/{effect}/{filename}": "one file of the "
                                                       "same shared cache",
    # ADR 33. The official mana symbols out of the same kind of shared
    # runtime cache: keyed on a public symbol code, identical for every
    # session, nothing per-account to reach.
    "/api/symbols/{code}.svg": "a shared cache of public mana-symbol art; "
                               "nothing per-account",
    # The reading engine, same runtime-cache shape again: a fixed table of
    # Apache-2.0 files, byte-identical for every session, fetched by the
    # camera door and by nothing else -- plus the worker's own licence
    # notice, which is served so the pointer inside the worker resolves
    # (NOTICE.md, "Tesseract").
    "/api/ocr/{name}": "a shared cache of the Apache-2.0 OCR engine and its "
                       "notice; the same files for everyone, nothing "
                       "per-account",
}

# Belongs to one person. Each entry says how to make one as user A and where to
# find it, and the test then tries to reach it as user B.
USER_SCOPED = {
    "/api/jobs": "list",
    "/api/jobs/{job_id}": "item",
    # ADR 22. Every per-deck route is addressed `/{owner}/{slug}` and answers
    # **404** for another account's *private* deck -- not 403, which is
    # reserved for a deck the caller can already read (a shared one they do
    # not own). `test_a_private_deck_is_a_404_to_another_account` is the
    # adversarial check; the write half is `tests/test_library_write_gate.py`.
    #
    # These were filed SHARED until 2026-08-14, which was correct about
    # *reading the curated six* and silent about everything else -- the gap
    # #80 fell through. A deck is now somebody's.
    "/api/decks/{owner}/{slug}": "item",
    "/api/decks/{owner}/{slug}/validate": "item",
    "/api/decks/{owner}/{slug}/stats": "item",
    # ADR 28. A deck's history is as reachable as the deck and no more: the
    # route resolves its source through `Library`, so a private deck's log is
    # a 404 for the same reason and by the same code path the deck itself is.
    "/api/decks/{owner}/{slug}/log": "item",
    "/api/decks/{owner}/{slug}/swap": "item",
    "/api/decks/{owner}/{slug}/cards": "item",
    "/api/decks/{owner}/{slug}/cards/{name}": "item",
    # ADR 27: the entombment routes are deck writes like any other -- the
    # graveyard is part of the deck file, so its reach is the deck's.
    "/api/decks/{owner}/{slug}/entomb": "item",
    "/api/decks/{owner}/{slug}/graveyard/{name}": "item",
    "/api/decks/{owner}/{slug}/graveyard/{name}/return": "item",
    "/api/decks/{owner}/{slug}/notes/{key}": "item",
    "/api/decks/{owner}/{slug}/shared": "item",
    "/api/decks/{owner}/{slug}/suggestions": "item",
    # The wheel reads the deck (its identity and its 99) to pick a card, so
    # its reach is the deck's: spinning a stranger's private wheel would
    # confirm the deck exists.
    "/api/decks/{owner}/{slug}/wheel": "item",
    "/api/decks/{owner}/{slug}/commander": "item",
    "/api/decks/{owner}/{slug}/printings": "item",
    "/api/decks/{owner}/{slug}/interview": "item",
    "/api/decks/{owner}/{slug}/argue": "item",
    "/api/decks/{owner}/{slug}/argue/deck": "item",
    "/api/decks/{owner}/{slug}/dossier": "item",
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
    "/api/admin/stats/system": "the process, memory and volume of the box",
    "/api/admin/stats/storage": "what every store on the volume weighs",
    "/api/admin/stats/claude": "where the instance's Claude tokens went",
    "/api/admin/stats/activity": "accounts, sessions and edits across time",
    "/api/admin/stats/traffic": "requests per day by route template",
    "/api/admin/stats/fly": "the platform's own view of this instance",
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
    # A scratch deck directory with one deck, so the shared-routes test below
    # has something shared to compare. Decks are live app data (ADR 30) -- a
    # checkout has none, and reading the default `decks/` here would mean the
    # suite passed on the maintainer's machine and failed everywhere else.
    decks_root = tmp_path / "decks"
    (decks_root / "mono-green").mkdir(parents=True)
    (decks_root / "mono-green" / "deck.yaml").write_text(
        tiny_pool.mono_green_deck().dump(), encoding="utf-8")
    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_root):
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
                .replace("{username}", "alice")
                .replace("{oracle_id}", "0aae2e33-0000-4000-8000-000000000000")
                .replace("{effect}", "depth-drift")
                .replace("{filename}", "loop.webm")
                .replace("{code}", "W"))


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
    client, _alice, _ = instance
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

#: The one field on a shared payload that is allowed to differ per caller.
#: It is not deck data -- it is the answer to "may *you* change this", which
#: has to vary or it would not be an answer. Everything else about a shared
#: deck must be identical, and the test below checks both halves.
PER_CALLER_FIELDS = {"writable"}


def test_shared_routes_are_reachable_by_any_account(instance):
    """The other half of the classification: shared really is shared.

    Checked on one representative rather than all of them -- the sweep above
    already proves every shared route needs a session, and this proves the
    session is enough. What would be wrong is a route filed as shared that
    quietly returns different data per user; that is a claim about handlers
    which do not read the scope at all.

    **That claim needed narrowing.** The library is still the same six decks
    for everybody -- but since the write gate, each one carries `writable`,
    and that field is *about the caller* rather than about the deck. So the
    assertion is now the sharper of the two: every field except `writable` is
    identical, and `writable` genuinely differs. A blanket `==` would have to
    be deleted to make the gate pass, and deleting it would take the real
    check with it.
    """
    client, _, _ = instance
    login(client, "alice", PASSWORD_A)
    as_alice = client.get("/api/decks").json()
    client.post("/api/auth/logout")
    login(client, "bob", PASSWORD_B)
    as_bob = client.get("/api/decks").json()

    def without_permissions(decks: list) -> list:
        return [{k: v for k, v in d.items() if k not in PER_CALLER_FIELDS}
                for d in decks]

    assert without_permissions(as_alice) == without_permissions(as_bob)
    # And the field that is allowed to differ must actually differ, or the
    # exemption above is quietly hiding a gate that stopped working. Alice
    # administers this instance; Bob does not.
    assert [d["writable"] for d in as_alice] == [True] * len(as_alice)
    assert [d["writable"] for d in as_bob] == [False] * len(as_bob)
    assert as_alice, "the fixture must serve at least one deck to prove this"


def test_shared_entries_carry_a_reason():
    """The classification is meant to be an argument, not a bucket."""
    for path, reason in SHARED.items():
        assert len(reason) > 15, f"{path} is filed as shared without a reason"


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))


# ------------------------------------------------------- ADR 22: deck owners

@pytest.fixture
def pool(instance):
    """A queryable pool inside the instance's own scratch data directory.

    Depends on `instance` rather than standing alone so it is built *inside*
    that fixture's `config.use_paths`, and so both share one app. Creating a
    deck is refused without a card pool (a commander nobody checked is a colour
    identity nobody checked), so the ownership tests below need one — and only
    they pay the second it costs.
    """
    return tiny_pool.build(config.DB_PATH)


def _make_private_deck(client, owner: str, slug: str) -> None:
    """Create a deck in `owner`'s own tier and leave it private.

    Written through the API rather than straight into SQLite, so what the
    adversarial tests below reach for is a deck the app itself made — the
    default really is private (ADR 22), and this fails if that ever changes.
    """
    r = client.post("/api/decks", json={
        "slug": slug, "name": slug, "commander": ["Gyome, Master Chef"]})
    assert r.status_code in (200, 201), r.text
    body = client.get(f"/api/decks/{owner}/{slug}").json()
    assert body["shared"] is False, "a new deck in the SQL tier must be private"


@pytest.mark.parametrize("suffix", [
    "", "/validate", "/stats", "/suggestions", "/commander", "/printings",
])
def test_a_private_deck_is_a_404_to_another_account(instance, pool, suffix):
    """ADR 22's hard case, and the opposite call from #80's.

    A 403 here would confirm the deck exists, which is the leak ADR 5 exists to
    prevent. #80 chose 403 for a *curated* deck because its existence is
    published in a public repository and `GET /api/decks` had just listed it to
    that caller — neither is true of somebody's private brew.
    """
    client, _alice, _bob = instance
    login(client, "alice", PASSWORD_A)
    _make_private_deck(client, "alice", "secret-brew")
    client.post("/api/auth/logout")

    login(client, "bob", PASSWORD_B)
    r = client.get(f"/api/decks/alice/secret-brew{suffix}")
    assert r.status_code == 404, (
        f"bob got {r.status_code} for alice's private deck{suffix}; a 403 "
        f"would confirm it exists (ADR 5)")


def test_a_private_deck_is_a_404_to_writes_too(instance, pool):
    """The half that is easy to get wrong.

    A refused *write* on a private deck must be 404 rather than #80's 403, for
    the same reason the reads above are: the write refusal is an answer about
    a deck the caller cannot see, and 403 would tell them it is there.
    """
    client, _alice, _bob = instance
    login(client, "alice", PASSWORD_A)
    _make_private_deck(client, "alice", "secret-brew")
    client.post("/api/auth/logout")

    login(client, "bob", PASSWORD_B)
    assert client.patch("/api/decks/alice/secret-brew",
                        json={"field": "status", "value": "built"}
                        ).status_code == 404
    assert client.request("DELETE",
                          "/api/decks/alice/secret-brew?confirm=secret-brew"
                          ).status_code == 404
    assert client.put("/api/decks/alice/secret-brew/shared",
                      json={"shared": True}).status_code == 404


def test_a_shared_deck_is_readable_but_403_to_write(instance, pool):
    """The other side of ADR 22's rule, and why it is not simply "404 always".

    Once alice shares a deck bob can read it, so answering 404 to his write
    would be a lie he can disprove by reloading the page. 403 is the honest
    answer and it is #80's, unchanged.
    """
    client, _alice, _bob = instance
    login(client, "alice", PASSWORD_A)
    _make_private_deck(client, "alice", "on-display")
    assert client.put("/api/decks/alice/on-display/shared",
                      json={"shared": True}).status_code == 200
    client.post("/api/auth/logout")

    login(client, "bob", PASSWORD_B)
    assert client.get("/api/decks/alice/on-display").status_code == 200
    assert client.patch("/api/decks/alice/on-display",
                        json={"field": "status", "value": "built"}
                        ).status_code == 403


def test_an_unknown_owner_is_a_404_and_not_a_different_error(instance):
    """The owner segment must not enumerate the account list.

    If "no such person" answered differently from "nothing of theirs for you",
    the path would be an oracle for which accounts exist. One answer for both.
    """
    client, _alice, _bob = instance
    login(client, "bob", PASSWORD_B)
    assert client.get("/api/decks/nobody-here/whatever").status_code == 404
    assert client.get("/api/decks/alice/not-a-deck").status_code == 404


def test_an_admin_still_cannot_write_another_persons_deck(instance, pool):
    """Administering the instance is not owning everybody's decks.

    The same property `test_an_admin_still_cannot_see_another_persons_job`
    asserts for jobs, and the assertion that would catch somebody "fixing" the
    ownership check by exempting admins from it. Alice is the admin here.
    """
    client, _alice, _bob = instance
    login(client, "bob", PASSWORD_B)
    _make_private_deck(client, "bob", "bobs-brew")
    assert client.put("/api/decks/bob/bobs-brew/shared",
                      json={"shared": True}).status_code == 200
    client.post("/api/auth/logout")

    login(client, "alice", PASSWORD_A)
    assert client.get("/api/decks/bob/bobs-brew").status_code == 200
    assert client.patch("/api/decks/bob/bobs-brew",
                        json={"field": "status", "value": "built"}
                        ).status_code == 403


@pytest.mark.parametrize("suffix", [
    "", "/validate", "/stats", "/suggestions", "/commander", "/printings",
])
def test_an_admin_still_cannot_see_another_persons_private_deck(
        instance, pool, suffix):
    """The half the test above leaves out, and the one that actually leaks.

    `test_an_admin_still_cannot_write_another_persons_deck` shares bob's deck
    before alice looks at it, so it only ever puts the admin in front of a deck
    that is *meant* to be visible — 200 to read, 403 to write, which is #80's
    pair. The private case had no coverage at all, and it is the one where the
    two rules pull opposite ways: a shared deck answers an admin 403, a private
    one must answer 404, and "administering the instance" is exactly the excuse
    an exemption would be written under.

    Parametrised over the same read verbs as
    `test_a_private_deck_is_a_404_to_another_account`, because an exemption
    added to `Library.source_for` reaches all of them at once and a single
    `GET` would only prove the plainest one.
    """
    client, _alice, _bob = instance
    login(client, "bob", PASSWORD_B)
    _make_private_deck(client, "bob", "bobs-brew")
    client.post("/api/auth/logout")

    login(client, "alice", PASSWORD_A)
    r = client.get(f"/api/decks/bob/bobs-brew{suffix}")
    assert r.status_code == 404, (
        f"the admin got {r.status_code} for bob's private deck{suffix}; "
        f"ADR 5 makes it invisible, and admin is not an exemption")


def test_an_admin_is_refused_a_private_deck_s_writes_as_404_too(instance,
                                                                pool):
    """And the writes, which must not fall back to 403 for an admin either.

    The same argument as `test_a_private_deck_is_a_404_to_writes_too`, one
    privilege level up: 403 here would confirm the deck exists to the one
    caller most likely to have been handed a shortcut.
    """
    client, _alice, _bob = instance
    login(client, "bob", PASSWORD_B)
    _make_private_deck(client, "bob", "bobs-brew")
    client.post("/api/auth/logout")

    login(client, "alice", PASSWORD_A)
    assert client.patch("/api/decks/bob/bobs-brew",
                        json={"field": "status", "value": "built"}
                        ).status_code == 404
    assert client.request("DELETE",
                          "/api/decks/bob/bobs-brew?confirm=bobs-brew"
                          ).status_code == 404
    assert client.put("/api/decks/bob/bobs-brew/shared",
                      json={"shared": True}).status_code == 404

# ------------------------------------------------------- the import ring

@pytest.mark.parametrize("first", [
    "mtglab.api.service",
    "mtglab.auth.users",
    "mtglab.claude.tools",
    "mtglab.api.flymetrics",
    "mtglab.decks.library",
])
def test_every_package_imports_first_without_a_cycle(first: str) -> None:
    """Import each module into a *fresh* interpreter, as the only import.

    A subprocess per case, and that is the whole point rather than an
    extravagance: a cycle like this one is invisible the moment anything else
    has already been imported, so a check that shares the suite's interpreter
    would pass while the bug was live. It was live — the tiers PR had
    `auth/users` import `mtglab.claude`, whose package `__init__` pulls the
    whole Claude surface, which imports `api.service`, which imports
    `decks.library`, which imports `auth.users`. Every real entry point
    happened to import `service` early enough to win the race, so the app
    worked, the suite passed, and `pytest tests/test_flymetrics.py` alone could
    not collect.

    Cheap insurance against a class of bug that only bites the next person to
    write a script, a worker, or a test file with a short import list.
    """
    proc = subprocess.run(
        [sys.executable, "-c", f"import {first}"],
        capture_output=True, text=True, timeout=120,
    )
    assert proc.returncode == 0, (
        f"importing {first} first fails:\n{proc.stderr[-1500:]}")
