"""Originals into the cache. Gitignored, resumable, and never committed.

Downloads land under `data/cache/animist/<recipe-slug>/<source-id>/`, which
sits inside the gitignored `data/` tree on purpose: an original is a working
file, not an asset — what gets committed is the encoded output plus the
recipe that derives it, and half a gigabyte of source photography in git
would be ADR 6's mistake with a different file extension.

The download discipline is `cards/db.py`'s, copied rather than reinvented:
write to a `.part`, rename only once complete (an interrupted download must
never be mistaken for a cached copy), skip the network entirely when the file
is already present, and send the project's User-Agent on every request.
"""

from __future__ import annotations

import urllib.request
from collections.abc import Callable
from pathlib import Path

from mtglab import config
from mtglab.animist.recipe import Recipe, Source
from mtglab.animist.sources import (
    TIMEOUT_SECONDS,
    USER_AGENT,
    Confirmed,
    Transport,
    confirm,
)


def cache_dir(recipe: Recipe, source: Source) -> Path:
    """Where this source's originals live. A function over `config.DATA_DIR`
    read at call time — the `forge_home()` rule — so `config.use_paths()`
    points tests at a scratch tree without touching module globals."""
    return config.DATA_DIR / "cache" / "animist" / recipe.slug / source.id


# `(url, target) -> None`, writing the file. A second, smaller seam beside
# `Transport`: the gate's HTTP is JSON APIs, this one moves image bytes, and a
# test that fakes both never touches a socket.
Downloader = Callable[[str, Path], None]


def _download(url: str, target: Path) -> None:
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    tmp = target.with_suffix(target.suffix + ".part")
    with urllib.request.urlopen(request, timeout=TIMEOUT_SECONDS) as response, \
            tmp.open("wb") as fh:
        while chunk := response.read(1 << 20):
            fh.write(chunk)
    tmp.replace(target)


def fetch_source(recipe: Recipe, source: Source, *,
                 transport: Transport | None = None,
                 download: Downloader | None = None,
                 ) -> tuple[Confirmed, dict[str, Path]]:
    """Gate, then download. Returns the gate's proof and name -> local path.

    The order is the point: `confirm` runs the licence gate over the
    provider's API *before* any image byte moves, so a refused source leaves
    the cache exactly as it found it. Files already cached are not
    re-downloaded — but the gate still runs, because a licence confirmed last
    month is a licence this run has not confirmed.
    """
    confirmed = confirm(source, transport=transport)
    fetch_one = download if download is not None else _download
    directory = cache_dir(recipe, source)
    directory.mkdir(parents=True, exist_ok=True)
    local: dict[str, Path] = {}
    for remote in confirmed.files:
        target = directory / remote.name
        if not target.exists():
            fetch_one(remote.url, target)
        local[remote.name] = target
    return confirmed, local
