# 3. Tier 1 stays Python; a compiled port is deferred with a written trigger

**Status:** Accepted · **Decided:** 2026-08-10 · **Recorded:** 2026-08-10

## Context

The instinct on a Monte Carlo simulator is to port the hot loop to a compiled
language. Before doing that, the engine was profiled on the Arahbo list.

| Metric | Measured |
| --- | --- |
| One goldfish game | 0.89 ms |
| 20,000 games (`sim mana`) | ~18 s |
| 11-count sweep at 25,000 games | ~4 min |
| Hot spot | `_consume`, ~21% alone, ~50% including callees |
| Process parallelism | 1.65x at 2 workers, 2.42x at 4, worse at 8 |

One measurement is worth keeping because it contradicts the obvious guess.
`ManaSource.units()` is called over a million times per 2,000 games, which looks
like a memoisation win. `ManaSource` is a frozen hashable dataclass, so it was
memoised and re-measured: **1.00x — no change at all.** The allocations are
cheap; the cost is adjacency-list construction and bipartite matching inside
`_consume`.

## Options considered

**Port Tier 1 to Rust now.** Rejected. It is an 18-second batch job, run a
handful of times a week, on a machine idle ~99% of the time. A 30x speedup saves
seventeen seconds nobody is waiting on, and caching by deck-content hash removes
most of the work outright, because deck files change rarely and the same numbers
get viewed repeatedly. A reviewer will ask what the rewrite bought; "it's
faster" is not an answer when the profile says the workload was never the
bottleneck.

**Port Tier 2 to Rust now.** Rejected, one level removed. Tier 2 does not exist
yet. The 50–100x-work-per-game figure is an extrapolation, not a measurement,
and writing Rust against a guess about a simulator nobody has built is the same
mistake.

**Go instead of Rust, if a port happens.** Rejected for this shape. The hot loop
is a library inside a Python process, not a service. Rust via PyO3 + maturin
keeps one deployable, one process and no serialisation boundary. Go needs either
cgo, which is awkward and gives up much of Go's ergonomics, or a separate
process with IPC — a network hop and a second thing to deploy, in place of a
function call.

## Decision

Stay on Python. Not "not yet, probably soon" — deferred until a measurement says
otherwise. **Reopen when any of these is true, and not before:**

- a single Tier 2 matchup at 10,000 games takes **over 5 minutes** after
  profiling and after the cheap wins below;
- a full six-deck round-robin cannot finish in **under an hour** on the dev
  machine;
- profiling shows a genuine hot loop that is not fixable in Python — time in
  arithmetic and branching, not in allocation, attribute lookup, or an algorithm
  that should be better.

Exhaust these first, in order: cache by deck-content hash; `multiprocessing`
across games; algorithmic work in the policy engine, which is where a pod
simulator's time will actually go; then ordinary Python optimisation
(`__slots__`, lookup tables, no per-game allocation).

If the trigger fires: **Rust, via PyO3 + maturin.**

## Consequences

- The seam is maintained deliberately. CLAUDE.md keeps `mana.py` and `sim/`
  stdlib-plus-numpy precisely so they could move, and on 2026-08-10 the Tier 1
  determinism probe was confirmed to run on a bare CPython 3.11 **with numpy not
  installed**. Tier 1 is pure stdlib today. The rule is holding.
- A port would be differentially testable from day one rather than as an
  afterthought: `mana.py` already has an enumerated corpus of 13,944 cases with
  a pinned answer digest, and Tier 1's seeded output is pinned to a digest
  verified identical on CPython 3.11 and 3.12. See
  [ADR 10](0010-correctness-against-independent-oracles.md).
- If the trigger never fires, that is a good outcome and the extrapolation was
  simply wrong — which is worth recording either way.
- Hand-written assembly stays off the table. There is no kernel here where the
  compiler is demonstrably leaving performance on the floor. The defensible
  low-level story is SIMD-batched RNG *inside a Rust engine that does not exist
  yet*, benchmarked against a scalar version that is kept.
