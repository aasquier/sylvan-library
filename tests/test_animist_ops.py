"""Every transform, on tiny synthesized images with known pixels."""

from __future__ import annotations

from itertools import pairwise

import numpy as np
import pytest
from PIL import Image

from mtglab.animist.ops import OPS, OpError, apply
from mtglab.animist.recipe import KNOWN_OPS


def test_registry_matches_the_schema_vocabulary() -> None:
    # The SIMULATOR_KEYS seam: the schema refuses ops by `KNOWN_OPS`, the
    # registry runs them by `OPS`, and this is the line that keeps a renamed
    # op from validating in a recipe it can no longer run in.
    assert set(OPS) == set(KNOWN_OPS)


def rgb(array: np.ndarray) -> Image.Image:
    return Image.fromarray(array.astype(np.uint8), mode="RGB")


def green_field(width: int = 12, height: int = 8) -> Image.Image:
    """Green on the left half, red on the right."""
    array = np.zeros((height, width, 3))
    array[:, : width // 2, 1] = 200
    array[:, width // 2:, 0] = 200
    return rgb(array)


def test_crop_by_box_and_by_fraction() -> None:
    image = green_field(12, 8)
    assert apply(image, "crop", {"box": [2, 1, 10, 7]}).size == (8, 6)
    assert apply(image, "crop",
                 {"frac_box": [0, 0, 0.5, 1]}).size == (6, 8)


def test_crop_refusals() -> None:
    image = green_field()
    with pytest.raises(OpError, match="exactly one"):
        apply(image, "crop", {})
    with pytest.raises(OpError, match="exactly one"):
        apply(image, "crop", {"box": [0, 0, 1, 1], "frac_box": [0, 0, 1, 1]})
    with pytest.raises(OpError, match="does not fit"):
        apply(image, "crop", {"box": [0, 0, 99, 99]})


def test_matte_green_keeps_green_and_drops_red() -> None:
    matted = apply(green_field(), "matte_green", {})
    alpha = np.asarray(matted.getchannel("A"))
    assert matted.mode == "RGBA"
    assert (alpha[:, :6] == 255).all()     # green half fully opaque
    assert (alpha[:, 6:] == 0).all()       # red half fully transparent


def test_matte_green_fade_below_frac_tapers_the_bottom() -> None:
    array = np.zeros((10, 4, 3))
    array[:, :, 1] = 200                   # all green
    matted = apply(rgb(array), "matte_green", {"fade_below_frac": 0.5})
    alpha = np.asarray(matted.getchannel("A")).astype(int)
    assert (alpha[0] == 255).all()         # top untouched
    assert (alpha[-1] == 0).all()          # bottom row gone
    column = alpha[:, 0]
    assert all(a >= b for a, b in pairwise(column))  # monotonic taper


def test_matte_green_refusals() -> None:
    with pytest.raises(OpError, match="soft_hi > soft_lo"):
        apply(green_field(), "matte_green", {"soft_lo": 5, "soft_hi": 5})
    with pytest.raises(OpError, match="between"):
        apply(green_field(), "matte_green", {"fade_below_frac": 1.5})


def hard_edge() -> Image.Image:
    """RGBA: fully opaque left half, fully transparent right half."""
    array = np.zeros((8, 12, 4))
    array[:, :, 1] = 200
    array[:, :6, 3] = 255
    return Image.fromarray(array.astype(np.uint8), mode="RGBA")


def test_feather_smooths_the_edge() -> None:
    feathered = apply(hard_edge(), "feather", {"radius": 1.5})
    alpha = np.asarray(feathered.getchannel("A"))
    # The hard 255/0 cliff now has intermediate values near the seam.
    assert ((alpha > 0) & (alpha < 255)).any()


def test_resteepen_pushes_back_toward_the_extremes() -> None:
    soft = apply(hard_edge(), "feather", {"radius": 1.5})
    steep = apply(hard_edge(), "feather", {"radius": 1.5, "resteepen": True})
    soft_mid = (((np.asarray(soft.getchannel("A")) > 32)
                 & (np.asarray(soft.getchannel("A")) < 224)).sum())
    steep_mid = (((np.asarray(steep.getchannel("A")) > 32)
                  & (np.asarray(steep.getchannel("A")) < 224)).sum())
    assert steep_mid < soft_mid


def test_feather_needs_positive_radius() -> None:
    with pytest.raises(OpError, match="positive"):
        apply(hard_edge(), "feather", {"radius": 0})


def test_mask_circle_keeps_the_disc_and_drops_the_corners() -> None:
    # A square field: the centred disc survives, the corners do not.
    image = Image.new("RGB", (40, 40), (200, 180, 120))
    out = apply(image, "mask_circle", {"cx": 0.5, "cy": 0.5, "r": 0.4,
                                       "feather": 0})
    alpha = np.asarray(out.getchannel("A"))
    assert alpha[20, 20] == 255          # dead centre
    assert alpha[20, 3] == 0             # outside the radius, same row
    assert alpha[0, 0] == 0              # every corner
    assert alpha[0, 39] == 0
    assert alpha[39, 0] == 0
    assert alpha[39, 39] == 0
    # The RGB is untouched -- this op moves alpha and nothing else, which is
    # what lets the glass keep the room it was photographed in.
    assert np.asarray(out.convert("RGB"))[20, 20].tolist() == [200, 180, 120]


def test_mask_circle_stays_round_on_a_wide_frame() -> None:
    # `r` measures against the width in both axes. On a 2:1 frame a radius
    # keyed to height would be an ellipse; this asserts it is not.
    image = Image.new("RGB", (80, 40), (255, 255, 255))
    out = apply(image, "mask_circle", {"cx": 0.5, "cy": 0.25, "r": 0.25,
                                       "feather": 0})
    alpha = np.asarray(out.getchannel("A"))
    centre_x, centre_y, radius = 40, 10, 20
    for dx, dy in ((radius - 2, 0), (0, radius - 2), (-(radius - 2), 0)):
        assert alpha[centre_y + dy, centre_x + dx] == 255, (dx, dy)
    for dx, dy in ((radius + 2, 0), (0, radius + 2)):
        assert alpha[centre_y + dy, centre_x + dx] == 0, (dx, dy)


def test_mask_circle_feathers_across_the_edge() -> None:
    image = Image.new("RGB", (40, 40), (255, 255, 255))
    out = apply(image, "mask_circle", {"cx": 0.5, "cy": 0.5, "r": 0.4,
                                       "feather": 4})
    alpha = np.asarray(out.getchannel("A")).astype(int)
    # Walking out along the centre row, the ramp is monotonically decreasing
    # and passes through the middle rather than jumping.
    walk = alpha[20, 20:40]
    assert all(a >= b for a, b in pairwise(walk))
    assert any(20 < value < 235 for value in walk)


def test_mask_circle_multiplies_an_alpha_that_is_already_there() -> None:
    # Order matters in a recipe: a feather before the mask must survive it.
    image = Image.new("RGBA", (40, 40), (255, 255, 255, 128))
    out = apply(image, "mask_circle", {"cx": 0.5, "cy": 0.5, "r": 0.4,
                                       "feather": 0})
    assert np.asarray(out.getchannel("A"))[20, 20] == 128


def test_mask_circle_refusals() -> None:
    image = Image.new("RGB", (10, 10), (0, 0, 0))
    with pytest.raises(OpError, match="positive `r`"):
        apply(image, "mask_circle", {"r": 0})
    with pytest.raises(OpError, match="negative"):
        apply(image, "mask_circle", {"r": 0.5, "feather": -1})


def test_mirror_tile_is_seamless() -> None:
    image = green_field(6, 4)
    tiled = apply(image, "mirror_tile", {"axis": "x"})
    assert tiled.size == (12, 4)
    array = np.asarray(tiled)
    # The right half is the left half flipped, so the tile's outer columns
    # match and `repeat-x` shows no seam.
    assert (array[:, :6] == array[:, 11:5:-1]).all()

    tall = apply(image, "mirror_tile", {"axis": "y"})
    assert tall.size == (6, 8)
    with pytest.raises(OpError, match="`x` or `y`"):
        apply(image, "mirror_tile", {"axis": "z"})


def test_resize_keeps_aspect_from_either_side() -> None:
    image = green_field(12, 8)
    assert apply(image, "resize", {"width": 6}).size == (6, 4)
    assert apply(image, "resize", {"height": 4}).size == (6, 4)
    assert apply(image, "resize", {"width": 5, "height": 9}).size == (5, 9)
    with pytest.raises(OpError, match="width"):
        apply(image, "resize", {})


def test_apply_refuses_an_unknown_op() -> None:
    with pytest.raises(OpError, match="unknown op"):
        apply(green_field(), "sharpen", {})
