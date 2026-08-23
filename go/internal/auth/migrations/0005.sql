
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
    