"""A small DuckDB pool, holding the slice of the schema cardmotion reads.

The app's `tests/tiny_pool.py` built its 21-card fixture through the real
Scryfall ingest, because the ingest was part of what it tested. This toolbox
has no ingest -- `cardmotion/pool.py` opens read-only on purpose -- so the
fixture here creates exactly the two table slices the motion derivation
queries and nothing else:

    oracle_cards (oracle_id, name, artist, image_art_crop, scryfall_uri)
    printings    (id, name, image_normal, artist)

**Every value here was read out of the real pool, never typed from memory** --
the rule the original fixture states, inherited with its rows: Gyome's oracle
row and c21 printing are pasted verbatim from the app's fixture, which pasted
them from the pool. Rule 1 applies to test data too; a fixture that names a
real card and then lies about it teaches the wrong thing.

`connect` here is read-write, unlike the toolbox's own `pool.connect`, and
that is the fixture's whole difference in stance: tests *build* pools (and
insert printings mid-test), the toolbox only ever reads one. The tests that
prove the read-only posture live in `test_pool.py`.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

SCHEMA = """
CREATE TABLE IF NOT EXISTS oracle_cards (
    oracle_id       VARCHAR PRIMARY KEY,
    name            VARCHAR,
    artist          VARCHAR,
    image_art_crop  VARCHAR,
    scryfall_uri    VARCHAR
);

CREATE TABLE IF NOT EXISTS printings (
    id              VARCHAR PRIMARY KEY,
    name            VARCHAR,
    image_normal    VARCHAR,
    artist          VARCHAR
);
"""

#: Gyome's oracle row, verbatim from the app's fixture. The one card the
#: motion tests derive; `scryfall_uri` is NULL exactly as the original
#: fixture left it (its Scryfall record was trimmed before the column).
GYOME = (
    "0f1b5cea-2035-444c-9f63-87dcb9782c74",
    "Gyome, Master Chef",
    "Steve Prescott",
    ("https://cards.scryfall.io/art_crop/front/8/2/"
     "8279d421-dd86-49d1-93f7-65f6046c542d.jpg?1783903751"),
    None,
)

#: Gyome's Commander 2021 printing, verbatim from the app's fixture -- so
#: the printings table is born non-empty and an id lookup that should miss
#: is missing among real rows rather than in a vacuum.
GYOME_C21 = (
    "5dd7dd1a-6dd1-43c3-8298-7db703d384a1",
    "Gyome, Master Chef",
    ("https://cards.scryfall.io/normal/front/5/d/"
     "5dd7dd1a-6dd1-43c3-8298-7db703d384a1.jpg?1783927614"),
    "Steve Prescott",
)


def connect(db_path: str | Path) -> Any:
    """A read-write handle on a fixture pool. Tests only; see the docstring."""
    import duckdb

    return duckdb.connect(str(db_path))


def build(db_path: Path) -> Path:
    """Create a card pool at `db_path`. Returns it, so callers can chain."""
    db_path = Path(db_path)
    db_path.parent.mkdir(parents=True, exist_ok=True)
    con = connect(db_path)
    try:
        con.execute(SCHEMA)
        con.execute("INSERT INTO oracle_cards VALUES (?, ?, ?, ?, ?)",
                    list(GYOME))
        con.execute("INSERT INTO printings VALUES (?, ?, ?, ?)",
                    list(GYOME_C21))
    finally:
        con.close()
    return db_path
