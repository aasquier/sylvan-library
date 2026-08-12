# 18. A cached simulation is keyed on its compiled input, not on the deck file

**Status:** Accepted · **Decided:** 2026-08-12 · **Implemented:** 2026-08-12

Refines [ADR 4](0004-two-embedded-databases.md), which already put a
`sim_cache` table in `app.db` and described a sim result as "a pure function of
deck content + parameters". That sentence is close enough to be dangerous, and
this record is what it should have said. It supersedes nothing: the storage
decision stands exactly as ADR 4 made it.

## Context

`docs/ENGINEERING.md` §1 and `docs/HOSTING.md` §3 arrive at the same conclusion
from opposite directions. A 20,000-game `sim mana` is ~18 CPU seconds and an
11-count land sweep is ~4 minutes; deck files change rarely; everybody opens the
same six decks and looks at the same numbers. So caching removes most of the
work outright, which is a bigger win than making the cold path thirty times
faster — and it is the reason both documents give for *not* porting Tier 1 to
Rust. Building the cache is therefore load-bearing for a decision already made.

The requirement that shapes everything below is one sentence: **a cached number
must never be a stale number.** A simulation that is merely slow is an
annoyance. A simulation that confidently reports the numbers for a deck list
that no longer exists is the silent-wrong-answer failure this project's rules
are written against, and it would be indistinguishable from a correct answer on
the screen.

Two facts make the obvious key wrong.

**Card facts come from the corpus, not from the deck file.** `compile_deck`
reads `mana_cost`, `type_line`, `oracle_text` and `produced_mana` out of DuckDB
to build the `SimCard`s the engine runs on. A `data refresh` can therefore
change what a card does while `deck.yaml` is byte-identical. This is not
hypothetical: Scryfall retemplated "enters the battlefield tapped" to "enters
tapped", `sim/compile.py` carries the comment about it, and the old wording
silently treated every modern tapland as untapped — overstating early mana for
every deck in the library. A cache keyed on the deck file would have kept
serving the pre-refresh numbers indefinitely, and the refresh that fixed them
would have looked like it had done nothing.

**The app was not seeding its runs at all.** `POST /api/sim/mana` accepted an
optional seed, the Simulator screen never sent one, and `run()` fell through to
`random.Random(None)`. So the numbers on a deck page were a fresh sample every
time — reproducible by nobody, comparable with nothing, and not a function of
anything a key could name. Caching a *sample* means first deciding which
sample, and that decision had never been made.

## Options considered

**Key on a hash of `deck.yaml`.** The obvious reading of ADR 4, and cheapest on
the hit path: no corpus query at all. Rejected on the corpus argument above. It
is also coarse in the other direction — editing a `why`, which cannot reach a
simulation, would throw away every stored result for that deck.

**Key on `deck.yaml` plus a corpus fingerprint** (the DuckDB file's size and
mtime). Fixes the correctness hole and keeps the hit path free of the corpus.
Rejected because it asks to be trusted rather than checked: mtime is a proxy,
and the failure mode when a proxy lies is a wrong number rather than a slow one.
It is also indiscriminate — every refresh invalidates every deck, including the
decks whose cards did not change.

**Key on the compiled `SimCard`s.** Chosen. The key is a hash of the exact list
the engine is handed, so it is the input rather than a name for the input.

**Cache only inside the process.** Rejected outright: Fly scales to zero, so the
machine sleeps between visits and nearly every real page view would be a cold
miss. It would optimise the case that is already fast.

**A third store, `/data/sim_cache.db`.** Genuinely tempting, because the rows
are *derived* and `app.db` is the irreplaceable half of ADR 4 — mixing them
muddies the backup story. Rejected: the rows are keyed to decks that live on the
same volume and must survive a deploy the same way those decks do, they are
kilobytes, and a second SQLite file means a second copy of the pragmas, the
migration ladder and the backup question. The lifecycle difference is recorded
in the migration instead, along with the fact that dropping the table is safe.

**Cache unseeded runs under a `seed: null` key.** Rejected in favour of seeding
by default. Freezing a nondeterministic sample and serving it back is defensible
— at 20,000 games the seed-to-seed difference is noise — but it means "run it
again" silently stops resampling. Seeding by default gets the same cache hit and
makes the numbers reproducible, which is what the determinism work in
`docs/ENGINEERING.md` §2 was for.

## Decision

**A cached Tier 1 result is keyed on a SHA-256 over: the canonically serialised
compiled deck (`library` and `commander`), the clamped run parameters, the seed,
a fingerprint of `engine.py` and `mana.py`'s own source, and a `SIM_VERSION`
constant.** Stored in `app.db`'s `sim_cache` table, per ADR 4.

Four consequences of that sentence are the decision rather than the
implementation:

1. **Every run is seeded.** An absent seed resolves to `DEFAULT_SEED`, and the
   seed is reported in the result. A different sample is an explicit request —
   the **New sample** button — not what happens by accident.
2. **The engine's source is part of the key.** A change to `engine.py` or
   `mana.py` invalidates every stored result, including a change nobody
   remembered to declare. If the sources cannot be read, caching switches off
   rather than falling back to something weaker: the fallback for "I cannot tell
   which engine this is" must be to compute.
3. **Land sweeps cache per count.** Each count is an independent seeded `run()`
   over a deck derived deterministically from the same compiled cards, so a
   sweep of 32–42 reuses nine of the eleven rows a 30–40 sweep left behind. A
   whole-sweep key would have reused none.
4. **A hit is a job that was born finished.** `POST /api/sim/mana` returns the
   same shape it always did, with `status: "done"` and the result already
   attached. The alternative — a second response shape meaning "no job, here is
   the answer" — was rejected because every client would need a branch and a hit
   would leave no trace in `/api/jobs`. The claim is not "this is not a job"; it
   is "this job took no time".

## Consequences

- **A hit is milliseconds, not zero.** Computing the key means compiling the
  deck: a parse and one indexed `get_cards` over ~100 names. Against eighteen
  seconds that is the right trade, and it is the price of the key being the
  input.
- **A rationale edit no longer costs a re-simulation**, and a corpus refresh
  only invalidates the decks whose cards actually moved. Both fall out of the
  key rather than being features anybody wrote.
- **`sim/` imports `auth/db.py`.** That module is the `app.db` connection
  helper, not an authentication dependency, and a second implementation of the
  pragmas and the migration ladder for the same file would be worse. Nothing in
  `sim/cache.py` imports FastAPI, DuckDB or anything outside the standard
  library, so the dependency rule for `sim/` holds.
- **The cache is global rather than per-user.** Two callers share an entry only
  when the compiled decks, parameters and seed are identical, and the stored
  result says nothing about who produced it. The residual signal — that a fast
  response means somebody has run this exact simulation before — requires
  already holding the identical deck to observe, at which point the numbers were
  computable anyway.
- **Concurrent identical requests both miss**, and the second recomputes. The
  job pool has one worker so they serialise rather than compete, and a lock held
  across a thirty-second job is a worse problem than the one it solves.
- **It never raises.** Every entry point in `sim/cache.py` swallows its own
  errors and behaves as a miss, because an optimisation that can turn a working
  simulation into a failed one is a bad trade.
- **`SIM_VERSION` is pinned against `test_determinism.REFERENCE_DIGEST` as a
  pair.** Changing what Tier 1 reports already requires updating that digest
  deliberately; the paired assertion means the same act forces a decision about
  whether stored results are still valid.
- **It is now cheap to precompute the standard sims when a deck is saved**
  (`docs/HOSTING.md` §3, item 2), which is one call on the write path.
  Deliberately not built: warming a cache for a deck nobody may open is
  speculative work.
