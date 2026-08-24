"""Every committed recipe, against every committed asset.

This is the test that closes the hole ADR 29 names: both PROVENANCE.md files
described a scripted pipeline that was never committed, so the reproducibility
they claimed was a claim nobody could check. Now the recipes are committed,
this holds on every checkout and in CI: the assets on disk match the contract
in their recipes -- dimensions, budgets, the count of 78, and no metadata.

**It used to hold two of twelve.** `ambience` and `tarot` were named here by
hand, and the ten recipes added since -- the wheel's menagerie, the seance
parchment, the Learn bookworm, the three ambience motions -- were verified by
nothing at all. Reachability was checked (the glob can see them) and mistaken
for verification, which is this project's most-repeated bug wearing recipe
YAML: a guard that fails silently reads as protection. The sweep below asks
the same glob `animist verify` asks, so a recipe committed tomorrow is
covered the day it lands rather than the day somebody remembers to add a
function here. It costs about three seconds.

Pure file reads: no network, no scratch dir, nothing written. If an asset is
ever hand-replaced or a recipe drifts, this is the test that says so.
"""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from animist.recipe import load_recipe
from animist.verify import verify_recipe

# tools/tests/ sits two levels below the checkout, and the committed assets
# live in the checkout -- the tarot plates at the path the Go door's
# MTGLAB_TAROT_DIR default names (assets/tarot), the frontend's under web/. The sweep in
# `animist.cli` anchors on the same root, so these tests and the CLI agree
# about where a recipe can hide.
REPO = Path(__file__).resolve().parents[2]
AMBIENCE = REPO / "web" / "src" / "assets" / "ambience" / "ambience.recipe.yaml"
TAROT = (REPO / "assets" / "tarot"
         / "tarot.recipe.yaml")


def test_the_ambience_recipe_validates_and_its_assets_hold() -> None:
    recipe = load_recipe(AMBIENCE)
    assert recipe.slug == "ambience"
    assert [out.file for out in recipe.outputs] == [
        "ivy-canopy.webp", "ivy-sprig-1.webp", "ivy-sprig-2.webp",
        "ivy-sprig-3.webp"]
    assert verify_recipe(recipe) == []


def test_the_tarot_recipe_validates_and_all_78_hold() -> None:
    recipe = load_recipe(TAROT)
    source = recipe.sources["rws"]
    # The trap guard is part of the record, not just the prose.
    assert source.require_filename_prefix == "RWS1909"
    (output,) = recipe.outputs
    assert output.each and output.expect.count == 78
    assert verify_recipe(recipe) == []


def _all_recipes() -> list[Path]:
    from animist.cli import _animist_all_recipes

    return _animist_all_recipes()


@pytest.mark.parametrize("path", _all_recipes(), ids=lambda p: p.name.removesuffix(".recipe.yaml"))
def test_every_committed_recipe_holds_its_own_assets(path: Path) -> None:
    """The sweep. Parametrised rather than looped so a drifted asset names
    itself in the failure line instead of hiding in a list of twelve."""
    assert verify_recipe(load_recipe(path)) == []


def test_the_sweep_covers_every_recipe_in_the_tree() -> None:
    """The sweep is only worth having if the glob really is exhaustive.

    `_animist_all_recipes` walks two asset roots; a recipe committed anywhere
    else would be missed by the sweep *and* by the CLI, silently. This asks
    the repository instead of the glob.
    """
    tracked = {(REPO / line).resolve() for line in subprocess.run(
        ["git", "ls-files", "*.recipe.yaml"], cwd=REPO,
        capture_output=True, text=True, check=True).stdout.split()}
    assert tracked, "no recipes tracked -- this test has stopped measuring"
    assert tracked <= {p.resolve() for p in _all_recipes()}


def test_every_committed_recipe_is_reachable_by_verify_all() -> None:
    # `animist verify` with no arguments globs these roots; a recipe
    # committed somewhere the glob cannot see is a recipe nobody verifies.
    from animist.cli import _animist_all_recipes

    found = [path.resolve() for path in _animist_all_recipes()]
    assert TAROT.resolve() in found
    assert AMBIENCE.resolve() in found   # the sweep runs from the checkout
