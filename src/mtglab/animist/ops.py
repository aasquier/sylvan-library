"""The transform registry: every operation a recipe may name.

`OPS` is the dispatch table and the extension point — a later phase's
procedural or depth op is one entry here plus its name in
`recipe.KNOWN_OPS`, and `tests/test_animist_ops.py` pins the two equal so
neither can drift ahead of the other.

Every op is `(image, params) -> image`, pure and deterministic: same input,
same parameters, same pixels out. Nothing stochastic lives here, which is
what lets `verify` promise anything at all about a committed file.

PIL is imported inside the functions (and under `TYPE_CHECKING` for the
annotations): this module is imported by `run.py` on installs that may not
carry the `animist` extra, and the ImportError belongs at the moment pixels
are touched, where the CLI turns it into "install the animist extra".
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Protocol

if TYPE_CHECKING:
    from PIL.Image import Image


class OpError(ValueError):
    """An op handed parameters it cannot act on."""


class OpFn(Protocol):
    def __call__(self, image: Image, params: dict[str, Any]) -> Image:
        ...


def _crop(image: Image, params: dict[str, Any]) -> Image:
    """`box: [l, t, r, b]` in pixels, or `frac_box` in fractions of the frame.

    Fractions are how a crop survives a differently sized re-fetch of the
    same photograph; pixels are for when the box was fitted against one
    specific file and says so.
    """
    box = params.get("box")
    frac = params.get("frac_box")
    if (box is None) == (frac is None):
        raise OpError("crop needs exactly one of `box` (pixels) or "
                      "`frac_box` (fractions of the frame)")
    if box is not None:
        left, top, right, bottom = (int(v) for v in box)
    else:
        assert frac is not None  # the exactly-one check above
        fl, ft, fr, fb = (float(v) for v in frac)
        left, top = round(fl * image.width), round(ft * image.height)
        right, bottom = round(fr * image.width), round(fb * image.height)
    if not (0 <= left < right <= image.width
            and 0 <= top < bottom <= image.height):
        raise OpError(f"crop box ({left}, {top}, {right}, {bottom}) does not "
                      f"fit inside {image.width}x{image.height}")
    return image.crop((left, top, right, bottom))


def _matte_green(image: Image, params: dict[str, Any]) -> Image:
    """Foliage to alpha by green dominance: `G − max(R, B)`, soft threshold.

    The ivy matte, generalised. `soft_lo`/`soft_hi` bound the ramp (a pixel
    at or below `soft_lo` dominance is transparent, at or above `soft_hi`
    fully opaque), and `fade_below_frac` fades the matte linearly to nothing
    from that fraction of the frame height down — the "sparse lower-wall
    growth" control.
    """
    import numpy as np
    from PIL import Image as PILImage

    soft_lo = float(params.get("soft_lo", 0.0))
    soft_hi = float(params.get("soft_hi", 48.0))
    if soft_hi <= soft_lo:
        raise OpError("matte_green needs soft_hi > soft_lo")
    fade = params.get("fade_below_frac")

    rgb = np.asarray(image.convert("RGB"), dtype=np.float32)
    green = rgb[:, :, 1]
    others = np.maximum(rgb[:, :, 0], rgb[:, :, 2])
    dominance = green - others
    alpha = np.clip((dominance - soft_lo) / (soft_hi - soft_lo), 0.0, 1.0)

    if fade is not None:
        frac = float(fade)
        if not 0.0 < frac < 1.0:
            raise OpError("matte_green `fade_below_frac` must be between "
                          "0 and 1")
        height = alpha.shape[0]
        start = frac * height
        rows = np.arange(height, dtype=np.float32)
        # 1.0 above the fade line, tapering linearly to 0.0 at the bottom row.
        taper = np.clip((height - 1 - rows) / max(height - 1 - start, 1.0),
                        0.0, 1.0)
        alpha *= taper[:, np.newaxis]

    out = np.dstack([rgb, alpha * 255.0]).astype(np.uint8)
    return PILImage.fromarray(out, mode="RGBA")


def _feather(image: Image, params: dict[str, Any]) -> Image:
    """Gaussian-blur the alpha channel; optionally re-steepen the ramp.

    Feathering alone halos — every edge pixel slides toward half-transparent.
    `resteepen: true` pushes the blurred ramp back toward opaque-or-not
    while keeping the smoothed *shape*, which is what "leaf edges stay
    leaf-shaped" meant.
    """
    import numpy as np
    from PIL import ImageFilter

    radius = float(params.get("radius", 1.0))
    if radius <= 0:
        raise OpError("feather needs a positive `radius`")
    rgba = image.convert("RGBA")
    alpha = rgba.getchannel("A").filter(ImageFilter.GaussianBlur(radius))
    if params.get("resteepen"):
        a = np.asarray(alpha, dtype=np.float32) / 255.0
        # A fixed smoothstep-style curve: 0.3..0.7 becomes the whole ramp.
        a = np.clip((a - 0.3) / 0.4, 0.0, 1.0)
        from PIL import Image as PILImage
        alpha = PILImage.fromarray((a * 255.0).astype(np.uint8), mode="L")
    rgba.putalpha(alpha)
    return rgba


def _mirror_tile(image: Image, params: dict[str, Any]) -> Image:
    """The image beside its own reflection, so `repeat-x` (or `-y`) is
    seamless at any viewport size. `axis: x` (default) or `y`."""
    from PIL import Image as PILImage
    from PIL.Image import Transpose

    axis = str(params.get("axis", "x"))
    if axis not in {"x", "y"}:
        raise OpError("mirror_tile `axis` must be `x` or `y`")
    if axis == "x":
        out = PILImage.new(image.mode, (image.width * 2, image.height))
        out.paste(image, (0, 0))
        out.paste(image.transpose(Transpose.FLIP_LEFT_RIGHT), (image.width, 0))
    else:
        out = PILImage.new(image.mode, (image.width, image.height * 2))
        out.paste(image, (0, 0))
        out.paste(image.transpose(Transpose.FLIP_TOP_BOTTOM), (0, image.height))
    return out


def _matte_backdrop(image: Image, params: dict[str, Any]) -> Image:
    """The studio ground to alpha, by flooding inward from the frame edges.

    The museum-plate matte. A luminance key is the obvious tool and it
    cannot work here: on the Met's bronze-and-crystal plates the metal is
    *darker* than the grey ground while the sphere is *brighter*, so any
    single threshold keeps one and eats the other. What actually separates
    subject from ground is not brightness but **connectivity** — the ground
    touches the frame edge and the object does not.

    So the seed is the border, and the fill spreads to any neighbour within
    `tolerance` of the sampled backdrop colour. A gradient-lit sweep (which
    is what a studio backdrop is) is followed the whole way down, because
    each step only compares against the backdrop's own colour rather than
    against its neighbour — a chain-tolerance fill would walk straight into
    the subject through a soft shadow.

    `tolerance` is in 0-255 channel distance and `border` is how many
    pixels of the frame edge are taken as seed.

    `soft` is what makes this usable on a real plate. With a hard
    threshold a photographed object's **cast shadow** has nowhere to go:
    on the Met fish the shadow falls at 143-167 against a ground of 192,
    so any tolerance tight enough to spare the bronze keeps the shadow as
    a grey wedge, and any tolerance wide enough to swallow it starts
    eating the highlights on the fish's scales. `soft` ramps alpha from
    fully transparent at `tolerance` to fully opaque at `tolerance +
    soft`, so the shadow fades out instead — faint where it touches the
    object, gone at its edges, which is what a contact shadow should do
    anyway.

    `enclosed` decides what happens to backdrop the flood cannot reach —
    the gap under a handle, the daylight inside a wave's arch. `keep` (the
    default) leaves it, which is the safe answer when a subject has pale
    passages of its own that a colour key would eat. `drop` removes it too,
    which is what a plate of openwork wants: on the Met's fish the arch of
    the wave frames a patch of studio grey that is not reachable from any
    edge and reads, on a table, as a hole cut in the felt.
    """
    import numpy as np
    from PIL import Image as PILImage

    tolerance = float(params.get("tolerance", 26.0))
    border = int(params.get("border", 2))
    enclosed = str(params.get("enclosed", "keep"))
    soft = float(params.get("soft", 0.0))
    if tolerance <= 0:
        raise OpError("matte_backdrop needs a positive `tolerance`")
    if border < 1:
        raise OpError("matte_backdrop needs a `border` of at least 1")
    if enclosed not in {"keep", "drop"}:
        raise OpError("matte_backdrop `enclosed` must be `keep` or `drop`")
    if soft < 0:
        raise OpError("matte_backdrop `soft` cannot be negative")

    rgba = image.convert("RGBA")
    rgb = np.asarray(rgba.convert("RGB"), dtype=np.float32)
    h, w = rgb.shape[:2]
    if border * 2 >= min(h, w):
        raise OpError("matte_backdrop `border` is wider than the image")

    # The backdrop's colour, sampled from the frame edge itself rather than
    # passed in -- the edge is the one place it is guaranteed to be.
    edge = np.concatenate([
        rgb[:border, :, :].reshape(-1, 3), rgb[-border:, :, :].reshape(-1, 3),
        rgb[:, :border, :].reshape(-1, 3), rgb[:, -border:, :].reshape(-1, 3)])
    backdrop = np.median(edge, axis=0)

    distance = np.max(np.abs(rgb - backdrop), axis=2)
    near = distance <= tolerance
    if soft > 0:
        # 0 at `tolerance`, 1 at `tolerance + soft`: how *opaque* a pixel
        # is allowed to be once the flood has reached it.
        keep = np.clip((distance - tolerance) / soft, 0.0, 1.0)
        # The flood still needs a binary notion of "passable", or a shadow
        # would halt it; a pixel is walkable while it is not fully opaque.
        near = keep < 1.0
    else:
        keep = (~near).astype(np.float32)

    # Flood from the border through `near`, by repeated dilation. Bounded:
    # it stops as soon as a pass adds nothing, and the worst case is the
    # image's own diameter.
    reached = np.zeros((h, w), dtype=bool)
    reached[:border, :] = near[:border, :]
    reached[-border:, :] = near[-border:, :]
    reached[:, :border] = near[:, :border]
    reached[:, -border:] = near[:, -border:]
    for _ in range(h + w):
        grown = reached.copy()
        grown[1:, :] |= reached[:-1, :]
        grown[:-1, :] |= reached[1:, :]
        grown[:, 1:] |= reached[:, :-1]
        grown[:, :-1] |= reached[:, 1:]
        grown &= near
        if np.array_equal(grown, reached):
            break
        reached = grown

    # Reached pixels take the ramp, which is what lets a cast shadow fade
    # out. Unreached ones keep their alpha -- unless `enclosed` is `drop`,
    # in which case a pocket that matches the backdrop on the *hard* test
    # goes too. The hard test matters: the soft band is deliberately wide
    # enough to catch a shadow, and applying that width to unreachable
    # pixels eats the highlights on the object's own surface.
    survives = np.where(reached, keep, 1.0)
    if enclosed == "drop":
        survives = np.where(~reached & (distance <= tolerance), 0.0, survives)

    existing = np.asarray(rgba.getchannel("A"), dtype=np.float32) / 255.0
    alpha = (existing * survives * 255.0).astype(np.uint8)
    rgba.putalpha(PILImage.fromarray(alpha, mode="L"))
    return rgba


def _duotone(image: Image, params: dict[str, Any]) -> Image:
    """Luminance mapped onto a three-stop colour ramp: `shadow`, `mid`,
    `light`, each a hex string.

    `color_ramp` in `motion.py` does this to a generated field; this is its
    still-image sibling, and it exists for one subject in particular. A
    monochrome museum plate carries no hue to preserve, so tinting it is
    not throwing information away — and bronze is close to the ideal case,
    being genuinely one hue with luminance variation rather than a subject
    whose colours a ramp would flatten.

    Alpha is carried through untouched, so this composes after a matte.
    """
    import numpy as np
    from PIL import Image as PILImage

    shadow = _hex(params, "shadow", (26, 18, 10))
    mid = _hex(params, "mid", (140, 110, 68))
    light = _hex(params, "light", (250, 240, 208))

    rgba = image.convert("RGBA")
    lum = np.asarray(rgba.convert("L"), dtype=np.float32)[..., None] / 255.0
    s, m, li = (np.array(c, dtype=np.float32) for c in (shadow, mid, light))
    # Two linear segments meeting at mid-grey, which is where a plate's
    # midtones actually sit.
    low = s + (m - s) * (np.clip(lum, 0.0, 0.5) / 0.5)
    high = m + (li - m) * (np.clip(lum - 0.5, 0.0, 0.5) / 0.5)
    rgb = np.where(lum < 0.5, low, high)
    out = np.dstack([np.clip(rgb, 0, 255),
                     np.asarray(rgba.getchannel("A"), dtype=np.float32)])
    return PILImage.fromarray(out.astype(np.uint8), mode="RGBA")


def _hex(params: dict[str, Any], key: str,
         default: tuple[int, int, int], op: str = "duotone",
         ) -> tuple[int, int, int]:
    value = params.get(key)
    if value is None:
        return default
    text = str(value).lstrip("#")
    if len(text) != 6:
        raise OpError(f"{op} `{key}` must be a #rrggbb colour")
    try:
        return (int(text[0:2], 16), int(text[2:4], 16), int(text[4:6], 16))
    except ValueError as exc:
        raise OpError(f"{op} `{key}` is not a hex colour") from exc


def _levels(image: Image, params: dict[str, Any]) -> Image:
    """A shadow floor that keeps the plate's own colour: every channel is
    mapped `out_black + v * (255 - out_black) / 255`, so nothing in the
    output can be darker than the named colour, a bright field barely
    moves, and the hues survive.

    The parchment's op, and the gap `duotone` could not fill. A museum
    plate carries specks a decoration must not: dust, pinholes, a fleck
    dark enough to drop text contrast below WCAG exactly where a word
    happens to land. `duotone` fixes that by construction — and throws
    away the plate's colour doing it, which is what made the first
    parchment read as monochrome. This lifts the floor and keeps the
    skin: the contrast guarantee becomes the luminance of `out_black`,
    a number a recipe can state and a test can hold.

    Alpha is carried through untouched, so it composes after a matte.
    """
    import numpy as np
    from PIL import Image as PILImage

    floor = _hex(params, "out_black", (0, 0, 0), op="levels")
    rgba = image.convert("RGBA")
    arr = np.asarray(rgba, dtype=np.float32).copy()
    lift = np.array(floor, dtype=np.float32)
    arr[..., :3] = lift + arr[..., :3] * (255.0 - lift) / 255.0
    return PILImage.fromarray(np.clip(arr, 0, 255).astype(np.uint8),
                              mode="RGBA")


def _mask_circle(image: Image, params: dict[str, Any]) -> Image:
    """Everything outside a circle to alpha. `cx`/`cy`/`r` in fractions of
    the frame's *width*, `feather` in pixels.

    The sphere matte, and deliberately geometric rather than perceptual.
    `matte_green` next door keys on colour because foliage has no edge a
    number can name; a photographed crystal ball does — it is a circle, and
    the photographer put it somewhere specific in the frame. Keying a glass
    sphere on its colour is the one thing that cannot work, since the whole
    subject is the background seen through it: a chroma matte would cut the
    ball out and leave the room.

    `cx` is a fraction of the width and `cy` a fraction of the height, so a
    centre means the same place it would in CSS; `r` is a fraction of the
    **width in both axes**, which is what keeps the mask a circle rather
    than an ellipse on a non-square frame. The antialiased edge is computed
    from the distance field rather than drawn, so the result does not
    depend on Pillow's polygon rasteriser.
    """
    import numpy as np
    from PIL import Image as PILImage

    cx = float(params.get("cx", 0.5))
    cy = float(params.get("cy", 0.5))
    r = float(params.get("r", 0.5))
    feather = float(params.get("feather", 1.0))
    if r <= 0:
        raise OpError("mask_circle needs a positive `r`")
    if feather < 0:
        raise OpError("mask_circle `feather` cannot be negative")

    rgba = image.convert("RGBA")
    w, h = rgba.width, rgba.height
    # The centre is placed in its own axis; the radius is the width's in
    # both, so the mask is round and not oval.
    px_cx, px_cy, px_r = cx * w, cy * h, r * w
    ys, xs = np.mgrid[0:h, 0:w].astype(np.float32)
    dist = np.hypot(xs - px_cx, ys - px_cy)
    if feather == 0:
        mask = (dist <= px_r).astype(np.float32)
    else:
        # 1 inside, 0 outside, one feather-wide ramp straddling the edge.
        mask = np.clip((px_r - dist) / feather + 0.5, 0.0, 1.0)
    existing = np.asarray(rgba.getchannel("A"), dtype=np.float32) / 255.0
    alpha = (existing * mask * 255.0).astype(np.uint8)
    rgba.putalpha(PILImage.fromarray(alpha, mode="L"))
    return rgba


def _resize(image: Image, params: dict[str, Any]) -> Image:
    """`width` (aspect kept) and/or `height`. Lanczos, always — the one
    resampler that neither softens linework nor invents ringing at these
    scales, and using exactly one keeps the output deterministic."""
    from PIL.Image import Resampling

    width = params.get("width")
    height = params.get("height")
    if width is None and height is None:
        raise OpError("resize needs `width`, `height`, or both")
    if width is not None and height is not None:
        size = (int(width), int(height))
    elif width is not None:
        w = int(width)
        size = (w, max(1, round(image.height * w / image.width)))
    else:
        assert height is not None  # the neither-case raised above
        h = int(height)
        size = (max(1, round(image.width * h / image.height)), h)
    return image.resize(size, Resampling.LANCZOS)


#: The registry. `recipe.KNOWN_OPS` is the schema's copy of these keys, and
#: the test that pins them equal is what lets the schema refuse an op the
#: registry cannot run.
OPS: dict[str, OpFn] = {
    "crop": _crop,
    "matte_green": _matte_green,
    "matte_backdrop": _matte_backdrop,
    "duotone": _duotone,
    "levels": _levels,
    "mask_circle": _mask_circle,
    "feather": _feather,
    "mirror_tile": _mirror_tile,
    "resize": _resize,
}


def apply(image: Image, name: str, params: dict[str, Any]) -> Image:
    op = OPS.get(name)
    if op is None:
        raise OpError(f"unknown op {name!r} (one of: {', '.join(sorted(OPS))})")
    return op(image, params)
