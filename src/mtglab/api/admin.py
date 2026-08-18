"""The admin surface: `mtglab users` as five routes. ADR 17.

Everything here lives under `auth.ADMIN_PREFIX`, and that is not a naming
convention — it is the enforcement. `api/auth.py`'s middleware refuses the whole
prefix to a non-admin before routing, so a route added to this module is
protected by being in it. Every handler *also* depends on `deps.Admin`, which is
the only check an admin route mounted somewhere else by mistake would have.

Why a page at all, when `mtglab users` already does this: administering a hosted
app over SSH works exactly as long as the maintainer is sitting at the machine
with the key on it. The CLI stays regardless — it is the bootstrap path on a
fresh volume, the break-glass path when mail is misconfigured, and the only path
that still works when the thing that is broken is this page.

What is deliberately **not** here:

- **Setting somebody's password.** ADR 16 is unconditional: no password is ever
  chosen by one person for another, because an admin-set password is a password
  that has existed in plaintext in a chat window. `POST .../reset` mails a link
  instead, which is the same intent routed through the account holder.
- **Any leak-shaped difference between accounts.** ADR 5's 404-not-403 rule is
  about resources that belong to one person; an admin listing accounts is the
  one caller for whom every account is in scope, so `404` here means what it
  says — no such username.

`DELETE .../users/{username}` was on this list until an address needed
re-inviting and disabling turned out not to free one — `username` and `email`
are `UNIQUE`, so the row holds them either way. The conditions the entry set
are the ones it now ships under: a typed confirmation, as `decks delete` has,
and it is the *client* that types it, because a route that deletes on a bare
verb is one a mis-aimed `fetch` can fire.

**Addresses are returned from these routes**, which ADR 17 decided explicitly
and which is why `_account` passes `include_email=True`. The rule that replaced
"one caller" is narrow and still checkable: an address may be serialised only
into a response an admin authenticated for. Two mechanisms already guarantee
that of this module, and `tests/test_isolation.py` pins the third — that no
*other* module acquires the habit.
"""

from __future__ import annotations

import logging
import sqlite3
from typing import Any

from fastapi import FastAPI, HTTPException

from mtglab.api import jobs
from mtglab.api.deps import Admin
from mtglab.auth import db, invites, mail, sessions, tokens, users
from mtglab.claude import tiers

_LOG = logging.getLogger("mtglab.auth")


def _state(con: sqlite3.Connection, user: users.User) -> str:
    """The one word `mtglab users list` prints, computed the same way.

    Four states and they are genuinely different things: `disabled` is revoked,
    `active` can log in, `invited` has a link outstanding, and `no password` is
    an account whose invite expired or was never sent — which is the one that
    needs somebody to do something.
    """
    if user.disabled:
        return "disabled"
    if users.has_password(con, user.id):
        return "active"
    if tokens.outstanding(con, user.id, tokens.Purpose.INVITE):
        return "invited"
    return "no password"


def _account(con: sqlite3.Connection, user: users.User) -> dict[str, Any]:
    """One account, as the admin page sees it. **Carries the address.**"""
    return {
        **user.as_dict(include_email=True),
        "state": _state(con, user),
        "sessions": sessions.count_for_user(con, user.id),
    }


def _find(con: sqlite3.Connection, username: str) -> users.User:
    user = users.get(con, username)
    if user is None:
        raise HTTPException(status_code=404, detail=f"no account {username!r}")
    return user


def install(app: FastAPI, *,
            email_sender: mail.EmailSender | None = None) -> None:
    """Add the admin routes. Mirrors `auth.install`, including the mail seam.

    `email_sender=None` means "decide from the environment when a message is
    actually being sent", which is what a real process wants and what keeps
    every test in this suite off the network (ADR 16).
    """

    def _sender() -> mail.EmailSender:
        try:
            return (email_sender if email_sender is not None
                    else mail.sender_from_env())
        except mail.EmailNotConfigured as exc:
            # 503 rather than 500: nothing is broken, something is unset, and
            # the maintainer reading this is the person who can set it.
            raise HTTPException(status_code=503, detail=str(exc)) from exc

    @app.get("/api/admin/users")
    def list_accounts(caller: Admin) -> dict[str, Any]:
        """Every account, with the address, the state and the session count.

        `admins` is the count of admins who can actually sign in, and it is here
        so the page can grey out the last demote button rather than offering a
        click that returns 409. It is the same predicate the refusal uses —
        `users.usable_admin_ids` — because a second spelling of it is a second
        thing to keep in step.
        """
        del caller
        with db.connection() as con:
            everyone = users.all_users(con)
            return {
                "users": [_account(con, user) for user in everyone],
                "admins": len(users.usable_admin_ids(con)),
                # The tiers this build knows, so the page offers exactly what
                # the server will accept. One list, serialised — a second one
                # written in TypeScript would drift the day a tier is added,
                # and the drift would present as a control that 422s.
                "tiers": tiers.roster(),
            }

    @app.post("/api/admin/users", status_code=201)
    def invite_account(payload: dict[str, Any], caller: Admin) -> dict[str, Any]:
        """Create an unclaimed account and mail its owner a setup link.

        The same three rules `mtglab users invite` follows, for the same
        reasons: the account is created **unclaimed** rather than disabled
        (`disabled_at` is the revocation lever and redeeming a link must not
        undo it); re-inviting an unclaimed account re-issues the link, which is
        the resend path; and re-inviting a *claimed* one is refused and points
        at the reset flow, because that is what somebody who has forgotten their
        password actually needs.

        Sent synchronously, unlike `POST /api/auth/reset`. That endpoint hides
        whether the address resolves to an account and so cannot report a
        delivery failure; here the caller is an authorised admin who is owed the
        truth about whether the message went.
        """
        try:
            address = users.normalise_email(payload.get("email"))
        except users.InvalidEmail as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        if address is None:
            raise HTTPException(status_code=422,
                                detail="an invite needs an email address")

        wanted = str(payload.get("username") or "").strip()
        make_admin = bool(payload.get("is_admin"))
        sender = _sender()

        with db.connection() as con:
            existing = users.get_by_email(con, address)
            if existing is not None:
                if users.has_password(con, existing.id):
                    raise HTTPException(
                        status_code=409,
                        detail=f"{existing.username} has already claimed that "
                               "address -- send them a reset link instead")
                user = existing
            else:
                try:
                    name = (users.normalise_username(wanted) if wanted
                            else users.normalise_username(
                                address.partition("@")[0]))
                except users.InvalidUsername as exc:
                    raise HTTPException(
                        status_code=422,
                        detail=f"{exc}; choose a username") from exc
                try:
                    user = users.create(con, name, email=address,
                                        is_admin=make_admin)
                except users.UserExists as exc:
                    raise HTTPException(status_code=409,
                                        detail=str(exc)) from exc
            try:
                invites.send_invite(con, user, sender=sender)
            except (mail.EmailNotSent, OSError) as exc:
                # The account exists and its link did not go out. Say so
                # plainly: the fix is to press invite again once mail works,
                # which this endpoint supports, rather than to wonder whether
                # half of something happened.
                raise HTTPException(
                    status_code=502,
                    detail=f"the account exists but the invite could not be "
                           f"sent: {exc}") from exc
            body = _account(con, user)

        _LOG.info("%r invited %r", caller.username, user.username)
        return body

    @app.patch("/api/admin/users/{username}")
    def update_account(username: str, payload: dict[str, Any],
                       caller: Admin) -> dict[str, Any]:
        """Grant or revoke admin, disable or re-enable, set a model tier.

        Every refusal comes from `auth/users.py` rather than from here, which
        is ADR 17's point: the rule that an instance may not be left without an
        admin belongs in the core, so the CLI and this route cannot disagree
        about it. A `LastAdmin` is a 409 — the request is well-formed and
        understood, and it conflicts with the state of the world. An
        `UnknownTier` is a 422: that one is a malformed request, not a
        conflicting one, and it comes from the same place for the same reason.
        """
        del caller
        with db.connection() as con:
            user = _find(con, username)
            changed: list[str] = []
            try:
                if "is_admin" in payload:
                    wanted = bool(payload["is_admin"])
                    if wanted != user.is_admin:
                        users.set_admin(con, user.id, wanted)
                        changed.append("admin" if wanted else "not admin")
                if "disabled" in payload:
                    wanted = bool(payload["disabled"])
                    if wanted != user.disabled:
                        users.set_disabled(con, user.id, wanted)
                        changed.append("disabled" if wanted else "enabled")
                # Compared through `tiers.get` on both sides so that an account
                # holding a stale key and one holding NULL — which resolve to
                # the same tier and are answered by the same model — do not
                # read as a change worth logging or a 422 worth raising.
                if "model_tier" in payload:
                    asked = payload["model_tier"]
                    # Its own name rather than reusing `wanted` above: that one
                    # is a bool and this is a tier key, and mypy is right that
                    # a variable holding both is a variable holding neither.
                    tier = None if asked is None else str(asked)
                    if tiers.get(tier).key != tiers.get(user.model_tier).key:
                        users.set_model_tier(con, user.id, tier)
                        changed.append(f"answered by {tiers.get(tier).label}")
            except users.LastAdmin as exc:
                raise HTTPException(status_code=409, detail=str(exc)) from exc
            except users.UnknownTier as exc:
                raise HTTPException(
                    status_code=422,
                    detail=f"no such tier: {exc}") from exc
            if not changed:
                raise HTTPException(
                    status_code=422,
                    detail="nothing to change -- send is_admin, disabled "
                           "or model_tier")
            fetched = _find(con, user.username)
            body = _account(con, fetched)

        _LOG.warning("%r is now %s", user.username, " and ".join(changed))
        return body

    @app.post("/api/admin/users/{username}/reset", status_code=202)
    def send_reset(username: str, caller: Admin) -> dict[str, Any]:
        """Mail this account a link to choose a new password.

        The admin's answer to "they are locked out" — and the whole of it. ADR
        16 forbids setting the password for them, so what an admin can do is
        cause the link to be sent, which is exactly what the account holder
        could have done from the sign-in page.

        202 and not 200 for the reason `POST /api/auth/reset` uses it: the
        message has been handed to a provider, and delivery is not something
        this response knows about.
        """
        del caller
        sender = _sender()
        with db.connection() as con:
            user = _find(con, username)
            if user.email is None:
                raise HTTPException(
                    status_code=422,
                    detail=f"{user.username} has no email address -- set a "
                           "password with `mtglab users passwd` instead")
            if user.disabled:
                # `invites.send_reset` would silently decline this one, which is
                # the correct behaviour for the public endpoint and the wrong
                # answer for an admin who pressed a button. Same rule, said out
                # loud: a lever the disabled party can undo from their own inbox
                # is not a lever.
                raise HTTPException(
                    status_code=409,
                    detail=f"{user.username} is disabled -- enable the account "
                           "first if they should be able to sign in")
            try:
                invites.send_reset(con, user.email, sender=sender)
            except (mail.EmailNotSent, OSError) as exc:
                raise HTTPException(status_code=502, detail=str(exc)) from exc

        _LOG.info("a reset link was sent to %r", user.username)
        return {"detail": f"a reset link is on its way to {user.username}"}

    @app.delete("/api/admin/users/{username}")
    def delete_account(username: str, payload: dict[str, Any],
                       caller: Admin) -> dict[str, Any]:
        """Delete an account, its sessions, its tokens and its jobs.

        The one irreversible thing an admin can do from a browser, and it
        carries the two guards that earns.

        **The caller types the username back.** `confirm` must equal the account
        being deleted, which is `decks delete`'s rule moved to the client: only
        somebody looking at the right account can produce it, and a `fetch`
        aimed at the wrong URL cannot. 422, not 400 -- the body is the thing
        that is wrong.

        **An admin may not delete themselves.** `LastAdmin` already prevents the
        lockout, but it permits the confusing case it does not cover: an admin
        with a colleague, deleting the account their own session belongs to and
        being signed out by their own request. 409, and `mtglab users delete`
        from the machine is the way to remove that account if it is really
        meant.

        Jobs are dropped rather than left, and `jobs.forget_owner` says why:
        `users.id` is reissued by SQLite, so the next account created could
        otherwise be handed this one's results.
        """
        typed = str(payload.get("confirm") or "").strip()
        with db.connection() as con:
            user = _find(con, username)
            if typed.casefold() != user.username.casefold():
                raise HTTPException(
                    status_code=422,
                    detail=f"type {user.username!r} in confirm to delete it")
            if user.id == caller.user_id:
                raise HTTPException(
                    status_code=409,
                    detail="you cannot delete the account you are signed in "
                           "as -- use `mtglab users delete` on the machine")
            try:
                ended = users.delete(con, user.id)
            except users.LastAdmin as exc:
                raise HTTPException(status_code=409, detail=str(exc)) from exc

        forgotten = jobs.forget_owner(user.id)
        _LOG.warning("%r deleted the account %r (%d session(s), %d job(s))",
                     caller.username, user.username, ended, forgotten)
        return {"username": user.username, "revoked": ended,
                "jobs_dropped": forgotten}

    @app.delete("/api/admin/users/{username}/sessions")
    def revoke_sessions(username: str, caller: Admin) -> dict[str, Any]:
        """Sign an account out everywhere, without disabling it.

        The lighter of the two revocations, and the one wanted for a lost
        laptop: the account is still good, the cookies on that machine are not.
        Disabling is for revoking the *account*, and it does this as well.
        """
        del caller
        with db.connection() as con:
            user = _find(con, username)
            ended = sessions.delete_for_user(con, user.id)

        _LOG.warning("every session for %r was revoked (%d)",
                     user.username, ended)
        return {"username": user.username, "revoked": ended}
