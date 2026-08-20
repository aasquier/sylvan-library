"""The mutation harness, held to its own standard.

The assertions that matter are the safety ones. This tool exists to *break*
code, so the questions worth testing are: does it break exactly one thing,
does it always put it back, and can it reach the working tree at all? The
answer to the last must be no, and it is tested by proving the shadow is what
pytest imports rather than by trusting the flag that says so.
"""

from __future__ import annotations

import textwrap
from pathlib import Path

import pytest

from mtglab import mutate
from mtglab.mutate import harness, operators

SRC = Path(__file__).resolve().parents[1] / "src"

SAMPLE = textwrap.dedent('''\
    def keep(n, limit):
        if n < 0:
            return None
        total = n + 1
        ok = n > limit and total == 3
        return total if ok else 0
    ''')


# ------------------------------------------------------------------ operators

def test_the_catalogue_finds_each_kind_it_claims():
    found = operators.find(SAMPLE, module="sample", relpath="sample.py")
    kinds = {m.operator for m in found}
    assert {"comparison", "arithmetic", "boolean", "boundary",
            "guard"} <= kinds


DECORATED = textwrap.dedent('''\
    from dataclasses import dataclass
    from functools import lru_cache


    @dataclass(frozen=True)
    class Limit:
        n: int


    @lru_cache(maxsize=16)
    def cached(n):
        strict = True
        return n if strict else 0
    ''')


def test_a_decorator_flag_is_not_a_mutation_site():
    """`@dataclass(frozen=True)` is a declaration, not a branch.

    Nothing reads it after import, so no assertion can tell the two values
    apart and no test could ever kill the mutation — it is a guaranteed
    survivor, forever, and it drags the kill rate down for a reason that says
    nothing about the suite. Measured 2026-08-19: 19 such sites across the 18
    declared modules, every one `frozen=True`, and 22% of the whole `constant`
    class. A 25-mutation sample drew two and both survived.
    """
    found = operators.find(DECORATED, module="sample", relpath="sample.py")
    on_decorator = [m for m in found if m.line == 5]
    assert not on_decorator, f"decorator flag catalogued: {on_decorator}"


def test_the_exclusion_reaches_the_decorator_and_nothing_past_it():
    """Both halves, because an over-broad rule is the worse failure here.

    A body-level `True` is an ordinary constant site and must stay; so must
    `@lru_cache(maxsize=16)`, which is a *boundary* on a number a test can
    absolutely notice. Only a decorator's booleans go.
    """
    found = operators.find(DECORATED, module="sample", relpath="sample.py")
    assert any(m.operator == "constant" and m.line == 12 for m in found), \
        "a boolean in a function body is still a mutation site"
    assert any(m.operator == "boundary" and m.line == 10 for m in found), \
        "a decorator's *number* is still a mutation site"


def test_the_real_catalogue_holds_no_decorator_flag():
    """The guard against the exclusion silently ceasing to apply.

    `_decorator_flags` matches on `(lineno, col_offset)`, so a change to how
    the catalogue walks the tree could stop the two agreeing and nothing else
    would say so — the count would simply drift back up.
    """
    import ast

    for relpath in harness.TARGETS:
        source = (SRC / relpath).read_text(encoding="utf-8")
        flags = operators._decorator_flags(ast.parse(source))
        if not flags:
            continue
        sites = {(m.line, m.col) for m in operators.find(
            source, module=relpath, relpath=relpath)}
        assert not (flags & sites), \
            f"{relpath}: decorator flags back in the catalogue"


def test_a_mutation_changes_exactly_one_span():
    mutation = next(m for m in operators.find(
        SAMPLE, module="sample", relpath="sample.py")
        if m.operator == "comparison" and m.was == "<")
    after = mutation.apply(SAMPLE)
    assert after != SAMPLE
    before_lines, after_lines = SAMPLE.splitlines(), after.splitlines()
    differing = [i for i, (a, b) in enumerate(zip(before_lines, after_lines,
                                                  strict=True)) if a != b]
    assert len(differing) == 1
    assert len(before_lines) == len(after_lines), \
        "line numbers must not move, or a shifted traceback reads as a kill"


def test_every_mutation_still_parses():
    import ast
    for mutation in operators.find(SAMPLE, module="sample",
                                   relpath="sample.py"):
        ast.parse(mutation.apply(SAMPLE))


def test_a_guard_mutation_makes_the_guard_never_fire():
    guard = next(m for m in operators.find(SAMPLE, module="sample",
                                           relpath="sample.py")
                 if m.operator == "guard")
    assert guard.now == "False"
    assert "if False:" in guard.apply(SAMPLE)


def test_apply_refuses_when_the_file_has_moved():
    """A stale catalogue must not silently mutate the wrong span."""
    mutation = operators.find(SAMPLE, module="sample",
                              relpath="sample.py")[0]
    with pytest.raises(ValueError, match="moved"):
        mutation.apply("# a completely different file\n" + SAMPLE)


def test_the_operator_token_is_scanned_not_assumed():
    """`a<b` and `a  <  b` put the token in different columns."""
    for source in ("x = 1<2\n", "x = 1  <  2\n", "x = (1) < (2)\n"):
        mutation = next(m for m in operators.find(
            source, module="s", relpath="s.py") if m.operator == "comparison")
        assert mutation.was == "<"
        assert mutation.apply(source).count("<=") == 1


def test_multi_line_expressions_are_skipped_rather_than_mangled():
    source = "x = (\n    1\n    < 2\n)\n"
    assert not [m for m in operators.find(source, module="s", relpath="s.py")
                if m.operator == "comparison"]


# -------------------------------------------------------------- the harness

def test_every_declared_test_file_exists():
    """A mapping is a narrowing, and a typo used to widen it silently.

    The harness shipped with `tests/test_sim.py` in the map, a file that has
    never existed. Each mutation of the simulator then ran the *entire* suite
    instead of two files -- 165 seconds apiece rather than one -- and the
    report said `ran: tests`, which is true and reads like nothing.
    """
    assert mutate.missing_tests(Path.cwd()) == []


def test_the_shadow_holds_the_package_and_not_the_paintings(tmp_path):
    root = mutate.shadow(SRC, tmp_path)
    assert (root / "mtglab" / "mana.py").is_file()
    assert (root / "mtglab" / "assets").is_symlink(), \
        "4.6MB of tarot art must be linked, not copied per run"
    assert not (root / "mtglab" / "__pycache__").exists()


def test_a_mutation_never_reaches_the_working_tree(tmp_path):
    """Proved by content, not by the flag that promises it."""
    real = SRC / "mtglab" / "mana.py"
    before = real.read_bytes()
    root = mutate.shadow(SRC, tmp_path)

    mutation = next(m for m in mutate.catalogue(SRC)
                    if m.relpath == "mtglab/mana.py")
    shadowed = root / "mtglab" / "mana.py"
    shadowed.write_text(mutation.apply(shadowed.read_text()))

    assert shadowed.read_bytes() != before
    assert real.read_bytes() == before


def test_the_sample_is_reproducible_from_its_seed():
    available = mutate.catalogue(SRC)
    assert len(available) > 100

    import random
    first = random.Random(11).sample(available, 5)
    second = random.Random(11).sample(available, 5)
    third = random.Random(12).sample(available, 5)
    assert [m.describe() for m in first] == [m.describe() for m in second]
    assert [m.describe() for m in first] != [m.describe() for m in third]


def test_a_named_site_is_selected_by_its_line_and_not_by_its_text():
    """`--only decks/analyze.py:33` must not also mean line 336.

    The first draft joined `relpath:line` and asked for a substring, and the
    very first use — re-checking a survivor the ledger had recorded — silently
    swept in a second line whose number merely started with the same digits.
    A path fragment is still a substring; the number is compared as a number.
    """
    available = mutate.catalogue(SRC)
    picked = mutate.select(available, ["decks/analyze.py:33"])
    assert picked, "the site the ledger names must still exist"
    assert {m.line for m in picked} == {33}


def test_a_filter_matching_nothing_raises_rather_than_running_nothing():
    """A sample of zero reports a flawless kill rate over no mutants at all.

    That is the harness's own shipped bug pointed the other way: a mistyped
    test filename used to silently *widen* what ran, and a mistyped path here
    would silently narrow it to nothing. Both read as success.
    """
    available = mutate.catalogue(SRC)
    with pytest.raises(ValueError, match="matched none"):
        mutate.select(available, ["decks/analyze.py:99999"])


def test_a_report_counts_survivors_against_a_real_denominator():
    report = mutate.Report(sites=1000, seed=3)
    killed = mutate.Result(mutation=mutate.catalogue(SRC)[0], killed=True,
                           tests=("tests/test_mana.py",), seconds=1.0)
    lived = mutate.Result(mutation=mutate.catalogue(SRC)[1], killed=False,
                          tests=("tests/test_mana.py",), seconds=1.0)
    report.results.extend([killed, lived, killed])
    assert report.kill_rate == pytest.approx(2 / 3)
    assert report.survivors == [lived]


def test_a_kill_rate_is_none_before_anything_ran():
    assert mutate.Report().kill_rate is None


def test_end_to_end_a_broken_guard_is_caught(tmp_path):
    """One real mutation, judged by real tests. Slow, and the only test here
    that proves the pieces fit together."""
    report = mutate.run(
        sample=1, seed=0, src=SRC, workdir=tmp_path, timeout=120.0,
        targets={"mtglab/mana.py": ("tests/test_mana.py",)})
    assert len(report.results) == 1
    assert report.results[0].tests == ("tests/test_mana.py",)
    assert (SRC / "mtglab" / "mana.py").read_text() == \
        (Path.cwd() / "src" / "mtglab" / "mana.py").read_text()


def test_a_module_the_catalogue_cannot_find_is_skipped_not_fatal(tmp_path):
    """A `TARGETS` entry for a file that has moved must not stop the run —
    `missing_tests` is where a stale mapping is reported, loudly, once."""
    found = mutate.catalogue(SRC, {"mtglab/not-a-file.py": ("tests",),
                                   "mtglab/mana.py": ("tests/test_mana.py",)})
    assert found and all(m.relpath == "mtglab/mana.py" for m in found)


def test_the_shadow_replaces_an_earlier_one(tmp_path):
    first = mutate.shadow(SRC, tmp_path)
    (first / "mtglab" / "left-behind.py").write_text("# stale\n")
    second = mutate.shadow(SRC, tmp_path)
    assert not (second / "mtglab" / "left-behind.py").exists()


def test_a_run_with_no_workdir_makes_its_own(monkeypatch, tmp_path):
    made: list[str] = []
    monkeypatch.setattr("tempfile.mkdtemp",
                        lambda prefix="": made.append(prefix) or str(tmp_path))
    report = mutate.run(sample=0, seed=0, src=SRC,
                        targets={"mtglab/mana.py": ("tests/test_mana.py",)})
    assert made == ["mtglab-mutate-"]
    assert report.results == []


def test_a_mutation_that_will_not_apply_is_reported_and_not_counted_as_alive(
        tmp_path, monkeypatch):
    """The file moved under the catalogue. That is a broken sample, and the
    one thing it must not be recorded as is a survivor — a survivor is a
    claim about the suite, and nothing here tested the suite at all."""
    root = mutate.shadow(SRC, tmp_path)
    mutation = next(m for m in mutate.catalogue(SRC)
                    if m.relpath == "mtglab/mana.py")
    monkeypatch.setattr(type(mutation), "apply",
                        lambda self, source: (_ for _ in ()).throw(
                            ValueError("moved")))
    result = harness._judge(
        mutation, root=root, src=SRC, full=False, timeout=5.0,
        targets={"mtglab/mana.py": ("tests/test_mana.py",)})
    assert result.killed
    assert result.tail == ""


def test_a_hang_is_a_kill(tmp_path, monkeypatch):
    """A mutation is a good way to write an infinite loop — widen a `<` at a
    loop bound and the condition stops being reachable. A suite that hangs has
    certainly noticed, so a timeout counts as a kill; the only thing a long
    limit buys is a run nobody can sit through."""
    import subprocess

    root = mutate.shadow(SRC, tmp_path)
    mutation = next(m for m in mutate.catalogue(SRC)
                    if m.relpath == "mtglab/mana.py")

    def hang(*args, **kwargs):
        raise subprocess.TimeoutExpired(cmd="pytest", timeout=5.0)

    monkeypatch.setattr(subprocess, "run", hang)
    result = harness._judge(
        mutation, root=root, src=SRC, full=False, timeout=5.0,
        targets={"mtglab/mana.py": ("tests/test_mana.py",)})
    assert result.killed
    assert (SRC / "mtglab" / "mana.py").read_text() == \
        (root / "mtglab" / "mana.py").read_text(), \
        "the shadow must be restored even when the run dies"
