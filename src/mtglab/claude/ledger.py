"""Where the Claude money went: one row per conversation, and a roll-up.

Every mode already counted its tokens — `modes.Turn` carries them and the CLI
prints them — but the hosted instance, where the spending actually happens,
discarded the numbers with the job payload. So the only cost figure this
project had was one argue run measured by hand, and every efficiency decision
(effort levels, a cheaper model for a mode, the Batch API) was going to be
made against vibes. This module is the accounting those decisions wait on.

Three properties, all copied from `sim/cache.py`, which solved the same
problems for the same file:

* **`record` never raises.** Accounting that can fail a paid, four-minute
  dossier run is worse than no accounting. A failed write is a logged warning.
* **`mtglab.auth.db` is imported for its connection helper, not for auth** —
  that module owns `app.db`'s pragmas and migration ladder, and a second
  ladder for the same file would be worse than the import.
* **Aggregate on purpose.** A row is counters, a mode name, a model id and a
  stop reason. No user id, no deck slug, no question text — it cannot drift
  into being a chat log, and ADR 17's who-may-read-what argument never has to
  be made for it.
"""

from __future__ import annotations

import logging
import sqlite3
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

_LOG = logging.getLogger("mtglab.claude.ledger")


def _now() -> str:
    return datetime.now(UTC).isoformat()


def record(*, mode: str, model: str, stop_reason: str, requests: int,
           input_tokens: int, output_tokens: int, cache_read_tokens: int,
           path: Path | str | None = None) -> None:
    """Write one conversation's accounting. Never raises.

    Called by `modes.converse` on every way out — answer, refusal, and the
    turn-ceiling exception too, because the tokens a conversation burned
    before failing are exactly the ones worth seeing in a roll-up.
    """
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            con.execute(
                "INSERT INTO claude_usage (created_at, mode, model,"
                " stop_reason, requests, input_tokens, output_tokens,"
                " cache_read_tokens) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                (_now(), mode, model, stop_reason, requests,
                 input_tokens, output_tokens, cache_read_tokens))
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("claude usage record failed (%s: %s)",
                     type(exc).__name__, exc)


#: The axes a roll-up may group on. A tuple rather than "whatever the caller
#: passes", because these names are interpolated into SQL: they are column
#: names, which cannot be bound as parameters, so the safety has to come from
#: the value never being caller-controlled in the first place.
AXES: tuple[str, ...] = ("mode", "model")


def summary(*, since: str | None = None, by: str = "mode",
            path: Path | str | None = None) -> list[dict[str, Any]]:
    """Totals per `by`, most expensive first.

    `since` is an ISO-8601 instant compared against `created_at`, which is
    itself ISO-8601 UTC — string comparison is date comparison for free.
    Raises on a broken database: unlike `record`, this runs when somebody
    asked a question and a wrong silent answer would be worse than an error.

    `by` is `'mode'` (which surface spent it) or `'model'` (which Claude spent
    it) — the second being what per-account tiers made worth asking, and what
    `claude/prices.py` needs to put a figure in dollars next to it. The column
    has been there since schema v7; only the grouping is new.

    Both axes come back with the same keys, and every row carries **both**
    `mode` and `model`: grouping by one aggregates the other, so the field that
    was not grouped on holds `'(various)'` rather than an arbitrary winner
    picked by SQLite. A caller pricing a per-mode roll-up therefore gets
    `unpriced` rather than a number computed from whichever model happened to
    sort first, which would be wrong and would look right.
    """
    if by not in AXES:
        raise ValueError(f"cannot group by {by!r} -- one of {AXES}")
    other = "model" if by == "mode" else "mode"

    query = (f"SELECT {by},"
             f" '(various)' AS {other},"
             " count(*) AS conversations,"
             " sum(requests) AS requests,"
             " sum(input_tokens) AS input_tokens,"
             " sum(output_tokens) AS output_tokens,"
             " sum(cache_read_tokens) AS cache_read_tokens,"
             " min(created_at) AS first_at,"
             " max(created_at) AS last_at"
             " FROM claude_usage")
    args: tuple[Any, ...] = ()
    if since is not None:
        query += " WHERE created_at >= ?"
        args = (since,)
    query += f" GROUP BY {by} ORDER BY sum(input_tokens + output_tokens) DESC"

    from mtglab.auth import db
    with db.connection(path) as con:
        return [dict(row) for row in con.execute(query, args).fetchall()]
