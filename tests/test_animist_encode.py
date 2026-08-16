"""Encoding: clean WebP out, metadata provably gone, quality that bites."""

from __future__ import annotations

import io

import numpy as np
import pytest
from PIL import Image

from mtglab.animist.encode import EncodeError, encode, has_metadata
from mtglab.animist.recipe import Encode


def noisy_image(seed: int = 7, size: int = 64) -> Image.Image:
    rng = np.random.default_rng(seed)
    return Image.fromarray(
        rng.integers(0, 255, (size, size, 3), dtype=np.uint8), mode="RGB")


def test_webp_round_trips_at_the_right_size() -> None:
    data = encode(noisy_image(), Encode(format="webp", quality=82))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.format == "WEBP"
        assert reopened.size == (64, 64)


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
        data = encode(fetched, Encode(format="webp", quality=82))
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
    image = noisy_image()
    high = encode(image, Encode(format="webp", quality=95))
    low = encode(image, Encode(format="webp", quality=30))
    assert len(low) < len(high)


def test_rgba_survives_encoding() -> None:
    array = np.zeros((16, 16, 4), dtype=np.uint8)
    array[:, :8] = (0, 200, 0, 255)
    image = Image.fromarray(array, mode="RGBA")
    data = encode(image, Encode(format="webp", quality=82))
    with Image.open(io.BytesIO(data)) as reopened:
        assert reopened.mode in {"RGBA", "RGBa"}
        assert np.asarray(reopened.convert("RGBA"))[0, 15, 3] == 0


def test_unbuilt_format_is_refused_with_directions() -> None:
    with pytest.raises(EncodeError, match="not built yet"):
        encode(noisy_image(), Encode(format="apng", quality=82))
