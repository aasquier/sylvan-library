# 8. The gate blocks, and an unevaluated rule warns rather than passing

**Status:** Accepted · **Recorded:** 2026-08-10

## Context

`mtglab decks validate` checks a deck against Commander's construction rules:
size, colour identity, singleton, banned cards, companion restrictions, partner
pairings, and a `why` on every card. Artifact generation runs behind it.

Two of the six curated decks fail. **Goreclaw** runs Primeval Titan and **Atla
Palani** runs Emrakul, the Aeons Torn — both genuinely banned in Commander,
confirmed against Scryfall's `legalities.commander`, and neither a transcription
slip. Both lists really contain them.

So the gate had to decide what to do about a deck its owner has not fixed yet.

## Options considered

**Warn and generate anyway.** Rejected. The artifacts are the shareable surface
of this project. A primer for an illegal deck is worse than no primer, because it
looks exactly like a primer for a legal one.

**Auto-substitute a replacement.** Rejected firmly. Picking the card that fills
"6/6 trample, fetches two lands on ETB and on attack" is a deckbuilding
judgement, and it belongs to the deck's owner. A tool that quietly swaps cards is
a tool whose output you can no longer trust to be your deck.

**Block, and say exactly why.** Chosen.

## Decision

An error blocks artifact generation. Both decks sit at 99 cards with one illegal
slot until a human picks the replacement. **This is the gate working, not a bug
to route around**, and it is recorded in ROADMAP as an open decision rather than
quietly suppressed.

The second half of the decision matters more, because it generalises: **a rule
the gate cannot evaluate warns loudly and is never reported as satisfied.**

- An unrecognised companion produces `companion-unchecked`, not a silent pass.
- Zirda's "every permanent has an activated ability" test is a
  colon-plus-keyword heuristic, so it reports at warning level rather than
  blocking generation on a guess.
- Three companions whose conditions reference expansion symbols, retro frames
  and specific sets — properties of a *printing*, not of an oracle card — are
  deliberately reported as unchecked. None is legal in Commander, so none can
  legitimately appear anyway.

An illegal pairing says precisely why: "Lore Weaver has Partner with Ley Weaver,
so it can only pair with that card", rather than refusing. That phrasing exists
because "does not say it can be your commander" reads like a data problem rather
than a rule, and sent one investigation in the wrong direction already.

## Consequences

- Two decks cannot generate artifacts, and that shows up as a visible gap rather
  than as a quietly-wrong document. `GET /api/decks` carries error and warning
  counts so the Library flags a deck that does not validate, instead of rendering
  a banned card exactly like a clean list.
- The gate is only as good as what it checks, so *what it does not check* has to
  be visible. Kaheera's companion restriction was verified by hand once and not
  by the gate; that gap is exactly the kind this decision exists to surface, and
  it is now checked mechanically.
- Warnings must stay rare enough to read. `category-mismatch` on one Tivit land
  is currently the only standing warning across six decks, which is the right
  order of magnitude.
- The cost is friction: you cannot generate a primer for a deck you are
  mid-way through fixing. Accepted, because the alternative is a confident,
  wrong, shareable document — the exact failure mode
  [ADR 1](0001-deck-yaml-in-git-is-the-source-of-truth.md) and
  [ADR 7](0007-card-facts-come-from-the-corpus.md) exist to prevent.
