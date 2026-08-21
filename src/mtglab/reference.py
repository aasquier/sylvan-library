"""The reference prose, rendered as the JSON both runtimes serve from.

Four modules here are reference prose as code -- `colors.py`, `glossary.py`,
`lore.py` and `tarotlore.py`, plus the labelling vocabulary in
`decks/model.py` -- and the Go migration serves the routes over them from
**generated JSON both runtimes share** (docs/go-migration/PLAN.md, Phase 3):
Python renders these payloads into `go/internal/reference/data/`, the Go
module embeds them, and `tests/test_go_fixtures.py` holds the committed
files equal to a fresh render so neither side can drift quietly. It is the
`web_dist` pattern applied to prose -- built output committed, CI checks it
is current -- and at retirement (Phase 8) the JSON becomes the authoritative
text and is edited directly, which is the one thing checked-in prose is for
(`colors.py`'s docstring: *bland prose is fixed by editing*).

Each function returns **exactly what the route serves today**: `colors()` is
`service.color_taxonomy()`'s payload, `glossary()` is `service.glossary()`'s,
`themes()` is `/api/themes`', and a test pins each equality -- the whole point
is one shape on the wire whichever runtime answered. `lore()` and
`tarotlore()` carry card *names*, never resolved cards: card facts come from
the pool at request time (CLAUDE.md rule 1) in whichever runtime serves the
route, and a name that does not resolve is dropped and counted there.

Regenerate with `python tests/go_fixtures.py` from the repository root.
"""

from __future__ import annotations

import json
from typing import Any

from mtglab import colors, glossary, lore, tarotlore
from mtglab.decks.model import ARCHETYPES, THEMES

#: The files, in the order they are written. The names are the Go package's
#: embed paths; a rename here is a rename there.
FILES = ("colors.json", "glossary.json", "themes.json", "lore.json",
         "tarotlore.json")


def colors_payload() -> dict[str, Any]:
    """`GET /api/colors`: the 32 combinations, the five colours, the eras.

    Names and roles only for the champions and signature cards -- the cards
    themselves are resolved through the pool by `/api/colors/{key}`, in
    whichever runtime serves it.
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
            "champions": [{"card": ch.card, "role": ch.role}
                          for ch in c.champions],
            "signature": list(c.signature),
        } for c in colors.COMBINATIONS],
    }


def glossary_payload() -> dict[str, Any]:
    """`GET /api/glossary`: the sections and the terms."""
    return {
        "sections": [{"key": s, "label": glossary.SECTION_LABELS[s],
                      "blurb": glossary.SECTION_BLURBS[s]}
                     for s in glossary.SECTIONS],
        "terms": [{"key": t.key, "term": t.term, "short": t.short,
                   "long": t.long, "section": t.section,
                   "see_also": list(t.see_also)} for t in glossary.TERMS],
    }


def themes_payload() -> dict[str, Any]:
    """`GET /api/themes`: the labelling vocabulary and the four class words,
    in `ARCHETYPES` order -- best-piloted first, which is the only thing the
    order carries (ADR 37)."""
    return {"themes": list(THEMES), "archetypes": list(ARCHETYPES)}


def lore_payload() -> dict[str, Any]:
    """The shelves: volumes and facts, with card *names* for the route to
    resolve. `learn` is `{tab, key}` or null, as the route emits it."""
    return {
        "volumes": [{"key": v, "label": lore.VOLUME_LABELS[v],
                     "blurb": lore.VOLUME_BLURBS[v]} for v in lore.VOLUMES],
        "facts": [{
            "key": f.key, "volume": f.volume, "fact": f.fact, "more": f.more,
            "cards": list(f.cards),
            "learn": {"tab": f.learn[0], "key": f.learn[1]} if f.learn else None,
        } for f in lore.FACTS],
    }


def tarotlore_payload() -> dict[str, Any]:
    """What the fortune-teller knows: every fact, deck tier first, with its
    id, its source and the card it is about (empty for the deck tier).
    Served to no route yet -- the reader cites these by id (ADR 21,
    `theme.keep_fact`) and that surface is Phase 6's -- but the corpus is
    data, and rendering it now costs nothing."""
    return {
        "facts": [{"id": f.id, "text": f.text, "source": f.source,
                   "card": f.card} for f in tarotlore.ALL],
    }


def payloads() -> dict[str, dict[str, Any]]:
    return {
        "colors.json": colors_payload(),
        "glossary.json": glossary_payload(),
        "themes.json": themes_payload(),
        "lore.json": lore_payload(),
        "tarotlore.json": tarotlore_payload(),
    }


def render() -> dict[str, str]:
    """Every file's text, indented for review -- a diff here is a diff to
    prose somebody reads -- with keys in the order the route emits them."""
    return {name: json.dumps(payload, indent=2, ensure_ascii=False) + "\n"
            for name, payload in payloads().items()}
