# 27. Entomb is the delete, and the graveyard is the undo

**Status:** Accepted · **Decided:** 2026-08-15 · **Recorded:** 2026-08-15 ·
**Implemented:** 2026-08-15

Refines [ADR 12](0012-decks-are-edited-by-surgical-operations.md), whose
`remove_card` was the one operation with no way back, and the delete-a-deck
decision from PR #35, which gave the *deck* a typed confirmation and a trash
directory while a *card* still died on one unconfirmed click.

## Context

Driving the deployed instance on 2026-08-15, the maintainer deleted a handful
of cards from a curated deck without meaning to and without being asked
anything. Three separate failures compounded:

1. **The remove button fired on one click.** No confirmation, no undo. The
   deck-level delete has demanded a typed word since branch 1; the card-level
   delete had nothing, and a card is deleted far more often.
2. **It did not look like a live control.** Muted text with no border and no
   hover state is the universal *disabled* idiom, so the buttons that worked
   read as buttons that would not — and the one that read weakest was the
   destructive one.
3. **Deployed, the loss is unrecorded.** Locally `deck.yaml` is in git and a
   removal is one `git checkout` away. On the instance, `/data/decks` is the
   live source of truth and nothing else records an edit (ADR 23's deploy
   notes; the seed copy in the image is stale the moment anybody edits). A
   mis-click there destroys the `why` — the user's own words, the field this
   whole project exists to keep — with no way back at all.

Separately, the maintainer asked for a bulk delete: reviewing a draft
produces a short list of cuts, and clicking one unconfirmed button 99 times
was both dangerous and tedious.

## Options considered

**A dialog per removal.** Safest, and wrong for the workflow: tuning a deck
removes ten cards in a sitting, and a modal answered the same way ten times
stops being read — the deck-delete dialog's own docstring makes this argument
against yes/no confirmations generally.

**A timed undo toast.** No schema change, but the undo dies with the toast, a
navigation, or a tab close — and "within reason" becomes "within fifteen
seconds", which is exactly when nobody notices a mistake. The Gyome deletions
were noticed well after any toast would have gone.

**A server-side journal in `app.db`.** Durable, but it puts deck state
outside the deck file, which ADR 1 and ADR 4 both forbid in spirit: the deck
file is the source of truth, and an undo buffer only the database knows about
is deck state the file does not show. It also does nothing for the CLI.

**A graveyard in `deck.yaml`** — chosen, with a two-step armed button in
front of it.

## Decision

**Removing a card from the 99 entombs it.** The entry moves, verbatim, to a
`graveyard:` list in `deck.yaml` — category, quantity, overrides and above
all the `why`. Two ways out: **return** puts it back into the 99 exactly as
it left, filed by its category the way `add_card` files one; **exile** drops
it for good. The graveyard is newest-first, written only when occupied, and
removed when emptied, so the six curated decks never grow an empty key.

**The confirmation is a two-step arm, not a dialog.** The entomb button is
red before it is touched (the label alone should not be the only thing saying
what it does), arms to a solid red consequence-naming label on the first
click, disarms on a four-second timeout, and acts only on the second click.
One stray click mutates nothing. Exile gets the same arm — and exile can only
ever act on a card already entombed, so the genuinely permanent delete is two
deliberate operations apart from a living card by construction.

**Bulk entombment is a mode plus the same arm.** Entering selection is the
first deliberate step, the armed button naming the count is the second, and
the server does the batch in **one write, all or nothing** — a name not in
the 99 refuses the whole batch, because a partially applied sweep is a deck
state nobody chose.

**Rule 4 is untouched, and that was checked rather than assumed.** The
graveyard preserves the user's own words; a return restores them without
composing anything. No write path in this feature accepts or produces a
rationale.

**The word is Magic's.** *Entomb* puts a card in the graveyard; *return*
brings it back; *exile* is gone for good. The deck-level delete keeps its
typed `bury` (branch 1, reaffirmed by the maintainer on 2026-08-13) and its
labels change to Entomb — the deck's "graveyard" was always `decks/.trash/`.

## Consequences

- `deck.yaml` gains an optional `graveyard:` list, the third `CardEntry`
  list. The gate, the compiler, the artifacts and the deck's card count all
  ignore it: an entombed card is out of the deck.
- One card, one place: `add_card` refuses a name waiting in the graveyard
  (return it or exile it instead), and `return_card` refuses a name already
  in the 99 or on the swap board.
- A card in the graveyard is frozen — no `why` edits, no quantity changes —
  until it is returned. `_locate_card` deliberately does not search it.
- A returned card is filed with its category, not at its original line. The
  content round-trips byte-identically; the position follows the category
  anchor, which for a file already grouped by category is the same place.
- The swap board keeps plain removal. It is already outside the 99 and is
  its own record of why; burying that record would say less than the board.
- `mtglab decks remove` entombs now, and says so; `decks return` and
  `decks exile` are the CLI's two ways out. A CLI user also has git.
- On the deployed instance, where deck edits have no git history, every
  deletion is now recoverable up to an explicit exile — which was the failure
  that motivated all of this.
