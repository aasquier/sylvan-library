"""Every transform, on tiny synthesized images with known pixels."""

from __future__ import annotations

from itertools import pairwise

import numpy as np
import pytest
from PIL import Image

from animist.ops import OPS, OpError, apply
from animist.recipe import KNOWN_OPS


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


def plate() -> Image.Image:
    """A museum plate in miniature: a grey ground lit unevenly, a DARK bar
    and a BRIGHT bar sitting on it. The bright bar is the whole point —
    it is the crystal sphere, and it is brighter than the ground."""
    array = np.zeros((40, 40, 3))
    # A backdrop sweep: 196 at the top down to 176 at the bottom.
    for y in range(40):
        array[y, :, :] = 196 - y * 0.5
    array[12:20, 8:32, :] = 40      # the bronze
    array[24:32, 8:32, :] = 250     # the crystal
    return rgb(array)


def test_matte_backdrop_drops_the_ground_and_keeps_both_subjects() -> None:
    out = apply(plate(), "matte_backdrop", {"tolerance": 26, "border": 1})
    alpha = np.asarray(out.getchannel("A"))
    assert alpha[0, 0] == 0, "the corner is backdrop"
    assert alpha[5, 20] == 0, "so is the space above the object"
    assert alpha[16, 20] == 255, "the dark bar survives"
    # The one a luminance key gets wrong: brighter than the ground, kept.
    assert alpha[28, 20] == 255, "the bright bar survives"


def test_matte_backdrop_follows_a_lit_gradient_to_the_far_edge() -> None:
    """A studio sweep is 20 levels darker at the bottom than the top. The
    fill must still reach it, or the matte leaves a band along one edge."""
    out = apply(plate(), "matte_backdrop", {"tolerance": 26, "border": 1})
    alpha = np.asarray(out.getchannel("A"))
    assert alpha[39, 2] == 0, "the far corner of the sweep is still backdrop"
    assert alpha[39, 20] == 0


def test_matte_backdrop_keeps_background_it_cannot_reach() -> None:
    """An enclosed hole is not removed, and that is the documented rule."""
    array = np.full((30, 30, 3), 200.0)
    array[8:22, 8:22, :] = 30        # a ring of object...
    array[13:17, 13:17, :] = 200     # ...around a pocket of backdrop
    out = apply(rgb(array), "matte_backdrop", {"tolerance": 20, "border": 1})
    alpha = np.asarray(out.getchannel("A"))
    assert alpha[0, 0] == 0, "the reachable ground goes"
    assert alpha[15, 15] == 255, "the enclosed pocket stays"


def test_matte_backdrop_can_drop_what_it_cannot_reach() -> None:
    """`enclosed: drop` for openwork — the Met fish frames studio grey in
    the arch of its wave, and on a table that reads as a hole in the felt."""
    array = np.full((30, 30, 3), 200.0)
    array[8:22, 8:22, :] = 30
    array[13:17, 13:17, :] = 200
    out = apply(rgb(array), "matte_backdrop",
                {"tolerance": 20, "border": 1, "enclosed": "drop"})
    alpha = np.asarray(out.getchannel("A"))
    assert alpha[0, 0] == 0
    assert alpha[15, 15] == 0, "the enclosed pocket goes too"
    assert alpha[10, 10] == 255, "the object is untouched"


def test_matte_backdrop_refusals() -> None:
    with pytest.raises(OpError, match="positive `tolerance`"):
        apply(plate(), "matte_backdrop", {"tolerance": 0})
    with pytest.raises(OpError, match="at least 1"):
        apply(plate(), "matte_backdrop", {"border": 0})
    with pytest.raises(OpError, match="wider than the image"):
        apply(plate(), "matte_backdrop", {"border": 40})
    with pytest.raises(OpError, match="`keep` or `drop`"):
        apply(plate(), "matte_backdrop", {"enclosed": "maybe"})


def test_duotone_maps_luminance_onto_the_ramp() -> None:
    array = np.zeros((3, 1, 3))
    array[0] = 0        # black
    array[1] = 128      # mid
    array[2] = 255      # white
    out = apply(rgb(array), "duotone",
                {"shadow": "#000000", "mid": "#808080", "light": "#ffffff"})
    got = np.asarray(out.convert("RGB"))
    assert got[0, 0].tolist() == [0, 0, 0]
    assert all(abs(int(v) - 128) <= 2 for v in got[1, 0])
    assert got[2, 0].tolist() == [255, 255, 255]


def test_duotone_actually_tints() -> None:
    """The point of the op: a grey plate comes out warm."""
    grey = rgb(np.full((4, 4, 3), 180.0))
    out = np.asarray(apply(grey, "duotone",
                           {"shadow": "#261a0e", "mid": "#8c6e44",
                            "light": "#faf0d0"}).convert("RGB"))
    r, g, b = (int(v) for v in out[2, 2])
    assert r > g > b, f"expected a warm cast, got {(r, g, b)}"


def test_duotone_carries_alpha_through() -> None:
    """So it composes after a matte instead of undoing one."""
    base = Image.new("RGBA", (4, 4), (180, 180, 180, 90))
    out = apply(base, "duotone", {})
    assert np.asarray(out.getchannel("A"))[2, 2] == 90


def test_duotone_refuses_a_bad_colour() -> None:
    grey = rgb(np.full((4, 4, 3), 180.0))
    with pytest.raises(OpError, match="#rrggbb"):
        apply(grey, "duotone", {"mid": "abc"})
    with pytest.raises(OpError, match="not a hex colour"):
        apply(grey, "duotone", {"mid": "zzzzzz"})


def test_levels_lifts_black_exactly_to_the_floor() -> None:
    """The op's guarantee: nothing darker than `out_black` survives."""
    array = np.zeros((3, 1, 3))
    array[0] = 0        # black -> exactly the floor
    array[1] = 255      # white stays white
    array[2] = 128
    out = np.asarray(apply(rgb(array), "levels",
                           {"out_black": "#b1945f"}).convert("RGB"))
    assert out[0, 0].tolist() == [0xb1, 0x94, 0x5f]
    assert out[1, 0].tolist() == [255, 255, 255]
    assert (out[2, 0] >= np.array([0xb1, 0x94, 0x5f])).all()


def test_levels_keeps_the_plates_own_colour() -> None:
    """The reason it exists instead of a duotone: hue survives."""
    warm = rgb(np.full((4, 4, 3), 0.0) + np.array([200.0, 160.0, 90.0]))
    out = np.asarray(apply(warm, "levels",
                           {"out_black": "#202020"}).convert("RGB"))
    r, g, b = (int(v) for v in out[2, 2])
    assert r > g > b, f"expected the warm cast kept, got {(r, g, b)}"


def test_levels_carries_alpha_through() -> None:
    base = Image.new("RGBA", (4, 4), (10, 10, 10, 90))
    out = apply(base, "levels", {"out_black": "#402000"})
    assert np.asarray(out.getchannel("A"))[2, 2] == 90


def test_levels_refuses_a_bad_colour() -> None:
    grey = rgb(np.full((4, 4, 3), 180.0))
    with pytest.raises(OpError, match="levels `out_black` must be"):
        apply(grey, "levels", {"out_black": "abc"})


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
