"""The licence gate, over fake transports. No test touches a socket."""

from __future__ import annotations

import json
from typing import Any

import pytest

from animist.recipe import Source
from animist.sources import (
    ALLOWED_COMMONS,
    ALLOWED_MET,
    ALLOWED_OPENVERSE,
    USER_AGENT,
    LicenceRefused,
    SourceError,
    confirm,
)


def transport_returning(*bodies: dict[str, Any]):
    """A fake `Transport` serving canned JSON, recording every request."""
    calls: list[tuple[str, dict[str, str]]] = []
    queue = list(bodies)

    def get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        calls.append((url, headers))
        return 200, json.dumps(queue.pop(0)).encode("utf-8")

    get.calls = calls  # type: ignore[attr-defined]
    return get


def openverse_source(**overrides: str) -> Source:
    fields: dict[str, str] = {"identifier": "abc-123", "licence": "CC0-1.0"}
    fields.update(overrides)
    return Source(id="ivy", provider="openverse", **fields)


def commons_page(name: str, licence: str) -> dict[str, Any]:
    return {"title": f"File:{name}",
            "imageinfo": [{"url": f"https://upload.example/{name}",
                           "extmetadata": {"LicenseShortName":
                                           {"value": licence}}}]}


def test_openverse_pass_returns_file_and_proof() -> None:
    get = transport_returning({"license": "cc0",
                               "url": "https://img.example/leaves%20gray.jpg"})
    confirmed = confirm(openverse_source(), transport=get)
    assert confirmed.confirmation.licence == "cc0"
    assert confirmed.confirmation.source_id == "ivy"
    assert "abc-123" in confirmed.confirmation.api_url
    assert len(confirmed.confirmation.checked_on) == 10   # ISO date
    (remote,) = confirmed.files
    assert remote.name == "leaves gray.jpg"
    assert remote.url == "https://img.example/leaves%20gray.jpg"


def test_openverse_cc_by_is_refused() -> None:
    get = transport_returning({"license": "by", "url": "https://x/y.jpg"})
    with pytest.raises(LicenceRefused) as excinfo:
        confirm(openverse_source(), transport=get)
    refusal = excinfo.value
    assert refusal.source_id == "ivy"
    assert refusal.found == "by"
    assert refusal.allowed == ALLOWED_OPENVERSE
    # The message states the doctrine: no flag gets past the gate.
    assert "no flag" in str(refusal)


def test_user_agent_is_pinned_through_the_seam() -> None:
    # The mail.py test, replayed: the header that decides whether requests
    # work at all is assembled above the seam, where a test can see it.
    get = transport_returning({"license": "cc0", "url": "https://x/y.jpg"})
    confirm(openverse_source(), transport=get)
    ((_, headers),) = get.calls  # type: ignore[attr-defined]
    assert headers["User-Agent"] == USER_AGENT
    assert USER_AGENT.startswith("mtg-lab/")


def test_wikimedia_single_file_pass() -> None:
    get = transport_returning(
        {"query": {"pages": [commons_page("Painting.jpg", "Public domain")]}})
    source = Source(id="art", provider="wikimedia", licence="Public domain",
                    title="Painting.jpg")
    confirmed = confirm(source, transport=get)
    (remote,) = confirmed.files
    assert remote.name == "Painting.jpg"
    assert confirmed.confirmation.licence == "Public domain"
    (url, _), = get.calls  # type: ignore[attr-defined]
    assert "titles=File%3APainting.jpg" in url


def test_wikimedia_cc_by_sa_is_refused_and_names_the_file() -> None:
    get = transport_returning(
        {"query": {"pages": [commons_page("Modern.jpg", "CC BY-SA 4.0")]}})
    source = Source(id="art", provider="wikimedia", licence="Public domain",
                    title="Modern.jpg")
    with pytest.raises(LicenceRefused) as excinfo:
        confirm(source, transport=get)
    assert excinfo.value.allowed == ALLOWED_COMMONS
    assert "Modern.jpg" in str(excinfo.value)


def category_source(prefix: str = "RWS1909") -> Source:
    return Source(id="cards", provider="wikimedia-category",
                  licence="Public domain", category="Some 1909 deck",
                  require_filename_prefix=prefix)


def test_category_prefix_guard_skips_and_reports() -> None:
    get = transport_returning({"query": {"pages": [
        commons_page("RWS1909 The Star.jpg", "Public domain"),
        commons_page("Category banner.jpg", "CC BY-SA 4.0"),
        commons_page("RWS1909 The Fool.jpg", "Public domain"),
    ]}})
    confirmed = confirm(category_source(), transport=get)
    assert [f.name for f in confirmed.files] == [
        "RWS1909 The Star.jpg", "RWS1909 The Fool.jpg"]
    # The banner was skipped by the prefix guard *before* the gate saw its
    # licence -- it is not a card, so its CC BY-SA is not a failure.
    assert confirmed.skipped == ("Category banner.jpg",)


def test_category_unfree_member_refuses_the_whole_source() -> None:
    get = transport_returning({"query": {"pages": [
        commons_page("RWS1909 The Star.jpg", "Public domain"),
        commons_page("RWS1909 Recoloured.jpg", "CC BY-SA 4.0"),
    ]}})
    with pytest.raises(LicenceRefused) as excinfo:
        confirm(category_source(), transport=get)
    assert "Recoloured" in str(excinfo.value)


def test_category_follows_continuation() -> None:
    get = transport_returning(
        {"query": {"pages": [commons_page("RWS1909 A.jpg", "Public domain")]},
         "continue": {"gcmcontinue": "page2", "continue": "gcmcontinue||"}},
        {"query": {"pages": [commons_page("RWS1909 B.jpg", "Public domain")]}})
    confirmed = confirm(category_source(), transport=get)
    assert [f.name for f in confirmed.files] == ["RWS1909 A.jpg",
                                                 "RWS1909 B.jpg"]
    second_url = get.calls[1][0]  # type: ignore[attr-defined]
    assert "gcmcontinue=page2" in second_url


def test_category_empty_after_prefix_is_an_error() -> None:
    get = transport_returning(
        {"query": {"pages": [commons_page("Banner.jpg", "Public domain")]}})
    with pytest.raises(SourceError) as excinfo:
        confirm(category_source(), transport=get)
    assert "RWS1909" in str(excinfo.value)


def test_non_json_and_bad_status_are_source_errors() -> None:
    def html(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, b"<html>rate limited</html>"

    with pytest.raises(SourceError) as excinfo:
        confirm(openverse_source(), transport=html)
    assert "not JSON" in str(excinfo.value)

    def teapot(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 503, b"{}"

    with pytest.raises(SourceError) as excinfo:
        confirm(openverse_source(), transport=teapot)
    assert "503" in str(excinfo.value)


def test_openverse_record_without_url_is_an_error() -> None:
    get = transport_returning({"license": "cc0"})
    with pytest.raises(SourceError) as excinfo:
        confirm(openverse_source(), transport=get)
    assert "no direct file URL" in str(excinfo.value)


# ── the Met ──────────────────────────────────────────────────────────────────
# The one provider whose gate reads a boolean rather than a licence string, so
# these pin the mapping in both directions rather than trusting it.

def met_source(**overrides: str) -> Source:
    fields: dict[str, str] = {"identifier": "44054", "licence": "CC0-1.0"}
    fields.update(overrides)
    return Source(id="fish", provider="met", **fields)


def test_met_open_access_object_passes_as_cc0() -> None:
    get = transport_returning({
        "objectID": 44054, "isPublicDomain": True,
        "primaryImage": "https://images.metmuseum.org/CRDImages/as/original/"
                        "17104.jpg"})
    confirmed = confirm(met_source(), transport=get)
    assert confirmed.confirmation.licence == "CC0"
    assert confirmed.confirmation.api_url.endswith("/objects/44054")
    (remote,) = confirmed.files
    assert remote.name == "17104.jpg"
    assert remote.licence == "CC0"


def test_met_object_not_in_open_access_is_refused() -> None:
    get = transport_returning({"isPublicDomain": False,
                               "primaryImage": "https://images.met/x.jpg"})
    with pytest.raises(LicenceRefused) as excinfo:
        confirm(met_source(), transport=get)
    assert excinfo.value.found == "restricted"
    assert excinfo.value.allowed == ALLOWED_MET
    assert "44054" in str(excinfo.value)


@pytest.mark.parametrize("flag", [None, "true", 1, "yes"])
def test_met_anything_but_a_true_boolean_is_refused(flag: object) -> None:
    # `isPublicDomain: "true"` is a string, and a truthy one. The gate must
    # not take a provider's word in the wrong type for a rights clearance --
    # this is the whole reason the check is `is True` and not `if flag`.
    get = transport_returning({"isPublicDomain": flag,
                               "primaryImage": "https://images.met/x.jpg"})
    with pytest.raises(LicenceRefused):
        confirm(met_source(), transport=get)


def test_met_object_without_an_image_is_an_error_not_a_refusal() -> None:
    get = transport_returning({"isPublicDomain": True, "primaryImage": ""})
    with pytest.raises(SourceError) as excinfo:
        confirm(met_source(), transport=get)
    assert "primaryImage" in str(excinfo.value)
