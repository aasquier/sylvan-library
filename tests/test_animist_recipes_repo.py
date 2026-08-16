"""The two committed recipes, against the two committed asset sets.

This is the test that closes the hole ADR 29 names: both PROVENANCE.md files
described a scripted pipeline that was never committed, so the reproducibility
they claimed was a claim nobody could check. Now the recipes are committed,
this holds on every checkout and in CI: the assets on disk match the contract
in their recipes -- dimensions, budgets, the count of 78, and no metadata.

Pure file reads: no network, no scratch dir, nothing written. If an asset is
ever hand-replaced or a recipe drifts, this is the test that says so.
"""

from __future__ import annotations

from pathlib import Path

import mtglab
from mtglab.animist.recipe import load_recipe
from mtglab.animist.verify import verify_recipe

REPO = Path(__file__).resolve().parents[1]
AMBIENCE = REPO / "web" / "src" / "assets" / "ambience" / "ambience.recipe.yaml"
TAROT = (Path(mtglab.__file__).parent / "assets" / "tarot"
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


def test_every_committed_recipe_is_reachable_by_verify_all() -> None:
    # `mtglab animist verify` with no arguments globs these roots; a recipe
    # committed somewhere the glob cannot see is a recipe nobody verifies.
    from mtglab.cli import _animist_all_recipes

    found = [path.resolve() for path in _animist_all_recipes()]
    assert TAROT.resolve() in found
    if AMBIENCE.exists():          # absent in an installed package, present
        assert AMBIENCE.resolve() in found   # in every checkout -- CI is one
