"""Argue a slot: the case against one card, and only against it.

The second mode
[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
names, and the one that sits closest to the line rule 4 draws. The rationale
interview returns *questions*, and `only_questions()` deletes anything that is
not one -- a declarative sentence about a card's merit is the first inch of the
slope, so it fails visibly. This mode's entire output is declarative sentences
about a card's merit. The guard therefore has to be a different shape, because
the interview's guard would delete the feature.

**The guard is that it argues one direction.** This mode makes the case
*against* a slot and has no way to make the case *for* it. That asymmetry is
not a tone preference; it is what keeps rule 4 structural instead of prompted.
A mode that argued both sides would hand back a paragraph explaining why a card
earns its place -- a `why` in everything but authorship, one paste away from the
field ADR 8 exists to protect. There is no honest version of "argue both sides"
that is not a rationale generator, so this mode does not have a side to switch
to. See [ADR 25](../../../docs/adr/0025-argue-a-slot-argues-one-direction.md).

Four things hold it, and as with the interview none of them is the system
prompt:

**The schema has no field for keeping the card.** No `defence`, no `in_favour`,
no `verdict`, no `summary`, and `additionalProperties: false` so one cannot be
added at run time. The model can report that every charge it found is weak --
`strength` is how -- but it cannot write the sentence that says why the card is
good, because there is nowhere to put it.

**Every charge must cite a fact.** `only_charges()` drops a charge whose `fact`
is empty and counts what it dropped, the same instrument and the same reason as
`only_questions()`: an argument with no citation is an opinion floating free,
and this project has been wrong twice by reasoning from memory about cards it
was confident about.

**Alternatives are names, and deterministic Python judges them.** The model may
name cards that do the job instead; it may not say why they are better. Each
name is resolved through the pool and **dropped if it does not resolve, is not
Commander-legal, or falls outside the deck's colour identity** -- counted in all
three cases. That is ADR 19's rival instrument pointed at rule 1 and
non-negotiable 2, and it is the specific failure `CLAUDE.md` records: *Ajani,
Nacatl Pariah* was proposed for a G/W deck and its back face is R/W. A model
cannot make that mistake here, because it is not the thing deciding.

Prose about a replacement is absent for the same reason it is absent about the
card itself, and the division is ADR 14's: where a deterministic scorer exists,
`suggest_replacements` already returns the reasons each candidate scored well.
Where one does not, the card's own oracle text renders beside the name and is
the better evidence anyway.

**The facts arrive before the model does.** `interview.brief()` is reused
verbatim rather than reimplemented -- the oracle text, the gate's verdict, the
category counts against the bracket's targets, the curve, and the neighbouring
cards' rationales are exactly what a case against a slot has to rest on, and a
second assembler that drifted from the first would be two rules to keep instead
of one.
"""

from __future__ import annotations

import json
from typing import Any

from mtglab.api import service
from mtglab.claude import stance as stance_mod
from mtglab.claude.interview import CardNotInDeck, brief
from mtglab.claude.modes import Mode, Turn, converse
from mtglab.decks.source import DeckSource

__all__ = ["CardNotInDeck", "SLOT_ARGUMENT", "ask", "only_charges",
           "resolve_alternatives"]

#: What a charge can rest on. An enum rather than free text so the UI can group
#: the case, and so the grounds this project actually cares about are
#: first-class rather than ones a model might not reach for. `redundancy` and
#: `count` are here because they are the two the pool can nearly prove.
GROUNDS = ("redundancy", "cost", "speed", "conditionality", "count",
           "ceiling", "legality")

#: How much weight a charge carries. Three levels, because two collapses
#: "this is fatal" into "this is worth a thought" and four invites a scale
#: nobody can calibrate.
STRENGTHS = ("decisive", "serious", "minor")

#: The shape of the case. Note what is absent and cannot be added:
#: `additionalProperties: false` at every level, and no property anywhere that
#: holds a reason to keep the card. `fact` is a citation -- what the brief, the
#: pool or the gate said -- never an argument.
RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "charges": {
            "type": "array",
            "description": (
                "The case against this card's slot, strongest first. Three to "
                "five. If the honest case is weak, say so by marking the "
                "charges minor -- do not pad it, and do not argue the other "
                "way."),
            "items": {
                "type": "object",
                "properties": {
                    "claim": {
                        "type": "string",
                        "description": (
                            "One argument against this card holding this slot "
                            "in this deck, stated plainly and in one or two "
                            "sentences. It argues against. There is no field "
                            "for an argument in favour and you must not put "
                            "one here."),
                    },
                    "ground": {
                        "type": "string",
                        "enum": list(GROUNDS),
                        "description": (
                            "redundancy: something in the deck already does "
                            "this. cost: the mana or the card is not worth "
                            "what it buys. speed: too slow for this deck's "
                            "clock. conditionality: needs too much to be true "
                            "before it does anything. count: its category is "
                            "already at or over its target. ceiling: what it "
                            "does at its best is not enough. legality: the "
                            "gate flagged it."),
                    },
                    "fact": {
                        "type": "string",
                        "description": (
                            "The one fact from the brief, the pool or the gate "
                            "that this charge rests on -- oracle text, a mana "
                            "value, a category count against its target, a "
                            "curve bucket, a gate error, a sibling card's "
                            "rationale. Quoted or paraphrased plainly. A fact, "
                            "never a view about whether the card is good. A "
                            "charge you cannot cite is a charge to leave out."),
                    },
                    "strength": {
                        "type": "string",
                        "enum": list(STRENGTHS),
                        "description": (
                            "decisive: on its own this is enough to cut the "
                            "card. serious: it needs answering. minor: worth "
                            "knowing, not worth acting on alone. Be honest "
                            "here -- marking a thin charge decisive is how "
                            "this becomes noise."),
                    },
                },
                "required": ["claim", "ground", "fact", "strength"],
                "additionalProperties": False,
            },
        },
        "alternatives": {
            "type": "array",
            "description": (
                "Up to six cards that could do this slot's job instead. "
                "**Names only.** Every one is checked against the pool, the "
                "Commander ban list and the deck's colour identity before it "
                "is shown, and one that fails any of those is dropped -- so a "
                "name you are unsure of costs nothing but is also not worth "
                "guessing at. Leave the list empty when the charge is that "
                "the slot itself should go."),
            "items": {
                "type": "string",
                "description": (
                    "An exact card name and nothing else. No reason, no "
                    "comparison, no parenthetical. The card's own text is "
                    "shown beside it and is better evidence than a sentence "
                    "about it."),
            },
        },
    },
    "required": ["charges", "alternatives"],
    "additionalProperties": False,
}


INSTRUCTIONS = """
You are the slot argument in sylvan-library, a Commander deckbuilding tool.
Someone is looking at one card in one deck and wants the strongest honest case
that it does not deserve its slot. Your job is to make that case, from the
facts in front of you, as well as it can be made.

You argue one direction. There is no field in your answer for a reason to keep
the card, and that is deliberate rather than an oversight: a paragraph
explaining why a card earns its place is a rationale, this deck file records
rationales the user wrote, and a tool-written one is the appearance of thinking
rather than thinking. So do not write the counter-case, do not close with a
balanced summary, do not add "that said..." to a charge. The user is the one
weighing this. You are the case against; they already know the case for,
because in most decks they wrote it.

Being one-directional is not permission to be unfair. The two failures here are
opposite and both are bad:

- **Padding.** Four thin charges dressed as serious ones make a good card look
  cuttable and teach the user to ignore you. If the honest case is weak, return
  fewer charges and mark them minor. Three minor charges on a card that is
  clearly correct is a complete and useful answer.
- **Softening.** If the card is genuinely bad here, say so plainly and mark the
  charge decisive. A tool that hedges on an obvious cut is worth nothing on the
  hard ones.

What makes a charge good:

- It rests on something specific from the brief. "There are already six ramp
  spells and four of them cost less" is a charge. "This feels win-more" is not.
- It is about *this deck*. A card that is weak in the abstract and load-bearing
  here has no charge against it, and a fine card that is the ninth thing at
  three mana does.
- It engages what the user claimed. If the card has a rationale, take it
  seriously and then test it: is the claim still true, is it the real reason,
  does a sibling card's rationale claim the same job?
- It is willing to be unwelcome. The most useful charge is usually the one
  about the card somebody is attached to.

Facts:

- Every claim you make about a card comes from the brief or from the get_cards
  tool. Your recollection of a card is not evidence. This project has already
  been wrong twice by reasoning from memory about cards it was confident about.
- Colour identity is the pool's own field. Never infer it from a mana cost.
- The gate -- legality, colour identity, singleton, deck size, category counts
  -- is deterministic Python and it has already run. Do not re-derive its
  verdict or contradict it.
- Alternatives are names. Deterministic Python checks each one against the
  pool, the ban list and the deck's colour identity, and drops what fails, so
  do not check those yourself and do not explain your picks.
- A card the pool does not have is a card you do not know. Leave it out.
""".strip()


SLOT_ARGUMENT = Mode(
    name="slot-argument",
    purpose="Makes the case against one card's slot. Never the case for it.",
    instructions=INSTRUCTIONS,
    # ADR 15's table for this mode, verbatim: get_cards, search_cards, suggest,
    # analyze, validate. `get_deck` is *not* on it and is not added here --
    # `brief()` already hands over the deck, the category and the siblings'
    # rationales, so the one thing a deck read would add is the other 88 cards'
    # prose, which is a bigger prompt in exchange for a wider argument than
    # this mode is scoped to make. `search_cards` and `suggest_replacements`
    # are what this mode has and the interview deliberately does not: shopping
    # for a replacement is the difference between the two surfaces.
    tool_names=("get_cards", "search_cards", "suggest_replacements",
                "deck_stats", "validate_deck"),
    response_schema=RESPONSE_SCHEMA,
)


# ------------------------------------------------------- reading the answer

#: More than this and a case becomes a pile. The interview caps questions for
#: the same reason and at the same number's neighbourhood.
MAX_CHARGES = 5

#: Enough to start shopping, few enough to read. `suggest_replacements` uses
#: five as its own default and this sits one above it deliberately: a model
#: that also reached for `search_cards` should not have its extra find
#: truncated away.
MAX_ALTERNATIVES = 6


def only_charges(items: list[Any]) -> tuple[list[dict[str, Any]], int]:
    """Keep the charges that cite something; count what did not.

    The interview's `only_questions()` one mode along. There the predicate is
    "it ends in a question mark", because the failure guarded against is a
    declarative sentence. Here every item is declarative by design, so the
    predicate moves to the thing that actually separates an argument from an
    opinion: **a charge with no `fact` is not a charge.**

    The count comes back rather than being swallowed, because "it dropped two"
    is how a mode that has started asserting rather than citing becomes visible
    instead of merely persuasive.
    """
    kept: list[dict[str, Any]] = []
    dropped = 0
    for item in items:
        if not isinstance(item, dict):
            dropped += 1
            continue
        claim = str(item.get("claim", "")).strip()
        fact = str(item.get("fact", "")).strip()
        if not claim or not fact:
            dropped += 1
            continue
        ground = str(item.get("ground", "")).strip()
        strength = str(item.get("strength", "")).strip()
        kept.append({
            "claim": claim,
            "fact": fact,
            # Fall back rather than drop: an unrecognised enum value is a
            # labelling miss, and throwing away a cited argument over its tag
            # would cost more than it protects.
            "ground": ground if ground in GROUNDS else "ceiling",
            "strength": strength if strength in STRENGTHS else "minor",
        })
    return kept, dropped


def resolve_alternatives(names: list[Any], *, identity: list[str],
                         ) -> tuple[list[dict[str, Any]], dict[str, list[str]]]:
    """Resolve suggested names against the pool, and drop what does not survive.

    Three filters, and Python owns all three because all three have right
    answers (ADR 14). A name that does not resolve is a card the model made up
    or misspelled. A card that is banned cannot go in. **A card outside the
    commander's colour identity cannot go in either, and that check is the
    reason this function exists** -- `CLAUDE.md`'s first recorded error is
    *Ajani, Nacatl Pariah* proposed for a G/W deck, whose back face is R/W and
    whose identity therefore is not. Rule 2: the pool's `color_identity` field
    already accounts for back faces, so it is read and never derived.

    Returns the survivors and a breakdown of what went, by reason. Dropped and
    *counted*, never dropped quietly -- the same instrument ADR 19 built for
    the dossier's competitors, and for the same reason: a list that silently got
    shorter reads as a model that had less to say. A dropped card is reported
    under **the pool's spelling**, which for a double-faced card is the full
    `A // B` name: naming both faces is what makes an off-colour verdict on
    Ajani legible rather than baffling.
    """
    wanted = []
    seen: set[str] = set()
    for raw in names:
        name = str(raw or "").strip()
        key = name.casefold()
        if name and key not in seen:
            seen.add(key)
            wanted.append(name)

    dropped: dict[str, list[str]] = {"not_in_pool": [], "banned": [],
                                     "off_colour": [], "no_pool": []}
    if not wanted:
        return [], dropped

    looked_up = service.cards_named(names=wanted)
    if not looked_up.get("pool_available", True):
        # Every name comes back unresolved when there is no pool, and filing
        # that under `not_in_pool` would accuse the model of inventing six
        # cards. A base install has no pool and this mode still has to say
        # something true.
        dropped["no_pool"] = list(wanted)
        return [], dropped

    # Indexed under both spellings. A double-faced card resolves from either
    # face but comes back under its full `A // B` name, so an index keyed only
    # on the pool's spelling misses every DFC a model names by its front face
    # -- and misses it *silently*, which is the one thing this function exists
    # not to do. `asked_as` is what `cards_named` returns for exactly this.
    by_key: dict[str, dict[str, Any]] = {}
    for record in looked_up["cards"]:
        by_key[record["name"].casefold()] = record
        if record.get("asked_as"):
            by_key[record["asked_as"].casefold()] = record

    dropped["not_in_pool"] = list(looked_up["not_found"])
    allowed = set(identity)
    kept: list[dict[str, Any]] = []
    already: set[str] = set()
    for name in wanted:
        record = by_key.get(name.casefold())
        if record is None:
            # Belt and braces: `not_found` should already hold it. If some
            # future resolution rule lets a name fall between the two, it gets
            # counted here rather than disappearing.
            if name not in dropped["not_in_pool"]:
                dropped["not_in_pool"].append(name)
            continue
        # Two faces of one card are one card. Deduplicated on the pool's
        # spelling rather than the caller's, which is the only stable identity.
        if record["name"] in already:
            continue
        already.add(record["name"])
        if not record["legal_commander"]:
            dropped["banned"].append(record["name"])
            continue
        if not set(record["color_identity"]) <= allowed:
            dropped["off_colour"].append(record["name"])
            continue
        kept.append(record)
    return kept[:MAX_ALTERNATIVES], dropped


def _report(turn: Turn | None, *, slug: str, card: str,
            effective: stance_mod.Stance,
            charges: list[dict[str, Any]], dropped: int,
            alternatives: list[dict[str, Any]] | None = None,
            alternatives_dropped: dict[str, list[str]] | None = None,
            asked: bool = True, reason: str = "") -> dict[str, Any]:
    """One response shape for every outcome, including not asking at all.

    `answered_by` is ADR 14's third boundary made a field, exactly as in the
    interview: the gate's output and Claude's never share a surface without a
    label, and this mode needs it more than the interview did. An interview
    returns questions, which nobody mistakes for a verdict; this returns a
    reasoned case against a card, which reads like one.
    """
    return {
        "answered_by": "claude",
        "mode": SLOT_ARGUMENT.name,
        "model": turn.model if turn else "",
        "slug": slug,
        "card": card,
        "asked": asked,
        "reason": reason,
        "stance": stance_mod.describe(effective),
        "charges": charges,
        "charges_dropped": dropped,
        "alternatives": alternatives or [],
        # Kept per reason rather than as a total. "Two were off-colour" and
        # "two do not exist" say different things about the answer, and a
        # single number says neither.
        "alternatives_dropped": alternatives_dropped or {
            "not_in_pool": [], "banned": [], "off_colour": [], "no_pool": []},
        "tool_calls": turn.tool_calls if turn else [],
        "usage": {"input_tokens": turn.input_tokens if turn else 0,
                  "output_tokens": turn.output_tokens if turn else 0,
                  "cache_read_tokens": turn.cache_read_tokens if turn else 0},
        # Said in the payload rather than only in the UI, so a second client
        # cannot render this as anything other than what it is.
        "never": ("This is the case against the card, and only that. A card "
                  "that survives it still needs a rationale, and the rationale "
                  "is yours to write."),
    }


def ask(slug: str, card: str, *, requested: Any = None, focus: str = "",
        source: DeckSource | None = None) -> dict[str, Any]:
    """Make the case against one card's slot.

    Same signature and same stance handling as `interview.ask`, deliberately:
    these are the two per-card modes and a client that can drive one should not
    have to learn a second shape to drive the other.

    **At `initiative: off` this makes no call and says so** -- `asked: false`
    and a reason, never an empty case that reads as "nothing to say about this
    card", which is the opposite of what an unset dial means.
    """
    facts = brief(slug, card, source=source)
    deck_obj = type("D", (), {"status": facts["deck"]["status"]})()
    effective = stance_mod.resolve(requested, deck=deck_obj)

    if not effective.allows_calls:
        return _report(None, slug=slug, card=facts["card"]["name"],
                       effective=effective, charges=[], dropped=0,
                       asked=False,
                       reason="The stance is off, so no call was made. "
                              "Everything else about this deck still works.")

    ask_for = [
        ("Here is everything the tool already knows about this card in this "
         "deck. All of it is deterministic: the card text is the pool's, the "
         "verdict is the gate's, the counts are computed."),
        "",
        json.dumps(facts, indent=2, default=str),
        "",
        ("Make the strongest honest case that this card does not deserve its "
         "slot. If that case is weak, return fewer charges and mark them "
         "minor rather than padding it."),
    ]
    if focus.strip():
        # The user's own steer, quoted rather than interpolated into the
        # instruction, so it reads as something they said rather than
        # something the tool decided.
        ask_for += ["", f"What I am weighing, in my words: {focus.strip()}"]

    turn = converse(
        SLOT_ARGUMENT,
        messages=[{"role": "user", "content": "\n".join(ask_for)}],
        stance=effective,
        source=source,
    )

    if turn.refused:
        return _report(turn, slug=slug, card=facts["card"]["name"],
                       effective=effective, charges=[], dropped=0,
                       asked=True,
                       reason="The model declined to answer this one.")

    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        # The schema makes this close to impossible, and "close to" is why the
        # branch exists: a truncated answer is not valid JSON either.
        return _report(turn, slug=slug, card=facts["card"]["name"],
                       effective=effective, charges=[], dropped=0,
                       asked=True,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}). Nothing was written.")

    charges, dropped = only_charges(payload.get("charges") or [])
    alternatives, alt_dropped = resolve_alternatives(
        payload.get("alternatives") or [],
        identity=facts["deck"]["color_identity"])
    return _report(turn, slug=slug, card=facts["card"]["name"],
                   effective=effective, charges=charges[:MAX_CHARGES],
                   dropped=dropped, alternatives=alternatives,
                   alternatives_dropped=alt_dropped)
