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
    "feather": _feather,
    "mirror_tile": _mirror_tile,
    "resize": _resize,
}


def apply(image: Image, name: str, params: dict[str, Any]) -> Image:
    op = OPS.get(name)
    if op is None:
        raise OpError(f"unknown op {name!r} (one of: {', '.join(sorted(OPS))})")
    return op(image, params)
