"""The contract suite, shown to bite.

`tests/contract/` is the referee the Go migration plays against
(docs/go-migration/PLAN.md, section 5): it has to fail when a server answers
the wrong shape, or it is a green check that checks nothing -- the failure
this repository has found under four different guards. So every assertion
the suite makes is a function in `contract.checks`, and this file runs each
one against an app that has been made wrong in exactly one way
(`contract.harness.MUTATIONS`) and asserts it raises. Then, once, it runs
the whole suite as wired, in a subprocess, against one of those mutations,
and asserts the run itself goes red -- the function-level proofs say the
assertions bite, this says the plumbing delivers them.

A mutation is applied in-process only. Over a socket there is no seam to
break on purpose, and proving the rig does not need one.
"""

from __future__ import annotations

import contextlib
import os
import re
import subprocess
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from contract import checks, harness
from contract.checks import observed
from contract.goldens import Goldens
from contract.harness import MUTATIONS
from contract.routes import ROUTES

ROOT = Path(__file__).resolve().parents[1]

#: Every mutation this file proves is caught; `test_every_mutation_is_proven`
#: holds it equal to the harness's table, so a mutation added without a
#: proof fails here rather than sitting unexercised.
PROVEN: set[str] = set()


def proves(name: str):
    PROVEN.add(name)
    return pytest.mark.usefixtures()


@pytest.fixture(scope="module")
def scratch(tmp_path_factory):
    """One seeded scratch for every mutation: seeding is the slow part and
    the mutations wrap the app, not the data."""
    where = harness.make_scratch(tmp_path_factory.mktemp("contract-proof"))
    harness.seed(where)
    return where


@contextlib.contextmanager
def broken(scratch, mutation: str):
    try:
        with harness.open_instance("in-process", scratch,
                                   mutation=mutation) as instance:
            yield instance
    finally:
        harness.undo_mutations()


@proves("envelope")
def test_a_renamed_error_envelope_is_caught(scratch):
    with broken(scratch, "envelope") as instance:
        refused = instance.as_user(None).get("/api/decks")
        assert refused.status_code == 401
        with pytest.raises(AssertionError, match="detail"):
            checks.check_refused_without_session(refused, "/api/decks")
        missing = instance.as_user("alice").get("/api/decks/local/nope")
        with pytest.raises(AssertionError, match="detail"):
            checks.check_error_envelope(missing)


@proves("open-route")
def test_a_route_that_opens_by_accident_is_caught(scratch):
    with broken(scratch, "open-route") as instance:
        answered = instance.as_user(None).get("/api/decks")
        assert answered.status_code == 200, "the mutation did not take"
        with pytest.raises(AssertionError, match="401"):
            checks.check_refused_without_session(answered, "/api/decks")


@proves("admin-404")
def test_an_admin_refusal_that_turns_into_a_404_is_caught(scratch):
    with broken(scratch, "admin-404") as instance:
        response = instance.as_user("bob").get("/api/admin/users")
        assert response.status_code == 404
        with pytest.raises(AssertionError, match="403"):
            checks.check_admin_refused(response, "/api/admin/users")


@proves("leak-403")
def test_a_private_deck_leaking_as_403_is_caught(scratch, run_id="proof"):
    with broken(scratch, "leak-403") as instance:
        bob = instance.as_user("bob")
        slug = f"ct-leak-{run_id}"
        assert bob.post("/api/decks", json={
            "slug": slug, "name": slug, "commander": ["Gyome, Master Chef"],
        }).status_code in (200, 201)
        try:
            response = instance.as_user("alice").get(f"/api/decks/bob/{slug}")
            assert response.status_code == 403
            with pytest.raises(AssertionError, match="ADR 5"):
                checks.check_not_found_not_forbidden(response, "bob's deck")
        finally:
            instance.as_user("bob").delete(f"/api/decks/bob/{slug}",
                                           params={"confirm": slug})


@proves("drop-field")
def test_a_dropped_field_is_caught_by_the_golden(scratch):
    with broken(scratch, "drop-field") as instance:
        response = instance.as_user("alice").get("/api/decks")
        assert response.status_code == 200
        assert "writable" not in response.json()[0], "the mutation did not take"
        with pytest.raises(AssertionError, match="drifted"):
            Goldens().check("reads", "decks", observed(response))


@proves("status-drift")
def test_a_drifted_status_is_caught_by_the_golden(scratch):
    with broken(scratch, "status-drift") as instance:
        response = instance.as_user(None).get("/api/health")
        assert response.status_code == 203
        with pytest.raises(AssertionError, match="status"):
            Goldens().check("reads", "health", observed(response))


@proves("header-drop")
def test_a_missing_security_header_is_caught(scratch):
    with broken(scratch, "header-drop") as instance:
        response = instance.as_user(None).get("/api/health")
        with pytest.raises(AssertionError, match="x-frame-options"):
            checks.check_security_headers(response, "/api/health")


@proves("retry-after-drop")
def test_a_429_without_retry_after_is_caught(scratch):
    from mtglab.auth import ratelimit
    with broken(scratch, "retry-after-drop") as instance:
        client = instance.as_user(None)
        try:
            for _ in range(ratelimit.PER_ACCOUNT.failures):
                client.post("/api/auth/login", json={"username": "throttle",
                                                     "password": "wrong"})
            throttled = client.post("/api/auth/login", json={
                "username": "throttle", "password": "wrong"})
            assert throttled.status_code == 429
            with pytest.raises(AssertionError, match="Retry-After"):
                checks.check_rate_limited(throttled)
        finally:
            harness.reset_rate_limits(scratch)


def test_every_mutation_is_proven():
    """A mutation nobody runs is a claim nobody checks."""
    assert set(MUTATIONS) == PROVEN, (
        f"unproven: {sorted(set(MUTATIONS) - PROVEN)}; "
        f"proofs without a mutation: {sorted(PROVEN - set(MUTATIONS))}")


def test_the_suite_as_wired_goes_red_against_a_broken_shape():
    """The plumbing, not just the assertions: a real `pytest tests/contract`
    against the envelope mutation must fail, and fail across the whole
    protected sweep -- every 401 the middleware writes now says `message`.

    A subprocess, because the mutation hook is read from the environment by
    the package's own fixture and the point is to run that fixture, the
    parametrisation and the goldens exactly as CI does.
    """
    env = dict(os.environ, MTGLAB_CONTRACT_MUTATE="envelope",
               PYTHONDONTWRITEBYTECODE="1")
    proc = subprocess.run(
        [sys.executable, "-m", "pytest", "tests/contract", "-q",
         "-p", "no:cacheprovider", "--no-header"],
        cwd=ROOT, env=env, capture_output=True, text=True, timeout=900)
    assert proc.returncode != 0, (
        "the contract suite passed against a server whose every error body "
        "says `message` instead of `detail`:\n" + proc.stdout[-2000:])
    summary = re.search(r"(\d+) failed, (\d+) passed", proc.stdout)
    assert summary, proc.stdout[-2000:]
    failed = int(summary.group(1))
    assert failed >= len(ROUTES.protected), (
        f"only {failed} contract tests failed; every one of the "
        f"{len(ROUTES.protected)} protected-route checks should have")
    assert "test_contract_routes.py" in proc.stdout
