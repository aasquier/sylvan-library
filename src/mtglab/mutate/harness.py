"""Break the code on purpose, run the tests, and count what nobody noticed.

`references/white.md` has prescribed this by hand for a while -- flip a
comparison, off-by-one a boundary, drop a guard, run the affected tests, count
the survivors -- with `git checkout --` as the harness. This is that protocol
with two things fixed.

**The working tree is never touched.** The manual version edits the real file
and restores it, so an interrupted run leaves a mutation in the tree and the
next `git status` is the only thing standing between that and a commit. Here
the package is copied to a scratch directory once, mutated *there*, and pytest
is pointed at the copy with `-o pythonpath=`. Nothing can go wrong in a way
that reaches the repository, which is what makes it safe to run a hundred.

**The sample is seeded.** A kill rate you cannot reproduce is a number that
can only ever be quoted, never checked -- and the ledger wants a figure that
trends across runs rather than a fresh anecdote each time.

Two honesty rules the report keeps. It says **which tests ran**, because a
survivor of six test files is a weaker claim than a survivor of the whole
suite. And a survivor is a **question, not a verdict**: some mutations are
semantically equivalent to the original and no test can kill them. Reading the
survivors is the work; this only finds them.
"""

from __future__ import annotations

import random
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path

from mtglab.mutate.operators import Mutation, find

#: How long one mutation's tests may run before it is declared killed.
#:
#: Set from a real hang rather than by taste. The first sample this harness
#: ever drew wedged for ten minutes against a 900-second default, because
#: **a mutation is a good way to write an infinite loop**: widen a `<` to a
#: `<=` at a loop bound and the condition stops being reachable. A timeout is
#: a kill -- a suite that hangs has certainly noticed -- so the only thing a
#: generous limit buys is a run nobody can sit through. The mapped test files
#: take single-digit seconds each; a full run takes about ninety.
TIMEOUT_SECONDS = 180.0

#: The modules worth mutating, and the tests that ought to defend them. Named
#: rather than discovered: mutating the whole package would spend most of its
#: time on CLI printing, and the point is the load-bearing middle -- the mana
#: solver, the gate, the simulator, and everything holding an account shut.
TARGETS: dict[str, tuple[str, ...]] = {
    "mtglab/mana.py": ("tests/test_mana.py", "tests/test_mana_properties.py"),
    "mtglab/colors.py": ("tests/test_colors.py",),
    "mtglab/decks/validate.py": ("tests/test_decks_and_artifacts.py",
                                 "tests/test_companion.py",
                                 "tests/test_partners.py"),
    "mtglab/decks/companion.py": ("tests/test_companion.py",),
    "mtglab/decks/partners.py": ("tests/test_partners.py",),
    "mtglab/decks/edit.py": ("tests/test_edit.py",),
    "mtglab/decks/analyze.py": ("tests/test_analyze.py",),
    "mtglab/decks/suggest.py": ("tests/test_suggest.py",),
    "mtglab/sim/tier1/engine.py": ("tests/test_sim_tier1.py",
                                   "tests/test_determinism.py"),
    "mtglab/sim/compile.py": ("tests/test_sim_compile.py",),
    "mtglab/sim/cache.py": ("tests/test_sim_cache.py",),
    "mtglab/auth/users.py": ("tests/test_auth.py", "tests/test_isolation.py"),
    "mtglab/auth/sessions.py": ("tests/test_auth.py",),
    "mtglab/auth/tokens.py": ("tests/test_auth_tokens.py",),
    "mtglab/auth/ratelimit.py": ("tests/test_auth.py",),
    "mtglab/cards/identify.py": ("tests/test_identify.py",),
    # The two runtime shelves. Added 2026-08-19 by the White run that noticed
    # the map had no entry for the module deciding whether somebody else's
    # WebAssembly runs in a browser: `ocr.py`'s digest check, size cap and
    # sticky refusal set are exactly the kind of guard this tool exists to
    # break on purpose, and none of them was ever sampled.
    "mtglab/ocr.py": ("tests/test_ocr.py",),
    "mtglab/symbols.py": ("tests/test_symbols.py",),
}


@dataclass(frozen=True)
class Result:
    """One mutation, applied and judged."""

    mutation: Mutation
    killed: bool
    tests: tuple[str, ...]
    seconds: float
    #: The tail of pytest's output when the mutation survived, so the report
    #: can show that the tests really did pass rather than error out.
    tail: str = ""


@dataclass
class Report:
    """What a sample found, and how much of the suite it asked."""

    results: list[Result] = field(default_factory=list)
    #: Sites the sample was drawn from, so a kill rate has a denominator that
    #: means something.
    sites: int = 0
    seed: int = 0
    full: bool = False

    @property
    def killed(self) -> int:
        return sum(1 for r in self.results if r.killed)

    @property
    def survivors(self) -> list[Result]:
        return [r for r in self.results if not r.killed]

    @property
    def kill_rate(self) -> float | None:
        return self.killed / len(self.results) if self.results else None


def missing_tests(root: Path | None = None) -> list[tuple[str, str]]:
    """Entries in `TARGETS` naming a test file that is not there.

    Pinned by `tests/test_mutate.py`, and the reason is a bug this harness
    shipped with for exactly one run: a mapping is a *narrowing*, so a typo in
    a filename used to fall through to running the whole suite. Every mutation
    then still got judged and the kill rate still looked plausible -- it just
    took 165 seconds each instead of one, and the report said `ran: tests`
    where nobody was reading. Silent widening, in a tool whose entire job is
    to find guards that fail silently.
    """
    base = root or Path.cwd()
    return [(module, test) for module, tests in TARGETS.items()
            for test in tests if not (base / test).exists()]


def catalogue(src: Path, targets: dict[str, tuple[str, ...]] | None = None,
              ) -> list[Mutation]:
    """Every mutation available across every declared target."""
    out: list[Mutation] = []
    for relpath in (targets or TARGETS):
        path = src / relpath
        if not path.exists():
            continue
        module = relpath.removesuffix(".py").replace("/", ".")
        out.extend(find(path.read_text(encoding="utf-8"), module=module,
                        relpath=relpath))
    return out


def shadow(src: Path, into: Path) -> Path:
    """A throwaway copy of the package, ready to be broken.

    `web_dist` and `assets` are symlinked rather than copied: together they are
    twelve megabytes of built bundle and public-domain paintings, they hold no
    Python, and a run that copied them per invocation would spend more time on
    the tarot deck than on the tests.
    """
    dst = into / "mtglab"
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(src / "mtglab", dst,
                    ignore=shutil.ignore_patterns("__pycache__", "web_dist",
                                                  "assets"))
    for share in ("web_dist", "assets"):
        origin = (src / "mtglab" / share).resolve()
        if origin.exists():
            (dst / share).symlink_to(origin)
    return into


def run(*, sample: int = 12, seed: int = 0, full: bool = False,
        src: Path | None = None, workdir: Path | None = None,
        targets: dict[str, tuple[str, ...]] | None = None,
        timeout: float = TIMEOUT_SECONDS,
        on_result: object = None) -> Report:
    """Draw a seeded sample of mutations, apply each, and see who notices."""
    src = src or Path(__file__).resolve().parents[2]
    work = Path(workdir) if workdir is not None else None
    if work is None:
        import tempfile
        work = Path(tempfile.mkdtemp(prefix="mtglab-mutate-"))
    root = shadow(src, work)

    available = catalogue(src, targets)
    rng = random.Random(seed)
    chosen = (list(available) if sample >= len(available)
              else rng.sample(available, sample))
    chosen.sort(key=lambda m: (m.relpath, m.line, m.col))

    report = Report(sites=len(available), seed=seed, full=full)
    for mutation in chosen:
        report.results.append(_judge(mutation, root=root, src=src, full=full,
                                     timeout=timeout,
                                     targets=targets or TARGETS))
        if callable(on_result):
            on_result(report.results[-1])
    return report


def _judge(mutation: Mutation, *, root: Path, src: Path, full: bool,
           timeout: float, targets: dict[str, tuple[str, ...]]) -> Result:
    target = root / mutation.relpath
    pristine = (src / mutation.relpath).read_text(encoding="utf-8")
    # One map decides both which sites exist and which tests defend them. They
    # were two for one run, and a caller narrowing the catalogue got the whole
    # suite run against it anyway -- correct, and forty times slower, and
    # invisible in a report that only says the mutation was killed.
    tests = ("tests",) if full else targets.get(mutation.relpath, ("tests",))

    start = time.perf_counter()
    try:
        target.write_text(mutation.apply(pristine), encoding="utf-8")
        proc = subprocess.run(
            [sys.executable, "-m", "pytest", "-x", "-q", "-p",
             "no:cacheprovider", "-o", f"pythonpath={root}", *tests],
            capture_output=True, text=True, timeout=timeout, check=False,
            cwd=Path.cwd())
        killed, tail = proc.returncode != 0, proc.stdout[-600:]
    except ValueError as exc:
        # `apply` refused: the file moved under the catalogue. Not a kill and
        # not a survival -- a broken sample, reported as such.
        killed, tail = True, f"mutation could not be applied: {exc}"
    except subprocess.TimeoutExpired:
        killed, tail = True, f"tests timed out after {timeout:.0f}s"
    finally:
        # Always, and always from the pristine original rather than from a
        # remembered string: the shadow is the only thing that was edited, and
        # it goes back to what the repository says even if the run is dying.
        target.write_text(pristine, encoding="utf-8")

    return Result(mutation=mutation, killed=killed, tests=tests,
                  seconds=time.perf_counter() - start,
                  tail="" if killed else tail)
