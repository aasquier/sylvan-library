"""cardmotion command line.

    cardmotion build --deck <slug> | --card <name>   derive one card's motion
    cardmotion sync                                  every deck's commander vs
                                                     the cache; build the missing
    cardmotion status                                every cached derivative

Extracted from the app's `mtglab cardmotion` family when the Python app
retired and the picture tooling survived (the same commands, one word
shorter). The handlers and their argparse wiring cross unchanged: same
flags, same prints, same refusals. What changed underneath is stated where
it lives -- the pool opens read-only (`cardmotion/pool.py`) and a deck is
read three fields wide (`cardmotion/decks.py`).
"""

from __future__ import annotations

import argparse
import sys
from typing import TYPE_CHECKING

from animist import config
from cardmotion import decks

if TYPE_CHECKING:  # pragma: no cover
    from collections.abc import Sequence

    from cardmotion.depth import DepthModel


def _animist_pixels() -> None:
    """The moment pixels get touched is the moment the codec is required.

    A local copy of `animist.cli`'s helper rather than an import of it, so
    the two console scripts stay independent doors over the shared engine.
    """
    import animist

    if not animist.available():
        sys.exit("Pillow is not importable -- it is a core dependency of "
                 "this toolbox, so the install is broken; reinstall it: "
                 "`pip install -e tools`")


def _load(slug: str) -> decks.Deck:
    path = config.DECKS_DIR / slug / "deck.yaml"
    if not path.exists():
        sys.exit(f"no deck at {path}")
    return decks.load(path)


def load_all_decks() -> list[decks.Deck]:
    """Every deck the sweep can see, through the shared path rules."""
    return [decks.load(p) for p in config.deck_paths()]


def cmd_cardmotion_build(args: argparse.Namespace) -> None:
    """Derive a card's motion (ADR 32): pool facts, Scryfall art, cached
    derivative. A dev-machine run -- the app only ever serves the cache."""
    _animist_pixels()
    from cardmotion.build import BuildRefused, build_derivative

    if not config.DB_PATH.exists():
        sys.exit("no card pool -- run `mtglab data refresh` first; card "
                 "facts come from the pool (rule 1)")
    if bool(args.deck) == bool(args.card):
        sys.exit("refused: name exactly one of --deck (its commander's art) "
                 "or --card")
    art_id = None
    if args.deck:
        if getattr(args, "art", None):
            sys.exit("refused: --art rides with --card; a deck names its "
                     "own printing")
        deck = _load(args.deck)
        if not deck.commander:
            sys.exit(f"refused: {args.deck} has no commander to animate")
        card = deck.commander[0]
        # The deck's chosen printing, so the loop is derived from the
        # painting the deck page actually shows -- the serving tier matches
        # on the art and would (rightly) refuse a mismatch.
        art_id = getattr(deck, "commander_art", "") or None
    else:
        card = args.card
        # A page that pins a particular painting (the About gallery) needs
        # the derivative built from that printing, for the same reason a
        # deck does.
        art_id = getattr(args, "art", None) or None

    model = None
    from cardmotion.effects import EFFECTS
    chosen = EFFECTS.get(args.effect)
    if chosen is not None and chosen.needs_depth:
        from cardmotion.depth import DepthError, load_model
        try:
            print("loading the depth model (first run downloads weights)...")
            model = load_model()
        except DepthError as exc:
            sys.exit(f"refused: {exc}")

    from cardmotion import pool
    con = pool.connect(config.DB_PATH)
    try:
        entry = build_derivative(con, card=card, effect_key=args.effect,
                                 model=model, art_id=art_id)
    except BuildRefused as exc:
        sys.exit(f"refused: {exc}")
    meta = entry.attribution()
    print(f"  {meta['card_name']} -- {args.effect} "
          f"(art by {meta['artist']})")
    for path in sorted(entry.directory.iterdir()):
        print(f"  wrote {path} ({path.stat().st_size / 1024:.0f} KB)")
    print("  deployed instances receive this over sftp -- HOSTING has the "
          "runbook line")


def cmd_cardmotion_sync(args: argparse.Namespace) -> None:
    """Every deck's commander vs the cache: build what is missing, from the
    printing each deck actually shows. The catch-all for imported decks and
    art swaps alike -- deployed instances then receive the cache over sftp,
    same runbook line as `build`."""
    _animist_pixels()
    from cardmotion.build import BuildRefused, sync

    if not config.DB_PATH.exists():
        sys.exit("no card pool -- run `mtglab data refresh` first; card "
                 "facts come from the pool (rule 1)")
    from cardmotion.effects import EFFECTS
    chosen = EFFECTS.get(args.effect)
    if chosen is None:
        sys.exit(f"refused: unknown effect {args.effect!r} "
                 f"(one of: {', '.join(sorted(EFFECTS))})")

    def load_model_once() -> DepthModel:
        from cardmotion.depth import DepthError, load_model
        try:
            print("loading the depth model (first run downloads weights)...")
            return load_model()
        except DepthError as exc:
            raise BuildRefused(str(exc)) from exc

    from cardmotion import pool
    con = pool.connect(config.DB_PATH)
    all_decks = load_all_decks()
    report = sync(con, all_decks, effect_key=args.effect,
                  model_loader=load_model_once)
    for slug in report.present:
        print(f"  {slug:<24} already breathes")
    for slug in report.built:
        print(f"  {slug:<24} built")
    for slug in report.skipped:
        print(f"  {slug:<24} no commander -- skipped")
    for line in report.refused:
        print(f"  refused: {line}")
    if report.built:
        print("  deployed instances receive this over sftp -- HOSTING has "
              "the runbook line")
    if report.refused:
        sys.exit(1)


def cmd_cardmotion_status(args: argparse.Namespace) -> None:
    """Every cached derivative: card, effect, artist, when."""
    from cardmotion import cache

    base = cache.root()
    entries = sorted(base.iterdir()) if base.is_dir() else []
    shown = 0
    for directory in entries:
        derivative = cache.CachedDerivative(directory)
        if not derivative.ready:
            continue
        meta = derivative.attribution()
        size = sum(p.stat().st_size for p in directory.iterdir())
        print(f"  {meta['card_name']:<30} {meta['effect']:<12} "
              f"{size / 1024:6.0f} KB  (art by {meta['artist']}, "
              f"{meta['generated_at'][:10]})")
        shown += 1
    if not shown:
        print("  nothing derived yet -- `cardmotion build --deck "
              "<slug> --effect depth-drift` is the first run")


def main(argv: Sequence[str] | None = None) -> None:
    p = argparse.ArgumentParser(prog="cardmotion", description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    # Card-art motion (ADR 32): derived at runtime into the gitignored
    # cache, never committed -- the runtime counterpart of the animist.
    cm = p.add_subparsers(dest="cmd", required=True)
    cb = cm.add_parser("build", help="derive one card's motion into the "
                                     "cache: pool facts, Scryfall art, "
                                     "dual-format loop")
    cb.add_argument("--deck", help="a deck slug; its commander's art")
    cb.add_argument("--card", help="or a card by name")
    cb.add_argument("--art", help="with --card: a printing id (the UUID in "
                                  "a Scryfall image URL), when the painting "
                                  "wanted is not the pool's default")
    cb.add_argument("--effect", default="depth-drift",
                    help="depth-drift (needs the depth extra), slow-pan, "
                         "or breath")
    cb.set_defaults(func=cmd_cardmotion_build)
    cy = cm.add_parser("sync", help="every deck's commander vs the cache: "
                                    "build what is missing, from the "
                                    "printing each deck shows")
    cy.add_argument("--effect", default="depth-drift",
                    help="depth-drift (needs the depth extra) or slow-pan")
    cy.set_defaults(func=cmd_cardmotion_sync)
    cs = cm.add_parser("status", help="every cached derivative")
    cs.set_defaults(func=cmd_cardmotion_status)

    args = p.parse_args(argv)
    args.func(args)


if __name__ == "__main__":
    main()
