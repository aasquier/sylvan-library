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
  mana.py                 cost parsing + castability solver
  cards/db.py             Scryfall bulk -> DuckDB, price history
  decks/model.py          deck.yaml schema
  decks/validate.py       the gate
  sim/tier1/engine.py     Monte Carlo goldfish
  artifacts/generate.py   the five deliverables
  cli.py
decks/<slug>/deck.yaml    SOURCE OF TRUTH
decks/<slug>/artifacts/   GENERATED — never edit by hand
```

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

## Out of scope

No purchase automation — the shopping tooling prices decks, watches for deals,
and builds carts, but never enters payment details and never checks out. No
marketplace scraping; prices come from Scryfall. No rules engine — the play UI
manages board state, it does not enforce rules.

## The decks

Five curated Commander decks: Arahbo cats (Selesnya, Kaheera companion, cats
only), Atla Palani dinosaurs (Naya), Goreclaw mono-green stompy (bracket 4),
Tivit (Esper cEDH, bracket 5), Gyome food (Golgari, bracket 4). None are
migrated into `deck.yaml` yet — that is Phase 2, starting with Gyome.
