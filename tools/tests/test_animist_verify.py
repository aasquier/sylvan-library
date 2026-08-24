"""Verify: the contract held, each breach named, and never a write."""

from __future__ import annotations

import io
from pathlib import Path

import numpy as np
from PIL import Image

from animist.recipe import load_recipe
from animist.verify import verify_recipe

RECIPE = """
animist: 1
why_committed: it is ours
sources:
  ivy:
    provider: openverse
    identifier: abc-123
    licence: CC0-1.0
  cards:
    provider: wikimedia-category
    category: Deck
    licence: Public domain
outputs:
  - file: canopy.webp
    from: ivy
    ops: []
    encode: {format: webp, quality: 82}
    expect: {width: 64, height: 32, budget_kb: 50, metadata: none}
  - each: cards
    ops: []
    encode: {format: webp, quality: 75}
    expect: {count: 2, width: 40, total_budget_kb: 50, metadata: none}
"""


def webp_bytes(width: int, height: int, *, exif: bool = False) -> bytes:
    rng = np.random.default_rng(width + height)
    image = Image.fromarray(
        rng.integers(0, 255, (height, width, 3), dtype=np.uint8), mode="RGB")
    buffer = io.BytesIO()
    if exif:
        tags = Image.Exif()
        tags[0x010F] = "ACME"
        image.save(buffer, format="WEBP", exif=tags.tobytes())
    else:
        image.save(buffer, format="WEBP")
    return buffer.getvalue()


def scene(tmp_path: Path):
    path = tmp_path / "scene.recipe.yaml"
    path.write_text(RECIPE, encoding="utf-8")
    (tmp_path / "canopy.webp").write_bytes(webp_bytes(64, 32))
    (tmp_path / "card-a.webp").write_bytes(webp_bytes(40, 70))
    (tmp_path / "card-b.webp").write_bytes(webp_bytes(40, 66))
    return load_recipe(path)


def test_a_held_contract_reports_nothing(tmp_path: Path) -> None:
    assert verify_recipe(scene(tmp_path)) == []


def test_each_breach_is_a_sentence_naming_both_values(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    (tmp_path / "canopy.webp").write_bytes(webp_bytes(60, 32))
    failures = verify_recipe(recipe)
    assert len(failures) == 1
    assert "width 60" in failures[0] and "expects 64" in failures[0]


def test_missing_file_and_undecodable_file(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    (tmp_path / "canopy.webp").unlink()
    assert any("missing" in failure for failure in verify_recipe(recipe))
    (tmp_path / "canopy.webp").write_bytes(b"not an image at all")
    assert any("cannot be decoded" in f for f in verify_recipe(recipe))


def test_wrong_count_fails_the_set_loudly(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    (tmp_path / "card-b.webp").unlink()
    failures = verify_recipe(recipe)
    # The image job's phrasing, on purpose: a glob that half-matches is
    # worse than one that fails.
    assert any("holds 1 files, recipe expects exactly 2" in f
               for f in failures)


def test_set_members_share_width_but_not_height(tmp_path: Path) -> None:
    # card-a is 70 tall and card-b is 66: heights vary across a fetched set
    # (the tarot scans do) and only width is contractual.
    recipe = scene(tmp_path)
    assert verify_recipe(recipe) == []
    (tmp_path / "card-b.webp").write_bytes(webp_bytes(39, 66))
    assert any("width 39" in f for f in verify_recipe(recipe))


def test_file_outputs_are_not_counted_into_the_set(tmp_path: Path) -> None:
    # canopy.webp sits in the same directory; the `each` glob must not
    # swallow it or the count would read 3.
    recipe = scene(tmp_path)
    failures = verify_recipe(recipe)
    assert failures == []


def test_metadata_in_a_committed_file_is_named(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    (tmp_path / "canopy.webp").write_bytes(webp_bytes(64, 32, exif=True))
    failures = verify_recipe(recipe)
    assert any("carries metadata" in f and "hand-replaced" in f
               for f in failures)


def test_budgets_per_file_and_per_set(tmp_path: Path) -> None:
    scene(tmp_path)
    strict = RECIPE.replace("budget_kb: 50", "budget_kb: 1") \
                   .replace("total_budget_kb: 50", "total_budget_kb: 1")
    (tmp_path / "scene.recipe.yaml").write_text(strict, encoding="utf-8")
    failures = verify_recipe(load_recipe(tmp_path / "scene.recipe.yaml"))
    assert any("over the 1 KB budget" in f for f in failures)
    assert any("the set totals" in f for f in failures)


def test_verify_never_writes(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    stamps = {p: p.stat().st_mtime_ns for p in tmp_path.iterdir()}
    verify_recipe(recipe)
    assert {p: p.stat().st_mtime_ns for p in tmp_path.iterdir()} == stamps


# --------------------------------------------------------- motion (ADR 31)

VIDEO_RECIPE = """
animist: 1
why_committed: generated here
sources:
  mist:
    provider: procedural
    seed: 7
    licence: ours-generated
outputs:
  - file: loop.webm
    from: mist
    ops:
      - spectral_noise: {width: 32, height: 16, frames: 4, fps: 8}
    encode: {format: webm, crf: 45}
    expect: {width: 32, height: 16, frames: 4, fps: 8, budget_kb: 100}
"""


def webm_bytes(width: int = 32, height: int = 16, frames: int = 4,
               fps: int = 8) -> bytes:
    from animist.encode import encode
    from animist.motion import FrameSequence
    from animist.recipe import Encode

    rng = np.random.default_rng(1)
    images = tuple(
        Image.fromarray(rng.integers(0, 255, (height, width, 3),
                                     dtype=np.uint8), mode="RGB")
        for _ in range(frames))
    return encode(FrameSequence(frames=images, fps=fps),
                  Encode(format="webm", crf=45))


def video_recipe(tmp_path: Path) -> Path:
    path = tmp_path / "mist.recipe.yaml"
    path.write_text(VIDEO_RECIPE, encoding="utf-8")
    return path


def test_a_committed_loop_that_matches_holds(tmp_path: Path) -> None:
    (tmp_path / "loop.webm").write_bytes(webm_bytes())
    assert verify_recipe(load_recipe(video_recipe(tmp_path))) == []


def test_a_loop_with_the_wrong_frame_count_is_named(tmp_path: Path) -> None:
    (tmp_path / "loop.webm").write_bytes(webm_bytes(frames=6))
    failures = verify_recipe(load_recipe(video_recipe(tmp_path)))
    assert any("6 frame(s)" in f and "expects 4" in f for f in failures)


def test_a_loop_at_the_wrong_rate_is_named(tmp_path: Path) -> None:
    (tmp_path / "loop.webm").write_bytes(webm_bytes(fps=24))
    failures = verify_recipe(load_recipe(video_recipe(tmp_path)))
    assert any("24" in f and "fps" in f for f in failures)


def test_a_loop_at_the_wrong_size_is_named(tmp_path: Path) -> None:
    (tmp_path / "loop.webm").write_bytes(webm_bytes(width=64, height=32))
    failures = verify_recipe(load_recipe(video_recipe(tmp_path)))
    assert any("width 64" in f for f in failures)


def test_a_file_that_is_not_video_at_all_is_named(tmp_path: Path) -> None:
    (tmp_path / "loop.webm").write_bytes(b"not a video")
    failures = verify_recipe(load_recipe(video_recipe(tmp_path)))
    assert any("cannot be decoded as webm" in f for f in failures)


def test_an_fps_expectation_nothing_can_read_fails_loudly(
        tmp_path: Path) -> None:
    """ADR 8 wearing a codec: Pillow reads no rate out of animated WebP, and
    an expectation the decoder cannot evaluate must not silently pass."""
    from animist.encode import encode
    from animist.motion import FrameSequence
    from animist.recipe import Encode

    recipe_text = VIDEO_RECIPE.replace(
        "file: loop.webm", "file: loop.webp").replace(
        "encode: {format: webm, crf: 45}",
        "encode: {format: awebp, quality: 80}")
    path = tmp_path / "mist.recipe.yaml"
    path.write_text(recipe_text, encoding="utf-8")

    rng = np.random.default_rng(2)
    images = tuple(
        Image.fromarray(rng.integers(0, 255, (16, 32, 3), dtype=np.uint8),
                        mode="RGB") for _ in range(4))
    data = encode(FrameSequence(frames=images, fps=8),
                  Encode(format="awebp", quality=80))
    (tmp_path / "loop.webp").write_bytes(data)
    failures = verify_recipe(load_recipe(path))
    assert any("no rate can be read" in f for f in failures)
