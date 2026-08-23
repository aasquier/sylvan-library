
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
    