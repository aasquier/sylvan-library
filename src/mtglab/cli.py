"""mtglab command line.

    mtglab data refresh                 pull Scryfall bulk + load DuckDB
    mtglab data snapshot                append today's prices to history
    mtglab decks list
    mtglab decks import <slug> --from   a pasted decklist -> a draft deck
    mtglab decks validate <slug>        the gate -- run before anything else
    mtglab decks promote <slug>         a draft becomes curated, once justified
    mtglab decks delete <slug>          remove one; it moves to decks/.trash/
    mtglab decks build <slug>           generate the artifacts
    mtglab sim mana <slug>              Tier 1 goldfish
    mtglab sim lands <slug> 30..40      land-count sweep, flood-aware
    mtglab price deck <slug>            cheapest legal printing per card
    mtglab users add <name>             an account; the password is prompted
    mtglab users list                   who exists, and who can log in
    mtglab users passwd <name>          set a password; ends every session
    mtglab users disable|enable <name>
    mtglab claude check                 one real API call -- is the key live?
    mtglab claude interview <slug>      questions about a card's slot; you
                            --card X    write the rationale, it never does
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from mtglab import config
from mtglab.decks.model import CATEGORIES, DECK_STAGES, DECK_STATUSES, Deck
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
    """Change one field -- of a card with `--card`, of the deck without it.

    Exactly one field per invocation, matching the operation underneath. A
    rationale is taken verbatim from the argument: nothing here writes one, and
    a blank one on a curated deck is refused rather than filled in (rule 4).
    """
    from mtglab.api import service

    card_fields = (("why", args.why), ("category", args.category),
                   ("qty", args.qty))
    deck_fields = (("stage", args.stage), ("status", args.status),
                   ("bracket", args.bracket))
    chosen = [(f, v) for f, v in card_fields + deck_fields if v is not None]
    if len(chosen) != 1:
        sys.exit("choose exactly one of --why, --category, --qty (with --card) "
                 "or --stage, --status, --bracket (without)")
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

    Interactive by default: it prints what is about to go and asks for the slug
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
        # Reading the slug back, not a y/n. The point is that the answer is
        # only producible by somebody looking at the right deck.
        typed = input(f"  type the slug to delete it [{args.slug}]: ").strip()
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
# What `users add` is for until then is the maintainer's own account, typed at
# a prompt on a machine they are sitting at (`docs/HOSTING.md` §4 step 8). It
# can also create an account with no password at all, which is exactly the
# state an invite leaves behind: the account exists and cannot log in yet.

def _auth():
    """The auth package, or a clean exit naming the extra that is missing."""
    try:
        from mtglab.auth import db, passwords, sessions, users
    except ModuleNotFoundError:
        # argon2-cffi rides with the api extra; see pyproject.toml.
        sys.exit("accounts need the api extra:  pip install -e '.[api]'")
    return db, passwords, sessions, users


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
    db, _, _, users = _auth()

    password = None if args.no_password else _prompt_new_password(args.username)
    con = db.connect()
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


def cmd_users_list(args):
    db, _, sessions, users = _auth()

    con = db.connect()
    try:
        everyone = users.all_users(con)
        if not everyone:
            print(f"  no accounts in {config.APP_DB_PATH}")
            print("  create one with `mtglab users add <name>`")
            return
        print(f"  {'username':<20} {'email':<28} {'state':<12} sessions")
        for user in everyone:
            state = "disabled" if user.disabled else (
                "active" if users.has_password(con, user.id) else "no password")
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
    db, _, _, users = _auth()

    con = db.connect()
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
    db, _, _, users = _auth()

    con = db.connect()
    try:
        user = users.get(con, username)
        if user is None:
            sys.exit(f"refused: no account {username!r}")
        ended = users.set_disabled(con, user.id, disabled)
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
    us.add_parser("list").set_defaults(func=cmd_users_list)
    up = us.add_parser("passwd", help="set a password; ends every session")
    up.add_argument("username"); up.set_defaults(func=cmd_users_passwd)
    ud = us.add_parser("disable"); ud.add_argument("username")
    ud.set_defaults(func=cmd_users_disable)
    ue = us.add_parser("enable"); ue.add_argument("username")
    ue.set_defaults(func=cmd_users_enable)

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

    args = p.parse_args(argv)
    args.func(args)


if __name__ == "__main__":
    main()
