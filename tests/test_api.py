"""API surface.

Two kinds of test live here, and the difference is deliberate.

The endpoints that read a deck file run against the **real** decks in `decks/`
and need no corpus, because a fresh clone has none until `data refresh` runs
and the app has to stay usable in that state rather than 500ing.

The endpoints that look a card up -- swap, add, suggestions, search, the Tier 1
jobs -- take the `corpus` fixture and the synthetic deck from `tiny_corpus`.
They used to run against the real decks too, gated on `data/mtg.duckdb`, which
meant they skipped on every pull request and passed only on the maintainer's
laptop. Card facts cannot be faked (rule 1) and the 500MB corpus cannot go in
CI (ADR 6), so the fixture is a real DuckDB corpus of 21 real cards with a
legal 99 built out of them.
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


@pytest.fixture
def corpus(tmp_path):
    """A real, queryable corpus -- built rather than borrowed.

    The card-fact endpoints (swap, add, suggestions, search) look every name
    up, so without this they were gated on `data/mtg.duckdb` being present.
    That file is a 500MB download nobody puts in CI, so 29 tests skipped on
    every pull request and passed only on the maintainer's laptop: the worst
    shape a test can have, because the green check was reporting on a suite
    five points of coverage smaller than the one being read.

    `tiny_corpus.build` produces a genuine DuckDB corpus in about a second.
    """
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_corpus.build(config.DB_PATH)


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


def test_deck_list_gate_counts_agree_with_the_validate_endpoint(corpus, client):
    """Two code paths compute the same thing; they must not drift.

    Takes `corpus` so the gate actually runs. What it reports does not matter
    here -- against a 21-card corpus the real decks are mostly unknown
    cards, and that is fine, because the subject is whether the two paths
    agree rather than whether any deck is clean.
    """
    for deck in client.get("/api/decks").json():
        assert deck["errors"] is not None, f"{deck['slug']}: the gate did not run"
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


def test_suggestions_are_offered_for_a_banned_card(swappable):
    """The fixture deck runs Primeval Titan, which is banned. The endpoint
    should name the offender and shortlist legal cards that resemble it."""
    with swappable as client:
        body = client.get("/api/decks/mono-green/suggestions").json()
        assert body["corpus_available"] is True
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
        body = client.get("/api/decks/mono-green/suggestions").json()
        for target in body["targets"]:
            names = {c["name"] for c in target["candidates"]}
            assert target["card"] not in names


def test_suggestions_never_include_a_card_already_in_the_deck(swappable):
    """Regal Behemoth and Vorinclex are in the list and would otherwise score
    well for the Titan's slot -- suggesting a card you already run is noise."""
    with swappable as client:
        body = client.get("/api/decks/mono-green/suggestions").json()
        held = {c["name"] for c in client.get("/api/decks/mono-green").json()["cards"]}
        for target in body["targets"]:
            assert not ({c["name"] for c in target["candidates"]} & held)


def test_a_clean_deck_has_nothing_to_suggest(clean_client):
    """The endpoint answers "what would fix the gate", so a deck the gate
    passes must return an empty list rather than unsolicited upgrades."""
    with clean_client as client:
        body = client.get("/api/decks/mono-green/suggestions").json()
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
def swappable(in_memory_client, corpus):
    """The fixture deck, held in memory so writes go nowhere, with a corpus
    behind it so card facts can actually be looked up.

    This was the real Goreclaw list until the corpus fixture existed. Same
    shape -- a mono-green commander, a legal 99, exactly one banned card --
    but built out of `tiny_corpus.CARDS`, so the gate has something real to
    catch without needing all 35,000 cards to find it.
    """
    return in_memory_client([tiny_corpus.mono_green_deck()])


@pytest.fixture
def clean_client(in_memory_client, corpus):
    """The same deck with a legal card in the Titan's slot, so "the gate found
    nothing" is testable as its own case rather than inferred."""
    return in_memory_client([tiny_corpus.mono_green_deck(clean=True)])


@pytest.fixture
def sim_client(in_memory_client, corpus):
    """A deck the simulator can actually compile.

    Tier 1 needs a card record for every slot, so these were pointed at
    `gyome-food` and the real corpus -- which is why the whole simulation
    surface, jobs and polling included, went untested in CI. The fixture deck
    compiles against `tiny_corpus`, so the job contract the UI depends on is
    now exercised on every pull request.

    A background job captures the `DeckSource` and outlives its request, which
    is fine here: `await_job` blocks inside the test, so `corpus` is still on
    the stack when the worker reads it.
    """
    jobs.clear()
    with in_memory_client([tiny_corpus.mono_green_deck(clean=True)]) as c:
        yield c


def test_a_swap_replaces_the_card_and_clears_the_gate(swappable):
    with swappable as client:
        before = client.get("/api/decks/mono-green/validate").json()
        assert any(i["card"] == "Primeval Titan" for i in before["errors"])

        resp = client.post("/api/decks/mono-green/swap", json={
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
        names = {c["name"] for c in client.get("/api/decks/mono-green").json()["cards"]}
        assert "Cultivator Colossus" in names
        assert "Primeval Titan" not in names


def test_a_swap_keeps_the_slot_and_records_the_given_why(swappable):
    with swappable as client:
        client.post("/api/decks/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A rationale a human wrote.",
        })
        card = next(c for c in client.get("/api/decks/mono-green").json()["cards"]
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
        resp = client.post("/api/decks/mono-green/swap", json=payload)
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
        resp = client.post("/api/decks/mono-green/swap", json=payload)
        assert resp.status_code == 422, resp.text
        assert expected in resp.json()["detail"]


def test_a_swap_without_a_corpus_says_so_rather_than_guessing(in_memory_client,
                                                              tmp_path):
    """Legality and colour identity are card facts, and CLAUDE.md rule 1 says
    those are looked up. With no corpus there is nothing to look them up in, so
    the swap is refused rather than waved through.

    Points at an empty directory rather than skipping when a corpus happens to
    be present. The fresh-clone path is a behaviour worth pinning, and it used
    to be tested only on machines that had never run `data refresh` -- which
    is to say, almost nowhere.
    """
    with config.use_paths(data_dir=tmp_path / "absent"), \
            in_memory_client([tiny_corpus.mono_green_deck()]) as client:
        assert client.get("/api/health").json()["corpus"] is False
        resp = client.post("/api/decks/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus",
            "why": "A real rationale."})
        assert resp.status_code == 422
        assert "corpus" in resp.json()["detail"]


def test_a_refused_swap_changes_nothing(swappable):
    """The check that matters most: a rejection must not leave the deck half
    edited."""
    with swappable as client:
        before = client.get("/api/decks/mono-green").json()["cards"]
        client.post("/api/decks/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Rhystic Study", "why": "no"})
        assert client.get("/api/decks/mono-green").json()["cards"] == before


def test_a_read_only_source_refuses_every_swap():
    """What the hosted two-tier model needs: curated decks stay read-only for
    someone who is not the maintainer, checked in one place."""
    deck = tiny_corpus.mono_green_deck()
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        resp = client.post("/api/decks/mono-green/swap", json={
            "out": "Primeval Titan", "into": "Cultivator Colossus", "why": "x"})
        assert resp.status_code == 422
        assert "read-only" in resp.json()["detail"]


# --------------------------------------------------------------- the edits
#
# The rest of the operations (ADR 12). Same in-memory source as the swaps, and
# the same property being checked each time: one narrow change, the gate re-run
# and reported, and a refusal that writes nothing.

@pytest.fixture
def draft_client(in_memory_client, corpus):
    """A draft, so the rule 4 bend is exercisable. Cards keep their `why`s --
    what makes it a draft is the stage, and a draft with rationales already
    written is one of ADR 13's four real combinations."""
    return in_memory_client([tiny_corpus.mono_green_deck(stage="draft")])


def test_a_card_can_be_added_and_the_gate_comes_back(swappable):
    with swappable as client:
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land",
            "why": "Enters with a +1/+1 counter to move onto a fatty."})
        assert resp.status_code == 200, resp.json()
        body = resp.json()
        assert body["added"] == "Llanowar Reborn"
        assert "ok" in body and "errors" in body
        names = {c["name"] for c in
                 client.get("/api/decks/mono-green").json()["cards"]}
        assert "Llanowar Reborn" in names


def test_a_card_outside_the_commanders_identity_is_refused(swappable):
    """Rule 2: identity comes from Scryfall's `color_identity`. Goreclaw is
    mono-green, so a card with any other pip in its identity cannot go in."""
    with swappable as client:
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Swords to Plowshares", "category": "interaction",
            "why": "One mana, exiles anything."})
        assert resp.status_code == 422
        assert "outside the commander's" in resp.json()["detail"]


def test_a_card_the_corpus_does_not_know_is_refused(swappable):
    with swappable as client:
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Definitely Not A Card", "category": "ramp", "why": "x"})
        assert resp.status_code == 422
        assert "not a card the corpus knows" in resp.json()["detail"]


def test_an_unknown_category_is_refused_before_any_lookup(swappable):
    with swappable as client:
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Sol Ring", "category": "rampp", "why": "typo"})
        assert resp.status_code == 422
        assert "is not a category" in resp.json()["detail"]


def test_a_curated_deck_refuses_a_card_with_no_rationale(swappable):
    """Rule 4 at the boundary. The tool declines to invent one -- there is no
    code path here that fills the field in."""
    with swappable as client:
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land", "why": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_a_draft_accepts_a_card_that_still_owes_its_rationale(draft_client):
    """The one bend (ADR 13): a draft is honestly incomplete and counts what it
    owes, rather than refusing work while the thinking is still to come."""
    with draft_client as client:
        before = client.get("/api/decks/mono-green").json()["needs_rationale"]
        resp = client.post("/api/decks/mono-green/cards", json={
            "name": "Llanowar Reborn", "category": "land"})
        assert resp.status_code == 200, resp.json()
        assert resp.json()["needs_rationale"] == before + 1


def test_a_card_can_be_removed_without_a_corpus(swappable):
    """Removing a card is a fact about this deck file, not about Magic, so it
    works on a machine that has never run `data refresh`."""
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/mono-green/cards/Primeval Titan")
        assert resp.status_code == 200, resp.json()
        assert resp.json()["removed"] == "Primeval Titan"
        names = {c["name"] for c in
                 client.get("/api/decks/mono-green").json()["cards"]}
        assert "Primeval Titan" not in names


def test_removing_a_card_that_is_not_there_is_refused(swappable):
    with swappable as client:
        resp = client.request("DELETE",
                              "/api/decks/mono-green/cards/Black Lotus")
        assert resp.status_code == 422
        assert "not in this deck" in resp.json()["detail"]


def test_a_rationale_can_be_written_through_the_api(swappable):
    """The gap `decks import` opened: a draft arrives owing 99 rationales, and
    until this endpoint the only way to write one was a text editor."""
    with swappable as client:
        resp = client.patch("/api/decks/mono-green/cards/Sol Ring",
                            json={"field": "why",
                                  "value": "Two mana for one, and it always has been."})
        assert resp.status_code == 200, resp.json()
        card = next(c for c in client.get("/api/decks/mono-green").json()["cards"]
                    if c["name"] == "Sol Ring")
        assert card["why"] == "Two mana for one, and it always has been."


def test_a_rationale_cannot_be_blanked_on_a_curated_deck(swappable):
    with swappable as client:
        resp = client.patch("/api/decks/mono-green/cards/Sol Ring",
                            json={"field": "why", "value": "   "})
        assert resp.status_code == 422
        assert "needs a `why`" in resp.json()["detail"]


def test_only_a_short_list_of_card_fields_is_settable(swappable):
    with swappable as client:
        for field in ("name", "scryfall_id", "tags"):
            resp = client.patch("/api/decks/mono-green/cards/Sol Ring",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not settable" in resp.json()["detail"]


def test_a_category_and_a_quantity_can_be_patched(swappable):
    with swappable as client:
        assert client.patch("/api/decks/mono-green/cards/Sol Ring",
                            json={"field": "category",
                                  "value": "utility"}).status_code == 200
        assert client.patch("/api/decks/mono-green/cards/Forest",
                            json={"field": "qty", "value": 26}).status_code == 200
        cards = client.get("/api/decks/mono-green").json()["cards"]
        assert next(c for c in cards if c["name"] == "Sol Ring")["category"] == "utility"
        assert next(c for c in cards if c["name"] == "Forest")["qty"] == 26


def test_a_note_can_be_set_and_read_back(swappable):
    with swappable as client:
        resp = client.put("/api/decks/mono-green/notes/mulligan",
                          json={"value": "Keep any two-lander with a one-mana dork."})
        assert resp.status_code == 200, resp.json()
        notes = client.get("/api/decks/mono-green").json()["notes"]
        assert notes["mulligan"] == "Keep any two-lander with a one-mana dork."


def test_an_empty_note_is_refused(swappable):
    with swappable as client:
        resp = client.put("/api/decks/mono-green/notes/mulligan",
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
        client.patch("/api/decks/mono-green/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        resp = client.patch("/api/decks/mono-green",
                            json={"field": "stage", "value": "curated"})
        assert resp.status_code == 422
        assert "Sol Ring" in resp.json()["detail"]
        # And it stayed a draft rather than landing somewhere in between.
        assert client.get("/api/decks/mono-green").json()["stage"] == "draft"


def test_deck_status_and_bracket_are_patchable(swappable):
    with swappable as client:
        assert client.patch("/api/decks/mono-green",
                            json={"field": "status",
                                  "value": "built"}).status_code == 200
        assert client.patch("/api/decks/mono-green",
                            json={"field": "bracket", "value": 5}).status_code == 200
        body = client.get("/api/decks/mono-green").json()
        assert (body["status"], body["bracket"]) == ("built", 5)


def test_a_field_that_is_not_the_decks_own_is_refused(swappable):
    with swappable as client:
        for field in ("name", "commander", "cards"):
            resp = client.patch("/api/decks/mono-green",
                                json={"field": field, "value": "x"})
            assert resp.status_code == 422, field
            assert "not a settable deck field" in resp.json()["detail"]


def test_a_refused_edit_changes_nothing(swappable):
    """The whole point of verifying before writing. Every refusal above must
    leave the deck byte-identical, not partly applied."""
    with swappable as client:
        before = client.get("/api/decks/mono-green").json()
        client.post("/api/decks/mono-green/cards",
                    json={"name": "Sol Ring", "category": "ramp", "why": "dup"})
        client.request("DELETE", "/api/decks/mono-green/cards/Black Lotus")
        client.patch("/api/decks/mono-green/cards/Sol Ring",
                     json={"field": "why", "value": ""})
        client.patch("/api/decks/mono-green/cards/Forest",
                     json={"field": "qty", "value": 0})
        client.put("/api/decks/mono-green/notes/x", json={"value": ""})
        assert client.get("/api/decks/mono-green").json() == before


def test_a_read_only_source_refuses_every_edit():
    deck = tiny_corpus.mono_green_deck()
    app = create_app()
    app.dependency_overrides[deck_source] = \
        lambda: MemoryDeckSource([deck], writable=False)
    with TestClient(app) as client:
        base = "/api/decks/mono-green"
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
    """An empty library, with the fixture corpus behind it.

    Built rather than borrowed: the real 500MB corpus is absent on a fresh
    clone and in CI, and the one thing worth proving about import is what it
    does *with* a corpus.
    """
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

def test_card_search_respects_the_limit(corpus, client):
    body = client.get("/api/cards/search", params={"limit": 5}).json()
    assert len(body["cards"]) <= 5


def test_card_search_identity_filter_is_a_subset_check(corpus, client):
    """Asking for BG must return colorless and mono-B cards too -- that is the
    question a Golgari deckbuilder is actually asking."""
    body = client.get("/api/cards/search",
                      params={"identity": "BG", "limit": 50}).json()
    for card in body["cards"]:
        assert set(card["color_identity"]) <= {"B", "G"}, card["name"]


def test_search_limit_is_clamped_not_rejected(client):
    assert client.get("/api/cards/search", params={"limit": 9999}).status_code == 422


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
# are the ones that need the whole stack: a request, a deck source, a corpus
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

    edited = sim_client.patch("/api/decks/mono-green/cards/Sol Ring", json={
        "field": "why",
        "value": "Two mana on turn one is the fastest thing this deck can do, "
                 "and every payoff here is expensive enough to want it.",
    })
    assert edited.status_code == 200, edited.text
    assert sim_client.post("/api/sim/mana", json=body).json()["result"]["cached"] \
        is True, "a rationale edit must not invalidate a simulation"

    swapped = sim_client.post("/api/decks/mono-green/swap", json={
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
        in_memory_client, corpus):
    """The other `plan_lands` failure, and the one `_resize` raises rather than
    `_compile`: it has to survive the same fallback."""
    from mtglab.decks.model import CardEntry

    landless = tiny_corpus.mono_green_deck(clean=True)
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

def test_create_makes_a_draft_from_a_commander(corpus, in_memory_client):
    with in_memory_client([]) as c:
        body = c.post("/api/decks", json={
            "slug": "brand-new", "commander": ["Gyome, Master Chef"]}).json()
    assert body["created"] is True
    # A new deck owes its rationales, so it starts as a draft -- ADR 13.
    assert body["stage"] == "draft"
    assert body["total_cards"] == 0
    assert body["color_identity"] == ["B", "G"]
    assert body["combination"]["name"] == "Golgari"


def test_create_refuses_a_card_that_cannot_lead_a_deck(corpus, in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": ["Sol Ring"]})
    assert r.status_code == 422
    assert "cannot be your commander" in r.json()["detail"]


def test_create_refuses_a_deck_with_no_commander(in_memory_client):
    with in_memory_client([]) as c:
        r = c.post("/api/decks", json={"slug": "nope", "commander": []})
    assert r.status_code == 422
    assert "needs a commander" in r.json()["detail"]


def test_create_refuses_a_duplicate_slug(corpus, in_memory_client):
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


@pytest.mark.needs_full_corpus
def test_commander_search_is_exact_and_returns_actual_commanders(client):
    """The bug this pins: `commanders_only` used to filter after the SQL
    limit, so a search for Selesnya commanders returned the sixty best
    Selesnya cards, none of which was a commander, and then nothing.

    The one test here that `tiny_corpus` genuinely cannot carry, and it is
    marked rather than quietly skipped so the gap is countable. Reproducing
    the bug needs *more* Selesnya cards than the limit, with the commander
    ranked below the cut -- against 21 cards the filter and the limit
    cannot disagree, so a fixture version would pass whether or not the bug
    was back. Eleven more fixture rows to keep one assertion honest is a
    worse trade than saying plainly that this one needs the real corpus.
    """
    if not config.DB_PATH.exists():
        pytest.skip("needs the full corpus -- run `mtglab data refresh`")
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
    yield source
    client.app.dependency_overrides.pop(deck_source, None)


def test_deleting_needs_the_slug_as_confirmation(client, deletable):
    """Not a boolean. A client that sends `confirm=true` for every deletion has
    confirmed nothing, and a mis-aimed request looks exactly like an intended
    one. The slug is a value only somebody looking at the right deck has."""
    r = client.request("DELETE", "/api/decks/doomed?confirm=true")
    assert r.status_code == 422
    assert "confirm with the slug" in r.json()["detail"]
    assert deletable.slugs() == ["doomed"], "and nothing was moved"


def test_deleting_with_no_confirmation_at_all_is_refused(client, deletable):
    assert client.request("DELETE", "/api/decks/doomed").status_code == 422
    assert deletable.slugs() == ["doomed"]


def test_deleting_the_wrong_slug_is_refused(client, deletable):
    """The mis-click this is actually protecting against: the right dialog
    open over the wrong row."""
    r = client.request("DELETE", "/api/decks/doomed?confirm=arahbo-cats")
    assert r.status_code == 422
    assert deletable.slugs() == ["doomed"]


def test_deleting_with_the_slug_removes_the_deck_and_says_where_it_went(
        client, deletable):
    r = client.request("DELETE", "/api/decks/doomed?confirm=doomed")
    assert r.status_code == 200
    body = r.json()
    assert body["deleted"] is True
    assert body["name"] == "Doomed Deck"
    # "Deleted" and "recoverable" have to be separately true and separately
    # visible, so the response carries a location rather than only a boolean.
    assert body["moved_to"]
    assert deletable.slugs() == []


def test_deleting_an_unknown_deck_is_a_404(client, deletable):
    r = client.request("DELETE", "/api/decks/no-such-deck?confirm=no-such-deck")
    assert r.status_code == 404


def test_a_read_only_library_refuses_a_deletion(client):
    """docs/HOSTING.md keeps the curated decks read-only for everyone but the
    maintainer. This is the operation where that matters most."""
    deck = Deck.from_text(
        "slug: doomed\nname: Doomed\ncommander:\n  - Gyome, Master Chef\n",
        slug="doomed")
    source = MemoryDeckSource([deck], writable=False)
    client.app.dependency_overrides[deck_source] = lambda: source
    try:
        r = client.request("DELETE", "/api/decks/doomed?confirm=doomed")
        assert r.status_code == 422
        assert "read-only" in r.json()["detail"]
        assert source.slugs() == ["doomed"]
    finally:
        client.app.dependency_overrides.pop(deck_source, None)


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
                             params={"slug": "goreclaw-stompy"}).json()
    built = client.get("/api/claude", params={"slug": "arahbo-cats"}).json()
    assert theoretical["default"]["preset"] == "second-opinion"
    assert built["default"]["preset"] == "consultant"
    # Neither default may write.
    assert not theoretical["default"]["may_write"]
    assert not built["default"]["may_write"]


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
    r = client.post("/api/decks/gyome-food/interview", json={})
    assert r.status_code == 422


def test_the_interview_refuses_a_card_the_deck_does_not_run(client):
    """A 422 rather than a 404: the deck is fine, the question is not."""
    r = client.post("/api/decks/gyome-food/interview",
                    json={"card": "Black Lotus", "stance": "consultant"})
    assert r.status_code == 422
    assert "not in gyome-food" in r.json()["detail"]


def test_the_interview_on_an_unknown_deck_is_a_404(client):
    r = client.post("/api/decks/no-such-deck/interview", json={"card": "Sol Ring"})
    assert r.status_code == 404


def test_the_interview_at_a_stance_of_off_makes_no_call(client):
    """`off` is a real position, all the way down to the route. The client is
    sabotaged so that any attempt to build one fails the test."""
    import mtglab.claude.client as cc
    original = cc.connect
    cc.connect = lambda: pytest.fail("a stance of off must make no call")
    try:
        r = client.post("/api/decks/gyome-food/interview",
                        json={"card": "Bag End Banquet", "stance": "off"})
    finally:
        cc.connect = original
    assert r.status_code == 200
    body = r.json()
    assert body["asked"] is False
    assert body["questions"] == []
    assert body["answered_by"] == "claude", "labelled even when it said nothing"
