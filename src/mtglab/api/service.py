"""Everything the API serves, as plain functions over plain data.

Routes stay thin on purpose: they parse arguments and call in here. That keeps
the interesting logic testable without HTTP, and keeps a single implementation
behind both the CLI and the app.
"""

from __future__ import annotations

import json
import re
import urllib.request
from datetime import date
from pathlib import Path
from typing import Any

from mtglab import config
from mtglab.cards import db
from mtglab.decks import decklist, edit, importer, suggest
from mtglab.decks.analyze import deck_stats
from mtglab.decks.edit import EditFailed
from mtglab.decks.importer import ImportRefused
from mtglab.decks.model import CATEGORIES, DECK_STATUSES, Deck
from mtglab.decks.source import DeckExists, DeckSource, FileDeckSource
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

    `config.DB_PATH` is read here rather than imported as a name, for the
    reason `config.py` exists: binding it at import time makes
    `config.use_paths()` silently ineffective, so a test can never point the
    service at a scratch corpus. `deck_paths` and `FileDeckSource` already
    resolve at call time; this was the last place that did not.
    """
    if not Path(config.DB_PATH).exists():
        return None
    try:
        import duckdb
        return duckdb.connect(str(config.DB_PATH), read_only=True)
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
            "status": deck.status,
            "stage": deck.stage,
            "needs_rationale": len(deck.unjustified),
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


class ImportRejected(Exception):
    """The import was refused, and nothing was written."""


# A slug becomes a directory name under `decks/`, so it is checked rather than
# trusted. The API takes it from a request body, and "sanitise it later" is how
# a path component turns into a path.
_SLUG = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")


def import_deck(*, text: str, slug: str, name: str = "",
                commander: list[str] | None = None, companion: str = "",
                bracket: int | None = None, status: str = "theoretical",
                dry_run: bool = False,
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
    decks = _source(source)
    if not decks.writable:
        raise ImportRejected("this library is read-only")

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
        # Without the corpus every name is unknown and no land is filed, so the
        # import would produce a deck whose facts were never checked -- the one
        # thing the gate exists to prevent. Refuse rather than half-do it.
        raise ImportRejected(
            "importing needs the card corpus -- run `mtglab data refresh`")
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

        gate = validate(report.deck, _corpus_for(report.deck, con))
        return {
            "slug": slug,
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


class EditRejected(Exception):
    """The edit was refused, and nothing was written."""


class SwapRejected(EditRejected):
    """The swap was refused, and nothing was written."""


def _issues(report) -> dict[str, list[dict[str, Any]]]:
    """The gate's verdict, flattened for JSON."""
    return {
        "errors": [{"code": i.code, "message": i.message, "card": i.card}
                   for i in report.errors],
        "warnings": [{"code": i.code, "message": i.message, "card": i.card}
                     for i in report.warnings],
    }


def _for_writing(source: DeckSource | None) -> DeckSource:
    decks = _source(source)
    if not decks.writable:
        raise EditRejected("this deck is read-only")
    return decks


def _identity_of(deck: Deck, con) -> frozenset[str]:
    """The commander's colour identity, from the corpus.

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


def _find_card(deck: Deck, name: str):
    wanted = name.strip().lower()
    return next((c for c in deck.cards + deck.swap_board
                 if c.name.lower() == wanted), None)


def _commit(slug: str, decks: DeckSource, updated: str,
            **extra: Any) -> dict[str, Any]:
    """Write an edited deck, and hand back the gate's verdict on the result.

    Every write goes out through here, so no caller can change a deck without
    being told what the change did to the gate. `stage` and `needs_rationale`
    ride along because an edit is the most likely moment for either to move --
    filling in the last blank `why` is what makes a draft promotable.
    """
    decks.write_text(slug, updated)
    after = decks.get(slug)
    con = _connect()
    try:
        report = validate(after, _corpus_for(after, con))
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
             source: DeckSource | None = None) -> dict[str, Any]:
    """Put a card into the 99 or onto the swap board.

    Checked against the corpus before anything is written -- the card has to
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
    decks = _for_writing(source)
    deck = decks.get(slug)

    con = _connect()
    if con is None:
        raise EditRejected("adding a card needs the card corpus -- "
                           "run `mtglab data refresh`")
    try:
        rec = db.get_cards(con, [name]).get(name)
        if rec is None:
            raise EditRejected(f"{name!r} is not a card the corpus knows")
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
        return _commit(slug, decks, updated, added=rec.name, category=category,
                       into=to)
    finally:
        con.close()


def remove_card(slug: str, *, name: str,
                source: DeckSource | None = None) -> dict[str, Any]:
    """Take a card out of the 99 or the swap board.

    Needs no corpus: removing a card is a fact about this deck file, not about
    Magic. That matters because it means a deck can still be pruned on a
    machine that has never run `data refresh`.
    """
    decks = _for_writing(source)
    entry = _find_card(decks.get(slug), name)
    if entry is None:
        raise EditRejected(f"{name!r} is not in this deck")
    try:
        updated = edit.remove_card(decks.read_text(slug), name=entry.name)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, removed=entry.name)


def set_card_field(slug: str, *, name: str, field: str, value: Any,
                   source: DeckSource | None = None) -> dict[str, Any]:
    """Change one card's category, quantity, or rationale.

    This is the write path behind the rationale editor, and the answer to the
    gap `decks import` opened: a 99-card draft arrives owing 99 rationales, and
    until now the only way to write one was a text editor.

    The rationale is the caller's text, written as given. Nothing here -- and
    nothing in the layer below -- composes, tidies, expands or infers one. That
    is rule 4, and [ADR 12](docs/adr/0012) rule 3 is where it binds a tool
    rather than a person.
    """
    decks = _for_writing(source)
    entry = _find_card(decks.get(slug), name)
    if entry is None:
        raise EditRejected(f"{name!r} is not in this deck")
    if field == "category":
        value = _check_category(str(value))
    try:
        updated = edit.set_card_field(decks.read_text(slug), name=entry.name,
                                      field=field, value=value)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, card=entry.name, field=field)


def set_note(slug: str, *, key: str, value: str,
             source: DeckSource | None = None) -> dict[str, Any]:
    """Set one deck-level note -- the prose the advanced primer reads directly.

    Notes are the deck's thinking, and they live in the source of truth rather
    than in an artifact precisely so that regenerating the five deliverables
    cannot lose them.
    """
    decks = _for_writing(source)
    decks.get(slug)                     # 404 before anything else
    try:
        updated = edit.set_note(decks.read_text(slug), key=key, value=value)
    except EditFailed as exc:
        raise EditRejected(str(exc)) from exc
    return _commit(slug, decks, updated, note=key.strip())


def swap_card(slug: str, *, out: str, into: str, why: str,
              source: DeckSource | None = None) -> dict[str, Any]:
    """Replace one card in a deck with another, and re-run the gate.

    This is not the auto-substitution ADR 8 rejected. The judgement stays with
    the caller: they name the card going in and they write the rationale. What
    changed is that carrying out a decision they already made no longer means
    hand-editing YAML.

    Everything is checked before anything is written, and the edit itself is
    surgical -- see `decks/edit.py` for why a load-and-dump was not an option.
    """
    try:
        decks = _for_writing(source)
    except EditRejected as exc:
        raise SwapRejected(str(exc)) from exc
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
        raise SwapRejected("swapping needs the card corpus -- run `mtglab data refresh`")
    try:
        found = db.get_cards(con, [into])
        rec = found.get(into)
        if rec is None:
            raise SwapRejected(f"{into!r} is not a card the corpus knows")
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

    return _commit(slug, decks, updated, swapped_out=entry.name,
                   swapped_in=rec.name, why=why.strip())


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
