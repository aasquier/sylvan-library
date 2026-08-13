"""Issue a link and deliver it. The one implementation both entry points call.

`mtglab users invite` and `POST /api/auth/reset` are the two doors ADR 16
describes, and this module is what is behind both of them, because the ADR is
explicit that a second path is how one of the two ends up weaker. What differs
between them is a `Purpose`, a subject line and a sentence; the token rules,
the link, and the fact that nothing is ever written into a `deck.yaml`-shaped
place are shared by construction.

**The token travels in the URL fragment**, not the query string:

    https://example.com/auth/claim#token=<43 characters>

A fragment is never sent to the server. The query-string spelling would put a
live credential into the access log of every hop that serves the page — the
platform's router, any proxy in front of it, and the `Referer` header of
anything the page later loads. The frontend reads `location.hash` and posts the
token to `/api/auth/claim`, which is the only request that carries it, and that
request is a POST with a JSON body rather than a URL. The login screen is a
separate build; this is the contract it has to meet.

`send_reset` takes an address rather than an account **and returns nothing
either way**. That is not indifference to the result — it is the shape that
makes ADR 16's "the reset endpoint answers identically whether or not the
address exists" hard to get wrong. A handler that cannot see whether the lookup
hit cannot branch on it, so the identical response is a property of this
signature rather than a rule somebody has to remember while editing the route.
"""

from __future__ import annotations

import sqlite3

from mtglab import config
from mtglab.auth import tokens, users
from mtglab.auth.mail import EmailSender, Message

CLAIM_PATH = "/auth/claim"

# In both messages, because the failure is in neither of them.
#
# **Some mail apps drop the `#...` when you click**, and nothing about that
# reaches the server: the fragment is client-side by design, so a stripped link
# looks exactly like no link at all, and re-sending produces one that fails
# identically. Seen on the deployed instance 2026-08-13, where the *visible* URL
# in a plain-text reset was whole and the click arrived with an empty hash.
#
# So the recovery has to travel with the message rather than live on the page
# somebody cannot reach. The claim screen takes a pasted address (`Claim.tsx`
# `tokenFromPaste`); this is the sentence that tells them to try it. Worth the
# four lines: without it the person is locked out and cannot say why.
_IF_THE_LINK_FAILS = (
    "If that opens a page asking for a link rather than a password box, copy\n"
    "the whole address above -- including the part after the # -- and paste it\n"
    "into your browser instead. Some mail apps cut the link short.\n"
)


def claim_link(token: str, base_url: str | None = None) -> str:
    """The URL that goes in the message. See the module docstring for the `#`."""
    base = (base_url if base_url is not None else config.base_url()).rstrip("/")
    return f"{base}{CLAIM_PATH}#token={token}"


def _invite_message(to: str, link: str) -> Message:
    return Message(
        to=to,
        subject="Your sylvan-library account",
        body=(
            "Somebody has set up an account for you on sylvan-library, a "
            "Commander deckbuilding and simulation tool.\n"
            "\n"
            "Choose a password to finish setting it up:\n"
            "\n"
            f"{link}\n"
            "\n"
            f"{_IF_THE_LINK_FAILS}"
            "\n"
            "The link works once and expires in a week. Nobody, including "
            "whoever invited you, ever sees the password you pick.\n"
            "\n"
            "If you were not expecting this, you can ignore it -- the account "
            "cannot be used until somebody follows that link.\n"
        ),
    )


def _reset_message(to: str, link: str) -> Message:
    return Message(
        to=to,
        subject="Reset your sylvan-library password",
        body=(
            "Somebody asked to reset the password on your sylvan-library "
            "account.\n"
            "\n"
            "Choose a new one here:\n"
            "\n"
            f"{link}\n"
            "\n"
            f"{_IF_THE_LINK_FAILS}"
            "\n"
            "The link works once and expires in an hour. Setting a new "
            "password signs you out everywhere.\n"
            "\n"
            "If this was not you, nothing has changed and you can ignore this "
            "message.\n"
        ),
    )


def send_invite(con: sqlite3.Connection, user: users.User, *,
                sender: EmailSender, base_url: str | None = None) -> None:
    """Issue an invite for an existing unclaimed account and mail the link.

    The account is created by the caller rather than here, because `users
    invite` has decisions to make about it — the username, the admin flag —
    that a reset does not.
    """
    if user.email is None:
        raise ValueError("an invite needs an address to send to")
    token = tokens.issue(con, user.id, tokens.Purpose.INVITE)
    sender.send(_invite_message(user.email, claim_link(token, base_url)))


def send_reset(con: sqlite3.Connection, email: str, *,
               sender: EmailSender, base_url: str | None = None) -> None:
    """Mail a reset link, if that address resolves to an account that can use one.

    Returns nothing in every case, including the three where no message goes
    out: no such address, a disabled account, and an address that is not
    shaped like one at all.

    **A disabled account gets no reset link.** Disabling is the maintainer's
    revocation lever, and one the disabled party can undo from their own inbox
    is not a lever. `tokens.redeem` refuses them too, so this is defence in
    depth rather than the only check.

    An *unclaimed* account does get one, and that is deliberate: somebody whose
    invite expired asking for a reset is the same request in different words,
    and refusing it would leave them with nothing to do but email the
    maintainer.
    """
    try:
        account = users.get_by_email(con, email)
    except users.InvalidEmail:
        # Not shaped like an address, so it resolves to nobody -- which is the
        # same outcome as an address that simply has no account, and gets the
        # same silence.
        return
    if account is None or account.disabled or account.email is None:
        return
    token = tokens.issue(con, account.id, tokens.Purpose.RESET)
    sender.send(_reset_message(account.email, claim_link(token, base_url)))
