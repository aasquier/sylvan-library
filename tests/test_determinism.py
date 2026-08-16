"""A seeded simulation must produce identical output every time it runs.

This is the precondition for everything else in docs/ENGINEERING.md 1 and 2: a
differential test between two implementations means nothing if the reference
implementation does not agree with itself, and a land-count sweep is only a
fair comparison if every point in it starts from the same state.

Three levels, weakest first:

  * within one process -- catches leftover state between runs;
  * across processes with different `PYTHONHASHSEED` -- catches results that
    depend on set or dict iteration order, which is the failure mode that looks
    perfectly reproducible until it runs somewhere else;
  * against a pinned digest -- catches a change in simulator behaviour, on any
    platform or Python version, and forces it to be a decision.
"""

import os
import random
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from determinism_probe import GAMES, SEED, SWEEP_COUNTS, TURNS, reference_digest
from mtglab.sim.tier1.engine import run, simulate_game, sweep_land_counts
from test_sim_tier1 import build_golgari

REPO_ROOT = Path(__file__).resolve().parents[1]
PROBE = REPO_ROOT / "tests" / "determinism_probe.py"
ORACLE = REPO_ROOT / "tests" / "mana_oracle.py"

# The digest of `determinism_probe.reference_outputs()`. Verified identical on
# CPython 3.11.15 and 3.12.13. This is a golden: a change here is a change in
# what the simulator reports, which is a decision, not a detail. Regenerate
# deliberately and say what moved and why:
#     python tests/determinism_probe.py
REFERENCE_DIGEST = "c3e278e3e09ae7766b145886bddf7e07314533c292b6c5aeb9340c73b3ee22d4"


def _digest_from_subprocess(script: Path, *args: str, hash_seed: str) -> str:
    """Run `script` in a fresh interpreter with a chosen hash seed.

    Hash randomisation is fixed when the process starts, so this cannot be done
    in-process -- which is the whole reason the probe is a script.
    """
    env = {**os.environ, "PYTHONHASHSEED": hash_seed}
    proc = subprocess.run(
        [sys.executable, str(script), *args],
        capture_output=True, text=True, env=env, cwd=REPO_ROOT, check=True,
    )
    return proc.stdout.strip()


# ------------------------------------------------------ within one process

def test_a_seeded_run_repeats_exactly():
    library, commander = build_golgari(34)
    first = run(library, commander, games=400, turns=8, seed=99)
    second = run(library, commander, games=400, turns=8, seed=99)
    assert first == second


def test_a_seeded_game_repeats_exactly():
    library, commander = build_golgari(34)
    first = simulate_game(library, commander, turns=10, rng=random.Random(7))
    second = simulate_game(library, commander, turns=10, rng=random.Random(7))
    assert first == second


def test_different_seeds_produce_different_results():
    """The mirror image, and the one that catches a seed that is accepted and
    then ignored -- which would make every test above pass for the wrong
    reason."""
    library, commander = build_golgari(34)
    assert run(library, commander, games=400, turns=8, seed=1) != run(
        library, commander, games=400, turns=8, seed=2
    )


def test_a_run_does_not_mutate_the_library_it_was_given():
    """`simulate_game` shuffles, draws and resolves land fetches. If any of
    that reached the caller's list, the second point of a sweep would be
    simulating a different deck than the first."""
    library, commander = build_golgari(34)
    before = list(library)
    run(library, commander, games=200, turns=8, seed=3)
    assert library == before


def test_the_progress_callback_does_not_change_the_result():
    """Instrumentation must be inert. It consumes no randomness and touches no
    state, and this is what keeps that true."""
    library, commander = build_golgari(34)
    seen: list[tuple[int, int]] = []
    plain = run(library, commander, games=400, turns=8, seed=5)
    watched = run(library, commander, games=400, turns=8, seed=5,
                  progress=lambda done, total: seen.append((done, total)))
    assert plain == watched
    assert seen and seen[-1] == (400, 400)


def test_every_sweep_point_starts_from_the_same_seed():
    """What makes a land-count sweep a comparison rather than a collection of
    unrelated samples: each point sees the same stream of shuffles, so a
    difference in the output is a difference in the deck."""
    counts = [30, 34]
    swept = sweep_land_counts(build_golgari, counts, games=300, turns=8, seed=13)
    for n, summary in swept:
        library, commander = build_golgari(n)
        assert summary == run(library, commander, games=300, turns=8, seed=13)


# -------------------------------------------------------- across processes

def test_simulation_does_not_depend_on_hash_randomisation():
    """Set and dict iteration order varies with PYTHONHASHSEED. A simulator
    that reads it produces a different answer on someone else's machine while
    looking perfectly reproducible on yours."""
    first = _digest_from_subprocess(PROBE, hash_seed="0")
    second = _digest_from_subprocess(PROBE, hash_seed="524287")
    assert first == second


def test_castability_does_not_depend_on_hash_randomisation():
    """Same guarantee for the mana solver, which is full of frozensets."""
    first = _digest_from_subprocess(ORACLE, "--digest", hash_seed="0")
    second = _digest_from_subprocess(ORACLE, "--digest", hash_seed="524287")
    assert first == second


# --------------------------------------------------------- against the pin

def test_the_reference_run_matches_its_pinned_digest():
    assert reference_digest() == REFERENCE_DIGEST, (
        "Tier 1 output changed. Every number in a generated primer moves with "
        "it. If that was deliberate, regenerate with "
        "`python tests/determinism_probe.py`, update REFERENCE_DIGEST, and say "
        "in the commit message what changed."
    )


def test_the_reference_run_is_the_shape_the_digest_assumes():
    """A digest is opaque. If the probe silently stopped simulating anything,
    the digest would still be stable and still be worthless."""
    library, commander = build_golgari(34)
    summary = run(library, commander, games=GAMES, turns=TURNS, seed=SEED)
    assert summary.games == GAMES
    assert len(summary.avg_spells_by_turn) == TURNS
    assert summary.median_commander_turn is not None
    assert len(SWEEP_COUNTS) >= 2


if __name__ == "__main__":
    failures = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"  PASS  {name}")
            except AssertionError as exc:
                failures += 1
                print(f"  FAIL  {name}: {exc}")
    print(f"\n{failures} failure(s)")
    sys.exit(1 if failures else 0)
