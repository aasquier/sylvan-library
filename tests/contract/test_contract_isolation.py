"""ADR 5 and ADR 22 over the wire: B asks for A's things and gets nothing.

The same matrix `tests/test_isolation.py` runs in-process, written against
the HTTP surface alone -- no `jobs.submit`, no `MemoryDeckSource`, nothing a
second implementation would not have. Alice administers and Bob does not,
so every check here also says that administering the instance is not the
same thing as reaching another account's work.
"""

from __future__ import annotations

import pytest

from contract import checks
from contract.harness import DECK_OWNER, DECK_SLUG

DECK_SUFFIXES = ("", "/validate", "/stats", "/suggestions", "/commander",
                 "/printings")


def _private_deck(instance, owner: str, slug: str) -> None:
    """Create `slug` in `owner`'s own tier through the API and leave it
    private -- what the adversarial checks reach for is a deck the app made."""
    client = instance.as_user(owner)
    r = client.post("/api/decks", json={"slug": slug, "name": slug,
                                        "commander": ["Gyome, Master Chef"]})
    assert r.status_code in (200, 201), r.text
    assert client.get(f"/api/decks/{owner}/{slug}").json()["shared"] is False


def _delete(instance, owner: str, slug: str) -> None:
    client = instance.as_user(owner)
    client.delete(f"/api/decks/{owner}/{slug}", params={"confirm": slug})


def test_another_accounts_job_is_a_404(instance):
    """A owns a job, B asks for it by id, B gets 404 and not 403 -- the one
    assertion ADR 5 says would still matter if everything else were deleted.
    The job is submitted through the route, so the ownership stamp is the
    handler's and not a test's."""
    alice = instance.as_user("alice")
    submitted = alice.post("/api/sim/mana", json={"slug": "no-such-deck",
                                                  "owner": DECK_OWNER})
    assert submitted.status_code == 200, submitted.text
    job_id = submitted.json()["id"]
    assert alice.get(f"/api/jobs/{job_id}").status_code == 200

    bob = instance.as_user("bob")
    checks.check_not_found_not_forbidden(bob.get(f"/api/jobs/{job_id}"),
                                         "alice's job, asked for by bob")
    assert job_id not in {j["id"] for j in bob.get("/api/jobs").json()}


def test_an_admin_still_cannot_see_another_persons_job(instance):
    """Admin is orthogonal to ADR 5's isolation: the direction nobody writes
    by accident, and the check that fails if a support shortcut ever exempts
    admins from the ownership filter."""
    bob = instance.as_user("bob")
    job_id = bob.post("/api/sim/mana", json={"slug": "no-such-deck",
                                             "owner": DECK_OWNER}).json()["id"]
    alice = instance.as_user("alice")
    checks.check_not_found_not_forbidden(alice.get(f"/api/jobs/{job_id}"),
                                         "bob's job, asked for by the admin")
    assert job_id not in {j["id"] for j in alice.get("/api/jobs").json()}


@pytest.mark.parametrize("suffix", DECK_SUFFIXES)
def test_a_private_deck_is_a_404_to_another_account_and_to_the_admin(
        instance, run_id, suffix):
    """ADR 22's hard case, both privilege levels at once: bob's private brew
    is invisible to alice even though alice administers the instance."""
    slug = f"ct-private-{run_id}"
    _private_deck(instance, "bob", slug)
    try:
        alice = instance.as_user("alice")
        checks.check_not_found_not_forbidden(
            alice.get(f"/api/decks/bob/{slug}{suffix}"),
            f"bob's private deck{suffix}, asked for by the admin")
    finally:
        _delete(instance, "bob", slug)


def test_a_private_deck_is_a_404_to_writes_too(instance, run_id):
    """A refused write on a private deck is 404 rather than 403 for the same
    reason the reads are: 403 would tell the caller the deck is there."""
    slug = f"ct-private-{run_id}"
    _private_deck(instance, "bob", slug)
    try:
        alice = instance.as_user("alice")
        checks.check_not_found_not_forbidden(
            alice.patch(f"/api/decks/bob/{slug}",
                        json={"field": "status", "value": "built"}),
            "a PATCH on bob's private deck")
        checks.check_not_found_not_forbidden(
            alice.delete(f"/api/decks/bob/{slug}", params={"confirm": slug}),
            "a DELETE on bob's private deck")
        checks.check_not_found_not_forbidden(
            alice.put(f"/api/decks/bob/{slug}/shared", json={"shared": True}),
            "sharing bob's private deck")
    finally:
        _delete(instance, "bob", slug)


def test_a_shared_deck_is_readable_but_403_to_write(instance, run_id):
    """Once shared, a 404 to a write would be a lie the reader can disprove
    by reloading; 403 is the honest answer (ADR 22), admin or not."""
    slug = f"ct-shared-{run_id}"
    _private_deck(instance, "bob", slug)
    try:
        bob = instance.as_user("bob")
        assert bob.put(f"/api/decks/bob/{slug}/shared",
                       json={"shared": True}).status_code == 200
        alice = instance.as_user("alice")
        assert alice.get(f"/api/decks/bob/{slug}").status_code == 200
        refused = alice.patch(f"/api/decks/bob/{slug}",
                              json={"field": "status", "value": "built"})
        assert refused.status_code == 403, refused.text
        checks.check_error_envelope(refused)
    finally:
        _delete(instance, "bob", slug)


def test_an_unknown_owner_is_a_404_and_not_a_different_error(instance):
    """The owner segment must not enumerate the account list."""
    bob = instance.as_user("bob")
    checks.check_not_found_not_forbidden(
        bob.get("/api/decks/nobody-here/whatever"), "an unknown owner")
    checks.check_not_found_not_forbidden(
        bob.get("/api/decks/alice/not-a-deck"), "a known owner's missing deck")


def test_shared_routes_show_every_account_the_same_library(instance):
    """The file tier is the library and reads the same for everyone; the one
    field allowed to differ is `writable`, which is about the caller."""
    def library(user: str) -> list[dict]:
        client = instance.as_user(user)
        decks = client.get("/api/decks").json()
        return sorted((d for d in decks if d["owner"] == DECK_OWNER),
                      key=lambda d: d["slug"])

    as_alice, as_bob = library("alice"), library("bob")
    assert as_alice, "the fixture must serve at least one shared deck"
    strip = [{k: v for k, v in d.items() if k != "writable"} for d in as_alice]
    assert strip == [{k: v for k, v in d.items() if k != "writable"}
                     for d in as_bob]
    assert all(d["writable"] for d in as_alice)
    assert not any(d["writable"] for d in as_bob)
    assert any(d["slug"] == DECK_SLUG for d in as_alice)
