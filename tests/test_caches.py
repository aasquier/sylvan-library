"""The cache register: does it see everything, and does anything it sees work?

Two different jobs in one file, and the second is the load-bearing one.

Testing the register itself is ordinary. **Sweeping the package for caches
that never registered** is the part that earns its place, because the failure
it prevents is silent: the 2026-08-19 pass shipped an `oracle_columns` cache
that was correct, tested, and never hit once, and nothing in a green suite
could say so. A cache outside the register is a cache nobody is measuring, and
this is the rule that keeps the next one from being written the same way.
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest

from mtglab import caches, config

SRC = Path(__file__).resolve().parents[1] / "src" / "mtglab"

#: Every module that owns a registered cache, imported so it can register.
#: Listed rather than discovered, because a sweep that imports the package to
#: find out what the package registers proves only that it agrees with itself.
OWNERS = (
    "mtglab.api.flymetrics",
    "mtglab.api.service",
    "mtglab.auth.passwords",
    "mtglab.cards.db",
    "mtglab.decks.model",
    "mtglab.sim.cache",
    "mtglab.sim.tier3.coverage",
)


@pytest.fixture(autouse=True)
def _owners_imported():
    """Import every owner, so the register is complete for these tests."""
    import importlib
    for name in OWNERS:
        importlib.import_module(name)


# --------------------------------------------------------------- the register

def test_a_registered_cache_counts_hits_and_misses():
    store: dict[str, int] = {}
    stats = caches.register("test.demo", clear=store.clear,
                            size=lambda: len(store), note="a demo")
    stats.miss()
    store["a"] = 1
    stats.hit()
    stats.hit()

    assert stats.asked == 3
    assert stats.rate == pytest.approx(2 / 3)
    row = next(r for r in caches.report() if r.name == "test.demo")
    assert (row.hits, row.misses, row.size, row.note) == (2, 1, 1, "a demo")


def test_a_cache_nobody_asked_reports_none_not_zero():
    """None and 0.0 are different findings and must not render alike."""
    stats = caches.register("test.untouched", clear=lambda: None)
    assert stats.rate is None
    stats.miss()
    assert stats.rate == 0.0


def test_the_report_puts_the_finding_at_the_top():
    caches.register("test.good", clear=lambda: None).hits = 9
    bad = caches.register("test.bad", clear=lambda: None)
    bad.misses = 9
    silent = caches.register("test.silent", clear=lambda: None)

    names = [r.name for r in caches.report()]
    assert names.index(silent.name) < names.index("test.bad")
    assert names.index("test.bad") < names.index("test.good")


def test_clear_all_survives_a_cache_that_refuses():
    """One stubborn entry must not abort a cold run."""
    cleared: list[str] = []

    def boom() -> None:
        raise RuntimeError("no")

    caches.register("test.boom", clear=boom)
    caches.register("test.fine", clear=lambda: cleared.append("fine"))
    caches.clear_all()
    assert cleared == ["fine"]


def test_reset_stats_does_not_empty_a_functools_cache():
    """The counters zero; the expensive value stays.

    `auth.passwords._dummy_hash` is why. Its whole design is that the Argon2
    hash is computed once, so a run that zeroed its statistics by clearing it
    would make the timing defence pay again -- and would do so invisibly,
    since the numbers afterwards look exactly right.
    """
    from functools import cache

    calls = []

    @cache
    def expensive() -> int:
        calls.append(1)
        return 42

    caches.register_lru("test.lru", expensive)
    for _ in range(3):
        expensive()
    assert len(calls) == 1

    row = next(r for r in caches.report() if r.name == "test.lru")
    assert (row.hits, row.misses) == (2, 1)

    caches.reset_stats()
    row = next(r for r in caches.report() if r.name == "test.lru")
    assert (row.hits, row.misses) == (0, 0)
    assert len(calls) == 1, "reset_stats must not have thrown the value away"

    expensive()
    row = next(r for r in caches.report() if r.name == "test.lru")
    assert row.hits == 1


def test_release_handles_leaves_ordinary_caches_alone():
    """A stale value costs a re-parse; a stale handle refuses somebody's work.

    `conftest.py` runs this after every test, so it has to be the narrow one:
    emptying every memo between tests would make the suite pay for the
    fixture, and the fixture is not about values.
    """
    emptied: list[str] = []
    caches.register("test.value", clear=lambda: emptied.append("value"))
    caches.register("test.handle", clear=lambda: emptied.append("handle"),
                    holds_handle=True)

    caches.release_handles()
    assert emptied == ["handle"]

    caches.clear_all()
    assert sorted(emptied) == ["handle", "handle", "value"]


def test_the_pool_keeper_is_the_thing_that_holds_a_handle():
    """Named rather than inferred, because the fixture is generic and the
    consequence is specific: DuckDB refuses a second connection to one file
    with a different configuration, so a held read-only lease makes a later
    `db.connect()` fail in an unrelated test."""
    holders = [name for name, reg in caches.registered().items()
               if reg.holds_handle and not name.startswith("test.")]
    assert holders == ["pool.keeper"]


def test_a_read_write_connect_survives_a_held_keeper(tmp_path):
    """The failure the fixture prevents, reproduced and then prevented."""
    from mtglab.api import service
    from mtglab.cards import db

    # `mtg.duckdb`, because `_pin` opens `config.DB_PATH` rather than the path
    # it is handed -- it stats one and connects to the other, deliberately.
    pool = tmp_path / "mtg.duckdb"
    db.connect(pool).close()
    with config.use_paths(data_dir=tmp_path):
        service._pin(pool)
        assert service._KEEPER is not None
        with pytest.raises(Exception, match="configuration"):
            db.connect(pool)

        caches.release_handles()
        assert service._KEEPER is None
        db.connect(pool).close()


# ------------------------------------------------------- the sweep for drift

def _module_state(path: Path) -> set[str]:
    """Module-level names this file mutates from inside a function.

    The proxy for "memo": assigned at module scope, written at call time. It
    catches constant tables built once and never touched (`_SOURCE_IDS` and
    friends) by *not* catching them, which is what keeps the allowlist short
    enough that every entry is a real decision.
    """
    tree = ast.parse(path.read_text(encoding="utf-8"))
    declared: set[str] = set()
    for node in tree.body:
        if isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name):
            declared.add(node.target.id)
        elif isinstance(node, ast.Assign):
            declared.update(t.id for t in node.targets
                            if isinstance(t, ast.Name))

    written: set[str] = set()
    mutators = {"update", "setdefault", "clear", "pop", "popitem",
                "move_to_end", "append", "add"}
    for fn in ast.walk(tree):
        if not isinstance(fn, ast.FunctionDef | ast.AsyncFunctionDef):
            continue
        for node in ast.walk(fn):
            if isinstance(node, ast.Assign):
                for target in node.targets:
                    if isinstance(target, ast.Subscript) \
                            and isinstance(target.value, ast.Name) \
                            and target.value.id in declared:
                        written.add(target.value.id)
                    elif isinstance(target, ast.Name) and target.id in declared:
                        written.add(target.id)
            elif isinstance(node, ast.Call) \
                    and isinstance(node.func, ast.Attribute) \
                    and isinstance(node.func.value, ast.Name) \
                    and node.func.value.id in declared \
                    and node.func.attr in mutators:
                written.add(node.func.value.id)
    return written


def _sweep() -> set[str]:
    found: set[str] = set()
    for path in sorted(SRC.rglob("*.py")):
        if "web_dist" in path.parts:
            continue
        parts = path.relative_to(SRC).with_suffix("").parts
        module = "mtglab." + ".".join(parts).removesuffix(".__init__")
        found |= {f"{module}.{name}" for name in _module_state(path)}
    return found


def test_the_sweep_can_actually_see_something():
    """The guard's own guard.

    A detector that silently finds nothing passes every test below while
    protecting nothing -- which is the exact failure this repo has now met
    four times, and the reason every new guard here gets a test that fails
    when the guard is *inert* rather than only when the guarded thing breaks.
    """
    found = _sweep()
    assert len(found) >= 15, f"the sweep has gone blind: {sorted(found)}"
    assert "mtglab.cards.db._CARD_CACHE" in found


def test_every_module_level_cache_is_registered_or_excused():
    """A cache outside the register is a cache nobody is measuring.

    When this fails, the fix is one of two lines, never a wider allowlist:
    call `caches.register` beside the cache, or add it to `NOT_CACHES` with a
    sentence saying why it is not one.
    """
    found = _sweep()
    known = {
        "mtglab.cards.db._CARD_CACHE", "mtglab.cards.db._COLUMN_CACHE",
        "mtglab.decks.model._PARSED", "mtglab.api.service._SETS_CACHE",
        "mtglab.sim.tier3.coverage._INDEX_CACHE",
        "mtglab.sim.cache._fingerprint_cache", "mtglab.api.flymetrics._cache",
    }
    unaccounted = found - known - set(caches.NOT_CACHES)
    assert not unaccounted, (
        "module-level state that is neither registered as a cache nor listed "
        f"in caches.NOT_CACHES with a reason: {sorted(unaccounted)}")


def test_not_caches_entries_all_carry_a_reason():
    blank = [k for k, v in caches.NOT_CACHES.items() if not v.strip()]
    assert not blank, f"excused without a reason: {blank}"


def test_not_caches_does_not_excuse_something_that_has_gone_away():
    """A stale exclusion is an allowlist entry nobody has to justify again."""
    found = _sweep()
    stale = [name for name in caches.NOT_CACHES if name not in found]
    assert not stale, (
        f"NOT_CACHES names state that no longer exists: {stale} — delete the "
        "entry rather than leaving a permanent excuse behind")
