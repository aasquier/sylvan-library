
    -- The Coliseum at Night's own rows (ADR 46): one per night the arena
    -- opens, and one per bout that night plans. Rung 13 recorded who wants
    -- in; these two tables are the nights actually held. Every question the
    -- runner asks -- is a night open, what is left of it, what became of
    -- each bout -- is answered from here rather than from process memory,
    -- because merging deploys (ADR 23) and a deploy restarts the process at
    -- any hour, including mid-night. A runner that keeps its progress in
    -- rows is crash-safe by being resumable rather than by being careful.
    --
    -- `forge_matches` deliberately gains no night marker (ADR 46 decision
    -- 7): a bout that played carries its `match_id`, so "last night's games"
    -- is a join from here, and the match ledger stays one undisturbed record
    -- of matches however they were asked for.
    CREATE TABLE night_runs (
        id          INTEGER PRIMARY KEY AUTOINCREMENT,
        -- The local date the window opened ('2026-09-06') in the configured
        -- zone; a night that crosses midnight keeps the key of the evening
        -- it started.
        night_key   TEXT    NOT NULL,
        -- 1: an admin-triggered measurement run -- the sample the window's
        -- first real value is chosen from -- bounded by its own deadline
        -- rather than by the schedule.
        sample      INTEGER NOT NULL DEFAULT 0,
        opened_at   TEXT    NOT NULL,
        -- The schedule's close, or the sample's deadline. An admin close
        -- pulls it back to "now"; a bout in flight still finishes (ADR 46
        -- decision 6).
        closes_at   TEXT    NOT NULL,
        -- Set when the runner declares the night over. NULL is the open run,
        -- and the resume read keys on exactly that.
        finished_at TEXT
    );

    -- One scheduled night per local date, held by the schema rather than by
    -- the runner's manners. Samples are exempt: a measurement may be re-run,
    -- and refusing two of them *at once* is the admin route's job.
    CREATE UNIQUE INDEX night_runs_one_per_night ON night_runs(night_key)
        WHERE sample = 0;

    CREATE TABLE night_bouts (
        id           INTEGER PRIMARY KEY AUTOINCREMENT,
        run_id       INTEGER NOT NULL REFERENCES night_runs(id),
        -- NULL owner is the house: a deck off the file tier, which plays
        -- every night and has no `user_decks` row to point at. No REFERENCES
        -- on either owner, deliberately: a seat records who was entered when
        -- the night was planned, and an account's later deletion must
        -- neither be blocked by last night's record nor reach back into it.
        seat_a_owner INTEGER,
        seat_a_slug  TEXT    NOT NULL,
        seat_b_owner INTEGER,
        seat_b_slug  TEXT    NOT NULL,
        games        INTEGER NOT NULL,
        -- Derived and stable per bout, so a night is reproducible in
        -- principle; stored on the bout because the match ledger must not
        -- learn it came from the night.
        seed         INTEGER NOT NULL,
        -- planned | playing | done | failed | skipped. The last three are
        -- terminal, and the store refuses to rewrite them.
        state        TEXT    NOT NULL,
        -- The skip or failure diagnosis, in log-grade words. Nothing in this
        -- column ever renders to a player (commandment 10).
        reason       TEXT,
        -- forge_matches.id once the ledger recorded the bout -- the join the
        -- morning shelf reads. NULL on a bout that never recorded, including
        -- one orphaned by a restart, where staying NULL is the honest state.
        match_id     INTEGER,
        created_at   TEXT    NOT NULL,
        updated_at   TEXT    NOT NULL
    );

    -- The runner's working set: tonight's bouts, by run.
    CREATE INDEX night_bouts_run ON night_bouts(run_id);
