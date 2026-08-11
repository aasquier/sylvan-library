"""Everything the API serves, as plain functions over plain data.

Routes stay thin on purpose: they parse arguments and call in here. That keeps
the interesting logic testable without HTTP, and keeps a single implementation
behind both the CLI and the app.
"""

from __future__ import annotations

import json
import urllib.request
from datetime import date
from pathlib import Path
from typing import Any

from mtglab.cards import db
from mtglab.config import DB_PATH
from mtglab.decks import suggest
from mtglab.decks.analyze import deck_stats
from mtglab.decks.model import Deck
from mtglab.decks.source import DeckSource, FileDeckSource
from mtglab.decks.validate import validate

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


# ------------------------------------------------------------------- corpus

def _connect():
    """A read-only handle, or None when the corpus has not been built yet.

    Read-only matters: DuckDB allows one writer, so a running `data refresh`
    would otherwise lock the whole app out.
    """
    if not Path(DB_PATH).exists():
        return None
    try:
        import duckdb
        return duckdb.connect(str(DB_PATH), read_only=True)
    except Exception:                                               # noqa: BLE001
        # Most likely a `data refresh` holding the write lock. The app stays
        # usable in degraded form rather than failing every request.
        return None


def health(*, source: DeckSource | None = None) -> dict[str, Any]:
    con = _connect()
    if con is None:
        return {"corpus": False, "oracle_cards": 0, "printings": 0,
                "message": "no corpus yet -- run `mtglab data refresh`"}
    try:
        oracle = con.execute("SELECT count(*) FROM oracle_cards").fetchone()[0]
        printings = con.execute("SELECT count(*) FROM printings").fetchone()[0]
    finally:
        con.close()
    files = sorted(Path("data/scryfall").glob("*.jsonl.gz")) if \
        Path("data/scryfall").exists() else []
    return {
        "corpus": True,
        "oracle_cards": oracle,
        "printings": printings,
        "bulk_files": [f.name for f in files],
        "decks": len(_source(source).slugs()),
    }


# -------------------------------------------------------------------- decks


def _corpus_for(deck: Deck, con) -> dict:
    if con is None:
        return {}
    names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    if deck.companion:
        names.append(deck.companion)
    return db.get_cards(con, names)


def _card_json(entry, rec) -> dict[str, Any]:
    """One row of the 99, merged with whatever the corpus knows about it."""
    out: dict[str, Any] = {
        "name": entry.name,
        "category": entry.category,
        "why": entry.why,
        "qty": entry.qty,
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
        })
    return out


def list_decks(*, source: DeckSource | None = None) -> list[dict[str, Any]]:
    """The library view. Includes the commander's art so the UI has a hero
    image without a second round trip per deck.

    Also carries the gate's error and warning counts. The gate is the point of
    this project, so a shelf that renders a deck with a banned card exactly
    like a clean one is hiding the only thing it is really for -- and asking
    the UI to fetch /validate per deck would be an N+1 on every page load.
    `errors` is None when the corpus is unavailable, which is different from
    zero and must not render as a pass.
    """
    con = _connect()
    try:
        out = []
        for deck in _source(source).all():
            art = None
            identity: list[str] = []
            errors = warnings = None
            if con is not None:
                names = deck.card_names() + [c.name for c in deck.swap_board]
                if deck.companion:
                    names.append(deck.companion)
                cards = db.get_cards(con, sorted(set(names)))
                rep = validate(deck, cards)
                errors, warnings = len(rep.errors), len(rep.warnings)
                rec = cards.get(deck.commander[0]) if deck.commander else None
                if rec is not None:
                    art = getattr(rec, "image_art_crop", None) or rec.image_normal
                    identity = sorted(rec.color_identity)
            out.append({
                "slug": deck.slug,
                "name": deck.name,
                "commander": deck.commander,
                "companion": deck.companion,
                "bracket": deck.bracket,
                "total_cards": deck.total_cards,
                "land_count": deck.land_count,
                "strategy": deck.strategy,
                "art_crop": art,
                "color_identity": identity,
                "errors": errors,
                "warnings": warnings,
            })
        return out
    finally:
        if con is not None:
            con.close()


def get_deck(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        cards = _corpus_for(deck, con)
        commander_rec = cards.get(deck.commander[0]) if deck.commander else None
        return {
            "slug": deck.slug,
            "name": deck.name,
            "commander": deck.commander,
            "companion": deck.companion,
            "bracket": deck.bracket,
            "strategy": deck.strategy,
            "notes": deck.notes,
            "total_cards": deck.total_cards,
            "land_count": deck.land_count,
            "color_identity": sorted(commander_rec.color_identity) if commander_rec else [],
            "commander_card": _card_json(
                type("E", (), {"name": deck.commander[0], "category": "commander",
                               "why": deck.notes.get("commander_why", ""), "qty": 1})(),
                commander_rec) if commander_rec else None,
            "cards": [_card_json(e, cards.get(e.name)) for e in deck.cards],
            "swap_board": [_card_json(e, cards.get(e.name)) for e in deck.swap_board],
            "corpus_available": con is not None,
        }
    finally:
        if con is not None:
            con.close()


def validate_deck(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        cards = _corpus_for(deck, con) if con is not None else None
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
        return {"slug": slug, "corpus_available": False, "targets": []}
    try:
        cards = _corpus_for(deck, con)
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
        return {"slug": slug, "corpus_available": True, "targets": targets}
    finally:
        con.close()


def stats_for(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    deck = _source(source).get(slug)
    con = _connect()
    try:
        stats = deck_stats(deck, _corpus_for(deck, con))
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

def search_cards(*, q: str = "", identity: str = "", type_line: str = "",
                 cmc_max: float | None = None, price_max: float | None = None,
                 sort: str = "edhrec", limit: int = 60) -> dict[str, Any]:
    """Corpus search: the 'deep hits from the whole history' tool.

    `identity` is a subset filter, not an exact match -- passing "BG" returns
    every card legal in a Golgari deck, which includes colorless and mono
    cards. That is the question a deckbuilder actually asks.
    """
    con = _connect()
    if con is None:
        return {"cards": [], "total": 0,
                "message": "no corpus yet -- run `mtglab data refresh`"}
    try:
        where = ["json_extract_string(legalities, 'commander') = 'legal'"]
        params: list[Any] = []

        if identity:
            allowed = [c for c in identity.upper() if c in "WUBRG"]
            listed = ", ".join(f"'{c}'" for c in allowed) or "''"
            where.append(
                f"len(list_filter(color_identity, x -> x NOT IN ({listed}))) = 0")
        if q:
            where.append("(name ILIKE ? OR oracle_text ILIKE ?)")
            params += [f"%{q}%", f"%{q}%"]
        if type_line:
            where.append("type_line ILIKE ?")
            params.append(f"%{type_line}%")
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
        return {"cards": cards, "total": len(cards)}
    finally:
        con.close()


# --------------------------------------------------------------------- sets

_SETS_CACHE: dict[str, Any] = {}


def upcoming_sets(*, force: bool = False) -> dict[str, Any]:
    """Unreleased sets, live from Scryfall, cached for the process lifetime.

    This is the one route that reaches the network on demand. Spoiler scanning
    is meaningless against a corpus that by definition does not have the cards
    yet, so the set list has to come from upstream.
    """
    today = date.today().isoformat()
    if not force and _SETS_CACHE.get("day") == today:
        return _SETS_CACHE["data"]

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
