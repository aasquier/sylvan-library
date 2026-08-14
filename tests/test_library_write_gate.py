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
    assert client.get("/api/decks/mini").status_code == 200


# ------------------------------------------------------ writing is not

@pytest.mark.parametrize("verb,path,payload", [
    ("put", "/api/decks/mini/notes/mulligan", {"value": "keep two lands"}),
    ("patch", "/api/decks/mini", {"field": "status", "value": "built"}),
    ("patch", "/api/decks/mini/cards/Sol Ring", {"field": "why",
                                                 "value": "rewritten"}),
    ("delete", "/api/decks/mini/cards/Sol Ring", None),
    ("post", "/api/decks/mini/swap", {"out": "Sol Ring",
                                      "into": "Arcane Signet", "why": "x"}),
    ("post", "/api/decks/mini/cards", {"name": "Forest", "category": "land",
                                       "why": "x"}),
    ("post", "/api/decks/import", {"slug": "theirs", "text": "1 Sol Ring\n"}),
    ("post", "/api/decks", {"slug": "theirs",
                            "commander": ["Gyome, Master Chef"]}),
    ("delete", "/api/decks/mini?confirm=mini", None),
])
def test_a_guest_cannot_write_anything(instance, verb, path, payload):
    """Every deck-writing route, refused to a signed-in non-owner.

    Parametrised over the route table rather than written as one test with
    nine asserts, so a regression names the endpoint that regressed. The list
    is every write verb `api/app.py` declares against a deck; a tenth added
    without a thought about ownership will not appear here, which is what
    `test_isolation.py`'s generated classification is for.
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
    assert client.get("/api/decks/mini").status_code == 200
    refused = client.put("/api/decks/mini/notes/mulligan", json={"value": "x"})
    assert refused.status_code == 403


def test_a_refused_write_does_not_reveal_whether_the_deck_exists(instance):
    """A guest gets the same answer for a real slug and an invented one.

    Not a live leak -- the library is readable, so the listing already answers
    this. It is pinned because the ordering is what carries into the per-user
    tier, where the pair (403, 404) *would* be a membership oracle over other
    people's private decks.
    """
    client, _ = instance
    login(client, "guest", GUEST_PASSWORD)
    real = client.put("/api/decks/mini/notes/mulligan", json={"value": "x"})
    absent = client.put("/api/decks/no-such-deck/notes/mulligan",
                        json={"value": "x"})
    assert real.status_code == absent.status_code == 403


# --------------------------------------------------------- the owner still can

def test_the_owner_can_still_write(instance):
    """The control that stops this being fixed by refusing everybody.

    Without it, `writable=False` unconditionally passes every test above.
    """
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    response = client.put("/api/decks/mini/notes/mulligan",
                          json={"value": "keep two lands"})
    assert response.status_code == 200, response.text
    assert "keep two lands" in on_disk(decks_dir)


def test_the_owner_can_delete(instance):
    """The operation the flag exists for, from the permitted side."""
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    response = client.request("DELETE", "/api/decks/mini?confirm=mini")
    assert response.status_code == 200, response.text
    assert not (decks_dir / "mini").exists()


def test_a_guest_is_refused_even_after_the_owner_has_written(instance):
    """Ordering: the source is rebuilt per request, so an admin's write does
    not leave a writable source behind for the next caller. A cached or
    module-level source would fail this and pass everything above."""
    client, decks_dir = instance
    login(client, "owner", OWNER_PASSWORD)
    assert client.put("/api/decks/mini/notes/mulligan",
                      json={"value": "owner wrote this"}).status_code == 200
    client.post("/api/auth/logout")

    login(client, "guest", GUEST_PASSWORD)
    refused = client.put("/api/decks/mini/notes/mulligan",
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
        response = client.put("/api/decks/mini/notes/mulligan",
                              json={"value": "local edit"})
        assert response.status_code == 200, response.text
    assert "local edit" in (decks_dir / "mini" / "deck.yaml").read_text(
        encoding="utf-8")


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
