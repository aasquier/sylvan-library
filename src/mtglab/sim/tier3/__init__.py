"""Tier 3: the bridge to Forge, a real rules engine with a real AI.

Tier 1 goldfishes. Tier 3 plays games -- ADR 14 puts that job on Forge rather
than on anything written here, because a rules engine is a decade of work this
project is not going to repeat.

Forge is a JVM desktop application, not a library and not a service. So the
bridge is four small pieces and no more:

    dck.py        deck.yaml -> Forge's own `.dck` text
    coverage.py   the pre-flight: does Forge implement every card?
    parse.py      `sim -q` output -> a GameResult
    run.py        find the install, build the argv, run it, time it

Nothing here imports DuckDB or the corpus, and nothing imports the CLI. A
`Deck` and a directory containing Forge is the whole input.

**The pre-flight is not optional.** `run.py` refuses to start a simulation it
has not first checked for coverage, because a Forge that silently drops three
cards from a 99 still reports a winner, and that number would be wrong in a way
nothing downstream could detect. CLAUDE.md says so; `run_games` enforces it.
"""

from __future__ import annotations
