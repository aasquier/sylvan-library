# 28. The activity log records edits, and never rationales

**Status:** Accepted · **Decided:** 2026-08-16 · **Recorded:** 2026-08-16 ·
**Implemented:** 2026-08-16

Builds the log [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md)
lists as a prerequisite for deck conversation's write autonomy. Sits beside
[ADR 27](0027-entomb-is-the-delete-and-the-graveyard-is-the-undo.md), which
rejected a server-side journal for a different job, and beside
[ADR 22](0022-decks-have-owners-and-sharing-is-a-flag.md), whose two deck
tiers are why "which deck" is not answered by a slug.

## Context

ADR 15 tabulated **deck conversation** — a mode that may make reversible edits
at the top stance — and listed four things that had to exist before it could
be built. Three are done. The fourth is this: *"what did it change while I was
not looking"* is a question a log answers and nothing else here can.

The gap is not hypothetical and it is not only about Claude. Two facts about
the project as it stands:

1. **Only the curated six have a history at all.** They are `deck.yaml` in
   git, and `git log` is their record. Every deck in ADR 22's second tier is a
   row in `app.db`, edited through the app, and the only evidence that a card
   was ever entombed is that it is now in the graveyard. There is no evidence
   at all that a rationale was rewritten.
2. **Deployed, even the curated six lose their history.** `/data/decks` on the
   volume is the live source of truth and nothing commits it. ADR 27's context
   makes this point about *loss*; it is equally true of *change*. An edit made
   through the app on the instance is invisible to git forever.

`service._commit` is the single function every deck write passes through, and
it has always assembled exactly the description such a record wants —
`added=…, category=…, into=…` for one operation, `swapped_out=…,
swapped_in=…` for another — put it in the response, and thrown it away.

## Options considered

**Nothing; rely on git.** It is the honest answer for the file tier on a
laptop and no answer at all for the other two cases above, which are the cases
that matter. It also cannot say *who*, which is the column ADR 15 needs.

**Journal the deck's text, so the log is also an undo.** Rejected, and ADR 27
already made the argument in the other direction: an undo buffer only the
database knows about is deck state the deck file does not show, which ADR 1
and ADR 4 both forbid in spirit. ADR 27's answer to undo is the graveyard, in
`deck.yaml`, and that answer stands. **This log is not an undo and must never
become one** — it records that something happened, not enough to put it back.
That is the line which lets both decisions be right: the graveyard is deck
state and lives in the deck file; a history is an observation about the deck
and does not.

**A file per deck, beside `deck.yaml`.** Keeps everything in one place for the
file tier and has nowhere to go for the SQL tier, which is the tier with no
history today. It would also make an edit two writes that can half-fail.

**One table in `app.db`, written from `_commit`** — chosen.

## Decision

**One row per edit, written from one call site.** `_commit` is where every
deck write already goes, so an edit that is not logged is not something a new
route can produce by forgetting — the same argument the gate's verdict makes
one line above it in the same function. `decks/log.py` renders the row and
`auth/db.py` migration 8 holds it.

**No rationale text ever lands in it.** `describe` builds its sentence out of
card names, categories and field *names*; where `swap_card` hands `_commit`
the `why` the user typed, it is dropped. The log records that a rationale
changed and never what it says. Rule 4's text lives in `deck.yaml`, which is
the source of truth; a second copy in a table nobody edits would go stale, and
would be a place a rationale could be read back out of by something that is
not allowed to write one.

**A deck is `owner_id` plus `slug`, and the file tier is `owner_id IS NULL`.**
Not the owner segment out of the URL: that segment is `local` on a laptop and
the maintainer's username on a deployment, so a history keyed on it would
split in two the day `MTGLAB_ADMIN_EMAIL` was set. NULL says what the file
tier is — there is exactly one of it per instance — and it is what makes
`mtglab decks log` and the deck page read the same rows.

**Who may read one is decided by where the route is mounted, not by a check of
its own.** `GET /api/decks/{owner}/{slug}/log` resolves its source through
`Library`, so a deck the caller cannot see 404s before a row is read. There is
deliberately no second visibility rule to keep in step with the first.

**`record` never raises.** The deck write has already happened by the time it
runs, so an exception would report a failure for work that succeeded. A failed
write is a logged warning — the property `sim/cache.py` and `claude/ledger.py`
both have, for the same file, for the same reason.

**The `actor` column exists now, though nothing autonomous writes yet.** A log
that cannot say who is a log that has to be migrated before it can answer the
question ADR 15 built it for. Today it holds a username, or NULL for whoever
is at the machine — the CLI, and the app with auth off.

**Creation, import and deletion are outside it**, because they are outside
`_commit`. That is the cost of having exactly one call site, and it is
affordable: none of the three is an edit *to* a deck's contents, and each
already leaves a trace elsewhere — a deck's first commit, `decks/.trash/`,
`user_decks.deleted_at`. Adding them means adding a second call site, which is
a decision to take on its own rather than by drift.

## Consequences

- `app.db` gains `deck_log` at schema version 8. Derived-adjacent like
  `sim_cache`: dropping it loses history and nothing a deck needs. It
  cascades on `users`, matching `user_decks` — deleting an account takes its
  decks, so a record of edits to decks that no longer exist, attributed to
  somebody with no account, must not outlive it.
- **A migration applies on boot, unwatched** (ADR 23). This one adds a table
  and touches nothing existing, but the rule stands: land it when somebody can
  watch it.
- Every deck-editing service function takes an `actor`, and every route passes
  `lib.actor`. The CLI passes nothing, which is correct: nobody is signed in
  on a command line.
- The sentence is rendered **once, server-side**, and both readers show it
  verbatim. A client that rendered its own from the parts would be a second
  account of the same edit, drifting.
- Editing a deck on a laptop with auth off now creates `app.db` if it is
  absent, as running a simulation already did. The narrower rule holds
  untouched: *reading* must not acquire a database.
- Tests do not write the developer's history. `conftest.py`'s `_no_deck_log`
  silences `record` for the suite, and the tests that are about the seam put
  the real module back — a stub nothing ever removes is how a broken seam
  stays green.
- Deck conversation's fourth prerequisite is met. The remaining ones are ADR
  15's own list: a stance axis no preset can express, four locks that each
  name the ADR that would have to supersede them, and a sim-results tool
  `service.py` does not have.
