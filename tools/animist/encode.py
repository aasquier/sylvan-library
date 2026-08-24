"""Frames to committed bytes, with the metadata stripped and proven stripped.

Stripping is not a courtesy here, it is a CI gap being closed: the secret
scan greps with `-I`, which skips binaries, so whatever rides inside an
asset's EXIF or XMP is text nobody's tooling will ever read. The rule is
therefore that a committed asset carries none at all — and rather than trust
the encoder's defaults, `encode()` re-opens what it just produced and
*checks*, so the promise `verify` later makes ("no metadata") is one this
module has already kept once.

`_ENCODERS` is the format table ADR 29 promised — every encoder takes a
`FrameSequence` and an `Encode` block and returns bytes. Stills go through
Pillow; the video formats go through the ffmpeg binary that imageio-ffmpeg
bundles, which is a build-time subprocess on the dev machine and CI, never a
runtime dependency and never in the image (NOTICE.md carries the licence
argument). Video has no Pillow reader, so its metadata check speaks
ffmetadata: `-map_metadata -1` strips at encode time, and the check dumps
what survived and holds it to the structural allowlist.
"""

from __future__ import annotations

import io
import subprocess
import tempfile
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

from animist.recipe import VIDEO_FORMATS, Encode

if TYPE_CHECKING:
    from PIL.Image import Image

    from animist.motion import FrameSequence


class EncodeError(RuntimeError):
    """The encoder could not keep the module's promise."""


#: Keys in `Image.info` that are metadata rather than image structure.
#: Pillow's WebP loader also puts `background` and `loop` into `info` on
#: every file it opens -- those are container fields, not metadata, and
#: treating `info == {}` as the test would fail every clean asset.
_METADATA_KEYS = ("exif", "xmp", "icc_profile")

#: ffmetadata tags a muxer writes structurally, not content anybody put
#: there: the encoder string is Lavf's signature and `-map_metadata -1`
#: does not remove it. Anything else surviving the strip is a failure.
_VIDEO_STRUCTURAL_TAGS = frozenset({"encoder", "major_brand", "minor_version",
                                    "compatible_brands"})


def _strip_info(image: Image) -> Image:
    # Pillow's writers fall back to `im.info` for exif/icc/xmp when the save
    # call does not name them, so a fetched JPEG's camera data would ride
    # silently into the output. Clearing the copy's `info` is the strip.
    clean = image.copy()
    for key in _METADATA_KEYS:
        clean.info.pop(key, None)
    return clean


def _webp(sequence: FrameSequence, settings: Encode) -> bytes:
    # A still encode of a multi-frame sequence takes frame 0 — that is how a
    # loop's poster image comes out of the same derivation.
    frame = _strip_info(sequence.frames[0])
    buffer = io.BytesIO()
    # `method=6` is the encoder's slowest/smallest setting; an asset is
    # encoded once and served forever, so build-time seconds buy shipped KB.
    frame.save(buffer, format="WEBP", quality=settings.quality, method=6)
    return buffer.getvalue()


def _animated_still(sequence: FrameSequence, settings: Encode,
                    fmt: str) -> bytes:
    first = _strip_info(sequence.frames[0])
    rest = list(sequence.frames[1:])
    duration_ms = round(1000.0 / sequence.fps)
    buffer = io.BytesIO()
    if fmt == "WEBP":
        first.save(buffer, format=fmt, save_all=True, append_images=rest,
                   duration=duration_ms, loop=0, quality=settings.quality,
                   method=6)
    else:  # APNG rides the PNG writer; it is lossless, so no quality knob.
        first.save(buffer, format="PNG", save_all=True, append_images=rest,
                   duration=duration_ms, loop=0)
    return buffer.getvalue()


def _awebp(sequence: FrameSequence, settings: Encode) -> bytes:
    return _animated_still(sequence, settings, "WEBP")


def _apng(sequence: FrameSequence, settings: Encode) -> bytes:
    return _animated_still(sequence, settings, "PNG")


def _video(sequence: FrameSequence, settings: Encode, *, codec: str,
           container: str, extra: list[str]) -> bytes:
    try:
        import imageio_ffmpeg
    except ImportError as exc:
        raise EncodeError("video encoding needs imageio-ffmpeg -- it is a "
                          "core dependency of this toolbox, so reinstall it: "
                          "`pip install -e tools`") from exc
    import numpy as np

    width, height = sequence.size
    if width % 2 or height % 2:
        # yuv420p subsamples chroma 2x2, so odd dimensions cannot encode.
        raise EncodeError(f"video needs even dimensions, got "
                          f"{width}x{height} -- resize first")
    if settings.crf is None:  # the loader enforces this; belt for callers
        raise EncodeError(f"{container} needs `crf` set on the encode block")

    with tempfile.TemporaryDirectory(prefix="animist-encode-") as tmp:
        target = Path(tmp) / f"out.{container}"
        # `macro_block_size=1` or ffmpeg silently pads frames to multiples of
        # 16 — a committed asset that came out a different size than its
        # recipe's `expect.width` would be a very confusing verify failure.
        writer = imageio_ffmpeg.write_frames(
            str(target), (width, height), pix_fmt_in="rgb24",
            pix_fmt_out="yuv420p", fps=sequence.fps, codec=codec,
            macro_block_size=1,
            output_params=["-an", "-map_metadata", "-1",
                           "-crf", str(settings.crf), *extra],
        )
        writer.send(None)  # seed the coroutine, per the imageio-ffmpeg API
        try:
            for frame in sequence.frames:
                rgb = np.asarray(frame.convert("RGB"), dtype=np.uint8)
                writer.send(np.ascontiguousarray(rgb).tobytes())
        finally:
            writer.close()
        return target.read_bytes()


def _webm(sequence: FrameSequence, settings: Encode) -> bytes:
    # `-b:v 0` puts VP9 in constant-quality mode; without it, crf is a cap
    # on top of a default bitrate target rather than the actual control.
    return _video(sequence, settings, codec="libvpx-vp9", container="webm",
                  extra=["-b:v", "0", "-row-mt", "1"])


def _mp4(sequence: FrameSequence, settings: Encode) -> bytes:
    # `+faststart` moves the moov atom to the front so a page backdrop can
    # start playing before the file finishes downloading.
    return _video(sequence, settings, codec="libx264", container="mp4",
                  extra=["-profile:v", "main", "-movflags", "+faststart"])


_ENCODERS: dict[str, Callable[[FrameSequence, Encode], bytes]] = {
    "webp": _webp,
    "awebp": _awebp,
    "apng": _apng,
    "webm": _webm,
    "mp4": _mp4,
}


def has_metadata(data: bytes, fmt: str = "webp") -> list[str]:
    """The metadata keys present in an encoded asset; empty means clean.

    Shared with `verify.py`, so the check made at encode time and the check
    made against a committed file can never disagree about what "none" means.
    Stills are asked through Pillow; video through ffmpeg's ffmetadata dump,
    ignoring the structural tags the muxer itself writes.
    """
    if fmt in VIDEO_FORMATS:
        return _video_metadata(data, fmt)

    from PIL import Image as PILImage

    with PILImage.open(io.BytesIO(data)) as reopened:
        found = [key for key in _METADATA_KEYS if key in reopened.info]
        if reopened.getexif():
            if "exif" not in found:
                found.append("exif")
        return found


def _video_metadata(data: bytes, fmt: str) -> list[str]:
    import imageio_ffmpeg

    ffmpeg = imageio_ffmpeg.get_ffmpeg_exe()
    with tempfile.TemporaryDirectory(prefix="animist-meta-") as tmp:
        source = Path(tmp) / f"probe.{fmt}"
        source.write_bytes(data)
        proc = subprocess.run(
            [ffmpeg, "-v", "error", "-i", str(source),
             "-f", "ffmetadata", "-"],
            capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        detail = proc.stderr.strip().splitlines()
        return [f"unreadable ({detail[-1] if detail else 'no detail'})"]
    found = []
    for line in proc.stdout.splitlines():
        if "=" not in line or line.startswith(";"):
            continue
        key = line.split("=", 1)[0].strip().lower()
        if key and key not in _VIDEO_STRUCTURAL_TAGS:
            found.append(key)
    return sorted(set(found))


def encode(sequence: FrameSequence, settings: Encode) -> bytes:
    """Encode per the recipe's `encode` block. The output is verified clean
    before it is returned; an encoder that smuggled metadata is an error,
    not an asset."""
    encoder = _ENCODERS.get(settings.format)
    if encoder is None:
        raise EncodeError(f"format {settings.format!r} is not built yet -- "
                          "the table in encode.py is where it would go")
    data = encoder(sequence, settings)
    leftover = has_metadata(data, settings.format)
    if leftover:
        raise EncodeError("the encoder kept metadata the pipeline promised "
                          f"to strip: {', '.join(leftover)}")
    return data
