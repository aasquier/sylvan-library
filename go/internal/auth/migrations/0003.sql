
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
    