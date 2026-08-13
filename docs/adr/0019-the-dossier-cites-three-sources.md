# 19. The commander dossier cites three sources, and the page shows the seams

**Status:** Accepted · **Decided:** 2026-08-12 · **Recorded:** 2026-08-12

Extends [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md), whose
table names four modes. This is a fifth, and it is the first one whose facts do
not all come from the corpus — which is the whole reason it gets a record of its
own rather than another row.

## Context

Branch 1 of the UI/UX pass gave the deck page a strip of counted facts about its
commander: how rare its subtypes are, what else carries the character's name,
when it was first printed. Every number there is a DuckDB query in
`service.commander_dossier`, and that was deliberate — "Gyome is one of eight
legendary Trolls" is exactly the sentence a language model writes fluently and
wrongly.

The response to it was that the *interesting* half is missing. Who is this
character? What archetype do they define, and where did it come from? Who are
their rivals? Where do they sit in Magic's history? None of that is a column in
the corpus, and none of it is a number.

That is squarely the half [ADR 14](0014-python-decides-claude-advises.md) gives
Claude — "the meta, whether a spoiled card earns a slot, what a ruling means in
practice". But it collides with the same ADR's first boundary:

> **Card facts come from the corpus** — not from the model's recall, and not
> from a web page.

The four modes ADR 15 names never had to resolve that collision, because every
one of them answers a question the corpus can already ground. A dossier cannot.
"Cats have been a tribe since Arabian Nights" is not in the corpus, is not a
number, and is the kind of claim that sounds equally confident whether or not it
is true. So the rule for this mode cannot be "facts come from the corpus" on its
own. It has to say **which source is allowed to support which kind of claim**,
and it has to be checkable afterwards.

There is a second constraint that shapes the answer. `CLAUDE.md` bans a web
crawler outright, and says in as many words that server-side web tooling "is not
a way around the scraping ban". Any design that gets its history by ingesting
somebody else's aggregated data is out before it starts.

## Options considered

**Don't build it; the corpus strip is the answer.** Rejected. The question is a
real one and ADR 14 already assigns it — declining it here would mean the
project has a Claude integration that is never allowed to say anything only
Claude can say.

**Let Claude write the history from recall, and label it as recall.** Rejected,
and it is the tempting option because ADR 14's third boundary (say which system
answered) appears to make it safe. It does not. A labelled paragraph of recalled
Magic history is still five checkable claims a reader will believe, and the two
errors `CLAUDE.md` records — Ajani's back-face colour identity, Arahbo's
eminence — were both produced by a confident model reasoning about cards it
knew well. Labelling makes a wrong claim attributable, not less wrong.

**Ingest EDHREC's themes or the MTG wiki's archetype pages.** Rejected on the
rule, not on the engineering. It is a crawl, the data is somebody's own
aggregate, and `ROADMAP.md` goal 8 already walked into this wall and wrote it
down.

**Server-side web search, with its sources shown.** Chosen for the half the
corpus cannot answer. This is Anthropic-hosted retrieval (`web_search_20260209`)
over the open web, at read time, for one commander, returning links the page
then displays. It maintains no crawler, mirrors nothing, and stores no third
party's aggregate — it is a reader with a citation, which is the distinction the
scraping ban is drawn around rather than an exception to it.

**One blended paragraph from all of it.** Rejected, and this is the option the
rest of the decision exists to prevent. A dossier that reads as a single voice
has erased the only thing that makes it trustworthy: which sentence is a query,
which is a citation, and which is a model's framing.

**Three sources with declared jurisdictions, kept apart all the way to the
page.** Chosen.

## Decision

### Each source has a jurisdiction, and it is narrow

| Claim | Source | Never |
| --- | --- | --- |
| Cost, type line, oracle text, colour identity, legality, power/toughness, printing history, how rare a subtype is | **The corpus**, always | not web search, not recall |
| The meta, the archetype and its history, rivals, where this sits in Magic's history | **Server-side web search**, with the source shown | not recall |
| Voice, framing, what is worth saying and in what order | **Claude** | carries no factual weight |

The third row is the one worth stating explicitly. Claude is the writer here,
not a witness. Every load-bearing sentence in a dossier traces to a query or to
a link; what Claude contributes is that it reads as prose rather than as a join.

### Four things enforce it, and none of them is the system prompt

This follows ADR 15's own lesson: the interview's boundary held because the
schema had nowhere to put a rationale, not because the prompt asked nicely.

1. **`may_write` stays empty**, and the package still has no write door.
   ADR 15's invariant is untouched — nothing under `src/mtglab/claude/` may
   name a deck-write function, and `tests/test_claude_boundary.py` still fails
   on the commit that adds one.
2. **The schema separates the sources.** Prose sections and a `sources` list
   are different fields, and a section declares which sources it rests on. A
   claim about the meta with no source attached is a claim with nothing behind
   it, and it is visible as such rather than blending in.
3. **A cited URL must be one the search tool actually returned.** Deterministic
   Python collects every URL out of the response's `web_search_tool_result`
   blocks and drops any citation that is not among them, counting what it
   dropped. A model that types a plausible URL it did not visit is the web
   equivalent of answering from recall, and this is the same instrument
   `interview.only_questions()` already points at the same failure.
4. **Every card the dossier names is looked up in the corpus**, and comes back
   with what the corpus says or is dropped and counted. Rule 1 does not get to
   depend on the model choosing to call `get_cards`.

### It is cached on the commander, not on the deck

The key is the commander's `oracle_id`, plus a fingerprint of the mode itself —
its prompt, its schema, and the model id. Not the deck slug: a dossier is about
a character, so every deck that commander leads shares one, including across
users on a hosted instance. Two Gyome decks are two lists and one Gyome.

This is deliberately a weaker contract than
[ADR 18](0018-a-cached-simulation-is-keyed-on-its-compiled-input.md)'s, and the
difference is worth naming rather than inheriting by accident. A cached Tier 1
number must never be stale, because it is reproducible and a stale one is simply
wrong. A dossier is an opinion assembled over a corpus of web writing that moves
on its own; there is no input hash that could tell you it has gone out of date.
So the honest contract is different: **generated once, stamped with the date it
was generated, regenerable on demand.** The date is on the page for the same
reason a cached simulation says `cached` — a reader who cannot see how old a
claim is cannot weigh it.

### When it runs

Generated automatically for a deck that arrives via create or import, and on a
button for the six that already exist. **At stance `off` the button does not
appear at all**, because off means no calls (ADR 15), and a control that exists
but refuses is a worse answer than a control that is honestly absent.

## Consequences

- **The page has to show the seams, or none of this matters.** Branch 1's
  counted strip stays exactly where it is; Claude's prose sits below it,
  labelled as Claude's; a claim that came from the web keeps its link next to
  it. This is ADR 14's third boundary made visual rather than editorial, and it
  is the acceptance criterion for the UI, not a nicety.
- **The tool loop grows, and it grows in a way that can fail silently.** This
  is the first mode to use a server-side tool, so `converse` has to collect
  server-tool results (it only tracked client tools) and it has to *resume* a
  `pause_turn` rather than reading it as a finished answer. A paused turn
  returns text that looks complete — the same shape as Forge playing on with 96
  cards and reporting a winner, and it gets the same treatment: handled
  explicitly, with a ceiling, rather than trusted.
- **A dossier can be wrong in a way the gate cannot catch.** Nothing here is
  reproducible and nothing validates a claim about Magic's history. What the
  design buys is that a wrong claim is *attributable* — it has a link, or it is
  Claude's framing and says so — and that it can never leak into a
  deterministic answer, because no dossier is an input to the gate, the solver,
  the simulator or a `why`.
- **If nothing survives validation, the dossier is refused rather than shown.**
  An unsourced dossier that renders anyway is exactly the blended paragraph this
  ADR rejected, arrived at by accident.
- **It costs one call per commander, once.** The cheapest possible shape for a
  feature that reaches the network, and the reason the key is `oracle_id`.
- **A fifth mode means ADR 15's table is now a list rather than a design.**
  Four was a guess and this ADR is the guess being wrong in the direction ADR 15
  predicted. What holds is the structure — a mode is a prompt, a tool set and a
  capability declaration — which absorbed a server-side tool and a second class
  of evidence without the write column moving.
