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
