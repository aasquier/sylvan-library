"""The stateful goldens: scenarios whose records depend on what came before.

Each function walks one scenario in order and records every step named in
`cases.SEQUENCES`. Everything a scenario creates carries the run id and is
deleted at the end, so the scenario is re-runnable against a server that
remembers the last run -- which an external server does.
"""

from __future__ import annotations

import time

import pytest

import tiny_pool
from contract import checks, harness
from contract.cases import JOB_VOLATILE
from contract.checks import observed
from contract.harness import DECK_OWNER, DECK_SLUG

JOB_TIMEOUT_S = 180

#: Inside a finished simulation: `computed_at` is null on a fresh run and a
#: timestamp on a cache hit (ADR 18), and an external server remembers the
#: last run. Presence is pinned, the kind is not.
RESULT_VOLATILE = ("computed_at",)


def _await(client, job_id: str) -> dict:
    """Poll one job to its terminal state; the response is what gets pinned."""
    deadline = time.monotonic() + JOB_TIMEOUT_S
    while time.monotonic() < deadline:
        response = client.get(f"/api/jobs/{job_id}")
        assert response.status_code == 200, response.text
        if response.json()["status"] in ("done", "error"):
            return response
        time.sleep(0.1)
    pytest.fail(f"job {job_id} did not finish in {JOB_TIMEOUT_S}s")


def _expect(response, statuses, what: str):
    assert response.status_code in statuses, (
        f"{what} answered {response.status_code}, not one of {statuses}: "
        f"{response.text[:300]}")
    return response


# ------------------------------------------------------------------- auth

def test_auth_sequence(instance, goldens):
    g = "auth"
    client = instance.as_user(None)
    r = _expect(client.post("/api/auth/login",
                            json={"username": "nobody", "password": "x"}),
                (401,), "a login for an unknown account")
    goldens.check(g, "login-unknown-user", observed(r))
    r = _expect(client.post("/api/auth/login", json={}), (401, 422),
                "a login with no body")
    goldens.check(g, "login-malformed", observed(r))

    r = instance.login("alice")
    goldens.check(g, "login", observed(r))
    goldens.check(g, "me", observed(_expect(client.get("/api/auth/me"), (200,),
                                            "me, signed in")))
    r = _expect(client.post("/api/auth/logout"), (200,), "logout")
    goldens.check(g, "logout", observed(r))
    client.cookies.clear()
    r = _expect(client.get("/api/auth/me"), (200,), "me, signed out")
    assert r.json()["authenticated"] is False
    goldens.check(g, "me-after-logout", observed(r))

    r = _expect(client.post("/api/auth/reset",
                            json={"email": "nobody@example.com"}),
                (202,), "a reset request")
    goldens.check(g, "reset", observed(r))
    r = _expect(client.post("/api/auth/reset", json={}), (202, 422),
                "a reset with no address")
    goldens.check(g, "reset-malformed", observed(r))
    r = _expect(client.post("/api/auth/claim",
                            json={"token": "nope",
                                  "password": "long-enough-to-be-a-password"}),
                (400, 401, 403, 404, 410, 422), "a claim with a bad token")
    goldens.check(g, "claim-bad-token", observed(r))
    r = _expect(client.post("/api/auth/claim/preview", json={"token": "nope"}),
                (400, 401, 403, 404, 410, 422), "a preview with a bad token")
    goldens.check(g, "claim-preview-bad-token", observed(r))

    # The budget, spent on an account that exists to be locked out. The
    # per-address budget is three times this, and the harness forgets every
    # counted failure afterwards, so the rest of the run is unaffected.
    from mtglab.auth import ratelimit
    for _ in range(ratelimit.PER_ACCOUNT.failures):
        client.post("/api/auth/login",
                    json={"username": "throttle", "password": "wrong"})
    throttled = client.post("/api/auth/login",
                            json={"username": "throttle", "password": "wrong"})
    checks.check_rate_limited(throttled)
    goldens.check(g, "login-throttled", observed(throttled))
    harness.reset_rate_limits(instance.scratch)
    # And the control: with the counters forgotten, the account works again.
    assert client.post("/api/auth/login", json={
        "username": "throttle",
        "password": harness.PASSWORDS["throttle"]}).status_code == 200
    instance.logout()


# ------------------------------------------------------------------- jobs

def test_jobs_sequence(instance, goldens):
    g = "jobs"
    client = instance.as_user("alice")
    deck = {"slug": DECK_SLUG, "owner": DECK_OWNER}

    r = _expect(client.post("/api/sim/mana", json={}), (422,), "sim/mana {}")
    goldens.check(g, "sim-mana-refused", observed(r))

    r = _expect(client.post("/api/sim/mana", json={**deck, "games": 20}),
                (200,), "sim/mana")
    goldens.check(g, "sim-mana", observed(r, volatile=JOB_VOLATILE))
    done = _await(client, r.json()["id"])
    assert done.json()["status"] == "done", done.text
    goldens.check(g, "sim-mana-done", observed(done, volatile=RESULT_VOLATILE))

    r = _expect(client.post("/api/sim/mana",
                            json={"slug": "no-such-deck", "owner": DECK_OWNER}),
                (200,), "sim/mana for a missing deck")
    goldens.check(g, "sim-mana-missing-deck", observed(r, volatile=JOB_VOLATILE))
    failed = _await(client, r.json()["id"])
    assert failed.json()["status"] == "error", failed.text
    goldens.check(g, "sim-mana-missing-deck-done", observed(failed))

    r = _expect(client.post("/api/sim/lands",
                            json={**deck, "low": 33, "high": 34, "games": 20}),
                (200,), "sim/lands")
    goldens.check(g, "sim-lands", observed(r, volatile=JOB_VOLATILE))
    done = _await(client, r.json()["id"])
    assert done.json()["status"] == "done", done.text
    goldens.check(g, "sim-lands-done", observed(done, volatile=RESULT_VOLATILE))

    r = _expect(client.post("/api/sim/policy", json={**deck, "games": 200}),
                (200,), "sim/policy")
    goldens.check(g, "sim-policy", observed(r, volatile=JOB_VOLATILE))
    done = _await(client, r.json()["id"])
    assert done.json()["status"] == "done", done.text
    goldens.check(g, "sim-policy-done", observed(done, volatile=RESULT_VOLATILE))

    r = _expect(client.post("/api/sim/shelf", json=deck), (200,), "sim/shelf")
    goldens.check(g, "sim-shelf", observed(r, volatile=RESULT_VOLATILE))
    r = _expect(client.post("/api/sim/shelf", json={}), (422,), "sim/shelf {}")
    goldens.check(g, "sim-shelf-refused", observed(r))
    r = _expect(client.post("/api/sim/shelf",
                            json={"slug": "no-such-deck", "owner": DECK_OWNER}),
                (404,), "sim/shelf for a missing deck")
    goldens.check(g, "sim-shelf-missing", observed(r))

    r = _expect(client.post("/api/sim/forge", json={}), (422,), "sim/forge {}")
    goldens.check(g, "sim-forge-refused", observed(r))
    r = _expect(client.post("/api/sim/forge", json={
        "a_slug": DECK_SLUG, "a_owner": DECK_OWNER,
        "b_slug": DECK_SLUG, "b_owner": DECK_OWNER}),
        (503,), "sim/forge with no Forge")
    goldens.check(g, "sim-forge-unavailable", observed(r))

    r = _expect(client.get("/api/jobs"), (200,), "the job list")
    assert r.json(), "alice has submitted jobs; the list must show them"
    goldens.check(g, "jobs-list", observed(r, volatile=JOB_VOLATILE))
    r = _expect(client.get("/api/jobs/no-such-job"), (404,), "a missing job")
    goldens.check(g, "job-missing", observed(r))


# ------------------------------------------------------------------ edits

def test_edits_sequence(instance, goldens, run_id):
    g = "edits"
    client = instance.as_user("alice")
    slug = f"ct-edit-{run_id}"
    deck = f"/api/decks/alice/{slug}"
    imported = f"ct-import-{run_id}"

    r = _expect(client.post("/api/decks", json={
        "slug": slug, "name": "Contract deck",
        "commander": ["Gyome, Master Chef"]}), (200, 201), "create")
    goldens.check(g, "create", observed(r))
    try:
        r = _expect(client.post("/api/decks", json={
            "slug": slug, "name": "Contract deck",
            "commander": ["Gyome, Master Chef"]}), (409, 422),
            "create, again")
        goldens.check(g, "create-duplicate", observed(r))
        r = _expect(client.post("/api/decks", json={}), (422,), "create {}")
        goldens.check(g, "create-refused", observed(r))
        goldens.check(g, "get", observed(_expect(client.get(deck), (200,),
                                                 "get")))

        r = _expect(client.post(f"{deck}/cards", json={
            "name": "Sol Ring", "category": "ramp",
            "why": "Two mana on turn one, in every deck that may run it."}),
            (200,), "add a card")
        goldens.check(g, "add-card", observed(r))
        r = _expect(client.post(f"{deck}/cards", json={}), (422,),
                    "add a card with no body")
        goldens.check(g, "add-card-refused", observed(r))
        r = _expect(client.post(f"{deck}/cards", json={
            "name": "Llanowar Reborn", "category": "land",
            "why": "A land that carries a counter onto a big body."}),
            (200,), "add a second card")
        goldens.check(g, "add-second-card", observed(r))

        r = _expect(client.patch(f"{deck}/cards/Sol Ring", json={
            "field": "why",
            "value": "The fastest thing this deck can do on turn one."}),
            (200,), "patch a card")
        goldens.check(g, "patch-card", observed(r))
        r = _expect(client.patch(f"{deck}/cards/Sol Ring",
                                 json={"field": "why"}), (422,),
                    "patch a card with no value")
        goldens.check(g, "patch-card-refused", observed(r))

        r = _expect(client.patch(deck, json={"field": "status",
                                             "value": "built"}),
                    (200,), "patch the deck")
        goldens.check(g, "patch-deck", observed(r))
        r = _expect(client.patch(deck, json={"field": "status"}), (422,),
                    "patch the deck with no value")
        goldens.check(g, "patch-deck-refused", observed(r))

        r = _expect(client.put(f"{deck}/notes/mulligan", json={
            "value": "Keep any hand with two lands and a ramp spell."}),
            (200,), "a note")
        goldens.check(g, "notes", observed(r))
        r = _expect(client.put(f"{deck}/shared", json={"shared": True}),
                    (200,), "share")
        goldens.check(g, "shared", observed(r))
        r = _expect(client.put(f"{deck}/shared", json={}), (422,),
                    "share with no flag")
        goldens.check(g, "shared-refused", observed(r))

        r = _expect(client.post(f"{deck}/swap", json={
            "out": "Llanowar Reborn", "into": "Cultivator Colossus",
            "why": "Trample body that puts lands onto the battlefield."}),
            (200,), "swap")
        goldens.check(g, "swap", observed(r))
        r = _expect(client.post(f"{deck}/swap", json={}), (422,), "swap {}")
        goldens.check(g, "swap-refused", observed(r))

        r = _expect(client.post(f"{deck}/entomb",
                                json={"names": ["Cultivator Colossus"]}),
                    (200,), "entomb")
        goldens.check(g, "entomb", observed(r))
        r = _expect(client.post(f"{deck}/entomb",
                                json={"names": "Cultivator Colossus"}),
                    (422,), "entomb with a string")
        goldens.check(g, "entomb-refused", observed(r))
        r = _expect(client.post(f"{deck}/graveyard/Cultivator Colossus/return"),
                    (200,), "return from the graveyard")
        goldens.check(g, "graveyard-return", observed(r))
        r = _expect(client.post(f"{deck}/graveyard/Not In The Deck/return"),
                    (404, 422), "return a card that is not there")
        goldens.check(g, "graveyard-return-refused", observed(r))
        _expect(client.post(f"{deck}/entomb",
                            json={"names": ["Cultivator Colossus"]}),
                (200,), "entomb, again")
        r = _expect(client.delete(f"{deck}/graveyard/Cultivator Colossus"),
                    (200,), "exile from the graveyard")
        goldens.check(g, "graveyard-exile", observed(r))

        # A created deck is a draft (ADR 13) and the artifacts refuse a
        # draft outright, so promotion comes first; with one justified card
        # it is allowed, and with one card the gate still fails, which is
        # what the unforced build records.
        r = _expect(client.patch(deck, json={"field": "stage",
                                             "value": "curated"}),
                    (200,), "promote")
        goldens.check(g, "promote", observed(r))
        r = _expect(client.post(f"{deck}/artifacts", json={}), (200, 422),
                    "build the artifacts")
        goldens.check(g, "artifacts-build", observed(r))
        r = _expect(client.post(f"{deck}/artifacts", json={"force": True}),
                    (200,), "build the artifacts, forced")
        goldens.check(g, "artifacts-build-forced", observed(r))
        r = _expect(client.get(f"{deck}/artifacts"), (200,), "the shelf")
        goldens.check(g, "artifacts-shelf", observed(r))
        r = _expect(client.get(f"{deck}/artifacts/primer-quick.md"), (200,),
                    "one artifact")
        goldens.check(g, "artifact", observed(r))
        r = _expect(client.get(f"{deck}/artifacts/swaps.md"), (404,),
                    "the artifact a first build does not write")
        goldens.check(g, "artifact-missing", observed(r))
        r = _expect(client.get(f"{deck}/log"), (200,), "the log")
        assert r.json()["entries"], "the edits above must have been recorded"
        goldens.check(g, "log", observed(r))

        r = _expect(client.post("/api/decks/import", json={
            "slug": imported, "name": "Imported", "text": tiny_pool.DECKLIST,
            "bracket": 4, "dry_run": True}), (200,), "import, dry run")
        goldens.check(g, "import-dry-run", observed(r))
        r = _expect(client.post("/api/decks/import", json={
            "slug": imported, "name": "Imported", "text": tiny_pool.DECKLIST,
            "bracket": 4}), (200,), "import")
        goldens.check(g, "import", observed(r))
        r = _expect(client.post("/api/decks/import", json={}), (422,),
                    "import {}")
        goldens.check(g, "import-refused", observed(r))
    finally:
        r = _expect(client.delete(deck), (422,), "delete without confirm")
        goldens.check(g, "delete-refused", observed(r))
        r = _expect(client.delete(deck, params={"confirm": slug}), (200,),
                    "delete")
        goldens.check(g, "delete", observed(r))
        client.delete(f"/api/decks/alice/{imported}",
                      params={"confirm": imported})
    assert client.get(deck).status_code == 404


# ------------------------------------------------------------------ admin

def test_admin_sequence(instance, goldens):
    g = "admin"
    client = instance.as_user("alice")

    goldens.check(g, "users", observed(_expect(client.get("/api/admin/users"),
                                               (200,), "the account list")))
    r = _expect(client.post("/api/admin/users",
                            json={"email": "someone@example.com"}),
                (503,), "an invite with no mail sender")
    goldens.check(g, "invite-unsendable", observed(r))
    r = _expect(client.post("/api/admin/users", json={}), (422,), "invite {}")
    goldens.check(g, "invite-refused", observed(r))

    r = _expect(client.patch("/api/admin/users/bob", json={"is_admin": True}),
                (200,), "grant admin")
    goldens.check(g, "patch-user", observed(r))
    _expect(client.patch("/api/admin/users/bob", json={"is_admin": False}),
            (200,), "revoke admin")
    r = _expect(client.patch("/api/admin/users/nobody-here",
                             json={"is_admin": True}), (404,),
                "patch a missing account")
    goldens.check(g, "patch-user-missing", observed(r))
    r = _expect(client.patch("/api/admin/users/alice",
                             json={"is_admin": False}), (409,),
                "demote the last admin")
    goldens.check(g, "patch-last-admin", observed(r))
    r = _expect(client.post("/api/admin/users/bob/reset"), (503,),
                "a reset link with no mail sender")
    goldens.check(g, "reset-unsendable", observed(r))

    # Sign the throwaway account in on a second client, revoke it from the
    # first, and watch the second lose its session.
    other = instance.new_client()
    assert other.post("/api/auth/login", json={
        "username": "throttle",
        "password": harness.PASSWORDS["throttle"]}).status_code == 200
    assert other.get("/api/auth/me").json()["authenticated"] is True
    r = _expect(client.delete("/api/admin/users/throttle/sessions"),
                (200, 204), "revoke every session")
    goldens.check(g, "sessions-revoked", observed(r))
    assert other.get("/api/auth/me").json()["authenticated"] is False

    harness.ensure_account(instance.scratch, "doomed")
    r = _expect(client.request("DELETE", "/api/admin/users/doomed", json={}),
                (422,), "delete an account without typing its name")
    goldens.check(g, "delete-user-refused", observed(r))
    r = _expect(client.request("DELETE", "/api/admin/users/doomed",
                               json={"confirm": "doomed"}),
                (200, 204), "delete an account")
    goldens.check(g, "delete-user", observed(r))

    for name in ("system", "storage", "claude", "activity", "traffic", "fly"):
        r = _expect(client.get(f"/api/admin/stats/{name}"), (200, 503),
                    f"stats/{name}")
        goldens.check(g, f"stats-{name}", observed(r))
