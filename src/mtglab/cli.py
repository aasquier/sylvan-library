"""mtglab command line.

    mtglab data refresh                 pull Scryfall bulk + load DuckDB
    mtglab data snapshot                append today's prices to history
    mtglab decks list
    mtglab decks validate <slug>        the gate -- run before anything else
    mtglab decks build <slug>           generate the artifacts
    mtglab sim mana <slug>              Tier 1 goldfish
    mtglab sim lands <slug> 30..40      land-count sweep, flood-aware
    mtglab price deck <slug>            cheapest legal printing per card
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from mtglab import config
from mtglab.decks.model import Deck
from mtglab.sim.compile import (
    CorpusRequired,
    compile_deck,
    enters_tapped,
    fetches_lands,
)

# Re-exported for callers that still import them from here. The definitions
# live in `config` so the API does not have to import the CLI, and so tests can
# point them at a scratch directory.
deck_paths = config.deck_paths

__all__ = ["main", "deck_paths", "load_all_decks",
           "enters_tapped", "fetches_lands"]


def _load(slug: str) -> Deck:
    path = config.DECKS_DIR / slug / "deck.yaml"
    if not path.exists():
        sys.exit(f"no deck at {path}")
    return Deck.load(path)


def _corpus(deck: Deck):
    """Look up every card in the deck. Returns None if the DB is absent, so
    callers can degrade to structural checks with a visible warning."""
    if not config.DB_PATH.exists():
        return None
    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    if deck.companion:
        names.append(deck.companion)
    return db.get_cards(con, names)


# --------------------------------------------------------------------- data

def cmd_data_refresh(args):
    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    print("downloading oracle_cards ...")
    oracle = db.download_bulk("oracle_cards")
    print(f"  {oracle}")
    n = db.load_oracle(con, oracle)
    print(f"  loaded {n:,} oracle cards")
    if not args.oracle_only:
        print("downloading default_cards (large) ...")
        default = db.download_bulk("default_cards")
        print(f"  {default}")
        m = db.load_printings(con, default)
        print(f"  loaded {m:,} printings")
    con.close()


def cmd_data_snapshot(args):
    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    n = db.snapshot_prices(con)
    print(f"snapshotted {n:,} prices for today")
    con.close()


# -------------------------------------------------------------------- decks

def load_all_decks(decks_dir: Path | None = None) -> list[Deck]:
    """Shared by the CLI and the API so both see exactly the same library."""
    return [Deck.load(p) for p in config.deck_paths(decks_dir)]


def cmd_decks_list(args):
    if not config.DECKS_DIR.exists():
        sys.exit("no decks/ directory")
    for deck in load_all_decks():
        cmd = ", ".join(deck.commander) or "?"
        bracket = f"B{deck.bracket}" if deck.bracket else "B?"
        print(f"  {deck.slug:<22} {bracket:<4} {deck.total_cards:>3} cards   {cmd}")


def cmd_decks_validate(args):
    from mtglab.decks.validate import validate
    deck = _load(args.slug)
    rep = validate(deck, _corpus(deck))
    print(rep.render())
    print(f"\n{len(rep.errors)} error(s), {len(rep.warnings)} warning(s)")
    sys.exit(1 if rep.errors else 0)


def cmd_decks_build(args):
    from mtglab.artifacts.generate import write_all
    from mtglab.decks.validate import validate

    deck = _load(args.slug)
    cards = _corpus(deck)
    rep = validate(deck, cards)
    if rep.errors and not args.force:
        print(rep.render())
        sys.exit(f"\nrefusing to generate with {len(rep.errors)} error(s). "
                 "Fix them, or pass --force if you know better.")
    if rep.warnings:
        print(rep.render(), "\n")

    previous = None
    if args.against:
        previous = Deck.load(Path(args.against))

    outdir = config.DECKS_DIR / args.slug / "artifacts"
    written = write_all(deck, outdir, cards=cards, previous=previous)
    for path in written:
        print(f"  wrote {path}")


# ----------------------------------------------------------------------- ui

def cmd_ui(args):
    """Serve the local app."""
    try:
        import uvicorn
    except ModuleNotFoundError:
        sys.exit("the UI needs the api extra:  pip install -e '.[api]'")

    from mtglab.api.app import WEB_DIST, create_app

    if not WEB_DIST.is_dir() and not args.dev:
        print(f"warning: no built frontend at {WEB_DIST}")
        print("         build it with `npm --prefix web run build`, or use "
              "--dev with the Vite server running")

    url = f"http://{args.host}:{args.port}"
    print(f"sylvan-library -> {url}")
    if not args.no_open:
        # Open after a beat so the server is accepting connections. A browser
        # that lands on a refused connection shows an error page and does not
        # retry, which reads as "the app is broken".
        import threading
        import webbrowser
        threading.Timer(1.2, lambda: webbrowser.open(url)).start()

    uvicorn.run(create_app(dev=args.dev), host=args.host, port=args.port,
                log_level="info")


# ---------------------------------------------------------------------- sim
#
# enters_tapped, fetches_lands and the deck compiler live in
# `mtglab.sim.compile` -- they are simulator concerns, and the API needs them
# without importing the command line. Imported above and re-exported.

def _sim_cards(deck: Deck, cards):
    """CLI wrapper: turn a missing corpus into a clean exit rather than a
    traceback. Library callers should use `compile_deck` and catch
    `CorpusRequired`."""
    try:
        return compile_deck(deck, cards)
    except CorpusRequired as exc:
        sys.exit(str(exc))


def cmd_sim_mana(args):
    from mtglab.sim.tier1.engine import KeepRule, run
    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _corpus(deck))
    rule = KeepRule(min_lands=args.min_lands, max_lands=args.max_lands,
                    min_mana_pieces=args.min_pieces)
    print(run(library, commander, games=args.games, turns=args.turns,
              keep_rule=rule, seed=args.seed).report())


def cmd_sim_lands(args):
    from mtglab.sim.tier1.engine import run
    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _corpus(deck))
    lands = [c for c in library if c.is_land]
    spells = [c for c in library if not c.is_land]
    if not lands:
        sys.exit("deck has no lands to sweep")

    print(" lands  P(cmdr T5)  spells thru T8  wasted thru T8  mull%")
    for n in range(args.low, args.high + 1):
        # Resize by cycling the existing land pool, preserving its colour mix.
        resized = [lands[i % len(lands)] for i in range(n)]
        lib = resized + spells[: len(library) - n]
        s = run(lib, commander, games=args.games, turns=10, seed=args.seed)
        print(f" {n:>5}     {s.commander_by_turn[5]:>6.1%}          "
              f"{s.spells_through(8):>5.2f}           {s.wasted_through(8):>5.2f}"
              f"          {s.mulligan_rate:>4.1%}")
    print("\nPick the land count where 'spells thru T8' plateaus -- past that "
          "you are buying commander speed with flood.")


# -------------------------------------------------------------------- price

def cmd_price_deck(args):
    from mtglab.cards import db
    if not config.DB_PATH.exists():
        sys.exit("run `mtglab data refresh` first")
    deck = _load(args.slug)
    con = db.connect(config.DB_PATH)
    names = [c.name for c in deck.cards] + deck.commander
    rows = con.execute("""
        SELECT name, min(price_usd) FROM printings
        WHERE lower(name) IN (SELECT unnest(?)) AND price_usd IS NOT NULL
          AND NOT promo
        GROUP BY name ORDER BY 2 DESC
    """, [[n.lower() for n in names]]).fetchall()
    total = sum(r[1] for r in rows)
    for name, price in rows:
        print(f"  {price:>8.2f}  {name}")
    print(f"\n  {total:>8.2f}  TOTAL ({len(rows)}/{len(names)} priced)")


# --------------------------------------------------------------------- main

def main(argv=None):
    p = argparse.ArgumentParser(prog="mtglab", description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="group", required=True)

    data = sub.add_parser("data").add_subparsers(dest="cmd", required=True)
    r = data.add_parser("refresh"); r.add_argument("--oracle-only", action="store_true")
    r.set_defaults(func=cmd_data_refresh)
    data.add_parser("snapshot").set_defaults(func=cmd_data_snapshot)

    decks = sub.add_parser("decks").add_subparsers(dest="cmd", required=True)
    decks.add_parser("list").set_defaults(func=cmd_decks_list)
    v = decks.add_parser("validate"); v.add_argument("slug")
    v.set_defaults(func=cmd_decks_validate)
    b = decks.add_parser("build"); b.add_argument("slug")
    b.add_argument("--against", help="path to a previous deck.yaml, to emit swaps.md")
    b.add_argument("--force", action="store_true")
    b.set_defaults(func=cmd_decks_build)

    sim = sub.add_parser("sim").add_subparsers(dest="cmd", required=True)
    m = sim.add_parser("mana"); m.add_argument("slug")
    m.add_argument("--games", type=int, default=20000)
    m.add_argument("--turns", type=int, default=12)
    m.add_argument("--min-lands", type=int, default=2)
    m.add_argument("--max-lands", type=int, default=5)
    m.add_argument("--min-pieces", type=int, default=3)
    m.add_argument("--seed", type=int, default=None)
    m.set_defaults(func=cmd_sim_mana)
    ld = sim.add_parser("lands"); ld.add_argument("slug")
    ld.add_argument("low", type=int); ld.add_argument("high", type=int)
    ld.add_argument("--games", type=int, default=5000)
    ld.add_argument("--seed", type=int, default=7)
    ld.set_defaults(func=cmd_sim_lands)

    ui = sub.add_parser("ui")
    ui.add_argument("--port", type=int, default=8765)
    ui.add_argument("--host", default="127.0.0.1")
    ui.add_argument("--no-open", action="store_true")
    ui.add_argument("--dev", action="store_true",
                    help="allow CORS from the Vite dev server on :5173")
    ui.add_argument("--reload", action="store_true")
    ui.set_defaults(func=cmd_ui)

    price = sub.add_parser("price").add_subparsers(dest="cmd", required=True)
    pd = price.add_parser("deck"); pd.add_argument("slug")
    pd.set_defaults(func=cmd_price_deck)

    args = p.parse_args(argv)
    args.func(args)


if __name__ == "__main__":
    main()
