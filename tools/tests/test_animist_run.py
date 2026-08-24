"""The choreography: build writes what it says, and refusals write nothing."""

from __future__ import annotations

import io
import json
from pathlib import Path

import numpy as np
import pytest
from PIL import Image

from animist import config
from animist.recipe import load_recipe
from animist.run import BuildError, Built, build
from animist.sources import LicenceRefused

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


# --------------------------------------------------------- motion (ADR 31)

PROCEDURAL_RECIPE = """
animist: 1
why_committed: generated here; committing it is the only way it exists
sources:
  mist:
    provider: procedural
    seed: 42
    licence: ours-generated
outputs:
  - file: mist-loop.webm
    from: mist
    ops:
      - spectral_noise: {width: 32, height: 16, frames: 4, fps: 8}
      - color_ramp: {low: "#0b1c14", high: "#9fe8c0"}
    encode: {format: webm, crf: 45}
    expect: {width: 32, height: 16, frames: 4, fps: 8, budget_kb: 100}
  - file: mist-loop.mp4
    from: mist
    ops:
      - spectral_noise: {width: 32, height: 16, frames: 4, fps: 8}
      - color_ramp: {low: "#0b1c14", high: "#9fe8c0"}
    encode: {format: mp4, crf: 40}
    expect: {width: 32, height: 16, frames: 4, budget_kb: 100}
  - file: mist-poster.webp
    from: mist
    ops:
      - spectral_noise: {width: 32, height: 16, frames: 4, fps: 8}
      - color_ramp: {low: "#0b1c14", high: "#9fe8c0"}
    encode: {format: webp, quality: 80}
    expect: {width: 32, height: 16, budget_kb: 50}
"""


def no_network(url, params):
    raise AssertionError(f"a procedural build reached the network: {url}")


def never_download(url, target):
    raise AssertionError(f"a procedural build tried to download: {url}")


def test_a_procedural_build_touches_no_network(tmp_path, monkeypatch):
    """The whole point of the provider: seed in, committed loop out, and the
    transport is a tripwire rather than a fake."""
    monkeypatch.setattr(config, "DATA_DIR", tmp_path / "data")
    path = tmp_path / "mist.recipe.yaml"
    path.write_text(PROCEDURAL_RECIPE, encoding="utf-8")
    result = build(load_recipe(path), transport=no_network,
                   download=never_download)
    names = sorted(p.name for p in result.written)
    assert names == ["mist-loop.mp4", "mist-loop.webm", "mist-poster.webp"]
    assert (tmp_path / "PROVENANCE.md").exists()
    text = (tmp_path / "PROVENANCE.md").read_text(encoding="utf-8")
    assert "seed 42" in text
    assert "ours-generated" in text
    assert "crf 45" in text


def test_dual_format_derives_once(tmp_path, monkeypatch):
    """Three outputs, one derivation: the memo key is (source, ops), so the
    webm, its mp4 twin and the poster share one generation pass."""
    from animist import motion

    calls = {"n": 0}
    real = motion.GENERATOR_OPS["spectral_noise"]

    def counting(params):
        calls["n"] += 1
        return real(params)

    monkeypatch.setitem(motion.GENERATOR_OPS, "spectral_noise", counting)
    monkeypatch.setattr(config, "DATA_DIR", tmp_path / "data")
    path = tmp_path / "mist.recipe.yaml"
    path.write_text(PROCEDURAL_RECIPE, encoding="utf-8")
    build(load_recipe(path), transport=no_network, download=never_download)
    assert calls["n"] == 1


def test_the_source_seed_reaches_the_generator(tmp_path, monkeypatch):
    """Two builds from the same recipe produce identical stills -- the
    procedural source's seed is inherited, not the clock's."""
    monkeypatch.setattr(config, "DATA_DIR", tmp_path / "data")
    path = tmp_path / "mist.recipe.yaml"
    path.write_text(PROCEDURAL_RECIPE, encoding="utf-8")
    build(load_recipe(path), transport=no_network, download=never_download)
    first = (tmp_path / "mist-poster.webp").read_bytes()
    build(load_recipe(path), transport=no_network, download=never_download)
    assert (tmp_path / "mist-poster.webp").read_bytes() == first
