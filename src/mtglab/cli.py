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
from collections.abc import Sequence
from math import fsum
from pathlib import Path
from types import ModuleType
from typing import TYPE_CHECKING, Any

from mtglab import config

# The one thing from the measuring shelf that is needed at parser-build time,
# so `--help` can print the threshold rather than a bare number. Cheap:
# `bench.targets` reaches the API and the pool inside `suite()`, never at
# import, so a plain `mtglab decks list` pays nothing for it.
from mtglab.bench.targets import PROFILE_OVER_MS
from mtglab.decks.model import (
    ARCHETYPES,
    CATEGORIES,
    DECK_STAGES,
    DECK_STATUSES,
    Deck,
)
from mtglab.sim.compile import (
    PoolRequired,
    compile_deck,
    enters_tapped,
    fetches_lands,
)

# Types only. Every one of these lives behind a module this file imports inside
# a function on purpose -- the pool's DuckDB, the depth extra's torch, Pillow --
# and `from __future__ import annotations` means a name used only in a
# signature never has to exist at run time. Importing them for real here would
# undo the laziness the commands were written for.
if TYPE_CHECKING:  # pragma: no cover
    import sqlite3

    from mtglab.animist.recipe import Recipe
    from mtglab.cardmotion.depth import DepthModel
    from mtglab.cards.db import CardRecord
    from mtglab.mutate.harness import Result
    from mtglab.sim.tier1.engine import SimCard

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


def _pool(deck: Deck) -> dict[str, CardRecord] | None:
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

def cmd_data_refresh(args: argparse.Namespace) -> None:
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


def cmd_data_snapshot(args: argparse.Namespace) -> None:
    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    n = db.snapshot_prices(con)
    print(f"snapshotted {n:,} prices for today")
    con.close()


# -------------------------------------------------------------------- decks

def load_all_decks(decks_dir: Path | None = None) -> list[Deck]:
    """Shared by the CLI and the API so both see exactly the same library."""
    return [Deck.load(p) for p in config.deck_paths(decks_dir)]


def cmd_decks_list(args: argparse.Namespace) -> None:
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


def cmd_decks_validate(args: argparse.Namespace) -> None:
    from mtglab.decks.validate import validate
    deck = _load(args.slug)
    rep = validate(deck, _pool(deck))
    print(rep.render())
    print(f"\n{len(rep.errors)} error(s), {len(rep.warnings)} warning(s)")
    sys.exit(1 if rep.errors else 0)


def cmd_decks_import(args: argparse.Namespace) -> None:
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


def cmd_decks_suggest(args: argparse.Namespace) -> None:
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


def _report_edit(result: dict[str, Any]) -> None:
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


def cmd_decks_swap(args: argparse.Namespace) -> None:
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


def cmd_decks_add(args: argparse.Namespace) -> None:
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


def cmd_decks_remove(args: argparse.Namespace) -> None:
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


def cmd_decks_return(args: argparse.Namespace) -> None:
    from mtglab.api import service

    try:
        result = service.return_card(args.slug, name=args.card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  + {result['returned']} <- graveyard, rationale intact")
    _report_edit(result)


def cmd_decks_exile(args: argparse.Namespace) -> None:
    from mtglab.api import service

    try:
        result = service.exile_card(args.slug, name=args.card)
    except service.EditRejected as exc:
        sys.exit(f"refused: {exc}")

    print(f"  x {result['exiled']} exiled -- gone from the graveyard for good")
    _report_edit(result)


def _art_id(args: argparse.Namespace, card: str | None = None) -> str | None:
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
        found: str = exact[0]["id"]
        return found

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
    chosen: str = matches[0]["id"]
    return chosen


def cmd_decks_set(args: argparse.Namespace) -> None:
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
                   ("themes", args.themes),
                   ("commander_art", _art_id(args)
                    if args.art is not None and not args.card else None))
    chosen = [(f, v) for f, v in card_fields + deck_fields if v is not None]
    if len(chosen) != 1:
        sys.exit("choose exactly one of --why, --category, --qty, --art (with "
                 "--card) or --stage, --status, --bracket, --pilot, "
                 "--themes, --art (without)")
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


def cmd_decks_promote(args: argparse.Namespace) -> None:
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


def cmd_decks_delete(args: argparse.Namespace) -> None:
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


def cmd_decks_note(args: argparse.Namespace) -> None:
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


def cmd_decks_log(args: argparse.Namespace) -> None:
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
        print("  Edits made before this log existed were never recorded.\n")
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


def _when(stamp: str) -> str:
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


def cmd_decks_build(args: argparse.Namespace) -> None:
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

def cmd_ui(args: argparse.Namespace) -> None:
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

def _sim_cards(deck: Deck,
               cards: dict[str, Any] | None
               ) -> tuple[list[SimCard], SimCard | None]:
    """CLI wrapper: turn a missing pool into a clean exit rather than a
    traceback. Library callers should use `compile_deck` and catch
    `PoolRequired`."""
    try:
        return compile_deck(deck, cards)
    except PoolRequired as exc:
        sys.exit(str(exc))


def cmd_sim_mana(args: argparse.Namespace) -> None:
    from mtglab.sim.tier1.engine import KeepRule, run
    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _pool(deck))
    rule = KeepRule(min_lands=args.min_lands, max_lands=args.max_lands,
                    min_mana_pieces=args.min_pieces)
    print(run(library, commander, games=args.games, turns=args.turns,
              keep_rule=rule, seed=args.seed).report())


def cmd_sim_lands(args: argparse.Namespace) -> None:
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


def cmd_sim_shelf(args: argparse.Namespace) -> None:
    """The closed form: what arithmetic says, before any shuffling happens.

    Printed as a teaching sheet rather than as a table of figures, which is
    commandment 2 in a terminal: every block says what it measured and what it
    could not see, because a number nobody can interpret is worse than no
    number at all.
    """
    from mtglab.sim import karsten

    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _pool(deck))
    shelf = karsten.shelf(library, commander, target=args.target,
                          on_the_play=not args.on_the_draw)

    seat = "on the play" if shelf.on_the_play else "on the draw"
    print(f"{deck.name}")
    print(f"{shelf.deck_size} cards, {shelf.lands} lands, "
          f"judged at {shelf.target:.0%} consistency, {seat}.\n")

    print("COLOURED SOURCES -- what your own cards demand")
    print("  A rung per pip count, because a deck short on triple-pip cards is")
    print("  not a deck short on colour.\n")
    for req in shelf.colors:
        print(f"  {req.color}: you have {req.have} sources "
              f"({req.have_lands} lands, {req.have - req.have_lands} other)")
        for tier in req.tiers:
            verdict = "ok" if tier.met else f"short {tier.shortfall}"
            more = f" (+{len(tier.cards) - 1} more)" if len(tier.cards) > 1 else ""
            print(f"      {tier.pips} pip on T{tier.turn}: wants {tier.need:>2}"
                  f"  -- {verdict:<8} you make it {tier.odds_now:.0%} of the "
                  f"time  [{tier.cards[0]}{more}]")
        print()

    est = shelf.land_estimate
    print("LAND COUNT -- a regression, not a simulation")
    print(f"  You run {est.lands_now}. The fit says {est.recommended} "
          f"({est.delta:+d}), from an average mana value of "
          f"{est.average_mana_value} and {est.cheap_accelerants} cheap "
          f"accelerants.")
    for caveat in est.caveats:
        print(f"    - {caveat}")
    print("  Read `mtglab sim lands` beside this: it simulates *this* deck and "
          "prices flood,")
    print("  which the fit cannot.\n")

    print("LATEST CARDS -- cost against when the mana is actually there")
    print("  'lag' is turns between what a card costs and when you can rely on")
    print("  casting it. This assumes the card is in your hand; it is a "
          "question")
    print("  about the mana base, not about drawing.\n")
    shown = [o for o in shelf.odds if o.mv <= karsten.HORIZON][: args.top]
    print(f"  {'card':38} {'cost':>4} {'on curve':>9} {'reliable':>9} {'lag':>6}")
    for odds in shown:
        curve = "n/a" if odds.on_curve is None else f"{odds.on_curve:.0%}"
        reliable = ("never" if odds.reliable_turn is None
                    else f"T{odds.reliable_turn}")
        lag = f"+{odds.lag}" if odds.lag is not None else f">={odds.lateness}"
        print(f"  {odds.name[:38]:38} {odds.mv:>4} {curve:>9} {reliable:>9} "
              f"{lag:>6}")
    if shelf.approximated:
        print(f"\n  {len(shelf.approximated)} card(s) demand two or more "
              "colours, where this method")
        print("  approximates and reads slightly low: "
              f"{', '.join(shelf.approximated[:4])}"
              + (", ..." if len(shelf.approximated) > 4 else ""))


def cmd_sim_mulligan(args: argparse.Namespace) -> None:
    """Search the keep-rule grid and report which policy deploys most."""
    from mtglab.sim import mulligan

    deck = _load(args.slug)
    library, commander = _sim_cards(deck, _pool(deck))
    grid = mulligan.candidates()
    print(f"{deck.name}: {len(grid)} keep rules x {args.games:,} games "
          f"(seed {args.seed}) ...\n")
    sweep = mulligan.search(library, commander, games=args.games,
                            turns=args.turns, seed=args.seed)

    print(f"  {'spells T8':>9} {'mull%':>7} {'cmdr':>5}  rule")
    for row in sweep.rows[: args.top]:
        marks = "".join((
            "*" if row is sweep.best else " ",
            "=" if (row.min_lands, row.max_lands, row.min_pieces)
            == (sweep.baseline.min_lands, sweep.baseline.max_lands,
                sweep.baseline.min_pieces) else " ",
        ))
        cmdr = ("--" if row.median_commander_turn is None
                else f"T{row.median_commander_turn:g}")
        print(f"{marks}{row.spells_through_t8:>9.2f} {row.mulligan_rate:>7.1%} "
              f"{cmdr:>5}  {row.describe}")
    print("\n  * best   = the default this simulator uses when you choose "
          "nothing\n")

    if sweep.flat:
        print(f"NO CHANGE WORTH MAKING. The best rule beats your default by "
              f"{sweep.gain:+.2f} spells")
        print(f"through turn 8, under the {mulligan.FLAT} threshold this calls "
              "noise. The grid")
        print(f"spans {sweep.spread:.2f} spells overall, but most of that "
              "range is rules nobody")
        print("would play -- flatness is measured against your default, not "
              "against the grid.")
        gentle = sweep.gentlest
        if gentle.mulligan_rate < sweep.baseline.mulligan_rate - 0.05:
            print(f"\nStill worth knowing: '{gentle.describe}' deploys the "
                  f"same ({gentle.spells_through_t8:.2f})")
            print(f"while mulliganing {gentle.mulligan_rate:.0%} of hands "
                  f"instead of {sweep.baseline.mulligan_rate:.0%}. Same "
                  "result, fewer hands thrown away.")
    else:
        print(f"BEST: {sweep.best.describe}")
        print(f"  {sweep.best.spells_through_t8:.2f} spells through turn 8, "
              f"{sweep.gain:+.2f} against your default's "
              f"{sweep.baseline.spells_through_t8:.2f},")
        print(f"  mulliganing {sweep.best.mulligan_rate:.0%} of hands against "
              f"{sweep.baseline.mulligan_rate:.0%}.")
    print()
    print("Judged on spells deployed through turn 8: mulligan rate alone "
          "recommends keeping")
    print("everything, and hand quality alone recommends mulliganing forever.")


def cmd_sim_cache(args: argparse.Namespace) -> None:
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


def cmd_sim_forge(args: argparse.Namespace) -> None:
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

    # The match ledger (ADR 36): recorded here and in the API job — the two
    # places a match finishes. Never raises; an overnight round-robin must
    # not die on a ledger hiccup with the JVM's work already done.
    from mtglab.sim.tier3 import ledger
    ledger.record(result, decks, seed=args.seed, clock=args.clock,
                  games_requested=args.games, hosted=False)

    wins: dict[str, int] = {}
    for game in result.games:
        # The clock-out rule, same as `forgeruns._shape` and the ledger's
        # reference reading: a game that hit the clock counts for nobody,
        # even when Forge printed a winner line after the slow-match warning.
        slug = result.winner_slug(game)
        key = slug if slug is not None and not game.timed_out else "draw"
        wins[key] = wins.get(key, 0) + 1

    played = [g.milliseconds / 1000 for g in result.games]
    print(f"{len(result.games)} games in {result.wall_seconds:.1f}s "
          f"({result.startup_seconds:.1f}s of it JVM + card database)")
    # `fsum` for the rule rather than for this number: these are wall-clock
    # readings off a JVM, so the mean is not reproducible whatever sums it.
    # It is spelled the same as every other float sum in the package so that
    # `sum(` over floats stays a defect anybody can grep for, with no
    # exception a reader has to re-derive as harmless. `tests/test_float_sums.py`
    # is that grep.
    print(f"per game: {min(played):.1f}s min / "
          f"{fsum(played) / len(played):.1f}s mean / {max(played):.1f}s max")
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


def cmd_sim_matches(args: argparse.Namespace) -> None:
    """The match ledger, newest first (ADR 36).

    The window onto the table the ratings will read — a ledger nobody can see
    into is a ledger that has to be trusted rather than checked, which is the
    `sim cache` argument re-applied to real games.
    """
    from mtglab.sim.tier3 import ledger

    if not config.APP_DB_PATH.exists():
        print("no matches recorded yet -- `mtglab sim forge` records as it plays")
        return
    matches = ledger.recent(limit=args.limit)
    if not matches:
        print("no matches recorded yet -- `mtglab sim forge` records as it plays")
        return
    print(f"ledger: {config.APP_DB_PATH}")
    for m in matches:
        when = m["created_at"][:19].replace("T", " ")
        where = "worker" if m["hosted"] else "local"
        version = f", Forge {m['forge_version']}" if m["forge_version"] else ""
        seeded = f"seed {m['seed']}" if m["seed"] is not None else "unseeded"
        print(f"\n#{m['id']}  {when} UTC  ({where}{version}, {seeded})")
        for seat in m["seats"]:
            labels = seat["archetype"] or "unlabelled"
            if seat["themes"]:
                labels += "; " + ", ".join(seat["themes"])
            print(f"  {seat['slug']:<22} {seat['wins']:>2} win"
                  f"{'s' if seat['wins'] != 1 else ''}  ({labels})")
        extras = []
        if m["draws"]:
            extras.append(f"{m['draws']} real draw{'s' if m['draws'] != 1 else ''}")
        if m["timed_out"]:
            extras.append(f"{m['timed_out']} hit the {m['clock']}s clock")
        tail = f"  ({', '.join(extras)})" if extras else ""
        print(f"  {m['played']} of {m['games_requested']} games{tail}")


# -------------------------------------------------------------------- price

def cmd_price_deck(args: argparse.Namespace) -> None:
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
    # `fsum`, not `sum`, for the reason `artifacts/generate.py` sets out: a
    # bare `sum` over floats is compensated on CPython 3.12 and left to right
    # on 3.11. This is the same money total the shopping list prints, from
    # the same column, and two spellings of one sum answering two ways is
    # how a difference nobody can explain gets discovered.
    total = fsum(r[1] for r in rows)
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

def _auth() -> tuple[ModuleType, ModuleType, ModuleType, ModuleType]:
    """The auth package, or a clean exit naming the extra that is missing."""
    try:
        from mtglab.auth import db, passwords, sessions, users
    except ModuleNotFoundError:
        # argon2-cffi rides with the api extra; see pyproject.toml.
        sys.exit("accounts need the api extra:  pip install -e '.[api]'")
    return db, passwords, sessions, users


def _connect() -> sqlite3.Connection:
    """`app.db`, with the maintainer reconciled to admin first (ADR 17).

    Every `users` command goes through here rather than calling `db.connect`,
    so the CLI and the app agree about who administers the instance no matter
    which one ran last. It is a no-op unless `MTGLAB_ADMIN_EMAIL` is set.
    """
    db, _, _, _ = _auth()
    from mtglab.auth import bootstrap

    con: sqlite3.Connection = db.connect()
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


def cmd_users_add(args: argparse.Namespace) -> None:
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


def _username_for(email: str, users: ModuleType) -> str:
    """A default login handle from an address' local part.

    `ada.lovelace@example.com` becomes `ada.lovelace`, which is both usable and
    the name the person would have picked. When it is not -- a `+` tag, a
    single character, a handle somebody already has -- the command asks for
    `--username` rather than inventing `ada2`, because a login handle chosen by
    a mangling rule is one its owner has to be told.
    """
    local = email.partition("@")[0]
    handle: str = users.normalise_username(local)
    return handle


def cmd_users_invite(args: argparse.Namespace) -> None:
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


def cmd_users_list(args: argparse.Namespace) -> None:
    _db, _, sessions, users = _auth()
    from mtglab.auth import tokens
    from mtglab.claude import tiers

    con = _connect()
    try:
        everyone = users.all_users(con)
        if not everyone:
            print(f"  no accounts in {config.APP_DB_PATH}")
            print("  create one with `mtglab users add <name>`")
            return
        print(f"  {'username':<20} {'email':<28} {'state':<12} "
              f"{'answered by':<12} sessions")
        for user in everyone:
            state = "disabled" if user.disabled else (
                "active" if users.has_password(con, user.id)
                else ("invited" if tokens.outstanding(con, user.id,
                                                      tokens.Purpose.INVITE)
                      else "no password"))
            live = sessions.count_for_user(con, user.id)
            admin = "*" if user.is_admin else " "
            answered = tiers.get(user.model_tier).label
            print(f" {admin}{user.username:<20} {user.email or '-':<28} "
                  f"{state:<12} {answered:<12} {live}")
        print("\n  * admin")
    finally:
        con.close()


def cmd_users_passwd(args: argparse.Namespace) -> None:
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


def _set_disabled(username: str, disabled: bool) -> None:
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


def cmd_users_disable(args: argparse.Namespace) -> None:
    _set_disabled(args.username, True)


def cmd_users_enable(args: argparse.Namespace) -> None:
    _set_disabled(args.username, False)


def cmd_users_delete(args: argparse.Namespace) -> None:
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


def _set_admin(username: str, is_admin: bool) -> None:
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


def cmd_users_tier(args: argparse.Namespace) -> None:
    """Choose which Claude answers an account. The break-glass twin of the
    Admin page's control, and the only one that works on a fresh volume.

    `--tier default` clears the grant rather than naming the default tier, so
    "nobody has chosen anything" has one spelling in the column whichever door
    it came through (`users.set_model_tier` stores NULL either way).
    """
    from mtglab.claude import tiers

    _db, _, _, users = _auth()
    wanted = None if args.tier == "default" else args.tier

    con = _connect()
    try:
        user = users.get(con, args.username)
        if user is None:
            sys.exit(f"refused: no account {args.username!r}")
        try:
            users.set_model_tier(con, user.id, wanted)
        except users.UnknownTier:
            offered = ", ".join(t["key"] for t in tiers.roster())
            sys.exit(f"refused: no such tier {args.tier!r} -- "
                     f"one of: default, {offered}")
        chosen = tiers.get(wanted)
        print(f"  {user.username} is answered by {chosen.label}")
        print(f"  {chosen.blurb}")
    finally:
        con.close()


def cmd_users_promote(args: argparse.Namespace) -> None:
    _set_admin(args.username, True)


def cmd_users_demote(args: argparse.Namespace) -> None:
    _set_admin(args.username, False)


# -------------------------------------------------------------------- claude

def cmd_claude_check(args: argparse.Namespace) -> None:
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


def cmd_claude_interview(args: argparse.Namespace) -> None:
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


def _wrapped(text: str, indent: str = "  ", width: int = 76) -> str:
    import textwrap
    return textwrap.fill(text, width=width, initial_indent=indent,
                         subsequent_indent=indent)


def cmd_claude_argue(args: argparse.Namespace) -> None:
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


def cmd_claude_dossier(args: argparse.Namespace) -> None:
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

    def passage(title: str, section: dict[str, Any],
                extra: str = "") -> None:
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


def cmd_claude_research(args: argparse.Namespace) -> None:
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


def cmd_claude_usage(args: argparse.Namespace) -> None:
    """Where the Claude money went, from the ledger in app.db.

    **This prints dollars now, and that reverses what this docstring used to
    say.** The old wording was "tokens and not dollars, deliberately: prices
    move (Sonnet 5's introductory rate ends 2026-08-31) and a stale hardcoded
    price table would turn an honest count into a wrong invoice." The objection
    was right and still is; what changed (2026-08-18, Aaron's call) is that an
    app which is free forever is one whose bill somebody has to watch, and an
    imperfect answer beats none. `claude/prices.py` is the table, and every
    part of its design answers that sentence rather than ignoring it — dated
    rates, a modelled changeover for the very rate the objection named, and an
    unknown model counted rather than priced at zero.

    Two roll-ups, because they answer different questions: per mode is which
    surface spent it, per model is on which Claude — the axis per-account
    tiers made worth asking. Only the second is priced; pricing the first
    would mean guessing a rate for rows that span models, and the guess would
    look like arithmetic.
    """
    from mtglab.claude import ledger, prices

    since = f"{args.since}T00:00:00+00:00" if args.since else None
    rows = ledger.summary(since=since)
    if not rows:
        print("  no conversations recorded"
              + (f" since {args.since}" if args.since else "") + ".")
        return

    def table(title: str, key: str, data: list[dict[str, Any]]) -> None:
        # 34 wide: `theme-conversation:fortune-teller` is 32 characters, and a
        # mode name carrying a persona is the longest thing this column holds.
        # At 22 the numbers no longer lined up under their headings, which on
        # a table whose whole job is comparison is not a cosmetic complaint.
        print(f"\n  {title:<34} {'conv':>5} {'req':>5} "
              f"{'tokens in':>11} {'tokens out':>11} {'cached':>11}")
        for row in data:
            print(f"  {row[key]:<34} {row['conversations']:>5} "
                  f"{row['requests']:>5} {row['input_tokens']:>11} "
                  f"{row['output_tokens']:>11} {row['cache_read_tokens']:>11}")
        print(f"  {'total':<34} {sum(r['conversations'] for r in data):>5} "
              f"{sum(r['requests'] for r in data):>5} "
              f"{sum(r['input_tokens'] for r in data):>11} "
              f"{sum(r['output_tokens'] for r in data):>11} "
              f"{sum(r['cache_read_tokens'] for r in data):>11}")

    table("mode", "mode", rows)

    by_model = ledger.summary(since=since, by="model")
    table("answered by", "model", by_model)

    spend = prices.estimate(by_model)
    print(f"\n  estimated {prices.render(spend.usd)} at list rates read "
          f"{prices.CHECKED.isoformat()}.")
    if not spend.complete:
        # Said out loud rather than folded into the figure. A model with no
        # rate contributes nothing, so a total printed without this is wrong
        # downward and reads as reassuring.
        unknown = ", ".join(spend.unpriced_models)
        print(f"  {spend.unpriced} conversation(s) could not be priced -- no "
              f"rate for {unknown}.\n  The figure above excludes them; add "
              f"the rate in claude/prices.py.")

    print("\n  'cached' counts prompt cache reads, at ~a tenth of the input "
          "price. They sit\n  *beside* 'tokens in' rather than inside it -- "
          "the API reports 'tokens in' as\n  the uncached remainder only, so "
          "a conversation's whole prompt is the two\n  added together.")
    print("  Prompt cache *writes* bill at 1.25x input and are not recorded "
          "at all, so\n  the figure is a floor on the bill rather than the "
          "bill. It is an estimate from\n  list rates, not an invoice: "
          f"{prices.SOURCE}")
    first = min(r["first_at"] for r in rows)
    last = max(r["last_at"] for r in rows)
    print(f"  counting {first[:10]} to {last[:10]}.\n")


# --------------------------------------------------------------------- main

# ---------------------------------------------------------------- cardmotion

def cmd_cardmotion_build(args: argparse.Namespace) -> None:
    """Derive a card's motion (ADR 32): pool facts, Scryfall art, cached
    derivative. A dev-machine run -- the app only ever serves the cache."""
    _animist_pixels()
    from mtglab.cardmotion.build import BuildRefused, build_derivative

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
    from mtglab.cardmotion.effects import EFFECTS
    chosen = EFFECTS.get(args.effect)
    if chosen is not None and chosen.needs_depth:
        from mtglab.cardmotion.depth import DepthError, load_model
        try:
            print("loading the depth model (first run downloads weights)...")
            model = load_model()
        except DepthError as exc:
            sys.exit(f"refused: {exc}")

    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
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
    from mtglab.cardmotion.build import BuildRefused, sync

    if not config.DB_PATH.exists():
        sys.exit("no card pool -- run `mtglab data refresh` first; card "
                 "facts come from the pool (rule 1)")
    from mtglab.cardmotion.effects import EFFECTS
    chosen = EFFECTS.get(args.effect)
    if chosen is None:
        sys.exit(f"refused: unknown effect {args.effect!r} "
                 f"(one of: {', '.join(sorted(EFFECTS))})")

    def load_model_once() -> DepthModel:
        from mtglab.cardmotion.depth import DepthError, load_model
        try:
            print("loading the depth model (first run downloads weights)...")
            return load_model()
        except DepthError as exc:
            raise BuildRefused(str(exc)) from exc

    from mtglab.cards import db
    con = db.connect(config.DB_PATH)
    decks = load_all_decks()
    report = sync(con, decks, effect_key=args.effect,
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
    from mtglab.cardmotion import cache

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
        print("  nothing derived yet -- `mtglab cardmotion build --deck "
              "<slug> --effect depth-drift` is the first run")


# ------------------------------------------------------------------- animist

def _animist_recipe(path_str: str | Path) -> Recipe:
    """Load and validate, or refuse with the schema's own sentence."""
    from mtglab.animist.recipe import RecipeError, load_recipe

    try:
        return load_recipe(Path(path_str))
    except RecipeError as exc:
        sys.exit(f"refused: {exc}")


def _animist_pixels() -> None:
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


def cmd_animist_build(args: argparse.Namespace) -> None:
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


def cmd_animist_fetch(args: argparse.Namespace) -> None:
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


def cmd_animist_licence(args: argparse.Namespace) -> None:
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


def cmd_animist_verify(args: argparse.Namespace) -> None:
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


def cmd_animist_measure(args: argparse.Namespace) -> None:
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


# ------------------------------------------------------------------- bench

def cmd_bench_run(args: argparse.Namespace) -> None:
    """Time the declared suite, cold or warm, and print a ledger-ready table."""
    from mtglab.bench import run as benchrun
    from mtglab.bench import targets as benchtargets

    picked = benchtargets.suite()
    if args.only:
        picked = [t for t in picked if args.only.lower() in t.name.lower()]
        if not picked:
            sys.exit(f"nothing in the suite matches {args.only!r}")

    state = "cold" if args.cold else "warm"
    print(f"timing {len(picked)} targets, {args.runs} runs each, {state}\n")
    samples = benchrun.run_suite(picked, runs=args.runs, cold=args.cold,
                                 profile_over_ms=args.profile_over)
    print(benchrun.as_markdown(samples, cold=args.cold))

    slow = [s for s in samples if s.profile is not None]
    if slow:
        print("\nAnything over "
              f"{args.profile_over:.0f}ms is profiled, because a number this "
              "size is a question:")
        for s in slow:
            prof = s.profile
            if prof is None:                       # unreachable; narrows Profile | None
                continue
            print(f"\n  {s.target.name}")
            print(f"    wall {prof.wall_s * 1000:.1f}ms  ="
                  f"  database {prof.db_s * 1000:.1f}ms"
                  f" ({prof.queries.count} statements)"
                  f"  +  everything else {prof.other_s * 1000:.1f}ms")
            print(f"    imports: {prof.import_calls} calls into importlib")
            print(f"    {prof.verdict()}")
            for frame in prof.frames[:5]:
                print(f"      {frame.tottime * 1000:7.2f}ms  {frame.where}")

    missing = [s for s in samples if s.skipped]
    if missing:
        print("\nnot measured here:")
        for s in missing:
            print(f"  {s.target.name}: {s.skipped}")


def cmd_bench_profile(args: argparse.Namespace) -> None:
    """One target, profiled in full."""
    from mtglab.bench import profile as benchprofile
    from mtglab.bench import targets as benchtargets

    matches = [t for t in benchtargets.suite()
               if args.target.lower() in t.name.lower()]
    if not matches:
        sys.exit(f"no target matches {args.target!r} -- "
                 f"`mtglab bench list` shows them all")
    target = matches[0]
    if target.unavailable:
        sys.exit(f"{target.name}: {target.unavailable}")

    prof = benchprofile.profile_target(target.name, target.call,
                                       repeat=args.repeat, top=args.top)
    print(f"{target.name}  ({args.repeat} runs)\n")
    print(f"  wall              {prof.wall_s * 1000:8.2f}ms")
    print(f"  database          {prof.db_s * 1000:8.2f}ms  "
          f"({prof.queries.count} statements, "
          f"{prof.db_share:.0%} of the wall)")
    print(f"  everything else   {prof.other_s * 1000:8.2f}ms")
    print(f"  import machinery  {prof.import_calls:8d} calls "
          f"({prof.import_share:.1%} of the traced run)")
    repeat = prof.queries.worst_repeat()
    if repeat is not None:
        print(f"\n  most-repeated statement, {repeat[1]}x:\n    {repeat[0]}")
    if prof.queries.slowest_sql:
        print(f"\n  slowest statement, {prof.queries.slowest_s * 1000:.1f}ms:"
              f"\n    {prof.queries.slowest_sql}")
    print(f"\n  {prof.verdict()}")
    print("\n  hottest frames -- a RANKING, not a budget: cProfile's clock is "
          "inflated\n  per call, so read which line rather than how many ms.")
    for frame in prof.frames:
        print(f"    {frame.tottime * 1000:8.2f}ms  {frame.calls:7d}x  "
              f"{frame.where}")


def cmd_bench_caches(args: argparse.Namespace) -> None:
    """What this process memoises, and whether any of it is earning its keep."""
    from mtglab import caches
    from mtglab.bench import run as benchrun
    from mtglab.bench import targets as benchtargets

    # Import the app so every cache has registered before anything is asked.
    picked = [t for t in benchtargets.suite() if not t.unavailable]
    caches.reset_stats()
    benchrun.run_suite(picked, runs=args.runs, cold=False,
                       profile_over_ms=float("inf"))

    print(f"after {args.runs} runs of {len(picked)} targets:\n")
    print(f"  {'cache':18} {'hits':>7} {'misses':>7} {'rate':>7} {'held':>6}")
    for row in caches.report():
        rate = "never" if row.rate is None else f"{row.rate:.0%}"
        held = "-" if row.size is None else str(row.size)
        print(f"  {row.name:18} {row.hits:7d} {row.misses:7d} {rate:>7} "
              f"{held:>6}")
        if row.note:
            print(f"    {row.note}")
    dead = [r for r in caches.report() if r.rate is not None and r.rate == 0.0]
    if dead:
        print("\n  A cache that never hits is complexity wearing a win's "
              "clothes:")
        for row in dead:
            print(f"    {row.name}")


def cmd_bench_list(args: argparse.Namespace) -> None:
    """The declared suite, and what each target needs in order to run."""
    from mtglab.bench import targets as benchtargets
    for target in benchtargets.suite():
        mark = f"unavailable: {target.unavailable}" if target.unavailable \
            else target.note
        print(f"  [{target.kind:8}] {target.name}")
        if mark:
            print(f"             {mark}")


# ------------------------------------------------------------------ mutate

def cmd_mutate_run(args: argparse.Namespace) -> None:
    """Break the code on purpose and count what the suite never noticed."""
    from mtglab import mutate as mutaterun

    src = Path(__file__).resolve().parents[1]
    available = mutaterun.catalogue(src)
    if args.only:
        print(f"{len(available)} mutation sites across "
              f"{len(mutaterun.TARGETS)} modules; re-running the ones "
              f"matching {', '.join(args.only)}\n")
    else:
        print(f"{len(available)} mutation sites across "
              f"{len(mutaterun.TARGETS)} modules; sampling {args.sample} "
              f"at seed {args.seed}\n")

    def announce(result: Result) -> None:
        verdict = "killed  " if result.killed else "SURVIVED"
        print(f"  {verdict} {result.seconds:5.1f}s  "
              f"{result.mutation.describe()}")

    try:
        report = mutaterun.run(sample=args.sample, seed=args.seed,
                               full=args.full, src=src, only=args.only,
                               on_result=announce)
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc
    rate = report.kill_rate
    drawn = ("named" if args.only
             else f"drawn from {report.sites} sites, seed {report.seed}")
    print(f"\nkill rate {rate:.0%} -- {report.killed} of "
          f"{len(report.results)}, {drawn}")
    if report.survivors:
        print("\nSurvivors. Each is a question rather than a verdict: some "
              "are\nequivalent mutants no test could ever kill, and telling "
              "those apart\nis reading work this tool does not do for you.")
        for result in report.survivors:
            print(f"\n  {result.mutation.describe()}")
            print(f"    ran: {' '.join(result.tests)}")
    print("\nThe working tree was never touched -- every mutation was applied "
          "to a\nthrowaway copy of the package. Nothing to restore.")


def cmd_mutate_list(args: argparse.Namespace) -> None:
    """Every mutation the catalogue can make, by module and kind."""
    from collections import Counter

    from mtglab import mutate as mutaterun

    src = Path(__file__).resolve().parents[1]
    available = mutaterun.catalogue(src)
    by_module = Counter(m.relpath for m in available)
    by_kind = Counter(m.operator for m in available)
    for relpath, count in sorted(by_module.items()):
        tests = " ".join(mutaterun.TARGETS.get(relpath, ()))
        print(f"  {count:5d}  {relpath}")
        print(f"         defended by: {tests or '(the whole suite)'}")
    print(f"\n  {len(available)} sites in total")
    for kind, count in by_kind.most_common():
        print(f"    {kind:12} {count}")


def main(argv: Sequence[str] | None = None) -> None:
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
    st.add_argument("--themes",
                    help="what the deck is about, comma-separated from the "
                         "hand-curated vocabulary (see `decks/model.py`); "
                         "'' clears the list. Strategy words included -- "
                         f"declare {', '.join(ARCHETYPES)} here and the "
                         "worst-piloted becomes the deck's rating board "
                         "(ADR 37)")
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
    sh = sim.add_parser("shelf",
                        help="Tier 1.5 -- the closed form: coloured source "
                             "requirements, a land count and per-card lag")
    sh.add_argument("slug")
    sh.add_argument("--target", type=float, default=0.90,
                    help="consistency to judge against (default 0.90)")
    sh.add_argument("--on-the-draw", action="store_true",
                    help="judge on the draw; the default is the harder case")
    sh.add_argument("--top", type=int, default=15,
                    help="how many of the latest cards to list")
    sh.set_defaults(func=cmd_sim_shelf)
    mu = sim.add_parser("mulligan",
                        help="search keep rules and report the best policy")
    mu.add_argument("slug")
    mu.add_argument("--games", type=int, default=2000,
                    help="games per rule; the grid is ~33 rules")
    mu.add_argument("--turns", type=int, default=10)
    mu.add_argument("--seed", type=int, default=7)
    mu.add_argument("--top", type=int, default=10)
    mu.set_defaults(func=cmd_sim_mulligan)
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
    fm = sim.add_parser("matches",
                        help="the match ledger -- every Forge match recorded")
    fm.add_argument("--limit", type=int, default=20)
    fm.set_defaults(func=cmd_sim_matches)

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
    # Which Claude answers this seat. `default` clears the grant; every other
    # value is checked against `claude/tiers.py` rather than listed here, so
    # adding a tier is one table and not two.
    ut = us.add_parser("tier", help="which Claude answers this account")
    ut.add_argument("username")
    ut.add_argument("--tier", required=True,
                    help="default | sonnet | opus | fable")
    ut.set_defaults(func=cmd_users_tier)
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
                           help="what every mode and model has spent -- "
                                "tokens, requests, and an estimate in dollars")
    cu.add_argument("--since", metavar="YYYY-MM-DD",
                    help="count only conversations on or after this date")
    cu.set_defaults(func=cmd_claude_usage)

    # Wake still images into the site's scenery -- fetched free-use,
    # licence-checked per file, every step written down (ADR 29). The gate
    # blocks: there is no flag past a refused licence, deliberately.
    # Card-art motion (ADR 32): derived at runtime into the gitignored
    # cache, never committed -- the runtime counterpart of the animist below.
    cm = sub.add_parser("cardmotion").add_subparsers(dest="cmd",
                                                     required=True)
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

    # The measuring shelf. Developer tooling, like `animist` above: the app
    # never imports it and the container never ships a benchmark.
    bench = sub.add_parser(
        "bench", help="what the app costs, and where the cost actually is"
    ).add_subparsers(dest="cmd", required=True)
    br = bench.add_parser("run", help="time the declared suite and profile "
                                      "anything slow enough to be a question")
    br.add_argument("--runs", type=int, default=12,
                    help="samples per target; the report is a median and a "
                         "p95, never a mean")
    br.add_argument("--cold", action="store_true",
                    help="empty every registered cache between samples -- the "
                         "first-request price, which is a different number "
                         "and needs its own ledger row")
    br.add_argument("--only", help="substring of a target name")
    br.add_argument("--profile-over", type=float, metavar="MS",
                    default=PROFILE_OVER_MS,
                    help="profile any target slower than this")
    br.set_defaults(func=cmd_bench_run)
    bp = bench.add_parser("profile", help="one target, in full: the database "
                                          "budget, the imports, the frames")
    bp.add_argument("target", help="substring of a target name")
    bp.add_argument("--repeat", type=int, default=5)
    bp.add_argument("--top", type=int, default=15)
    bp.set_defaults(func=cmd_bench_profile)
    bc = bench.add_parser("caches", help="hit rates -- a cache that never "
                                         "hits is complexity, not a win")
    bc.add_argument("--runs", type=int, default=6)
    bc.set_defaults(func=cmd_bench_caches)
    bench.add_parser("list", help="the declared suite").set_defaults(
        func=cmd_bench_list)

    # Ikoria's keyword, and the operation is the same one: something is
    # merged into what is already there, and the board has to answer it.
    mutate = sub.add_parser(
        "mutate", help="break the code on purpose; count what nobody noticed"
    ).add_subparsers(dest="cmd", required=True)
    mr = mutate.add_parser("run", help="a seeded sample of mutations, applied "
                                       "to a throwaway copy of the package")
    mr.add_argument("--sample", type=int, default=12,
                    help="how many mutations to draw")
    mr.add_argument("--seed", type=int, default=0,
                    help="the same seed draws the same sample, so a kill rate "
                         "can be checked rather than only quoted")
    mr.add_argument("--full", action="store_true",
                    help="run the whole suite against each mutation instead "
                         "of the tests that ought to defend it -- slower, and "
                         "the only way a survivor is a claim about the suite")
    mr.add_argument("--only", action="append", metavar="PATH:LINE",
                    help="re-run named sites instead of drawing a sample -- "
                         "'decks/analyze.py:33'. Repeatable. This is how a "
                         "survivor the ledger recorded gets re-checked; a "
                         "pattern matching nothing is an error, never an "
                         "empty run")
    mr.set_defaults(func=cmd_mutate_run)
    mutate.add_parser("list", help="every site the catalogue can reach"
                      ).set_defaults(func=cmd_mutate_list)

    args = p.parse_args(argv)
    args.func(args)


if __name__ == "__main__":
    main()
