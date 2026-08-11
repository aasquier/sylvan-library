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
  decks/edit.py           surgical deck.yaml edits, minimal diffs
  decks/decklist.py       pasted decklist -> parsed lines; pure text
  decks/importer.py       parsed lines + corpus -> a draft deck.yaml
  decks/source.py         DeckSource protocol; file-backed and in-memory
  decks/suggest.py        similarity scorer -> replacement shortlists
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
web/                      frontend source (React + Vite); `npm test` is Vitest
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
cannot justify its slot is a card to cut. **Never write one on the user's
behalf** — every write path refuses an empty rationale rather than inventing
one, which is what keeps [ADR 8](docs/adr/0008-the-gate-blocks.md) intact now
that the tool can edit decks. See
[ADR 11](docs/adr/0011-the-api-may-apply-a-swap.md). The rationale editor in
the app is the same rule in a UI: the box opens empty, its placeholder is a
question rather than a draft, and a test pins that.

The one bend, and it does not bend the rule: a deck declares a `stage` as well
as a `status`. In a **draft** — what `decks import` writes — a missing `why` is
a single counted warning rather than 99 errors, so the deck's *facts* get
checked on day one while the thinking is still owed. Promotion to **curated** is
refused while any card is blank, and the five artifacts refuse a draft outright.
Absent means `curated`, the opposite default from `status`, so the six existing
decks are never silently demoted. [ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md).

**5. Never commit** card corpus data, collection/wishlist/purchase data, or
credentials. CI enforces this. A public inventory of expensive cards tied to a
real identity is a targeting list.

## Workflow

```bash
mtglab decks import <slug> --from list.txt --commander 'X'   # -> a draft
mtglab decks validate <slug>      # gate — fix errors before anything else
mtglab decks suggest <slug>       # shortlist replacements for what it flagged
mtglab decks swap <slug> --out X --in Y --why '...'   # apply your choice
mtglab sim mana <slug>            # baseline consistency
mtglab sim lands <slug> 30 40     # is the land count right?
git commit -am "before refactor"  # so swaps.md has something to diff
```

Editing, all surgical and self-verifying ([ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md)),
each also a route and a control on the deck page:

```bash
mtglab decks add <slug> --card X --category ramp --why '...'  # corpus-checked
mtglab decks remove <slug> --card X
mtglab decks set <slug> --card X --why '...'        # or --category / --qty
mtglab decks set <slug> --status built              # no --card: a deck field
mtglab decks note <slug> --key mulligan --value '...'
mtglab decks promote <slug>       # draft -> curated, once every card is justified
mtglab decks build <slug> --against <(git show HEAD:decks/<slug>/deck.yaml)
```

`swaps.md` is a **git diff**. Commit before editing or you won't get one.

## Python decides, Claude advises

The split, decided 2026-08-11 and argued in
[ADR 14](docs/adr/0014-python-decides-claude-advises.md): **anything with a
right answer belongs in deterministic Python; Claude is for opinions and
research.** Not built yet — there is no LLM SDK in `pyproject.toml` — but it is
the work in progress, so check before assuming.

[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md) says
what a surface *is*: a **mode** (a system prompt, a tool set, and what it may
write) plus a **stance** (the user's dial over initiative, scope, and write
autonomy). A stance may widen what a mode does, never what it is allowed to do.
Card facts reach a mode through corpus tools rather than recall, which is how
rule 1 below becomes structural instead of a request. Target model is
**Claude Sonnet 5** to begin with — the user's call, not a default to override;
**load the `claude-api` skill before writing any integration code.**

Deterministic Python owns legality, colour identity, singleton, deck size,
companion and partner rules, mana solving, Tier 1, category counts, similarity
and price. Reproducible, tested without a network, no model consulted. Claude
owns conversation about a deck and the questions the corpus cannot answer — the
meta, whether a spoiled card earns a slot, what a ruling means in practice.

Three boundaries, all of which apply to you in this session as much as to
anything built later:

1. **Rule 1 binds Claude too.** Card facts come from the corpus, not from
   recall and not from a web page. Research is for what the corpus lacks —
   discussion, meta, rulings, cards spoiled ahead of the next bulk refresh.
2. **Argue about a `why`; never write one.** Interrogating a card's slot and
   making the case against it is the conversation the curated six came out of.
   Authoring the text that lands in `deck.yaml` is not, and no surface may
   pre-fill that field. Rule 4 above is the rule; this is where its edge is.
   In code the line is **no path passes a model response into
   `set_card_field(field="why")`** — an interview supplies questions, the user
   supplies the words.
3. **Say which system answered.** The gate's output is reproducible and
   checkable; an opinion is neither. Never present one as the other.

## Interpreting simulations

**Tier 1** shuffles, draws, and pays costs. It does not model opponents,
interaction, tutors, cost reduction, or card text beyond mana production.
State that caveat when quoting its numbers.

**Choosing a land count: read "spells deployed through T8", not commander
speed.** Commander speed rises monotonically with land count, so optimising it
alone always recommends more lands. Deployment peaks and then falls as flood
sets in. That peak is the answer.

**Tier 2** (pod simulator, not yet built, and **deferred behind Tier 3** as of
2026-08-11) is a model of Magic. Right for bracket placement and matchup
matrices, wrong for "is this line correct." Forge goes first; Tier 2 gets built
only if Forge cannot answer those questions. See ROADMAP goal 2.

**Tier 3** (Forge bridge, not yet built, and now the next simulator) runs
`forge.jar sim -d ... -f
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
manages board state, it does not enforce rules, and Forge plays the games.
No web crawler either: research goes through Anthropic's server-side web
tooling, which is not a way around the scraping ban.

## The decks

Six curated Commander decks: Arahbo cats (Selesnya, Kaheera companion, cats
only), Atla Palani dinosaurs (Naya), Goreclaw mono-green stompy (bracket 4),
Tivit (Esper cEDH, bracket 5), Gyome food (Golgari, bracket 4), and Trostani
tokens (Selesnya — an older token deck retooled into this list).

All six are migrated into `decks/<slug>/deck.yaml`, which is now the only
source of truth — the original markdown in `~/Downloads` is historical and
should not be edited or re-imported. `ROADMAP.md` records what the migration
turned up.

Each deck declares `status: built | theoretical`. **Goreclaw and Tivit are
theoretical** — lists under consideration, not boxes of cards; the other four
are built. Absent means theoretical, so nothing is ever silently claimed as
owned.

Separately, each declares `stage: draft | curated` — whether it has been
reasoned about, as opposed to whether it exists. **All six are curated.** A
deck brought in with `decks import` starts as a draft; see rule 4.

Two decks currently fail the gate on one card each, deliberately and not as a
bug to route around: **Goreclaw** runs Primeval Titan and **Atla Palani** runs
Emrakul, the Aeons Torn, both banned in Commander. Picking the replacement is
the user's call.
