"""The theme interview: it asks about you, not about Magic.

Two modes, one feature, and
[ADR 20](../../../docs/adr/0020-the-theme-interview-reads-a-person.md) is why
they are two. `THEME_CONVERSATION` talks -- prose questions, no card pool, no
deck -- and `THEME_PROPOSAL` fires once at the end with a schema, the pool
and a web search. A conversation is prose and a proposal is a shape; a single
mode doing both either loses the shape or forces every chatty turn through it.

**The way in is that the colour pie is a personality taxonomy.** It was built as
five philosophies about how to fix a problem, and the mechanics fall out of the
philosophy. So the questions here are not Magic questions -- a film, a period,
your sign, how you are at game night -- and the translation into white-green is
the part a model is actually for. The create flow's two existing doors both open
onto "which of the 32 do you want", which is a question somebody who has never
played cannot answer.

Four things hold the boundary, and none of them is the system prompt:

**Neither mode can see a deck.** No `DeckSource` is passed, ever. This is a
surface for building, not for critiquing: it cannot rate a list, name a cut or
comment on a deck because it cannot reach one. The same trick the other two
modes already use -- the dossier has no `search_cards` so it cannot go shopping
-- stated as a rule in ADR 20: a mode is narrowed by what it can reach, not by
what it is asked to avoid.

**A preference counts only if you said it.** Every slot the model fills carries
a quote, and `ground()` checks that quote against the user's own turns. A slot
whose quote is not in the transcript is dropped and counted. That is the third
instrument in a family: `interview.only_questions()` checks a shape,
`dossier.keep_sources()` intersects a citation with what the search returned,
and this intersects a claimed preference with the text of the person supposedly
holding it. The failure it catches is a model that has decided you are a blue
player and started reporting that back as something you said.

**Readiness is computed, not declared.** There is no `ready` field for the
model to set. `may_propose()` counts grounded slots and answers in Python.

**Every commander is resolved through the pool.** `search_cards` with
`commanders_only` and an exact identity, then `get_cards` to confirm -- or the
name is dropped and counted, the same as a dossier's competitors.

It writes nothing. `may_write` is empty on both, this package has no write door,
and `tests/test_claude_boundary.py` fails on the commit that adds one. The
output is a proposal; the existing create flow is what makes a deck.
"""

from __future__ import annotations

import json
import re
from collections.abc import Callable
from dataclasses import dataclass, replace
from typing import Any

from mtglab import tarot
from mtglab.api import service
from mtglab.claude import persona as persona_mod
from mtglab.claude import stance as stance_mod
from mtglab.claude.modes import Mode, Turn, converse

# --------------------------------------------------------------- the shape

#: What the conversation is trying to learn, and deliberately not a list of
#: things about Magic. `axis` -- "what should the deck do" -- was drafted and
#: cut: it is the worst offender in the set, a Magic question wearing a
#: friendly hat, unanswerable by exactly the person this feature is for.
#:
#: `anchor` is the mirror image. It is the strongest single signal for picking
#: a commander and the one a genuine newcomer will not have, which is why the
#: floor below is three rather than four.
SLOT_KINDS = ("taste", "temperament", "posture", "anchor")

SLOT_QUESTIONS = {
    "taste": "A film, book, artwork, period or band they love that is not Magic.",
    "temperament": "How they are -- their sign, planner or improviser, what "
                   "they do when a plan falls apart.",
    "posture": "How they behave at game night: going for the throat, quietly "
               "building, or making deals.",
    "anchor": "A Magic card, character or deck they already love. Optional -- "
              "a newcomer will not have one, and that is fine.",
}

#: How many grounded slots before a proposal is available. Three, so that
#: having no favourite card does not lock a beginner out of the feature.
FLOOR = 3

#: The other gate. A multi-turn mode has no natural stopping point, which is
#: where `MAX_TOOL_TURNS` came from for the single-shot ones -- but that one is
#: about the tool loop and this one is about the conversation. After this many
#: exchanges the interview either proposes from what it has or ends honestly
#: without one.
MAX_EXCHANGES = 10

#: A single answer longer than this is not an answer, it is a paste. Capped
#: because the transcript is client-held and resent, so its size is the
#: client's to inflate and the bill is not.
MAX_TURN_CHARS = 2000


class TranscriptRejected(ValueError):
    """The conversation handed in is not one this mode will run.

    Its own exception because the caller's answer differs from every other
    failure here: nothing is wrong with the pool, the key or the model. The
    request is malformed, and it is a 422.
    """


class NotReady(ValueError):
    """Asked to propose before three slots survived grounding.

    Raised rather than proposing anyway. The floor is the whole point of ADR
    20's readiness argument, and a version of it that could be talked past by
    calling a different endpoint would not be a floor.
    """


# ------------------------------------------------------------ the vocabulary

def _vocabulary() -> str:
    """The colour pie and the 32, as the mode's shared reference data.

    Handed over rather than recalled, for the same reason `interview.brief()`
    assembles pool facts before the call: `colors.py` is checked in, carries
    `verified_by`, and is the same table the carousel teaches. A second copy of
    the four-colour names living in a prompt would be a second copy to disagree
    with `colors.py`, which `CLAUDE.md` warns about by name.

    It goes in the **system prompt**, not the first message, because it is
    byte-identical for every conversation -- which is exactly the content a
    cache breakpoint wants in front of it.
    """
    taxonomy = service.color_taxonomy()
    return json.dumps({
        "the_five_colours_as_philosophies": [
            {"code": c["code"], "name": c["name"],
             "wants": c["wants"], "fears": c["fears"]}
            for c in taxonomy["colors"]
        ],
        "the_32_combinations": [
            {"key": c["key"], "name": c["name"], "colors": c["colors"],
             "tagline": c["tagline"]}
            for c in taxonomy["combinations"]
        ],
        "what_each_size_of_combination_is_called": [
            {"key": t["key"], "label": t["label"], "blurb": t["blurb"]}
            for t in taxonomy["tiers"]
        ],
    }, indent=2)


#: Anthropic's hosted search, and the reason it is here rather than in a
#: crawler is ADR 19's: it maintains nothing, mirrors nothing, and stores no
#: third party's aggregate. One use in the conversation, because there is
#: exactly one thing worth searching for mid-conversation and a chatty mode
#: with an open search budget is a bill.
WEB_SEARCH_ONCE: dict[str, Any] = {
    "type": "web_search_20260209",
    "name": "web_search",
    "max_uses": 1,
}

#: Three: one per combination, plus one to spend on whichever needed a second
#: look. Measured at four the proposal took 226 seconds end to end, and the
#: searches are most of it -- see the note in ROADMAP about this needing to be
#: a background job before it is deployed.
WEB_SEARCH: dict[str, Any] = {
    "type": "web_search_20260209",
    "name": "web_search",
    "max_uses": 3,
}


# ----------------------------------------------------- the conversation mode

_SLOT_ITEM: dict[str, Any] = {
    "type": "object",
    "properties": {
        "kind": {"type": "string", "enum": list(SLOT_KINDS)},
        "value": {
            "type": "string",
            "description": ("Your reading of what they told you, in a few "
                            "words. Yours to phrase -- this is the one field "
                            "that is allowed to be an interpretation."),
        },
        "quote": {
            "type": "string",
            "description": (
                "The words *they* used, copied exactly from something they "
                "actually typed in this conversation. Not paraphrased, not "
                "tidied, not your summary of it. This is checked against the "
                "transcript and the slot is discarded if it is not found, so "
                "a slot you cannot quote is a slot you should not fill."),
        },
    },
    "required": ["kind", "value", "quote"],
    "additionalProperties": False,
}

#: Note the gaps, the way the rationale interview's schema is defined by its.
#: There is no field for a colour recommendation, no field for a card, and no
#: `ready` flag -- readiness is counted in Python from the slots below, and the
#: model has nowhere to assert it.
CONVERSATION_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "question": {
            "type": "string",
            "description": (
                "One question, addressed to them, ending in a question mark. "
                "It must be answerable by somebody who has never played Magic "
                "-- about them, their taste, or how they are with other "
                "people. Never about cards, colours, mana or decks."),
        },
        "fact": {
            "type": "object",
            "description": (
                "Optional. One genuinely interesting thing about Magic that "
                "connects to what they just said -- the hook that makes them "
                "curious. Omit it rather than reaching for a dull one."),
            "properties": {
                "text": {"type": "string",
                         "description": "One or two sentences."},
                "source": {
                    "type": "string",
                    "description": (
                        "Either the exact URL of a page you actually read, or "
                        "the literal string 'taxonomy' when it came from the "
                        "colour reference data you were given. Anything else "
                        "is discarded."),
                },
            },
            "required": ["text", "source"],
            "additionalProperties": False,
        },
        "slots": {
            "type": "array",
            "description": (
                "Everything you now believe about them, including what you "
                "already believed -- this replaces the previous set rather "
                "than adding to it. One entry per kind at most."),
            "items": _SLOT_ITEM,
        },
    },
    "required": ["question", "slots"],
    "additionalProperties": False,
}


CONVERSATION_INSTRUCTIONS = """
You are the theme interview in sylvan-library, a Commander deckbuilding tool.
Somebody wants to build a Magic deck and does not yet know what it should be.
Many of them have never played. Your job is to find out who they are, well
enough that a colour combination and a commander can be suggested to them -- and
to make the ten minutes it takes genuinely fun.

**Do not ask them about Magic.** Not what the deck should do, not whether they
like attacking or blocking, not which colours appeal. Those are questions in a
vocabulary they do not have, and asking them is how this feature fails. Ask
about *them*:

- What they read, watch, listen to, or would put on a wall.
- What period they would live in, and why that one.
- Their sign, if they will play along -- and take it seriously as a
  conversation, not as a joke you are both above.
- How they are when a plan falls apart.
- What they are like at game night with friends: the one going for the throat,
  the one quietly building something, the one making deals.

This works because the colour pie is a personality taxonomy before it is a set
of mechanics. Richard Garfield built the five colours as five philosophies
about how to fix a problem; the cards follow from the philosophy. The reference
data you have been given lists what each colour *wants* and what it *fears*.
That is the translation you are performing, and it is the interesting part.

Ask **one** question at a time. Read the answer properly before choosing the
next one -- the third question should depend on the second answer, or this
could have been a form. Follow what they are actually enthusiastic about.
Somebody who lights up about a specific film is telling you far more than
somebody being polite about a genre.

Fill in the slots as you learn things:

- **taste** -- a work, artist, period or scene they love, outside Magic.
- **temperament** -- how they are: their sign, planner or improviser, what they
  do under pressure.
- **posture** -- how they behave in a game with other people.
- **anchor** -- a Magic card, character or deck they already love. Optional and
  often absent. Do not fish for it, and never ask for it first.

**Every slot carries a quote of their own words, copied exactly from something
they typed.** Not your paraphrase of it. Quotes are checked against the
transcript and a slot whose quote is not found is thrown away, so if you cannot
point at where they said it, you do not know it yet. This is the rule that
stops you deciding who somebody is and then reporting it back to them as
something they said.

**Fun facts are the point, not decoration.** When you have a genuinely good one
-- something about a card, a set, a plane, a piece of Magic's history that
connects to what they just told you -- include it. You may use the web_search
tool **once**, and the best moment is right after they tell you what they love,
to find the specific thing that connects it to Magic. A fact from a page you
read carries that page's URL. A fact from the colour reference data you were
given carries the string 'taxonomy'. Do not include a fact you cannot source
either way, and do not pad with a mediocre one -- a question with no fact is
better than a question with a boring one.

Never give the same fact twice. If the interesting thing you have to say is one
you have already said in this conversation, omit the fact entirely and just ask
the question.

You have no access to the card pool, so you do not know what any card does.
Do not state card facts. Talk about colours, planes, history and people; the
cards come later, from a different surface that can look them up.

Never mention decks they might build, never name a colour combination as a
recommendation, and never tell them what you have concluded. You are gathering,
not proposing -- somebody clicks a button for that, and it is not you.

Write to them the way you would speak: ordinary punctuation, real dashes rather
than a pair of hyphens, no markdown, no headings, no bullet points. These
instructions are written in a code file's conventions and yours are not.
""".strip()


THEME_CONVERSATION = Mode(
    name="theme-conversation",
    purpose="Asks about you -- not about Magic -- until it knows enough to "
            "suggest a colour combination.",
    instructions=f"{CONVERSATION_INSTRUCTIONS}\n\n"
                 f"The colour pie and the 32 combinations, as reference data. "
                 f"This is checked-in, human-written and the same table the "
                 f"rest of the app teaches from; use it rather than your "
                 f"recollection.\n\n{_vocabulary()}",
    # Nothing from this package's registry at all. It cannot look a card up,
    # so it has no grounded card facts; it cannot read a deck, so it cannot
    # critique one. Both are structural rather than requested.
    tool_names=(),
    server_tools=(WEB_SEARCH_ONCE,),
    # Room for a search, a short answer and adaptive thinking. Nowhere near
    # the dossier's, because the output is one question.
    max_tokens=8192,
    response_schema=CONVERSATION_SCHEMA,
)


# --------------------------------------------------------- the proposal mode

_SOURCE_IDS: dict[str, Any] = {
    "type": "array",
    "items": {"type": "string"},
    "description": ("The ids of the sources this rests on, from the sources "
                    "list. Empty only when it rests on the colour reference "
                    "data you were given."),
}

PROPOSAL_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "combinations": {
            "type": "array",
            "description": (
                "Exactly two: the one you would actually recommend first, and "
                "a runner-up that is genuinely different, so the contrast "
                "teaches something."),
            "items": {
                "type": "object",
                "properties": {
                    "key": {
                        "type": "string",
                        "description": ("The combination's key from the "
                                        "reference data, e.g. 'GW' or 'BG'. "
                                        "Exactly as written there."),
                    },
                    "reading": {
                        "type": "string",
                        "description": (
                            "Why *this person* lands here -- the leap from "
                            "what they told you to these colours. This is an "
                            "interpretation and it is labelled as one to the "
                            "reader, so it is allowed to be playful and does "
                            "not need a source. Quote them where you can."),
                    },
                    "grounding": {
                        "type": "string",
                        "description": (
                            "What is factually true about these colours and "
                            "what people build in them: the philosophy, the "
                            "archetype's name, its history. Every claim here "
                            "rests on a source you list or on the reference "
                            "data. This is not the place for the leap."),
                    },
                    "source_ids": _SOURCE_IDS,
                    "commanders": {
                        "type": "array",
                        "description": (
                            "Three legends that lead these exact colours. "
                            "Look them up with search_cards using "
                            "commanders_only and identity_exact before you "
                            "name them."),
                        "items": {
                            "type": "object",
                            "properties": {
                                "card": {
                                    "type": "string",
                                    "description": (
                                        "The exact printed card name. It is "
                                        "looked up in the pool and dropped "
                                        "if it does not resolve."),
                                },
                                "prose": {
                                    "type": "string",
                                    "description": (
                                        "One or two sentences: what leading a "
                                        "deck with this one feels like, and "
                                        "why it suits them. Say plainly if it "
                                        "is a demanding one to pilot."),
                                },
                                "source_ids": _SOURCE_IDS,
                            },
                            "required": ["card", "prose", "source_ids"],
                            "additionalProperties": False,
                        },
                    },
                },
                "required": ["key", "reading", "grounding", "source_ids",
                             "commanders"],
                "additionalProperties": False,
            },
        },
        "sources": {
            "type": "array",
            "description": ("Every page you actually read, with the id the "
                            "sections refer to. A page you did not visit is "
                            "not a source."),
            "items": {
                "type": "object",
                "properties": {
                    "id": {"type": "string",
                           "description": "Short id, e.g. 's1'."},
                    "title": {"type": "string"},
                    "url": {"type": "string"},
                },
                "required": ["id", "title", "url"],
                "additionalProperties": False,
            },
        },
    },
    "required": ["combinations", "sources"],
    "additionalProperties": False,
}


PROPOSAL_INSTRUCTIONS = """
You are the theme proposal in sylvan-library, a Commander deckbuilding tool.
Somebody has just spent ten minutes telling an interviewer about themselves --
what they love, how they are, how they play with other people. None of it was
about Magic. You are given that conversation and the readings taken from it,
and your job is to turn it into two colour combinations and six commanders they
could actually build.

Many of these people have never played. Write for them: assume intelligence,
assume no vocabulary. Explain what a colour combination *means* before you
explain what it does.

**Two kinds of sentence, and they live in different fields.**

`reading` is the leap -- from a favourite film, a century, a star sign, a way of
behaving at a table, to a pair of colours. It is an interpretation, it is shown
to the reader as one, and it is therefore allowed to be warm and playful and a
little bold. Quote their own words back to them where you can; being seen is
the whole pleasure of this. Do not hedge it into mush.

`grounding` is what is true: what these colours want and fear, what the
combination is called and where the name comes from, what people actually build
in it and what that archetype is called. Every claim there rests on the colour
reference data you were given, or on a page you actually read and listed. Do
not compose a URL -- cited pages are checked against what the search really
returned and anything else is discarded before the user sees it.

**Search before you write, every time.** Call web_search at least once for each
combination you are going to propose, before drafting its `grounding`. The
reference data tells you what the colours *mean*; it does not tell you what
people actually build in them today, what that archetype is called, or which
commanders a newcomer gets on with -- and those are the claims a reader will
act on. Naming an archetype from memory is the same failure as naming a card
from memory, and this project has been wrong that way twice. If a search comes
back with nothing useful, say less rather than filling the gap from
recollection.

Keeping those apart is not a formatting preference. A reading cannot be wrong,
because it is offered as a reading. A claim about 1993 can be, and it is the
one a reader will believe.

**Every commander comes out of the pool, and it must be a legend of exactly
these colours.** Call search_cards with commanders_only true, identity_exact
true, and identity set to the combination you are proposing. Pick your three
**only** from what that call returns. If they mentioned a budget, pass
price_max. Then call get_cards on the three you chose before writing a word
about any of them; what you remember about a card is not evidence, the pool
row is.

Do not name a commander from memory and do not name one whose identity is a
*subset* of the combination -- a mono-green legend is perfectly legal in a
Simic deck, but the deck it leads is a mono-green deck and fills a different
one of the 32. Both mistakes are checked and both are silently fatal to your
suggestion: a commander that fails either check is dropped, and a combination
that loses all three commanders is dropped whole, however good the paragraph
you wrote for it.

Choose commanders somebody could actually start with. Prefer ones that do
something visible and fun on their own over ones that need three other cards
and a rules lawyer, and say so honestly when a suggestion is a demanding one.
Popularity is a reasonable signal and not the only one -- if a deep cut fits
what they told you better than the obvious face of the guild, say the deep cut
and say why.

Pick the second combination to *teach by contrast*. Not the near-neighbour that
differs by a shade -- something that takes a different reading of the same
person and shows them there was a choice being made.

Never rate a deck, never mention power level or brackets, and never suggest
what somebody should cut. You are not looking at a decklist and there isn't one.

Write the way you would speak: ordinary punctuation, real dashes rather than a
pair of hyphens, no markdown, no headings, no bullet points. These instructions
are written in a code file's conventions and yours are not.
""".strip()


THEME_PROPOSAL = Mode(
    name="theme-proposal",
    purpose="Turns a theme conversation into two colour combinations and six "
            "pool-checked commanders.",
    instructions=f"{PROPOSAL_INSTRUCTIONS}\n\n"
                 f"The colour pie and the 32 combinations, as reference data. "
                 f"The `key` you return must be one of these.\n\n"
                 f"{_vocabulary()}",
    # `search_cards` to find legends, `get_cards` to confirm what they do.
    # No `get_deck`, no `validate_deck`, no `deck_stats`, no `suggest` -- there
    # is no deck, and a mode that could see one could critique it.
    tool_names=("search_cards", "get_cards"),
    server_tools=(WEB_SEARCH,),
    # Two combinations, six commanders, a search and adaptive thinking. The
    # dossier's ceiling, for much the same reason.
    max_tokens=16384,
    response_schema=PROPOSAL_SCHEMA,
)


# ---------------------------------------------------------- reading the answer

#: Anything in the C0 range, plus DEL. Not a security measure -- a defence
#: against a real thing that happened.
_CONTROL = re.compile(r"[\x00-\x1f\x7f]")


def prose(text: Any) -> str:
    """Model-written text, fit to put in front of somebody.

    Control characters out, whitespace collapsed. This exists because of one
    observed answer: the model wrote a question containing ``\\f`` where it
    meant "a fight", `json.loads` faithfully decoded that as a form feed, and
    the sentence rendered as "than policing the table [FF]ight two other people
    are having" -- a word with its first letter eaten and no error anywhere.

    The API docs warn that recent models vary their JSON string escaping and
    that tool inputs must be parsed rather than pattern-matched. Parsing was
    never the problem; the escape was valid JSON. So the check belongs one step
    later, on the way out, with the other things this module refuses to pass
    along untouched.
    """
    return re.sub(r"\s+", " ", _CONTROL.sub(" ", str(text or ""))).strip()


def _normalise(text: str) -> str:
    """Lowercased, whitespace collapsed. Punctuation is left alone.

    Deliberately not a fuzzy match. Normalising away punctuation would let
    "cats" match "cats?" but would also start matching things that are not
    quotes, and the value of this check is that it is hard to pass by accident.
    """
    return re.sub(r"\s+", " ", text).strip().casefold()


#: Below this a "quote" is a coincidence rather than evidence. Three characters
#: keeps real short answers -- "cat", "red", "80s" -- and rejects a stray "a".
MIN_QUOTE_CHARS = 3


def ground(slots: list[Any], transcript: list[dict[str, str]]
           ) -> tuple[list[dict[str, str]], int]:
    """Keep the slots whose quote is really in the user's own turns.

    The whole readiness argument in one substring test. A model that has
    decided who somebody is will happily report that back as a preference they
    expressed, and this is the check that turns "I think you want blue" into a
    dropped row rather than a fact about the user.

    Only **user** turns count. Quoting the interviewer's own question back
    would let the mode ground a slot on a preference it suggested, which is the
    failure with a bow on it.

    Crude on purpose, and the same bluntness `only_questions()` has: a model
    that quotes one stray real word can carry an invented reading past it. It
    catches wholesale invention, which is the failure that matters, and the
    dropped count is surfaced so a prompt that has started making things up
    reads as a number rather than as helpfulness.
    """
    # Joined with a separator no keyboard produces, so a "quote" cannot span
    # two turns and pick up words the user never put next to each other.
    said = " ␟ ".join(_normalise(t["text"]) for t in transcript
                      if t.get("role") == "user")
    kept: dict[str, dict[str, str]] = {}
    dropped = 0
    for item in slots:
        if not isinstance(item, dict):
            dropped += 1
            continue
        kind = str(item.get("kind") or "").strip()
        quote = prose(item.get("quote"))
        value = prose(item.get("value"))
        needle = _normalise(quote)
        if (kind not in SLOT_KINDS or not value
                or len(needle) < MIN_QUOTE_CHARS or needle not in said):
            dropped += 1
            continue
        # Last reading of a kind wins: the schema asks for the whole set each
        # turn, so a later turn refining "likes cats" into "likes big cats,
        # specifically" should replace it rather than sit beside it.
        kept[kind] = {"kind": kind, "value": value, "quote": quote}
    return [kept[k] for k in SLOT_KINDS if k in kept], dropped


def may_propose(grounded: list[dict[str, str]]) -> bool:
    """Whether there is enough to propose from. Counted, never declared.

    `FLOOR` distinct grounded kinds. There is no field anywhere in
    `CONVERSATION_SCHEMA` a model could set to change this answer, which is the
    point: ADR 15's lesson was that the interview's boundary held because the
    schema had nowhere to put a rationale, not because the prompt asked nicely.
    """
    return len({s["kind"] for s in grounded}) >= FLOOR


def keep_fact(raw: Any, searched: list[dict[str, Any]]) -> dict[str, str] | None:
    """A fun fact, if it came from somewhere. None otherwise.

    Two legitimate origins and they are checked differently. `taxonomy` means
    the colour reference data in the system prompt -- checked-in, human-written,
    carrying `verified_by`, and so trusted at the file level rather than the
    sentence level. Anything else must be a URL the search actually returned,
    checked the way `dossier.keep_sources()` checks a citation, because a
    response schema suppresses the API's own citations and a URL in the payload
    is otherwise a string the model typed.
    """
    if not isinstance(raw, dict):
        return None
    text = prose(raw.get("text"))
    source = prose(raw.get("source"))
    if not text or not source:
        return None
    if source.casefold() == "taxonomy":
        return {"text": text, "source": "taxonomy", "url": ""}
    from mtglab.claude.dossier import canonical_url
    match = next((p for p in searched
                  if canonical_url(p["url"]) == canonical_url(source)), None)
    if match is None:
        return None
    return {"text": text, "source": match["title"] or match["url"],
            "url": match["url"]}


# ------------------------------------------------------------- the transcript

def check_transcript(raw: Any) -> list[dict[str, str]]:
    """Validate a client-held transcript, and refuse anything else.

    ADR 20 keeps conversation state on the client, which means the wire format
    is a thing a client composes -- so this is the door. **It is not Anthropic's
    message format and must never become it.** An endpoint that accepted
    `messages` blocks would be a free proxy for somebody else's spend, which on
    a hosted instance is the entire game. What crosses is plain text with a
    role, and everything structural -- a tool_use block, a system turn, an
    image -- has nowhere to ride.

    Tampering with the *content* is allowed and buys nothing, which is worth
    stating rather than asserting: every card in the proposal is re-resolved
    against the pool, every source is checked against the search, and
    readiness is recomputed here from the text rather than carried in it. What
    is left is somebody lying to themselves about their own taste.
    """
    if raw is None:
        return []
    if not isinstance(raw, list):
        raise TranscriptRejected("transcript must be a list of turns")
    if len(raw) > MAX_EXCHANGES * 2 + 1:
        raise TranscriptRejected(
            f"this conversation is over after {MAX_EXCHANGES} exchanges. "
            f"Propose from what it has, or start again.")

    out: list[dict[str, str]] = []
    for i, turn in enumerate(raw):
        if not isinstance(turn, dict):
            raise TranscriptRejected(f"turn {i} is not an object")
        role = str(turn.get("role") or "").strip()
        text = str(turn.get("text") or "").strip()
        if role not in ("user", "assistant"):
            raise TranscriptRejected(
                f"turn {i} has role {role!r}; only 'user' and 'assistant' "
                f"cross this boundary")
        if not text:
            raise TranscriptRejected(f"turn {i} is empty")
        if len(text) > MAX_TURN_CHARS:
            raise TranscriptRejected(
                f"turn {i} is {len(text)} characters; the cap is "
                f"{MAX_TURN_CHARS}")
        # Alternation is deliberately **not** required, and the first draft of
        # this got it wrong on a false premise: the Messages API accepts
        # consecutive same-role turns and combines them. Requiring alternation
        # here turned a recoverable hiccup into a wedged conversation — when a
        # turn comes back without a usable question there is no assistant turn
        # to record, so the next answer is legitimately a second user turn in a
        # row, and refusing it means the only way out is starting over.
        out.append({"role": role, "text": text})
    # The **interviewer** opens, so a transcript starts with an assistant turn
    # rather than a user one. That is backwards from every other conversation
    # shape in this codebase and it is the point of the feature: somebody who
    # does not know what to build cannot be expected to speak first.
    if out and out[0]["role"] != "assistant":
        raise TranscriptRejected(
            "a transcript starts with the interviewer's own opening question")
    return out


#: The app's own opening turn, and it exists for a wire-level reason rather
#: than a conversational one: the Messages API requires `messages[0]` to be a
#: user turn, and this interview's first *real* turn is the interviewer's
#: question. Without a frame the whole transcript is off by one role.
#:
#: Byte-stable, so it costs nothing across turns or across conversations.
_FRAME = ("Somebody has opened the deckbuilder and does not know what to "
          "build. Interview them.")


def _frame_for(reading: tarot.Reading | None) -> str:
    """The opening frame, plus the spread when there is one.

    **Here rather than in the system prompt, and that is a caching decision.**
    A mode's instructions are byte-stable so `converse` can cache them; a
    spread is different every reading, so putting it there would make every
    first turn a cache miss for the whole block. As a message it sits after the
    breakpoint and costs one short paragraph.
    """
    if reading is None:
        return _FRAME
    return (f"{_FRAME}\n\nYou have already dealt three cards, face up, in this "
            f"order. These are the cards on the table and there are no others:\n"
            f"{reading.describe()}")


def _messages(transcript: list[dict[str, str]], *, closing: str,
              frame: str = _FRAME) -> list[dict[str, Any]]:
    """The transcript as a request: a frame, the conversation, and the ask.

    The system block carries the instructions and the taxonomy and is
    byte-stable per mode, so `converse` already caches it. That is enough for a
    single-shot mode and not enough here: a conversation's history is the part
    that grows, and it grows by appending, which is the ideal shape for a
    second breakpoint. Without one, every turn re-reads the whole conversation
    at full price.
    """
    out: list[dict[str, Any]] = [{"role": "user", "content": frame}]
    out += [{"role": t["role"], "content": t["text"]} for t in transcript]

    # The closing instruction rides with the last user turn rather than as a
    # turn of its own, so the transcript the model sees is the conversation and
    # nothing else. The breakpoint goes after it: everything up to here is
    # settled, and the next turn appends.
    tail = out[-1]
    text = tail["content"] if isinstance(tail["content"], str) else ""
    out[-1] = {"role": "user", "content": [
        *([{"type": "text", "text": text}] if text else []),
        {"type": "text", "text": closing,
         "cache_control": {"type": "ephemeral"}},
    ]} if tail["role"] == "user" else tail
    if tail["role"] != "user":
        # The conversation ends on the interviewer's own question, which only
        # happens if a client asks for another turn without answering. Append
        # the instruction as its own turn rather than editing theirs.
        out.append({"role": "user", "content": [
            {"type": "text", "text": closing,
             "cache_control": {"type": "ephemeral"}}]})
    return out


def _with_voice(base: Mode, who: persona_mod.Persona) -> Mode:
    """The same mode, speaking differently.

    Appended, never substituted: `CONVERSATION_INSTRUCTIONS` and
    `PROPOSAL_INSTRUCTIONS` carry the rules that make this feature work, and a
    persona is not allowed a say in them. Last, because recency is worth
    something and the reference data in between is a table rather than an
    instruction.

    The default persona gets the base object back unchanged -- same name, same
    bytes -- so nothing about the interview as it already exists moves.
    """
    if not who.voice:
        return base
    return replace(base, name=f"{base.name}:{who.key}",
                   instructions=f"{base.instructions}\n\n{who.voice}")


#: Built once at import, so each persona's system block is byte-stable and
#: `converse` can go on caching it. Building one per request would silently
#: turn every conversation into a cache miss.
CONVERSATION_MODES: dict[str, Mode] = {
    key: _with_voice(THEME_CONVERSATION, who)
    for key, who in persona_mod.PERSONAS.items()}

PROPOSAL_MODES: dict[str, Mode] = {
    key: _with_voice(THEME_PROPOSAL, who)
    for key, who in persona_mod.PERSONAS.items()}


def _reading_for(who: persona_mod.Persona, seed: Any) -> tarot.Reading | None:
    """The spread this conversation is being read from, or None.

    Re-dealt from the seed on every turn rather than stored. The deal is
    deterministic (`tarot.deal`), so the client carrying one integer is enough
    to make the cards the same cards for the whole reading -- the same trick
    the transcript uses, and the reason this mode still needs no table.
    """
    if not who.deals or seed is None:
        return None
    try:
        return tarot.deal(int(seed))
    except (TypeError, ValueError) as exc:
        raise TranscriptRejected(f"not a usable reading seed: {seed!r}") from exc


def stance_for(requested: Any) -> stance_mod.Stance:
    """The stance that applies when there is no deck to derive one from.

    Every other mode reads `status: built | theoretical` off the deck it is
    about (ADR 15). This one runs *before a deck exists*, and
    `stance.resolve(None, deck=None)` is `off` -- correct as a default for "I
    have no idea what this is about", wrong here, where the answer is knowable:
    a deck that has not been built yet is as theoretical as a deck can get.
    Nothing is sleeved and no suggestion costs anybody a trip to the box, so
    the default is the one a theoretical deck gets.

    Still clamped to the deployment ceiling, so `off` remains reachable and an
    operator who has turned this off has turned it off.

    **Public because the dial has to be able to ask.** `/api/claude` renders
    the stance readout beside the create flow, and with no deck to name it
    would otherwise resolve through `stance.resolve(None, None)` and report
    `off` — while this function was about to run the conversation at
    `second-opinion`. A dial that misreports the stance it governs is worse
    than no dial, and the fix is one caller rather than a second copy of this
    default living in TypeScript.
    """
    if requested is None:
        return stance_mod.clamp(stance_mod.SECOND_OPINION, stance_mod.ceiling())
    return stance_mod.resolve(requested)


# ----------------------------------------------------------------- the modes

def _report(turn: Turn | None, *, mode: str, effective: stance_mod.Stance,
            **extra: Any) -> dict[str, Any]:
    """One response shape for every outcome, including not asking at all.

    `answered_by` is ADR 14's third boundary as a field. It matters more here
    than anywhere else in the package: this is the first surface whose output
    is *meant* to be enjoyed, and a reader enjoying it should still never be
    unsure which system produced a sentence.
    """
    return {
        "answered_by": "claude",
        "mode": mode,
        "model": turn.model if turn else "",
        "stance": stance_mod.describe(effective),
        "usage": {"input_tokens": turn.input_tokens if turn else 0,
                  "output_tokens": turn.output_tokens if turn else 0,
                  "cache_read_tokens": turn.cache_read_tokens if turn else 0},
        **extra,
    }


@dataclass(frozen=True)
class AskRequest:
    """A conversation turn that has passed every check not needing the network.

    The same split `ProposalRequest` makes, and made for the same reason one
    surface later: a turn was measured at 4.3-37.7 seconds with **one outlier at
    133.8s**, and the transport ceiling it has to fit under is bounded above at
    236s only because that is where the dossier broke. Nobody knows where it
    actually is. A turn that overruns it fails the way that one did -- no status
    code, no access-log line, and a finished answer thrown away -- so the call
    goes in a job and the refusals stay in the request.

    Two of them, both decidable here: a malformed transcript (422) and an
    unknown persona or unusable seed (422, `UnknownPersona` being a
    `ValueError`). 503 for a missing key is the planner's, as it is for the
    proposal.
    """

    history: list[dict[str, str]]
    #: The slots the client carried, re-grounded against the transcript on the
    #: way in -- the client is not the authority on what its user said.
    carried: list[dict[str, str]]
    effective: stance_mod.Stance
    persona: str = persona_mod.DEFAULT
    seed: int | None = None

    @property
    def exchanges(self) -> int:
        return _exchanges(self.history)

    @property
    def exhausted(self) -> bool:
        """Whether the conversation has already gone as long as it goes."""
        return self.exchanges >= MAX_EXCHANGES

    @property
    def needs_call(self) -> bool:
        """Whether running this reaches Anthropic at all.

        False at stance `off` and false past `MAX_EXCHANGES`. Neither is an
        error -- both are answers, and both are available immediately, so a
        caller that queues work can hand back a job born finished rather than
        making somebody poll for a sentence Python already had.
        """
        return bool(self.effective.allows_calls) and not self.exhausted


def check_ask(transcript: Any = None, slots: Any = None, *,
              requested: Any = None, persona: Any = None,
              seed: Any = None) -> AskRequest:
    """Everything that can refuse a turn, done before anything is spent.

    Unlike `check_proposal` there is no floor to fail: a conversation with
    nothing in it yet is exactly the case this mode exists for.
    """
    history = check_transcript(transcript)
    effective = stance_for(requested)
    # Resolved here rather than in the worker, the same argument
    # `check_proposal` makes: an unknown persona is a malformed request and
    # belongs with the other 422s, not a moment later as a job in state `error`.
    who = persona_mod.get(persona)
    _reading_for(who, seed)              # raises now if the seed is unusable
    carried, _ = ground(list(slots or []), history)

    return AskRequest(history=history, carried=carried, effective=effective,
                      persona=who.key,
                      seed=int(seed) if who.deals and seed is not None
                      else None)


def ask(transcript: Any = None, slots: Any = None, *,
        requested: Any = None, persona: Any = None, seed: Any = None
        ) -> dict[str, Any]:
    """One turn of the theme conversation: a question back, and what it heard.

    Check and run in one call, which is what the tests of the whole path use.
    The app splits the two so the network half can be a job; see `check_ask`.
    """
    return run_ask(check_ask(transcript, slots, requested=requested,
                             persona=persona, seed=seed))


def run_ask(req: AskRequest, *,
            on_turn: Callable[[int, int], None] | None = None) -> dict[str, Any]:
    """The half that reaches Anthropic: ask the next question, hear the answer.

    Stateless. The transcript arrives from the client and leaves again; nothing
    is stored, which is why this needs no table, no expiry sweep and no ADR 5
    ownership rule -- and, as it turns out, is the reason the most personal
    thing this app handles never touches the server's disk.

    `req.persona` picks the voice and `req.seed` re-deals the spread it is
    reading from. Both are client-held for the same reason the transcript is,
    and both default to what every client sent before they existed.
    """
    history, carried, effective = req.history, req.carried, req.effective
    who = persona_mod.PERSONAS[req.persona]
    mode = CONVERSATION_MODES[who.key]
    reading = _reading_for(who, req.seed)

    if not effective.allows_calls:
        return _report(None, mode=mode.name, effective=effective, persona=who.key,
                       asked=False, question="", fact=None, slots=carried,
                       slots_dropped=0, grounded=len(carried),
                       floor=FLOOR, may_propose=may_propose(carried),
                       exchanges=_exchanges(history), max_exchanges=MAX_EXCHANGES,
                       reason="The stance is off, so no call was made.")

    if req.exhausted:
        # The conversation ceiling, and it is not the tool loop's. Reported as
        # a finished conversation rather than an error: what it has is what it
        # has, and if that is three grounded slots there is still a proposal.
        return _report(None, mode=mode.name, effective=effective, persona=who.key,
                       asked=False, question="", fact=None, slots=carried,
                       slots_dropped=0, grounded=len(carried),
                       floor=FLOOR, may_propose=may_propose(carried),
                       exchanges=_exchanges(history), max_exchanges=MAX_EXCHANGES,
                       reason=f"That is {MAX_EXCHANGES} exchanges, which is as "
                              f"long as this conversation goes.")

    closing = _closing_for(carried, history)
    turn = converse(mode,
                    messages=_messages(history, closing=closing,
                                       frame=_frame_for(reading)),
                    stance=effective,
                    # No client tools at all, so the only reason to go round
                    # again is the one search resuming after a pause_turn.
                    max_turns=4,
                    on_turn=on_turn)

    if turn.refused:
        return _report(turn, mode=mode.name, effective=effective, persona=who.key,
                       asked=True, question="", fact=None, slots=carried,
                       slots_dropped=0, grounded=len(carried), floor=FLOOR,
                       may_propose=may_propose(carried),
                       exchanges=_exchanges(history),
                       max_exchanges=MAX_EXCHANGES,
                       reason="The model declined to answer this one.")
    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        return _report(turn, mode=mode.name, effective=effective, persona=who.key,
                       asked=True, question="", fact=None, slots=carried,
                       slots_dropped=0, grounded=len(carried), floor=FLOOR,
                       may_propose=may_propose(carried),
                       exchanges=_exchanges(history),
                       max_exchanges=MAX_EXCHANGES,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}).")

    question = prose(payload.get("question"))
    # The same predicate the rationale interview uses, and for a related
    # reason: a declarative sentence here is the mode telling somebody what
    # they think instead of asking.
    if not question.endswith("?"):
        question = ""
    grounded, dropped = ground(payload.get("slots") or [], history)

    return _report(
        turn, mode=mode.name, effective=effective, persona=who.key, asked=True,
        question=question,
        fact=keep_fact(payload.get("fact"), turn.searched),
        slots=grounded,
        slots_dropped=dropped,
        grounded=len(grounded),
        floor=FLOOR,
        may_propose=may_propose(grounded),
        exchanges=_exchanges(history),
        max_exchanges=MAX_EXCHANGES,
        reason="" if question else "Nothing usable came back.",
        never="These are questions about you. What you build is your call.",
    )


def _exchanges(transcript: list[dict[str, str]]) -> int:
    return sum(1 for t in transcript if t["role"] == "user")


def _closing_for(grounded: list[dict[str, str]],
                 transcript: list[dict[str, str]]) -> str:
    """What to ask the model for, given how far along the conversation is.

    Assembled in Python from the grounded slots rather than left to the model
    to work out, so "what is still missing" is the same answer the button is
    computed from. A mode that thought it had heard something the readiness
    check disagreed with would ask the wrong next question.
    """
    if not transcript:
        return ("Open the conversation. One warm, specific question about them "
                "-- not about Magic, and not a list. Introduce yourself in a "
                "sentence first so they know what this is.")
    have = {s["kind"] for s in grounded}
    missing = [k for k in SLOT_KINDS if k not in have and k != "anchor"]
    if missing:
        wanted = "; ".join(f"{k}: {SLOT_QUESTIONS[k]}" for k in missing)
        return (f"Ask the next question. Still unknown -- {wanted} Follow what "
                f"they are actually interested in rather than working through "
                f"that list in order, and re-state every slot you are confident "
                f"of, including ones from earlier turns.")
    return ("You have enough to go on, and they can ask for a suggestion "
            "whenever they like. Keep going while they are enjoying it: ask "
            "something that sharpens what you already know, or that might turn "
            "up an anchor. Re-state every slot you are confident of.")


@dataclass(frozen=True)
class ProposalRequest:
    """A proposal that has passed every check not needing the network.

    The split exists because the expensive half runs in a background job and
    the refusals must not go with it. A malformed transcript, a floor not yet
    reached and an unparseable stance are all decidable in the request, and
    each has a status code the UI acts on -- 422, 409, 422. Carried into a
    worker they would arrive as a *job* in state `error`, which turns three
    distinct answers into one string somebody has to pattern-match. So
    `check_proposal` refuses synchronously and this is what survives it.
    """

    history: list[dict[str, str]]
    grounded: list[dict[str, str]]
    dropped: int
    effective: stance_mod.Stance
    budget: float | None = None
    avoid: str = ""
    #: The voice that ran the conversation, carried so the payoff arrives in
    #: the same one. A reading that turns into a committee memo at the moment
    #: it matters has thrown away the only thing the persona was for.
    persona: str = persona_mod.DEFAULT
    #: The spread's seed, or None. Re-dealt rather than carried whole, for the
    #: reason `_reading_for` gives.
    seed: int | None = None

    @property
    def needs_call(self) -> bool:
        """Whether running this reaches Anthropic at all.

        False at stance `off`, which is a real position and not an error: the
        answer is a report saying no call was made, available immediately. A
        caller that queues work can use this to hand back a job born finished,
        the way a cached simulation does.
        """
        return bool(self.effective.allows_calls)


def check_proposal(transcript: Any = None, slots: Any = None, *,
                   requested: Any = None, budget: float | None = None,
                   avoid: str = "", persona: Any = None,
                   seed: Any = None) -> ProposalRequest:
    """Everything that can refuse a proposal, done before anything is spent.

    Refuses below the floor rather than proposing anyway. The button is dark in
    the UI for the same reason, but a floor that only existed in the client
    would not be one.
    """
    history = check_transcript(transcript)
    grounded, dropped = ground(list(slots or []), history)
    effective = stance_for(requested)

    if not may_propose(grounded):
        raise NotReady(
            f"{len(grounded)} of {FLOOR} things are known about this person, "
            f"and every one has to be something they actually said. Keep "
            f"talking.")

    # Resolved here rather than in the worker: an unknown persona is a
    # malformed request and belongs with the other 422s, not four minutes
    # later as a job in state `error`.
    who = persona_mod.get(persona)
    _reading_for(who, seed)          # raises now if the seed is unusable

    return ProposalRequest(history=history, grounded=grounded, dropped=dropped,
                           effective=effective, budget=budget, avoid=avoid,
                           persona=who.key,
                           seed=int(seed) if who.deals and seed is not None
                           else None)


def propose(transcript: Any = None, slots: Any = None, *,
            requested: Any = None, budget: float | None = None,
            avoid: str = "", persona: Any = None, seed: Any = None
            ) -> dict[str, Any]:
    """Two colour combinations and six commanders, from what they told you.

    Check and run in one call, which is what the CLI wants and what every test
    of the whole path uses. The app splits the two so the minutes-long half can
    be a job; see `check_proposal`.
    """
    return run_proposal(check_proposal(transcript, slots, requested=requested,
                                       budget=budget, avoid=avoid,
                                       persona=persona, seed=seed))


def run_proposal(req: ProposalRequest, *,
                 on_turn: Callable[[int, int], None] | None = None) -> dict[str, Any]:
    """The expensive half: read around, name legends, check every one.

    **Measured at 226 seconds** with four searches, since it reads a dozen-odd
    pages and resolves every legend against the pool. That is why the caller
    may hand in `on_turn` -- run as a background job this is the only thing that
    says it is still moving.
    """
    grounded, dropped, effective = req.grounded, req.dropped, req.effective
    who = persona_mod.PERSONAS[req.persona]
    mode = PROPOSAL_MODES[who.key]
    reading = _reading_for(who, req.seed)

    if not req.needs_call:
        return _report(None, mode=mode.name, effective=effective,
                       persona=who.key,
                       asked=False, combinations=[], sources=[],
                       slots=grounded, slots_dropped=dropped,
                       reason="The stance is off, so no call was made.")

    turn = converse(mode,
                    messages=_messages(req.history,
                                       closing=_proposal_ask(grounded,
                                                             req.budget,
                                                             req.avoid),
                                       frame=_frame_for(reading)),
                    stance=effective,
                    # A search, a look at what came back, a commander search, a
                    # get_cards to confirm, and the write-up -- with a paused
                    # turn able to spend one.
                    max_turns=8,
                    on_turn=on_turn)

    if turn.refused:
        return _report(turn, mode=mode.name, effective=effective, persona=who.key,
                       asked=True, combinations=[], sources=[],
                       slots=grounded, slots_dropped=dropped,
                       reason="The model declined to write this one.")
    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        return _report(turn, mode=mode.name, effective=effective, persona=who.key,
                       asked=True, combinations=[], sources=[],
                       slots=grounded, slots_dropped=dropped,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}).")

    from mtglab.claude.dossier import keep_sources
    sources, sources_dropped = keep_sources(payload.get("sources") or [],
                                            turn.searched)
    allowed = {s["id"] for s in sources}
    combinations, commanders_dropped, combinations_dropped = _combinations(
        payload.get("combinations"), allowed)

    if not combinations:
        # The one refusal this mode does make, and it is not ADR 19's. A
        # dossier with no source is empty; a proposal with no *combination* has
        # nothing to propose, which is a different failure and the only one
        # worth refusing over.
        return _report(turn, mode=mode.name, effective=effective, persona=who.key,
                       asked=True, combinations=[], sources=sources,
                       slots=grounded, slots_dropped=dropped,
                       reason="Nothing came back that resolved against the "
                              "pool, so there is nothing to suggest.")

    return _report(
        turn, mode=mode.name, effective=effective, persona=who.key, asked=True,
        combinations=combinations,
        sources=sources,
        # ADR 20 diverges from ADR 19 here and it is deliberate: an unsourced
        # dossier is refused, an unsourced proposal is not. A dossier is
        # entirely web-sourced claims, so stripping them leaves voice; a
        # proposal's load-bearing content is a colour combination and six real
        # commanders, and both are pool facts that survive a failed search.
        # The dropped counts are surfaced instead.
        sources_dropped=sources_dropped,
        commanders_dropped=commanders_dropped,
        combinations_dropped=combinations_dropped,
        searched=len(turn.searched),
        slots=grounded,
        slots_dropped=dropped,
        reason="",
        never=("The reading is Claude's interpretation of what you said, not a "
               "finding. The cards and their colours are the pool's."),
    )


def _proposal_ask(grounded: list[dict[str, str]], budget: float | None,
                  avoid: str) -> str:
    """The closing instruction, carrying the readings and the filters.

    Constraints ride here rather than as slots, because they are filters on a
    query and not readings of a person (ADR 20). A beginner should not have to
    declare a budget before the tool will talk to them, and `price_max` is
    literally a `search_cards` argument.
    """
    lines = [
        ("That is the conversation. Here is what was heard, and every one of "
         "these is quoted from something they actually typed:"),
        "",
        json.dumps(grounded, indent=2),
        "",
    ]
    if budget:
        lines.append(f"They have about ${budget:g} for the whole deck, so pass "
                     f"price_max to search_cards and prefer commanders that do "
                     f"not need an expensive shell.")
    if avoid.strip():
        lines.append(f"Colours or things to steer away from, in their words: "
                     f"{avoid.strip()}")
    lines.append(
        "Propose two colour combinations and three commanders for each. The "
        "second combination should take a different reading of the same "
        "person, so the contrast shows them a choice was being made.")
    return "\n".join(lines)


def _combinations(raw: Any, allowed: set[str]
                  ) -> tuple[list[dict[str, Any]], int, int]:
    """Combinations with a real key, and commanders the pool confirms.

    Two checks in one pass, and both drop rather than repair. A `key` that is
    not one of the 32 is a combination that cannot be rendered or turned into a
    create-flow state, and a commander that does not resolve is a card the
    model invented -- the same instrument `dossier._competitors()` points at the
    same failure.
    """
    valid = {c["key"]: c for c in service.color_taxonomy()["combinations"]}
    out: list[dict[str, Any]] = []
    dropped = 0
    lost = 0
    for item in (raw or []):
        if not isinstance(item, dict):
            continue
        combo = valid.get(str(item.get("key") or "").strip().upper())
        if combo is None:
            lost += 1
            continue
        commanders, missed = _commanders(item.get("commanders"), combo, allowed)
        dropped += missed
        if not commanders:
            # A combination with no confirmed commander is a colour name and a
            # paragraph. The user cannot act on it, so it is not shown -- and
            # it is counted, because losing half a proposal silently is how a
            # measured run looks fine and a real one looks thin. Measured: a
            # combination goes when every legend named for it turned out to
            # have a *subset* identity, which is legal in those colours and
            # does not make a deck that fills that slot.
            lost += 1
            continue
        out.append({
            "key": combo["key"],
            "name": combo["name"],
            "colors": combo["colors"],
            "tier": combo["tier"],
            "tagline": combo["tagline"],
            # Kept apart all the way to the page, which is the acceptance
            # criterion rather than a nicety: one of these can be wrong and the
            # other cannot.
            "reading": prose(item.get("reading")),
            "grounding": prose(item.get("grounding")),
            "source_ids": [str(i) for i in (item.get("source_ids") or [])
                           if str(i) in allowed],
            "commanders": commanders,
        })
    return out[:2], dropped, lost


def _commanders(raw: Any, combo: dict[str, Any], allowed: set[str]
                ) -> tuple[list[dict[str, Any]], int]:
    """Named legends, resolved against the pool or dropped and counted.

    Two things are checked and the second is the one that matters. The name has
    to exist -- and the card has to actually be able to lead *this* combination,
    because a mono-white legend is legal in a Selesnya deck but a deck it leads
    is a mono-white deck and fills a different one of the 32. Rule 1 is not the
    only rule the pool is enforcing here.
    """
    items = [c for c in (raw or []) if isinstance(c, dict)]
    names = [str(c.get("card") or "").strip() for c in items]
    wanted = [n for n in names if n]
    if not wanted:
        return [], len(items)

    found = {c["name"].casefold(): c
             for c in service.cards_named(names=wanted).get("cards", [])}
    identity = set(combo["colors"])
    out: list[dict[str, Any]] = []
    dropped = 0
    for item, name in zip(items, names, strict=True):
        record = found.get(name.casefold())
        if record is None or set(record.get("color_identity") or []) != identity:
            dropped += 1
            continue
        out.append({
            "name": record["name"],
            "prose": prose(item.get("prose")),
            "source_ids": [str(i) for i in (item.get("source_ids") or [])
                           if str(i) in allowed],
            "mana_cost": record.get("mana_cost"),
            "type_line": record.get("type_line"),
            "oracle_text": record.get("oracle_text"),
            "color_identity": record.get("color_identity"),
            "image": record.get("image"),
            "art_crop": record.get("art_crop"),
        })
    return out[:3], dropped
