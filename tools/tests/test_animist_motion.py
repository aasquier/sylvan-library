"""Motion (ADR 31): seeded determinism, loop closure, and the registry seam.

The two claims worth pinning are the ones whose failure would be silent.
Determinism: a stochastic op re-run with the same seed must produce the same
pixels, or `verify`'s contract stops meaning anything about a committed
loop. Loop closure: `spectral_noise` promises periodicity *by construction*,
and nothing downstream would notice if a refactor traded the FFT synthesis
for something that merely looked similar — the page would just acquire a
seam at second six.
"""

from __future__ import annotations

import numpy as np
import pytest
from PIL import Image

from animist.motion import (
    GENERATOR_OPS,
    MOTION_OPS,
    SEEDED_OPS,
    FrameSequence,
    apply_motion,
    generate,
    lift_still,
)
from animist.ops import OpError
from animist.recipe import (
    KNOWN_GENERATOR_OPS,
    KNOWN_MOTION_OPS,
    KNOWN_SEEDED_OPS,
)

NOISE = {"seed": 11, "width": 48, "height": 32, "frames": 6, "fps": 12}


def as_arrays(sequence: FrameSequence) -> list[np.ndarray]:
    return [np.asarray(frame) for frame in sequence.frames]


# ------------------------------------------------------------- the seam

def test_the_schema_and_the_registries_agree() -> None:
    """The SIMULATOR_KEYS seam, a third time: `recipe.py` cannot import the
    registries, so its copies are pinned here and a renamed op fails loudly
    instead of validating recipes it cannot run."""
    assert set(GENERATOR_OPS) == KNOWN_GENERATOR_OPS
    assert set(MOTION_OPS) == KNOWN_MOTION_OPS
    assert SEEDED_OPS == KNOWN_SEEDED_OPS
    # A seeded op that is neither a generator nor a motion op would be a
    # vocabulary entry nothing can dispatch.
    assert KNOWN_SEEDED_OPS <= (KNOWN_GENERATOR_OPS | KNOWN_MOTION_OPS)


# --------------------------------------------------------- determinism

def test_same_seed_same_frames() -> None:
    first = generate("spectral_noise", dict(NOISE))
    second = generate("spectral_noise", dict(NOISE))
    for a, b in zip(as_arrays(first), as_arrays(second), strict=True):
        assert (a == b).all()


def test_a_different_seed_is_a_different_texture() -> None:
    """The control for the test above -- if every seed produced the same
    field, 'deterministic' would be vacuously true."""
    first = generate("spectral_noise", dict(NOISE))
    second = generate("spectral_noise", {**NOISE, "seed": 12})
    assert any((a != b).any()
               for a, b in zip(as_arrays(first), as_arrays(second), strict=True))


def test_advect_is_deterministic_too() -> None:
    noise = generate("spectral_noise", dict(NOISE))
    once = apply_motion(noise, "advect", {"seed": 3, "strength": 5.0})
    again = apply_motion(noise, "advect", {"seed": 3, "strength": 5.0})
    for a, b in zip(as_arrays(once), as_arrays(again), strict=True):
        assert (a == b).all()


def test_a_stochastic_op_without_a_seed_is_refused() -> None:
    with pytest.raises(OpError, match="seed"):
        generate("spectral_noise", {k: v for k, v in NOISE.items()
                                    if k != "seed"})
    with pytest.raises(OpError, match="seed"):
        apply_motion(generate("spectral_noise", dict(NOISE)), "advect", {})


# -------------------------------------------------------- loop closure
#
# Periodicity by construction: the frame *after* the last is the first, so
# the texture's temporal step from frame N-1 to frame 0 must look like any
# other step -- similar in magnitude to the step from 0 to 1, rather than a
# discontinuity. Byte equality is the wrong assertion (frame N-1 is not
# frame 0; it is one step before it), so the test compares step sizes.

def test_spectral_noise_wraps_without_a_seam() -> None:
    sequence = generate("spectral_noise", dict(NOISE))
    frames = as_arrays(sequence)
    step = np.abs(frames[1].astype(float) - frames[0].astype(float)).mean()
    wrap = np.abs(frames[0].astype(float) - frames[-1].astype(float)).mean()
    assert wrap <= step * 1.5, (
        f"the loop seam moved {wrap:.2f} per pixel against {step:.2f} for an "
        "interior step -- periodicity by construction has been lost")


def test_spectral_noise_tiles_at_its_own_edges() -> None:
    """Spatial periodicity, same argument sideways: the column beyond the
    right edge is column 0, so the horizontal step across the wrap must look
    like an interior step. This is what lets a texture repeat-x."""
    frame = as_arrays(generate("spectral_noise", dict(NOISE)))[0].astype(float)
    interior = np.abs(frame[:, 1] - frame[:, 0]).mean()
    wrap = np.abs(frame[:, 0] - frame[:, -1]).mean()
    assert wrap <= max(interior, 1.0) * 2.0


def test_advect_preserves_the_loop() -> None:
    noise = generate("spectral_noise", dict(NOISE))
    moved = apply_motion(noise, "advect", {"seed": 5, "strength": 4.0})
    frames = as_arrays(moved)
    step = np.abs(frames[1].astype(float) - frames[0].astype(float)).mean()
    wrap = np.abs(frames[0].astype(float) - frames[-1].astype(float)).mean()
    assert wrap <= max(step, 1.0) * 1.5


# ------------------------------------------------------------ the ops

def test_color_ramp_maps_the_endpoints() -> None:
    black = Image.new("L", (4, 4), 0)
    white = Image.new("L", (4, 4), 255)
    ramp = {"low": "#102030", "high": "#a0b0c0"}
    dark = apply_motion(FrameSequence.still(black), "color_ramp", dict(ramp))
    light = apply_motion(FrameSequence.still(white), "color_ramp", dict(ramp))
    assert np.asarray(dark.frames[0])[0, 0].tolist() == [0x10, 0x20, 0x30]
    assert np.asarray(light.frames[0])[0, 0].tolist() == [0xA0, 0xB0, 0xC0]


def test_color_ramp_alpha_from_luma() -> None:
    gray = Image.new("L", (4, 4), 128)
    out = apply_motion(FrameSequence.still(gray), "color_ramp",
                       {"low": "#000000", "high": "#ffffff",
                        "alpha_from_luma": True})
    frame = np.asarray(out.frames[0])
    assert out.frames[0].mode == "RGBA"
    assert frame[0, 0, 3] == 128


def test_color_ramp_refuses_a_malformed_colour() -> None:
    with pytest.raises(OpError, match="hex"):
        apply_motion(FrameSequence.still(Image.new("L", (2, 2))),
                     "color_ramp", {"low": "red"})


def test_color_ramp_gamma_sinks_the_midtones() -> None:
    """gamma > 1 pulls mid luminance toward `low` -- how a full-field cloud
    becomes sparse wisps -- while the endpoints stay the endpoints."""
    ramp = {"low": "#000000", "high": "#ffffff"}
    mid = Image.new("L", (2, 2), 128)
    linear = apply_motion(FrameSequence.still(mid), "color_ramp", dict(ramp))
    curved = apply_motion(FrameSequence.still(mid), "color_ramp",
                          {**ramp, "gamma": 2.4})
    assert int(np.asarray(curved.frames[0])[0, 0, 0]) \
        < int(np.asarray(linear.frames[0])[0, 0, 0])
    for value, name in ((0, "black"), (255, "white")):
        flat = Image.new("L", (2, 2), value)
        out = apply_motion(FrameSequence.still(flat), "color_ramp",
                           {**ramp, "gamma": 2.4})
        assert int(np.asarray(out.frames[0])[0, 0, 0]) == value, name


def test_color_ramp_refuses_a_nonpositive_gamma() -> None:
    with pytest.raises(OpError, match="gamma"):
        apply_motion(FrameSequence.still(Image.new("L", (2, 2))),
                     "color_ramp", {"gamma": 0})


def test_ken_burns_builds_the_timeline_from_a_still() -> None:
    still = FrameSequence.still(Image.new("RGB", (64, 40), (10, 90, 40)))
    clip = apply_motion(still, "ken_burns",
                        {"frames": 9, "fps": 12, "zoom_to": 1.2})
    assert len(clip.frames) == 9
    assert clip.fps == 12
    assert clip.size == (64, 40)


def test_ken_burns_bounce_returns_home() -> None:
    """`bounce: true` is the loop story: the camera path runs there and back,
    so the last frame sits beside the first."""
    rng = np.random.default_rng(4)
    art = Image.fromarray(rng.integers(0, 255, (40, 64, 3), dtype=np.uint8))
    clip = apply_motion(FrameSequence.still(art), "ken_burns",
                        {"frames": 11, "fps": 12, "zoom_to": 1.3,
                         "bounce": True})
    first = np.asarray(clip.frames[0]).astype(float)
    last = np.asarray(clip.frames[-1]).astype(float)
    middle = np.asarray(clip.frames[5]).astype(float)
    assert np.abs(first - last).mean() < np.abs(first - middle).mean()


def test_breath_builds_the_timeline_from_a_still() -> None:
    still = FrameSequence.still(Image.new("RGB", (64, 40), (10, 90, 40)))
    clip = apply_motion(still, "breath",
                        {"frames": 12, "fps": 12, "amplitude": 0.25})
    assert len(clip.frames) == 12
    assert clip.fps == 12
    assert clip.size == (64, 40)


def test_breath_loops_without_a_duplicated_beat() -> None:
    """Phase is sampled at k/N with the endpoint excluded: the last frame
    sits *beside* the first — close enough to loop, but never the same
    frame twice, which would read as a held beat at every wrap."""
    rng = np.random.default_rng(7)
    art = Image.fromarray(rng.integers(0, 255, (40, 64, 3), dtype=np.uint8))
    clip = apply_motion(FrameSequence.still(art), "breath",
                        {"frames": 12, "fps": 12, "amplitude": 0.25})
    first = np.asarray(clip.frames[0]).astype(float)
    last = np.asarray(clip.frames[-1]).astype(float)
    middle = np.asarray(clip.frames[6]).astype(float)
    assert np.abs(first - last).mean() < np.abs(first - middle).mean()
    # Inclusive endpoints would make the wrap a byte-identical duplicate.
    assert not np.array_equal(first, last)


def test_breath_dwells_at_rest() -> None:
    """The phase warp is the whole point: motion near rest is slower than
    motion mid-swell, or the breath is just a metronome wearing a new
    name."""
    rng = np.random.default_rng(9)
    art = Image.fromarray(rng.integers(0, 255, (40, 64, 3), dtype=np.uint8))
    clip = apply_motion(FrameSequence.still(art), "breath",
                        {"frames": 12, "fps": 12, "amplitude": 0.25,
                         "skew": 0.45})
    frames = [np.asarray(f).astype(float) for f in clip.frames]
    at_rest = np.abs(frames[1] - frames[0]).mean()
    mid_swell = np.abs(frames[4] - frames[3]).mean()
    assert at_rest < mid_swell


def test_breath_is_deterministic() -> None:
    art = Image.new("RGB", (48, 32), (30, 60, 120))
    params = {"frames": 8, "fps": 12, "amplitude": 0.1, "lift": 0.4}
    first = apply_motion(FrameSequence.still(art), "breath", dict(params))
    second = apply_motion(FrameSequence.still(art), "breath", dict(params))
    for a, b in zip(as_arrays(first), as_arrays(second), strict=True):
        assert np.array_equal(a, b)


def test_breath_refuses_a_sequence() -> None:
    noise = generate("spectral_noise", dict(NOISE))
    with pytest.raises(OpError, match="still"):
        apply_motion(noise, "breath", {"frames": 4, "fps": 8})


def test_breath_refuses_a_stalling_skew() -> None:
    still = FrameSequence.still(Image.new("RGB", (8, 8)))
    with pytest.raises(OpError, match="holds its breath"):
        apply_motion(still, "breath", {"frames": 4, "fps": 8, "skew": 1.0})


def test_breath_refuses_a_zero_amplitude() -> None:
    still = FrameSequence.still(Image.new("RGB", (8, 8)))
    with pytest.raises(OpError, match="still wearing a video"):
        apply_motion(still, "breath",
                     {"frames": 4, "fps": 8, "amplitude": 0.0})


def test_ken_burns_refuses_a_sequence() -> None:
    noise = generate("spectral_noise", dict(NOISE))
    with pytest.raises(OpError, match="still"):
        apply_motion(noise, "ken_burns", {"frames": 4, "fps": 8})


def test_ken_burns_refuses_zooming_past_the_canvas() -> None:
    still = FrameSequence.still(Image.new("RGB", (8, 8)))
    with pytest.raises(OpError, match="void beyond the canvas"):
        apply_motion(still, "ken_burns",
                     {"frames": 4, "fps": 8, "zoom_from": 0.8})


def test_lift_still_maps_the_old_registry_over_frames() -> None:
    noise = generate("spectral_noise", dict(NOISE))
    small = lift_still(noise, "resize", {"width": 24})
    assert small.size == (24, 16)
    assert len(small.frames) == len(noise.frames)
    assert small.fps == noise.fps


# ------------------------------------------------------- the sequence

def test_a_sequence_refuses_disagreeing_sizes() -> None:
    with pytest.raises(OpError, match="disagree"):
        FrameSequence(frames=(Image.new("L", (4, 4)),
                              Image.new("L", (8, 4))), fps=8)


def test_a_sequence_refuses_emptiness_and_bad_rates() -> None:
    with pytest.raises(OpError, match="at least one frame"):
        FrameSequence(frames=(), fps=8)
    with pytest.raises(OpError, match="fps"):
        FrameSequence(frames=(Image.new("L", (4, 4)),), fps=0)


def test_unknown_ops_are_refused_with_the_vocabulary() -> None:
    with pytest.raises(OpError, match="spectral_noise"):
        generate("perlin", {})
    with pytest.raises(OpError, match="advect"):
        apply_motion(FrameSequence.still(Image.new("L", (2, 2))),
                     "wobble", {})
