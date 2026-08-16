"""The choreography: build writes what it says, and refusals write nothing."""

from __future__ import annotations

import io
import json
from pathlib import Path

import numpy as np
import pytest
from PIL import Image

from mtglab import config
from mtglab.animist.recipe import load_recipe
from mtglab.animist.run import BuildError, Built, build
from mtglab.animist.sources import LicenceRefused

RECIPE = """
animist: 1
why_committed: it is ours
sources:
  ivy:
    provider: openverse
    identifier: abc-123
    title: "Ivy cascading"
    licence: CC0-1.0
outputs:
  - file: canopy.webp
    from: ivy
    ops:
      - resize: {width: 32}
    encode: {format: webp, quality: 82}
    expect: {width: 32, budget_kb: 50, metadata: none}
"""

CATEGORY_RECIPE = """
animist: 1
why_committed: public domain scans
sources:
  cards:
    provider: wikimedia-category
    category: Deck
    require_filename_prefix: RWS1909
    licence: Public domain
outputs:
  - each: cards
    ops:
      - resize: {width: 20}
    encode: {format: webp, quality: 75}
    expect: {count: 2, width: 20, metadata: none}
"""


def jpeg_bytes(width: int = 64, height: int = 48) -> bytes:
    rng = np.random.default_rng(3)
    image = Image.fromarray(
        rng.integers(0, 255, (height, width, 3), dtype=np.uint8), mode="RGB")
    buffer = io.BytesIO()
    image.save(buffer, format="JPEG")
    return buffer.getvalue()


def cc0_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
    return 200, json.dumps({"license": "cc0",
                            "url": "https://img.example/leaves.jpg"}).encode()


def image_downloader(url: str, target: Path) -> None:
    target.write_bytes(jpeg_bytes())


def scene(tmp_path: Path, text: str = RECIPE):
    directory = tmp_path / "scene"
    directory.mkdir()
    path = directory / "scene.recipe.yaml"
    path.write_text(text, encoding="utf-8")
    return load_recipe(path)


def test_build_writes_asset_and_provenance(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    with config.use_paths(data_dir=tmp_path / "data"):
        result = build(recipe, transport=cc0_transport,
                       download=image_downloader)
    target = recipe.directory / "canopy.webp"
    assert result.written == [target]
    with Image.open(target) as written:
        assert written.size == (32, 24)
    assert result.sizes["canopy.webp"] == target.stat().st_size
    provenance = (recipe.directory / "PROVENANCE.md").read_text("utf-8")
    assert "## canopy.webp" in provenance
    assert "Why committed rather than hotlinked: it is ours" in provenance
    assert result.provenance == recipe.directory / "PROVENANCE.md"


def test_dry_run_reports_and_writes_nothing(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    with config.use_paths(data_dir=tmp_path / "data"):
        result = build(recipe, dry_run=True, transport=cc0_transport,
                       download=image_downloader)
    assert result.dry_run and result.written == []
    assert result.sizes["canopy.webp"] > 0
    assert not (recipe.directory / "canopy.webp").exists()
    assert not (recipe.directory / "PROVENANCE.md").exists()


def test_over_budget_is_refused_before_any_write(tmp_path: Path) -> None:
    recipe = scene(tmp_path, RECIPE.replace("budget_kb: 50", "budget_kb: 1")
                                  .replace("width: 32}", "width: 64}")
                                  .replace("expect: {width: 32",
                                           "expect: {width: 64"))
    with config.use_paths(data_dir=tmp_path / "data"), \
            pytest.raises(BuildError, match="over the recipe's 1 KB budget"):
        build(recipe, transport=cc0_transport, download=image_downloader)
    assert not (recipe.directory / "canopy.webp").exists()
    assert not (recipe.directory / "PROVENANCE.md").exists()


def test_refused_licence_reaches_the_caller_with_nothing_written(
        tmp_path: Path) -> None:
    recipe = scene(tmp_path)

    def by_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"license": "by",
                                "url": "https://img.example/x.jpg"}).encode()

    with config.use_paths(data_dir=tmp_path / "data"), \
            pytest.raises(LicenceRefused):
        build(recipe, transport=by_transport, download=image_downloader)
    assert list(recipe.directory.iterdir()) == [recipe.path]
    assert not (tmp_path / "data").exists()


def test_unknown_output_name_is_refused(tmp_path: Path) -> None:
    recipe = scene(tmp_path)
    with pytest.raises(BuildError, match="no output named 'nope'"):
        build(recipe, only="nope")


def test_each_output_slugs_upstream_names(tmp_path: Path) -> None:
    recipe = scene(tmp_path, CATEGORY_RECIPE)

    def category_transport(url: str, headers: dict[str, str],
                           ) -> tuple[int, bytes]:
        def page(name: str) -> dict[str, object]:
            return {"title": f"File:{name}",
                    "imageinfo": [{"url": f"https://upload.example/{name}",
                                   "extmetadata": {"LicenseShortName":
                                                   {"value": "Public domain"}}}]}
        return 200, json.dumps({"query": {"pages": [
            page("RWS1909 The Star.jpg"), page("Banner.jpg"),
            page("RWS1909 The Fool.jpg")]}}).encode()

    with config.use_paths(data_dir=tmp_path / "data"):
        result = build(recipe, transport=category_transport,
                       download=image_downloader)
    names = sorted(path.name for path in result.written)
    assert names == ["rws1909-the-fool.webp", "rws1909-the-star.webp"]
    assert result.skipped == ("Banner.jpg",)
    provenance = (recipe.directory / "PROVENANCE.md").read_text("utf-8")
    assert "Skipped by the filename guard" in provenance


def test_built_default_shape() -> None:
    result = Built()
    assert result.written == [] and result.sizes == {}
    assert not result.dry_run and result.provenance is None
