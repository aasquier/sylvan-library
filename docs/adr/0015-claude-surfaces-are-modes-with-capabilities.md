# 15. Claude surfaces are modes, and no mode may write a rationale

**Status:** Proposed · **Decided:** 2026-08-11 · **Recorded:** 2026-08-11

Refines [ADR 14](0014-python-decides-claude-advises.md), which drew the line
between what Python decides and what Claude advises on. This one says what a
Claude surface actually *is* in this codebase, and where its hands are tied.

Nothing implements it yet. It is recorded now because the surface it constrains
— a rationale interview — sits directly on top of the editor built alongside it
in [ADR 12](0012-decks-are-edited-by-surgical-operations.md), and the boundary
is much easier to hold if it is written before the code than after.

## Context

ADR 14 settled the split and named three boundaries. The second one is the
sharp one:

> **Claude may argue about a `why`; it may not write one.**

That is unambiguous for a chat window. It is *not* unambiguous for the thing
this project most wants, which is an **interview**: `decks import` brings in a
99-card list owing 99 rationales, and the obvious way to help is for the tool to
ask about each card and put the answers in the file.

Consider the sequence. Claude asks "what does Sol Ring do here that a Signet
does not?" The user answers, in their own words. Those words land in `why`. No
rule was broken: the keystrokes are the user's, and interrogating a card's slot
is precisely the conversation the curated six came out of.

Now consider the next feature request, which is one button away: *tidy that up*.
Or: *the user typed three words, expand them*. Or: *summarise what we just
discussed into a rationale*. Each is a small step from the last, each is
defensible on its own, and the last one is a machine-written `why` — the empty
justification that rule 4 and [ADR 8](0008-the-gate-blocks.md) exist to prevent.

There is a second problem, and it is about facts rather than authorship. Rule 1
binds Claude: card facts come from the corpus, not from recall. A prompt saying
"do not rely on your memory of card text" is not a mechanism. A model asked
about Arahbo with no corpus access will answer from its weights, and the failure
mode is exactly the one `CLAUDE.md` documents as having already happened twice —
eminence attributed to the wrong ability, an illegal colour identity proposed —
except now nobody is reading the output as carefully.

## Options considered

**One general chat surface, constrained by its system prompt.** Rejected on
both counts. A single prompt has to carry every rule for every use, which makes
it long, unauditable, and unenforceable — a prompt is a request, not a
guarantee. And "which system answered" (ADR 14 boundary 3) becomes unanswerable
when everything comes through one undifferentiated box.

**Let Claude propose a `why` and require the user to edit before saving.**
Rejected, and ADR 14 already rejected it once: it adds a click to the same
failure. A proposed rationale that a tired user accepts unchanged is a
tool-written rationale with an extra keystroke of laundering. Worse, it is
undetectable afterwards — the deck file records the text, not who typed it.

**Trust the prompt for corpus grounding.** Rejected per above.

**Modes, each declaring its tools and what it may write.** Chosen.

## Decision

A Claude surface is a **mode**: a named bundle of three things.

1. **A system prompt** — what this mode is for.
2. **A tool set** — the deterministic Python it may call, and nothing else.
3. **A capability set** — what, if anything, it may write.

The capability set is the load-bearing part, because it is code rather than
prose. Sketched for the four modes worth building first:

| Mode | Tools | May write |
| --- | --- | --- |
| Rationale interview | `get_cards`, deck read, `analyze` | **nothing** |
| Argue a slot | `get_cards`, `search_cards`, `suggest`, `analyze`, `validate` | **nothing** |
| Deck conversation | all corpus reads, `validate`, `stats`, sim results | **nothing** |
| Research | server-side web search | **nothing** |

Every cell in the last column is the same, and that is the point of the table
rather than an accident of its first four rows. Three rules make it real:

1. **No mode may write `why`.** Not directly, not by proposing text a surface
   pre-fills, not by "tidying". The rationale editor's box takes the user's
   keystrokes and nothing else. A mode may put a *question* beside that box; it
   may never put text inside it. This is testable — the assertion is that no
   code path passes a model response into `set_card_field(field="why")` — and
   it is tested, in the same place the UI test already pins that the box opens
   empty for a card with no rationale.
2. **Card facts arrive through tools, not through the model.** Rule 1 is
   enforced structurally: a mode that needs to know what Arahbo does calls
   `get_cards` and the tool result is the fact. This is why the modes live in
   `api/service.py` — that is where the corpus queries and the gate already
   are, and the tool definitions are thin wrappers over functions that exist.
3. **A mode's output is labelled as a mode's output.** ADR 14 boundary 3: the
   user can always tell the gate's answer (reproducible) from Claude's (an
   opinion), because they never share a surface without a label.

Write capabilities are not forbidden forever — a future mode might reasonably
propose a *swap* for the user to approve, since `swaps.md` and git make that
reviewable. But `why` is different in kind from every other field, and the
column stays empty for it specifically.

## Consequences

- The rationale interview is buildable without weakening rule 4, which is the
  whole reason to write this down. It supplies questions; the user supplies
  answers; `set_card_field` writes what the user typed.
- The modes are cheap. The tools they need — `get_cards`, `search_cards`,
  `validate`, `suggest`, `deck_stats` — are already written and already tested
  without a network, because ADR 14 put the deterministic half in Python first.
  What is new is schemas, a tool-use loop, and the SDK dependency.
- Grounding gets a *cost* rather than a promise: every card fact is a round
  trip. That is the right trade — a wrong answer about Arahbo's eminence is
  worth more than the tokens — but it makes research turns slower and dearer,
  which feeds the open question about who pays for a hosted surface.
- A mode with no write capability cannot fix what it finds. Claude will say
  "Bag End Banquet looks weak against the three-mana slot" and someone else has
  to act on it. That is deliberate and it is the same shape as
  [ADR 8](0008-the-gate-blocks.md) refusing auto-substitution: a tool that
  quietly changes a deck is one whose output is no longer your deck.
- The capability table has to be enforced somewhere real, not just documented
  here. If a later mode gains a write capability, the test that pins "no model
  response reaches `why`" is the thing that must keep passing.
- Four modes is a guess. The set will be wrong in some direction, and the
  structure — not the list — is what this ADR is committing to.
