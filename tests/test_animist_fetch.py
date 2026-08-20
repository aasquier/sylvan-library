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


# ---------------------------------------------- the downloader behind the seam

class _FakeResponse:
    """Enough of an HTTP response for the chunked read the downloader does.

    `watch` runs on every chunk, which is where the ordering claim is
    checkable: what the filesystem looks like *during* the body read is the
    whole point of writing through a `.part`.
    """

    def __init__(self, payload: bytes, chunk: int,
                 watch: Any = None) -> None:
        self._rest = payload
        self._chunk = chunk
        self._watch = watch

    def read(self, size: int) -> bytes:
        if self._watch is not None:
            self._watch()
        take, self._rest = self._rest[:self._chunk], self._rest[self._chunk:]
        return take

    def __enter__(self) -> _FakeResponse:
        return self

    def __exit__(self, *exc: object) -> None:
        return None


def test_the_real_downloader_writes_a_part_file_and_renames_it(tmp_path,
                                                               monkeypatch):
    """`_download` is what runs when nothing injects a downloader, and every
    other test in this file replaces it -- so the discipline it copied from
    `cards/db.py` had no test of its own.

    Both halves of that discipline are asserted: the project's User-Agent
    goes out on the request, and the bytes land through a `.part` that is gone
    once the rename completes. The chunk size is deliberately smaller than the
    payload, because a single-read fake would pass whether or not the loop
    works.
    """
    from mtglab.animist import fetch as fetch_mod
    from mtglab.animist.sources import USER_AGENT

    seen: dict[str, Any] = {}
    target = tmp_path / "leaves.jpg"

    def mid_flight() -> None:
        # What the filesystem looks like while the body is arriving. The
        # rename at the end proves nothing about order unless this does.
        seen.setdefault("part_mid_flight",
                        (tmp_path / "leaves.jpg.part").exists())
        seen.setdefault("target_mid_flight", target.exists())

    def fake_urlopen(request: Any, timeout: float | None = None) -> Any:
        seen["url"] = request.full_url
        seen["agent"] = request.get_header("User-agent")
        seen["timeout"] = timeout
        return _FakeResponse(b"a" * 5000, chunk=1024, watch=mid_flight)

    monkeypatch.setattr(fetch_mod.urllib.request, "urlopen", fake_urlopen)

    fetch_mod._download("https://img.example/leaves.jpg", target)

    assert target.read_bytes() == b"a" * 5000
    assert seen["part_mid_flight"] is True
    assert seen["target_mid_flight"] is False, \
        "the finished name appeared before the body did"
    assert not (tmp_path / "leaves.jpg.part").exists(), \
        "an interrupted download must never be mistaken for a cached copy"
    assert seen["url"] == "https://img.example/leaves.jpg"
    assert seen["agent"] == USER_AGENT
    assert seen["timeout"] is not None, "a fetch with no timeout can hang"


def test_a_failed_download_leaves_no_finished_file(tmp_path, monkeypatch):
    """The reason the `.part` exists at all. A connection that dies mid-body
    must not leave something the next run reads as cached -- `fetch_source`
    skips anything already at the target path, so a truncated file there would
    be permanent."""
    from mtglab.animist import fetch as fetch_mod

    class _Dying(_FakeResponse):
        def read(self, size: int) -> bytes:
            raise OSError("the connection went away")

    monkeypatch.setattr(fetch_mod.urllib.request, "urlopen",
                        lambda request, timeout=None: _Dying(b"", 1))
    target = tmp_path / "leaves.jpg"

    with pytest.raises(OSError):
        fetch_mod._download("https://img.example/leaves.jpg", target)

    assert not target.exists(), "a half-download must not look cached"
