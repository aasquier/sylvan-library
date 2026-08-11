# 14. Python decides, Claude advises, and Forge plays the games

**Status:** Proposed — nothing implements this yet · **Recorded:** 2026-08-11

## Context

Everything this project has built so far is deterministic on purpose. The gate,
the mana solver, Tier 1, the similarity scorer and the price lookups all have
right answers, and they are reproducible: same input, same output, no judgement
in the middle. [Rule 1](../../CLAUDE.md) — never evaluate a card from memory —
exists because two specific, checkable facts were once got wrong by reasoning
instead of looking, and [ADR 7](0007-card-facts-come-from-the-corpus.md) made
that structural.

But several of the things this project actually wants are not determinable from
a card corpus. What is the format doing right now. Whether a card spoiled last
week is worth a slot. What a rules interaction means in practice rather than on
paper. Whether a deck's plan holds together, which is a judgement about a
strategy rather than a fact about a card. A DuckDB query cannot answer any of
those, and no amount of Python will make it.

Two more things forced the question now.

**`ROADMAP.md` goal 3 said "play against Claude in an opponent seat, reasoning
over board JSON."** That was written before the split was clear, and it is the
wrong shape: a language model reasoning over board state is neither a rules
engine nor a strong player. Meanwhile `CLAUDE.md` already documents a Tier 3
Forge bridge in detail — including which archetypes Forge's AI plays well —
because Forge is a real rules engine with a real AI, and building one of those
took Forge and XMage a decade each.

**And the deck lifecycle now produces decks with unjustified cards.** As of
[ADR 13](0013-an-imported-deck-is-a-draft.md), importing a list yields 85-odd
cards with an empty `why`, and the app counts them at the user. The moment
anything in this codebase can call a language model, the shortest path from
there is a "generate rationales" button — which would make
[rule 4](../../CLAUDE.md) and [ADR 8](0008-the-gate-blocks.md) decorative. The
boundary is worth writing down before the client exists, not after.

Nothing implements any of this today: no LLM SDK is a dependency, and the only
`ANTHROPIC_API_KEY` in the repository belongs to the CI pull-request reviewer in
`docs/ENGINEERING.md`, which is infrastructure rather than the app.

## Options considered

**Claude in an opponent seat, playing games.** Rejected, and it was the
published plan. A model reasoning over board JSON has to be handed the rules,
the board, and every card's behaviour on every decision — and it still plays
worse than an engine built for it, at a per-turn cost. Forge already solved
this. The one thing Claude would add over Forge is coverage of cards Forge does
not implement, which is exactly the case where its answers would be least
checkable.

**Claude writes the `why` fields.** Rejected, again — this is
[ADR 13](0013-an-imported-deck-is-a-draft.md) and
[ADR 11](0011-the-api-may-apply-a-swap.md) restated against a new capability.
A rationale written by the tool is the empty justification rule 4 exists to
prevent, and a draft's counted to-do list only means something because nothing
can silently discharge it.

**Keep everything deterministic; no model in the product at all.** Rejected.
It is coherent, and it is what the README claimed. But it permanently forecloses
the research and conversation the project is most short of, and it does not even
describe the status quo — the six curated decks were built by conversation, and
their `why` fields are the record of it.

**Split by whether the question has a right answer.** Chosen.

## Decision

**Python decides. Claude advises. Forge plays the games.**

**Deterministic Python owns everything with a right answer**, and keeps owning
it: legality, colour identity, singleton, deck size, companion and partner
rules, mana solving, Tier 1 goldfishing, category counts against bracket
targets, replacement similarity, and price. These are reproducible, they are
tested without a network, and no model is consulted about them. If a question
can be settled by querying the corpus, it is settled by querying the corpus.

**Claude owns opinions and research** — conversation with the user about a deck,
and the questions the corpus cannot answer: the current meta, whether a spoiled
card earns a slot, how a ruling resolves in practice, what a deck's plan is
missing. This is the part of the toolkit that has always been done in a chat
window; the decision is to bring it into the product rather than leave it
outside.

**Three boundaries make that safe.**

1. **Rule 1 still binds Claude.** Card facts come from the corpus — not from
   the model's recall, and not from a web page either. Web research is for what
   the corpus does not contain: discussion, meta, rulings, and cards spoiled
   ahead of the next bulk refresh. A claim about what a card *does* is looked
   up, always, whoever is speaking.

2. **Claude may argue about a `why`. It may not write one.** It can
   interrogate, challenge, propose angles, and make the case against a card's
   slot — that is precisely the conversation the curated six came out of. What
   it must not do is author the text that lands in `deck.yaml`, and no surface
   may pre-fill that field with generated prose for a human to wave through.
   This clarifies ADR 13 rather than reversing it: what rule 4 protects is that
   a person stands behind every claim in a generated document, and a rationale
   argued out with Claude and then written by the user satisfies that. One
   pre-filled with a draft and saved unread does not, and an edit-before-save
   gate does not fix it — it adds a click to the same failure.

3. **The provenance of an answer is always visible.** A user must be able to
   tell, without asking, whether they are looking at the gate's output —
   reproducible, checkable, the same tomorrow — or Claude's opinion. These are
   different kinds of claim and the interface must not blur them.

**Playing games is Forge's job.** `CLAUDE.md`'s Tier 3 requirements stand
unchanged and were written for this: per-archetype reporting because Forge's AI
is good with aggro and midrange and bad with control and most combo, a card
coverage pre-flight because silently dropped cards would poison the results, a
raised clock, and draws reported separately. Whether Forge is reachable at all
from a hosted deployment is an open question recorded in `ROADMAP.md`.

**No scraper.** Research goes through Anthropic's server-side web tooling, not
a crawler this project maintains. `CLAUDE.md`'s existing ban on marketplace
scraping is unchanged, and this decision does not become a way around it.

## Consequences

- **The README's "no cloud, no accounts, no API keys" is no longer true**, and
  had already stopped being true when `docs/HOSTING.md` was written. Corrected
  there rather than left as a pleasing claim.
- **An API key becomes a deployment concern.** `docs/HOSTING.md` §4 already has
  the pattern; this is one more secret, and it is the first one whose *usage*
  costs money per request.
- **A hosted instance means the maintainer pays for other people's questions.**
  That is a real constraint on "shareable with friends" and a reason the
  conversational surface may need a per-user budget before it is opened up. Not
  solved here; flagged so it is not discovered by invoice.
- **The determinism boundary is now a UI problem, not just an architectural
  one.** Consequence 3 above is easy to state and easy to erode one component
  at a time.
- **Tier 2 is deferred behind Forge** rather than cancelled — see `ROADMAP.md`.
  [ADR 3](0003-tier-1-stays-python.md)'s compiled-rewrite trigger was written
  against Tier 2's measurements, so that trigger now waits on whichever
  simulator gets built first. The trigger's *shape* — a written, measured
  threshold rather than a guess — is unchanged, which is why this does not
  supersede ADR 3.
- **This makes the project harder to reason about, and that is the real cost.**
  Every answer now belongs to one of two systems with different guarantees.
  The gate is worth trusting precisely because it never has an opinion; the
  moment a user cannot tell which system answered, the gate's credibility pays
  for Claude's mistakes.
- **Nothing here is built.** This ADR is the boundary, recorded before the
  first client call rather than after — see `ROADMAP.md` for the order of work.
