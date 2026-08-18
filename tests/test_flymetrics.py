"""Fly's managed Prometheus, and the properties that matter when it is off.

**No test here reaches the network.** `flymetrics.fetch` takes a transport,
the same seam `auth/mail.py` put in front of Resend and for the same
reason: the one thing that ever broke delivery there was below the seam
and therefore untestable until it moved above it.

What is pinned:

- unconfigured is a *state*, not a failure — `configured: false`, no
  request attempted, and nothing cached (so setting the token takes effect
  on the next look rather than in five minutes);
- a failure is reported, never raised — a metrics panel must not be able to
  take down the dashboard of the instance it monitors;
- the `User-Agent` is explicit, because `Python-urllib/3.12` is a banned
  browser signature to at least one WAF and this project has already paid
  for that lesson once;
- an empty Prometheus result is `None`, not `0` — "the edge served nothing"
  and "this series does not exist" are different answers.
"""

import json
import sys
import urllib.error
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.api import flymetrics


def vector(value: float) -> bytes:
    return json.dumps(
        {"status": "success",
         "data": {"resultType": "vector",
                  "result": [{"metric": {}, "value": [1787000000, str(value)]}]}}
    ).encode("utf-8")


EMPTY = json.dumps(
    {"status": "success", "data": {"resultType": "vector", "result": []}}
).encode("utf-8")


@pytest.fixture(autouse=True)
def clean_cache():
    flymetrics.reset()
    yield
    flymetrics.reset()


def test_unconfigured_is_a_state_rather_than_a_failure(monkeypatch):
    monkeypatch.delenv("FLY_METRICS_TOKEN", raising=False)

    def refuse(url, headers):  # pragma: no cover - must never run
        raise AssertionError("asked Fly without a token")

    answer = flymetrics.fetch(transport=refuse)

    assert answer == {"configured": False, "ok": False, "values": {}}


def test_a_blank_token_counts_as_absent(monkeypatch):
    """`config.py`'s rule for `ANTHROPIC_API_KEY`, applied here: an empty
    string presented as a credential is how a 401 is mistaken for a bug."""
    monkeypatch.setenv("FLY_METRICS_TOKEN", "   ")

    assert flymetrics.fetch(transport=lambda url, headers: (200, EMPTY)) \
        == {"configured": False, "ok": False, "values": {}}


def test_it_asks_for_every_query_and_carries_the_credentials(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")
    seen: list[tuple[str, dict[str, str]]] = []

    def transport(url, headers):
        seen.append((url, headers))
        return 200, vector(42)

    answer = flymetrics.fetch(transport=transport)

    assert answer["configured"] and answer["ok"]
    assert set(answer["values"]) == set(flymetrics.QUERIES)
    assert answer["values"]["memory_bytes"] == 42
    assert len(seen) == len(flymetrics.QUERIES)
    url, headers = seen[0]
    assert url.startswith(flymetrics.BASE_URL)
    assert headers["Authorization"] == "Bearer fo1_readonly"
    # The lesson auth/mail.py paid for: never the default urllib agent.
    assert headers["User-Agent"] == flymetrics.USER_AGENT
    assert "urllib" not in headers["User-Agent"]
    # The app is selected by label inside the query, not by the org URL.
    assert flymetrics.APP_NAME in url or "%22" in url


def test_an_empty_series_is_none_rather_than_zero(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    answer = flymetrics.fetch(transport=lambda url, headers: (200, EMPTY))

    assert answer["ok"]
    assert answer["values"]["edge_5xx"] is None


@pytest.mark.parametrize("failure", [
    lambda url, headers: (500, b"upstream is unwell"),
    lambda url, headers: (200, b"<html>a WAF page</html>"),
])
def test_a_refusal_is_reported_never_raised(monkeypatch, failure):
    """A monitoring panel must not take down the dashboard it lives on."""
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    answer = flymetrics.fetch(transport=failure)

    assert answer["configured"] is True
    assert answer["ok"] is False
    assert answer["error"]
    assert answer["values"] == {}


def test_a_network_error_is_reported_too(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    def boom(url, headers):
        raise OSError("dns went walkabout")

    answer = flymetrics.fetch(transport=boom)

    assert answer["ok"] is False and "could not reach Fly" in answer["error"]


def test_an_http_error_is_reported_too(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    def unauthorised(url, headers):
        raise urllib.error.HTTPError(url, 401, "no", {}, None)

    answer = flymetrics.fetch(transport=unauthorised)

    assert answer["ok"] is False and "401" in answer["error"]


def test_answers_are_cached_and_the_cache_expires(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")
    calls = {"n": 0}

    def counting(url, headers):
        calls["n"] += 1
        return 200, vector(1)

    per_sweep = len(flymetrics.QUERIES)
    flymetrics.fetch(transport=counting, now=1000.0)
    flymetrics.fetch(transport=counting, now=1000.0 + flymetrics.CACHE_SECONDS - 1)
    assert calls["n"] == per_sweep, "a cached answer asked Fly again"

    flymetrics.fetch(transport=counting, now=1000.0 + flymetrics.CACHE_SECONDS + 1)
    assert calls["n"] == 2 * per_sweep, "the cache never expired"


def test_a_failure_is_cached_so_a_bad_token_is_not_retried_per_tile(monkeypatch):
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_wrong")
    calls = {"n": 0}

    def refusing(url, headers):
        calls["n"] += 1
        return 403, b"nope"

    flymetrics.fetch(transport=refusing, now=500.0)
    flymetrics.fetch(transport=refusing, now=500.0 + 10)

    assert calls["n"] == 1, "the failure was retried inside the cache window"
