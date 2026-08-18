"""What the platform sees, when the platform is asked: Fly's managed Prometheus.

The last of ROADMAP item 14's seven, and the only one that leaves the box.
`api/adminstats.py` reports what the process can see about itself; this
reports what the *edge* saw — requests that never reached the app, memory
as the platform accounts for it — which is the half a process cannot know
about its own life.

It is **off unless configured**, and that is the design rather than a
degradation. `FLY_METRICS_TOKEN` is a read-only token the maintainer mints
(`fly tokens create readonly`), because a credential is a human act; unset,
`fetch` reports `configured: false`, the widget hides itself, and nothing
about the dashboard changes. A laptop has no Fly org and is never asked to
pretend otherwise.

Three habits inherited rather than reinvented:

- **The secret is read from `os.environ` directly**, the `RESEND_API_KEY`
  pattern. `config.py` never resolves secrets, and `tests/test_config.py`
  forbids a public config name containing KEY/TOKEN/SECRET.
- **stdlib `urllib` behind an injected seam** (`auth/mail.py`'s `Transport`),
  so no test reaches the network and the transport is pinnable. `httpx` and
  `requests` are dev-only dependencies here.
- **An explicit `User-Agent`, assembled above the seam.** `auth/mail.py`
  learnt this expensively: `Python-urllib/3.12` is a banned browser
  signature to at least one WAF, which cost an afternoon of blaming a valid
  key. Set it, and set it where a test can see it.

**A 401 did not prove the address was right, and believing it did cost this
panel its first fortnight.** The commit that added this module reported that it
had "verified the URL by probing with an invalid token: 401 not 404 — that
isolates wrong-credential from wrong-address." The inference was wrong. The
header was malformed on *every* request (see `authorization`), so 401 was the
only answer the endpoint could ever give, and the probe was incapable of
distinguishing anything. Two good tokens were then suspected in turn, because
the one measurement everybody trusted had never measured anything.

The lesson generalises past Fly: **a probe that cannot fail differently is not
a probe.** Before an error code is used to rule something out, check that the
other code was reachable — here, that meant asking the running container to try
two header shapes side by side, which took one command and settled it at once
(`Bearer <tok>` → 401, `<tok>` verbatim → 200).

Answers are cached for five minutes in memory. The numbers move on
Prometheus's own scrape interval, the dashboard refreshes every thirty
seconds, and a metrics API is not something to poll once per tile per
refresh.
"""

from __future__ import annotations

import json
import logging
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Callable
from typing import Any

_LOG = logging.getLogger("mtglab.api.flymetrics")

#: The org slug, not the app name. Fly's managed Prometheus is per
#: organisation and `personal` is the slug of a personal account — the app
#: is selected inside the query instead, by label.
ORG_SLUG = os.environ.get("FLY_ORG_SLUG", "personal")

#: The app whose series this asks about, matching `fly.toml`'s `app`.
APP_NAME = os.environ.get("FLY_APP_NAME", "sylvan-library")

BASE_URL = f"https://api.fly.io/prometheus/{ORG_SLUG}/api/v1/query"

USER_AGENT = "mtg-lab/0.1 (personal deckbuilding tool; instance metrics)"

TIMEOUT_SECONDS = 8

#: Five minutes. Long enough that a dashboard left open all afternoon is a
#: handful of requests, short enough that a number nobody trusts is never
#: what is shown.
CACHE_SECONDS = 300

#: What is asked, and why each one. Instantaneous queries only — a range
#: query would be a chart, and the chart this dashboard has is the app's own
#: request ledger (schema v9), which needs no token and no network.
QUERIES: dict[str, str] = {
    # Resident memory as the platform accounts for it, which is the number
    # that decides whether this machine gets OOM-killed — not the process's
    # own view of itself.
    "memory_bytes":
        f'sum(fly_instance_memory_resident{{app="{APP_NAME}"}})',
    "memory_total_bytes":
        f'sum(fly_instance_memory_total{{app="{APP_NAME}"}})',
    # Edge traffic over a day, by status class. The visitor ledger counts
    # what the app answered; this counts what the edge saw, and the gap
    # between them is exactly the requests the app never got to answer.
    "edge_2xx":
        f'sum(increase(fly_edge_http_responses_count{{app="{APP_NAME}",'
        f'status="2xx"}}[24h]))',
    "edge_4xx":
        f'sum(increase(fly_edge_http_responses_count{{app="{APP_NAME}",'
        f'status="4xx"}}[24h]))',
    "edge_5xx":
        f'sum(increase(fly_edge_http_responses_count{{app="{APP_NAME}",'
        f'status="5xx"}}[24h]))',
}

#: `(url, headers) -> (status, body)`. The seam a test injects.
Transport = Callable[[str, dict[str, str]], tuple[int, bytes]]

_cache: tuple[float, dict[str, Any]] | None = None


def _urllib_get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
    request = urllib.request.Request(url, headers=headers, method="GET")
    with urllib.request.urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        return int(response.status), response.read()


def token() -> str:
    """The read-only Fly token, or empty. Read fresh, never held.

    `os.environ` directly, and blank counts as absent — the same rule
    `config.py` applies to `ANTHROPIC_API_KEY`, because an empty string
    presented as a credential is how a 401 gets mistaken for a bug.
    """
    return os.environ.get("FLY_METRICS_TOKEN", "").strip()


def authorization(secret: str) -> str:
    """The `Authorization` value for `secret` — **scheme included, or added.**

    The bug this function exists for, found 2026-08-18 by asking the running
    container instead of reasoning: a Fly token is a macaroon whose value
    *begins with its own scheme*, `FlyV1 fm2_...`. Wrapping that in `Bearer `
    produces `Authorization: Bearer FlyV1 fm2_...` — two schemes, no valid
    credential — and Fly answers 401 to every request forever. The panel had
    never worked, through two different tokens, and the fault was here rather
    than in either of them.

    So: a value that already carries a scheme is sent **verbatim**. Only a bare
    token gets `Bearer ` put in front of it, which is what a plain API key
    would want and costs nothing to keep.

    A scheme is detected as "first word, no underscore" rather than by matching
    `FlyV1` literally. Fly has versioned this prefix before (`FlyV1` is not the
    first), and a check that only knows today's name would fail the same silent
    way on the next one — whereas `fm2_...` and every bare key shape has no
    space at all.
    """
    head, _, rest = secret.partition(" ")
    if rest and "_" not in head:
        return secret
    return f"Bearer {secret}"


def _scalar(payload: dict[str, Any]) -> float | None:
    """The one number out of a Prometheus instant-vector response.

    Shape: `{"data": {"result": [{"value": [<ts>, "<number>"]}]}}`. An empty
    result is `None` rather than `0` — the same distinction the storage view
    makes, and here it is the difference between "the edge served nothing"
    and "this series does not exist on this plan".
    """
    try:
        result = payload["data"]["result"]
        if not result:
            return None
        return float(result[0]["value"][1])
    except (KeyError, IndexError, TypeError, ValueError):
        return None


def fetch(*, transport: Transport | None = None,
          now: float | None = None) -> dict[str, Any]:
    """Every query, once, cached. Never raises.

    A metrics panel that can 500 the admin page would be a monitoring tool
    that takes the instance's dashboard down with the thing it monitors —
    so a failure is `configured: true, ok: false` with the reason, and the
    widget says the glass is clouded rather than vanishing (which would read
    as "not set up").
    """
    global _cache
    stamp = time.monotonic() if now is None else now
    if _cache is not None and stamp - _cache[0] < CACHE_SECONDS:
        return _cache[1]

    secret = token()
    if not secret:
        # Not cached: configuring the token should take effect on the next
        # look rather than five minutes later.
        return {"configured": False, "ok": False, "values": {}}

    get: Transport = transport if transport is not None else _urllib_get
    headers = {
        "Authorization": authorization(secret),
        "Accept": "application/json",
        # Above the seam, so a test can pin it. See the module docstring for
        # what learning this the hard way cost.
        "User-Agent": USER_AGENT,
    }

    values: dict[str, float | None] = {}
    for name, query in QUERIES.items():
        url = f"{BASE_URL}?query={urllib.parse.quote(query)}"
        try:
            status, body = get(url, headers)
        except urllib.error.HTTPError as exc:
            return _failed(f"Fly answered HTTP {exc.code}", stamp)
        except OSError as exc:
            return _failed(f"could not reach Fly: {exc}", stamp)
        if not 200 <= status < 300:
            return _failed(f"Fly answered HTTP {status}", stamp)
        try:
            values[name] = _scalar(json.loads(body))
        except (ValueError, TypeError):
            return _failed("Fly's answer was not the JSON this expects", stamp)

    answer = {"configured": True, "ok": True, "values": values,
              "app": APP_NAME, "org": ORG_SLUG}
    _cache = (stamp, answer)
    return answer


def _failed(reason: str, stamp: float) -> dict[str, Any]:
    """Cache the failure too, so a broken token is not retried per tile."""
    global _cache
    _LOG.warning("fly metrics unavailable (%s)", reason)
    answer: dict[str, Any] = {"configured": True, "ok": False,
                              "error": reason, "values": {}}
    _cache = (stamp, answer)
    return answer


def reset() -> None:
    """Test helper — drop the cached answer."""
    global _cache
    _cache = None
