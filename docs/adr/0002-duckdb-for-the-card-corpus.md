# 2. DuckDB for the card corpus

**Status:** Accepted · **Decided:** when the corpus was first loaded · **Recorded:** 2026-08-10

## Context

One of the project's goals is "deep hits from the entire history of Magic".
That is a query over ~35k oracle cards, not a recall exercise — and the whole
premise of [ADR 7](0007-card-facts-come-from-the-corpus.md) is that card facts
must be looked up rather than remembered. So the corpus has to be local, and it
has to be queryable in the shapes deckbuilding actually asks for:

- *every card whose colour identity fits inside {G}{W}* — a set-containment
  test against an array column;
- *every green card whose oracle text mentions "landfall"* — a text scan;
- *what did this printing cost last month* — an aggregate over an append-only
  daily snapshot table.

Scale: 35k oracle rows, 107k printings with prices, and `price_history` growing
by a snapshot per refresh. About 63 MB on disk.

## Options considered

**Query Scryfall's API card by card.** Rejected. It puts a network round trip
and a rate limit between the tool and every question, it makes the tool useless
offline, and asking Scryfall 35k questions to answer one of ours is impolite
when they publish the whole thing as a file. Bulk data is explicitly licensed
for this; the per-card API is not the way to consume it.

**SQLite.** A genuine contender, and the right engine for the *other* store
(see [ADR 4](0004-two-embedded-databases.md)). Rejected for the corpus on one
concrete point: **no native array type.** `color_identity` and `produced_mana`
are arrays, and the identity filter the card search actually runs is

```sql
len(list_filter(color_identity, x -> x NOT IN ('G','W'))) = 0
```

— a lambda over an array column (`api/service.py`). In SQLite that becomes JSON
strings plus correlated subqueries, or a junction table and a `GROUP BY … HAVING`
per query. Both work; both are worse at the thing this database exists to do.

**Postgres.** Rejected. A service to run, back up, connect to and keep a
password for, in exchange for durability guarantees that read-mostly, publicly
regenerable data does not need. It is also the wrong shape for a local-first
tool: `pip install` and one command should be the whole setup.

## Decision

DuckDB, one file at `data/mtg.duckdb`, built from Scryfall bulk by
`mtglab data refresh`. **All DuckDB access lives behind `cards/db.py`.** No
other module imports duckdb, and `connect()` imports it lazily.

## Consequences

- Colour-identity, legality and best-in-slot questions are *verified* rather
  than remembered, which is the whole point.
- The boundary keeps `mana.py` and `sim/` dependency-light, and it is why the
  **test suite runs without a database at all** — 250 tests, no corpus needed.
  That property is also what would make a compiled port cheap; see
  [ADR 3](0003-tier-1-stays-python.md).
- The corpus is gitignored and never committed. See
  [ADR 6](0006-never-redistribute-scryfall-bulk-data.md).
- **DuckDB's write lock is held per process, not per connection, and there is
  exactly one writer.** Reads are fine, including across processes — verified
  with four concurrent readers. But while `mtglab data refresh` runs, nothing
  else can open the corpus read-write, so `service._connect()` deliberately
  swallows the failure and reports "no corpus" instead of returning 500s. Card
  lookups being briefly unavailable during a refresh is by design.
- Sim pool workers must be handed already-compiled `SimCard` objects and must
  never open the corpus themselves, for the same reason.
