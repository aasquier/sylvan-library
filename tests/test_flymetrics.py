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
    # A bare key keeps `Bearer`. The macaroon case is below, and is the one
    # that was broken in production for a fortnight.
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


def quiet_edge(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
    """A healthy day: the edge answered, and nothing failed.

    2xx is populated; 4xx and 5xx match no samples, which is what Prometheus
    does with a class that did not occur. The URL is matched after
    `urllib.parse.quote`, where `status=~"5.."` reads `status%3D~%225..%22`.
    """
    if "%222..%22" in url:
        return 200, vector(3581)
    if "status%3D~" in url:
        return 200, EMPTY
    return 200, vector(1)


def test_a_quiet_alarm_reads_zero_and_not_an_em_dash(monkeypatch):
    """The tile that matters must not render "all clear" as "I could not ask".

    An empty 5xx vector is the *good* answer and looks identical to the two
    broken queries #172 fixed. `edge_2xx` is the witness that tells them
    apart: the metric answered, so nothing failing is a real zero.
    """
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    answer = flymetrics.fetch(transport=quiet_edge)

    assert answer["values"]["edge_2xx"] == 3581
    assert answer["values"]["edge_4xx"] == 0
    assert answer["values"]["edge_5xx"] == 0


def test_without_the_witness_nothing_is_claimed(monkeypatch):
    """The other half, and the reason this is not `or vector(0)`.

    With `edge_2xx` empty too, the series may not exist at all — so every
    counter stays `None` and the page keeps saying so. Collapsing these to
    zero would report a healthy edge for a query that never worked, which is
    the exact failure this module was built after.
    """
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    answer = flymetrics.fetch(transport=lambda url, headers: (200, EMPTY))

    assert answer["values"]["edge_2xx"] is None
    assert answer["values"]["edge_4xx"] is None
    assert answer["values"]["edge_5xx"] is None


def test_memory_is_never_settled_by_the_edge_witness(monkeypatch):
    """The witness speaks only for its own series.

    Memory is a different metric on a different subsystem, so a live edge says
    nothing about whether it is being scraped — reporting zero bytes of memory
    for a running machine would be a worse lie than the em-dash.
    """
    monkeypatch.setenv("FLY_METRICS_TOKEN", "fo1_readonly")

    answer = flymetrics.fetch(transport=quiet_edge)

    # The subtraction is inside the query, so the transport answers it whole.
    assert answer["values"]["memory_bytes"] == 1
    assert set(flymetrics.EDGE_SILENT) == {"edge_4xx", "edge_5xx"}
    assert "memory_bytes" not in flymetrics.EDGE_SILENT


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


# ------------------------------------------------- the header that was wrong

def test_a_fly_macaroon_is_sent_verbatim_because_it_carries_its_own_scheme():
    """The bug the panel shipped with, as an assertion.

    A Fly token *is* `FlyV1 fm2_...` — the scheme is part of the value. This
    module wrapped it in `Bearer `, so every request carried two schemes and no
    valid credential, and Fly answered 401 forever. Two good tokens were
    suspected before the code was.
    """
    assert (flymetrics.authorization("FlyV1 fm2_abc123")
            == "FlyV1 fm2_abc123")


def test_no_header_ever_contains_two_schemes():
    """The shape of the failure, guarded directly.

    Written against the symptom rather than the mechanism: whatever
    `authorization` does next, `Bearer FlyV1` is never a thing to send.
    """
    for secret in ("FlyV1 fm2_abc", "FlyV2 fm3_abc", "fo1_bare"):
        assert "Bearer FlyV1" not in flymetrics.authorization(secret)
        assert "Bearer FlyV2" not in flymetrics.authorization(secret)


def test_a_bare_token_still_gets_bearer():
    """A plain API key has no scheme of its own and needs one."""
    assert flymetrics.authorization("fo1_readonly") == "Bearer fo1_readonly"


def test_a_future_scheme_name_is_honoured_without_naming_it():
    """`FlyV1` is not the first version of this prefix and will not be the
    last. Matching the literal string would fail the same silent way on the
    next one, so the check is structural: a first word with no underscore is a
    scheme, and every bare key shape has no space at all."""
    assert flymetrics.authorization("FlyV2 fm3_abc") == "FlyV2 fm3_abc"
    assert flymetrics.authorization("fm2_looks_like_a_token") == (
        "Bearer fm2_looks_like_a_token")


def test_the_live_header_shape_reaches_the_transport(monkeypatch):
    """End to end through `fetch`, because `authorization` being right does not
    prove the caller uses it — which is exactly the gap that let the original
    bug through a suite that already asserted a header."""
    monkeypatch.setenv("FLY_METRICS_TOKEN", "FlyV1 fm2_abc123")
    flymetrics.reset()
    seen: list[tuple[str, dict[str, str]]] = []

    def transport(url, headers):
        seen.append((url, headers))
        return 200, vector(1)

    flymetrics.fetch(transport=transport)
    assert seen[0][1]["Authorization"] == "FlyV1 fm2_abc123"


# ------------------------------------------- the queries, against real labels

def test_the_edge_queries_match_a_status_class_not_a_literal_2xx():
    """The bug that made every edge tile an em-dash.

    Fly's `fly_edge_http_responses_count` carries the **full status code** in
    its `status` label — `200`, `206`, `301`, `401` — so `status="2xx"` matched
    no series at all. Confirmed by listing the live label sets from inside the
    container, which is the only place the truth was available: the panel had
    never authenticated, so nobody had ever seen a populated response.
    """
    for name in ("edge_2xx", "edge_4xx", "edge_5xx"):
        query = flymetrics.QUERIES[name]
        digit = name[len("edge_")]
        assert f'status=~"{digit}.."' in query, query
        assert f'status="{digit}xx"' not in query


def test_memory_is_derived_because_fly_publishes_no_resident_series():
    """`fly_instance_memory_resident` does not exist and never did.

    What Fly publishes is `mem_total` and `mem_available`; used is the
    subtraction. Asserted by name because the failure is silent — a query for
    a metric nobody exports returns an empty vector, which this module
    correctly reports as `None`, which the page correctly renders as an
    em-dash. Three correct behaviours in a row hiding one wrong string.
    """
    blob = " ".join(flymetrics.QUERIES.values())
    assert "fly_instance_memory_resident" not in blob
    assert "fly_instance_memory_mem_total" in flymetrics.QUERIES["memory_bytes"]
    assert ("fly_instance_memory_mem_available"
            in flymetrics.QUERIES["memory_bytes"])
    assert (flymetrics.QUERIES["memory_total_bytes"]
            == 'sum(fly_instance_memory_mem_total{app="'
               + flymetrics.APP_NAME + '"})')


def test_every_query_scopes_itself_to_this_app():
    """Prometheus is per *organisation*. A query with no `app` label would
    silently total every app in the org — which on a personal account is one
    app today and a wrong number the day it is two."""
    for name, query in flymetrics.QUERIES.items():
        assert f'app="{flymetrics.APP_NAME}"' in query, name
