"""API surface.

Two kinds of test live here, and the difference is deliberate.

The endpoints that read a deck file run against a scratch deck directory the
`client` fixture writes (decks are live app data, ADR 30 -- a checkout has
none) and need no card pool, because a fresh clone has none until
`data refresh` runs and the app has to stay usable in that state rather than
500ing.

The endpoints that look a card up -- swap, add, suggestions, search, the Tier 1
jobs -- take the `pool` fixture and the synthetic deck from `tiny_pool`.
They used to run against the real decks too, gated on `data/mtg.duckdb`, which
meant they skipped on every pull request and passed only on the maintainer's
laptop. Card facts cannot be faked (rule 1) and the 500MB pool cannot go in
CI (ADR 6), so the fixture is a real DuckDB pool of 21 real cards with a
legal 99 built out of them.
"""

import base64
import hashlib
import sys
import time
from dataclasses import replace
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

fastapi = pytest.importorskip("fastapi")
pytest.importorskip("httpx")

from fastapi.testclient import TestClient  # noqa: E402

import tiny_pool  # noqa: E402
from mtglab import config  # noqa: E402
from mtglab.api import jobs, service  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.api.deps import deck_source, library  # noqa: E402
from mtglab.cards import db  # noqa: E402
from mtglab.decks.library import LOCAL_OWNER, Library  # noqa: E402
from mtglab.decks.model import Deck  # noqa: E402
from mtglab.decks.source import DeckNotFound, MemoryDeckSource  # noqa: E402

# The job pool has a single worker, so a queued job may sit behind another
# test's. Poll on a clock rather than an iteration count.
JOB_TIMEOUT_S = 60


@pytest.fixture
def client(tmp_path):
    """The app over a scratch deck directory holding the fixture deck.

    These tests used to run against the repository's real decks. ADR 30 made
    decks live app data — a checkout has none — so the fixture writes its own:
    `mono-green` as a real file, exercising the file-backed tier end to end,
    plus an underscore-prefixed directory to pin that scaffolding is skipped.
    The card pool is deliberately not arranged here: the deck-file endpoints
    must stay usable with no pool at all rather than 500ing.
    """
    jobs.clear()
    root = tmp_path / "decks"
    (root / "mono-green").mkdir(parents=True)
    (root / "mono-green" / "deck.yaml").write_text(
        tiny_pool.mono_green_deck().dump(), encoding="utf-8")
    # A built sibling, because two things key off `status`: the stance default
    # follows it (ADR 15), and nothing else in the fixture exercises "built".
    built = tiny_pool.mono_green_deck()
    built.slug, built.status, built.name = ("mono-green-built", "built",
                                            "Mono-Green Fixture, Built")
    (root / "mono-green-built").mkdir(parents=True)
    (root / "mono-green-built" / "deck.yaml").write_text(
        built.dump(), encoding="utf-8")
    (root / "_template").mkdir()
    (root / "_template" / "deck.yaml").write_text(
        "slug: _template\nname: Scaffolding\ncards: []\n", encoding="utf-8")
    with config.use_paths(decks_dir=root), TestClient(create_app()) as c:
        yield c


@pytest.fixture
def pool(tmp_path):
    """A real, queryable pool -- built rather than borrowed.

    The card-fact endpoints (swap, add, suggestions, search) look every name
    up, so without this they were gated on `data/mtg.duckdb` being present.
    That file is a 500MB download nobody puts in CI, so 29 tests skipped on
    every pull request and passed only on the maintainer's laptop: the worst
    shape a test can have, because the green check was reporting on a suite
    five points of coverage smaller than the one being read.

    `tiny_pool.build` produces a genuine DuckDB pool in about a second.
    """
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


def await_job(client, job_id: str) -> dict:
    """Block until a job leaves the queue, or fail with what it was doing."""
    deadline = time.monotonic() + JOB_TIMEOUT_S
    body = {}
    while time.monotonic() < deadline:
        body = client.get(f"/api/jobs/{job_id}").json()
        if body["status"] in ("done", "error"):
            return body
        time.sleep(0.05)
    pytest.fail(f"job did not finish in {JOB_TIMEOUT_S}s: {body}")


# -------------------------------------------------------------------- meta

def test_health_reports_pool_state(client):
    body = client.get("/api/health").json()
    assert "pool" in body
    assert isinstance(body["oracle_cards"], int)


def test_every_response_carries_the_security_headers(client):
    """On the API and on the static shell alike, because the middleware sits
    outside routing. HSTS is deliberately absent here: this client is the
    local, plain-HTTP configuration, and a Strict-Transport-Security header
    served over HTTP is at best ignored."""
    for path in ("/api/health", "/"):
        r = client.get(path)
        assert r.headers["X-Content-Type-Options"] == "nosniff"
        assert r.headers["X-Frame-Options"] == "DENY"
        assert r.headers["Referrer-Policy"] == "same-origin"
        assert r.headers["Permissions-Policy"] == (
            "camera=(self), microphone=(), geolocation=()")
        assert "Strict-Transport-Security" not in r.headers


def test_the_camera_door_is_the_one_sensor_the_policy_allows(client):
    """Asserted as a truth table rather than as the literal string above,
    because the literal is what let this ship broken.

    `camera=()` denies the origin itself, so the camera import (ADR 34) could
    not open a camera on the deployed instance at all -- `getUserMedia`
    rejects and the door reports a refusal the reader never made. The string
    assertion passed the whole time, being an exact match for the bug. What
    matters is which capability each entry grants and to whom, so that is
    what is checked: the camera door works, and nothing else is reachable.
    """
    policy = client.get("/api/health").headers["Permissions-Policy"]
    entries = dict(
        part.strip().split("=", 1) for part in policy.split(",") if part.strip()
    )
    # The one sensor with a door: allowed to this origin, and only this one.
    # `()` here would break the feature; `*` would hand it to any frame.
    assert entries["camera"] == "(self)"
    # The two the app has no use for: denied to everyone, self included.
    assert entries["microphone"] == "()"
    assert entries["geolocation"] == "()"


def test_large_responses_are_gzipped(client):
    """The wire format for anything over a kilobyte, when the client asks.

    Fly's edge turns out to compress on its own -- discovered on the deployed
    instance after this middleware merged -- but that is one proxy's
    undocumented habit, and the app owning compression is what holds on any
    host, including a laptop with no edge in front of it. `/api/colors` is the
    probe because it is comfortably over the floor and needs no pool. The
    client re-inflates transparently, so the body must still parse --
    asserting the header alone would pass against a response gzip had mangled.
    """
    r = client.get("/api/colors", headers={"Accept-Encoding": "gzip"})
    assert r.status_code == 200
    assert r.headers.get("Content-Encoding") == "gzip"
    assert r.json()["combinations"]


def test_small_responses_are_not_gzipped(client):
    """Under the floor the body ships whole -- a handful of bytes saved is not
    worth a Content-Encoding header and a decompress on every poll. The probe
    is `/api/jobs` on a cleared registry: two bytes, and the polling endpoint
    is exactly the traffic the floor exists to spare. Not `/api/health`, whose
    size depends on whether the machine running the suite has a pool."""
    r = client.get("/api/jobs", headers={"Accept-Encoding": "gzip"})
    assert r.status_code == 200
    assert r.headers.get("Content-Encoding") != "gzip"


def test_a_refusal_carries_the_security_headers_too():
    """The headers middleware wraps the auth middleware, not just the routes.

    The 401 never reaches routing -- it is manufactured inside `auth.py` --
    so this is the response that would silently lose the headers if the
    registration order in `create_app` ever flipped."""
    with TestClient(create_app(require_auth=True, secure_cookies=True)) as c:
        r = c.get("/api/decks")
        assert r.status_code == 401
        assert r.headers["X-Content-Type-Options"] == "nosniff"
        # And with TLS assumed (the same `secure` the cookie flag uses), the
        # transport pin appears -- a year, no preload, argued in `create_app`.
        assert r.headers["Strict-Transport-Security"] == "max-age=31536000"


# ------------------------------------------------------------------- decks

def test_decks_lists_the_library(client):
    body = client.get("/api/decks").json()
    assert isinstance(body, list)
    slugs = {d["slug"] for d in body}
    assert "mono-green" in slugs
    assert "_template" not in slugs


def test_deck_list_carries_the_gate_counts(client):
    """The shelf renders a deck with a banned card identically to a clean one
    unless the list endpoint says so, and fetching /validate per deck would be
    an N+1 on every page load. `errors` may be None when the pool is missing
    -- that is 'the gate did not run', not 'the deck passed'."""
    body = client.get("/api/decks").json()
    assert body, "no decks found"
    for deck in body:
        assert "errors" in deck and "warnings" in deck
        assert deck["errors"] is None or isinstance(deck["errors"], int)
        assert deck["warnings"] is None or isinstance(deck["warnings"], int)
        # Never report a count without the pool that produced it.
        assert (deck["errors"] is None) == (deck["warnings"] is None)


def test_deck_list_gate_counts_agree_with_the_validate_endpoint(pool, client):
    """Two code paths compute the same thing; they must not drift.

    Takes `pool` so the gate actually runs. What it reports does not matter
    here -- against a 21-card pool the real decks are mostly unknown
    cards, and that is fine, because the subject is whether the two paths
    agree rather than whether any deck is clean.
    """
    for deck in client.get("/api/decks").json():
        assert deck["errors"] is not None, f"{deck['slug']}: the gate did not run"
        rep = client.get(f"/api/decks/local/{deck['slug']}/validate").json()
        assert deck["errors"] == len(rep["errors"]), \
            f"{deck['slug']} error count disagrees with /validate"
        assert deck["warnings"] == len(rep["warnings"]), \
            f"{deck['slug']} warning count disagrees with /validate"
        assert rep["ok"] == (deck["errors"] == 0)


def test_the_commander_card_carries_its_oracle_id(swappable):
    """The motion tier (ADR 32) is keyed on oracle_id, and the deck page
    learns it from this payload rather than from a second request."""
    with swappable as client:
        card = client.get("/api/decks/local/mono-green").json()["commander_card"]
    assert card["oracle_id"], "the commander resolved against a real pool"


def test_deck_detail_has_every_card_with_its_why(client):
    body = client.get("/api/decks/local/mono-green").json()
    assert body["total_cards"] == 99
    assert body["land_count"] == 95, "the fixture's 95 Forests"
    assert all(c["why"] for c in body["cards"]), "a card lost its rationale"


def test_a_mistyped_api_path_is_a_json_404_not_the_shell(client):
    """The SPA catch-all must refuse /api/* misses. Before this was pinned, a
    mistyped endpoint returned 200 with text/html -- a *success* carrying a
    web page, which is the silent-wrong-answer shape this project is written
    against."""
    for path in ("/api/no-such-endpoint", "/api"):
        resp = client.get(path)
        assert resp.status_code == 404, path
        assert resp.headers["content-type"].startswith("application/json"), path
        assert "no such endpoint" in resp.json()["detail"]

    # `//api/decks` does not match the real route (Starlette matches raw
    # paths) and lands on the catch-all, where a naive `startswith("api/")`
    # reads it as a frontend route and serves the shell. The guard normalises
    # first, through the same helper the auth middleware uses. The full URL
    # form is deliberate: a bare `client.get("//api/decks")` parses `api` as
    # a host and never sends this path.
    resp = client.get("http://testserver//api/decks")
    assert resp.status_code == 404
    assert resp.headers["content-type"].startswith("application/json")
    assert "no such endpoint" in resp.json()["detail"]


def test_the_spa_catch_all_refuses_a_path_traversal():
    """The catch-all serves a bundle file only on an exact allowlist match, so a
    raw traversal path -- which reaches the handler un-normalised the same way
    `//api` does, since `WEB_DIST / "../../../etc/hosts"` would not collapse the
    `..` in the string -- is not a known name and falls through to the shell.
    The client normalises `..` away before sending, so the handler is exercised
    directly. The payload resolves to a real out-of-tree file, which the earlier
    `WEB_DIST / full_path` + `is_file()` form would have served."""
    from mtglab.api.app import WEB_DIST, create_app

    app = create_app()
    spa = next(r.endpoint for r in app.routes  # type: ignore[attr-defined]
               if getattr(r, "name", None) == "spa")

    web_root = WEB_DIST.resolve()
    depth = len(web_root.parts)
    payload = "../" * depth + "etc/hosts"
    assert (web_root / payload).resolve().is_file(), \
        "test payload must resolve to a real out-of-tree file to be meaningful"

    resp = spa(payload)
    assert resp.path == WEB_DIST / "index.html", \
        "a traversal path must fall back to the shell, never serve out-of-tree"


def test_frontend_routes_still_get_the_shell(client):
    """The refusal above must not overreach: an SPA route is the shell's to
    serve, and so is the root."""
    for path in ("/", "/decks/gyome-food", "/apiary"):
        resp = client.get(path)
        assert resp.status_code == 200, path
        assert resp.headers["content-type"].startswith("text/html"), path


def test_the_shell_and_its_assets_are_served_revalidated(client):
    """The bug this pins cost a black screen and an hour, 2026-08-13.

    `web/vite.config.ts` emits stable filenames on purpose, because the bundle
    is committed so `mtglab ui` needs no Node. Starlette sends an etag and a
    last-modified, which is freshness *if the browser asks* — and with no
    `Cache-Control` at all it may assign its own heuristic lifetime and not
    ask. Safari did: after a deploy it re-fetched `app.js` and never requested
    `DeckDetail.js`, so an old panel met a new server contract and the page
    went black.

    The failure mode is a returning visitor running two halves of two
    different versions, which no amount of testing either half would catch.
    """
    for path in ("/", "/decks/gyome-food"):
        assert client.get(path).headers.get("cache-control") == "no-cache", path

    asset = client.get("/assets/app.js")
    if asset.status_code == 404:                      # pragma: no cover
        pytest.skip("no built bundle in this tree")
    assert asset.headers.get("cache-control") == "no-cache"


def test_a_tarot_picture_is_served_as_a_picture(client):
    """`image/webp`, and named by us rather than by the operating system.

    Starlette resolves a static file's type through `mimetypes`, which reads
    the *host's* database. The slim image has no `/etc/mime.types`, so `.webp`
    answered `None` there and all 78 cards went out as
    `application/octet-stream` — on the deployed instance, and only there.
    Browsers sniff the bytes and render them, so it was invisible in
    production as well as in the suite.

    This test cannot see that on its own: macOS and CI's ubuntu both know the
    type, so it passed before the fix and passes after. What it pins is the
    contract; the `image` job in `ci.yml` asks the *container* the same
    question, because that is the only machine that has ever answered it
    differently. Two halves of one check, and neither is sufficient alone —
    the same division `tests/test_packaging.py` makes about the Dockerfile.
    """
    card = client.get("/tarot/00-fool.webp")
    if card.status_code == 404:                       # pragma: no cover
        pytest.skip("no packaged tarot art in this tree")
    assert card.headers["content-type"] == "image/webp"


def test_an_unchanged_asset_still_answers_304(client):
    """Which is what makes `no-cache` cheap rather than expensive.

    `no-cache` means "revalidate", not "do not store" — so the conditional
    request it forces must still come back empty when nothing has changed. If
    this ever fails, the fix above has quietly turned into a full re-download
    of the bundle on every page load.
    """
    first = client.get("/assets/app.js")
    if first.status_code == 404:                      # pragma: no cover
        pytest.skip("no built bundle in this tree")
    etag = first.headers.get("etag")
    assert etag, "revalidation needs something to revalidate against"

    again = client.get("/assets/app.js", headers={"If-None-Match": etag})
    assert again.status_code == 304
    assert not again.content


def test_missing_deck_is_a_404_not_a_500(client):
    resp = client.get("/api/decks/local/does-not-exist")
    assert resp.status_code == 404
    assert "does-not-exist" in resp.json()["detail"]


def test_missing_deck_stats_is_also_404(client):
    assert client.get("/api/decks/local/nope/stats").status_code == 404
    assert client.get("/api/decks/local/nope/validate").status_code == 404


def test_validate_returns_structured_issues(client):
    body = client.get("/api/decks/local/mono-green/validate").json()
    assert set(body) == {"ok", "errors", "warnings"}
    assert isinstance(body["errors"], list)


def test_stats_are_json_serialisable(client):
    """The curve buckets are dataclasses; they must be flattened for the wire."""
    body = client.get("/api/decks/local/mono-green/stats").json()
    assert body["total_cards"] == 99
    assert body["curve"]["buckets"], "no curve buckets"
    assert isinstance(body["curve"]["buckets"][0]["mv"], int)
    assert isinstance(body["categories"], list)


def test_the_log_endpoint_shows_what_an_edit_did(swappable, monkeypatch):
    """ADR 28's HTTP surface, and the one thing only it can check: that the
    edit routes reach `_commit` with an actor and that the read route hands
    the same rows back.

    `_no_deck_log` comes off for this test alone. That stub keeps the rest of
    the suite out of the developer's real history; a test of the log that ran
    against it would pass with the recording deleted. The data directory is a
    scratch one either way — `pool`, behind `swappable`, is what points it
    there — so the rows land in a temporary `app.db`.
    """
    from mtglab.api import service
    from mtglab.decks import log

    monkeypatch.setattr(service, "log", log)
    with swappable as client:
        assert client.delete(
            "/api/decks/local/mono-green/cards/Sol Ring").status_code == 200
        body = client.get("/api/decks/local/mono-green/log").json()

    assert body["slug"] == "mono-green"
    assert [e["summary"] for e in body["entries"]] == ["entombed Sol Ring"]
    assert body["entries"][0]["action"] == "entomb"
    # Auth is off in this client, so there is no account to name and `local`
    # is the owner segment rather than a username. See `Library.actor`.
    assert body["entries"][0]["actor"] is None


def test_the_log_endpoint_is_404_for_a_deck_that_is_not_there(client):
    """The whole authorisation check, and it is the source's, not a second
    rule written here (ADR 28)."""
    assert client.get("/api/decks/local/nope/log").status_code == 404


def test_suggestions_are_offered_for_a_banned_card(swappable):
    """The fixture deck runs Primeval Titan, which is banned. The endpoint
    should name the offender and shortlist legal cards that resemble it."""
    with swappable as client:
        body = client.get("/api/decks/local/mono-green/suggestions").json()
        assert body["pool_available"] is True
        targets = {t["card"]: t for t in body["targets"]}
        assert "Primeval Titan" in targets, body

        target = targets["Primeval Titan"]
        assert target["code"] == "banned"
        assert target["why"], \
            "the slot's rationale is what makes a suggestion legible"
        assert target["candidates"], "a banned card with no shortlist helps nobody"

        for candidate in target["candidates"]:
            # Mono-green deck: anything outside {G} is not a legal suggestion.
            assert set(candidate["color_identity"]) <= {"G"}, candidate["name"]
            assert candidate["reasons"], "a score with no reason is not a suggestion"
            assert 0.0 <= candidate["score"] <= 1.2


def test_suggestions_never_include_the_card_being_replaced(swappable):
    with swappable as client:
        body = client.get("/api/decks/local/mono-green/suggestions").json()
        for target in body["targets"]:
            names = {c["name"] for c in target["candidates"]}
            assert target["card"] not in names


def test_suggestions_never_include_a_card_already_in_the_deck(swappable):
    """Regal Behemoth and Vorinclex are in the list and would otherwise score
    well for the Titan's slot -- suggesting a card you already run is noise."""
    with swappable as client:
        body = client.get("/api/decks/local/mono-green/suggestions").json()
        held = {c["name"] for c in client.get("/api/decks/local/mono-green").json()["cards"]}
        for target in body["targets"]:
            assert not ({c["name"] for c in target["candidates"]} & held)


def test_a_clean_deck_has_nothing_to_suggest(clean_client):
    """The endpoint answers "what would fix the gate", so a deck the gate
    passes must return an empty list rather than unsolicited upgrades."""
    with clean_client as client:
        body = client.get("/api/decks/local/mono-green/suggestions").json()
        assert body["targets"] == []


def test_suggestions_for_a_missing_deck_are_a_404(client):
    assert client.get("/api/decks/local/nope/suggestions").status_code == 404


def test_suggestion_limit_is_bounded(client):
    assert client.get("/api/decks/local/mono-green/suggestions",
                      params={"limit": 999}).status_code == 422


# ------------------------------------------------- decks, from elsewhere
#
# The point of the deck source seam (ADR 4): the endpoints read whatever the
# request scope hands them, so hosting can add a second tier by swapping one
# dependency instead of touching thirteen handlers. These tests are the proof,
# and they are also the cheapest way to exercise library states the filesystem
# makes awkward.

def library_over(source):
    """A `Library` serving one source as the local user's own decks.

    **The seam moved with ADR 22.** Routes resolve the owner segment through
    `deps.library`, so overriding `deck_source` alone no longer reaches them —
    they fall back to the real filesystem and the test quietly stops testing
    what it names. Overriding both is what keeps these fixtures honest.
    """
    class _One(Library):
        def __init__(self) -> None:
            super().__init__(username=None, user_id=None, maintainer=None,
                             authenticated=False)

        def source_for(self, owner):
            if owner.casefold() != LOCAL_OWNER:
                raise DeckNotFound(owner)
            return source

        def mine(self):
            return source

        def visible(self):
            return [(LOCAL_OWNER, source)]

    return _One()


@pytest.fixture
def in_memory_client():
    """The app, serving exactly the decks a test puts in front of it.

    The source is built once and handed back on every request, not constructed
    per call. For a file-backed source that distinction is invisible because
    the state is on disk; for this one the instance *is* the store, so a fresh
    one per request would quietly discard every write.
    """
    def make(decks):
        source = MemoryDeckSource(decks)
        app = create_app()
        app.dependency_overrides[deck_source] = lambda: source
        app.dependency_overrides[library] = lambda: library_over(source)
        return TestClient(app)
    return make


def test_endpoints_read_the_request_scope_not_the_filesystem(in_memory_client):
    only = tiny_pool.mono_green_deck()
    with in_memory_client([only]) as client:
        assert [d["slug"] for d in client.get("/api/decks").json()] == ["mono-green"]
        assert client.get("/api/decks/local/mono-green").status_code == 200
        # A slug this scope was not handed, however plausible, is absent.
        assert client.get("/api/decks/local/arahbo-cats").status_code == 404


def test_the_request_scope_does_not_leak_into_the_public_schema(client):
    """A dependency injected by annotation can end up documented as a query
    parameter if it is wired wrong. The endpoint would still work and every
    other test would still pass, so check the schema itself."""
    schema = client.get("/openapi.json")
    assert schema.status_code == 200
    paths = schema.json()["paths"]
    assert [p["name"] for p in paths["/api/decks"]["get"].get("parameters", [])] == []
    assert [p["name"] for p in
            paths["/api/decks/{owner}/{slug}"]["get"]["parameters"]] == [
        "owner", "slug"]


# ------------------------------------------------------------------ swaps
#
# Every one of these runs against an in-memory source. A test that wrote to
# decks/ would be editing the repository's own source of truth, and the seam
# exists precisely so it does not have to.

@pytest.fixture
def swappable(in_memory_client, pool):
    """The fixture deck, held in memory so writes go nowhere, with a card pool
    behind it so card facts can actually be looked up.

    This was the real Goreclaw list until the pool fixture existed. Same
    shape -- a mono-green commander, a legal 99, exactly one banned card --
    but built out of `tiny_pool.CARDS`, so the gate has something real to
    catch without needing all 35,000 cards to find it.
    """
    return in_memory_client([tiny_pool.mono_green_deck()])


@pytest.fixture
def clean_client(in_memory_client, pool):
    """The same deck with a legal card in the Titan's slot, so "the gate found
    nothing" is testable as its own case rather than inferred."""
    return in_memory_client([tiny_pool.mono_green_deck(clean=True)])


@pytest.fixture
def sim_client(in_memory_client, pool):
    """A deck the simulator can actually compile.

    Tier 1 needs a card record for every slot, so these were pointed at
    `gyome-food` and the real pool -- which is why the whole simulation
    surface, jobs and polling included, went untested in CI. The fixture deck
    compiles against `tiny_pool`, so the job contract the UI depends on is
    now exercised on every pull request.

    A background job captures the `DeckSource` and outlives its request, which
    is fine here: `await_job` blocks inside the test, so `pool` is still on
    the stack when the worker reads it.
    """
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck(clean=True)]) as c:
        yield c


def test_a_swap_replaces_the_card_and_clears_the_gate(swappable):
    with swappable as client:
        before = client.get("/api/decks/local/mono-green/validate").json()
        assert any(i["card"] == "Primeval Titan" for i in before["errors"])

        resp = client.post("/api/decks/local/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "Trample body that puts lands onto the battlefield; ramp and "
                   "threat in one card, same as the slot it replaces.",
        })
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert (body["swapped_out"], body["swapped_in"]) == \
            ("Primeval Titan", "Cultivator Colossus")
        assert body["ok"] is True and body["errors"] == []

        # ...and the deck itself now reflects it.
        names = {c["name"] for c in client.get("/api/decks/local/mono-green").json()["cards"]}
        assert "Cultivator Colossus" in names
        assert "Primeval Titan" not in names


def test_a_swap_keeps_the_slot_and_records_the_given_why(swappable):
    with swappable as client:
        client.post("/api/decks/local/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A rationale a human wrote.",
        })
        card = next(c for c in client.get("/api/decks/local/mono-green").json()["cards"]
                    if c["name"] == "Cultivator Colossus")
        assert card["why"] == "A rationale a human wrote."
        assert card["category"] == "threat", "the slot it filled should carry over"


# Split by what each refusal needs. The first group is checked before the
# pool is even opened, so it runs on a fresh clone -- which is where most of
# these mistakes actually get made.

@pytest.mark.parametrize(("payload", "expected"), [
    ({"out": "Primeval Titan", "into": "Cultivator Colossus", "why": "  "},
     "needs a `why`"),
    ({"out": "Not In Deck", "into": "Cultivator Colossus", "why": "x"},
     "not in this deck"),
    ({"out": "Primeval Titan", "into": "Sol Ring", "why": "x"},
     "already in this deck"),
    ({"out": "Primeval Titan", "into": "Goreclaw, Terror of Qal Sisma", "why": "x"},
     "is the commander"),
])
def test_a_swap_refused_on_the_deck_alone_needs_no_pool(swappable, payload, expected):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/swap", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]


@pytest.mark.parametrize(("payload", "expected"), [
    ({"out": "Primeval Titan", "into": "Not A Real Card At All", "why": "x"},
     "pool knows"),
    ({"out": "Primeval Titan", "into": "Rhystic Study", "why": "x"},
     "outside the commander's"),
    ({"out": "Primeval Titan", "into": "Black Lotus", "why": "x"},
     "not legal in Commander"),
])
def test_a_swap_refused_on_a_card_fact_is_looked_up(swappable, payload, expected):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/swap", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]


def test_a_swap_without_a_pool_says_so_rather_than_guessing(in_memory_client,
                                                              tmp_path):
    """Legality and colour identity are card facts, and CLAUDE.md rule 1 says
    those are looked up. With no card pool there is nothing to look them up in, so
    the swap is refused rather than waved through.

    Points at an empty directory rather than skipping when a card pool happens to
    be present. The fresh-clone path is a behaviour worth pinning, and it used
    to be tested only on machines that had never run `data refresh` -- which
    is to say, almost nowhere.
    """
    with config.use_paths(data_dir=tmp_path / "absent"), \
            in_memory_client([tiny_pool.mono_green_deck()]) as client:
        assert client.get("/api/health").json()["pool"] is False
        resp = client.post("/api/decks/local/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A real rationale."})
        assert resp.status_code == 422
        assert "pool" in resp.json()["detail"]


def test_a_refused_swap_changes_nothing(swappable):
    """The check that matters most: a rejection must not leave the deck half
    edited."""
    with swappable as client:
        before = client.get("/api/decks/local/mono-green").json()["cards"]
        client.post("/api/decks/local/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Rhystic Study", "why": "no"})
        assert client.get("/api/decks/local/mono-green").json()["cards"] == before


def test_a_read_only_source_refuses_every_swap():
    """What the hosted two-tier model needs: curated decks stay read-only for
    someone who is not the maintainer, checked in one place."""
    deck = tiny_pool.mono_green_deck()
    app = create_app()
    _ro = MemoryDeckSource([deck], writable=False)
    app.dependency_overrides[deck_source] = lambda: _ro
    app.dependency_overrides[library] = lambda: library_over(_ro)
    with TestClient(app) as client:
        resp = client.post("/api/decks/local/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus", "why": "x"})
        assert resp.status_code == 403
        assert "read-only" in resp.json()["detail"]


# --------------------------------------------------------------- the edits
#
# The rest of the operations (ADR 12). Same in-memory source as the swaps, and
# the same property being checked each time: one narrow change, the gate re-run
# and reported, and a refusal that writes nothing.

@pytest.fixture
def draft_client(in_memory_client, pool):
    """A draft, so the rule 4 bend is exercisable. Cards keep their `why`s --
    what makes it a draft is the stage, and a draft with rationales already
    written is one of ADR 13's four real combinations."""
    return in_memory_client([tiny_pool.mono_green_deck(stage="draft")])


def test_a_card_can_be_added_and_the_gate_comes_back(swappable):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land",
            "why": "Enters with a +1/+1 counter to move onto a fatty."})
        assert resp.status_code == 200, resp.json()
        body = resp.json()
        assert body["added"] == "Llanowar Reborn"
        assert "ok" in body and "errors" in body
        names = {c["name"] for c in
                 client.get("/api/decks/local/mono-green").json()["cards"]}
        assert "Llanowar Reborn" in names


def test_a_card_outside_the_commanders_identity_is_refused(swappable):
    """Rule 2: identity comes from Scryfall's `color_identity`. Goreclaw is
    mono-green, so a card with any other pip in its identity cannot go in."""
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Swords to Plowshares", "category": "interaction",
            "why": "One mana, exiles anything."})
        assert resp.status_code == 422
        assert "outside the commander's" in resp.json()["detail"]


def test_a_card_the_pool_does_not_know_is_refused(swappable):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Definitely Not A Card", "category": "ramp", "why": "x"})
        assert resp.status_code == 422
        assert "not a card the pool knows" in resp.json()["detail"]


def test_an_unknown_category_is_refused_before_any_lookup(swappable):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Sol Ring", "category": "rampp", "why": "typo"})
        assert resp.status_code == 422
        assert "is not a category" in resp.json()["detail"]


def test_a_curated_deck_refuses_a_card_with_no_rationale(swappable):
    """Rule 4 at the boundary. The tool declines to invent one -- there is no
    code path here that fills the field in."""
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land", "why": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_a_draft_accepts_a_card_that_still_owes_its_rationale(draft_client):
    """The one bend (ADR 13): a draft is honestly incomplete and counts what it
    owes, rather than refusing work while the thinking is still to come."""
    with draft_client as client:
        before = client.get("/api/decks/local/mono-green").json()["needs_rationale"]
        resp = client.post("/api/decks/local/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land"})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["needs_rationale"] == before + 1


def test_removing_a_99_card_entombs_it(swappable):
    """ADR 27: the delete from the 99 is an entombment, not a disappearance.

    The card leaves the 99 and lands in the graveyard with its category and
    its `why` intact -- and the response says `entombed`, not `removed`, so a
    client can tell the user where the card went. Still needs no pool:
    burying a card is a fact about this deck file, not about Magic.
    """
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/local/mono-green/cards/Primeval Titan")
        assert resp.status_code == 200, resp.json()
        assert resp.json()["entombed"] == "Primeval Titan"
        deck = client.get("/api/decks/local/mono-green").json()
        assert "Primeval Titan" not in {c["name"] for c in deck["cards"]}
        dead = deck["graveyard"]
        assert [c["name"] for c in dead] == ["Primeval Titan"]
        assert dead[0]["why"], "the rationale rides into the graveyard"


def test_a_bulk_entombment_is_one_write(swappable):
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/entomb",
                           json={"names": ["Sol Ring", "Primeval Titan"]})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["entombed"] == ["Sol Ring", "Primeval Titan"]
        deck = client.get("/api/decks/local/mono-green").json()
        assert {c["name"] for c in deck["graveyard"]} == \
            {"Sol Ring", "Primeval Titan"}


def test_a_bulk_entombment_is_all_or_nothing(swappable):
    """A sweep that silently skipped two of its ten cards would report a deck
    state nobody chose, so one bad name refuses the whole batch."""
    with swappable as client:
        resp = client.post("/api/decks/local/mono-green/entomb",
                           json={"names": ["Sol Ring", "Black Lotus"]})
        assert resp.status_code == 422
        assert "Black Lotus" in resp.json()["detail"]
        deck = client.get("/api/decks/local/mono-green").json()
        assert deck["graveyard"] == []
        assert "Sol Ring" in {c["name"] for c in deck["cards"]}

        assert client.post("/api/decks/local/mono-green/entomb",
                           json={"names": "Sol Ring"}).status_code == 422


def test_a_returned_card_keeps_the_words_it_left_with(swappable):
    """The undo. The `why` that comes back is the user's own text preserved
    through the graveyard -- nothing composed, which is what keeps rule 4 out
    of this path entirely."""
    with swappable as client:
        before = next(c["why"] for c in
                      client.get("/api/decks/local/mono-green").json()["cards"]
                      if c["name"] == "Sol Ring")
        client.request("DELETE", "/api/decks/local/mono-green/cards/Sol Ring")
        resp = client.post(
            "/api/decks/local/mono-green/graveyard/Sol Ring/return")
        assert resp.status_code == 200, resp.json()
        assert resp.json()["returned"] == "Sol Ring"
        deck = client.get("/api/decks/local/mono-green").json()
        card = next(c for c in deck["cards"] if c["name"] == "Sol Ring")
        assert card["why"] == before
        assert deck["graveyard"] == []


def test_exile_is_permanent_and_only_reaches_the_buried(swappable):
    with swappable as client:
        client.request("DELETE", "/api/decks/local/mono-green/cards/Sol Ring")
        resp = client.request(
            "DELETE", "/api/decks/local/mono-green/graveyard/Sol Ring")
        assert resp.status_code == 200, resp.json()
        assert resp.json()["exiled"] == "Sol Ring"
        deck = client.get("/api/decks/local/mono-green").json()
        assert deck["graveyard"] == []
        assert "Sol Ring" not in {c["name"] for c in deck["cards"]}
        # Exile cannot touch a living card: it only ever acts on the buried.
        assert client.request(
            "DELETE",
            "/api/decks/local/mono-green/graveyard/Forest").status_code == 422


def test_removing_a_card_that_is_not_there_is_refused(swappable):
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/local/mono-green/cards/Black Lotus")
        assert resp.status_code == 422
        assert "not in this deck" in resp.json()["detail"]


def test_a_rationale_can_be_written_through_the_api(swappable):
    """The gap `decks import` opened: a draft arrives owing 99 rationales, and
    until this endpoint the only way to write one was a text editor."""
    with swappable as client:
        resp = client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                            json={"field": "why",
                                  "value": "Two mana for one, and it always has been."})
        assert resp.status_code == 200, resp.json()
        card = next(c for c in client.get("/api/decks/local/mono-green").json()["cards"]
                    if c["name"] == "Sol Ring")
        assert card["why"] == "Two mana for one, and it always has been."


def test_a_rationale_cannot_be_blanked_on_a_curated_deck(swappable):
    with swappable as client:
        resp = client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                            json={"field": "why", "value": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_only_a_short_list_of_card_fields_is_settable(swappable):
    with swappable as client:
        for field in ("name", "scryfall_id", "tags"):
            resp = client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not settable" in resp.json()["detail"]


def test_a_category_and_a_quantity_can_be_patched(swappable):
    with swappable as client:
        assert client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                            json={"field": "category",
                                  "value": "utility"}).status_code == 200
        assert client.patch("/api/decks/local/mono-green/cards/Forest",
                            json={"field": "qty", "value": 26}).status_code == 200
        cards = client.get("/api/decks/local/mono-green").json()["cards"]
        assert next(c for c in cards if c["name"] == "Sol Ring")["category"] == "utility"
        assert next(c for c in cards if c["name"] == "Forest")["qty"] == 26


def test_a_note_can_be_set_and_read_back(swappable):
    with swappable as client:
        resp = client.put("/api/decks/local/mono-green/notes/mulligan",
                          json={"value": "Keep any two-lander with a one-mana dork."})
        assert resp.status_code == 200, resp.json()
        notes = client.get("/api/decks/local/mono-green").json()["notes"]
        assert notes["mulligan"] == "Keep any two-lander with a one-mana dork."


def test_an_empty_note_is_refused(swappable):
    with swappable as client:
        resp = client.put("/api/decks/local/mono-green/notes/mulligan",
                          json={"value": "   "})
        assert resp.status_code == 422
        assert "needs text" in resp.json()["detail"]


def test_a_draft_is_promoted_once_every_card_is_justified(in_memory_client):
    """The last step of an import, and until now the last thing in the whole
    lifecycle that could only be done in a text editor."""
    deck = tiny_pool.mono_green_deck(stage="draft")
    with in_memory_client([deck]) as client:
        resp = client.patch("/api/decks/local/mono-green",
                            json={"field": "stage", "value": "curated"})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["stage"] == "curated"
        assert client.get("/api/decks/local/mono-green").json()["stage"] == "curated"


def test_promotion_is_refused_while_a_card_is_blank(draft_client):
    with draft_client as client:
        client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        resp = client.patch("/api/decks/local/mono-green",
                            json={"field": "stage", "value": "curated"})
        assert resp.status_code == 422
        assert "Sol Ring" in resp.json()["detail"]
        # And it stayed a draft rather than landing somewhere in between.
        assert client.get("/api/decks/local/mono-green").json()["stage"] == "draft"


def test_deck_status_and_bracket_are_patchable(swappable):
    with swappable as client:
        assert client.patch("/api/decks/local/mono-green",
                            json={"field": "status",
                                  "value": "built"}).status_code == 200
        assert client.patch("/api/decks/local/mono-green",
                            json={"field": "bracket", "value": 5}).status_code == 200
        body = client.get("/api/decks/local/mono-green").json()
        assert (body["status"], body["bracket"]) == ("built", 5)


def test_a_pilot_is_patchable_and_travels_to_the_shelf(swappable):
    """The household tag (second 2026-08-15 punch list, item 10): set through
    the same field PATCH as stage and status, case kept, visible on both the
    deck and the shelf listing, and clearable."""
    with swappable as client:
        resp = client.patch("/api/decks/local/mono-green",
                            json={"field": "pilot", "value": "Mark's Wife"})
        assert resp.status_code == 200, resp.json()
        assert client.get(
            "/api/decks/local/mono-green").json()["pilot"] == "Mark's Wife"
        tile = next(d for d in client.get("/api/decks").json()
                    if d["slug"] == "mono-green")
        assert tile["pilot"] == "Mark's Wife"

        # Untagging is a real operation, not an error.
        assert client.patch("/api/decks/local/mono-green",
                            json={"field": "pilot",
                                  "value": ""}).status_code == 200
        assert client.get("/api/decks/local/mono-green").json()["pilot"] == ""


def test_a_field_that_is_not_the_decks_own_is_refused(swappable):
    with swappable as client:
        for field in ("name", "commander", "cards"):
            resp = client.patch("/api/decks/local/mono-green",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not a settable deck field" in resp.json()["detail"]


def test_a_refused_edit_changes_nothing(swappable):
    """The whole point of verifying before writing. Every refusal above must
    leave the deck byte-identical, not partly applied."""
    with swappable as client:
        before = client.get("/api/decks/local/mono-green").json()
        client.post("/api/decks/local/mono-green/cards",
                    json={"name": "Sol Ring", "category": "ramp", "why": "dup"})
        client.request("DELETE", "/api/decks/local/mono-green/cards/Black Lotus")
        client.patch("/api/decks/local/mono-green/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        client.patch("/api/decks/local/mono-green/cards/Forest",
                     json={"field": "qty", "value": 0})
        client.put("/api/decks/local/mono-green/notes/x", json={"value": ""})
        assert client.get("/api/decks/local/mono-green").json() == before


def test_a_read_only_source_refuses_every_edit():
    deck = tiny_pool.mono_green_deck()
    app = create_app()
    _ro = MemoryDeckSource([deck], writable=False)
    app.dependency_overrides[deck_source] = lambda: _ro
    app.dependency_overrides[library] = lambda: library_over(_ro)
    with TestClient(app) as client:
        base = "/api/decks/local/mono-green"
        responses = [
            client.post(f"{base}/cards", json={"name": "Forest",
                                               "category": "land", "why": "x"}),
            client.request("DELETE", f"{base}/cards/Sol Ring"),
            client.patch(f"{base}/cards/Sol Ring",
                         json={"field": "why", "value": "x"}),
            client.put(f"{base}/notes/mulligan", json={"value": "x"}),
            client.patch(base, json={"field": "status", "value": "built"}),
        ]
        for resp in responses:
            assert resp.status_code == 403
            assert "read-only" in resp.json()["detail"]


# ----------------------------------------------------------------- import
#
# The import endpoint is the only other write in the API, and it creates rather
# than edits. Its refusals run against an in-memory source for the same reason
# the swap refusals do: a test that wrote to decks/ would be editing the
# repository's own source of truth.

@pytest.fixture
def importable(in_memory_client, tmp_path):
    """An empty library, with the fixture pool behind it.

    Built rather than borrowed: the real 500MB pool is absent on a fresh
    clone and in CI, and the one thing worth proving about import is what it
    does *with* a card pool.
    """
    with config.use_paths(data_dir=tmp_path / "data"):
        tiny_pool.build(config.DB_PATH)
        yield in_memory_client([])


def test_import_creates_a_draft_and_gates_it_immediately(importable):
    """ADR 13's bargain, over HTTP: the facts are checked on day one, and the
    thinking still owed is a number rather than a wall."""
    with importable as client:
        resp = client.post("/api/decks/import", json={
            "slug": "gyome-x", "name": "Gyome imported",
            "text": tiny_pool.DECKLIST, "bracket": 4})
        assert resp.status_code == 200, resp.text
        body = resp.json()

        assert body["created"] is True
        assert body["stage"] == "draft"
        assert body["commander"] == ["Gyome, Master Chef"]
        assert body["total_cards"] == 99
        assert body["land_count"] == 97
        assert body["needs_rationale"] == 3
        assert [i["card"] for i in body["errors"]] == ["Primeval Titan"]

        # And it is a deck the rest of the API can see.
        deck = client.get("/api/decks/local/gyome-x").json()
        assert deck["stage"] == "draft"
        assert deck["needs_rationale"] == 3
        assert all(c["why"] == "" for c in deck["cards"]), "no rationale invented"
        assert {d["slug"] for d in client.get("/api/decks").json()} == {"gyome-x"}


def test_import_dry_run_previews_without_creating(importable):
    """The preview runs the identical code path, so what the user approves is
    the result rather than an estimate of it."""
    with importable as client:
        resp = client.post("/api/decks/import", json={
            "slug": "gyome-x", "text": tiny_pool.DECKLIST, "dry_run": True})
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["created"] is False
        assert body["total_cards"] == 99
        assert [i["card"] for i in body["errors"]] == ["Primeval Titan"]
        assert "stage: draft" in body["yaml"]
        assert client.get("/api/decks").json() == []
        assert client.get("/api/decks/local/gyome-x").status_code == 404


def test_import_reports_an_unresolved_name_rather_than_guessing(importable):
    with importable as client:
        body = client.post("/api/decks/import", json={
            "slug": "typo-x", "dry_run": True,
            "text": "Commander\n1 Gyome, Master Chef\nDeck\n1 Sol Rng\n97 Swamp\n",
        }).json()
        assert body["unknown"] == ["Sol Rng"]
        assert any(i["code"] == "unknown-card" for i in body["errors"])


@pytest.mark.parametrize(("payload", "expected"), [
    ({"slug": "../escape", "text": "1 Sol Ring\n"}, "not a usable slug"),
    ({"slug": "Has Caps", "text": "1 Sol Ring\n"}, "not a usable slug"),
    ({"slug": "fine", "text": "\n\n# only comments\n"}, "parsed as a card"),
    ({"slug": "fine", "text": "1 Sol Ring\n1 Swamp\n"}, "no commander"),
    ({"slug": "fine", "text": "1 Sol Ring\n", "status": "owned"}, "status 'owned'"),
])
def test_import_refusals(importable, payload, expected):
    with importable as client:
        resp = client.post("/api/decks/import", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]
        assert client.get("/api/decks").json() == []


def test_import_will_not_overwrite_an_existing_deck(in_memory_client, tmp_path):
    deck = tiny_pool.mono_green_deck()
    with config.use_paths(data_dir=tmp_path / "data"):
        tiny_pool.build(config.DB_PATH)
        with in_memory_client([deck]) as client:
            resp = client.post("/api/decks/import", json={
                "slug": "mono-green", "text": tiny_pool.DECKLIST})
            assert resp.status_code == 422
            assert "already exists" in resp.json()["detail"]
            # The deck it refused to touch is untouched.
            assert client.get("/api/decks/local/mono-green").json()["stage"] == "curated"


def test_import_without_a_pool_refuses_rather_than_guessing(in_memory_client,
                                                              tmp_path):
    """Every name would be unknown and no land filed, so the deck's facts would
    never be checked -- the one thing the gate exists to do."""
    with config.use_paths(data_dir=tmp_path / "absent"), \
            in_memory_client([]) as client:
        resp = client.post("/api/decks/import",
                           json={"slug": "x", "text": "1 Sol Ring\n"})
        assert resp.status_code == 422
        assert "pool" in resp.json()["detail"]


def test_a_read_only_library_refuses_import():
    deck = tiny_pool.mono_green_deck()
    app = create_app()
    _ro = MemoryDeckSource([deck], writable=False)
    app.dependency_overrides[deck_source] = lambda: _ro
    app.dependency_overrides[library] = lambda: library_over(_ro)
    with TestClient(app) as client:
        resp = client.post("/api/decks/import",
                           json={"slug": "new-deck", "text": "1 Sol Ring\n"})
        assert resp.status_code == 403
        assert "read-only" in resp.json()["detail"]


def test_the_import_path_does_not_shadow_a_deck_called_import(importable):
    """`/api/decks/import` is a POST and `/api/decks/local/{slug}` is a GET, so the
    two cannot collide -- pinned because the route order looks like it matters
    and one day somebody will move it."""
    with importable as client:
        assert client.get("/api/decks/import").status_code == 404


def test_an_empty_library_is_empty_rather_than_broken(in_memory_client):
    """Two lines here; creating and removing directories on disk otherwise."""
    with in_memory_client([]) as client:
        assert client.get("/api/decks").json() == []
        health = client.get("/api/health").json()
        if health["pool"]:
            assert health["decks"] == 0


# ------------------------------------------------------------------- cards

def test_card_search_respects_the_limit(pool, client):
    body = client.get("/api/cards/search", params={"limit": 5}).json()
    assert len(body["cards"]) <= 5


def test_card_search_identity_filter_is_a_subset_check(pool, client):
    """Asking for BG must return colorless and mono-B cards too -- that is the
    question a Golgari deckbuilder is actually asking."""
    body = client.get("/api/cards/search",
                      params={"identity": "BG", "limit": 50}).json()
    for card in body["cards"]:
        assert set(card["color_identity"]) <= {"B", "G"}, card["name"]


def test_search_limit_is_clamped_not_rejected(client):
    assert client.get("/api/cards/search", params={"limit": 9999}).status_code == 422


def test_identify_resolves_a_corner_outright(pool, client):
    """A set code and a collector number are a lookup, so they land."""
    body = client.post("/api/cards/identify", json={"sightings": [
        {"set": "LTC", "number": "284/281"},
    ]}).json()
    reading = body["readings"][0]
    assert reading["via"] == "printing"
    assert reading["resolved"]["name"] == "Sol Ring"
    assert reading["candidates"] == []
    assert (body["resolved"], body["offered"], body["unread"]) == (1, 0, 0)


def test_identify_only_ever_offers_a_title(pool, client):
    """The whole design in one assertion: a perfect title still resolves
    nothing, because the scores of right and wrong answers overlap. See
    `cards/identify.py` for the measurement."""
    body = client.post("/api/cards/identify", json={"sightings": [
        {"title": "Craterhoof Behemoth"},
    ]}).json()
    reading = body["readings"][0]
    assert reading["via"] == "title"
    assert reading["resolved"] is None
    assert reading["candidates"][0]["name"] == "Craterhoof Behemoth"
    assert reading["candidates"][0]["score"] == pytest.approx(1.0)
    assert (body["resolved"], body["offered"], body["unread"]) == (0, 1, 0)


def test_identify_counts_the_three_outcomes_apart(pool, client):
    """Resolved, offered and unread are never added together: two of them
    are still work somebody has to do."""
    body = client.post("/api/cards/identify", json={"sightings": [
        {"set": "ltc", "number": "284"},
        {"title": "Sol Rlng"},
        {},
    ]}).json()
    assert [r["via"] for r in body["readings"]] == ["printing", "title", "nothing"]
    assert (body["resolved"], body["offered"], body["unread"]) == (1, 1, 1)


def test_identify_renders_enough_to_show_the_card(pool, client):
    """A review list is pictures; a name alone cannot be checked by eye."""
    body = client.post("/api/cards/identify", json={"sightings": [
        {"set": "ltc", "number": "284"},
    ]}).json()
    card = body["readings"][0]["resolved"]
    assert set(card) == {"name", "mana_cost", "type_line", "color_identity",
                         "image", "art_crop"}


def test_identify_refuses_a_body_that_is_not_a_list(client):
    assert client.post("/api/cards/identify",
                       json={"sightings": "Sol Ring"}).status_code == 422
    assert client.post("/api/cards/identify", json={}).status_code == 422


def test_identify_of_nothing_is_not_an_error(pool, client):
    body = client.post("/api/cards/identify", json={"sightings": []}).json()
    assert body["readings"] == []
    assert body["resolved"] == 0


def test_identify_without_a_pool_says_so(client, tmp_path):
    """A fresh clone has no pool, and the camera must say that rather than
    500 -- the same rule every other card endpoint here follows.

    The empty `data_dir` is not decoration. The `client` fixture redirects
    only `decks_dir`, so `config.DB_PATH` still points at the repository's
    real `data/mtg.duckdb` -- which exists on the maintainer's laptop and
    never in CI. Written the obvious way, this test asserted the no-pool
    behaviour in CI and asserted the *opposite* here, where it found
    `Solarion` in the 35,000-card pool. A test whose subject is the absence
    of a file has to arrange that absence itself.
    """
    with config.use_paths(data_dir=tmp_path / "no-pool-here"):
        body = client.post("/api/cards/identify", json={"sightings": [
            {"title": "Sol Ring"},
        ]}).json()
    assert body["readings"] == []
    assert "data refresh" in body["message"]

def test_the_reader_shelf_refuses_a_name_it_does_not_hold(client):
    """`ocr.ASSETS` is the guard: there is no pattern to get wrong."""
    assert client.get("/api/ocr/worker.js").status_code == 404
    assert client.get("/api/ocr/eng.traineddata").status_code == 404


def test_the_reader_shelf_serves_a_cached_file(client, tmp_path, monkeypatch):
    """Served first-party and immutable, because the cache path carries the
    pinned versions -- these bytes can never come to mean anything else."""
    from mtglab import ocr

    monkeypatch.setattr(ocr, "_download", lambda url: b"console.log(1)")
    monkeypatch.setitem(
        ocr.ASSETS, "worker.min.js",
        ocr.Asset(url="https://example.invalid/w.js",
                  digest=hashlib.sha256(b"console.log(1)").hexdigest(),
                  size=14, media_type="text/javascript"))
    with config.use_paths(data_dir=tmp_path / "shelf"):
        response = client.get("/api/ocr/worker.min.js")
    assert response.status_code == 200
    assert response.headers["cache-control"] == (
        "public, max-age=31536000, immutable")


def test_the_worker_licence_notice_is_reachable(client, tmp_path, monkeypatch):
    """The worker is served byte-identical, so it still says `For license
    information please see worker.min.js.LICENSE.txt` -- and that pointer is
    relative to *our* origin. It has to answer here or every recipient of the
    worker is pointed at a 404 (NOTICE.md, "Tesseract").

    The name is taken off the shelf rather than written out, so deleting the
    row fails this test rather than leaving it green against a table it
    supplied itself.
    """
    from mtglab import ocr

    name, = [n for n in ocr.ASSETS if n.endswith(".LICENSE.txt")]
    notice = b"/*! ieee754. BSD-3-Clause License. */"
    monkeypatch.setattr(ocr, "_download", lambda url: notice)
    monkeypatch.setitem(
        ocr.ASSETS, name,
        ocr.Asset(url="https://example.invalid/w.js.LICENSE.txt",
                  digest=hashlib.sha256(notice).hexdigest(),
                  size=len(notice), media_type="text/plain"))
    with config.use_paths(data_dir=tmp_path / "shelf"):
        response = client.get(f"/api/ocr/{name}")
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/plain")
    assert b"BSD-3-Clause" in response.content

# --------------------------------------------------------------------- sim

def test_sim_requires_a_slug(client):
    assert client.post("/api/sim/mana", json={}).status_code == 422


def test_sim_job_runs_to_completion(sim_client):
    """Submit, poll, and get a result -- the whole contract the UI relies on."""

    submitted = sim_client.post("/api/sim/mana",
                            json={"slug": "mono-green", "games": 300,
                                  "turns": 8, "seed": 1}).json()
    assert submitted["status"] in ("queued", "running", "done")

    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    result = body["result"]
    assert result["games"] == 300
    assert len(result["by_turn"]) == 8
    assert result["caveat"], "Tier 1 numbers must ship with their caveat"


def test_failed_job_reports_the_error_rather_than_hanging(client):
    """A job for a deck that cannot compile must end in `error`, not spin."""
    from mtglab.api import simruns

    job = jobs.submit("test", lambda _p: simruns._compile("no-such-deck"))
    current = await_job(client, job.id)
    assert current["status"] == "error"
    assert current["error"]


def test_land_sweep_returns_a_row_per_count_and_reports_the_spread(sim_client):
    """The sweep is the command that actually decides a land count, and it was
    entirely untested. The spread matters as much as the winner: Gyome's curve
    was flat to 0.07 across 30-40, and reading the argmax of that is reading
    noise."""

    submitted = sim_client.post("/api/sim/lands",
                            json={"slug": "mono-green", "low": 32, "high": 34,
                                  "games": 200, "seed": 1}).json()
    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    result = body["result"]

    assert [r["lands"] for r in result["rows"]] == [32, 33, 34]
    for row in result["rows"]:
        assert 0.0 <= row["commander_by_t5"] <= 1.0
        assert 0.0 <= row["mulligan_rate"] <= 1.0
        assert row["spells_through_t8"] >= 0
    # A flat curve must be reported as flat rather than leaving the caller to
    # read the argmax of noise.
    assert result["deployment_spread"] >= 0
    assert isinstance(result["flat"], bool)
    assert result["flat"] == (result["deployment_spread"] < 0.25)
    assert result["argmax_lands"] in [r["lands"] for r in result["rows"]]
    assert result["caveat"], "Tier 1 numbers must ship with their caveat"


# ------------------------------------------------- Tier 1.5, over the wire
#
# The HTTP surface, tested as a surface. CLAUDE.md's dossier lesson is exactly
# this: forty-two tests exercised that module, none asked what the route did,
# and it shipped broken. The shelf is the more dangerous of the two here
# because it is the only simulation route that answers *in the request* -- so
# the thing a client depends on is a 200 with a body, not a job id.


def test_a_simulation_carries_the_gate_verdict(sim_client):
    """Every reported number must be able to say whether its deck is legal.

    The simulator deliberately does not refuse an invalid deck -- see
    `_compile_checked` for why -- so this field is the whole of what keeps
    that honest. A result without it is a number about a possibly-illegal deck
    with no way to know.
    """
    for path, payload in (("/api/sim/shelf", {"slug": "mono-green"}),):
        body = sim_client.post(path, json=payload).json()
        check = body["deck_check"]
        assert set(check) >= {"ok", "error_count", "affects_numbers",
                              "unresolved", "declared_size", "simulated_size"}
        assert check["declared_size"] == check["simulated_size"], (
            "the fixture deck resolves entirely, so nothing should be dropped")


def test_the_mana_job_carries_the_gate_verdict_too(sim_client):
    submitted = sim_client.post("/api/sim/mana",
                                json={"slug": "mono-green", "games": 200}).json()
    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body
    assert "deck_check" in body["result"]


def test_an_invalid_deck_is_simulated_rather_than_refused(sim_client, monkeypatch):
    """The design decision, pinned as a decision.

    Two of the library's decks fail the gate on a banned card by choice, and a
    deck mid-import fails it by construction. Refusing to simulate those would
    take the diagnosis away at exactly the moment somebody wants it, so an
    invalid deck answers with numbers **and** a verdict rather than with an
    error. If this ever starts refusing, that was a decision to make
    deliberately rather than to arrive at.
    """
    from mtglab.api import simruns
    from mtglab.decks.validate import Issue

    real = simruns.validate

    def failing(deck, cards=None, **kw):
        rep = real(deck, cards, **kw)
        rep.issues.append(
            Issue("error", "banned", "not legal in Commander", "Black Lotus"))
        return rep

    monkeypatch.setattr(simruns, "validate", failing)
    body = sim_client.post("/api/sim/shelf", json={"slug": "mono-green"})
    assert body.status_code == 200, "an illegal deck must still be measurable"
    check = body.json()["deck_check"]
    assert check["ok"] is False
    assert check["affects_numbers"] is True, (
        "a banned card sits in the 99 being shuffled, so it moves the numbers")
    assert any(e["card"] == "Black Lotus" for e in check["errors"])


def test_a_paperwork_failure_is_marked_as_not_moving_the_numbers(sim_client,
                                                                 monkeypatch):
    """A missing rationale blocks a curated deck and changes nothing about mana.

    The client says something different for this than for a banned card, so
    the split has to be real rather than "any error is scary".
    """
    from mtglab.api import simruns
    from mtglab.decks.validate import Issue

    real = simruns.validate

    def failing(deck, cards=None, **kw):
        rep = real(deck, cards, **kw)
        rep.issues.append(
            Issue("error", "missing-rationale", "no why", "Forest"))
        return rep

    monkeypatch.setattr(simruns, "validate", failing)
    check = sim_client.post("/api/sim/shelf",
                            json={"slug": "mono-green"}).json()["deck_check"]
    assert check["ok"] is False
    assert check["affects_numbers"] is False


def test_a_deck_with_nothing_in_it_is_refused_rather_than_answered(sim_client):
    """The one state that IS blocked, and the reason the rest are not.

    An empty deck used to simulate perfectly happily: a 100% mulligan rate,
    zero spells through turn 8, and a shelf demanding coloured sources against
    a library of nought. Every figure arithmetically correct, none of them
    about anything. `adrix-and-nev-twincasters` was in that state on the
    deployed instance.
    """
    from mtglab.decks.model import Deck
    from mtglab.sim.compile import NothingToSimulate, compile_report

    # A pool that is present but does not hold this deck's cards, which is
    # the shape a half-finished import actually has.
    empty = Deck(slug="hollow", name="Hollow", commander=["Cmd"], cards=[])
    try:
        compile_report(empty, {"Some Other Card": None})
    except NothingToSimulate as exc:
        assert "nothing to simulate" in str(exc)
        assert "hollow" in str(exc)
        return
    raise AssertionError("an empty deck must be refused, not answered")


def test_the_shelf_answers_in_the_request_rather_than_handing_back_a_job(sim_client):
    """The one simulation route that is not a job, asserted as not a job.

    A regression that made this a job would still 200 and would still "work"
    to a careless test; the client would then poll a `id` that is really a
    payload and render nothing.
    """
    body = sim_client.post("/api/sim/shelf", json={"slug": "mono-green"})
    assert body.status_code == 200, body.text
    payload = body.json()
    assert "status" not in payload and "id" not in payload, (
        "the shelf must answer with its result, not with a job")
    assert payload["deck_size"] > 0
    assert payload["caveat"]


def test_the_shelf_requires_a_slug(sim_client):
    assert sim_client.post("/api/sim/shelf", json={}).status_code == 422


@pytest.mark.parametrize(("sent", "want"), [
    (None, 422), (0, 422), (False, 422), ("", 422),      # nothing was named
    (123, 404), ([1], 404), ({"a": 1}, 404),             # something, and no deck
])
def test_a_slug_that_is_not_a_string_is_refused_and_not_a_crash(
        sim_client, sent, want):
    """422 or 404, never a 500. Found by sweeping bug 2's shape (2026-08-22).

    `payload` is a bare JSON object, so `payload.get("slug")` is whatever
    arrived; a list is *truthy*, so it walked past the `if not slug` guard and
    reached the deck source, where SQLite refused to bind it and the route
    answered a server error for a merely malformed request. The four
    `/api/sim/*` routes shared the block; `sim_forge` beside them had coerced
    its two slugs since the day it was written, which is the same
    already-defended-sibling shape as the invite route's `email`.

    Only the shelf can show it as a status code -- it is the one plain route
    here, so its exception is the response. On the three job routes the same
    input produced a job in state `error`, which is quieter and no better.
    """
    assert sim_client.post("/api/sim/shelf",
                           json={"slug": sent}).status_code == want


def test_an_unknown_deck_is_a_404_from_the_shelf(sim_client):
    assert sim_client.post("/api/sim/shelf",
                           json={"slug": "no-such-deck"}).status_code == 404


def test_the_shelf_reports_a_colour_ladder_and_a_land_estimate(sim_client):
    payload = sim_client.post("/api/sim/shelf",
                              json={"slug": "mono-green"}).json()
    green = next(c for c in payload["colors"] if c["color"] == "G")
    assert green["tiers"], "a colour with no rungs is a colour nobody asked for"
    for tier in green["tiers"]:
        assert tier["need"] >= tier["pips"]
        assert 0.0 <= tier["odds_now"] <= 1.0
        # The card list is capped for the wire but the count is the truth, so
        # a client can say "and 27 more" without being handed 27 names.
        assert len(tier["cards"]) <= 6
        assert tier["card_count"] >= len(tier["cards"])
    assert payload["lands_estimate"]["caveats"], (
        "the estimate must never ship without saying what it could not see")


def test_the_shelf_rows_carry_a_full_turn_series(sim_client):
    payload = sim_client.post("/api/sim/shelf",
                              json={"slug": "mono-green"}).json()
    horizon = payload["horizon"]
    assert payload["cards"], "a deck with spells must produce rows"
    for row in payload["cards"]:
        assert len(row["by_turn"]) == horizon
        assert all(0.0 <= x <= 1.0 for x in row["by_turn"])
        # Null rather than zero past the horizon: "not asked" is not "no".
        if row["mv"] > horizon:
            assert row["on_curve"] is None


def test_the_shelf_clamps_the_consistency_target(sim_client):
    """An absurd target is clamped, not rejected -- `_mana_params`' rule."""
    for target, expected in ((9.0, 0.99), (0.0, 0.5)):
        payload = sim_client.post(
            "/api/sim/shelf",
            json={"slug": "mono-green", "target": target}).json()
        assert payload["target"] == expected


def test_a_higher_target_never_asks_for_fewer_sources(sim_client):
    """Raising the bar cannot lower a requirement. A sign error here would
    read as a deck improving because somebody got stricter."""
    def need(target):
        payload = sim_client.post(
            "/api/sim/shelf",
            json={"slug": "mono-green", "target": target}).json()
        green = next(c for c in payload["colors"] if c["color"] == "G")
        return green["tiers"][0]["need"]
    assert need(0.95) >= need(0.75)


def test_the_policy_search_is_a_job_and_finishes(sim_client):
    submitted = sim_client.post(
        "/api/sim/policy",
        json={"slug": "mono-green", "games": 200}).json()
    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body
    result = body["result"]
    assert result["rows"], "a search with no rows has recommended nothing"
    assert result["best"]["spells_through_t8"] == max(
        r["spells_through_t8"] for r in result["rows"])
    # The verdict rides on the payload so the client never recomputes it.
    assert isinstance(result["flat"], bool)
    assert result["gain"] == round(
        result["best"]["spells_through_t8"]
        - result["baseline"]["spells_through_t8"], 2)


def test_the_policy_search_requires_a_slug(sim_client):
    assert sim_client.post("/api/sim/policy", json={}).status_code == 422


def test_a_policy_search_for_an_unknown_deck_fails_as_a_job(sim_client):
    """`plan_policy` defers a planning failure into the job, like `plan_mana`.

    A 200 then a job in state `error` is the shape the UI knows; failing at
    the route instead would be an optimisation that broke something.
    """
    submitted = sim_client.post(
        "/api/sim/policy", json={"slug": "no-such-deck", "games": 200})
    assert submitted.status_code == 200
    body = await_job(sim_client, submitted.json()["id"])
    assert body["status"] == "error"


def test_land_sweep_normalises_a_reversed_range(sim_client):
    """low/high the wrong way round should sweep, not return nothing."""

    submitted = sim_client.post("/api/sim/lands",
                            json={"slug": "mono-green", "low": 34, "high": 32,
                                  "games": 200, "seed": 1}).json()
    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert [r["lands"] for r in body["result"]["rows"]] == [32, 33, 34]


def test_sim_payload_bounds_are_clamped_not_rejected(sim_client):
    """An absurd game count should be pulled into range rather than 500."""

    submitted = sim_client.post("/api/sim/mana",
                            json={"slug": "mono-green", "games": 1,
                                  "turns": 99, "seed": 1}).json()
    body = await_job(sim_client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert body["result"]["games"] >= 100, "games floor"
    assert len(body["result"]["by_turn"]) <= 20, "turns ceiling"


def test_keep_rule_uses_documented_defaults():
    from mtglab.api.simruns import _keep_rule
    rule = _keep_rule({})
    assert (rule.min_lands, rule.max_lands) == (2, 5)


# ------------------------------------------------------------- sim caching
#
# The unit-level guarantees -- what reaches the key, that it survives a
# restart, that a broken store is a miss -- are in `test_sim_cache.py`. These
# are the ones that need the whole stack: a request, a deck source, a card pool
# and the job registry, which is where a cache turns into a wrong answer if it
# is going to.

def test_a_repeated_simulation_is_served_without_running_again(sim_client):
    """The claim the whole feature rests on: the second identical request
    costs no CPU, and says that it did not."""
    from mtglab.sim.tier1 import engine

    body = {"slug": "mono-green", "games": 300, "turns": 8, "seed": 5}
    first = await_job(sim_client, sim_client.post("/api/sim/mana", json=body).json()["id"])
    assert first["status"] == "done", first.get("error")
    assert first["result"]["cached"] is False
    assert first["result"]["computed_at"] is None

    calls = []
    original = engine.run

    def counted(*args, **kwargs):
        calls.append(1)
        return original(*args, **kwargs)

    # Patched on the module the runner imported from, so a hit is provable by
    # the simulator never being entered rather than by a stopwatch.
    engine.run = counted
    try:
        second = sim_client.post("/api/sim/mana", json=body).json()
    finally:
        engine.run = original

    assert second["status"] == "done", "a hit is answered in the request"
    assert second["percent"] == 100
    assert calls == [], "the simulator ran again on a cache hit"
    assert second["result"]["cached"] is True
    assert second["result"]["computed_at"], "a cached figure must date itself"
    # The numbers themselves, not just the envelope.
    assert second["result"]["by_turn"] == first["result"]["by_turn"]
    assert second["result"]["mulligan_rate"] == first["result"]["mulligan_rate"]
    assert second["result"]["caveat"], "a cached result still carries its caveat"


def test_a_cache_hit_is_still_the_callers_job_and_nobody_elses(sim_client):
    """A job born done is a job: it lists, it polls, and ADR 5's scoping is
    unchanged by it having taken no time."""
    body = {"slug": "mono-green", "games": 300, "turns": 8, "seed": 6}
    await_job(sim_client, sim_client.post("/api/sim/mana", json=body).json()["id"])
    hit = sim_client.post("/api/sim/mana", json=body).json()

    polled = sim_client.get(f"/api/jobs/{hit['id']}")
    assert polled.status_code == 200, "a finished job must still be pollable"
    assert polled.json()["result"]["cached"] is True
    assert hit["id"] in [j["id"] for j in sim_client.get("/api/jobs").json()]


def test_editing_a_card_invalidates_but_editing_a_rationale_does_not(sim_client):
    """The reason the key is the compiled deck rather than the deck file.

    A `why` cannot reach a simulation, so rewriting one must not throw the
    numbers away. Swapping a card obviously must.
    """
    body = {"slug": "mono-green", "games": 300, "turns": 8, "seed": 8}
    first = await_job(sim_client, sim_client.post("/api/sim/mana", json=body).json()["id"])
    assert first["result"]["cached"] is False

    edited = sim_client.patch("/api/decks/local/mono-green/cards/Sol Ring", json={
        "field": "why",
        "value": "Two mana on turn one is the fastest thing this deck can do, "
                 "and every payoff here is expensive enough to want it.",
    })
    assert edited.status_code == 200, edited.text
    assert sim_client.post("/api/sim/mana", json=body).json()["result"]["cached"] \
        is True, "a rationale edit must not invalidate a simulation"

    swapped = sim_client.post("/api/decks/local/mono-green/swap", json={
        "out": "Cultivator Colossus", "into": "Terastodon",
        "why": "Blows up three permanents on the way in, which this list has "
               "no other answer for.",
    })
    assert swapped.status_code == 200, swapped.text
    submitted = sim_client.post("/api/sim/mana", json=body).json()
    assert submitted["status"] != "done", \
        "changing the 99 must invalidate the deck's cached numbers"
    after = await_job(sim_client, submitted["id"])
    assert after["result"]["cached"] is False
    assert after["result"]["by_turn"] != first["result"]["by_turn"], \
        "a different 99 should produce different numbers, not a relabelled hit"


def test_a_land_sweep_reuses_the_counts_it_has_already_run(sim_client):
    """Per-count caching, and the invariant that makes it sound: a reused row
    is byte-identical to the one a fresh run produces."""
    base = {"slug": "mono-green", "games": 200, "seed": 3}
    first = await_job(sim_client, sim_client.post(
        "/api/sim/lands", json={**base, "low": 32, "high": 34}).json()["id"])
    assert first["status"] == "done", first.get("error")
    assert first["result"]["cached"] is False
    rows = {r["lands"]: r for r in first["result"]["rows"]}

    # Overlapping range: 33 and 34 are known, 35 is not, so this is still a job.
    partial = sim_client.post("/api/sim/lands",
                              json={**base, "low": 33, "high": 35}).json()
    body = await_job(sim_client, partial["id"])
    assert body["status"] == "done", body.get("error")
    assert body["result"]["cached"] is False, "one count still had to be run"
    reused = {r["lands"]: r for r in body["result"]["rows"]}
    assert [r["lands"] for r in body["result"]["rows"]] == [33, 34, 35]
    for count in (33, 34):
        assert reused[count] == rows[count], \
            "a reused row must equal what a fresh run produced"

    # Now every count is known, so the whole sweep is answered in the request.
    whole = sim_client.post("/api/sim/lands",
                            json={**base, "low": 33, "high": 35}).json()
    assert whole["status"] == "done"
    assert whole["result"]["cached"] is True
    assert whole["result"]["computed_at"]
    assert whole["result"]["rows"] == body["result"]["rows"]
    assert whole["result"]["argmax_lands"] == body["result"]["argmax_lands"]


def test_an_unseeded_run_is_reproducible_rather_than_random(sim_client):
    """An absent seed used to mean a fresh sample every time, which the app
    never asked for and could not cache. It resolves to a default now, and the
    result says which sample it is."""
    from mtglab.api.simruns import DEFAULT_SEED

    body = {"slug": "mono-green", "games": 300, "turns": 8}
    first = await_job(sim_client, sim_client.post("/api/sim/mana", json=body).json()["id"])
    assert first["result"]["seed"] == DEFAULT_SEED
    assert sim_client.post("/api/sim/mana", json=body).json()["result"]["cached"] \
        is True


def test_clamping_happens_before_the_key(sim_client):
    """Two requests that run the identical simulation must share an entry.
    Keying on the raw payload would store 200,000 games twice."""
    absurd = {"slug": "mono-green", "games": 10 ** 9, "turns": 4, "seed": 9}
    exact = {**absurd, "games": 200_000}
    from mtglab.api import simruns
    assert simruns._mana_params(absurd) == simruns._mana_params(exact)


@pytest.mark.parametrize("route", ["/api/sim/mana", "/api/sim/lands"])
def test_a_deck_that_cannot_compile_still_fails_through_the_job(sim_client, route):
    """Planning moved compilation into the request. A missing deck must still
    be reported the way it was before -- as a job error, not an exception out
    of the route.

    Both routes, and the assertion is on the *message* rather than on the
    status. An earlier draft handled a planning failure by re-running the
    failing compile inside the job, which made `plan_lands` call `run_lands`
    and `run_lands` call `plan_lands`: mutual recursion on exactly this input.
    It still ended in `status == "error"`, so only the text gave it away --
    a RecursionError where the caller needed to read "no such deck".
    """
    submitted = sim_client.post(route, json={"slug": "no-such-deck",
                                             "games": 300, "low": 32, "high": 33})
    assert submitted.status_code == 200
    body = await_job(sim_client, submitted.json()["id"])
    assert body["status"] == "error"
    assert "no-such-deck" in body["error"], \
        f"the error should name the deck, not the failure to look for it: {body['error']}"


def test_a_land_sweep_with_no_lands_reports_that_and_nothing_else(
        in_memory_client, pool):
    """The other `plan_lands` failure, and the one `_resize` raises rather than
    `_compile`: it has to survive the same fallback."""
    from mtglab.decks.model import CardEntry

    landless = tiny_pool.mono_green_deck(clean=True)
    landless.cards = [CardEntry(name="Sol Ring", category="ramp", qty=99,
                                why="A fixture with no lands at all.")]
    jobs.clear()
    with in_memory_client([landless]) as client:
        submitted = client.post("/api/sim/lands", json={
            "slug": "mono-green", "low": 32, "high": 33, "games": 200})
        body = await_job(client, submitted.json()["id"])
        assert body["status"] == "error"
        assert "no lands" in body["error"]


def test_unknown_job_is_404(client):
    assert client.get("/api/jobs/deadbeef").status_code == 404


def test_job_progress_is_reported(client):
    job = jobs.submit("test", lambda p: [p(i, 10) for i in range(10)] and "ok")
    body = await_job(client, job.id)
    assert body["status"] == "done"
    assert body["percent"] == 100
    assert client.get("/api/jobs").json()[0]["id"] == job.id


# ---------------------------------------------------------------- colours

def test_colors_needs_no_pool_and_no_decks(client):
    """The one deck-facing page that works on a fresh clone.

    Nothing here touches DuckDB, the deck source or the network, which is what
    makes the create flow's first screen safe to show before `data refresh`
    has ever run.
    """
    body = client.get("/api/colors").json()
    assert len(body["combinations"]) == 32
    assert [c["code"] for c in body["colors"]] == list("WUBRG")
    assert {e["name"] for e in body["eras"]} == {"Ravnica", "Alara", "Tarkir"}
    selesnya = next(c for c in body["combinations"] if c["key"] == "WG")
    assert selesnya["name"] == "Selesnya"
    assert selesnya["tier"] == "guild"


def test_four_colour_slots_expose_both_naming_conventions(client):
    """Scryfall's name is primary, EDHREC's Nephilim is the alias -- someone
    arriving from EDHREC has to find the slot they came for."""
    body = client.get("/api/colors").json()
    quad = next(c for c in body["combinations"] if c["key"] == "WUBR")
    assert quad["name"] == "Artifice"
    assert quad["aliases"] == ["Yore-Tiller"]


def test_the_taxonomy_carries_the_teaching_depth_without_a_pool(client):
    """Lore, champion names and signature names ride on the table itself.

    Names only -- the cards come from `/api/colors/{key}`, which does need a
    pool. Splitting it that way is what lets the create flow's first screen
    keep working on a fresh clone while still saying who Trostani is.
    """
    body = client.get("/api/colors").json()
    selesnya = next(c for c in body["combinations"] if c["key"] == "WG")
    assert len(selesnya["lore"].split()) >= 40
    assert "Trostani, Selesnya's Voice" in [
        ch["card"] for ch in selesnya["champions"]]
    assert selesnya["signature"]
    # Mono-Green is not a faction and does not pretend to be one.
    green = next(c for c in body["combinations"] if c["key"] == "G")
    assert green["lore"] == "" and green["champions"] == []
    assert green["signature"]


def test_combination_detail_reports_no_pool_rather_than_failing(tmp_path):
    """A fresh clone gets the prose and an honest empty card list.

    Pointed at an empty data directory rather than run on the bare `client`
    fixture, which would find the maintainer's own 500MB pool and pass here
    while asserting the opposite of what CI sees.
    """
    with config.use_paths(data_dir=tmp_path / "empty"), \
            TestClient(create_app()) as c:
        body = c.get("/api/colors/WG").json()
    assert body["name"] == "Selesnya"
    assert len(body["lore"].split()) >= 40
    assert body["pool"] is False
    assert body["champions"] == [] and body["signature"] == []
    assert body["exact_total"] is None


def test_combination_detail_canonicalises_the_key_and_404s_on_nonsense(client):
    assert client.get("/api/colors/gw").json()["name"] == "Selesnya"
    assert client.get("/api/colors/C").json()["name"] == "Colourless"
    assert client.get("/api/colors/ZZ").status_code == 404


def test_combination_detail_resolves_cards_and_drops_what_is_missing(
        pool, client, monkeypatch):
    """The ADR 19 instrument, pointed at reference data.

    A misspelled name here would otherwise render as a confident empty card,
    so an unresolved one is dropped and counted. The lists are monkeypatched
    rather than borrowed from the real table because the tiny pool holds 21
    cards and none of them is a Selesnya staple -- what is under test is the
    resolution, and the real names are checked against the real pool in
    `test_colors.py`.
    """
    from mtglab import colors
    patched = dict(colors.BY_KEY)
    patched["G"] = replace(
        patched["G"],
        signature=("Goreclaw, Terror of Qal Sisma", "No Such Card"),
        champions=(colors.Champion("Regal Behemoth", "A fixture, not a face."),
                   colors.Champion("Also Not A Card", "Dropped on the way.")))
    monkeypatch.setattr(colors, "BY_KEY", patched)

    body = client.get("/api/colors/G").json()
    assert body["pool"] is True
    assert [c["name"] for c in body["signature"]] == [
        "Goreclaw, Terror of Qal Sisma"]
    assert [c["name"] for c in body["champions"]] == ["Regal Behemoth"]
    assert body["champions"][0]["role"] == "A fixture, not a face."
    # Two names in, two dropped, and the count is reported rather than implied.
    assert body["dropped"] == 2
    # Counted over the pool rather than stored. The tiny fixture is mostly
    # mono-green, so this is a real number and not a placeholder.
    assert body["exact_total"] > 0


def test_glossary_needs_no_pool_and_no_decks(client):
    """Reference prose, like the taxonomy. Same fresh-clone property."""
    body = client.get("/api/glossary").json()
    keys = {t["key"] for t in body["terms"]}
    assert {"commander", "color-identity", "mulligan"} <= keys
    # The simulator's own controls are in the same table as the Magic words,
    # which is what lets one component serve both.
    assert {"sim.min_pieces", "stat.spells_through_t8"} <= keys
    assert [s["key"] for s in body["sections"]] == [
        "format", "building", "simulator"]
    for term in body["terms"]:
        assert term["short"] and term["long"]
        assert all(ref in keys for ref in term["see_also"])


def test_lore_serves_the_shelves_with_pool_cards(client, pool):
    """Reference prose plus pool cards, `/api/colors/{key}`-style.

    The `pool` fixture is deliberate and is the lesson of this test's first
    version, which took the bare `client` and asserted `pool is True`: that
    passed on the laptop, where `data/mtg.duckdb` exists at the default path,
    and failed in CI, where nothing does -- the exact environment-dependent
    shape `tiny_pool` was built to kill. With the fixture the world is
    pinned: a pool exists, it holds none of the shelves' historical cards,
    so every named card drops and is counted rather than invented -- the
    instrument working, not a gap.
    """
    body = client.get("/api/lore").json()
    assert [v["key"] for v in body["volumes"]] == [
        "history", "mechanics", "artists", "table", "curiosities"]
    assert body["facts"], "the shelves are empty"
    for fact in body["facts"]:
        assert fact["fact"] and fact["more"]
    assert body["pool"] is True
    named = sum(len(f["cards"]) for f in body["facts"])
    from mtglab import lore as lore_mod
    total = sum(len(f.cards) for f in lore_mod.FACTS)
    assert named + body["dropped"] == total


def test_lore_answers_whole_with_no_pool_at_all(client, tmp_path):
    """The fresh-clone property: no pool, and the shelves still stock every
    fact -- prose complete, cards absent, and `dropped` at zero, because an
    unresolvable name is only a *fault* when there was a pool to ask."""
    with config.use_paths(data_dir=tmp_path / "absent"):
        body = client.get("/api/lore").json()
    assert body["pool"] is False
    assert body["dropped"] == 0
    assert body["facts"]
    for fact in body["facts"]:
        assert fact["fact"] and fact["more"]
        assert fact["cards"] == []


def test_challenge_progress_counts_filled_slots(in_memory_client):
    with in_memory_client([tiny_pool.mono_green_deck()]) as c:
        body = c.get("/api/colors/progress").json()
    assert body["total"] == 32
    assert len(body["slots"]) == 32
    # Goreclaw is mono-green. Without a card pool the identity cannot be
    # derived at all, so the assertion is conditional -- the same tolerance
    # the rest of this file has for a missing pool.
    green = next(s for s in body["slots"] if s["key"] == "G")
    if body["filled"]:
        assert [d["slug"] for d in green["decks"]] == ["mono-green"]


# ----------------------------------------------------------------- create

def test_create_makes_a_draft_from_a_commander(pool, in_memory_client):
    with in_memory_client([]) as c:
        body = c.post("/api/decks", json={
            "slug": "brand-new", "commander": ["Gyome, Master Chef"]}).json()
    assert body["created"] is True
    # A new deck owes its rationales, so it starts as a draft -- ADR 13.
    assert body["stage"] == "draft"
    assert body["total_cards"] == 0
    assert body["color_identity"] == ["B", "G"]
    assert body["combination"]["name"] == "Golgari"


def test_create_refuses_a_card_that_cannot_lead_a_deck(pool, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": ["Sol Ring"]})
    assert r.status_code == 422
    assert "cannot be your commander" in r.json()["detail"]


def test_create_refuses_a_deck_with_no_commander(in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": []})
    assert r.status_code == 422
    assert "needs a commander" in r.json()["detail"]


def test_create_refuses_a_duplicate_slug(pool, in_memory_client):
    existing = Deck(slug="gyome-food", name="Gyome Food",
                    commander=["Gyome, Master Chef"], cards=[])
    with in_memory_client([existing]) as c:
        r = c.post("/api/decks", json={
            "slug": "gyome-food", "commander": ["Gyome, Master Chef"]})
    assert r.status_code == 422
    assert "already exists" in r.json()["detail"]


def test_create_is_refused_on_a_read_only_library():
    deck = tiny_pool.mono_green_deck()
    app = create_app()
    _ro = MemoryDeckSource([deck], writable=False)
    app.dependency_overrides[deck_source] = lambda: _ro
    app.dependency_overrides[library] = lambda: library_over(_ro)
    with TestClient(app) as client:
        r = client.post("/api/decks", json={
            "slug": "nope", "commander": ["Gyome, Master Chef"]})
    assert r.status_code == 403


@pytest.mark.needs_full_pool
def test_commander_search_is_exact_and_returns_actual_commanders(client):
    """The bug this pins: `commanders_only` used to filter after the SQL
    limit, so a search for Selesnya commanders returned the sixty best
    Selesnya cards, none of which was a commander, and then nothing.

    The one test here that `tiny_pool` genuinely cannot carry, and it is
    marked rather than quietly skipped so the gap is countable. Reproducing
    the bug needs *more* Selesnya cards than the limit, with the commander
    ranked below the cut -- against 21 cards the filter and the limit
    cannot disagree, so a fixture version would pass whether or not the bug
    was back. Eleven more fixture rows to keep one assertion honest is a
    worse trade than saying plainly that this one needs the real pool.
    """
    if not config.DB_PATH.exists():
        pytest.skip("needs the full pool -- run `mtglab data refresh`")
    cards = client.get("/api/cards/search", params={
        "identity": "WG", "identity_exact": True,
        "commanders_only": True, "limit": 10}).json()["cards"]
    assert cards, "a search for Selesnya commanders must return some"
    for card in cards:
        assert set(card["color_identity"]) == {"W", "G"}, card["name"]
        assert "Legendary" in card["type_line"] or \
               "can be your commander" in (card["oracle_text"] or "").lower()


# ---------------------------------------------------------------- deleting
#
# Against an in-memory source, so no test here can delete a real deck out of
# `decks/` -- which is the failure mode worth designing the fixture around
# when the operation under test is the one that removes things.

@pytest.fixture
def deletable(client):
    """A one-deck library the tests may destroy."""
    deck = Deck.from_text(
        "slug: doomed\nname: Doomed Deck\nstage: draft\n"
        "commander:\n  - Gyome, Master Chef\n"
        "cards:\n  - name: Swamp\n    category: land\n    qty: 99\n",
        slug="doomed")
    source = MemoryDeckSource([deck])
    client.app.dependency_overrides[deck_source] = lambda: source
    client.app.dependency_overrides[library] = lambda: library_over(source)
    yield source
    client.app.dependency_overrides.pop(deck_source, None)
    client.app.dependency_overrides.pop(library, None)


def test_deleting_needs_a_typed_word_as_confirmation(client, deletable):
    """Not a boolean. A client that sends `confirm=true` for every deletion has
    confirmed nothing, and a mis-aimed request looks exactly like an intended
    one. Both accepted answers have to be typed out."""
    r = client.request("DELETE", "/api/decks/local/doomed?confirm=true")
    assert r.status_code == 422
    detail = r.json()["detail"]
    # The refusal names what it wants. A gate whose answer is only in the
    # source is the shape of the bug this replaced.
    assert service.DELETE_WORD in detail and "slug" in detail
    assert deletable.slugs() == ["doomed"], "and nothing was moved"


def test_the_delete_word_is_bury(client):
    """Pinned here because `web/src/routes/Library.tsx` types this word into
    its own dialog and cannot import it. If one side changes, this fails and
    names the other."""
    assert service.DELETE_WORD == "bury"


def test_deleting_with_the_magic_word_removes_the_deck(client, deletable):
    """The short answer, and the one the app asks for. The slug is a stronger
    confirmation and still works, but it was also 26 hyphenated characters on
    the deck it was most often aimed at, which is a gate that gets bypassed in
    the shell rather than satisfied."""
    r = client.request("DELETE", "/api/decks/local/doomed?confirm=bury")
    assert r.status_code == 200
    assert r.json()["deleted"] is True
    assert deletable.slugs() == []


@pytest.mark.parametrize("confirm", ["BURY", "  Bury  ", "DOOMED"])
def test_confirmation_ignores_case_and_surrounding_space(
        client, deletable, confirm):
    """The regression, at the layer that decides it. The app rendered the
    confirmation string uppercased next to a case-sensitive comparison, so
    typing what was on screen was refused with no explanation. Whatever a
    client displays has to be an answer this accepts."""
    r = client.request("DELETE", f"/api/decks/local/doomed?confirm={confirm.strip()}")
    assert r.status_code == 200
    assert deletable.slugs() == []


def test_deleting_with_no_confirmation_at_all_is_refused(client, deletable):
    assert client.request("DELETE", "/api/decks/local/doomed").status_code == 422
    assert deletable.slugs() == ["doomed"]


def test_deleting_the_wrong_slug_is_refused(client, deletable):
    """The mis-click this is actually protecting against: the right dialog
    open over the wrong row."""
    r = client.request("DELETE", "/api/decks/local/doomed?confirm=arahbo-cats")
    assert r.status_code == 422
    assert deletable.slugs() == ["doomed"]


def test_deleting_with_the_slug_removes_the_deck_and_says_where_it_went(
        client, deletable):
    r = client.request("DELETE", "/api/decks/local/doomed?confirm=doomed")
    assert r.status_code == 200
    body = r.json()
    assert body["deleted"] is True
    assert body["name"] == "Doomed Deck"
    # "Deleted" and "recoverable" have to be separately true and separately
    # visible, so the response carries a location rather than only a boolean.
    assert body["moved_to"]
    assert deletable.slugs() == []


def test_deleting_an_unknown_deck_is_a_404(client, deletable):
    r = client.request("DELETE", "/api/decks/local/no-such-deck?confirm=no-such-deck")
    assert r.status_code == 404


def test_a_read_only_library_refuses_a_deletion(client):
    """docs/HOSTING.md keeps the curated decks read-only for everyone but the
    maintainer. This is the operation where that matters most."""
    deck = Deck.from_text(
        "slug: doomed\nname: Doomed\ncommander:\n  - Gyome, Master Chef\n",
        slug="doomed")
    source = MemoryDeckSource([deck], writable=False)
    client.app.dependency_overrides[deck_source] = lambda: source
    client.app.dependency_overrides[library] = lambda: library_over(source)
    try:
        r = client.request("DELETE", "/api/decks/local/doomed?confirm=doomed")
        assert r.status_code == 403
        assert "read-only" in r.json()["detail"]
        assert source.slugs() == ["doomed"]
    finally:
        client.app.dependency_overrides.pop(deck_source, None)
        client.app.dependency_overrides.pop(library, None)
    client.app.dependency_overrides.pop(library, None)


# ----------------------------------------------------------------- claude

def test_claude_status_separates_installed_configured_and_wanted(client):
    """Three questions a UI must not conflate: "no opinions here" reads very
    differently from "you never set a key"."""
    body = client.get("/api/claude").json()
    assert isinstance(body["installed"], bool)
    assert isinstance(body["configured"], bool)
    assert body["model"].startswith("claude-")
    # Off until somebody says otherwise, whatever is installed.
    assert body["stance"]["preset"] == "off"
    assert body["stance"]["allows_calls"] is False


def test_claude_status_reaches_no_network(client):
    """It answers on a base install with no account — so it may not call out
    to check. Availability is a fact about the environment."""
    import mtglab.claude.client as cc
    original = cc.connect
    cc.connect = lambda: pytest.fail("claude_status must not build a client")
    try:
        assert client.get("/api/claude").status_code == 200
    finally:
        cc.connect = original


def test_the_stance_default_comes_from_the_decks_status(client):
    """ADR 15: a theoretical deck is a list under consideration, a built one is
    sleeved cardboard. The default follows a field that already exists."""
    theoretical = client.get("/api/claude",
                             params={"slug": "mono-green"}).json()
    built = client.get("/api/claude", params={"slug": "mono-green-built"}).json()
    assert theoretical["default"]["preset"] == "second-opinion"
    assert built["default"]["preset"] == "consultant"
    # Neither default may write.
    assert not theoretical["default"]["may_write"]
    assert not built["default"]["may_write"]


def test_the_theme_surface_reports_its_own_default_not_off(client):
    """The stance a surface reports must be the stance it will run.

    Caught by building the dial and looking at it: the create flow has no deck,
    so `/api/claude` resolved through `stance.resolve(None, None)` and answered
    `off` — while `theme.stance_for` was about to run the conversation at
    `second-opinion`, because a deck nobody has built yet is as theoretical as
    a deck gets. Every test here passed, because every one of them asked about
    a deck.

    A readout that says `off` next to a mode that is about to make calls is
    worse than no readout, and the sentence "no calls, ever" is the specific
    thing it would have got wrong.
    """
    plain = client.get("/api/claude").json()
    theme = client.get("/api/claude", params={"surface": "theme"}).json()

    # Unchanged for a caller that names nothing: `off` is right when there is
    # genuinely nothing to go on.
    assert plain["default"]["preset"] == "off"
    assert plain["stance"]["preset"] == "off"

    assert theme["default"]["preset"] == "second-opinion"
    assert theme["stance"]["preset"] == "second-opinion"
    assert theme["stance"]["allows_calls"] is True
    # And still no write, at the default, on any surface.
    assert theme["stance"]["may_write"] is False


def test_a_named_surface_never_widens_past_a_pin_or_the_ceiling(client):
    """`surface` picks a *default*; it is not a second way to ask for more.

    Worth pinning because the parameter is new and reads like a mode selector:
    an explicit stance still wins over it, and `resolve` still clamps.
    """
    pinned = client.get("/api/claude",
                        params={"surface": "theme", "stance": "consultant"}).json()
    assert pinned["stance"]["preset"] == "consultant"

    import mtglab.claude.stance as stance_mod
    original = stance_mod.ceiling
    stance_mod.ceiling = lambda: stance_mod.CONSULTANT
    try:
        capped = client.get("/api/claude", params={"surface": "theme"}).json()
        # The surface's own default is `second-opinion`; the ceiling narrows it.
        assert capped["stance"]["preset"] == "consultant"
    finally:
        stance_mod.ceiling = original


def test_a_requested_stance_is_reported_back_resolved(client):
    body = client.get("/api/claude", params={"stance": "collaborator"}).json()
    assert body["stance"]["preset"] == "collaborator"
    assert body["stance"]["may_write"] is True


def test_an_unknown_stance_is_refused_rather_than_ignored(client):
    """Silently falling back would hand someone a different dial than the one
    they asked for."""
    r = client.get("/api/claude", params={"stance": "chatty"})
    assert r.status_code == 422


def test_a_deployment_ceiling_marks_presets_unavailable(client, monkeypatch):
    """So a UI can grey out what it may not offer, rather than letting someone
    pick a level that is silently clamped."""
    from mtglab.claude import stance as st
    monkeypatch.setenv(st.CEILING_ENV, "consultant")
    body = client.get("/api/claude", params={"stance": "collaborator"}).json()
    assert body["stance"]["preset"] == "consultant", "clamped to the ceiling"
    unavailable = {p["name"] for p in body["presets"] if not p["available"]}
    assert "collaborator" in unavailable
    assert "off" not in unavailable


def test_the_rule_with_no_stance_above_it_is_stated_next_to_the_dial(client):
    body = client.get("/api/claude").json()
    assert "rationale" in body["never"]


def test_the_status_lists_the_modes_that_actually_exist(client):
    """ADR 15 planned four. A UI should offer the ones that are built, and the
    capability column is the part worth serving: it is empty, and saying so is
    more useful than leaving a client to assume."""
    body = client.get("/api/claude").json()
    names = {m["name"] for m in body["modes"]}
    assert "rationale-interview" in names
    assert all(m["writes"] == [] for m in body["modes"])


# --------------------------------------------------- the rationale interview
#
# No test here makes a real call. The route is exercised on the paths that stop
# before one — a bad request, and a stance of `off` — because those are the
# paths a suite can assert on for free, and the rest is a command
# (`mtglab claude interview`) for the same reason `claude check` is.

def test_the_interview_needs_a_card(client):
    r = client.post("/api/decks/local/mono-green/interview", json={})
    assert r.status_code == 422


def test_the_interview_refuses_a_card_the_deck_does_not_run(client):
    """A 422 rather than a 404: the deck is fine, the question is not."""
    r = client.post("/api/decks/local/mono-green/interview",
                    json={"card": "Black Lotus", "stance": "consultant"})
    assert r.status_code == 422
    assert "not in mono-green" in r.json()["detail"]


def test_the_interview_on_an_unknown_deck_is_a_404(client):
    r = client.post("/api/decks/local/no-such-deck/interview", json={"card": "Sol Ring"})
    assert r.status_code == 404


def test_the_interview_at_a_stance_of_off_makes_no_call(client):
    """`off` is a real position, all the way down to the route. The client is
    sabotaged so that any attempt to build one fails the test."""
    import mtglab.claude.client as cc
    original = cc.connect
    cc.connect = lambda: pytest.fail("a stance of off must make no call")
    try:
        r = client.post("/api/decks/local/mono-green/interview",
                        json={"card": "Sol Ring", "stance": "off"})
    finally:
        cc.connect = original
    assert r.status_code == 200
    body = r.json()
    assert body["asked"] is False
    assert body["questions"] == []
    assert body["answered_by"] == "claude", "labelled even when it said nothing"


def test_the_interview_answers_a_malformed_stance_with_422_not_502(client):
    """The request was wrong, not the call.

    Until 2026-08-23 this answered 502: the service layer's re-raise tuple did
    not name the stance's ValueError, so it was swallowed by the broad
    `except Exception` and reported as a failed call, and the route's own
    `except ValueError` branch -- commented "A malformed stance" -- was dead
    code. The Go port reproduced the 502 rather than improve on one runtime,
    raised it, and both moved together. No key is needed to see it: the stance
    is resolved before any call could be made.
    """
    r = client.post("/api/decks/local/mono-green/interview",
                    json={"card": "Sol Ring", "stance": "emperor"})
    assert r.status_code == 422
    assert "is not a stance preset" in r.json()["detail"]


# ------------------------------------------------------- the slot argument
#
# The same paths as the interview above, and the same reason for stopping
# short of a call. Written at the same time as the route rather than after it,
# which the dossier is the argument for: 42 tests matched "dossier" and every
# one exercised the module, so the endpoint shipped with no test at all and
# broke deployed in a way no green suite could have seen.

def test_the_argument_needs_a_card(client):
    r = client.post("/api/decks/local/mono-green/argue", json={})
    assert r.status_code == 422


def test_the_argument_refuses_a_card_the_deck_does_not_run(client):
    """A 422 rather than a 404: the deck is fine, the question is not."""
    r = client.post("/api/decks/local/mono-green/argue",
                    json={"card": "Black Lotus", "stance": "consultant"})
    assert r.status_code == 422
    assert "not in mono-green" in r.json()["detail"]


def test_the_argument_on_an_unknown_deck_is_a_404(client):
    r = client.post("/api/decks/local/no-such-deck/argue",
                    json={"card": "Sol Ring"})
    assert r.status_code == 404


def test_the_argument_at_a_stance_of_off_makes_no_call(client):
    import mtglab.claude.client as cc
    original = cc.connect
    cc.connect = lambda: pytest.fail("a stance of off must make no call")
    try:
        r = client.post("/api/decks/local/mono-green/argue",
                        json={"card": "Sol Ring", "stance": "off"})
    finally:
        cc.connect = original
    assert r.status_code == 200
    body = r.json()
    assert body["asked"] is False
    assert body["charges"] == []
    assert body["answered_by"] == "claude", "labelled even when it said nothing"


def test_the_argument_carries_no_field_for_the_case_in_favour(client):
    """ADR 25 at the HTTP surface, which is where a second client meets it.

    The guard is the response schema and it lives in the mode, but the thing a
    client can actually see is the payload — and a payload with a `defence`
    field would make the one-direction rule a UI convention rather than a
    property of the endpoint.
    """
    r = client.post("/api/decks/local/mono-green/argue",
                    json={"card": "Sol Ring", "stance": "off"})
    body = r.json()
    for field in ("defence", "in_favour", "verdict", "rationale", "why",
                  "recommendation", "summary"):
        assert field not in body, f"the argument may not carry {field}"


def test_the_argument_is_a_different_mode_from_the_interview(client):
    """Two per-card surfaces, and the answer says which one answered.

    They share `brief()` and a request shape, so the field that distinguishes
    them is the one a client renders from -- and rendering a one-sided
    argument under the interview's framing is the misread worth preventing.
    """
    r = client.post("/api/decks/local/mono-green/argue",
                    json={"card": "Sol Ring", "stance": "off"})
    assert r.json()["mode"] == "slot-argument"


def test_the_argument_answers_a_malformed_stance_with_422_not_502(client):
    """The interview's twin, down to the status code -- see its test."""
    r = client.post("/api/decks/local/mono-green/argue",
                    json={"card": "Sol Ring", "stance": {"scope": "everywhere"}})
    assert r.status_code == 422
    assert "is not a scope level" in r.json()["detail"]


# ------------------------------------------------- the commander dossier

def test_commander_dossier_counts_subtypes_off_the_pool(
        pool, in_memory_client):
    """Gyome is a Troll Warlock, and how unusual that is comes from counting
    type lines rather than from anybody's recollection.

    This is the endpoint most tempting to fill with remembered trivia, which
    is exactly why the numbers are queries. A wrong one is a bug with a
    reproducible query behind it."""
    deck = Deck.from_text(
        "slug: gyome\nname: Gyome\ncommander:\n  - Gyome, Master Chef\n"
        "cards:\n  - name: Swamp\n    category: land\n    why: mana\n    qty: 99\n",
        slug="gyome")
    with in_memory_client([deck]) as client:
        body = client.get("/api/decks/local/gyome/commander").json()

    assert body["card"]["type_line"] == "Legendary Creature — Troll Warlock"
    assert body["supertypes"] == ["Legendary", "Creature"]
    names = [s["name"] for s in body["subtypes"]]
    assert names == ["Troll", "Warlock"]
    for row in body["subtypes"]:
        # Every legendary one is also one of the total, so this ordering holds
        # for any pool and catches the two counts being swapped.
        assert 0 < row["legendary"] <= row["total"]


def test_commander_dossier_reads_the_front_face_of_a_double_faced_card(pool):
    """A DFC's `type_line` carries both halves around a `//`. The commander's
    types are the ones on the side you cast, which is the same reason
    `CardRecord.front_type_line` exists."""
    supertypes, subtypes = service._type_parts(
        "Legendary Creature — Cat Warrior // Legendary Planeswalker — Ajani")
    assert supertypes == ["Legendary", "Creature"]
    assert subtypes == ["Cat", "Warrior"], "the back face must not leak in"


def test_commander_dossier_handles_a_type_line_with_no_subtypes(pool):
    assert service._type_parts("Artifact") == (["Artifact"], [])
    assert service._type_parts("Basic Land — Swamp") == (["Basic", "Land"], ["Swamp"])


def test_commander_dossier_is_empty_rather_than_a_404_without_a_pool(
        tmp_path, in_memory_client):
    """A decorative panel must not take the deck page down with it. The deck
    still renders on a fresh clone; the dossier is simply empty."""
    with config.use_paths(data_dir=tmp_path / "absent"), \
            in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.get("/api/decks/local/mono-green/commander")
        assert r.status_code == 200
        assert r.json()["card"] is None
        assert r.json()["subtypes"] == []


def test_commander_dossier_survives_a_pool_with_no_printings(
        pool, in_memory_client):
    """A partially-built pool has oracle rows and no printings. Zero
    printings is a fact to report, not a crash.

    The table is emptied here rather than assumed empty: `tiny_pool` carries
    twelve real printings now, for the camera's corner tier, and a test whose
    subject is an absence has to arrange that absence itself.
    """
    con = db.connect(pool)
    try:
        con.execute("DELETE FROM printings")
    finally:
        con.close()
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        body = client.get("/api/decks/local/mono-green/commander").json()
    assert body["printings"]["count"] == 0
    assert body["printings"]["first_released"] is None
    assert body["printings"]["first_set"] is None


def test_commander_dossier_never_lists_the_commander_among_its_own_relatives(
        pool, in_memory_client):
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        body = client.get("/api/decks/local/mono-green/commander").json()
    assert body["card"]["name"] not in [c["name"] for c in body["other_cards"]]


def test_commander_dossier_404s_for_an_unknown_deck(pool, in_memory_client):
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        assert client.get("/api/decks/local/nope/commander").status_code == 404


# ------------------------------------------- the theme proposal, as a job
#
# ADR 20's expensive half. It was measured at 226 seconds, which no hosted
# proxy will hold a POST open for, so it runs as a background job the way Tier
# 1 does (`api/themeruns.py`). No test here makes a real call: what is asserted
# is the split — everything refusable is still refused *by the POST*, with its
# own status code, and only the network call was moved.

# Three grounded kinds, so the floor is met. Every quote is really in the
# transcript, because `ground()` checks and this is not the place to test that.
THEME_TRANSCRIPT = [
    {"role": "assistant", "text": "Something you love that isn't a game?"},
    {"role": "user", "text": "Dune, easily. I reread it every couple of years"},
    {"role": "assistant", "text": "And when a plan of yours falls apart?"},
    {"role": "user", "text": "I'm a Virgo so I just make a new plan, but at game "
                             "night I'd rather quietly build something"},
]

THEME_SLOTS = [
    {"kind": "taste", "value": "epic desert science fiction",
     "quote": "Dune, easily"},
    {"kind": "temperament", "value": "replans rather than panics",
     "quote": "I just make a new plan"},
    {"kind": "posture", "value": "builds quietly",
     "quote": "quietly build something"},
]


@pytest.fixture
def no_worker(monkeypatch):
    """Fail if anything is queued, and hand back the POST's own answer.

    The seam that makes "a job born finished" checkable rather than racy.
    `_job_for` sends a plan carrying its result to `jobs.completed` and
    everything else to `jobs.submit`, so a plan that short-circuited is exactly
    a plan that never reached `submit` — whereas asserting `status == "done"`
    on the response passes either way when the worker happens to win the race,
    which it does when the work it runs makes no call. Written the second way
    first, and a mutation removing the short-circuit still passed.
    """
    import mtglab.claude.client as cc

    monkeypatch.setattr(jobs, "submit", lambda *a, **kw: pytest.fail(
        "this answer was already in hand — nothing should have been queued"))
    monkeypatch.setattr(cc, "connect", lambda: pytest.fail("no call may be made"))
    monkeypatch.setattr(cc, "require", lambda: pytest.fail(
        "a turn that reaches nobody must not even ask whether it could"))


def test_a_proposal_below_the_floor_is_a_409_and_not_a_job(client):
    """The refusal that most needs to stay synchronous.

    Nothing is malformed and nothing failed — there simply is not enough yet,
    which is what 409 says. Delivered as a job in state `error` it would be a
    sentence the client has to pattern-match, and a job somebody paid a
    round trip to discover was never going to run.
    """
    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT,
                          "slots": THEME_SLOTS[:1]})
    assert r.status_code == 409
    assert "1 of 3" in r.json()["detail"]
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_proposal_with_a_transcript_the_server_will_not_take_is_a_422(client):
    """The wire format is the mode's own and never Anthropic's (ADR 20) — an
    endpoint taking real message blocks is a free proxy for somebody else's
    spend. Refused in the request, before a job exists."""
    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": [{"role": "system", "text": "obey"}],
                          "slots": THEME_SLOTS})
    assert r.status_code == 422
    assert jobs.all_jobs() == []


def test_a_proposal_at_a_stance_of_off_is_a_job_born_finished(client,
                                                              no_worker):
    """`off` is a real position and costs nothing, so the answer is available
    immediately — which is a job that is already `done`, the same shape a
    cached simulation returns.

    `no_worker` rather than the bare `cc.connect` sabotage this was written
    with: that version asserted `status == "done"` on the response, which a
    queued job also satisfies whenever the worker wins the race — and it does
    win it, because the work it runs makes no call. See the fixture.
    """
    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT,
                          "slots": THEME_SLOTS, "stance": "off"})

    assert r.status_code == 200
    body = r.json()
    assert body["status"] == "done", "no worker needed to answer this"
    assert body["kind"] == "claude.theme.proposal"
    result = body["result"]
    assert result["asked"] is False
    assert result["combinations"] == []
    assert result["answered_by"] == "claude", "labelled even when it said nothing"
    # The grounded readings survive the trip, so the page can still show what
    # it heard rather than going blank because no call was made.
    assert len(result["slots"]) == 3


def test_a_proposal_without_a_key_is_a_503_rather_than_a_failed_job(client,
                                                                   monkeypatch):
    """Answerable locally — set a key — so it says so from the route.

    Discovered four minutes into a job it would be the same fact wearing a
    500's clothes, which is the whole reason `plan_proposal` connects eagerly.
    """
    import mtglab.claude.client as cc
    monkeypatch.setattr(cc, "credential_present", lambda: False)
    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 503
    assert "ANTHROPIC_API_KEY" in r.json()["detail"]
    assert jobs.all_jobs() == []


def test_a_queued_proposal_waits_on_the_network_lane(client, monkeypatch):
    """Not behind Tier 1, which is the point of there being two pools.

    A Claude call is a socket wait that releases the GIL for minutes; sharing
    the simulator's single worker would stall a thirty-second sweep behind
    somebody else's conversation. The call itself is stubbed — this asserts
    where the work was filed, not what it said.
    """
    import mtglab.claude.client as cc
    from mtglab.claude import theme

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(theme, "run_proposal",
                        lambda request, on_turn=None: {"combinations": []})

    lanes = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (lanes.append(kw.get("lane")),
                                          real_submit(*a, **kw))[1])

    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 200
    assert lanes == [jobs.NET]
    assert await_job(client, r.json()["id"])["status"] == "done"


# The three error translations inside the worker. Everything refusable is
# refused by the POST above; these are the failures that can only happen once
# the call is already in flight, and until 2026-08-14 not one of them had a
# test. They matter more than their line count suggests: this is the code that
# decides whether an expired key reads as "your key may have expired" or as a
# stack trace in a job's error field, and the key this project runs on has a
# fixed lifetime, so the 401 path is a question of when rather than whether.


def _claude_is_available(monkeypatch):
    """A key and an SDK, without either being real."""
    import mtglab.claude.client as cc
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)


def _expired_key_error():
    """A real `anthropic.AuthenticationError`, which is the failure these three
    paths exist for.

    Deliberately not a bare `RuntimeError`: `explain()` returns `str(exc)` for
    anything it does not recognise, so a generic exception cannot tell a worker
    that translates from one that re-raises. A 401 can — only the translation
    produces the word "expired".
    """
    anthropic = pytest.importorskip("anthropic")
    import httpx

    request = httpx.Request("POST", "https://api.anthropic.com/v1/messages")
    response = httpx.Response(401, request=request, json={
        "type": "error", "error": {"type": "authentication_error",
                                   "message": "invalid x-api-key"}})
    return anthropic.AuthenticationError("invalid x-api-key",
                                         response=response, body=None)


def test_a_proposal_that_fails_mid_call_becomes_a_readable_job_error(
        client, monkeypatch):
    """A broad `except` on purpose, and narrow in effect: everything reaching
    it is the SDK failing, and the job carries `explain()`'s account of it
    rather than a traceback.

    The key this project runs on has a fixed lifetime, so this is the failure
    to expect rather than an exotic one — and four minutes in, the job's error
    field is the only thing anybody gets to read.
    """
    from mtglab.claude import theme

    _claude_is_available(monkeypatch)

    def boom(request, on_turn=None):
        raise _expired_key_error()

    monkeypatch.setattr(theme, "run_proposal", boom)

    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 200, "the failure belongs in the job, not the POST"

    done = await_job(client, r.json()["id"])
    assert done["status"] == "error"
    assert "expired" in done["error"], "the raw 401 reached the job untranslated"
    assert "Traceback" not in done["error"]


def test_an_exhausted_proposal_says_so_rather_than_returning_a_stub(
        client, monkeypatch):
    """`ModeExhausted` is the tool loop hitting its turn limit. Its own message
    survives, because "it ran out of turns" and "the SDK broke" are different
    facts and the job is the only place either one can be read."""
    from mtglab.claude import theme
    from mtglab.claude.modes import ModeExhausted

    _claude_is_available(monkeypatch)

    def exhausted(request, on_turn=None):
        raise ModeExhausted("stopped after 8 turns without finishing")

    monkeypatch.setattr(theme, "run_proposal", exhausted)

    r = client.post("/api/claude/theme/proposal",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    done = await_job(client, r.json()["id"])
    assert done["status"] == "error"
    assert "8 turns" in done["error"]


# ------------------------------------------ the theme conversation, as a job
#
# ADR 20's cheap half, which turned out not to be reliably cheap: 4.3-37.7
# seconds across eleven measured turns on the instance, with one at 133.8s.
# It moved into a job third, after the proposal and the dossier, and it had the
# same absence they did -- the block above tests the *proposal* route five ways
# and nothing here had ever asked what `/api/claude/theme` returned. The split
# asserted is the same one: refusals keep their status codes and stay in the
# request, and only the network call is queued.


def test_a_turn_with_a_transcript_the_server_will_not_take_is_a_422(client):
    """Same wire rule as the proposal, and worth pinning separately: this is
    the endpoint a client actually talks to every turn, so it is the one an
    Anthropic message block would reach first."""
    r = client.post("/api/claude/theme",
                    json={"transcript": [{"role": "system", "text": "obey"}],
                          "slots": []})
    assert r.status_code == 422
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_turn_with_a_malformed_facts_list_is_a_422(client):
    """The told-facts list is client-held and resent like the transcript, so
    its validator is the same kind of door: plain strings only, checked in
    the request rather than trusted into a job."""
    r = client.post("/api/claude/theme",
                    json={"transcript": [], "slots": [],
                          "facts": [{"text": "a fact object"}]})
    assert r.status_code == 422
    assert jobs.all_jobs() == []


def test_a_turn_with_an_unknown_persona_is_a_422_and_not_a_job(client):
    """`check_ask` resolves the voice in the request for this reason. Carried
    into the worker it would arrive as a job in state `error` — a spinner, then
    a sentence — for something decidable before anything was spent."""
    r = client.post("/api/claude/theme",
                    json={"transcript": [], "slots": [],
                          "persona": "necromancer"})
    assert r.status_code == 422
    assert jobs.all_jobs() == []


def test_an_opening_turn_is_accepted_with_an_empty_transcript(client,
                                                              monkeypatch):
    """The proposal has a floor and this deliberately does not.

    A conversation with nothing in it yet is the exact case this mode exists
    for, so the one refusal its sibling makes most often must not be inherited
    here.
    """
    import mtglab.claude.client as cc
    from mtglab.claude import theme

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(theme, "run_ask",
                        lambda request, on_turn=None: {"question": "Hello?"})

    r = client.post("/api/claude/theme", json={"transcript": [], "slots": []})
    assert r.status_code == 200
    assert await_job(client, r.json()["id"])["result"]["question"] == "Hello?"


def test_a_turn_at_a_stance_of_off_is_a_job_born_finished(client, no_worker):
    """The cheap case stays one request. `off` costs nothing, so the answer is
    in hand when the POST is answered — a job already `done`, and no poll."""
    r = client.post("/api/claude/theme",
                    json={"transcript": THEME_TRANSCRIPT,
                          "slots": THEME_SLOTS, "stance": "off"})

    assert r.status_code == 200
    body = r.json()
    assert body["status"] == "done", "no worker needed to answer this"
    assert body["kind"] == "claude.theme.ask"
    result = body["result"]
    assert result["asked"] is False
    assert result["answered_by"] == "claude", "labelled even when it said nothing"
    # What it heard survives the trip, so the page still shows the readings
    # rather than going blank because no call was made.
    assert len(result["slots"]) == 3


def test_a_conversation_past_its_ceiling_is_a_job_born_finished(client,
                                                                no_worker):
    """The second no-call case, and the one the proposal has no equivalent of.

    Past `MAX_EXCHANGES` the answer is a finished conversation rather than an
    error — and it is Python's answer, not the model's, so making somebody poll
    for it would be a round trip spent on a sentence already written.
    """
    from mtglab.claude import theme

    long_talk = []
    for i in range(theme.MAX_EXCHANGES):
        long_talk.append({"role": "assistant", "text": f"Question {i}?"})
        long_talk.append({"role": "user", "text": f"Answer {i}"})

    r = client.post("/api/claude/theme",
                    json={"transcript": long_talk, "slots": []})

    body = r.json()
    assert body["status"] == "done"
    assert body["result"]["asked"] is False
    assert str(theme.MAX_EXCHANGES) in body["result"]["reason"]


def test_a_turn_that_fails_mid_call_becomes_a_readable_job_error(client,
                                                                 monkeypatch):
    """The third copy of the same error translation — `plan_ask` has it too,
    and it was the last of the three to move off the request path. A turn is
    the cheapest of the three surfaces and the most frequently run, so it is
    the one most likely to be the first to meet an expired key."""
    from mtglab.claude import theme

    _claude_is_available(monkeypatch)

    def boom(request, on_turn=None):
        raise _expired_key_error()

    monkeypatch.setattr(theme, "run_ask", boom)

    r = client.post("/api/claude/theme",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 200, "the failure belongs in the job, not the POST"

    done = await_job(client, r.json()["id"])
    assert done["status"] == "error"
    assert "expired" in done["error"], "the raw 401 reached the job untranslated"
    assert "Traceback" not in done["error"]


def test_a_turn_without_a_key_is_a_503_rather_than_a_failed_job(client,
                                                               monkeypatch):
    """Answerable locally — set a key — so it says so from the route, with the
    status the UI already renders as a sentence about `.env`."""
    import mtglab.claude.client as cc
    monkeypatch.setattr(cc, "credential_present", lambda: False)
    r = client.post("/api/claude/theme",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 503
    assert "ANTHROPIC_API_KEY" in r.json()["detail"]
    assert jobs.all_jobs() == []


def test_a_queued_turn_waits_on_the_network_lane(client, monkeypatch):
    """Not behind Tier 1. A conversation turn is short, which makes this worse
    rather than better: queued behind a thirty-second sweep, the surface that
    is supposed to feel like typing to somebody stalls for the whole sweep."""
    import mtglab.claude.client as cc
    from mtglab.claude import theme

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(theme, "run_ask",
                        lambda request, on_turn=None: {"question": "What else?"})

    lanes = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (lanes.append(kw.get("lane")),
                                          real_submit(*a, **kw))[1])

    r = client.post("/api/claude/theme",
                    json={"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS})
    assert r.status_code == 200
    assert lanes == [jobs.NET]
    assert await_job(client, r.json()["id"])["status"] == "done"


def test_two_turns_in_flight_are_two_jobs_and_not_one(client, monkeypatch):
    """`key=None`, and the opposite call from the dossier's.

    `jobs.submit(key=...)` collapses concurrent duplicates, which is right for
    a dossier — two clicks inside four minutes are one question asked twice.
    A transcript is client-held, so two turns in flight are two *conversations*,
    and joining them would hand one of them the other's question.
    """
    import mtglab.claude.client as cc
    from mtglab.claude import theme

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(theme, "run_ask",
                        lambda request, on_turn=None: {"question": "Again?"})

    keys = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (keys.append(kw.get("key")),
                                          real_submit(*a, **kw))[1])

    body = {"transcript": THEME_TRANSCRIPT, "slots": THEME_SLOTS}
    first = client.post("/api/claude/theme", json=body).json()
    second = client.post("/api/claude/theme", json=body).json()

    assert keys == [None, None], "a turn must not opt into dedupe"
    assert first["id"] != second["id"]


# The commander dossier, which moved to a job for the same reason and one
# session later. It had **no route tests at all** until this block, which is a
# large part of how a 236-second synchronous endpoint reached production: the
# 42 tests matching "dossier" all exercise the module, and none of them ever
# asked what the HTTP surface did with it.

@pytest.fixture
def dossier_client(in_memory_client, pool):
    """A deck whose commander the pool can actually resolve.

    The planning half looks the commander up before anything is queued -- that
    is what makes a bad deck a 422 in the request rather than four minutes
    later -- so this needs a real pool behind a real deck, the same shape
    `sim_client` needs for compilation.
    """
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck(clean=True)]) as c:
        yield c


def test_writing_a_dossier_is_a_job_and_not_a_four_minute_post(dossier_client,
                                                               monkeypatch):
    """The regression the whole of `api/dossierruns.py` exists for.

    Measured at 236 seconds on the deployed instance, where it presented as a
    spinner and then Safari's `Load failed` — a *transport* error, so no status
    code ever reached the client, and no access-log line was written either,
    because uvicorn writes one when a response completes. The call is stubbed;
    what is asserted is that the answer is a **job**, filed on the network
    lane, whose result is the report.
    """
    import mtglab.claude.client as cc
    from mtglab.claude import dossier

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(dossier, "run_dossier",
                        lambda request, on_turn=None: {
                            "commander": request.commander, "asked": True})

    lanes = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (lanes.append(kw.get("lane")),
                                          real_submit(*a, **kw))[1])

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    assert r.status_code == 200
    body = r.json()
    assert body["kind"] == "claude.dossier"
    assert lanes == [jobs.NET], "a socket wait must not queue behind Tier 1"

    done = await_job(dossier_client, body["id"])
    assert done["status"] == "done"
    assert done["result"]["commander"] == "Goreclaw, Terror of Qal Sisma"


def test_a_dossier_without_a_key_is_a_503_rather_than_a_failed_job(
        dossier_client, monkeypatch):
    """Answerable locally — set a key — so it is answered from the route.

    Discovered four minutes into a job it would be the same fact wearing a
    job-error's clothes, and the UI already knows what to do with a 503.
    """
    import mtglab.claude.client as cc
    monkeypatch.setattr(cc, "credential_present", lambda: False)

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    assert r.status_code == 503
    assert "ANTHROPIC_API_KEY" in r.json()["detail"]
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_stored_dossier_is_a_job_born_finished(dossier_client, monkeypatch):
    """ADR 19 caches on the commander's `oracle_id`, so a hit is the answer and
    not a substitute for it. It comes back already `done` — the response shape
    does not fork — and the client is sabotaged so any attempt to call fails."""
    import mtglab.claude.client as cc
    from mtglab.claude import dossier

    monkeypatch.setattr(dossier, "get", lambda key, path=None: {
        "result": {"who": {"prose": "A bear.", "source_ids": []},
                   "sources": [], "competitors": [],
                   "rivals": {"prose": "", "source_ids": []}, "searched": 0},
        "created_at": "2026-08-13T18:08:16+00:00"})
    monkeypatch.setattr(cc, "connect",
                        lambda: pytest.fail("a stored dossier must make no call"))

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    assert r.status_code == 200
    body = r.json()
    assert body["status"] == "done", "no worker needed to answer this"
    assert body["kind"] == "claude.dossier"
    assert body["result"]["asked"] is False
    assert body["result"]["cached"] is True, "quote a cached dossier as cached"


def test_a_deck_with_no_commander_the_pool_knows_is_a_422_before_any_job(
        dossier_client, monkeypatch):
    """A fact about the deck, not a failure of the model — and a poor thing to
    wait four minutes to be told."""
    from mtglab.claude import dossier

    def no_commander(slug, source=None):
        raise dossier.NoCommander("no commander this pool knows")

    monkeypatch.setattr(dossier, "brief", no_commander)

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    assert r.status_code == 422
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_dossier_that_fails_mid_call_becomes_a_readable_job_error(
        dossier_client, monkeypatch):
    """The counterpart to the 503 above, for the failures that can only happen
    once the call is already in flight. The 236-second run is the one place
    nobody is watching, so what it leaves in the job's error field is the whole
    of what a person gets to debug from."""
    from mtglab.claude import dossier

    _claude_is_available(monkeypatch)

    def boom(request, on_turn=None):
        raise _expired_key_error()

    monkeypatch.setattr(dossier, "run_dossier", boom)

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    assert r.status_code == 200, "the failure belongs in the job, not the POST"

    done = await_job(dossier_client, r.json()["id"])
    assert done["status"] == "error"
    assert "expired" in done["error"], "the raw 401 reached the job untranslated"
    assert "Traceback" not in done["error"]


def test_an_exhausted_dossier_is_an_error_and_not_a_half_written_one(
        dossier_client, monkeypatch):
    """ADR 19 refuses a dossier with no surviving source rather than showing a
    thin one; a dossier that ran out of turns is the same argument one step
    earlier. A truncated answer that reads finished is the failure worth
    avoiding — the Forge-with-96-cards shape."""
    from mtglab.claude import dossier
    from mtglab.claude.modes import ModeExhausted

    _claude_is_available(monkeypatch)

    def exhausted(request, on_turn=None):
        raise ModeExhausted("stopped after 8 turns without finishing")

    monkeypatch.setattr(dossier, "run_dossier", exhausted)

    r = dossier_client.post("/api/decks/local/mono-green/dossier", json={})
    done = await_job(dossier_client, r.json()["id"])
    assert done["status"] == "error"
    assert "8 turns" in done["error"]


@pytest.fixture
def held_dossier(dossier_client, monkeypatch):
    """A dossier run that blocks until the test lets it finish.

    Four minutes is the window the dedupe exists to cover, and a test cannot
    wait four minutes -- so the run is stopped in the middle instead, which is
    the same shape and the only way to have two requests overlap on purpose.
    """
    import threading

    import mtglab.claude.client as cc
    from mtglab.claude import dossier

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)

    started, release, calls = threading.Event(), threading.Event(), []

    def held(request, on_turn=None):
        calls.append(request.commander)
        started.set()
        release.wait(10)
        return {"commander": request.commander, "asked": True}

    monkeypatch.setattr(dossier, "run_dossier", held)
    try:
        yield dossier_client, started, release, calls
    finally:
        # Always, even when an assertion fired: a worker still parked on this
        # event would hold a NET slot for the next test in the file.
        release.set()


def test_a_second_ask_joins_the_run_already_going(held_dossier):
    """The money bug, and the one on the punchlist worth doing first.

    A dossier takes about four minutes and pays for a web search. Nothing
    stopped a second click, a second tab or a second device inside that window
    from starting a **second paid run for the same commander**, and on
    2026-08-13 two went concurrently on the deployed instance for exactly that
    reason. The reattach built alongside it lives in one tab's localStorage and
    so only ever covered one tab; this is the server knowing.
    """
    client, started, release, calls = held_dossier

    first = client.post("/api/decks/local/mono-green/dossier", json={}).json()
    assert started.wait(10), "the first run never started"
    second = client.post("/api/decks/local/mono-green/dossier", json={}).json()

    assert second["id"] == first["id"], "a second ask must join, not pay again"
    release.set()

    assert await_job(client, first["id"])["status"] == "done"
    assert calls == ["Goreclaw, Terror of Qal Sisma"], "one run, one bill"
    assert len(jobs.all_jobs()) == 1, "and one job in the list, not two"


def test_a_finished_run_is_not_joined(held_dossier):
    """Only a *live* job is reused. What covers "somebody asked before" is the
    cache (ADR 19), one step earlier in time and with a stored answer to hand
    back; joining a finished job would hand over a result whose freshness
    nothing had checked."""
    client, started, release, _calls = held_dossier

    first = client.post("/api/decks/local/mono-green/dossier", json={}).json()
    assert started.wait(10)
    release.set()
    assert await_job(client, first["id"])["status"] == "done"

    second = client.post("/api/decks/local/mono-green/dossier", json={}).json()
    assert second["id"] != first["id"]


def test_two_accounts_asking_at_once_do_not_share_a_job():
    """Matching is per owner as well as per key, and that is not caution.

    A job belongs to a person (ADR 5) and `get` reports somebody else's as
    absent -- so handing two accounts one id would give the second a 404 for a
    job it had just been told about. Every case the bug actually covers is one
    person with two tabs.
    """
    jobs.clear()
    mine = jobs.submit("claude.dossier", lambda _p: None, owner=1, key="k")
    theirs = jobs.submit("claude.dossier", lambda _p: None, owner=2, key="k")
    assert mine.id != theirs.id
    assert jobs.get(theirs.id, owner=1) is None


def test_work_with_no_key_is_never_deduplicated():
    """The default, and right for anything not reproducible. A theme proposal
    is a conversation nobody else is having; two at once are two proposals."""
    jobs.clear()
    one = jobs.submit("claude.theme.proposal", lambda _p: None)
    two = jobs.submit("claude.theme.proposal", lambda _p: None)
    assert one.id != two.id


def test_an_unknown_job_lane_is_refused_before_a_job_is_recorded():
    """A typo must not become a job that sits `queued` forever.

    The registry has two pools now — CPU for the simulator, NET for anything
    that waits on Anthropic — and picking a third by accident is the one
    failure that would look like a hang rather than an error.
    """
    jobs.clear()
    with pytest.raises(ValueError, match="unknown job lane"):
        jobs.submit("test", lambda _p: None, lane="nope")
    assert jobs.all_jobs() == []


# ------------------------------------------------- research, as a job (ADR 26)
#
# **The first Claude route in this codebase that had tests before it had a
# deploy.** The dossier is the reason that sentence is worth writing: it shipped
# as a 236-second synchronous POST because the 42 tests matching "dossier" all
# exercised the module and none of them ever asked what the HTTP surface did.
#
# What is asserted here is the split — everything refusable is refused *by the
# POST*, with its own status code, and only the Anthropic call was moved — plus
# the one thing that is peculiar to this mode and not to any sibling: the route
# takes no owner, no slug and no deck, which is ADR 26's first decision reaching
# all the way out to the URL.


def test_research_takes_no_deck_anywhere_in_its_signature(client):
    """ADR 26 at the layer a diff would change first.

    A deck-aware research mode is one path parameter away, and this is the
    assertion that fails when somebody adds it. `/api/claude/research` sits
    beside the theme routes rather than under `/api/decks/{owner}/{slug}`
    precisely so that reaching a deck from here is a visible change.
    """
    paths = {r.path for r in client.app.routes}
    assert "/api/claude/research" in paths
    assert not any("research" in p and "decks" in p for p in paths)


def test_an_empty_question_is_a_422_and_not_a_job(client):
    """Refused in the request. Delivered as a job in state `error` it would be
    a sentence the client has to pattern-match, and a round trip somebody paid
    to discover the request was never going to run."""
    jobs.clear()
    r = client.post("/api/claude/research", json={"question": "   "})
    assert r.status_code == 422
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_pasted_decklist_is_a_422_that_says_why(client):
    """The refusal a user is most likely to hit, and the one that has to teach
    something: this surface cannot see decks, so pasting one is not the way in."""
    jobs.clear()
    r = client.post("/api/claude/research",
                    json={"question": "Sol Ring\n" * 400})
    assert r.status_code == 422
    assert "cannot see decks" in r.json()["detail"]
    assert jobs.all_jobs() == []


def test_a_malformed_stance_is_a_422_rather_than_a_job(client):
    jobs.clear()
    r = client.post("/api/claude/research",
                    json={"question": "What is the meta?",
                          "stance": "omniscient"})
    assert r.status_code == 422
    assert jobs.all_jobs() == []


def test_research_with_no_key_is_a_503_before_anything_is_queued(client,
                                                                 monkeypatch):
    """503 means no call was possible; it is not the same answer as a call that
    came back unusable, and collapsing them tells somebody their key is missing
    when the model was merely rate limited."""
    import mtglab.claude.client as cc

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: False)
    r = client.post("/api/claude/research", json={"question": "Anything?"})
    assert r.status_code == 503
    assert jobs.all_jobs() == []


def test_research_at_a_stance_of_off_is_a_job_born_finished(client, no_worker):
    """`off` is a real position and costs nothing, so the answer is in hand —
    which is a job already `done` rather than a pretence that it is not a job.

    `no_worker` rather than asserting `status == "done"`, which a queued job
    also satisfies whenever the worker wins the race — and it wins it here,
    because the work it would run makes no call. See the fixture.
    """
    r = client.post("/api/claude/research",
                    json={"question": "What is the meta?", "stance": "off"})
    assert r.status_code == 200
    body = r.json()
    assert body["status"] == "done", "no worker needed to answer this"
    assert body["kind"] == "claude.research"
    assert body["result"]["asked"] is False
    assert "stance is off" in body["result"]["reason"]


def test_research_runs_in_the_net_lane(client, monkeypatch):
    """A Claude call waits on a socket with the GIL released. Queued behind the
    single CPU worker it would stall a Tier 1 sweep for minutes, and be stalled
    by one."""
    import mtglab.claude.client as cc
    from mtglab.claude import research

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(research, "run_research",
                        lambda request, on_turn=None: {"question": "x"})

    lanes = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (lanes.append(kw.get("lane")),
                                          real_submit(*a, **kw))[1])

    r = client.post("/api/claude/research", json={"question": "Anything?"})
    assert r.status_code == 200
    assert lanes == [jobs.NET]
    assert await_job(client, r.json()["id"])["status"] == "done"


def test_the_same_question_twice_in_flight_is_one_job(client, monkeypatch):
    """The opposite call from the theme conversation's `key=None`, and the
    reason is that here the question text *is* the whole input.

    A transcript is client-held, so two identical-looking theme turns are two
    conversations. Two identical question strings from one account inside the
    minutes a search takes are one question asked twice — the argument
    `api/dossierruns.py` had to learn after two paid runs for one commander
    went concurrently on the deployed instance.
    """
    import mtglab.claude.client as cc
    from mtglab.claude import research

    started = []

    def slow(request, on_turn=None):
        started.append(request.question)
        time.sleep(0.4)
        return {"question": request.question}

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(research, "run_research", slow)

    body = {"question": "Is Goreclaw still played?"}
    first = client.post("/api/claude/research", json=body).json()
    # Same question, different spacing and case: normalised to the same key,
    # because neither makes it a different question.
    second = client.post(
        "/api/claude/research",
        json={"question": "is  goreclaw   STILL played?"}).json()

    assert first["id"] == second["id"], "a second click must join the run"
    await_job(client, first["id"])
    assert len(started) == 1, "the search must not have been paid for twice"


def test_a_different_question_is_a_different_job(client, monkeypatch):
    """The other half of the dedupe, which is the half that would be wrong: a
    key too coarse would hand somebody an answer to a question they did not
    ask."""
    import mtglab.claude.client as cc
    from mtglab.claude import research

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(research, "run_research",
                        lambda request, on_turn=None: {"q": request.question})

    first = client.post("/api/claude/research",
                        json={"question": "Is Goreclaw played?"}).json()
    second = client.post("/api/claude/research",
                         json={"question": "Is Gyome played?"}).json()
    assert first["id"] != second["id"]


def test_a_call_that_comes_back_unusable_is_a_job_error_not_a_502(client,
                                                                  monkeypatch):
    """The status code that is deliberately *not* here. By the time the call
    fails the response has long since been sent, so the failure belongs on the
    job — the same move the dossier and both theme routes made."""
    import mtglab.claude.client as cc
    from mtglab.claude import research

    def boom(request, on_turn=None):
        raise RuntimeError("the API said no")

    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(research, "run_research", boom)

    r = client.post("/api/claude/research", json={"question": "Anything?"})
    assert r.status_code == 200
    finished = await_job(client, r.json()["id"])
    assert finished["status"] == "error"
    assert "the API said no" in finished["error"]


def test_the_research_surface_reports_its_own_default_not_off(client):
    """The fault the stance dial found once already, in the only other place it
    could happen.

    There is no deck here, so `/api/claude` would resolve through
    `stance.resolve(None, None)` and report `off` — while `research.stance_for`
    was about to run the search at `second-opinion`. A readout is a claim, and
    this is the second surface with cause to check it.
    """
    from mtglab.claude.research import DEFAULT_PRESET

    bare = client.get("/api/claude").json()
    assert bare["default"]["preset"] == "off"

    surface = client.get("/api/claude",
                         params={"surface": "research"}).json()
    assert surface["default"]["preset"] == DEFAULT_PRESET
    assert surface["stance"]["preset"] == DEFAULT_PRESET
    assert surface["stance"]["allows_calls"] is True
    # And still no write, at any stance, on any surface.
    assert surface["stance"]["may_write"] is False


def test_the_status_lists_research_among_the_modes_that_exist(client):
    """A UI offers what is built. `server_tools` is the field that matters
    here: "searches the web" is something a user cares about in a way
    "get_cards" is not."""
    body = client.get("/api/claude").json()
    modes = {m["name"]: m for m in body["modes"]}
    assert "research" in modes
    assert modes["research"]["server_tools"] == ["web_search"]
    assert modes["research"]["writes"] == []
    # ADR 26's first decision, published: no deck tool in the capability list a
    # client renders.
    assert "get_deck" not in modes["research"]["tools"]


# ------------------------------------------------- service: the seams
#
# The translation layer between the Claude modes and the routes, and the
# refusals `create_deck` makes before it will touch the pool. Every test here
# calls the service directly: what is being pinned is which exception (and
# which sentence) crosses the boundary, not what a route does with it --
# test_api's route tests already cover that half.

@pytest.mark.parametrize("module_path", [
    "mtglab.claude.interview", "mtglab.claude.argue", "mtglab.claude.dossier"])
def test_an_exhausted_mode_keeps_its_own_sentence(module_path, monkeypatch):
    """`ModeExhausted` is the tool loop hitting its limit -- a fact about the
    conversation, not the SDK, so its message must survive the translation
    to `ClaudeFailed` rather than arriving re-explained."""
    import importlib

    from mtglab.claude.modes import ModeExhausted

    mod = importlib.import_module(module_path)

    def exhausted(*a, **kw):
        raise ModeExhausted("stopped after 8 turns without finishing")
    monkeypatch.setattr(mod, "ask", exhausted)

    call = {"mtglab.claude.interview":
                lambda: service.claude_interview(slug="x", card="y"),
            "mtglab.claude.argue":
                lambda: service.claude_argue(slug="x", card="y"),
            "mtglab.claude.dossier":
                lambda: service.claude_dossier(slug="x")}[module_path]
    with pytest.raises(service.ClaudeFailed, match="8 turns"):
        call()


@pytest.mark.parametrize("module_path", [
    "mtglab.claude.interview", "mtglab.claude.argue", "mtglab.claude.dossier"])
def test_an_sdk_failure_is_explained_not_dumped(module_path, monkeypatch):
    """Anything else out of a mode is the SDK breaking, and `explain` is the
    function that knows how to say so -- a raw exception in a 502 detail is
    the stack-trace-in-a-job failure wearing a status code."""
    import importlib
    mod = importlib.import_module(module_path)

    def boom(*a, **kw):
        raise RuntimeError("socket exploded mid-call")
    monkeypatch.setattr(mod, "ask", boom)

    call = {"mtglab.claude.interview":
                lambda: service.claude_interview(slug="x", card="y"),
            "mtglab.claude.argue":
                lambda: service.claude_argue(slug="x", card="y"),
            "mtglab.claude.dossier":
                lambda: service.claude_dossier(slug="x")}[module_path]
    with pytest.raises(service.ClaudeFailed, match="socket exploded"):
        call()


def test_dossier_status_of_a_deck_with_no_commander_is_an_empty_shell(
        monkeypatch):
    """The status probe is a GET and must answer 200-shaped for any deck --
    a commanderless draft asks it constantly while the create flow runs."""
    from mtglab.claude import dossier as dossier_mod

    def no_commander(slug, source=None):
        raise dossier_mod.NoCommander("nothing leads this deck")
    monkeypatch.setattr(dossier_mod, "brief", no_commander)
    body = service.claude_dossier_cached(slug="empty")
    assert body["commander"] == ""
    assert body["cached"] is False
    assert body["dossier"] == {}


def test_commander_dossier_shell_when_the_pool_lacks_the_commander(pool):
    deck = Deck.from_text(
        "slug: ghost\nname: Ghost\ncommander:\n  - No Such Legend\ncards: []",
        slug="ghost")
    body = service.commander_dossier("ghost",
                                     source=MemoryDeckSource([deck]))
    assert body["card"] is None
    assert body["printings"] is None


def test_create_refuses_a_malformed_slug(pool, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={
            "slug": "Bad Slug!", "commander": ["Gyome, Master Chef"]})
    assert r.status_code == 422
    assert "not a usable slug" in r.json()["detail"]


def test_create_refuses_three_commanders(pool, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={
            "slug": "trio", "commander": ["Gyome, Master Chef",
                                          "Goreclaw, Terror of Qal Sisma",
                                          "Vorinclex, Voice of Hunger"]})
    assert r.status_code == 422
    assert "at most two" in r.json()["detail"]


def test_create_refuses_an_unknown_status(pool, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={
            "slug": "statusy", "commander": ["Gyome, Master Chef"],
            "status": "aspirational"})
    assert r.status_code == 422
    assert "is not one of" in r.json()["detail"]


def test_create_refuses_a_commander_the_pool_does_not_know(
        pool, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={
            "slug": "ghost", "commander": ["No Such Legend"]})
    assert r.status_code == 422
    assert "not in the pool" in r.json()["detail"]


def test_create_refuses_two_legends_that_cannot_pair(pool, in_memory_client):
    """Two commanders is not 'any two legends' -- Gyome and Goreclaw are both
    legal commanders alone and have no pairing ability between them."""
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={
            "slug": "duo", "commander": ["Gyome, Master Chef",
                                         "Goreclaw, Terror of Qal Sisma"]})
    assert r.status_code == 422
    assert r.json()["detail"], "a refused pair must say why"


def test_upcoming_sets_filters_to_unreleased_paper_sets(monkeypatch):
    """The one on-demand network route, faked at urllib: only future,
    non-digital sets survive, sorted by release."""
    import io
    import json as _json
    import urllib.request

    payload = {"data": [
        {"code": "ftr", "name": "Future Set", "released_at": "2099-01-02",
         "card_count": 300, "icon_svg_uri": "x", "set_type": "expansion"},
        {"code": "ftr2", "name": "Nearer Set", "released_at": "2099-01-01",
         "card_count": 250, "icon_svg_uri": "x", "set_type": "expansion"},
        {"code": "old", "name": "Released Set", "released_at": "2001-01-01"},
        {"code": "dig", "name": "Arena Only", "released_at": "2099-06-01",
         "digital": True},
    ]}

    class FakeResp(io.BytesIO):
        def __enter__(self):
            return self

        def __exit__(self, *exc):
            return False

    monkeypatch.setattr(urllib.request, "urlopen",
                        lambda req, timeout: FakeResp(
                            _json.dumps(payload).encode()))
    body = service.upcoming_sets(force=True)
    assert [s["code"] for s in body["sets"]] == ["ftr2", "ftr"]

    # And the second ask the same day never reaches the network.
    def refuse(req, timeout):
        raise AssertionError("cache miss: urlopen reached")
    monkeypatch.setattr(urllib.request, "urlopen", refuse)
    assert service.upcoming_sets()["sets"][0]["code"] == "ftr2"


GHOST_DECK_YAML = """\
slug: ghost
name: Ghost
commander:
  - No Such Legend
cards: []
"""


def test_add_refuses_a_card_the_pool_does_not_know(pool, in_memory_client):
    deck = tiny_pool.mono_green_deck()
    with in_memory_client([deck]) as c:
        r = c.post(f"/api/decks/{LOCAL_OWNER}/mono-green/cards", json={
            "name": "No Such Card", "category": "ramp", "why": "x"})
    assert r.status_code == 422
    assert "not a card the pool knows" in r.json()["detail"]


def test_add_refuses_a_banned_card(pool, in_memory_client):
    """Rule 1's cheapest dividend: the pool knows Emrakul is banned, so the
    add is refused before a rationale is ever asked for."""
    deck = tiny_pool.mono_green_deck()
    with in_memory_client([deck]) as c:
        r = c.post(f"/api/decks/{LOCAL_OWNER}/mono-green/cards", json={
            "name": "Emrakul, the Aeons Torn", "category": "threat",
            "why": "big"})
    assert r.status_code == 422
    assert "not legal in Commander" in r.json()["detail"]


def test_set_category_refuses_a_word_off_the_taxonomy(pool, in_memory_client):
    deck = tiny_pool.mono_green_deck()
    with in_memory_client([deck]) as c:
        r = c.patch(f"/api/decks/{LOCAL_OWNER}/mono-green/cards/Sol Ring",
                    json={"field": "category", "value": "nonsense"})
    # The category list is fixed so counts stay comparable across decks.
    assert r.status_code == 422
    assert "nonsense" in r.json()["detail"]


def test_set_art_on_a_commanderless_deck_is_refused(pool):
    deck = Deck.from_text("slug: headless\nname: H\ncommander: []\ncards: []",
                          slug="headless")
    src = MemoryDeckSource([deck])
    with pytest.raises(service.EditRejected, match="no commander"):
        service.set_deck_field("headless", field="commander_art",
                               value="11111111-1111-4111-8111-111111111111",
                               source=src)


def test_set_art_refuses_a_printing_of_somebody_else(pool):
    """The service half of the two-layer check: `edit.py` validates the shape,
    and only a query can know whose painting an id names. A well-formed id
    that is not this commander's is refused with the route that lists the
    ones that are."""
    deck = Deck(slug="gyome-food", name="Gyome Food",
                commander=["Gyome, Master Chef"], cards=[])
    src = MemoryDeckSource([deck])
    with pytest.raises(service.EditRejected,
                       match="not a printing of Gyome, Master Chef"):
        service.set_deck_field(
            "gyome-food", field="commander_art",
            value="99999999-9999-4999-8999-999999999999", source=src)


def test_suggestions_without_a_pool_say_so_instead_of_guessing(tmp_path):
    deck = Deck.from_text(GHOST_DECK_YAML, slug="ghost")
    with config.use_paths(data_dir=tmp_path / "absent"):
        body = service.suggestions_for("ghost",
                                       source=MemoryDeckSource([deck]))
    assert body["pool_available"] is False
    assert body["targets"] == []


def test_the_shelf_asks_for_every_pinned_printing_at_once(pool, monkeypatch):
    """Three decks flying pinned art, one `printings` query.

    The shelf batches the gate and the pool lookup already; the commander's
    chosen printing was the one thing left asking per deck. Counted rather
    than timed, for the reason `test_challenge_progress_asks_the_pool_once`
    gives below: on a shared machine the wall clock is noise and the query
    count is what moved.
    """
    seen: list[str] = []
    real = service._connect

    class Counting:
        def __init__(self, con):
            self._con = con

        def execute(self, sql, *args, **kwargs):
            if "FROM printings" in sql:
                seen.append(sql)
            return self._con.execute(sql, *args, **kwargs)

        def __getattr__(self, name):
            return getattr(self._con, name)

    def counting():
        con = real()
        return Counting(con) if con is not None else None

    monkeypatch.setattr(service, "_connect", counting)

    # Two decks pinning a printing, one leaving the default, and one pinning
    # an id the pool has never held -- the stale case, which must drop out of
    # the batch rather than blanking a tile.
    pins = {
        "goreclaw": ("Goreclaw, Terror of Qal Sisma",
                     "080a5356-61d9-46cf-9095-d59c048f9d77"),
        "gyome": ("Gyome, Master Chef",
                  "5dd7dd1a-6dd1-43c3-8298-7db703d384a1"),
        "plain": ("Craterhoof Behemoth", ""),
        "stale": ("Sol Ring", "00000000-0000-0000-0000-000000000000"),
    }
    decks = [
        Deck.from_text(
            f"slug: {slug}\nname: {slug}\ncommander:\n  - {name}\n"
            + (f"commander_art: {art}\n" if art else "")
            + "cards: []",
            slug=slug)
        for slug, (name, art) in pins.items()
    ]
    tiles = service.list_decks(source=MemoryDeckSource(decks))

    assert len(seen) == 1, f"one printings query for the shelf, got {len(seen)}"
    by_slug = {t["slug"]: t for t in tiles}
    # The two real pins show their chosen printing; the unpinned and the stale
    # both fall back to the pool's own art rather than to nothing.
    assert "080a5356" in by_slug["goreclaw"]["art_crop"]
    assert "5dd7dd1a" in by_slug["gyome"]["art_crop"]
    assert by_slug["plain"]["art_crop"]
    assert by_slug["stale"]["art_crop"]


def test_challenge_progress_asks_the_pool_once(pool, monkeypatch):
    """Three decks, one `get_cards`.

    The obvious loop asks per deck, which is one query per deck for one name
    each -- fine at six and linear in a library aimed at thirty-two slots.
    Counting the calls is what pins it: a wall-clock assertion would be noise
    on a shared machine, and the query count is the thing that actually moved.
    """
    calls: list[list[str]] = []
    real = db.get_cards

    def counting(con, names):
        wanted = list(names)
        calls.append(wanted)
        return real(con, wanted)

    monkeypatch.setattr(db, "get_cards", counting)

    decks = [
        Deck.from_text(f"slug: d{i}\nname: D{i}\ncommander:\n"
                       f"  - {name}\ncards: []", slug=f"d{i}")
        for i, name in enumerate(("Gyome, Master Chef",
                                  "Goreclaw, Terror of Qal Sisma",
                                  "No Such Legend"))
    ]
    body = service.challenge_progress(source=MemoryDeckSource(decks))

    assert len(calls) == 1, f"one query for three decks, got {len(calls)}"
    assert sorted(calls[0]) == ["Goreclaw, Terror of Qal Sisma",
                                "Gyome, Master Chef", "No Such Legend"]
    # Still the right answer: two resolvable commanders in two slots, and the
    # one the pool has never heard of contributes nothing.
    assert body["filled"] == 2


def test_challenge_progress_skips_what_it_cannot_resolve(pool):
    """A commanderless draft and a commander the pool lacks both contribute
    nothing -- skipped, not crashed on, and not counted as filled."""
    headless = Deck.from_text(
        "slug: headless\nname: H\ncommander: []\ncards: []", slug="headless")
    ghost = Deck.from_text(GHOST_DECK_YAML, slug="ghost")
    body = service.challenge_progress(source=MemoryDeckSource([headless, ghost]))
    assert body["filled"] == 0


def test_search_filters_by_type_cost_and_price(pool):
    typed = service.search_cards(type_line="Creature")
    assert typed["total"] >= 1
    assert all("Creature" in c["type_line"] for c in typed["cards"])

    cheap = service.search_cards(cmc_max=1)
    assert all((c["cmc"] or 0) <= 1 for c in cheap["cards"])

    priced = service.search_cards(price_max=0.01)
    assert all(c["price_usd"] is not None and c["price_usd"] <= 0.01
               for c in priced["cards"])


def test_search_without_a_pool_answers_with_advice(tmp_path):
    with config.use_paths(data_dir=tmp_path / "absent"):
        body = service.search_cards(q="anything")
    assert body["cards"] == []
    assert "data refresh" in body["message"]


def test_commander_printings_of_a_headless_deck_is_empty(pool):
    deck = Deck.from_text("slug: headless\nname: H\ncommander: []\ncards: []",
                          slug="headless")
    body = service.commander_printings("headless",
                                       source=MemoryDeckSource([deck]))
    assert body == {"slug": "headless", "commander": "", "selected": "",
                    "printings": []}


def test_commander_dossier_of_a_headless_deck_is_a_shell(pool):
    deck = Deck.from_text("slug: headless\nname: H\ncommander: []\ncards: []",
                          slug="headless")
    body = service.commander_dossier("headless",
                                     source=MemoryDeckSource([deck]))
    assert body["card"] is None


def test_commander_dossier_reports_the_first_printing(pool):
    """The `first_set` lookup: min(released_at) resolved back to a set name."""
    deck = Deck(slug="gyome-food", name="Gyome Food",
                commander=["Gyome, Master Chef"], cards=[])
    body = service.commander_dossier("gyome-food",
                                     source=MemoryDeckSource([deck]))
    assert body["card"]["name"] == "Gyome, Master Chef"
    assert body["printings"] is None or "first_set" in body["printings"]


def test_art_crop_from_rejects_a_url_of_the_wrong_shape():
    assert service.art_crop_from(None) is None
    assert service.art_crop_from("https://c1.scryfall.com/large/x.jpg") is None
    assert service.art_crop_from(
        "https://c1.scryfall.com/normal/x.jpg") == \
        "https://c1.scryfall.com/art_crop/x.jpg"


def test_connect_answers_none_while_a_refresh_holds_the_lock(monkeypatch):
    """`data refresh` takes DuckDB's exclusive lock; the app must degrade to
    'no pool' rather than 500 on every request for the duration."""
    import duckdb

    def locked(path, read_only=False):
        raise duckdb.IOException("database is locked")
    monkeypatch.setattr(duckdb, "connect", locked)
    assert service._connect() is None


def test_pool_stale_never_raises(pool):
    class Broken:
        def execute(self, *a):
            raise RuntimeError("catalog from the future")
    assert service.pool_stale(Broken()) is False


def test_import_refuses_a_slug_that_already_exists(pool, in_memory_client):
    deck = tiny_pool.mono_green_deck()
    with in_memory_client([deck]) as c:
        r = c.post("/api/decks/import", json={
            "text": "1 Forest\n", "slug": "mono-green",
            "commander": "Gyome, Master Chef"})
    assert r.status_code == 422
    assert "already exists" in r.json()["detail"]


def test_create_without_a_pool_refuses_rather_than_guessing(tmp_path):
    """A deck whose commander was never checked is a deck whose colour
    identity is a guess -- the same refusal `import` makes."""
    with config.use_paths(data_dir=tmp_path / "absent"), \
            pytest.raises(service.CreateRejected, match="needs the card pool"):
        service.create_deck(slug="fresh", commander=["Gyome, Master Chef"],
                            source=MemoryDeckSource([]))


def test_search_by_text_matches_name_and_oracle_text(pool):
    by_name = service.search_cards(q="Gyome")
    assert any(c["name"] == "Gyome, Master Chef" for c in by_name["cards"])
    by_text = service.search_cards(q="Food")
    assert by_text["total"] >= 1


def test_cards_named_reports_misses_rather_than_omitting_them(pool):
    body = service.cards_named(names=["Gyome, Master Chef", "No Such Card"])
    assert [c["name"] for c in body["cards"]] == ["Gyome, Master Chef"]
    assert body["not_found"] == ["No Such Card"]


def test_the_job_registry_evicts_finished_jobs_over_its_bound(monkeypatch):
    """The eviction takes the oldest finished job and never a live one."""
    jobs.clear()
    monkeypatch.setattr(jobs, "MAX_JOBS", 3)
    live = jobs.submit("test", lambda p: __import__("time").sleep(2) or "ok")
    for i in range(5):
        jobs.completed("test", result=i, label=f"hit-{i}")
    with jobs._LOCK:
        held = list(jobs._JOBS.values())
    assert len(held) <= 4, "the bound must hold (live job + MAX_JOBS finished)"
    assert any(j.id == live.id for j in held), "a live job must never be evicted"


# ------------------------------------------------- the argue sweep (mass argue)
#
# `/api/decks/{owner}/{slug}/argue/deck` is the fourth planner of the
# themeruns/dossierruns shape: one Claude call per selected card means the
# sweep was never going to fit under the transport ceiling the single-card
# endpoint measures itself against, so it is a job -- one job for the whole
# selection, sequential inside the NET lane, with everything refusable
# refused in the request.


def test_an_empty_selection_is_a_422_and_not_a_job(in_memory_client):
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": []})
        assert r.status_code == 422
        assert jobs.all_jobs() == [], "nothing should have been queued"


def test_a_card_the_deck_does_not_hold_is_a_422_that_names_it(in_memory_client):
    """Named, not counted: the deck page sent the selection, so a miss means
    its list is stale, and "which card" is the actionable part."""
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring", "Island"]})
        assert r.status_code == 422
        assert "Island" in r.json()["detail"]
        assert jobs.all_jobs() == []


def test_a_malformed_stance_is_a_422_rather_than_a_sweep(in_memory_client):
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring"], "stance": "omniscient"})
        assert r.status_code == 422
        assert jobs.all_jobs() == []


def test_a_sweep_with_no_key_is_a_503_before_anything_is_queued(
        in_memory_client, monkeypatch):
    import mtglab.claude.client as cc

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: False)
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring"]})
        assert r.status_code == 503
        assert jobs.all_jobs() == []


def test_a_sweep_at_a_stance_of_off_is_a_job_born_finished(in_memory_client,
                                                           no_worker):
    """One report saying no calls were made -- never N copies of it, and never
    a queued job. `no_worker` is what makes the second half checkable rather
    than racy; see the fixture."""
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring", "Regal Behemoth"],
                              "stance": "off"})
        assert r.status_code == 200
        body = r.json()
        assert body["status"] == "done", "no worker needed to answer this"
        assert body["kind"] == "claude.argue.deck"
        assert body["result"]["asked"] is False
        assert "stance is off" in body["result"]["reason"]
        assert body["result"]["reports"] == []


def test_a_sweep_is_one_job_in_the_net_lane_reporting_progress(
        in_memory_client, monkeypatch):
    """One job for the selection, not one per card: the NET lane is two
    workers wide and shared with the dossier and the theme proposal, and a
    selection submitted as N jobs would starve every sibling surface."""
    import mtglab.claude.client as cc
    from mtglab.api import service as service_mod

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(
        service_mod, "claude_argue",
        lambda *, slug, card, requested=None, focus="", source=None, tier=None:
            {"card": card, "asked": True})

    lanes = []
    real_submit = jobs.submit
    monkeypatch.setattr(jobs, "submit",
                        lambda *a, **kw: (lanes.append(kw.get("lane")),
                                          real_submit(*a, **kw))[1])

    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring", "Regal Behemoth"]})
        assert r.status_code == 200
        assert lanes == [jobs.NET], "one job, in the NET lane"

        body = await_job(client, r.json()["id"])
        assert body["status"] == "done"
        result = body["result"]
        # The reports come back in selection order, one per card.
        assert [rep["card"] for rep in result["reports"]] == [
            "Sol Ring", "Regal Behemoth"]
        assert result["errors"] == {}
        assert body["total"] == 2 and body["done"] == 2


def test_one_failed_card_does_not_cost_the_rest_of_the_sweep(
        in_memory_client, monkeypatch):
    """Partial results are the point of paying for a sweep. A single flaky
    call is recorded against its card and the sweep continues."""
    import mtglab.claude.client as cc
    from mtglab.api import service as service_mod
    from mtglab.api.service import ClaudeFailed

    def flaky(*, slug, card, requested=None, focus="", source=None, tier=None):
        if card == "Sol Ring":
            raise ClaudeFailed("the model was rate limited")
        return {"card": card, "asked": True}

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(service_mod, "claude_argue", flaky)

    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring", "Regal Behemoth"]})
        body = await_job(client, r.json()["id"])
        assert body["status"] == "done", "one bad call is not a failed sweep"
        result = body["result"]
        assert [rep["card"] for rep in result["reports"]] == ["Regal Behemoth"]
        assert "rate limited" in result["errors"]["Sol Ring"]


def test_the_same_selection_twice_in_flight_is_one_sweep(in_memory_client,
                                                         monkeypatch):
    """The dossier's argument, per selection: a double-click inside the
    minutes a sweep takes is one question asked twice. A different order or
    case is still the same selection; a different selection is its own job."""
    import mtglab.claude.client as cc
    from mtglab.api import service as service_mod

    started = []

    def slow(*, slug, card, requested=None, focus="", source=None, tier=None):
        started.append(card)
        time.sleep(0.3)
        return {"card": card, "asked": True}

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(service_mod, "claude_argue", slow)

    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        first = client.post("/api/decks/local/mono-green/argue/deck",
                            json={"cards": ["Sol Ring", "Regal Behemoth"]}).json()
        second = client.post("/api/decks/local/mono-green/argue/deck",
                             json={"cards": ["regal behemoth", "SOL RING"]}).json()
        other = client.post("/api/decks/local/mono-green/argue/deck",
                            json={"cards": ["Sol Ring"]}).json()

        assert first["id"] == second["id"], "a second click must join the sweep"
        assert other["id"] != first["id"], "a different selection is new work"
        await_job(client, first["id"])
        await_job(client, other["id"])
        assert started.count("Regal Behemoth") == 1, \
            "the sweep must not have been paid for twice"


@pytest.mark.parametrize("cards", [None, "Sol Ring", ["Sol Ring", 7], {}])
def test_a_selection_that_is_not_a_list_of_names_is_a_422(in_memory_client,
                                                          cards):
    """The shape check runs before the deck is even loaded. A payload the
    client could not have sent is still a payload the endpoint is public to,
    and `cards` is what the whole plan is built out of."""
    jobs.clear()
    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": cards})
        assert r.status_code == 422
        assert jobs.all_jobs() == [], "nothing should have been queued"


def test_the_credential_going_away_stops_the_sweep_rather_than_burning_it(
        in_memory_client, monkeypatch):
    """The one failure that is not per-card. Every remaining slot would fail
    the same way, so they are marked unattempted and the sweep stops -- and it
    stops as `done`, because the partial reports are the thing that was paid
    for."""
    import mtglab.claude.client as cc
    from mtglab.api import service as service_mod

    def vanishes(*, slug, card, requested=None, focus="", source=None,
                 tier=None):
        if card == "Sol Ring":
            return {"card": card, "asked": True}
        raise cc.ClaudeUnavailable("no credential")

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(service_mod, "claude_argue", vanishes)

    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post(
            "/api/decks/local/mono-green/argue/deck",
            json={"cards": ["Sol Ring", "Regal Behemoth",
                              "Vorinclex, Voice of Hunger"]})
        body = await_job(client, r.json()["id"])

        assert body["status"] == "done"
        result = body["result"]
        assert [rep["card"] for rep in result["reports"]] == ["Sol Ring"]
        # The one that failed says why; the one after it says it was never
        # tried, which is a different thing to tell somebody.
        assert "Regal Behemoth" in result["errors"]
        assert result["errors"]["Vorinclex, Voice of Hunger"] == \
            "not attempted: the credential went away"
        # Reported as finished rather than left mid-bar: the sweep stopped on
        # purpose, and a progress bar frozen at 1 of 3 reads as a hang.
        assert body["done"] == body["total"] == 3


def test_a_surprise_from_one_card_is_recorded_like_any_other_failure(
        in_memory_client, monkeypatch):
    """Not every failure is one the mode declared. An unexpected exception is
    still one card's problem, and the sweep owes the other slots their
    answers -- the same treatment `ClaudeFailed` gets, which is what the bare
    `except` is there to guarantee."""
    import mtglab.claude.client as cc
    from mtglab.api import service as service_mod

    def surprising(*, slug, card, requested=None, focus="", source=None,
                   tier=None):
        if card == "Sol Ring":
            raise RuntimeError("something nobody predicted")
        return {"card": card, "asked": True}

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: True)
    monkeypatch.setattr(cc, "sdk_installed", lambda: True)
    monkeypatch.setattr(service_mod, "claude_argue", surprising)

    with in_memory_client([tiny_pool.mono_green_deck()]) as client:
        r = client.post("/api/decks/local/mono-green/argue/deck",
                        json={"cards": ["Sol Ring", "Regal Behemoth"]})
        body = await_job(client, r.json()["id"])

        assert body["status"] == "done", "one surprise is not a failed sweep"
        result = body["result"]
        assert [rep["card"] for rep in result["reports"]] == ["Regal Behemoth"]
        assert result["errors"]["Sol Ring"]


# ------------------------------------------------------------------ the wheel

def test_wheel_route_spins_and_a_seed_replays(pool, client):
    """Punch list item 9: the spin is the server's, seeded like Tier 1."""
    first = client.post("/api/decks/local/mono-green/wheel",
                        json={"seed": 5})
    assert first.status_code == 200, first.text
    body = first.json()
    assert body["pool_available"] is True
    assert body["symbol"] in ("cup", "heart", "sword", "skull")
    assert body["seed"] == 5
    assert body["answered_by"] == "python"
    again = client.post("/api/decks/local/mono-green/wheel",
                        json={"seed": 5}).json()
    assert again == body


def test_wheel_route_rolls_its_own_seed_when_unseeded(pool, client):
    body = client.post("/api/decks/local/mono-green/wheel", json={}).json()
    assert isinstance(body["seed"], int)


def test_wheel_on_a_missing_deck_is_404(pool, client):
    assert client.post("/api/decks/local/nope/wheel",
                       json={}).status_code == 404

# ------------------------------------------------------------ claude scan

def test_the_scan_route_reaches_no_deck(client):
    """ADR 34 rides on ADR 26's shape: the camera's fallback has no deck to
    critique, and it is mounted where that stays visible."""
    paths = {r.path for r in client.app.routes}
    assert "/api/claude/scan" in paths
    assert not any("scan" in p and "decks" in p for p in paths)


@pytest.mark.parametrize("payload, expected", [
    ({}, "empty"),
    ({"image": ""}, "empty"),
    ({"image": "///", "media_type": "image/tiff"}, "not an image"),
    ({"image": "not base64 at all!!"}, "base64"),
])
def test_an_unusable_capture_is_a_422_and_not_a_job(client, payload, expected):
    """Refused in the request. Delivered as a job in state `error` these would
    be four cases sharing one string and no status code."""
    jobs.clear()
    r = client.post("/api/claude/scan", json=payload)
    assert r.status_code == 422
    assert expected in r.json()["detail"]
    assert jobs.all_jobs() == [], "nothing should have been queued"


def test_an_oversized_capture_says_what_to_do_about_it(client):
    """'photograph one card, closer' is actionable; a platform 400 is not."""
    from mtglab.claude import scan

    jobs.clear()
    huge = base64.b64encode(b"x" * (scan.MAX_BYTES + 1)).decode()
    r = client.post("/api/claude/scan", json={"image": huge})
    assert r.status_code == 422
    assert "closer" in r.json()["detail"]
    assert jobs.all_jobs() == []


def test_scanning_with_no_key_is_a_503_before_anything_is_queued(client,
                                                                monkeypatch):
    """Not the same answer as a call that came back unusable."""
    import mtglab.claude.client as cc

    jobs.clear()
    monkeypatch.setattr(cc, "credential_present", lambda: False)
    r = client.post("/api/claude/scan",
                    json={"image": base64.b64encode(b"\xff\xd8\xff").decode()})
    assert r.status_code == 503
    assert jobs.all_jobs() == []


def test_two_presses_on_one_photograph_are_one_job(client, no_worker,
                                                   monkeypatch):
    """A digest of the image is the dedupe key, so a double click does not pay
    twice. The `key` is what carries this — see `api/scanruns.py`."""
    import mtglab.claude.client as cc
    from mtglab.api.scanruns import plan_scan

    monkeypatch.setattr(cc, "require", lambda: None)
    shot = base64.b64encode(b"\xff\xd8\xff\xe0 a card").decode()
    first = plan_scan(image=shot)
    again = plan_scan(image=shot)
    other = plan_scan(image=base64.b64encode(b"\xff\xd8\xff\xe0 another").decode())
    assert first.key == again.key
    assert first.key != other.key



# -------------------------------------------------------------- artifacts
#
# The hosted half of `mtglab decks build`. These run against an in-memory
# source for the reason the swap tests do -- a build writes files, and the
# seam exists so it does not write them into anybody's library.

BUILD = "/api/decks/local/mono-green/artifacts"


def test_a_deck_that_has_never_been_built_says_so(clean_client):
    with clean_client as client:
        body = client.get(BUILD).json()
    assert body["artifacts"] == []
    # Not `stale`: nothing has been built, so there is nothing to be stale.
    assert body["baseline"] == "unknown"
    assert body["buildable"] is True


def test_building_produces_the_deliverables_and_a_current_baseline(clean_client):
    with clean_client as client:
        assert client.post(BUILD, json={}).status_code == 200
        body = client.get(BUILD).json()
    assert [a["name"] for a in body["artifacts"]] == [
        "primer-quick.md", "primer-advanced.md",
        "decklist-annotated.md", "moxfield.txt"]
    # No `swaps.md` on a first build: there was nothing to diff against.
    assert body["baseline"] == "current"


def test_an_edited_deck_reports_its_artifacts_as_different(clean_client):
    with clean_client as client:
        client.post(BUILD, json={})
        assert client.get(BUILD).json()["baseline"] == "current"
        # Any edit through the ordinary route, so the baseline moves the way
        # it would in life rather than by reaching into the source.
        client.patch("/api/decks/local/mono-green",
                     json={"field": "bracket", "value": 3})
        assert client.get(BUILD).json()["baseline"] == "different"
        # ...and a rebuild now has a baseline, so it writes a swap list.
        client.post(BUILD, json={})
        body = client.get(BUILD).json()
    assert "swaps.md" in [a["name"] for a in body["artifacts"]]
    assert body["baseline"] == "current"


@pytest.fixture
def pool_less_client(in_memory_client, tmp_path):
    """The clean deck on an instance with no card pool at all.

    `data_dir` is pointed at an empty directory rather than simply left alone.
    `config.DB_PATH` defaults into the repository, where this machine's own
    500MB pool lives, so a fixture that merely omitted `pool` would find one
    anyway -- passing here while asserting the opposite of what CI sees. It is
    the trap `test_combination_detail_reports_no_pool_rather_than_failing`
    already names.
    """
    with config.use_paths(data_dir=tmp_path / "no-pool"), \
            in_memory_client([tiny_pool.mono_green_deck(clean=True)]) as c:
        yield c


def test_with_no_pool_every_surface_says_the_same_thing_about_one_deck(
        pool_less_client):
    """A fresh instance, before `mtglab data refresh`, and one deck's verdict.

    **This is the assertion nothing made until 2026-08-22, and all three
    surfaces disagreed.** `service._pool_for` answered `{}` for a missing
    connection where `cli._pool` answered `None`, and the gate reads those
    differently on purpose: `None` is "the pool was never consulted", one
    `unverified` warning and stop, while `{}` is a pool that has never heard of
    any of these cards and every one of them comes back `unknown-card`. Only
    `validate_deck` guarded, so on a pool-less instance this deck was
    simultaneously clean (validate), unbuildable (`BuildRejected`, 99 errors)
    and failing (`ok: false` on every edit, from `_commit`).

    `mtglab decks build` was right throughout, which is what makes it a bug
    rather than a preference: rule 3 says the CLI and the deck page run the
    same `render_all` and so cannot produce different files, and here one built
    and the other refused.

    The three are compared to **each other** rather than to a copied literal,
    because agreeing is the property -- a future change that moves all three
    together is not this bug coming back.
    """
    client = pool_less_client
    verdict = client.get("/api/decks/local/mono-green/validate").json()
    assert verdict["ok"] is True
    assert verdict["errors"] == []
    assert [w["code"] for w in verdict["warnings"]] == ["unverified"]

    built = client.post(BUILD, json={})
    assert built.status_code == 200, built.text
    # Unforced: there is nothing to override, which is the whole of the fix.
    assert built.json()["forced"] is False
    assert built.json()["issues"]["errors"] == verdict["errors"]
    assert built.json()["issues"]["warnings"] == verdict["warnings"]

    edited = client.patch("/api/decks/local/mono-green",
                          json={"field": "bracket", "value": 3})
    assert edited.status_code == 200, edited.text
    assert edited.json()["ok"] is True
    assert edited.json()["errors"] == verdict["errors"]
    assert edited.json()["warnings"] == verdict["warnings"]


def test_a_deliverable_is_served_verbatim(clean_client):
    with clean_client as client:
        client.post(BUILD, json={})
        r = client.get(f"{BUILD}/moxfield.txt")
    assert r.status_code == 200
    assert r.json()["name"] == "moxfield.txt"
    assert "Forest" in r.json()["text"]


@pytest.mark.parametrize("name", [
    "../../../etc/passwd", "../deck.yaml", "deck.yaml",
    "deck.last-built.yaml", "swaps.md", "nonsense.md",
])
def test_only_the_deliverables_are_reachable_by_name(clean_client, name):
    """The snapshot and a traversal are the same answer: there is no such
    artifact. Note `swaps.md` is in here — a real deliverable this build did
    not produce is a 404 too, and for the reader that is the same fact."""
    with clean_client as client:
        client.post(BUILD, json={})
        assert client.get(f"{BUILD}/{name}").status_code == 404


def test_the_gate_refuses_a_build_and_force_overrides_it(swappable):
    """`swappable`'s deck holds one banned card, which is Goreclaw's live
    situation: a theoretical deck whose author knows about the Titan."""
    with swappable as client:
        refused = client.post(BUILD, json={})
        assert refused.status_code == 422
        assert "gate" in refused.json()["detail"]
        assert client.get(BUILD).json()["artifacts"] == []

        forced = client.post(BUILD, json={"force": True})
        assert forced.status_code == 200
        assert forced.json()["forced"] is True
        assert forced.json()["issues"]["errors"]
        assert client.get(BUILD).json()["artifacts"]


def test_a_draft_is_refused_and_force_does_not_reach_it(in_memory_client, pool):
    """ADR 13: the way out of a draft is to write the rationales, not a flag.
    The CLI has no `--force` for this either, and the refusal lives in
    `render_all` so both callers inherit it."""
    draft = tiny_pool.mono_green_deck(clean=True)
    draft.stage = "draft"
    draft.cards[0].why = ""
    with in_memory_client([draft]) as client:
        assert client.get(BUILD).json()["buildable"] is False
        for payload in ({}, {"force": True}):
            r = client.post(BUILD, json=payload)
            assert r.status_code == 422
            assert "draft" in r.json()["detail"]
        assert client.get(BUILD).json()["artifacts"] == []


def test_a_reader_may_see_the_artifacts_but_not_rebuild_them(pool):
    """The artifacts are the shareable surface, so reading follows the deck.
    Building is a write and answers 403 like every other one."""
    source = MemoryDeckSource([tiny_pool.mono_green_deck(clean=True)])
    source.write_artifacts("mono-green", {"moxfield.txt": "1 Forest\n"})

    read_only = MemoryDeckSource(source.all(), writable=False)
    read_only._artifacts = source._artifacts
    app = create_app()
    app.dependency_overrides[deck_source] = lambda: read_only
    app.dependency_overrides[library] = lambda: library_over(read_only)
    with TestClient(app) as client:
        assert client.get(BUILD).status_code == 200
        assert client.get(f"{BUILD}/moxfield.txt").status_code == 200
        assert client.post(BUILD, json={}).status_code == 403


def test_artifacts_of_a_deck_this_scope_never_had_are_a_404(clean_client):
    with clean_client as client:
        assert client.get("/api/decks/local/ghost/artifacts").status_code == 404
        assert client.post("/api/decks/local/ghost/artifacts",
                           json={}).status_code == 404
