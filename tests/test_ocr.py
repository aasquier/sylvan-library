"""The reading engine's shelf: what it caches, and what it refuses.

Six megabytes of somebody else's WebAssembly arrive over the network at
runtime, so the interesting tests here are all about the refusals. **No test
sends a request** -- `_download` is replaced everywhere, the same way
`auth/mail.py`'s seam means no test sends mail.

The one to read first is `test_bytes_that_miss_their_digest_are_refused`.
This module fetches *executable code*, and a digest is the only thing
standing between a compromised CDN, a captive portal's login page, a proxy
that rewrites scripts -- and a browser that would simply run whatever
arrived.
"""

import hashlib
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab import config, ocr

REAL = "tesseract-core-simd-lstm.wasm.js"


@pytest.fixture(autouse=True)
def shelf(tmp_path, monkeypatch):
    """A scratch cache, and a clean refusal set for every test."""
    monkeypatch.setattr(ocr, "_refused", set())
    with config.use_paths(data_dir=tmp_path / "data"):
        yield


def serving(body: bytes, *, digest: str | None = None):
    """Pin `ASSETS[REAL]` to whatever these bytes are, and serve them."""
    return body, digest or hashlib.sha256(body).hexdigest()


def install(monkeypatch, body: bytes, *, digest: str | None = None) -> None:
    payload, want = serving(body, digest=digest)
    monkeypatch.setitem(
        ocr.ASSETS, REAL,
        ocr.Asset(url="https://example.invalid/core.js", digest=want,
                  size=len(payload), media_type="text/javascript"))
    monkeypatch.setattr(ocr, "_download", lambda url: payload)


# ----------------------------------------------------------- the refusals

def test_bytes_that_miss_their_digest_are_refused(monkeypatch, caplog):
    """The reason this module pins hashes at all.

    A captive portal answering 200 with a login page and a compromised CDN
    are the same event from here, and neither gets to be executed.
    """
    install(monkeypatch, b"<html>sign in to the airport wifi</html>",
            digest="0" * 64)
    assert ocr.ensure(REAL) is None
    assert not (ocr.cache_dir() / REAL).exists()
    assert "digest mismatch" in caplog.text


def test_a_digest_failure_is_not_retried(monkeypatch):
    """Sticky, because a bad pin is not a transient condition and a capture
    must not re-download four megabytes to be told so again."""
    calls = []
    install(monkeypatch, b"wrong", digest="0" * 64)
    real = ocr._download
    monkeypatch.setattr(ocr, "_download",
                        lambda url: (calls.append(url), real(url))[1])
    assert ocr.ensure(REAL) is None
    assert ocr.ensure(REAL) is None
    assert len(calls) == 1


def test_an_unknown_name_never_reaches_the_network(monkeypatch):
    """The table *is* the path-traversal guard: absence, not a pattern."""
    monkeypatch.setattr(ocr, "_download", lambda url: pytest.fail(
        "an unknown asset name asked the network"))
    for name in ["../../../etc/passwd", "eng.traineddata", "", "worker.js"]:
        assert ocr.ensure(name) is None


def test_network_trouble_is_not_remembered_as_absence(monkeypatch):
    """Transient, so the next capture tries again -- the distinction
    `symbols.py` draws between a 404 and a dropped connection."""
    def boom(url):
        raise OSError("connection reset")
    install(monkeypatch, b"unused")
    monkeypatch.setattr(ocr, "_download", boom)
    assert ocr.ensure(REAL) is None
    assert REAL not in ocr._refused


def test_an_oversized_body_is_refused(monkeypatch):
    install(monkeypatch, b"x" * 16)
    monkeypatch.setattr(ocr, "_MAX_BYTES", 8)
    assert ocr.ensure(REAL) is None


# ------------------------------------------------------------- the shelf

def test_a_matching_digest_is_cached_and_served(monkeypatch):
    install(monkeypatch, b"console.log('tesseract')")
    path = ocr.ensure(REAL)
    assert path is not None
    assert path.read_bytes() == b"console.log('tesseract')"


def test_the_second_ask_is_a_file_read(monkeypatch):
    install(monkeypatch, b"core")
    assert ocr.ensure(REAL) is not None
    monkeypatch.setattr(ocr, "_download", lambda url: pytest.fail(
        "a cached asset was downloaded again"))
    assert ocr.ensure(REAL) is not None


def test_no_staging_file_is_left_behind(monkeypatch):
    install(monkeypatch, b"core")
    ocr.ensure(REAL)
    assert [p.name for p in ocr.cache_dir().iterdir()] == [REAL]


def test_the_cache_path_carries_the_pinned_versions():
    """So bumping a pin is a new file rather than a stale one, and nobody
    has to remember to clear a volume."""
    stamp = ocr.cache_dir().name
    assert ocr.CORE_VERSION in stamp
    assert ocr.WORKER_VERSION in stamp
    assert ocr.TESSDATA_VERSION in stamp


def test_installed_reports_the_whole_table(monkeypatch):
    assert set(ocr.installed()) == set(ocr.ASSETS)
    assert not any(ocr.installed().values())
    install(monkeypatch, b"core")
    ocr.ensure(REAL)
    assert ocr.installed()[REAL] is True


# --------------------------------------------------------- the pins hold

def test_every_pinned_asset_is_described_completely():
    """A row with a placeholder digest would cache nothing and refuse
    everything, which reads as "the camera is broken" rather than as a
    missing checksum."""
    for name, asset in ocr.ASSETS.items():
        assert len(asset.digest) == 64, name
        assert int(asset.digest, 16) >= 0, name
        assert asset.size > 0, name
        assert asset.url.startswith("https://"), name
        assert asset.size < ocr._MAX_BYTES, name


def test_the_trained_data_is_the_small_one():
    """2.0MB against 10.9MB, for a job that reads a card name and a
    three-digit number. The larger file buys accuracy on prose."""
    assert "_fast" in ocr.TESSDATA_VERSION
    assert ocr.ASSETS["eng.traineddata.gz"].size < 3_000_000
