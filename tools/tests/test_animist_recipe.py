"""The recipe schema: what loads, and the named refusals for what does not."""

from __future__ import annotations

from pathlib import Path

import pytest

from animist.recipe import (
    KNOWN_OPS,
    PROVIDERS,
    RecipeError,
    load_recipe,
)

FULL = """
animist: 1
why_committed: >
  This photograph is CC0, so committing the derived asset is clean.
sources:
  ivy:
    provider: openverse
    identifier: 0aae2e33-1234-5678-9abc-def012345678
    landing: https://example.org/ivy
    title: "Ivy cascading"
    found_via: "Openverse search with a license=cc0 filter"
    licence: CC0-1.0
  cards:
    provider: wikimedia-category
    category: "Some 1909 deck"
    require_filename_prefix: RWS1909
    licence: Public domain
outputs:
  - file: ivy-canopy.webp
    from: ivy
    ops:
      - crop: {band: top}
      - matte_green: {fade_below_frac: 0.42}
      - feather: {radius: 1.2, resteepen: true}
      - mirror_tile: {axis: x}
      - resize: {width: 2048}
    encode: {format: webp, quality: 82}
    expect: {width: 2048, height: 250, budget_kb: 240, metadata: none}
  - each: cards
    ops:
      - resize: {width: 400}
    encode: {format: webp, quality: 75}
    expect: {count: 78, width: 400, total_budget_kb: 4700, metadata: none}
"""


def write(tmp_path: Path, text: str) -> Path:
    path = tmp_path / "scene.recipe.yaml"
    path.write_text(text, encoding="utf-8")
    return path


def test_full_recipe_round_trips(tmp_path: Path) -> None:
    recipe = load_recipe(write(tmp_path, FULL))
    assert recipe.slug == "scene"
    assert recipe.directory == tmp_path
    assert "committing the derived asset is clean" in recipe.why_committed
    assert set(recipe.sources) == {"ivy", "cards"}
    assert recipe.sources["cards"].require_filename_prefix == "RWS1909"

    canopy, cards = recipe.outputs
    assert canopy.file == "ivy-canopy.webp" and not canopy.each
    assert [op.name for op in canopy.ops] == [
        "crop", "matte_green", "feather", "mirror_tile", "resize"]
    assert canopy.ops[2].params == {"radius": 1.2, "resteepen": True}
    assert canopy.encode.quality == 82
    assert canopy.expect.width == 2048 and canopy.expect.budget_kb == 240
    assert cards.each and cards.source == "cards"
    assert cards.expect.count == 78 and cards.expect.total_budget_kb == 4700
    # `metadata: none` is the only accepted value and always resolves True.
    assert canopy.expect.metadata_none and cards.expect.metadata_none


def refusal(tmp_path: Path, text: str) -> str:
    with pytest.raises(RecipeError) as excinfo:
        load_recipe(write(tmp_path, text))
    return str(excinfo.value)


def test_wrong_version_is_named(tmp_path: Path) -> None:
    message = refusal(tmp_path, "animist: 2\nwhy_committed: x\n")
    assert "animist: 2" in message and "version 1" in message


def test_missing_why_committed_refuses_and_instructs(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        "why_committed: >\n"
        "  This photograph is CC0, so committing the derived asset is clean.",
        "why_committed: ''"))
    # The refusal is doctrine (ADR 8's spirit): the pipeline never writes the
    # sentence, and the message says so rather than just naming a field.
    assert "why_committed" in message
    assert "will not invent" in message


def test_unknown_op_is_named_with_the_vocabulary(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace("- resize: {width: 2048}",
                                             "- sharpen: {amount: 2}"))
    assert "sharpen" in message
    for known in KNOWN_OPS:
        assert known in message


def test_unknown_provider_is_named(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace("provider: openverse",
                                             "provider: pinterest"))
    assert "pinterest" in message
    for known in PROVIDERS:
        assert known in message


def test_output_needs_exactly_one_of_file_or_each(tmp_path: Path) -> None:
    both = refusal(tmp_path, FULL.replace("  - each: cards",
                                          "  - each: cards\n    file: x.webp"))
    assert "exactly one" in both
    neither = refusal(tmp_path, FULL.replace(
        "  - file: ivy-canopy.webp\n    from: ivy\n    ops:", "  - ops:"))
    assert "exactly one" in neither


def test_output_source_must_be_declared(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace("from: ivy", "from: oak"))
    assert "oak" in message and "not a declared source" in message


def test_expect_is_required_and_metadata_must_be_none(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        "    expect: {count: 78, width: 400, total_budget_kb: 4700, metadata: none}\n",
        ""))
    assert "expect" in message and "verify" in message
    message = refusal(tmp_path, FULL.replace(
        "budget_kb: 240, metadata: none", "budget_kb: 240, metadata: exif"))
    assert "metadata" in message and "none" in message


def test_count_only_makes_sense_on_each(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        "expect: {width: 2048, height: 250, budget_kb: 240, metadata: none}",
        "expect: {width: 2048, count: 3, metadata: none}"))
    assert "count" in message and "each" in message


def test_quality_bounds(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace("quality: 82", "quality: 0"))
    assert "quality" in message


def test_openverse_needs_identifier(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        "    identifier: 0aae2e33-1234-5678-9abc-def012345678\n", ""))
    assert "identifier" in message


def test_category_needs_category(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        '    category: "Some 1909 deck"\n', ""))
    assert "category" in message


def test_op_must_be_single_key_mapping(tmp_path: Path) -> None:
    message = refusal(tmp_path, FULL.replace(
        "- crop: {band: top}", "- {crop: {band: top}, resize: {width: 4}}"))
    assert "single-key" in message


def test_not_yaml_and_not_mapping_are_named(tmp_path: Path) -> None:
    assert "valid YAML" in refusal(tmp_path, "animist: [unclosed")
    assert "mapping" in refusal(tmp_path, "- just\n- a\n- list\n")


def test_missing_file_is_a_recipe_error(tmp_path: Path) -> None:
    with pytest.raises(RecipeError) as excinfo:
        load_recipe(tmp_path / "absent.recipe.yaml")
    assert "cannot be read" in str(excinfo.value)


# --------------------------------------------------------- motion (ADR 31)

MIST = """
animist: 1
why_committed: >
  Generated by this pipeline, so committing it is the only way it exists.
sources:
  mist:
    provider: procedural
    seed: 42
    licence: ours-generated
outputs:
  - file: mist-loop.webm
    from: mist
    ops:
      - spectral_noise: {width: 64, height: 32, frames: 8, fps: 12}
      - color_ramp: {low: "#0b1c14", high: "#9fe8c0"}
    encode: {format: webm, crf: 40}
    expect: {width: 64, height: 32, frames: 8, fps: 12, budget_kb: 100}
  - file: mist-loop.mp4
    from: mist
    ops:
      - spectral_noise: {width: 64, height: 32, frames: 8, fps: 12}
      - color_ramp: {low: "#0b1c14", high: "#9fe8c0"}
    encode: {format: mp4, crf: 34}
    expect: {width: 64, height: 32, frames: 8, budget_kb: 100}
"""


def test_a_procedural_motion_recipe_round_trips(tmp_path: Path) -> None:
    recipe = load_recipe(write(tmp_path, MIST))
    source = recipe.sources["mist"]
    assert source.provider == "procedural" and source.seed == 42
    webm, mp4 = recipe.outputs
    assert webm.encode.format == "webm" and webm.encode.crf == 40
    assert webm.expect.frames == 8 and webm.expect.fps == 12
    assert mp4.encode.crf == 34


def test_a_procedural_source_needs_a_seed(tmp_path: Path) -> None:
    with pytest.raises(RecipeError, match="seed"):
        load_recipe(write(tmp_path, MIST.replace("    seed: 42\n", "")))


def test_a_procedural_source_licence_is_ours_generated(tmp_path: Path) -> None:
    with pytest.raises(RecipeError, match="ours-generated"):
        load_recipe(write(tmp_path,
                          MIST.replace("licence: ours-generated",
                                       "licence: CC0-1.0")))


def test_a_procedural_output_must_start_with_a_generator(
        tmp_path: Path) -> None:
    broken = MIST.replace(
        "      - spectral_noise: {width: 64, height: 32, frames: 8, fps: 12}\n",
        "", 1)
    with pytest.raises(RecipeError, match="generator"):
        load_recipe(write(tmp_path, broken))


def test_a_generator_on_a_fetched_source_is_refused(tmp_path: Path) -> None:
    """A generator would discard the image the licence gate just confirmed."""
    broken = FULL.replace(
        "      - crop: {band: top}",
        "      - spectral_noise: {width: 8, height: 8, frames: 2, fps: 2, seed: 1}")
    with pytest.raises(RecipeError, match="procedural"):
        load_recipe(write(tmp_path, broken))


def test_a_stochastic_op_with_no_seed_to_inherit_is_refused(
        tmp_path: Path) -> None:
    """`advect` on a fetched source has no procedural seed to fall back on,
    so its params must carry one -- the derivation must be written down."""
    broken = FULL.replace(
        "      - mirror_tile: {axis: x}",
        "      - advect: {strength: 4.0}")
    with pytest.raises(RecipeError, match="seed"):
        load_recipe(write(tmp_path, broken))
    fine = FULL.replace(
        "      - mirror_tile: {axis: x}",
        "      - advect: {strength: 4.0, seed: 9}")
    load_recipe(write(tmp_path, fine))


def test_video_formats_take_crf_and_refuse_quality(tmp_path: Path) -> None:
    with pytest.raises(RecipeError, match="crf"):
        load_recipe(write(tmp_path, MIST.replace(
            "encode: {format: webm, crf: 40}",
            "encode: {format: webm, quality: 80}")))
    with pytest.raises(RecipeError, match="crf"):
        load_recipe(write(tmp_path, MIST.replace(
            "encode: {format: webm, crf: 40}",
            "encode: {format: webm}")))


def test_still_formats_refuse_crf(tmp_path: Path) -> None:
    with pytest.raises(RecipeError, match="video"):
        load_recipe(write(tmp_path, FULL.replace(
            "encode: {format: webp, quality: 82}",
            "encode: {format: webp, crf: 30}")))


def test_a_procedural_output_cannot_be_each(tmp_path: Path) -> None:
    broken = MIST.replace("  - file: mist-loop.mp4\n    from: mist\n",
                          "  - each: mist\n")
    with pytest.raises(RecipeError, match="file"):
        load_recipe(write(tmp_path, broken))
