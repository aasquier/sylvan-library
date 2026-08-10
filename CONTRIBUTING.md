# Contributing / using this with your playgroup

## First: the two rules

**1. Never commit card data.** `data/` is gitignored. Scryfall's bulk files are
theirs, they're large, and redistributing them is against their guidelines.
`mtglab data refresh` fetches everything in one command.

**2. Never commit anything describing what you physically own.** Collections,
wishlists, purchase history, order confirmations. A public list of expensive
cards attached to a real identity is a targeting list. `.gitignore` blocks the
obvious filenames and CI fails the build if one slips through, but neither is a
substitute for not doing it.

Git history is permanent and forks can't be recalled. If something sensitive
lands on `main`, deleting it in a later commit does not remove it.

## Getting set up

```bash
git clone <repo> && cd mtg-lab
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
mtglab data refresh          # ~15 min, one time
pytest -q
```

## Adding your own decks

Decks live at `decks/<slug>/deck.yaml`. Start from `decks/_template/deck.yaml`.

If several people are keeping decks here, namespace them —
`decks/<yourname>/<slug>/` — so nobody's refactor collides with anybody else's.
If you'd rather keep your lists to yourself, fork the repo instead; the tool
doesn't care where the deck files live.

Every card needs a `category` and a `why`. `mtglab decks validate` fails
without them, on purpose: a card you can't justify is a card to cut.

## The workflow

```bash
mtglab decks validate <slug>          # gate. fix errors first
mtglab sim mana <slug>                # baseline
mtglab sim lands <slug> 30 40         # is the land count right?
git commit -am "before refactor"      # so the swap list has something to diff
# ...edit deck.yaml...
mtglab decks build <slug> --against <(git show HEAD:decks/<slug>/deck.yaml)
```

`swaps.md` is a **git diff**. Commit before you edit or you won't get one.

## Reading the simulator honestly

Tier 1 shuffles, draws, and pays costs. That's all. It doesn't model opponents,
interaction, tutors, cost reduction, or card text beyond mana production. It
answers mana and consistency questions well and answers nothing else. Quote it
with that caveat attached.

For land counts, read **spells deployed through T8**, not commander speed.
Commander speed rises forever with more lands, so optimising it alone tells you
to play 40. Deployment peaks and then falls as flood sets in. That peak is the
answer.

## Code

- Pure stdlib + numpy in the sim core; DuckDB stays behind `cards/db.py`. Keeping
  the core dependency-light is what makes it fast to test.
- Every bug fix gets a test. The mana solver is subtle — `tests/test_mana.py`
  pins the cases where naive source-counting gives the wrong answer.
- `ruff check src tests` before pushing.

## What this project won't do

- No commercial use of any kind. The Fan Content Policy permits noncommercial
  only, and that constraint travels with every fork. See `NOTICE.md`.
- No purchase automation. The shopping tooling prices decks, watches for deals,
  and builds carts. It does not enter payment details and does not check out.
- No scraping of marketplaces. Prices come from Scryfall's feed.
- No rules engine. The play UI manages board state; it does not enforce rules.
  Building a real engine is what took Forge and XMage a decade each.
