
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
    