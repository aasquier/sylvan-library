"""`app.db` — the transactional half of the two-database split (ADR 4).

DuckDB holds the corpus: public, regenerable, gitignored, rebuilt by one
command. SQLite holds everything a person typed that nothing else records —
accounts today, their decks at `docs/HOSTING.md` §6 step 6. Different
lifecycles and different backup rules, which is why §2 says not to merge them
for tidiness.

Three pragmas, each load-bearing:

- **WAL**, so a reader never blocks the writer. The API serves sync endpoints
  from a threadpool, so concurrent reads during a login write are the normal
  case rather than the exotic one.
- **`foreign_keys=ON`**, because SQLite defaults it *off* per connection and a
  `REFERENCES` clause with it off is a comment. Sessions reference users with
  `ON DELETE CASCADE`; without this pragma deleting a user would leave live
  sessions pointing at nothing.
- **`busy_timeout`**, so two writers collide as a short wait rather than an
  immediate `database is locked`.

Connections are opened per operation and closed, never held. That is the same
constraint `decks/source.py` documents for a `DeckSource` and for the same
reason: the thing that outlives a request must be a locator, not a handle.
"""

from __future__ import annotations

import sqlite3
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path

from mtglab import config

# Bumped when `_MIGRATIONS` grows. Stored in SQLite's own `user_version`, which
# costs no table and cannot be forgotten in a schema dump.
SCHEMA_VERSION = 2

# One entry per version, applied in order to whatever the file is at. A fresh
# database runs all of them; an existing one runs the tail. The invite and
# reset tokens ADR 16 describes arrived as version 2 rather than as an edit to
# version 1 -- a migration you can no longer see is a migration nobody can
# reason about, and by the time this was written there was already an `app.db`
# on a laptop with an account in it.
_MIGRATIONS: tuple[str, ...] = (
    # -- 1 ------------------------------------------------------------------
    """
    CREATE TABLE users (
        id            INTEGER PRIMARY KEY,
        username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
        -- Nullable, and that is the shape ADR 16 needs rather than an
        -- oversight: an invited account exists before its holder has chosen a
        -- password, and "cannot log in yet" is a state the login path must
        -- handle either way. `users.authenticate` treats NULL as a refusal
        -- that still costs a hash, so it is not distinguishable by timing.
        password_hash TEXT,
        -- Also nullable. The maintainer's own bootstrap account does not need
        -- one; an invited account cannot be created without one. UNIQUE in
        -- SQLite permits many NULLs, which is exactly the wanted behaviour.
        email         TEXT UNIQUE COLLATE NOCASE,
        is_admin      INTEGER NOT NULL DEFAULT 0,
        disabled_at   TEXT,
        created_at    TEXT NOT NULL
    );

    CREATE TABLE sessions (
        -- The hash of the token, never the token. Reading this file must not
        -- hand over a live session (ADR 5).
        token_hash   TEXT PRIMARY KEY,
        user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        created_at   TEXT NOT NULL,
        expires_at   TEXT NOT NULL,
        last_seen_at TEXT
    );

    CREATE INDEX sessions_by_user ON sessions(user_id);

    -- Fixed-window login throttling. Keyed by account and, separately, by
    -- client address; see `ratelimit.py` for why both.
    CREATE TABLE login_attempts (
        key          TEXT PRIMARY KEY,
        window_start TEXT NOT NULL,
        failures     INTEGER NOT NULL
    );
    """,
    # -- 2 ------------------------------------------------------------------
    # ADR 16's token machinery. One table for both entry points, because "a
    # bespoke second path is how one of them ends up weaker" is the reason the
    # ADR gives for not writing two.
    """
    CREATE TABLE auth_tokens (
        -- The hash of the token, for the same reason `sessions` stores one:
        -- reading this file must not hand over a live credential, and an
        -- unused invite row is exactly that until it expires.
        token_hash TEXT PRIMARY KEY,
        user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        -- 'invite' or 'reset'. Stored rather than inferred, because the two
        -- have different lifetimes and a token issued for one must not be
        -- redeemable as the other.
        purpose    TEXT NOT NULL,
        created_at TEXT NOT NULL,
        expires_at TEXT NOT NULL,
        -- Set on redemption rather than deleting the row. Single-use either
        -- way; what the row buys is that a second click on the same link can
        -- be told it has already been used instead of that it never existed.
        -- That distinction is not a leak: whoever is holding the token already
        -- knows it was real.
        used_at    TEXT
    );

    CREATE INDEX auth_tokens_by_user ON auth_tokens(user_id, purpose);
    """,
)


def _apply_migrations(con: sqlite3.Connection) -> None:
    version: int = con.execute("PRAGMA user_version").fetchone()[0]
    if version >= SCHEMA_VERSION:
        return
    for statement in _MIGRATIONS[version:]:
        con.executescript(statement)
    # `PRAGMA user_version = ?` is not parameterisable -- pragmas are parsed,
    # not bound. The value is an int constant in this module, never input.
    con.execute(f"PRAGMA user_version = {SCHEMA_VERSION}")
    con.commit()


def connect(path: Path | str | None = None) -> sqlite3.Connection:
    """Open `app.db`, creating the file and the schema if they are absent.

    `None` means "ask `config` now", so `config.use_paths()` in a test actually
    takes effect -- the same rule every other path in this project follows.
    """
    target = Path(path) if path is not None else config.APP_DB_PATH
    target.parent.mkdir(parents=True, exist_ok=True)
    # The default (deferred) isolation level, deliberately, rather than
    # autocommit: `with con:` is then a real transaction, and the operations
    # here that must be all-or-nothing -- setting a password *and* revoking
    # that user's sessions -- can say so.
    con = sqlite3.connect(target)
    con.row_factory = sqlite3.Row
    con.execute("PRAGMA journal_mode=WAL")
    con.execute("PRAGMA foreign_keys=ON")
    con.execute("PRAGMA busy_timeout=5000")
    _apply_migrations(con)
    return con


@contextmanager
def connection(path: Path | str | None = None) -> Iterator[sqlite3.Connection]:
    """`connect()` that closes afterwards. The form a request should use."""
    con = connect(path)
    try:
        yield con
    finally:
        con.close()
