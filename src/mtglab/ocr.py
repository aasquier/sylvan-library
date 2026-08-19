"""The reading engine, fetched once and served from our own shelf.

The camera door (ROADMAP item 14) reads a card in the browser, and reading
needs Tesseract: about six megabytes of WebAssembly and trained data. None
of it is in git, none of it is in the image, and none of it is hotlinked at
render time. It is downloaded the first time somebody opens the camera,
cached under ``data/cache/ocr/``, and every later request is a local file
read -- exactly the shape ``symbols.py`` uses for the mana pips, and
recorded by the same argument (ADR 33).

The sizes are why. ``tesseract.js-core`` unpacks to **30MB**; the one build
we use is 4MB and the trained data another 2MB. The committed frontend
bundle is in git (see ``vite.config.ts``), so a megabyte here is a megabyte
in every clone forever, and the asset pipeline that would otherwise own a
binary (ADR 29) is for pictures with recipes, not for somebody else's
compiler output.

**Two things here that ``symbols.py`` does not do, both because this is
executable code rather than an SVG.**

*Every file is pinned by digest.* A version in a URL says which release was
asked for; a SHA-256 says which bytes arrived. A CDN that is compromised, a
captive portal that answers 200 with a login page, a proxy that helpfully
"optimises" a script -- all three are the same failure, and all three are
caught here rather than in a browser that would simply run it. A digest
mismatch is a refusal and a loud log line, never a cache write.

*The cache path carries the version.* Upgrading the pin is then a new file
rather than a stale one, so nobody has to remember to clear a volume.

The **trained data is the `_fast` variant**, and that is a measurement:
2.0MB against 10.9MB for the standard one, for a job that reads a card
name and a three-digit number in a known alphabet. The accuracy that the
larger file buys is accuracy on prose.

The core is the **SIMD LSTM** build alone rather than all four that
``tesseract.js-core`` ships. LSTM because the legacy engine is not used;
SIMD because this project's browser floor is already Safari 16.4 (Tailwind
v4 put it there) and WebAssembly SIMD arrived in 16.4. If that floor ever
drops, the non-SIMD core is one more row in the table below.
"""

from __future__ import annotations

import hashlib
import logging
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path

from mtglab import config
from mtglab.cards.db import USER_AGENT

log = logging.getLogger("mtglab.ocr")


@dataclass(frozen=True)
class Asset:
    """One file on the shelf: where it comes from and what it must be."""

    url: str
    #: SHA-256 of the exact bytes. Recorded by downloading and hashing, never
    #: copied from a page that could be edited.
    digest: str
    #: Bytes, for the cap. Recorded the same way.
    size: int
    media_type: str


#: Pinned versions. Bumping one means re-recording every digest below it,
#: which is the point: an upgrade is a deliberate act with a checksum in the
#: diff, not a silent re-fetch.
CORE_VERSION = "6.1.2"
WORKER_VERSION = "7.0.0"
TESSDATA_VERSION = "4.0.0_fast"

#: What the browser may ask for, keyed by the name it asks under. A name
#: that is not one of these keys never touches the filesystem or the
#: network, which is this module's whole path-traversal story -- there is no
#: pattern to get wrong, only a table to be absent from.
ASSETS: dict[str, Asset] = {
    "tesseract-core-simd-lstm.wasm.js": Asset(
        url=(f"https://cdn.jsdelivr.net/npm/tesseract.js-core@{CORE_VERSION}"
             "/tesseract-core-simd-lstm.wasm.js"),
        digest="9d7c43fb206dc9f48475228b46bf35f888fa9e6259da2e67d5a75c77049f2dc7",
        size=3_954_569,
        media_type="text/javascript",
    ),
    "worker.min.js": Asset(
        url=(f"https://cdn.jsdelivr.net/npm/tesseract.js@{WORKER_VERSION}"
             "/dist/worker.min.js"),
        digest="576b7df7e3393e137e51849357c9adb53fe7ac1bb69bfa06cf3d61520f182c6d",
        size=111_307,
        media_type="text/javascript",
    ),
    "eng.traineddata.gz": Asset(
        url=(f"https://tessdata.projectnaptha.com/{TESSDATA_VERSION}"
             "/eng.traineddata.gz"),
        digest="18c1ac52b75e35d44735fb6c2a60acfaf23033524653200738e98f0243edb75b",
        size=1_984_273,
        media_type="application/gzip",
    ),
}

#: Headroom over the largest pinned asset. The digest is what actually
#: decides correctness; this only stops a misbehaving host streaming the
#: volume full before the hash can be checked.
_MAX_BYTES = 16 * 1024 * 1024

#: Assets whose download failed for a reason that is not transient, so one
#: cold start with a broken pin does not re-ask on every capture. In memory
#: only, like `symbols._missing`.
_refused: set[str] = set()


def cache_dir() -> Path:
    """Read at call time so ``config.use_paths()`` redirects tests.

    Versioned, so bumping a pin cannot serve yesterday's bytes off a volume
    nobody thought to clear.
    """
    stamp = f"{CORE_VERSION}-{WORKER_VERSION}-{TESSDATA_VERSION}"
    return config.DATA_DIR / "cache" / "ocr" / stamp


def _download(url: str) -> bytes:
    """One bounded GET. Split out so tests can stand in for the network."""
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=30) as response:
        data: bytes = response.read(_MAX_BYTES + 1)
        return data


def ensure(name: str) -> Path | None:
    """The cached file for one asset name, fetching it on the first ask.

    ``None`` means "not available here today" and the caller answers 404:
    an unknown name, a cold cache with no network, or -- the one worth
    reading a log for -- bytes that did not match their pinned digest.
    """
    asset = ASSETS.get(name)
    if asset is None:
        return None
    target = cache_dir() / name
    if target.exists():
        return target
    if name in _refused:
        return None

    try:
        body = _download(asset.url)
    except urllib.error.HTTPError as exc:
        log.warning("ocr asset %s: host answered %s", name, exc.code)
        return None
    except OSError as exc:
        # Transient, and must not be remembered as absence.
        log.warning("ocr asset %s: %s", name, exc)
        return None

    if len(body) > _MAX_BYTES:
        log.warning("ocr asset %s: over the size cap", name)
        return None

    got = hashlib.sha256(body).hexdigest()
    if got != asset.digest:
        # Deliberately loud and deliberately sticky. Every innocent cause of
        # this (a captive portal, a rewriting proxy) and every guilty one
        # look identical from here, so neither gets to be executed.
        log.error("ocr asset %s: digest mismatch, refusing to cache "
                  "(expected %s, got %s)", name, asset.digest, got)
        _refused.add(name)
        return None

    target.parent.mkdir(parents=True, exist_ok=True)
    # A unique staging name per call, for `symbols.py`'s reason: two cold
    # asks each write whole bytes and the second `replace` wins with an
    # identical file, where a shared stage could leave a torn one forever.
    stage = target.with_name(f".{name}.{os.urandom(4).hex()}.tmp")
    stage.write_bytes(body)
    stage.replace(target)
    return target


def installed() -> dict[str, bool]:
    """Which assets are on the shelf right now. For the health endpoint and
    for a client that would rather not open a viewfinder it cannot use."""
    return {name: (cache_dir() / name).exists() for name in ASSETS}
