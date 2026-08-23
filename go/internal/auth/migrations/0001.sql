
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
    