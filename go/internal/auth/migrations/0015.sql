
    -- Four at the table (Aaron, 2026-09-06): the night deals pods as well as
    -- duels, so a bout stops being a pair and becomes a list of seats.
    --
    -- **Everything below the night was already N-player and none of it moves.**
    -- `scribe.Main` takes deck paths variadically, `tier3.RunGames` bounds only
    -- at `len(decks) < 2`, and the match ledger records seats in `forge_seats`
    -- keyed `(match_id, seat)` — all three were driven with four decks on
    -- 2026-09-06 and came back with a correct 1..4 seat map and a real winner.
    -- `night_bouts` held the only hardcoded pair in the tree, and this rung is
    -- the whole of removing it.
    CREATE TABLE night_bout_seats (
        bout_id  INTEGER NOT NULL REFERENCES night_bouts(id) ON DELETE CASCADE,
        -- 1-based, and it is the order the decks are handed to Forge, which is
        -- what `forge_games.winner_seat` will point at once the bout records.
        -- The same contract `forge_seats.seat` carries, deliberately: a bout's
        -- seat 3 and its match's seat 3 are the same chair.
        seat     INTEGER NOT NULL,
        -- NULL owner is the house, exactly as the old columns meant it. No
        -- REFERENCES to `users`, for the reason rung 14 already argued: a seat
        -- records who was entered when the night was planned, and an account's
        -- later deletion must neither be blocked by last night's record nor
        -- reach back into it.
        owner_id INTEGER,
        slug     TEXT    NOT NULL,
        PRIMARY KEY (bout_id, seat)
    );

    -- Carry every existing bout over before its columns go. Two statements
    -- rather than a union, so the seat number is a literal at each site and
    -- cannot be got the wrong way round.
    INSERT INTO night_bout_seats (bout_id, seat, owner_id, slug)
        SELECT id, 1, seat_a_owner, seat_a_slug FROM night_bouts;
    INSERT INTO night_bout_seats (bout_id, seat, owner_id, slug)
        SELECT id, 2, seat_b_owner, seat_b_slug FROM night_bouts;

    -- **The clock is per bout now, because a pod cannot share a duel's.**
    -- Measured 2026-09-06: a duel game runs ~16s and a four-seat game ~135s
    -- median with a tail past 349s, so at `tier3.ClockDefault` (300) about one
    -- pod game in six is cut and written down as a draw with no winner — and
    -- Forge honours no interrupt, so it then plays on invisibly. 300 stays the
    -- default because that is what every bout already in this table played at.
    ALTER TABLE night_bouts ADD COLUMN clock INTEGER NOT NULL DEFAULT 300;

    -- The pair is gone. Dropped rather than left to rot: two places that both
    -- claim to say who sat down is exactly the drift the child table exists to
    -- end, and a reader that finds both has no way to know which one lied.
    ALTER TABLE night_bouts DROP COLUMN seat_a_owner;
    ALTER TABLE night_bouts DROP COLUMN seat_a_slug;
    ALTER TABLE night_bouts DROP COLUMN seat_b_owner;
    ALTER TABLE night_bouts DROP COLUMN seat_b_slug;

    -- The runner's per-bout read: every seat of one bout, in seat order.
    CREATE INDEX night_bout_seats_bout ON night_bout_seats(bout_id);
