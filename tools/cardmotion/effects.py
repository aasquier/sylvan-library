"""The effect vocabulary: what may be done to a card painting, and how.

Small on purpose, and bounded by Scryfall's imagery guidelines rather than
by imagination: motion and parallax read as presentation; distortion, blur
and colour-shift of the artwork read as editing it, and are not offered.
An `Effect` is frozen and fully parameterised, and its `fingerprint()` is
the cache story — change a parameter or the derivation code and old
derivatives simply stop being found, which is ADR 18's compiled-input
argument wearing pixels.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from PIL.Image import Image

    from animist.motion import FrameSequence

#: Bump when `derive` changes meaning: it rides every fingerprint, so stale
#: derivatives age out of the cache instead of being served as current.
CODE_VERSION = 1


class EffectError(ValueError):
    """An effect asked for something the vocabulary does not offer."""


@dataclass(frozen=True)
class Effect:
    """One named derivation: its knobs, and whether it needs a depth map."""

    key: str
    needs_depth: bool
    fps: int
    seconds: float
    params: dict[str, Any] = field(default_factory=dict)

    @property
    def frames(self) -> int:
        return max(2, round(self.fps * self.seconds))

    def fingerprint(self) -> str:
        """What identifies a derivative besides its inputs. Stable across
        processes (sorted JSON), changed by any knob or by CODE_VERSION."""
        payload = json.dumps(
            {"key": self.key, "needs_depth": self.needs_depth,
             "fps": self.fps, "seconds": self.seconds, "params": self.params,
             "code": CODE_VERSION},
            sort_keys=True, separators=(",", ":"))
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16]


#: The roster. Adding one is an entry here plus a branch in `derive` --
#: and a thought about the guidelines note in this module's docstring.
EFFECTS: dict[str, Effect] = {
    # The marquee: 2.5D drift over an inferred depth map. Near brushwork
    # sways more than far sky, and the loop closes because the camera path
    # is a closed ellipse.
    "depth-drift": Effect(key="depth-drift", needs_depth=True, fps=24,
                          seconds=6.0,
                          params={"amplitude": 6.0, "zoom": 1.06}),
    # The no-model fallback: a slow Ken Burns bounce. Same camera language
    # the scene lanes already speak in CSS, precomputed.
    "slow-pan": Effect(key="slow-pan", needs_depth=False, fps=24,
                       seconds=8.0,
                       params={"zoom_to": 1.09, "pan_from": [0.5, 0.42],
                               "pan_to": [0.5, 0.58]}),
    # The living portrait: a body rather than a camera. A phase-warped
    # swell that dwells at rest and rises from the chest — for paintings
    # whose depth estimate disappoints (flat watercolour, art nouveau) and
    # anywhere a pan would survey a composition that should simply live.
    # 6.5 seconds is a calm resting breath, about nine to the minute.
    # 2.2% at peak, not more: on an edge-to-edge composition the eye
    # tracks the border, and 3.5% read as a zoom rather than a breath
    # (Aaron's eye, first walk, 2026-08-18).
    "breath": Effect(key="breath", needs_depth=False, fps=24, seconds=6.5,
                     params={"amplitude": 0.022, "lift": 0.3, "skew": 0.45}),
}


def derive(art: Image, depth: Image | None, effect: Effect) -> FrameSequence:
    """Frames for one effect. Pure and deterministic — the camera paths are
    closed curves and nothing here rolls dice, so the same art and depth
    always yield the same loop (up to encoder drift, which nothing byte-
    checks; ADR 18's stance)."""
    from animist.motion import (
        FrameSequence,
        apply_motion,
        parallax_frames,
    )

    if effect.key == "depth-drift":
        if depth is None:
            raise EffectError("depth-drift needs a depth map -- run the "
                              "depth model, or pick an effect that does not")
        return parallax_frames(
            art, depth, frames=effect.frames, fps=effect.fps,
            amplitude=float(effect.params["amplitude"]),
            zoom=float(effect.params["zoom"]))
    if effect.key == "slow-pan":
        return apply_motion(
            FrameSequence.still(art), "ken_burns",
            {"frames": effect.frames, "fps": effect.fps, "bounce": True,
             **effect.params})
    if effect.key == "breath":
        return apply_motion(
            FrameSequence.still(art), "breath",
            {"frames": effect.frames, "fps": effect.fps, **effect.params})
    raise EffectError(f"unknown effect {effect.key!r} "
                      f"(one of: {', '.join(sorted(EFFECTS))})")
