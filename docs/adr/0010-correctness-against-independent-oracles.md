# 10. Correctness is established against independent oracles, not against the implementation

**Status:** Accepted · **Decided:** 2026-08-10 · **Recorded:** 2026-08-10

## Context

`mana.py` answers "can this pool of sources pay this cost?" with Kuhn's
augmenting-path matching. It is the most correctness-critical function in the
project: every castability question, every simulated turn and every land-count
recommendation runs through it. It is also subtle — `tests/test_mana.py` exists
specifically to pin cases where naive source-counting gives the wrong answer,
such as a W/U dual that satisfies "I have a white source" and "I have a blue
source" but cannot pay {W}{U}.

Example-based tests only cover what somebody thought of. The failure mode to
worry about is a clever algorithm that is subtly wrong in a case nobody imagined
— and it produces a *confident, plausible number*, not a crash.

## Options considered

**More examples.** Necessary but insufficient, for the reason above.

**Property tests against a second copy of the same algorithm.** Rejected. Two
implementations of Kuhn's can be wrong in the same way, and often are, because
the second one gets written by reading the first.

**Property tests against an independent reference.** Chosen — and against *two*,
derived differently.

## Decision

**Three routes to the same answer, sharing no code.**

- `mtglab.mana.can_pay` — Kuhn's matching. The implementation.
- `brute_force_can_pay` — enumerate every injective assignment of pips to mana
  units. Factorial, and nothing in it to get wrong beyond the definition.
- `hall_can_pay` — Hall's marriage theorem. A different theorem, so it fails
  differently rather than sharing a blind spot with the search.

The references live in `tests/mana_oracle.py` and **deliberately do not import
from `mana.py`**, down to reimplementing the four-line unit expansion: a
reference that shares code with the implementation cannot catch a bug in the
part they share.

**The corpus is enumerated, not sampled.** Hypothesis generates cases each run,
which is what finds the unimagined ones; alongside it sits an exhaustive
enumeration of 13,944 `(cost, pool)` pairs over a small alphabet, built with
`combinations_with_replacement`. It yields the same cases in the same order on
any machine in any language, which a seeded generator does not — that is what
makes it usable as the differential corpus for a port
([ADR 3](0003-tier-1-stays-python.md)) and what gives CI deterministic coverage
while its Hypothesis profile is derandomised.

**Seeded output is pinned.** Two digests are goldens: the solver's answers over
the whole corpus, and a fixed Tier 1 run (verified byte-identical on CPython
3.11 and 3.12, on macOS and Linux). Neither proves correctness — the oracles do
that — they make a change in behaviour impossible to make by accident.

## Consequences

- The solver came out clean on every generated case and all 13,944 corpus cases.
  That is a result worth having: it means the subtle part was not the problem.
- **The parser was not clean.** Phyrexian mana was dropped outright, so `{U/P}`
  parsed to mana value 0 with no colours where Scryfall says cmc 1 and blue, and
  the curve filed two Tivit cards in the wrong buckets. The bug was in the boring
  code feeding the clever code — which is where this kind of bug usually is.
- `mana.py` was at **100% line coverage** with that branch covered, by a test
  asserting the half of the behaviour that was right. Coverage says a line ran;
  it does not say a test would have noticed. That is the argument for mutation
  testing, which is the open item in `docs/ENGINEERING.md` §2.
- `can_pay` and `engine._consume` are pinned against each other. The second
  solver exists because casting needs the leftovers rather than a yes/no, and two
  implementations of one predicate drift apart silently. This is the differential
  testing a compiled port would need, available today without a second language.
- Cost: the property suite is ~35 seconds in CI, per Python version. Proportionate
  for the function every number in the project depends on.
