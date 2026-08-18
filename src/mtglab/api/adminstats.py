"""The admin dashboard's numbers: four read-only views of this instance.

Everything here lives under `auth.ADMIN_PREFIX`, exactly as `api/admin.py`
does and for the same reason: the middleware refuses the whole prefix to a
non-admin **before routing** (ADR 17), every handler also depends on
`deps.Admin`, and every route is classified in `tests/test_isolation.py` —
the suite fails until it is.

All four views are facts about this box, read from the box: the process,
the filesystem the volume is mounted on, `app.db`, and the in-memory job
registry. No external API and no new secret. Two deliberate absences:

- **No dollar figure.** Decided with Aaron 2026-08-18: his Console account
  is individual, and Anthropic's Usage & Cost Admin API exists only for
  organizations. The ledger's token totals below are the ceiling of what
  this box can know, and they are labelled what they are — a floor on the
  bill (cache writes are not captured), never the bill. Adding money to the
  account stays a human act; the page links out rather than pretending.
- **No machine-level metrics from outside.** Fly's managed Prometheus
  (edge traffic, instance memory as the platform sees it) is a later PR
  behind a read-only token; this module reports what the process can see
  without asking anybody.
"""

from __future__ import annotations

import logging
import os
import shutil
import sqlite3
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

from fastapi import FastAPI

from mtglab import config
from mtglab.api import jobs
from mtglab.api.deps import Admin
from mtglab.auth import db, tokens, users

_LOG = logging.getLogger("mtglab.api.adminstats")


# ------------------------------------------------------------------ helpers

def _size_of(path: Path) -> int | None:
    """Bytes on disk, or None for "nothing there" — a fresh instance has no
    pool and an unseeded cache, and the dashboard should say absent rather
    than zero, which reads as "present and empty"."""
    try:
        if path.is_file():
            return path.stat().st_size
        if path.is_dir():
            return sum(f.stat().st_size for f in path.rglob("*") if f.is_file())
    except OSError as exc:
        _LOG.warning("could not size %s (%s)", path, exc)
    return None


def _count_dirs(path: Path) -> int:
    try:
        return sum(1 for p in path.iterdir() if p.is_dir()) if path.is_dir() else 0
    except OSError:
        return 0


def _rss() -> dict[str, Any]:
    """This process's resident memory, as well as the platform tells it.

    Deployed — the audience that matters — this is Linux, and
    `/proc/self/status` reports the *current* RSS. The dev Mac has no /proc,
    so it falls back to `getrusage`, whose `ru_maxrss` is the *peak* (and in
    bytes there, not kilobytes — the units differ per platform and both
    branches say which). `kind` rides along so the page can label the number
    honestly instead of showing a peak as a level.
    """
    proc = Path("/proc/self/status")
    if proc.exists():
        try:
            for line in proc.read_text().splitlines():
                if line.startswith("VmRSS:"):
                    return {"bytes": int(line.split()[1]) * 1024,
                            "kind": "current"}
        except (OSError, ValueError, IndexError) as exc:
            _LOG.warning("could not read /proc/self/status (%s)", exc)
    import resource
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    # Linux reports kilobytes, macOS bytes. If /proc was unreadable we are
    # probably not on Linux, but check the platform rather than assume.
    import sys
    factor = 1 if sys.platform == "darwin" else 1024
    return {"bytes": peak * factor, "kind": "peak"}


def _machine_memory() -> dict[str, Any]:
    """Total physical memory, and — where the kernel says — what is free.

    `MemAvailable` is Linux's own answer to "how much could be allocated
    before swapping"; there is no portable equivalent, so elsewhere it is
    absent rather than approximated.
    """
    out: dict[str, Any] = {"total_bytes": None, "available_bytes": None}
    try:
        pages = os.sysconf("SC_PHYS_PAGES")
        page = os.sysconf("SC_PAGE_SIZE")
        out["total_bytes"] = pages * page
    except (ValueError, OSError):
        pass
    meminfo = Path("/proc/meminfo")
    if meminfo.exists():
        try:
            for line in meminfo.read_text().splitlines():
                if line.startswith("MemAvailable:"):
                    out["available_bytes"] = int(line.split()[1]) * 1024
                    break
        except (OSError, ValueError, IndexError):
            pass
    return out


def _user_state(con: sqlite3.Connection, user: users.User) -> str:
    """The same four states `api/admin.py:_state` computes, restated here
    rather than imported because that one is that module's private helper
    and this one feeds an aggregate — if the two ever disagree, the accounts
    table and the dashboard tile disagree visibly on the same page, which is
    the failure mode a shared import would also produce, one refactor later
    and harder to see."""
    if user.disabled:
        return "disabled"
    if users.has_password(con, user.id):
        return "active"
    if tokens.outstanding(con, user.id, tokens.Purpose.INVITE):
        return "invited"
    return "no password"


def _ago(days: int) -> str:
    return (datetime.now(UTC) - timedelta(days=days)).isoformat()


# ------------------------------------------------------------------- routes

def install(app: FastAPI) -> None:
    """Add the stats routes. Read-only, admin-only, all on-box."""

    @app.get("/api/admin/stats/system")
    def system_stats(caller: Admin) -> dict[str, Any]:
        """The process and the machine under it, as the box reports them."""
        del caller
        disk = shutil.disk_usage(config.DATA_DIR)
        try:
            load = list(os.getloadavg())
        except OSError:
            load = []
        return {
            "process": _rss(),
            "memory": _machine_memory(),
            "load": load,
            "cpus": os.cpu_count(),
            "disk": {"path": str(config.DATA_DIR), "total_bytes": disk.total,
                     "used_bytes": disk.used, "free_bytes": disk.free},
        }

    @app.get("/api/admin/stats/storage")
    def storage_stats(caller: Admin) -> dict[str, Any]:
        """What is on the volume, named the way the architecture names it.

        Sizes are bytes or null — null meaning "nothing there yet", which on
        a fresh instance is most of them and is information, not an error.
        """
        del caller
        cache = config.DATA_DIR / "cache"
        return {
            "app_db_bytes": _size_of(config.APP_DB_PATH),
            "pool_bytes": _size_of(config.DB_PATH),
            "scryfall_bulk_bytes": _size_of(config.SCRYFALL_DIR),
            "cache_bytes": _size_of(cache),
            "cache": {
                "symbols_bytes": _size_of(cache / "symbols"),
                "cardmotion_bytes": _size_of(cache / "cardmotion"),
            },
            "decks": {
                "count": _count_dirs(config.DECKS_DIR),
                "bytes": _size_of(config.DECKS_DIR),
                "trashed": _count_dirs(config.DECKS_DIR / ".trash"),
            },
        }

    @app.get("/api/admin/stats/claude")
    def claude_stats(caller: Admin) -> dict[str, Any]:
        """Where the Claude tokens went, per mode, over three windows.

        The ledger's numbers (ADR 28's sibling in `claude/ledger.py`):
        aggregate counters only — no user, no deck, no question text. The
        caveat is part of the payload because it must ride with the numbers
        anywhere they are shown, the same rule the simulator's Tier 1
        caveats follow: cache *writes* are not captured, so these totals
        are a floor on the bill, never the bill.
        """
        del caller
        return {
            "windows": {
                "week": ledger_summary(since=_ago(7)),
                "month": ledger_summary(since=_ago(30)),
                "all": ledger_summary(),
            },
            "caveat": "Token counts are a floor on the bill, not the bill: "
                      "cache writes are not captured, and no price table is "
                      "kept here to go stale.",
        }

    @app.get("/api/admin/stats/activity")
    def activity_stats(caller: Admin) -> dict[str, Any]:
        """Who has been here and what the instance has been doing.

        Counts throughout — the visitor ledger (route-template request
        counts, schema v9) is a later PR on its own watched branch, so
        today this is what `app.db` already knows: accounts by state,
        sessions by recency, deck edits by day, memoised simulations, and
        the job registry's census.
        """
        del caller
        with db.connection() as con:
            states: dict[str, int] = {}
            for user in users.all_users(con):
                state = _user_state(con, user)
                states[state] = states.get(state, 0) + 1

            def one(query: str, args: tuple[Any, ...] = ()) -> int:
                row = con.execute(query, args).fetchone()
                return int(row[0]) if row and row[0] is not None else 0

            sessions_total = one("SELECT count(*) FROM sessions")
            seen_day = one(
                "SELECT count(*) FROM sessions WHERE last_seen_at >= ?",
                (_ago(1),))
            seen_week = one(
                "SELECT count(*) FROM sessions WHERE last_seen_at >= ?",
                (_ago(7),))
            edits = [
                {"day": row[0], "edits": row[1]}
                for row in con.execute(
                    "SELECT substr(created_at, 1, 10) AS day, count(*)"
                    " FROM deck_log WHERE created_at >= ?"
                    " GROUP BY day ORDER BY day",
                    (_ago(30),)).fetchall()
            ]
            sim_rows = one("SELECT count(*) FROM sim_cache")

        return {
            "accounts": states,
            "sessions": {"total": sessions_total, "seen_day": seen_day,
                         "seen_week": seen_week},
            "deck_edits_by_day": edits,
            "sim_cache_rows": sim_rows,
            "jobs": jobs.census(),
        }


def ledger_summary(*, since: str | None = None) -> list[dict[str, Any]]:
    """`claude.ledger.summary`, imported lazily the way the runs modules
    import their planners: the ledger rides the base install, but keeping
    the import at call time keeps this module importable in any test that
    stubs it."""
    from mtglab.claude import ledger
    return ledger.summary(since=since)
