"""A fixed window in SQLite, guarding the one endpoint that takes a password.

`docs/HOSTING.md` §1 lists this under "none of it optional", and says a fixed
window in SQLite is enough — you do not need Redis. It is right: this is a
private site with a dozen accounts on one machine, and the thing being
prevented is an unattended script working through a password list, not a
distributed campaign.

**Two keys per attempt, because either alone is trivially sidestepped.** By
account only, and an attacker sprays one password across every username. By
address only, and a botnet's worth of addresses each get a full budget against
one account. The account key bounds how hard any single account can be
attacked; the address key bounds how much any single client can attempt.

Only *failures* count, and a success clears both. Somebody logging in
repeatedly and correctly — a shared machine, a flaky network, a test suite — is
not what this is for, and locking them out would be the limiter causing the
outage it exists to prevent.

The window is fixed rather than sliding, which means an attacker who times it
right gets two full budgets across a window boundary. That is a real property
and an acceptable one: 20 attempts instead of 10, against Argon2 at 19 MiB a
go, is not the difference between safe and breached.

**Two things widened when the email half arrived (ADR 16).** The table is still
called `login_attempts`, which is now half a lie — it counts attempts against a
budget, and the reset endpoint is the caller that made that distinction
visible: a reset has no "success" to clear the counter with, so *every* request
is counted, not only the ones that went nowhere. Renaming a table nothing else
reads was not worth a third migration.

And "address" is overloaded here in a way worth stating rather than tidying:
`address_key` has always meant the client's IP, and ADR 16's "rate-limited per
address" means the mailbox. The mailbox keys are `email_key`, which hashes,
and the reason it hashes is not secrecy — it is that a limiter keyed on the
plaintext would accumulate the addresses of people who do *not* have accounts,
which is personal data this project has no reason to hold (ADR 16 again).
"""

from __future__ import annotations

import hashlib
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta


@dataclass(frozen=True)
class Limit:
    """How many failures inside how long."""

    failures: int
    window: timedelta

    def describe(self) -> str:
        minutes = int(self.window.total_seconds() // 60)
        return f"{self.failures} attempts per {minutes} minutes"


# Ten wrong passwords for one account in a quarter hour is somebody who has
# forgotten theirs; the eleventh is a script. Thirty from one address is
# generous enough for a household behind one NAT and far short of useful for
# guessing.
PER_ACCOUNT = Limit(failures=10, window=timedelta(minutes=15))
PER_ADDRESS = Limit(failures=30, window=timedelta(minutes=15))

# Password reset. Three links to one mailbox in an hour covers "it did not
# arrive, try again" twice over; beyond that somebody is using the endpoint to
# mail-bomb an address they do not control, which is the abuse this stops.
# Ten from one client covers a household sharing a NAT and a person with two
# accounts.
RESET_PER_MAILBOX = Limit(failures=3, window=timedelta(hours=1))
RESET_PER_ADDRESS = Limit(failures=10, window=timedelta(hours=1))

# Redeeming a link. The token is 256 bits, so this is not about guessing --
# it is that `/api/auth/claim` is unauthenticated and hashes a password with
# Argon2, and 19 MiB per request is worth a ceiling even behind a check that
# rejects a bad token before any of it is spent.
CLAIM_PER_ADDRESS = Limit(failures=20, window=timedelta(minutes=15))


def account_key(username: str) -> str:
    return f"user:{username.strip().lower()}"


def address_key(address: str, *, scope: str = "login") -> str:
    """A client's IP, per flow.

    Scoped so that failing to redeem a link cannot spend the budget somebody
    needs to log in. One shared IP counter across every unauthenticated
    endpoint would make any one of them a way to lock a client out of the
    others.
    """
    return f"ip:{scope}:{address}"


def email_key(address: str) -> str:
    """A mailbox, keyed by hash.

    Never the plaintext: a reset request for an address with no account would
    otherwise store that address, which is personal data the project has no
    reason to be holding for somebody who is not a user. 32 hex characters of
    SHA-256 is 128 bits, which is a great deal more than is needed to keep a
    handful of accounts from colliding.
    """
    digest = hashlib.sha256(address.strip().lower().encode("utf-8")).hexdigest()
    return f"mailbox:{digest[:32]}"


def _now() -> datetime:
    return datetime.now(UTC)


def _current(con: sqlite3.Connection, key: str,
             limit: Limit) -> tuple[datetime, int]:
    """This key's window start and failure count, both reset if it has lapsed."""
    now = _now()
    row = con.execute(
        "SELECT window_start, failures FROM login_attempts WHERE key = ?",
        (key,)).fetchone()
    if row is None:
        return now, 0
    started = datetime.fromisoformat(row["window_start"])
    if now - started >= limit.window:
        return now, 0
    return started, int(row["failures"])


def exhausted(con: sqlite3.Connection, key: str, limit: Limit) -> bool:
    """Is this key out of attempts? A read; records nothing."""
    _, failures = _current(con, key, limit)
    return failures >= limit.failures


def record_failure(con: sqlite3.Connection, key: str, limit: Limit) -> int:
    """Count one failed attempt against this key. Returns the new count."""
    started, failures = _current(con, key, limit)
    with con:
        con.execute(
            "INSERT INTO login_attempts (key, window_start, failures)"
            " VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET"
            " window_start = excluded.window_start, failures = excluded.failures",
            (key, started.isoformat(), failures + 1))
    return failures + 1


def clear(con: sqlite3.Connection, key: str) -> None:
    """Forget this key's failures. Called on a successful login."""
    with con:
        con.execute("DELETE FROM login_attempts WHERE key = ?", (key,))


def retry_after(con: sqlite3.Connection, key: str, limit: Limit) -> int:
    """Seconds until this key's window lapses. For a `Retry-After` header."""
    started, _ = _current(con, key, limit)
    remaining = (started + limit.window) - _now()
    return max(1, int(remaining.total_seconds()))


def purge_stale(con: sqlite3.Connection,
                older_than: timedelta = timedelta(days=1)) -> int:
    """Drop windows nothing will read again. Returns how many."""
    cutoff = (_now() - older_than).isoformat()
    with con:
        cur = con.execute("DELETE FROM login_attempts WHERE window_start <= ?",
                          (cutoff,))
    return cur.rowcount
