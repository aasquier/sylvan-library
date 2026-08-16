"""Accounts, and the single path that verifies one.

`authenticate()` is the only function in the project that turns a password into
an identity. Everything the checklist in `docs/HOSTING.md` §1 asks for about
login lives inside it rather than in the route that calls it — one generic
answer for every failure, a hash computed even when the account does not exist,
and disabled checked *after* the verification so that "no such user", "wrong
password" and "account disabled" cost the same and say the same thing.

Which is worth stating plainly, because the temptation runs the other way: a
helpful login form that says "that account is disabled" is a login form that
confirms the account exists to anybody who asks.

Email is here as a column and nowhere else. ADR 16 makes it the first personal
data this project stores, and it inherits CLAUDE.md rule 5's reasoning about
what a breach means: **it must never reach a log line, an artifact, or a Claude
tool result.** The `User` record below is what the API serialises, and it goes
to its own holder, to an authenticated admin, or to the maintainer's terminal —
never into anything shared.

`set_admin` and `set_disabled` are the other two functions here worth reading
before editing. Both **refuse to remove the last admin who can sign in**
(`LastAdmin`), and both do it inside a `BEGIN IMMEDIATE` transaction rather than
in the handler that called them. ADR 17: the maintainer is the only admin this
deployment has by design, so each of those calls was one keystroke away from a
lockout that only a shell on the machine could undo.
"""

from __future__ import annotations

import re
import sqlite3
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

from mtglab.auth import passwords

# Deliberately narrow. A username is a login handle, not a display name -- it
# ends up in URLs, log lines and `mtglab users list` output, and the set of
# characters that are unambiguous in all three is small. Case-insensitive at
# the database (COLLATE NOCASE), so `Aaron` and `aaron` are one account.
USERNAME_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._-]{1,31}$")

# Not an RFC 5322 validator, and not trying to be. The only check that means
# anything about deliverability is sending a message to the address, which is
# what the invite flow does; everything short of that is a shape test whose
# false negatives are somebody's perfectly real address.
EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")

#: RFC 5321 §4.5.3.1.3 caps a whole address at 254 octets, so this rejects
#: nothing that was ever deliverable. It is a length bound rather than a
#: taste, and it is checked *before* `EMAIL_RE` because that pattern is
#: polynomial in what it is handed and sits on the unauthenticated claim
#: path -- the one place a stranger chooses the input.
MAX_EMAIL = 254


class UserExists(ValueError):
    """Raised rather than silently taking over an existing account."""


class NoSuchUser(ValueError):
    """Raised when an operation names an account that is not there."""


class InvalidUsername(ValueError):
    """Raised when a username could not be a username."""


class InvalidEmail(ValueError):
    """Raised when an email address could not be an email address."""


class LastAdmin(ValueError):
    """Raised rather than leaving an instance with nobody who can administer it.

    ADR 17. `set_admin(..., False)` and `set_disabled(..., True)` are each one
    call away from a lockout whose only remedy is a shell on the machine, and
    this is here rather than in a handler so that the CLI, the admin routes and
    anything written later inherit the refusal instead of each remembering it.
    """


@dataclass(frozen=True)
class User:
    """An account, without its password hash.

    The hash is not a field on purpose. This record gets returned from the API,
    logged, and printed; a hash that is never loaded into it cannot leak out of
    it. `authenticate` reads the column directly and keeps it local.
    """

    id: int
    username: str
    email: str | None
    is_admin: bool
    disabled_at: datetime | None
    created_at: datetime

    @property
    def disabled(self) -> bool:
        return self.disabled_at is not None

    def as_dict(self, *, include_email: bool = False) -> dict[str, Any]:
        """The account, serialised. **Without the address unless asked.**

        Opt-in rather than opt-out, because the rule is easy to state and hard
        to remember: an address must never reach a log line, an artifact or a
        tool result (ADR 16), and every one of those would have been written by
        somebody serialising a user and not thinking about the email field.

        Two callers pass `True`, and the pair is the whole of the rule as ADR 17
        restates it: **an address may be serialised only into a response an
        admin authenticated for.** `mtglab users list` prints to the
        maintainer's own terminal, and `api/admin.py` answers a caller the
        middleware has already established is an admin. A third caller needs the
        argument made again — `tests/test_isolation.py` is where it is pinned.
        """
        body = {
            "id": self.id,
            "username": self.username,
            "is_admin": self.is_admin,
            "disabled": self.disabled,
            "created_at": self.created_at.isoformat(),
        }
        return {**body, "email": self.email} if include_email else body


def _row_to_user(row: sqlite3.Row) -> User:
    return User(
        id=row["id"],
        username=row["username"],
        email=row["email"],
        is_admin=bool(row["is_admin"]),
        disabled_at=datetime.fromisoformat(row["disabled_at"])
        if row["disabled_at"] else None,
        created_at=datetime.fromisoformat(row["created_at"]),
    )


def normalise_username(username: str) -> str:
    """Trim and validate. Raises `InvalidUsername`; does not change case."""
    candidate = username.strip()
    if not USERNAME_RE.match(candidate):
        raise InvalidUsername(
            f"{username!r} is not a usable username -- 2 to 32 characters, "
            "letters, digits, dot, dash or underscore, starting with a letter "
            "or digit")
    return candidate


def normalise_email(email: str | None) -> str | None:
    """Trim, lowercase and shape-check. `None` and empty both mean absent."""
    if email is None or not email.strip():
        return None
    candidate = email.strip().lower()
    if len(candidate) > MAX_EMAIL or not EMAIL_RE.match(candidate):
        # The message quotes a bounded slice rather than the input: an
        # oversized address is exactly the case that reaches here, and echoing
        # it whole would put the caller's own megabyte back into an exception
        # string, a log line and an API response.
        raise InvalidEmail(
            f"{email[:MAX_EMAIL]!r} does not look like an email address")
    return candidate


def create(con: sqlite3.Connection, username: str, *,
           password: str | None = None, email: str | None = None,
           is_admin: bool = False) -> User:
    """Add an account.

    `password=None` creates an account that **cannot log in yet** — which is
    what ADR 16's invite issues, and why the column is nullable. Until the
    email half lands, the only caller that uses it is a test.
    """
    name = normalise_username(username)
    address = normalise_email(email)
    password_hash = passwords.hash_password(password) if password else None
    try:
        with con:
            cur = con.execute(
                "INSERT INTO users (username, password_hash, email, is_admin,"
                " created_at) VALUES (?, ?, ?, ?, ?)",
                (name, password_hash, address, int(is_admin),
                 datetime.now(UTC).isoformat()))
    except sqlite3.IntegrityError as exc:
        # Both unique columns land here. Say which, because "that is taken" for
        # an address the caller cannot see is an unhelpful place to stop.
        which = "email address" if "email" in str(exc) else "username"
        raise UserExists(f"that {which} is already registered") from exc
    fetched = get_by_id(con, int(cur.lastrowid or 0))
    if fetched is None:                                # unreachable in practice
        raise NoSuchUser(name)
    return fetched


def get(con: sqlite3.Connection, username: str) -> User | None:
    """One account by name, case-insensitively. `None` if there is no such."""
    row = con.execute("SELECT * FROM users WHERE username = ?",
                      (username.strip(),)).fetchone()
    return _row_to_user(row) if row else None


def get_by_id(con: sqlite3.Connection, user_id: int) -> User | None:
    row = con.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    return _row_to_user(row) if row else None


def get_by_email(con: sqlite3.Connection, email: str) -> User | None:
    """One account by address. The lookup the reset flow will need.

    Note for whoever builds that flow: a `None` from here must produce the same
    response as a hit (ADR 16), so this returning `None` is not permission to
    tell the caller anything.
    """
    address = normalise_email(email)
    if address is None:
        return None
    row = con.execute("SELECT * FROM users WHERE email = ?",
                      (address,)).fetchone()
    return _row_to_user(row) if row else None


def all_users(con: sqlite3.Connection) -> list[User]:
    rows = con.execute("SELECT * FROM users ORDER BY username COLLATE NOCASE")
    return [_row_to_user(row) for row in rows]


def count(con: sqlite3.Connection) -> int:
    return int(con.execute("SELECT count(*) AS n FROM users").fetchone()["n"])


@contextmanager
def _exclusive(con: sqlite3.Connection) -> Iterator[None]:
    """`with con:`, except the write lock is taken before the first read.

    sqlite3 in its default mode opens a transaction on the first *DML* statement
    and not on a `SELECT`, so the obvious spelling — check inside `with con:`,
    then update — runs the check outside any transaction at all. Two callers
    demoting two different admins would each read two, each write one, and the
    instance would end with none.

    `BEGIN IMMEDIATE` takes the write lock up front, so the second caller waits
    (`busy_timeout`) and then re-reads a world in which the first has already
    committed. A rule enforced by a read followed by an unrelated write is not
    enforced. ADR 17.
    """
    con.execute("BEGIN IMMEDIATE")
    try:
        yield
    except BaseException:
        con.rollback()
        raise
    con.commit()


# An admin who could actually log in right now: the flag, plus a password to
# log in with, plus not disabled. All three, and the two beyond the flag are the
# ones that make this worth a named query.
#
# An instance whose only remaining admin is an unclaimed invite is locked out
# exactly as thoroughly as one with no admin at all -- nobody can sign in, and
# nobody can promote anybody. A guard that counted that row would report success
# while the lockout happened, which is the failure it exists to prevent. ADR 17.
_USABLE_ADMINS = ("SELECT id FROM users WHERE is_admin = 1"
                  " AND password_hash IS NOT NULL AND disabled_at IS NULL")


def usable_admin_ids(con: sqlite3.Connection) -> set[int]:
    """Every account that is an admin *and* able to sign in as one.

    Public because "how many admins does this instance have" is a question the
    admin surface asks too — a page that greys out the last demote button is
    friendlier than one whose button returns 409 — and a second spelling of the
    predicate is a second thing to keep in step.
    """
    return {int(row["id"]) for row in con.execute(_USABLE_ADMINS)}


def _refuse_if_last_admin(con: sqlite3.Connection, user_id: int,
                          doing: str) -> None:
    """Raise `LastAdmin` if this operation would take the last one away.

    Called from inside the caller's transaction, after `BEGIN IMMEDIATE`, so the
    count cannot change between the check and the write.

    An operation on an account that is not currently a usable admin changes the
    count by nothing and is always allowed — including when the count is
    already zero, where refusing would mean an instance with no admin could
    never disable anybody.
    """
    admins = usable_admin_ids(con)
    if user_id in admins and len(admins) == 1:
        raise LastAdmin(
            f"refusing to {doing}: this is the only admin who can sign in. "
            "Promote somebody else first.")


def has_password(con: sqlite3.Connection, user_id: int) -> bool:
    """Whether this account can log in at all. For `mtglab users list`."""
    row = con.execute("SELECT password_hash FROM users WHERE id = ?",
                      (user_id,)).fetchone()
    if row is None:
        raise NoSuchUser(str(user_id))
    return row["password_hash"] is not None


def set_password(con: sqlite3.Connection, user_id: int, password: str) -> int:
    """Set a password and revoke every session for that account.

    Returns the number of sessions ended. One transaction, so there is no
    window in which the password has changed and the old sessions are still
    live — which is the failure ADR 16 is pointing at when it says a reset that
    leaves the attacker logged in has answered the wrong question.

    The `DELETE` is written out here rather than delegated to
    `sessions.delete_for_user`, which opens a transaction of its own: sqlite3's
    connection context manager does not nest, so the inner block would commit
    the outer one and reopen exactly the window this function exists to close.
    """
    password_hash = passwords.hash_password(password)
    with con:
        cur = con.execute("UPDATE users SET password_hash = ? WHERE id = ?",
                          (password_hash, user_id))
        if cur.rowcount == 0:
            raise NoSuchUser(str(user_id))
        revoked = con.execute("DELETE FROM sessions WHERE user_id = ?",
                              (user_id,)).rowcount
    return revoked


def apply_username(con: sqlite3.Connection, user_id: int,
                   username: str) -> str:
    """Rename an account **without opening a transaction of its own.**

    For callers that already hold one. `tokens.redeem` is the reason it exists:
    a name and a password chosen on the same form have to land in the same
    transaction as the token being spent, or a rejected name leaves a spent
    link behind — an invite that cannot be retried, which is the exact failure
    that function's docstring is written against. `set_password` records the
    same constraint from the other side: sqlite3's connection context manager
    does not nest, so an inner `with con:` would commit the outer one.

    Returns the normalised name. Raises `UserExists` if it is taken, which
    `COLLATE NOCASE` on the column makes case-insensitive — so `Ada` collides
    with `ada`, while *this* account changing its own capitalisation does not
    collide with itself.
    """
    name = normalise_username(username)
    try:
        cur = con.execute("UPDATE users SET username = ? WHERE id = ?",
                          (name, user_id))
    except sqlite3.IntegrityError as exc:
        raise UserExists("that username is already taken") from exc
    if cur.rowcount == 0:
        raise NoSuchUser(str(user_id))
    return name


def set_username(con: sqlite3.Connection, user_id: int, username: str) -> str:
    """Rename an account, in a transaction of its own.

    **Sessions are deliberately left alone.** A username is an identifier, not
    a credential: nothing is authenticated by it once a session exists, since
    `sessions` keys on `user_id`. That is the opposite of `set_password`, which
    revokes everything, and the difference is worth stating rather than
    inferring — changing your handle should not sign you out of your other
    browser.
    """
    with con:
        return apply_username(con, user_id, username)


def set_disabled(con: sqlite3.Connection, user_id: int, disabled: bool) -> int:
    """Disable or re-enable an account. Returns sessions ended.

    Disabling revokes sessions too. An account that can no longer log in but
    whose existing cookie still works has not been disabled in any sense the
    person doing it meant.

    Raises `LastAdmin` rather than disabling the only admin who can sign in.
    """
    stamp = datetime.now(UTC).isoformat() if disabled else None
    with _exclusive(con):
        if disabled:
            _refuse_if_last_admin(con, user_id, "disable that account")
        cur = con.execute("UPDATE users SET disabled_at = ? WHERE id = ?",
                          (stamp, user_id))
        if cur.rowcount == 0:
            raise NoSuchUser(str(user_id))
        revoked = con.execute("DELETE FROM sessions WHERE user_id = ?",
                              (user_id,)).rowcount if disabled else 0
    return revoked


def set_admin(con: sqlite3.Connection, user_id: int, is_admin: bool) -> None:
    """Grant or revoke admin. Raises `LastAdmin` rather than revoking the last.

    Sessions are deliberately *not* revoked either way. Admin is read fresh from
    the account on every request (`api/auth.py:scope_for_token`), so a demotion
    takes effect on the demoted party's very next call without signing them out
    of an app they are still entitled to use as themselves.
    """
    with _exclusive(con):
        if not is_admin:
            _refuse_if_last_admin(con, user_id, "revoke admin")
        cur = con.execute("UPDATE users SET is_admin = ? WHERE id = ?",
                          (int(is_admin), user_id))
        if cur.rowcount == 0:
            raise NoSuchUser(str(user_id))


def delete(con: sqlite3.Connection, user_id: int) -> int:
    """Delete an account outright. Returns the number of sessions ended.

    The irreversible sibling of `set_disabled`, and wanted for the case
    disabling cannot serve: `username` and `email` are `UNIQUE COLLATE NOCASE`,
    so a disabled account still holds both, and an address cannot be re-invited
    while a row is sitting on it. Disabling revokes; this releases.

    **Sessions and tokens go with it, and not by a `DELETE` written here.**
    Both tables declare `ON DELETE CASCADE`, and `db.py` turns `foreign_keys`
    on, which is what makes those clauses more than a comment. The count is
    taken first only so the caller has something true to print.

    Raises `LastAdmin` rather than deleting the only admin who can sign in --
    the same guard `set_disabled` and `set_admin` use, for the reason ADR 17
    gives: the rule belongs in the core so the CLI and the admin route cannot
    disagree about it. Deleting is the third door to a lockout and the only one
    with nothing to undo it.
    """
    with _exclusive(con):
        _refuse_if_last_admin(con, user_id, "delete that account")
        revoked = int(con.execute(
            "SELECT count(*) AS n FROM sessions WHERE user_id = ?",
            (user_id,)).fetchone()["n"])
        cur = con.execute("DELETE FROM users WHERE id = ?", (user_id,))
        if cur.rowcount == 0:
            raise NoSuchUser(str(user_id))
    return revoked


def authenticate(con: sqlite3.Connection, username: str,
                 password: str) -> User | None:
    """The one place a password becomes an identity. `None` means no.

    Every refusal costs one Argon2 verification, including the ones that could
    be answered without doing any work at all — an unknown username, and an
    account created by an invite that has not been claimed. That is the point:
    a login that returns faster for a name nobody has registered has told the
    person asking that nobody has registered it.
    """
    row = con.execute(
        "SELECT id, password_hash, disabled_at FROM users WHERE username = ?",
        (username.strip(),)).fetchone()

    if row is None:
        passwords.verify_dummy(password)
        return None

    stored: str | None = row["password_hash"]
    if not passwords.verify(stored, password):
        return None

    # Checked *after* the hash, not before, so a disabled account is refused in
    # the same time and with the same message as a wrong password.
    if row["disabled_at"] is not None:
        return None

    # The one moment an old hash can be upgraded for free: the plaintext is in
    # hand and has just been proven correct.
    if stored is not None and passwords.needs_rehash(stored):
        with con:
            con.execute("UPDATE users SET password_hash = ? WHERE id = ?",
                        (passwords.hash_password(password), row["id"]))

    return get_by_id(con, int(row["id"]))
