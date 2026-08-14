"""The simulation cache, and the guards that keep it from lying.

The interesting tests here are not "does a round trip work". They are the ones
that would fail if the cache ever started serving a number that a fresh run
would not reproduce, because that -- not a slow simulation -- is the failure
this feature can actually cause.

Four of them are structural rather than behavioural, and they are the point:

* the key covers every argument `run()` reads, checked against the real
  signature, so a new parameter fails the suite instead of being silently
  excluded from the key;
* the key is stable across processes, checked in a subprocess at two hash
  seeds, because `frozenset` order is not;
* a change to the engine's source changes the key;
* `SIM_VERSION` is pinned against the determinism digest as a pair, so
  a deliberate change to Tier 1's output cannot be recorded without this being
  looked at.
"""

from __future__ import annotations

import inspect
import os
import subprocess
import sys
from dataclasses import dataclass, fields, replace
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from mtglab import config
from mtglab.mana import ManaCost, ManaSource
from mtglab.sim import cache
from mtglab.sim.tier1.engine import KeepRule, SimCard, run
from test_sim_tier1 import build_golgari

REPO_ROOT = Path(__file__).resolve().parents[1]
PROBE = Path(__file__).resolve().parent / "sim_cache_probe.py"


@pytest.fixture
def store(tmp_path):
    """An `app.db` of this test's own, so nothing shares a cache."""
    with config.use_paths(data_dir=tmp_path / "data"):
        yield config.APP_DB_PATH


def a_key(**overrides):
    """A key over the reference deck, with any argument swapped out."""
    kind = overrides.pop("kind", "sim.mana")
    library, commander = build_golgari(34)
    args = {"library": library, "commander": commander, "games": 1000, "turns": 10,
                "keep_rule": KeepRule(), "seed": 11}
    args.update(overrides)
    return cache.key(kind, **args)


# ------------------------------------------------------------- the key holds

def test_the_key_covers_every_input_the_engine_reads():
    """Checked against `run`'s real signature, not against a list somebody
    remembered to update.

    This is the test that catches the future bug: a parameter added to `run`
    that changes its output and is not in the key would make every result
    computed with the new parameter collide with one computed without it. The
    suite fails until whoever added it decides which set it belongs in.
    """
    parameters = set(inspect.signature(run).parameters)
    assert parameters == cache.RUN_INPUTS | cache.RUN_NON_INPUTS, (
        "engine.run's signature has changed. Every parameter that changes its "
        "output belongs in cache.RUN_INPUTS *and* in cache.key; one that "
        "cannot (like `progress`) belongs in RUN_NON_INPUTS."
    )
    # And the key function actually takes them. `library` and `commander` are
    # positional-by-name here too, so the whole set is checkable.
    assert set(inspect.signature(cache.key).parameters) >= cache.RUN_INPUTS


def test_every_run_parameter_changes_the_key():
    base = a_key()
    assert base is not None
    assert a_key(games=1001) != base
    assert a_key(turns=11) != base
    assert a_key(seed=12) != base
    assert a_key(keep_rule=KeepRule(min_lands=3)) != base
    assert a_key(commander=None) != base
    assert cache.key("sim.lands.count", library=build_golgari(34)[0],
                     commander=build_golgari(34)[1], games=1000, turns=10,
                     keep_rule=KeepRule(), seed=11) != base


def test_every_keep_rule_field_changes_the_key():
    """`asdict` is used rather than naming the fields, so a new mulligan lever
    is in the key the day it is added. This pins that it stays that way."""
    base = a_key()
    for field in fields(KeepRule):
        current = getattr(KeepRule(), field.name)
        assert a_key(keep_rule=replace(KeepRule(), **{field.name: current + 1})) \
            != base, f"KeepRule.{field.name} does not reach the key"


def test_a_changed_card_changes_the_key():
    """The whole argument for hashing compiled cards rather than deck.yaml.

    Every mutation below is something a `data refresh` can do to a card while
    the deck file does not move: a land retemplated to enter tapped, a card
    whose produced mana changed, a cost correction.
    """
    library, commander = build_golgari(34)
    base = cache.key("sim.mana", library=library, commander=commander,
                     games=1000, turns=10, keep_rule=KeepRule(), seed=11)

    def key_with(index: int, **changes) -> str | None:
        mutated = list(library)
        mutated[index] = replace(mutated[index], **changes)
        return cache.key("sim.mana", library=mutated, commander=commander,
                         games=1000, turns=10, keep_rule=KeepRule(), seed=11)

    tapped = next(i for i, c in enumerate(library)
                  if c.is_land and not c.enters_tapped)
    assert key_with(tapped, enters_tapped=True) != base
    assert key_with(0, produces=(ManaSource(frozenset("WUBRG")),)) != base
    assert key_with(0, cost=ManaCost(generic=9)) != base
    assert key_with(0, name="Something Else") != base
    assert key_with(0, fetches_lands=3) != base
    assert key_with(0, produce_delay=2) != base
    assert key_with(0, category="ramp") != base


def test_card_order_reaches_the_key():
    """Two libraries with the same cards in a different order are not the same
    input: `rng.shuffle` starts from the list it is given, so a seeded run over
    one does not reproduce the other."""
    library, commander = build_golgari(34)
    swapped = [library[1], library[0], *library[2:]]
    assert cache.key("sim.mana", library=swapped, commander=commander,
                     games=1000, turns=10, keep_rule=KeepRule(), seed=11) \
        != a_key()


def test_an_identical_deck_gives_an_identical_key():
    """The other half: rebuilding the same deck must land on the same key, or
    the cache never hits at all."""
    assert a_key() == a_key()


def test_the_key_is_stable_across_processes():
    """Two fresh interpreters, two hash seeds, one key.

    `SimCard.produces` and `ManaCost.pips` hold frozensets, whose iteration
    order varies with `PYTHONHASHSEED` and is fixed for a process's life. A key
    that serialised one directly would be stable in any single test and
    different after every restart -- a cache that quietly never hits.
    """
    def probe(hash_seed: str) -> str:
        env = {**os.environ, "PYTHONHASHSEED": hash_seed}
        proc = subprocess.run([sys.executable, str(PROBE)], capture_output=True,
                              text=True, env=env, cwd=REPO_ROOT, check=True)
        return proc.stdout.strip()

    first, second = probe("0"), probe("524287")
    assert first == second, "the cache key depends on hash randomisation"
    assert len(first) == 64


# --------------------------------------------------------- the engine holds

def test_the_engine_source_is_part_of_the_key(monkeypatch):
    """A code change must invalidate, even one nobody declared.

    Simulated by moving the fingerprint, which is what editing `engine.py` or
    `mana.py` does.
    """
    base = a_key()
    monkeypatch.setattr(cache, "_fingerprint_done", True)
    monkeypatch.setattr(cache, "_fingerprint_cache", "a-different-engine")
    assert a_key() != base


def test_the_version_is_part_of_the_key(monkeypatch):
    base = a_key()
    monkeypatch.setattr(cache, "SIM_VERSION", cache.SIM_VERSION + 1)
    assert a_key() != base


def test_an_unreadable_engine_source_fails_closed(monkeypatch, store):
    """The fail-closed path, executed rather than described.

    `fingerprint()` swallows whatever went wrong and returns `None`, and a
    `None` fingerprint switches caching off. The test that matters is that it
    switches caching *off* rather than falling back to something weaker -- an
    engine that cannot be identified must be recomputed against, because the
    alternative is serving a number from an engine that may not be this one.
    """
    monkeypatch.setattr(cache, "_ENGINE_SOURCES", ("mtglab.no.such.module",))
    monkeypatch.setattr(cache, "_fingerprint_done", False)
    monkeypatch.setattr(cache, "_fingerprint_cache", None)

    assert cache.fingerprint() is None
    assert a_key() is None
    assert cache.stats()["enabled"] is False


def test_an_unfingerprintable_engine_disables_caching(monkeypatch, store):
    """Failing closed. If the code cannot be identified, the honest answer is
    to compute rather than to serve something that might predate it."""
    monkeypatch.setattr(cache, "_fingerprint_done", True)
    monkeypatch.setattr(cache, "_fingerprint_cache", None)
    assert a_key() is None
    # A `None` key is a miss that never touches the database, and a store that
    # keeps nothing -- so the caller needs no branch of its own.
    cache.put(None, "sim.mana", {"games": 1})
    assert cache.get(None) is None
    assert cache.stats()["enabled"] is False


def test_the_fingerprint_is_a_real_hash_of_real_files():
    """A digest is opaque, so check the claim underneath it: both modules were
    read, and a fingerprint is not being fabricated from a constant."""
    from mtglab import mana
    from mtglab.sim.tier1 import engine
    for module in (engine, mana):
        assert Path(module.__file__).exists()
    assert cache.fingerprint() is not None
    assert len(cache.fingerprint()) == 64


def test_the_cache_version_is_pinned_against_the_determinism_digest():
    """The two must move together.

    `test_determinism.REFERENCE_DIGEST` is the pin on what Tier 1 reports.
    Changing it is a deliberate act that says "these numbers are different
    now", and results computed before that change must not be served after it.
    The engine fingerprint above covers this automatically for a source edit;
    this pair is the backstop for the case it cannot see -- a change in output
    that comes from somewhere else, like `compile.py`.

    If this fails: decide whether stored results are still valid. If they are
    not -- and if the digest moved, they are not -- bump `cache.SIM_VERSION`
    and update the tuple below.
    """
    from test_determinism import REFERENCE_DIGEST
    assert (REFERENCE_DIGEST, cache.SIM_VERSION) == (
        "3a19995ef9ae506dbf9bb05eecf11a7a2576c073f9b03e6876d5b30ab60b1239", 1)


# ------------------------------------------------------------- the store holds

def test_a_result_survives_a_round_trip(store):
    key = a_key()
    assert cache.get(key) is None
    cache.put(key, "sim.mana", {"games": 1000, "caveat": "Tier 1 only"})
    hit = cache.get(key)
    assert hit is not None
    assert hit.result == {"games": 1000, "caveat": "Tier 1 only"}
    # A cached figure that cannot say how old it is cannot be reported honestly.
    assert hit.created_at


def test_a_second_put_replaces_rather_than_duplicating(store):
    key = a_key()
    cache.put(key, "sim.mana", {"games": 1})
    cache.put(key, "sim.mana", {"games": 2})
    assert cache.get(key).result == {"games": 2}
    assert cache.stats()["rows"] == 1


def test_the_store_is_bounded_and_evicts_least_recently_used(store, monkeypatch):
    monkeypatch.setattr(cache, "MAX_ROWS", 3)
    for n in range(3):
        cache.put(f"key-{n}", "sim.mana", {"n": n})
    # Touch the oldest, so recency and insertion order disagree.
    assert cache.get("key-0") is not None
    cache.put("key-3", "sim.mana", {"n": 3})

    assert cache.stats()["rows"] == 3
    assert cache.get("key-1") is None, "the least recently used row should go"
    assert cache.get("key-0") is not None, "a touched row should survive"
    assert cache.get("key-3") is not None


def test_stats_report_what_is_stored(store):
    cache.put("a", "sim.mana", {"games": 1})
    cache.put("b", "sim.lands.count", {"lands": 34})
    cache.put("c", "sim.lands.count", {"lands": 35})
    info = cache.stats()
    assert info["rows"] == 3
    assert info["by_kind"] == {"sim.mana": 1, "sim.lands.count": 2}
    assert info["bytes"] > 0
    assert info["oldest"] <= info["newest"]
    assert cache.clear() == 3
    assert cache.stats()["rows"] == 0


def test_a_broken_store_is_a_miss_rather_than_a_failure(tmp_path, monkeypatch):
    """A cache that can fail a simulation is worse than no cache.

    Pointed at a path that cannot hold a database, every entry point has to
    behave as though the cache is simply empty.
    """
    wall = tmp_path / "not-a-directory"
    wall.write_text("this is a file, so app.db cannot be created under it")
    with config.use_paths(data_dir=wall):
        assert cache.get("some-key") is None
        cache.put("some-key", "sim.mana", {"games": 1})       # must not raise
        assert cache.stats()["rows"] == 0
        assert cache.clear() == 0


def test_an_unstorable_result_is_dropped_rather_than_raising(store):
    @dataclass
    class NotJson:
        pass

    cache.put("k", "sim.mana", {"bad": NotJson()})            # must not raise
    assert cache.get("k") is None


def test_the_stored_card_form_sorts_sets_but_keeps_tuple_order():
    """The serialisation's two rules, checked directly rather than inferred
    from a hash: colours come out sorted, pips do not get reordered."""
    card = SimCard(
        name="Test",
        cost=ManaCost(generic=1, pips=(frozenset("G"), frozenset("BW"))),
        produces=(ManaSource(frozenset("GB"), 2),),
    )
    form = cache._card_form(card)
    assert form[3] == [["G"], ["B", "W"]]          # sorted within, order kept
    assert form[8] == [[["B", "G"], 2]]
