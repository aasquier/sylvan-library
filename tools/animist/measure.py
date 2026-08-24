"""The size curve, and its knee: the tarot 400px decision made repeatable.

The tarot PROVENANCE chose 400px "at the knee of the size curve: 560px cost
11.0 MB and 340px saved only 1.5 MB more while starting to soften the
linework." That judgement had two halves — the numbers and the eye — and
this module owns the half that is numbers: encode the same derivation at a
range of widths, report each cost, and mark where the marginal saving stops
paying for the pixels. The eye stays yours.

`knee()` is the classic elbow method — the sampled point farthest from the
straight line between the curve's endpoints — chosen because it is
deterministic, parameter-free, and indifferent to the units on either axis.
It is pure arithmetic on `(width, bytes)` pairs, so it tests without PIL.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from animist.encode import encode
from animist.recipe import Encode

if TYPE_CHECKING:
    from PIL.Image import Image

    from animist.motion import FrameSequence


def size_curve(image: Image, settings: Encode,
               widths: list[int]) -> list[tuple[int, int]]:
    """Encode `image` resized to each width (aspect kept): (width, bytes)."""
    from PIL.Image import Resampling

    from animist.motion import FrameSequence

    curve: list[tuple[int, int]] = []
    for width in sorted(set(widths)):
        height = max(1, round(image.height * width / image.width))
        resized = image.resize((width, height), Resampling.LANCZOS)
        curve.append((width,
                      len(encode(FrameSequence.still(resized), settings))))
    return curve


def crf_curve(sequence: FrameSequence, settings: Encode,
              crfs: list[int]) -> list[tuple[int, int]]:
    """The video formats' half of the same question: encode the sequence at
    each crf, report (crf, bytes). The knee finder is shared — its arithmetic
    never asked what the x-axis meant."""
    from dataclasses import replace

    curve: list[tuple[int, int]] = []
    for crf in sorted(set(crfs)):
        cost = len(encode(sequence, replace(settings, crf=crf)))
        curve.append((crf, cost))
    return curve


def knee(curve: list[tuple[int, int]]) -> int:
    """The width at the curve's elbow. Needs at least three points."""
    if len(curve) < 3:
        raise ValueError("a knee needs at least three sampled widths")
    points = sorted(curve)
    (x0, y0), (x1, y1) = points[0], points[-1]
    span_x = float(x1 - x0) or 1.0
    span_y = float(y1 - y0) or 1.0

    def distance(point: tuple[int, int]) -> float:
        # Perpendicular distance to the endpoint chord, on axes normalised
        # so bytes and pixels weigh the same.
        nx = (point[0] - x0) / span_x
        ny = (point[1] - y0) / span_y
        return abs(nx - ny)

    best = max(points[1:-1], key=distance)
    return best[0]
