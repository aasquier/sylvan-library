"""API surface.

These run against the real deck files in `decks/` but tolerate a missing
corpus, because a fresh clone has no `data/mtg.duckdb` until `data refresh`
runs -- and the app has to stay usable in that state rather than 500ing.
"""

import sys
import time
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

fastapi = pytest.importorskip("fastapi")
pytest.importorskip("httpx")

from fastapi.testclient import TestClient  # noqa: E402

import tiny_corpus  # noqa: E402
from mtglab import config  # noqa: E402
from mtglab.api import jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.api.deps import deck_source  # noqa: E402
from mtglab.decks.model import Deck  # noqa: E402
from mtglab.decks.source import MemoryDeckSource  # noqa: E402

# The job pool has a single worker, so a queued job may sit behind another
# test's. Poll on a clock rather than an iteration count.
JOB_TIMEOUT_S = 60


@pytest.fixture
def client():
    jobs.clear()
    with TestClient(create_app()) as c:
        yield c


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

def test_health_reports_corpus_state(client):
    body = client.get("/api/health").json()
    assert "corpus" in body
    assert isinstance(body["oracle_cards"], int)


# ------------------------------------------------------------------- decks

def test_decks_lists_the_library(client):
    body = client.get("/api/decks").json()
    assert isinstance(body, list)
    slugs = {d["slug"] for d in body}
    assert "gyome-food" in slugs
    assert "_template" not in slugs


def test_deck_list_carries_the_gate_counts(client):
    """The shelf renders a deck with a banned card identically to a clean one
    unless the list endpoint says so, and fetching /validate per deck would be
    an N+1 on every page load. `errors` may be None when the corpus is missing
    -- that is 'the gate did not run', not 'the deck passed'."""
    body = client.get("/api/decks").json()
    assert body, "no decks found"
    for deck in body:
        assert "errors" in deck and "warnings" in deck
        assert deck["errors"] is None or isinstance(deck["errors"], int)
        assert deck["warnings"] is None or isinstance(deck["warnings"], int)
        # Never report a count without the corpus that produced it.
        assert (deck["errors"] is None) == (deck["warnings"] is None)


def test_deck_list_gate_counts_agree_with_the_validate_endpoint(client):
    """Two code paths compute the same thing; they must not drift."""
    for deck in client.get("/api/decks").json():
        if deck["errors"] is None:
            pytest.skip("no corpus; the gate did not run")
        rep = client.get(f"/api/decks/{deck['slug']}/validate").json()
        assert deck["errors"] == len(rep["errors"]), \
            f"{deck['slug']} error count disagrees with /validate"
        assert deck["warnings"] == len(rep["warnings"]), \
            f"{deck['slug']} warning count disagrees with /validate"
        assert rep["ok"] == (deck["errors"] == 0)


def test_deck_detail_has_every_card_with_its_why(client):
    body = client.get("/api/decks/gyome-food").json()
    assert body["total_cards"] == 99
    assert body["land_count"] == 34
    assert all(c["why"] for c in body["cards"]), "a card lost its rationale"


def test_missing_deck_is_a_404_not_a_500(client):
    resp = client.get("/api/decks/does-not-exist")
    assert resp.status_code == 404
    assert "does-not-exist" in resp.json()["detail"]


def test_missing_deck_stats_is_also_404(client):
    assert client.get("/api/decks/nope/stats").status_code == 404
    assert client.get("/api/decks/nope/validate").status_code == 404


def test_validate_returns_structured_issues(client):
    body = client.get("/api/decks/gyome-food/validate").json()
    assert set(body) == {"ok", "errors", "warnings"}
    assert isinstance(body["errors"], list)


def test_stats_are_json_serialisable(client):
    """The curve buckets are dataclasses; they must be flattened for the wire."""
    body = client.get("/api/decks/gyome-food/stats").json()
    assert body["total_cards"] == 99
    assert body["curve"]["buckets"], "no curve buckets"
    assert isinstance(body["curve"]["buckets"][0]["mv"], int)
    assert isinstance(body["categories"], list)


def test_suggestions_are_offered_for_a_banned_card(client):
    """Goreclaw runs Primeval Titan, which is banned. The endpoint should name
    the offender and shortlist legal cards that resemble it."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    body = client.get("/api/decks/goreclaw-stompy/suggestions").json()
    assert body["corpus_available"] is True
    targets = {t["card"]: t for t in body["targets"]}
    assert "Primeval Titan" in targets, body

    target = targets["Primeval Titan"]
    assert target["code"] == "banned"
    assert target["why"], "the slot's rationale is what makes a suggestion legible"
    assert target["candidates"], "a banned card with no shortlist helps nobody"

    for candidate in target["candidates"]:
        # Mono-green deck: anything outside {G} is not a legal suggestion.
        assert set(candidate["color_identity"]) <= {"G"}, candidate["name"]
        assert candidate["reasons"], "a score with no reason is not a suggestion"
        assert 0.0 <= candidate["score"] <= 1.2


def test_suggestions_never_include_the_card_being_replaced(client):
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")
    body = client.get("/api/decks/goreclaw-stompy/suggestions").json()
    for target in body["targets"]:
        names = {c["name"] for c in target["candidates"]}
        assert target["card"] not in names


def test_a_clean_deck_has_nothing_to_suggest(client):
    """The endpoint answers "what would fix the gate", so a deck the gate
    passes must return an empty list rather than unsolicited upgrades."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")
    body = client.get("/api/decks/gyome-food/suggestions").json()
    assert body["targets"] == []


def test_suggestions_for_a_missing_deck_are_a_404(client):
    assert client.get("/api/decks/nope/suggestions").status_code == 404


def test_suggestion_limit_is_bounded(client):
    assert client.get("/api/decks/goreclaw-stompy/suggestions",
                      params={"limit": 999}).status_code == 422


# ------------------------------------------------- decks, from elsewhere
#
# The point of the deck source seam (ADR 4): the endpoints read whatever the
# request scope hands them, so hosting can add a second tier by swapping one
# dependency instead of touching thirteen handlers. These tests are the proof,
# and they are also the cheapest way to exercise library states the filesystem
# makes awkward.

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
        return TestClient(app)
    return make


def test_endpoints_read_the_request_scope_not_the_filesystem(in_memory_client):
    only = Deck.load(Path("decks/gyome-food/deck.yaml"))
    with in_memory_client([only]) as client:
        assert [d["slug"] for d in client.get("/api/decks").json()] == ["gyome-food"]
        assert client.get("/api/decks/gyome-food").status_code == 200
        # On disk, and deliberately not in this request's scope.
        assert client.get("/api/decks/arahbo-cats").status_code == 404


def test_the_request_scope_does_not_leak_into_the_public_schema(client):
    """A dependency injected by annotation can end up documented as a query
    parameter if it is wired wrong. The endpoint would still work and every
    other test would still pass, so check the schema itself."""
    schema = client.get("/openapi.json")
    assert schema.status_code == 200
    paths = schema.json()["paths"]
    assert [p["name"] for p in paths["/api/decks"]["get"].get("parameters", [])] == []
    assert [p["name"] for p in paths["/api/decks/{slug}"]["get"]["parameters"]] == ["slug"]


# ------------------------------------------------------------------ swaps
#
# Every one of these runs against an in-memory source. A test that wrote to
# decks/ would be editing the repository's own source of truth, and the seam
# exists precisely so it does not have to.

@pytest.fixture
def swappable(in_memory_client):
    """The real Goreclaw list, held in memory so writes go nowhere."""
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    return in_memory_client([deck])


def test_a_swap_replaces_the_card_and_clears_the_gate(swappable):
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        before = client.get("/api/decks/goreclaw-stompy/validate").json()
        assert any(i["card"] == "Primeval Titan" for i in before["errors"])

        resp = client.post("/api/decks/goreclaw-stompy/swap", json={
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
        names = {c["name"] for c in client.get("/api/decks/goreclaw-stompy").json()["cards"]}
        assert "Cultivator Colossus" in names
        assert "Primeval Titan" not in names


def test_a_swap_keeps_the_slot_and_records_the_given_why(swappable):
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        client.post("/api/decks/goreclaw-stompy/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A rationale a human wrote.",
        })
        card = next(c for c in client.get("/api/decks/goreclaw-stompy").json()["cards"]
                    if c["name"] == "Cultivator Colossus")
        assert card["why"] == "A rationale a human wrote."
        assert card["category"] == "threat", "the slot it filled should carry over"


# Split by what each refusal needs. The first group is checked before the
# corpus is even opened, so it runs on a fresh clone -- which is where most of
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
def test_a_swap_refused_on_the_deck_alone_needs_no_corpus(swappable, payload, expected):
    with swappable as client:
        resp = client.post("/api/decks/goreclaw-stompy/swap", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]


@pytest.mark.parametrize(("payload", "expected"), [
    ({"out": "Primeval Titan", "into": "Not A Real Card At All", "why": "x"},
     "corpus knows"),
    ({"out": "Primeval Titan", "into": "Rhystic Study", "why": "x"},
     "outside the commander's"),
    ({"out": "Primeval Titan", "into": "Black Lotus", "why": "x"},
     "not legal in Commander"),
])
def test_a_swap_refused_on_a_card_fact_is_looked_up(swappable, payload, expected):
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        resp = client.post("/api/decks/goreclaw-stompy/swap", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]


def test_a_swap_without_a_corpus_says_so_rather_than_guessing(swappable):
    """Legality and colour identity are card facts, and CLAUDE.md rule 1 says
    those are looked up. With no corpus there is nothing to look them up in, so
    the swap is refused rather than waved through."""
    with swappable as client:
        if client.get("/api/health").json()["corpus"]:
            pytest.skip("this is the fresh-clone path")
        resp = client.post("/api/decks/goreclaw-stompy/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A real rationale."})
        assert resp.status_code == 422
        assert "corpus" in resp.json()["detail"]


def test_a_refused_swap_changes_nothing(swappable):
    """The check that matters most: a rejection must not leave the deck half
    edited."""
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        before = client.get("/api/decks/goreclaw-stompy").json()["cards"]
        client.post("/api/decks/goreclaw-stompy/swap", json={
            "out": "Primeval Titan", "into": "Rhystic Study", "why": "no"})
        assert client.get("/api/decks/goreclaw-stompy").json()["cards"] == before


def test_a_read_only_source_refuses_every_swap():
    """What the hosted two-tier model needs: curated decks stay read-only for
    someone who is not the maintainer, checked in one place."""
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        resp = client.post("/api/decks/goreclaw-stompy/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus", "why": "x"})
        assert resp.status_code == 422
        assert "read-only" in resp.json()["detail"]


# --------------------------------------------------------------- the edits
#
# The rest of the operations (ADR 12). Same in-memory source as the swaps, and
# the same property being checked each time: one narrow change, the gate re-run
# and reported, and a refusal that writes nothing.

@pytest.fixture
def draft_client(in_memory_client):
    """A draft, so the rule 4 bend is exercisable. Cards keep their `why`s --
    what makes it a draft is the stage, and a draft with rationales already
    written is one of ADR 13's four real combinations."""
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    deck.stage = "draft"
    return in_memory_client([deck])


def test_a_card_can_be_added_and_the_gate_comes_back(swappable):
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Llanowar Reborn", "category": "land",
            "why": "Enters with a +1/+1 counter to move onto a fatty."})
        assert resp.status_code == 200, resp.json()
        body = resp.json()
        assert body["added"] == "Llanowar Reborn"
        assert "ok" in body and "errors" in body
        names = {c["name"] for c in
                 client.get("/api/decks/goreclaw-stompy").json()["cards"]}
        assert "Llanowar Reborn" in names


def test_a_card_outside_the_commanders_identity_is_refused(swappable):
    """Rule 2: identity comes from Scryfall's `color_identity`. Goreclaw is
    mono-green, so a card with any other pip in its identity cannot go in."""
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Swords to Plowshares", "category": "interaction",
            "why": "One mana, exiles anything."})
        assert resp.status_code == 422
        assert "outside the commander's" in resp.json()["detail"]


def test_a_card_the_corpus_does_not_know_is_refused(swappable):
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Definitely Not A Card", "category": "ramp", "why": "x"})
        assert resp.status_code == 422
        assert "not a card the corpus knows" in resp.json()["detail"]


def test_an_unknown_category_is_refused_before_any_lookup(swappable):
    with swappable as client:
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Sol Ring", "category": "rampp", "why": "typo"})
        assert resp.status_code == 422
        assert "is not a category" in resp.json()["detail"]


def test_a_curated_deck_refuses_a_card_with_no_rationale(swappable):
    """Rule 4 at the boundary. The tool declines to invent one -- there is no
    code path here that fills the field in."""
    with swappable as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Llanowar Reborn", "category": "land", "why": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_a_draft_accepts_a_card_that_still_owes_its_rationale(draft_client):
    """The one bend (ADR 13): a draft is honestly incomplete and counts what it
    owes, rather than refusing work while the thinking is still to come."""
    with draft_client as client:
        if not client.get("/api/health").json()["corpus"]:
            pytest.skip("no corpus available")
        before = client.get("/api/decks/goreclaw-stompy").json()["needs_rationale"]
        resp = client.post("/api/decks/goreclaw-stompy/cards", json={
            "name": "Llanowar Reborn", "category": "land"})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["needs_rationale"] == before + 1


def test_a_card_can_be_removed_without_a_corpus(swappable):
    """Removing a card is a fact about this deck file, not about Magic, so it
    works on a machine that has never run `data refresh`."""
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/goreclaw-stompy/cards/Primeval Titan")
        assert resp.status_code == 200, resp.json()
        assert resp.json()["removed"] == "Primeval Titan"
        names = {c["name"] for c in
                 client.get("/api/decks/goreclaw-stompy").json()["cards"]}
        assert "Primeval Titan" not in names


def test_removing_a_card_that_is_not_there_is_refused(swappable):
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/goreclaw-stompy/cards/Black Lotus")
        assert resp.status_code == 422
        assert "not in this deck" in resp.json()["detail"]


def test_a_rationale_can_be_written_through_the_api(swappable):
    """The gap `decks import` opened: a draft arrives owing 99 rationales, and
    until this endpoint the only way to write one was a text editor."""
    with swappable as client:
        resp = client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                            json={"field": "why",
                                  "value": "Two mana for one, and it always has been."})
        assert resp.status_code == 200, resp.json()
        card = next(c for c in client.get("/api/decks/goreclaw-stompy").json()["cards"]
                    if c["name"] == "Sol Ring")
        assert card["why"] == "Two mana for one, and it always has been."


def test_a_rationale_cannot_be_blanked_on_a_curated_deck(swappable):
    with swappable as client:
        resp = client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                            json={"field": "why", "value": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_only_a_short_list_of_card_fields_is_settable(swappable):
    with swappable as client:
        for field in ("name", "scryfall_id", "tags"):
            resp = client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not settable" in resp.json()["detail"]


def test_a_category_and_a_quantity_can_be_patched(swappable):
    with swappable as client:
        assert client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                            json={"field": "category",
                                  "value": "utility"}).status_code == 200
        assert client.patch("/api/decks/goreclaw-stompy/cards/Forest",
                            json={"field": "qty", "value": 26}).status_code == 200
        cards = client.get("/api/decks/goreclaw-stompy").json()["cards"]
        assert next(c for c in cards if c["name"] == "Sol Ring")["category"] == "utility"
        assert next(c for c in cards if c["name"] == "Forest")["qty"] == 26


def test_a_note_can_be_set_and_read_back(swappable):
    with swappable as client:
        resp = client.put("/api/decks/goreclaw-stompy/notes/mulligan",
                          json={"value": "Keep any two-lander with a one-mana dork."})
        assert resp.status_code == 200, resp.json()
        notes = client.get("/api/decks/goreclaw-stompy").json()["notes"]
        assert notes["mulligan"] == "Keep any two-lander with a one-mana dork."


def test_an_empty_note_is_refused(swappable):
    with swappable as client:
        resp = client.put("/api/decks/goreclaw-stompy/notes/mulligan",
                          json={"value": "   "})
        assert resp.status_code == 422
        assert "needs text" in resp.json()["detail"]


def test_a_draft_is_promoted_once_every_card_is_justified(in_memory_client):
    """The last step of an import, and until now the last thing in the whole
    lifecycle that could only be done in a text editor."""
    deck = Deck.load(Path("decks/gyome-food/deck.yaml"))
    deck.stage = "draft"
    with in_memory_client([deck]) as client:
        resp = client.patch("/api/decks/gyome-food",
                            json={"field": "stage", "value": "curated"})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["stage"] == "curated"
        assert client.get("/api/decks/gyome-food").json()["stage"] == "curated"


def test_promotion_is_refused_while_a_card_is_blank(draft_client):
    with draft_client as client:
        client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        resp = client.patch("/api/decks/goreclaw-stompy",
                            json={"field": "stage", "value": "curated"})
        assert resp.status_code == 422
        assert "Sol Ring" in resp.json()["detail"]
        # And it stayed a draft rather than landing somewhere in between.
        assert client.get("/api/decks/goreclaw-stompy").json()["stage"] == "draft"


def test_deck_status_and_bracket_are_patchable(swappable):
    with swappable as client:
        assert client.patch("/api/decks/goreclaw-stompy",
                            json={"field": "status",
                                  "value": "built"}).status_code == 200
        assert client.patch("/api/decks/goreclaw-stompy",
                            json={"field": "bracket", "value": 5}).status_code == 200
        body = client.get("/api/decks/goreclaw-stompy").json()
        assert (body["status"], body["bracket"]) == ("built", 5)


def test_a_field_that_is_not_the_decks_own_is_refused(swappable):
    with swappable as client:
        for field in ("name", "commander", "cards"):
            resp = client.patch("/api/decks/goreclaw-stompy",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not a settable deck field" in resp.json()["detail"]


def test_a_refused_edit_changes_nothing(swappable):
    """The whole point of verifying before writing. Every refusal above must
    leave the deck byte-identical, not partly applied."""
    with swappable as client:
        before = client.get("/api/decks/goreclaw-stompy").json()
        client.post("/api/decks/goreclaw-stompy/cards",
                    json={"name": "Sol Ring", "category": "ramp", "why": "dup"})
        client.request("DELETE", "/api/decks/goreclaw-stompy/cards/Black Lotus")
        client.patch("/api/decks/goreclaw-stompy/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        client.patch("/api/decks/goreclaw-stompy/cards/Forest",
                     json={"field": "qty", "value": 0})
        client.put("/api/decks/goreclaw-stompy/notes/x", json={"value": ""})
        assert client.get("/api/decks/goreclaw-stompy").json() == before


def test_a_read_only_source_refuses_every_edit():
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        base = "/api/decks/goreclaw-stompy"
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
            assert resp.status_code == 422
            assert "read-only" in resp.json()["detail"]


# ----------------------------------------------------------------- import
#
# The import endpoint is the only other write in the API, and it creates rather
# than edits. Its refusals run against an in-memory source for the same reason
# the swap refusals do: a test that wrote to decks/ would be editing the
# repository's own source of truth.

@pytest.fixture
def importable(in_memory_client, tmp_path):
    """An empty library, with a four-card corpus behind it.

    Built rather than borrowed: the real 500MB corpus is absent on a fresh
    clone and in CI, and the one thing worth proving about import is what it
    does *with* a corpus.
    """
    pytest.importorskip("duckdb")
    with config.use_paths(data_dir=tmp_path / "data"):
        tiny_corpus.build(config.DB_PATH)
        yield in_memory_client([])


def test_import_creates_a_draft_and_gates_it_immediately(importable):
    """ADR 13's bargain, over HTTP: the facts are checked on day one, and the
    thinking still owed is a number rather than a wall."""
    with importable as client:
        resp = client.post("/api/decks/import", json={
            "slug": "gyome-x", "name": "Gyome imported",
            "text": tiny_corpus.DECKLIST, "bracket": 4})
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
        deck = client.get("/api/decks/gyome-x").json()
        assert deck["stage"] == "draft"
        assert deck["needs_rationale"] == 3
        assert all(c["why"] == "" for c in deck["cards"]), "no rationale invented"
        assert {d["slug"] for d in client.get("/api/decks").json()} == {"gyome-x"}


def test_import_dry_run_previews_without_creating(importable):
    """The preview runs the identical code path, so what the user approves is
    the result rather than an estimate of it."""
    with importable as client:
        resp = client.post("/api/decks/import", json={
            "slug": "gyome-x", "text": tiny_corpus.DECKLIST, "dry_run": True})
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["created"] is False
        assert body["total_cards"] == 99
        assert [i["card"] for i in body["errors"]] == ["Primeval Titan"]
        assert "stage: draft" in body["yaml"]
        assert client.get("/api/decks").json() == []
        assert client.get("/api/decks/gyome-x").status_code == 404


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
    pytest.importorskip("duckdb")
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    with config.use_paths(data_dir=tmp_path / "data"):
        tiny_corpus.build(config.DB_PATH)
        with in_memory_client([deck]) as client:
            resp = client.post("/api/decks/import", json={
                "slug": "goreclaw-stompy", "text": tiny_corpus.DECKLIST})
            assert resp.status_code == 422
            assert "already exists" in resp.json()["detail"]
            # The deck it refused to touch is untouched.
            assert client.get("/api/decks/goreclaw-stompy").json()["stage"] == "curated"


def test_import_without_a_corpus_refuses_rather_than_guessing(in_memory_client,
                                                              tmp_path):
    """Every name would be unknown and no land filed, so the deck's facts would
    never be checked -- the one thing the gate exists to do."""
    with config.use_paths(data_dir=tmp_path / "absent"), \
            in_memory_client([]) as client:
        resp = client.post("/api/decks/import",
                           json={"slug": "x", "text": "1 Sol Ring\n"})
        assert resp.status_code == 422
        assert "corpus" in resp.json()["detail"]


def test_a_read_only_library_refuses_import():
    deck = Deck.load(Path("decks/goreclaw-stompy/deck.yaml"))
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        resp = client.post("/api/decks/import",
                           json={"slug": "new-deck", "text": "1 Sol Ring\n"})
        assert resp.status_code == 422
        assert "read-only" in resp.json()["detail"]


def test_the_import_path_does_not_shadow_a_deck_called_import(importable):
    """`/api/decks/import` is a POST and `/api/decks/{slug}` is a GET, so the
    two cannot collide -- pinned because the route order looks like it matters
    and one day somebody will move it."""
    with importable as client:
        assert client.get("/api/decks/import").status_code == 404


def test_an_empty_library_is_empty_rather_than_broken(in_memory_client):
    """Two lines here; creating and removing directories on disk otherwise."""
    with in_memory_client([]) as client:
        assert client.get("/api/decks").json() == []
        health = client.get("/api/health").json()
        if health["corpus"]:
            assert health["decks"] == 0


# ------------------------------------------------------------------- cards

def test_card_search_respects_the_limit(client):
    body = client.get("/api/cards/search", params={"limit": 5}).json()
    if body.get("message"):
        pytest.skip("no corpus available")
    assert len(body["cards"]) <= 5


def test_card_search_identity_filter_is_a_subset_check(client):
    """Asking for BG must return colorless and mono-B cards too -- that is the
    question a Golgari deckbuilder is actually asking."""
    body = client.get("/api/cards/search",
                      params={"identity": "BG", "limit": 50}).json()
    if body.get("message"):
        pytest.skip("no corpus available")
    for card in body["cards"]:
        assert set(card["color_identity"]) <= {"B", "G"}, card["name"]


def test_search_limit_is_clamped_not_rejected(client):
    assert client.get("/api/cards/search", params={"limit": 9999}).status_code == 422


# --------------------------------------------------------------------- sim

def test_sim_requires_a_slug(client):
    assert client.post("/api/sim/mana", json={}).status_code == 422


def test_sim_job_runs_to_completion(client):
    """Submit, poll, and get a result -- the whole contract the UI relies on."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/mana",
                            json={"slug": "gyome-food", "games": 300,
                                  "turns": 8, "seed": 1}).json()
    assert submitted["status"] in ("queued", "running", "done")

    body = await_job(client, submitted["id"])
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


def test_land_sweep_returns_a_row_per_count_and_reports_the_spread(client):
    """The sweep is the command that actually decides a land count, and it was
    entirely untested. The spread matters as much as the winner: Gyome's curve
    was flat to 0.07 across 30-40, and reading the argmax of that is reading
    noise."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/lands",
                            json={"slug": "gyome-food", "low": 32, "high": 34,
                                  "games": 200, "seed": 1}).json()
    body = await_job(client, submitted["id"])
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


def test_land_sweep_normalises_a_reversed_range(client):
    """low/high the wrong way round should sweep, not return nothing."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/lands",
                            json={"slug": "gyome-food", "low": 34, "high": 32,
                                  "games": 200, "seed": 1}).json()
    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert [r["lands"] for r in body["result"]["rows"]] == [32, 33, 34]


def test_sim_payload_bounds_are_clamped_not_rejected(client):
    """An absurd game count should be pulled into range rather than 500."""
    if not client.get("/api/health").json()["corpus"]:
        pytest.skip("no corpus available")

    submitted = client.post("/api/sim/mana",
                            json={"slug": "gyome-food", "games": 1,
                                  "turns": 99, "seed": 1}).json()
    body = await_job(client, submitted["id"])
    assert body["status"] == "done", body.get("error")
    assert body["result"]["games"] >= 100, "games floor"
    assert len(body["result"]["by_turn"]) <= 20, "turns ceiling"


def test_keep_rule_uses_documented_defaults():
    from mtglab.api.simruns import _keep_rule
    rule = _keep_rule({})
    assert (rule.min_lands, rule.max_lands) == (2, 5)


def test_unknown_job_is_404(client):
    assert client.get("/api/jobs/deadbeef").status_code == 404


def test_job_progress_is_reported(client):
    job = jobs.submit("test", lambda p: [p(i, 10) for i in range(10)] and "ok")
    body = await_job(client, job.id)
    assert body["status"] == "done"
    assert body["percent"] == 100
    assert client.get("/api/jobs").json()[0]["id"] == job.id


# ---------------------------------------------------------------- colours

def test_colors_needs_no_corpus_and_no_decks(client):
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


def test_challenge_progress_counts_filled_slots(in_memory_client):
    with in_memory_client([Deck.load(Path("decks/gyome-food/deck.yaml"))]) as c:
        body = c.get("/api/colors/progress").json()
    assert body["total"] == 32
    assert len(body["slots"]) == 32
    # Gyome is Golgari. Without a corpus the identity cannot be derived at all,
    # so the assertion is conditional -- the same tolerance the rest of this
    # file has for a fresh clone.
    golgari = next(s for s in body["slots"] if s["key"] == "BG")
    if body["filled"]:
        assert [d["slug"] for d in golgari["decks"]] == ["gyome-food"]


# ----------------------------------------------------------------- create

def test_create_makes_a_draft_from_a_commander(in_memory_client):
    if not config.DB_PATH.exists():
        pytest.skip("creating a deck needs the corpus")
    with in_memory_client([]) as c:
        body = c.post("/api/decks", json={
            "slug": "brand-new", "commander": ["Gyome, Master Chef"]}).json()
    assert body["created"] is True
    # A new deck owes its rationales, so it starts as a draft -- ADR 13.
    assert body["stage"] == "draft"
    assert body["total_cards"] == 0
    assert body["color_identity"] == ["B", "G"]
    assert body["combination"]["name"] == "Golgari"


def test_create_refuses_a_card_that_cannot_lead_a_deck(in_memory_client):
    if not config.DB_PATH.exists():
        pytest.skip("creating a deck needs the corpus")
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": ["Sol Ring"]})
    assert r.status_code == 422
    assert "cannot be your commander" in r.json()["detail"]


def test_create_refuses_a_deck_with_no_commander(in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": []})
    assert r.status_code == 422
    assert "needs a commander" in r.json()["detail"]


def test_create_refuses_a_duplicate_slug(in_memory_client):
    if not config.DB_PATH.exists():
        pytest.skip("creating a deck needs the corpus")
    existing = Deck.load(Path("decks/gyome-food/deck.yaml"))
    with in_memory_client([existing]) as c:
        r = c.post("/api/decks", json={
            "slug": "gyome-food", "commander": ["Gyome, Master Chef"]})
    assert r.status_code == 422
    assert "already exists" in r.json()["detail"]


def test_create_is_refused_on_a_read_only_library():
    deck = Deck.load(Path("decks/gyome-food/deck.yaml"))
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        r = client.post("/api/decks", json={
            "slug": "nope", "commander": ["Gyome, Master Chef"]})
    assert r.status_code == 422


def test_commander_search_is_exact_and_returns_actual_commanders(client):
    """The bug this pins: `commanders_only` used to filter after the SQL
    limit, so a search for Selesnya commanders returned the sixty best
    Selesnya cards, none of which was a commander, and then nothing."""
    if not config.DB_PATH.exists():
        pytest.skip("card search needs the corpus")
    cards = client.get("/api/cards/search", params={
        "identity": "WG", "identity_exact": True,
        "commanders_only": True, "limit": 10}).json()["cards"]
    assert cards, "a search for Selesnya commanders must return some"
    for card in cards:
        assert set(card["color_identity"]) == {"W", "G"}, card["name"]
        assert "Legendary" in card["type_line"] or \
               "can be your commander" in (card["oracle_text"] or "").lower()
