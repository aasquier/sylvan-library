"""mtglab command line.

    mtglab data refresh                 pull Scryfall bulk + load DuckDB
    mtglab data snapshot                append today's prices to history
    mtglab decks list
    mtglab decks import <slug> --from   a pasted decklist -> a draft deck
    mtglab decks validate <slug>        the gate -- run before anything else
    mtglab decks promote <slug>         a draft becomes curated, once justified
    mtglab decks delete <slug>          remove one; it moves to decks/.trash/
    mtglab decks log <slug>             what has been done to it, and by whom
    mtglab decks build <slug>           generate the artifacts
    mtglab sim mana <slug>              Tier 1 goldfish
    mtglab sim lands <slug> 30..40      land-count sweep, flood-aware
    mtglab price deck <slug>            cheapest legal printing per card
    mtglab users invite <email>         an account, and a link they claim it with
    mtglab users add <name>             an account; the password is prompted
    mtglab users list                   who exists, and who can log in
    mtglab users passwd <name>          set a password; ends every session
    mtglab users disable|enable <name>
    mtglab users promote|demote <name>  admin, and never the last one
    mtglab claude check                 one real API call -- is the key live?
    mtglab claude argue <slug>          the case against a card's slot
    mtglab claude dossier <slug>        who the commander is, with sources
    mtglab claude research "<q>"        a question the pool cannot answer,
                                        with the pages it read; takes no deck
    mtglab claude interview <slug>      questions about a card's slot; you
                            --card X    write the rationale, it never does
    mtglab claude usage                 what every mode has spent, in tokens
    mtglab animist build <recipe>       wake a still image into scenery:
                                        fetch, licence-check, transform, encode
    mtglab animist verify --all         committed assets vs their recipes
    mtglab animist licence <recipe>     the gate alone -- what may pass, dated
    mtglab animist measure <recipe>     the size curve, and its knee
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from mtglab import config
from mtglab.decks.model import CATEGORIES, DECK_STAGES, DECK_STATUSES, Deck
from mtglab.sim.compile import (
    PoolRequired,
    compile_deck,
    enters_tapped,
    fetches_lands,
)

# Re-exported for callers that still import them from here. The definitions
# live in `config` so the API does not have to import the CLI, and so tests can
# point them at a scratch directory.
deck_paths = config.deck_paths

__all__ = [
    "deck_paths",
    "enters_tapped",
    "fetches_lands",
    "load_all_decks",
    "main",
]


def _load(slug: str) -> Deck:
    path = config.DECKS_DIR / slug / "deck.yaml"
    if not path.exists():
        sys.exit(f"no deck at {path}")
    return Deck.load(path)


def _pool(deck: Deck):
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
    rep = validate(deck, _pool(deck))
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
        print(f"\n  {len(result['unknown'])} name(s) the pool does not know. "
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
        sys.exit("suggestions need the card pool -- run `mtglab data refresh`")

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
            print("      no candidates -- the pool does not know this card.\n")
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
    print("\n  deck.yaml is the source of truth; `mtglab decks build` records\n"
          "  it as the baseline, so the next build's swaps.md shows what this\n"
          "  edit changed.")


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

    if "entombed" in result:
        # ADR 27: a 99-card is entombed rather than deleted, and the file
        # keeps its rationale. Say where it went and how to walk it back.
        print(f"  ⚰ {result['entombed']} -> graveyard "
              f"(return with `mtglab decks return`, or `decks exile` for good)")
    else:
        print(f"  - {result['removed']}")
    _report_edit(result)


def cmd_decks_return(args):
    from mtglab.api import service

    try:
        result = service.return_card(args.slug, name=args.card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  + {result['returned']} <- graveyard, rationale intact")
    _report_edit(result)


def cmd_decks_exile(args):
    from mtglab.api import service

    try:
        result = service.exile_card(args.slug, name=args.card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  x {result['exiled']} exiled -- gone from the graveyard for good")
    _report_edit(result)


def _art_id(args, card=None):
    """Turn `--art <set-code>` into the printing id the deck file stores.

    A set code is what a person has to hand -- nobody knows a Scryfall UUID --
    but the deck file stores the id, because a set code is not unique: the
    Multiverse Legends printings of Goreclaw share `MUL` and are three
    different paintings. So a code matching several printings prints them and
    refuses, rather than picking one and being wrong about which art the user
    meant. `--art ''` clears the choice back to the default printing.

    With `card`, the same resolution runs against that card's printings
    instead of the commander's -- `--card X --art <set-code>` is how any of
    the 99 picks its painting.
    """
    from mtglab.api import service

    ref = args.art.strip()
    if not ref:
        return ""

    try:
        listing = service.commander_printings(args.slug, card=card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")
    printings = listing["printings"]
    if not printings:
        sys.exit(f"no printings found for {listing['commander'] or args.slug} "
                 f"-- is the pool loaded? (`mtglab data refresh`)")

    exact = [p for p in printings if p["id"] == ref]
    if exact:
        return exact[0]["id"]

    matches = [p for p in printings if p["set_code"] == ref.upper()]
    if not matches:
        codes = sorted({p["set_code"] for p in printings})
        sys.exit(f"{listing['commander']} has no printing in {ref.upper()!r}. "
                 f"It is in: {', '.join(codes)}")
    if len(matches) > 1:
        print(f"\n  {ref.upper()} has {len(matches)} printings of "
              f"{listing['commander']}. Pick one by its id:\n")
        for match in matches:
            print(f"    {match['id']}  #{match['collector_number']:<6} "
                  f"{match['rarity']:<9} {match['set_name']}")
        print()
        sys.exit("re-run with --art <id>")
    return matches[0]["id"]


def cmd_decks_set(args):
    """Change one field -- of a card with `--card`, of the deck without it.

    Exactly one field per invocation, matching the operation underneath. A
    rationale is taken verbatim from the argument: nothing here writes one, and
    a blank one on a curated deck is refused rather than filled in (rule 4).
    """
    from mtglab.api import service

    # `--art` reads the flags around it: with `--card` it dresses that card,
    # without one it dresses the commander -- the same word for the same act,
    # aimed by context rather than by a second flag.
    card_fields = (("why", args.why), ("category", args.category),
                   ("qty", args.qty),
                   ("art", _art_id(args, card=args.card)
                    if args.art is not None and args.card else None))
    deck_fields = (("stage", args.stage), ("status", args.status),
                   ("bracket", args.bracket), ("pilot", args.pilot),
                   ("commander_art", _art_id(args)
                    if args.art is not None and not args.card else None))
    chosen = [(f, v) for f, v in card_fields + deck_fields if v is not None]
    if len(chosen) != 1:
        sys.exit("choose exactly one of --why, --category, --qty, --art (with "
                 "--card) or --stage, --status, --bracket, --pilot, "
                 "--art (without)")
    field, value = chosen[0]
    on_a_card = field in dict(card_fields)

    if on_a_card and not args.card:
        sys.exit(f"--{field} changes a card; name it with --card")
    if not on_a_card and args.card:
        sys.exit(f"--{field} is a deck field, not a card's; drop --card")

    try:
        if on_a_card:
            result = service.set_card_field(args.slug, name=args.card,
                                            field=field, value=value)
            print(f"  {result['card']}: {field} set")
        else:
            result = service.set_deck_field(args.slug, field=field, value=value)
            print(f"  {args.slug}: {field} -> {value}")
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    _report_edit(result)


def cmd_decks_promote(args):
    """Promote a draft to curated -- the last step of an import.

    Refused while any card is still blank, and the refusal names them. That is
    the gate's rule enforced before the write rather than after it: promoting
    into a deck with 17 `missing-rationale` errors is a state nobody wants to
    be left holding.
    """
    from mtglab.api import service

    try:
        result = service.set_deck_field(args.slug, field="stage", value="curated")
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  {args.slug}: every card justifies its slot -- promoted to curated.")
    print("  `mtglab decks build` will now generate the five artifacts.")
    _report_edit(result)


def cmd_decks_delete(args):
    """Remove a deck from the library, recoverably.

    Interactive by default: it prints what is about to go and asks for a word
    back. `--yes` is for scripts, and it still has to name the slug on the
    command line, so there is no spelling of this that deletes a deck the
    caller did not type out.
    """
    from mtglab.api import service

    try:
        deck = service.get_deck(args.slug)
    except Exception as exc:                                       # noqa: BLE001
        sys.exit(f"refused: {exc}")

    print(f"  {deck['name']} ({args.slug})")
    print(f"  {deck['total_cards']} cards, {deck['stage']}, {deck['status']}")
    if not args.yes:
        # A typed word, not a y/n -- the answer has to be produced rather than
        # clicked past. The slug is still accepted and is the stronger of the
        # two, since only somebody looking at the right deck can produce it.
        typed = input(
            f"  type '{service.DELETE_WORD}' (or the slug) to delete it: ",
        ).strip()
    else:
        typed = args.slug

    try:
        result = service.delete_deck(slug=args.slug, confirm=typed)
    except service.DeleteRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"\n  deleted {result['slug']}.")
    print(f"  moved to {result['moved_to']} -- it is not gone, it is aside.")


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


def cmd_decks_log(args):
    """What has been done to this deck, newest first (ADR 28).

    Reads the file tier -- `owner_id IS NULL` -- because that is the only
    library the CLI ever edits. On a deployed instance the maintainer's own
    decks are that tier too, so `fly ssh console -C 'mtglab decks log gyome'`
    shows the same history the deck page does.

    `_load` first, so a slug that is not a deck says so rather than printing an
    empty history, which is the same fact wearing a misleading face.
    """
    from mtglab.decks import log

    deck = _load(args.slug)
    rows = log.entries(args.slug, limit=args.limit)
    if not rows:
        print(f"\n  {deck.name}: nothing recorded yet.")
        print("  Edits made before this log existed are in git, not here.\n")
        return

    capped = " (most recent; raise --limit for more)" \
        if len(rows) == args.limit else ""
    print(f"\n  {deck.name} -- {len(rows)} entries{capped}\n")
    for row in rows:
        # `None` is whoever is at this machine: the CLI itself, and the app
        # with auth off. Not an unknown actor -- an unnamed one.
        who = row["actor"] or "local"
        print(f"  {_when(row['created_at']):<17} {who:<14} {row['summary']}")
    print()


def _when(stamp):
    """An ISO-8601 UTC instant as something a terminal column can hold.

    Falls back to the raw string rather than raising: a row that cannot be
    parsed is still a row that says what happened, and a history that refuses
    to print because one timestamp is odd is worse than one ugly line.
    """
    from datetime import datetime

    try:
        return datetime.fromisoformat(stamp).strftime("%Y-%m-%d %H:%M")
    except (TypeError, ValueError):
        return str(stamp)


def cmd_decks_build(args):
    from mtglab.artifacts.generate import SNAPSHOT, DraftDeck, write_all
    from mtglab.decks.validate import validate

    deck = _load(args.slug)
    cards = _pool(deck)
    rep = validate(deck, cards)
    if rep.errors and not args.force:
        print(rep.render())
        sys.exit(f"\nrefusing to generate with {len(rep.errors)} error(s). "
                 "Fix them, or pass --force if you know better.")
    if rep.warnings:
        print(rep.render(), "\n")

    outdir = config.DECKS_DIR / args.slug / "artifacts"

    # The baseline for swaps.md, resolved before `write_all` moves it: an
    # explicit --against wins, and otherwise the previous build's snapshot is
    # the diff base (ADR 30 — build before editing, where it used to be
    # commit before editing). A first build has neither, and emits no swaps.md.
    previous = None
    if args.against:
        previous = Deck.load(Path(args.against))
    elif (outdir / SNAPSHOT).exists():
        previous = Deck.load(outdir / SNAPSHOT)

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
    """CLI wrapper: turn a missing pool into a clean exit rather than a
    traceback. Library callers should use `compile_deck` and catch
    `PoolRequired`."""
    try:
        return compile_deck(deck, cards)
    except PoolRequired as exc:
        sys.exit(str(exc))


def cmd_sim_mana(args):
    from mtglab.sim.tier1.engine import KeepRule, run
    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _pool(deck))
    rule = KeepRule(min_lands=args.min_lands, max_lands=args.max_lands,
                    min_mana_pieces=args.min_pieces)
    print(run(library, commander, games=args.games, turns=args.turns,
              keep_rule=rule, seed=args.seed).report())


def cmd_sim_lands(args):
    from mtglab.sim.tier1.engine import run
    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _pool(deck))
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


def cmd_sim_cache(args):
    """Inspect or empty the memoised Tier 1 results.

    Break-glass and a window, in that order. The cache is keyed on the compiled
    deck and a fingerprint of the engine's source, so it should never need
    clearing by hand -- but a cache nobody can see into or drop is a cache that
    has to be trusted rather than checked, and this project does not do that.

    Note the `enabled` line. "No rows" and "caching is switched off because the
    engine's source could not be fingerprinted" look identical from a count and
    want completely different responses.
    """
    from mtglab.sim import cache

    if args.clear:
        print(f"cleared {cache.clear()} cached result(s) from {config.APP_DB_PATH}")
        return

    info = cache.stats()
    print(f"store:   {config.APP_DB_PATH}")
    print(f"enabled: {'yes' if info['enabled'] else 'no -- results are not cached'}")
    print(f"rows:    {info['rows']} ({info['bytes'] / 1024:.1f} kB)")
    for kind, count in sorted(info["by_kind"].items()):
        print(f"  {kind:<18} {count}")
    if info["oldest"]:
        print(f"computed between {info['oldest'][:19]} and {info['newest'][:19]} UTC")


def cmd_sim_forge(args):
    """Tier 3: hand the decks to Forge and report what it played.

    `--check-only` is the coverage pre-flight on its own. It reads a zip and
    needs no JVM, so it is the cheap thing to run first and the only half of
    this command that works without the distribution installed.
    """
    from mtglab.sim.tier3 import run as forge
    from mtglab.sim.tier3.coverage import ForgeNotInstalled

    decks = [_load(slug) for slug in args.slugs]
    try:
        if args.check_only:
            for report in forge.check_coverage(decks):
                print(report.summary())
            return
        result = forge.run_games(decks, games=args.games, clock=args.clock,
                                 seed=args.seed)
    except (ForgeNotInstalled, forge.CoverageFailed,
            forge.ResultsUntrustworthy) as exc:
        sys.exit(str(exc))

    wins: dict[str, int] = {}
    for game in result.games:
        wins[result.winner_slug(game) or "draw"] = \
            wins.get(result.winner_slug(game) or "draw", 0) + 1

    played = [g.milliseconds / 1000 for g in result.games]
    print(f"{len(result.games)} games in {result.wall_seconds:.1f}s "
          f"({result.startup_seconds:.1f}s of it JVM + card database)")
    print(f"per game: {min(played):.1f}s min / "
          f"{sum(played) / len(played):.1f}s mean / {max(played):.1f}s max")
    for slug in args.slugs:
        print(f"  {slug:<22} {wins.get(slug, 0)}")
    if wins.get("draw"):
        print(f"  {'draw':<22} {wins['draw']}")
    clocked = sum(1 for g in result.games if g.timed_out)
    if clocked:
        # Never folded into the draw count: a clock-out is the measurement
        # giving up, not the game ending.
        print(f"  ({clocked} hit the {args.clock}s clock and were called draws)")
    print("\nForge's AI is best at aggro and midrange, poor at control and bad "
          "at most combo.\nRead these per archetype, not as one ranking.")


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


# -------------------------------------------------------------------- users
#
# ADR 16 is the rule these commands live under: **no password is ever chosen by
# one person for another.** The way an invited account gets a password is a
# single-use emailed link, and that is the next build -- there is no
# `--password` flag here and there will not be one, because a password passed
# as an argument is a password in the shell history and in the process table.
#
# `users invite` is that rule as a command: it creates the account and mails a
# single-use link, and the password that comes back is one nobody else has ever
# seen. `users add` remains for the maintainer's own account, typed at a prompt
# on a machine they are sitting at (`docs/HOSTING.md` §4 step 8), and for the
# unclaimed account an invite leaves behind when there is no mail provider yet.

def _auth():
    """The auth package, or a clean exit naming the extra that is missing."""
    try:
        from mtglab.auth import db, passwords, sessions, users
    except ModuleNotFoundError:
        # argon2-cffi rides with the api extra; see pyproject.toml.
        sys.exit("accounts need the api extra:  pip install -e '.[api]'")
    return db, passwords, sessions, users


def _connect():
    """`app.db`, with the maintainer reconciled to admin first (ADR 17).

    Every `users` command goes through here rather than calling `db.connect`,
    so the CLI and the app agree about who administers the instance no matter
    which one ran last. It is a no-op unless `MTGLAB_ADMIN_EMAIL` is set.
    """
    db, _, _, _ = _auth()
    from mtglab.auth import bootstrap

    con = db.connect()
    try:
        bootstrap.ensure_maintainer(con)
    except Exception:
        con.close()
        raise
    return con


def _prompt_new_password(who: str) -> str:
    """Read a password twice, from the terminal, never from an argument."""
    import getpass

    _, passwords, _, _ = _auth()
    first = getpass.getpass(f"  new password for {who}: ")
    if first != getpass.getpass("  again: "):
        sys.exit("refused: the two entries did not match")
    try:
        passwords.check_strength(first)
    except passwords.WeakPassword as exc:
        sys.exit(f"refused: {exc}")
    return first


def cmd_users_add(args):
    _db, _, _, users = _auth()

    password = None if args.no_password else _prompt_new_password(args.username)
    con = _connect()
    try:
        user = users.create(con, args.username, password=password,
                            email=args.email, is_admin=args.admin)
    except (users.UserExists, users.InvalidUsername,
            users.InvalidEmail) as exc:
        # Named rather than a bare ValueError, which all three subclass: a
        # genuine programming error should be a traceback, not "refused:".
        sys.exit(f"refused: {exc}")
    finally:
        con.close()

    role = " (admin)" if user.is_admin else ""
    print(f"  created {user.username}{role} in {config.APP_DB_PATH}")
    if password is None:
        print("  no password set -- this account cannot log in yet.")


def _username_for(email: str, users) -> str:
    """A default login handle from an address' local part.

    `ada.lovelace@example.com` becomes `ada.lovelace`, which is both usable and
    the name the person would have picked. When it is not -- a `+` tag, a
    single character, a handle somebody already has -- the command asks for
    `--username` rather than inventing `ada2`, because a login handle chosen by
    a mangling rule is one its owner has to be told.
    """
    local = email.partition("@")[0]
    return users.normalise_username(local)


def cmd_users_invite(args):
    """Create an account nobody has a password for, and mail its owner a link.

    ADR 16 in one command. The account is created **unclaimed** -- a real row
    with `password_hash IS NULL`, which `users.authenticate` refuses at the
    cost of a full hash -- rather than disabled, because `disabled_at` is the
    maintainer's revocation lever and reusing it here would mean redeeming a
    link had to un-revoke an account. "Cannot log in yet" and "has been shut
    off" are different states and the database already tells them apart.

    Re-inviting an unclaimed account is allowed and is the resend path: the
    previous link stops working the moment a new one is issued. Re-inviting a
    *claimed* one is refused, and points at the reset flow, because that is
    what somebody who has forgotten their password actually needs.
    """
    _db, _, _, users = _auth()
    from mtglab.auth import invites, mail

    try:
        address = users.normalise_email(args.email)
    except users.InvalidEmail as exc:
        sys.exit(f"refused: {exc}")
    if address is None:
        sys.exit("refused: an invite needs an email address to send to")

    try:
        sender = mail.sender_from_env()
    except mail.EmailNotConfigured as exc:
        sys.exit(f"refused: {exc}")

    con = _connect()
    try:
        existing = users.get_by_email(con, address)
        if existing is not None:
            if users.has_password(con, existing.id):
                sys.exit(f"refused: {existing.username} has already claimed "
                         "that address -- they can use the reset link on the "
                         "sign-in page")
            user = existing
        else:
            try:
                name = (users.normalise_username(args.username)
                        if args.username else _username_for(address, users))
            except users.InvalidUsername as exc:
                sys.exit(f"refused: {exc}; pass --username")
            try:
                user = users.create(con, name, email=address,
                                    is_admin=args.admin)
            except users.UserExists:
                sys.exit(f"refused: the username {name!r} is taken -- "
                         "pass --username to choose another")
        try:
            invites.send_invite(con, user, sender=sender)
        except (mail.EmailNotSent, OSError) as exc:
            sys.exit(f"refused: {exc}")
    finally:
        con.close()

    role = " (admin)" if user.is_admin else ""
    print(f"  invited {user.username}{role}")
    print("  they choose their own password; the link works once and "
          "expires in a week.")


def cmd_users_list(args):
    _db, _, sessions, users = _auth()
    from mtglab.auth import tokens

    con = _connect()
    try:
        everyone = users.all_users(con)
        if not everyone:
            print(f"  no accounts in {config.APP_DB_PATH}")
            print("  create one with `mtglab users add <name>`")
            return
        print(f"  {'username':<20} {'email':<28} {'state':<12} sessions")
        for user in everyone:
            state = "disabled" if user.disabled else (
                "active" if users.has_password(con, user.id)
                else ("invited" if tokens.outstanding(con, user.id,
                                                      tokens.Purpose.INVITE)
                      else "no password"))
            live = sessions.count_for_user(con, user.id)
            admin = "*" if user.is_admin else " "
            print(f" {admin}{user.username:<20} {user.email or '-':<28} "
                  f"{state:<12} {live}")
        print("\n  * admin")
    finally:
        con.close()


def cmd_users_passwd(args):
    """Set a password, ending every session for that account.

    The revocation is not a courtesy. A password change is usually somebody who
    suspects compromise, and one that leaves the other party logged in has
    answered the wrong question (ADR 16).
    """
    _db, _, _, users = _auth()

    con = _connect()
    try:
        user = users.get(con, args.username)
        if user is None:
            sys.exit(f"refused: no account {args.username!r}")
        password = _prompt_new_password(user.username)
        ended = users.set_password(con, user.id, password)
    finally:
        con.close()

    print(f"  password set for {user.username}")
    if ended:
        print(f"  {ended} session(s) ended -- every device signs in again.")


def _set_disabled(username: str, disabled: bool):
    _db, _, _, users = _auth()

    con = _connect()
    try:
        user = users.get(con, username)
        if user is None:
            sys.exit(f"refused: no account {username!r}")
        try:
            ended = users.set_disabled(con, user.id, disabled)
        except users.LastAdmin as exc:
            # The likelier of the two doors to a lockout (ADR 17): disabling is
            # what somebody reaches for in a hurry, and `disable root` on this
            # deployment would be the whole administration of it.
            sys.exit(f"refused: {exc}")
    finally:
        con.close()

    print(f"  {user.username} is now "
          f"{'disabled' if disabled else 'enabled'}")
    if ended:
        print(f"  {ended} session(s) ended.")


def cmd_users_disable(args):
    _set_disabled(args.username, True)


def cmd_users_enable(args):
    _set_disabled(args.username, False)


def cmd_users_delete(args):
    """Delete an account for good. The break-glass path the browser refuses.

    Interactive by default and shaped like `decks delete`: it prints what is
    about to go and asks for the username back. `--yes` is for scripts and still
    requires the name on the command line, so there is no spelling of this that
    deletes an account nobody typed out.

    This command will delete the account you are using, which the admin route
    deliberately will not -- there is no session here to sign out of, and
    somebody standing at the machine with the key is the one caller for whom
    "remove this account" is unambiguous. The last-admin guard still applies.
    """
    _, _, sessions, users = _auth()

    con = _connect()
    try:
        user = users.get(con, args.username)
        if user is None:
            sys.exit(f"refused: no account {args.username!r}")
        print(f"  {user.username}{' (admin)' if user.is_admin else ''}, "
              f"{sessions.count_for_user(con, user.id)} session(s)")
        print("  deleted for good -- there is no undo and no trash")
        if not args.yes:
            typed = input(f"  type '{user.username}' to delete it: ").strip()
            if typed.casefold() != user.username.casefold():
                sys.exit("refused: that is not the username")
        try:
            ended = users.delete(con, user.id)
        except users.LastAdmin as exc:
            # The third and worst door to a lockout: `disable` and `demote` can
            # be walked back from another admin's session, and this cannot be
            # walked back at all.
            sys.exit(f"refused: {exc}")
    finally:
        con.close()

    print(f"  {user.username} is gone")
    if ended:
        print(f"  {ended} session(s) ended.")


def _set_admin(username: str, is_admin: bool):
    """Grant or revoke admin. The caller `users.set_admin` never had (ADR 17).

    `--admin` at creation was the only way to make one, so an account promoted
    after the fact needed a hand-written `UPDATE`. The refusal it can hit is the
    interesting part: an instance may not be left with no admin who can sign in,
    and that rule lives in `auth/users.py` so this command and the admin page
    inherit it rather than each implementing it.
    """
    _db, _, _, users = _auth()

    con = _connect()
    try:
        user = users.get(con, username)
        if user is None:
            sys.exit(f"refused: no account {username!r}")
        if user.is_admin == is_admin:
            print(f"  {user.username} is already "
                  f"{'an admin' if is_admin else 'not an admin'}")
            return
        try:
            users.set_admin(con, user.id, is_admin)
        except users.LastAdmin as exc:
            sys.exit(f"refused: {exc}")
        remaining = len(users.usable_admin_ids(con))
        claimed = users.has_password(con, user.id)
    finally:
        con.close()

    print(f"  {user.username} is now "
          f"{'an admin' if is_admin else 'not an admin'}")
    if is_admin and not claimed:
        # Worth saying, because it is the case where the command appears to
        # have worked and the instance still has nobody who can administer it.
        # The last-admin guard counts the same way, so promoting an unclaimed
        # account is not a step towards being allowed to demote the real one.
        print("  they have no password yet, so they cannot sign in to use it.")
    print(f"  {remaining} admin(s) can sign in.")


def cmd_users_promote(args):
    _set_admin(args.username, True)


def cmd_users_demote(args):
    _set_admin(args.username, False)


# -------------------------------------------------------------------- claude

def cmd_claude_check(args):
    """One real call, so "is the pipe open" is a command rather than a guess.

    Worth having as a command rather than only a test because of when it gets
    run: after a key rotation, or weeks later when something returns a 401 and
    the question is whether the integration broke or the key simply lapsed.
    `client.explain` answers that in the message rather than leaving it to be
    rediscovered.
    """
    from mtglab.claude import client as claude

    report = claude.check()
    print(f"  model     {report['model']}")
    if not report["ok"]:
        print(f"  status    unavailable\n  reason    {report['error']}")
        sys.exit(1)
    print(f"  served by {report['served_by']}")
    print(f"  reply     {report['text']!r}")
    print(f"  tokens    {report['input_tokens']} in / "
          f"{report['output_tokens']} out")

    if args.tools:
        from mtglab.claude import tools
        print(f"\n  {len(tools.READ_ONLY)} tools, all read-only:")
        for name in sorted(tools.READ_ONLY):
            print(f"    {name}")


def cmd_claude_interview(args):
    """Questions about one card's slot, so you can write its rationale.

    Prints questions and no rationale, which is the whole design rather than a
    presentation choice -- there is no rationale in the response to print. The
    output is labelled as Claude's because the gate's output is reproducible
    and this is not, and a terminal that renders them alike is the one place
    ADR 14's third boundary is easiest to lose.
    """
    from mtglab.api import service
    from mtglab.claude.client import ClaudeUnavailable
    from mtglab.claude.interview import CardNotInDeck

    try:
        report = service.claude_interview(
            slug=args.slug, card=args.card,
            requested=args.stance, focus=args.focus or "")
    except (CardNotInDeck, ClaudeUnavailable, service.ClaudeFailed) as exc:
        print(f"  {exc}")
        sys.exit(1)

    stance = report["stance"]
    print(f"\n  {report['card']} — {report['slug']}")
    print(f"  asked as: {stance['preset'] or 'custom'} "
          f"(scope: {stance['axes'][1]['level']})")

    if not report["asked"]:
        print(f"\n  {report['reason']}")
        return

    if report["tool_calls"]:
        looked = ", ".join(sorted({c["tool"] for c in report["tool_calls"]}))
        print(f"  looked up: {looked}")

    print(f"\n  Claude asks — {report['model']}, not the gate:\n")
    for i, q in enumerate(report["questions"], 1):
        print(f"  {i}. [{q['angle']}] {q['question']}")
        if q["fact"]:
            print(f"       ({q['fact']})")
    if not report["questions"]:
        print(f"  nothing usable came back. {report['reason']}")
    if report["questions_dropped"]:
        print(f"\n  {report['questions_dropped']} answer(s) dropped: not questions.")

    print(f"\n  {report['never']}")
    print(f"  Write it with: mtglab decks set {args.slug} "
          f"--card {report['card']!r} --why '...'")
    usage = report["usage"]
    print(f"  tokens: {usage['input_tokens']} in / {usage['output_tokens']} out")


def _wrapped(text, indent="  ", width=76):
    import textwrap
    return textwrap.fill(text, width=width, initial_indent=indent,
                         subsequent_indent=indent)


def cmd_claude_argue(args):
    """The case against one card's slot (ADR 25). One direction, on purpose.

    Prints charges and alternatives and no case in favour, because there is no
    case in favour in the response to print -- the schema has no field for one.
    Labelled as Claude's for the same reason the interview is, and with more
    need: a reasoned case against a card reads like a verdict, and the gate's
    verdicts are the reproducible ones.
    """
    from mtglab.api import service
    from mtglab.claude.argue import CardNotInDeck
    from mtglab.claude.client import ClaudeUnavailable

    try:
        report = service.claude_argue(
            slug=args.slug, card=args.card,
            requested=args.stance, focus=args.focus or "")
    except (CardNotInDeck, ClaudeUnavailable, service.ClaudeFailed) as exc:
        print(f"  {exc}")
        sys.exit(1)

    stance = report["stance"]
    print(f"\n  {report['card']} — {report['slug']}")
    print(f"  asked as: {stance['preset'] or 'custom'} "
          f"(scope: {stance['axes'][1]['level']})")

    if not report["asked"]:
        print(f"\n  {report['reason']}")
        return

    if report["tool_calls"]:
        looked = ", ".join(sorted({c["tool"] for c in report["tool_calls"]}))
        print(f"  looked up: {looked}")

    print(f"\n  The case against — {report['model']}, not the gate:\n")
    for i, charge in enumerate(report["charges"], 1):
        print(f"  {i}. [{charge['strength']}/{charge['ground']}]")
        print(_wrapped(charge["claim"], indent="     "))
        print(_wrapped(f"({charge['fact']})", indent="       "))
    if not report["charges"]:
        print(f"  nothing usable came back. {report['reason']}")
    if report["charges_dropped"]:
        print(f"\n  {report['charges_dropped']} charge(s) dropped: nothing cited.")

    if report["alternatives"]:
        print("\n  Cards that could do the job instead — pool-checked, "
              "in-identity:\n")
        for card in report["alternatives"]:
            print(f"    {card['name']}  {card['mana_cost'] or ''}  "
                  f"{card['type_line']}")
    # Said out loud rather than left as a shorter list. Which filter removed a
    # name is the informative part: "you made that up" and "that is off-colour"
    # are different failures and only one of them is about the deck.
    for reason, names in report["alternatives_dropped"].items():
        if names:
            print(f"\n  dropped ({reason.replace('_', ' ')}): "
                  f"{', '.join(names)}")

    print(f"\n{_wrapped(report['never'])}")
    print(f"  Cut it with:   mtglab decks remove {args.slug} "
          f"--card {report['card']!r}")
    print(f"  Keep it with:  mtglab decks set {args.slug} "
          f"--card {report['card']!r} --why '...'")
    usage = report["usage"]
    print(f"  tokens: {usage['input_tokens']} in / {usage['output_tokens']} out")


def cmd_claude_dossier(args):
    """Who a deck's commander is (ADR 19), from the pool and the open web.

    The printing is where ADR 14's third boundary lives in a terminal, so it is
    not decoration: every passage prints the source ids it rests on, the source
    list prints the real URLs, and the header says which system wrote it. A
    dossier rendered as unattributed prose would be the failure the whole mode
    is built to avoid, arrived at in the last ten lines.
    """
    from mtglab.api import service
    from mtglab.claude import dossier as dossier_mod
    from mtglab.claude.client import ClaudeUnavailable

    if args.list:
        rows = dossier_mod.stored()
        if not rows:
            print("\n  no dossiers stored yet.\n")
            return
        print(f"\n  {len(rows)} stored:\n")
        for row in rows:
            print(f"  {row['commander']:<34} {row['created_at'][:10]}")
        print()
        return

    if args.clear:
        print(f"\n  cleared {dossier_mod.clear()} dossier(s).\n")
        return

    if not args.slug:
        print("  a deck slug is required (or --list / --clear)")
        sys.exit(1)

    try:
        report = service.claude_dossier(slug=args.slug, requested=args.stance,
                                        refresh=args.refresh)
    except (dossier_mod.NoCommander, ClaudeUnavailable,
            service.ClaudeFailed) as exc:
        print(f"  {exc}")
        sys.exit(1)

    print(f"\n  {report['commander']} — {report['slug']}")
    body = report["dossier"]
    if not body:
        print(f"\n  {report['reason']}\n")
        return

    origin = ("stored " + report["generated_at"][:10] if report["cached"]
              else f"written by {report['model']}")
    print(f"  Claude's writing, not the gate's — {origin}\n")

    def passage(title, section, extra=""):
        cites = " ".join(f"[{i}]" for i in section["source_ids"])
        print(f"  {title}{extra}{'  ' + cites if cites else ''}")
        print(_wrapped(section["prose"], indent="    "))
        print()

    passage("WHO", body["who"])
    passage("ARCHETYPE", body["archetype"],
            extra=f" — {body['archetype']['name']}" if body["archetype"]["name"] else "")
    if body["competitors"]:
        print("  COMPETITORS")
        for competitor in body["competitors"]:
            cost = f" {competitor['mana_cost']}" if competitor.get("mana_cost") else ""
            cites = " ".join(f"[{i}]" for i in competitor["source_ids"])
            print(f"    {competitor['name']}{cost}{'  ' + cites if cites else ''}")
            print(_wrapped(competitor["prose"], indent="      "))
        print()
    if body["allies"]["prose"]:
        passage("ALLIES", body["allies"])
    if body["rivals"]["prose"]:
        passage("RIVALS", body["rivals"])
    passage("STANDING", body["standing"])

    print("  SOURCES — every one of these was actually fetched\n")
    for source in body["sources"]:
        print(f"    [{source['id']}] {source['title'][:64]}")
        print(f"          {source['url']}")

    print(f"\n  {report['never']}")
    # The dropped counts are printed rather than logged, because a number that
    # climbs is a prompt inventing citations and nobody checks what they cannot
    # see. Silence here means everything the model cited, it had read.
    if body["sources_dropped"] or body["competitors_dropped"]:
        print(f"  dropped: {body['sources_dropped']} cited page(s) the search "
              f"never returned, {body['competitors_dropped']} competitor(s) "
              f"not in the pool.")
    print(f"  {body['searched']} pages searched, {len(body['sources'])} cited.")
    usage = report["usage"]
    if report["cached"]:
        print("  no tokens: served from the store. --refresh to write it again.")
    else:
        print(f"  tokens: {usage['input_tokens']} in / "
              f"{usage['output_tokens']} out "
              f"({usage['cache_read_tokens']} cached)")
    print()


def cmd_claude_research(args):
    """A question about Magic the pool cannot answer (ADR 26).

    Note what this command does *not* take: a slug. Research is deck-blind by
    construction, and the absence of the argument is the first place a user
    meets the decision — asked what to cut from a deck, the mode says it cannot
    see one, because it cannot.

    Printed with the same discipline the dossier uses, and it matters more
    here: an answer with citations under it is the exact shape of something
    reproducible, and it is not. Two kinds of card fact are also visibly
    different — a card the pool has prints its real text, a card it does not
    prints as a claim resting on a page.
    """
    from mtglab.claude import research as research_mod
    from mtglab.claude.client import ClaudeUnavailable
    from mtglab.claude.modes import ModeExhausted

    question = " ".join(args.question).strip()
    try:
        report = research_mod.ask(question, requested=args.stance)
    except (research_mod.QuestionRejected, ClaudeUnavailable,
            ModeExhausted) as exc:
        print(f"  {exc}")
        sys.exit(1)

    stance = report["stance"]
    print(f"\n  {report['question']}")
    print(f"  asked as: {stance['preset'] or 'custom'} "
          f"(scope: {stance['axes'][1]['level']})")

    body = report["research"]
    if not body:
        print(f"\n  {report['reason']}\n")
        return

    print(f"  Claude's reading of cited pages, not the tool's own answer "
          f"— {report['model']}\n")
    print(f"  ANSWER  [{body['confidence']}]")
    print(_wrapped(body["answer"], indent="    "))
    print()

    if body["findings"]:
        print("  FINDINGS")
        for finding in body["findings"]:
            cites = " ".join(f"[{i}]" for i in finding["source_ids"])
            print(_wrapped(f"{finding['claim']} {cites}", indent="    "))
        print()

    if body["cards"]:
        print("  CARDS NAMED")
        for card in body["cards"]:
            if not card["in_pool"]:
                # Said out loud, every time. This is the seam ADR 26 exists to
                # keep visible: everything about this card came off a page.
                print(f"    {card['name']}  — not in the pool; whatever is "
                      f"said about it above rests on a cited page, not on a "
                      f"card lookup")
                continue
            cost = f"  {card['mana_cost']}" if card.get("mana_cost") else ""
            print(f"    {card['name']}{cost}  {card['type_line'] or ''}")
        print()

    print("  SOURCES — every one of these was actually fetched\n")
    for source in body["sources"]:
        print(f"    [{source['id']}] {source['title'][:64]}")
        print(f"          {source['url']}")

    print(f"\n  {report['never']}")
    # Printed rather than logged, for the reason the dossier prints its own: a
    # number that climbs is a prompt that has started inventing, and nobody
    # checks a number they cannot see. `cards_unresolved` is the one that is
    # not a fault — for a spoiler question the right value is above zero.
    if body["sources_dropped"] or body["findings_dropped"]:
        print(f"  dropped: {body['sources_dropped']} cited page(s) the search "
              f"never returned, {body['findings_dropped']} finding(s) left "
              f"citing nothing.")
    if body["cards_unresolved"]:
        print(f"  {body['cards_unresolved']} named card(s) are not in the "
              f"pool — expected for anything spoiled since the last "
              f"`mtglab data refresh`.")
    print(f"  {body['searched']} pages searched, {len(body['sources'])} cited.")
    usage = report["usage"]
    print(f"  tokens: {usage['input_tokens']} in / "
          f"{usage['output_tokens']} out "
          f"({usage['cache_read_tokens']} cached)")
    print()


def cmd_claude_usage(args):
    """Where the Claude money went, per mode, from the ledger in app.db.

    Tokens and not dollars, deliberately: prices move (Sonnet 5's introductory
    rate ends 2026-08-31) and a stale hardcoded price table would turn an
    honest count into a wrong invoice. The cached column is the exception that
    proves it — those tokens were billed at roughly a tenth of the input rate,
    which is worth saying because it is the number that justifies the cache.
    """
    from mtglab.claude import ledger

    since = f"{args.since}T00:00:00+00:00" if args.since else None
    rows = ledger.summary(since=since)
    if not rows:
        print("  no conversations recorded"
              + (f" since {args.since}" if args.since else "") + ".")
        return

    print(f"\n  {'mode':<22} {'conv':>5} {'req':>5} "
          f"{'tokens in':>11} {'tokens out':>11} {'cached':>11}")
    for row in rows:
        print(f"  {row['mode']:<22} {row['conversations']:>5} "
              f"{row['requests']:>5} {row['input_tokens']:>11} "
              f"{row['output_tokens']:>11} {row['cache_read_tokens']:>11}")
    total_in = sum(r["input_tokens"] for r in rows)
    total_out = sum(r["output_tokens"] for r in rows)
    total_cached = sum(r["cache_read_tokens"] for r in rows)
    print(f"  {'total':<22} {sum(r['conversations'] for r in rows):>5} "
          f"{sum(r['requests'] for r in rows):>5} {total_in:>11} "
          f"{total_out:>11} {total_cached:>11}")
    print("\n  'cached' counts prompt cache reads, at ~a tenth of the input "
          "price. They sit\n  *beside* 'tokens in' rather than inside it -- "
          "the API reports 'tokens in' as\n  the uncached remainder only, so "
          "a conversation's whole prompt is the two\n  added together.")
    print("  Prompt cache *writes* bill at 1.25x input and are not recorded "
          "at all, so\n  this table is a floor on the bill rather than the "
          "bill.")
    first = min(r["first_at"] for r in rows)
    last = max(r["last_at"] for r in rows)
    print(f"  counting {first[:10]} to {last[:10]}.\n")


# --------------------------------------------------------------------- main

# ------------------------------------------------------------------- animist

def _animist_recipe(path_str):
    """Load and validate, or refuse with the schema's own sentence."""
    from mtglab.animist.recipe import RecipeError, load_recipe

    try:
        return load_recipe(Path(path_str))
    except RecipeError as exc:
        sys.exit(f"refused: {exc}")


def _animist_pixels():
    """The moment pixels get touched is the moment the extra is required."""
    from mtglab import animist

    if not animist.available():
        sys.exit("the animist extra is not installed -- "
                 "`pip install -e '.[animist]'` brings Pillow, the one "
                 "image dependency, and nothing else here needs it")


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


def cmd_animist_build(args):
    """The whole pipeline for one recipe. The gate runs first and blocks."""
    _animist_pixels()
    from mtglab.animist.encode import EncodeError
    from mtglab.animist.ops import OpError
    from mtglab.animist.run import BuildError, build
    from mtglab.animist.sources import LicenceRefused, SourceError

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


def cmd_animist_fetch(args):
    """Gate, then warm the cache. No pixels are transformed."""
    from mtglab.animist.fetch import fetch_source
    from mtglab.animist.sources import LicenceRefused, SourceError

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


def cmd_animist_licence(args):
    """The gate alone: every source's confirmed licence, dated. No download."""
    from mtglab.animist.sources import LicenceRefused, SourceError, confirm

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
    """Every committed recipe: the package's asset tree, and -- when run from
    a checkout -- the frontend's."""
    import mtglab

    roots = [Path(mtglab.__file__).parent / "assets",
             Path("web") / "src" / "assets"]
    found: list[Path] = []
    for root in roots:
        if root.is_dir():
            found.extend(sorted(root.rglob("*.recipe.yaml")))
    return found


def cmd_animist_verify(args):
    """Committed assets against their recipes. The suite runs this too."""
    _animist_pixels()
    from mtglab.animist.verify import verify_recipe

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


def cmd_animist_measure(args):
    """The size curve for one output, and where its knee sits.

    For a still, runs the output's ops *except* any `resize`, then encodes
    at each width in the sweep -- the question is what the final resize
    should be, so the recipe's own answer is held out. For a video output
    the axis is `crf` instead of width (ADR 31): the derivation runs as
    written and the sweep asks what the rate control should be.
    """
    _animist_pixels()
    from mtglab.animist.fetch import fetch_source
    from mtglab.animist.measure import crf_curve, knee, size_curve
    from mtglab.animist.ops import apply
    from mtglab.animist.recipe import VIDEO_FORMATS
    from mtglab.animist.sources import LicenceRefused, SourceError

    recipe = _animist_recipe(args.recipe)
    named = [out for out in recipe.outputs if out.file == args.output]
    if not named:
        sys.exit(f"refused: {recipe.path.name} has no `file:` output named "
                 f"{args.output!r} -- `measure` sweeps one file at a time")
    (output,) = named

    if output.encode.format in VIDEO_FORMATS:
        from mtglab.animist.run import _derive

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
    rm = decks.add_parser(
        "remove", help="take a card out -- a 99-card goes to the graveyard")
    rm.add_argument("slug"); rm.add_argument("--card", required=True)
    rm.set_defaults(func=cmd_decks_remove)
    rt = decks.add_parser(
        "return", help="bring an entombed card back to the 99, why intact")
    rt.add_argument("slug"); rt.add_argument("--card", required=True)
    rt.set_defaults(func=cmd_decks_return)
    ex = decks.add_parser(
        "exile", help="drop a card from the graveyard for good")
    ex.add_argument("slug"); ex.add_argument("--card", required=True)
    ex.set_defaults(func=cmd_decks_exile)
    st = decks.add_parser("set", help="change one field, of a card or of the deck")
    st.add_argument("slug")
    st.add_argument("--card", help="the card to change; omit for a deck field")
    st.add_argument("--why", help="the rationale, in your words")
    st.add_argument("--category", help=f"one of: {', '.join(CATEGORIES)}")
    st.add_argument("--qty", type=int)
    st.add_argument("--stage", choices=list(DECK_STAGES),
                    help="curated needs every card justified; see `decks promote`")
    st.add_argument("--status", choices=list(DECK_STATUSES))
    st.add_argument("--bracket", type=int)
    st.add_argument("--pilot",
                    help="who sleeves this deck up — a name, or '' to untag "
                         "(second 2026-08-15 punch list, item 10)")
    st.add_argument("--art", metavar="SET",
                    help="which printing's art the deck shows for its "
                         "commander: a set code, or a printing id when a set "
                         "has several. Empty clears it back to the default.")
    st.set_defaults(func=cmd_decks_set)
    pr = decks.add_parser("promote", help="mark a draft curated, once every card "
                                          "carries a `why`")
    pr.add_argument("slug")
    pr.set_defaults(func=cmd_decks_promote)
    dl = decks.add_parser("delete", help="remove a deck; it moves to .trash/")
    dl.add_argument("slug")
    dl.add_argument("--yes", action="store_true",
                    help="skip the prompt; the slug on the command line is the "
                         "confirmation")
    dl.set_defaults(func=cmd_decks_delete)
    nt = decks.add_parser("note", help="set a deck-level note")
    nt.add_argument("slug"); nt.add_argument("--key", required=True)
    nt.add_argument("--value")
    nt.add_argument("--from-file", dest="from_file",
                    help="read the note's text from a file, for long prose")
    nt.set_defaults(func=cmd_decks_note)
    lg = decks.add_parser("log", help="what has been done to this deck")
    lg.add_argument("slug")
    lg.add_argument("--limit", type=int, default=20,
                    help="how many entries to show, newest first (default 20)")
    lg.set_defaults(func=cmd_decks_log)
    b = decks.add_parser("build"); b.add_argument("slug")
    b.add_argument("--against",
                   help="path to a previous deck.yaml for swaps.md; defaults "
                        "to the last build's own snapshot when one exists")
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
    sc = sim.add_parser("cache", help="what Tier 1 results are memoised")
    sc.add_argument("--clear", action="store_true",
                    help="drop every cached result; they recompute on demand")
    sc.set_defaults(func=cmd_sim_cache)
    fg = sim.add_parser("forge", help="Tier 3 -- Forge plays real games")
    fg.add_argument("slugs", nargs="+", help="two to four decks")
    fg.add_argument("--games", type=int, default=10)
    # 300, not Forge's default of 120: a long game should be a long game, not
    # a draw the clock invented.
    fg.add_argument("--clock", type=int, default=300)
    fg.add_argument("--seed", type=int, default=None)
    fg.add_argument("--check-only", action="store_true",
                    help="card-coverage pre-flight only; needs no JVM")
    fg.set_defaults(func=cmd_sim_forge)

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

    us = sub.add_parser("users", help="accounts for the hosted app")\
        .add_subparsers(dest="cmd", required=True)
    ua = us.add_parser("add", help="create an account; the password is prompted")
    ua.add_argument("username")
    ua.add_argument("--email", help="for password resets, once that lands")
    ua.add_argument("--admin", action="store_true")
    # There is no --password, deliberately. See the note above cmd_users_add.
    ua.add_argument("--no-password", action="store_true",
                    help="create the account unclaimed: it exists and cannot "
                         "log in until a password is set")
    ua.set_defaults(func=cmd_users_add)
    uinv = us.add_parser("invite",
                        help="create an account and mail a setup link")
    uinv.add_argument("email")
    uinv.add_argument("--username",
                     help="login handle; defaults to the address' local part")
    uinv.add_argument("--admin", action="store_true")
    uinv.set_defaults(func=cmd_users_invite)
    us.add_parser("list").set_defaults(func=cmd_users_list)
    up = us.add_parser("passwd", help="set a password; ends every session")
    up.add_argument("username"); up.set_defaults(func=cmd_users_passwd)
    ud = us.add_parser("disable"); ud.add_argument("username")
    ud.set_defaults(func=cmd_users_disable)
    ue = us.add_parser("enable"); ue.add_argument("username")
    ue.set_defaults(func=cmd_users_enable)
    # `--admin` at creation was the only way to make one until ADR 17. Both
    # refuse to leave the instance with no admin who can sign in.
    upr = us.add_parser("promote", help="make an account an admin")
    upr.add_argument("username"); upr.set_defaults(func=cmd_users_promote)
    ude = us.add_parser("demote", help="take admin away")
    ude.add_argument("username"); ude.set_defaults(func=cmd_users_demote)
    # The only irreversible one. `disable` is what you almost always want; this
    # is for releasing a username or an address so it can be invited again.
    udel = us.add_parser("delete", help="remove an account for good")
    udel.add_argument("username")
    udel.add_argument("--yes", action="store_true",
                      help="skip the typed confirmation")
    udel.set_defaults(func=cmd_users_delete)

    claude = sub.add_parser("claude").add_subparsers(dest="cmd", required=True)
    cc = claude.add_parser("check", help="one real call -- is the key working?")
    cc.add_argument("--tools", action="store_true",
                    help="also list the tools a Claude surface may call")
    cc.set_defaults(func=cmd_claude_check)
    ci = claude.add_parser("interview",
                           help="questions about one card's slot; you write the why")
    ci.add_argument("slug")
    ci.add_argument("--card", required=True, help="a card already in the deck")
    ci.add_argument("--stance", default="consultant",
                    help="off | consultant | second-opinion | collaborator")
    ci.add_argument("--focus", help="what you are stuck on, in your own words")
    ci.set_defaults(func=cmd_claude_interview)
    ca = claude.add_parser("argue",
                           help="the case against one card's slot -- never for it")
    ca.add_argument("slug")
    ca.add_argument("--card", required=True, help="a card already in the deck")
    ca.add_argument("--stance", default="consultant",
                    help="off | consultant | second-opinion | collaborator")
    ca.add_argument("--focus", help="what you are weighing, in your own words")
    ca.set_defaults(func=cmd_claude_argue)
    cd = claude.add_parser("dossier",
                           help="who a deck's commander is, with its sources")
    cd.add_argument("slug", nargs="?")
    cd.add_argument("--stance", default="consultant",
                    help="off | consultant | second-opinion | collaborator")
    cd.add_argument("--refresh", action="store_true",
                    help="write it again even if one is stored")
    cd.add_argument("--list", action="store_true",
                    help="what dossiers are stored, and when they were written")
    cd.add_argument("--clear", action="store_true", help="drop every one")
    cd.set_defaults(func=cmd_claude_dossier)
    # No `slug` argument, and its absence is ADR 26 rather than an oversight:
    # this mode cannot reach a deck, so there is nothing for a slug to name.
    cr = claude.add_parser("research",
                           help="a question the pool cannot answer, with its "
                                "sources -- takes no deck")
    cr.add_argument("question", nargs="+",
                    help="the question, in plain words")
    cr.add_argument("--stance", default=None,
                    help="off | consultant | second-opinion | collaborator "
                         "(default: second-opinion -- there is no deck to "
                         "derive one from)")
    cr.set_defaults(func=cmd_claude_research)
    cu = claude.add_parser("usage",
                           help="what every mode has spent -- tokens, "
                                "requests, and how much the cache saved")
    cu.add_argument("--since", metavar="YYYY-MM-DD",
                    help="count only conversations on or after this date")
    cu.set_defaults(func=cmd_claude_usage)

    # Wake still images into the site's scenery -- fetched free-use,
    # licence-checked per file, every step written down (ADR 29). The gate
    # blocks: there is no flag past a refused licence, deliberately.
    an = sub.add_parser("animist").add_subparsers(dest="cmd", required=True)
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
