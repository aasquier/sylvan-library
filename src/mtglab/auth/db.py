"""`app.db` — the transactional half of the two-database split (ADR 4).

DuckDB holds the pool: public, regenerable, gitignored, rebuilt by one
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
SCHEMA_VERSION = 11

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
    # pool, and dropping the table loses nothing but CPU time. That is why
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
        -- hash of deck.yaml: card facts come from the pool, so a refresh can
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
    # file-backed permanently -- on disk rather than in git, once ADR 30 said
    # so -- and are *not* in here; this is where everybody else's decks live,
    # one row each.
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
    # -- 7 ------------------------------------------------------------------
    # The Claude usage ledger. One row per conversation `claude/modes.converse`
    # ran, written by `claude/ledger.py` and read by `mtglab claude usage`.
    # Every mode already counted its tokens and the CLI printed them; the
    # hosted instance -- where the spending actually happens -- discarded the
    # numbers with the job payload, so nothing could answer "what did this
    # month's dossiers cost". This table is that answer.
    #
    # Deliberately aggregate: counters, a mode name and a model id. No user id,
    # no deck slug, no question text -- so it never becomes a chat log by
    # accretion, and ADR 17's who-may-read-what argument never has to be made
    # for it. Derived-adjacent like `sim_cache` above (droppable without losing
    # anything but history), and it lives here for the same reasons: same
    # volume, same backup, same migration ladder.
    """
    CREATE TABLE claude_usage (
        id                INTEGER PRIMARY KEY AUTOINCREMENT,
        created_at        TEXT    NOT NULL,
        -- The mode's own name ('rationale-interview', 'commander-dossier'...),
        -- which is the per-surface axis every cost question starts from.
        mode              TEXT    NOT NULL,
        -- The model that answered, from the response rather than the request:
        -- an A/B run via MTGLAB_CLAUDE_MODEL should be legible here later.
        model             TEXT    NOT NULL,
        -- 'end_turn', 'refusal', 'exhausted'... -- so spend on conversations
        -- that produced nothing usable is visible as exactly that.
        stop_reason       TEXT    NOT NULL,
        -- API requests the tool loop made for this one conversation.
        requests          INTEGER NOT NULL,
        input_tokens      INTEGER NOT NULL,
        output_tokens     INTEGER NOT NULL,
        -- Prompt tokens served from the cache at ~a tenth of the input price.
        -- Counted *beside* input_tokens, never inside it: the API reports
        -- input_tokens as the uncached remainder, so a conversation's whole
        -- prompt is the two added together. The number that says whether the
        -- cache is working at all -- and note what is missing next to it,
        -- cache *writes*, which bill at 1.25x input and are recorded nowhere.
        cache_read_tokens INTEGER NOT NULL
    );

    CREATE INDEX claude_usage_by_time ON claude_usage(created_at);
    """,
    # -- 8 ------------------------------------------------------------------
    # The deck activity log (ADR 28). One row per edit, written by
    # `decks/log.py` from `service._commit` -- the single place every deck
    # write already passes through, which is what makes "an edit that is not
    # logged" a thing no route can produce by forgetting.
    #
    # This is the table ADR 15 lists as a prerequisite for deck conversation's
    # write autonomy: "what did it change while I was not looking" is a
    # question a log answers and nothing else here can. Nothing autonomous
    # writes yet, and the column that will carry the answer -- `actor` -- is
    # here from the start rather than added later, because a log that cannot
    # say *who* only postpones the question.
    #
    # Three column decisions, each argued rather than defaulted:
    #
    # - **`owner_id`, not an owner name.** `/api/decks/{owner}/{slug}` names
    #   its owner as a *segment*, and that segment is not stable across
    #   configurations: the file tier is `local` on a laptop and the
    #   maintainer's username on a deployment (`decks/library.py:_file_owner`).
    #   Keying on the segment would split one deck's history in two the day
    #   `MTGLAB_ADMIN_EMAIL` was set. So the file tier is **NULL** -- there is
    #   exactly one of it per instance, which is the whole of what it means --
    #   and everybody else's decks carry `user_decks.owner_id`. NULL also
    #   keeps `mtglab decks log` honest: the CLI only ever edits the file tier,
    #   so it reads and writes exactly the rows the app files under it.
    # - **CASCADE, matching `user_decks`.** Deleting an account takes its decks
    #   (migration 6); a history of edits to decks that no longer exist,
    #   attributed to somebody who no longer has an account, is not something
    #   to keep. NULL is exempt from the constraint, which is what lets the
    #   same column mean "the file tier" without a second table.
    # - **`summary` is rendered here, not at read time.** `_commit` already
    #   assembles the per-operation description and throws it away; this
    #   stores the sentence it made. One renderer, server-side, rather than
    #   one in Python for the CLI and a second in TypeScript for the panel
    #   that would drift. `action` rides alongside so the row stays queryable
    #   ("how many cards did I entomb in July") without parsing prose.
    #
    # **No rationale text ever lands here**, which is a property rather than an
    # accident: `describe` builds the sentence from card names, categories and
    # field *names*, and drops `why` where `swap_card` passes it. The log says
    # a rationale changed and never what it says -- rule 4's text lives in
    # `deck.yaml`, which is the source of truth, and a second copy in a table
    # nobody edits is a copy that goes stale and a place a rationale could be
    # mined from. `tests/test_deck_log.py` pins it.
    """
    CREATE TABLE deck_log (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        created_at TEXT NOT NULL,
        -- NULL is the file-backed curated library, of which there is one per
        -- instance. See the note above for why this is not an owner segment.
        owner_id   INTEGER REFERENCES users(id) ON DELETE CASCADE,
        slug       TEXT NOT NULL,
        -- The account that made the edit, or NULL for whoever is at this
        -- machine -- the CLI, and the app with auth off. A username, never an
        -- email: this is read by anyone who may read the deck, and an address
        -- must never reach a log line.
        actor      TEXT,
        -- A stable verb: 'add', 'entomb', 'return', 'exile', 'remove',
        -- 'swap', 'set-card', 'set-deck', 'note', 'edit'.
        action     TEXT    NOT NULL,
        -- The sentence both the CLI and the deck panel show.
        summary    TEXT    NOT NULL
    );

    -- The one query there is: this deck's history, newest first. `id` is in
    -- the index so the ordering comes off it too -- `created_at` would order
    -- identically but two edits inside the same second would tie, and an
    -- autoincrementing id cannot.
    CREATE INDEX deck_log_by_deck ON deck_log(owner_id, slug, id);
    """,
    # -- 9 ------------------------------------------------------------------
    # The visitor ledger (`api/traffic.py`, ROADMAP item 14 PR 6): how much
    # this instance is used, kept the way this project keeps personal data --
    # by not keeping it. Four columns and no fifth: the UTC day, the matched
    # route TEMPLATE (`/api/decks/{owner}/{slug}`, never the concrete path a
    # person typed -- a path can carry a slug and a slug can carry a person),
    # the status class, and a count. No IP, no user agent, no username, no
    # timestamp finer than the day. `tests/test_traffic.py` pins the column
    # set. Derived-adjacent like `sim_cache`: dropping it loses history,
    # never anybody's data.
    """
    CREATE TABLE request_log (
        day          TEXT    NOT NULL,
        route        TEXT    NOT NULL,
        status_class TEXT    NOT NULL,
        count        INTEGER NOT NULL,
        PRIMARY KEY (day, route, status_class)
    );
    """,
    # -- 10 -----------------------------------------------------------------
    # Per-account model tiers (2026-08-18, Aaron's call). Every Claude surface
    # has answered on one model since ADR 14; a handful of seats may now reach
    # for a more capable one, and everybody else keeps the house answer.
    #
    # An `ALTER TABLE ... ADD COLUMN`, not a rebuild: it is a new nullable
    # column with no constraint to add and nothing to backfill, which is the
    # one shape SQLite can do in place. Migration 5's rebuild machinery is
    # still above and still needed by whatever wants a constraint next.
    #
    # NULL means "the default tier", and is what every existing row gets --
    # so this migration cannot change what any account is answered by. It
    # holds a tier **key** (`'opus'`), never a model id: see `claude/tiers.py`
    # for why a column full of model ids would be a migration a year from now.
    # An unknown key resolves to the default rather than raising, which is what
    # makes a rolled-back deploy survivable.
    """
    ALTER TABLE users ADD COLUMN model_tier TEXT;
    """,
    # -- 11 -----------------------------------------------------------------
    # The match ledger (ADR 36): every Forge match this instance plays,
    # recorded by `sim/tier3/ledger.py` from the two places a match finishes
    # -- the API job and the CLI. The data foundry the Simulator's next phase
    # drinks from: Bayesian ratings, the win-probability regression and the
    # game-length survival curves are all queries over these three tables,
    # and Tier 2's calibration anchor is too.
    #
    # Three tables rather than one, because the three kinds of row answer
    # different questions and are joined on demand: a *match* is one JVM run
    # (its seed, clock and provenance), a *seat* is a deck's part in it, and a
    # *game* is one measured outcome. Not JSON blobs like `sim_cache`, and
    # deliberately: `sim_cache` rows are opaque memoisation, while these are
    # the dataset the rating boards GROUP BY -- a shape SQL should see.
    #
    # Decisions, each argued rather than defaulted:
    #
    # - **Games store facts as parsed, never verdicts.** `winner_seat` is kept
    #   even when `timed_out` is set, exactly as `parse.GameResult` keeps
    #   them apart -- a clocked-out game with a winner line is a real
    #   measurement of a fake outcome, and folding the rule "a clock-out
    #   counts for nobody" into the row would make it unqueryable the day the
    #   rule needs re-examining. Readers apply the rule; `ledger.py`'s
    #   tallies are the reference implementation.
    # - **Seats snapshot the deck's labels at match time.** `archetype` and
    #   `themes` are copied from the deck when it plays, because the boards
    #   group matches by the class a deck *wore when it played* -- relabelling
    #   a deck must not silently rewrite the history of every game it ever
    #   played. `themes` is a JSON array in TEXT: it is carried for display
    #   and later analysis, never grouped on, so first normal form buys
    #   nothing here but a fourth table.
    # - **`owner_id` follows `deck_log`**: NULL is the file-backed curated
    #   library, everybody else's decks carry `user_decks.owner_id`, and the
    #   CASCADE takes a deleted account's seats with it -- their slugs are
    #   their content. A match missing a seat drops out of per-deck queries
    #   by construction (the JOIN finds nothing), which is the intended
    #   afterlife rather than an accident.
    # - **`seed` is nullable.** API runs are always seeded; a CLI run can be
    #   deliberately unseeded, and recording NULL is honest where inventing a
    #   number would poison "is this reproducible" forever.
    # - **`forge_version` is nullable**, because an old worker image answers
    #   the wire without one; the reader treats NULL as "not reported".
    #
    # Derived-adjacent like nothing else here: these rows can NOT be
    # recomputed (a Forge game is a real event), but losing them costs
    # history and ratings, never anybody's words or credentials.
    """
    CREATE TABLE forge_matches (
        id              INTEGER PRIMARY KEY AUTOINCREMENT,
        created_at      TEXT    NOT NULL,
        seed            INTEGER,
        clock           INTEGER NOT NULL,
        games_requested INTEGER NOT NULL,
        forge_version   TEXT,
        -- 0: this machine's JVM; 1: the forge-worker machine (ADR 35).
        hosted          INTEGER NOT NULL DEFAULT 0,
        wall_seconds    REAL
    );

    CREATE TABLE forge_seats (
        match_id  INTEGER NOT NULL REFERENCES forge_matches(id)
                  ON DELETE CASCADE,
        -- 1-based, the order the decks were handed to Forge; what
        -- `forge_games.winner_seat` points at.
        seat      INTEGER NOT NULL,
        owner_id  INTEGER REFERENCES users(id) ON DELETE CASCADE,
        slug      TEXT    NOT NULL,
        -- The commander names as a JSON array (partners are two), for the
        -- theater and the war room to render without loading the deck.
        commander TEXT    NOT NULL,
        -- The labels the deck wore when it played. '' / '[]' when undeclared.
        archetype TEXT    NOT NULL,
        themes    TEXT    NOT NULL,
        PRIMARY KEY (match_id, seat)
    );

    -- The per-deck question ("every match goreclaw played"), which is the
    -- one every board and every rating update starts from.
    CREATE INDEX forge_seats_by_deck ON forge_seats(owner_id, slug);

    CREATE TABLE forge_games (
        match_id     INTEGER NOT NULL REFERENCES forge_matches(id)
                     ON DELETE CASCADE,
        game_index   INTEGER NOT NULL,
        -- As parsed, even beside timed_out = 1. See the note above.
        winner_seat  INTEGER,
        milliseconds INTEGER NOT NULL,
        turns        INTEGER,
        draw         INTEGER NOT NULL,
        timed_out    INTEGER NOT NULL,
        PRIMARY KEY (match_id, game_index)
    );
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
