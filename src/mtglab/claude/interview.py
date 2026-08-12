"""The rationale interview: it asks, you answer, your answer is the rationale.

The first mode, and the one
[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
was written before rather than after -- because this is exactly where rule 4
is easiest to lose. `decks import` brings in ninety-nine cards owing ninety-nine
rationales, the obvious help is a tool that writes them, and every step from
"ask a question" to "summarise what we just discussed into a rationale" is
small and defensible on its own. So the line is drawn where it can be checked
instead of promised.

Three things hold it, and none of them is the system prompt:

**There is no write door.** `tools.READ_ONLY` is the entire surface this
package can reach, and `tests/test_claude_boundary.py` fails on the commit that
names a write function anywhere under `src/mtglab/claude/`. The prompt below
asks the model not to draft a rationale; the reason that request is safe to
rely on is that nothing bad happens if it is ignored.

**The answer's shape has no field for one.** `RESPONSE_SCHEMA` has a slot for
a question and a slot for the corpus fact behind it, and no slot for prose
about the card's merit. A model that wanted to hand over a draft rationale has
nowhere to put it.

**Every question is checked to be a question.** `only_questions()` drops
anything that does not end in a question mark and reports how many it dropped.
That is a blunt instrument and it is meant to be: a declarative sentence in the
questions column is the first inch of the slope, and it now fails visibly
rather than reading as helpful.

**Facts arrive before the model does.** Rule 1 says card facts come from the
corpus rather than recall. The mode has `get_cards` and is told to use it, but
the card actually under discussion never depends on that: `brief()` assembles
the oracle text, the gate's verdict, the category counts and the sibling
rationales in deterministic Python and hands them over in the opening message.
The tool is for the cards the conversation *reaches for*; the card in front of
it is already on the table. That matters most in the case this project is
deliberately wrong about -- a banned card comes back from `cards_named` with
its real text and `legal_commander: false`, so the interview can ask about the
ban rather than discovering it cannot look the card up.
"""

from __future__ import annotations

import json
from typing import Any

from mtglab.api import service
from mtglab.claude import stance as stance_mod
from mtglab.claude.modes import Mode, Turn, converse
from mtglab.decks.source import DeckSource

#: The kinds of question worth asking about a slot. An enum rather than free
#: text so a UI can group them, and so "what would make you cut it" is a
#: first-class angle rather than one the model might not think of.
ANGLES = ("role", "alternative", "redundancy", "cost", "cut", "legality")

#: The shape of an answer. Note what is absent: there is no `rationale`,
#: `draft`, `suggestion` or `summary` field, and `additionalProperties: false`
#: means one cannot be added by the model at run time. `fact` is a citation --
#: what the corpus or the gate said -- not an argument about the card.
RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "questions": {
            "type": "array",
            "description": "Three to five questions, hardest first.",
            "items": {
                "type": "object",
                "properties": {
                    "question": {
                        "type": "string",
                        "description": (
                            "A question addressed to the user, in the second "
                            "person, ending in a question mark. It must be "
                            "answerable only by them -- if you could answer it "
                            "yourself from the corpus, it is not a question "
                            "for this list."),
                    },
                    "angle": {
                        "type": "string",
                        "enum": list(ANGLES),
                        "description": (
                            "role: what job it does here. alternative: what "
                            "else could do that job. redundancy: what already "
                            "does it. cost: whether the mana or the card is "
                            "worth it. cut: what would have to be true to cut "
                            "it. legality: the gate flagged something."),
                    },
                    "fact": {
                        "type": "string",
                        "description": (
                            "The one corpus or gate fact this question rests "
                            "on, quoted or paraphrased plainly -- oracle text, "
                            "a mana value, a category count, a gate error. A "
                            "fact, never a view about whether the card is "
                            "good."),
                    },
                },
                "required": ["question", "angle", "fact"],
                "additionalProperties": False,
            },
        },
    },
    "required": ["questions"],
    "additionalProperties": False,
}


INSTRUCTIONS = """
You are the rationale interview in sylvan-library, a Commander deckbuilding
tool. Someone is deciding what to write in the `why` field for one card in one
deck -- the sentence or two that says why that card earns its slot. Your job is
to ask them the questions that make writing it possible, and that make a weak
slot obvious before it gets defended.

You do not write the rationale. Not a draft, not an example, not a tidied-up
version of something they typed, not a sentence they could paste. That is not a
limitation of your tools, it is the point of the feature: a justification the
tool wrote is the appearance of a justification, and this deck file is supposed
to record what a person actually thought. You supply the questions. They supply
the words.

So: no declarative sentences about whether the card is good, no "consider
that...", no "you might say...". Every item you return is a question they have
to answer themselves.

What makes a good question here:

- It is answerable only by them. "What does this deck do when it draws this on
  turn nine?" is a question. "What is this card's mana value?" is not -- you
  were told that, and asking it wastes their time.
- It rests on something specific. The brief gives you the card's real oracle
  text, the gate's verdict, the deck's category counts against its bracket
  targets, and the rationales already written for the other cards in the same
  category. Use them. A question grounded in "there are already six ramp
  spells and four of them cost less" is worth five that could be asked about
  any card in any deck.
- It is willing to be unwelcome. The most useful question is often the one
  that makes the card hard to keep. If two cards in the same category claim
  the same job in their rationales, ask which one is really doing it. If the
  card is the ninth thing at three mana, ask what it beats out.
- It engages what they already claimed. If the card has a rationale, do not
  ignore it -- interrogate it. Is the claim still true? Is it the real reason,
  or the reason that was easy to write?

Facts:

- Every claim you make about a card comes from the brief or from the get_cards
  tool. Your recollection of a card is not evidence. This project has already
  been wrong twice by reasoning from memory about cards it was confident about,
  which is why the corpus is on the table in front of you.
- Colour identity is the corpus's own field. Never infer it from a mana cost.
- The gate -- legality, colour identity, singleton, deck size, category counts
  -- is deterministic Python and it has already run. Do not re-derive its
  verdict or contradict it. If it flagged this card, that is a fact to ask
  about, not a finding to report.
- A card the corpus does not have is a card you do not know. Say so by asking
  about it rather than assuming what it does.
""".strip()


RATIONALE_INTERVIEW = Mode(
    name="rationale-interview",
    purpose="Asks about one card's slot so the user can write its rationale.",
    instructions=INSTRUCTIONS,
    # ADR 15's table for this mode: `get_cards`, the deck read, and `analyze`.
    # `validate_deck` joins them because the gate's verdict on *this* card is
    # half of what makes a legality question askable, and `suggest` and
    # `search_cards` are deliberately absent -- shopping for a replacement is
    # a different mode's job, and offering one here turns an interview into a
    # recommendation the user did not ask for.
    tool_names=("get_cards", "get_deck", "deck_stats", "validate_deck"),
    response_schema=RESPONSE_SCHEMA,
)


class CardNotInDeck(ValueError):
    """Asked about a card the deck does not run.

    Its own exception because the caller's response differs: an unknown deck is
    a 404 and this is a 422 -- the deck is fine, the question is not.
    """


# --------------------------------------------------------------- the facts

#: Sibling rationales are the most useful thing in the brief and the most
#: expensive, so they are capped. Ten is enough to see a category's shape.
MAX_SIBLINGS = 10


def _find(deck: dict[str, Any], name: str) -> dict[str, Any]:
    """The card's entry in the deck, matched the way a person would type it."""
    wanted = name.strip().casefold()
    pool = list(deck["cards"]) + list(deck.get("swap_board") or [])
    if deck.get("commander_card"):
        pool.append(deck["commander_card"])
    for entry in pool:
        if entry["name"].casefold() == wanted:
            # `deck` is an untyped JSON payload, so the entry is `Any`;
            # say what the signature already promises.
            return dict(entry)
    raise CardNotInDeck(
        f"{name!r} is not in {deck['slug']}. The interview argues about a card "
        f"already in a deck; adding one is a different operation.")


def brief(slug: str, card: str, *,
          source: DeckSource | None = None) -> dict[str, Any]:
    """Everything deterministic Python knows about this card in this deck.

    Assembled rather than asked for. The mode could reach most of this through
    its tools, and would then be one forgotten tool call away from asking a
    question about a card it had not actually read -- which is rule 1 failing
    in the one place this feature spends all its time. Handing the facts over
    costs one round trip's worth of tokens and removes the failure mode.
    """
    deck = service.get_deck(slug, source=source)
    entry = _find(deck, card)
    name = entry["name"]

    looked_up = service.cards_named(names=[name])
    record = looked_up["cards"][0] if looked_up["cards"] else None

    report = service.validate_deck(slug, source=source)
    about_card = [
        {"severity": severity, "code": i["code"], "message": i["message"]}
        for severity, issues in (("error", report["errors"]),
                                 ("warning", report["warnings"]))
        for i in issues
        if (i["card"] or "").casefold() == name.casefold()
    ]

    stats = service.stats_for(slug, source=source)
    category = entry["category"]
    row = next((r for r in stats["categories"] if r["category"] == category),
               None)
    siblings = [
        {"name": c["name"], "why": c["why"]}
        for c in deck["cards"]
        if c["category"] == category and c["name"] != name
    ][:MAX_SIBLINGS]

    mv = record["cmc"] if record else entry.get("cmc")
    bucket = next((b for b in stats["curve"]["buckets"] if b["mv"] == mv), None)

    return {
        "deck": {
            "slug": deck["slug"],
            "name": deck["name"],
            "commander": deck["commander"],
            # The deck's axis, in the commander's own words. Half the useful
            # questions are some form of "what does this do for *that*".
            "commander_text": (deck["commander_card"] or {}).get("oracle_text"),
            "bracket": deck["bracket"],
            "stage": deck["stage"],
            "status": deck["status"],
            "color_identity": deck["color_identity"],
            "strategy": deck["strategy"],
            "total_cards": deck["total_cards"],
            "land_count": deck["land_count"],
            "cards_still_owing_a_rationale": deck["needs_rationale"],
        },
        "card": {
            "name": name,
            "category": category,
            "quantity": entry["qty"],
            # Named for what it is. A blank here is the normal case after an
            # import, not an error, and the questions differ: with a rationale
            # the job is to interrogate it, without one it is to find it.
            "rationale_so_far": entry["why"],
            "in_corpus": record is not None,
            "corpus": record,
        },
        "gate": {
            "about_this_card": about_card,
            "deck_errors": len(report["errors"]),
            "deck_warnings": len(report["warnings"]),
        },
        "category": {
            "name": category,
            "count": row["count"] if row else None,
            "target_low": row["target_low"] if row else None,
            "target_high": row["target_high"] if row else None,
            "verdict": row["status"] if row else None,
            "other_cards_in_it": siblings,
        },
        "curve": {
            "average_mana_value": stats["curve"]["average_mv"],
            "cards_at_this_mana_value": bucket["count"] if bucket else None,
        },
    }


# ------------------------------------------------------- reading the answer

def only_questions(items: list[Any]) -> tuple[list[dict[str, Any]], int]:
    """Keep the questions; count what was not one.

    The whole rule in one predicate: a question ends in a question mark. It is
    crude, it will occasionally drop something serviceable, and that is the
    right trade -- the failure this guards against is a declarative sentence
    appearing in the column beside an empty rationale box, which reads as a
    draft whatever the surrounding UI calls it.

    The count comes back rather than being swallowed, because "it dropped two"
    is how a prompt that has started editorialising becomes visible.
    """
    kept = []
    dropped = 0
    for item in items:
        text = str((item or {}).get("question", "")).strip() if isinstance(item, dict) else ""
        if text.endswith("?") and len(text) > 1:
            kept.append({"question": text,
                         "angle": item.get("angle", "role"),
                         "fact": str(item.get("fact", "")).strip()})
        else:
            dropped += 1
    return kept, dropped


#: More than this and it stops being an interview and starts being a wall.
MAX_QUESTIONS = 6


def _report(turn: Turn | None, *, slug: str, card: str,
            effective: stance_mod.Stance,
            questions: list[dict[str, Any]], dropped: int,
            asked: bool = True, reason: str = "") -> dict[str, Any]:
    """One response shape for every outcome, including not asking at all.

    `answered_by` is ADR 14's third boundary made a field: the gate's output
    and Claude's are never allowed to share a surface without a label, and a
    caller that has to infer which one it is holding will eventually infer
    wrong.
    """
    return {
        "answered_by": "claude",
        "mode": RATIONALE_INTERVIEW.name,
        "model": turn.model if turn else "",
        "slug": slug,
        "card": card,
        "asked": asked,
        "reason": reason,
        "stance": stance_mod.describe(effective),
        "questions": questions,
        "questions_dropped": dropped,
        "tool_calls": turn.tool_calls if turn else [],
        "usage": {"input_tokens": turn.input_tokens if turn else 0,
                  "output_tokens": turn.output_tokens if turn else 0,
                  # The evidence the prompt cache works. Zero across repeated
                  # interviews means the prefix is drifting -- surfaced here
                  # because a number nobody can see is a number nobody checks.
                  "cache_read_tokens": turn.cache_read_tokens if turn else 0},
        # Said in the payload rather than only in the UI, so a second client
        # cannot render this as anything other than what it is.
        "never": "These are questions. The rationale is yours to write.",
    }


def ask(slug: str, card: str, *, requested: Any = None, focus: str = "",
        source: DeckSource | None = None) -> dict[str, Any]:
    """Interview the user about one card's slot.

    `requested` is the stance, in any form `stance.resolve` accepts. It is
    resolved against the deck's own default and then clamped to the
    deployment's ceiling, so a client asking for more than a hosted instance
    allows gets the instance's answer rather than an error.

    **At `initiative: off` this makes no call and says so.** Not an empty list
    that looks like it had nothing to say -- `asked: false` and a reason.
    """
    facts = brief(slug, card, source=source)
    deck_obj = type("D", (), {"status": facts["deck"]["status"]})()
    effective = stance_mod.resolve(requested, deck=deck_obj)

    if not effective.allows_calls:
        return _report(None, slug=slug, card=facts["card"]["name"],
                       effective=effective, questions=[], dropped=0,
                       asked=False,
                       reason="The stance is off, so no call was made. "
                              "Everything else about this deck still works.")

    ask_for = [
        ("Here is everything the tool already knows about this card in this "
         "deck. All of it is deterministic: the card text is the corpus's, "
         "the verdict is the gate's, the counts are computed."),
        "",
        json.dumps(facts, indent=2, default=str),
        "",
        ("Ask three to five questions that would help me write this card's "
         "rationale, or decide it does not deserve one."),
    ]
    if focus.strip():
        # The user's own steer, quoted rather than interpolated into the
        # instruction, so it reads as something they said rather than
        # something the tool decided.
        ask_for += ["", f"What I am stuck on, in my words: {focus.strip()}"]

    turn = converse(
        RATIONALE_INTERVIEW,
        messages=[{"role": "user", "content": "\n".join(ask_for)}],
        stance=effective,
        source=source,
    )

    if turn.refused:
        return _report(turn, slug=slug, card=facts["card"]["name"],
                       effective=effective, questions=[], dropped=0,
                       asked=True,
                       reason="The model declined to answer this one.")

    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        # The schema makes this close to impossible, and "close to" is why the
        # branch exists: a truncated answer is not valid JSON either.
        return _report(turn, slug=slug, card=facts["card"]["name"],
                       effective=effective, questions=[], dropped=0,
                       asked=True,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}). Nothing was written.")

    questions, dropped = only_questions(payload.get("questions") or [])
    return _report(turn, slug=slug, card=facts["card"]["name"],
                   effective=effective,
                   questions=questions[:MAX_QUESTIONS], dropped=dropped)
