"""The official-symbol cache (ADR 33): fill on first ask, serve local ever after.

No test here touches the network — `_download` is replaced wholesale, which is
the seam it exists to be. What is checked is the contract the client leans on:
a symbol is fetched exactly once, a code Scryfall refuses is remembered as
absent, a network failure is *not* remembered (the next ask retries), and
nothing that fails the shape check ever reaches the filesystem.
"""

from __future__ import annotations

import urllib.error
from email.message import Message

import pytest
from fastapi.testclient import TestClient

from mtglab import config, symbols
from mtglab.api.app import create_app

SVG = b'<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>'


@pytest.fixture(autouse=True)
def clean_slate(tmp_path, monkeypatch):
    """A scratch cache dir per test, and no negative-cache carryover."""
    monkeypatch.setattr(symbols, "_missing", set())
    with config.use_paths(data_dir=tmp_path / "data",
                          decks_dir=tmp_path / "decks"):
        yield


def http_404(url: str) -> urllib.error.HTTPError:
    return urllib.error.HTTPError(url, 404, "Not Found", Message(), None)


class Fetcher:
    """A stand-in `_download` that counts its calls and serves a script."""

    def __init__(self, body: bytes = SVG, error: Exception | None = None):
        self.body = body
        self.error = error
        self.calls: list[str] = []

    def __call__(self, url: str) -> bytes:
        self.calls.append(url)
        if self.error is not None:
            raise self.error
        return self.body


# ------------------------------------------------------------------ ensure()


def test_first_ask_downloads_and_caches(monkeypatch):
    fetcher = Fetcher()
    monkeypatch.setattr(symbols, "_download", fetcher)
    path = symbols.ensure("W")
    assert path is not None and path.read_bytes() == SVG
    again = symbols.ensure("W")
    assert again == path
    assert fetcher.calls == ["https://svgs.scryfall.io/card-symbols/W.svg"]


def test_a_malformed_code_never_reaches_network_or_disk(monkeypatch):
    fetcher = Fetcher()
    monkeypatch.setattr(symbols, "_download", fetcher)
    for code in ("", "w", "../etc", "W/U", "A" * 11, "W.svg", "..", "%2e"):
        assert symbols.ensure(code) is None, code
    assert fetcher.calls == []
    assert not symbols.cache_dir().exists()


def test_scryfalls_404_is_remembered(monkeypatch):
    fetcher = Fetcher(error=http_404("https://svgs.scryfall.io/x"))
    monkeypatch.setattr(symbols, "_download", fetcher)
    assert symbols.ensure("ZZZ") is None
    assert symbols.ensure("ZZZ") is None
    assert len(fetcher.calls) == 1  # the second ask never re-fetched


def test_a_network_failure_is_not_remembered(monkeypatch):
    fetcher = Fetcher(error=OSError("wifi fell over"))
    monkeypatch.setattr(symbols, "_download", fetcher)
    assert symbols.ensure("W") is None
    fetcher.error = None
    path = symbols.ensure("W")
    assert path is not None and path.read_bytes() == SVG
    assert len(fetcher.calls) == 2  # transient trouble means the next ask retries


def test_a_response_that_is_not_svg_is_refused(monkeypatch):
    monkeypatch.setattr(
        symbols, "_download", Fetcher(body=b"<html>hotel wifi login</html>"))
    assert symbols.ensure("W") is None
    assert not (symbols.cache_dir() / "W.svg").exists()


def test_an_oversize_response_is_refused(monkeypatch):
    monkeypatch.setattr(
        symbols, "_download", Fetcher(body=b"<svg" + b"x" * 70_000))
    assert symbols.ensure("W") is None


# -------------------------------------------------------------------- route


@pytest.fixture
def client():
    with TestClient(create_app()) as c:
        yield c


def test_route_serves_the_symbol_with_a_week_of_cache(client, monkeypatch):
    monkeypatch.setattr(symbols, "_download", Fetcher())
    response = client.get("/api/symbols/W.svg")
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("image/svg+xml")
    assert response.headers["cache-control"] == "public, max-age=604800"
    assert response.content == SVG


def test_route_uppercases_so_one_symbol_is_one_cache_entry(client, monkeypatch):
    fetcher = Fetcher()
    monkeypatch.setattr(symbols, "_download", fetcher)
    assert client.get("/api/symbols/wu.svg").status_code == 200
    assert client.get("/api/symbols/WU.svg").status_code == 200
    assert fetcher.calls == ["https://svgs.scryfall.io/card-symbols/WU.svg"]


def test_route_answers_404_for_an_unknown_symbol(client, monkeypatch):
    monkeypatch.setattr(
        symbols, "_download", Fetcher(error=http_404("https://x")))
    response = client.get("/api/symbols/NOPE.svg")
    assert response.status_code == 404


def test_route_answers_404_with_no_network(client, monkeypatch):
    monkeypatch.setattr(
        symbols, "_download", Fetcher(error=OSError("offline")))
    assert client.get("/api/symbols/W.svg").status_code == 404


def test_nothing_downloads_when_the_cache_is_warm(client, monkeypatch):
    warm = symbols.cache_dir()
    warm.mkdir(parents=True)
    (warm / "G.svg").write_bytes(SVG)

    def explode(url: str) -> bytes:
        raise AssertionError(f"warm cache still fetched {url}")

    monkeypatch.setattr(symbols, "_download", explode)
    response = client.get("/api/symbols/G.svg")
    assert response.status_code == 200 and response.content == SVG
