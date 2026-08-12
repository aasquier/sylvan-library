"""Tool schemas and dispatch, with no model and no network involved.

Every test here runs against a `MemoryDeckSource` and tolerates a missing
corpus, the same way `test_api.py` does -- a fresh clone has no
`data/mtg.duckdb` until `data refresh` runs, and a tool layer that only works
on a fully-populated machine is one nobody can test on CI.

What is *not* here: any call to Claude. The pipe is verified by
`mtglab claude check`, deliberately a command rather than a test, because a
suite that spends money on every run is a suite people stop running. The
boundary that must hold regardless lives in `test_claude_boundary.py`.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.claude import client, tools  # noqa: E402
from mtglab.decks.model import Deck  # noqa: E402
from mtglab.decks.source import MemoryDeckSource  # noqa: E402

DECK_YAML = """\
slug: mini
name: Mini Deck
status: theoretical
stage: draft
commander:
  - Sol Ring
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
def source(tmp_path):
    path = tmp_path / "deck.yaml"
    path.write_text(DECK_YAML, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


# ------------------------------------------------------------------ schemas

def test_every_tool_renders_a_well_formed_schema():
    for schema in tools.schemas():
        assert schema["name"] in tools.READ_ONLY
        assert len(schema["description"]) > 80, (
            f"{schema['name']} is under-described. A vague description is the "
            f"most common reason a model answers from recall instead of "
            f"calling the tool, which here is rule 1 failing quietly.")
        body = schema["input_schema"]
        assert body["type"] == "object"
        assert body["additionalProperties"] is False
        # Anything required must actually be declared, or the model is being
        # asked for a field it was never told about.
        assert set(body["required"]) <= set(body["properties"])


def test_schemas_are_ordered_so_the_prompt_prefix_is_stable():
    """Tools render first in a request. An unstable order would invalidate the
    prompt cache on every turn, for no benefit and with no visible symptom."""
    names = [s["name"] for s in tools.schemas()]
    assert names == sorted(names)
    assert names == [s["name"] for s in tools.schemas()]


def test_a_mode_can_narrow_the_tool_set():
    narrowed = tools.schemas(("get_deck", "validate_deck"))
    assert [s["name"] for s in narrowed] == ["get_deck", "validate_deck"]


def test_the_lookup_tool_tells_the_model_not_to_trust_itself():
    """Rule 1 is structural -- the tool is the only source of card facts -- but
    the description is what makes the model reach for it in the first place."""
    description = tools.READ_ONLY["get_cards"].description.lower()
    assert "every claim you make about a specific card" in description
    assert "remember" in description
    assert "not_found" in description, (
        "the model has to be told that an unresolved name means it does not "
        "know the card, or silence reads as confirmation")


def test_the_search_tool_says_it_is_not_a_lookup():
    """The two card tools are not interchangeable, and the failure mode when a
    model treats them as such is a banned card silently reading as absent."""
    description = tools.READ_ONLY["search_cards"].description.lower()
    assert "not a lookup tool" in description
    assert "get_cards" in description


# ----------------------------------------------------------------- dispatch

def test_list_decks_runs_against_the_given_source(source):
    result = tools.run("list_decks", source=source)
    assert [d["slug"] for d in result] == ["mini"]


def test_get_deck_runs_and_returns_the_rationales(source):
    """Reading a `why` is allowed and necessary -- arguing against a card's
    slot means seeing what was claimed for it. Writing one is not."""
    result = tools.run("get_deck", {"slug": "mini"}, source=source)
    assert result["name"] == "Mini Deck"
    assert {c["why"] for c in result["cards"]} == {"Black mana.",
                                                   "Two mana for one."}


def test_validate_deck_runs_and_reports_the_gate(source):
    result = tools.run("validate_deck", {"slug": "mini"}, source=source)
    assert set(result) == {"ok", "errors", "warnings"}


def test_deck_stats_runs(source):
    result = tools.run("deck_stats", {"slug": "mini"}, source=source)
    assert "curve" in result


def test_suggest_replacements_runs(source):
    result = tools.run("suggest_replacements", {"slug": "mini", "limit": 3},
                       source=source)
    assert result["slug"] == "mini"


def test_search_cards_runs_without_a_deck_source():
    """The corpus tools take no source -- they are not deck-facing. Tolerates a
    missing corpus, which is what a fresh clone has."""
    result = tools.run("search_cards", {"q": "Sol Ring", "limit": 1})
    assert "cards" in result


def requires_corpus():
    if not tools.run("get_cards", {"names": ["Sol Ring"]})["cards"]:
        pytest.skip("no corpus -- run `mtglab data refresh`")


def test_get_cards_looks_a_card_up_by_name():
    requires_corpus()
    result = tools.run("get_cards", {"names": ["sol ring"]})
    card = result["cards"][0]
    # The corpus's spelling comes back, not the caller's.
    assert card["name"] == "Sol Ring"
    assert card["asked_as"] == "sol ring"
    assert "Add {C}{C}" in card["oracle_text"]


def test_a_banned_card_can_be_looked_up_and_says_it_is_banned():
    """The hole this tool exists to close.

    `search_cards` filters to Commander-legal, so the two cards the library
    deliberately fails the gate on were invisible to it -- and a first turn
    duly answered about them from recall. A lookup filters on nothing and
    reports legality as a field, which is strictly more useful than absence.
    """
    requires_corpus()
    for name in ("Primeval Titan", "Emrakul, the Aeons Torn"):
        looked_up = tools.run("get_cards", {"names": [name]})
        assert looked_up["not_found"] == [], f"{name} is still not reachable"
        card = looked_up["cards"][0]
        assert card["name"] == name
        assert card["legal_commander"] is False
        assert card["oracle_text"], "no oracle text -- the point of the lookup"

        # And the reason the split matters: discovery still hides it.
        found = tools.run("search_cards", {"q": name, "limit": 10})
        assert not any(c["name"] == name for c in found["cards"])


def test_a_name_that_does_not_resolve_is_reported_not_dropped():
    """Silence is the dangerous answer. A lookup that returns one card for two
    names is how a confident claim gets made about the second."""
    requires_corpus()
    result = tools.run("get_cards",
                       {"names": ["Sol Ring", "Sol Ringg, Destroyer of Typos"]})
    assert [c["name"] for c in result["cards"]] == ["Sol Ring"]
    assert result["not_found"] == ["Sol Ringg, Destroyer of Typos"]


def test_colour_identity_comes_back_whole_for_a_double_faced_card():
    """Rule 2, and the specific error that caused it: Ajani, Nacatl Pariah has
    a white front and a red back, so looking it up by the front face must still
    report {R}{W}. Derived from the mana cost it would read as mono-white and
    pass a Selesnya legality check it should fail."""
    requires_corpus()
    result = tools.run("get_cards", {"names": ["Ajani, Nacatl Pariah"]})
    if not result["cards"]:
        pytest.skip("corpus predates the card")
    assert sorted(result["cards"][0]["color_identity"]) == ["R", "W"]


def test_a_lookup_is_capped_at_a_deck():
    """A model can ask for anything. 100 is a commander plus the 99."""
    from mtglab.api import service
    result = tools.run("get_cards", {"names": ["Sol Ring"] * 400})
    assert len(result["cards"]) + len(result["not_found"]) <= service.MAX_NAMED_CARDS


def test_an_empty_lookup_is_not_an_error():
    assert tools.run("get_cards", {"names": []})["cards"] == []


def test_a_lookup_without_a_corpus_says_so_rather_than_reporting_nothing(tmp_path):
    """The state a fresh clone is in, before `data refresh`.

    Reporting every name as `not_found` with `corpus_available: false` is very
    different from reporting an empty result: one says "I cannot check", the
    other reads as "no such card". A model that conflates them will tell
    somebody a real card does not exist.
    """
    from mtglab import config
    with config.use_paths(data_dir=tmp_path / "empty"):
        result = tools.run("get_cards", {"names": ["Sol Ring"]})
    assert result["corpus_available"] is False
    assert result["not_found"] == ["Sol Ring"]
    assert result["cards"] == []


# ------------------------------------------------------ argument validation

def test_an_unknown_argument_is_rejected_rather_than_ignored(source):
    """Silently dropping an argument teaches a model that the call worked."""
    with pytest.raises(tools.ToolArgumentsRejected, match="destination"):
        tools.run("get_deck", {"slug": "mini", "destination": "Tokyo"},
                  source=source)


def test_a_missing_required_argument_is_rejected(source):
    with pytest.raises(tools.ToolArgumentsRejected, match="slug"):
        tools.run("get_deck", {}, source=source)


def test_an_explicit_null_is_dropped_rather_than_forwarded():
    """A model writing `"price_max": null` means "no filter", which is what the
    service function's own default already says -- more clearly."""
    result = tools.run("search_cards", {"q": "Sol Ring", "price_max": None,
                                        "limit": 1})
    assert "cards" in result


# ------------------------------------------------------------------- client

def test_the_model_is_sonnet_by_default():
    """Aaron's call, not a default to upgrade away from. Changing this is a
    decision with evidence behind it -- see ADR 14 and ROADMAP."""
    assert client.MODEL == "claude-sonnet-5"


def test_the_model_can_be_overridden_for_a_comparison_run(monkeypatch):
    monkeypatch.setenv("MTGLAB_CLAUDE_MODEL", "claude-opus-5")
    assert client.model() == "claude-opus-5"


def test_availability_is_answerable_without_a_key(monkeypatch):
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)
    assert client.credential_present() is False
    assert client.available() is False


def test_no_credential_is_a_fixable_error_not_a_network_failure(monkeypatch):
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)
    with pytest.raises(client.ClaudeUnavailable, match="ANTHROPIC_API_KEY"):
        client.connect()
    report = client.check()
    assert report["ok"] is False and "ANTHROPIC_API_KEY" in report["error"]


def _api_error(kind, status, message):
    """Build a real SDK exception without a real request.

    `explain` branches on exception class, so the branches have to be exercised
    with the actual classes rather than a stand-in.
    """
    import httpx

    request = httpx.Request("POST", "https://api.anthropic.com/v1/messages")
    response = httpx.Response(status, request=request, json={
        "type": "error", "error": {"type": "api_error", "message": message}})
    return kind(message, response=response, body=None)


@pytest.mark.parametrize("status,expect", [
    (403, "403"), (429, "rate limited"), (404, "404"), (500, "API error 500"),
])
def test_every_api_failure_explains_itself(status, expect):
    """These messages are the whole user interface of a failed call. A bare
    `APIStatusError` repr is not something to debug a key against."""
    anthropic = pytest.importorskip("anthropic")
    kind = {403: anthropic.PermissionDeniedError,
            429: anthropic.RateLimitError,
            404: anthropic.NotFoundError,
            500: anthropic.InternalServerError}[status]
    assert expect in client.explain(_api_error(kind, status, "boom"))


def test_an_unreachable_api_explains_itself():
    anthropic = pytest.importorskip("anthropic")
    import httpx

    exc = anthropic.APIConnectionError(
        request=httpx.Request("POST", "https://api.anthropic.com/v1/messages"))
    assert "could not reach" in client.explain(exc)


def test_check_reports_a_successful_call_without_making_one(monkeypatch):
    """Pins the shape of the success report -- the fields the CLI prints and a
    future health route will render -- with a stub in place of the network.

    The real call is `mtglab claude check`, deliberately not a test: a suite
    that spends money on every run is a suite people stop running.
    """
    class _Usage:
        input_tokens, output_tokens = 18, 6

    class _Block:
        type, text = "text", "pipe open"

    class _Response:
        model, stop_reason, content, usage = (
            "claude-sonnet-5", "end_turn", [_Block()], _Usage())

    class _Messages:
        def create(self, **kwargs):
            # The request surface Sonnet 5 actually accepts. A regression here
            # is a 400 in production and nothing at all in the tests.
            assert "budget_tokens" not in str(kwargs)
            assert not {"temperature", "top_p", "top_k"} & set(kwargs)
            assert kwargs["output_config"] == {"effort": "low"}
            return _Response()

    monkeypatch.setattr(client, "connect", lambda: type("C", (), {
        "messages": _Messages()})())

    report = client.check()
    assert report["ok"] is True
    assert report["text"] == "pipe open"
    assert (report["input_tokens"], report["output_tokens"]) == (18, 6)
    assert report["served_by"] == "claude-sonnet-5"


def test_a_refusal_is_not_reported_as_success(monkeypatch):
    """`stop_reason: "refusal"` arrives as a normal 200. Code that reads
    `content` without checking it reports a refusal as a working pipe."""
    class _Response:
        model, stop_reason, content = "claude-sonnet-5", "refusal", []
        usage = type("U", (), {"input_tokens": 9, "output_tokens": 0})()

    monkeypatch.setattr(client, "connect", lambda: type("C", (), {
        "messages": type("M", (), {"create": lambda self, **k: _Response()})()})())
    assert client.check()["ok"] is False


def test_an_api_failure_becomes_a_report_not_an_exception(monkeypatch):
    """`check()` is called by a CLI command that should print a reason and exit
    1, not produce a traceback."""
    anthropic = pytest.importorskip("anthropic")

    def _raise(self, **kwargs):
        raise _api_error(anthropic.RateLimitError, 429, "slow down")

    monkeypatch.setattr(client, "connect", lambda: type("C", (), {
        "messages": type("M", (), {"create": _raise})()})())
    report = client.check()
    assert report["ok"] is False and "rate limited" in report["error"]


def test_a_rejected_key_reads_as_a_possible_expiry():
    """The key this runs on has a fixed lifetime and cannot be extended, so the
    first thing a 401 means is probably that it lapsed. That has to be in the
    message: it gets read weeks later by someone who has forgotten."""
    anthropic = pytest.importorskip("anthropic")
    import httpx

    request = httpx.Request("POST", "https://api.anthropic.com/v1/messages")
    response = httpx.Response(401, request=request, json={
        "type": "error", "error": {"type": "authentication_error",
                                   "message": "invalid x-api-key"}})
    exc = anthropic.AuthenticationError("invalid x-api-key", response=response,
                                        body=None)
    assert "expired" in client.explain(exc)


def test_the_check_report_carries_no_credential(monkeypatch):
    """`check()`'s output is printed, and a printed thing gets pasted into an
    issue. `config.py` goes out of its way never to bind the key to a name;
    this is the other end of that -- nothing derived from it comes back out.

    Asserted as a closed set of keys rather than by scanning values, so a field
    added later has to be named here before it can be returned.
    """
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)
    assert set(client.check()) <= {"model", "ok", "error", "served_by",
                                   "stop_reason", "text", "input_tokens",
                                   "output_tokens"}
