"""animist command line.

    animist build <recipe>       wake a still image into scenery:
                                 fetch, licence-check, transform, encode
    animist verify [recipe ...]  committed assets vs their recipes;
                                 no arguments means every committed one
    animist licence <recipe>     the gate alone -- what may pass, dated
    animist measure <recipe>     the size curve, and its knee

Extracted from the app's `mtglab animist` family when the Python app retired
and the picture tooling survived (the same commands, one word shorter). The
handlers and their argparse wiring cross unchanged: same flags, same prints,
same refusals -- the only sentences that moved are the ones that named the
old package's extras, which no longer exist to install.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:  # pragma: no cover
    from collections.abc import Sequence

    from animist.recipe import Recipe


def _animist_recipe(path_str: str | Path) -> Recipe:
    """Load and validate, or refuse with the schema's own sentence."""
    from animist.recipe import RecipeError, load_recipe

    try:
        return load_recipe(Path(path_str))
    except RecipeError as exc:
        sys.exit(f"refused: {exc}")


def _animist_pixels() -> None:
    """The moment pixels get touched is the moment the codec is required."""
    import animist

    if not animist.available():
        sys.exit("Pillow is not importable -- it is a core dependency of "
                 "this toolbox, so the install is broken; reinstall it: "
                 "`pip install -e tools`")


def _animist_widths(spec: str) -> list[int]:
    """`300:600:20` (start:stop:step) or `300,400,560` (a list)."""
    try:
        if ":" in spec:
            start, stop, step = (int(part) for part in spec.split(":"))
            widths = list(range(start, stop + 1, step))
        else:
            widths = [int(part) for part in spec.split(",") if part]
    except ValueError:
        sys.exit(f"refused: could not read --widths {spec!r} -- give "
                 "start:stop:step or a comma-separated list")
    if len(widths) < 3:
        sys.exit("refused: a size curve needs at least three widths, or "
                 "there is no knee to find")
    return widths


def _animist_crfs(spec: str) -> list[int]:
    """`24:48:4` (start:stop:step) or `24,32,40` -- the video sweep's axis."""
    try:
        if ":" in spec:
            start, stop, step = (int(part) for part in spec.split(":"))
            crfs = list(range(start, stop + 1, step))
        else:
            crfs = [int(part) for part in spec.split(",") if part]
    except ValueError:
        sys.exit(f"refused: could not read --crfs {spec!r} -- give "
                 "start:stop:step or a comma-separated list")
    if len(crfs) < 3:
        sys.exit("refused: a crf curve needs at least three points, or "
                 "there is no knee to find")
    return crfs


def cmd_animist_build(args: argparse.Namespace) -> None:
    """The whole pipeline for one recipe. The gate runs first and blocks."""
    _animist_pixels()
    from animist.encode import EncodeError
    from animist.ops import OpError
    from animist.run import BuildError, build
    from animist.sources import LicenceRefused, SourceError

    recipe = _animist_recipe(args.recipe)
    try:
        result = build(recipe, only=args.output, dry_run=args.dry_run)
    except (LicenceRefused, SourceError, BuildError, OpError,
            EncodeError) as exc:
        sys.exit(f"refused: {exc}" if not str(exc).startswith("refused")
                 else str(exc))
    for name in result.skipped:
        print(f"  skipped   {name} (filename guard)")
    for name, size in sorted(result.sizes.items()):
        mark = "would write" if result.dry_run else "wrote"
        print(f"  {mark:11s} {recipe.directory / name}  ({size / 1024:.1f} KB)")
    if result.provenance is not None:
        print(f"  provenance {result.provenance}")
    if result.dry_run:
        print("  dry run -- nothing touched the asset directory")


def cmd_animist_fetch(args: argparse.Namespace) -> None:
    """Gate, then warm the cache. No pixels are transformed."""
    from animist.fetch import fetch_source
    from animist.sources import LicenceRefused, SourceError

    recipe = _animist_recipe(args.recipe)
    try:
        for source in recipe.sources.values():
            confirmed, local = fetch_source(recipe, source)
            proof = confirmed.confirmation
            print(f"  {source.id}: {proof.licence} "
                  f"(confirmed {proof.checked_on}), "
                  f"{len(local)} file(s) cached")
            for name in confirmed.skipped:
                print(f"    skipped {name} (filename guard)")
    except (LicenceRefused, SourceError) as exc:
        sys.exit(f"refused: {exc}")


def cmd_animist_licence(args: argparse.Namespace) -> None:
    """The gate alone: every source's confirmed licence, dated. No download."""
    from animist.sources import LicenceRefused, SourceError, confirm

    recipe = _animist_recipe(args.recipe)
    failed = False
    for source in recipe.sources.values():
        try:
            confirmed = confirm(source)
        except (LicenceRefused, SourceError) as exc:
            print(f"  {source.id}: REFUSED -- {exc}")
            failed = True
            continue
        proof = confirmed.confirmation
        print(f"  {source.id}: {proof.licence} -- confirmed "
              f"{proof.checked_on} via {proof.api_url}")
        if confirmed.skipped:
            print(f"    would skip {len(confirmed.skipped)} file(s) by the "
                  "filename guard")
    if failed:
        sys.exit(1)


def _animist_all_recipes() -> list[Path]:
    """Every committed recipe, anchored to the checkout this toolbox sits in.

    The app resolved its half of the sweep off the installed package and the
    frontend's half off the working directory; with both trees living in one
    repository the anchor is simpler and stronger -- `tools/animist/cli.py`
    is two directories below the repo root, so the sweep answers the same
    from any CWD. The tarot root travels with the Go door's own default
    (`MTGLAB_TAROT_DIR` falls back to the same path), so the day those
    assets move, both lists move together or neither does -- and the repo
    sweep test fails until this list follows.
    """
    repo = Path(__file__).resolve().parents[2]
    roots = [repo / "assets",
             repo / "web" / "src" / "assets"]
    found: list[Path] = []
    for root in roots:
        if root.is_dir():
            found.extend(sorted(root.rglob("*.recipe.yaml")))
    return found


def cmd_animist_verify(args: argparse.Namespace) -> None:
    """Committed assets against their recipes. The suite runs this too."""
    _animist_pixels()
    from animist.verify import verify_recipe

    paths = [Path(r) for r in args.recipes] if args.recipes \
        else _animist_all_recipes()
    if not paths:
        sys.exit("refused: no recipes named and none found -- a committed "
                 "asset without a recipe is what ADR 29 forbids")
    failures = []
    for path in paths:
        recipe = _animist_recipe(path)
        held = verify_recipe(recipe)
        print(f"  {recipe.path}: " + ("held" if not held
                                      else f"{len(held)} failure(s)"))
        failures.extend(held)
    for failure in failures:
        print(f"    {failure}")
    if failures:
        sys.exit(1)


def cmd_animist_measure(args: argparse.Namespace) -> None:
    """The size curve for one output, and where its knee sits.

    For a still, runs the output's ops *except* any `resize`, then encodes
    at each width in the sweep -- the question is what the final resize
    should be, so the recipe's own answer is held out. For a video output
    the axis is `crf` instead of width (ADR 31): the derivation runs as
    written and the sweep asks what the rate control should be.
    """
    _animist_pixels()
    from animist.fetch import fetch_source
    from animist.measure import crf_curve, knee, size_curve
    from animist.ops import apply
    from animist.recipe import VIDEO_FORMATS
    from animist.sources import LicenceRefused, SourceError

    recipe = _animist_recipe(args.recipe)
    named = [out for out in recipe.outputs if out.file == args.output]
    if not named:
        sys.exit(f"refused: {recipe.path.name} has no `file:` output named "
                 f"{args.output!r} -- `measure` sweeps one file at a time")
    (output,) = named

    if output.encode.format in VIDEO_FORMATS:
        from animist.run import _derive

        crfs = _animist_crfs(args.crfs)
        source = recipe.sources[output.source]
        try:
            _, local = fetch_source(recipe, source)
        except (LicenceRefused, SourceError) as exc:
            sys.exit(f"refused: {exc}")
        original = next(iter(local.values())) if local else None
        sequence = _derive(original, output, source)
        curve = crf_curve(sequence, output.encode, crfs)
        for crf, size in curve:
            print(f"  crf {crf:3d}  {size / 1024:8.1f} KB")
        elbow = knee(curve)
        print(f"  the knee is at crf {elbow} -- the numbers' half of the "
              "answer; look at the result before trusting it")
        return

    widths = _animist_widths(args.widths)
    from PIL import Image

    try:
        _, local = fetch_source(recipe, recipe.sources[output.source])
    except (LicenceRefused, SourceError) as exc:
        sys.exit(f"refused: {exc}")
    with Image.open(next(iter(local.values()))) as image:
        image.load()
        current: Image.Image = image
        for op in output.ops:
            if op.name != "resize":
                current = apply(current, op.name, op.params)
        curve = size_curve(current, output.encode, widths)
    for width, size in curve:
        print(f"  {width:5d}px  {size / 1024:8.1f} KB")
    elbow = knee(curve)
    print(f"  the knee is at {elbow}px -- the numbers' half of the answer; "
          "look at the result before trusting it")


def main(argv: Sequence[str] | None = None) -> None:
    p = argparse.ArgumentParser(prog="animist", description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    an = p.add_subparsers(dest="cmd", required=True)
    ab = an.add_parser("build", help="fetch, gate, transform, encode, and "
                                     "write the provenance entry")
    ab.add_argument("recipe", help="path to a *.recipe.yaml")
    ab.add_argument("--output", help="build only this `file:` output")
    ab.add_argument("--dry-run", action="store_true",
                    help="report what would be written, write nothing")
    ab.set_defaults(func=cmd_animist_build)
    af = an.add_parser("fetch", help="licence-check and cache the originals; "
                                     "transform nothing")
    af.add_argument("recipe"); af.set_defaults(func=cmd_animist_fetch)
    al = an.add_parser("licence", help="the gate alone -- each source's "
                                       "confirmed licence, dated")
    al.add_argument("recipe"); al.set_defaults(func=cmd_animist_licence)
    av = an.add_parser("verify", help="committed assets vs their recipes: "
                                      "dimensions, budgets, count, no metadata")
    av.add_argument("recipes", nargs="*",
                    help="recipe paths; none means every committed recipe")
    av.set_defaults(func=cmd_animist_verify)
    am = an.add_parser("measure", help="the size curve for one output, and "
                                       "its knee")
    am.add_argument("recipe")
    am.add_argument("--output", required=True, help="the `file:` output to sweep")
    am.add_argument("--widths", default="300:600:20",
                    help="start:stop:step, or a comma-separated list")
    am.add_argument("--crfs", default="24:48:4",
                    help="the sweep for a video output, where the axis is "
                         "crf rather than width")
    am.set_defaults(func=cmd_animist_measure)

    args = p.parse_args(argv)
    args.func(args)


if __name__ == "__main__":
    main()
