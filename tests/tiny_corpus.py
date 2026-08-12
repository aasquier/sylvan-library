"""A four-card DuckDB corpus, for the tests that cannot run without one.

Most of this project's tests deliberately need no database -- the mana solver,
the gate and the simulator all take plain records, which is what keeps them
fast and what ADR 2 was protecting. Import is the exception: refusing to run
without the corpus is *the* rule it enforces (rule 1, never evaluate a card
from memory), so testing it against a fake would test the wrong thing.

Four cards is enough to cover every resolution rule: a legendary commander, a
colourless artifact, a basic land, and a banned card so the gate has something
real to catch on day one. A fifth was added with the power/toughness columns:
a double-faced card, because that is the one shape where Scryfall puts the
stats *and the mana cost* on the faces rather than on the card, and reading
only the top level is how 501 cards ended up in the corpus as free spells.
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
     "released_at": "2010-10-01", "set": "m11",
     "power": "6", "toughness": "6", "artist": "Aleksi Briclot",
     "flavor_text": "The land itself rises to his call."},
    # A double-faced card, shaped exactly as Scryfall serves one: no top-level
    # `mana_cost`, `power` or `toughness`, all of it on the faces. Reading only
    # the top level makes this a free 0/0, which is precisely the bug the
    # front-face fallback fixes.
    #
    # Every value here is copied from the real Scryfall row, including the
    # {G}{R} identity that a mono-green reading of the name would get wrong.
    # A fixture that names a real card and then lies about it is a fixture
    # that teaches the wrong thing -- rule 1 applies to test data too.
    {"oracle_id": "id-etali", "name": "Etali, Primal Conqueror // Etali, Primal "
                                      "Sickness",
     "cmc": 7,
     "type_line": "Legendary Creature — Elder Dinosaur // Legendary Creature "
                  "— Phyrexian Elder Dinosaur",
     "colors": ["R"], "color_identity": ["G", "R"],
     "keywords": ["Indestructible", "Transform", "Trample"],
     "produced_mana": [], "legalities": {"commander": "legal"},
     "layout": "transform", "reserved": False, "edhrec_rank": 790,
     "released_at": "2023-04-21", "set": "one",
     "game_changer": False,
     "card_faces": [
         {"name": "Etali, Primal Conqueror", "mana_cost": "{5}{R}{R}",
          "type_line": "Legendary Creature — Elder Dinosaur",
          "oracle_text": "When this creature enters, each player exiles cards "
                         "from the top of their library.",
          "power": "7", "toughness": "7", "artist": "Ryan Pancoast",
          "image_uris": {"normal": "https://img/etali.jpg",
                         "art_crop": "https://img/etali-art.jpg"}},
         {"name": "Etali, Primal Sickness", "mana_cost": "",
          "type_line": "Legendary Creature — Phyrexian Elder Dinosaur",
          "oracle_text": "Toxic 10. Trample.",
          "power": "11", "toughness": "11", "artist": "Ryan Pancoast",
          "flavor_text": "The contagion spreads and the Multiverse quakes."},
     ]},
    # The one card here on the official Game Changers list, so a bracket count
    # has something real to find.
    {"oracle_id": "id-tithe", "name": "Smothering Tithe", "mana_cost": "{3}{W}",
     "cmc": 4, "type_line": "Enchantment",
     "oracle_text": "Whenever an opponent draws a card, that player may pay "
                    "{2}. If they don't, you create a Treasure token.",
     "colors": ["W"], "color_identity": ["W"], "keywords": [],
     "produced_mana": [], "legalities": {"commander": "legal"},
     "layout": "normal", "reserved": False, "edhrec_rank": 20,
     "released_at": "2019-01-25", "set": "rna", "game_changer": True},
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
