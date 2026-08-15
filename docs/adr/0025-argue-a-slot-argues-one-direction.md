# 25. Argue a slot argues one direction, and that asymmetry is the guard

**Status:** Accepted · **Decided:** 2026-08-14 · **Recorded:** 2026-08-14 ·
**Implemented:** 2026-08-14

Refines [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md), which
named four modes and built one. This is the second — *argue a slot* — and it
needs a decision ADR 15 did not have to make, because it is the first mode
whose output is declarative prose about a card's merit.

## Context

The rationale interview holds rule 4 with a predicate anybody can read:
`only_questions()` drops any returned item that does not end in a question
mark, and counts what it dropped. That is crude on purpose. A declarative
sentence appearing in the column beside an empty rationale box reads as a
draft whatever the surrounding UI calls it, so the format itself is the guard.

*Argue a slot* cannot use that guard, because the guard would delete the
feature. Its entire output is declarative sentences about whether a card
deserves its place. Pointed at Vorinclex it says "Regal Behemoth already
doubles your mana for two less" — a sentence, not a question, and the reason
to build the mode at all.

So the question this ADR exists to answer is: **what stops the case for a card
from being a `why`?**

Because the symmetric version is one small step away and sounds better. A mode
that argued both sides would be more useful on its face, more balanced, more
like a colleague. It would also produce, on request, a paragraph explaining
why a card earns its slot — grounded in the pool, engaged with the deck's
category counts, written in the user's own deck's terms. That is not adjacent
to a rationale. That is a rationale, and it is one paste away from the field
[ADR 8](0008-the-gate-blocks.md) and rule 4 exist to protect.

This is the same slope ADR 15 traced for the interview — "ask a question" to
"tidy that up" to "summarise what we discussed into a rationale" — arriving by
a different road. There the slope was about *authorship*: the user's words,
lightly edited, then not the user's words. Here it is about *direction*: the
case against, then the case either way, then the case for. Each step is
defensible alone and the last one is a rationale generator.

## Options considered

**Argue both sides, and forbid the output from reaching `why` by convention.**
Rejected. This is ADR 14's already-rejected "let Claude propose and require the
user to edit" wearing a different hat. A convention is not a mechanism, the
deck file records the text and not who typed it, and the failure is
undetectable afterwards.

**Argue both sides, and guard the UI so the case-for never renders beside the
rationale box.** Rejected, and it is worth saying why since it is the
reasonable-sounding one. It puts the whole boundary in one client. The payload
would still contain a finished rationale, `/api/decks/{owner}/{slug}/argue` is
a documented endpoint in a public repository, and the CLI renders the same
response with no box to sit beside. A boundary that only exists in one renderer
is a boundary that a second client removes without noticing.

**Return a balanced case but strip the case-for server-side before responding.**
Rejected as the worst of both: it pays for the tokens, it depends on a filter
correctly identifying which half is which — a semantic judgement, not a
predicate — and a filter that gets it wrong fails open.

**Argue one direction, and give the schema no field for the other.** Chosen.

**Do not build the mode; a pointed enough interview question covers it.**
Considered seriously, because the overlap is real — "what does this beat out at
three mana?" is most of a charge already. Rejected because the output contract
is the thing that differs: a question you have to answer is not a case you can
weigh, and weighing cases is what the curated six came out of. The interview
finds out what you think; this tells you what is wrong. Someone who already
knows what they think is not served by being asked.

## Decision

**The mode makes the case against a card's slot and has no way to make the case
for it.** It is one-directional by construction, not by instruction.

Four things enforce that, and consistent with ADR 15 none of them is the system
prompt:

1. **The response schema has no field for a reason to keep the card.** No
   `defence`, `verdict`, `summary`, `recommendation` or `rationale`, and
   `additionalProperties: false` at every level so one cannot be added at run
   time. A model that wanted to hand over the counter-case has nowhere to put
   it. `tests/test_claude_argue.py` checks this against a list of the names
   such a field would plausibly carry, because a guard that knew one spelling
   of the thing it forbids is not much of one.
2. **Every charge must cite a fact, or it is dropped and counted.** This mode's
   `only_charges()` is the interview's `only_questions()` with the predicate
   moved to where the signal is: every item here is declarative by design, so
   what separates an argument from an opinion is whether it rests on anything.
   The count comes back rather than being swallowed — a mode that has started
   asserting instead of citing becomes a number rather than merely persuasive.
3. **Alternatives are bare names, and deterministic Python judges them.** The
   model may name cards that could do the job; it may not say why they are
   better, and the schema's `alternatives` array holds strings with nowhere to
   attach a sentence. Every name is then resolved through the pool and dropped
   if it does not exist, is not Commander-legal, or falls outside the deck's
   colour identity — counted separately in each case, because "you invented
   that card" and "that card is off-colour" are different failures and only one
   of them is about the deck.
4. **`strength` is how a weak case gets reported.** The honest answer for a
   correct card is three charges marked `minor`, not silence and not padding.
   Removing the counter-case must not create pressure to invent a case, so the
   mode is given a way to say "this is what I found and it is thin."

### Why point 3 is not merely tidiness

The colour-identity filter is the reason this function exists rather than a
convenience inside it. `CLAUDE.md`'s first recorded error is *Ajani, Nacatl
Pariah* proposed for a Selesnya deck: white on its front face, red on its back,
identity {R}{W}, and illegal. A model reasoning from the mana cost gets that
wrong, which is exactly what happened. Here the model is not the thing
deciding — it names candidates, and rule 2's field decides. `tiny_pool` carries
Ajani for this, so the cautionary tale in the docs is now an assertion that
runs on every pull request.

The same filter is why a *banned* alternative is dropped. A mode arguing that a
card should be cut and offering Primeval Titan as the replacement would be the
tool undoing its own gate.

## Consequences

- **The mode is not balanced and must not be presented as balanced.** Its
  output is labelled `answered_by: claude` and carries a `never` line saying
  what it is, and the UI reads as *the case against* rather than *an
  assessment*. A user reading a one-sided argument as a verdict is the
  misreading this design makes possible, and naming it is the mitigation.
- **A card that survives the case against still owes a rationale.** Nothing
  here writes one and nothing here pre-fills one; the CLI prints the
  `decks set --why` invocation and stops. That is the same handoff the
  interview makes.
- **The overlap with the interview is real and accepted.** Two per-card modes
  is one more than strictly needed, and they share `brief()` verbatim rather
  than growing two assemblers that drift. If usage shows one of them never
  gets reached, that is evidence for merging them, and the merge would have to
  keep both guards rather than pick one.
- **`suggest_replacements` and `search_cards` are this mode's and not the
  interview's.** ADR 15's table already drew that line; this is where it starts
  mattering. Shopping for a replacement is the difference between the two
  surfaces, and an interview that offered one would have stopped interviewing.
- **It stays a synchronous POST, and that is a measured claim rather than an
  assumption.** It shares the interview's brief — ~4,900 input tokens and no
  tool calls, because the facts arrive assembled — and adds a tool set it
  reaches for only when it goes shopping. ADR 20's lesson is that *a duration
  measured for one surface is a question to ask of every sibling surface*, so
  the number to watch is what happens when the model does search: if that turn
  lands in the minutes, this endpoint moves to `api/jobs.py` like its three
  siblings, and the docstring saying "it is a few seconds" without a number is
  the one that broke the dossier.
- **This does not decide anything about deck conversation.** That mode is ADR
  15's only one with a write column, and reaching it needs a preset above
  `COLLABORATOR`, a write tool in a registry that has none, a relaxation of
  both `Mode.__post_init__` and `tests/test_claude_boundary.py`, and the
  activity log ADR 15 lists as required. Each of those is a decision, and none
  of them is made here.
