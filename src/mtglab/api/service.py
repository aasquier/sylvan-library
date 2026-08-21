"""Everything the API serves, as plain functions over plain data.

Routes stay thin on purpose: they parse arguments and call in here. That keeps
the interesting logic testable without HTTP, and keeps a single implementation
behind both the CLI and the app.
"""

from __future__ import annotations

import contextlib
import json
import re
import threading
import time
import urllib.request
from datetime import date
from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

import yaml

from mtglab import caches, colors, config, lore
from mtglab import glossary as gloss
from mtglab.cards import db, identify
from mtglab.cards.db import art_crop_from
from mtglab.decks import decklist, edit, importer, log, partners, suggest, wheel
from mtglab.decks.analyze import deck_stats
from mtglab.decks.edit import EditFailed
from mtglab.decks.importer import ImportRefused
from mtglab.decks.library import Library
from mtglab.decks.model import CATEGORIES, DECK_STATUSES, Deck
from mtglab.decks.source import (
    DeckExists,
    DeckNotFound,
    DeckSource,
    FileDeckSource,
    ReadOnlySource,
)
from mtglab.decks.validate import validate

if TYPE_CHECKING:
    # Type-only, so the lazy-import discipline holds at runtime: duckdb is
    # imported inside `cards.db.connect_readonly` and never here, and the
    # Claude package stays out of a base install's import graph entirely.
    from duckdb import DuckDBPyConnection

    from mtglab.cards.db import CardRecord
    from mtglab.claude.modes import Mode
    from mtglab.decks.model import CardEntry
    from mtglab.decks.validate import ValidationReport

SCRYFALL_SETS = "https://api.scryfall.com/sets"
USER_AGENT = "mtg-lab/0.1 (local personal deckbuilding tool)"


def _source(source: DeckSource | None) -> DeckSource:
    """The caller's deck source, or the default file-backed library.

    Routes always pass one, resolved from the request scope in `deps.py`. The
    default keeps these functions callable from a script or a test without
    ceremony, which is the property that made them worth extracting from the
    routes in the first place.
    """
    return source if source is not None else FileDeckSource()


# ------------------------------------------------------------------- pool

#: One read-only handle held open for as long as the pool file is unchanged,
#: and never queried. See `_pin`.
_KEEPER: DuckDBPyConnection | None = None
#: What the pool file looked like when `_KEEPER` was opened: (mtime_ns, size).
_KEEPER_STAMP: tuple[int, int] | None = None
#: Sync endpoints run in Starlette's threadpool and jobs run in their own
#: workers, so two requests can be in `_pin` at once.
_KEEPER_LOCK = threading.Lock()
#: `time.monotonic()` when the keeper was last wanted. See `_reap_keeper`.
_KEEPER_USED = 0.0
#: How long the keeper may go unwanted before it lets the pool go.
#:
#: Long enough that it spans a person reading a page and clicking the next
#: one, short enough that nobody waits on it: a `data refresh` started while
#: the app is idle blocks for at most this.
#:
#: **It must also be shorter than the platform's health-check interval, and
#: this was 30.0 against a 30s check until 2026-08-19.** `fly.toml` calls
#: `/api/health` every thirty seconds and `service.health` opens the pool --
#: it counts both tables and asks `pool_stale` -- so the lease was renewed by
#: the one caller that never stops asking, exactly as often as it expired.
#: The app therefore held the shared lock forever and `mtglab data refresh`
#: could not take the exclusive one **at all** on the instance: forty
#: consecutive attempts over five minutes, every one refused by the same
#: holder. `_reap_keeper` below argues at length that the keeper must not
#: lock a refresh out; it was right, and the number underneath it was not.
#:
#: Ten seconds still spans the burst the lease exists for -- a page load is
#: four requests -- and leaves twenty seconds of every health-check cycle in
#: which the pool is free. `tests/test_pool_keeper.py` derives the ceiling
#: from `fly.toml` rather than restating it, so moving the check's interval
#: fails there instead of silently re-closing the door.
_KEEPER_IDLE = 10.0
_REAPER: threading.Thread | None = None


def _drop_keeper() -> None:
    with _KEEPER_LOCK:
        _release_keeper()


#: A hit here is the stamp matching, which is exactly what the keeper is for:
#: DuckDB's loaded instance stayed alive, so the next `connect_readonly` costs
#: 0.7ms instead of 17.5ms. A low rate means the lease is expiring inside the
#: burst it exists to cover -- a finding about `_KEEPER_IDLE`, not about DuckDB.
_KEEPER_STATS = caches.register(
    "pool.keeper", clear=_drop_keeper, holds_handle=True,
    size=lambda: 0 if _KEEPER is None else 1,
    note="the held read-only handle that keeps the pool's instance loaded")


def _reap_keeper() -> None:
    """Let go of the pool once nobody has asked for it in `_KEEPER_IDLE`.

    **A held handle is a held lock.** DuckDB gives a read-only connection a
    shared lock on the file, and `mtglab data refresh` wants an exclusive one
    -- so a keeper held for the life of the process would mean that a running
    `mtglab ui` refuses every refresh, on Aaron's own laptop, with an error
    about a lock rather than about a server. That is a worse bug than the
    17.5ms `_pin` exists to save, and it is the exact inversion of the promise
    `cards.db.connect_readonly` makes: the app opens the pool read-only *so
    that* a refresh degrades it rather than locking it out. It must not repay
    that by locking the refresh out.

    So the keeper is a lease rather than a claim. Requests arrive in bursts --
    a page load is four of them -- and the lease covers the burst; half a
    minute after the last one the pool is free again, and a refresh started
    against an idle app waits `_KEEPER_IDLE` at worst. A daemon thread, so it
    never keeps the process alive, and one per process: the reaper is started
    under the same lock that owns the keeper it reaps.
    """
    while True:
        time.sleep(_KEEPER_IDLE / 3)
        _reap_once()


def _reap_once() -> bool:
    """One pass of the lease check. True when the pool was handed back.

    Split out of the loop so the *decision* is testable without waiting on a
    sleep -- a reaper tested only through its thread is a reaper tested by
    whether the test was patient enough, which is how a lease that never
    fires goes green.
    """
    with _KEEPER_LOCK:
        if (_KEEPER is not None
                and time.monotonic() - _KEEPER_USED > _KEEPER_IDLE):
            _release_keeper()
            return True
    return False


def _release_keeper() -> None:
    """Drop the keeper. Callers hold `_KEEPER_LOCK`."""
    global _KEEPER, _KEEPER_STAMP
    if _KEEPER is not None:
        # A handle that will not close is a handle already gone; the only
        # thing this function owes anybody is that the reference is dropped.
        with contextlib.suppress(Exception):
            _KEEPER.close()
    _KEEPER, _KEEPER_STAMP = None, None


def _pin(path: Path) -> None:
    """Keep DuckDB's database instance alive between requests.

    **Opening a DuckDB file is only cheap while something already has it
    open.** The Python client caches the loaded database instance per path and
    frees it when the last connection closes -- so an app that opens a handle,
    answers, and closes it pays the full load again on the very next request:
    17.5ms of nothing, on every endpoint that touches the pool. Hold one
    connection that is never used for anything, and the same open costs 0.7ms
    (measured 2026-08-19, this laptop, the full 78MB pool).

    This is deliberately *not* a shared connection. Every caller still opens
    its own handle and still closes it in a `finally`, exactly as before --
    the contract does not change, only the price. A shared connection would
    serialise every query in the app behind one lock and make a caller's
    `close()` a bug affecting every other request.

    The stamp is what keeps it honest. A cached instance is a snapshot: with
    the keeper held forever, a `mtglab data refresh` that rewrites the file
    would be invisible to a running app until it was restarted -- and Aaron
    runs exactly that, on this laptop, with the UI open. So the file's
    mtime and size are checked on the way past (a `stat`, microseconds) and a
    pool that moved drops the keeper, which frees the instance once the last
    live handle closes and lets the next connection read the new file.
    """
    global _KEEPER, _KEEPER_STAMP, _KEEPER_USED, _REAPER
    try:
        st = path.stat()
    except OSError:
        return
    stamp = (st.st_mtime_ns, st.st_size)
    _KEEPER_USED = time.monotonic()
    if _KEEPER is not None and stamp == _KEEPER_STAMP:
        _KEEPER_STATS.hit()
        return
    _KEEPER_STATS.miss()
    with _KEEPER_LOCK:
        if _KEEPER is not None and stamp == _KEEPER_STAMP:
            return
        _release_keeper()
        try:
            _KEEPER = cast("DuckDBPyConnection",
                           db.connect_readonly(config.DB_PATH))
            _KEEPER_STAMP = stamp
        except Exception:                                           # noqa: BLE001
            # A `data refresh` mid-write, or a pool that is not readable yet.
            # The keeper is an optimisation and nothing else: without it every
            # request simply pays the old price.
            _KEEPER, _KEEPER_STAMP = None, None
        if _REAPER is None and _KEEPER is not None:
            _REAPER = threading.Thread(target=_reap_keeper, daemon=True,
                                       name="mtglab-pool-keeper")
            _REAPER.start()


def _connect() -> DuckDBPyConnection | None:
    """A read-only handle, or None when the pool has not been built yet.

    Read-only matters: DuckDB allows one writer, so a running `data refresh`
    would otherwise lock the whole app out.

    `config.DB_PATH` is read here rather than imported as a name, for the
    reason `config.py` exists: binding it at import time makes
    `config.use_paths()` silently ineffective, so a test can never point the
    service at a scratch pool. `deck_paths` and `FileDeckSource` already
    resolve at call time; this was the last place that did not.
    """
    path = Path(config.DB_PATH)
    if not path.exists():
        return None
    _pin(path)
    try:
        # `cards.db.Connection` is `Any` -- duckdb ships no stubs -- so the
        # cast is what keeps this function's own return type meaningful.
        return cast("DuckDBPyConnection", db.connect_readonly(config.DB_PATH))
    except Exception:                                               # noqa: BLE001
        # Most likely a `data refresh` holding the write lock. The app stays
        # usable in degraded form rather than failing every request.
        return None


def pool_stale(con: DuckDBPyConnection) -> bool:
    """Whether the pool predates the columns the app reads. Never raises.

    Wrapped because `health()` must answer on a database in any state at all,
    including one from a future version of this code. A health check that
    500s is worse than one that says "I could not tell".
    """
    try:
        return db.pool_is_stale(con)
    except Exception:                                               # noqa: BLE001
        return False


def health(*, source: DeckSource | None = None) -> dict[str, Any]:
    con = _connect()
    if con is None:
        return {"pool": False, "oracle_cards": 0, "printings": 0,
                "message": "no card pool yet -- run `mtglab data refresh`"}
    try:
        def count(table: str) -> int:
            # `fetchone` is typed as possibly-None. count(*) always yields a
            # row, but this is the platform's health-check target and must
            # not 500 on the impossible case either.
            row = con.execute(f"SELECT count(*) FROM {table}").fetchone()
            return int(row[0]) if row else 0

        oracle = count("oracle_cards")
        printings = count("printings")
        stale = pool_stale(con)
    finally:
        con.close()
    # `config.SCRYFALL_DIR`, not a relative literal: this is the platform's
    # health check target, and under `MTGLAB_DATA_DIR=/data` a hardcoded
    # `data/scryfall` resolves against the working directory instead of the
    # volume -- so a fully seeded instance reported no bulk files at all.
    files = sorted(config.SCRYFALL_DIR.glob("*.jsonl.gz")) if \
        config.SCRYFALL_DIR.exists() else []
    return {
        "pool": True,
        "oracle_cards": oracle,
        "printings": printings,
        "bulk_files": [f.name for f in files],
        "decks": len(_source(source).slugs()),
        # A card pool loaded before the printed-stat columns existed answers every
        # question about power with NULL, which reads as "this card has no
        # power". Saying so is the difference between a prompt to re-ingest and
        # a quiet wrong answer about every creature in the library. Since
        # 2026-08-19 it also covers `printings.artist`, where the same NULL
        # reads as "this painting is unsigned" and costs a deck its credit
        # line -- see `db.pool_is_stale`.
        "pool_stale": stale,
        **({"message": "pool predates the printed stats or the painters -- "
                       "run `mtglab data refresh`"} if stale else {}),
    }


# -------------------------------------------------------------------- decks


def _pool_for(deck: Deck,
                con: DuckDBPyConnection | None) -> dict[str, CardRecord]:
    if con is None:
        return {}
    names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board] + [c.name for c in deck.graveyard]
    if deck.companion:
        names.append(deck.companion)
    return db.get_cards(con, names)


def _card_json(entry: CardEntry, rec: CardRecord | None, *,
               full: bool = False) -> dict[str, Any]:
    """One row of the 99, merged with whatever the pool knows about it.

    `full` adds the fields only a hero panel wants — the flavour text and the
    artist. Opt-in rather than always-on because this runs 99 times per deck
    and those two fields are read exactly once, for the commander: carrying
    them on every row would add a few kilobytes to every payload for nothing.
    """
    out: dict[str, Any] = {
        "name": entry.name,
        "category": entry.category,
        "why": entry.why,
        "qty": entry.qty,
        # Which printing's art this deck picked for the slot, or "". Carried
        # on every row so the art picker can mark the current choice without
        # a second request; the image swap itself happens in `get_deck`.
        "art": getattr(entry, "art", ""),
        "known": rec is not None,
    }
    if rec is not None:
        out.update({
            "mana_cost": rec.mana_cost,
            "cmc": rec.cmc,
            "type_line": rec.type_line,
            "oracle_text": rec.oracle_text,
            "color_identity": sorted(rec.color_identity),
            "image": rec.image_normal,
            "art_crop": getattr(rec, "image_art_crop", None),
            "edhrec_rank": rec.edhrec_rank,
            "reserved": rec.reserved,
            # Printed stats, so a rationale claiming "a 6/6" is checkable
            # against the card rather than against a memory of it.
            "power": rec.power,
            "toughness": rec.toughness,
            "loyalty": rec.loyalty,
            "game_changer": rec.game_changer,
        })
        if full:
            # Flavour text is a property of a *printing*, so plenty of cards
            # have none and the UI has to treat it as optional rather than as
            # missing data. Two of the six commanders here have one.
            out.update({
                "flavor_text": rec.flavor_text,
                "artist": rec.artist,
            })
    return out


def list_decks(*, source: DeckSource | None = None,
               owner: str | None = None) -> list[dict[str, Any]]:
    """The library view. Includes the commander's art so the UI has a hero
    image without a second round trip per deck.

    Also carries the gate's error and warning counts. The gate is the point of
    this project, so a shelf that renders a deck with a banned card exactly
    like a clean one is hiding the only thing it is really for -- and asking
    the UI to fetch /validate per deck would be an N+1 on every page load.
    `errors` is None when the pool is unavailable, which is different from
    zero and must not render as a pass.
    """
    decks = _source(source)
    con = _connect()
    try:
        return _tiles(decks.all(), con, writable=decks.writable, owner=owner)
    finally:
        if con is not None:
            con.close()


def list_library(lib: Library) -> list[dict[str, Any]]:
    """Every deck this caller may see, across every owner (ADR 22).

    The library view once decks have owners. One pool connection for the
    whole page rather than one per owner, which is the same N+1 the docstring
    above refuses for the gate — a shelf showing four people's decks must not
    open DuckDB four times to do it.

    Each tile carries its `owner`, which is what the browse tab groups on and
    what the client needs to build the deck's URL at all.
    """
    con = _connect()
    try:
        out: list[dict[str, Any]] = []
        showcase = lib.file_owner.casefold()
        for owner, src in lib.visible():
            tiles = _tiles(src.all(), con, writable=src.writable, owner=owner)
            for tile in tiles:
                tile["showcase"] = owner.casefold() == showcase
            out.extend(tiles)
        return out
    finally:
        if con is not None:
            con.close()


def _tiles(decks: list[Deck], con: Any, *, writable: bool,
           owner: str | None) -> list[dict[str, Any]]:
    """The shelf's payload for a run of decks sharing one owner and pool.

    One pool lookup for the whole shelf, not one per deck -- the same N+1 the
    `list_decks` docstring refuses for `/validate`, one layer further down.
    Six decks were six ~60ms queries; the union of their names is one. Safe
    because `validate` and everything below only ever *look up* names in the
    dict, so a superset spanning other decks changes no answer.
    """
    all_names: set[str] = set()
    for deck in decks:
        all_names.update(deck.card_names())
        all_names.update(c.name for c in deck.swap_board)
        if deck.companion:
            all_names.add(deck.companion)
    cards = (db.get_cards(con, sorted(all_names))
             if con is not None and all_names else {})
    # And one query for every pinned printing on the shelf, for the same
    # reason -- see `_chosen_arts`. Keyed on the art id rather than the slug,
    # so two decks flying the same commander printing share a row.
    arts = _chosen_arts([deck.commander_art for deck in decks
                         if deck.commander_art], con)

    out = []
    for deck in decks:
        art = None
        identity: list[str] = []
        errors = warnings = None
        if con is not None:
            rep = validate(deck, cards)
            errors, warnings = len(rep.errors), len(rep.warnings)
            rec = cards.get(deck.commander[0]) if deck.commander else None
            if rec is not None:
                art = getattr(rec, "image_art_crop", None) or rec.image_normal
                identity = sorted(rec.color_identity)
            # A deck's chosen printing shows on the shelf too. The library
            # is where somebody notices they picked one, and a tile that
            # kept the default while the deck page showed a Secret Lair
            # would read as a bug in one of the two.
            chosen = arts.get(deck.commander_art)
            if chosen:
                art = chosen["art_crop"] or chosen["image"] or art
        out.append({
            "slug": deck.slug,
            # Whose deck this is. Half of its address now (ADR 22), so the
            # client cannot build a link without it, and the key the browse
            # tab groups on.
            #
            # `list_library` adds one more field on top of this, `showcase`,
            # and only there: it says whether this owner is the curated six's,
            # which is a question about *the library view* and not about a
            # deck. The Claude tool that shares this function is asking about
            # one source and would only be given a `false` to misread.
            "owner": owner,
            "name": deck.name,
            # On the shelf as well as the deck page, so the library grid can
            # decide whether to offer a delete control without asking who
            # the viewer is. Per deck rather than per response because that
            # is what it becomes when decks have owners -- which they now do,
            # so this genuinely differs between tiles in one response.
            "writable": writable,
            # Whether anybody signed in may read it, or only its owner. Shown
            # so somebody can see at a glance which of their decks are on
            # display, and never a claim about somebody *else's* deck being
            # private: a private deck of another owner is not in this list.
            "shared": deck.shared,
            # Who sleeves this one up (second 2026-08-15 punch list, item 10):
            # a household tag, not an account. Empty means untagged.
            "pilot": deck.pilot,
            "status": deck.status,
            "stage": deck.stage,
            # The draft's to-do list, as a number. Carried on the library
            # payload for the same reason the gate counts are: a shelf that
            # renders an unreasoned list exactly like a curated deck hides
            # the distinction the stage exists to draw.
            "needs_rationale": len(deck.unjustified),
            "commander": deck.commander,
            "companion": deck.companion,
            "bracket": deck.bracket,
            # The labels, on the shelf because the shelf is where filtering
            # by them will live. `archetype` is the ADR 37 reading of the
            # declared themes; empty means undeclared, not zero.
            "archetype": deck.archetype,
            "themes": deck.themes,
            "total_cards": deck.total_cards,
            "land_count": deck.land_count,
            "strategy": deck.strategy,
            "art_crop": art,
            "color_identity": identity,
            "errors": errors,
            "warnings": warnings,
        })
    return out


def _type_parts(type_line: str) -> tuple[list[str], list[str]]:
    """A type line split into the part before the dash and the part after.

    `"Legendary Creature — Troll Warlock"` -> `(["Legendary", "Creature"],
    ["Troll", "Warlock"])`. Both em dash and hyphen are accepted because
    Scryfall uses the em dash and hand-typed data does not always. A card with
    no subtypes gives an empty second list rather than a fake one.

    Only the front face. A double-faced card's `type_line` carries both halves
    around a `//`, and the commander's own types are the ones on the side you
    cast — which is the same reason `CardRecord.front_type_line` exists.
    """
    front = type_line.split("//")[0].strip()
    for dash in ("—", "–", " - "):
        if dash in front:
            before, after = front.split(dash, 1)
            return before.split(), after.split()
    return front.split(), []


def commander_dossier(slug: str, *,
                      source: DeckSource | None = None) -> dict[str, Any]:
    """Everything interesting the pool knows about a deck's commander.

    The deck page's header used to say a name and show a painting, which meant
    the one card that governs all 99 others was the card you knew least about.
    This is the answer to "who is this, actually" — and every part of it is a
    query, not a recollection.

    That distinction is the whole design. "Gyome is one of eight legendary
    Trolls" and "Trostani has five other cards" are exactly the kind of claim
    a language model will produce fluently and wrongly, and `CLAUDE.md` rule 1
    exists because that has already happened twice on this project. So the
    facts are counted here, in Python, over the pool: a wrong number is a
    bug with a reproducible query behind it rather than a confident sentence
    nobody can check.

    Three kinds of fact, chosen because each says something the card's own
    text does not:

    **Subtypes and how rare they are.** A Troll Warlock is a more interesting
    thing to be when eight legendary Trolls exist and eighty-one legendary
    Warlocks do.

    **Other cards for the same character.** Trostani has been printed five
    more times with different mechanics each time; Arahbo got a second version
    years later. Matched on the character's name — the part before the comma,
    which is how Magic names legends — so `Trostani, Selesnya's Voice` finds
    `Trostani Discordant` and `Trostani's Summoner`. A loose match on purpose:
    these are offered as related cards, not asserted as the same character.

    **How often it has been printed, and when it first was.** From the
    `printings` table, so a 2012 guild leader reads differently from a card
    that arrived last year.

    Returns `None` for `card` when there is no card pool, rather than failing —
    this is a decorative panel and a fresh clone should still show its decks.
    """
    deck = _source(source).get(slug)
    if not deck.commander:
        return {"slug": deck.slug, "card": None, "subtypes": [],
                "other_cards": [], "printings": None}

    name = deck.commander[0]
    con = _connect()
    if con is None:
        return {"slug": deck.slug, "card": None, "subtypes": [],
                "other_cards": [], "printings": None}

    try:
        rec = db.get_cards(con, [name]).get(name)
        if rec is None:
            return {"slug": deck.slug, "card": None, "subtypes": [],
                    "other_cards": [], "printings": None}

        supertypes, subtypes = _type_parts(rec.type_line)

        # One query per subtype, and there are at most a handful. `ilike` with
        # word boundaries would be better, but DuckDB's `ilike` has no \b --
        # so the count is over type lines *containing* the word, and the
        # payload says "type lines" rather than claiming anything sharper.
        subtype_rows: list[dict[str, Any]] = []
        for sub in subtypes:
            pattern = f"%{sub}%"
            total = con.execute(
                "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ?",
                [pattern]).fetchone()
            legends = con.execute(
                "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ? "
                "AND type_line ILIKE '%Legendary%'", [pattern]).fetchone()
            subtype_rows.append({
                "name": sub,
                "total": int(total[0]) if total else 0,
                "legendary": int(legends[0]) if legends else 0,
            })

        # The character's name: everything before the first comma, which is
        # how Magic writes a legend. A mononym like "Goreclaw, Terror of Qal
        # Sisma" gives "Goreclaw"; one with no comma gives the whole name and
        # simply matches itself, which the `name <> ?` filter then drops.
        character = name.split(",")[0].strip()
        others = con.execute(
            "SELECT name, type_line, mana_cost, image_normal, image_art_crop "
            "FROM oracle_cards WHERE name ILIKE ? AND name <> ? "
            "ORDER BY edhrec_rank NULLS LAST LIMIT 6",
            [f"%{character}%", name]).fetchall()

        # Scryfall's stable id for the *card*, across every printing of it.
        # Two things need it and neither is decorative: the art picker lists a
        # card's printings by it, and ADR 19 keys a cached dossier on it —
        # a dossier is about a character, so every deck that commander leads
        # shares one. `CardRecord` does not carry it, hence the query.
        row = con.execute("SELECT oracle_id FROM oracle_cards WHERE name = ? "
                          "LIMIT 1", [name]).fetchone()
        oracle_id = row[0] if row else None

        printed = con.execute(
            "SELECT count(*), min(released_at) FROM printings "
            "WHERE oracle_id = (SELECT oracle_id FROM oracle_cards "
            "WHERE name = ? LIMIT 1)", [name]).fetchone()

        first_set = None
        if printed and printed[1] is not None:
            row = con.execute(
                "SELECT set_name FROM printings WHERE oracle_id = "
                "(SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1) "
                "AND released_at = ? LIMIT 1", [name, printed[1]]).fetchone()
            first_set = row[0] if row else None

        # This panel is about a *deck's* commander, so the two printing facts
        # below follow the deck's chosen printing exactly as `get_deck`'s hero
        # panel does. Nothing renders them here today; the payload is right
        # anyway, because the surface that starts rendering them is the one
        # that would otherwise rediscover the wrong-painter bug. One
        # primary-key lookup, and only for a deck that pinned a printing.
        chosen = _chosen_art(deck, con)
        artist = chosen["artist"] if chosen else rec.artist
        flavor_text = chosen["flavor_text"] if chosen else rec.flavor_text

        return {
            "slug": deck.slug,
            "card": {
                "name": rec.name,
                "oracle_id": oracle_id,
                "mana_cost": rec.mana_cost,
                "type_line": rec.type_line,
                "oracle_text": rec.oracle_text,
                "flavor_text": flavor_text,
                "artist": artist,
                "power": rec.power,
                "toughness": rec.toughness,
                "loyalty": rec.loyalty,
                "image": rec.image_normal,
                "art_crop": getattr(rec, "image_art_crop", None),
                "color_identity": sorted(rec.color_identity),
                "edhrec_rank": rec.edhrec_rank,
                "game_changer": rec.game_changer,
            },
            "supertypes": supertypes,
            "subtypes": subtype_rows,
            "other_cards": [{
                "name": r[0], "type_line": r[1], "mana_cost": r[2],
                "image": r[3], "art_crop": r[4],
            } for r in others],
            "printings": {
                "count": int(printed[0]) if printed else 0,
                "first_released": printed[1].isoformat()
                if printed and printed[1] else None,
                "first_set": first_set,
            },
        }
    finally:
        if con is not None:
            con.close()


def commander_printings(slug: str, *, card: str | None = None,
                        source: DeckSource | None = None) -> dict[str, Any]:
    """Every printing of this deck's commander -- or of any card in it.

    **Non-digital only.** Arena and MTGO printings have their own art in the
    pool and are not things you can put in a sleeve, so offering one as a
    deck's art is offering something that does not exist as a card. The count
    the maintainer will recognise -- Goreclaw has twelve, Gyome three -- is the
    physical count, and this is what makes it so.

    `selected` marks the deck's current choice so a picker does not have to
    compare ids itself, and it is computed here rather than in the client so
    that the CLI and the app agree about which one is showing.

    `card` widens the question from the commander to the 99 (and the swap
    board, and the graveyard -- a choice made before entombment should survive
    a return). Asking about a card the deck does not hold is refused, because
    the answer would invite writing an `art` for a slot that does not exist.
    """
    deck = _source(source).get(slug)
    if card is not None:
        entry = _find_card(deck, card)
        if entry is None:
            raise EditRejected(f"{card!r} is not in this deck")
        name = entry.name
        selected = getattr(entry, "art", "")
    else:
        name = deck.commander[0] if deck.commander else ""
        selected = deck.commander_art
    empty: dict[str, Any] = {"slug": deck.slug, "commander": name,
                             "selected": selected, "printings": []}
    if not name:
        return empty

    con = _connect()
    if con is None:
        return empty
    try:
        rows = con.execute(
            "SELECT p.id, p.set_code, p.set_name, p.collector_number, "
            "       p.rarity, p.released_at, p.promo, p.image_normal, "
            "       p.price_usd "
            "FROM printings p "
            "WHERE p.oracle_id = (SELECT oracle_id FROM oracle_cards "
            "                     WHERE name = ? LIMIT 1) "
            # `digital IS NOT TRUE` rather than `= FALSE`: the column is
            # nullable, and a NULL means "Scryfall did not say", which for a
            # printing that exists on paper is the common case in older rows.
            # `= FALSE` would silently hide them.
            "  AND p.digital IS NOT TRUE "
            "  AND p.image_normal IS NOT NULL "
            "ORDER BY p.released_at DESC NULLS LAST, p.set_code, "
            "         p.collector_number",
            [name]).fetchall()
    finally:
        con.close()

    return {
        **empty,
        "printings": [{
            "id": r[0],
            "set_code": (r[1] or "").upper(),
            "set_name": r[2],
            "collector_number": r[3],
            "rarity": r[4],
            "released_at": r[5].isoformat() if r[5] else None,
            "promo": bool(r[6]),
            "image": r[7],
            "art_crop": art_crop_from(r[7]),
            "price_usd": r[8],
            "selected": r[0] == selected,
        } for r in rows],
    }


def _chosen_arts(art_ids: list[str], con: Any) -> dict[str, dict[str, Any]]:
    """The printings behind a run of chosen art ids, keyed by id. One query.

    The shelf asks this question once per deck and used to ask it *as* one
    query per deck -- the same N+1 `_card_art_overrides` refuses one layer
    down for a deck's own cards, and the one `challenge_progress` refused
    before that. Three of six decks pin a printing today, so it was three
    queries a shelf; a library aimed at thirty-two slots would be thirty-two.

    An id the pool no longer has simply does not come back, which is what
    makes the single-deck wrapper below able to return None for it.

    **The painter and the flavour text come back with the picture**, because
    they belong to the printing exactly as the set name does. Read off the
    oracle row instead — which is what happened until 2026-08-19 — the deck
    page renders "Art by ⟨one printing's painter⟩ · ⟨another printing's set⟩"
    in a single sentence. Three decks pin a printing and one of them was
    wrong: Trostani credited Sidharth Chaturvedi (`c19`) over Chippy's
    Return to Ravnica painting. The other two pin a *re-scan of the same
    painting*, so the oracle row happened to name the right person — which is
    a coincidence, not a correct credit, and the next art choice re-rolls it.

    `db.printing_columns` rather than a fixed column list, for the reason
    `_select` gives: the app's handle is read-only and cannot migrate itself,
    so a pool built before these columns existed must degrade to "no credit
    known" rather than failing to bind. `pool_is_stale` is what stops that
    from being mistaken for "this painting is unsigned".
    """
    if not art_ids or con is None:
        return {}
    have = db.printing_columns(con)
    extra = ", ".join(c if c in have else f"NULL AS {c}"
                      for c in ("artist", "flavor_text"))
    rows = con.execute(
        f"SELECT id, image_normal, set_name, set_code, {extra} "
        f"FROM printings WHERE id IN ({','.join('?' * len(art_ids))})",
        art_ids).fetchall()
    return {r[0]: {"image": r[1], "art_crop": art_crop_from(r[1]),
                   "set_name": r[2], "set_code": (r[3] or "").upper(),
                   "artist": r[4], "flavor_text": r[5]}
            for r in rows if r[1]}


def _chosen_art(deck: Any, con: Any) -> dict[str, Any] | None:
    """The printing this deck picked for its commander, if it picked one.

    Returns None for the common case -- no choice made -- so every caller's
    fallback is the pool's default printing, unchanged. A choice pointing at
    a printing that no longer exists also returns None rather than blanking
    the art: a stale id is a deck showing its default, not a deck with no
    commander picture.

    One deck's worth of `_chosen_arts`, and right for the deck page, which
    only ever has one. The shelf calls the batched form directly.
    """
    art_id = getattr(deck, "commander_art", "")
    if not art_id:
        return None
    return _chosen_arts([art_id], con).get(art_id)


def _card_art_overrides(deck: Deck,
                        con: DuckDBPyConnection | None) -> dict[str, dict[str, Any]]:
    """The chosen printings for every card that picked one, keyed by name.

    One query for the whole deck rather than one per card, for the same N+1
    reason the shelf batches the gate. A stale id -- a printing that left the
    pool -- simply does not come back, and the card renders its default, the
    same forgiving answer `_chosen_art` gives the commander.

    Carries the painter and the flavour text for the same reason `_chosen_arts`
    does. Nothing renders a card's credit today -- `_card_json` sends those two
    only for the commander -- so this is the rule holding for the 99 *before*
    a surface needs it, rather than the commander's bug waiting to be
    rediscovered one row down.
    """
    chosen = {c.name: c.art
              for c in [*deck.cards, *deck.swap_board, *deck.graveyard]
              if getattr(c, "art", "")}
    if not chosen or con is None:
        return {}
    ids = list(chosen.values())
    have = db.printing_columns(con)
    extra = ", ".join(c if c in have else f"NULL AS {c}"
                      for c in ("artist", "flavor_text"))
    rows = con.execute(
        f"SELECT id, image_normal, {extra} FROM printings "
        f"WHERE id IN ({','.join('?' * len(ids))}) "
        f"AND image_normal IS NOT NULL",
        ids).fetchall()
    by_id = {r[0]: {"image": r[1], "artist": r[2], "flavor_text": r[3]}
             for r in rows}
    return {name: {**by_id[art_id],
                   "art_crop": art_crop_from(by_id[art_id]["image"])}
            for name, art_id in chosen.items() if art_id in by_id}


def _with_art(row: dict[str, Any],
              overrides: dict[str, dict[str, Any]]) -> dict[str, Any]:
    """One row, wearing its chosen printing.

    The picture, the painter and the flavour text, because all three are the
    *printing's*. Oracle text, cost, type line and identity are the card's and
    genuinely do not vary between printings -- swapping those would make a
    cosmetic choice look like it changed what the card does.

    The credits are written only onto a row that already carries them, i.e.
    `_card_json(full=True)`. Adding the keys to a row that asked for the short
    form would put two fields on all 99 that nothing reads, which is the cost
    `full` exists to avoid.
    """
    chosen = overrides.get(row["name"])
    if chosen and row.get("known"):
        row["image"] = chosen["image"]
        row["art_crop"] = chosen["art_crop"] or row.get("art_crop")
        for field in ("artist", "flavor_text"):
            if field in row:
                row[field] = chosen[field]
    return row


def get_deck(slug: str, *, source: DeckSource | None = None,
             owner: str | None = None) -> dict[str, Any]:
    decks = _source(source)
    deck = decks.get(slug)
    con = _connect()
    try:
        cards = _pool_for(deck, con)
        commander_rec = cards.get(deck.commander[0]) if deck.commander else None
        commander_card = _card_json(
            type("E", (), {"name": deck.commander[0], "category": "commander",
                           "why": deck.notes.get("commander_why", ""), "qty": 1})(),
            commander_rec, full=True) if commander_rec else None
        # The motion tier (ADR 32) is keyed on oracle_id, and this payload is
        # the one place the deck page learns who its commander *is* rather
        # than what the card says -- so the id rides here, not in a second
        # request.
        if commander_card is not None and con is not None:
            row = con.execute(
                "SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1",
                [deck.commander[0]]).fetchone()
            commander_card["oracle_id"] = row[0] if row else None
        # The chosen printing replaces everything that is the *printing's* --
        # the images, the painter, the flavour text -- and nothing else. Oracle
        # text, cost, type line and colour identity are the *card's* and do not
        # vary by printing; swapping those would make a cosmetic choice look
        # like it changed what the commander does.
        #
        # That second sentence was the whole comment until 2026-08-19, and the
        # categorisation was wrong: `artist` and `flavor_text` came off the
        # oracle row, which is Scryfall's representative printing rather than
        # the one on screen, so the page rendered one printing's set name
        # beside another printing's painter -- see `_chosen_arts` for which
        # deck it actually cost, and why the other two were lucky.
        #
        # Assigned unconditionally rather than `or`-ed with what was there: a
        # printing the pool has no painter for (an un-refreshed pool, or a
        # printing Scryfall never attributed) must show *no* credit, because
        # falling back would restore the bug in its quietest form. `health()`
        # reports `pool_stale` so a missing credit has somewhere to be
        # explained.
        chosen = _chosen_art(deck, con)
        if commander_card and chosen:
            commander_card["image"] = chosen["image"]
            commander_card["art_crop"] = chosen["art_crop"] or commander_card.get("art_crop")
            commander_card["artist"] = chosen["artist"]
            commander_card["flavor_text"] = chosen["flavor_text"]
            commander_card["printing"] = {"set_name": chosen["set_name"],
                                          "set_code": chosen["set_code"]}
        # The 99's own choices, one batch query (see `_card_art_overrides`).
        chosen_art = _card_art_overrides(deck, con)
        return {
            "commander_art": deck.commander_art,
            "slug": deck.slug,
            "name": deck.name,
            # Whether *this caller* may change *this deck*. Sent as data rather
            # than re-derived in the client from `is_admin`, because that rule
            # is about to stop being the rule: when decks have owners, the
            # answer is a comparison the server can make and the browser
            # cannot. A UI that hides its buttons on `is_admin` today would
            # have to be found and changed then; one that reads this will not.
            #
            # It is a courtesy, not the enforcement. Every write route refuses
            # independently -- see `api/app.py`'s `ReadOnlySource` handler.
            "writable": decks.writable,
            # Whose deck this is, and whether anybody signed in may read it
            # (ADR 22). `owner` is what the client needs to build any URL
            # back to this deck at all, so it travels with the deck rather
            # than being remembered from the list that linked here.
            "owner": owner,
            "shared": deck.shared,
            "pilot": deck.pilot,
            "status": deck.status,
            "stage": deck.stage,
            "needs_rationale": len(deck.unjustified),
            "commander": deck.commander,
            "companion": deck.companion,
            "bracket": deck.bracket,
            # The labelling axis (model.THEMES) and its reading (ADR 37):
            # `themes` is declared in deck.yaml and edited through
            # `set_deck_field`; `archetype` is derived from it, worst-piloted
            # declared class word winning, and is read-only on the wire.
            "archetype": deck.archetype,
            "themes": deck.themes,
            "strategy": deck.strategy,
            "notes": deck.notes,
            "total_cards": deck.total_cards,
            "land_count": deck.land_count,
            "color_identity": sorted(commander_rec.color_identity) if commander_rec else [],
            "commander_card": commander_card,
            "cards": [_with_art(_card_json(e, cards.get(e.name)), chosen_art)
                      for e in deck.cards],
            "swap_board": [_with_art(_card_json(e, cards.get(e.name)), chosen_art)
                           for e in deck.swap_board],
            # Entombed cards (ADR 27): out of the 99, waiting on a return or
            # an exile. Rendered with the same pool facts as the living so the
            # graveyard panel can show real cards, not just names.
            "graveyard": [_with_art(_card_json(e, cards.get(e.name)), chosen_art)
                          for e in deck.graveyard],
            "pool_available": con is not None,
        }
    finally:
        if con is not None:
            con.close()


def validate_deck(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        cards = _pool_for(deck, con) if con is not None else None
        rep = validate(deck, cards)
        return {
            "ok": rep.ok,
            "errors": [{"code": i.code, "message": i.message, "card": i.card}
                       for i in rep.errors],
            "warnings": [{"code": i.code, "message": i.message, "card": i.card}
                         for i in rep.warnings],
        }
    finally:
        if con is not None:
            con.close()


def history_for(slug: str, *, source: DeckSource | None = None,
                limit: int = log.DEFAULT_LIMIT) -> dict[str, Any]:
    """What has been done to this deck, newest first (ADR 28).

    `source.get(slug)` runs first and is not decoration: it is the whole
    authorisation check. A deck this caller may not see is absent from their
    source, so it raises `DeckNotFound` -> 404 before a row is read -- the same
    answer, arrived at the same way, as every other route about one deck. There
    is deliberately no second rule here about who may read a history: it is the
    people who may read the deck, and that set is decided in
    `decks/library.py` once.

    A shared deck's history is therefore visible to whoever it is shared with,
    and every actor in it is the deck's owner, because writes are owner-only
    (ADR 22). So it discloses a username that the URL already carried.
    """
    decks = _source(source)
    decks.get(slug)                     # 404 before anything is read
    return {
        "slug": slug,
        "entries": log.entries(slug, owner_id=owner_id_of(decks),
                               limit=limit),
    }


class ImportRejected(Exception):
    """The import was refused, and nothing was written."""


# A slug becomes a directory name under `decks/`, so it is checked rather than
# trusted. The API takes it from a request body, and "sanitise it later" is how
# a path component turns into a path.
_SLUG = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")


def import_deck(*, text: str, slug: str, name: str = "",
                commander: list[str] | None = None, companion: str = "",
                bracket: int | None = None, status: str = "theoretical",
                dry_run: bool = False, owner: str | None = None,
                source: DeckSource | None = None) -> dict[str, Any]:
    """Turn a pasted decklist into a draft deck.

    Everything is checked before anything is written, and `dry_run` runs the
    whole thing -- parse, resolve, build, gate -- without creating the deck.
    That is what the app's preview is: the same code path, so what you approve
    is what gets written rather than an estimate of it.

    The deck arrives as `stage: draft` with an empty `why` on every card. That
    is the entire point of ADR 13: the facts get checked immediately, the
    thinking is counted rather than fabricated, and the artifacts stay shut
    until a human has done it.
    """
    # The library, not the slug: this runs before the slug is even validated,
    # and a caller who may not write should not learn whether their slug was
    # acceptable. A fourth bespoke exception (`ImportRejected`) collapsed into
    # `ReadOnlySource` with the other three.
    decks = _for_writing(source)

    slug = slug.strip().lower()
    if not _SLUG.match(slug):
        raise ImportRejected(
            f"{slug!r} is not a usable slug -- lowercase letters, digits and "
            "single hyphens, e.g. 'arahbo-cats'")
    if slug in decks.slugs():
        raise ImportRejected(
            f"a deck called {slug!r} already exists; pick another slug")
    if status not in DECK_STATUSES:
        raise ImportRejected(
            f"status {status!r} is not one of {', '.join(DECK_STATUSES)}")

    parsed = decklist.parse(text)
    if not parsed.cards:
        raise ImportRejected("nothing in that list parsed as a card")

    con = _connect()
    if con is None:
        # Without the pool every name is unknown and no land is filed, so the
        # import would produce a deck whose facts were never checked -- the one
        # thing the gate exists to prevent. Refuse rather than half-do it.
        raise ImportRejected(
            "importing needs the card pool -- run `mtglab data refresh`")
    try:
        cards = db.get_cards(con, importer.names_in(
            parsed, commander=commander, companion=companion or None))
        try:
            report = importer.build_deck(
                parsed, cards, slug=slug, name=name or slug,
                commander=commander, companion=companion or None,
                bracket=bracket, status=status)
        except ImportRefused as exc:
            raise ImportRejected(str(exc)) from exc

        if not dry_run:
            try:
                decks.create(slug, report.yaml_text)
            except DeckExists as exc:
                raise ImportRejected(f"a deck called {slug!r} already exists") from exc

        gate = validate(report.deck, _pool_for(report.deck, con))
        return {
            "slug": slug,
            "owner": owner,
            "name": report.deck.name,
            "stage": report.deck.stage,
            "status": report.deck.status,
            "created": not dry_run,
            "commander": report.deck.commander,
            "companion": report.deck.companion,
            "total_cards": report.deck.total_cards,
            "land_count": report.deck.land_count,
            "swap_board": [c.name for c in report.deck.swap_board],
            "needs_rationale": report.needs_rationale,
            "unknown": report.unknown,
            "unreadable": [{"line": n, "text": t} for n, t in report.unreadable],
            "skipped": [{"line": n, "text": t} for n, t in report.skipped],
            "notes": report.notes,
            "yaml": report.yaml_text,
            "ok": gate.ok,
            "errors": [{"code": i.code, "message": i.message, "card": i.card}
                       for i in gate.errors],
            "warnings": [{"code": i.code, "message": i.message, "card": i.card}
                         for i in gate.warnings],
        }
    finally:
        con.close()


class CreateRejected(Exception):
    """The deck was not created, and nothing was written."""


def create_deck(*, slug: str, name: str = "", commander: list[str] | None = None,
                companion: str = "", bracket: int | None = None,
                status: str = "theoretical", owner: str | None = None,
                source: DeckSource | None = None) -> dict[str, Any]:
    """Start a new deck from a commander and nothing else.

    The last gap in the deck lifecycle. `import_deck` refuses an empty list --
    correctly, since an import with no cards is a mistake -- so create is its
    own path rather than an import of nothing.

    What it will not do is exactly what import will not do: **it never picks a
    commander for you**, and the deck arrives as a `draft` with no rationales,
    because there is nothing yet to justify. The 99 gets filled in afterwards
    by the edit operations that already exist.

    Colour identity is deliberately *not* a parameter. It is derived from the
    commander by rule 2, and accepting it here would create a second, weaker
    source of truth for the one fact this project refuses to guess at. The
    colour taxonomy is how a person *finds* a commander, not how a deck records
    what it is.
    """
    # The deck does not exist yet, so the subject of the refusal is the library
    # rather than a slug. It goes through `_for_writing` like everything else
    # now -- the separate check here existed because `EditRejected` was the
    # wrong exception for a route catching `CreateRejected`, and there is only
    # one exception left for it to be wrong about.
    decks = _for_writing(source)
    commander = [c.strip() for c in (commander or []) if c.strip()]

    slug = slug.strip().lower()
    if not _SLUG.match(slug):
        raise CreateRejected(
            f"{slug!r} is not a usable slug -- lowercase letters, digits and "
            "single hyphens, e.g. 'arahbo-cats'")
    if slug in decks.slugs():
        raise CreateRejected(
            f"a deck called {slug!r} already exists; pick another slug")
    if not commander:
        raise CreateRejected("a new deck needs a commander")
    if len(commander) > 2:
        raise CreateRejected(
            f"{len(commander)} commanders listed; Commander allows at most two")
    if status not in DECK_STATUSES:
        raise CreateRejected(
            f"status {status!r} is not one of {', '.join(DECK_STATUSES)}")

    con = _connect()
    if con is None:
        # Same refusal as import, for the same reason: a deck whose commander
        # was never checked is a deck whose colour identity is a guess.
        raise CreateRejected(
            "creating a deck needs the card pool -- run `mtglab data refresh`")
    try:
        names = [*commander] + ([companion] if companion else [])
        found = db.get_cards(con, names)
        missing = [n for n in names if n not in found]
        if missing:
            raise CreateRejected(
                "not in the pool: " + ", ".join(sorted(missing)))

        paired = len(commander) == 2
        for cmd in commander:
            if not partners.can_be_commander(found[cmd], paired=paired):
                raise CreateRejected(
                    f"{found[cmd].name} cannot be your commander -- it is "
                    f"{found[cmd].type_line}")
        if paired:
            # The same check the gate runs. Two commanders is not "any two
            # legends" -- Partner, Partner with, Friends forever, Choose a
            # Background and Doctor's companion each have their own rule.
            why = partners.check_pair(found[commander[0]], found[commander[1]])
            if why:
                raise CreateRejected(why)

        identity = frozenset().union(
            *(found[c].color_identity for c in commander)) if commander else frozenset()

        deck = Deck(
            slug=slug,
            name=name.strip() or found[commander[0]].name,
            status=status,
            stage="draft",
            commander=commander,
            companion=companion or None,
            bracket=bracket,
        )
        decks.create(slug, deck.dump())

        combo = colors.of(identity)
        return {
            "slug": slug,
            "owner": owner,
            "name": deck.name,
            "stage": deck.stage,
            "status": deck.status,
            "created": True,
            "commander": deck.commander,
            "companion": deck.companion,
            "color_identity": sorted(identity),
            "combination": {"key": combo.key, "name": combo.name,
                            "tier": combo.tier},
            "total_cards": 0,
        }
    finally:
        con.close()


class DeleteRejected(Exception):
    """The deletion was refused, and nothing was moved."""


#: The short word that confirms a deletion, alongside the slug itself.
#:
#: "Bury" is Magic's own retired templating for destroying a permanent so that
#: it cannot regenerate, and it is the right verb here for the reason the
#: obvious alternatives are the wrong ones: the deck goes to `decks/.trash/`
#: and can be brought back, so "exile" — which in Magic means gone for good —
#: would misdescribe what this does.
#:
#: `web/src/routes/Library.tsx` types this word into its own dialog and
#: `tests/test_api.py` pins it here, the same way `Simulator.tsx` mirrors
#: `simruns.DEFAULT_SEED`. If the two ever drift, the refusal message names
#: what the server actually wants.
DELETE_WORD = "bury"


def delete_deck(*, slug: str, confirm: str,
                source: DeckSource | None = None) -> dict[str, Any]:
    """Remove a deck from the library. Recoverably, and only on purpose.

    The last operation in the deck lifecycle, and the only one that can lose
    work. Three safeguards, chosen so that each catches something the others
    do not:

    **`confirm` must be a typed word — the slug, or `bury`.** Not a boolean and
    not a `force=true` flag: a client that sends `{"confirm": true}` for every
    deletion has not confirmed anything, and neither spelling here can be
    satisfied by a mis-clicked row.

    The slug was originally the only answer, on the argument that it is a value
    only somebody looking at the right deck can produce. That argument is
    sound and the slug still works — but it was also, in practice, a gate
    people could not get through: `ishai-ojutai-dragonspeaker` is 26 characters
    of hyphenated name to copy by eye, and the app rendered it uppercased next
    to a case-sensitive comparison, so typing exactly what was on screen was
    refused with no explanation. A confirmation nobody can satisfy does not
    protect the deck; it just moves the deletion to the shell, unconfirmed.
    So the check now takes either, and takes both case-insensitively.

    **A read-only source refuses.** `docs/HOSTING.md` keeps the curated decks
    read-only for everyone but the maintainer, and this is the operation where
    that matters most.

    **The deck moves rather than vanishing.** The source says where it went and
    this returns it, because "deleted" and "recoverable" have to be separately
    true and separately visible. Committed decks have git as their undo; the
    draft imported ten minutes ago has only this.

    Deliberately *not* a safeguard: refusing to delete a curated or built deck.
    A tool that will not let you throw away your own work because it disagrees
    about the work's importance is the same failure as one that edits a deck
    without asking — it is your library, and the confirmation is the check.
    """
    # Refused before the deck is even looked up, which is the ordering that
    # matters most here: this is the one operation that moves a directory.
    decks = _for_writing(source, slug)
    deck = decks.get(slug)                       # raises DeckNotFound

    typed = confirm.strip().casefold() if isinstance(confirm, str) else ""
    if typed not in {slug.casefold(), DELETE_WORD}:
        raise DeleteRejected(
            f"to delete {slug!r}, confirm by typing {DELETE_WORD!r} or the "
            f"slug itself. Got {confirm!r}. This is deliberately not a yes/no: "
            f"it is the one operation here that can lose work nothing else "
            f"recorded.")

    moved_to = decks.delete(slug)
    return {
        "slug": slug,
        "name": deck.name,
        "deleted": True,
        # Where it went, so the answer to "can I get it back" is in the
        # response rather than in someone's memory of how this was built.
        "moved_to": moved_to,
        "total_cards": deck.total_cards,
        "stage": deck.stage,
        "status": deck.status,
    }


# -------------------------------------------------------------------- claude

#: Modes whose default stance is not the deck's, because they have no deck.
#: A table rather than an `if`, so the second entry was a row -- and it was:
#: research (ADR 26) joined the theme interview here, and joined it for exactly
#: the same reason. **The value is the owning module's own function**, never a
#: literal; a default copied into this file is a second copy to disagree with.
_SURFACE_DEFAULTS = {"theme", "research"}


def _surface_stance_for(surface: str) -> Any:
    """Ask the module that owns `surface` what "no preference" means to it."""
    if surface == "research":
        from mtglab.claude.research import stance_for as research_stance_for
        return research_stance_for(None)
    from mtglab.claude.theme import stance_for as theme_stance_for
    return theme_stance_for(None)


def _default_stance(deck: Any, surface: str | None) -> Any:
    """What "no preference" resolves to, asked of whoever owns the answer.

    Three cases and none of them is a literal here: a deckless surface has its
    own (`theme.stance_for`, `research.stance_for`), a deck has one derived
    from its `status` (`stance.default_for`), and a caller who named neither
    gets `off`, because "I have no idea what this is about" is the one case
    where silence is right.
    """
    from mtglab.claude import stance as claude_stance
    if surface in _SURFACE_DEFAULTS and deck is None:
        return _surface_stance_for(surface)
    return claude_stance.default_for(deck) if deck else claude_stance.OFF


def claude_status(*, requested: Any = None, slug: str | None = None,
                  surface: str | None = None,
                  source: DeckSource | None = None) -> dict[str, Any]:
    """What the Claude surface is, and how much of it is switched on.

    Answers three separate questions that are easy to conflate:

    * **Is it installed?** The SDK rides with the `claude` extra.
    * **Is it configured?** A credential is present, or it is not.
    * **Is it wanted?** The stance, which is the user's decision and is `off`
      until somebody says otherwise.

    All three can be false independently, and a UI needs to say which — "no
    opinions here" reads very differently from "you have not set a key". None
    of this reaches the network: the stance is stdlib-only and availability is
    a question about the environment, so this endpoint answers on a base
    install with no account.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude import stance as claude_stance

    deck = None
    if slug:
        decks = _source(source)
        deck = Deck.from_text(decks.read_text(slug), slug=slug)

    # Which default applies is the surface's business, not this function's, so
    # it asks the module that owns it rather than keeping a second copy. The
    # theme interview is the one mode with no deck to derive from and a default
    # that is emphatically not `off` — see `theme.stance_for`.
    if surface in _SURFACE_DEFAULTS and deck is None:
        if surface == "research":
            from mtglab.claude.research import stance_for as surface_stance_for
        else:
            from mtglab.claude.theme import stance_for as surface_stance_for
        effective = surface_stance_for(requested)
    else:
        effective = claude_stance.resolve(requested, deck=deck)
    limit = claude_stance.ceiling()
    interview = claude_interview_mode()
    return {
        "installed": claude_client.sdk_installed(),
        "configured": claude_client.credential_present(),
        "model": claude_client.model(),
        "stance": claude_stance.describe(effective),
        # The deployment's cap, so a UI can grey out what it may not offer
        # rather than letting someone pick a level that is silently clamped.
        "ceiling": claude_stance.describe(limit),
        # What "no preference" means here, which is the same question the
        # effective stance above just answered with `requested=None`.
        "default": claude_stance.describe(_default_stance(deck, surface)),
        "presets": [{
            "name": name,
            "blurb": claude_stance.PRESET_BLURBS[name],
            "stance": claude_stance.describe(preset_stance),
            # Whether this deployment will actually honour it unclamped.
            "available": claude_stance.clamp(preset_stance, limit)
            == preset_stance,
        } for name, preset_stance in claude_stance.PRESETS.items()],
        # Stated here rather than only in an ADR, because it is the sentence a
        # user should be able to read next to the dial. Reworded on the second
        # 2026-08-15 punch list (item 6): the old "No stance lets Claude…"
        # parsed as a fragment about some stance called "No", not as the
        # guarantee it is.
        "never": "One rule holds at every setting: Claude never writes a "
                 "card's rationale. The why is always yours.",
        # The modes that exist, so a UI can offer what is built rather than
        # what ADR 15 planned. Six today, across five features.
        "modes": [{
            "name": mode.name,
            "purpose": mode.purpose,
            "tools": list(mode.tool_names),
            # Anthropic's own, listed separately because they are a different
            # kind of thing: they never reach this package's tool door, and a
            # UI that says "searches the web" is saying something a user cares
            # about in a way "get_cards" is not.
            "server_tools": [t["name"] for t in mode.server_tools],
            "writes": list(mode.may_write),
        } for mode in (interview, claude_argue_mode(), claude_dossier_mode(),
                       claude_research_mode(), *claude_theme_modes())],
    }


def claude_interview_mode() -> Mode:
    """The rationale interview's mode object. Imported lazily like the rest.

    A function rather than a module constant so that `service` keeps working
    on a base install: importing it at module scope would drag the mode --
    and the tool registry it validates itself against -- into every process
    that only wanted to list decks.
    """
    from mtglab.claude.interview import RATIONALE_INTERVIEW
    return RATIONALE_INTERVIEW


def claude_argue_mode() -> Mode:
    """The slot argument's mode object (ADR 25). Imported lazily too."""
    from mtglab.claude.argue import SLOT_ARGUMENT
    return SLOT_ARGUMENT


def claude_dossier_mode() -> Mode:
    """The commander dossier's mode object (ADR 19). Imported lazily too."""
    from mtglab.claude.dossier import COMMANDER_DOSSIER
    return COMMANDER_DOSSIER


def claude_research_mode() -> Mode:
    """The research mode's object (ADR 26). Imported lazily too.

    Worth reading its `tool_names` here rather than trusting the name: it is
    `("get_cards",)` and a hosted search, with every deck-facing tool absent on
    purpose. A UI listing this mode's capabilities is listing the argument.
    """
    from mtglab.claude.research import RESEARCH
    return RESEARCH


def claude_theme_modes() -> tuple[Mode, Mode]:
    """The theme interview's two mode objects (ADR 20).

    Two rather than one because a conversation is prose and a proposal is a
    schema, and a single mode doing both either loses the schema or forces
    every chatty turn through it.
    """
    from mtglab.claude.theme import THEME_CONVERSATION, THEME_PROPOSAL
    return THEME_CONVERSATION, THEME_PROPOSAL


class ClaudeFailed(Exception):
    """A Claude call was attempted and did not come back usable.

    Distinct from `ClaudeUnavailable`, which means no call was possible in the
    first place. The caller's answer differs: one is fixed by installing an
    extra or setting a key, the other by retrying, waiting, or reading what the
    API actually said.
    """


def claude_interview(*, slug: str, card: str, requested: Any = None,
                     focus: str = "",
                     source: DeckSource | None = None,
                     tier: str | None = None) -> dict[str, Any]:
    """Ask the rationale interview about one card. Returns questions.

    The whole of this project's Claude surface, so far. It reads the deck, the
    pool and the gate, and it comes back with things to ask yourself. It
    cannot write anything -- not because this function declines to, but because
    nothing under `mtglab.claude` can name a write path at all (ADR 15).
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude.interview import CardNotInDeck, ask
    from mtglab.claude.modes import ModeExhausted

    try:
        return ask(slug, card, requested=requested, focus=focus,
                   source=source, tier=tier)
    except (claude_client.ClaudeUnavailable, CardNotInDeck, DeckNotFound):
        # Answerable by the caller, and each maps to its own status code.
        raise
    except ModeExhausted as exc:
        raise ClaudeFailed(str(exc)) from exc
    except Exception as exc:
        # Broad on purpose, and narrow in effect: everything reaching here is
        # the SDK failing, and `explain` is the function that already knows how
        # to turn a 401 into "your key may have expired" rather than a stack
        # trace. Same treatment `claude check` gives it.
        raise ClaudeFailed(claude_client.explain(exc)) from exc


def claude_argue(*, slug: str, card: str, requested: Any = None,
                 focus: str = "",
                 source: DeckSource | None = None,
                 tier: str | None = None) -> dict[str, Any]:
    """Make the case against one card's slot (ADR 25). Returns charges.

    The interview's sibling and deliberately its twin: same arguments, same
    exceptions, same status codes. What differs is the direction of the answer
    -- the interview asks what you think, this says what is wrong -- and the
    one thing that does not differ is that neither may say what is right about
    a card in a form anybody could paste into `why`.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude.argue import CardNotInDeck, ask
    from mtglab.claude.modes import ModeExhausted

    try:
        return ask(slug, card, requested=requested, focus=focus,
                   source=source, tier=tier)
    except (claude_client.ClaudeUnavailable, CardNotInDeck, DeckNotFound):
        # Answerable by the caller, and each maps to its own status code.
        raise
    except ModeExhausted as exc:
        raise ClaudeFailed(str(exc)) from exc
    except Exception as exc:
        # See `claude_interview`: broad on purpose, narrow in effect.
        raise ClaudeFailed(claude_client.explain(exc)) from exc


def claude_dossier(*, slug: str, requested: Any = None, refresh: bool = False,
                   source: DeckSource | None = None) -> dict[str, Any]:
    """Who this deck's commander is (ADR 19). Cached on the commander.

    Note the two ways this differs from `claude_interview` above, both of them
    consequences of what a dossier *is*. It takes no card, because it is about
    the one card the deck is named for. And it can answer without reaching the
    network at all — a stored dossier is served whatever the stance, since
    reading a row somebody else's call produced is not a call.
    """
    from mtglab.claude import client as claude_client
    from mtglab.claude.dossier import NoCommander, ask
    from mtglab.claude.modes import ModeExhausted

    try:
        return ask(slug, requested=requested, refresh=refresh, source=source)
    except (claude_client.ClaudeUnavailable, NoCommander, DeckNotFound):
        raise
    except ModeExhausted as exc:
        raise ClaudeFailed(str(exc)) from exc
    except Exception as exc:
        raise ClaudeFailed(claude_client.explain(exc)) from exc


# **Neither half of the theme interview has a function here**, and the absence
# is deliberate rather than an omission. A service function is the natural seam
# for a Claude surface that answers inside its own request — which is what the
# rationale interview above still does. Both theme surfaces are background jobs,
# so their seam is a `Plan` rather than a return value, and both live in
# `api/themeruns.py` beside `api/simruns.py` for exactly the reason the sim
# planners are not in this file either.
#
# The proposal went first, at 226 measured seconds. The conversation turn
# followed it once somebody measured *that* too instead of trusting the
# docstring which said it was a few seconds; see `themeruns.plan_ask`.


def claude_dossier_cached(*, slug: str,
                          source: DeckSource | None = None) -> dict[str, Any]:
    """A stored dossier for this deck's commander, or a payload saying there is
    none. Never calls Anthropic, so the deck page can ask on every load.

    This is why the write path and the read path are separate functions rather
    than one with a flag: a GET that could spend money is a GET somebody will
    eventually put in a polling loop.
    """
    from mtglab.claude.dossier import NoCommander, brief, cache_key, get

    try:
        facts = brief(slug, source=source)
    except NoCommander:
        return {"slug": slug, "commander": "", "dossier": {}, "cached": False,
                "generated_at": None}
    card = facts["card"]
    hit = get(cache_key(card.get("oracle_id") or ""))
    return {
        "answered_by": "claude" if hit else "",
        "slug": slug,
        "commander": card["name"],
        "dossier": hit["result"] if hit else {},
        "cached": hit is not None,
        "generated_at": hit["created_at"] if hit else None,
    }


# ------------------------------------------------------------------- colours

def color_taxonomy() -> dict[str, Any]:
    """The 32 colour combinations, the five colours, and the three eras.

    Pure reference data -- no card pool, no deck source, no network. It is the
    vocabulary the create flow teaches, and it is the same table the 32 Deck
    Challenge is scored against.
    """
    return {
        "colors": [{"code": c.code, "name": c.name, "wants": c.wants,
                    "fears": c.fears} for c in colors.COLORS],
        "tiers": [{"key": t, "label": colors.TIER_LABELS[t],
                   "blurb": colors.TIER_BLURBS[t]}
                  for t in colors.TIERS],
        "eras": [{"name": e.name, "setting": e.setting, "named": e.named,
                  "story": e.story} for e in colors.ERAS],
        "combinations": [{
            "key": c.key,
            "name": c.name,
            "tier": c.tier,
            "colors": list(c.colors),
            "size": c.size,
            "tagline": c.tagline,
            "history": c.history,
            "aliases": list(c.aliases),
            "verified_by": c.verified_by,
            "lore": c.lore,
            # Names and roles only. The cards themselves come from
            # `combination_detail` below, which needs a card pool -- this payload
            # deliberately still does not.
            "champions": [{"card": ch.card, "role": ch.role}
                          for ch in c.champions],
            "signature": list(c.signature),
        } for c in colors.COMBINATIONS],
    }


def glossary() -> dict[str, Any]:
    """The vocabulary. Same properties as the taxonomy above: reference data,
    no card pool, no deck source, no network."""
    return {
        "sections": [{"key": s, "label": gloss.SECTION_LABELS[s],
                      "blurb": gloss.SECTION_BLURBS[s]}
                     for s in gloss.SECTIONS],
        "terms": [{"key": t.key, "term": t.term, "short": t.short,
                   "long": t.long, "section": t.section,
                   "see_also": list(t.see_also)} for t in gloss.TERMS],
    }


def lore_shelves() -> dict[str, Any]:
    """The fact volumes, with every named card resolved through the pool.

    Reference prose like `glossary()` -- checked-in, no key, no network --
    except that a fact may *name* cards, and those go through `get_cards`
    exactly as `combination_detail`'s champions do: resolved into the card's
    own cost, type and text, and **dropped and counted** when a name does not
    resolve, because a misspelled name rendering as a confident empty card is
    the failure this instrument exists to prevent. With no pool at all the
    prose still answers whole -- every fact reads complete without its cards,
    which is a writing rule in `lore.py`, not a hope.
    """
    con = _connect()
    found: dict[str, db.CardRecord] = {}
    wanted = sorted({name for f in lore.FACTS for name in f.cards})
    if con is not None:
        try:
            found = db.get_cards(con, wanted)
        finally:
            con.close()

    dropped = 0
    facts = []
    for f in lore.FACTS:
        cards = []
        for name in f.cards:
            rec = found.get(name)
            if rec is None:
                if con is not None:
                    dropped += 1
                continue
            cards.append({
                "name": rec.name, "mana_cost": rec.mana_cost,
                "type_line": rec.type_line, "oracle_text": rec.oracle_text,
                "color_identity": sorted(rec.color_identity),
                "image": rec.image_normal,
                "art_crop": getattr(rec, "image_art_crop", None),
            })
        facts.append({
            "key": f.key, "volume": f.volume, "fact": f.fact, "more": f.more,
            "cards": cards,
            "learn": {"tab": f.learn[0], "key": f.learn[1]} if f.learn else None,
        })

    return {
        "volumes": [{"key": v, "label": lore.VOLUME_LABELS[v],
                     "blurb": lore.VOLUME_BLURBS[v]} for v in lore.VOLUMES],
        "facts": facts,
        "pool": con is not None,
        "dropped": dropped,
    }


def combination_detail(key: str) -> dict[str, Any]:
    """One of the 32, with its champions and signature cards resolved.

    The split from `color_taxonomy` is the point. That payload is the table and
    works on a fresh clone; this one asks the pool, so it is where every card
    fact enters. A named card that does not resolve is **dropped and counted**
    rather than rendered from the name alone -- the instrument ADR 19 built for
    the dossier's competitors, pointed at reference data this time, because a
    misspelled name here would otherwise render as a confident empty card.

    `exact_total` is counted rather than stored, and it teaches something the
    prose cannot: exactly two cards in the pool have the Artifice identity,
    which is a sharper statement of what a four-colour slot is than any
    paragraph about refusing green.
    """
    # `key_for` canonicalises, so "GW" and "WG" are the same slot and a stray
    # lower-case URL still lands. Anything that is not five letters of WUBRG
    # collapses to "C", so an unknown key is answered by Colourless rather than
    # a 404 -- which would be wrong, so the spelling is checked afterwards.
    combo = colors.BY_KEY.get(colors.key_for(key.upper()))
    if combo is None or set(combo.colors) != set(key.upper()) - {"C"}:
        raise KeyError(key)

    base = {
        "key": combo.key, "name": combo.name, "tier": combo.tier,
        "colors": list(combo.colors), "size": combo.size,
        "tagline": combo.tagline, "history": combo.history,
        "lore": combo.lore, "aliases": list(combo.aliases),
        "verified_by": combo.verified_by,
    }
    con = _connect()
    if con is None:
        return {**base, "pool": False, "champions": [], "signature": [],
                "dropped": 0, "exact_total": None}
    try:
        wanted = [ch.card for ch in combo.champions] + list(combo.signature)
        found = db.get_cards(con, wanted)

        def as_card(rec: db.CardRecord) -> dict[str, Any]:
            return {
                "name": rec.name, "mana_cost": rec.mana_cost,
                "type_line": rec.type_line, "oracle_text": rec.oracle_text,
                "color_identity": sorted(rec.color_identity),
                "image": rec.image_normal,
                "art_crop": getattr(rec, "image_art_crop", None),
            }

        champions = [{"role": ch.role, **as_card(found[ch.card])}
                     for ch in combo.champions if ch.card in found]
        signature = [as_card(found[name])
                     for name in combo.signature if name in found]
        dropped = len(wanted) - len(champions) - len(signature)

        # Interpolated rather than parameterised because both values are
        # derived from the table above and neither is caller input: `listed` is
        # single letters of WUBRG and `size` is their count.
        listed = ", ".join(f"'{c}'" for c in combo.colors) or "''"
        row = con.execute(
            "SELECT count(*) FROM oracle_cards WHERE "
            "json_extract_string(legalities, 'commander') = 'legal' AND "
            f"len(list_filter(color_identity, x -> x NOT IN ({listed}))) = 0 "
            f"AND len(color_identity) = {combo.size}").fetchone()
        total = int(row[0]) if row else 0
    finally:
        con.close()

    return {**base, "pool": True, "champions": champions,
            "signature": signature, "dropped": dropped,
            "exact_total": total}


def challenge_progress(*, source: DeckSource | None = None) -> dict[str, Any]:
    """Which of the 32 slots the library has filled, and which are empty.

    A deck's slot is `colors.of(its colour identity)`, and that identity comes
    from the commander via the pool -- so without one this reports the slots
    as unknown rather than inventing them from the deck file.

    **The pool is asked once, not once per deck.** Read every deck first,
    collect the commanders, then make a single `get_cards` call -- which is
    what that function is for, and what every other batch caller here already
    does. The per-deck form cost one query per deck for one name each: six
    today, and linear in a library that is meant to reach thirty-two. Deck
    order is preserved by walking the same list twice rather than sorting it.
    """
    decks = _source(source)
    con = _connect()
    filled: dict[str, list[dict[str, str]]] = {}
    try:
        # Distinct *spellings*, not distinct cards: `get_cards` keys its result
        # by the name it was handed, so two decks naming the same commander
        # with different casing each need their own key back.
        ordered: list[tuple[str, Deck]] = []
        wanted: set[str] = set()
        if con is not None:
            for slug in decks.slugs():
                deck = Deck.from_text(decks.read_text(slug), slug=slug)
                if not deck.commander:
                    continue
                ordered.append((slug, deck))
                wanted.update(deck.commander)

        found = db.get_cards(con, sorted(wanted)) if wanted else {}
        for slug, deck in ordered:
            recs = [found[c] for c in deck.commander if c in found]
            if not recs:
                continue
            identity = frozenset().union(*(r.color_identity for r in recs))
            key = colors.key_for(identity)
            filled.setdefault(key, []).append({"slug": slug, "name": deck.name})
    finally:
        if con is not None:
            con.close()

    return {
        "pool": con is not None or bool(filled),
        "filled": len(filled),
        "total": len(colors.COMBINATIONS),
        "slots": [{
            "key": c.key, "name": c.name, "tier": c.tier,
            "decks": filled.get(c.key, []),
        } for c in colors.COMBINATIONS],
    }


class EditRejected(Exception):
    """The edit was refused, and nothing was written."""


class SwapRejected(EditRejected):
    """The swap was refused, and nothing was written."""


class BuildRejected(Exception):
    """Artifacts were refused, and nothing was written.

    Not an `EditRejected`: a build does not touch `deck.yaml`, so a refusal
    here has not prevented an edit and must not be reported as though it had.
    """


def _issues(report: ValidationReport) -> dict[str, list[dict[str, Any]]]:
    """The gate's verdict, flattened for JSON."""
    return {
        "errors": [{"code": i.code, "message": i.message, "card": i.card}
                   for i in report.errors],
        "warnings": [{"code": i.code, "message": i.message, "card": i.card}
                     for i in report.warnings],
    }


_WHOLE_LIBRARY = "this library"


def _for_writing(source: DeckSource | None,
                 subject: str = _WHOLE_LIBRARY) -> DeckSource:
    """The source, if this caller may write to it. `ReadOnlySource` if not.

    **This used to raise `EditRejected`, and so answered 422.** That was
    defensible while "read-only" was a property of the *source* — there was one
    library, everybody who could see it could write it, and the flag was
    unreachable in production because `FileDeckSource.writable` was hardcoded
    `True`. It stopped being defensible the moment the answer depended on *who
    was asking*: 422 says the request was malformed, and there is nothing wrong
    with this request except the person making it.

    So all three bespoke refusals — `EditRejected` here, `CreateRejected` in
    `create_deck` and `import_deck`, `DeleteRejected` in `delete_deck`, each
    chosen to match what its own route happened to catch — collapse into the
    one exception the protocol already defines, handled once in `api/app.py`
    as a 403. No client has ever seen the 422, so nothing is being broken.

    `subject` is what the message names: a slug where there is one, and the
    library itself where the deck does not exist yet.
    """
    decks = _source(source)
    if not decks.writable:
        # **Resolve the deck first when the source hides things** (ADR 22).
        # For a deck this caller cannot see the answer must be 404, and a 403
        # raised before the lookup would confirm it exists — the leak ADR 5
        # exists to prevent. So ask for it: a hidden deck raises
        # `DeckNotFound` here and a visible one falls through to the 403,
        # which is #80's answer and still the right one for a deck the caller
        # has just been listed.
        #
        # Still ahead of any pool work either way, so #80's other property
        # holds: the refusal does not depend on how far the edit would have
        # got, and a delete cannot fail after the deck has moved.
        if getattr(decks, "hides_decks", False) and subject != _WHOLE_LIBRARY:
            decks.get(subject)
        raise ReadOnlySource(subject)
    return decks


def _identity_of(deck: Deck, con: DuckDBPyConnection) -> frozenset[str]:
    """The commander's colour identity, from the pool.

    Rule 2: read off Scryfall's `color_identity`, never derived from the mana
    cost. It already accounts for back faces, reminder text and land types --
    which is what caught *Ajani, Nacatl Pariah* being illegal in a G/W deck.
    """
    identity: frozenset[str] = frozenset()
    for name in deck.commander:
        rec = db.get_cards(con, [name]).get(name)
        if rec is not None:
            identity |= rec.color_identity
    return identity


def owner_id_of(decks: DeckSource) -> int | None:
    """Which library these decks are in, as the activity log keys it.

    Public since the match ledger (ADR 36): `api/app.py`'s Forge route asks
    the same question of each seat's source, and the two ledgers must key
    ownership identically or one deck's history splits in two.

    `None` is the file-backed curated library, which is one per instance and
    has no row in `users` to point at; a `SqlDeckSource` answers with the
    account whose decks it holds. Asked of the *source* rather than passed in
    from the route, because the source is already the thing that decided which
    library the caller is in (`decks/library.py`) -- a second answer arriving
    beside it could disagree with it.

    `getattr` rather than a method on the protocol, for the same reason
    `_for_writing` asks about `hides_decks` that way: this is a fact one
    implementation has and the others do not, and putting it on `DeckSource`
    would oblige every future source to have an opinion about a table it does
    not use.
    """
    owner = getattr(decks, "owner_id", None)
    return owner if isinstance(owner, int) else None


def _find_card(deck: Deck, name: str) -> CardEntry | None:
    wanted = name.strip().lower()
    return next((c for c in deck.cards + deck.swap_board
                 if c.name.lower() == wanted), None)


def _commit(slug: str, decks: DeckSource, updated: str,
            *, actor: str | None = None, **extra: Any) -> dict[str, Any]:
    """Write an edited deck, and hand back the gate's verdict on the result.

    Every write goes out through here, so no caller can change a deck without
    being told what the change did to the gate. `stage` and `needs_rationale`
    ride along because an edit is the most likely moment for either to move --
    filling in the last blank `why` is what makes a draft promotable.

    **And so no caller can change a deck without the change being recorded**
    (ADR 28). `extra` has always been the per-operation description and has
    always been thrown away with the response; `decks/log.py` keeps it. That it
    is written here rather than in the nine functions above is the same
    argument the gate makes one paragraph up: the tenth edit operation is the
    one somebody adds in a year, and it inherits both.

    The entry is written **after** the deck and before the gate runs, which is
    the honest order: the edit has happened by then, and a validation that
    blows up afterwards must not be able to erase the record of it.
    """
    decks.write_text(slug, updated)
    action, summary = log.describe(extra)
    log.record(slug=slug, action=action, summary=summary,
               owner_id=owner_id_of(decks), actor=actor)
    after = decks.get(slug)
    con = _connect()
    try:
        report = validate(after, _pool_for(after, con))
    finally:
        if con is not None:
            con.close()
    return {
        "slug": slug,
        **extra,
        "stage": after.stage,
        "total_cards": after.total_cards,
        "needs_rationale": len(after.unjustified),
        "ok": report.ok,
        **_issues(report),
    }


def _check_category(category: str) -> str:
    """Refuse a category outside the fixed set.

    Stricter than the gate, which only warns, and deliberately so: the warning
    is there for hand-written files, while an edit through this path is a
    choice from a list the caller was shown. `CATEGORIES` is small and fixed so
    that counts are comparable across decks, and a typo accepted here would
    quietly cost exactly that comparability.
    """
    category = category.strip().lower()
    if category not in CATEGORIES:
        raise EditRejected(
            f"{category!r} is not a category; choose one of {', '.join(CATEGORIES)}")
    return category


def add_card(slug: str, *, name: str, category: str, why: str = "",
             qty: int = 1, to: str = "cards",
             source: DeckSource | None = None,
             actor: str | None = None) -> dict[str, Any]:
    """Put a card into the 99 or onto the swap board.

    Checked against the pool before anything is written -- the card has to
    exist, be legal in Commander, and sit inside the commander's colour
    identity. That is rule 1 applied to a write: a card nobody looked up is a
    card whose legality is a guess.

    The rationale requirement is the edit layer's, not this function's, because
    it depends on the deck: a curated deck refuses a blank `why`, a draft
    counts it as work still owed (ADR 13). Neither writes one.
    """
    if to not in edit.CARD_LISTS:
        raise EditRejected(f"cards go into {' or '.join(edit.CARD_LISTS)}, not {to!r}")
    category = _check_category(category)
    decks = _for_writing(source, slug)
    deck = decks.get(slug)

    con = _connect()
    if con is None:
        raise EditRejected("adding a card needs the card pool -- "
                           "run `mtglab data refresh`")
    try:
        rec = db.get_cards(con, [name]).get(name)
        if rec is None:
            raise EditRejected(f"{name!r} is not a card the pool knows")
        if not rec.legal_commander:
            raise EditRejected(f"{rec.name} is not legal in Commander")

        identity = _identity_of(deck, con)
        outside = rec.color_identity - identity
        if outside:
            raise EditRejected(
                f"{rec.name}'s identity {{{''.join(sorted(rec.color_identity))}}} "
                f"includes {{{''.join(sorted(outside))}}}, outside the "
                f"commander's {{{''.join(sorted(identity)) or 'C'}}}")

        try:
            updated = edit.add_card(decks.read_text(slug), name=rec.name,
                                    category=category, why=why, qty=qty,
                                    list_key=to)
        except EditFailed as exc:
            raise EditRejected(str(exc)) from exc
        return _commit(slug, decks, updated, actor=actor, added=rec.name,
                       category=category, into=to)
    finally:
        con.close()


def remove_card(slug: str, *, name: str,
                source: DeckSource | None = None,
                actor: str | None = None) -> dict[str, Any]:
    """Take a card out of the deck -- to the graveyard, if it was in the 99.

    Since ADR 27 a removal from the 99 is an **entombment**: the entry moves to
    the deck's graveyard with its category and its `why` intact, and can be
    returned or exiled from there. A swap-board card is still removed outright
    -- it was already outside the deck, and the board is its own record of why.

    Needs no card pool either way: removing a card is a fact about this deck
    file, not about Magic. That matters because it means a deck can still be
    pruned on a machine that has never run `data refresh`.
    """
    decks = _for_writing(source, slug)
    deck = decks.get(slug)
    entry = _find_card(deck, name)
    if entry is None:
        raise EditRejected(f"{name!r} is not in this deck")
    in_99 = any(c.name == entry.name for c in deck.cards)
    try:
        if in_99:
            updated = edit.entomb_card(decks.read_text(slug), name=entry.name)
        else:
            updated = edit.remove_card(decks.read_text(slug), name=entry.name)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    if in_99:
        return _commit(slug, decks, updated, actor=actor, entombed=entry.name)
    return _commit(slug, decks, updated, actor=actor, removed=entry.name)


def entomb_cards(slug: str, *, names: list[str],
                 source: DeckSource | None = None,
                 actor: str | None = None) -> dict[str, Any]:
    """Entomb several cards from the 99 in one write.

    All or nothing: a name that is not in the 99 refuses the whole batch
    before anything is written, because a sweep that silently skipped two of
    its ten cards would report a deck state nobody chose. One write, one gate
    verdict, one entry in the deck's activity log (ADR 28).
    """
    wanted = [n.strip() for n in names if str(n).strip()]
    if not wanted:
        raise EditRejected("nothing to entomb; give at least one card name")
    decks = _for_writing(source, slug)
    deck = decks.get(slug)
    in_99 = {c.name.lower(): c.name for c in deck.cards}
    resolved: list[str] = []
    for name in wanted:
        match = in_99.get(name.lower())
        if match is None:
            raise EditRejected(
                f"{name!r} is not in the 99, so nothing was entombed")
        if match in resolved:
            raise EditRejected(f"{match!r} is listed twice")
        resolved.append(match)
    text = decks.read_text(slug)
    try:
        for name in resolved:
            text = edit.entomb_card(text, name=name)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, text, actor=actor, entombed=resolved)


def return_card(slug: str, *, name: str,
                source: DeckSource | None = None,
                actor: str | None = None) -> dict[str, Any]:
    """Bring an entombed card back into the 99, exactly as it left.

    The `why` that returns is the user's own words, preserved through the
    graveyard -- nothing is composed or re-invented, which is what keeps
    rule 4 out of this path entirely.
    """
    decks = _for_writing(source, slug)
    try:
        updated = edit.return_card(decks.read_text(slug), name=name)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, actor=actor, returned=name)


def exile_card(slug: str, *, name: str,
               source: DeckSource | None = None,
               actor: str | None = None) -> dict[str, Any]:
    """Remove an entombed card from the graveyard permanently.

    The only genuinely destructive delete left, and it can only ever act on a
    card that was already entombed -- two deliberate steps, by construction.
    """
    decks = _for_writing(source, slug)
    try:
        updated = edit.exile_card(decks.read_text(slug), name=name)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, actor=actor, exiled=name)


def set_card_field(slug: str, *, name: str, field: str, value: Any,
                   source: DeckSource | None = None,
                   actor: str | None = None) -> dict[str, Any]:
    """Change one card's category, quantity, or rationale.

    This is the write path behind the rationale editor, and the answer to the
    gap `decks import` opened: a 99-card draft arrives owing 99 rationales, and
    until now the only way to write one was a text editor.

    The rationale is the caller's text, written as given. Nothing here -- and
    nothing in the layer below -- composes, tidies, expands or infers one. That
    is rule 4, and [ADR 12](docs/adr/0012) rule 3 is where it binds a tool
    rather than a person.
    """
    decks = _for_writing(source, slug)
    entry = _find_card(decks.get(slug), name)
    if entry is None:
        raise EditRejected(f"{name!r} is not in this deck")
    if field == "category":
        value = _check_category(str(value))
    # `art` is checked against the pool here for the same reason
    # `commander_art` is in `set_deck_field`: the editor can tell a printing
    # id from a typo by its shape, and only a query can tell whether that id
    # is a printing *of this card*.
    if field == "art" and str(value or "").strip():
        _check_printing_of(
            entry.name, str(value).strip(),
            hint="the card's own printings list has the ones that are.")
    try:
        updated = edit.set_card_field(decks.read_text(slug), name=entry.name,
                                      field=field, value=value)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, actor=actor, card=entry.name,
                   field=field)


def _check_printing_of(name: str, printing_id: str, *, hint: str) -> None:
    """Refuse an art id that is not a printing of this card.

    Silence when there is no card pool: a fresh clone cannot check, and refusing
    every art change on a machine without a 500MB download would be a worse
    answer than accepting one that renders as the default.
    """
    con = _connect()
    if con is None:
        return
    try:
        row = con.execute(
            "SELECT p.name FROM printings p WHERE p.id = ? AND p.oracle_id = "
            "(SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)",
            [printing_id, name]).fetchone()
    finally:
        con.close()
    if row is None:
        raise EditRejected(
            f"{printing_id} is not a printing of {name}. {hint}")


def _check_printing(deck: Any, printing_id: str) -> None:
    """The commander's version of the check above."""
    if not deck.commander:
        raise EditRejected("this deck has no commander, so it has no art to set")
    _check_printing_of(
        deck.commander[0], printing_id,
        hint=f"`GET /api/decks/{deck.slug}/printings` lists the ones that are.")


def set_deck_field(slug: str, *, field: str, value: Any,
                   source: DeckSource | None = None,
                   actor: str | None = None) -> dict[str, Any]:
    """Change one of the deck's own fields: stage, status, a label, and kin.

    Promotion -- `stage` to `curated` -- is the one that closes the import
    lifecycle. It is refused while any card is blank, by the edit layer, so the
    deck is never written into a state its author has to undo.

    `commander_art` is checked here rather than in `edit.py`, because it is the
    one settable field whose validity is a question for the pool: the editor
    can tell a printing id from a typo by its shape, and only a query can tell
    whether that id is a printing *of this commander*. Pointing a deck at some
    other card's art would be accepted by every check that did not ask.
    """
    decks = _for_writing(source, slug)
    deck = decks.get(slug)              # 404 before anything else
    if field == "commander_art" and str(value or "").strip():
        _check_printing(deck, str(value).strip())
    try:
        updated = edit.set_deck_field(decks.read_text(slug), field=field,
                                      value=value)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, actor=actor, field=field, value=value)


def set_note(slug: str, *, key: str, value: str,
             source: DeckSource | None = None,
             actor: str | None = None) -> dict[str, Any]:
    """Set one deck-level note -- the prose the advanced primer reads directly.

    Notes are the deck's thinking, and they live in the source of truth rather
    than in an artifact precisely so that regenerating the five deliverables
    cannot lose them.
    """
    decks = _for_writing(source, slug)
    decks.get(slug)                     # 404 before anything else
    try:
        updated = edit.set_note(decks.read_text(slug), key=key, value=value)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, actor=actor, note=key.strip())


def swap_card(slug: str, *, out: str, into: str, why: str,
              source: DeckSource | None = None,
              actor: str | None = None) -> dict[str, Any]:
    """Replace one card in a deck with another, and re-run the gate.

    This is not the auto-substitution ADR 8 rejected. The judgement stays with
    the caller: they name the card going in and they write the rationale. What
    changed is that carrying out a decision they already made no longer means
    hand-editing YAML.

    Everything is checked before anything is written, and the edit itself is
    surgical -- see `decks/edit.py` for why a load-and-dump was not an option.
    """
    # No longer wrapped into `SwapRejected`: that translation existed only to
    # turn `_for_writing`'s `EditRejected` into something this route caught,
    # and both ends of it are gone. `ReadOnlySource` goes straight to the 403
    # handler, like every other refused write.
    decks = _for_writing(source, slug)
    if not why.strip():
        # Rule 4, enforced at the boundary as well as in the editor: a card
        # that cannot justify its slot is a card to cut.
        raise SwapRejected("a replacement needs a `why`")

    deck = decks.get(slug)
    entry = _find_card(deck, out)
    if entry is None:
        raise SwapRejected(f"{out!r} is not in this deck")
    if any(c.name.lower() == into.strip().lower()
           for c in deck.cards + deck.swap_board):
        raise SwapRejected(f"{into!r} is already in this deck")
    if any(c.lower() == into.strip().lower() for c in deck.commander):
        raise SwapRejected(f"{into!r} is the commander")

    con = _connect()
    if con is None:
        raise SwapRejected("swapping needs the card pool -- run `mtglab data refresh`")
    try:
        found = db.get_cards(con, [into])
        rec = found.get(into)
        if rec is None:
            raise SwapRejected(f"{into!r} is not a card the pool knows")
        if not rec.legal_commander:
            raise SwapRejected(f"{rec.name} is not legal in Commander")

        identity = _identity_of(deck, con)
        outside = rec.color_identity - identity
        if outside:
            raise SwapRejected(
                f"{rec.name}'s identity {{{''.join(sorted(rec.color_identity))}}} "
                f"includes {{{''.join(sorted(outside))}}}, outside the "
                f"commander's {{{''.join(sorted(identity)) or 'C'}}}")

        try:
            updated = edit.replace_card(decks.read_text(slug), old_name=entry.name,
                                        new_name=rec.name, why=why.strip())
        except EditFailed as exc:
            raise SwapRejected(str(exc)) from exc
    finally:
        con.close()

    return _commit(slug, decks, updated, actor=actor, swapped_out=entry.name,
                   swapped_in=rec.name, why=why.strip())


def wheel_spin(slug: str, *, source: DeckSource | None = None,
               seed: int | None = None) -> dict[str, Any]:
    """One turn of the Wheel of Fortune for this deck (punch list item 9).

    Deterministic and seeded like Tier 1: `decks/wheel.py` picks a fate and a
    random pool card in the commander's identity that answers to it, and the
    seed comes back so the same spin can be spun again. Read-only -- the wheel
    suggests, it never edits, and no rationale is prefilled (rule 4).
    """
    deck = _source(source).get(slug)
    con = _connect()
    if con is None:
        return {"pool_available": False, "card": None, "symbol": None,
                "message": "no card pool yet -- run `mtglab data refresh`"}
    try:
        cards = _pool_for(deck, con)
        commander = cards.get(deck.commander[0]) if deck.commander else None
        identity = commander.color_identity if commander else frozenset()
        return {"pool_available": True,
                **wheel.spin(deck, identity, con, seed=seed)}
    finally:
        con.close()


def suggestions_for(slug: str, *, source: DeckSource | None = None,
                    limit: int = 5) -> dict[str, Any]:
    """Replacement shortlists for the cards the gate says have to go.

    Only for errors a different card would actually fix -- a missing `why` is a
    real error, and swapping the card does not resolve it.

    Deliberately reports rather than resolves. ADR 8 rejected auto-substitution
    on the grounds that a tool which quietly swaps cards is one whose output you
    can no longer trust to be your deck, and that has not changed because the
    suggestions got good.
    """
    deck = _source(source).get(slug)
    con = _connect()
    if con is None:
        return {"slug": slug, "pool_available": False, "targets": []}
    try:
        cards = _pool_for(deck, con)
        rep = validate(deck, cards)
        fixable = [(i.card, i.code) for i in rep.errors
                   if i.card and i.code in ("banned", "color-identity")]

        targets = []
        for name, code in fixable:
            candidates = suggest.replacements_for(deck, cards, con, name, limit=limit)
            targets.append({
                "card": name,
                "code": code,
                "why": next((c.why for c in deck.cards if c.name == name), ""),
                "candidates": [{
                    "name": c.name,
                    "mana_cost": c.record.mana_cost,
                    "cmc": c.record.cmc,
                    "type_line": c.record.type_line,
                    "oracle_text": c.record.oracle_text,
                    "color_identity": sorted(c.record.color_identity),
                    "image": c.record.image_normal,
                    "art_crop": c.record.image_art_crop,
                    "edhrec_rank": c.record.edhrec_rank,
                    "score": c.score,
                    "reasons": list(c.reasons),
                } for c in candidates],
            })
        return {"slug": slug, "pool_available": True, "targets": targets}
    finally:
        con.close()


def stats_for(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        stats = deck_stats(deck, _pool_for(deck, con))
        # Dataclasses in the curve buckets need flattening for JSON.
        stats["curve"] = {
            "average_mv": stats["curve"]["average_mv"],
            "nonland_cards": stats["curve"]["nonland_cards"],
            "buckets": [{"mv": b.mv, "count": b.count, "names": b.names}
                        for b in stats["curve"]["buckets"]],
        }
        return stats
    finally:
        if con is not None:
            con.close()


# -------------------------------------------------------------------- cards

#: A rationale interview walks a whole deck, so the cap is a deck: 99 plus a
#: commander. Beyond that a caller wants `search_cards`, not a lookup.
MAX_NAMED_CARDS = 100


def cards_named(*, names: list[str]) -> dict[str, Any]:
    """Exact-name card lookup. The answer to "what does this card actually do".

    Distinct from `search_cards`, and the distinction is the point.
    `search_cards` filters to `legalities.commander = 'legal'`, which is right
    when the question is "what could I play" and wrong when the question is
    "what is this card" -- **a banned card is invisible to it.** That was found
    by running a Claude turn rather than by reasoning about it: asked what the
    two cards failing the gate do, it could not look up either Emrakul, the
    Aeons Torn or Primeval Titan, said so, and answered from labelled recall.
    Honest, and still rule 1 failing.

    So this filters on nothing. It reports `legal_commander` per card instead,
    which is strictly more useful: a banned card comes back with its real
    oracle text *and* the fact that it is banned, rather than as a shrug.

    **Names that do not resolve are returned in `not_found`, never omitted.**
    `db.get_cards` drops misses silently and says callers must handle that
    loudly; this is that handling. A lookup that quietly returns four cards for
    five names is how a confident claim gets made about the fifth.
    """
    wanted = [n.strip() for n in names if n and n.strip()][:MAX_NAMED_CARDS]
    if not wanted:
        return {"cards": [], "not_found": [], "pool_available": True}

    con = _connect()
    if con is None:
        return {"cards": [], "not_found": wanted, "pool_available": False,
                "message": "no card pool yet -- run `mtglab data refresh`"}
    try:
        found = db.get_cards(con, wanted)
        cards = []
        for asked in wanted:
            rec = found.get(asked)
            if rec is None:
                continue
            cards.append({
                # The pool's spelling, not the caller's. Asked for "arahbo,
                # roar of the world" you get the real name back, which is what
                # any follow-up edit has to be keyed on.
                "name": rec.name,
                "asked_as": asked if asked != rec.name else None,
                "mana_cost": rec.mana_cost,
                "cmc": rec.cmc,
                "type_line": rec.type_line,
                "oracle_text": rec.oracle_text,
                # From Scryfall's own field, never derived from the mana cost:
                # it already accounts for back faces, reminder text and land
                # types. Rule 2, and the reason Ajani, Nacatl Pariah is {R}{W}.
                "color_identity": sorted(rec.color_identity),
                "keywords": list(rec.keywords),
                "layout": rec.layout,
                "legal_commander": rec.legal_commander,
                "reserved": rec.reserved,
                "edhrec_rank": rec.edhrec_rank,
                "image": rec.image_normal,
                # Cropped art, for callers rendering a card as a chip rather
                # than as a scan -- the dossier's competitors, for one.
                "art_crop": getattr(rec, "image_art_crop", None),
                # Strings, because "*" and "1+*" are real printed values. For
                # a double-faced card these are the front face's, matching
                # `mana_cost`; both faces stay available in `card_faces`.
                "power": rec.power,
                "toughness": rec.toughness,
                "loyalty": rec.loyalty,
                "defense": rec.defense,
                "game_changer": rec.game_changer,
                "flavor_text": rec.flavor_text,
                "artist": rec.artist,
            })
        return {
            "cards": cards,
            "not_found": [n for n in wanted if n not in found],
            "pool_available": True,
        }
    finally:
        con.close()


def _identified_card(rec: CardRecord) -> dict[str, Any]:
    """The compact card a camera review list renders.

    Narrower than `search_cards`' shape on purpose: this list can hold forty
    entries, each with a picture, on a phone. Oracle text and price are what
    the card's own page is for.
    """
    return {
        "name": rec.name,
        "mana_cost": rec.mana_cost,
        "type_line": rec.type_line,
        "color_identity": sorted(rec.color_identity),
        "image": rec.image_normal,
        "art_crop": rec.image_art_crop or art_crop_from(rec.image_normal),
    }


def identify_cards(sightings: list[dict[str, Any]]) -> dict[str, Any]:
    """Read a batch of camera captures against the pool.

    The serving half of `cards/identify.py`; the argument for the two tiers
    is in that module's docstring and is not repeated here. What this layer
    adds is hydration -- a reading names cards, and a review list has to show
    them -- and the counts, which are what the page says out loud.

    **`resolved` and `offered` are counted apart, and that is the point.**
    Only a corner lookup resolves; a title is a shortlist somebody still has
    to choose from. A page that added the two together would be reporting
    work as finished that nobody has done, which is the same mistake the
    import preview refuses when it counts the rationales still owed.

    A name that no longer resolves is dropped and counted, the instrument
    ADR 19 built for the dossier's rivals -- here it can only mean the pool
    moved under a reading, which a `data refresh` mid-session can do.
    """
    con = _connect()
    if con is None:
        return {"readings": [], "resolved": 0, "offered": 0, "unread": 0,
                "dropped": 0,
                "message": "no card pool yet -- run `mtglab data refresh`"}
    try:
        seen = [
            identify.Sighting(
                set_code=(s.get("set") or None),
                collector_number=(s.get("number") or None),
                title=(s.get("title") or None),
                # The bottom-left block as the reader saw it. Preferred over
                # the two fields above, because finding a set code inside
                # `LTCENLIK` needs the pool's own 986 -- see `from_corner`.
                corner=(s.get("corner") or None),
            )
            for s in sightings[:identify.MAX_SIGHTINGS]
        ]
        readings = identify.read(con, seen)

        wanted = {r.resolved for r in readings if r.resolved}
        wanted |= {c.name for r in readings for c in r.candidates}
        records = db.get_cards(con, sorted(wanted)) if wanted else {}

        out: list[dict[str, Any]] = []
        dropped = 0
        for reading in readings:
            resolved = records.get(reading.resolved or "")
            if reading.resolved and resolved is None:
                dropped += 1
            candidates = []
            for candidate in reading.candidates:
                rec = records.get(candidate.name)
                if rec is None:
                    dropped += 1
                    continue
                candidates.append({**_identified_card(rec),
                                   "score": round(candidate.score, 4)})
            # Recomputed rather than passed through: a reading whose names
            # all dropped is a reading of nothing, whichever tier found them.
            via = ("printing" if resolved
                   else "title" if candidates
                   else "nothing")
            out.append({
                "via": via,
                "resolved": _identified_card(resolved) if resolved else None,
                "candidates": candidates,
            })

        return {
            "readings": out,
            "resolved": sum(1 for r in out if r["resolved"]),
            "offered": sum(1 for r in out
                           if not r["resolved"] and r["candidates"]),
            "unread": sum(1 for r in out
                          if not r["resolved"] and not r["candidates"]),
            "dropped": dropped,
        }
    finally:
        con.close()


def search_cards(*, q: str = "", identity: str = "", type_line: str = "",
                 cmc_max: float | None = None, price_max: float | None = None,
                 sort: str = "edhrec", limit: int = 60,
                 identity_exact: bool = False,
                 commanders_only: bool = False) -> dict[str, Any]:
    """Pool search: the 'deep hits from the whole history' tool.

    `identity` is a subset filter, not an exact match -- passing "BG" returns
    every card legal in a Golgari deck, which includes colorless and mono
    cards. That is the question a deckbuilder actually asks.

    `identity_exact` flips it, and the create flow needs the flip. Choosing
    Selesnya and being offered mono-white legends is not wrong -- they are
    legal in a Selesnya deck -- but a deck led by one *is* a mono-white deck,
    so it fills a different slot in the 32. When the question is "which
    commander makes this combination", only an exact match answers it.

    `commanders_only` filters to cards that may actually lead a deck, using
    the same `partners.can_be_commander` the gate uses rather than a type-line
    guess -- which matters for the cards that are legal commanders without
    being Legendary Creatures.
    """
    con = _connect()
    if con is None:
        return {"cards": [], "total": 0,
                "message": "no card pool yet -- run `mtglab data refresh`"}
    try:
        where = ["json_extract_string(legalities, 'commander') = 'legal'"]
        params: list[Any] = []

        if identity or identity_exact:
            allowed = [c for c in identity.upper() if c in "WUBRG"]
            listed = ", ".join(f"'{c}'" for c in allowed) or "''"
            where.append(
                f"len(list_filter(color_identity, x -> x NOT IN ({listed}))) = 0")
            if identity_exact:
                # Subset plus the right size is set equality, and it lets the
                # colourless slot work: an empty identity with length 0.
                where.append(f"len(color_identity) = {len(allowed)}")
        # `contains(lower(col), ?)` rather than `col ILIKE '%?%'`, which is the
        # same question asked cheaply: ILIKE runs a pattern matcher with case
        # folding over every row of `oracle_text`, and this walks the string
        # once. **67.1ms -> 18.6ms** on the full pool for `goblin`, the same
        # 431 cards back; checked against ILIKE over eight queries including
        # accents and `//`, byte-identical every time (2026-08-19).
        #
        # It also stops `%` and `_` in a search box from behaving as wildcards
        # nobody typed on purpose -- a search for "50%" now looks for "50%".
        if q:
            where.append("(contains(lower(name), ?) "
                         "OR contains(lower(oracle_text), ?))")
            params += [q.lower(), q.lower()]
        if type_line:
            where.append("contains(lower(type_line), ?)")
            params.append(type_line.lower())
        if commanders_only:
            # A *superset* of `partners.can_be_commander`, pushed into SQL so
            # that LIMIT counts candidates rather than counting spells that are
            # about to be discarded. Without it the filter runs after the
            # limit, and a search for Selesnya commanders returns the sixty
            # best Selesnya cards, none of which is a commander, and then
            # nothing at all. The authoritative check still runs in Python
            # below -- this only narrows what the database hands over, and it
            # is deliberately loose (a card whose *back* face is a legendary
            # creature matches here and is rejected there).
            where.append("(type_line ILIKE '%Legendary%Creature%'"
                         " OR contains(lower(oracle_text),"
                         " 'can be your commander'))")
        if cmc_max is not None:
            where.append("cmc <= ?")
            params.append(cmc_max)

        order = {
            "edhrec": "edhrec_rank NULLS LAST",
            "cmc": "cmc, edhrec_rank NULLS LAST",
            "name": "name",
            "newest": "released_at DESC NULLS LAST",
        }.get(sort, "edhrec_rank NULLS LAST")

        sql = f"""
            SELECT o.name, o.mana_cost, o.cmc, o.type_line, o.oracle_text,
                   o.color_identity, o.edhrec_rank, o.image_normal,
                   o.image_art_crop, o.reserved,
                   (SELECT min(p.price_usd) FROM printings p
                     WHERE p.oracle_id = o.oracle_id AND p.price_usd IS NOT NULL) AS usd
            FROM oracle_cards o
            WHERE {' AND '.join(where)}
            ORDER BY {order}
            LIMIT ?
        """
        rows = con.execute(sql, [*params, min(int(limit), 200)]).fetchall()
        cards = [{
            "name": r[0], "mana_cost": r[1], "cmc": r[2], "type_line": r[3],
            "oracle_text": r[4], "color_identity": sorted(r[5] or []),
            "edhrec_rank": r[6], "image": r[7], "art_crop": r[8],
            "reserved": r[9], "price_usd": r[10],
        } for r in rows]
        if price_max is not None:
            cards = [c for c in cards
                     if c["price_usd"] is not None and c["price_usd"] <= price_max]
        if commanders_only:
            # Applied after the query rather than in SQL, because the rule is
            # `partners.can_be_commander` and that reads oracle text as well as
            # the type line. One implementation of the rule, not two.
            keep = db.get_cards(con, [c["name"] for c in cards])
            cards = [c for c in cards
                     if c["name"] in keep
                     and partners.can_be_commander(keep[c["name"]])]
        return {"cards": cards, "total": len(cards)}
    finally:
        con.close()


# --------------------------------------------------------------------- sets

_SETS_CACHE: dict[str, Any] = {}
_SETS_STATS = caches.register(
    "sets.upcoming", clear=_SETS_CACHE.clear,
    size=lambda: 1 if _SETS_CACHE else 0,
    note="Scryfall's unreleased sets, held for the calendar day")


def upcoming_sets(*, force: bool = False) -> dict[str, Any]:
    """Unreleased sets, live from Scryfall, cached for the process lifetime.

    This is the one route that reaches the network on demand. Spoiler scanning
    is meaningless against a card pool that by definition does not have the cards
    yet, so the set list has to come from upstream.
    """
    today = date.today().isoformat()
    if not force and _SETS_CACHE.get("day") == today:
        _SETS_STATS.hit()
        # The cache maps two keys to two shapes, so indexing is Any; the cast
        # states what "data" holds rather than widening the return type.
        return cast("dict[str, Any]", _SETS_CACHE["data"])
    _SETS_STATS.miss()

    req = urllib.request.Request(
        SCRYFALL_SETS, headers={"User-Agent": USER_AGENT,
                                "Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as resp:
        payload = json.load(resp)

    out = []
    for s in payload.get("data", []):
        released = s.get("released_at")
        if released and released > today and not s.get("digital"):
            out.append({
                "code": s.get("code"), "name": s.get("name"),
                "released_at": released, "card_count": s.get("card_count"),
                "icon": s.get("icon_svg_uri"), "set_type": s.get("set_type"),
            })
    data = {"sets": sorted(out, key=lambda s: s["released_at"]), "as_of": today}
    _SETS_CACHE.update(day=today, data=data)
    return data


# ------------------------------------------------------------------ artifacts

#: How a build's output compares to the deck as it stands now. Three states
#: rather than a `stale` boolean, because the honest third answer exists and a
#: boolean would have to lie about it: a deck built before ADR 30's snapshot
#: mechanism has artifacts and no baseline, so nothing can say whether they
#: match. Every deck on the volume was in exactly that position on 2026-08-21.
BASELINE_STATES = ("current", "different", "unknown")

#: What a corrupt snapshot can raise on the way through `Deck.from_text`.
#: Named rather than caught blind, so a genuine bug in the parser surfaces as
#: a 500 instead of being quietly reported as "no baseline" -- which would be
#: the same shape of silence the stale-artifact problem already was.
_UNPARSEABLE = (yaml.YAMLError, AttributeError, TypeError, ValueError)


def _baseline_state(deck: Deck, baseline: str | None) -> str:
    """Compare this deck against the snapshot the last build stashed.

    A normalised `dump()` on both sides rather than a text diff, for the reason
    `SNAPSHOT` is a dump in the first place: the question is whether the deck
    changed, and a reflowed comment is not a change. This is the same
    comparison `swap_list` makes, asked as a yes/no.
    """
    if baseline is None:
        return "unknown"
    try:
        previous = Deck.from_text(baseline, slug=deck.slug)
    except _UNPARSEABLE:
        # A snapshot that will not parse is a baseline that cannot answer.
        # Not an error: the next build simply overwrites it.
        return "unknown"
    return "current" if previous.dump() == deck.dump() else "different"


def _artifacts_json(source: DeckSource, deck: Deck) -> dict[str, Any]:
    """What this deck has been built into, and whether that is still true."""
    held = source.artifacts(deck.slug)
    return {
        "artifacts": [{"name": a.name, "size": a.size,
                       "built_at": a.built_at.isoformat()} for a in held],
        "baseline": _baseline_state(deck, source.read_baseline(deck.slug)),
        "buildable": deck.stage != "draft",
        "stage": deck.stage,
    }


def list_artifacts(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    """The five deliverables this deck has, and how current they are.

    A plain read, so it answers for a deck nobody may write: the artifacts are
    the *shareable* surface, and hiding them from a reader who can already see
    the deck would be the wrong way round.
    """
    decks = _source(source)
    return _artifacts_json(decks, decks.get(slug))


def read_artifact(slug: str, name: str, *,
                  source: DeckSource | None = None) -> str:
    """One deliverable's text, verbatim. Raises `ArtifactNotFound`.

    Text rather than a parsed anything: these are the files a person copies
    into Moxfield or reads on the train, and the app's job is to hand them
    over unchanged.
    """
    decks = _source(source)
    decks.get(slug)             # raises DeckNotFound before anything else
    return decks.read_artifact(slug, name)


def build_artifacts(slug: str, *, force: bool = False,
                    source: DeckSource | None = None) -> dict[str, Any]:
    """Generate the five deliverables and store them. Raises `BuildRejected`.

    The hosted half of `mtglab decks build`, and it exists because the ruling
    that made `mtglab ui` a development harness turned "the CLI can do it" into
    a gap rather than a design. Until 2026-08-21 the only way to rebuild a
    deployed deck's artifacts was to open a shell on the instance, which is the
    laptop coupling the volume ruling was meant to end.

    **A plain route, not a job, and that was measured rather than assumed.**
    Four real decks on the deployed instance: 70-83ms warm, end to end --
    load, pool, gate and render. That is the shelf's order of magnitude
    (`api/shelfruns.py`, 0.03-0.04s), where the argument against a job is that
    submitting and polling costs more than the work. The 1.5s a cold
    `mtglab decks build` takes is almost all interpreter start-up, which a
    served process has already paid.

    Two refusals, and only one of them is forceable. A **draft** is refused by
    `render_all` and cannot be forced from here any more than from the CLI --
    the way out is to write the rationales and promote it (ADR 13). **Gate
    errors** are refused by default and `force` overrides them, which mirrors
    `mtglab decks build --force` exactly; Goreclaw's banned card is the live
    example, and a theoretical deck whose author knows about it is the case
    the flag is for.

    **Deliberately not recorded in the activity log.** ADR 28 logs edits to
    `deck.yaml` from one call site, and a build changes no deck field -- it
    derives files *from* the deck. Logging it would mean a second call site
    outside `_commit`, which CLAUDE.md names as a decision to take
    deliberately rather than by drift. This is that decision, taken: no.
    """
    from mtglab.artifacts.generate import DraftDeck, render_all

    decks = _for_writing(source, slug)
    deck = decks.get(slug)

    con = _connect()
    try:
        cards = _pool_for(deck, con)
    finally:
        if con is not None:
            con.close()

    report = validate(deck, cards)
    if report.errors and not force:
        raise BuildRejected(
            f"the gate reports {len(report.errors)} error(s) on {slug}. "
            "Fix them, or build again with force if you know better.")

    # Resolved before anything is written, because `write_artifacts` replaces
    # the snapshot this reads.
    baseline = decks.read_baseline(slug)
    previous = None
    if baseline is not None:
        with contextlib.suppress(*_UNPARSEABLE):
            previous = Deck.from_text(baseline, slug=slug)

    try:
        files = render_all(deck, cards=cards, previous=previous)
    except DraftDeck as exc:
        raise BuildRejected(str(exc)) from exc

    decks.write_artifacts(slug, files)
    out = _artifacts_json(decks, deck)
    out["issues"] = _issues(report)
    out["forced"] = bool(report.errors and force)
    return out
