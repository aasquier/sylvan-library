"""Opaque server-side sessions. Not JWTs, and for one reason: `DELETE`.

A JWT cannot be revoked without a server-side denylist, which is the very state
it exists to avoid — and this app has no stateless-scaling problem to trade
that away for. It is one machine. A row you can delete is simpler and strictly
more controllable, and "changing a password invalidates every session for that
user" (ADR 16) is one statement here and an unsolved problem with tokens.

The token is `secrets.token_urlsafe(32)` — 256 bits from the OS. What is stored
is its SHA-256, never the token itself, so reading `app.db` does not hand over
live sessions.

**SHA-256 and not Argon2**, which looks like an inconsistency next to
`passwords.py` and is not. Argon2's cost exists to make guessing a *low-entropy*
secret expensive; a 256-bit random token cannot be guessed at any cost, so the
only thing a slow hash would buy is a 19 MiB allocation on every single
authenticated request — a denial of service the app performs on itself.
"""

from __future__ import annotations

import hashlib
import secrets
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

# 32 bytes, urlsafe-encoded to 43 characters. ADR 5 names this exactly.
TOKEN_BYTES = 32

# Two weeks, matching the `Max-Age` on the cookie in `docs/HOSTING.md` §1. The
# cookie's expiry is a request to the browser; this one is the enforcement.
LIFETIME = timedelta(days=14)

# `last_seen_at` is written at most this often. Every authenticated request
# would otherwise be a write, which on WAL is cheap but not free, and the value
# is only ever read by a human wondering when an account was last used.
TOUCH_INTERVAL = timedelta(minutes=5)


@dataclass(frozen=True)
class Session:
    """A live session. Never carries the token -- only what it resolved to."""

    user_id: int
    created_at: datetime
    expires_at: datetime


def _now() -> datetime:
    return datetime.now(UTC)


def _hash(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def create(con: sqlite3.Connection, user_id: int, *,
           lifetime: timedelta = LIFETIME) -> str:
    """Open a session and return its token. The only time the token exists.

    The caller sends it to the browser and forgets it; there is no way to read
    it back out of the database, by design.
    """
    token = secrets.token_urlsafe(TOKEN_BYTES)
    now = _now()
    with con:
        con.execute(
            "INSERT INTO sessions (token_hash, user_id, created_at, expires_at,"
            " last_seen_at) VALUES (?, ?, ?, ?, ?)",
            (_hash(token), user_id, now.isoformat(),
             (now + lifetime).isoformat(), now.isoformat()))
    return token


def lookup(con: sqlite3.Connection, token: str) -> Session | None:
    """Resolve a token, or `None` if it is unknown or expired.

    An expired row is deleted on the way past rather than left to `purge()`.
    The cost is one write on a request that was going to be refused anyway, and
    the benefit is that the common case needs no scheduled cleanup at all.
    """
    if not token:
        return None
    row = con.execute(
        "SELECT user_id, created_at, expires_at, last_seen_at FROM sessions"
        " WHERE token_hash = ?", (_hash(token),)).fetchone()
    if row is None:
        return None

    now = _now()
    expires = datetime.fromisoformat(row["expires_at"])
    if expires <= now:
        delete(con, token)
        return None

    last_seen = datetime.fromisoformat(row["last_seen_at"]) \
        if row["last_seen_at"] else None
    if last_seen is None or now - last_seen >= TOUCH_INTERVAL:
        with con:
            con.execute("UPDATE sessions SET last_seen_at = ?"
                        " WHERE token_hash = ?", (now.isoformat(), _hash(token)))

    return Session(user_id=row["user_id"],
                   created_at=datetime.fromisoformat(row["created_at"]),
                   expires_at=expires)


def delete(con: sqlite3.Connection, token: str) -> None:
    """End one session. Logging out, and half of regenerating on login."""
    with con:
        con.execute("DELETE FROM sessions WHERE token_hash = ?", (_hash(token),))


def delete_for_user(con: sqlite3.Connection, user_id: int) -> int:
    """End every session for one account. Returns how many.

    Called whenever a password changes (ADR 16). A reset is usually somebody
    who suspects compromise, and a reset that leaves the attacker logged in has
    answered the wrong question.
    """
    with con:
        cur = con.execute("DELETE FROM sessions WHERE user_id = ?", (user_id,))
    return cur.rowcount


def purge_expired(con: sqlite3.Connection) -> int:
    """Drop every session past its expiry. Returns how many."""
    with con:
        cur = con.execute("DELETE FROM sessions WHERE expires_at <= ?",
                          (_now().isoformat(),))
    return cur.rowcount


def count_for_user(con: sqlite3.Connection, user_id: int) -> int:
    """How many live sessions this account has. For `mtglab users list`."""
    row = con.execute(
        "SELECT count(*) AS n FROM sessions WHERE user_id = ? AND expires_at > ?",
        (user_id, _now().isoformat())).fetchone()
    return int(row["n"])
