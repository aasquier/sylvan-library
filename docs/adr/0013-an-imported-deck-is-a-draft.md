# 13. An imported deck is a draft until every card is justified

**Status:** Accepted · **Recorded:** 2026-08-11 · **Implemented:** 2026-08-11

## Context

You can analyse a deck, simulate it, gate it and now edit it. You cannot create
one, and you cannot bring one in. `ROADMAP.md` has said "generating a deck from
scratch is not started" since the beginning, and import has never been planned
at all — the six curated decks were migrated by hand, once.

That blocks the hosted instance. `docs/HOSTING.md` §6 step 6 is "user decks in
`user_decks`, reusing the existing YAML parser, gate and artifact generator",
which assumes creation exists. A logged-in friend today would get an empty
library and no way to fill it.

Import collides head-on with **rule 4: every card carries a `why`, and
validation fails without one.** Paste a 99-card list and you have 99 cards with
no rationale — a deck that cannot pass the gate until someone writes 99
justifications. Rule 4 is one of the most opinionated things about this project
and the reason its decks are documented at all. It is also, applied naively to
import, a wall nobody climbs.

## Options considered

**Generate a `why` on import.** Rejected outright, and it is worth being blunt
about why: a rationale written by the tool is exactly the empty justification
rule 4 exists to prevent. It would make every deck pass the gate and mean
nothing. [ADR 11](0011-the-api-may-apply-a-swap.md) already refuses this for a
single swap; doing it 99 times at once is worse, not better.

**Make `why` optional; downgrade it to a warning everywhere.** Rejected. It
erodes the rule for the decks that currently satisfy it, to solve a problem
only imported decks have. The curated six would silently become degradable.

**Require every `why` up front — import refuses an unannotated list.**
Rejected. It makes import useless for its main purpose, which is getting a list
you already play into the tool so you can start working on it.

**Import produces a draft; the gate reports missing rationales as warnings
until the deck is promoted.** Chosen.

## Decision

A deck carries a lifecycle stage alongside its physical status:

```yaml
status: built        # do the cards exist   -- built | theoretical
stage:  draft        # has it been reasoned -- draft | curated
```

**These are orthogonal, which is the argument for two fields rather than one.**
All four combinations are real and each means something different:

| | `draft` | `curated` |
| --- | --- | --- |
| **`built`** | A deck you own and play, just imported, not yet written up | Your six, as they are today |
| **`theoretical`** | A netdeck pasted in to simulate before buying | Goreclaw and Tivit — a plan you have reasoned through |

Behaviour follows from the stage:

- **Draft.** A missing `why` is a **warning**, not an error. Everything else the
  gate checks — legality, colour identity, singleton, deck size, companion and
  partner rules — stays an error. Those are facts about cards and they do not
  become negotiable because a deck is new.
- **Curated.** A missing `why` is an error, exactly as today.
- **Promotion is mechanical, not a claim.** `stage: curated` is only accepted
  when every card has a rationale; the gate rejects the deck otherwise. You
  cannot declare a deck curated and skip the work.
- **Artifacts require `curated`.** The five documents are the shareable surface,
  and a primer for a deck nobody has reasoned about is worse than no primer —
  the same argument as [ADR 8](0008-the-gate-blocks.md). `decks build` refuses
  on a draft and says which cards are missing a `why`.
- **Absent `stage` means `curated`.** The opposite default from `status`, and
  for the same reason: the six existing decks have a rationale on every card and
  must not be silently demoted. New decks are created as drafts explicitly.

**What import does and does not infer.** Names resolve against the corpus, which
already handles double-faced cards by face name. Unknown names are *reported*,
never guessed — rule 1. Category is inferred **only** for lands, because
`is_land` is a corpus fact and the gate already cross-checks it; everything else
lands in the model's `utility` default for a human to file. Guessing that a card
is "ramp" or "interaction" would put an invented claim into a generated primer,
which is the failure mode this project was built to stop.

## Consequences

- Import becomes useful without rule 4 becoming decorative. You get a deck that
  validates its *facts* immediately — an illegal card is still an error on day
  one — and a visible, countable list of the thinking still owed.
- **The draft state is a to-do list with a number on it.** "17 cards still need
  a rationale" is a better prompt than a wall of red, and promotion is a real
  milestone rather than a formality.
- Two state fields is more surface than one, and someone will eventually set
  them inconsistently. The 2×2 above is the defence: if a combination ever stops
  being meaningful, collapse the fields rather than keeping a field nobody sets
  honestly.
- The gate's report grows a mode. `validate()` must take the stage into account,
  which is one more branch in the most important function in the project — and
  the reason the stage lives in the deck file rather than being passed in per
  call, so there is one answer to "what is this deck" rather than a caller's
  opinion.
- Artifacts gated on `curated` means a hosted user cannot generate a primer
  until they have done the work. That is intended, and it is also the strongest
  argument the tool makes for its own premise.
- Built on 2026-08-11, in `decks/decklist.py` (the grammar),
  `decks/importer.py` (resolution), `decks/model.py` and `decks/validate.py`
  (the stage), and the `decks import` command, `POST /api/decks/import` and the
  app's Import page. One thing changed shape in the building: a draft's missing
  rationales are reported as **one counted warning**, not one per card. Ninety-
  nine identical warnings is the wall this ADR set out to replace, and it would
  have buried the banned card that the same run is meant to surface —
  [ADR 8](0008-the-gate-blocks.md) requires warnings to stay rare enough to
  read. The per-card list lives in the deck file and on the deck page.
