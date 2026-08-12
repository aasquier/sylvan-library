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

import gzip
import json
import urllib.request
from collections.abc import Iterable, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, TypeAlias

#: A DuckDB connection.
#:
#: `Any`, and deliberately so: duckdb ships no type stubs, and it is imported
#: lazily inside `connect()` precisely so the simulation core stays free of it
#: (CLAUDE.md's dependency rule). An alias rather than a bare `Any` at each of
#: the two dozen call sites, so the intent reads as "the checker cannot see
#: into this library" rather than "nobody got round to annotating this" -- and
#: so there is one place to narrow if duckdb ever publishes stubs.
Connection: TypeAlias = Any

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
    image_art_crop  VARCHAR,
    -- Text, not numbers. Scryfall gives "*", "1+*", "7-*" and similar for
    -- cards whose stats are computed, and coercing those to integers either
    -- throws or silently invents a number. A caller that wants arithmetic can
    -- ask for it; a caller that wants to know what is printed on the card gets
    -- what is printed on the card.
    power           VARCHAR,
    toughness       VARCHAR,
    loyalty         VARCHAR,
    defense         VARCHAR,
    -- The official Commander bracket criterion, and the one thing a bracket
    -- declaration could never be checked against before. 53 cards carry it.
    game_changer    BOOLEAN,
    flavor_text     VARCHAR,
    artist          VARCHAR
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


#: Columns added to `oracle_cards` after the table first shipped. `CREATE TABLE
#: IF NOT EXISTS` does nothing to a database that already has the table, so an
#: existing corpus would keep the old shape and every query naming a new column
#: would fail. Adding them here makes an old database *readable*; it does not
#: make it *correct*, because the values only arrive on the next ingest --
#: which is why `corpus_is_stale` exists rather than leaving a silently
#: all-NULL column to be mistaken for "no card has power".
_ADDED_COLUMNS = (
    ("power", "VARCHAR"), ("toughness", "VARCHAR"), ("loyalty", "VARCHAR"),
    ("defense", "VARCHAR"), ("game_changer", "BOOLEAN"),
    ("flavor_text", "VARCHAR"), ("artist", "VARCHAR"),
)


def connect(db_path: str | Path = "data/mtg.duckdb") -> Connection:
    import duckdb  # imported lazily so the sim core stays dependency-free

    path = Path(db_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    con = duckdb.connect(str(path))
    con.execute(SCHEMA)
    for name, sql_type in _ADDED_COLUMNS:
        con.execute(f"ALTER TABLE oracle_cards ADD COLUMN IF NOT EXISTS "
                    f"{name} {sql_type}")
    return con


def oracle_columns(con) -> set[str]:
    """The columns `oracle_cards` actually has on this connection.

    Needed because the migration in `connect()` cannot always run: the API
    opens the corpus **read-only** so a `data refresh` cannot lock the app out,
    and `ALTER TABLE` on a read-only handle fails. Without this, pulling a
    schema change would break every card query for anyone with an existing
    database — not on their next ingest, immediately.
    """
    rows = con.execute(
        "SELECT column_name FROM information_schema.columns "
        "WHERE table_name = 'oracle_cards'").fetchall()
    return {r[0] for r in rows}


def corpus_is_stale(con) -> bool:
    """Does this corpus predate the power/toughness columns?

    Worth a named question rather than a silent NULL. A database loaded before
    those columns existed answers every query about them with NULL, which reads
    exactly like "this card has no power" — the quiet wrong answer rule 1
    exists to prevent, arriving through the very field added to prevent it.
    `health()` reports this so the app can say "re-ingest" instead of showing
    every creature as statless.

    Cheap: one scan short-circuited by LIMIT, and creatures are over half the
    corpus, so a current database answers immediately.
    """
    if not con.execute("SELECT 1 FROM oracle_cards LIMIT 1").fetchall():
        # An empty corpus is not a stale one — there is nothing to be wrong
        # about, and `health()` already reports the corpus as missing.
        return False
    if "power" not in oracle_columns(con):
        return True
    return not con.execute(
        "SELECT 1 FROM oracle_cards WHERE power IS NOT NULL LIMIT 1").fetchall()


def _fetch_json(url: str) -> Any:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT,
                                               "Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.load(resp)


def bulk_download_url(entry: dict) -> str:
    """The download URL for a bulk-data index entry.

    Scryfall retired the plain-JSON `download_uri` in favour of gzipped JSONL
    under `jsonl_download_uri`. Prefer the current key, fall back to the old
    one so an archived index still loads, and fail loudly rather than with a
    bare KeyError if both ever disappear.
    """
    url = entry.get("jsonl_download_uri") or entry.get("download_uri")
    if not url:
        raise ValueError(
            f"bulk entry {entry.get('type')!r} has no download URL; Scryfall's "
            f"index format may have changed again (keys: {sorted(entry)})")
    return url


def download_bulk(kind: str, dest_dir: str | Path = "data/scryfall") -> Path:
    """Download a Scryfall bulk file. `kind` is 'oracle_cards' or 'default_cards'.

    Returns the local path. Skips the download if the local copy is already
    at or newer than Scryfall's published `updated_at`.

    The file is stored exactly as served, compression included -- `_iter_cards`
    decompresses on the fly. `default_cards` is ~2GB expanded but well under
    500MB gzipped, and it is only ever read as a stream.
    """
    index = _fetch_json(BULK_INDEX)
    entry = next((e for e in index["data"] if e["type"] == kind), None)
    if entry is None:
        raise ValueError(f"unknown bulk type {kind!r}")

    url = bulk_download_url(entry)
    # Preserve the served extension so the reader can dispatch on it.
    suffix = ".jsonl.gz" if url.endswith(".jsonl.gz") else \
        ".jsonl" if url.endswith(".jsonl") else \
        ".json.gz" if url.endswith(".json.gz") else ".json"

    dest_dir = Path(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)
    stamp = entry["updated_at"][:10]
    target = dest_dir / f"{kind}-{stamp}{suffix}"
    if target.exists():
        return target

    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    tmp = target.with_suffix(target.suffix + ".part")
    with urllib.request.urlopen(req, timeout=600) as resp, tmp.open("wb") as fh:
        while chunk := resp.read(1 << 20):
            fh.write(chunk)
    # Rename only once complete, so an interrupted download is never mistaken
    # for a valid cached copy on the next run.
    tmp.replace(target)
    return target


#: Fields Scryfall puts on the *faces* of a double-faced card rather than on
#: the card itself. Reading only the top level leaves them NULL, and NULL is
#: indistinguishable from "this card has none".
_FRONT_FACE_FIELDS = ("mana_cost", "power", "toughness", "loyalty", "defense",
                      "flavor_text", "artist")


def _front(c: dict, field: str):
    """A field from the card, falling back to its front face.

    **This is a correctness fix, not tidiness.** `mana_cost` is absent at the
    top level for all 501 double-faced cards, so the corpus recorded them as
    NULL, `parse_mana_cost(None)` returns a cost of `{0}`, and the Tier 1
    compiler handed every one of them to the simulator as a *free spell*. Etali
    is a seven-drop; it was being cast on turn one. Two of the six decks were
    affected.

    The front face is the right answer rather than a convenient one: it is the
    face you cast from hand. A transforming permanent's back arrives by
    flipping something already on the battlefield, and a modal DFC's back is a
    separate choice the simulator does not model. Both faces remain available
    in full through the `card_faces` JSON, which is stored unchanged.
    """
    value = c.get(field)
    if value is not None:
        return value
    faces = c.get("card_faces") or []
    return faces[0].get(field) if faces else None


def _oracle_row(c: dict) -> tuple:
    img = c.get("image_uris") or {}
    if not img and c.get("card_faces"):
        img = (c["card_faces"][0].get("image_uris") or {})
    front = {f: _front(c, f) for f in _FRONT_FACE_FIELDS}
    return (
        c.get("oracle_id"), c.get("name"), front["mana_cost"], c.get("cmc"),
        c.get("type_line"), c.get("oracle_text"), c.get("colors") or [],
        c.get("color_identity") or [], c.get("keywords") or [],
        c.get("produced_mana") or [], json.dumps(c.get("legalities") or {}),
        c.get("layout"), json.dumps(c.get("card_faces") or []),
        bool(c.get("reserved")), c.get("edhrec_rank"), c.get("released_at"),
        c.get("set"), c.get("scryfall_uri"),
        img.get("normal"), img.get("art_crop"),
        front["power"], front["toughness"], front["loyalty"], front["defense"],
        # Scryfall omits this on older rows rather than sending false, and an
        # absent game changer is not a game changer.
        bool(c.get("game_changer")),
        front["flavor_text"], front["artist"],
    )


#: Named rather than positional, so adding a column is one edit here instead of
#: a silent off-by-one between the tuple and the placeholder count.
_ORACLE_COLUMNS = (
    "oracle_id", "name", "mana_cost", "cmc", "type_line", "oracle_text",
    "colors", "color_identity", "keywords", "produced_mana", "legalities",
    "layout", "card_faces", "reserved", "edhrec_rank", "released_at",
    "set_code", "scryfall_uri", "image_normal", "image_art_crop",
    "power", "toughness", "loyalty", "defense", "game_changer",
    "flavor_text", "artist",
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
        str(c.get("tcgplayer_id") or "") or None,
    )


def _iter_json_array(fh):
    """Yield objects from a JSON array whose opening '[' is already consumed.

    Kept for the legacy `download_uri` files, which were single arrays up to
    ~2GB -- too big to hand to json.load().
    """
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


def _iter_cards(path: Path):
    """Stream a bulk file, one card object at a time.

    Handles both formats Scryfall has served: newline-delimited JSONL (current)
    and a single JSON array (legacy). Either may be gzipped. Dispatch is on the
    first non-whitespace byte rather than the filename, so a file that was
    decompressed or renamed by hand still reads correctly.
    """
    opener = gzip.open if path.name.endswith(".gz") else open
    with opener(path, "rt", encoding="utf-8") as fh:
        first = fh.read(1)
        while first and first.isspace():
            first = fh.read(1)
        if not first:
            return
        if first == "[":
            yield from _iter_json_array(fh)
            return
        # JSONL: one complete card object per line. The character already
        # consumed belongs to the first record, so put it back by hand.
        head = first + fh.readline()
        if head.strip():
            yield json.loads(head)
        for line in fh:
            if line.strip():
                yield json.loads(line)


def load_oracle(con, path: Path, *, batch: int = 5000) -> int:
    insert = (f"INSERT OR REPLACE INTO oracle_cards "
              f"({', '.join(_ORACLE_COLUMNS)}) VALUES "
              f"({','.join('?' * len(_ORACLE_COLUMNS))})")
    con.execute("DELETE FROM oracle_cards")
    rows, total = [], 0
    for card in _iter_cards(path):
        if card.get("layout") in {"art_series", "token", "double_faced_token"}:
            continue
        rows.append(_oracle_row(card))
        if len(rows) >= batch:
            con.executemany(insert, rows)
            total += len(rows)
            rows = []
    if rows:
        con.executemany(insert, rows)
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
    # Cropped art only, no frame or text box. The UI uses it for hero images,
    # where a full card scan reads as clutter.
    image_art_crop: str | None = None
    # Scryfall's layout. Needed to tell a modal DFC (either face is playable)
    # apart from a transforming permanent (you cast the front; the back only
    # ever arrives by flipping).
    layout: str = "normal"
    # Scryfall's own keyword list (Trample, Flying, Landfall...). Authoritative,
    # and far cheaper than trying to recover keywords from oracle text.
    keywords: tuple[str, ...] = ()
    # Printed stats, as strings, because "*" and "1+*" are real values. A deck
    # that claims a creature is a 6/6 can now be checked against the card
    # instead of against somebody's memory of it -- which is how this got
    # added: the rationale interview asked about a "6/6 trample" it had no way
    # to verify. For a double-faced card these are the *front* face's.
    power: str | None = None
    toughness: str | None = None
    loyalty: str | None = None
    defense: str | None = None
    # Scryfall's flag for the official Commander Game Changers list. The one
    # bracket rule that was previously undecidable: a bracket is declared in
    # the deck file, and nothing could count what it permits.
    game_changer: bool = False
    flavor_text: str | None = None
    artist: str | None = None

    @property
    def is_creature(self) -> bool:
        """Read off the type line's front face, matching `power`.

        A creature with no printed power is possible (a back face only), so
        this asks the type line rather than inferring from the stats.
        """
        return "Creature" in (self.type_line or "").split(" // ")[0]

    @property
    def is_land(self) -> bool:
        """Is this card a land you can actually put onto the battlefield as a
        land drop?

        Testing `"Land" in type_line` was wrong for every double-faced card
        whose *back* is a land. Scryfall reports a combined type line, so
        Ojer Taq ("Legendary Creature — God // Land"), Growing Rites of Itlimoc
        and Welcome to . . . all read as lands -- and the Tier 1 compiler used
        this to decide what a land is, so those decks simulated with two or
        three phantom extra lands.

        The distinction is the layout. A modal DFC lets you choose which face
        to play, so a land back face is a real land drop. A transforming
        permanent does not: you cast the front face, and the back arrives only
        by flipping something already on the battlefield.
        """
        faces = (self.type_line or "").split(" // ")
        if "Land" in faces[0]:
            return True
        return self.layout == "modal_dfc" and any("Land" in f for f in faces[1:])


def _to_record(row: Sequence[Any]) -> CardRecord:
    legalities = json.loads(row[8]) if row[8] else {}

    def at(i: int, default=None):
        """Positional read that tolerates a shorter row.

        The existing style here, kept: `search()` and `get_cards()` share
        `_SELECT`, but a caller with its own query may hand over fewer columns.
        """
        return row[i] if len(row) > i else default

    return CardRecord(
        name=row[0], mana_cost=row[1], cmc=row[2] or 0.0, type_line=row[3] or "",
        oracle_text=row[4] or "",
        color_identity=frozenset(row[5] or []),
        produced_mana=tuple(row[6] or []),
        legal_commander=legalities.get("commander") == "legal",
        reserved=bool(row[7]), edhrec_rank=row[9], image_normal=row[10],
        image_art_crop=at(11),
        layout=at(12) or "normal",
        keywords=tuple(at(13) or ()),
        power=at(14), toughness=at(15), loyalty=at(16), defense=at(17),
        game_changer=bool(at(18, False)),
        flavor_text=at(19), artist=at(20),
    )


#: The columns `_to_record` reads, in the order it reads them.
_READ_COLUMNS = (
    "name", "mana_cost", "cmc", "type_line", "oracle_text", "color_identity",
    "produced_mana", "reserved", "legalities", "edhrec_rank",
    "image_normal", "image_art_crop", "layout", "keywords",
    "power", "toughness", "loyalty", "defense", "game_changer",
    "flavor_text", "artist",
)


def _select(con) -> str:
    """The SELECT clause, with any column this database lacks filled as NULL.

    The alternative — a fixed column list — turns a schema change into an
    immediate outage for every existing corpus, because the read-only handle
    the API uses cannot migrate itself. Filling with NULL degrades to "we do
    not know this card's power", which is true, and `corpus_is_stale` is what
    stops that from being mistaken for "this card has no power".
    """
    have = oracle_columns(con)
    cols = ", ".join(c if c in have else f"NULL AS {c}" for c in _READ_COLUMNS)
    return f"SELECT {cols} FROM oracle_cards"


def get_cards(con, names: Iterable[str]) -> dict[str, CardRecord]:
    """Look up many cards at once, case-insensitively. Missing names are simply
    absent from the result -- callers must handle that, loudly.

    Double-faced cards resolve by face name as well as by Scryfall's combined
    `Front // Back` name. Decklists everywhere -- Moxfield, Archidekt, and
    people -- write "Darkbore Pathway", not "Darkbore Pathway // Slitherbore
    Pathway", and rejecting those as unknown cards is a false negative on a
    perfectly legal card.

    The record returned is always the WHOLE card, so `color_identity` covers
    every face. That is deliberate and load-bearing: Ajani, Nacatl Pariah is
    a white front with a red back, and looking it up by its front face must
    still report identity {R}{W}.
    """
    wanted = list(names)
    if not wanted:
        return {}
    lowered = [n.lower() for n in wanted]
    placeholders = ",".join("?" * len(wanted))
    rows = con.execute(
        f"""{_select(con)} WHERE lower(name) IN ({placeholders})
               OR lower(split_part(name, ' // ', 1)) IN ({placeholders})
               OR lower(split_part(name, ' // ', 2)) IN ({placeholders})""",
        lowered * 3,
    ).fetchall()

    # Exact full-name matches win; a face-name match only fills a gap. Without
    # that precedence a card whose full name equals another card's face name
    # could shadow the real one.
    by_lower: dict[str, CardRecord] = {}
    faces: dict[str, CardRecord] = {}
    for r in rows:
        rec = _to_record(r)
        by_lower[r[0].lower()] = rec
        for face in r[0].lower().split(" // "):
            faces.setdefault(face, rec)

    out: dict[str, CardRecord] = {}
    for name, low in zip(wanted, lowered, strict=True):
        # Named apart from the `rec` above: reusing that binding made this an
        # optional assignment to a non-optional variable, which is exactly the
        # shape a type checker should object to even though the guard below
        # makes it safe today.
        found = by_lower.get(low) or faces.get(low)
        if found is not None:
            out[name] = found
    return out


def search(con, where: str, params: Sequence[Any] = (), limit: int = 100,
           order_by: str | None = None) -> list[CardRecord]:
    """Escape hatch for ad-hoc corpus queries, e.g.

        search(con, "oracle_text ILIKE ? AND list_contains(color_identity,'G')",
               ['%create a Food token%'])

    `order_by` matters more than it looks: without it a LIMIT returns an
    arbitrary slice of the matches, which is fine for "show me some" and wrong
    for anything that then ranks what it got back.
    """
    ordering = f" ORDER BY {order_by}" if order_by else ""
    rows = con.execute(
        f"{_select(con)} WHERE {where}{ordering} LIMIT {int(limit)}",
        list(params),
    ).fetchall()
    return [_to_record(r) for r in rows]
