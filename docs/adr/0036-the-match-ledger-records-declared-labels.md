# 36. The match ledger records real games, and the labels it snapshots are declared

**Status:** Accepted · **Decided:** 2026-08-20 with Aaron · Rides on ADR 35
(the Forge plays hosted) and ADR 28 (one call site, and how a deck's history
is keyed); the first branch of the Simulator's next phase, whose sequence is
recorded in `ROADMAP.md`.

## Context

ADR 35 finished the Forge bridge: real Commander games run from the app,
locally or on a worker machine that sleeps between matches, at about a penny
a match hosted and free overnight on the Mac. Every match then evaporated —
the job payload was shaped for one response and discarded, so nothing could
ever answer "how do these two decks actually run against each other over a
hundred games", and everything the next phase wants to build on real games
(rating boards with honest uncertainty, a win-probability regression,
game-length survival curves, Tier 2's calibration anchor) had no data to
drink.

The blocking design question was labelling. CLAUDE.md's standing law is that
Forge results are reported **per archetype, never as a single ranking** —
Forge's AI is best with aggro and midrange, poor with control, bad with most
combo, and the measured 8–2 loss of the bracket-5 cEDH deck to a casual
dinosaur list is the permanent counterexample. So a rating board needs a
class to group by. But a class coarse enough for boards is far too coarse to
say what a deck *is*: EDHREC maintains roughly four thousand themes, and
Aaron ruled (2026-08-20) that a single small archetype set is not a
sufficient identity vocabulary. One axis was being asked two questions.

## Options considered

**One label axis, sized somewhere between the two needs.** Every size is
wrong: four buckets cannot say "food" or "aristocrats", and four thousand
buckets over a seven-deck library put one deck on each rating board, where
no grouping holds enough games to mean anything.

**Derive the labels from the list** — classify a deck by its compiled
contents, or by similarity to labelled decks. Rejected outright, and not for
accuracy: derivation *launders*. Whatever signal a classifier reads
correlates with how well Forge pilots the deck, so the boards would end up
grouping decks by the very bias the class exists to contain, while wearing
the neutrality of a computed answer. It also violates the standing scraping
boundary at one remove — the obvious training vocabulary is EDHREC's, and
their themes are derived from their own aggregated data.

**Join labels live at read time** rather than storing them per match.
Rejected because relabelling a deck would silently rewrite the history of
every game it ever played — a rating computed today and tomorrow from the
same rows would disagree without a single new game.

## Decision

**1. Two label axes on the deck, both declared in `deck.yaml`, never
derived.** An open `themes` list (`model.THEMES`) is the identity axis —
rich, several per deck, what users will see and filter by; the vocabulary is
hand-curated and grows only by editing the tuple, which is a decision
somebody makes, never a scrape. A closed `archetype` class
(`model.ARCHETYPES`: aggro, midrange, control, combo — ordered as Forge
pilots them, best first) is the grouping axis that **only** the rating
boards use; it is coarse on purpose, because the grouping must hold enough
games to mean anything. The gate warns on a label outside its vocabulary;
the edit paths refuse one outright — the same bargain categories already
strike.

**2. Migration 11 adds the ledger: `forge_matches`, `forge_seats`,
`forge_games`.** Real tables rather than JSON blobs, because these rows are
the dataset the boards `GROUP BY` — a shape SQL should see. A match is one
JVM run (seed, clock, Forge version, hosted-or-local, wall clock); a seat is
a deck's part in it, keyed like the activity log (`owner_id` + slug, NULL
for the file tier — never the URL's owner segment); a game is one measured
outcome.

**3. Games store facts as parsed; readers apply the rules.** `winner_seat`
is kept even beside `timed_out`, exactly as `parse.GameResult` keeps them
apart — a clocked-out game with a winner line is a real measurement of a
fake outcome. The clock-out rule ("counts for nobody") lives in the readers,
with `ledger.recent` as the reference implementation, so the rule can be
re-examined someday without the data having pre-judged it.

**4. Seats snapshot the labels the deck wore when it played.** `archetype`
and `themes` are copied at match time. Relabelling a deck changes its next
match, never its history.

**5. Recording happens where the results land, from the two places a match
finishes.** `ledger.record` is called by the API job and by `mtglab sim
forge` — the worker machine never records; its rootfs is ephemeral scratch
and results cross the wire before they are written. `record` never raises
(the match is already played and paid for; a ledger failure is a logged
warning, as in `decks/log.py` and `sim/cache.py`), and reading raises,
because a silently empty history is worse than an error.

## Consequences

- Every Forge match from now on feeds the ratings, the regression, the
  survival curves and Tier 2's calibration anchor — an overnight Mac
  round-robin accumulates in the same table a hosted match does.
- The rows are **not** derived data: a Forge game is a real event that
  cannot be recomputed. Losing the ledger costs history and ratings, never
  anybody's words — a backup rule between `sim_cache`'s "just CPU" and the
  accounts' "irreplaceable".
- `SimRun` carries `forge_version` and the worker wire carries it optionally
  in both directions, so deploy skew degrades to "not reported" rather than
  an error — and ratings can refuse to mix two judges.
- The boards themselves are **not** built here. When they are, they group by
  the snapshotted class and never across — and a deck with no declared
  archetype sits on no board at all, which is the honest reading of
  "undeclared".
- An unseeded CLI match records `seed` as NULL: the ledger says "not
  reproducible" rather than inventing a number.
