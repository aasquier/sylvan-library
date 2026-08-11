# sylvan-library

Local-first Commander toolkit: deck files in git, Monte Carlo simulation,
Scryfall-validated decklists, generated primers.

Python 3.11+ · DuckDB · numpy. The package and CLI are named `mtglab`; the repo
is `sylvan-library`. That mismatch is intentional and not a bug to fix.

## Setup

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
mtglab data refresh          # ~500MB from Scryfall, several minutes
pytest -q
```

`data refresh` needs network access to `api.scryfall.com` and
`data.scryfall.io`. In a cloud session with default Trusted network access
those are not reachable — widen the environment's access level first, or run
`--oracle-only` (much smaller, covers everything except pricing). Do not put
`data refresh` in a setup script; it will blow the five-minute budget.

## Architecture

```
src/mtglab/
  config.py               where decks and the corpus live; env-overridable
  mana.py                 cost parsing + castability solver
  cards/db.py             Scryfall bulk -> DuckDB, price history
  decks/model.py          deck.yaml schema
  decks/source.py         DeckSource protocol; file-backed and in-memory
  decks/validate.py       the gate
  decks/companion.py      companion deckbuilding restrictions
  decks/partners.py       Partner / Background / Doctor pairings
  decks/analyze.py        macro category counts vs bracket targets
  sim/compile.py          deck.yaml + corpus -> SimCards
  sim/tier1/engine.py     Monte Carlo goldfish
  artifacts/generate.py   the five deliverables
  api/                    FastAPI app, services, background sim jobs
  web_dist/               built frontend, committed so `mtglab ui` needs no Node
  cli.py
web/                      frontend source (React + Vite)
decks/<slug>/deck.yaml    SOURCE OF TRUTH
decks/<slug>/artifacts/   GENERATED — never edit by hand
```

Layering: `api/` must not import from `cli.py`. Anything both need lives in
`config.py` or the relevant package — that rule is why `deck_paths` and the
deck compiler are where they are.

Deck-facing endpoints never read the filesystem. They take a `DeckSource` from
the request scope (`api/deps.py`), so a second deck tier is one dependency to
swap rather than thirteen handlers to edit. A `DeckSource` is a **locator, not
a connection**: background jobs capture one and outlive the request.

Paths come from `config.py` and honour `MTGLAB_DATA_DIR` and
`MTGLAB_DECKS_DIR`, defaulting to `data/` and `decks/`. Tests point them at a
scratch directory with `config.use_paths()`; never reassign the globals.

Keep `mana.py` and `sim/` dependency-light (stdlib + numpy). DuckDB stays
behind `cards/db.py`. That boundary is what keeps the simulation core fast to
test, and it is why the test suite runs without a database.

## Non-negotiables

**1. Never evaluate a card from memory. Look it up.**

```python
from mtglab.cards import db
con = db.connect('data/mtg.duckdb')
for n, r in db.get_cards(con, ['Arahbo, Roar of the World']).items():
    print(r.name, r.mana_cost, r.type_line, sorted(r.color_identity))
    print(r.oracle_text)
```

This rule exists because of two real errors. *Ajani, Nacatl Pariah* was
proposed for a G/W deck — its back face is R/W, so its color identity is
illegal. And *Arahbo's* {1}{G}{W} doubling ability was described as eminence;
it is not, eminence is only the +3/+3 and the doubling requires him on the
battlefield. Both are checkable facts that were missed by reasoning from
memory. The user reads card text closely and will catch this.

**2. Color identity comes from Scryfall's `color_identity` field**, never
derived from the mana cost. It already accounts for back faces, reminder text,
and land types.

**3. Five artifacts for every new deck or refactor**, no exceptions:
`primer-quick.md`, `primer-advanced.md`, `decklist-annotated.md`,
`moxfield.txt`, and `swaps.md` when anything changed. Generate them with
`mtglab decks build <slug>`. Never hand-write them.

**4. Every card carries a `why`.** Validation fails without one. A card that
cannot justify its slot is a card to cut.

**5. Never commit** card corpus data, collection/wishlist/purchase data, or
credentials. CI enforces this. A public inventory of expensive cards tied to a
real identity is a targeting list.

## Workflow

```bash
mtglab decks validate <slug>      # gate — fix errors before anything else
mtglab sim mana <slug>            # baseline consistency
mtglab sim lands <slug> 30 40     # is the land count right?
git commit -am "before refactor"  # so swaps.md has something to diff
# ...edit deck.yaml...
mtglab decks validate <slug>
mtglab decks build <slug> --against <(git show HEAD:decks/<slug>/deck.yaml)
```

`swaps.md` is a **git diff**. Commit before editing or you won't get one.

## Interpreting simulations

**Tier 1** shuffles, draws, and pays costs. It does not model opponents,
interaction, tutors, cost reduction, or card text beyond mana production.
State that caveat when quoting its numbers.

**Choosing a land count: read "spells deployed through T8", not commander
speed.** Commander speed rises monotonically with land count, so optimising it
alone always recommends more lands. Deployment peaks and then falls as flood
sets in. That peak is the answer.

**Tier 2** (pod simulator, not yet built) is a model of Magic. Right for
bracket placement and matchup matrices, wrong for "is this line correct."

**Tier 3** (Forge bridge, not yet built) runs `forge.jar sim -d ... -f
commander`. Forge's AI is best with aggro and midrange, poor with control, bad
with most combo. The user's decks sit right on that fault line — Dino and Cat
are what Forge plays well; Tivit and Gyome are what it plays badly. **Report
Forge results per archetype with that caveat, never as a single ranking.** Also
required: a pre-flight card-coverage check (Forge does not implement every
card, and silently dropping cards would poison results), a raised `-c` clock
since the 120s default will draw out Tivit games, and draws reported separately
rather than folded into losses.

## Working style

- Ask before large design decisions rather than guessing.
- Prefer surgical trims over mass restructuring. The user pushes back on
  aggressive cut lists and expects each cut argued against the specific slot
  it vacates.
- Deep cuts from old Magic are actively wanted. Query the whole corpus.
- Price is not usually an object, but prefer the cheaper option when a genuine
  functional equivalent exists.
- Reserved List is allowed or forbidden **per deck** — check the deck file.
- Every bug fix gets a test. `mana.py` is subtle; `tests/test_mana.py` pins the
  cases where naive source-counting gives the wrong answer.
- `ruff check src tests` before pushing.

## Landing work

The repo is public and `main` is protected: pull request required, all four CI
checks green, branch up to date, enforced for admins. A direct push to `main`
is rejected — branch first, then open a PR. Squash merge; linear history is
required.

## Planning documents

`ROADMAP.md` (goals vs reality, open decisions), `docs/ENGINEERING.md` (the
next phase: compiled backend, testing rigor, CI/CD) and `docs/HOSTING.md`
(deploying a shared instance). These are kept current deliberately — read them
before proposing direction, and update them when direction changes.

`docs/adr/` records the decisions themselves — context, options considered,
decision, consequences. Unlike the three above, **ADRs are immutable once
accepted**: do not edit a decision, write a new one that supersedes it. Read
`docs/adr/README.md` before arguing for a change to something already decided.

## Out of scope

No purchase automation — the shopping tooling prices decks, watches for deals,
and builds carts, but never enters payment details and never checks out. No
marketplace scraping; prices come from Scryfall. No rules engine — the play UI
manages board state, it does not enforce rules.

## The decks

Six curated Commander decks: Arahbo cats (Selesnya, Kaheera companion, cats
only), Atla Palani dinosaurs (Naya), Goreclaw mono-green stompy (bracket 4),
Tivit (Esper cEDH, bracket 5), Gyome food (Golgari, bracket 4), and Trostani
tokens (Selesnya — an older token deck retooled into this list).

All six are migrated into `decks/<slug>/deck.yaml`, which is now the only
source of truth — the original markdown in `~/Downloads` is historical and
should not be edited or re-imported. `ROADMAP.md` records what the migration
turned up.

Two decks currently fail the gate on one card each, deliberately and not as a
bug to route around: **Goreclaw** runs Primeval Titan and **Atla Palani** runs
Emrakul, the Aeons Torn, both banned in Commander. Picking the replacement is
the user's call.
