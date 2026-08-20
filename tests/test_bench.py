"""The measuring shelf, measured.

The tests that matter here are the ones about **honesty**, not about arithmetic.
A benchmark that quietly measures nothing, or that attributes a database's time
to this repo's Python, is worse than no benchmark: it produces a number, the
number goes into the ledger, and a later run reasons from it. So the load-
bearing assertions are that an unavailable target comes back as a *row*, that
the database budget is measured rather than subtracted, and that the import
counter can actually see an import storm.
"""

from __future__ import annotations

import pytest

from mtglab import caches, config
from mtglab.bench import profile as benchprofile
from mtglab.bench import run as benchrun
from mtglab.bench import targets as benchtargets
from mtglab.cards import db

pytest.importorskip("fastapi")


# ------------------------------------------------------------------ targets

def test_a_target_that_cannot_run_is_a_row_and_not_a_gap(tmp_path):
    """A suite that silently shrank is the failure this whole pass keeps
    finding. An unmeasurable target must say so, by name."""
    with config.use_paths(data_dir=tmp_path, decks_dir=tmp_path / "decks"):
        suite = benchtargets.suite()
    unavailable = [t for t in suite if t.unavailable]
    assert unavailable, "with no pool and no decks, something must be skipped"
    assert all(t.unavailable.strip() for t in unavailable)
    names = {t.name for t in suite}
    assert "db.get_cards" in " ".join(names)


def test_an_unavailable_target_is_never_called(tmp_path):
    with config.use_paths(data_dir=tmp_path, decks_dir=tmp_path / "decks"):
        suite = benchtargets.suite()
        samples = benchrun.run_suite(suite, runs=2)
    skipped = [s for s in samples if s.skipped]
    assert skipped
    assert all(not s.ran for s in skipped)
    assert all(not s.failed for s in skipped), \
        "an unavailable target must be skipped, not called and caught"


def test_a_failing_target_is_reported_as_failed_not_as_fast():
    def boom():
        raise ValueError("nope")

    target = benchtargets.Target("boom", "library", boom)
    sample = benchrun.run_suite([target], runs=2)[0]
    assert not sample.ran
    assert "ValueError" in sample.failed
    assert "**failed" in benchrun.as_markdown([sample], cold=False)


# ---------------------------------------------------------------- the runner

def test_medians_not_means():
    """One slow sample must not move the headline number."""
    calls = {"n": 0}

    def uneven():
        calls["n"] += 1
        if calls["n"] == 3:
            import time
            time.sleep(0.05)

    sample = benchrun.run_suite(
        [benchtargets.Target("uneven", "library", uneven)],
        runs=11, profile_over_ms=float("inf"))[0]
    assert sample.median_ms < 10, "a single outlier moved the median"
    assert sample.p95_ms >= sample.median_ms


def test_cold_empties_every_registered_cache_between_samples():
    cleared = {"n": 0}
    caches.register("test.bench-cold", clear=lambda: cleared.__setitem__(
        "n", cleared["n"] + 1))
    target = benchtargets.Target("noop", "library", lambda: None)

    benchrun.run_suite([target], runs=3, cold=True,
                       profile_over_ms=float("inf"))
    assert cleared["n"] >= 4, "cold must clear before the warm-up and each run"

    before = cleared["n"]
    benchrun.run_suite([target], runs=3, cold=False,
                       profile_over_ms=float("inf"))
    assert cleared["n"] == before, "a warm run must clear nothing"


def test_slow_targets_are_profiled_and_fast_ones_are_not():
    """Gap 1 of the 2026-08-19 refinement, made mechanical: a number over the
    threshold is a question the tool asks rather than one a run must remember
    to ask."""
    import time

    slow = benchtargets.Target("slow", "library", lambda: time.sleep(0.02))
    fast = benchtargets.Target("fast", "library", lambda: None)
    samples = benchrun.run_suite([slow, fast], runs=3, profile_over_ms=10.0)
    assert samples[0].profile is not None
    assert samples[1].profile is None


def test_a_cold_run_profiles_a_cold_call():
    """A profile printed under a cold heading must describe a cold request.

    It did not, for one run: the table said 38.3ms and the profile under it
    said 7.2ms, because the profiler was handed the bare callable. A
    breakdown that does not explain the number above it is read as if it
    does.
    """
    seen: list[bool] = []
    cleared = {"n": 0}
    caches.register("test.cold-profile",
                    clear=lambda: cleared.__setitem__("n", cleared["n"] + 1))

    def slow():
        seen.append(cleared["n"] > 0)
        import time
        time.sleep(0.02)

    sample = benchrun.run_suite(
        [benchtargets.Target("slow", "library", slow)],
        runs=2, cold=True, profile_over_ms=10.0)[0]
    assert sample.profile is not None
    assert all(seen), "some call ran against a cache nobody had emptied"


def test_the_markdown_says_which_state_it_measured():
    target = benchtargets.Target("noop", "library", lambda: None)
    warm = benchrun.as_markdown(benchrun.run_suite([target], runs=2),
                                cold=False)
    cold = benchrun.as_markdown(benchrun.run_suite([target], runs=2),
                                cold=True)
    assert "warm median" in warm
    assert "cold median" in cold


# --------------------------------------------------------------- the profile

def test_the_database_budget_is_measured_not_subtracted(tmp_path):
    """The whole reason `db.set_query_probe` exists.

    cProfile raises no event for an extension method, so a DuckDB `execute`
    lands in the tottime of whatever Python called it. Subtracting a profiled
    total from a wall clock would therefore call a slow query slow Python --
    which is the misattribution this tool was built to stop it committing.
    """
    pool = tmp_path / "pool.duckdb"
    con = db.connect(pool)
    con.close()

    def query():
        handle = db.connect_readonly(pool)
        try:
            handle.execute("SELECT count(*) FROM oracle_cards").fetchall()
        finally:
            handle.close()

    prof = benchprofile.profile_target("one query", query, repeat=2)
    assert prof.queries.count >= 2, "the probe saw no statements at all"
    assert prof.db_s > 0
    assert prof.other_s == pytest.approx(max(prof.wall_s - prof.db_s, 0.0))


def test_the_probe_is_off_unless_somebody_is_measuring(tmp_path):
    """No wrapper, no branch, nothing to pay for in the app."""
    pool = tmp_path / "pool.duckdb"
    con = db.connect(pool)
    con.close()
    assert db._QUERY_PROBE is None
    handle = db.connect_readonly(pool)
    try:
        assert not isinstance(handle, db._ProbedConnection)
    finally:
        handle.close()


def test_a_probed_connection_still_keys_the_pool_caches(tmp_path):
    """A wrapper that could not be a WeakKeyDictionary key would disable both
    pool caches, turning the benchmark into a measurement of an app nobody
    runs."""
    pool = tmp_path / "pool.duckdb"
    db.connect(pool).close()
    db.set_query_probe(lambda sql, seconds: None)
    try:
        handle = db.connect_readonly(pool)
        try:
            assert isinstance(handle, db._ProbedConnection)
            assert db._pool_stamp(handle) is not None
            first = db.oracle_columns(handle)
            assert db.oracle_columns(handle) == first
        finally:
            handle.close()
    finally:
        db.set_query_probe(None)


def test_an_n_plus_one_shows_as_a_repeated_statement(tmp_path):
    pool = tmp_path / "pool.duckdb"
    db.connect(pool).close()

    def many():
        handle = db.connect_readonly(pool)
        try:
            for _ in range(30):
                handle.execute("SELECT 1 FROM oracle_cards LIMIT 1").fetchall()
        finally:
            handle.close()

    prof = benchprofile.profile_target("n+1", many, repeat=1)
    worst = prof.queries.worst_repeat()
    assert worst is not None and worst[1] >= 30
    assert "n+1" in prof.verdict() or prof.db_share <= 0.6


def test_the_import_counter_can_see_an_import_storm():
    """The instrument's own mutation test.

    #181's fix is one line: a `None` sentinel in `sys.modules` so DuckDB's
    per-parameter `import pandas` probe fails without walking `sys.path`.
    Take the sentinel away and the storm comes back — and the counter has to
    notice, or it would have been just as blind as the checklist line that
    recorded 224ms and moved on.
    """
    import sys

    def quiet():
        sys.modules.setdefault("pandas", None)                       # type: ignore[arg-type]

    def storm():
        sys.modules.pop("pandas", None)
        for _ in range(40):
            __import__("importlib.util").util.find_spec("pandas")

    calm = benchprofile.profile_target("quiet", quiet, repeat=1)
    noisy = benchprofile.profile_target("storm", storm, repeat=1)
    sys.modules.setdefault("pandas", None)                           # type: ignore[arg-type]

    assert noisy.import_calls > calm.import_calls
    assert noisy.import_calls > benchprofile.IMPORT_CALLS_SUSPECT
    assert "import machinery" in noisy.verdict()


def test_a_warm_request_imports_something_and_that_is_not_the_alarm(tmp_path):
    """The other half of the storm test, and the half that was prose.

    `import_calls` was documented in three places as "zero is the only right
    answer on a warm request" while `IMPORT_CALLS_SUSPECT` sat at 200 and the
    tool reported 7–31 every time. Both cannot be true, and the threshold is
    the half that behaves: #181 did not *remove* DuckDB's per-parameter
    `import pandas` probe, it answered it with a `sys.modules` sentinel, so
    the probe still enters the import machinery and still counts — it simply
    stopped walking `sys.path`. Two calls per bound value, deterministically.

    Pinned from below because that is the direction nobody was watching. A
    later run "fixing" the count to match the old sentence, or tightening the
    threshold onto the warm band, would make every healthy profile read as a
    finding — and an instrument that cries wolf is one nobody reads.
    """
    con = db.connect(tmp_path / "pool.duckdb")

    def bind_three():
        con.execute("SELECT ?, ?, ?", ["a", "b", "c"]).fetchone()

    bind_three()                                   # warm every lazy import
    prof = benchprofile.profile_target("bind", bind_three, repeat=3)
    assert prof.import_calls > 0, \
        "a bound parameter still asks; the sentinel answers it, cheaply"
    assert prof.import_calls < benchprofile.IMPORT_CALLS_SUSPECT
    assert "import machinery" not in prof.verdict()


def test_the_verdict_routes_rather_than_describes():
    """Two numbers get read as two numbers; the verdict says which lever."""
    prof = benchprofile.profile_target("pure python", lambda: sum(range(9999)),
                                       repeat=2)
    assert prof.queries.count == 0
    assert "no pool involved" in prof.verdict()


def test_the_stand_in_for_an_unavailable_target_refuses_to_be_called():
    """Belt and braces on `test_an_unavailable_target_is_never_called`: the
    runner skips these, and if it ever stopped, this raises rather than
    reporting a target that does nothing as the fastest one in the suite."""
    with pytest.raises(RuntimeError, match="unavailable"):
        benchtargets._never()


def test_with_no_api_every_endpoint_is_a_row_saying_so(monkeypatch):
    """A base install has no fastapi. The suite must still be a full page."""
    monkeypatch.setattr(benchtargets, "_build",
                        lambda owner="local": benchtargets._Bench(
                            reason="no API extra (ImportError)"))
    endpoints = [t for t in benchtargets.suite() if t.kind == "endpoint"]
    assert len(endpoints) == 6
    assert all("no API extra" in t.unavailable for t in endpoints)


def test_an_app_that_will_not_start_is_reported_with_its_reason(monkeypatch):
    class Boom:
        def get(self, path):
            raise RuntimeError("the shelf collapsed")

    monkeypatch.setattr("fastapi.testclient.TestClient", lambda app: Boom())
    bench = benchtargets._build()
    assert "app would not start" in bench.reason
    assert "the shelf collapsed" in bench.reason


def test_the_name_list_falls_back_when_no_deck_can_be_read(tmp_path):
    """A broken deck file must not empty the benchmark."""
    with config.use_paths(data_dir=tmp_path, decks_dir=tmp_path / "decks"):
        (tmp_path / "decks" / "broken").mkdir(parents=True)
        (tmp_path / "decks" / "broken" / "deck.yaml").write_text("{[bad")
        assert benchtargets._bench_names() == [
            "Sol Ring", "Command Tower", "Arcane Signet"]


def test_a_frame_says_where_it_is_without_wrapping_the_table():
    short = benchprofile._short
    assert short("~") == "<native>"
    assert short("/a/b/src/mtglab/api/service.py") == "mtglab/api/service.py"
    assert short("/x/site-packages/httpx/_client.py").startswith("site-packages")
    assert short("/some/deep/path/thing.py") == "path/thing.py"


def test_a_target_that_fails_only_once_warm_is_still_reported():
    """The warm-up call is outside the timing loop, so a target that raises
    there must not come back as a fast zero."""
    def once():
        raise KeyError("cold")

    sample = benchrun.run_suite(
        [benchtargets.Target("once", "library", once)], runs=2)[0]
    assert not sample.ran and "KeyError" in sample.failed


def test_the_verdict_names_an_n_plus_one_rather_than_a_slow_query():
    """Two database-bound requests, opposite fixes. One statement taking 40ms
    is a query to rewrite; four hundred taking 0.1ms each is a loop to batch,
    and a verdict that called both "database-bound" would send the reader to
    the wrong file."""
    log = benchprofile.QueryLog()
    for _ in range(40):
        log.record("SELECT 1 FROM oracle_cards WHERE name = ?", 0.002)
    n_plus_one = benchprofile.Profile(
        name="n+1", wall_s=0.1, db_s=0.09, queries=log, import_calls=0,
        import_share=0.0, frames=[])
    assert "n+1" in n_plus_one.verdict()

    single = benchprofile.QueryLog()
    single.record("SELECT * FROM oracle_cards", 0.09)
    assert "n+1" not in benchprofile.Profile(
        name="one", wall_s=0.1, db_s=0.09, queries=single, import_calls=0,
        import_share=0.0, frames=[]).verdict()


def test_a_target_too_fast_to_attribute_says_so():
    empty = benchprofile.Profile(name="x", wall_s=0.0, db_s=0.0,
                                 queries=benchprofile.QueryLog(),
                                 import_calls=0, import_share=0.0, frames=[])
    assert empty.verdict() == "too fast to attribute"


def test_a_profile_that_blows_up_leaves_the_timing_intact():
    """The sample is the measurement; the profile is the explanation. Losing
    the explanation must not lose the number."""
    import time

    def slow():
        time.sleep(0.02)

    target = benchtargets.Target("slow", "library", slow)
    calls = {"n": 0}

    def boom(*args, **kwargs):
        calls["n"] += 1
        raise RuntimeError("profiler fell over")

    original = benchrun.profile_target
    benchrun.profile_target = boom
    try:
        sample = benchrun.run_suite([target], runs=3, profile_over_ms=1.0)[0]
    finally:
        benchrun.profile_target = original

    assert calls["n"] == 1
    assert sample.ran and sample.median_ms > 0
    assert sample.profile is None
