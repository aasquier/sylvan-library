---
name: mtg-lab
description: Use for any Magic&#58; the Gathering deckbuilding, tuning, playtesting, simulation, spoiler review, or card-shopping work in this repo. Triggers on requests to build or refactor a Commander deck, analyse a mana base, run Monte Carlo or pod simulations, check color identity, compare decks, scan a new set for inclusions, or produce shopping and swap lists. Also use when the user names one of their decks (cat/Arahbo, dino/Atla, mono-green/Goreclaw, Tivit/cEDH, Gyome/food, Trostani/tokens).
---

# mtg-lab

Local toolkit for Commander deckbuilding, simulation, and shopping.

## Non-negotiables

**1. Verify before asserting. Never evaluate a card from memory.**
Every card mentioned in any output must be looked up first:

```bash
mtglab cards show 'Arahbo, Roar of the World'
```

(From `go/` in a checkout that has not installed the binary:
`go run ./cmd/mtglab cards show '<name>'` — cost, types, color identity and
oracle text, straight from the pool. Several names may be passed at once;
a name the pool lacks is a refusal that names it.)

This rule exists because of two real errors:
- *Ajani, Nacatl Pariah* was proposed for a G/W deck. Its back face is R/W, so
  its color identity is illegal. Caught by the user, not by us.
- *Arahbo's* {1}{G}{W} doubling ability was described as eminence. It is not —
  eminence is only the +3/+3. The doubling needs Arahbo on the battlefield.

The user reads card text closely and will catch this. Look it up.

**2. Color identity comes from Scryfall's `color_identity` field**, never from
the mana cost. It already accounts for back faces, reminder text, and land
types. `mtglab decks validate` enforces this — run it.

**3. Five artifacts, every time.** New deck or refactor, no exceptions:
`primer-quick.md`, `primer-advanced.md`, `decklist-annotated.md`,
`moxfield.txt`, and `swaps.md` when anything changed. Generate them with
`mtglab decks build <slug>` — never hand-write them.

**4. Every card carries a `why`.** Validation fails without it. A card that
cannot justify its slot is a card to cut.

**5. Don't hand back a cut list without surgical reasoning.** The user pushes
back on mass restructuring and prefers targeted trims. Explain each cut against
the specific slot it vacates.

## Workflow: refactoring a deck

```bash
mtglab data refresh                  # once a day at most
mtglab decks validate gyome-food     # gate — fix errors before proceeding
mtglab sim mana gyome-food           # baseline consistency
mtglab sim lands gyome-food 30 40    # is the land count right?
mtglab decks build gyome-food        # snapshot BEFORE editing (stashes the baseline)
# ... edit the deck (UI editor, or the YAML on the instance's volume) ...
mtglab decks validate gyome-food
mtglab decks build gyome-food        # swaps.md diffs against the stashed snapshot
```

Decks are **not in git** (ADR 30) — the deployed volume holds the library,
and `swaps.md` diffs against the previous build's own snapshot. That is why
the rule is **build before editing**: the build is what stashes the
baseline the next build diffs against. Deck history is the activity log
(ADR 28), not git.

## Reading simulation output

Tier 1 answers mana and consistency questions only — it does not model
opponents, interaction, or card text beyond mana production. Say so when
quoting its numbers.

Choosing a land count: look at **spells deployed through T8**, not commander
speed. Commander speed rises monotonically with land count, so optimising it
alone always recommends more lands. The right count is where deployment
plateaus and wasted mana starts climbing.

Tier 3 (Forge headless) is a cross-check, not ground truth: Forge's AI is
competent with aggro and midrange, weak with control, and poor with combo. It
will systematically undersell the Tivit and Gyome decks.

## Deck files

`decks/<slug>/deck.yaml` is the source of truth. Artifacts are derived and
overwritten — never edit them directly. Narrative prose lives under `notes:` in
the deck file so it survives regeneration.

## Shopping

TCGplayer's developer API is closed to new applicants, so pricing comes from
the local Scryfall printings table and deal-detection from `price_history`.
Generate TCGplayer Mass Entry blocks for carts.

**Hard boundary:** never enter payment details, card numbers, or passwords, and
never complete a purchase. Stage the cart and hand it to the user to confirm.

## User preferences

- Price is not usually an object, but prefer the cheaper option when a genuine
  functional equivalent exists.
- Reserved List: allowed or not **per deck** — check the deck file, don't assume.
- Deep cuts from old Magic are wanted, actively. Query the whole pool.
- Ask before large design decisions rather than guessing.
