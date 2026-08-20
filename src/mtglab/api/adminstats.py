"""The admin dashboard's numbers: four read-only views of this instance.

Everything here lives under `auth.ADMIN_PREFIX`, exactly as `api/admin.py`
does and for the same reason: the middleware refuses the whole prefix to a
non-admin **before routing** (ADR 17), every handler also depends on
`deps.Admin`, and every route is classified in `tests/test_isolation.py` —
the suite fails until it is.

All four views are facts about this box, read from the box: the process,
the filesystem the volume is mounted on, `app.db`, and the in-memory job
registry. No external API and no new secret.

**No invoice — an estimate, and the difference is the whole design.** Decided
with Aaron 2026-08-18: his Console account is individual, and Anthropic's Usage
& Cost Admin API exists only for organizations, so *the real bill is not
reachable from here and never will be by this route*. That has not changed.
What changed later the same day is that the page stopped refusing to say
anything about money: `claude/prices.py` multiplies the ledger's tokens by list
rates a person read on a dated day, which is an estimate and is labelled one
everywhere it appears. It remains a **floor** — cache writes bill at 1.25x
input and are captured nowhere — and a conversation whose model carries no rate
is counted rather than priced at zero, so the figure is never quietly wrong
downward. Adding money to the account stays a human act; the page links out
rather than pretending.

One absence survives:

- **No machine-level metrics from outside.** Fly's managed Prometheus
  (edge traffic, instance memory as the platform sees it) came later, behind a
  read-only token in `api/flymetrics.py`; *this* module still reports only what
  the process can see without asking anybody.
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
from mtglab.claude import prices, tiers

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


def _cache_breakdown(cache: Path, total: int | None) -> dict[str, int | None]:
    """The cache, tenant by tenant, with whatever is left over named as such.

    Three shelves live under `data/cache` on a deployed volume: the mana
    symbols (ADR 33), the card-motion derivatives (ADR 32) and the reading
    engine (`ocr.py`). `other_bytes` is what the named three do not account
    for, and it is the part that matters: this panel shipped naming two of
    the three while the reading engine was **38% of the cache**, so the tile
    read as a breakdown while quietly under-reporting its newest tenant. A
    remainder cannot hide a fourth shelf the way a fixed list can -- the next
    one somebody adds shows up as unexplained bytes rather than as nothing.
    """
    named = {"symbols_bytes": _size_of(cache / "symbols"),
             "cardmotion_bytes": _size_of(cache / "cardmotion"),
             "ocr_bytes": _size_of(cache / "ocr")}
    other = (None if total is None
             else max(0, total - sum(v for v in named.values() if v)))
    return {**named, "other_bytes": other}


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
        cache_bytes = _size_of(cache)
        return {
            "app_db_bytes": _size_of(config.APP_DB_PATH),
            "pool_bytes": _size_of(config.DB_PATH),
            "scryfall_bulk_bytes": _size_of(config.SCRYFALL_DIR),
            "cache_bytes": cache_bytes,
            "cache": _cache_breakdown(cache, cache_bytes),
            "decks": {
                "count": _count_dirs(config.DECKS_DIR),
                "bytes": _size_of(config.DECKS_DIR),
                "trashed": _count_dirs(config.DECKS_DIR / ".trash"),
            },
        }

    @app.get("/api/admin/stats/claude")
    def claude_stats(caller: Admin) -> dict[str, Any]:
        """Where the Claude tokens went — per mode, per model, and in dollars.

        The ledger's numbers (ADR 28's sibling in `claude/ledger.py`):
        aggregate counters only — no user, no deck, no question text. The
        caveat is part of the payload because it must ride with the numbers
        anywhere they are shown, the same rule the simulator's Tier 1
        caveats follow: cache *writes* are not captured, so these totals
        are a floor on the bill, never the bill.

        Two axes per window, because they answer different questions and
        neither substitutes for the other: **per mode** is "which surface is
        spending this", **per model** is "on which Claude" — the question
        per-account tiers made worth asking. Each is grouped in SQL rather
        than pivoted here, so a window's two lists sum to the same totals by
        construction.

        The **dollar figure is estimated from the per-model rollup only**, and
        that is not an optimisation. Pricing the per-mode one would mean
        pricing rows whose model column reads `(various)` — every rate would be
        a guess, and the guess would look like arithmetic.

        `estimated_usd` carries its own honesty with it: the date the rates
        were last checked by a person, and a count of conversations whose model
        this build cannot price. A caller rendering the figure without the
        second is showing a number that is wrong downward.
        """
        del caller

        def labelled(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
            """Each row, plus how to name the model on a screen.

            Beside the id rather than instead of it. The label is what renders
            (commandment 10 — a model id is technology, and the tier picker two
            tabs over already says "Sonnet"); the id stays in the payload
            because it is what the pricing question is actually about, and a
            client that wanted to group or key on it should not have to parse
            prose to get it back.
            """
            return [{**row, "model_label": tiers.label_for(row["model"])}
                    for row in rows]

        def window(since: str | None) -> dict[str, Any]:
            by_mode = ledger_summary(since=since)
            by_model = ledger_summary(since=since, by="model")
            return {
                "by_mode": labelled(by_mode),
                "by_model": labelled(by_model),
                # Priced from the unlabelled rows: the estimate keys on the id,
                # and two ids that share a label (two Opus generations, say)
                # are two rates.
                "estimated_usd": prices.estimate(by_model).as_dict(),
            }

        return {
            "windows": {
                "week": window(_ago(7)),
                "month": window(_ago(30)),
                "all": window(None),
            },
            "caveat": "Token counts are a floor on the bill, not the bill: "
                      "cache writes bill at 1.25x input and are not captured.",
            "prices": {
                "checked": prices.CHECKED.isoformat(),
                "source": prices.SOURCE,
                "note": "Estimated from list rates read by a person on the "
                        "date above, not from an invoice. A conversation "
                        "whose model is not in the table is counted, never "
                        "priced at zero.",
            },
        }

    @app.get("/api/admin/stats/fly")
    async def fly_stats(caller: Admin) -> dict[str, Any]:
        """What the platform sees: Fly's managed Prometheus, if configured.

        The one view here that leaves the box, and the only one that can
        be switched off — `FLY_METRICS_TOKEN` is the maintainer's to mint,
        and unset means `configured: false` and a widget that hides rather
        than a dashboard that looks broken.

        `run_in_threadpool` because `flymetrics.fetch` is blocking stdlib
        `urllib` and this handler is `async`: called directly it would
        stall the event loop for every other request for as long as Fly
        takes to answer.
        """
        del caller
        from starlette.concurrency import run_in_threadpool

        from mtglab.api import flymetrics
        return await run_in_threadpool(flymetrics.fetch)

    @app.get("/api/admin/stats/traffic")
    def traffic_stats(caller: Admin) -> dict[str, Any]:
        """The visitor ledger: requests per day by status class, top routes.

        Route templates and status classes only — the recording side
        (`api/traffic.py`, schema v9) never held anything finer, so this
        view cannot leak what was never kept: no addresses, no agents, no
        names, no concrete paths. `note` rides in the payload the way the
        Claude view's caveat does, because the sentence belongs wherever
        the numbers go.
        """
        del caller
        from mtglab.api import traffic
        return {
            **traffic.summary(days=30),
            "note": "Route templates and status classes only — the ledger "
                    "never records an address, an agent, a name, or a "
                    "concrete path.",
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


def ledger_summary(*, since: str | None = None,
                   by: str = "mode") -> list[dict[str, Any]]:
    """`claude.ledger.summary`, imported lazily the way the runs modules
    import their planners: the ledger rides the base install, but keeping
    the import at call time keeps this module importable in any test that
    stubs it.

    `claude.prices` is imported at module level instead, and the difference is
    deliberate: this one touches the database and is the thing tests stub,
    while that one is a table and some arithmetic with nothing to fail.
    """
    from mtglab.claude import ledger
    return ledger.summary(since=since, by=by)
