"""No ignore rule may shadow a directory the source lives in.

`.gitignore` carried `decks/` for ADR 30's sake -- decks are the app's live
data and not repository content -- and a bare directory pattern in gitignore
matches **at every depth**. So it also matched `src/mtglab/decks/`, the deck
source package: thirteen modules, all of them tracked, all of them fine,
because an ignore rule cannot untrack what git already follows.

That is precisely why it was invisible. Nothing was broken today; what was
broken was tomorrow. `git add src/mtglab/decks/newthing.py` refuses outright,
and `git add -A` -- which this repository forbids for a different reason --
skips it without a word. The failure would have surfaced in CI as an
ImportError for a module the diff does not mention, and ADR 28's own note
says when to expect it: "the tenth edit operation is the one somebody adds in
a year".

The fix was one character, `/decks/`. This test is the part worth keeping: it
asks git itself whether a *new* file in each directory the repository already
holds source in would be tracked, which is the question the eye cannot answer
by reading a pattern list. Every future ignore rule is checked against every
such directory, so the next too-broad pattern fails here instead of in a year.
"""

from __future__ import annotations

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

#: Names a probe file must not itself trip -- otherwise the test measures the
#: probe rather than the directory.
PROBE = "zzz_gitignore_probe.py"


def directories() -> list[Path]:
    """Every directory git already tracks a file in.

    Derived from `git ls-files` rather than from a list of trees, and that is
    the whole reliability of this test. A hand-kept list needs an exclusion
    for `src/mtg_lab.egg-info/` and one for `.claude/worktrees/` -- both
    correctly ignored, neither source -- and an exclusion list is a place for
    the next real offender to be waved through. "Git tracks something here"
    is the same claim without the maintenance: a generated directory has no
    tracked file, and a source directory always does.
    """
    listed = subprocess.run(["git", "ls-files"], cwd=ROOT,
                            capture_output=True, text=True, check=True)
    return sorted({ROOT / Path(line).parent
                   for line in listed.stdout.splitlines() if line})


def ignored(paths: list[Path]) -> dict[str, str]:
    """`git check-ignore` over every candidate at once, path -> the rule."""
    result = subprocess.run(
        ["git", "check-ignore", "--verbose", "--no-index", "--stdin"],
        cwd=ROOT, input="\n".join(str(p) for p in paths),
        capture_output=True, text=True)
    # Exit 1 simply means nothing matched, which is the passing case.
    if result.returncode not in (0, 1):
        raise AssertionError(result.stderr)
    hits = {}
    for line in result.stdout.splitlines():
        source, _, pattern_and_path = line.partition(":")
        pattern, _, path = pattern_and_path.rpartition("\t")
        hits[path] = f"{source} -> {pattern.rpartition(':')[2]}"
    return hits


def test_there_are_directories_to_check() -> None:
    """A sweep over an empty list passes forever and measures nothing."""
    assert len(directories()) > 20


def test_no_source_directory_is_inside_an_ignored_one() -> None:
    probes = [d / PROBE for d in directories()]
    hits = ignored(probes)
    assert not hits, (
        "these source directories cannot accept a new file:\n"
        + "\n".join(f"  {path}\n      ignored by {rule}"
                    for path, rule in sorted(hits.items())))


def test_the_app_data_directory_is_still_ignored() -> None:
    """The other half: anchoring the rule must not have unignored the decks.

    `decks/` at the repository root is live app data (ADR 30) and committing
    one is how two diverging copies start. A fix to the pattern above that
    quietly let it back in would be worse than the bug.
    """
    hits = ignored([ROOT / "decks" / "gyome-food" / "deck.yaml"])
    assert hits, "the app's deck directory is no longer ignored"
