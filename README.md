# mtg-lab

Local toolkit for Commander deckbuilding, playtesting, simulation, and shopping.

Python 3.11+ · DuckDB · numpy · FastAPI. Runs on your own machine against a
local card corpus; nothing is required to leave it for the toolkit to work.

Two planned additions do reach outside, and both are opt-in rather than
load-bearing: a **Claude surface** for conversation and research
([ADR 14](docs/adr/0014-python-decides-claude-advises.md)) would need an API
key, and a **shared instance** ([docs/HOSTING.md](docs/HOSTING.md)) would need
accounts. Neither is built. The deterministic core — the gate, the mana solver,
the simulator — never calls either, and is tested without a network.

## Setup

Needs Python 3.11+. macOS ships 3.9 or older, so check before assuming:

```bash
python3 --version
```

If that is below 3.11, the quickest fix is [uv](https://docs.astral.sh/uv/),
which installs a standalone interpreter without touching the system one:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
uv python install 3.12
uv venv --python 3.12
uv pip install -e ".[dev,api]"
```

Otherwise the usual path works:

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev,api]"
```

Then:

```bash
mtglab data refresh          # ~15 min first run: Scryfall bulk -> DuckDB
mtglab decks list
mtglab ui                    # the app, at http://127.0.0.1:8765
```

## The app

`mtglab ui` serves a local web app: deck library with real card art, the
annotated 99 with every card's rationale, deterministic deck stats, a corpus
search over the whole printed history, and the Tier 1 simulator with live
progress.

**You do not need Node to run it.** The built frontend is committed at
`src/mtglab/web_dist/`. Node is only required to *change* the frontend:

```bash
npm --prefix web install
npm --prefix web run dev     # Vite on :5173, proxying /api to :8765
mtglab ui --dev              # run the API alongside it
npm --prefix web run build   # rebuild the committed bundle
```

Card images load in the browser directly from Scryfall's CDN; the URLs are
already in the local corpus, so nothing is proxied or re-hosted.

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

mtglab ui [--port 8765] [--dev]       # the local app
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

**Tier 3 — Forge headless (next).** `forge.jar sim -d ... -n 100` gives real
rules and real cards, which is a decade of engine work this project is not
going to repeat. Its AI is competent with aggro/midrange, weak with control,
poor with combo, so it systematically undersells combo decks — results are
reported per archetype, never as one ranking. A cross-check on Tier 1, never
ground truth.

**Tier 2 — abstract pod simulator (deferred behind Tier 3).** Four-player
table, each deck compiled to a policy profile (curve, interaction density,
threat clock, combo turn, tutor count, protection). Archetype opponents. This
is a *model of* Magic — right for bracket placement and matchup matrices, wrong
for "is this line correct." It is a large build whose fidelity nobody has had
to defend yet, so Forge goes first: if Forge answers those questions, Tier 2
may never need building. See `ROADMAP.md` goal 2.

**Playing games is the engine's job, not a model's.** An earlier plan had
Claude in an opponent seat reasoning over board JSON; ADR 14 retired it. Claude
is for conversation and research — the questions a corpus cannot answer —
while anything with a right answer stays in deterministic Python.

## Status

All six decks are migrated to `deck.yaml`, the local app runs, and the
simulator, gate and artifact generator are in daily use. **216 tests**, CI on
3.11 and 3.12.

| Area | Where |
| --- | --- |
| Cost parsing, castability solver, colour identity | `mana.py` |
| Scryfall bulk ingest, DuckDB schema, price history | `cards/db.py` |
| Deck file format and YAML round-trip | `decks/model.py` |
| The gate | `decks/validate.py` |
| Companion restrictions, Partner/Background pairings | `decks/companion.py`, `decks/partners.py` |
| Macro category counts vs bracket targets | `decks/analyze.py` |
| Deck + corpus to SimCards | `sim/compile.py` |
| Monte Carlo, mulligan policies, land sweeps | `sim/tier1/engine.py` |
| The five deliverables | `artifacts/generate.py` |
| Local app: HTTP API, background sim jobs, React UI | `api/`, `web/` |
| Paths, environment overrides | `config.py` |

Two decks fail the gate on one card each — Goreclaw runs Primeval Titan and
Atla Palani runs Emrakul, the Aeons Torn, both banned in Commander. That is
the gate working, not a defect.

Not built: the Tier 2 pod simulator, the deck tier list that depends on it,
card-level spoiler scanning, and deal-watching or cart generation.

## Roadmap

**[docs/ENGINEERING.md](docs/ENGINEERING.md)** is where the project is
heading next: property-based and differential testing, container hardening,
and automated review on PRs — with the measurements behind each call,
including the ones that say *don't*. A compiled rewrite is deliberately
deferred, with a written trigger for reopening it.

**[ROADMAP.md](ROADMAP.md) is the plan** — original goals mapped to what
actually works, plus the open decisions. It is kept current; this file only
summarises. **[docs/HOSTING.md](docs/HOSTING.md)** covers deploying a shared
instance: auth, per-user data, measured compute costs and a Fly.io setup guide.

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
