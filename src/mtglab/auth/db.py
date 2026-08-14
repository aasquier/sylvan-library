"""`app.db` — the transactional half of the two-database split (ADR 4).

DuckDB holds the corpus: public, regenerable, gitignored, rebuilt by one
command. SQLite holds everything a person typed that nothing else records —
accounts today, their decks at `docs/HOSTING.md` §6 step 6. Different
lifecycles and different backup rules, which is why §2 says not to merge them
for tidiness.

One table is the exception and says so in its own migration: `sim_cache` is
derived, and dropping it costs CPU rather than data. It is here because the
results are keyed to decks on the same volume and must survive a deploy the
same way they do.

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
SCHEMA_VERSION = 6

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
    # -- 3 ------------------------------------------------------------------
    # Memoised Tier 1 results (`sim/cache.py`), the table `docs/HOSTING.md` §2
    # sketched and ADR 4 listed in this file's contents. It is the one thing in
    # here that is *derived*: every row can be recomputed from the deck and the
    # corpus, and dropping the table loses nothing but CPU time. That is why
    # `mtglab sim cache --clear` is a supported operation and why nothing else
    # in `app.db` has one.
    #
    # It lives here anyway rather than in a third store, because the rows are
    # keyed to decks that live on the same volume and have to survive a deploy
    # the same way -- and because a second SQLite file would be a second copy
    # of the pragmas, the migration ladder and the backup question, for data
    # measured in kilobytes.
    """
    CREATE TABLE sim_cache (
        -- sha256 over the *compiled* deck, the run parameters, the seed and a
        -- fingerprint of the engine's own source. Not a deck slug and not a
        -- hash of deck.yaml: card facts come from the corpus, so a refresh can
        -- change a simulation while the deck file does not move. See
        -- `sim/cache.py` for the full argument.
        key          TEXT PRIMARY KEY,
        -- 'sim.mana' or 'sim.lands.count'. Stored for `mtglab sim cache` and
        -- so a future eviction policy can prefer one kind over another; the
        -- key alone would make both opaque.
        kind         TEXT NOT NULL,
        result_json  TEXT NOT NULL,
        created_at   TEXT NOT NULL,
        -- Touched on every hit, so eviction is least-recently-used. The
        -- numbers everyone opens are the ones worth keeping.
        last_used_at TEXT NOT NULL
    );

    CREATE INDEX sim_cache_by_use ON sim_cache(last_used_at);
    """,
    # -- 4 ------------------------------------------------------------------
    # Commander dossiers (ADR 19). Derived like `sim_cache` above and droppable
    # for the same reason, but the contract is deliberately weaker and the
    # column list is where that shows.
    #
    # A cached simulation must never be stale, because it is reproducible: the
    # key is the whole input, so a changed input is a miss. A dossier is an
    # opinion assembled over web writing that moves on its own, and no hash of
    # anything here could tell you it has gone out of date. So the row carries
    # `created_at` and the app *shows* it, which is the honest version of the
    # same promise -- generated once, dated, regenerable on demand.
    """
    CREATE TABLE dossier_cache (
        -- The commander's Scryfall `oracle_id` plus a fingerprint of the mode
        -- (its prompt, its schema, the model id). Not the deck slug: a dossier
        -- is about a character, so every deck that commander leads shares one,
        -- including across users on a hosted instance.
        key         TEXT PRIMARY KEY,
        oracle_id   TEXT NOT NULL,
        -- Carried for `mtglab claude dossier --list`, which otherwise could
        -- only show hashes. The name is on the card, not personal data.
        commander   TEXT NOT NULL,
        result_json TEXT NOT NULL,
        created_at  TEXT NOT NULL
    );

    CREATE INDEX dossier_cache_by_oracle ON dossier_cache(oracle_id);
    """,
    # -- 5 ------------------------------------------------------------------
    # `users.id` gains AUTOINCREMENT. Version 1 wrote `INTEGER PRIMARY KEY`,
    # which makes the column an alias for the rowid -- and a plain rowid is
    # *reused*: SQLite picks max(rowid)+1, so deleting the newest account hands
    # its integer to the next one created. Jobs are held in memory keyed on
    # exactly that integer, so the new holder of an id inherited the dead
    # account's results and the deck names in their labels. #68 fixed the
    # instance by dropping a deleted owner's jobs; this fixes the class, and
    # the class is the point -- `jobs` is merely the first thing to key on a
    # user id, and an isolation filter that is written correctly can still be
    # defeated by arithmetic underneath it.
    #
    # AUTOINCREMENT cannot be added by ALTER TABLE, so this is SQLite's
    # documented table rebuild. Two things about it are load-bearing:
    #
    # - **`DROP TABLE users` would cascade.** `sessions` and `auth_tokens`
    #   reference it `ON DELETE CASCADE`, and `connect()` turns `foreign_keys`
    #   on *before* migrations run, so with the pragma left alone this
    #   migration would silently sign out every account and void every
    #   outstanding invite. `_apply_migrations` turns it off around the ladder
    #   and runs `foreign_key_check` before giving it back.
    # - **The transaction is written here rather than left to Python.**
    #   `executescript` commits whatever is pending and then performs no
    #   transaction control of its own, so without an explicit BEGIN a failure
    #   between the DROP and the RENAME would leave `users_rebuilt` behind with
    #   `user_version` still at 4 -- and the next start would re-run this and
    #   fail on a table that already exists, which is an app that never boots.
    #
    # What it cannot do is repair ids already issued: the high-water mark it
    # sets is max(id) over the rows that exist, and an id handed out and then
    # deleted is not in them. It stops the *next* one from being recycled.
    """
    BEGIN;

    CREATE TABLE users_rebuilt (
        id            INTEGER PRIMARY KEY AUTOINCREMENT,
        username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
        password_hash TEXT,
        email         TEXT UNIQUE COLLATE NOCASE,
        is_admin      INTEGER NOT NULL DEFAULT 0,
        disabled_at   TEXT,
        created_at    TEXT NOT NULL
    );

    -- Ids are carried across unchanged. Sessions and tokens point at them and
    -- are not being rewritten; renumbering here would sign everyone out to no
    -- purpose.
    INSERT INTO users_rebuilt
        (id, username, password_hash, email, is_admin, disabled_at, created_at)
    SELECT id, username, password_hash, email, is_admin, disabled_at, created_at
    FROM users;

    DROP TABLE users;

    ALTER TABLE users_rebuilt RENAME TO users;

    COMMIT;
    """,
    # -- 6 ------------------------------------------------------------------
    # ADR 4's second deck tier, arriving as ADR 22. The curated six stay
    # file-backed in git permanently and are *not* in here; this is where
    # everybody else's decks live, one row each.
    #
    # `yaml` holds the same text `deck.yaml` holds, which is the property ADR 4
    # bought and this table spends: `Deck.from_text` parses both, so the gate,
    # the compiler and the artifact generator never learn there are two tiers.
    #
    # Three columns are denormalised copies of what the YAML already says --
    # `slug`, `name` and `shared`. The YAML is the truth and they are the
    # index: "list the decks I may see" and "is this slug free" must not parse
    # every row to answer, and the browse tab groups by owner across all of
    # them. `_write` sets them from the parsed deck on every write, so they
    # cannot drift from the text they summarise.
    """
    CREATE TABLE user_decks (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        -- CASCADE matches `sessions` and `auth_tokens`: deleting an account
        -- takes its decks. That is a real consequence rather than a default
        -- copied across, and ADR 22 records it as one -- before this table
        -- there were no decks to orphan.
        owner_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        slug       TEXT NOT NULL,
        name       TEXT NOT NULL,
        yaml       TEXT NOT NULL,
        -- 0 or 1. Governs reading only; writing is the owner's either way.
        shared     INTEGER NOT NULL DEFAULT 0,
        -- A delete marks the row rather than removing it, which is the promise
        -- `DeckSource.delete` makes on the protocol: an implementation that
        -- cannot say where the deck went has destroyed it rather than removed
        -- it. The file source moves a directory into `.trash/`; this is the
        -- same guarantee for a row.
        deleted_at TEXT,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    );

    -- Unique per owner, never globally: ADR 22's whole namespace argument is
    -- that "is this slug free" must be answerable without consulting anybody
    -- else's decks. Partial, so deleting a deck frees its slug again -- a
    -- plain UNIQUE would make a trashed deck block the name forever.
    CREATE UNIQUE INDEX user_decks_slug
        ON user_decks(owner_id, slug) WHERE deleted_at IS NULL;

    CREATE INDEX user_decks_by_owner
        ON user_decks(owner_id) WHERE deleted_at IS NULL;

    -- The browse tab's query: every shared deck, everybody's, grouped by owner.
    CREATE INDEX user_decks_shared
        ON user_decks(shared) WHERE shared = 1 AND deleted_at IS NULL;
    """,
)


class MigrationFailed(RuntimeError):
    """A migration left the file in a state it must not be used in.

    Raised rather than returned, and raised *before* `connect()` hands the
    connection out: a schema change that has broken referential integrity is
    not something an API process should serve requests on top of.
    """


def _apply_migrations(con: sqlite3.Connection) -> None:
    version: int = con.execute("PRAGMA user_version").fetchone()[0]
    if version >= SCHEMA_VERSION:
        return
    # Foreign keys off for the duration, which is SQLite's own instruction for
    # any migration that rebuilds a table -- and migration 5 rebuilds `users`,
    # which `sessions` and `auth_tokens` reference `ON DELETE CASCADE`. With
    # the pragma left on as `connect()` sets it, the `DROP TABLE` inside that
    # rebuild would take every session and every unspent invite with it, and
    # would do it silently: a cascade is not an error.
    #
    # It is done here rather than inside the one migration that needs it for
    # two reasons. The pragma is a **no-op inside a transaction**, so a script
    # that opens one cannot set it; and the next rebuild -- there will be one,
    # this is the second schema in the file to want a column it cannot ALTER
    # into place -- inherits the safety instead of rediscovering the trap.
    con.execute("PRAGMA foreign_keys=OFF")
    try:
        for statement in _MIGRATIONS[version:]:
            con.executescript(statement)
        # `PRAGMA user_version = ?` is not parameterisable -- pragmas are
        # parsed, not bound. The value is an int constant in this module,
        # never input.
        con.execute(f"PRAGMA user_version = {SCHEMA_VERSION}")
        con.commit()
        # The other half of the bargain: having switched enforcement off, prove
        # nothing broke while it was off before anyone is served from this
        # file. `foreign_key_check` returns a row per violation and nothing at
        # all when the file is sound.
        violations = con.execute("PRAGMA foreign_key_check").fetchall()
        if violations:
            raise MigrationFailed(
                f"migrating app.db to version {SCHEMA_VERSION} left "
                f"{len(violations)} foreign key violation(s); the file has "
                f"not been signed off and must be restored from a backup "
                f"(docs/HOSTING.md §5)")
    finally:
        con.execute("PRAGMA foreign_keys=ON")


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
