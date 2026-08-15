# 26. Research answers about Magic, never about your deck

**Status:** Accepted · **Decided:** 2026-08-14 · **Recorded:** 2026-08-14 ·
**Implemented:** 2026-08-14

Builds the fourth row of [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md)'s
table — *Research*, `server-side web search`, writes nothing — and answers the
question that row left open, which
[ADR 19](0019-the-dossier-cites-three-sources.md) and
[ADR 25](0025-argue-a-slot-argues-one-direction.md) each answered for
themselves: **what is the narrow, checkable contract this mode has to hold?**

## Context

Five modes are built and every one of them is safe for a reason that can be
read off the code rather than off a prompt.

* The **rationale interview** returns only questions, and
  `only_questions()` deletes anything that does not end in `?`.
* The **slot argument** argues one direction, and the schema has no field for
  the case in favour (ADR 25).
* The **dossier** resolves every rival through `get_cards` or drops it, and
  **refuses outright when no cited page survives checking** (ADR 19).
* The **theme interview** intersects every claimed preference with what the
  user actually typed, and cannot reach a deck at all
  ([ADR 20](0020-the-theme-interview-reads-a-person.md)).

Research is the first mode where that property was not obvious in advance, and
ROADMAP recorded it as the reason to think before building: *"the dossier is
safe because every rival resolves through `get_cards` or is dropped and it
refuses when no source survives, and 'the meta, rulings, spoiled cards' is
three questions with no such invariant."*

The three questions really are different. **The meta** is opinion aggregated
from pages, and the honest answer is often that people disagree. **A ruling in
practice** is a fact somebody else holds — a judge, a Gatherer entry, a rules
document — and the pool does not carry it. **A card spoiled ahead of the next
bulk refresh** is the case that breaks the dossier's own instrument: it is a
real card the pool has never heard of, and dropping it, as the dossier drops an
unresolved rival, would delete the third of the feature people most want.

There is also a second hazard, and it is not about facts. Research is the first
mode whose *input* is free text from the user. "Should I cut Bag End Banquet
from my Gyome deck?" is a question somebody will type on day one, and answering
it well means writing a paragraph about a card's merit in a specific deck —
which is [ADR 8](0008-the-gate-blocks.md) and rule 4's field, arrived at
through the one door nobody thought to lock, because the interview's guard is
about format and the slot argument's is about direction and neither is about
*subject*.

## Options considered

**Give research the deck tools and let the prompt keep it honest.** Rejected,
and it is the option that would have been taken by default: `tools.READ_ONLY`
is right there, `get_deck` is one string in a tuple, and a research mode that
could see your list would obviously answer better. It would also be deck
conversation — ADR 15's third mode, deliberately unbuilt, and unbuilt for five
separate reasons recorded elsewhere. Building it accidentally, under another
name, with no activity log and no write-autonomy argument settled, is the worst
version of building it.

**Drop every card name the pool does not have, as the dossier drops a rival.**
Rejected. It is the same instrument pointed at a case it does not fit. A rival
commander that does not resolve is a card the model invented; a *spoiled* card
that does not resolve is a card the pool has not ingested yet, and the two are
indistinguishable from inside `get_cards`. Applying the dossier's rule here
would make the mode silently worst at the thing it is uniquely for.

**Cache the answer, keyed on the question.** Rejected. ADR 18 caches a
simulation because it is reproducible and ADR 19 caches a dossier because its
subject is a character who outlives any conversation. Research's subject is the
part of Magic that *moves* — which set just came out, what people think this
month, whether a card is still good. A cache here would serve last month's
answer to this month's question and stamp it with a date nobody reads.

**Refuse to answer deck questions.** Rejected as the *mechanism*, though the
mode does say so in prose. A classifier deciding whether a question is "about a
deck" is a judgement call handed to the model, which is exactly the shape of
guard this project keeps refusing. The structural version is below.

## Decision

**Research is deck-blind by construction, cites what it read, and labels what
the pool has not seen.** Four parts, and each is a mechanism rather than an
instruction.

### 1. The mode cannot reach a deck, and neither can the route

`RESEARCH` declares `tool_names=("get_cards",)` — no `get_deck`, no
`list_decks`, no `validate_deck`, no `deck_stats`, no `suggest_replacements`.
`research.ask()` takes **no `DeckSource` and no slug**, `converse` is called
with `source=None`, and the endpoint is `POST /api/claude/research` rather than
anything under `/api/decks/{owner}/{slug}`.

This is ADR 20's rule — *a mode is narrowed by what it can reach* — applied to
the mode that most wants to reach further. It does two jobs at once. Rule 4 is
out of reach because the mode cannot read a rationale, cannot see the 99, and
cannot be asked what to cut from a list it has never been shown; and **deck
conversation cannot be built by accident**, because the thing that would make
this mode into that one is a dependency somebody would have to add on purpose,
in a diff, with this ADR to supersede.

A user can still paste their decklist into the question. That is fine and it is
not a hole: those are the user's own words in their own request, the same as
any other prompt, and nothing in the answer can reach `deck.yaml` regardless.

### 2. Every finding rests on a page the search actually returned

`dossier.keep_sources()` is reused verbatim — not reimplemented — so a cited
URL survives only if it appears in what `Turn.searched` recorded, matched
through `dossier.canonical_url()`. Dropped citations are counted.

Then one step the dossier did not need: **a finding whose citations all failed
the check is itself dropped and counted.** The dossier narrows a section's
`source_ids` and keeps the prose, because a passage may legitimately rest
entirely on the pool facts in its brief. Research has no brief — see point 3 —
so a claim about the meta with no surviving source is resting on nothing, and
what it is resting on is recall.

**If no source survives, the answer is refused**, exactly as ADR 19 refuses a
dossier. An unsourced research answer is a model talking about Magic from
memory, which is rule 1's original failure with a search box drawn around it.

### 3. A card the pool has is a pool fact; a card it lacks is a labelled claim

Every card name the answer mentions comes back as a **bare name**, and Python
resolves the list through `service.cards_named()`. What resolves carries the
pool's own oracle text, cost, type line and colour identity, and renders beside
the prose. What does not resolve is **kept and marked `in_pool: false`**,
counted in `cards_unresolved`.

That asymmetry with the dossier is the whole point, and it is why this mode
needs its own ADR. The dossier drops because a rival that does not exist is an
error. Research labels because a card that does not exist *yet* is the
question. The reader gets a visible boundary rather than a silent filter: on
one side of it, card facts came from the pool as rule 1 requires; on the other,
they are a claim from a page the search returned, and the page is shown.

Note also what this mode does **not** get to do, which the interview and the
slot argument both do: assemble the pool facts before the call. `brief()` works
for those because the subject is known in advance — one card, in one deck. Here
the subject is whatever the user asked about, so **`get_cards` as a tool is not
belt-and-braces, it is the only door**, and the post-hoc resolution above is
the half that does not depend on the model choosing to walk through it.

### 4. It is a background job from the first commit, and it is not cached

Nothing is stored. Two runs of the same question legitimately differ, and the
subject moves; `generated_at` is in the payload as the honest substitute for a
freshness claim, the way ADR 19 stamps a dossier.

It is a job anyway. ADR 20's lesson — *a duration measured for one surface is a
question to ask of every sibling surface* — has now cost this project three
separate incidents, and the docstring that said "it is a few seconds" is the
one that left the dossier synchronous until it broke deployed at 236 seconds.
Research searches more than the dossier does, so a synchronous POST was never a
candidate. Everything refusable is refused in the request (an empty question,
no key), and only the Anthropic call is queued, in the `NET` lane.

It **is** deduplicated in flight, on a hash of the question text, which is the
one place research is easier than the theme proposal: two theme turns in flight
are two conversations, but two identical question strings from one account
inside a four-minute window are one question asked twice — `jobs.submit`'s
`key` already matches per owner.

### 5. A thin answer is reported as thin

`confidence` is `settled | contested | thin`, and it is ADR 25's `strength`
argument in another mode: removing the ability to hedge in prose creates
pressure to manufacture agreement. "The meta says" is a phrase that implies
more consensus than usually exists, and a mode with no field for *"people
disagree about this"* will write consensus that is not there.

## Consequences

**Research cannot answer the most-asked question**, which is "is this card good
in my deck". It answers "what do people say about this card" and shows who
said it, and the deck page's own surfaces — the gate, Tier 1, the slot
argument — answer the rest. That is a real cost and it is the cost of not
building deck conversation sideways.

**Card facts split into two visibly different kinds**, which every client has
to render as two kinds. A UI that merged them would produce exactly the blended
paragraph ADR 19 rejected, one source further out.

**A question that finds nothing is refused rather than answered**, so a
narrow, obscure or badly-worded question can cost a search and return no
answer. The alternative is a fluent unsourced paragraph, which is worse.

**Four modes now share `keep_sources` and `canonical_url`.** They stay in
`dossier.py` where they were written; a second copy is a second chance to
disagree about whether a trailing slash is a different page.

**Deck conversation is now harder to build**, deliberately. Anything that
wants a deck in a Claude surface has to say so in an ADR that supersedes this
one, and list what it does about the five things ADR 15 still owes.
