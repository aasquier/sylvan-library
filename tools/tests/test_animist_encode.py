"""Encoding: clean bytes out, metadata provably gone, quality that bites.

Since ADR 31 the encoder takes a `FrameSequence` — a still is the one-frame
case — and the format table holds five entries. The video round-trips here
run the real bundled ffmpeg on tiny sequences; that is deliberate, and it is
why imageio-ffmpeg is vendored into `dev`: an importorskip would move the
pinned skip count and quietly stop reading the video half of the table.
"""

from __future__ import annotations

import io

import numpy as np
import pytest
from PIL import Image

from animist.encode import EncodeError, encode, has_metadata
from animist.motion import FrameSequence
from animist.recipe import Encode


def noisy_image(seed: int = 7, size: int = 64) -> Image.Image:
    rng = np.random.default_rng(seed)
    return Image.fromarray(
        rng.integers(0, 255, (size, size, 3), dtype=np.uint8), mode="RGB")


def noisy_sequence(frames: int = 4, size: int = 32,
                   fps: int = 8) -> FrameSequence:
    return FrameSequence(
        frames=tuple(noisy_image(seed, size) for seed in range(frames)),
        fps=fps)


def test_webp_round_trips_at_the_right_size() -> None:
    data = encode(FrameSequence.still(noisy_image()),
                  Encode(format="webp", quality=82))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.format == "WEBP"
        assert reopened.size == (64, 64)


def test_a_still_encode_of_a_sequence_is_its_poster() -> None:
    """Frame 0 through the webp row -- how a loop's poster comes out of the
    same derivation as the loop."""
    sequence = noisy_sequence(frames=3)
    poster = encode(sequence, Encode(format="webp", quality=82))
    first = encode(FrameSequence.still(sequence.frames[0]),
                   Encode(format="webp", quality=82))
    assert poster == first


def test_exif_from_a_fetched_original_is_stripped() -> None:
    # A fetched JPEG arrives with camera metadata; the pipeline's promise is
    # that none of it reaches a committed file -- CI's secret scan skips
    # binaries, so whatever hid in here would be scanned by nobody.
    exif = Image.Exif()
    exif[0x010F] = "ACME Camera Corp"      # Make
    exif[0x0110] = "Model 9000"            # Model
    jpeg = io.BytesIO()
    noisy_image().save(jpeg, format="JPEG", exif=exif.tobytes())
    with Image.open(io.BytesIO(jpeg.getvalue())) as fetched:
        assert "exif" in fetched.info      # the original really carries it
        data = encode(FrameSequence.still(fetched),
                      Encode(format="webp", quality=82))
    assert has_metadata(data) == []
    with Image.open(io.BytesIO(data)) as reopened:
        assert not reopened.getexif()


def test_has_metadata_actually_detects() -> None:
    # The checker itself must be able to see metadata, or the clean answers
    # above prove nothing.
    exif = Image.Exif()
    exif[0x010F] = "ACME Camera Corp"
    tagged = io.BytesIO()
    noisy_image().save(tagged, format="WEBP", exif=exif.tobytes())
    assert "exif" in has_metadata(tagged.getvalue())


def test_lower_quality_costs_fewer_bytes() -> None:
    still = FrameSequence.still(noisy_image())
    high = encode(still, Encode(format="webp", quality=95))
    low = encode(still, Encode(format="webp", quality=30))
    assert len(low) < len(high)


def test_rgba_survives_encoding() -> None:
    array = np.zeros((16, 16, 4), dtype=np.uint8)
    array[:, :8] = (0, 200, 0, 255)
    image = Image.fromarray(array, mode="RGBA")
    data = encode(FrameSequence.still(image), Encode(format="webp", quality=82))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.mode in {"RGBA", "RGBa"}
        assert np.asarray(reopened.convert("RGBA"))[0, 15, 3] == 0


# ------------------------------------------------- the animated stills

def test_awebp_round_trips_every_frame() -> None:
    # No duration assertion: Pillow's WebP reader exposes no frame rate,
    # which is why verify refuses `expect.fps` on this format (ADR 8's
    # unevaluated-must-not-pass, applied to a codec).
    data = encode(noisy_sequence(frames=5, fps=10),
                  Encode(format="awebp", quality=80))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.format == "WEBP"
        assert reopened.n_frames == 5
    assert has_metadata(data, "awebp") == []


def test_apng_round_trips_every_frame() -> None:
    data = encode(noisy_sequence(frames=4, fps=8),
                  Encode(format="apng", quality=82))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.format == "PNG"
        assert reopened.n_frames == 4
    assert has_metadata(data, "apng") == []


# --------------------------------------------------------- the video rows

def test_webm_encodes_and_reads_back_clean() -> None:
    data = encode(noisy_sequence(frames=4, size=32, fps=8),
                  Encode(format="webm", crf=45))
    assert data[:4] == b"\x1a\x45\xdf\xa3"  # EBML magic: really a webm
    assert has_metadata(data, "webm") == []


def test_mp4_encodes_and_reads_back_clean() -> None:
    data = encode(noisy_sequence(frames=4, size=32, fps=8),
                  Encode(format="mp4", crf=40))
    assert b"ftyp" in data[:32]             # ISO BMFF box: really an mp4
    assert has_metadata(data, "mp4") == []


def test_video_metadata_check_actually_detects() -> None:
    """The video half of `has_metadata` must be able to see a tag, or its
    clean answers prove nothing -- the same control as the exif test above,
    spoken in ffmetadata."""
    import subprocess
    import tempfile
    from pathlib import Path

    import imageio_ffmpeg

    clean = encode(noisy_sequence(frames=4, size=32, fps=8),
                   Encode(format="mp4", crf=40))
    ffmpeg = imageio_ffmpeg.get_ffmpeg_exe()
    with tempfile.TemporaryDirectory() as tmp:
        source = Path(tmp) / "clean.mp4"
        tagged = Path(tmp) / "tagged.mp4"
        source.write_bytes(clean)
        subprocess.run([ffmpeg, "-v", "error", "-i", str(source),
                        "-c", "copy", "-metadata", "title=smuggled",
                        str(tagged)], check=True, capture_output=True)
        assert "title" in has_metadata(tagged.read_bytes(), "mp4")


def test_video_refuses_odd_dimensions() -> None:
    odd = FrameSequence(frames=(noisy_image(size=33),), fps=8)
    with pytest.raises(EncodeError, match="even dimensions"):
        encode(odd, Encode(format="mp4", crf=40))


def test_lower_crf_costs_more_bytes() -> None:
    sequence = noisy_sequence(frames=4, size=32)
    fine = encode(sequence, Encode(format="webm", crf=10))
    coarse = encode(sequence, Encode(format="webm", crf=60))
    assert len(coarse) < len(fine)


def test_unbuilt_format_is_refused_with_directions() -> None:
    """`apng` moved into the table (ADR 31); the refusal stays pinned on a
    format that is still fictional."""
    with pytest.raises(EncodeError, match="not built yet"):
        encode(FrameSequence.still(noisy_image()),
               Encode(format="gifv", quality=82))
