"""The Wheel of Fortune (punch list 2026-08-15 item 9).

Daniel Gelon's Alpha painting: a red-cloaked figure spins a plank wheel
nailed to a great tree, and the wheel's face carries four fates -- a cup, a
heart, a sword, a skull. This module is the spin. Deterministic Python
picks the fate and then a card in the deck's colours that answers to it,
seeded so the same spin can be dealt again -- Claude is not consulted,
because a wheel has never needed an opinion (ADR 14).

What a fate *means* is a filter written here, once, in prose and SQL that
say the same thing:

- **cup** -- the cup runneth over: a card that refills your hand.
- **heart** -- a card that gives life back.
- **sword** -- violence: a card that removes or damages.
- **skull** -- the graveyard: a card that fills it or robs it.

The card that comes back is a suggestion to argue with, exactly like the
gate's shortlists: nothing here writes a deck and no rationale is
prefilled (rule 4). A card the deck already runs is excluded before the
draw -- the lesson item 5 taught the slot argument, applied one feature
early -- and so is anything banned or outside the commander's identity,
which is rule 2 made executable a third time.
"""

from __future__ import annotations

import random
from typing import TYPE_CHECKING, Any

from mtglab.cards import db

if TYPE_CHECKING:
    from mtglab.decks.model import Deck

#: The four fates, in the order they sit on the painted wheel read
#: clockwise from the top: cup, heart, sword, skull. The client draws its
#: own wheel to the same order, so the index is the contract.
SYMBOLS: tuple[str, ...] = ("cup", "heart", "sword", "skull")

#: What each fate answers with. `where` is a fragment for `db.search`;
#: `meaning` is the sentence the table shows. Both live in this one table
#: so the prose and the filter cannot drift apart separately.
FATES: dict[str, dict[str, str]] = {
    "cup": {
        "label": "The Cup",
        "meaning": "The cup runneth over — a card that refills your hand.",
        "where": ("(oracle_text ILIKE '%draw a card%'"
                  " OR oracle_text ILIKE '%draws a card%'"
                  " OR oracle_text ILIKE '%draw two cards%'"
                  " OR oracle_text ILIKE '%draw three cards%'"
                  " OR oracle_text ILIKE '%draw cards%')"),
    },
    "heart": {
        "label": "The Heart",
        "meaning": "A beating heart — a card that gives life back.",
        "where": "(oracle_text ILIKE '%gain%life%')",
    },
    "sword": {
        "label": "The Sword",
        "meaning": "The sword falls — a card that removes or damages.",
        "where": ("(oracle_text ILIKE '%destroy target%'"
                  " OR oracle_text ILIKE '%deals%damage%'"
                  " OR oracle_text ILIKE '%exile target%')"),
    },
    "skull": {
        "label": "The Skull",
        "meaning": "The skull grins — a card that fills the graveyard, "
                   "or robs it.",
        "where": ("(oracle_text ILIKE '%graveyard%'"
                  " OR oracle_text ILIKE '%sacrifice%'"
                  " OR oracle_text ILIKE '%dies%')"),
    },
}

#: How the caveat states which system answered (ADR 14 boundary 3) —
#: without naming one (commandment 10, Aaron's ruling 2026-08-17: a
#: programming language is an implementation detail, and so is a seed).
#: What the user needs from boundary 3 is the DISTINCTION: this was
#: blind dice, nobody's judgment — and that survives with no language
#: named. The wire still carries `answered_by: "python"` and `seed` as
#: tokens for clients and tests; rendering them is the sin, not sending
#: them. (The Simulator's seed is different: there it is an input
#: control with a glossary entry — a feature, not trivia.)
CAVEAT = ("The wheel is blind dice over the card pool — a fate, then a "
          "random legal card in this deck's colours that answers to it. "
          "A suggestion to argue with, never a recommendation: the "
          "rationale, if it earns one, is yours to write.")


def spin(deck: Deck, identity: frozenset[str], con: Any, *,
         seed: int | None = None) -> dict[str, Any]:
    """One turn of the wheel: a fate, and a card that answers to it.

    Seeded exactly like Tier 1: an unseeded spin invents its own seed and
    reports it, so any spin can be spun again. The candidate is drawn by
    counted offset over a name-ordered query rather than `ORDER BY
    random()`, because DuckDB's randomness cannot be seeded from here and a
    spin that cannot be replayed is a number with no provenance.
    """
    if seed is None:
        seed = random.SystemRandom().randrange(2**32)
    rng = random.Random(seed)
    symbol = SYMBOLS[rng.randrange(len(SYMBOLS))]
    fate = FATES[symbol]

    in_deck = sorted({c.name for c in deck.cards}
                     | set(deck.commander)
                     | ({deck.companion} if deck.companion else set()))
    colors = sorted(c for c in identity if c in "WUBRG")
    listed = ", ".join(f"'{c}'" for c in colors)
    # A colourless identity means colourless cards only -- `NOT IN ()` is not
    # SQL, and "no colours allowed" is a length check anyway.
    fits = (f"len(list_filter(color_identity, x -> x NOT IN ({listed}))) = 0"
            if colors else "len(color_identity) = 0")
    where = " AND ".join([
        "json_extract_string(legalities, 'commander') = 'legal'",
        fits,
        fate["where"],
        "name NOT IN (" + ", ".join("?" for _ in in_deck) + ")"
        if in_deck else "1=1",
    ])

    total = con.execute(
        f"SELECT count(*) FROM oracle_cards WHERE {where}", in_deck,
    ).fetchone()[0]

    base = {
        "symbol": symbol,
        "label": fate["label"],
        "meaning": fate["meaning"],
        "seed": seed,
        "answered_by": "python",
        "caveat": CAVEAT,
    }
    if not total:
        return {**base, "card": None,
                "reason": "The pool holds no legal card in these colours "
                          "that answers to this fate. The wheel has spoken; "
                          "it just had nothing to hand you."}

    picked = db.search(con, where, in_deck, limit=1, order_by="name",
                       offset=rng.randrange(total))
    if not picked:
        # A pool refreshed between the count and the fetch, or an offset
        # past the end. Rare enough to report rather than to retry.
        return {**base, "card": None,
                "reason": "The card slipped off the wheel — spin again."}
    rec = picked[0]
    return {
        **base,
        "card": {
            "name": rec.name,
            "mana_cost": rec.mana_cost,
            "type_line": rec.type_line,
            "oracle_text": rec.oracle_text,
            "color_identity": sorted(rec.color_identity),
            "image": rec.image_normal,
            "art_crop": rec.image_art_crop,
        },
    }
