"""One cache key, printed, in a fresh interpreter.

`SimCard.produces` holds `frozenset`s and `ManaCost.pips` is a tuple of them.
Serialising a set directly gives an order that varies with `PYTHONHASHSEED`,
which is fixed for the lifetime of a process -- so an in-process test cannot
see the bug at all. It would show up as a cache that misses every time the
server restarts, which is invisible from the outside: the numbers stay right
and the feature simply does not work.

So the key has to be computed in a *fresh* interpreter, twice, at two different
hash seeds. Same shape as `determinism_probe.py`, and for the same reason.

The deck comes from `test_sim_tier1.build_golgari`, like the determinism probe,
so the fixture cannot drift away from the one everything else uses.
"""

from __future__ import annotations

import sys
from pathlib import Path

# Both inserts are needed because this module also runs as a bare script, with
# none of pytest's path handling: one for `mtglab`, one for `test_sim_tier1`.
sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.sim import cache
from mtglab.sim.tier1.engine import KeepRule
from test_sim_tier1 import build_golgari


def probe_key() -> str:
    library, commander = build_golgari(34)
    key = cache.key("sim.mana", library=library, commander=commander,
                    games=1000, turns=10, keep_rule=KeepRule(), seed=11)
    assert key is not None, "the engine fingerprint must be readable here"
    return key


if __name__ == "__main__":
    print(probe_key())
