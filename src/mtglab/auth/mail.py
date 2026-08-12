"""The `EmailSender` seam, and the two implementations behind it.

ADR 16 puts a transactional email provider behind a protocol for one stated
reason: **"the seam is what keeps that dependency out of the tests."** No test
in this suite sends mail, the same rule that keeps the Claude tests off the
network, and the way that rule is kept is that nothing above this module knows
which implementation it is holding.

Two of them:

- `ConsoleSender` — writes the message to a stream. The development default,
  and the reason `mtglab users invite` is usable on a laptop with no account
  anywhere: the link you need is the one it prints.
- `ResendSender` — one HTTPS POST to `api.resend.com`, over `urllib` rather
  than a new dependency. It is a single request with a JSON body; an HTTP
  client library would be four hundred kilobytes to save six lines.

**Where the address is allowed to appear.** ADR 16 is unconditional that an
email address must never reach a log line, an artifact or a Claude tool result,
and this module is the one place that handles addresses in bulk, so the line
has to be drawn here rather than assumed:

- `ConsoleSender` prints it. That is what makes it useful in development — you
  are standing at the terminal, you need the link — and exactly what makes it
  wrong in production, where "the console" is the platform's log aggregator.
  So `sender_from_env()` **refuses to fall back to it when auth is on**, which
  is the flag that means deployed. A deployment missing its API key gets a loud
  configuration error instead of a stream of addresses in the log.
- `ResendSender` never puts one in an exception. The provider's own error
  bodies quote the recipient back at you, which is why the failure below
  carries the status and the error *name* and drops the body.

Nothing here imports FastAPI, and nothing here knows what a session is. The
sender is passed in from the edge — `create_app(email_sender=...)` and the
`users invite` command — so both entry points can be handed a recorder.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from collections.abc import Callable
from dataclasses import dataclass
from typing import IO, Protocol, runtime_checkable

from mtglab import config

RESEND_ENDPOINT = "https://api.resend.com/emails"

# Long enough that a slow provider is not mistaken for a hung one, short enough
# that a hung one does not hold a worker for the length of a coffee break. The
# reset endpoint sends in a background task, so this is never on the path of a
# response a person is waiting for.
TIMEOUT_SECONDS = 10


class EmailNotConfigured(RuntimeError):
    """Raised when mail is required and nothing is set up to send it."""


class EmailNotSent(RuntimeError):
    """Raised when the provider refused or could not be reached.

    Deliberately carries no recipient and no response body -- see the module
    docstring for why. What it does carry is enough to search the provider's
    dashboard with: a status code and their own name for the error.
    """


@dataclass(frozen=True)
class Message:
    """One plain-text message.

    Plain text and no HTML alternative, on purpose. The entire content of both
    messages this project sends is a sentence and a link; an HTML template
    would be a second thing to keep in step, a rendering surface to test, and a
    reason for a mail client to hide the URL behind a button somebody cannot
    read before clicking.
    """

    to: str
    subject: str
    body: str


@runtime_checkable
class EmailSender(Protocol):
    """Anything that can send a `Message`. The seam ADR 16 asks for."""

    def send(self, message: Message) -> None:
        ...


class ConsoleSender:
    """Print the message instead of sending it. Development and tests.

    Writes to stderr by default rather than stdout, so a CLI command's own
    output stays parseable when a message goes out in the middle of it.
    """

    def __init__(self, stream: IO[str] | None = None) -> None:
        self._stream = stream

    @property
    def stream(self) -> IO[str]:
        # Resolved per call rather than bound in `__init__`: pytest's capture
        # replaces `sys.stderr` after the object would have been built.
        return self._stream if self._stream is not None else sys.stderr

    def send(self, message: Message) -> None:
        out = self.stream
        out.write(
            "\n"
            "  -- no RESEND_API_KEY set, so this message was NOT sent ------\n"
            f"  to:      {message.to}\n"
            f"  subject: {message.subject}\n\n"
            + "\n".join(f"  {line}" for line in message.body.splitlines())
            + "\n  -------------------------------------------------------------\n"
        )
        out.flush()


# `(url, headers, body) -> (status, response_body)`. A seam inside the seam,
# and the reason `ResendSender` itself is covered by tests: the protocol above
# lets a test swap the whole sender, which proves everything except that the
# request this one builds is the request Resend documents.
Transport = Callable[[str, dict[str, str], bytes], tuple[int, bytes]]


def _urllib_post(url: str, headers: dict[str, str],
                 body: bytes) -> tuple[int, bytes]:
    request = urllib.request.Request(url, data=body, headers=headers,
                                     method="POST")
    with urllib.request.urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        return int(response.status), response.read()


class ResendSender:
    """The deployed implementation. One POST, no SDK.

    The key is held on the instance rather than read from the environment on
    each send, which is the opposite of what `config.py` does with
    `ANTHROPIC_API_KEY` and worth saying why: that key is never bound because
    the Anthropic SDK reads it itself, so holding one would be a copy nobody
    needs. This one has to be put in a header by this code. It is built once,
    at the edge, and never logged or serialised.
    """

    def __init__(self, api_key: str, sender: str, *,
                 transport: Transport | None = None) -> None:
        if not api_key.strip():
            raise EmailNotConfigured("RESEND_API_KEY is empty")
        self._api_key = api_key.strip()
        self._sender = sender
        self._post: Transport = transport if transport is not None \
            else _urllib_post

    def send(self, message: Message) -> None:
        body = json.dumps({
            "from": self._sender,
            "to": [message.to],
            "subject": message.subject,
            "text": message.body,
        }).encode("utf-8")
        headers = {
            "Authorization": f"Bearer {self._api_key}",
            "Content-Type": "application/json",
        }
        try:
            status, response = self._post(RESEND_ENDPOINT, headers, body)
        except urllib.error.HTTPError as exc:
            raise EmailNotSent(_describe(exc.code, exc.read())) from exc
        except OSError as exc:
            # Timeouts, DNS, TLS. `exc` here is about the network, not about
            # the recipient, so it is safe to quote.
            raise EmailNotSent(f"could not reach the mail provider: {exc}") \
                from exc
        if not 200 <= status < 300:
            raise EmailNotSent(_describe(status, response))


def _describe(status: int, body: bytes) -> str:
    """A failure worth logging, with the recipient stripped out.

    Resend's error bodies look like `{"statusCode": 422, "message": "...",
    "name": "validation_error"}` and the message quotes the address back. The
    name is the part that says what went wrong; the message is the part that
    would put somebody's address in the application log.
    """
    name = ""
    try:
        parsed = json.loads(body.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        parsed = {}
    if isinstance(parsed, dict):
        name = str(parsed.get("name", "") or "")
    detail = f" ({name})" if name else ""
    return f"the mail provider refused the message: HTTP {status}{detail}"


def sender_from_env(*, transport: Transport | None = None) -> EmailSender:
    """The sender this process should use, decided when it is asked for.

    `RESEND_API_KEY` picks the real one. Without it:

    - **auth off** — a laptop, one person, `mtglab ui`. The console sender is
      the right answer and the printed link is the feature.
    - **auth on** — a deployment. Refused, loudly, rather than writing
      addresses into whatever collects stdout there. `docs/HOSTING.md` §7 lists
      the key as a deploy-day requirement and this is that list enforced.

    Read at call time and not at import, for the reason every other lookup in
    this project is: a value captured at import is a value a test cannot change
    and a container cannot set late.
    """
    key = os.environ.get("RESEND_API_KEY", "").strip()
    if key:
        return ResendSender(key, config.email_from(), transport=transport)
    if config.require_auth():
        raise EmailNotConfigured(
            "this instance requires authentication but has no RESEND_API_KEY, "
            "so invites and password resets cannot be delivered -- set one "
            "(`fly secrets set RESEND_API_KEY=...`) or the console fallback "
            "would print email addresses into the application log")
    return ConsoleSender()
