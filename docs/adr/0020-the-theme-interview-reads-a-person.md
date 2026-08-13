# 20. The theme interview reads a person, and Python decides when it may propose

**Status:** Accepted · **Decided:** 2026-08-12 · **Recorded:** 2026-08-12

Extends [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md), whose
table named four modes and which
[ADR 19](0019-the-dossier-cites-three-sources.md) already stretched to five.
This is the sixth and seventh — one feature, two modes — and it is the first
**conversational** surface in the project. Everything before it asks once and
answers once. That single difference is what needs a record of its own: a
multi-turn mode has state to keep and no natural place to stop, and both of
those are decisions rather than details.

## Context

The UI/UX pass moved this branch to the front for a reason worth restating,
because it is a finding and not a preference. Asked to test the deck page, the
maintainer went looking for an interactive Claude in the builder and found
none. He was right. "Start a deck" has no Claude in it at all, and the one
interactive surface that does exist — the rationale interview — is reachable
only by opening a deck, clicking *Edit why* on a card, and then *Ask for
questions*. Nothing announces it. Four modes built or planned, and between them
a read surface and a hidden one.

But the gap in the create flow is narrower and more specific than "there is no
assistant here". `NewDeck.tsx` has two doors and **both of them open onto the
same question**: which of the 32 colour combinations do you want? The guided
door teaches Ravnica on the way and the direct door skips the lesson, and
either way the first thing the tool needs from you is an answer in Magic's own
vocabulary. Someone who has never played cannot give one. The carousel is a
good teacher and it is teaching the wrong thing first — it explains what
Selesnya *is* before it has any reason to believe you care.

The way in turns out to be something the game already did. **The colour pie is
a personality taxonomy before it is a set of mechanics** — five philosophies
about how to fix a problem, and the mechanics fall out of the philosophy. So
the question that gets a newcomer to a colour combination is not a question
about Magic. It is a question about them: what you read, the period you would
live in, your sign, how you behave at game night. The translation from that to
white-green is the interesting part, and it is exactly the part a language
model is for.

That framing collides with three things this project has already decided.

**Rule 1 binds.** The moment the conversation names a card, card facts come
from the corpus. A commander proposed from recall is the Ajani failure with
better manners.

**No mode may write.** `create_deck` is on the write surface
`tests/test_claude_boundary.py` forbids naming, so the output of this feature
is a *proposal* and the existing create flow is what makes a deck. That
constraint arrived as a limitation and turns out to be the right product: the
deck is made by the person whose deck it is.

**Multi-turn is new.** `converse` runs a tool loop inside one request and
returns. A conversation whose history has to survive between HTTP requests has
never existed here, and neither has a mode that must decide it has heard
enough. ROADMAP named both as open before this branch started.

## Options considered

**A form with fixed questions, then a proposal.** Rejected, and rejected first
because it is the cheap version that looks the same in a screenshot. A fixed
list of questions could have been a fixed list of questions without a language
model anywhere near it. The whole value is that the third question depends on
the second answer — that saying "Dune" leads somewhere different from saying
"Studio Ghibli", rather than both landing in the same funnel.

**Teach the vocabulary first, then ask Magic questions.** Rejected. That is
branch 4's job and it is a good job, but it puts the lesson before the
interest. A newcomer will answer "what's your favourite film" before they will
sit through an explanation of the colour pie, and once they have answered, the
colour pie is a payoff rather than a prerequisite.

**One mode that both converses and proposes.** Rejected on response shape. The
conversation is prose and the proposal is a schema, and a single mode that does
both either loses the schema or forces every chatty turn through it. ADR 15
says a mode is a prompt, a tool set and a capability declaration; two of those
differ here, so it is two modes.

**Server-held conversation state, keyed and stored in `app.db`.** Rejected. It
costs a table, a migration, an expiry sweep, and an ADR 5 ownership rule
saying another user's conversation is a 404 — all for state that is worthless
the moment the deck is created. The transcript is also append-only, which is
precisely the shape prompt caching wants, and holding it server-side buys
nothing back.

**The model declares it has heard enough.** Rejected, and this is the one the
rest of the decision exists to prevent. A `ready: true` field is a promise, and
this project's whole pattern is to replace promises with checks —
`only_questions()` for a mode that started editorialising, `keep_sources()` for
a URL nobody visited. "I know what you want now" needs the same treatment.

**A turn count.** Rejected as the substitute. Someone who says everything in
one sentence would wait for nothing, and someone who rambles would get proposed
at before they had said anything usable. A ceiling on turns is still needed —
it just cannot be the readiness test.

**Read the person, ground every reading in their own words, count the grounded
ones in Python.** Chosen.

## Decision

### Two modes

| Mode | Tools | May ever write |
| --- | --- | --- |
| `theme-conversation` | `search_web` after `taste` is grounded; **no corpus, no deck** | nothing |
| `theme-proposal` | `get_cards`, `search_cards`, `search_web`; **no deck** | nothing |

**Neither mode takes a `DeckSource`, and that is the enforcement, not the
prompt.** This is a surface for building, not for critiquing. It cannot rate a
deck, name a cut, or comment on a list, because it cannot see one. The pattern
is already latent in the two modes that exist — the dossier has no
`search_cards` so it cannot go shopping, the rationale interview has no
`search_cards` so it cannot turn an interview into a recommendation — and this
ADR names it: **a mode is narrowed by what it can reach, not by what it is
asked to avoid.**

### The four kinds, and the floor of three

Every conversational turn returns a question *and* the mode's current reading
of what it has heard, as slots. A slot is a kind, a value, and **a quote**.

| Kind | The question behind it |
| --- | --- |
| `taste` | A film, book, artwork, period, band — anything loved that is not Magic |
| `temperament` | Your sign, planner or improviser, how you are when a plan falls apart |
| `posture` | At game night: going for the throat, quietly building, or making deals |
| `anchor` | *Optional.* A Magic card, character or deck already loved |

There is deliberately no slot for *what the deck should do*. It was the first
one drafted and it is the worst offender in the set — a question in Magic's
vocabulary wearing a friendly hat, unanswerable by exactly the person this
feature is for. `anchor` is the mirror image: the strongest single signal for
picking a commander and the one a genuine newcomer will not have, which is why
the floor is three rather than four.

**A slot counts only if its quote is really in the transcript.** Deterministic
Python normalises the claimed quote and checks it against the user's own turns;
a slot whose quote is not there is dropped and counted. That is the third
instrument in a family — `only_questions()` checks a shape, `keep_sources()`
intersects with evidence, and this intersects a claimed preference with the
text of the person who supposedly holds it. The failure it guards against is
specific and likely: a model that has decided you are a blue player and starts
reporting that back as something you said.

**Three grounded slots and the proposal is available.** Not automatic — the
user asks for it. Below three the control is honestly absent rather than
present and refusing. Above it the conversation may continue; the floor is a
floor.

**Constraints are not slots.** Budget, colours ruled out, cards already owned —
these are filters on a query rather than readings of a person, and `price_max`
is literally a `search_cards` argument. They are collected whenever they come
up and passed to the proposal as parameters, and they never gate the button. A
beginner should not have to declare a budget before the tool will talk to them.

### The transcript is the client's, and it is not the API's

Conversation state is **client-held and resent**. The server is stateless: no
table, no migration, no sweep, no ownership rule.

The wire format is **the mode's own type and deliberately not Anthropic's**.
The endpoint takes a list of `{role: user|assistant, text}` with a cap on turns
and on length, and reassembles the request server-side. This is the part that
matters: an endpoint accepting raw `messages` blocks is a free proxy for
somebody else's spend, and on a hosted instance that is the whole game. Roles
alternate or the request is refused, and nothing structural — a `tool_use`
block, a system turn — survives the crossing.

Tampering with the *content* buys nothing, and it is worth saying why rather
than asserting it. Every card in the proposal is re-resolved against the
corpus, so a doctored transcript cannot conjure one. Every source is checked
against what the search returned. And readiness is recomputed from the
transcript rather than carried in it, so the one flag worth forging does not
exist. What remains is a user lying to themselves about their own taste, which
is not a boundary worth defending.

### Four sources, each with a jurisdiction

ADR 19 established three. This mode has a fourth, and it is the cheapest one:

| Claim | Source |
| --- | --- |
| Cost, type, oracle text, colour identity, legality, **price**, whether a legend exists | **The corpus**, always |
| What a combination *means* — its philosophy, plane, era, story | **`colors.py`**, checked in and carrying `verified_by` |
| What an archetype is called, whether a commander is kind to a beginner, the meta, the deeper history | **Hosted web search**, with the page shown |
| The reading of the person, the bridge, the voice | **Claude**, no factual weight |

The second row is new and it is the reason the fun facts do not all cost a
search. `colors.py` already owns the tier blurbs, the era settings and the
stories the carousel teaches; it is human-written, checked into the repository,
and free. It is the backbone. The web is what makes one conversation's facts
different from the last one's — the specific hook that connects a particular
answer to a particular corner of Magic's history.

Price sits with the corpus and not the web, which is the kind of line that gets
blurred if nobody writes it down. "What can I afford" is a query.

**The conversational half gets one search, unlocked after `taste` is
grounded.** Before that there is nothing to search *for*. The alternative —
keeping the talking half toolless and saving all web-sourced history for the
proposal — is cheaper and was rejected on product grounds: the moment the tool
says something surprising about the thing you just named is what makes a
newcomer keep going, and it needs to happen while they are still talking rather
than at the end.

### An interpretation is not a claim

"You're a Scorpio, so black-green" is not a fact and this design does not
pretend otherwise. The distinction is load-bearing and it is drawn in the
schema:

- The colour pie's philosophy, the guild's era, the archetype's name — **claims**,
  each resting on `colors.py` or on a cited page.
- The leap from a sign, a film or a favourite century to a colour — an
  **interpretation**, in its own field, labelled as one in the payload and on
  the page.

ADR 19 rejected "let Claude write from recall and label it as recall" because a
labelled paragraph of history is still five checkable claims a reader will
believe. That argument does not apply here, and the difference is not
convenience. A claim about 1993 is either true or false; a claim that Dune
sounds like Jund is neither. It is a reading, it is offered as one, and the
reader is the authority on whether it lands.

This mode is therefore the first one allowed to be playful, and it can be
because **nothing it outputs is load-bearing**. A colour suggestion is a
suggestion. No proposal reaches the gate, the solver, the simulator or a `why`.

### It survives a failed search, unlike the dossier

ADR 19 refuses a dossier outright when no cited source survives checking, on
the grounds that an unsourced dossier is the blended paragraph the design
rejected, arrived at by accident. **The proposal does not inherit that**, and
the divergence is deliberate.

A dossier is *entirely* web-sourced claims; strip them and there is nothing
left but voice. A proposal's load-bearing content is a colour combination and a
list of real commanders, both of which are corpus facts and both of which are
still true when every search fails. So the rule is narrower: an unsourced
sentence is dropped and counted; the proposal stands. A newcomer who gets two
colour combinations and six real commanders with no history attached has been
helped. One who gets an error because a search timed out has not.

### The shape of the proposal, and where it lands

**Two colour combinations** — a clear first and a runner-up that teaches by
contrast — and **three commanders each**, matching the strip the carousel
already shows. Every commander is resolved through the corpus with
`commanders_only` and an exact identity filter, or dropped and counted.

It lands as a third door in `NewDeck.tsx` beside `guided` and `direct`, and its
output is **the same state the carousel produces**: a chosen combination and a
commander. So the proposal drops the user on the existing name-and-slug step,
and the button that makes the deck is the one that was already there. "It
proposes; you create" stops being a rule being honoured and becomes how the
screen is wired.

## Consequences

- **The cost shape is a handful of cheap turns and one rich call.** The
  conversational half carries no corpus tools and its prefix is append-only, so
  it is close to the ideal prompt-cache case; the one search it may run is
  bounded at one. The proposal is dossier-sized and fires once, on a click.
- **This is the most personal thing the app has ever handled, and it never
  touches the server's disk.** Somebody types their sign, their favourite
  novel and how they behave with their friends into a box. `CLAUDE.md` is
  strict about personal data because of email addresses; this is a different
  and softer category, and the client-held transcript means there is no row to
  leak, no retention question and nothing to purge. That was not why the
  decision was made, and it is the best thing about it.
- **A conversation dies with the browser tab.** Mitigated with `localStorage`,
  which keeps the server stateless, and accepted: ten minutes of thinking is
  worth preserving, and a schema is not the way to preserve it.
- **The grounding check is crude, on purpose.** It is a substring test, so a
  model that quotes one stray word from a user turn can carry an invented
  reading past it. It is the same bluntness `only_questions()` has and the same
  trade: it catches wholesale invention, which is the failure that matters, and
  the count of dropped slots is visible so a prompt that has started making
  things up shows up as a number rather than as helpfulness.
- **The conversational half can still name a card in prose.** It has no lookup,
  so it has no grounded card facts, and its schema has no field for one — but
  nothing stops a sentence. The residual is stated rather than papered over;
  the defence is that a mode with no corpus access has no confident card facts
  to state, not that the output is filtered.
- **ADR 15's table is now a list of six, and the structure is what held.** Four
  was a guess, 19 was the guess being wrong once, and this is it being wrong
  twice — in the direction 15 predicted. The frame absorbed a server-side tool,
  a second class of evidence, a fourth source and now a multi-turn transcript
  without the write column ever moving.
- **`search_cards`'s tool schema was missing two arguments this needs.**
  `commanders_only` and `identity_exact` exist on the service and not in
  `claude/tools.py`, and `tools.run` refuses unknown arguments — so no mode
  could ask for legends only. Found by writing this down before writing the
  code, which is the argument for doing it in that order.
- **The refactor pass stays out, and now for a design reason rather than a
  scheduling one.** It is a critique surface over an existing deck; this branch
  argues that the deckbuilding surface must not reach a deck. Shipping both
  together would blur the boundary on the branch that draws it. ROADMAP goal 10
  stands.
