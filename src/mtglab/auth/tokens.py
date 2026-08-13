"""Single-use, time-limited tokens. One implementation, two entry points.

ADR 16 is specific about why this is one module rather than an invite flow and
a reset flow: **"a bespoke second path is how one of them ends up weaker."** So
an invite and a reset differ here in exactly two ways — the string in the
`purpose` column and how long the row lives — and every rule below applies to
both without being written twice.

The rules, all of them from ADR 16:

- **Stored hashed**, never in the clear. Same reasoning as `sessions.py`, and
  the same SHA-256 for the same reason: a 256-bit random token cannot be
  guessed at any cost, so Argon2 would buy nothing and cost 19 MiB a check.
- **Single use.** `redeem` marks the row consumed inside the transaction that
  sets the password, so two clicks on one link cannot both succeed.
- **Short-lived** — one hour for a reset, a week for an invite. Different
  numbers because they are different risks: a reset is a response to something
  the account holder is doing right now, and an invite grants nothing at all
  until it is used.
- **Redeeming revokes every session for that account.** A reset is usually
  somebody who suspects compromise, and one that leaves the attacker logged in
  has answered the wrong question.

`redeem` is the whole of that last pair in one transaction, and it is the
reason this module imports `users` rather than calling `users.set_password`:
sqlite3's connection context manager does not nest, so an inner `with con:`
would commit the outer one and reopen the window the transaction exists to
close. `users.set_password` documents the same trap from the other side.

**What is deliberately not here is anything that sends a message.** Issuing a
token and delivering it are separate jobs; `invites.py` is the one that joins
them, and it is the only module in this package that talks to a network.
"""

from __future__ import annotations

import hashlib
import secrets
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from enum import StrEnum

from mtglab.auth import passwords, users

# 32 bytes, urlsafe-encoded to 43 characters -- the same size as a session
# token, and for the same reason. This one travels in a URL fragment, so it has
# to survive being pasted out of a mail client.
TOKEN_BYTES = 32


class Purpose(StrEnum):
    """Why a token was issued. Not interchangeable, and checked on redemption.

    An invite lives longer than a reset, so a flow that accepted either would
    quietly hand a reset the invite's week.
    """

    INVITE = "invite"
    RESET = "reset"


# An hour for a reset, exactly as ADR 16 says. A week for an invite: it is a
# link somebody has to notice in their inbox, act on, and possibly ask about
# first, and it confers nothing until it is used -- an expired invite is a
# message to the maintainer, an expired reset is a person locked out for the
# length of one more round trip.
LIFETIMES: dict[Purpose, timedelta] = {
    Purpose.INVITE: timedelta(days=7),
    Purpose.RESET: timedelta(hours=1),
}


class TokenError(ValueError):
    """Base for every reason a token is not redeemable."""


class InvalidToken(TokenError):
    """No such token. A typo, a forgery, or a link from a purged database."""


class ExpiredToken(TokenError):
    """Real, but past its expiry."""


class UsedToken(TokenError):
    """Real, and already redeemed. Single use is the rule; this is it holding."""


class WrongPurpose(ValueError):
    """The token is fine; what was asked of it is not.

    **Deliberately not a `TokenError`.** Everything above means "this link
    cannot be redeemed", and the endpoint answers those with a 400 and a
    counted rate-limit failure, because a stream of them is somebody guessing.
    This one means the link is perfectly good and the request asked it to do
    something that kind of link does not do — renaming an account from a reset
    link. Spending the token or counting a failure for that would punish the
    holder for their client's bug.
    """


@dataclass(frozen=True)
class Token:
    """A resolved token. Never carries the secret -- only what it resolved to."""

    user_id: int
    purpose: Purpose
    created_at: datetime
    expires_at: datetime


def _now() -> datetime:
    return datetime.now(UTC)


def _hash(token: str) -> str:
    return hashlib.sha256(token.encode("utf-8")).hexdigest()


def issue(con: sqlite3.Connection, user_id: int, purpose: Purpose, *,
          lifetime: timedelta | None = None) -> str:
    """Mint a token and return it. The only time it exists in the clear.

    **Any outstanding token of the same purpose for that account is dropped**,
    so there is one live link per purpose at a time. That is what makes a
    re-issued reset a *replacement*: somebody who suspects a message went
    astray asks for another, and the one they are worried about stops working
    the moment they do. Tokens of the *other* purpose are left alone, because
    an unclaimed invite and a reset request are answers to different questions.

    Used rows survive, so a second click still reports "already used" rather
    than the flat "invalid" that leaves somebody wondering whether they
    mistyped it.
    """
    token = secrets.token_urlsafe(TOKEN_BYTES)
    now = _now()
    window = LIFETIMES[purpose] if lifetime is None else lifetime
    with con:
        con.execute(
            "DELETE FROM auth_tokens WHERE user_id = ? AND purpose = ?"
            " AND used_at IS NULL", (user_id, str(purpose)))
        con.execute(
            "INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at,"
            " expires_at) VALUES (?, ?, ?, ?, ?)",
            (_hash(token), user_id, str(purpose), now.isoformat(),
             (now + window).isoformat()))
    return token


def lookup(con: sqlite3.Connection, token: str,
           purpose: Purpose | None = None) -> Token:
    """Resolve a token, or raise saying why it cannot be used.

    Raising rather than returning `None`, unlike `sessions.lookup`, because the
    three refusals are worth telling apart *to the person holding the link* —
    "this expired" and "you already used this" are actionable and "invalid" is
    not. None of them tells anybody anything they did not already have: the
    token is the secret, and possessing it is the only way to get an answer
    that is not `InvalidToken`.

    `purpose` is checked when given. A caller that does not care is a caller
    that would accept a week-old invite as an hour-old reset.
    """
    if not token:
        raise InvalidToken("no token supplied")
    row = con.execute(
        "SELECT user_id, purpose, created_at, expires_at, used_at"
        " FROM auth_tokens WHERE token_hash = ?", (_hash(token),)).fetchone()
    if row is None:
        raise InvalidToken("that link is not valid")

    found = Purpose(row["purpose"])
    if purpose is not None and found is not purpose:
        # Deliberately the same message as an unknown token. A caller who has
        # an invite and is poking the reset endpoint learns nothing from it.
        raise InvalidToken("that link is not valid")
    if row["used_at"] is not None:
        raise UsedToken("that link has already been used")

    expires = datetime.fromisoformat(row["expires_at"])
    if expires <= _now():
        raise ExpiredToken("that link has expired")

    return Token(user_id=int(row["user_id"]), purpose=found,
                 created_at=datetime.fromisoformat(row["created_at"]),
                 expires_at=expires)


def redeem(con: sqlite3.Connection, token: str, password: str,
           purpose: Purpose | None = None,
           username: str | None = None) -> users.User:
    """Consume a token and set that account's password. One transaction.

    `username` renames the account as it is claimed, and is **refused on a
    reset**. An invite's holder is naming themselves for the first time; a
    reset's holder already has a name that other people have seen, and a
    forgotten password is not a reason to be handed a rename — worse, it makes
    "somebody got into my email" and "somebody took over my identity here" the
    same incident. The gate is the token's own `purpose`, read from the
    database, so a client that sends the field anyway cannot widen it.

    Everything that must not happen separately happens here: the token is
    marked used, the password is written, and every session for the account is
    deleted. A failure anywhere rolls back all three, so there is no state in
    which the link is spent but the password is not set — which is the failure
    that turns a forgotten password into a locked-out account.

    Three things happen *before* the transaction opens, on purpose:

    - the token is resolved, so an invalid link costs a primary-key lookup
      rather than an Argon2 hash. This endpoint is unauthenticated; hashing
      first would make it a 19 MiB-per-request denial of service.
    - the password is checked and hashed, because ~50 ms of Argon2 inside a
      write transaction is 50 ms of held lock for no reason.
    - the account is checked for `disabled_at`. **A reset must not re-enable a
      disabled account.** Disabling is the maintainer's revocation lever, and a
      lever somebody can undo from their own inbox is not one.
    """
    resolved = lookup(con, token, purpose)

    account = users.get_by_id(con, resolved.user_id)
    if account is None:                                # cascade should prevent
        raise InvalidToken("that link is not valid")
    if account.disabled:
        raise InvalidToken("that link is not valid")

    if username is not None and resolved.purpose is not Purpose.INVITE:
        raise WrongPurpose("a reset link cannot change your username")

    # Shape-checked before the transaction, for the same reason the password
    # is: a name the rules refuse should cost a regex and not a write. The
    # *uniqueness* of it cannot be settled here — only the UNIQUE index can do
    # that without a race — so the rename happens inside, where a collision
    # rolls the token spend back with it.
    if username is not None:
        users.normalise_username(username)

    password_hash = passwords.hash_password(password)
    with con:
        marked = con.execute(
            "UPDATE auth_tokens SET used_at = ?"
            " WHERE token_hash = ? AND used_at IS NULL",
            (_now().isoformat(), _hash(token)))
        if marked.rowcount == 0:
            # Redeemed by another request between the lookup above and here.
            # `UPDATE ... WHERE used_at IS NULL` is what makes single-use a
            # property of the database rather than of the check order.
            raise UsedToken("that link has already been used")
        if username is not None:
            # Raising here rolls back the `used_at` above with it, which is the
            # whole reason this is not two transactions: a taken name must
            # leave a *retryable* invite rather than a spent link and an
            # account nobody can get into.
            users.apply_username(con, resolved.user_id, username)
        con.execute("UPDATE users SET password_hash = ? WHERE id = ?",
                    (password_hash, resolved.user_id))
        con.execute("DELETE FROM sessions WHERE user_id = ?",
                    (resolved.user_id,))

    refreshed = users.get_by_id(con, resolved.user_id)
    if refreshed is None:                              # unreachable in practice
        raise InvalidToken("that link is not valid")
    return refreshed


def outstanding(con: sqlite3.Connection, user_id: int,
                purpose: Purpose) -> bool:
    """Whether this account has a live link of that kind. For `users list`."""
    row = con.execute(
        "SELECT count(*) AS n FROM auth_tokens WHERE user_id = ?"
        " AND purpose = ? AND used_at IS NULL AND expires_at > ?",
        (user_id, str(purpose), _now().isoformat())).fetchone()
    return int(row["n"]) > 0


def purge_expired(con: sqlite3.Connection,
                  keep_used_for: timedelta = timedelta(days=30)) -> int:
    """Drop what nothing will read again. Returns how many rows went.

    Expired-and-unused rows go immediately; used ones linger, so that "already
    used" survives long enough to be the answer somebody actually gets when
    they click a link twice a week apart.
    """
    now = _now()
    with con:
        cur = con.execute(
            "DELETE FROM auth_tokens WHERE (used_at IS NULL AND expires_at <= ?)"
            " OR (used_at IS NOT NULL AND used_at <= ?)",
            (now.isoformat(), (now - keep_used_for).isoformat()))
    return cur.rowcount
