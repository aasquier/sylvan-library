"""A four-card DuckDB corpus, for the tests that cannot run without one.

Most of this project's tests deliberately need no database -- the mana solver,
the gate and the simulator all take plain records, which is what keeps them
fast and what ADR 2 was protecting. Import is the exception: refusing to run
without the corpus is *the* rule it enforces (rule 1, never evaluate a card
from memory), so testing it against a fake would test the wrong thing.

Four cards is enough to cover every resolution rule: a legendary commander, a
colourless artifact, a basic land, and a banned card so the gate has something
real to catch on day one.
"""

from __future__ import annotations

import json
from pathlib import Path

CARDS = [
    {"oracle_id": "id-gyome", "name": "Gyome, Master Chef",
     "mana_cost": "{2}{B}{G}", "cmc": 4,
     "type_line": "Legendary Creature — Troll Peasant",
     "oracle_text": "Other creatures you control have ward {1}.",
     "colors": ["B", "G"], "color_identity": ["B", "G"], "keywords": ["Ward"],
     "produced_mana": [], "legalities": {"commander": "legal"},
     "layout": "normal", "reserved": False, "edhrec_rank": 500,
     "released_at": "2022-06-10", "set": "clb",
     "image_uris": {"normal": "https://img/gyome.jpg",
                    "art_crop": "https://img/gyome-art.jpg"}},
    {"oracle_id": "id-sol", "name": "Sol Ring", "mana_cost": "{1}", "cmc": 1,
     "type_line": "Artifact", "oracle_text": "{T}: Add {C}{C}.",
     "colors": [], "color_identity": [], "keywords": [],
     "produced_mana": ["C"], "legalities": {"commander": "legal"},
     "layout": "normal", "reserved": False, "edhrec_rank": 1,
     "released_at": "1993-08-05", "set": "lea"},
    {"oracle_id": "id-swamp", "name": "Swamp", "mana_cost": None, "cmc": 0,
     "type_line": "Basic Land — Swamp", "oracle_text": "({T}: Add {B}.)",
     "colors": [], "color_identity": [], "keywords": [],
     "produced_mana": ["B"], "legalities": {"commander": "legal"},
     "layout": "normal", "reserved": False, "edhrec_rank": None,
     "released_at": "1993-08-05", "set": "lea"},
    {"oracle_id": "id-titan", "name": "Primeval Titan", "mana_cost": "{4}{G}{G}",
     "cmc": 6, "type_line": "Creature — Giant",
     "oracle_text": "Trample. Whenever this creature enters or attacks, search "
                    "your library for up to two land cards.",
     "colors": ["G"], "color_identity": ["G"], "keywords": ["Trample"],
     "produced_mana": [], "legalities": {"commander": "banned"},
     "layout": "normal", "reserved": False, "edhrec_rank": 300,
     "released_at": "2010-10-01", "set": "m11"},
]


def build(db_path: Path) -> Path:
    """Create a corpus at `db_path`. Returns it, so callers can chain."""
    from mtglab.cards.db import connect, load_oracle

    jsonl = Path(db_path).parent / "tiny-oracle.jsonl"
    jsonl.parent.mkdir(parents=True, exist_ok=True)
    jsonl.write_text("\n".join(json.dumps(c) for c in CARDS), encoding="utf-8")

    con = connect(db_path)
    try:
        load_oracle(con, jsonl)
    finally:
        con.close()
    return Path(db_path)


# A 99 built from the four cards above: one Sol Ring, one banned Titan, and
# basics for the rest. Basics are exempt from the singleton rule, so this is a
# legally sized deck that fails the gate on exactly one thing.
DECKLIST = """\
Commander
1 Gyome, Master Chef

Deck
1 Sol Ring
1 Primeval Titan
97 Swamp
"""
