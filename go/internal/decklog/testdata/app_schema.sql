-- app.db, as `auth/db.py`'s migration ladder leaves it at schema
-- version 12. Written by `python tests/go_fixtures.py`; Python
-- owns the ladder until Phase 8 and this is a reading of what it made,
-- for the Go tests that need a real table in CI. Do not hand-edit.
PRAGMA user_version = 12;
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
CREATE TABLE login_attempts (
        key          TEXT PRIMARY KEY,
        window_start TEXT NOT NULL,
        failures     INTEGER NOT NULL
    );
CREATE TABLE request_log (
        day          TEXT    NOT NULL,
        route        TEXT    NOT NULL,
        status_class TEXT    NOT NULL,
        count        INTEGER NOT NULL,
        PRIMARY KEY (day, route, status_class)
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
CREATE TABLE user_deck_artifacts (
        deck_id  INTEGER NOT NULL REFERENCES user_decks(id) ON DELETE CASCADE,
        name     TEXT    NOT NULL,
        body     TEXT    NOT NULL,
        built_at TEXT    NOT NULL,
        PRIMARY KEY (deck_id, name)
    );
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
CREATE TABLE "users" (
        id            INTEGER PRIMARY KEY AUTOINCREMENT,
        username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
        password_hash TEXT,
        email         TEXT UNIQUE COLLATE NOCASE,
        is_admin      INTEGER NOT NULL DEFAULT 0,
        disabled_at   TEXT,
        created_at    TEXT NOT NULL
    , model_tier TEXT);
CREATE INDEX auth_tokens_by_user ON auth_tokens(user_id, purpose);
CREATE INDEX claude_usage_by_time ON claude_usage(created_at);
CREATE INDEX deck_log_by_deck ON deck_log(owner_id, slug, id);
CREATE INDEX dossier_cache_by_oracle ON dossier_cache(oracle_id);
CREATE INDEX forge_seats_by_deck ON forge_seats(owner_id, slug);
CREATE INDEX sessions_by_user ON sessions(user_id);
CREATE INDEX sim_cache_by_use ON sim_cache(last_used_at);
CREATE INDEX user_decks_by_owner
        ON user_decks(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX user_decks_shared
        ON user_decks(shared) WHERE shared = 1 AND deleted_at IS NULL;
CREATE UNIQUE INDEX user_decks_slug
        ON user_decks(owner_id, slug) WHERE deleted_at IS NULL;
