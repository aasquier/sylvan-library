"""The conversation loop, with a stub in place of the Anthropic client.

`converse` was the least-covered function in the package at 55%, and it is the
one place where a model's output meets this project's tools. That combination
is worth pinning: it is where a refusal has to be distinguished from an answer,
where a tool error has to come back as a tool result rather than an exception,
and where the turn ceiling has to stop a loop rather than run up a bill.

**No network and no key.** `client.connect()` is monkeypatched to a stub that
replays a scripted list of responses, so these assert what the loop does with
what a model says, not what a model says. `mtglab claude check` remains the
thing that proves the pipe, deliberately a command rather than a test.
"""

import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab.claude import client, modes, tools
from mtglab.claude.modes import Mode, ModeExhausted, converse
from mtglab.claude.stance import Stance
from mtglab.decks.model import Deck
from mtglab.decks.source import MemoryDeckSource

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


# ------------------------------------------------------- the stubbed SDK
#
# Shaped to the attributes `converse` actually reads, and no further. A
# `MagicMock` would answer every attribute, including the ones a future
# refactor gets wrong -- which is the opposite of what a test should do here.

@dataclass
class _Text:
    text: str
    type: str = "text"


@dataclass
class _ToolUse:
    name: str
    input: dict[str, Any]
    id: str = "tu_1"
    type: str = "tool_use"


@dataclass
class _Usage:
    input_tokens: int = 10
    output_tokens: int = 5
    # None by default, the way an SDK that predates the field -- or a platform
    # that does not report it -- would answer. The accounting must read that
    # as zero rather than crash or sum None.
    cache_read_input_tokens: int | None = None


@dataclass
class _Response:
    content: list[Any]
    stop_reason: str = "end_turn"
    model: str = "claude-sonnet-5"
    usage: _Usage = field(default_factory=_Usage)


class _Messages:
    def __init__(self, script: list[_Response]) -> None:
        self._script = list(script)
        self.calls: list[dict[str, Any]] = []

    def create(self, **kwargs: Any) -> _Response:
        # `messages` is snapshotted rather than stored. `converse` keeps one
        # `history` list and appends to it between turns, so holding the
        # reference would make every recorded call show the *final* state --
        # and an assertion about what turn 2 was sent would silently be an
        # assertion about turn 3.
        self.calls.append({**kwargs, "messages": list(kwargs["messages"])})
        if not self._script:
            raise AssertionError("the loop asked for more turns than scripted")
        return self._script.pop(0)


class _Client:
    def __init__(self, script: list[_Response]) -> None:
        self.messages = _Messages(script)


@pytest.fixture
def scripted(monkeypatch):
    """Install a stub client that replays `script`, and hand back the stub."""
    def install(script: list[_Response]) -> _Client:
        stub = _Client(script)
        monkeypatch.setattr(client, "connect", lambda: stub)
        monkeypatch.setattr(client, "model", lambda: "claude-sonnet-5")
        return stub
    return install


@pytest.fixture
def source(tmp_path):
    path = tmp_path / "deck.yaml"
    path.write_text(DECK_YAML, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


MODE = Mode(
    name="test-mode",
    purpose="A mode that exists to exercise the loop.",
    instructions="Answer the question.",
    tool_names=("get_deck", "get_cards"),
)

ASK = [{"role": "user", "content": "why is this card here?"}]


# --------------------------------------------------------------- the loop

def test_a_plain_answer_comes_back_with_its_accounting(scripted):
    stub = scripted([_Response([_Text("Because it ramps.")])])
    turn = converse(MODE, messages=ASK, stance=Stance())

    assert turn.text == "Because it ramps."
    assert turn.mode == "test-mode"
    assert turn.stop_reason == "end_turn"
    assert turn.tool_calls == []
    assert (turn.input_tokens, turn.output_tokens) == (10, 5)
    assert len(stub.messages.calls) == 1


def test_the_mode_sets_the_model_tools_and_effort(scripted):
    """ADR 15: a mode *is* a system prompt, a tool set and an effort. If any of
    those stopped reaching the API the mode would still answer, just not as the
    mode it claims to be."""
    stub = scripted([_Response([_Text("ok")])])
    converse(MODE, messages=ASK, stance=Stance())

    sent = stub.messages.calls[0]
    assert sent["model"] == "claude-sonnet-5"
    assert {t["name"] for t in sent["tools"]} == {"get_deck", "get_cards"}
    assert sent["output_config"]["effort"] == "high"
    assert "Answer the question." in sent["system"][0]["text"]


def test_the_system_block_carries_a_cache_breakpoint(scripted):
    """Tools render ahead of system, so one marker on the system block caches
    both -- the byte-stable prefix -- while the per-call brief stays outside.
    Losing this silently costs ~10x on every repeated prefix token and nothing
    visibly breaks, which is why it is pinned."""
    stub = scripted([_Response([_Text("ok")])])
    converse(MODE, messages=ASK, stance=Stance())

    system = stub.messages.calls[0]["system"]
    assert isinstance(system, list) and len(system) == 1
    assert system[0]["cache_control"] == {"type": "ephemeral"}


def test_cache_reads_are_accounted_when_the_api_reports_them(scripted, source):
    """`usage.cache_read_input_tokens` is the only evidence the cache works;
    a Turn that dropped it would leave callers unable to check."""
    scripted([
        _Response([_ToolUse("get_deck", {"slug": "mini"})], stop_reason="tool_use",
                  usage=_Usage(cache_read_input_tokens=1500)),
        _Response([_Text("done")], usage=_Usage(cache_read_input_tokens=1600)),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)
    assert turn.cache_read_tokens == 3100


def test_a_usage_without_cache_reads_reports_zero(scripted):
    """The default _Usage answers None for the cache field, mirroring an SDK
    or platform that does not report one -- accounting must degrade to zero,
    not crash or sum None."""
    scripted([_Response([_Text("ok")])])
    turn = converse(MODE, messages=ASK, stance=Stance())
    assert turn.cache_read_tokens == 0


def test_a_response_schema_is_passed_as_the_output_format(scripted):
    schema = {"type": "object", "properties": {"q": {"type": "string"}}}
    structured = Mode(name="m", purpose="p", instructions="i",
                      tool_names=("get_deck",), response_schema=schema)
    stub = scripted([_Response([_Text("{}")])])
    converse(structured, messages=ASK, stance=Stance())

    fmt = stub.messages.calls[0]["output_config"]["format"]
    assert fmt == {"type": "json_schema", "schema": schema}


def test_a_mode_without_a_schema_asks_for_no_format(scripted):
    stub = scripted([_Response([_Text("prose")])])
    converse(MODE, messages=ASK, stance=Stance())
    assert "format" not in stub.messages.calls[0]["output_config"]


def test_a_tool_call_is_run_and_its_result_fed_back(scripted, source):
    stub = scripted([
        _Response([_ToolUse("get_deck", {"slug": "mini"})], stop_reason="tool_use"),
        _Response([_Text("It ramps.")]),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)

    assert turn.text == "It ramps."
    assert turn.tool_calls == [{"tool": "get_deck", "arguments": {"slug": "mini"}}]
    # Two turns: the tool call, then the answer that used its result.
    assert len(stub.messages.calls) == 2

    # The result went back as a tool_result the model can read.
    followup = stub.messages.calls[1]["messages"][-1]
    assert followup["role"] == "user"
    result = followup["content"][0]
    assert result["type"] == "tool_result"
    assert result["tool_use_id"] == "tu_1"
    assert result["is_error"] is False
    assert json.loads(result["content"])["slug"] == "mini"


def test_tokens_accumulate_across_turns(scripted, source):
    """Billing is per turn, and a caller shown only the last one would be told
    a multi-tool conversation cost what its final exchange cost."""
    scripted([
        _Response([_ToolUse("get_deck", {"slug": "mini"})], stop_reason="tool_use"),
        _Response([_Text("done")]),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)
    assert (turn.input_tokens, turn.output_tokens) == (20, 10)


def test_the_assistant_turn_is_returned_unedited(scripted, source):
    """Thinking blocks come back with empty text on Sonnet 5 and still have to
    be echoed verbatim -- reconstructing the turn from its text drops them."""
    blocks = [_ToolUse("get_deck", {"slug": "mini"})]
    stub = scripted([_Response(blocks, stop_reason="tool_use"),
                     _Response([_Text("done")])])
    converse(MODE, messages=ASK, stance=Stance(), source=source)

    assistant = stub.messages.calls[1]["messages"][-2]
    assert assistant["role"] == "assistant"
    assert assistant["content"] is blocks


# ------------------------------------------------------------- the refusals

def test_a_refusal_is_reported_rather_than_indexed_into(scripted):
    """A refusal can carry an empty content list. Reading `content[0]` is how
    that becomes an IndexError instead of something a caller can show."""
    scripted([_Response([], stop_reason="refusal")])
    turn = converse(MODE, messages=ASK, stance=Stance())

    assert turn.refused is True
    assert turn.text == ""
    assert turn.stop_reason == "refusal"


def test_a_rejected_tool_comes_back_as_a_tool_result_not_an_exception(scripted,
                                                                     source):
    """A model asking for a tool that does not exist should learn that the door
    does not exist, and carry on -- not end the conversation."""
    stub = scripted([
        _Response([_ToolUse("set_card_field", {"slug": "mini"})],
                  stop_reason="tool_use"),
        _Response([_Text("understood")]),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)

    assert turn.text == "understood"
    result = stub.messages.calls[1]["messages"][-1]["content"][0]
    assert result["is_error"] is True
    assert "ToolNotAllowed" in result["content"]


def test_a_registered_tool_outside_the_mode_is_refused(scripted, source):
    """ADR 15 says a mode *is* a tool set, and this is where that holds at the
    door rather than in the advertisement: `converse` dispatches with
    `allowed=mode.tool_names`, so a *registered* read-only tool the mode did
    not offer -- here `search_cards`, which MODE does not carry -- is refused
    exactly like an unregistered one, and the refusal names the mode's own
    tools so the model can recover."""
    stub = scripted([
        _Response([_ToolUse("search_cards", {"text": "ramp"})],
                  stop_reason="tool_use"),
        _Response([_Text("noted")]),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)

    assert turn.text == "noted"
    result = stub.messages.calls[1]["messages"][-1]["content"][0]
    assert result["is_error"] is True
    assert "ToolNotAllowed" in result["content"]
    assert "not offered by this mode" in result["content"]
    assert "get_cards" in result["content"]


def test_bad_tool_arguments_also_come_back_as_a_result(scripted, source):
    stub = scripted([
        _Response([_ToolUse("get_deck", {"slug": "no-such-deck"})],
                  stop_reason="tool_use"),
        _Response([_Text("ok")]),
    ])
    converse(MODE, messages=ASK, stance=Stance(), source=source)
    result = stub.messages.calls[1]["messages"][-1]["content"][0]
    assert result["is_error"] is True


def test_a_loop_that_never_stops_asking_hits_the_ceiling(scripted, source):
    """The bill, not the correctness, is what this protects. The message has to
    say nothing was written, because that is the question somebody asks first."""
    scripted([_Response([_ToolUse("get_deck", {"slug": "mini"})],
                        stop_reason="tool_use") for _ in range(4)])
    with pytest.raises(ModeExhausted) as exc:
        converse(MODE, messages=ASK, stance=Stance(), source=source,
                 max_turns=4)
    assert "Nothing was written" in str(exc.value)
    assert "4 turns" in str(exc.value)


def test_non_tool_blocks_in_a_tool_turn_are_skipped(scripted, source):
    """A turn can mix prose and tool calls. Only the tool blocks are runnable,
    and the prose must not be handed to `tools.run`."""
    stub = scripted([
        _Response([_Text("let me look"), _ToolUse("get_deck", {"slug": "mini"})],
                  stop_reason="tool_use"),
        _Response([_Text("done")]),
    ])
    turn = converse(MODE, messages=ASK, stance=Stance(), source=source)
    assert len(turn.tool_calls) == 1
    assert len(stub.messages.calls[1]["messages"][-1]["content"]) == 1


def test_only_text_blocks_contribute_to_the_answer(scripted):
    scripted([_Response([_Text("first."), _Text(" second.")])])
    turn = converse(MODE, messages=ASK, stance=Stance())
    assert turn.text == "first. second."


# ---------------------------------------------------------------- the mode

def test_a_mode_may_not_declare_a_write():
    """ADR 15, enforced at construction. `tests/test_claude_boundary.py` is the
    structural half; this is the one a mode author trips over first."""
    with pytest.raises(ValueError) as exc:
        Mode(name="bad", purpose="p", instructions="i",
             tool_names=("get_deck",), may_write=("deck.yaml",))
    assert "No mode may write anything" in str(exc.value)


def test_a_typo_in_a_tool_name_fails_at_construction():
    """Rather than mid-conversation, which is when it would otherwise surface."""
    with pytest.raises(tools.ToolNotAllowed):
        Mode(name="bad", purpose="p", instructions="i",
             tool_names=("get_dekc",))


def test_the_stance_widens_the_prompt_without_widening_the_tools():
    """The whole of ADR 15's second half in one assertion: a stance changes the
    framing, never the capability."""
    narrow, wide = Stance(scope="flagged"), Stance(scope="rethink")
    assert MODE.system(narrow) != MODE.system(wide)
    assert {t["name"] for t in MODE.schemas()} == {"get_deck", "get_cards"}


def test_max_tool_turns_is_a_documented_ceiling_not_a_magic_number():
    assert modes.MAX_TOOL_TURNS >= 3
