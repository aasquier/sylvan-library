"""The rationale interview: everything about it that does not need a model.

No network, no key, no tokens. The one call this feature makes is deliberately
not made here -- a suite that spends money on every run is a suite people stop
running, which is the same reasoning that keeps `mtglab claude check` a command
rather than a test.

What *is* here is the part that has to hold whatever the model says. The mode
declares no write capability, its tool set is a subset of the one read-only
registry, its response schema has nowhere to put a rationale, and anything
coming back that is not a question gets dropped and counted. Those four are the
feature: the prompt asks the model to behave, and this file is the reason it
does not matter very much whether it does.

The boundary that outranks all of it -- nothing under `src/mtglab/claude/` may
name a write function -- lives in `test_claude_boundary.py` and picks these
modules up automatically.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.claude import client, interview, modes, stance, tools  # noqa: E402
from mtglab.decks.model import Deck  # noqa: E402
from mtglab.decks.source import MemoryDeckSource  # noqa: E402

# A deck shaped like the case this mode exists for: an imported draft where one
# card owes a rationale and its neighbours already have theirs, so a question
# about redundancy has something real to be about.
DECK_YAML = """\
slug: mini
name: Mini Deck
status: theoretical
stage: draft
bracket: 3
commander:
  - Gyome, Master Chef
cards:
  - name: Swamp
    category: land
    qty: 60
    why: Black mana.
  - name: Sol Ring
    category: ramp
    why: ''
  - name: Arcane Signet
    category: ramp
    why: Two mana for a rock that fixes both colours.
  - name: Primeval Titan
    category: ramp
    why: Two lands per attack.
"""


@pytest.fixture
def source(tmp_path):
    path = tmp_path / "deck.yaml"
    path.write_text(DECK_YAML, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


@pytest.fixture
def no_network(monkeypatch):
    """Make any attempt to reach the API a test failure rather than a bill."""
    def refuse():
        raise AssertionError("a Claude call was made; this test forbids one")
    monkeypatch.setattr(client, "connect", refuse)


def turn_returning(payload_json: str, **kw) -> modes.Turn:
    return modes.Turn(
        mode=interview.RATIONALE_INTERVIEW.name,
        model="claude-sonnet-5-test", stop_reason=kw.pop("stop_reason", "end_turn"),
        text=payload_json, tool_calls=kw.pop("tool_calls", []),
        input_tokens=100, output_tokens=50, **kw)


# --------------------------------------------------------------- the mode

def test_the_mode_declares_no_write_capability():
    """ADR 15's table, as an assertion rather than a document."""
    assert interview.RATIONALE_INTERVIEW.may_write == ()


def test_a_mode_that_declares_a_write_is_refused_at_construction():
    """The field exists so that filling it in has to be deliberate. Here is
    the thing that makes 'deliberate' mean 'fails loudly'."""
    with pytest.raises(ValueError, match="ADR 15"):
        modes.Mode(name="bad", purpose="", instructions="",
                   tool_names=("get_cards",), may_write=("set_card_field",))


def test_the_mode_can_only_name_tools_that_exist():
    with pytest.raises(tools.ToolNotAllowed):
        modes.Mode(name="bad", purpose="", instructions="",
                   tool_names=("get_cards", "write_the_deck_file"))


def test_the_interview_tool_set_is_inside_the_read_only_registry():
    """A mode subsets the registry. It cannot extend it, and this is the
    cheapest place for that to be visible."""
    assert set(interview.RATIONALE_INTERVIEW.tool_names) <= set(tools.READ_ONLY)


def test_the_interview_cannot_shop_for_replacements():
    """`suggest` and `search_cards` are absent on purpose: an interview that
    offers a replacement card has stopped interviewing and started
    recommending, which is a different mode with a different stance."""
    named = set(interview.RATIONALE_INTERVIEW.tool_names)
    assert "suggest_replacements" not in named
    assert "search_cards" not in named


def test_the_scope_axis_changes_the_prompt_and_the_tool_set_never_moves():
    """A stance may widen what a mode does, never what it may do (ADR 15)."""
    mode = interview.RATIONALE_INTERVIEW
    prompts = {s: mode.system(stance.Stance("on-request", s, "none"))
               for s in stance.SCOPE}
    assert len(set(prompts.values())) == 3, "scope must change the prompt"
    for text in prompts.values():
        assert mode.instructions in text
    assert len({tuple(mode.tool_names) for _ in stance.SCOPE}) == 1


# ------------------------------------------------------------- the schema

def test_the_response_schema_has_nowhere_to_put_a_rationale():
    """The structural half of rule 4 in this mode.

    A model cannot hand over a draft rationale it has no field for, and
    `additionalProperties: false` means it cannot invent one at run time
    either. Checked by name against the words a laundering field would
    plausibly be called.
    """
    item = interview.RESPONSE_SCHEMA["properties"]["questions"]["items"]
    assert item["additionalProperties"] is False
    assert interview.RESPONSE_SCHEMA["additionalProperties"] is False
    forbidden = {"why", "rationale", "draft", "suggestion", "summary",
                 "text", "answer", "justification"}
    assert not (set(item["properties"]) & forbidden), (
        f"{sorted(set(item['properties']) & forbidden)} would be a place to "
        f"put a rationale. See ADR 15.")


def test_every_question_field_is_required_so_a_fact_cannot_be_skipped():
    item = interview.RESPONSE_SCHEMA["properties"]["questions"]["items"]
    assert set(item["required"]) == {"question", "angle", "fact"}


# ------------------------------------------------- questions, and only those

@pytest.mark.parametrize("text", [
    "Sol Ring is the best rock in the format.",
    "Consider whether the deck needs more ramp.",
    "You might say it fixes colours and accelerates.",
    "",
    "?",
])
def test_anything_that_is_not_a_question_is_dropped(text):
    kept, dropped = interview.only_questions(
        [{"question": text, "angle": "role", "fact": "f"}])
    assert kept == []
    assert dropped == 1


def test_questions_survive_and_keep_their_grounding():
    kept, dropped = interview.only_questions([
        {"question": "What does this beat out at two mana?",
         "angle": "cost", "fact": "Nine cards at mana value 2."},
        {"question": "  Which of these two is really doing the job?  ",
         "angle": "redundancy", "fact": "Two ramp spells claim the same role."},
    ])
    assert dropped == 0
    assert [q["angle"] for q in kept] == ["cost", "redundancy"]
    assert kept[1]["question"].startswith("Which"), "must be stripped"
    assert kept[0]["fact"] == "Nine cards at mana value 2."


def test_a_malformed_item_is_dropped_rather_than_crashing():
    kept, dropped = interview.only_questions(["not a dict", None, {}])
    assert kept == []
    assert dropped == 3


# ---------------------------------------------------------------- the brief

def test_the_brief_assembles_without_a_corpus(source, no_network):
    """A fresh clone has no `data/mtg.duckdb`. The brief still has to render,
    saying the card is unknown rather than pretending to know it."""
    facts = interview.brief("mini", "Sol Ring", source=source)
    assert facts["card"]["name"] == "Sol Ring"
    assert facts["card"]["category"] == "ramp"
    assert facts["deck"]["slug"] == "mini"
    assert isinstance(facts["card"]["in_corpus"], bool)


def test_the_brief_matches_a_card_the_way_someone_would_type_it(source, no_network):
    facts = interview.brief("mini", "  sol ring  ", source=source)
    assert facts["card"]["name"] == "Sol Ring", "the corpus's spelling comes back"


def test_a_card_the_deck_does_not_run_is_refused(source, no_network):
    with pytest.raises(interview.CardNotInDeck, match="Black Lotus"):
        interview.brief("mini", "Black Lotus", source=source)


def test_the_brief_carries_the_sibling_rationales(source, no_network):
    """The most useful question this mode asks is 'which of these two is
    actually doing the job', and it can only ask it if it can see what was
    claimed for the neighbours."""
    facts = interview.brief("mini", "Sol Ring", source=source)
    siblings = {s["name"]: s["why"] for s in facts["category"]["other_cards_in_it"]}
    assert "Arcane Signet" in siblings
    assert siblings["Arcane Signet"].startswith("Two mana for a rock")
    assert "Sol Ring" not in siblings, "a card is not its own neighbour"


def test_the_brief_reports_the_rationale_already_written(source, no_network):
    """Blank and non-blank are different interviews: one finds the reason, the
    other interrogates the one on the page."""
    assert interview.brief("mini", "Sol Ring", source=source)["card"][
        "rationale_so_far"] == ""
    assert interview.brief("mini", "Arcane Signet", source=source)["card"][
        "rationale_so_far"]


def test_the_brief_carries_the_gates_verdict_not_its_own(source, no_network):
    facts = interview.brief("mini", "Primeval Titan", source=source)
    assert "about_this_card" in facts["gate"]
    assert isinstance(facts["gate"]["deck_errors"], int)


# ------------------------------------------------------------------- asking

def test_off_makes_no_call_at_all(source, no_network):
    """The single most important line in the stance module, exercised through
    the mode that would otherwise spend the money. `no_network` turns any
    attempt into a failure, so this passes only if nothing was tried."""
    report = interview.ask("mini", "Sol Ring", requested="off", source=source)
    assert report["asked"] is False
    assert report["questions"] == []
    assert "off" in report["reason"]


def test_a_normal_run_returns_questions_and_says_who_answered(source, monkeypatch):
    monkeypatch.setattr(interview, "converse", lambda *a, **k: turn_returning(
        '{"questions": ['
        '{"question": "What does this do that Arcane Signet does not?",'
        ' "angle": "redundancy", "fact": "Both are ramp."},'
        '{"question": "What would make you cut it?", "angle": "cut",'
        ' "fact": "Four ramp sources."}]}',
        tool_calls=[{"tool": "get_cards", "arguments": {"names": ["Sol Ring"]}}]))

    report = interview.ask("mini", "Sol Ring", requested="consultant",
                           source=source)
    assert report["asked"] is True
    # ADR 14 boundary 3: never present an opinion as the gate's output.
    assert report["answered_by"] == "claude"
    assert report["model"] == "claude-sonnet-5-test"
    assert len(report["questions"]) == 2
    assert report["questions_dropped"] == 0
    assert report["tool_calls"][0]["tool"] == "get_cards"
    assert "yours to write" in report["never"]


def test_a_statement_dressed_as_a_question_never_reaches_the_caller(source, monkeypatch):
    """The slope, caught at the last step. A model that answers with a draft
    rationale gets it dropped and the count reported, rather than having it
    rendered next to an empty rationale box."""
    monkeypatch.setattr(interview, "converse", lambda *a, **k: turn_returning(
        '{"questions": ['
        '{"question": "Sol Ring is simply the best rock in Commander.",'
        ' "angle": "role", "fact": "Costs one mana."},'
        '{"question": "What is it accelerating you into?", "angle": "role",'
        ' "fact": "Bracket 3."}]}'))

    report = interview.ask("mini", "Sol Ring", requested="consultant",
                           source=source)
    assert [q["question"] for q in report["questions"]] == [
        "What is it accelerating you into?"]
    assert report["questions_dropped"] == 1


def test_an_unparseable_answer_is_reported_rather_than_guessed_at(source, monkeypatch):
    monkeypatch.setattr(interview, "converse",
                        lambda *a, **k: turn_returning("not json at all"))
    report = interview.ask("mini", "Sol Ring", requested="consultant",
                           source=source)
    assert report["questions"] == []
    assert "did not parse" in report["reason"]


def test_a_refusal_comes_back_labelled(source, monkeypatch):
    refused = turn_returning("", stop_reason="refusal")
    refused.refused = True
    monkeypatch.setattr(interview, "converse", lambda *a, **k: refused)
    report = interview.ask("mini", "Sol Ring", requested="consultant",
                           source=source)
    assert report["questions"] == []
    assert "declined" in report["reason"]


def test_more_questions_than_anyone_can_answer_are_trimmed(source, monkeypatch):
    many = ", ".join(
        f'{{"question": "Question {i}?", "angle": "role", "fact": "f"}}'
        for i in range(12))
    monkeypatch.setattr(interview, "converse", lambda *a, **k: turn_returning(
        '{"questions": [' + many + ']}'))
    report = interview.ask("mini", "Sol Ring", requested="consultant",
                           source=source)
    assert len(report["questions"]) == interview.MAX_QUESTIONS


def test_the_stance_is_clamped_to_the_deployment_ceiling(source, monkeypatch):
    """A hosted instance capping the dial is not something a client can ask
    its way past, and the interview reports what actually applied."""
    monkeypatch.setenv(stance.CEILING_ENV, "off")
    report = interview.ask("mini", "Sol Ring", requested="collaborator",
                           source=source)
    assert report["asked"] is False
    assert report["stance"]["preset"] == "off"
