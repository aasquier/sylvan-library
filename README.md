# mtg-lab

Local toolkit for Commander deckbuilding, playtesting, simulation, and shopping.

Python 3.11+ · DuckDB · numpy · FastAPI (later phases). No cloud, no accounts,
no API keys. Everything runs on this laptop.

## Setup

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"

mtglab data refresh          # ~15 min first run: Scryfall bulk -> DuckDB
mtglab decks list
```

`data refresh` pulls Scryfall's daily bulk files (one HTTP request each, not
per-card scraping) and loads ~35k oracle cards plus ~500k printings locally.
After that, corpus queries are local and instant.

## Concepts

**`decks/<slug>/deck.yaml` is the source of truth.** Not Moxfield, not a
Google Doc. Every card carries its category and the reason it's in the deck.
The five deliverables are *generated* from it:

| File | What it is |
| --- | --- |
| `primer-quick.md` | One page — pick the deck up and play it |
| `primer-advanced.md` | Lines, sequencing, matchups, failure modes |
| `decklist-annotated.md` | The 99, categorised, every card justified |
| `moxfield.txt` | Bulk import (Moxfield has no public API; text is the path) |
| `swaps.md` | Out/in diff + shopping list, when the deck changed |

`swaps.md` is a **git diff of deck.yaml**, so commit before you edit. Deck
history is git history.

**Validation is a gate, not a report.** `mtglab decks build` refuses to emit
anything while there are errors: wrong deck size, a duplicate non-basic, a card
outside the commander's color identity, a banned card, a card missing its
`why`. This exists because color-identity and oracle-text mistakes are
*checkable facts* that get missed when cards are evaluated from memory.

## Commands

```bash
mtglab data refresh [--oracle-only]   # Scryfall bulk -> DuckDB
mtglab data snapshot                  # append today's prices to history

mtglab decks list
mtglab decks validate <slug>          # the gate
mtglab decks build <slug> [--against path/to/old/deck.yaml]

mtglab sim mana <slug>                # Tier 1 goldfish
mtglab sim lands <slug> 30 40         # land-count sweep, flood-aware

mtglab price deck <slug>              # cheapest non-promo printing per card
```

## Simulation tiers

Kept deliberately separate so it's always clear which claims are load-bearing.

**Tier 1 — stochastic goldfish (built).** Shuffles, draws, and pays costs.
Answers: mulligan policy, land count, commander speed, color consistency. No
opponents, no interaction, no card text beyond mana production. Every number it
produces follows from sampling, with no hidden judgement about "good" play.

Castability is solved as **bipartite matching**, not counting. Counting gets
this wrong: with a W/U dual and a Forest you "have a white source" and "have a
blue source" and two total mana, but `{W}{U}` is not castable, because the dual
can't be tapped twice. `tests/test_mana.py` pins that case.

Reading a land sweep: use **spells deployed through T8**, not commander speed.
Commander speed rises monotonically with land count, so optimising it alone
recommends 40 lands. Deployment peaks and then falls as flood sets in — that
peak is the answer.

**Tier 2 — abstract pod simulator (next).** Four-player table, each deck
compiled to a policy profile (curve, interaction density, threat clock, combo
turn, tutor count, protection). Archetype opponents. This is a *model of*
Magic — right for bracket placement and matchup matrices, wrong for "is this
line correct."

**Tier 3 — Forge headless (later).** `forge.jar sim -d ... -n 100` gives real
rules and real cards. Its AI is competent with aggro/midrange, weak with
control, poor with combo, so it systematically undersells combo decks. A
cross-check on Tiers 1–2, never ground truth.

## Status

Built and tested (38 tests):

- `mana.py` — cost parsing incl. hybrid/Phyrexian/monocolor-hybrid, exact
  castability solver, color identity
- `cards/db.py` — Scryfall bulk ingest, DuckDB schema, price history table
- `decks/model.py` — deck file format, YAML round-trip
- `decks/validate.py` — the gate
- `sim/tier1/engine.py` — Monte Carlo, mulligan policies, land sweeps
- `artifacts/generate.py` — all five deliverables
- `cli.py`, `.claude/skills/mtg-lab/SKILL.md`

Not built yet — roadmap below.

## Roadmap

**Phase 2 — decks in, corpus wired.** Migrate the five existing decks into
`deck.yaml` files, validate each against the real corpus, regenerate all
artifacts. Expect the gate to find things.

**Phase 3 — Tier 2 pod simulator.** Policy-profile compiler, archetype agents,
round-robin, Elo-style ranking, matchup matrix. Delivers the deck tier list.

**Phase 4 — shopping and deal-watching.** Daily price snapshots (cron), a
`deals` command flagging cards below their trailing median, cheapest-printing
selection, Mass Entry cart generation. *Hard boundary: never enters payment
details or completes a purchase.*

**Phase 5 — spoiler scanning.** Pull unreleased set codes from Scryfall
previews, score each new card against every deck (identity legal? tag match?
beats a current slot?). Next targets: Reality Fracture (Oct 2), Mystery
Booster: Commander Edition (Nov 9), Star Trek (Nov 13).

**Phase 6 — the local UI.** FastAPI + React, Scryfall art. Board-state manager
plus Claude in an opponent seat, reasoning over the board as JSON — *not* a
rules engine. Building a real one is what took Forge and XMage a decade each.

**Phase 7 — Tier 3 Forge bridge.** deck.yaml → `.dck` export, batch sim runner,
result parser.

## Sharing this

Unofficial Fan Content permitted under the Wizards of the Coast Fan Content
Policy. Not approved or endorsed by Wizards. Portions of the materials used are
property of Wizards of the Coast. ©Wizards of the Coast LLC.

The Fan Content Policy permits **noncommercial use only** — this stays free,
including in forks. Card data comes from Scryfall at runtime and is never
committed. See `NOTICE.md` for full attribution and `CONTRIBUTING.md` before
adding decks.

**Do not commit collection, wishlist, or purchase data.** CI fails the build if
you try.

## Known limits

- Tier 1 runs ~750 games/sec at 12 turns in pure Python. 60k games ≈ 80s.
  If that becomes annoying, the inner loop is the thing to move to Rust.
- Land-sweep resizing cycles the existing land pool, so it preserves the color
  mix but not specific utility lands. Good for finding the count; re-validate
  the actual list afterwards.
- Tier 1 doesn't model card draw beyond one per turn, tutors, or cost
  reduction. Decks leaning hard on those will look worse than they play.
