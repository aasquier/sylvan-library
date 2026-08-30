
    -- "Coliseum at Night": whether this deck's owner has asked for it to be
    -- taken down to the arena after dark, once the night games begin. Nothing
    -- reads this to schedule anything yet -- the flag is the standing consent,
    -- recorded before there is a thing to consent to, so that the first night
    -- is played with decks whose owners chose them rather than with whatever
    -- happened to be in the library.
    --
    -- **In the column and nowhere else, unlike `shared`.** Aaron ruled it: the
    -- deck file is the truth about the *deck* (ADR 1), and whether somebody
    -- wants overnight games is a fact about the owner's appetite rather than
    -- about the 99 cards. Writing it into `deck.yaml` would put a key for an
    -- unbuilt feature into every hand-written file and into every export, and
    -- would have to be carried by a tier that has no rows -- so `SetShared`'s
    -- two-places-one-verb arrangement is deliberately *not* mirrored here.
    -- The consequence is recorded rather than hidden: a deck the file tier
    -- serves has no row, therefore no flag, and the write verb refuses it.
    --
    -- ALTER ... ADD COLUMN with a constant default is the whole rung: SQLite
    -- rewrites no rows for it, so this is additive on a live volume in the way
    -- ADR 23 needs a boot-time ladder step to be.
    ALTER TABLE user_decks ADD COLUMN coliseum_at_night INTEGER NOT NULL DEFAULT 0;

    -- The query the night games will open with -- "whose decks are in?" -- and
    -- the same partial shape `user_decks_shared` uses, for the same reason: the
    -- index covers the opted-in few rather than the whole table, and a deck in
    -- the crypt is not in the draw.
    CREATE INDEX user_decks_coliseum_at_night
        ON user_decks(coliseum_at_night)
        WHERE coliseum_at_night = 1 AND deleted_at IS NULL;
