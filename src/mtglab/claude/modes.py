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

**Server tools are a second kind of tool and the loop treats them differently.**
`tool_names` are this package's own, dispatched through `tools.run`;
`server_tools` run on Anthropic's side and never reach that door. They arrive
with the dossier ([ADR 19](../../../docs/adr/0019-the-dossier-cites-three-sources.md))
and bring two things the loop did not previously handle. Their results are
*evidence* -- with a response schema in play the API attaches no citations, so
the pages a search actually returned are collected here and are the only thing
a caller can check an answer's sources against. And a server-side tool loop
that hits its own limit stops with `pause_turn`, carrying text that looks like
a finished answer; that is resumed rather than returned, for the same reason
`sim/tier3` refuses to report a game Forge played with 96 cards.
"""

from __future__ import annotations

import json
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from mtglab.claude import client, ledger, tools
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
    #: Anthropic-hosted tools, passed through verbatim. These run on Anthropic's
    #: side and produce no round trip here, which is exactly why they are a
    #: separate field from `tool_names`: `tools.run` is the door this package
    #: controls, and a server tool does not go through it. Only the dossier uses
    #: one (`web_search_20260209`, ADR 19), and `CLAUDE.md`'s no-crawler rule is
    #: the reason it is a *hosted* search rather than something here that fetches.
    server_tools: tuple[dict[str, Any], ...] = ()
    #: What this mode may change. Empty, and checked empty -- see the module
    #: docstring for why the field exists at all.
    may_write: tuple[str, ...] = ()
    #: Thinking and answer share this ceiling on Sonnet 5.
    max_tokens: int = 8192
    #: `high` deliberately, and not for depth. Lower effort levels reach for
    #: tools less often, and a mode that answers from recall instead of calling
    #: `get_cards` is rule 1 failing quietly. The interview also hands the
    #: pool facts over in the opening brief rather than relying on this, but
    #: the two together are cheaper than finding out which one was load-bearing.
    effort: str = "high"
    #: A JSON schema for the final answer, or None for prose.
    response_schema: dict[str, Any] | None = field(default=None)
    #: What each scope level means *for this mode*, keyed by the scope axis's
    #: levels. `None` uses `_SCOPE_NOTES` below, which is written for the modes
    #: that are about one card in one deck -- and reads as nonsense to one that
    #: is not. A mode with no card and no deck (research, ADR 26) has to say
    #: what its own scope axis widens, or the prompt tells it to stay on
    #: something that does not exist.
    scope_notes: tuple[tuple[str, str], ...] | None = None

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
        """Everything for the request's `tools`: ours, then Anthropic's.

        Server tools go last and in declaration order so the rendered block
        stays byte-stable -- tools render first in the prompt, so an unstable
        order would invalidate the prompt cache on every call, for free and
        invisibly. `tools.schemas` already sorts our half for the same reason.
        """
        return [*tools.schemas(self.tool_names), *(dict(t) for t in self.server_tools)]

    def system(self, stance: Stance) -> str:
        """The system prompt, with what the stance widens appended.

        A stance may widen what a mode *does* and never what it is allowed to
        do (ADR 15), which is exactly the difference between this method and
        `schemas()`: the tool set does not move, the framing does.
        """
        return f"{self.instructions.strip()}\n\n{_scope_note(stance, self.scope_notes).strip()}"


def _scope_note(stance: Stance,
                overrides: tuple[tuple[str, str], ...] | None = None) -> str:
    """How far from the question this stance lets a mode range.

    Only the scope axis says anything here, and that is deliberate rather than
    an oversight. **Initiative** decides whether a call happens at all -- `off`
    makes none, and above that the interview is invoked by someone clicking a
    button, so there is nothing left for it to gate. **Write autonomy** is
    moot: no mode may write. Inventing behaviour for the other two axes so the
    function looks symmetrical would be pretending the dial does more than it
    does.

    `overrides` exists because the table below is not universal: it talks about
    "the card you were asked about", which is exactly right for the interview
    and the slot argument and meaningless to a mode that was not asked about a
    card. A mode supplies its own table or gets this one; either way the text
    is byte-stable per mode and stance, which is what the prompt cache needs.
    """
    if overrides is not None:
        return dict(overrides)[stance.scope]
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
    tell which system answered. For an opinion assembled from six pool
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
    #: Every page a server-side search actually returned, in the order it came
    #: back. This is evidence, not decoration: with a response schema in play
    #: the API attaches no citations to the answer, so the only way to tell a
    #: page the model read from a URL it composed is to check the answer's
    #: citations against this list. ADR 19 makes that check mandatory, and
    #: `dossier.py` is where it happens.
    searched: list[dict[str, Any]] = field(default_factory=list)
    #: Search errors, kept rather than dropped. A search that returned nothing
    #: and a search that failed look identical from an empty `searched`, and
    #: they want different answers.
    search_errors: list[str] = field(default_factory=list)
    refused: bool = False
    #: Prompt tokens served from the cache, at ~a tenth of the input price.
    #: **Counted beside `input_tokens`, not inside it** -- the API reports
    #: `input_tokens` as the uncached remainder only, so this conversation's
    #: whole prompt is `input_tokens + cache_read_tokens` (plus cache writes,
    #: which nothing here records). Reading it as a slice understates the
    #: prompt by however well the cache worked, which is backwards.
    #:
    #: Zero on a cold call; if it stays zero across repeated calls of the same
    #: mode, the prefix is drifting and the breakpoint in `converse` is buying
    #: nothing -- that is the number to check, per the caching guidance,
    #: before believing the cache works.
    cache_read_tokens: int = 0

    def parsed(self) -> Any:
        """The answer as JSON, for a mode that constrained its format."""
        return json.loads(self.text)


#: Enough for a lookup, a search and a reconsider. A mode that has not
#: finished by then is looping rather than working, and the ceiling turns that
#: into an exception instead of a bill.
MAX_TOOL_TURNS = 6


def _server_results(content: Any) -> tuple[list[dict[str, Any]], list[str]]:
    """Pages a hosted search returned this turn, and any search that failed.

    Two shapes matter and they are easy to confuse. On success a
    `web_search_tool_result` block's `content` is a **list** of results; on
    failure it is a single **error object**. Indexing the second as the first
    is how a failed search becomes an empty page list that reads as "found
    nothing" -- so the type is checked rather than assumed.
    """
    pages: list[dict[str, Any]] = []
    errors: list[str] = []
    for block in content:
        if getattr(block, "type", None) != "web_search_tool_result":
            continue
        results = getattr(block, "content", None)
        if not isinstance(results, list):
            errors.append(str(getattr(results, "error_code", None) or results))
            continue
        for item in results:
            url = getattr(item, "url", None)
            if url:
                pages.append({"url": url,
                              "title": getattr(item, "title", "") or url})
    return pages, errors


def converse(mode: Mode, *, messages: list[dict[str, Any]], stance: Stance,
             source: DeckSource | None = None,
             tier: str | None = None,
             max_turns: int = MAX_TOOL_TURNS,
             on_turn: Callable[[int, int], None] | None = None) -> Turn:
    """Run `mode` over `messages` until it stops asking for tools.

    The caller is responsible for having checked `stance.allows_calls` first.
    That check is not repeated here on purpose: a function that silently did
    nothing when the stance was `off` would be indistinguishable from one that
    ran and found nothing to say, and "off means no calls" deserves a caller
    that had to decide rather than a default that happened.

    `on_turn(done, max_turns)` fires as each model turn begins, and exists for
    the one mode slow enough to be a background job: the theme proposal takes
    minutes, and a job that reports nothing for that long is indistinguishable
    from a wedged one. It is a **ceiling and not an estimate** -- a loop that
    finishes on turn four of eight jumps straight to done, which `jobs.submit`
    already squares up. Anything more truthful would need to know in advance
    how many searches the model was going to run.

    `tier` is the asking account's model grant (`tiers.py`), resolved **once**
    here rather than per turn: a conversation that changed model halfway would
    throw away its prompt cache -- caches are model-scoped -- and would put two
    models' answers in one transcript. `None` is every account by default and
    means the house model. A background job captures the tier at plan time, the
    way it captures a `DeckSource`, because a job outlives the request that
    knew who was asking.
    """
    con = client.connect()
    answering = client.model(tier)
    schemas = mode.schemas()
    output_config: dict[str, Any] = {"effort": mode.effort}
    if mode.response_schema is not None:
        output_config["format"] = {"type": "json_schema",
                                   "schema": mode.response_schema}

    history = list(messages)
    calls: list[dict[str, Any]] = []
    pages: list[dict[str, Any]] = []
    search_errors: list[str] = []
    seen_urls: set[str] = set()
    container: str | None = None
    tokens_in = tokens_out = tokens_cached = 0
    # The one *moving* cache marker, on the newest tool-result block. The
    # system breakpoint above caches the byte-stable prefix; this one caches
    # the growing conversation, which for a searching mode is where the bulk
    # is -- turn six of a dossier otherwise re-buys turns one through five,
    # search results included, at full input price. It moves rather than
    # accumulates because the API allows four markers per request and the
    # theme flow already spends one inside `messages`.
    marked: dict[str, Any] | None = None

    # API requests actually made, for the ledger: a conversation's cost in
    # round trips as well as tokens. `finish` reads it at call time.
    requests = 0

    def finish(turn: Turn) -> Turn:
        ledger.record(mode=turn.mode, model=turn.model,
                      stop_reason=turn.stop_reason, requests=requests,
                      input_tokens=turn.input_tokens,
                      output_tokens=turn.output_tokens,
                      cache_read_tokens=turn.cache_read_tokens)
        return turn

    for done in range(max_turns):
        if on_turn is not None:
            on_turn(done, max_turns)
        # `container` only ever has a value once a *server* tool has run, and
        # it is required rather than optional after that: the dated web search
        # does its own result filtering inside a code-execution container, so
        # a second request carrying that first turn's blocks is refused with
        # "container_id is required when there are pending tool uses generated
        # by code execution" unless the container comes with it. Found by
        # running the dossier rather than by reading the shapes -- the failure
        # needs both a server tool *and* a second turn, so a single-turn probe
        # never sees it.
        extra: dict[str, Any] = {"container": container} if container else {}
        resp = con.messages.create(
            model=answering,
            max_tokens=mode.max_tokens,
            **extra,
            # A cache breakpoint on the system block caches the tools and the
            # system prompt together (tools render first, so the marker covers
            # both). That boundary is byte-stable for a given mode and stance
            # while everything after it -- the brief, the question, the tool
            # round trips -- varies, which is exactly where the skill says to
            # cut. It pays twice: across the turns of one tool loop, and across
            # consecutive calls of the same mode, which is the shape of
            # interviewing a draft's 99 owed rationales one card at a time.
            #
            # Below the model's minimum cacheable prefix the marker is inert,
            # and one mode is. Measured 2026-08-19 with `count_tokens`, which
            # is free -- every earlier figure here was characters over four
            # and read ~40% low. The six conversational modes run 2,062
            # (research) to 6,587 (theme proposal), interview 2,373, all
            # clear of Sonnet 5's 1,024. **`scan` is 478** and clears
            # nothing -- not Sonnet's 1,024, not the 512 an Opus or Fable
            # seat gets -- which five back-to-back scans confirm by reading
            # zero cached tokens between them. That is the prompt being
            # short rather than a bug, and padding it to reach the floor
            # would buy a tenth of 478 tokens with 546 wasted ones.

            system=[{"type": "text", "text": mode.system(stance),
                     "cache_control": {"type": "ephemeral"}}],
            tools=schemas,
            output_config=output_config,
            messages=history,
        )
        requests += 1
        tokens_in += resp.usage.input_tokens
        tokens_out += resp.usage.output_tokens
        # `or 0`: the SDK types the field `int | None`, and None means the
        # platform did not report cache reads, not that zero were served.
        tokens_cached += resp.usage.cache_read_input_tokens or 0
        container = getattr(getattr(resp, "container", None), "id", None) or container

        # Checked before `content` is read, because a refusal can carry an
        # empty content list and indexing into it is how this becomes an
        # IndexError instead of a message somebody can act on.
        if resp.stop_reason == "refusal":
            return finish(Turn(
                mode=mode.name, model=resp.model,
                stop_reason=resp.stop_reason, text="", tool_calls=calls,
                input_tokens=tokens_in, output_tokens=tokens_out,
                searched=pages, search_errors=search_errors,
                refused=True, cache_read_tokens=tokens_cached))

        # Harvested before the stop-reason branch, so a turn that pauses or
        # ends still contributes what its searches found. Deduplicated on URL:
        # a mode that searches twice around a topic gets overlapping pages back
        # and the same source listed twice is noise in the evidence list.
        found, failed = _server_results(resp.content)
        for page in found:
            if page["url"] not in seen_urls:
                seen_urls.add(page["url"])
                pages.append(page)
        search_errors.extend(failed)

        # Appended whole, thinking blocks included. Sonnet 5 returns them with
        # empty text by default and they still have to go back unedited.
        history.append({"role": "assistant", "content": resp.content})

        # A server-side tool loop that hits its own iteration limit stops with
        # `pause_turn` and hands back text that reads finished. Returning it
        # here would be this project's own worst failure mode -- the Forge run
        # that plays on with 96 cards and reports a plausible winner. So it is
        # resumed instead: re-send with the paused turn last and the server
        # picks up where it stopped. Nothing is appended to nudge it along; a
        # trailing "continue" would be a new instruction rather than a
        # resumption. The loop's own ceiling is what stops this recurring.
        if resp.stop_reason == "pause_turn":
            continue

        if resp.stop_reason != "tool_use":
            text = "".join(b.text for b in resp.content
                           if b.type == "text").strip()
            return finish(Turn(
                mode=mode.name, model=resp.model,
                stop_reason=resp.stop_reason, text=text,
                tool_calls=calls, input_tokens=tokens_in,
                output_tokens=tokens_out, searched=pages,
                search_errors=search_errors,
                cache_read_tokens=tokens_cached))

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
        # Move the conversation's cache marker onto the newest results. The
        # next request then reads everything before them -- the earlier turns
        # and their search results -- from the cache at ~a tenth of the price,
        # instead of re-buying it. Moved rather than added: markers max out at
        # four per request, and earlier breakpoints keep working as read
        # points after the marker itself has gone.
        if results:
            if marked is not None:
                del marked["cache_control"]
            marked = results[-1]
            marked["cache_control"] = {"type": "ephemeral"}
        history.append({"role": "user", "content": results})

    # `answering` — what was *asked for* — rather than a served-by value,
    # because this path has no final response to read one off. Every other
    # ledger row carries `resp.model`; this is the one that cannot, and a row
    # that named the house model while a tiered seat ran up the bill would
    # misattribute exactly the spend the tiers were added to make visible.
    ledger.record(mode=mode.name, model=answering,
                  stop_reason="exhausted", requests=requests,
                  input_tokens=tokens_in, output_tokens=tokens_out,
                  cache_read_tokens=tokens_cached)
    raise ModeExhausted(
        f"{mode.name} still wanted tools after {max_turns} turns "
        f"({len(calls)} calls made). Nothing was written; nothing is half-done.")
