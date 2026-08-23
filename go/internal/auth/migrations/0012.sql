
    CREATE TABLE user_deck_artifacts (
        deck_id  INTEGER NOT NULL REFERENCES user_decks(id) ON DELETE CASCADE,
        name     TEXT    NOT NULL,
        body     TEXT    NOT NULL,
        built_at TEXT    NOT NULL,
        PRIMARY KEY (deck_id, name)
    );
    