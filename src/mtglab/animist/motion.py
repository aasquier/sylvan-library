"""Frames, and the ops that make or move them (ADR 31).

`ops.py` is one frame in, one frame out. Motion needs a second shape — a
sequence of frames with a rate — and this module owns it, plus two new op
protocols over it: a **generator** makes a `FrameSequence` from parameters
alone (the procedural source's whole point), and a **motion op** transforms
one sequence into another. The still registry is not duplicated: `run.py`
lifts any `ops.py` op over a sequence by mapping it frame by frame.

The purity doctrine survives intact, and the wording matters. `ops.py` says
"nothing stochastic lives here"; what that sentence was protecting is that
`verify` can hold a committed file to its recipe, and the protection is
determinism, not the absence of randomness. Every stochastic-looking op below
is a pure function of an explicit integer `seed` — same seed, same params,
same frames — and the loader refuses a recipe that omits one, so a committed
asset's derivation is still written down entire.

Two properties are load-bearing for background loops and are established by
construction rather than post-processing:

- **`spectral_noise` is periodic in x, y, and t.** The field is synthesised
  in the frequency domain over integer cycle counts, so frame N-1 flows into
  frame 0 exactly and the texture tiles at its own edges. No crossfade, no
  seam, nothing to hide.
- **`advect` preserves that periodicity.** Its internal flow field comes from
  the same spectral machinery, so displacing a periodic sequence yields a
  periodic sequence.

numpy is a core dependency; PIL enters inside functions, as everywhere else
in the package.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, Protocol

from mtglab.animist.ops import OpError

if TYPE_CHECKING:
    import numpy as np
    from numpy.typing import NDArray
    from PIL.Image import Image


@dataclass(frozen=True)
class FrameSequence:
    """One or more frames of identical size, and the rate they play at.

    A still image is the one-frame case (`still()`), which is what lets the
    encode table treat every format uniformly: a `webp` encoder takes the
    only frame, a `webm` encoder takes them all.
    """

    frames: tuple[Image, ...]
    fps: int

    def __post_init__(self) -> None:
        if not self.frames:
            raise OpError("a FrameSequence needs at least one frame")
        if self.fps < 1:
            raise OpError("a FrameSequence needs fps >= 1")
        sizes = {frame.size for frame in self.frames}
        if len(sizes) > 1:
            raise OpError(f"frames disagree about their size: {sorted(sizes)}")

    @classmethod
    def still(cls, image: Image) -> FrameSequence:
        """Lift one image into the sequence algebra. fps=1 is a placeholder
        no still encoder reads."""
        return cls(frames=(image,), fps=1)

    @property
    def size(self) -> tuple[int, int]:
        return self.frames[0].size

    @property
    def is_still(self) -> bool:
        return len(self.frames) == 1


class GeneratorOpFn(Protocol):
    """params -> frames. The first op of a procedural output."""

    def __call__(self, params: dict[str, Any]) -> FrameSequence:
        ...


class MotionOpFn(Protocol):
    """frames -> frames."""

    def __call__(self, sequence: FrameSequence,
                 params: dict[str, Any]) -> FrameSequence:
        ...


#: Ops that must be handed a `seed` — by their own params or, in a recipe,
#: injected from the procedural source's. `recipe.py` refuses a derivation
#: that would leave one unseeded, which is what keeps the purity claim above
#: checkable rather than aspirational.
SEEDED_OPS = frozenset({"spectral_noise", "advect"})


def _require_int(params: dict[str, Any], key: str, op: str,
                 minimum: int = 1) -> int:
    value = params.get(key)
    if not isinstance(value, int) or value < minimum:
        raise OpError(f"{op} needs `{key}`, an integer >= {minimum}")
    return value


def _spectral_field(seed: int, width: int, height: int, frames: int, *,
                    scale: float, time_scale: float,
                    falloff: float) -> NDArray[np.float32]:
    """A (frames, height, width) float32 field, periodic on every axis.

    Synthesised in the frequency domain: seeded random phases, amplitude
    falling off with combined spatial/temporal frequency, inverse FFT. The
    frequencies are integer cycle counts, which is the whole periodicity
    argument — there is nothing at the edges to blend because the function
    wraps by construction.
    """
    import numpy as np

    rng = np.random.default_rng(seed)
    shape = (frames, height, width)

    kt = np.fft.fftfreq(frames, d=1.0 / frames).reshape(-1, 1, 1)
    ky = np.fft.fftfreq(height, d=1.0 / height).reshape(1, -1, 1)
    kx = np.fft.fftfreq(width, d=1.0 / width).reshape(1, 1, -1)

    spatial = np.sqrt((kx / max(scale, 1e-6)) ** 2
                      + (ky / max(scale, 1e-6)) ** 2)
    temporal = np.abs(kt) / max(time_scale, 1e-6)
    radius = np.sqrt(spatial ** 2 + temporal ** 2)
    amplitude = 1.0 / (1.0 + radius ** falloff)
    amplitude[0, 0, 0] = 0.0  # no DC term; the mean is added back by the ramp

    phases = rng.uniform(0.0, 2.0 * np.pi, size=shape)
    spectrum = (amplitude * np.exp(1j * phases)).astype(np.complex64)
    field = np.fft.ifftn(spectrum).real.astype(np.float32)

    lo, hi = float(field.min()), float(field.max())
    if hi - lo < 1e-12:
        return np.zeros(shape, dtype=np.float32)
    out: NDArray[np.float32] = (field - lo) / (hi - lo)
    return out


def _spectral_noise(params: dict[str, Any]) -> FrameSequence:
    """Seeded, loop-perfect, tileable grayscale noise.

    `scale` is how many noise features span the frame (higher = finer grain —
    parchment at 60, mist at 3), `time_scale` how many temporal cycles fit in
    one loop (1.0 drifts once around per loop), `falloff` the spectral
    exponent (2.0 is cloudlike; lower is harsher). Colour is a separate op
    (`color_ramp`), because a palette is a decision and noise is not.
    """
    import numpy as np
    from PIL import Image as PILImage

    seed = _require_int(params, "seed", "spectral_noise", minimum=0)
    width = _require_int(params, "width", "spectral_noise")
    height = _require_int(params, "height", "spectral_noise")
    frames = _require_int(params, "frames", "spectral_noise")
    fps = _require_int(params, "fps", "spectral_noise")
    scale = float(params.get("scale", 4.0))
    time_scale = float(params.get("time_scale", 1.0))
    falloff = float(params.get("falloff", 2.0))
    if falloff <= 0:
        raise OpError("spectral_noise needs a positive `falloff`")

    field = _spectral_field(seed, width, height, frames, scale=scale,
                            time_scale=time_scale, falloff=falloff)
    grays = (field * 255.0).astype(np.uint8)
    images = tuple(PILImage.fromarray(grays[i], mode="L")
                   for i in range(frames))
    return FrameSequence(frames=images, fps=fps)


def _bilinear_wrap(channel: NDArray[np.float32], sample_y: NDArray[np.float32],
                   sample_x: NDArray[np.float32], *,
                   clamp: bool = False) -> NDArray[np.float32]:
    """Sample `channel` at fractional coordinates — numpy only, so nothing
    here adds a scipy dependency. Edges wrap by default (the periodic
    textures); `clamp=True` pins them instead, for imagery like a painting
    whose left edge must never bleed in from its right."""
    import numpy as np

    height, width = channel.shape
    if clamp:
        sample_y = np.clip(sample_y, 0.0, height - 1.0)
        sample_x = np.clip(sample_x, 0.0, width - 1.0)
    y0 = np.floor(sample_y).astype(np.int64)
    x0 = np.floor(sample_x).astype(np.int64)
    fy = (sample_y - y0).astype(np.float32)
    fx = (sample_x - x0).astype(np.float32)
    if clamp:
        y0 = np.clip(y0, 0, height - 1)
        x0 = np.clip(x0, 0, width - 1)
        y1 = np.clip(y0 + 1, 0, height - 1)
        x1 = np.clip(x0 + 1, 0, width - 1)
    else:
        y0 %= height
        x0 %= width
        y1 = (y0 + 1) % height
        x1 = (x0 + 1) % width

    import numpy as _np

    top = channel[y0, x0] * (1 - fx) + channel[y0, x1] * fx
    bottom = channel[y1, x0] * (1 - fx) + channel[y1, x1] * fx
    out: NDArray[np.float32] = (top * (1 - fy) + bottom * fy) \
        .astype(_np.float32)
    return out


def _advect(sequence: FrameSequence, params: dict[str, Any]) -> FrameSequence:
    """Displace every frame along a seeded, divergence-free flow field.

    The flow is the curl of a spectral potential — curl because it swirls
    without sources or sinks, which reads as weather rather than as a drain.
    The potential is periodic in x, y and t, so a loop-perfect input stays
    loop-perfect. `strength` is the maximum displacement in pixels.
    """
    import numpy as np
    from PIL import Image as PILImage

    seed = _require_int(params, "seed", "advect", minimum=0)
    strength = float(params.get("strength", 8.0))
    scale = float(params.get("scale", 2.0))
    time_scale = float(params.get("time_scale", 1.0))
    if strength <= 0:
        raise OpError("advect needs a positive `strength`")

    width, height = sequence.size
    count = len(sequence.frames)
    potential = _spectral_field(seed, width, height, count, scale=scale,
                                time_scale=time_scale, falloff=2.0)

    # Periodic curl: (d/dy, -d/dx) via wrapped central differences.
    dy = (np.roll(potential, -1, axis=1) - np.roll(potential, 1, axis=1)) / 2.0
    dx = (np.roll(potential, -1, axis=2) - np.roll(potential, 1, axis=2)) / 2.0
    flow_x, flow_y = dy, -dx
    peak = float(max(np.abs(flow_x).max(), np.abs(flow_y).max(), 1e-12))
    flow_x = flow_x / peak * strength
    flow_y = flow_y / peak * strength

    ys, xs = np.meshgrid(np.arange(height, dtype=np.float32),
                         np.arange(width, dtype=np.float32), indexing="ij")
    out_frames = []
    for i, frame in enumerate(sequence.frames):
        array = np.asarray(frame, dtype=np.float32)
        sample_y = ys + flow_y[i]
        sample_x = xs + flow_x[i]
        if array.ndim == 2:
            moved = _bilinear_wrap(array, sample_y, sample_x)
        else:
            moved = np.dstack([
                _bilinear_wrap(np.ascontiguousarray(array[:, :, c]),
                               sample_y, sample_x)
                for c in range(array.shape[2])])
        out = np.clip(moved, 0.0, 255.0).astype(np.uint8)
        out_frames.append(PILImage.fromarray(out, mode=frame.mode))
    return FrameSequence(frames=tuple(out_frames), fps=sequence.fps)


def _parse_hex(value: Any, key: str) -> tuple[int, int, int]:
    if not isinstance(value, str) or not value.startswith("#") \
            or len(value) != 7:
        raise OpError(f"color_ramp `{key}` must be a `#rrggbb` hex colour")
    try:
        return (int(value[1:3], 16), int(value[3:5], 16), int(value[5:7], 16))
    except ValueError as exc:
        raise OpError(f"color_ramp `{key}` is not valid hex: {value!r}") \
            from exc


def _color_ramp(sequence: FrameSequence,
                params: dict[str, Any]) -> FrameSequence:
    """Map luminance through a two-colour gradient: `low` and `high` hex.

    This is where grayscale noise becomes mist, mana, or candlelight.
    `gamma` curves the luminance first (default 1.0, linear): above 1 sinks
    the midtones toward `low`, which is how a full-field cloud becomes
    sparse filaments with dark air between them — a wisp is mostly the
    space around it. `alpha_from_luma: true` writes luminance into an alpha
    channel too, for the still formats — video encodes drop alpha at
    yuv420p anyway.
    """
    import numpy as np
    from PIL import Image as PILImage

    low = _parse_hex(params.get("low", "#000000"), "low")
    high = _parse_hex(params.get("high", "#ffffff"), "high")
    with_alpha = bool(params.get("alpha_from_luma", False))
    gamma = float(params.get("gamma", 1.0))
    if gamma <= 0:
        raise OpError("color_ramp `gamma` must be positive")

    low_arr = np.array(low, dtype=np.float32)
    high_arr = np.array(high, dtype=np.float32)
    out_frames = []
    for frame in sequence.frames:
        luma = np.asarray(frame.convert("L"), dtype=np.float32) / 255.0
        if gamma != 1.0:
            luma = np.power(luma, gamma)
        rgb = (low_arr + (high_arr - low_arr) * luma[:, :, np.newaxis])
        if with_alpha:
            alpha = (luma * 255.0)[:, :, np.newaxis]
            stacked = np.concatenate([rgb, alpha], axis=2)
            image = PILImage.fromarray(
                np.clip(stacked, 0, 255).astype(np.uint8), mode="RGBA")
        else:
            image = PILImage.fromarray(
                np.clip(rgb, 0, 255).astype(np.uint8), mode="RGB")
        out_frames.append(image)
    return FrameSequence(frames=tuple(out_frames), fps=sequence.fps)


def _smoothstep(t: NDArray[np.float32]) -> NDArray[np.float32]:
    out: NDArray[np.float32] = t * t * (3.0 - 2.0 * t)
    return out


def _ken_burns(sequence: FrameSequence,
               params: dict[str, Any]) -> FrameSequence:
    """A still painting gains a slow camera: zoom and pan over N frames.

    Takes a single-frame sequence (a fetched image, lifted) and produces the
    drift. `bounce: true` runs the path there and back, which is what makes
    the result loop; without it the clip ends where it ends. Deterministic —
    a camera path has no randomness to seed.
    """
    import numpy as np
    from PIL.Image import Resampling

    if not sequence.is_still:
        raise OpError("ken_burns takes a still (one frame); it creates the "
                      "timeline itself")
    frames = _require_int(params, "frames", "ken_burns", minimum=2)
    fps = _require_int(params, "fps", "ken_burns")
    zoom_from = float(params.get("zoom_from", 1.0))
    zoom_to = float(params.get("zoom_to", 1.08))
    if min(zoom_from, zoom_to) < 1.0:
        raise OpError("ken_burns zoom factors must be >= 1.0 -- zooming out "
                      "past the frame would show the void beyond the canvas")
    pan_from = tuple(float(v) for v in params.get("pan_from", (0.5, 0.5)))
    pan_to = tuple(float(v) for v in params.get("pan_to", (0.5, 0.5)))
    bounce = bool(params.get("bounce", False))

    image = sequence.frames[0]
    width, height = image.size
    t = np.linspace(0.0, 1.0, frames, dtype=np.float32)
    if bounce:
        # 0 -> 1 -> 0 over the clip, so frame N-1 sits beside frame 0.
        t = (1.0 - np.abs(1.0 - 2.0 * t)).astype(np.float32)
    eased = _smoothstep(t)

    out_frames = []
    for k in range(frames):
        a = float(eased[k])
        zoom = zoom_from + (zoom_to - zoom_from) * a
        cx = (pan_from[0] + (pan_to[0] - pan_from[0]) * a) * width
        cy = (pan_from[1] + (pan_to[1] - pan_from[1]) * a) * height
        view_w, view_h = width / zoom, height / zoom
        left = min(max(cx - view_w / 2.0, 0.0), width - view_w)
        top = min(max(cy - view_h / 2.0, 0.0), height - view_h)
        box = (round(left), round(top),
               round(left + view_w), round(top + view_h))
        out_frames.append(
            image.resize((width, height), Resampling.LANCZOS, box=box))
    return FrameSequence(frames=tuple(out_frames), fps=fps)


def _breath(sequence: FrameSequence,
            params: dict[str, Any]) -> FrameSequence:
    """A still painting breathes: a slow swell held at rest, no travel.

    Ken Burns is a camera move; this is a body. The waveform is a
    phase-warped cosine — theta = 2·pi·t − skew·sin(2·pi·t) — whose warp
    makes the painting dwell near rest and swell briefly, the way a sleeper
    breathes, rather than metronoming up and down. `lift` raises the crop
    window with the swell (the chest rising), expressed as a fraction of
    the margin the zoom itself opens, so the rise always fits without
    clamping and inherits the waveform's smoothness. Phase is sampled at
    k/N with the endpoint excluded, so frame N−1 sits one step before the
    wrap instead of duplicating frame 0. The crop box stays fractional —
    at this amplitude a rounded box would turn a three-pixel rise into
    visible stepping. Deterministic — a breath has no dice to roll.
    """
    import numpy as np
    from PIL.Image import Resampling

    if not sequence.is_still:
        raise OpError("breath takes a still (one frame); it creates the "
                      "timeline itself")
    frames = _require_int(params, "frames", "breath", minimum=2)
    fps = _require_int(params, "fps", "breath")
    amplitude = float(params.get("amplitude", 0.035))
    lift = float(params.get("lift", 0.4))
    skew = float(params.get("skew", 0.45))
    if amplitude <= 0.0:
        raise OpError("breath amplitude must be positive -- at zero the "
                      "result is a still wearing a video's cost")
    if not 0.0 <= skew < 1.0:
        raise OpError("breath skew must sit in [0, 1) -- at 1 the phase "
                      "stalls and the clip holds its breath forever")
    if not 0.0 <= lift <= 1.0:
        raise OpError("breath lift is a fraction of the zoom margin -- "
                      "beyond 1 the window would leave the canvas")

    image = sequence.frames[0]
    width, height = image.size
    t = np.arange(frames, dtype=np.float32) / np.float32(frames)
    theta = 2.0 * np.pi * t - skew * np.sin(2.0 * np.pi * t)
    swell = (0.5 * (1.0 - np.cos(theta))).astype(np.float32)

    out_frames = []
    for k in range(frames):
        zoom = 1.0 + amplitude * float(swell[k])
        view_w, view_h = width / zoom, height / zoom
        left = (width - view_w) / 2.0
        top = ((height - view_h) / 2.0) * (1.0 - lift)
        box = (left, top, left + view_w, top + view_h)
        out_frames.append(
            image.resize((width, height), Resampling.LANCZOS, box=box))
    return FrameSequence(frames=tuple(out_frames), fps=fps)


def parallax_frames(art: Image, depth: Image, *, frames: int, fps: int,
                    amplitude: float = 6.0, zoom: float = 1.06) -> FrameSequence:
    """2.5D drift: a still painting plus its depth map become a loop.

    The camera runs a closed elliptical path (cosine in x, sine in y), so
    frame N-1 sits beside frame 0 and the clip loops. Each pixel is
    displaced along the camera offset in proportion to its depth — near
    things move more, which is the whole illusion — with a small constant
    zoom so the parallax never reveals the void past the canvas edge.
    Deterministic: a camera path has no randomness to seed.

    A public function rather than a registry entry, deliberately: the
    registry's ops are recipe vocabulary, and a recipe's story for *where a
    depth map lives* is not decided here. ADR 32's runtime tier calls this
    directly; the day a committed recipe wants it is the day it earns a
    registry name and that story.
    """
    import numpy as np
    from PIL import Image as PILImage

    if frames < 2:
        raise OpError("parallax needs at least two frames")
    if fps < 1:
        raise OpError("parallax needs fps >= 1")
    if amplitude <= 0:
        raise OpError("parallax needs a positive `amplitude`")

    art_rgb = art.convert("RGB")
    width, height = art_rgb.size
    depth_arr = np.asarray(
        depth.convert("L").resize((width, height)), dtype=np.float32) / 255.0
    # Centre the displacement so the mid-depth plane holds still and the
    # scene pivots around it rather than sliding wholesale.
    depth_arr -= float(depth_arr.mean())

    array = np.asarray(art_rgb, dtype=np.float32)
    ys, xs = np.meshgrid(np.arange(height, dtype=np.float32),
                         np.arange(width, dtype=np.float32), indexing="ij")
    # The zoom margin: sample from a slightly larger window than shown.
    cy, cx = (height - 1) / 2.0, (width - 1) / 2.0
    base_y = cy + (ys - cy) / zoom
    base_x = cx + (xs - cx) / zoom

    out_frames = []
    for k in range(frames):
        angle = 2.0 * np.pi * k / frames
        dx = amplitude * np.cos(angle)
        dy = amplitude * 0.6 * np.sin(angle)
        sample_y = base_y + depth_arr * dy
        sample_x = base_x + depth_arr * dx
        moved = np.dstack([
            _bilinear_wrap(np.ascontiguousarray(array[:, :, c]),
                           sample_y, sample_x, clamp=True)
            for c in range(3)])
        out_frames.append(PILImage.fromarray(
            np.clip(moved, 0.0, 255.0).astype(np.uint8), mode="RGB"))
    return FrameSequence(frames=tuple(out_frames), fps=fps)


#: The registries. `recipe.KNOWN_GENERATOR_OPS` / `KNOWN_MOTION_OPS` are the
#: schema's copies, pinned equal in `tests/test_animist_motion.py` — the
#: SIMULATOR_KEYS seam, a third time.
GENERATOR_OPS: dict[str, GeneratorOpFn] = {
    "spectral_noise": _spectral_noise,
}

MOTION_OPS: dict[str, MotionOpFn] = {
    "advect": _advect,
    "breath": _breath,
    "color_ramp": _color_ramp,
    "ken_burns": _ken_burns,
}


def generate(name: str, params: dict[str, Any]) -> FrameSequence:
    op = GENERATOR_OPS.get(name)
    if op is None:
        raise OpError(f"unknown generator op {name!r} "
                      f"(one of: {', '.join(sorted(GENERATOR_OPS))})")
    return op(params)


def apply_motion(sequence: FrameSequence, name: str,
                 params: dict[str, Any]) -> FrameSequence:
    op = MOTION_OPS.get(name)
    if op is None:
        raise OpError(f"unknown motion op {name!r} "
                      f"(one of: {', '.join(sorted(MOTION_OPS))})")
    return op(sequence, params)


def lift_still(sequence: FrameSequence, name: str,
               params: dict[str, Any]) -> FrameSequence:
    """Map an `ops.py` still op over every frame — the two registries stay
    separate, and a sequence still gets `resize` and friends for free."""
    from mtglab.animist.ops import apply

    frames = tuple(apply(frame, name, params) for frame in sequence.frames)
    return FrameSequence(frames=frames, fps=sequence.fps)
