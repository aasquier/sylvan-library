# sylvan-library

[![tests](https://github.com/aasquier/sylvan-library/actions/workflows/ci.yml/badge.svg)](https://github.com/aasquier/sylvan-library/actions/workflows/ci.yml)

A Commander deckbuilding lab where the deck file is the source of truth, every
card has to justify its slot, and every claim about a deck is traceable to the
system that made it.

Live at **[sylvan-libraries.com](https://sylvan-libraries.com)** — invite-only,
because accounts exist to share decks with a playgroup rather than to sign the
public up. Free forever, and it will stay that way.

## The idea

Most deckbuilding tools are lists. This one is an argument.

A deck is one YAML file. Each of the 99 cards carries a category and a `why` —
the reason it is in the deck rather than something else. Nothing generates that
sentence for you, and no write path accepts an empty one: a card you cannot
justify is a card to cut. The documents a deck ships with — two primers, an
annotated list, an import file, and a diff of what changed since the last build
— are generated from that file and never edited by hand.

Around it sit three things, each answering a different kind of question:

- **A gate**, not a report. Deck size, singleton, colour identity, bans,
  companion and partner rules, and a missing rationale are all *checkable
  facts*. The build refuses to emit anything while one is wrong, because
  colour-identity and oracle-text mistakes are exactly what gets missed when a
  card is remembered instead of looked up.
- **A simulator.** Tier 1 is a Monte Carlo goldfish — shuffle, draw, pay costs
  — answering mulligan policy, land count, commander speed and colour
  consistency. Castability is solved as a matching problem rather than by
  counting sources, because counting gets it wrong: with a W/U dual and a
  Forest you "have" a white source and a blue source and two mana, but
  `{W}{U}` is not castable, since the dual cannot tap twice. Tier 3 drives
  [Forge](https://github.com/Card-Forge/forge) headless for real games under
  real rules.
- **Claude**, for the questions with no right answer — and fenced off from the
  ones that have one. Legality, identity, mana, simulation and price are
  deterministic code, reproducible and tested without a network. Claude gets
  the meta, the history, and whether a card earns a slot, and may argue about
  a card while never writing the `why` that lands in the file. That boundary
  is enforced structurally: no module in the Claude package may so much as
  name a deck-write function, checked over the syntax tree, so the commit that
  adds one fails before a prompt is ever written.

## What it does

**Decks.** Import a pasted decklist, validate it against the gate, get ranked
replacements for whatever it flagged, and apply edits that re-run the gate on
the result. Every edit lands in the deck's activity log.

**Simulation.** Tier 1 with a result cache keyed on the compiled deck, so the
same question asked twice is answered instantly and says that it was. Tier 3
plays real games on a worker machine that exists only while it is working.

**A web app.** Deck library with real card art, the annotated 99, deterministic
stats, card search over the whole printed history, the simulator with live
progress, a teaching layer for Magic's colours and vocabulary — and a
fortune-teller's table where a tarot spread asks about films and family rather
than about Magic, and reads a set of colours out of the answers.

## Built with

| | |
| --- | --- |
| **Go 1.26** | One static binary — HTTP server, CLI, simulator, and every API route. CGO is on; the card pool rides DuckDB. |
| **DuckDB** | The card pool: Scryfall's daily bulk files loaded locally, ~35k oracle cards and ~107k printings. Fetched on demand, never committed, never redistributed. |
| **SQLite** | Accounts, sessions, invites, the activity log, the match ledger. |
| **React + TypeScript** | The frontend, built with Vite and Tailwind. The bundle is committed, so the binary serves the app with no Node anywhere near it. |
| **Claude** | The Anthropic API behind the advisory surfaces, each one a mode with a declared scope and no write access. |
| **Fly.io** | One machine, one volume. A green merge to `main` deploys itself. |

The binary and CLI are named `mtglab`; the repository is `sylvan-library`. The
project's remaining Python lives in `tools/` — a local pipeline that renders
the site's committed art and card motion. It never ships and never serves.

## Running it

```bash
cd go && go build -o ../mtglab ./cmd/mtglab && cd ..
./mtglab data refresh        # Scryfall bulk -> DuckDB
./mtglab ui                  # http://127.0.0.1:8765
```

Go 1.26+ with CGO enabled is the only requirement. The Claude surfaces want an
`ANTHROPIC_API_KEY` in the environment; everything else — the gate, the mana
solver, the simulator — needs neither an account nor a network.

Setup in full, the command reference and the deck workflow are in
**[CONTRIBUTING.md](CONTRIBUTING.md)**; standing up your own instance is
**[docs/HOSTING.md](docs/HOSTING.md)**.

## Status

In daily use against six curated Commander decks. **The hosted instance is the
product** — `mtglab ui` runs the same app on a laptop, as a development harness
and for anyone who would rather keep their decks on their own machine.

Built: the gate, the mana solver, both simulator tiers, the artifact generator,
the web app, accounts with invites and email password resets, deck ownership
and sharing, and the Claude surfaces.

Not built, stated plainly so nothing here reads as a promise: the pod simulator
and the deck tier list that would depend on it, free-form deck conversation
with Claude, card-level spoiler scanning, and anything resembling a checkout.
Prices are shown; nothing is ever bought.

One deck deliberately fails the gate: Goreclaw runs Primeval Titan, banned in
Commander. That is the gate working, and picking the replacement is a human's
call.

### Known limits

- Tier 1 models no opponents, no interaction, and no card text beyond mana
  production. Decks leaning on tutors or cost reduction look worse than they
  play, and any number it gives should be quoted with that attached.
- Forge's AI is competent with aggro and midrange, weak with control and poor
  with combo, so it systematically undersells combo decks. Results are reported
  per archetype, never as one ranking.
- Land-sweep resizing preserves the colour mix but not specific utility lands.
  Good for finding the count; re-validate the real list afterwards.

## Documentation

| File | What it is |
| --- | --- |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Setup, the command reference, the deck workflow |
| [ROADMAP.md](ROADMAP.md) | What is being built next, and the open questions |
| [docs/ENGINEERING.md](docs/ENGINEERING.md) | Testing rigor, containers, CI/CD — with the measurements behind each call, including the ones that say *don't* |
| [docs/HOSTING.md](docs/HOSTING.md) | The maintainer's runbook: deploying, refreshing, backing up, rolling back |
| [docs/FORGE.md](docs/FORGE.md) | The Tier 3 bridge: setup, coverage, measured timings |
| [docs/HISTORY.md](docs/HISTORY.md) | How it was designed, kept for the reasoning rather than as a changelog |
| [docs/adr/](docs/adr/) | 38 architecture decision records — context, options, decision, consequences. Immutable once accepted |

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
