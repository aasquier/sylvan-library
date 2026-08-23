
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
    