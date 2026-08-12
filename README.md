# mtg-lab

Local toolkit for Commander deckbuilding, playtesting, simulation, and shopping.

Python 3.11+ · DuckDB · numpy · FastAPI. Runs on your own machine against a
local card corpus. **The deterministic core — the gate, the mana solver, the
simulator — never reaches the network, and its whole test suite runs without
one.**

One thing now reaches outside, opt-in and behind its own install extra: a
**Claude surface** for conversation and research
([ADR 14](docs/adr/0014-python-decides-claude-advises.md),
[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)). The
pipe is built — a client and read-only tools over the corpus — and the modes
and UI on top of it are not. Install without `[claude]` and nothing about the
toolkit changes: no account, no key, no calls.

A **shared instance** ([docs/HOSTING.md](docs/HOSTING.md)) would need accounts.
Not built.

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

Extras: `api` (the app), `claude` (the Anthropic SDK), `dev` (both, plus test
tooling). A bare `pip install -e .` gets the gate, the mana solver and the
simulator, which need neither an account nor a network.

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

**A deck also declares a `stage`, separately from whether it exists.** A list
brought in with `decks import` starts as a `draft`, where a missing `why` is one
counted warning rather than 99 errors — so the deck's *facts* get checked on day
one while the thinking is still owed. Promotion to `curated` is refused while
any card is blank, and the artifacts refuse a draft outright
([ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md)).

**Nothing writes a rationale for you.** Every write path refuses an empty `why`
rather than inventing one — including the Claude surface, which may argue about
a card's slot and may never author the text that lands in `deck.yaml`. That is
enforced structurally, not by asking nicely: no module under
`src/mtglab/claude/` may reference a deck-write function at all, and a test
fails on the commit that adds one
([ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)).

## Commands

```bash
mtglab data refresh [--oracle-only]   # Scryfall bulk -> DuckDB
mtglab data snapshot                  # append today's prices to history

mtglab decks list
mtglab decks import <slug> --from list.txt --commander 'X'   # -> a draft
mtglab decks validate <slug>          # the gate
mtglab decks suggest <slug>           # replacements for what the gate flagged
mtglab decks build <slug> [--against path/to/old/deck.yaml]

# Surgical edits (ADR 12) -- each one re-runs the gate on the result
mtglab decks add <slug> --card X --category ramp --why '...'
mtglab decks remove <slug> --card X
mtglab decks set <slug> --card X --why '...'      # or --category / --qty
mtglab decks swap <slug> --out X --in Y --why '...'
mtglab decks note <slug> --key mulligan --value '...'
mtglab decks promote <slug>           # draft -> curated, once every card is justified

mtglab sim mana <slug>                # Tier 1 goldfish
mtglab sim lands <slug> 30 40         # land-count sweep, flood-aware

mtglab price deck <slug>              # cheapest non-promo printing per card

mtglab claude check                   # one real API call -- is the key live?
mtglab ui [--port 8765] [--dev]       # the local app

mtglab users add <name> [--admin]     # prompts twice; there is no --password
mtglab users list                     # who exists, and who can log in
mtglab users passwd <name>            # prompts; ends every session
mtglab users disable|enable <name>
```

The `users` commands are for a **hosted** instance and do nothing to a local
one: authentication is off unless `MTGLAB_REQUIRE_AUTH` is set, so `mtglab ui`
on your own machine has no login and never will. See
[docs/HOSTING.md](docs/HOSTING.md) §1.

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

**Tier 3 — Forge headless (next, not built).** [Forge](https://github.com/Card-Forge/forge)
is an open-source rules engine with a documented headless mode:

```bash
forge sim -d <deck.dck> ... -f Commander -n 100 -c 300 -q
```

Real rules and real cards — a decade of engine work this project is not going
to repeat. Its AI is competent with aggro/midrange, weak with control and poor
with combo, so it systematically undersells combo decks; results get reported
per archetype, never as one ranking. A cross-check on Tier 1, never ground
truth.

**Nothing else is a candidate**, which is worth stating so it stops being
re-asked. [XMage](https://github.com/magefree/mage) has excellent rules
coverage but is a networked play server with no headless batch mode. Cockatrice
has no rules enforcement at all — it is a virtual tabletop. Arena and MTGO are
closed, forbid automation, and Arena has no Commander. Deck sites like
MTGGoldfish are prices and lists, not engines.

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
simulator, gate and artifact generator are in daily use. **532 Python tests and
90 frontend tests** as of 2026-08-11, CI on 3.11 and 3.12 with a coverage floor,
ruff, a committed-bundle drift check, and a secrets scan that fails on an API
key in any tracked file.

| Area | Where |
| --- | --- |
| Cost parsing, castability solver, colour identity | `mana.py` |
| Scryfall bulk ingest, DuckDB schema, price history | `cards/db.py` |
| Deck file format and YAML round-trip | `decks/model.py` |
| The gate | `decks/validate.py` |
| Pasted decklist -> parsed lines -> a draft deck | `decks/decklist.py`, `decks/importer.py` |
| Surgical deck edits, minimal diffs | `decks/edit.py` |
| Replacement similarity scoring | `decks/suggest.py` |
| Companion restrictions, Partner/Background pairings | `decks/companion.py`, `decks/partners.py` |
| Macro category counts vs bracket targets | `decks/analyze.py` |
| Deck + corpus to SimCards | `sim/compile.py` |
| Monte Carlo, mulligan policies, land sweeps | `sim/tier1/engine.py` |
| The five deliverables | `artifacts/generate.py` |
| Local app: HTTP API, background sim jobs, React UI | `api/`, `web/` |
| Claude client and read-only corpus tools | `claude/` |
| Paths, environment overrides | `config.py` |

Two decks fail the gate on one card each — Goreclaw runs Primeval Titan and
Atla Palani runs Emrakul, the Aeons Torn, both banned in Commander. That is
the gate working, not a defect.

Not built, and stated plainly so nothing here reads as a promise: **the Forge
bridge (Tier 3)** and the Tier 2 pod simulator deferred behind it, the deck tier
list that depends on them, the **Claude modes, stance and UI** on top of the
client that does exist, card-level spoiler scanning, and deal-watching or cart
generation.

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
