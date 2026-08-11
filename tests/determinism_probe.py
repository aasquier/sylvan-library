"""One canonical Tier 1 run, reduced to a single hash.

Determinism is only testable against a fixed reference run, and the interesting
failure mode -- iteration order that varies with `PYTHONHASHSEED` -- is fixed
for the lifetime of a process. So the reference has to be runnable in a *fresh*
interpreter, which is why this is a module with a `__main__` rather than a
pytest fixture.

The deck comes from `test_sim_tier1.build_golgari` on purpose. A second builder
here would drift from that one, and then the determinism guarantee would be
about a deck nothing else exercises.
"""

from __future__ import annotations

import hashlib
import random
import sys
from pathlib import Path

# Both inserts are needed because this module also runs as a bare script, with
# none of pytest's path handling: one for `mtglab`, one for `test_sim_tier1`.
sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.sim.tier1.engine import run, simulate_game, sweep_land_counts
from test_sim_tier1 import build_golgari

# Small enough to run twice in a couple of seconds, large enough that any real
# nondeterminism shows up: 300 games is 300 independent shuffles.
GAMES = 300
TURNS = 10
SEED = 20260810
SWEEP_COUNTS = (32, 35, 38)


def reference_outputs() -> list[str]:
    """Every number the reference run produces, as text, in a fixed order."""
    library, commander = build_golgari(34)

    single = simulate_game(library, commander, turns=TURNS,
                           rng=random.Random(SEED))
    summary = run(library, commander, games=GAMES, turns=TURNS, seed=SEED)
    sweep = sweep_land_counts(build_golgari, SWEEP_COUNTS,
                              games=GAMES // 3, turns=TURNS, seed=SEED)

    out = [repr(single), repr(summary)]
    out += [f"{n}:{s!r}" for n, s in sweep]
    return out


def reference_digest() -> str:
    digest = hashlib.sha256()
    for line in reference_outputs():
        digest.update(line.encode())
        digest.update(b"\n")
    return digest.hexdigest()


if __name__ == "__main__":
    print(reference_digest())
