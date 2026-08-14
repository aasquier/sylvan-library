"""The curated library is readable by everyone and writable by its owner.

This is the test that did not exist, and its absence is the whole story. Every
deck route is classified **shared** in `tests/test_isolation.py`, with reasons
like *"edits a shared deck"* -- and that classification was correct about
reading and silently wrong about writing. `deps.deck_source` handed the same
`FileDeckSource()` to everybody, `FileDeckSource.writable` was hardcoded
`True`, and no test anywhere logged in as a second account and tried to write.
So the suite was green while any invited user could edit or delete the
maintainer's six decks on the deployed instance.

`tests/test_api.py` did cover read-only sources, but through
`dependency_overrides` with a `MemoryDeckSource(writable=False)` -- it asserted
what the route does *when handed* a read-only source, never that anyone is
actually handed one. That gap is the same shape as the dossier's: 42 tests
exercising a module, none asking what the HTTP surface did with it.

**These tests own their own decks directory.** The `instance` fixture in
`test_isolation.py` leaves `DECKS_DIR` pointing at the repository's real
`decks/`, which is harmless there because every assertion is about a refusal.
Here the admin half genuinely writes, and pointing that at the curated six
would edit the source of truth from a test run.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient  # noqa: E402

import tiny_corpus  # noqa: E402
from mtglab import config  # noqa: E402
from mtglab.api import jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.auth import db, users  # noqa: E402

OWNER_PASSWORD = "correct-horse-battery-staple"
GUEST_PASSWORD = "a-different-long-passphrase"

DECK_YAML = """\
slug: mini
name: Mini Deck
commander:
  - Gyome, Master Chef
bracket: 4
strategy: A minimal but legally sized deck used by the write-gate tests.
cards:
  - name: Swamp
    category: land
    qty: 98
    why: Black mana.
  - name: Sol Ring
    category: ramp
    why: Two mana for one.
"""


@pytest.fixture
def instance(tmp_path):
    """Auth on, a scratch library, and two accounts.

    `owner` administers this instance and `guest` does not, which is the only
    difference between them and -- until the per-user tier lands -- the whole
    of what decides who may write. Both names are chosen over alice/bob to say
    what the distinction is about here.
    """
    jobs.clear()
    decks_dir = tmp_path / "decks"
    (decks_dir / "mini").mkdir(parents=True)
    (decks_dir / "mini" / "deck.yaml").write_text(DECK_YAML, encoding="utf-8")

    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_dir):
        con = db.connect()
        try:
            users.create(con, "owner", password=OWNER_PASSWORD, is_admin=True)
            users.create(con, "guest", password=GUEST_PASSWORD)
        finally:
            con.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as client:
            yield client, decks_dir


def login(client, username: str, password: str) -> None:
    response = client.post("/api/auth/login",
                           json={"username": username, "password": password})
    assert response.status_code == 200, response.text


def on_disk(decks_dir: Path) -> str:
    return (decks_dir / "mini" / "deck.yaml").read_text(encoding="utf-8")


# ------------------------------------------------------- reading is shared

def test_a_guest_can_read_the_library(instance):
    """The control, and the half that must not change.

    If this ever fails the fix has gone too far: the library is *meant* to be
    visible to everyone, and ADR 1 calls it the library rather than anybody's
    property. Only writing is being taken away.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    listing = client.get("/api/decks")
    assert listing.status_code == 200
    assert [d["slug"] for d in listing.json()] == ["mini"]
    assert client.get("/api/decks/local/mini").status_code == 200


# ------------------------------------------------------ writing is not

@pytest.mark.parametrize("verb,path,payload", [
    ("put", "/api/decks/local/mini/notes/mulligan", {"value": "keep two lands"}),
    ("patch", "/api/decks/local/mini", {"field": "status", "value": "built"}),
    ("patch", "/api/decks/local/mini/cards/Sol Ring", {"field": "why",
                                                 "value": "rewritten"}),
    ("delete", "/api/decks/local/mini/cards/Sol Ring", None),
    ("post", "/api/decks/local/mini/swap", {"out": "Sol Ring",
                                      "into": "Arcane Signet", "why": "x"}),
    ("post", "/api/decks/local/mini/cards", {"name": "Forest", "category": "land",
                                       "why": "x"}),
    ("delete", "/api/decks/local/mini?confirm=mini", None),
])
def test_a_guest_cannot_write_anything(instance, verb, path, payload):
    """Every deck-writing route, refused to a signed-in non-owner.

    Parametrised over the route table rather than written as one test with
    nine asserts, so a regression names the endpoint that regressed. The list
    is every write verb `api/app.py` declares against *an existing deck*; one
    added without a thought about ownership will not appear here, which is
    what `test_isolation.py`'s generated classification is for.

    **`POST /api/decks` and `/api/decks/import` are deliberately no longer in
    this list.** They used to be, and answering 403 to them was the interim
    state ADR 22 exists to end: with nowhere for a guest's decks to live, the
    only safe answer was that they could not make one. They write into
    `lib.mine()` now, so a guest creating a deck is the feature rather than
    the leak — see `test_a_guest_may_create_in_their_own_library`.
    """
    client, decks_dir = instance
    login(client, "guest", GUEST_PASSWORD)
    before = on_disk(decks_dir)

    response = client.request(verb.upper(), path, json=payload)

    assert response.status_code == 403, (
        f"{verb.upper()} {path} answered {response.status_code}")
    assert "read-only" in response.json()["detail"]
    # The assertion that separates "refused" from "refused after writing".
    assert on_disk(decks_dir) == before
    assert sorted(p.name for p in decks_dir.iterdir()) == ["mini"]


def test_the_refusal_is_403_and_not_404(instance):
    """Deliberately *not* ADR 5's 404, and the exception is argued.

    ADR 5 hides resources whose existence is the secret. This deck's existence
    is not a secret -- it is in a public repository, `GET /api/decks` just
    listed it to this very caller, and they are most likely looking at it. A
    404 would hide nothing and would tell somebody their deck had vanished.
    Same exception ADR 17 makes for `/api/admin`, same reason.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    assert client.get("/api/decks/local/mini").status_code == 200
    refused = client.put("/api/decks/local/mini/notes/mulligan", json={"value": "x"})
    assert refused.status_code == 403


def test_a_refused_write_distinguishes_a_visible_deck_from_an_absent_one(instance):
    """**This assertion inverted with ADR 22, and the old docstring said why.**

    It used to require 403 for a real slug *and* an invented one, on the
    argument that the pair (403, 404) would become a membership oracle once
    there were private decks. That was the right worry and the wrong place to
    answer it. ADR 22 answers it where it actually bites: a deck the caller
    **cannot see** answers 404 to everything, writes included, so no pair
    exists to read. `tests/test_isolation.py` pins that half.

    Here the caller can see the whole library -- `GET /api/decks` just listed
    it to them -- so the two answers may differ, and they should. 403 says
    "not yours to change"; 404 says "no such deck". Collapsing them would only
    tell somebody their deck had vanished.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    real = client.put("/api/decks/local/mini/notes/mulligan",
                      json={"value": "x"})
    absent = client.put("/api/decks/local/no-such-deck/notes/mulligan",
                        json={"value": "x"})
    assert real.status_code == 403, "a shared deck the guest may read"
    assert absent.status_code == 404, "no such deck"


# --------------------------------------------------------- the owner still can

def test_the_owner_can_still_write(instance):
    """The control that stops this being fixed by refusing everybody.

    Without it, `writable=False` unconditionally passes every test above.
    """
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    response = client.put("/api/decks/local/mini/notes/mulligan",
                          json={"value": "keep two lands"})
    assert response.status_code == 200, response.text
    assert "keep two lands" in on_disk(decks_dir)


def test_the_owner_can_delete(instance):
    """The operation the flag exists for, from the permitted side."""
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    response = client.request("DELETE", "/api/decks/local/mini?confirm=mini")
    assert response.status_code == 200, response.text
    assert not (decks_dir / "mini").exists()


def test_a_guest_is_refused_even_after_the_owner_has_written(instance):
    """Ordering: the source is rebuilt per request, so an admin's write does
    not leave a writable source behind for the next caller. A cached or
    module-level source would fail this and pass everything above."""
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    assert client.put("/api/decks/local/mini/notes/mulligan",
                      json={"value": "owner wrote this"}).status_code == 200
    client.post("/api/auth/logout")

    login(client, "guest", GUEST_PASSWORD)
    refused = client.put("/api/decks/local/mini/notes/mulligan",
                         json={"value": "guest wrote this"})
    assert refused.status_code == 403
    assert "owner wrote this" in on_disk(decks_dir)


# ------------------------------------------------------------ auth off

def test_with_auth_off_the_local_user_still_writes(tmp_path):
    """A laptop is unchanged, which is the point of `deps.LOCAL`.

    `LOCAL.is_admin` is `True` because with no authentication there is nobody
    for it to be true relative to -- one person holding the file the app reads.
    A fix that keyed on `authenticated` rather than `is_admin` would make the
    local tool read-only, and that would be the regression this catches.
    """
    jobs.clear()
    decks_dir = tmp_path / "decks"
    (decks_dir / "mini").mkdir(parents=True)
    (decks_dir / "mini" / "deck.yaml").write_text(DECK_YAML, encoding="utf-8")

    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_dir), \
            TestClient(create_app()) as client:
        response = client.put("/api/decks/local/mini/notes/mulligan",
                              json={"value": "local edit"})
        assert response.status_code == 200, response.text
    assert "local edit" in (decks_dir / "mini" / "deck.yaml").read_text(
        encoding="utf-8")


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))


# --------------------------------------------- ADR 22: a guest has a library

@pytest.fixture
def corpus(instance):
    """A queryable corpus inside the instance's scratch data directory.

    Depends on `instance` so it is built inside that fixture's
    `config.use_paths` and both share one app. Creating a deck is refused
    without a corpus -- a commander nobody checked is a colour identity nobody
    checked -- so only the two tests that create decks pay for it.
    """
    return tiny_corpus.build(config.DB_PATH)


def test_a_guest_may_create_in_their_own_library(instance, corpus):
    """The consequence #80 deliberately deferred, now built.

    A guest could not create a deck at all while there was nowhere to put one;
    the alternative was their decks landing in the maintainer's library. They
    have their own tier now (ADR 22), so this is the route working rather than
    the gate failing -- and the deck lands under *their* name, not `local`.
    """
    client, decks_dir = instance
    login(client, "guest", GUEST_PASSWORD)

    response = client.post("/api/decks", json={
        "slug": "theirs", "commander": ["Gyome, Master Chef"]})
    assert response.status_code in (200, 201), response.text
    assert response.json()["owner"] == "guest"

    # And it did not touch the file-backed library on its way.
    assert sorted(p.name for p in decks_dir.iterdir()) == ["mini"]

    # Private by default, so nothing is published the moment it exists.
    assert client.get("/api/decks/guest/theirs").json()["shared"] is False


def test_a_guest_still_cannot_create_inside_the_curated_library(
        instance, corpus):
    """Creating writes to the caller's own tier and cannot be aimed elsewhere.

    `POST /api/decks` carries no owner segment at all, which is the structural
    version of this rule: the API has no way to express "make a deck in
    somebody else's library", so there is no check here to forget. Even a slug
    that collides with a curated deck lands in the guest's own tier.
    """
    client, decks_dir = instance
    login(client, "guest", GUEST_PASSWORD)
    client.post("/api/decks", json={"slug": "mini",
                                    "commander": ["Gyome, Master Chef"]})
    assert sorted(p.name for p in decks_dir.iterdir()) == ["mini"]
    assert on_disk(decks_dir) == DECK_YAML


# --------------------------------------------------- the shelf's two halves

def test_the_listing_marks_the_showcase_and_nothing_else(instance, corpus):
    """`GET /api/decks` says which owner the curated six belong to (ADR 22).

    The browse tab needs three groups out of one flat list -- yours, the
    showcase, everybody else's -- and it can only work two of them out for
    itself. `writable` identifies the caller's own decks; **nothing identifies
    the maintainer's**, because the client is never told who that is. Without
    this field a browser would have to infer the showcase from the *order* of
    this response, and ordering is not a contract.

    Read the two flags together: they are what "mine and the showcase in front,
    everybody else behind a tab" is made of.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    client.post("/api/decks", json={"slug": "theirs",
                                    "commander": ["Gyome, Master Chef"]})

    tiles = {d["slug"]: d for d in client.get("/api/decks").json()}
    assert set(tiles) == {"mini", "theirs"}

    # The file tier: the showcase, and not this caller's to change.
    assert tiles["mini"]["showcase"] is True
    assert tiles["mini"]["writable"] is False
    # Their own: theirs to change, and not the showcase.
    assert tiles["theirs"]["showcase"] is False
    assert tiles["theirs"]["writable"] is True


def test_a_private_deck_is_absent_from_somebody_else_s_shelf(instance, corpus):
    """And so cannot be marked anything at all.

    The tile's `shared` flag is only ever a fact about a deck the caller can
    already see, which is why the app may render "private" from it without
    ever claiming that about a stranger's deck: a stranger's private deck is
    not in this response. It is the same fact its 404 states, arrived at the
    same way -- `Library` never hands out a source that can see it.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    client.post("/api/decks", json={"slug": "theirs",
                                    "commander": ["Gyome, Master Chef"]})
    client.post("/api/auth/logout")

    login(client, "owner", OWNER_PASSWORD)
    slugs = {d["slug"] for d in client.get("/api/decks").json()}
    assert slugs == {"mini"}, "the guest's private deck is not on this shelf"

    # Shared, and now it is -- under its owner's name, and not writable here.
    client.post("/api/auth/logout")
    login(client, "guest", GUEST_PASSWORD)
    assert client.put("/api/decks/guest/theirs/shared",
                      json={"shared": True}).status_code == 200
    client.post("/api/auth/logout")

    login(client, "owner", OWNER_PASSWORD)
    tiles = {d["slug"]: d for d in client.get("/api/decks").json()}
    assert set(tiles) == {"mini", "theirs"}
    assert tiles["theirs"]["owner"] == "guest"
    assert tiles["theirs"]["writable"] is False
    assert tiles["theirs"]["showcase"] is False


# ------------------------ a laptop, with a maintainer address in the .env

def test_a_laptop_with_an_admin_address_still_has_exactly_one_library(
        tmp_path, monkeypatch):
    """`MTGLAB_ADMIN_EMAIL` set and auth **off** — the maintainer's own laptop.

    This is the configuration every test had, and no test had. `.env` sets the
    address on the machine where `mtglab ui` runs with auth off, and the two
    halves of `Library` disagreed about what that meant: `my_owner` answered
    `local` because nobody is signed in, `_file_owner` answered the maintainer's
    username because the variable is set, and `visible()` added the one
    file-backed library under both names. The shelf showed all six curated decks
    **twice**, and the tiles filed under `local` linked to a route that 404ed,
    because `source_for("local")` fell through to the SQL tier and found no such
    account.

    Every existing test missed it from one side or the other: the auth-off
    fixtures `monkeypatch.delenv` the address precisely so they do not create an
    `app.db`, and every fixture that sets it also turns auth on.

    Found by loading the page, which is the fourth time that has been the only
    way to see something.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    monkeypatch.setenv("MTGLAB_ADMIN_USERNAME", "gyome")
    jobs.clear()
    decks_dir = tmp_path / "decks"
    (decks_dir / "mini").mkdir(parents=True)
    (decks_dir / "mini" / "deck.yaml").write_text(DECK_YAML, encoding="utf-8")

    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_dir), \
            TestClient(create_app()) as client:
        tiles = client.get("/api/decks").json()
        assert [d["slug"] for d in tiles] == ["mini"], "one deck, listed once"
        assert tiles[0]["owner"] == "local"
        assert tiles[0]["writable"] is True
        assert tiles[0]["showcase"] is True

        # And the address on the tile is the one that works. The shelf's own
        # link 404ing is what this looked like on the page.
        assert client.get("/api/decks/local/mini").status_code == 200
        assert client.get("/api/decks/gyome/mini").status_code == 404

        # A stray owner segment is a 404 rather than a lookup that enumerates.
        assert client.get("/api/decks/nobody/mini").status_code == 404

    # No assertion about `app.db` here, deliberately: `auth/bootstrap.py`
    # reconciles the maintainer account at every start of the app whenever
    # `MTGLAB_ADMIN_EMAIL` is set, so the file exists in this configuration by
    # design. The test below is the one that can see the difference.


def test_a_bare_laptop_acquires_no_database_from_a_typed_deck_url(tmp_path,
                                                                  monkeypatch):
    """Nothing set, auth off, and somebody types an owner segment.

    `test_the_local_app_touches_no_database` pins the *listing*, and
    `Library.visible()` carries a paragraph about why it must not reach
    `shared_decks()`. The per-deck path had no such guard and a URL reaches it
    directly: `source_for("nobody")` fell through to a user lookup, which opens
    `app.db` -- and on a laptop, opening it creates it. A single mistyped
    address would leave a database behind for a feature this run does not use.
    """
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    decks_dir = tmp_path / "decks"
    (decks_dir / "mini").mkdir(parents=True)
    (decks_dir / "mini" / "deck.yaml").write_text(DECK_YAML, encoding="utf-8")

    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_dir), \
            TestClient(create_app()) as client:
        assert client.get("/api/decks/nobody/mini").status_code == 404
        assert client.get("/api/decks/local/mini").status_code == 200

    assert not (tmp_path / "data" / "app.db").exists()


# ------------------- a deployment: auth on, and a maintainer configured

@pytest.fixture
def deployed(tmp_path, monkeypatch):
    """Auth **on** with `MTGLAB_ADMIN_EMAIL` set — what the instance runs.

    The third of three configurations, the only one a deployment is ever in,
    and until now the only one with no test at all. The other two each miss it
    from one side:

    - `instance` above is auth-on with **no** maintainer, so
      `Library._file_owner` falls back to `local` and the write gate is the
      `self._maintainer is None and self._is_admin` clause.
    - `test_a_laptop_with_an_admin_address_still_has_exactly_one_library` sets
      the address but with auth **off**, where `_file_owner` returns `local`
      before the maintainer is ever consulted.

    Neither reaches the branch every request on the instance takes, where
    `_file_owner` is a real username and the `is_admin` escape hatch is dead
    code. `tests/conftest.py` clears both variables for every test by design —
    inheriting the maintainer's own `.env` makes a suite that passes in CI and
    fails on their laptop — so reaching this branch has to be opted into, and
    nothing had.

    Three accounts, because the distinction this branch draws needs three.
    `gyome` is the maintainer. `deputy` administers the instance and is not the
    maintainer — the account the fallback branch cannot tell from `gyome`, and
    this one must. `guest` is neither.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    monkeypatch.setenv("MTGLAB_ADMIN_USERNAME", "gyome")
    jobs.clear()
    decks_dir = tmp_path / "decks"
    (decks_dir / "mini").mkdir(parents=True)
    (decks_dir / "mini" / "deck.yaml").write_text(DECK_YAML, encoding="utf-8")

    with config.use_paths(data_dir=tmp_path / "data", decks_dir=decks_dir):
        con = db.connect()
        try:
            users.create(con, "gyome", password=OWNER_PASSWORD,
                         email="maintainer@example.com", is_admin=True)
            users.create(con, "deputy", password=GUEST_PASSWORD, is_admin=True)
            users.create(con, "guest", password=GUEST_PASSWORD)
        finally:
            con.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as client:
            yield client


def test_the_curated_library_sits_under_the_maintainer(deployed):
    """The URL shape a deployment serves, which no test had exercised.

    Every assertion above this line addresses the file tier as
    `/api/decks/local/mini`, because with no maintainer configured that is
    where `_file_owner` puts it. A deployment files it under a username
    instead — so the deck paths the frontend builds in production are a shape
    the suite had never once produced.
    """
    client = deployed
    login(client, "gyome", OWNER_PASSWORD)
    assert client.get("/api/decks/gyome/mini").status_code == 200
    assert client.get("/api/decks/local/mini").status_code == 404, (
        "with a maintainer configured the file tier is theirs, not `local`'s")


def test_the_maintainer_writes_their_own_library(deployed):
    """The control for the refusal below: the gate is shut, not jammed."""
    client = deployed
    login(client, "gyome", OWNER_PASSWORD)
    assert client.put("/api/decks/gyome/mini/notes/mulligan",
                      json={"value": "keep two lands"}).status_code == 200


def test_an_admin_who_is_not_the_maintainer_cannot_write_the_library(deployed):
    """The assertion the fallback branch is structurally unable to make.

    With no maintainer configured `Library` grants the file tier to any admin,
    which is #80's rule and is the right answer when the six are nobody's in
    particular. Configure one and that clause is dead: ownership is `mine`, and
    administering the instance stops conferring it (ADR 22 §5, the same
    property `test_an_admin_still_cannot_see_another_persons_job` asserts for
    jobs).

    Moot on the instance *today*, where the only admin is also the maintainer
    and the two are one account. It stops being moot the first time there are
    two admins, and this is already the branch every real request takes — so
    the coverage should not wait for the second admin to arrive.

    Read is 200 and write is 403 rather than 404, which is #80's call
    unchanged: the curated library is deliberately visible, so refusing the
    write with 404 would be a lie the caller disproves by reloading.
    """
    client = deployed
    login(client, "deputy", GUEST_PASSWORD)
    assert client.get("/api/decks/gyome/mini").status_code == 200
    refused = client.put("/api/decks/gyome/mini/notes/mulligan",
                         json={"value": "keep two lands"})
    assert refused.status_code == 403, (
        f"an admin who is not the maintainer wrote the curated library "
        f"(got {refused.status_code}); ADR 22 §5 makes ownership the test, "
        f"not the admin flag")


def test_a_guest_reads_the_showcase_and_is_refused_the_write(deployed):
    """The same pair for somebody with no privileges, under the real owner.

    Covered above under `local`; repeated here because it is a different
    branch of `source_for` and because a fix that over-corrected — filing the
    six under the maintainer and then hiding them — would pass every other
    test in this fixture.
    """
    client = deployed
    login(client, "guest", GUEST_PASSWORD)
    assert client.get("/api/decks/gyome/mini").status_code == 200
    assert client.put("/api/decks/gyome/mini/notes/mulligan",
                      json={"value": "keep two lands"}).status_code == 403
