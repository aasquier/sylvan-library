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

from mtglab.decks.model import Deck

DECKS_DIR = Path("decks")
DATA_DIR = Path("data")
DB_PATH = DATA_DIR / "mtg.duckdb"


def _load(slug: str) -> Deck:
    path = DECKS_DIR / slug / "deck.yaml"
    if not path.exists():
        sys.exit(f"no deck at {path}")
    return Deck.load(path)


def _corpus(deck: Deck):
    """Look up every card in the deck. Returns None if the DB is absent, so
    callers can degrade to structural checks with a visible warning."""
    if not DB_PATH.exists():
        return None
    from mtglab.cards import db
    con = db.connect(DB_PATH)
    names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    if deck.companion:
        names.append(deck.companion)
    return db.get_cards(con, names)


# --------------------------------------------------------------------- data

def cmd_data_refresh(args):
    from mtglab.cards import db
    con = db.connect(DB_PATH)
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
    con = db.connect(DB_PATH)
    n = db.snapshot_prices(con)
    print(f"snapshotted {n:,} prices for today")
    con.close()


# -------------------------------------------------------------------- decks

def deck_paths(decks_dir: Path = DECKS_DIR) -> list[Path]:
    """Every real deck file, newest-name-first order aside. `_template` and any
    other underscore-prefixed directory is scaffolding, not a deck."""
    if not decks_dir.exists():
        return []
    return [p for p in sorted(decks_dir.glob("*/deck.yaml"))
            if not p.parent.name.startswith("_")]


def load_all_decks(decks_dir: Path = DECKS_DIR) -> list[Deck]:
    """Shared by the CLI and the API so both see exactly the same library."""
    return [Deck.load(p) for p in deck_paths(decks_dir)]


def cmd_decks_list(args):
    if not DECKS_DIR.exists():
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

    outdir = DECKS_DIR / args.slug / "artifacts"
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

def enters_tapped(oracle_text: str) -> bool:
    """Whether a land unconditionally enters tapped.

    Scryfall retemplated this: current oracle text reads "This land enters
    tapped", not "enters the battlefield tapped". Matching only the old
    wording silently treated every modern tapland as untapped, which
    overstates early mana for every deck.

    Conditional lands are deliberately treated as untapped. Tier 1 cannot
    evaluate "unless you control a Forest" or a shock land's "you may pay 2
    life", and in practice those resolve untapped in most real games; calling
    them tapped would systematically slow every deck instead.
    """
    text = (oracle_text or "").lower()
    if "enters tapped" not in text and "enters the battlefield tapped" not in text:
        return False
    return not ("unless" in text or "you may pay" in text)


def fetches_lands(oracle_text: str) -> int:
    """How many lands a spell puts onto the battlefield from the library.

    Nature's Lore, Three Visits, Skyshroud Claim and Sakura-Tribe Elder are
    ramp that produces no mana of its own. Without this they compile to blank
    cards, which understates the deck's acceleration and skews the land-count
    recommendation.
    """
    text = (oracle_text or "").lower()
    if "search your library" not in text or "onto the battlefield" not in text:
        return 0
    if not any(w in text for w in ("land", "forest", "swamp", "island",
                                   "mountain", "plains")):
        return 0
    return 2 if ("two" in text or "up to two" in text) else 1


def _sim_cards(deck: Deck, cards):
    """Compile a deck into SimCards. Requires the corpus for mana production."""
    from mtglab.mana import ManaSource, parse_mana_cost
    from mtglab.sim.tier1.engine import SimCard

    if cards is None:
        sys.exit("simulation needs the card corpus -- run `mtglab data refresh` first")

    def compile_one(name: str) -> SimCard | None:
        rec = cards.get(name)
        if rec is None:
            return None
        # Only permanents stay on the battlefield making mana. Scryfall reports
        # produced_mana for Treasure-makers like Deadly Dispute too, and
        # without this guard an instant compiles into a permanent mana source.
        front = rec.type_line.split(" // ")[0]
        is_permanent = not ("Instant" in front or "Sorcery" in front)
        produced = frozenset(p for p in rec.produced_mana if p in "WUBRGC")
        produces = (ManaSource(produced),) if (produced and is_permanent) else ()
        is_creature = "Creature" in rec.type_line
        # A fetchland sacrifices itself, so it is net-zero lands and must not
        # count here -- only spells that add a land to the board do.
        fetch = 0 if rec.is_land else fetches_lands(rec.oracle_text)
        return SimCard(
            name=rec.name,
            cost=parse_mana_cost(rec.mana_cost),
            is_land=rec.is_land,
            enters_tapped=rec.is_land and enters_tapped(rec.oracle_text),
            produces=produces,
            produce_delay=1 if (produces and is_creature and not rec.is_land) else 0,
            fetches_lands=fetch,
        )

    # Expand by qty. Basics carry qty 8-16, so ignoring it simulated a deck of
    # ~83 cards with ~20 lands instead of 99 with 34 -- which made every
    # mulligan rate and land-count recommendation wrong.
    library = []
    for entry in deck.cards:
        compiled = compile_one(entry.name)
        if compiled is not None:
            library.extend([compiled] * entry.qty)
    commander = compile_one(deck.commander[0]) if deck.commander else None
    return library, commander


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
    if not DB_PATH.exists():
        sys.exit("run `mtglab data refresh` first")
    deck = _load(args.slug)
    con = db.connect(DB_PATH)
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
