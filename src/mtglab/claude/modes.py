"""What a mode is, and the loop that runs one.

[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
defines a Claude surface as four things: a system prompt, a tool set, a
declaration of what it may write, and the user's stance over it. The first
three are the mode and live here; the fourth is `stance.py` and is the user's.

**The write declaration is a field, and it is checked empty.** Every mode ADR
15 names may write nothing, so `may_write` could have been left out entirely
and the code would behave identically. It is here because the ADR's claim is
that a mode *is* a capability declaration -- a field that exists and is
asserted empty says that out loud, and it is the line a future ADR would have
to change deliberately rather than a silence somebody could fill in by
accident. The package has no write door regardless: `tools.READ_ONLY` is the
whole surface and `tests/test_claude_boundary.py` fails on the commit that
names a write function anywhere under this directory.

**The loop is here rather than in the interview** because ADR 15 names four
modes and this is the first. Everything mode-specific -- which facts to
assemble, how to read the answer back -- belongs to the mode's own module;
what belongs here is the part that would otherwise be copied four times, which
is the request shape and the tool round trip.

Two things about the request shape, both Sonnet 5 specifics that `client.py`
pins and this module depends on: `output_config` carries **both** the effort
and the response format, and adaptive thinking runs by default -- so
`max_tokens` is a ceiling over thinking and answer together and needs headroom
that a non-thinking model would not have wanted.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any

from mtglab.claude import client, tools
from mtglab.claude.stance import Stance
from mtglab.decks.source import DeckNotFound, DeckSource


class ModeExhausted(Exception):
    """The tool loop hit its turn limit without the model finishing.

    Raised rather than returning whatever the last turn happened to say. A
    truncated answer that looks complete is the failure mode worth avoiding
    here -- the same shape as a Forge game that plays on with 96 cards.
    """


@dataclass(frozen=True)
class Mode:
    """One Claude surface: a prompt, a tool set, and what it may write."""

    name: str
    purpose: str
    #: The system prompt, minus anything the stance contributes.
    instructions: str
    #: A subset of `tools.READ_ONLY`. Naming one that does not exist is a
    #: `ToolNotAllowed` at construction rather than a surprise at call time.
    tool_names: tuple[str, ...]
    #: What this mode may change. Empty, and checked empty -- see the module
    #: docstring for why the field exists at all.
    may_write: tuple[str, ...] = ()
    #: Thinking and answer share this ceiling on Sonnet 5.
    max_tokens: int = 8192
    #: `high` deliberately, and not for depth. Lower effort levels reach for
    #: tools less often, and a mode that answers from recall instead of calling
    #: `get_cards` is rule 1 failing quietly. The interview also hands the
    #: corpus facts over in the opening brief rather than relying on this, but
    #: the two together are cheaper than finding out which one was load-bearing.
    effort: str = "high"
    #: A JSON schema for the final answer, or None for prose.
    response_schema: dict[str, Any] | None = field(default=None)

    def __post_init__(self) -> None:
        if self.may_write:
            raise ValueError(
                f"mode {self.name!r} declares it may write {list(self.may_write)}. "
                f"No mode may write anything (ADR 15) -- and this package "
                f"cannot reach a write path in any case. Changing that needs a "
                f"new ADR superseding 15, not a value here.")
        # Raises `ToolNotAllowed` for a name outside the registry, so a typo in
        # a mode definition fails at import rather than mid-conversation.
        tools.schemas(self.tool_names)

    def schemas(self) -> list[dict[str, Any]]:
        return tools.schemas(self.tool_names)

    def system(self, stance: Stance) -> str:
        """The system prompt, with what the stance widens appended.

        A stance may widen what a mode *does* and never what it is allowed to
        do (ADR 15), which is exactly the difference between this method and
        `schemas()`: the tool set does not move, the framing does.
        """
        return f"{self.instructions.strip()}\n\n{_scope_note(stance).strip()}"


def _scope_note(stance: Stance) -> str:
    """How far from the question this stance lets a mode range.

    Only the scope axis says anything here, and that is deliberate rather than
    an oversight. **Initiative** decides whether a call happens at all -- `off`
    makes none, and above that the interview is invoked by someone clicking a
    button, so there is nothing left for it to gate. **Write autonomy** is
    moot: no mode may write. Inventing behaviour for the other two axes so the
    function looks symmetrical would be pretending the dial does more than it
    does.
    """
    return {
        "flagged": (
            "Scope: stay on the card you were asked about, and on anything the "
            "gate flagged about it. Do not range into the rest of the deck."),
        "adjacent": (
            "Scope: the card you were asked about, plus the cards that "
            "actually interact with it -- others in its category, cards it "
            "needs in play, cards that do its job more cheaply. You may ask "
            "about those in service of the question at hand."),
        "rethink": (
            "Scope: the whole deck, including its axis. If the honest question "
            "is whether this card's *kind* of card belongs here at all -- "
            "whether the deck is trying to do two things at once, whether the "
            "commander wants a different shape -- ask that. It is still a "
            "question, not a plan."),
    }[stance.scope]


# ------------------------------------------------------------------ the loop

@dataclass
class Turn:
    """What one exchange produced, including how it got there.

    `tool_calls` is kept because ADR 14's third boundary is that a user can
    tell which system answered. For an opinion assembled from six corpus
    lookups, "which system" is only half the answer -- the other half is what
    it read, and a caller that wants to show its working needs the list.
    """

    mode: str
    model: str
    stop_reason: str
    text: str
    tool_calls: list[dict[str, Any]]
    input_tokens: int
    output_tokens: int
    refused: bool = False

    def parsed(self) -> Any:
        """The answer as JSON, for a mode that constrained its format."""
        return json.loads(self.text)


#: Enough for a lookup, a search and a reconsider. A mode that has not
#: finished by then is looping rather than working, and the ceiling turns that
#: into an exception instead of a bill.
MAX_TOOL_TURNS = 6


def converse(mode: Mode, *, messages: list[dict[str, Any]], stance: Stance,
             source: DeckSource | None = None,
             max_turns: int = MAX_TOOL_TURNS) -> Turn:
    """Run `mode` over `messages` until it stops asking for tools.

    The caller is responsible for having checked `stance.allows_calls` first.
    That check is not repeated here on purpose: a function that silently did
    nothing when the stance was `off` would be indistinguishable from one that
    ran and found nothing to say, and "off means no calls" deserves a caller
    that had to decide rather than a default that happened.
    """
    con = client.connect()
    schemas = mode.schemas()
    output_config: dict[str, Any] = {"effort": mode.effort}
    if mode.response_schema is not None:
        output_config["format"] = {"type": "json_schema",
                                   "schema": mode.response_schema}

    history = list(messages)
    calls: list[dict[str, Any]] = []
    tokens_in = tokens_out = 0

    for _ in range(max_turns):
        resp = con.messages.create(
            model=client.model(),
            max_tokens=mode.max_tokens,
            system=mode.system(stance),
            tools=schemas,
            output_config=output_config,
            messages=history,
        )
        tokens_in += resp.usage.input_tokens
        tokens_out += resp.usage.output_tokens

        # Checked before `content` is read, because a refusal can carry an
        # empty content list and indexing into it is how this becomes an
        # IndexError instead of a message somebody can act on.
        if resp.stop_reason == "refusal":
            return Turn(mode=mode.name, model=resp.model,
                        stop_reason=resp.stop_reason, text="", tool_calls=calls,
                        input_tokens=tokens_in, output_tokens=tokens_out,
                        refused=True)

        # Appended whole, thinking blocks included. Sonnet 5 returns them with
        # empty text by default and they still have to go back unedited.
        history.append({"role": "assistant", "content": resp.content})

        if resp.stop_reason != "tool_use":
            text = "".join(b.text for b in resp.content
                           if b.type == "text").strip()
            return Turn(mode=mode.name, model=resp.model,
                        stop_reason=resp.stop_reason, text=text,
                        tool_calls=calls, input_tokens=tokens_in,
                        output_tokens=tokens_out)

        results = []
        for block in resp.content:
            if block.type != "tool_use":
                continue
            arguments = dict(block.input)
            calls.append({"tool": block.name, "arguments": arguments})
            try:
                out = tools.run(block.name, arguments, source=source,
                                allowed=mode.tool_names)
                content, is_error = json.dumps(out, default=str), False
            except (tools.ToolNotAllowed, tools.ToolArgumentsRejected,
                    DeckNotFound) as exc:
                # Handed back as a tool result rather than raised. All three
                # are things the model can recover from by asking differently,
                # and a refused `set_card_field` in particular should read to
                # the model as "that door does not exist" rather than ending
                # the conversation.
                content, is_error = f"{type(exc).__name__}: {exc}", True
            results.append({"type": "tool_result", "tool_use_id": block.id,
                            "content": content, "is_error": is_error})
        history.append({"role": "user", "content": results})

    raise ModeExhausted(
        f"{mode.name} still wanted tools after {max_turns} turns "
        f"({len(calls)} calls made). Nothing was written; nothing is half-done.")
