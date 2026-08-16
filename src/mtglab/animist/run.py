"""Orchestration: a recipe, start to finish, in the only order allowed.

fetch -> gate -> transform -> encode -> budget -> provenance. The gate is
inside `fetch_source`, before any image byte moves, so a refused source
leaves both the cache and the asset directory exactly as found — the
licence check cannot be reordered past the work by a later edit here
without deleting the call that does the work.

This is the one module that imports the whole package. Everything it calls
is seam-tested on its own; what the tests here pin is the choreography —
that a build writes what it says, that `--dry-run` writes nothing, and that
a budget refusal happens before a byte lands.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path

from mtglab.animist.encode import encode
from mtglab.animist.fetch import Downloader, fetch_source
from mtglab.animist.ops import apply
from mtglab.animist.provenance import render, update
from mtglab.animist.recipe import Output, Recipe
from mtglab.animist.sources import Confirmed, Transport


class BuildError(RuntimeError):
    """A build that must not complete: over budget, or nothing to do."""


@dataclass
class Built:
    """What one build produced (or, dry, would have)."""

    written: list[Path] = field(default_factory=list)
    sizes: dict[str, int] = field(default_factory=dict)
    skipped: tuple[str, ...] = ()
    provenance: Path | None = None
    dry_run: bool = False


def _slug_name(upstream: str, fmt: str) -> str:
    """`RWS1909 The Star.jpg` -> `rws1909-the-star.webp`."""
    stem = Path(upstream).stem.lower()
    cleaned = re.sub(r"[^a-z0-9]+", "-", stem).strip("-") or "asset"
    return f"{cleaned}.{fmt}"


def _budget_check(name: str, size: int, output: Output) -> None:
    budget = output.expect.budget_kb
    if budget is not None and size > budget * 1024:
        raise BuildError(
            f"refused: {name} came out at {size:,} bytes, over the recipe's "
            f"{budget} KB budget -- lower the quality, shrink the target, or "
            "raise the budget deliberately, but a committed asset does not "
            "drift past its own recipe")


def _derive(original: Path, output: Output) -> bytes:
    from PIL import Image

    with Image.open(original) as image:
        image.load()
        current: Image.Image = image
        for op in output.ops:
            current = apply(current, op.name, op.params)
        return encode(current, output.encode)


def build(recipe: Recipe, *, only: str | None = None, dry_run: bool = False,
          transport: Transport | None = None,
          download: Downloader | None = None) -> Built:
    """Run the pipeline for every output (or the one named by `only`)."""
    outputs = [out for out in recipe.outputs
               if only is None or out.file == only]
    if not outputs:
        raise BuildError(f"refused: {recipe.path.name} has no output named "
                         f"{only!r}")

    # One gate run and one fetch per source, however many outputs draw on it.
    confirmations: dict[str, Confirmed] = {}
    originals: dict[str, dict[str, Path]] = {}
    for out in outputs:
        if out.source not in confirmations:
            confirmed, local = fetch_source(
                recipe, recipe.sources[out.source],
                transport=transport, download=download)
            confirmations[out.source] = confirmed
            originals[out.source] = local

    result = Built(dry_run=dry_run)
    produced: dict[str, list[str]] = {}
    for out in outputs:
        local = originals[out.source]
        if out.file:
            # A `file:` output derives from its source's first original --
            # single-file providers only ever have one.
            targets = {out.file: next(iter(local.values()))}
        else:
            targets = {_slug_name(name, out.encode.format): path
                       for name, path in sorted(local.items())}
        for name, original in targets.items():
            data = _derive(original, out)
            _budget_check(name, len(data), out)
            result.sizes[name] = len(data)
            produced.setdefault(out.source, []).append(name)
            if not dry_run:
                target = recipe.directory / name
                target.write_bytes(data)
                result.written.append(target)

    skipped = tuple(name for confirmed in confirmations.values()
                    for name in confirmed.skipped)
    result.skipped = skipped
    if not dry_run:
        entry = render(recipe, confirmations, produced)
        result.provenance = update(recipe, entry)
    return result
