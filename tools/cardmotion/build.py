"""The dev-machine run: art in, cached derivative out (ADR 32).

This is `cardmotion build`'s engine and nothing the app ever calls:
generation is a deliberate run on the machine that has this toolbox (and,
for depth, its `depth` extra), and the results travel to the instance the
way the pool does — over sftp, documented in HOSTING. Card facts come from
the pool (rule 1); the art comes from Scryfall at build time with the
project's User-Agent; the encodes come from the animist's table.
"""

from __future__ import annotations

import io
from collections.abc import Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from animist.fetch import Downloader, _download
from cardmotion import cache
from cardmotion.effects import EFFECTS, Effect, EffectError, derive

if TYPE_CHECKING:
    from PIL.Image import Image

    from cardmotion.depth import DepthModel
    from cardmotion.pool import Connection


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


#: `(printing_id) -> the card's Scryfall JSON`. A seam so tests never reach
#: the network; the default asks api.scryfall.com with the project's
#: User-Agent, which a dev-machine build already does for the art itself.
CardJsonFetch = Callable[[str], "dict[str, Any]"]


def _scryfall_card_json(printing_id: str) -> dict[str, Any]:
    import json
    import urllib.request

    from animist.sources import USER_AGENT

    request = urllib.request.Request(
        f"https://api.scryfall.com/cards/{printing_id}",
        headers={"User-Agent": USER_AGENT, "Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        parsed = json.loads(response.read().decode("utf-8"))
    assert isinstance(parsed, dict)
    return parsed


def resolve_subject(con: Connection, name: str, *, art_id: str | None = None,
                    fetch_card_json: CardJsonFetch | None = None) -> ArtSubject:
    """The pool row for one card's art, refused in sentences when a field
    the derivation needs is absent.

    `art_id` is the deck's chosen printing (`commander_art`): the derivative
    must be built from the painting the deck actually shows, or the serving
    tier's art match will — correctly — refuse to serve it.

    The credit for that printing comes from the pool's own `printings.artist`
    since 2026-08-19, and from Scryfall at build time when the pool has none —
    a pool built before that column existed, or a printing Scryfall never
    attributed. An unreachable answer degrades to "(artist unrecorded)" rather
    than crediting the default printing's painter with someone else's work,
    which is the same rule `service._chosen_arts` keeps for the deck page and
    is not negotiable: this is somebody's painting, hotlinked under Wizards'
    Fan Content Policy, and a wrong name is worse than no name.
    """
    row = con.execute(
        "SELECT oracle_id, name, artist, image_art_crop, scryfall_uri "
        "FROM oracle_cards WHERE lower(name) = lower(?) "
        "OR lower(name) LIKE lower(?) LIMIT 1",
        [name, name + " // %"]).fetchone()
    if row is None:
        raise BuildRefused(f"{name!r} is not a card the pool knows -- "
                           "rule 1: card facts come from the pool")
    oracle_id, card_name, artist, art_url, scryfall_uri = row
    if art_id:
        from cardmotion.pool import art_crop_from, printing_columns

        has_artist = "artist" in printing_columns(con)
        printing = con.execute(
            f"SELECT image_normal, {'artist' if has_artist else 'NULL'} "
            f"FROM printings WHERE id = ?", [art_id]).fetchone()
        chosen_url = art_crop_from(printing[0]) if printing else None
        if not chosen_url:
            raise BuildRefused(
                f"{card_name}: chosen printing {art_id!r} is not in the "
                "pool (or has no image) -- refresh the pool, or clear the "
                "deck's art choice")
        if cache.art_stem(chosen_url) != cache.art_stem(art_url or ""):
            art_url = chosen_url
            artist = printing[1] or None
            if artist is None:
                # The pool cannot say -- it predates the column, or Scryfall
                # never attributed this printing. One request, the same trip
                # the art bytes already make.
                fetch = fetch_card_json or _scryfall_card_json
                try:
                    card_json = fetch(art_id)
                    artist = str(card_json.get("artist", "")) or None
                    scryfall_uri = str(card_json.get("scryfall_uri", "")) \
                        or scryfall_uri
                except (OSError, ValueError):
                    # Offline, refused, or not JSON: the painting still gets
                    # derived, and the credit degrades honestly rather than
                    # naming the default printing's painter.
                    artist = None
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
    import hashlib

    from PIL import Image as PILImage

    # The art's own identity rides in the filename: one commander can have
    # derivatives from two printings, and a cache keyed on oracle_id alone
    # would hand the second build the first painting's bytes.
    stamp = hashlib.sha256(
        cache.art_stem(subject.art_url).encode("utf-8")).hexdigest()[:12]
    target = cache.root() / "originals" \
        / f"{subject.oracle_id or 'art'}-{stamp}.img"
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
                     art_id: str | None = None,
                     fetch_card_json: CardJsonFetch | None = None,
                     ) -> cache.CachedDerivative:
    """Fetch, derive, encode, attribute. Returns the cache entry, which may
    already have been ready — a rebuild of the same inputs lands on the same
    key and simply refreshes it."""
    from animist.encode import encode
    from animist.recipe import Encode

    effect = effect_named(effect_key)
    subject = resolve_subject(con, card, art_id=art_id,
                              fetch_card_json=fetch_card_json)
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


@dataclass
class SyncReport:
    """What one sweep found and did, per deck slug."""

    built: list[str]
    present: list[str]
    skipped: list[str]
    refused: list[str]


def _chosen_art_url(con: Connection, art_id: str) -> str | None:
    from cardmotion.pool import art_crop_from

    row = con.execute(
        "SELECT image_normal FROM printings WHERE id = ?",
        [art_id]).fetchone()
    return art_crop_from(row[0]) if row else None


def sync(con: Connection, decks: list[Any], *, effect_key: str,
         model_loader: Callable[[], DepthModel] | None = None,
         download: Downloader | None = None,
         fetch_card_json: CardJsonFetch | None = None) -> SyncReport:
    """Every deck's commander, checked against the cache; missing loops
    built from the painting the deck actually shows.

    This is the resync verb for the two ways a derivative goes missing:
    a deck arrives (import, create) whose commander was never derived, and
    a deck's art choice moves to a printing that was. The check matches
    the way the serving tier matches (`find_ready`, on the art stem), so
    "present" here means "the deck page breathes" and nothing else.

    `model_loader` is called at most once, on the first miss that needs
    depth — a fully synced library never loads the model at all.
    """
    effect = effect_named(effect_key)
    report = SyncReport(built=[], present=[], skipped=[], refused=[])
    model: DepthModel | None = None
    for deck in decks:
        slug = getattr(deck, "slug", "?")
        commander = getattr(deck, "commander", [])
        if not commander:
            report.skipped.append(slug)
            continue
        name = commander[0]
        art_id = getattr(deck, "commander_art", "") or None
        try:
            subject = resolve_subject(con, name)
            shown = (_chosen_art_url(con, art_id) if art_id else None) \
                or subject.art_url
            if cache.find_ready(subject.oracle_id, effect, shown) is not None:
                report.present.append(slug)
                continue
            if effect.needs_depth and model is None:
                if model_loader is None:
                    raise BuildRefused(
                        f"{effect.key} needs the depth model and no loader "
                        "was provided")
                model = model_loader()
            build_derivative(con, card=name, effect_key=effect_key,
                             model=model, download=download, art_id=art_id,
                             fetch_card_json=fetch_card_json)
            report.built.append(slug)
        except BuildRefused as exc:
            report.refused.append(f"{slug}: {exc}")
    return report
