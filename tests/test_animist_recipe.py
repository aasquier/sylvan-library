"""The recipe schema: what loads, and the named refusals for what does not."""

from __future__ import annotations

from pathlib import Path

import pytest

from mtglab.animist.recipe import (
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
