"""The dev-machine run: art in, cached derivative out (ADR 32).

This is `mtglab cardmotion build`'s engine and nothing the app ever calls:
generation is a deliberate run on the machine that has the `animist` (and,
for depth, the `depth`) extra, and the results travel to the instance the
way the pool does — over sftp, documented in HOSTING. Card facts come from
the pool (rule 1); the art comes from Scryfall at build time with the
project's User-Agent; the encodes come from the animist's table.
"""

from __future__ import annotations

import io
from dataclasses import dataclass
from typing import TYPE_CHECKING

from mtglab.animist.fetch import Downloader, _download
from mtglab.cardmotion import cache
from mtglab.cardmotion.effects import EFFECTS, Effect, EffectError, derive

if TYPE_CHECKING:
    from PIL.Image import Image

    from mtglab.cardmotion.depth import DepthModel
    from mtglab.cards.db import Connection


class BuildRefused(RuntimeError):
    """A build that cannot proceed, with the reason in the sentence."""


@dataclass(frozen=True)
class ArtSubject:
    """What the pool says about the painting being animated."""

    oracle_id: str
    card_name: str
    artist: str
    art_url: str
    scryfall_uri: str


def resolve_subject(con: Connection, name: str) -> ArtSubject:
    """The pool row for one card's art, refused in sentences when a field
    the derivation needs is absent."""
    row = con.execute(
        "SELECT oracle_id, name, artist, image_art_crop, scryfall_uri "
        "FROM oracle_cards WHERE lower(name) = lower(?) "
        "OR lower(name) LIKE lower(?) LIMIT 1",
        [name, name + " // %"]).fetchone()
    if row is None:
        raise BuildRefused(f"{name!r} is not a card the pool knows -- "
                           "rule 1: card facts come from the pool")
    oracle_id, card_name, artist, art_url, scryfall_uri = row
    if not art_url:
        raise BuildRefused(f"{card_name} has no art crop in the pool -- "
                           "nothing to animate")
    return ArtSubject(oracle_id=oracle_id or "", card_name=card_name,
                      artist=artist or "(artist unrecorded)",
                      art_url=art_url, scryfall_uri=scryfall_uri or "")


def effect_named(key: str) -> Effect:
    effect = EFFECTS.get(key)
    if effect is None:
        raise BuildRefused(f"unknown effect {key!r} "
                           f"(one of: {', '.join(sorted(EFFECTS))})")
    return effect


def _fetch_art(subject: ArtSubject, download: Downloader) -> Image:
    from PIL import Image as PILImage

    target = cache.root() / "originals" / f"{subject.oracle_id or 'art'}.img"
    target.parent.mkdir(parents=True, exist_ok=True)
    if not target.exists():
        download(subject.art_url, target)
    with PILImage.open(io.BytesIO(target.read_bytes())) as image:
        image.load()
        rgb = image.convert("RGB")
    # Video needs even dimensions (yuv420p); trim a single row/column at
    # most rather than resampling the painting.
    width, height = rgb.size
    return rgb.crop((0, 0, width - width % 2, height - height % 2))


def build_derivative(con: Connection, *, card: str, effect_key: str,
                     model: DepthModel | None = None,
                     download: Downloader | None = None,
                     ) -> cache.CachedDerivative:
    """Fetch, derive, encode, attribute. Returns the cache entry, which may
    already have been ready — a rebuild of the same inputs lands on the same
    key and simply refreshes it."""
    from mtglab.animist.encode import encode
    from mtglab.animist.recipe import Encode

    effect = effect_named(effect_key)
    subject = resolve_subject(con, card)
    fetch_one = download if download is not None else _download

    art = _fetch_art(subject, fetch_one)
    depth: Image | None = None
    if effect.needs_depth:
        if model is None:
            raise BuildRefused(
                f"{effect.key} needs the depth model -- install the depth "
                "extra, or choose an effect that does not")
        depth = model.infer(art)

    try:
        sequence = derive(art, depth, effect)
    except EffectError as exc:
        raise BuildRefused(str(exc)) from exc

    entry = cache.locate(subject.oracle_id, subject.art_url, effect)
    entry.directory.mkdir(parents=True, exist_ok=True)
    entry.file(cache.LOOP_WEBM).write_bytes(
        encode(sequence, Encode(format="webm", crf=36)))
    entry.file(cache.LOOP_MP4).write_bytes(
        encode(sequence, Encode(format="mp4", crf=28)))
    entry.file(cache.POSTER_WEBP).write_bytes(
        encode(sequence, Encode(format="webp", quality=82)))
    if depth is not None:
        buffer = io.BytesIO()
        depth.convert("L").save(buffer, format="PNG")
        entry.file(cache.DEPTH_PNG).write_bytes(buffer.getvalue())

    # Last, because `ready` is defined as its existence: a build that died
    # above leaves a directory the serving tier treats as absent.
    cache.write_attribution(
        entry, oracle_id=subject.oracle_id, card_name=subject.card_name,
        artist=subject.artist, art_url=subject.art_url,
        scryfall_uri=subject.scryfall_uri, effect=effect)
    return entry
