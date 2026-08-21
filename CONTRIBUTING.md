# Contributing / using this with your playgroup

## First: the two rules

**1. Never commit card data.** `data/` is gitignored. Scryfall's bulk files are
theirs, they're large, and redistributing them is against their guidelines
([ADR 6](docs/adr/0006-never-redistribute-scryfall-bulk-data.md)).
`mtglab data refresh` fetches everything in one command.

**2. Never commit anything describing what you physically own.** Collections,
wishlists, purchase history, order confirmations. A public list of expensive
cards attached to a real identity is a targeting list. `.gitignore` blocks the
obvious filenames and CI fails the build if one slips through — by filename,
and by scanning the contents of every tracked file for an API key — but neither
is a substitute for not doing it.

The same applies to `data/app.db`, which holds password hashes and email
addresses. It is gitignored and irreplaceable.

Git history is permanent and forks can't be recalled. If something sensitive
lands on `main`, deleting it in a later commit does not remove it.

## Getting set up

Needs Python 3.11+. macOS ships something older, so check first:

```bash
python3 --version
```

If that is below 3.11, [uv](https://docs.astral.sh/uv/) installs a standalone
interpreter without touching the system one:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
uv python install 3.12
uv venv --python 3.12
uv pip install -e ".[dev,api]"
```

Otherwise the usual path works:

```bash
git clone https://github.com/aasquier/sylvan-library && cd sylvan-library
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev,api]"
mtglab data refresh          # Scryfall bulk -> DuckDB; ~28 minutes, measured
pytest -q
```

`data refresh` needs network access to `api.scryfall.com` and
`data.scryfall.io`, and downloads roughly 100MB compressed. Budget about half
an hour — loading the printings is the slow half — and do not interrupt it: a
killed refresh leaves the pool with no printings until the next one finishes.
`--oracle-only` is much smaller and covers everything except pricing.

Five install extras: `api` (FastAPI and the app), `claude` (the Anthropic
SDK), `animist` (Pillow and the asset pipeline's video encoders), `depth`
(CPU torch and the depth-model loader — deliberately not vendored by `dev`:
~800MB of wheels for a loader no test may import), and `dev` (the first three,
plus the test tooling). A bare `pip install -e .` gets the gate, the mana
solver and Tier 1, which need neither a network nor an account.

### Working on the frontend

You do not need Node to *run* the app — the built bundle is committed at
`src/mtglab/web_dist/` ([ADR 9](docs/adr/0009-commit-the-built-frontend-bundle.md)).
Node is only needed to change it:

```bash
npm --prefix web install
npm --prefix web run dev     # Vite on :5173, proxying /api to :8765
mtglab ui --dev              # run the API alongside it
npm --prefix web run build   # rebuild the committed bundle -- required if you
                             # touched anything under web/src
```

### The Go front door

Since the port began ([ADR 38](docs/adr/0038-the-served-backend-is-rewritten-in-go.md),
[`docs/go-migration/`](docs/go-migration/README.md)) the deployed process is
a Go binary — `go/cmd/mtglab ui` — that takes the port, enforces the login in
front of everything, serves the bundle, and proxies `/api` to the Python
server running behind it. You do not need Go to run the app locally:
`mtglab ui` (Python) alone is still the whole app on a laptop. You need it to
change the door:

```bash
# Go 1.26 -- the last release that runs on the maintainer's macOS 12; the
# module pins it, so use a 1.26.x toolchain (https://go.dev/dl/)
cd go
go vet ./... && go test -race ./... && golangci-lint run ./...   # what CI requires
mtglab ui --port 8766 --no-open &        # the Python server, on another port
go run ./cmd/mtglab ui --upstream http://127.0.0.1:8766 \
    --web-dist ../src/mtglab/web_dist --tarot ../src/mtglab/assets/tarot
                                         # the door, on :8765 -- the app's usual address
```

golangci-lint is `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1`
(with `CGO_ENABLED=0` on an old macOS, where the cgo link fails).
`tests/contract/README.md` shows how to run the contract suite through the
door, which is the check every change to it is held to.

## Adding your own decks

Decks live at `decks/<slug>/deck.yaml` — your app data, not repository
content ([ADR 30](docs/adr/0030-decks-are-live-app-data.md)): the directory
is gitignored, so your lists stay yours and never ride into a pull request.
Start from [`docs/deck-template.yaml`](docs/deck-template.yaml), or skip the
template entirely with `mtglab decks import`.

Every card needs a `category` and a `why`. Validation fails without them, on
purpose: a card you cannot justify is a card to cut. **Nothing writes that
sentence for you** — not the CLI, not the app, and not the Claude surfaces,
which may interrogate a card's slot and may never author the text
([ADR 8](docs/adr/0008-the-gate-blocks.md),
[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)).

A deck imported with `decks import` starts as a **draft**, where a missing
`why` is one counted warning rather than 99 errors — so the deck's *facts* get
checked on day one while the thinking is still owed. Promotion to **curated**
is refused while any card is blank, and the artifacts refuse a draft outright
([ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md)).

## The workflow

```bash
mtglab decks import <slug> --from list.txt --commander 'X'   # -> a draft
mtglab decks validate <slug>          # the gate. fix errors first
mtglab decks suggest <slug>           # replacements for what the gate flagged
mtglab sim mana <slug>                # baseline consistency
mtglab sim lands <slug> 30 40         # is the land count right?
mtglab decks build <slug>             # stashes the baseline for the swap list
# ...edit deck.yaml...
mtglab decks build <slug>             # swaps.md now shows what you changed
```

`swaps.md` diffs against the last build's snapshot
(`artifacts/deck.last-built.yaml`). **Build before you edit** or the next
build has no baseline; `--against <path>` accepts an explicit one.

## Command reference

```bash
mtglab data refresh [--oracle-only]   # Scryfall bulk -> DuckDB
mtglab data snapshot                  # append today's prices to history

mtglab decks list
mtglab decks import <slug> --from list.txt --commander 'X'
mtglab decks validate <slug>
mtglab decks suggest <slug>
mtglab decks build <slug> [--against path/to/old/deck.yaml]
mtglab decks promote <slug>           # draft -> curated
mtglab decks delete <slug>            # confirm by typing `bury`; -> decks/.trash/

# Surgical edits (ADR 12) -- each re-runs the gate on the result
mtglab decks add <slug> --card X --category ramp --why '...'
mtglab decks remove <slug> --card X
mtglab decks set <slug> --card X --why '...'      # or --category / --qty
mtglab decks set <slug> --status built            # no --card: a deck field
mtglab decks set <slug> --art <set-code>          # which printing's art shows
mtglab decks swap <slug> --out X --in Y --why '...'
mtglab decks note <slug> --key mulligan --value '...'

mtglab sim mana <slug>                # Tier 1 goldfish
mtglab sim lands <slug> 30 40         # land-count sweep, flood-aware
mtglab sim cache [--clear]            # what Tier 1 results are memoised
mtglab sim forge <a> <b> [c] [d]      # Tier 3 -- Forge plays real games

mtglab price deck <slug>              # cheapest non-promo printing per card

mtglab claude check                   # one real API call -- is the key live?
mtglab claude interview <slug> --card X   # questions about a slot
mtglab claude dossier <slug>          # who the commander is (ADR 19)

mtglab ui [--port 8765] [--dev]       # the app on your own machine

mtglab users invite <email>           # an account, and a link to claim it
mtglab users add <name> [--admin]     # prompts twice; there is no --password
mtglab users list                     # who exists, and who can log in
mtglab users passwd <name>            # prompts; ends every session
mtglab users disable|enable <name>
mtglab users promote|demote <name>    # admin, and never the last one
```

The `users` commands are for a **hosted** instance and do nothing to a local
one: authentication is off unless `MTGLAB_REQUIRE_AUTH` is set, so `mtglab ui`
on your own machine has no login and never will. See
[docs/HISTORY.md](docs/HISTORY.md) §1, which is where that design is written
up; `docs/HOSTING.md` §1 is a stub pointing at it.

**What `mtglab ui` is.** Two things, and neither is "the product" — the
deployed instance is that. It is the **development harness**: the surface a
user-visible change is walked in before it lands, and what `--dev` exists for
(it runs the API alongside a Vite dev server). And it is the **contributor
engine**: the whole app, running over decks you keep locally, which is the
story this file tells. Aaron's own library lives on the deployed volume and
his laptop keeps no copy of it ([ADR 30](docs/adr/0030-decks-are-live-app-data.md)
and the 2026-08-21 ruling above it) — but a stranger's checkout is exactly the
local-first shape, so `decks/` being your own app data is still true here.

## Reading the simulator honestly

Tier 1 shuffles, draws, and pays costs. That is all. It does not model
opponents, interaction, tutors, cost reduction, or card text beyond mana
production. It answers mana and consistency questions well and answers nothing
else. Quote it with that caveat attached.

For land counts, read **spells deployed through T8**, not commander speed.
Commander speed rises forever with more lands, so optimising it alone tells you
to play 40. Deployment peaks and then falls as flood sets in. That peak is the
answer.

Tier 1 results are cached and seeded ([ADR 18](docs/adr/0018-a-cached-simulation-is-keyed-on-its-compiled-input.md)),
and every result carries `seed`, `cached` and `computed_at`. **Quote a cached
number as cached.**

Forge (Tier 3) is best with aggro and midrange, poor with control and bad with
most combo, so report its results per archetype rather than as one ranking.
Quote a median and a tail, never a mean — game length is heavily right-skewed.
A card Forge does not implement is *silently dropped*, which is why coverage is
checked before and after every run. See [docs/FORGE.md](docs/FORGE.md).

## Code

- Pure stdlib + numpy in the sim core; DuckDB stays behind `cards/db.py`.
  Keeping the core dependency-light is what makes it fast to test.
- `api/` must not import from `cli.py`. Anything both need lives in
  `config.py` or the relevant package.
- Deck-facing endpoints never read the filesystem — they take a `DeckSource`
  from the request scope (`api/deps.py`).
- Never evaluate a card from memory; look it up in the card pool. Colour
  identity comes from Scryfall's `color_identity` field, never derived from the
  mana cost — it already accounts for back faces, reminder text and land types
  ([ADR 7](docs/adr/0007-card-facts-come-from-the-corpus.md)).
- Every bug fix gets a test. The mana solver is subtle — `tests/test_mana.py`
  pins the cases where naive source-counting gives the wrong answer, and
  `tests/test_mana_properties.py` covers what nobody thought of.
- Adding a route means classifying it in `tests/test_isolation.py`; the suite
  fails until you do.
- A new module is strict under mypy from the day it is written. The exemption
  list in `pyproject.toml` is meant to shrink.

### Before you push

```bash
ruff check src tests && mypy
pytest -q
npm --prefix web run check      # tsc, oxlint and vitest in one
npm --prefix web run build      # if anything under web/src changed
```

CI runs all of it. `main` is protected: pull request required, all six checks
green, branch up to date, squash merge, linear history. A direct push to `main`
is rejected.

**Coverage runs against a `fail_under = 95` floor** (the suite sits around
96–97%). The floor is deliberately a point under the suite, so a change that
costs a full point is loud and ordinary churn is not. Local and CI numbers
agree to within a point these days; when they disagree, judge a coverage
change by CI's number — a populated `data/` directory means the "no card
pool" fallback branches never execute locally.

The `image` job builds the container and **cannot be run locally** on macOS 12
or older, where no container runtime installs — treat a red `image` job as the
first real feedback on a container change rather than as a surprise.

### Documentation changes

`ROADMAP.md`, `docs/ENGINEERING.md` and `docs/HOSTING.md` are kept current
deliberately — read them before proposing direction, and update them when
direction changes. `docs/HISTORY.md` is the opposite and is **not** kept
current: it holds the landed narrative those two files used to carry, moved
out on 2026-08-21 so a live head was not buried under it. Add to it only when
something finishes. `docs/adr/` is different again: **an ADR is immutable once
accepted.** A decision that changes gets a new ADR that supersedes the old one,
and the old one stays.

Write the doc change *when the decision is made*, and commit it on the branch
doing the work it describes. A pull request whose whole diff is prose costs six
CI jobs and a review round trip to land something nobody was blocked on; that
is only worth it for a correction to something already merged and wrong.

## What this project won't do

- No commercial use of any kind. The Fan Content Policy permits noncommercial
  only, and that constraint travels with every fork. See `NOTICE.md`.
- No purchase automation. The shopping tooling prices decks, watches for deals,
  and builds carts. It does not enter payment details and does not check out.
- No scraping of marketplaces. Prices come from Scryfall's feed, and research
  goes through Anthropic's server-side web tooling — which is not a way around
  the scraping ban.
- No rules engine. The play UI manages board state; it does not enforce rules.
  Building a real engine is what took Forge and XMage a decade each.
