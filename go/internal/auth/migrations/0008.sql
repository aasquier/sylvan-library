
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
    