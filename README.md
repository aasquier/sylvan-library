# sylvan-library

[![tests](https://github.com/aasquier/sylvan-library/actions/workflows/ci.yml/badge.svg)](https://github.com/aasquier/sylvan-library/actions/workflows/ci.yml)

A Commander deckbuilding lab where the deck file is the source of truth, every
card has to justify its slot, and every claim about a deck is traceable to the
system that made it.

Python 3.11+ · DuckDB · numpy · FastAPI · React. The package and CLI are named
`mtglab`; the repository is `sylvan-library`.

## The idea

Most deckbuilding tools are lists. This one is an argument.

A deck is one YAML file. Each of the 99 cards carries a `category` and a
`why` — the reason it is in the deck rather than something else. Nothing
generates that sentence for you, and no write path accepts an empty one: a card
you cannot justify is a card to cut. The five documents a deck ships with —
two primers, an annotated list, a Moxfield import, and a swap diff — are
*generated* from that file and never edited by hand.

Around it sit three things, each answering a different kind of question:

- **A gate**, not a report. Deck size, singleton, colour identity, bans,
  companion and partner rules, and a missing rationale are all *checkable
  facts*. The build refuses to emit anything while one is wrong, because
  colour-identity and oracle-text mistakes are exactly what gets missed when a
  card is evaluated from memory instead of looked up.
- **A simulator**, in three tiers, kept separate so it is always clear which
  claims are load-bearing.
- **A model**, for the questions with no right answer — and fenced off from the
  ones that have one.

That last split is the project's central design decision
([ADR 14](docs/adr/0014-python-decides-claude-advises.md)): **anything with a
right answer belongs in deterministic Python; Claude is for opinions and
research.** Legality, colour identity, mana solving, simulation and pricing are
reproducible, tested without a network, and never ask a model. Claude gets the
meta, the history, and whether a spoiled card earns a slot — and may argue
about a card's slot while never authoring the `why` that lands in the file.
That rule is enforced structurally rather than by prompt: no module under
`src/mtglab/claude/` may name a deck-write function at all, checked over the
package's syntax tree, so the commit that adds one fails before a prompt is
written.

## What it does

**Deck files.** Import a pasted decklist, validate it against the gate, get
ranked replacement suggestions for whatever it flagged, and apply surgical
edits that re-run the gate on the result and produce minimal diffs. Every
edit lands in the deck's activity log, and the swap list is a real diff
against the last build's snapshot.

**Simulation.** Tier 1 is a Monte Carlo goldfish in pure Python — shuffles,
draws, and pays costs, answering mulligan policy, land count, commander speed
and colour consistency. Castability is solved as bipartite matching rather than
by counting sources, because counting gets it wrong: with a W/U dual and a
Forest you "have" a white source and a blue source and two mana, but `{W}{U}`
is not castable, since the dual cannot tap twice. Tier 3 drives
[Forge](https://github.com/Card-Forge/forge) headless for real games under real
rules. Tier 2 — an abstract pod simulator — is deferred behind Tier 3 and may
never need building.

**A web app.** Deck library with real card art, the annotated 99, deterministic
stats, card search over the whole printed history, the simulator with live
progress, and a teaching layer for Magic's colour combinations and vocabulary.
It needs no Node to run: the built frontend is committed.

**Claude surfaces**, opt-in and behind their own install extra. A rationale
interview that asks about a card's slot so you can write its `why`; a slot
argument that makes the case *against* a card and is built with no field for
the case in favour, because the balanced version would be a rationale
generator; a commander
dossier that says who a commander is and where they sit in Magic's history,
citing sources checked against what the search actually returned; and a theme
interview whose questions are *not about Magic* — a film, a period, how you are
at game night — which then proposes colour combinations and commanders. That
one has a tarot door: 78 cards, a seeded shuffle, and no card meanings in the
code, because the reader reads.

## How it fits together

```
decks/<slug>/deck.yaml   the source of truth
  -> decks/validate.py   the gate: legality, identity, bans, rationale
  -> sim/compile.py      deck + card pool -> SimCards
  -> sim/tier1/engine.py Monte Carlo, memoised on compiled input
  -> artifacts/          the five generated documents
```

| Area | Where |
| --- | --- |
| Cost parsing, castability solver, colour identity | `mana.py` |
| Scryfall bulk ingest, DuckDB schema, price history | `cards/db.py` |
| Deck format, the gate, surgical edits, suggestions | `decks/` |
| Monte Carlo, result cache, the Forge bridge | `sim/` |
| The five generated documents | `artifacts/generate.py` |
| Colour and vocabulary reference prose | `colors.py`, `glossary.py` |
| Client, stance, personas, and the five modes | `claude/` |
| Accounts, sessions, invites, admin bootstrap | `auth/` |
| HTTP API, background jobs, request scope | `api/` |
| React frontend (source, and committed build) | `web/`, `web_dist/` |

Card facts come from a local **card pool** — Scryfall's daily bulk files loaded
into DuckDB, around 35k oracle cards and 107k printings. It is fetched on
demand, never committed, and never redistributed
([ADR 6](docs/adr/0006-never-redistribute-scryfall-bulk-data.md)). Card images
load in the browser straight from Scryfall's CDN.

## Running it

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev,api]"
mtglab data refresh          # Scryfall bulk -> DuckDB, several minutes
mtglab ui                    # the app, at http://127.0.0.1:8765
```

macOS ships a Python older than 3.11; [uv](https://docs.astral.sh/uv/) is the
quickest fix. Install extras are `api` (the app), `claude` (the Anthropic SDK)
and `dev` (both, plus test tooling) — a bare `pip install -e .` still gets the
gate, the mana solver and Tier 1, which need neither an account nor a network.

Full setup, the command reference and the deck workflow are in
**[CONTRIBUTING.md](CONTRIBUTING.md)**.

## Status

In daily use against six curated Commander decks. A hosted instance is
deployed and running — invite-only, because accounts exist to share decks with
a playgroup rather than to sign the public up.
[docs/HOSTING.md](docs/HOSTING.md) is the guide to standing up your own.

**Built:** the gate, the mana solver, Tier 1 with a result cache, the Tier 3
Forge bridge, the artifact generator, the web app, accounts with invites and
email password resets, admin authorization, deck ownership and sharing, the
stance dial, and five Claude modes across four features — the rationale
interview, the slot argument, the commander dossier, and the theme interview's
two halves.

**Not built**, stated plainly so nothing here reads as a promise: the Tier 2
pod simulator, the deck tier list that depends on a pod measurement, the two
Claude modes [ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
names that remain (research and deck conversation), card-level spoiler
scanning, and deal-watching or cart generation.

Two decks deliberately fail the gate on one card each — Goreclaw runs Primeval
Titan and Atla Palani runs Emrakul, the Aeons Torn, both banned in Commander.
That is the gate working, and picking the replacement is a human's call.

### Known limits

- Tier 1 runs ~750 games/sec at 12 turns in pure Python; 60k games is about 80
  seconds. If that becomes annoying, the inner loop is what moves to Rust —
  [ADR 3](docs/adr/0003-tier-1-stays-python.md) holds the written trigger.
- Tier 1 models no opponents, no interaction, and no card text beyond mana
  production. Decks leaning on tutors or cost reduction look worse than they
  play. Quote it with that caveat attached.
- Forge's AI is competent with aggro and midrange, weak with control and poor
  with combo, so it systematically undersells combo decks. Results are reported
  per archetype, never as one ranking.
- Land-sweep resizing preserves the colour mix but not specific utility lands.
  Good for finding the count; re-validate the real list afterwards.

## Documentation

| File | What it is |
| --- | --- |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Setup, the command reference, the deck workflow, and the two rules |
| [ROADMAP.md](ROADMAP.md) | Goals versus reality, and the open decisions |
| [docs/ENGINEERING.md](docs/ENGINEERING.md) | Testing rigor, containers, CI/CD — with the measurements behind each call, including the ones that say *don't* |
| [docs/HOSTING.md](docs/HOSTING.md) | Deploying a shared instance: auth, per-user data, costs, and a Fly.io runbook |
| [docs/FORGE.md](docs/FORGE.md) | The Tier 3 bridge: setup, coverage checking, measured timings |
| [docs/adr/](docs/adr/) | 25 architecture decision records — context, options, decision, consequences. Immutable once accepted |

## Licence and fan content

Unofficial Fan Content permitted under the Wizards of the Coast Fan Content
Policy. Not approved or endorsed by Wizards. Portions of the materials used are
property of Wizards of the Coast. ©Wizards of the Coast LLC.

The Fan Content Policy permits **noncommercial use only** — this stays free,
including in forks. Card data comes from Scryfall at runtime and is never
committed. See [NOTICE.md](NOTICE.md) for full attribution.

**Do not commit collection, wishlist, or purchase data.** A public inventory of
expensive cards tied to a real identity is a targeting list, and CI fails the
build if one is committed.
