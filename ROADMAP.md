# Roadmap

The original goals, what actually works today, and what comes next. This file
is the durable plan — it survives a fresh session, unlike a conversation.

Status keys: **done** · **partial** · **not started**

---

## 1. Analyse or generate decks with simulation

| Sub-goal | Status | Where |
| --- | --- | --- |
| Mana base analysis | **done** | `sim/tier1/engine.py`, `mtglab sim mana` |
| Commander strategy / speed to online | **done** | commander-by-turn curve, median turn |
| Macro categories covered | **done** | `decks/analyze.py` — counts vs bracket targets |
| Colour identity confirmation | **done** | `decks/validate.py`, from Scryfall `color_identity` |
| Deep hits from all of Magic | **partial** | `mtglab ui` card search queries all 35k oracle cards; no "suggest for this deck" scoring yet |
| Best-in-slot alternatives | **not started** | needs a similarity/upgrade scorer |
| Upcoming spoilers for new decks | **partial** | `GET /api/sets/upcoming` is live; no card-level scan |
| Frugal alternatives | **partial** | price data loaded, shown in search; no "cheaper equivalent" logic |
| Pod simulation of real games | **not started** | Tier 2 |

Generating a deck from scratch is **not started**. Everything so far analyses a
list you supply.

## 2. Adversarial simulation between decks

**Not started.** Needs Tier 2 (pod simulator): four seats, each deck compiled to
a policy profile, archetype opponents, round-robin. This is also what produces
goal 7's tier list.

Opponent decks sourced from EDHREC/Moxfield/Archidekt is an **open decision** —
see below.

## 3. Play against Claude in a local UI

**Partial.** The UI exists (`mtglab ui`) with real Scryfall art, but there is no
play mode. Needs a board-state manager and Claude in an opponent seat reasoning
over board JSON. Explicitly *not* a rules engine — building one is what took
Forge and XMage a decade each.

## 4. Shopping, swaps, deals

**Partial.** `mtglab price deck` works and 107k printings with prices are
loaded, plus a `price_history` table for deal detection. No `deals` command, no
cart generation, no wishlist.

Hard boundary, unchanged: never enters payment details, never completes a
purchase. Carts are staged for a human to confirm.

## 5. Five artifacts per deck or refactor

**done** — `artifacts/generate.py`, via `mtglab decks build <slug>`:
`primer-quick.md`, `primer-advanced.md`, `decklist-annotated.md`,
`moxfield.txt`, and `swaps.md` when something changed.

Not yet *run* for any deck, because `swaps.md` is a git diff and wants a
committed baseline first.

## 6. Scan upcoming sets against curated decks

**Partial.** Upcoming set list is live from Scryfall. The card-level scan —
pull spoiled cards, filter to each deck's identity, score against current
slots — is not built.

## 7. Tier list of curated decks

**Not started.** Depends on Tier 2 and on having more than one deck migrated.

---

## Deck migration status

`decks/<slug>/deck.yaml` is the source of truth. Source material lives in
`~/Downloads` from earlier sessions; take the highest-numbered file per deck.

| Deck | Colours | Status | Source |
| --- | --- | --- | --- |
| Gyome, Master Chef — Food | Golgari, B4 | **migrated**, validates 0/0 | `02-the-99-annotated_1.md` |
| Arahbo — Cats | Selesnya (Kaheera companion) | not migrated | `arahbo-cats-decklist_4.md` |
| Atla Palani — Dinos | Naya | not migrated | `Atla-Palani-Annotated-Decklist.md` (+ `.txt`, swap list `_8`) |
| Goreclaw — Mono-green stompy | Green, B4 | not migrated | `goreclaw-mono-green-stompy_2.md` |
| Tivit — cEDH | Esper, B5 | not migrated | `tivit-cedh-bracket5.md` |
| Trostani — Tokens | Selesnya | not migrated | `trostani-tokens-FINAL-decklist_2.md` |

**Note:** CLAUDE.md says "five curated decks" and does not mention Trostani.
There are six sets of source files. Worth confirming whether Trostani is still
in rotation.

Expect the gate to find real problems in each — Gyome's migration turned up a
97-vs-99 card discrepancy and a tool bug that rejected every double-faced card.

---

## Suggested order

1. **Migrate the remaining decks.** Highest value: the Library screen is built
   for a shelf, a tier list is meaningless with one deck, and each migration
   exercises the gate against a new list.
2. **Play mode in the UI** — the fun one, and it needs no new engine work
   beyond board state.
3. **Tier 2 pod simulator** — unlocks adversarial sims and the tier list
   together.
4. **Spoiler scan** and **deals/carts** — both self-contained.

---

## Open decisions

### Hosting

Wanted: follow along remotely, and eventually point friends at it.

The constraint is data, not code. The corpus is ~63 MB of DuckDB built from
~98 MB of compressed Scryfall bulk, and it is gitignored on purpose — Scryfall
asks that bulk data not be redistributed, and it is re-downloadable in one
command. So a deployment must run `mtglab data refresh` at build or boot, not
ship the database in an image layer.

Also relevant: the Fan Content Policy permits **noncommercial use only**, so
whatever this runs on stays free to use.

Nothing about the current architecture blocks this — the API is a normal
FastAPI app and the frontend is static files.

### Rust or Go for the simulation core

Measured on this machine: `sim mana` at 20,000 games takes ~30s; a land sweep
across 11 counts at 25,000 games each takes ~5 minutes. Tier 1 is tolerable.

**Tier 2 is where this decides itself.** A pod simulator is four seats making
real decisions over more turns — plausibly 50-100x the work per game. If Tier 2
in Python turns out to take minutes per matchup, the inner loop moves to a
compiled language and the rest stays Python.

Do not port Tier 1 pre-emptively. `mana.py` and `sim/tier1/` are deliberately
stdlib-plus-numpy precisely so they *could* move later; the boundary already
exists. Measure Tier 2 first.

### Reaching outside Scryfall

CLAUDE.md currently forbids marketplace scraping and purchase automation. Goal
2 wants opponent decklists from EDHREC/Moxfield/Archidekt, and goal 4 mentions
acting as a user on TCGplayer.

Unresolved. Note that a shared repo spreads whatever is chosen to everyone who
runs it, and that entering payment details or completing a purchase stays off
the table regardless.

---

## What is solid underneath

Worth knowing before trusting any number: several bugs found this session were
producing confident, wrong answers for *every* deck, not just one.

- `qty` was ignored when compiling a deck for simulation, so a 99-card deck
  simulated as ~83 cards with 20 lands instead of 34.
- Tapland detection matched Scryfall's old wording, so every modern tapland
  compiled as untapped.
- Land-fetch ramp compiled to blank cards.
- `get_cards` matched only Scryfall's combined `Front // Back` name, so every
  modal DFC and adventure card was reported as unknown.

All four are fixed and pinned by tests. The lesson worth keeping: logic in
tested code gets caught, logic in conversation does not. 104 tests, CI runs
them on 3.11 and 3.12, typechecks and builds the frontend, and fails if the
committed bundle drifts from source.
