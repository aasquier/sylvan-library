"""Image to committed bytes, with the metadata stripped and proven stripped.

Stripping is not a courtesy here, it is a CI gap being closed: the secret
scan greps with `-I`, which skips binaries, so whatever rides inside an
asset's EXIF or XMP is text nobody's tooling will ever read. The rule is
therefore that a committed asset carries none at all — and rather than trust
the encoder's defaults, `encode()` re-opens what it just produced and
*checks*, so the promise `verify` later makes ("no metadata") is one this
module has already kept once.

The format table is the extension point: `apng` and the video formats of
ADR 29's later phases are new entries with the same signature, not a new
code path.
"""

from __future__ import annotations

import io
from typing import TYPE_CHECKING

from mtglab.animist.recipe import Encode

if TYPE_CHECKING:
    from PIL.Image import Image


class EncodeError(RuntimeError):
    """The encoder could not keep the module's promise."""


#: Keys in `Image.info` that are metadata rather than image structure.
#: Pillow's WebP loader also puts `background` and `loop` into `info` on
#: every file it opens -- those are container fields, not metadata, and
#: treating `info == {}` as the test would fail every clean asset.
_METADATA_KEYS = ("exif", "xmp", "icc_profile")


def _webp(image: Image, quality: int) -> bytes:
    # Pillow's WebP writer falls back to `im.info` for exif/icc/xmp when the
    # save call does not name them, so a fetched JPEG's camera data would
    # ride silently into the output. Clearing the copy's `info` is the strip.
    clean = image.copy()
    for key in _METADATA_KEYS:
        clean.info.pop(key, None)
    buffer = io.BytesIO()
    # `method=6` is the encoder's slowest/smallest setting; an asset is
    # encoded once and served forever, so build-time seconds buy shipped KB.
    clean.save(buffer, format="WEBP", quality=quality, method=6)
    return buffer.getvalue()


def has_metadata(data: bytes) -> list[str]:
    """The metadata keys present in an encoded image; empty means clean.

    Shared with `verify.py`, so the check made at encode time and the check
    made against a committed file can never disagree about what "none" means.
    """
    from PIL import Image as PILImage

    with PILImage.open(io.BytesIO(data)) as reopened:
        found = [key for key in _METADATA_KEYS if key in reopened.info]
        if reopened.getexif():
            if "exif" not in found:
                found.append("exif")
        return found


def encode(image: Image, settings: Encode) -> bytes:
    """Encode per the recipe's `encode` block. The output is verified clean
    before it is returned; an encoder that smuggled metadata is an error,
    not an asset."""
    if settings.format != "webp":
        raise EncodeError(f"format {settings.format!r} is not built yet -- "
                          "the table in encode.py is where it would go")
    data = _webp(image, settings.quality)
    leftover = has_metadata(data)
    if leftover:
        raise EncodeError("the encoder kept metadata the pipeline promised "
                          f"to strip: {', '.join(leftover)}")
    return data
