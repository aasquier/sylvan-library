# 15. Claude surfaces are modes with a user-set stance, and no stance may write a rationale

**Status:** Accepted · **Decided:** 2026-08-11 · **Recorded:** 2026-08-11 · **Implemented:** 2026-08-11, in part

> The body below is the decision as argued and is not edited. This note says
> only how far it has been built.
>
> **Built:** the capability set, which is the part that had to exist first.
> `mtglab.claude.tools.READ_ONLY` is the complete list of service functions any
> surface can reach, and `run()` refuses by name anything outside it. Rule 1 of
> the three under *The modes* — "no mode may write `why`, at any stance" — is
> the assertion this ADR asked for and it is tested, structurally: nothing
> under `src/mtglab/claude/` may so much as name a write function, checked over
> the package's syntax tree so it fails on the commit that adds one rather than
> on a call path that exists today.
>
> Every tool in the mode table above now exists, `get_cards` included — it was
> the one with no service function behind it, and the gap turned out to matter:
> a banned card was unreadable, so rule 1 leaked on exactly the cards this
> project fails the gate on. `service.cards_named()` closed it the same day.
>
> **The stance and its three axes landed 2026-08-11.** `mtglab.claude.stance`
> is the dial: initiative, scope and write autonomy as independent ordered
> axes, four named presets over them, `off` as a real position that makes no
> calls, and the default derived from the deck's `status` exactly as argued
> below. It is stdlib-only and needs neither the SDK nor a key — which is the
> property that makes "off" trustworthy rather than merely configured. Two
> things the ADR did not anticipate and the build added: a **deployment
> ceiling** (`MTGLAB_CLAUDE_STANCE_CEILING`) that clamps per axis, so a hosted
> instance can cap what any user selects while a local run does not have to;
> and an unreadable ceiling **failing closed**, because a typo in a deployment
> variable should cost a feature rather than open one.
>
> **Not built:** the modes themselves, the activity log, and the rationale
> interview.

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

**One fixed level of assistance for everybody.** Rejected once the question was
asked out loud. People want very different amounts of this — a tool that never
speaks unless spoken to, versus the thing that proposes an axis you had not
considered — and picking one setting for everyone gets half the users a product
they do not want. A mute button is not an answer either: it gets the second
group nothing.

**A single slider from "off" to "runs wild".** Rejected as the *shape*, though
not as the idea. It conflates axes that are genuinely independent: "never
interrupt me, but go wild when I ask" is a coherent setting a single slider
cannot express. Worse, a slider invites a top position that reads as "Claude
does everything, including the rationales", which would cash out rule 4 in one
control.

**Modes for what is permitted, a stance for how much of it the user wants.**
Chosen. The mode is the ceiling and the stance is the dial, and the dial cannot
raise the ceiling.

## Decision

A Claude surface is a **mode**: a named bundle of four things.

1. **A system prompt** — what this mode is for.
2. **A tool set** — the deterministic Python it may call, and nothing else.
3. **A capability set** — what, if anything, it may write.
4. **A stance** — how much initiative it takes, set by the user, off by default.

The first three are the mode. The fourth is the user's dial on it, and the two
are separated deliberately: a stance may widen what a mode *does*, and may never
widen what it is *allowed* to do.

### The modes

| Mode | Tools | May ever write |
| --- | --- | --- |
| Rationale interview | `get_cards`, deck read, `analyze` | nothing |
| Argue a slot | `get_cards`, `search_cards`, `suggest`, `analyze`, `validate` | nothing |
| Deck conversation | all corpus reads, `validate`, `stats`, sim results | the reversible edits, at the top stance only |
| Research | server-side web search | nothing |

Three rules make the table real:

1. **No mode may write `why`, at any stance.** Not directly, not by proposing
   text a surface pre-fills, not by "tidying". The rationale editor's box takes
   the user's keystrokes and nothing else. A mode may put a *question* beside
   that box; it may never put text inside it. This is testable — the assertion
   is that no code path passes a model response into
   `set_card_field(field="why")` — and it is tested, in the same place the UI
   test already pins that the box opens empty for a card with no rationale.
2. **Card facts arrive through tools, not through the model.** Rule 1 is
   enforced structurally: a mode that needs to know what Arahbo does calls
   `get_cards` and the tool result is the fact. This is why the modes live in
   `api/service.py` — that is where the corpus queries and the gate already
   are, and the tool definitions are thin wrappers over functions that exist.
3. **A mode's output is labelled as a mode's output.** ADR 14 boundary 3: the
   user can always tell the gate's answer (reproducible) from Claude's (an
   opinion), because they never share a surface without a label.

### The stance

People want different amounts of this. Some want a deckbuilding tool that
never speaks unless spoken to; some want the thing that dreams up an axis they
had not considered. One product cannot be both without a control, and a
control that is only a mute button gets the second group nothing.

The stance is **three axes with named presets over them**, rather than a single
slider — "never interrupt me, but go wild when I ask" is a coherent and probably
common setting that one slider cannot express.

| Axis | Range |
| --- | --- |
| Initiative | silent until invoked → volunteers → interjects while you work |
| Scope | only what the gate flagged → adjacent cards → rethink the deck's axis |
| Write autonomy | none → proposes a batch for one approval → applies reversible edits |

**Off is a real position.** No calls, not "quiet but still watching". That is a
trust property before it is a cost one, though it is also the reason a hosted
instance can default conservative while a local run on your own key does not
have to.

**The default comes from the deck.** `status: built | theoretical` already draws
this distinction: Goreclaw and Tivit are lists under consideration, where a wild
suggestion costs nothing, and Arahbo and Gyome are sleeved cardboard, where one
costs money and a trip to the box. Deriving the default from a field that
already exists means the sensible thing happens with no configuration.

### Why the top of the write axis is safe

Two properties, neither of which is a policy check bolted on top.

**Every write re-runs the gate.** `service._commit` is the only write path and
it returns the gate's verdict on the result, so an autonomous edit is a checked
edit. Claude is not the thing deciding legality, colour identity or deck size —
that is the whole of ADR 14 — so however wild the stance, the output is still
bounded by facts. It is bounded by *taste* nowhere, which is the point.

**The rationale rule does most of the limiting by itself.** Trace what an
autonomous write can actually be, given rule 1 above and the operations in
[ADR 12](0012-decks-are-edited-by-surgical-operations.md):

| Operation | Autonomous? | Why |
| --- | --- | --- |
| `remove_card` | yes | needs no rationale |
| `set_card_field` (category, qty) | yes | needs no rationale |
| `add_card` to a **draft** | yes | a draft's blank `why` is counted work, not invented text |
| `add_card` to a **curated** deck | **no** | the operation refuses a blank `why` and Claude cannot supply one |
| `replace_card` (a swap) | **no** | requires a `why` unconditionally |
| `set_note` | **no** | deck-level prose is the deck's thinking, the same kind of thing as a `why` |

So the single most attractive thing to automate — a twelve-card swap — is
blocked, and blocked by the edit operation rather than by a rule about models.
The way through is the interview: Claude proposes the swap, the user says why
they accept it, and the user's sentence is the rationale. The write stops being
autonomous at exactly the point a human judgement enters.

`set_note` is ruled out by judgement rather than by the operation, and it is the
one line here that a future ADR might reasonably move. A note is the mulligan
rule, the pitfalls, the lines — prose that only a conversation produces. Letting
a model write it while forbidding it the `why` field would be honouring the
letter of rule 4 and not the point of it.

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
- At the default stance a mode cannot fix what it finds. Claude will say "Bag
  End Banquet looks weak against the three-mana slot" and someone else has to
  act on it. That is deliberate and it is the same shape as
  [ADR 8](0008-the-gate-blocks.md) refusing auto-substitution: a tool that
  quietly changes a deck is one whose output is no longer your deck. The top
  stance relaxes this by consent, per deck, and the consent is revocable in one
  control.
- The capability table has to be enforced somewhere real, not just documented
  here. The test that pins "no model response reaches `why`" is the thing that
  must keep passing as stances are added.
- **Autonomous writes need an activity log.** "What did it change while I was
  not looking" is a fair question and the answer cannot be "read the git diff",
  because a user of a hosted instance is not at a terminal. Every write already
  goes through `service._commit`, so there is one place to record it, but this
  is real work that the top stance requires and the default stance does not.
- The stance is per-conversation state first, deliberately not persisted. It
  costs no schema and no migration, and it will show which presets people
  actually reach for before a default is written into anything. Deriving from
  `status` comes after there is evidence, not before.
- Four modes is a guess, and so are three axes. The set will be wrong in some
  direction; the structure — a mode is what it may do, a stance is how much of
  it the user wants — is what this ADR commits to.
