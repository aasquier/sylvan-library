"""Research: the questions the pool cannot answer, with the pages that can.

The sixth mode and ADR 15's fourth row --
[ADR 26](../../../docs/adr/0026-research-answers-about-magic-not-about-your-deck.md)
is the argument. The pool knows what every card does and the gate knows what is
legal; neither knows what people think of a card this month, what a ruling means
when it comes up, or that a set was spoiled last week. That is the whole of what
this mode is for.

**It cannot reach a deck, and that is the load-bearing decision.** No
`DeckSource` reaches `ask()`, `converse` is called with `source=None`, and the
tool set is `get_cards` and a hosted search. So the mode cannot read a
rationale, cannot see the 99, and cannot be asked what to cut from a list it has
never been shown -- rule 4 is out of reach rather than forbidden. The same
absence keeps **deck conversation** (ADR 15's third mode, deliberately unbuilt)
from being built here by accident: making this mode into that one means adding a
dependency on purpose, in a diff.

Three instruments hold what is left, and none of them is the system prompt:

**A cited page must be one the search actually returned.** `keep_sources()` is
`dossier.py`'s, reused rather than copied, for the reason ADR 19 gives: with a
response schema in play the API attaches no citations, so a URL in the payload
is a string the model typed. One step further here -- **a finding whose
citations all failed the check is dropped and counted**, because unlike a
dossier section there is no brief for it to be resting on instead. And if no
source survives at all, the answer is refused.

**A card the pool has is a pool fact; a card it lacks is a labelled claim.**
This is where the mode deliberately differs from the dossier, which drops an
unresolved rival. A card spoiled ahead of the next `data refresh` is a real card
the pool has never heard of, and dropping it would delete the third of this
feature people most want. So an unresolved name is *kept and marked*
`in_pool: false`, counted, and the reader gets a visible boundary instead of a
silent filter.

**A thin answer says it is thin.** `confidence` is ADR 25's `strength` argument
one mode along: with no field for "people disagree about this", a mode will
write consensus that is not there.

It writes nothing. `may_write` is empty, this package has no write door, and
`tests/test_claude_boundary.py` fails on the commit that adds one.
"""

from __future__ import annotations

import hashlib
import json
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

from mtglab.api import service
from mtglab.claude import stance as stance_mod
from mtglab.claude.dossier import WEB_SEARCH, canonical_url, keep_sources
from mtglab.claude.modes import Mode, Turn, converse

__all__ = ["RESEARCH", "QuestionRejected", "ask", "check_research",
           "check_question", "run_research", "only_grounded", "resolve_cards",
           "question_key", "stance_for",
           # Re-exported rather than reimplemented: a caller checking a URL
           # against this mode's sources must use the rule the check used.
           "canonical_url"]

#: How sure the answer is that it is *agreed*, not how sure it is that it is
#: right. Three levels for the reason `argue.STRENGTHS` has three: two collapse
#: "this is settled" into "this is arguable", and four invite a scale nobody
#: can calibrate.
CONFIDENCE = ("settled", "contested", "thin")

#: A question longer than this is a decklist, an essay, or a paste accident,
#: and none of the three is a research question. Refused in the request rather
#: than four minutes later.
MAX_QUESTION = 2000

#: More than this and an answer is a page rather than a reply. The interview
#: and the slot argument cap their lists for the same reason.
MAX_FINDINGS = 6

#: Enough to name the cards a question is actually about. `get_cards` takes 100
#: at a time; this is the ceiling on what comes *back* to the reader, who has to
#: look at every one of them.
MAX_CARDS = 12

_SOURCE_IDS: dict[str, Any] = {
    "type": "array",
    "items": {"type": "string"},
    "description": (
        "The ids of the sources this rests on, from the sources list. Never "
        "empty: every claim here is about something the pool does not hold, so "
        "there is always a page behind it or there is no claim."),
}

#: Note what is absent and cannot be added -- `additionalProperties: false` at
#: every level. There is no field for a deck, no field for a recommendation, no
#: field for a verdict on whether to run a card. The mode cannot see a deck, so
#: a field naming one would have nothing to put in it.
RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "answer": {
            "type": "string",
            "description": (
                "The direct answer, in two to five sentences. Lead with it. If "
                "the honest answer is that the pages disagree, or that you "
                "found little, say that here rather than picking a side."),
        },
        "findings": {
            "type": "array",
            "description": (
                "Two to six specific things the pages said, each one able to "
                "stand on its own. This is where the detail goes; the answer "
                "above is the summary."),
            "items": {
                "type": "object",
                "properties": {
                    "claim": {
                        "type": "string",
                        "description": (
                            "One thing you read, stated plainly in one or two "
                            "sentences. Attribute disagreement rather than "
                            "averaging it: 'cEDH primers rate it a trap while "
                            "casual lists run it' is a finding, 'opinions "
                            "vary' is not."),
                    },
                    "source_ids": _SOURCE_IDS,
                },
                "required": ["claim", "source_ids"],
                "additionalProperties": False,
            },
        },
        "cards": {
            "type": "array",
            "description": (
                "Every card your answer names, as exact printed names and "
                "nothing else. Each is looked up in the card pool and shown "
                "beside your prose with its real text. A name the pool does "
                "not have is **not** dropped -- it is shown marked as one the "
                "pool has not seen, which is the right answer for a card "
                "spoiled since the last data refresh. So include a spoiled "
                "card's name; do not include a name you are guessing at."),
            "items": {"type": "string"},
        },
        "confidence": {
            "type": "string",
            "enum": list(CONFIDENCE),
            "description": (
                "settled: the pages agree and the question has a known "
                "answer. contested: informed people disagree and the answer "
                "says so. thin: you found little, or only weak sources. Be "
                "honest -- reporting thin findings as settled is how this "
                "becomes worse than no answer."),
        },
        "sources": {
            "type": "array",
            "description": (
                "Every page you actually read, with the id the findings refer "
                "to. A page you did not open is not a source, and a URL you "
                "composed is checked against what the search returned and "
                "discarded."),
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
    "required": ["answer", "findings", "cards", "confidence", "sources"],
    "additionalProperties": False,
}


INSTRUCTIONS = """
You are the research surface in sylvan-library, a Commander deckbuilding tool.
Somebody has a question about Magic that the tool itself cannot answer, and you
have a web search and the card pool. Answer the question they asked, from pages
you actually read, and show them the pages.

What this surface is for, and it is a narrow list: the meta and what people
think of a card, what a ruling means when it comes up in practice, cards spoiled
ahead of the next bulk data refresh, and the history and discussion around an
archetype. Everything with a deterministic answer already has one here -- card
text, legality, colour identity, mana curves, category counts, prices,
simulation results -- and none of it is yours to re-derive.

**You cannot see anybody's deck, and you must not pretend otherwise.** This
surface has no access to the library: no decklist, no card rationales, no gate
output, no simulation. If the question assumes you can see a deck, say plainly
that you cannot and answer the general version of it instead. If the question
asks whether to cut or add a card in a deck, answer what is *known about the
card* and leave the decision where it belongs -- with the person who wrote the
deck, and with the tool's own deck page, which has the gate and the simulator
and the slot argument on it.

Sources:

- **Every claim you make about the meta, a ruling, a spoiler or an opinion must
  rest on a page you actually opened**, listed in `sources` and referenced by id
  from the finding it supports. Do not list a page you did not read, and do not
  compose a plausible URL: cited pages are checked against what the search
  actually returned, and anything else is discarded before the user sees it. A
  finding whose sources all fail that check is deleted, so an invented citation
  costs you the finding rather than buying you one.
- If the searches come back with nothing usable, say so. An answer with no
  surviving source is refused outright and the user is told why, which is the
  honest outcome; a fluent paragraph from memory is not.
- Prefer a primary source to somebody's summary of it. For a ruling, the rules
  document or an official ruling beats a forum post; for a spoiler, the preview
  article beats an aggregator.

Cards:

- **Call get_cards on every card you are going to name, before you write about
  it.** What you remember about a card is not evidence. This project has twice
  been wrong about cards it was confident about, and both were checkable facts.
- Colour identity is the pool's own field and accounts for back faces. Never
  infer it from a mana cost.
- List every card you name in `cards`, by exact printed name. Each is resolved
  against the pool and shown beside your answer with its real text.
- **A card the pool does not have is kept and shown as one the pool has not
  seen** -- that is the right answer for something spoiled since the last data
  refresh, and it is why this surface exists at all. When you write about such a
  card, everything you say about it must come from a page you cite, and you
  should say that the tool has not ingested it yet.

How to write it:

- Answer the question that was asked. Do not answer a larger, more interesting
  question next to it.
- Lead with the answer; put the detail in the findings.
- Where informed people disagree, say who thinks what. Averaging two positions
  into a bland one is the failure mode of this kind of writing.
- Set `confidence` honestly. `thin` on a real answer is worth more than
  `settled` on a manufactured one.
- Never write a rationale for a card's slot in a deck. That text is the user's
  to write, and this tool refuses to compose it anywhere.
""".strip()


RESEARCH = Mode(
    name="research",
    purpose="Answers a question about Magic from pages it read: the meta, a "
            "ruling in practice, a card spoiled ahead of the pool.",
    instructions=INSTRUCTIONS,
    # `get_cards` and nothing else from this side, and the absences are the
    # decision rather than an oversight (ADR 26). No `get_deck`, no
    # `list_decks`, no `validate_deck`, no `deck_stats`, no
    # `suggest_replacements`: a mode that cannot reach a deck cannot critique
    # one, and cannot quietly become the deck conversation ADR 15 leaves
    # unbuilt. `search_cards` is absent for a smaller reason -- shopping the
    # pool for candidates is what the slot argument is for, and this mode's
    # discovery tool is the web.
    tool_names=("get_cards",),
    server_tools=(WEB_SEARCH,),
    # Headroom for adaptive thinking, four searches' worth of results, and an
    # answer with its findings. The dossier's ceiling, for the same shape of
    # work; the interview's 8192 would truncate and the symptom is an answer
    # that fails to parse.
    max_tokens=16384,
    response_schema=RESPONSE_SCHEMA,
    # Its own scope table, because `modes._SCOPE_NOTES`' default talks about
    # "the card you were asked about" and there is no card here. What the scope
    # axis widens for this mode is how far past the literal question it may
    # read -- which is a real axis, and a different one.
    scope_notes=(
        ("flagged",
         ("Scope: answer the question as asked and nothing beside it. One "
          "search topic, the pages it returns, and a short answer.")),
        ("adjacent",
         ("Scope: the question, plus what somebody asking it would need next "
          "-- the card the answer turns on, the ruling behind the "
          "interaction, the set the spoiler came from. Follow those when they "
          "are load-bearing for the answer, not when they are merely "
          "interesting.")),
        ("rethink",
         ("Scope: the question and the assumption under it. If what was asked "
          "rests on something the pages say is not so -- a card that does not "
          "do what the asker thinks, a format that has moved on -- say that "
          "first and then answer. You are still answering their question, not "
          "replacing it with a better one.")),
    ),
)


class QuestionRejected(ValueError):
    """The question is empty, or long enough to be something other than one.

    Its own exception so the API can answer 422 rather than spending a search
    to discover the same thing.
    """


# --------------------------------------------------------------- the question

def check_question(raw: Any) -> str:
    """The question as a string, or `QuestionRejected`.

    Deliberately thin. There is no attempt to classify what the question is
    *about* -- a model deciding whether a question counts as "about a deck" is
    exactly the judgement-call guard this project keeps refusing, and the
    structural version is that the mode has no deck to reach (ADR 26).
    """
    question = str(raw or "").strip()
    if not question:
        raise QuestionRejected("Ask something. The research surface takes a "
                               "question about Magic in plain words.")
    if len(question) > MAX_QUESTION:
        raise QuestionRejected(
            f"That question is {len(question)} characters, and the ceiling is "
            f"{MAX_QUESTION}. Anything longer is usually a pasted decklist, "
            f"and this surface cannot see decks in any case -- ask the "
            f"question on its own.")
    return question


def question_key(question: str) -> str:
    """An identity for "somebody is asking this right now".

    Not a cache key -- nothing is stored (ADR 26). This is `jobs.submit`'s
    dedupe, so a second click inside the minutes a search takes joins the run
    already in flight instead of paying for it twice. Whitespace and case are
    normalised because they do not make it a different question; anything else
    does.
    """
    normalised = " ".join(question.split()).casefold()
    return f"research:{hashlib.sha256(normalised.encode()).hexdigest()[:16]}"


# ------------------------------------------------------- reading the answer

def only_grounded(items: list[Any], allowed: set[str],
                  ) -> tuple[list[dict[str, Any]], int]:
    """Keep the findings that cite a surviving source; count the rest.

    The family's third instrument, aimed one step past where the dossier aims
    it. `dossier._section` narrows a passage's `source_ids` and keeps the
    prose, which is right there: a dossier section may legitimately rest
    entirely on the pool facts in its brief. **This mode has no brief.** Its
    subject is whatever was asked, so a finding with no surviving citation is
    resting on nothing, and what "nothing" means in practice is the model's
    recall -- rule 1's original failure with a search box drawn around it.

    The count comes back rather than being swallowed, for the reason
    `only_questions` and `only_charges` return theirs: a mode that has started
    inventing citations should show up as a number rather than as confidence.
    """
    kept: list[dict[str, Any]] = []
    dropped = 0
    for item in items:
        if not isinstance(item, dict):
            dropped += 1
            continue
        claim = str(item.get("claim", "")).strip()
        ids = [str(i) for i in (item.get("source_ids") or []) if str(i) in allowed]
        if not claim or not ids:
            dropped += 1
            continue
        kept.append({"claim": claim, "source_ids": ids})
    return kept, dropped


def resolve_cards(names: list[Any]) -> tuple[list[dict[str, Any]], int]:
    """Resolve every named card against the pool. Label what is missing.

    **The deliberate difference from `dossier._competitors`, and the reason
    ADR 26 exists.** A competing commander that does not resolve is a card the
    model invented, so the dossier drops it. A card that does not resolve *here* may
    be that -- or it may be a card spoiled since the last `data refresh`, which
    is one of the three things this surface is for. The two are
    indistinguishable from inside `get_cards`, so the mode does not try: it
    keeps both and marks them, and the reader gets a boundary they can see
    rather than a filter they cannot.

    Returns the cards in the order named, and how many the pool did not have.
    `in_pool` is the field that matters on every one of them.
    """
    wanted: list[str] = []
    seen: set[str] = set()
    for raw in names:
        name = str(raw or "").strip()
        key = name.casefold()
        if name and key not in seen:
            seen.add(key)
            wanted.append(name)
    if not wanted:
        return [], 0

    looked_up = service.cards_named(names=wanted[:MAX_CARDS])
    # Indexed under both spellings, because a double-faced card resolves from
    # either face and comes back under its full `A // B` name. An index keyed
    # only on the pool's spelling misses every DFC named by its front face, and
    # misses it silently -- the bug `argue.resolve_alternatives` was written
    # around, and the same fix.
    by_key: dict[str, dict[str, Any]] = {}
    for record in looked_up.get("cards", []):
        by_key[record["name"].casefold()] = record
        if record.get("asked_as"):
            by_key[record["asked_as"].casefold()] = record

    out: list[dict[str, Any]] = []
    already: set[str] = set()
    unresolved = 0
    for name in wanted[:MAX_CARDS]:
        record = by_key.get(name.casefold())
        if record is None:
            unresolved += 1
            # Kept, and kept honest. `in_pool: false` is what a client renders
            # differently, and `name` is the model's spelling because there is
            # no other spelling to offer.
            out.append({"name": name, "in_pool": False})
            continue
        if record["name"] in already:
            continue
        already.add(record["name"])
        out.append({
            "name": record["name"],
            "in_pool": True,
            "mana_cost": record.get("mana_cost"),
            "type_line": record.get("type_line"),
            "oracle_text": record.get("oracle_text"),
            "color_identity": record.get("color_identity"),
            "image": record.get("image"),
            "art_crop": record.get("art_crop"),
            "legal_commander": record.get("legal_commander"),
        })
    return out, unresolved


def _now() -> str:
    return datetime.now(UTC).isoformat()


def _report(turn: Turn | None, *, question: str,
            effective: stance_mod.Stance, body: dict[str, Any] | None = None,
            asked: bool = True, reason: str = "") -> dict[str, Any]:
    """One response shape for every outcome, including not asking at all.

    `answered_by` is ADR 14's third boundary as a field. This mode needs it
    more than any built so far: its output is prose about Magic with citations
    under it, which is the exact look of something reproducible, and it is not.
    """
    return {
        "answered_by": "claude",
        "mode": RESEARCH.name,
        "model": turn.model if turn else "",
        "question": question,
        "asked": asked,
        "reason": reason,
        "stance": stance_mod.describe(effective),
        # Empty until one exists, so a client never has to tell a missing key
        # from an absent answer.
        "research": body or {},
        # When it was written. Nothing is cached (ADR 26) and the subject moves,
        # so this is the honest substitute for a freshness claim rather than a
        # cache stamp.
        "generated_at": _now(),
        "usage": {"input_tokens": turn.input_tokens if turn else 0,
                  "output_tokens": turn.output_tokens if turn else 0,
                  "cache_read_tokens": turn.cache_read_tokens if turn else 0},
        "never": ("This is Claude's reading of the cited pages, not the "
                  "tool's own answer. It has not seen any of your decks."),
    }


# ------------------------------------------------------------- the stance

#: What "no preference" means here. `second-opinion` and not `collaborator`:
#: this mode answers a question that was asked, and `collaborator`'s write axis
#: has nothing to reach on a surface with no deck in it.
DEFAULT_PRESET = "second-opinion"


def stance_for(requested: Any) -> stance_mod.Stance:
    """This surface's default stance, and the clamp over what was asked for.

    There is no deck to derive a default from, and `stance.resolve(None, None)`
    is `off` -- the right answer for "I have no idea what this is about" and
    the wrong one for a screen whose only control is a question box. Somebody
    typing a question has asked for a call.

    **Public for the reason `theme.stance_for` is**, and it is the same bug
    waiting in the same place: `/api/claude` renders the dial's readout, and
    with no slug to name it would resolve `off` while this function was about
    to run the search at `second-opinion`. A surface's default is asked of the
    module that owns it, never copied into the endpoint.
    """
    if requested is None:
        return stance_mod.clamp(stance_mod.PRESETS[DEFAULT_PRESET],
                                stance_mod.ceiling())
    return stance_mod.resolve(requested)


# ------------------------------------------------------ the two halves

@dataclass
class ResearchRequest:
    """What `check_research` settled, and everything `run_research` needs.

    The split `api/simruns.py` introduced and `api/dossierruns.py` had to be
    retrofitted with. Here it is present from the first commit: everything
    refusable is refused in the request, and only the Anthropic call is queued.

    Note what this dataclass does *not* carry, and could not: a `DeckSource`,
    a slug, a deck. ADR 26's first decision is visible in the type.
    """

    question: str
    key: str
    effective: stance_mod.Stance
    answer: dict[str, Any] | None = None

    @property
    def needs_call(self) -> bool:
        """Whether anything still has to be asked of Anthropic."""
        return self.answer is None


def check_research(question: Any, *, requested: Any = None) -> ResearchRequest:
    """Everything that can be decided without spending anything.

    Raises `QuestionRejected` to the caller, which is what keeps its 422 rather
    than flattening it into a job in state `error` some minutes later.

    **The stance resolves with no deck**, and that is the case `/api/claude`
    got wrong once already: `stance.resolve(None, None)` answers `off`, which
    would make this surface silently do nothing. So the default is asked of
    this module, the way `theme.stance_for` owns the create flow's.
    """
    text = check_question(question)
    effective = stance_for(requested)
    request = ResearchRequest(question=text, key=question_key(text),
                              effective=effective)
    if not effective.allows_calls:
        request.answer = _report(
            None, question=text, effective=effective, asked=False,
            reason="The stance is off, so no call was made. Nothing else "
                   "about the app is affected.")
    return request


def run_research(request: ResearchRequest, *,
                 on_turn: Callable[[int, int], None] | None = None,
                 ) -> dict[str, Any]:
    """Make the call, check what came back, and hand it over.

    `on_turn` goes straight to `converse` and exists because this runs as a
    background job: minutes of reporting nothing is indistinguishable from a
    wedged worker.

    Nothing is stored at the end of this function, and the absence is the
    decision -- see ADR 26 on why a research answer is the one thing in this
    codebase that must *not* be cached.
    """
    if request.answer is not None:
        return request.answer

    question, effective = request.question, request.effective

    turn = converse(
        RESEARCH,
        messages=[{"role": "user", "content": _ask_for(question)}],
        stance=effective,
        # No deck source, and the `None` is the point rather than a default
        # nobody filled in. `tools.run` receives it for `get_cards`, which does
        # not take one; every tool that would have needed one is absent from
        # this mode's tool set.
        source=None,
        # A search, a look at what came back, a lookup of the cards named, a
        # second search, and the write-up. The dossier's eight, and for the
        # same reason: a paused turn can spend one.
        max_turns=8,
        on_turn=on_turn,
    )

    if turn.refused:
        return _report(turn, question=question, effective=effective,
                       reason="The model declined to answer this one.")
    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        return _report(turn, question=question, effective=effective,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}).")

    sources, sources_dropped = keep_sources(payload.get("sources") or [],
                                            turn.searched)
    if not sources:
        # ADR 26, following ADR 19: an unsourced research answer is a model
        # talking about Magic from memory with a search box drawn around it.
        # Refusing is the only answer that keeps the seam meaningful.
        detail = (f" {sources_dropped} cited page(s) were not among the "
                  f"{len(turn.searched)} the search returned."
                  if sources_dropped else "")
        if turn.search_errors:
            detail += (f" The search itself reported: "
                       f"{'; '.join(turn.search_errors)}.")
        return _report(turn, question=question, effective=effective,
                       reason="No source survived checking, so there is "
                              "nothing to stand behind an answer." + detail)

    allowed = {s["id"] for s in sources}
    findings, findings_dropped = only_grounded(payload.get("findings") or [],
                                               allowed)
    cards, cards_unresolved = resolve_cards(payload.get("cards") or [])
    confidence = str(payload.get("confidence") or "").strip()

    body = {
        "answer": str(payload.get("answer") or "").strip(),
        "findings": findings[:MAX_FINDINGS],
        "cards": cards,
        # Fall back rather than drop, the way `argue.only_charges` falls back
        # on an unrecognised ground: an answer is worth more than its label,
        # and `thin` is the honest default for one that arrived mislabelled.
        "confidence": confidence if confidence in CONFIDENCE else "thin",
        "sources": sources,
        # All three surfaced rather than logged. A number that climbs is a
        # prompt that has started inventing, and nobody checks a number they
        # cannot see.
        "sources_dropped": sources_dropped,
        "findings_dropped": findings_dropped,
        # How many named cards the pool has never heard of. Not an error: for a
        # spoiler question the right value is greater than zero.
        "cards_unresolved": cards_unresolved,
        "searched": len(turn.searched),
    }
    return _report(turn, question=question, effective=effective, body=body)


def _ask_for(question: str) -> str:
    """The question, framed as the user's rather than as the tool's.

    Quoted rather than interpolated into an instruction, the same treatment
    `argue.ask` gives its `focus`: what follows is something a person typed,
    not something this module decided to ask.
    """
    opening = ("Here is the question, in the user's own words. Search for what "
               "you need, look up every card you are going to name, and "
               "answer it.")
    return "\n".join([opening, "", question])


def ask(question: Any, *, requested: Any = None) -> dict[str, Any]:
    """Answer one research question. Both halves, back to back.

    What `mtglab claude research` wants: a terminal holds a connection open for
    minutes perfectly happily, so the CLI has no reason to poll a job. The API
    does, and reaches for the two halves separately through
    `api/researchruns.py`.

    **At `initiative: off` this makes no call and says so** -- and unlike the
    dossier there is no stored row to serve instead, because nothing here is
    stored.
    """
    return run_research(check_research(question, requested=requested))
