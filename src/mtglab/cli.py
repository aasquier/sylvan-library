"""mtglab command line.

    mtglab data refresh                 pull Scryfall bulk + load DuckDB
    mtglab data snapshot                append today's prices to history
    mtglab decks list
    mtglab decks import <slug> --from   a pasted decklist -> a draft deck
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
from mtglab.decks.model import CATEGORIES, Deck
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
        # Only drafts are flagged. Curated is the norm and labelling it would
        # make the one thing worth noticing harder to see.
        draft = f"  draft, {len(deck.unjustified)} to justify" \
            if deck.stage == "draft" else ""
        print(f"  {deck.slug:<22} {bracket:<4} {deck.total_cards:>3} cards   "
              f"{cmd}{draft}")


def cmd_decks_validate(args):
    from mtglab.decks.validate import validate
    deck = _load(args.slug)
    rep = validate(deck, _corpus(deck))
    print(rep.render())
    print(f"\n{len(rep.errors)} error(s), {len(rep.warnings)} warning(s)")
    sys.exit(1 if rep.errors else 0)


def cmd_decks_import(args):
    """Bring a decklist in as a draft.

    Shares its implementation with the API, so a list imported in the terminal
    and the same list imported in the app produce the same deck file.
    """
    from mtglab.api import service

    text = sys.stdin.read() if args.source == "-" else \
        Path(args.source).read_text(encoding="utf-8")

    try:
        result = service.import_deck(
            text=text, slug=args.slug, name=args.name or "",
            commander=args.commander, companion=args.companion or "",
            bracket=args.bracket, status=args.status, dry_run=args.dry_run)
    except service.ImportRejected as exc:
        sys.exit(f"refused: {exc}")

    if args.dry_run:
        print("dry run -- nothing was written.\n")

    print(f"  {result['name']} ({result['slug']})")
    print(f"  commander: {', '.join(result['commander'])}"
          + (f"   companion: {result['companion']}" if result["companion"] else ""))
    print(f"  {result['total_cards']} cards in the 99, "
          f"{result['land_count']} lands, "
          f"{len(result['swap_board'])} on the swap board")

    for note in result["notes"]:
        print(f"  note: {note}")
    if result["unknown"]:
        print(f"\n  {len(result['unknown'])} name(s) the corpus does not know. "
              "Kept exactly as written -- nothing was guessed:")
        for name in result["unknown"]:
            print(f"    {name}")
    if result["unreadable"]:
        print(f"\n  {len(result['unreadable'])} line(s) could not be read:")
        for line in result["unreadable"]:
            print(f"    line {line['line']}: {line['text']}")
    if result["skipped"]:
        print(f"\n  {len(result['skipped'])} token line(s) skipped.")

    print(f"\n  gate: {len(result['errors'])} error(s), "
          f"{len(result['warnings'])} warning(s)")
    for issue in result["errors"]:
        card = f"[{issue['card']}] " if issue["card"] else ""
        print(f"    {issue['code']}: {card}{issue['message']}")

    print(f"\n  This deck is a DRAFT. {result['needs_rationale']} card(s) still "
          "need a `why`;\n  write them in deck.yaml, then set `stage: curated`. "
          "Artifacts stay\n  blocked until then, and nothing will write a "
          "rationale for you.")


def cmd_decks_suggest(args):
    """Shortlist replacements for a card that has to leave the deck.

    Informational, so it exits 0 even when the deck is broken. `decks validate`
    is the gate; this is the thing you run afterwards to get unstuck.
    """
    from mtglab.cards import db
    from mtglab.decks import suggest
    from mtglab.decks.validate import validate

    deck = _load(args.slug)
    if not config.DB_PATH.exists():
        sys.exit("suggestions need the card corpus -- run `mtglab data refresh`")

    con = db.connect(config.DB_PATH)
    names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    cards = db.get_cards(con, names)

    if args.card:
        targets = [(args.card, "asked for")]
    else:
        rep = validate(deck, cards)
        # Only the errors a different card would actually fix. A missing `why`
        # is a real error and swapping the card does not resolve it.
        targets = [(i.card, i.code) for i in rep.errors
                   if i.card and i.code in ("banned", "color-identity")]

    if not targets:
        print(f"{deck.slug}: nothing to replace -- the gate reports no card "
              "that a swap would fix.")
        return

    print("Ranked by measurable similarity to the card being replaced -- type, "
          "mana value,\nkeywords, oracle text, then EDHREC rank as a tiebreak. "
          "This is a shortlist to\nargue with, not a recommendation; the choice "
          "is yours.\n")

    for name, code in targets:
        why = next((c.why for c in deck.cards if c.name == name), "")
        header = f"  {name} ({code})"
        print(header if not why else f"{header} — {why.strip()}")

        candidates = suggest.replacements_for(deck, cards, con, name,
                                              limit=args.limit)
        if not candidates:
            print("      no candidates -- the corpus does not know this card.\n")
            continue
        for i, cand in enumerate(candidates, 1):
            cost = cand.record.mana_cost or ""
            print(f"    {i}. {cand.name:<32} {cost:<14} {cand.score:.2f}")
            print(f"       {' · '.join(cand.reasons)}")
        print()


def _report_edit(result):
    """What every deck edit prints: the gate, then the reminder to commit.

    An edit that did not re-run the gate would be a deck changed and
    unchecked, so the verdict is not optional output.
    """
    print(f"\n  gate: {len(result['errors'])} error(s), "
          f"{len(result['warnings'])} warning(s)")
    for issue in result["errors"] + result["warnings"]:
        card = f"[{issue['card']}] " if issue["card"] else ""
        print(f"    {issue['code']}: {card}{issue['message']}")
    if result.get("stage") == "draft" and result.get("needs_rationale"):
        print(f"\n  draft: {result['needs_rationale']} card(s) still owe a `why`.")
    print("\n  deck.yaml is the source of truth and its history is git history:\n"
          "  commit this, then `mtglab decks build` --against the previous "
          "revision for swaps.md.")


def cmd_decks_swap(args):
    """Apply a swap the user has already decided on.

    Shares its implementation with the API rather than duplicating it, so the
    terminal and the app can never disagree about what a swap does.
    """
    from mtglab.api import service

    try:
        result = service.swap_card(args.slug, out=args.out, into=getattr(args, "in"),
                                   why=args.why)
    except service.SwapRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  {result['swapped_out']}  ->  {result['swapped_in']}")
    _report_edit(result)


def cmd_decks_add(args):
    from mtglab.api import service

    try:
        result = service.add_card(args.slug, name=args.card, category=args.category,
                                  why=args.why or "", qty=args.qty, to=args.to)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    where = "the swap board" if args.to == "swap_board" else "the 99"
    qty = f"{args.qty}x " if args.qty != 1 else ""
    print(f"  + {qty}{result['added']}  ({result['category']}) -> {where}")
    _report_edit(result)


def cmd_decks_remove(args):
    from mtglab.api import service

    try:
        result = service.remove_card(args.slug, name=args.card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  - {result['removed']}")
    _report_edit(result)


def cmd_decks_set(args):
    """Change one field of one card -- including its `why`.

    Exactly one field per invocation, matching the operation underneath. The
    rationale is taken verbatim from the argument: nothing here writes one, and
    a blank one on a curated deck is refused rather than filled in (rule 4).
    """
    from mtglab.api import service

    chosen = [(f, v) for f, v in (("why", args.why), ("category", args.category),
                                  ("qty", args.qty)) if v is not None]
    if len(chosen) != 1:
        sys.exit("choose exactly one of --why, --category, --qty")
    field, value = chosen[0]

    try:
        result = service.set_card_field(args.slug, name=args.card, field=field,
                                        value=value)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  {result['card']}: {field} set")
    _report_edit(result)


def cmd_decks_note(args):
    from mtglab.api import service

    value = args.value
    if args.from_file:
        value = Path(args.from_file).read_text(encoding="utf-8")

    try:
        result = service.set_note(args.slug, key=args.key, value=value or "")
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  note {result['note']!r} set")
    _report_edit(result)


def cmd_decks_build(args):
    from mtglab.artifacts.generate import DraftDeck, write_all
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
    try:
        written = write_all(deck, outdir, cards=cards, previous=previous)
    except DraftDeck as exc:
        # No --force here on purpose: see the note on `write_all`. The way out
        # of a draft is to write the rationales, not to pass a flag.
        sys.exit(f"refusing to generate: {exc}")
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
    i = decks.add_parser("import", help="bring a decklist in as a draft")
    i.add_argument("slug", help="directory name under decks/")
    i.add_argument("--from", dest="source", required=True,
                   help="path to a decklist, or - for stdin")
    i.add_argument("--name", help="display name; defaults to the slug")
    i.add_argument("--commander", action="append", default=[],
                   help="repeat for a partner pair; overrides the list's own")
    i.add_argument("--companion", help="sits outside the 100")
    i.add_argument("--bracket", type=int)
    i.add_argument("--status", default="theoretical",
                   choices=("built", "theoretical"),
                   help="whether the cards physically exist")
    i.add_argument("--dry-run", action="store_true",
                   help="resolve and gate the list without writing anything")
    i.set_defaults(func=cmd_decks_import)
    g = decks.add_parser("suggest"); g.add_argument("slug")
    g.add_argument("--card", help="replace this card, instead of the gate's offenders")
    g.add_argument("--limit", type=int, default=5)
    g.set_defaults(func=cmd_decks_suggest)
    w = decks.add_parser("swap"); w.add_argument("slug")
    w.add_argument("--out", required=True, help="the card leaving the deck")
    w.add_argument("--in", required=True, dest="in", help="the card replacing it")
    w.add_argument("--why", required=True,
                   help="why the new card earns the slot; the gate requires one")
    w.set_defaults(func=cmd_decks_swap)
    a = decks.add_parser("add", help="add a card to the 99 or the swap board")
    a.add_argument("slug"); a.add_argument("--card", required=True)
    a.add_argument("--category", required=True,
                   help=f"one of: {', '.join(CATEGORIES)}")
    a.add_argument("--why", help="why the card earns its slot; required unless "
                                 "the deck is a draft")
    a.add_argument("--qty", type=int, default=1)
    a.add_argument("--to", default="cards", choices=["cards", "swap_board"])
    a.set_defaults(func=cmd_decks_add)
    rm = decks.add_parser("remove", help="take a card out")
    rm.add_argument("slug"); rm.add_argument("--card", required=True)
    rm.set_defaults(func=cmd_decks_remove)
    st = decks.add_parser("set", help="change one field of one card")
    st.add_argument("slug"); st.add_argument("--card", required=True)
    st.add_argument("--why", help="the rationale, in your words")
    st.add_argument("--category", help=f"one of: {', '.join(CATEGORIES)}")
    st.add_argument("--qty", type=int)
    st.set_defaults(func=cmd_decks_set)
    nt = decks.add_parser("note", help="set a deck-level note")
    nt.add_argument("slug"); nt.add_argument("--key", required=True)
    nt.add_argument("--value")
    nt.add_argument("--from-file", dest="from_file",
                    help="read the note's text from a file, for long prose")
    nt.set_defaults(func=cmd_decks_note)
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
