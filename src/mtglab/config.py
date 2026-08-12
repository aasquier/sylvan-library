"""Where things live on disk.

These were module-level constants in `cli.py`, which caused two problems.

The web API had to reach into the CLI to find them (`from mtglab.cli import
DB_PATH, DECKS_DIR`), so the HTTP layer depended on the command-line layer --
backwards, and the reason `cli.py` sat at 27% test coverage: there was no way
to point a test at a temporary deck directory.

And they were *relative* paths resolved against the process working directory,
so a container could not put the corpus on a mounted volume. `docs/HOSTING.md`
lists this as the first prerequisite for deploying anything.

Both are read from the environment now, defaulting to exactly what they were,
so local use is unchanged:

    MTGLAB_DATA_DIR    default "data"    corpus, price history, app.db
    MTGLAB_DECKS_DIR   default "decks"   deck.yaml source of truth

Resolved once at import. Tests that need a different location should use
`use_paths()` rather than reassigning the module globals, so the derived
`DB_PATH` cannot drift out of step with `DATA_DIR`.

A `.env` file is loaded first, if one is present and python-dotenv is
installed. That is here rather than in an entry point because the app is
started several ways -- `mtglab ui`, uvicorn, the launch configuration in
`.claude/` -- and only some of them inherit a login shell's environment. A file
the process reads itself works regardless of how it was started.

**Secrets are not resolved here.** `.env` is loaded into `os.environ` and that
is the end of this module's involvement: `ANTHROPIC_API_KEY` is read by the
Anthropic SDK directly, never bound to a name in this project. The reason is
the one this module's own history teaches -- a value captured at import is a
value that cannot be changed later -- plus a value we never hold is a value we
cannot log, serialise into an error, or leak into a prompt.

The one exception is a blank credential, which is deleted rather than passed
along; see `_drop_blank_credentials` for why empty is worse than absent.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path


def _load_dotenv() -> None:
    """Populate `os.environ` from a `.env`, without overriding what is set.

    Optional: python-dotenv rides with the `api` extra, and a base install has
    no `.env` to read. The real environment wins over the file, so an exported
    variable is never silently shadowed by a stale one on disk -- which matters
    most for the API key, where using the wrong one is confusing rather than
    loud.
    """
    try:
        from dotenv import find_dotenv, load_dotenv
    except ImportError:
        return
    # Search upward from the working directory, not from this file: installed
    # in site-packages, the package's own parents are the wrong tree.
    found = find_dotenv(usecwd=True)
    if found:
        load_dotenv(found, override=False)


# Anthropic's credential precedence treats an empty string as a credential that
# is *present*: with `ANTHROPIC_API_KEY=""` the SDK selects the API-key path
# with an empty key and fails as a 401, instead of falling through or reporting
# that nothing is configured. Their docs say to unset rather than blank these.
# A half-filled `.env` is the obvious way to produce one by accident, so drop
# them here and turn a confusing 401 into an honest "no credentials".
_BLANK_IS_WORSE_THAN_ABSENT = ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")


def _drop_blank_credentials() -> None:
    for name in _BLANK_IS_WORSE_THAN_ABSENT:
        if name in os.environ and not os.environ[name].strip():
            del os.environ[name]


_load_dotenv()
_drop_blank_credentials()

DATA_DIR: Path
DECKS_DIR: Path
DB_PATH: Path
APP_DB_PATH: Path


def _read_env() -> tuple[Path, Path]:
    return (Path(os.environ.get("MTGLAB_DATA_DIR", "data")),
            Path(os.environ.get("MTGLAB_DECKS_DIR", "decks")))


def _apply(data_dir: Path, decks_dir: Path) -> None:
    global DATA_DIR, DECKS_DIR, DB_PATH, APP_DB_PATH
    DATA_DIR = data_dir
    DECKS_DIR = decks_dir
    # Derived, never set independently -- that is the whole reason this lives
    # behind a function instead of three assignable globals.
    DB_PATH = data_dir / "mtg.duckdb"
    # The second embedded database (ADR 4): SQLite, transactional, holding
    # users and sessions. Beside the corpus on the same volume, but a different
    # lifecycle -- the corpus is regenerable and gitignored, `app.db` is the
    # irreplaceable half and the thing that gets backed up.
    APP_DB_PATH = data_dir / "app.db"


_apply(*_read_env())


def reload_from_env() -> None:
    """Re-read the environment. For tests that set the variables directly."""
    _apply(*_read_env())


# Forge's installed distribution: the desktop jar plus `res/`, several hundred
# megabytes of it. Deliberately outside the repository -- CLAUDE.md rule 5
# forbids committing card corpus data, and Forge's `res/cardsfolder` is exactly
# that for a second engine.
FORGE_HOME_DEFAULT = Path.home() / ".local" / "share" / "mtglab" / "forge"


def forge_home() -> Path:
    """Where the Forge distribution is unpacked. `MTGLAB_FORGE_HOME` overrides.

    A function rather than a module constant, unlike everything above it. Forge
    is optional -- a base install has no JVM and never wants one -- so the path
    is resolved when something actually reaches for it, and a machine that
    installs Forge mid-session does not need the process restarted. It is also
    the only path here that points outside both the project and its data
    directory, which is the point: nothing under it may ever be tracked.
    """
    return Path(os.environ.get("MTGLAB_FORGE_HOME") or FORGE_HOME_DEFAULT)


# --------------------------------------------------------------------- auth
#
# Functions rather than constants, for the reason `forge_home` is one: these
# are read when something reaches for them, so a test or a container can change
# the answer without reimporting the module.
#
# The default is **off**, and that is not laziness. `mtglab ui` is a local
# single-user tool that a friend is meant to be able to clone and run; putting a
# login in front of it would be a regression for the only way the app is used
# today. The deployment turns it on (`MTGLAB_REQUIRE_AUTH=1` in `fly.toml`), and
# `docs/HOSTING.md` §6 step 5 is explicit that the whole of the auth core is
# testable locally with no deployment -- which it is, by flipping this.

_TRUE = frozenset({"1", "true", "yes", "on"})


def _flag(name: str, *, default: bool = False) -> bool:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    return raw.strip().lower() in _TRUE


def require_auth() -> bool:
    """Whether every non-public route needs a session. `MTGLAB_REQUIRE_AUTH`."""
    return _flag("MTGLAB_REQUIRE_AUTH")


def secure_cookies() -> bool:
    """Whether the session cookie carries `Secure`. `MTGLAB_SECURE_COOKIES`.

    Defaults to whatever `require_auth()` says, because the two travel
    together: auth on means deployed means TLS, auth off means
    `http://127.0.0.1` where a `Secure` cookie is simply never sent back and
    login appears to succeed and then not work. Overridable for the case that
    breaks the correlation -- a local instance behind a real certificate.
    """
    return _flag("MTGLAB_SECURE_COOKIES", default=require_auth())


def base_url() -> str:
    """Where this instance answers, for links that have to work in an inbox.

    An invite or a reset is a URL somebody clicks days later in another
    application, so it cannot be relative and cannot be guessed from the
    request -- a `Host` header is client-supplied, and building a password-reset
    link out of one is how a reset lands on an attacker's domain. It is
    configuration, and it is wrong-by-default rather than absent: the local
    port is what `mtglab ui` serves on, so a laptop needs no setting and a
    deployment that forgets one sends links to localhost, which is visibly
    broken rather than quietly hijackable.

    `MTGLAB_BASE_URL`, no trailing slash.
    """
    return os.environ.get("MTGLAB_BASE_URL", "").strip().rstrip("/") \
        or "http://127.0.0.1:8765"


def email_from() -> str:
    """The From address on invites and resets. `MTGLAB_EMAIL_FROM`.

    Resend refuses anything outside a verified sending domain, so this is a
    deploy-day setting rather than a preference (`docs/HOSTING.md` §7). The
    default is only ever seen by the console sender.
    """
    return os.environ.get("MTGLAB_EMAIL_FROM", "").strip() \
        or "mtglab <no-reply@localhost>"


def client_ip_header() -> str | None:
    """Header naming the real client IP, if a trusted proxy sets one.

    Unset by default, and deliberately so: rate limiting keyed on a header any
    client can send is rate limiting an attacker opts out of by typing a
    different number. Set this (`Fly-Client-IP`, `X-Forwarded-For`) only when a
    proxy you control is guaranteed to overwrite it on the way in.
    """
    name = os.environ.get("MTGLAB_CLIENT_IP_HEADER", "").strip()
    return name or None


@contextmanager
def use_paths(*, data_dir: Path | str | None = None,
              decks_dir: Path | str | None = None) -> Iterator[None]:
    """Temporarily point mtglab at other directories.

    Used by the CLI tests to run real commands against a scratch deck
    directory without touching the repository's own decks.
    """
    previous = (DATA_DIR, DECKS_DIR)
    _apply(Path(data_dir) if data_dir is not None else DATA_DIR,
           Path(decks_dir) if decks_dir is not None else DECKS_DIR)
    try:
        yield
    finally:
        _apply(*previous)


def deck_paths(decks_dir: Path | None = None) -> list[Path]:
    """Every real deck file. `_template` and any other underscore-prefixed
    directory is scaffolding, not a deck.

    Reads `DECKS_DIR` at call time rather than binding it as a default
    argument, so `use_paths()` actually takes effect.
    """
    root = Path(decks_dir) if decks_dir is not None else DECKS_DIR
    if not root.exists():
        return []
    return [p for p in sorted(root.glob("*/deck.yaml"))
            if not p.parent.name.startswith("_")]
