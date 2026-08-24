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

The app is one Go binary. Go 1.26+ with CGO enabled (the card pool rides
DuckDB, which links a prebuilt static archive):

```bash
git clone https://github.com/aasquier/sylvan-library && cd sylvan-library
cd go && go build -o ../mtglab ./cmd/mtglab && cd ..
./mtglab data refresh        # Scryfall bulk -> DuckDB; ~100MB down, seconds to load
./mtglab ui                  # the app, at http://127.0.0.1:8765
```

`data refresh` needs network access to `api.scryfall.com` and
`data.scryfall.io`. Do not interrupt it mid-load; `--oracle-only` is much
smaller and covers everything except pricing.

The Claude surfaces (the interview, the dossier, research, the tarot table's
reader) want an `ANTHROPIC_API_KEY` in the environment — `mtglab claude check`
proves the key with one real call. Everything else — the gate, the mana
solver, the simulator — needs neither an account nor a network.

### Working on the frontend

You do not need Node to *run* the app — the built bundle is committed at
`web_dist/` ([ADR 9](docs/adr/0009-commit-the-built-frontend-bundle.md)).
Node is only needed to change it:

```bash
npm --prefix web install
npm --prefix web run dev     # Vite on :5173, proxying /api to :8765
./mtglab ui                  # the API alongside it
npm --prefix web run build   # rebuild the committed bundle -- required if you
                             # touched anything under web/src
```

### The media toolbox

The one piece of Python in the tree is `tools/` — the local picture and video
pipeline that makes the site's committed art (`animist`, ADR 29) and the
card-art motion derivatives (`cardmotion`, ADR 32). It runs on a dev machine,
never ships, and never serves; its README covers setup. You only need it to
change the site's imagery.

## Adding your own decks

Decks live at `decks/<slug>/deck.yaml` — your app data, not repository
content ([ADR 30](docs/adr/0030-decks-are-live-app-data.md)): the directory
is gitignored, so your lists stay yours and never ride into a pull request.
Bring a deck in through the app's import screen (paste a decklist, or point
the camera at the cards), or start from
[`docs/deck-template.yaml`](docs/deck-template.yaml) by hand.

Every card needs a `category` and a `why`. Validation fails without them, on
purpose: a card you cannot justify is a card to cut. **Nothing writes that
sentence for you** — not the CLI, not the app, and not the Claude surfaces,
which may interrogate a card's slot and may never author the text
([ADR 8](docs/adr/0008-the-gate-blocks.md),
[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)).

An imported deck starts as a **draft**, where a missing `why` is one counted
warning rather than 99 errors — so the deck's *facts* get checked on day one
while the thinking is still owed. Promotion to **curated** is refused while
any card is blank, and the artifacts refuse a draft outright
([ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md)).

Editing — swaps, categories, rationales, notes, the share toggle — happens on
the deck page, each edit a surgical operation that re-runs the gate
([ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md)) and
lands in the deck's activity log
([ADR 28](docs/adr/0028-the-activity-log-records-edits-and-never-rationales.md)).

## The command reference

The binary carries the shell surface — what a deployed instance's
`fly ssh console` reaches for, and what a local library wants beside the app:

```bash
mtglab ui [--host] [--port]           # serve the app
mtglab data refresh [--oracle-only]   # Scryfall bulk -> DuckDB
mtglab data snapshot                  # append today's prices to history

mtglab decks list
mtglab decks validate <slug>          # the gate. fix errors first
mtglab decks build <slug> [--against path]  # the five artifacts; stashes the
                                      # swap-diff baseline (build BEFORE editing)
mtglab decks log <slug>               # what has been done to it, and by whom

mtglab sim mana <slug>                # Tier 1 goldfish
mtglab sim lands <slug> 30 40         # land-count sweep, flood-aware
mtglab sim shelf <slug>               # Tier 1.5 -- the closed form
mtglab sim mulligan <slug>            # search keep rules
mtglab sim cache [--clear]            # what Tier 1 results are memoised
mtglab sim forge <a> <b> [c] [d]      # Tier 3 -- Forge plays real games
mtglab sim matches                    # the match ledger

mtglab cards show <name>...           # a card's facts, from the pool

mtglab claude check [--tools]         # one real API call -- is the key live?

mtglab users invite <email>           # an account, and a link to claim it
mtglab users add <name> [--admin]     # prompts twice; there is no --password
mtglab users list
mtglab users passwd <name>            # prompts; ends every session
mtglab users disable|enable <name>
mtglab users promote|demote <name>    # admin, and never the last one
mtglab users tier <name> --tier X     # which Claude answers an account
mtglab users delete <name>            # confirm by typing the name
```

The `users` commands are for a **hosted** instance and do nothing to a local
one: authentication is off unless `MTGLAB_REQUIRE_AUTH` is set, so `mtglab ui`
on your own machine has no login and never will.

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

- Never evaluate a card from memory; look it up (`mtglab cards show`). Colour
  identity comes from Scryfall's `color_identity` field, never derived from the
  mana cost — it already accounts for back faces, reminder text and land types
  ([ADR 7](docs/adr/0007-card-facts-come-from-the-corpus.md)).
- Every bug fix gets a test. The mana solver is subtle; `internal/mana`'s
  tables pin the cases where naive source-counting gives the wrong answer.
- A route is protected unless it is named public: the middleware refuses
  before routing, and the door's own sweeps derive from the served route
  table, so a new route is deny-by-default the day it is written.
- Package comments carry the argument, the way a good docstring does.

### Before you push

```bash
cd go
gofmt -l . && go vet ./... && go test -race ./... && golangci-lint run ./...
cd ..
npm --prefix web run check      # tsc, oxlint and vitest in one
npm --prefix web run build      # if anything under web/src changed
```

CI runs all of it. `main` is protected: pull request required, every required
check green, branch up to date, squash merge, linear history. A direct push to
`main` is rejected.

The `image` job builds the container and **cannot be run locally** on macOS 12
or older, where no container runtime installs — treat a red `image` job as the
first real feedback on a container change rather than as a surprise.

### Documentation changes

`ROADMAP.md` and `docs/HOSTING.md` are kept current deliberately — read them
before proposing direction, and update them when direction changes.
`docs/HISTORY.md` is the opposite and is **not** kept current; add to it only
when something finishes. `docs/adr/` is different again: **an ADR is immutable
once accepted.** A decision that changes gets a new ADR that supersedes the
old one, and the old one stays.

Write the doc change *when the decision is made*, and commit it on the branch
doing the work it describes. A pull request whose whole diff is prose costs
every CI job and a review round trip to land something nobody was blocked on;
that is only worth it for a correction to something already merged and wrong.

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
