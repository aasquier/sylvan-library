
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
    