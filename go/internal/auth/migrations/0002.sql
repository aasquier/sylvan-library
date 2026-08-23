
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

    CREATE INDEX auth_tokens_by_user ON auth_tokens(user_id, purpose);
    