"""The size curve and its knee. The knee is pure arithmetic; the curve is not."""

from __future__ import annotations

import numpy as np
import pytest
from PIL import Image

from mtglab.animist.measure import knee, size_curve
from mtglab.animist.recipe import Encode


def test_knee_finds_the_elbow() -> None:
    # Steep saving from 100 to 200, nearly flat after: the elbow is 200 --
    # the shape of the tarot decision (560px dear, 340px pointless).
    curve = [(100, 30_000), (200, 9_000), (300, 8_200), (400, 7_800)]
    assert knee(curve) == 200


def test_knee_is_order_independent_and_deterministic() -> None:
    curve = [(400, 7_800), (100, 30_000), (300, 8_200), (200, 9_000)]
    assert knee(curve) == knee(sorted(curve)) == 200


def test_knee_needs_three_points() -> None:
    with pytest.raises(ValueError, match="three"):
        knee([(100, 1000), (200, 500)])


def test_size_curve_grows_with_width() -> None:
    rng = np.random.default_rng(11)
    image = Image.fromarray(
        rng.integers(0, 255, (60, 90, 3), dtype=np.uint8), mode="RGB")
    curve = size_curve(image, Encode(format="webp", quality=82),
                       [30, 60, 45, 30])          # duplicates collapse
    widths = [w for w, _ in curve]
    sizes = [s for _, s in curve]
    assert widths == [30, 45, 60]
    assert sizes == sorted(sizes)                  # more pixels, more bytes
    assert all(size > 0 for size in sizes)
