"""The cache: layout, skip-if-cached, and nothing written past a refusal."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from mtglab import config
from mtglab.animist.fetch import cache_dir, fetch_source
from mtglab.animist.recipe import Recipe, Source
from mtglab.animist.sources import LicenceRefused


def recipe_at(tmp_path: Path) -> tuple[Recipe, Source]:
    source = Source(id="ivy", provider="openverse", licence="CC0-1.0",
                    identifier="abc-123")
    recipe = Recipe(path=tmp_path / "scene.recipe.yaml",
                    why_committed="it is ours", sources={"ivy": source},
                    outputs=())
    return recipe, source


def cc0_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
    return 200, json.dumps({"license": "cc0",
                            "url": "https://img.example/leaves.jpg"}).encode()


def recording_downloader():
    calls: list[str] = []

    def download(url: str, target: Path) -> None:
        calls.append(url)
        target.write_bytes(b"image-bytes")

    return download, calls


def test_cache_layout_and_download(tmp_path: Path) -> None:
    recipe, source = recipe_at(tmp_path)
    download, calls = recording_downloader()
    with config.use_paths(data_dir=tmp_path / "data"):
        confirmed, local = fetch_source(recipe, source,
                                        transport=cc0_transport,
                                        download=download)
        expected = (tmp_path / "data" / "cache" / "animist" / "scene" / "ivy"
                    / "leaves.jpg")
        assert local == {"leaves.jpg": expected}
        assert expected.read_bytes() == b"image-bytes"
        assert calls == ["https://img.example/leaves.jpg"]
        assert confirmed.confirmation.licence == "cc0"


def test_cached_copy_skips_the_download_but_not_the_gate(tmp_path: Path) -> None:
    recipe, source = recipe_at(tmp_path)
    download, calls = recording_downloader()
    gate_hits: list[str] = []

    def counting_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        gate_hits.append(url)
        return cc0_transport(url, headers)

    with config.use_paths(data_dir=tmp_path / "data"):
        fetch_source(recipe, source, transport=counting_transport,
                     download=download)
        fetch_source(recipe, source, transport=counting_transport,
                     download=download)
    # One download, two gate runs: a licence confirmed last run is a licence
    # this run has not confirmed.
    assert len(calls) == 1
    assert len(gate_hits) == 2


def test_refused_source_writes_nothing(tmp_path: Path) -> None:
    recipe, source = recipe_at(tmp_path)
    download, calls = recording_downloader()

    def by_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"license": "by",
                                "url": "https://img.example/x.jpg"}).encode()

    with config.use_paths(data_dir=tmp_path / "data"):
        with pytest.raises(LicenceRefused):
            fetch_source(recipe, source, transport=by_transport,
                         download=download)
        assert calls == []
        assert not cache_dir(recipe, source).exists()
        # Stronger: the refusal left no trace at all under data/.
        assert not (tmp_path / "data").exists()


def test_cache_dir_reads_config_at_call_time(tmp_path: Path) -> None:
    recipe, source = recipe_at(tmp_path)
    before = cache_dir(recipe, source)
    with config.use_paths(data_dir=tmp_path / "elsewhere"):
        inside = cache_dir(recipe, source)
    assert inside == (tmp_path / "elsewhere" / "cache" / "animist" / "scene"
                      / "ivy")
    assert cache_dir(recipe, source) == before


def fake_page(name: str, licence: str = "Public domain") -> dict[str, Any]:
    return {"title": f"File:{name}",
            "imageinfo": [{"url": f"https://upload.example/{name}",
                           "extmetadata": {"LicenseShortName":
                                           {"value": licence}}}]}


def test_category_fetch_lands_every_passing_file(tmp_path: Path) -> None:
    source = Source(id="cards", provider="wikimedia-category",
                    licence="Public domain", category="Deck",
                    require_filename_prefix="RWS1909")
    recipe = Recipe(path=tmp_path / "tarot.recipe.yaml",
                    why_committed="public domain", sources={"cards": source},
                    outputs=())

    def category_transport(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"query": {"pages": [
            fake_page("RWS1909 The Star.jpg"),
            fake_page("Banner.jpg"),
        ]}}).encode()

    download, calls = recording_downloader()
    with config.use_paths(data_dir=tmp_path / "data"):
        confirmed, local = fetch_source(recipe, source,
                                        transport=category_transport,
                                        download=download)
    assert set(local) == {"RWS1909 The Star.jpg"}
    assert confirmed.skipped == ("Banner.jpg",)
    assert len(calls) == 1
