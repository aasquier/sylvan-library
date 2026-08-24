"""Where derivatives live, and what identifies one (ADR 32).

The key is ADR 18's shape translated to pixels: the inputs (the commander's
`oracle_id` and the exact art URL, because a reprint's crop is a different
painting) plus the effect's fingerprint (its knobs and the derivation code
version). A hit is served; a miss is simply not there — generation is a
deliberate dev-machine run, never something a request triggers.

`attribution.json` is written at build time from the pool row, so the
serving tier never needs the pool to render credit — an instance whose pool
is mid-refresh still names the artist.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from animist import config
from cardmotion.effects import Effect

#: The files a finished derivative consists of. `depth.png` ships so the
#: browser's WebGL parallax player can drive the same map interactively;
#: absent for effects that never inferred one.
LOOP_WEBM = "loop.webm"
LOOP_MP4 = "loop.mp4"
POSTER_WEBP = "poster.webp"
DEPTH_PNG = "depth.png"
ATTRIBUTION = "attribution.json"

SERVABLE = frozenset({LOOP_WEBM, LOOP_MP4, POSTER_WEBP, DEPTH_PNG})


def cache_key(oracle_id: str, art_url: str, effect: Effect) -> str:
    payload = f"{oracle_id}\n{art_url}\n{effect.fingerprint()}"
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:24]


def root() -> Path:
    """Read `config.DATA_DIR` at call time — the `forge_home()` rule — so
    `config.use_paths()` points tests at a scratch tree."""
    return config.DATA_DIR / "cache" / "cardmotion"


@dataclass(frozen=True)
class CachedDerivative:
    """One derivative's directory and its members."""

    directory: Path

    @property
    def attribution_path(self) -> Path:
        return self.directory / ATTRIBUTION

    def file(self, name: str) -> Path:
        return self.directory / name

    @property
    def ready(self) -> bool:
        """Done means the attribution exists — it is written last, so a
        build that died mid-encode reads as absent rather than half-served."""
        return self.attribution_path.exists()

    def attribution(self) -> dict[str, Any]:
        raw = json.loads(self.attribution_path.read_text(encoding="utf-8"))
        assert isinstance(raw, dict)
        return raw


def locate(oracle_id: str, art_url: str, effect: Effect) -> CachedDerivative:
    return CachedDerivative(root() / cache_key(oracle_id, art_url, effect))


def art_stem(url: str) -> str:
    """A Scryfall image URL sans query string. The path names the painting;
    the query is a cache-buster a bulk refresh may rewrite, and a refresh
    must not orphan every derivative."""
    return url.split("?", 1)[0]


def find_ready(oracle_id: str, effect: Effect,
               art_url: str | None = None) -> CachedDerivative | None:
    """The serving tier's lookup. `art_url` is the painting the page is
    actually showing: a deck that chose a printing must never be served a
    loop derived from a different one — the still is the correct painting,
    so no match means `ready: false`, not the nearest neighbour. `None`
    keeps the older any-art answer for callers that do not know the crop.
    The attribution names all three, so scan the small cache and match —
    dozens of directories, not thousands; a card has one or two
    derivatives, ever."""
    base = root()
    if not base.is_dir():
        return None
    for directory in sorted(base.iterdir()):
        derivative = CachedDerivative(directory)
        if not derivative.ready:
            continue
        meta = derivative.attribution()
        if meta.get("oracle_id") != oracle_id \
                or meta.get("fingerprint") != effect.fingerprint():
            continue
        if art_url is not None \
                and art_stem(str(meta.get("art_url", ""))) != art_stem(art_url):
            continue
        return derivative
    return None


def write_attribution(derivative: CachedDerivative, *, oracle_id: str,
                      card_name: str, artist: str, art_url: str,
                      scryfall_uri: str, effect: Effect) -> None:
    payload = {
        "oracle_id": oracle_id,
        "card_name": card_name,
        "artist": artist,
        "art_url": art_url,
        "scryfall_uri": scryfall_uri,
        "effect": effect.key,
        "fingerprint": effect.fingerprint(),
        "generated_at": datetime.now(UTC).isoformat(),
        "notice": ("Unofficial Fan Content permitted under the Wizards of "
                   "the Coast Fan Content Policy. Artwork remains the "
                   "property of its artist and Wizards of the Coast."),
    }
    derivative.directory.mkdir(parents=True, exist_ok=True)
    derivative.attribution_path.write_text(
        json.dumps(payload, indent=2, sort_keys=True) + "\n",
        encoding="utf-8")
