"""Local card corpus: Scryfall bulk data in DuckDB.

Why local, and why DuckDB: "deep hits from the entire history of Magic" is a
query over ~35k oracle cards, not a recall exercise. Holding the whole corpus
locally turns colour-identity checks, legality checks, and best-in-slot
searches into things that are *verified* rather than remembered -- which is
exactly where card-evaluation mistakes come from.

Scryfall's bulk files are published daily and are explicitly licensed for this
use. We never hammer their API card-by-card; we pull one file a day.

Bulk types we care about:
  oracle_cards  (~35k rows, one per distinct card)  -- rules, identity, legality
  default_cards (~500k rows, one per printing)      -- prices, art, set codes
"""

from __future__ import annotations

import json
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Sequence

BULK_INDEX = "https://api.scryfall.com/bulk-data"
USER_AGENT = "mtg-lab/0.1 (local personal deckbuilding tool)"

SCHEMA = """
CREATE TABLE IF NOT EXISTS oracle_cards (
    oracle_id       VARCHAR PRIMARY KEY,
    name            VARCHAR,
    mana_cost       VARCHAR,
    cmc             DOUBLE,
    type_line       VARCHAR,
    oracle_text     VARCHAR,
    colors          VARCHAR[],
    color_identity  VARCHAR[],
    keywords        VARCHAR[],
    produced_mana   VARCHAR[],
    legalities      JSON,
    layout          VARCHAR,
    card_faces      JSON,
    reserved        BOOLEAN,
    edhrec_rank     INTEGER,
    released_at     DATE,
    set_code        VARCHAR,
    scryfall_uri    VARCHAR,
    image_normal    VARCHAR,
    image_art_crop  VARCHAR
);

CREATE TABLE IF NOT EXISTS printings (
    id              VARCHAR PRIMARY KEY,
    oracle_id       VARCHAR,
    name            VARCHAR,
    set_code        VARCHAR,
    set_name        VARCHAR,
    collector_number VARCHAR,
    rarity          VARCHAR,
    released_at     DATE,
    digital         BOOLEAN,
    promo           BOOLEAN,
    finishes        VARCHAR[],
    image_normal    VARCHAR,
    price_usd       DOUBLE,
    price_usd_foil  DOUBLE,
    price_eur       DOUBLE,
    tcg_product_id  VARCHAR
);

-- Append-only daily snapshots. This is what makes deal-watching possible:
-- "cheap right now" is meaningless without a baseline to compare against.
CREATE TABLE IF NOT EXISTS price_history (
    snapshot_date   DATE,
    printing_id     VARCHAR,
    oracle_id       VARCHAR,
    name            VARCHAR,
    price_usd       DOUBLE,
    price_usd_foil  DOUBLE,
    PRIMARY KEY (snapshot_date, printing_id)
);

CREATE INDEX IF NOT EXISTS idx_oracle_name ON oracle_cards(name);
CREATE INDEX IF NOT EXISTS idx_printings_oracle ON printings(oracle_id);
"""


def connect(db_path: str | Path = "data/mtg.duckdb"):
    import duckdb  # imported lazily so the sim core stays dependency-free

    path = Path(db_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    con = duckdb.connect(str(path))
    con.execute(SCHEMA)
    return con


def _fetch_json(url: str) -> Any:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT,
                                               "Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.load(resp)


def download_bulk(kind: str, dest_dir: str | Path = "data/scryfall") -> Path:
    """Download a Scryfall bulk file. `kind` is 'oracle_cards' or 'default_cards'.

    Returns the local path. Skips the download if the local copy is already
    at or newer than Scryfall's published `updated_at`.
    """
    index = _fetch_json(BULK_INDEX)
    entry = next((e for e in index["data"] if e["type"] == kind), None)
    if entry is None:
        raise ValueError(f"unknown bulk type {kind!r}")

    dest_dir = Path(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)
    stamp = entry["updated_at"][:10]
    target = dest_dir / f"{kind}-{stamp}.json"
    if target.exists():
        return target

    req = urllib.request.Request(entry["download_uri"],
                                 headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=600) as resp, target.open("wb") as fh:
        while chunk := resp.read(1 << 20):
            fh.write(chunk)
    return target


def _oracle_row(c: dict) -> tuple:
    img = c.get("image_uris") or {}
    if not img and c.get("card_faces"):
        img = (c["card_faces"][0].get("image_uris") or {})
    return (
        c.get("oracle_id"), c.get("name"), c.get("mana_cost"), c.get("cmc"),
        c.get("type_line"), c.get("oracle_text"), c.get("colors") or [],
        c.get("color_identity") or [], c.get("keywords") or [],
        c.get("produced_mana") or [], json.dumps(c.get("legalities") or {}),
        c.get("layout"), json.dumps(c.get("card_faces") or []),
        bool(c.get("reserved")), c.get("edhrec_rank"), c.get("released_at"),
        c.get("set"), c.get("scryfall_uri"),
        img.get("normal"), img.get("art_crop"),
    )


def _printing_row(c: dict) -> tuple:
    prices = c.get("prices") or {}
    img = c.get("image_uris") or {}
    if not img and c.get("card_faces"):
        img = (c["card_faces"][0].get("image_uris") or {})

    def f(key):
        v = prices.get(key)
        return float(v) if v else None

    return (
        c.get("id"), c.get("oracle_id"), c.get("name"), c.get("set"),
        c.get("set_name"), c.get("collector_number"), c.get("rarity"),
        c.get("released_at"), bool(c.get("digital")), bool(c.get("promo")),
        c.get("finishes") or [], img.get("normal"),
        f("usd"), f("usd_foil"), f("eur"),
        str((c.get("tcgplayer_id") or "")) or None,
    )


def _iter_cards(path: Path):
    """Stream a bulk file. These are single JSON arrays up to ~2GB, so we
    parse incrementally rather than loading the whole thing."""
    with path.open("r", encoding="utf-8") as fh:
        first = fh.read(1)
        if first != "[":
            fh.seek(0)
            yield from json.load(fh)
            return
        buf, depth, in_str, esc = "", 0, False, False
        while chunk := fh.read(1 << 20):
            for ch in chunk:
                if depth:
                    buf += ch
                if in_str:
                    if esc:
                        esc = False
                    elif ch == "\\":
                        esc = True
                    elif ch == '"':
                        in_str = False
                    continue
                if ch == '"':
                    in_str = True
                elif ch == "{":
                    if not depth:
                        buf = "{"
                    depth += 1
                elif ch == "}":
                    depth -= 1
                    if not depth:
                        yield json.loads(buf)
                        buf = ""


def load_oracle(con, path: Path, *, batch: int = 5000) -> int:
    con.execute("DELETE FROM oracle_cards")
    rows, total = [], 0
    for card in _iter_cards(path):
        if card.get("layout") in {"art_series", "token", "double_faced_token"}:
            continue
        rows.append(_oracle_row(card))
        if len(rows) >= batch:
            con.executemany(
                "INSERT OR REPLACE INTO oracle_cards VALUES "
                "(" + ",".join("?" * 20) + ")", rows)
            total += len(rows)
            rows = []
    if rows:
        con.executemany(
            "INSERT OR REPLACE INTO oracle_cards VALUES "
            "(" + ",".join("?" * 20) + ")", rows)
        total += len(rows)
    return total


def load_printings(con, path: Path, *, batch: int = 5000) -> int:
    con.execute("DELETE FROM printings")
    rows, total = [], 0
    for card in _iter_cards(path):
        if card.get("digital"):
            continue
        rows.append(_printing_row(card))
        if len(rows) >= batch:
            con.executemany(
                "INSERT OR REPLACE INTO printings VALUES "
                "(" + ",".join("?" * 16) + ")", rows)
            total += len(rows)
            rows = []
    if rows:
        con.executemany(
            "INSERT OR REPLACE INTO printings VALUES "
            "(" + ",".join("?" * 16) + ")", rows)
        total += len(rows)
    return total


def snapshot_prices(con, *, on_date: str | None = None) -> int:
    """Append today's prices to price_history. Run daily via cron/launchd."""
    date_expr = f"DATE '{on_date}'" if on_date else "CURRENT_DATE"
    con.execute(f"""
        INSERT OR REPLACE INTO price_history
        SELECT {date_expr}, id, oracle_id, name, price_usd, price_usd_foil
        FROM printings
        WHERE price_usd IS NOT NULL OR price_usd_foil IS NOT NULL
    """)
    return con.execute(
        f"SELECT count(*) FROM price_history WHERE snapshot_date = {date_expr}"
    ).fetchone()[0]


# ------------------------------------------------------------------ queries

@dataclass
class CardRecord:
    name: str
    mana_cost: str | None
    cmc: float
    type_line: str
    oracle_text: str
    color_identity: frozenset[str]
    produced_mana: tuple[str, ...]
    legal_commander: bool
    reserved: bool
    edhrec_rank: int | None
    image_normal: str | None

    @property
    def is_land(self) -> bool:
        return "Land" in (self.type_line or "")


def _to_record(row: Sequence[Any]) -> CardRecord:
    legalities = json.loads(row[8]) if row[8] else {}
    return CardRecord(
        name=row[0], mana_cost=row[1], cmc=row[2] or 0.0, type_line=row[3] or "",
        oracle_text=row[4] or "",
        color_identity=frozenset(row[5] or []),
        produced_mana=tuple(row[6] or []),
        legal_commander=legalities.get("commander") == "legal",
        reserved=bool(row[7]), edhrec_rank=row[9], image_normal=row[10],
    )


_SELECT = """SELECT name, mana_cost, cmc, type_line, oracle_text, color_identity,
                    produced_mana, reserved, legalities, edhrec_rank, image_normal
             FROM oracle_cards"""


def get_cards(con, names: Iterable[str]) -> dict[str, CardRecord]:
    """Look up many cards at once, case-insensitively. Missing names are simply
    absent from the result -- callers must handle that, loudly."""
    wanted = list(names)
    if not wanted:
        return {}
    placeholders = ",".join("?" * len(wanted))
    rows = con.execute(
        f"{_SELECT} WHERE lower(name) IN ({placeholders})",
        [n.lower() for n in wanted],
    ).fetchall()
    by_lower = {r[0].lower(): _to_record(r) for r in rows}
    return {n: by_lower[n.lower()] for n in wanted if n.lower() in by_lower}


def search(con, where: str, params: Sequence[Any] = (), limit: int = 100) -> list[CardRecord]:
    """Escape hatch for ad-hoc corpus queries, e.g.

        search(con, "oracle_text ILIKE ? AND list_contains(color_identity,'G')",
               ['%create a Food token%'])
    """
    rows = con.execute(f"{_SELECT} WHERE {where} LIMIT {int(limit)}", list(params)).fetchall()
    return [_to_record(r) for r in rows]
