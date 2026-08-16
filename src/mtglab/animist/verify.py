"""Committed assets against their recipes. Reads everything, writes nothing.

This is the half of ADR 9's committed-bundle discipline that survives contact
with image encoders. The bundle can be regenerated in CI and diffed because
Vite's output is byte-reproducible; WebP across libwebp versions is not, so
byte-diffing an asset would fail on the next encoder release with nothing
actually wrong. What *is* stable is the contract: the dimensions the
component was measured against, the byte budget the page can afford, the
count a set must keep, and the absence of metadata. `expect` blocks state
that contract and this module holds committed files to it.

Failures are sentences naming the file and both values, collected rather
than raised, so one broken asset does not hide the second.
"""

from __future__ import annotations

from pathlib import Path

from mtglab.animist.encode import has_metadata
from mtglab.animist.recipe import Expect, Output, Recipe


def _check_file(path: Path, expect: Expect, *,
                enforce_height: bool = True) -> list[str]:
    from PIL import Image, UnidentifiedImageError

    if not path.exists():
        return [f"{path}: missing -- the recipe expects it committed"]
    failures: list[str] = []
    data = path.read_bytes()
    try:
        with Image.open(path) as image:
            width, height = image.size
    except UnidentifiedImageError:
        return [f"{path}: cannot be decoded as an image"]
    if expect.width is not None and width != expect.width:
        failures.append(f"{path}: width {width}, recipe expects {expect.width}")
    if enforce_height and expect.height is not None and height != expect.height:
        failures.append(f"{path}: height {height}, recipe expects "
                        f"{expect.height}")
    if expect.budget_kb is not None and len(data) > expect.budget_kb * 1024:
        failures.append(f"{path}: {len(data):,} bytes is over the "
                        f"{expect.budget_kb} KB budget")
    if expect.metadata_none:
        found = has_metadata(data)
        if found:
            failures.append(f"{path}: carries metadata the pipeline strips "
                            f"({', '.join(found)}) -- was this file "
                            "hand-replaced?")
    return failures


def _set_members(recipe: Recipe, output: Output) -> list[Path]:
    """The files an `each:` output owns: every encoded file in the recipe
    directory not named by a `file:` output of the same recipe."""
    claimed = {out.file for out in recipe.outputs if out.file}
    suffix = f".{output.encode.format}"
    return sorted(path for path in recipe.directory.glob(f"*{suffix}")
                  if path.name not in claimed)


def verify_output(recipe: Recipe, output: Output) -> list[str]:
    if output.file:
        return _check_file(recipe.directory / output.file, output.expect)
    members = _set_members(recipe, output)
    failures: list[str] = []
    expect = output.expect
    if expect.count is not None and len(members) != expect.count:
        failures.append(
            f"{recipe.directory}: the `{output.source}` set holds "
            f"{len(members)} files, recipe expects exactly {expect.count} -- "
            "a glob that half-matches is worse than one that fails")
    total = 0
    for path in members:
        total += path.stat().st_size
        failures.extend(_check_file(path, expect, enforce_height=False))
    if (expect.total_budget_kb is not None
            and total > expect.total_budget_kb * 1024):
        failures.append(f"{recipe.directory}: the set totals {total:,} bytes, "
                        f"over the {expect.total_budget_kb} KB budget")
    return failures


def verify_recipe(recipe: Recipe) -> list[str]:
    """Every failure in every output, in output order. Empty means held."""
    failures: list[str] = []
    for output in recipe.outputs:
        failures.extend(verify_output(recipe, output))
    return failures
