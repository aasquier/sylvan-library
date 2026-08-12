"""The maintainer, reconciled to admin at every start. ADR 17.

A fresh instance has an empty `users` table, which means no session can be
created, which means any bootstrap that runs "as an admin" is circular. The way
out of that circle is configuration: `MTGLAB_ADMIN_EMAIL` names the maintainer,
and this module makes three things true of that address every time the app or a
`mtglab users` command starts.

- The account **exists** — created unclaimed (`password_hash IS NULL`) if it
  did not, which is ADR 16's own shape for an account whose password nobody
  else has ever seen.
- It is an **admin**.
- It is **not disabled**.

Reconciling rather than only creating is the whole point, and it is the
difference between this and the first-account-wins option ADR 17 rejected. A
demotion, an accidental disable, or a backup restored from before the account
was made an admin are all repaired by a restart — an instance whose maintainer
has been locked out is one reboot from being administrable again, with no shell.

**Nothing here sends mail.** A boot that depends on a mail provider is a boot
that fails when the provider does, and the account does not need one to become
usable: `invites.send_reset` serves unclaimed accounts deliberately, so the
maintainer claims a bootstrapped account from the sign-in page's reset link, or
over `mtglab users invite` from a shell. That also keeps this module inside the
package rule that no test sends mail.

Unset, `ensure_maintainer` does nothing and touches no file. `mtglab ui` on a
laptop has no accounts at all and must not acquire one as a side effect of being
run.
"""

from __future__ import annotations

import logging
import sqlite3

from mtglab import config
from mtglab.auth import db, users

_LOG = logging.getLogger("mtglab.auth")


def username_for(email: str) -> str:
    """A login handle from an address' local part, sanitised to fit.

    `mtglab users invite` has the same problem and answers it differently — it
    refuses and asks for `--username`, because an invited person has to be told
    the handle they were given and one invented by a mangling rule is a bad
    thing to have to explain.

    Here there is nobody to ask. This runs unattended at boot, and a refusal
    would mean an instance with no admin because an address had a `+` in it. So
    it mangles, deterministically, and the maintainer can rename by editing the
    row or by taking the fallback: `admin`.
    """
    local = email.partition("@")[0]
    cleaned = "".join(c for c in local if c.isalnum() or c in "._-").lstrip("._-")
    try:
        return users.normalise_username(cleaned)
    except users.InvalidUsername:
        return "admin"


def _unique_username(con: sqlite3.Connection, wanted: str) -> str:
    """`wanted`, or `wanted2`, `wanted3`... if somebody already has it.

    Only reachable when the configured address is new *and* its handle collides
    with an existing account — a friend invited as `aaron` before the maintainer
    was configured. Renaming theirs would be worse.
    """
    if users.get(con, wanted) is None:
        return wanted
    for suffix in range(2, 100):
        candidate = f"{wanted}{suffix}"
        if users.get(con, candidate) is None:
            return candidate
    raise users.UserExists(f"no free username near {wanted!r}")


def ensure_maintainer(con: sqlite3.Connection | None = None) -> users.User | None:
    """Make the configured address an enabled admin. `None` when unconfigured.

    Idempotent, and silent when there is nothing to do: the steady state is
    every boot after the first finding the account already correct and writing
    no log line. Every *change* is logged, because a reconciliation that
    silently promotes an account is one nobody can audit after the fact.

    `con=None` opens and closes `app.db` itself, which is what a startup hook
    wants. Passing one in is for a caller that already has the connection open.
    """
    address = config.admin_email()
    if not address:
        return None

    if con is None:
        with db.connection() as owned:
            return ensure_maintainer(owned)

    try:
        normalised = users.normalise_email(address)
    except users.InvalidEmail:
        # Loud, and not fatal. Refusing to start would turn a typo in one
        # environment variable into an instance that serves nothing, when the
        # app is perfectly capable of running while its admin is misconfigured.
        # The address is the maintainer's own and is already in the deployment
        # config, so it is the one address ADR 16's no-logging rule is not
        # protecting from the maintainer.
        _LOG.error("MTGLAB_ADMIN_EMAIL=%r is not an email address; "
                   "no maintainer account was reconciled", address)
        return None
    if normalised is None:                             # unreachable: `if not`
        return None

    account = users.get_by_email(con, normalised)
    if account is None:
        name = _unique_username(con, username_for(normalised))
        account = users.create(con, name, email=normalised, is_admin=True)
        _LOG.warning(
            "created maintainer account %r from MTGLAB_ADMIN_EMAIL -- it has "
            "no password yet; use the reset link on the sign-in page or "
            "`mtglab users invite`", account.username)
        return account

    if not account.is_admin:
        users.set_admin(con, account.id, True)
        _LOG.warning("promoted %r to admin from MTGLAB_ADMIN_EMAIL",
                     account.username)
    if account.disabled:
        # Re-enabling is the break-glass half, and it is deliberate: whoever
        # can set this variable can already deploy code to the instance, so it
        # confers nothing new -- and without it a maintainer disabled by
        # accident has no way back that does not involve SSH.
        users.set_disabled(con, account.id, False)
        _LOG.warning("re-enabled %r from MTGLAB_ADMIN_EMAIL", account.username)

    fetched = users.get_by_id(con, account.id)
    return fetched if fetched is not None else account
