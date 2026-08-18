"""The visitor ledger: how much this instance is used, without who.

ROADMAP item 14's sixth PR, and the privacy stance is the design. The
project's rule for personal data is not to keep it (rule 5, ADR 16's
narrowness about addresses), so what this records is exactly four things
per request: the UTC **day**, the matched **route template**, the
**status class**, and a **count**. Never the concrete path — a path can
carry a slug and a slug can carry a person — and no IP, no user agent,
no username, no timestamp finer than the day. `tests/test_traffic.py`
pins the table to those four columns so a helpful fifth one fails a test
instead of an audit.

Two mechanical decisions, both about not taxing the requests being
counted:

- **No per-request database write.** Counts buffer in memory under a
  lock and flush when a request arrives more than `FLUSH_EVERY` seconds
  after the last flush, on shutdown, and when the traffic endpoint reads
  (so the dashboard is never a minute behind). The buffer is bounded by
  construction: its keys are (day, template, class), and templates are a
  fixed set — the one bucket unrouted requests share is what keeps a
  garbage flood of made-up paths from minting keys.
- **The flush never raises** (`sim/cache.py`'s rule, then the ledger's,
  then the deck log's): a full disk loses a minute of counts, not a
  request.

`(unrouted)` is every request no route matched: real 404s, and — with
auth on — everything the middleware refused **before routing** (401 and
the admin 403, ADR 17). Those refusals dominate the bucket on a deployed
instance and that is the honest shape: refused-at-the-door is its own
kind of traffic, and recording which door was tried would be recording
the concrete path.
"""

from __future__ import annotations

import logging
import sqlite3
import threading
import time
from collections.abc import Awaitable, Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

from fastapi import FastAPI, Request, Response

from mtglab import config

_LOG = logging.getLogger("mtglab.api.traffic")

#: Seconds between opportunistic flushes. A quiet instance flushes on the
#: next request, the endpoint's read, or shutdown — whichever comes first.
FLUSH_EVERY = 60.0

#: The one bucket concrete paths are allowed to share.
UNROUTED = "(unrouted)"

#: Keyed by (database path, day, template, class). The path is captured at
#: **record** time, because the write is deferred: a count belongs to the
#: instance whose configuration saw the request, and by the time a flush
#: runs, `config` may point somewhere else entirely — the suite re-points
#: it per test, which is how a deferred write becomes a write into the
#: wrong database.
_BUFFER: dict[tuple[str, str, str, str], int] = {}
_LOCK = threading.Lock()
_last_flush = time.monotonic()


def _class_of(status: int) -> str:
    return f"{status // 100}xx"


def _template(request: Request) -> str:
    """The matched route's template, or the shared unrouted bucket.

    Set by the router during matching, so it is only present after
    `call_next` — and absent exactly when recording the path would mean
    recording what somebody typed: a 404, or a pre-routing refusal.
    `path_format` is the templated form on API routes; a mount (the
    static tiers) has only `path`, which is its prefix and equally
    template-shaped.
    """
    route = request.scope.get("route")
    template = getattr(route, "path_format", None) or getattr(route, "path", None)
    return str(template) if template else UNROUTED


def record(template: str, status: int) -> None:
    """Count one response, and flush if one is due. Never raises."""
    day = datetime.now(UTC).strftime("%Y-%m-%d")
    key = (str(config.APP_DB_PATH), day, template, _class_of(status))
    global _last_flush
    taken: dict[tuple[str, str, str, str], int] | None = None
    with _LOCK:
        _BUFFER[key] = _BUFFER.get(key, 0) + 1
        if time.monotonic() - _last_flush >= FLUSH_EVERY:
            taken = dict(_BUFFER)
            _BUFFER.clear()
            _last_flush = time.monotonic()
    if taken:
        _write(taken)


def flush() -> None:
    """Write everything buffered, now. Shutdown and the traffic endpoint."""
    global _last_flush
    with _LOCK:
        taken = dict(_BUFFER)
        _BUFFER.clear()
        _last_flush = time.monotonic()
    if taken:
        _write(taken)


def reset() -> None:
    """Test helper — drop anything buffered without writing it."""
    with _LOCK:
        _BUFFER.clear()


def _write(rows: dict[tuple[str, str, str, str], int]) -> None:
    """Upsert a snapshot. A failure is a logged warning and lost counts —
    deliberately not re-buffered, because a broken database plus a growing
    buffer is two problems.

    **The ledger never mints a database.** A path whose file does not exist
    is skipped and its counts dropped: `tests/test_library_write_gate.py`
    pins that a bare laptop acquires no `app.db` from a typed URL, and a
    request counter is not the thing that gets to break that. On any
    instance with an account — every deployed one — the file exists and
    nothing is dropped; on a fresh laptop the counts start the moment
    something real (a login, a simulation) creates the database.
    """
    try:
        from mtglab.auth import db
        for path_str in {path for path, _, _, _ in rows}:
            if not Path(path_str).exists():
                continue
            with db.connection(path_str) as con, con:
                for (path, day, template, cls), count in rows.items():
                    if path != path_str:
                        continue
                    con.execute(
                        "INSERT INTO request_log (day, route, status_class,"
                        " count) VALUES (?, ?, ?, ?)"
                        " ON CONFLICT(day, route, status_class)"
                        " DO UPDATE SET count = count + excluded.count",
                        (day, template, cls, count))
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("request counts were not written (%s: %s)",
                     type(exc).__name__, exc)


def summary(*, days: int = 30) -> dict[str, Any]:
    """What the ledger holds: requests per day by class, and the top routes.

    Flushes first, so the dashboard reads the present rather than the last
    minute but one. Raises on a broken database, unlike the write path —
    somebody asked, and a wrong silent answer would be worse (the same
    split `claude/ledger.py` makes).
    """
    flush()
    cutoff = (datetime.now(UTC) - timedelta(days=days)).strftime("%Y-%m-%d")
    from mtglab.auth import db
    with db.connection() as con:
        by_day: dict[str, dict[str, Any]] = {}
        for day, cls, count in con.execute(
                "SELECT day, status_class, sum(count) FROM request_log"
                " WHERE day >= ? GROUP BY day, status_class ORDER BY day",
                (cutoff,)).fetchall():
            row = by_day.setdefault(day, {"day": day, "total": 0})
            row[cls] = count
            row["total"] += count
        top = [
            {"route": route, "count": count}
            for route, count in con.execute(
                "SELECT route, sum(count) FROM request_log"
                " WHERE day >= ? GROUP BY route"
                " ORDER BY sum(count) DESC LIMIT 12",
                (cutoff,)).fetchall()
        ]
    return {"days": list(by_day.values()), "top_routes": top}


def install(app: FastAPI) -> None:
    """Register the counting middleware.

    Called after the security-headers middleware so this wraps it and the
    auth middleware both — a refusal that never reached routing is still a
    request the instance answered, and the census would be fiction without
    them. An exception out of the stack is counted as a 500 and re-raised;
    the error path is the traffic most worth seeing.
    """

    @app.middleware("http")
    async def count_requests(
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        try:
            response = await call_next(request)
        except Exception:
            record(_template(request), 500)
            raise
        record(_template(request), response.status_code)
        return response


#: The real implementations, aliased. The suite's autouse stub
#: (`tests/conftest.py::_no_request_counts`) replaces `record` and `flush`
#: for every test — the third writer found resolving its database from
#: ambient `config`, after the usage ledger and the deck log — and
#: `tests/test_traffic.py`, the one file that is *about* this module,
#: points them back at these to exercise the real path against a scratch
#: directory.
_REAL_RECORD = record
_REAL_FLUSH = flush
