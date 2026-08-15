"""Research: everything about it that does not need a model.

No network, no key, no tokens — the same rule the other five modes' tests
follow, and for the same reason. What is here is the part that has to hold
whatever the model says.

[ADR 26](../docs/adr/0026-research-answers-about-magic-not-about-your-deck.md)
names four things, and the first is checkable by looking at what the mode
*cannot do*:

* **It cannot reach a deck.** No deck tool in its tool set, no `DeckSource` in
  its signatures, no slug on its route. That is what keeps rule 4 out of reach
  and what keeps deck conversation — ADR 15's third mode, deliberately unbuilt
  — from being built here by accident.
* **A finding whose citations all failed the check is dropped, not narrowed.**
  This is the one place research goes further than the dossier, and the reason
  is that a dossier section may rest on its brief while research has no brief.
* **An unresolved card name is labelled, not dropped** — the deliberate
  opposite of `dossier._rivals`, because a card spoiled since the last
  `data refresh` is exactly what this surface is for.
* **No source surviving means no answer.**

The write boundary that outranks all of it lives in `test_claude_boundary.py`,
which picked this module up the moment it existed.
"""

import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_pool
from mtglab import config
from mtglab.claude import client, dossier, modes, research, stance, tools


@pytest.fixture
def pool(tmp_path):
    """A real DuckDB pool with 21 real cards, built in about a second.

    Needed because `resolve_cards` is not a string filter — it asks the pool
    what each named card actually is, which is the whole point of it.
    """
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


@pytest.fixture
def no_network(monkeypatch):
    """Any attempt to reach the API is a test failure rather than a bill."""
    def refuse():
        raise AssertionError("a Claude call was made; this test forbids one")
    monkeypatch.setattr(client, "connect", refuse)


# ---------------------------------------------------------------- the mode

def test_the_mode_declares_no_write_capability():
    """ADR 26 leaves ADR 15's invariant alone, as an assertion."""
    assert research.RESEARCH.may_write == ()


def test_the_tool_set_is_inside_the_read_only_registry():
    assert set(research.RESEARCH.tool_names) <= set(tools.READ_ONLY)


@pytest.mark.parametrize("tool", ["get_deck", "list_decks", "validate_deck",
                                  "deck_stats", "suggest_replacements"])
def test_research_cannot_reach_a_deck_through_any_tool(tool):
    """ADR 26's first decision, one tool at a time.

    Parametrised rather than a set comparison so a failure names the tool
    somebody added. Every one of these would make this mode able to read a
    decklist, and a mode that can read a decklist and write declarative prose
    about cards is deck conversation — ADR 15's third mode, unbuilt on purpose
    and owing five separate arguments before it is built on purpose.
    """
    assert tool not in research.RESEARCH.tool_names


def test_the_only_pool_door_is_get_cards():
    """Positively, not just by absence: rule 1 needs *a* door, and unlike the
    interview and the dossier this mode cannot assemble a brief before the
    call, because it does not know what the question is about until the model
    reads it. So `get_cards` is not belt-and-braces here; it is the only one."""
    assert research.RESEARCH.tool_names == ("get_cards",)


def test_the_only_server_tool_is_a_hosted_search():
    """`CLAUDE.md` bans a crawler. This is the assertion that the mode reaches
    the web through Anthropic's hosted search and through nothing else."""
    server = research.RESEARCH.server_tools
    assert [t["name"] for t in server] == ["web_search"]
    assert server[0]["type"] == "web_search_20260209"


def test_code_execution_is_not_declared_alongside_the_search():
    """The dated search runs its own filtering in a container; declaring a
    second execution environment confuses the model, and the docs say so."""
    types = {t["type"] for t in research.RESEARCH.server_tools}
    assert not any(t.startswith("code_execution") for t in types)


def test_the_request_tool_list_is_stable_across_calls():
    """Tools render first in the prompt, so an unstable order would invalidate
    the prompt cache on every call — for free, and invisibly."""
    assert json.dumps(research.RESEARCH.schemas()) == \
        json.dumps(research.RESEARCH.schemas())


# ------------------------------------------------------------- the schema

def test_the_schema_forbids_extra_properties_at_every_level():
    """A field nobody argued for cannot appear at run time."""
    def walk(node):
        if isinstance(node, dict):
            if node.get("type") == "object":
                assert node.get("additionalProperties") is False, node
            for value in node.values():
                walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)
    walk(research.RESPONSE_SCHEMA)


@pytest.mark.parametrize("field", ["deck", "slug", "recommendation", "verdict",
                                   "why", "rationale", "swap", "cut"])
def test_the_schema_has_no_field_for_a_deck_or_a_decision(field):
    """The mode cannot see a deck, so a field naming one would have nothing to
    put in it — and a field for a recommendation would be a rationale with the
    deck left implicit."""
    assert field not in research.RESPONSE_SCHEMA["properties"]


def test_every_finding_must_declare_its_sources():
    """`source_ids` is required on a finding rather than optional, which is the
    schema half of `only_grounded`. The dossier's is optional there, because a
    dossier passage may rest on the brief it was handed; this mode has no
    brief."""
    finding = research.RESPONSE_SCHEMA["properties"]["findings"]["items"]
    assert set(finding["required"]) == {"claim", "source_ids"}


def test_confidence_is_an_enum_that_can_say_the_answer_is_thin():
    assert research.CONFIDENCE == ("settled", "contested", "thin")
    assert research.RESPONSE_SCHEMA["properties"]["confidence"]["enum"] == \
        list(research.CONFIDENCE)


def test_the_instructions_say_it_cannot_see_a_deck():
    """The prompt is not the guard — the tool set is — but a mode that failed
    to *say* this would answer deck questions confidently from nothing."""
    assert "cannot see anybody's deck" in research.INSTRUCTIONS.lower()


def test_the_scope_note_never_mentions_a_card_it_was_asked_about():
    """`modes._scope_note`'s default table is written for the per-card modes and
    reads as nonsense here. This mode brings its own, and the failure if it did
    not would be a prompt telling the model to stay on something that does not
    exist."""
    for level in ("flagged", "adjacent", "rethink"):
        text = research.RESEARCH.system(
            stance.Stance("volunteers", level, "none"))
        assert "the card you were asked about" not in text
        assert "Scope:" in text


# ----------------------------------------------------------- the question

def test_an_empty_question_is_refused_without_a_call(no_network):
    with pytest.raises(research.QuestionRejected):
        research.check_research("   ")


def test_a_pasted_decklist_is_refused_by_length(no_network):
    """Refused in the request rather than after a paid search, and the message
    says the thing the user needs to hear: this surface cannot see decks."""
    with pytest.raises(research.QuestionRejected) as caught:
        research.check_research("Sol Ring\n" * 400)
    assert "cannot see decks" in str(caught.value)


def test_the_question_survives_the_round_trip(pool, no_network):
    request = research.check_research("  Is Bag End Banquet played?  ")
    assert request.question == "Is Bag End Banquet played?"


def test_two_spellings_of_one_question_are_one_job():
    """`jobs.submit`'s dedupe key. Whitespace and case do not make it a
    different question; anything else does."""
    assert research.question_key("Is  Bag End Banquet   GOOD?") == \
        research.question_key("is bag end banquet good?")
    assert research.question_key("is it good?") != \
        research.question_key("is it bad?")


def test_the_key_is_not_a_cache_key():
    """ADR 26 refuses to cache a research answer, so nothing reads this key
    from a store. Asserted by absence: the module has no get/put/clear the way
    `dossier.py` does, and adding one is the change this test is watching
    for."""
    for name in ("get", "put", "clear", "stored", "cache_key"):
        assert not hasattr(research, name), name


# --------------------------------------------------------- the stance

def test_the_default_stance_is_not_off():
    """The bug `/api/claude` already shipped once, in the one other place it
    could happen. There is no deck here, and `stance.resolve(None, None)` is
    `off` — which would make a screen whose only control is a question box do
    nothing, silently."""
    assert research.stance_for(None).initiative != "off"
    assert research.stance_for(None) == stance.clamp(
        stance.PRESETS[research.DEFAULT_PRESET], stance.ceiling())


def test_off_is_still_reachable_and_makes_no_call(no_network):
    request = research.check_research("what is the meta?", requested="off")
    assert not request.needs_call
    answer = research.run_research(request)
    assert answer["asked"] is False
    assert answer["research"] == {}
    assert "stance is off" in answer["reason"]


def test_a_deployment_ceiling_still_clamps_this_surface(monkeypatch):
    """`off` means off, including for the surface that defaults above it."""
    monkeypatch.setenv("MTGLAB_CLAUDE_STANCE_CEILING", "off")
    assert research.stance_for(None).initiative == "off"


# --------------------------------------------------- grounding the findings

SOURCES = [{"id": "s1", "title": "A primer", "url": "https://example.com/a"}]


def test_a_finding_citing_nothing_is_dropped_and_counted():
    """The instrument, and the reason it goes one step past the dossier's:
    with no brief behind it, an uncited claim is resting on the model's
    recall."""
    kept, dropped = research.only_grounded([
        {"claim": "Cited.", "source_ids": ["s1"]},
        {"claim": "Not cited.", "source_ids": []},
        {"claim": "Cited to a page that was thrown away.",
         "source_ids": ["s9"]},
    ], {"s1"})
    assert [k["claim"] for k in kept] == ["Cited."]
    assert dropped == 2


def test_a_finding_with_no_claim_is_dropped():
    kept, dropped = research.only_grounded(
        [{"claim": "   ", "source_ids": ["s1"]}], {"s1"})
    assert kept == []
    assert dropped == 1


def test_junk_in_the_findings_list_is_counted_rather_than_crashing():
    kept, dropped = research.only_grounded(["not a dict", None], {"s1"})
    assert (kept, dropped) == ([], 2)


def test_the_source_check_is_the_dossiers_own():
    """Shared rather than copied (ADR 26's consequences). Two copies would be
    two chances to disagree about whether a trailing slash is a different
    page."""
    assert research.keep_sources is dossier.keep_sources
    assert research.canonical_url is dossier.canonical_url


# ---------------------------------------------------- resolving the cards

def test_a_card_the_pool_has_comes_back_as_a_pool_fact(pool):
    cards, unresolved = research.resolve_cards(["Goreclaw, Terror of Qal Sisma"])
    assert unresolved == 0
    assert cards[0]["in_pool"] is True
    assert cards[0]["type_line"].startswith("Legendary Creature")
    # The real text, so the reader can check the prose against the card rather
    # than against the model's word for it.
    assert cards[0]["oracle_text"]


def test_a_card_the_pool_lacks_is_labelled_rather_than_dropped(pool):
    """**The deliberate difference from the dossier, and the whole of ADR 26's
    third decision.**

    `dossier._rivals` drops an unresolved name because a rival that does not
    exist is an error. Here it may instead be a card spoiled since the last
    `data refresh` — one of the three things this surface exists for — and the
    two are indistinguishable from inside `get_cards`. So both are kept and
    marked, and the reader gets a boundary they can see.
    """
    cards, unresolved = research.resolve_cards(
        ["Goreclaw, Terror of Qal Sisma", "Sporeback Wurmcaller"])
    assert unresolved == 1
    by_name = {c["name"]: c for c in cards}
    assert by_name["Sporeback Wurmcaller"]["in_pool"] is False
    # And nothing else. An unresolved card carries no oracle text, because
    # there is none to carry — anything said about it rests on a cited page.
    assert "oracle_text" not in by_name["Sporeback Wurmcaller"]
    assert by_name["Goreclaw, Terror of Qal Sisma"]["in_pool"] is True


def test_a_double_faced_card_resolves_from_its_front_face(pool):
    """The bug `argue.resolve_alternatives` was written around, and the same
    fix. A DFC comes back under its full `A // B` name, so an index keyed only
    on the pool's spelling drops every one a model names by its front face —
    and drops it *silently*, which here would report a real card as one the
    pool has never seen."""
    cards, unresolved = research.resolve_cards(["Ajani, Nacatl Pariah"])
    assert unresolved == 0
    assert cards[0]["in_pool"] is True
    assert cards[0]["name"] == "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"


def test_a_banned_card_still_resolves_and_says_so(pool):
    """`get_cards` filters on nothing, which is what makes it the only way to
    read a banned card. Research has to be able to answer "why is this
    banned"."""
    cards, _ = research.resolve_cards(["Primeval Titan"])
    assert cards[0]["in_pool"] is True
    assert cards[0]["legal_commander"] is False


def test_the_same_card_named_twice_appears_once(pool):
    cards, _ = research.resolve_cards(
        ["Goreclaw, Terror of Qal Sisma", "goreclaw, terror of qal sisma"])
    assert len(cards) == 1


def test_the_card_list_is_capped(pool):
    cards, _ = research.resolve_cards(
        [f"Nonexistent Card {i}" for i in range(40)])
    assert len(cards) == research.MAX_CARDS


def test_no_cards_named_is_not_a_lookup(pool):
    assert research.resolve_cards([]) == ([], 0)


# ------------------------------------------------------- reading an answer

class _Turn:
    """A `Turn` as `converse` would return it, without the call."""

    def __init__(self, payload, searched, *, refused=False, text=None):
        self.mode = research.RESEARCH.name
        self.model = "claude-sonnet-5"
        self.stop_reason = "end_turn"
        self.text = json.dumps(payload) if text is None else text
        self.tool_calls = []
        self.input_tokens = 100
        self.output_tokens = 200
        self.cache_read_tokens = 0
        self.searched = searched
        self.search_errors = []
        self.refused = refused

    def parsed(self):
        return json.loads(self.text)


def _run(monkeypatch, payload, searched, **turn_kwargs):
    turn = _Turn(payload, searched, **turn_kwargs)
    monkeypatch.setattr(research, "converse", lambda *a, **k: turn)
    request = research.check_research("Is Goreclaw still played?")
    return research.run_research(request)


PAGE = {"url": "https://example.com/a", "title": "A primer"}


def test_an_answer_with_no_surviving_source_is_refused(monkeypatch, pool):
    """ADR 26, following ADR 19. An unsourced research answer is a model
    talking about Magic from memory with a search box drawn around it — the
    exact failure rule 1 exists to prevent, one source further out."""
    report = _run(monkeypatch, {
        "answer": "Confidently wrong.",
        "findings": [{"claim": "Everyone says so.", "source_ids": ["s1"]}],
        "cards": [],
        "confidence": "settled",
        "sources": [{"id": "s1", "title": "Invented",
                     "url": "https://example.com/never-fetched"}],
    }, [PAGE])
    assert report["research"] == {}
    assert "No source survived" in report["reason"]
    # And it says how many, so an invented citation is visible rather than
    # merely absent.
    assert "1 cited page(s)" in report["reason"]


def test_a_grounded_answer_comes_back_whole(monkeypatch, pool):
    report = _run(monkeypatch, {
        "answer": "It is still played in stompy lists.",
        "findings": [
            {"claim": "Primers list it as a top-ten green commander.",
             "source_ids": ["s1"]},
            {"claim": "Made up.", "source_ids": []},
        ],
        "cards": ["Goreclaw, Terror of Qal Sisma", "Sporeback Wurmcaller"],
        "confidence": "contested",
        "sources": [{"id": "s1", "title": "A primer",
                     "url": "https://example.com/a"}],
    }, [PAGE])

    body = report["research"]
    assert report["asked"] is True
    assert body["answer"].startswith("It is still played")
    assert len(body["findings"]) == 1
    assert body["findings_dropped"] == 1
    assert body["confidence"] == "contested"
    assert body["cards_unresolved"] == 1
    assert body["searched"] == 1
    assert report["answered_by"] == "claude"
    # The label that says this is not the gate's, and says the deck part out
    # loud because a citation-backed answer looks reproducible and is not.
    assert "has not seen any of your decks" in report["never"]


def test_a_mislabelled_confidence_falls_back_to_thin(monkeypatch, pool):
    """Falling back rather than dropping, the way `argue.only_charges` falls
    back on an unrecognised ground: the answer is worth more than its label,
    and `thin` is the honest default for one that arrived mislabelled."""
    report = _run(monkeypatch, {
        "answer": "Something.",
        "findings": [{"claim": "A thing.", "source_ids": ["s1"]}],
        "cards": [],
        "confidence": "extremely settled",
        "sources": [{"id": "s1", "title": "A primer",
                     "url": "https://example.com/a"}],
    }, [PAGE])
    assert report["research"]["confidence"] == "thin"


def test_an_unparsable_answer_says_so_rather_than_raising(monkeypatch, pool):
    report = _run(monkeypatch, {}, [PAGE], text="not json at all")
    assert report["research"] == {}
    assert "did not parse" in report["reason"]


def test_a_refusal_is_reported_rather_than_raised(monkeypatch, pool):
    report = _run(monkeypatch, {}, [PAGE], refused=True, text="")
    assert report["research"] == {}
    assert "declined" in report["reason"]


def test_the_findings_are_capped(monkeypatch, pool):
    report = _run(monkeypatch, {
        "answer": "Lots.",
        "findings": [{"claim": f"Fact {i}.", "source_ids": ["s1"]}
                     for i in range(20)],
        "cards": [],
        "confidence": "settled",
        "sources": [{"id": "s1", "title": "A primer",
                     "url": "https://example.com/a"}],
    }, [PAGE])
    assert len(report["research"]["findings"]) == research.MAX_FINDINGS


def test_the_call_is_made_with_no_deck_source(monkeypatch, pool):
    """ADR 26's first decision at the point it would actually leak. Every
    other assertion here is about what the mode *declares*; this one is about
    what `converse` is *handed*, which is the value a deck tool would use."""
    seen = {}

    def spy(mode, **kwargs):
        seen.update(kwargs)
        return _Turn({
            "answer": "x", "findings": [{"claim": "y", "source_ids": ["s1"]}],
            "cards": [], "confidence": "thin",
            "sources": [{"id": "s1", "title": "A primer",
                         "url": "https://example.com/a"}],
        }, [PAGE])

    monkeypatch.setattr(research, "converse", spy)
    research.ask("Anything?")
    assert seen["source"] is None


def test_nothing_is_stored_after_an_answer(monkeypatch, pool):
    """A research answer is the one thing in this codebase that must not be
    cached (ADR 26): its subject is the part of Magic that moves. Asserted by
    running the same question twice and checking the second one still calls."""
    calls = []

    def spy(mode, **kwargs):
        calls.append(1)
        return _Turn({
            "answer": "x", "findings": [{"claim": "y", "source_ids": ["s1"]}],
            "cards": [], "confidence": "thin",
            "sources": [{"id": "s1", "title": "A primer",
                         "url": "https://example.com/a"}],
        }, [PAGE])

    monkeypatch.setattr(research, "converse", spy)
    research.ask("Is Goreclaw still played?")
    research.ask("Is Goreclaw still played?")
    assert len(calls) == 2


def test_the_answer_is_stamped_with_when_it_was_written(monkeypatch, pool):
    """The honest substitute for the freshness guarantee it cannot make."""
    report = _run(monkeypatch, {
        "answer": "x", "findings": [{"claim": "y", "source_ids": ["s1"]}],
        "cards": [], "confidence": "thin",
        "sources": [{"id": "s1", "title": "A primer",
                     "url": "https://example.com/a"}],
    }, [PAGE])
    assert report["generated_at"].startswith("20")


# ------------------------------------------------------------ the request

def test_check_research_takes_no_deck_source():
    """The signature is the decision. A `DeckSource` parameter here is the one
    line somebody would add to make this mode deck-aware, and the diff that
    adds it has to fail something."""
    import inspect
    for fn in (research.check_research, research.run_research, research.ask):
        names = set(inspect.signature(fn).parameters)
        assert "source" not in names, fn.__name__
        assert "slug" not in names, fn.__name__


def test_the_mode_is_exhaustible_rather_than_endless():
    """`converse`'s ceiling applies here as everywhere; a mode still asking for
    tools after its turns is an exception rather than a bill."""
    assert modes.ModeExhausted is not None
