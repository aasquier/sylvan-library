"""Every place this package adds up floats, and the rule about how.

**A bare `sum()` over floats is not one function.** CPython 3.12 gave it
compensated (Neumaier) accumulation; 3.11 adds left to right. Measured on the
interpreters this project actually supports, `sum([0.1] * 10)` is `1.0` under
3.12.13 and `0.9999999999999999` under 3.11.15. Both are tested in CI and the
container runs 3.12, so a float sum written as `sum` answers one way on a
laptop and another way on the instance -- and a Go port written as
`for ... { total += x }` reproduces **3.11**, which is the interpreter the
image is not running.

So: **float sums go through `math.fsum`**, which is correctly rounded and
therefore nobody's dialect. Integer sums stay `sum`, because Python's ints are
exact and there is nothing to choose between.

It cost three separate lanes to learn on 2026-08-22 -- `sim/curve.py` (#245),
`sim/tier1/engine.py` (#240), and then the sweep that found the other six --
and the reason it cost three is that nothing enforced it. This is that
something. The lesson it encodes is the repo's own, recorded four times
already: **a rule enforced by nothing drifts.**

Both tables below are exact in both directions, `test_packaging.py`-style: a
new `sum()` anywhere under `src/mtglab` fails here until somebody writes down
which kind it is. That is the point. The failure is cheap -- add a line -- and
it buys a moment's thought at exactly the place the thought is owed.
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest

PACKAGE = Path(__file__).resolve().parents[1] / "src" / "mtglab"

#: `(module, function) -> how many `math.fsum` calls it makes`.
#:
#: Every one of these sums floats, and every one was a bare `sum` at some
#: point on 2026-08-22. What each is for, and why one ulp is not cosmetic
#: there, is argued where it lives; the note here is only enough to find it.
FLOAT_SUMS: dict[tuple[str, str], int] = {
    # The shopping list's money total, into `swaps.md` -- and the bytes of the
    # five deliverables are the product (rule 3), not a view of one.
    ("artifacts/generate.py", "swap_list"): 1,
    # `mtglab sim forge`'s mean game length. Wall-clock readings off a JVM, so
    # this number is not reproducible whatever adds it up; it is spelled like
    # the others so that a bare `sum` over floats stays a defect anybody can
    # grep for, with no exception a reader has to re-derive as harmless.
    ("cli.py", "cmd_sim_forge"): 1,
    # `mtglab decks price`'s total: the same money, the same column, and the
    # same rendering as the shopping list's.
    ("cli.py", "cmd_price_deck"): 1,
    # `keepable`: three hypergeometric probabilities, straight onto the deck
    # page. 5,098 (deck size, land count) shapes answer differently.
    ("decks/analyze.py", "opening_hand"): 1,
    # The similarity score -- rounded to four places and then *sorted on*, so
    # the difference is a different fourth decimal and, at a tie, a different
    # shortlist.
    ("decks/suggest.py", "score"): 1,
    # #245's two, the closed form's.
    ("sim/curve.py", "expected_lands_in_play"): 1,
    ("sim/curve.py", "on_curve_odds"): 1,
    # `castable_odds` reached for `fsum` first and on its own, before any of
    # this was known: a hundred small products of probabilities against a
    # `>= 0.90`.
    ("sim/karsten.py", "castable_odds"): 1,
    # #240's two, the goldfish's -- and `spells_through` is the number
    # CLAUDE.md tells readers to use instead of commander speed.
    ("sim/tier1/engine.py", "spells_through"): 1,
    ("sim/tier1/engine.py", "wasted_through"): 1,
    # The tarot deal's running weight total. `ECHO_WEIGHT` is 0.14, which no
    # binary float holds exactly, so every 134-card pool this reaches on its
    # third draw answered differently on the two interpreters. A seed is a
    # promise to somebody who reloads the page.
    ("tarot.py", "_weighted_sample"): 1,
}

#: `module -> how many bare `sum()` calls it makes`, all of them over **ints**
#: -- counts, quantities, byte sizes, `math.comb` products. Python's ints are
#: exact, so `sum` is the right and only spelling there.
#:
#: A module appearing here with a float sum inside it is the bug this file
#: exists to catch. The count is exact so that a *new* `sum` shows up as a
#: change rather than hiding inside an existing entry.
INTEGER_SUMS: dict[str, int] = {
    "animist/verify.py": 1,          # frames in an encoded animation
    "api/adminstats.py": 3,          # bytes on disk, and directory counts
    "api/forgeruns.py": 2,           # draws and clock-outs in a match
    "api/service.py": 3,             # resolved / offered / unread counts
    "artifacts/generate.py": 1,      # `qty` in a category heading
    "claude/theme.py": 1,            # user turns in a transcript
    "cli.py": 7,                     # clock-outs, token counts, byte sizes
    "decks/model.py": 2,             # `qty`, twice
    "mutate/harness.py": 1,          # mutations killed
    "sim/compile.py": 1,             # declared `qty` against the 99
    "sim/curve.py": 6,               # mana amounts, land counts, int averages
    "sim/karsten.py": 9,             # `math.comb` products, and card counts
    "sim/tier1/engine.py": 4,        # lands in hand, cheap ramp, casts by T8
    "sim/tier3/ledger.py": 2,        # draws and clock-outs, again
    "sim/tier3/run.py": 1,           # milliseconds, which `parse` reads as int
}


def _modules() -> list[Path]:
    return sorted(PACKAGE.rglob("*.py"))


def _calls_to(node: ast.AST, name: str) -> int:
    return sum(1 for n in ast.walk(node)
               if isinstance(n, ast.Call) and isinstance(n.func, ast.Name)
               and n.func.id == name)


def test_every_float_sum_in_the_package_is_declared_and_is_fsum():
    """The `fsum` register, exact in both directions.

    Reverting any one of these to `sum` deletes its entry and fails here --
    on **both** interpreters, which matters because the value assertions
    cannot: at three and four terms CPython 3.12's `sum` already equals
    `fsum`, so a reverted fix is invisible to a number on 3.12 and visible
    only on CI's 3.11 leg. This one is visible everywhere.
    """
    found: dict[tuple[str, str], int] = {}
    for path in _modules():
        rel = str(path.relative_to(PACKAGE))
        tree = ast.parse(path.read_text(encoding="utf-8"))
        for node in ast.walk(tree):
            if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                continue
            calls = _calls_to(node, "fsum")
            if calls:
                found[(rel, node.name)] = calls
    assert found == FLOAT_SUMS, (
        "the `math.fsum` register in tests/test_float_sums.py no longer "
        "matches src/mtglab.\n"
        f"  no longer there: {sorted(set(FLOAT_SUMS) - set(found))}\n"
        f"  newly there:     {sorted(set(found) - set(FLOAT_SUMS))}\n"
        "If a float sum became a bare `sum`, that is the bug this file is "
        "about -- read its docstring. If a new one arrived, add it with a "
        "note saying what it sums and why the last bit matters there.")


def test_no_module_grows_a_bare_sum_without_somebody_classifying_it():
    """The `sum` register, exact in both directions.

    A new `sum()` under `src/mtglab` fails here until it is written down as
    an integer sum -- or, if it is a float sum, until it stops being one.
    """
    found: dict[str, int] = {}
    for path in _modules():
        tree = ast.parse(path.read_text(encoding="utf-8"))
        calls = _calls_to(tree, "sum")
        if calls:
            found[str(path.relative_to(PACKAGE))] = calls
    assert found == INTEGER_SUMS, (
        "the bare-`sum()` register in tests/test_float_sums.py no longer "
        "matches src/mtglab.\n"
        f"  changed or gone: "
        f"{sorted(k for k in INTEGER_SUMS if found.get(k) != INTEGER_SUMS[k])}\n"
        f"  newly there:     {sorted(set(found) - set(INTEGER_SUMS))}\n"
        "Every entry here sums **integers**, which are exact. If what you "
        "added sums floats, use `math.fsum` and declare it in FLOAT_SUMS "
        "instead -- read this file's docstring for why.")


@pytest.mark.parametrize(("module", "func"), sorted(FLOAT_SUMS),
                         ids=[f"{m}::{f}" for m, f in sorted(FLOAT_SUMS)])
def test_each_float_sum_imports_fsum_from_math(module: str, func: str):
    """`fsum` is `math.fsum` and not something local that shadows it.

    Cheap, and it closes the one way the register above could be satisfied
    by a lie: a module-level `def fsum(...)` would parse identically.
    """
    tree = ast.parse((PACKAGE / module).read_text(encoding="utf-8"))
    imported = any(
        isinstance(n, ast.ImportFrom) and n.module == "math"
        and any(a.name == "fsum" and a.asname is None for a in n.names)
        for n in ast.walk(tree))
    assert imported, f"{module} calls fsum without `from math import fsum`"
    shadowed = [n.name for n in ast.walk(tree)
                if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))
                and n.name == "fsum"]
    assert not shadowed, f"{module} defines its own fsum"
