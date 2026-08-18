"""The official mana symbols, fetched once and served from our own shelf.

Scryfall draws every symbol Magic has ever printed — the five colours, tap
and untap, the hybrid split discs, Phyrexian phi — and serves each as a tiny
SVG. The app used to draw its own five (PR #61) so that a pip in prose never
depended on a third party at render time; that property is kept here by
moving the dependency to *fill* time instead. A symbol is downloaded the
first time anybody asks for it, cached under ``data/cache/symbols/``, and
every later request is a local file read. Nothing enters git (rule 5, ADR 6),
nothing renders from Scryfall's origin, and a cold cache with no network
degrades to the drawn glyphs the client still carries. ADR 33 records the
decision.

Deliberately shaped like ``cardmotion/cache.py``'s serving half: the route
that fronts this module never generates anything at request time beyond one
bounded download, and the cache directory lives with the rest of the app's
derived data, on the volume when deployed.
"""

from __future__ import annotations

import logging
import os
import re
import urllib.error
import urllib.request
from pathlib import Path

from mtglab import config
from mtglab.cards.db import USER_AGENT

log = logging.getLogger("mtglab.symbols")

# Scryfall's card-symbol CDN. The code in the URL is the symbol with its
# braces and slash dropped: {W/U} is WU.svg, {T} is T.svg.
_CDN = "https://svgs.scryfall.io/card-symbols"

# What a symbol code may look like: letters and digits, short. This is the
# path-traversal guard — a code that fails this never reaches the filesystem
# or the network. Real codes run from "W" to "1000000" (yes, Gleemax) and
# "CHAOS"; ten characters covers the lot with room.
_CODE = re.compile(r"[0-9A-Z]{1,10}")

# An SVG is a few KB; anything bigger is not a symbol. The cap means a
# misbehaving proxy or a captive portal can never fill the volume.
_MAX_BYTES = 65_536

# Codes Scryfall answered 404 for, so one typo'd deck note cannot re-ask on
# every render. In memory only — a symbol Scryfall gains later (a new set's
# mechanic) is one process restart away, which is the right staleness for a
# set that changes a few times a year.
_missing: set[str] = set()


def cache_dir() -> Path:
    """Read at call time so ``config.use_paths()`` redirects tests."""
    return config.DATA_DIR / "cache" / "symbols"


def _download(url: str) -> bytes:
    """One bounded GET. Split out so tests can stand in for the network."""
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=10) as response:
        data: bytes = response.read(_MAX_BYTES + 1)
        return data


def ensure(code: str) -> Path | None:
    """The cached SVG for one symbol code, downloading it if this is the
    first ask. ``None`` means "no such symbol here today": a malformed code,
    a code Scryfall has never heard of, or a cold cache with no network —
    the caller answers 404 and the client falls back to its drawn glyphs.
    """
    if not _CODE.fullmatch(code):
        return None
    target = cache_dir() / f"{code}.svg"
    if target.exists():
        return target
    if code in _missing:
        return None

    try:
        body = _download(f"{_CDN}/{code}.svg")
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            _missing.add(code)
        else:
            log.warning("symbol %s: Scryfall answered %s", code, exc.code)
        return None
    except OSError as exc:
        # Network trouble is transient and must not be remembered as absence.
        log.warning("symbol %s: %s", code, exc)
        return None

    # A captive portal's login page is not a symbol. The check is light on
    # purpose — it only has to keep non-SVG bytes out of the cache, not
    # validate SVG.
    if len(body) > _MAX_BYTES or b"<svg" not in body[:1024]:
        log.warning("symbol %s: response did not look like an SVG", code)
        return None

    target.parent.mkdir(parents=True, exist_ok=True)
    # A unique staging name per call: two threads asking for the same cold
    # symbol each write whole bytes, and whichever `replace` lands second
    # wins with an identical file. Interleaved writes to one shared stage
    # could land a corrupt SVG in the cache forever.
    stage = target.with_name(f".{code}.{os.urandom(4).hex()}.tmp")
    stage.write_bytes(body)
    stage.replace(target)
    return target
