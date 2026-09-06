
    -- Two more feats worth a top ten (Aaron, 2026-09-06): the biggest creature
    -- that stood on a battlefield, and the deepest stack of one token a seat
    -- held at once.
    --
    -- **"At once" is the whole of the second one.** A deck that makes forty
    -- Food across a long game and eats each one never had a stack; a deck that
    -- assembles nineteen Cats in a turn did. The parser tracks live
    -- battlefield membership per seat and per token name and keeps the
    -- high-water mark, so this column is a pile that existed rather than a
    -- running total of tokens made. Two seats holding two apiece is two stacks
    -- of two and never one of four.
    --
    -- **The creature ranks on power**, which is a judgement: a player says
    -- "a 15/15" and means the first number, and a wall with enormous toughness
    -- is not the thing anybody tells a story about. Toughness is stored beside
    -- it so a row can print the pair properly.
    ALTER TABLE forge_games ADD COLUMN big_card      TEXT;
    ALTER TABLE forge_games ADD COLUMN big_power     INTEGER;
    ALTER TABLE forge_games ADD COLUMN big_toughness INTEGER;
    -- Whose battlefield it stood on. Resolved through the parser's own
    -- id-to-seat map: a `stats` line names a card and never a player, so
    -- without that map a creature pumped to fifteen would belong to nobody.
    ALTER TABLE forge_games ADD COLUMN big_seat      INTEGER;
    -- Player-turns, like `kill_turn` and unlike `turns`. Same argument.
    ALTER TABLE forge_games ADD COLUMN big_turn      INTEGER;

    ALTER TABLE forge_games ADD COLUMN stack_card    TEXT;
    ALTER TABLE forge_games ADD COLUMN stack_count   INTEGER;
    ALTER TABLE forge_games ADD COLUMN stack_seat    INTEGER;
    ALTER TABLE forge_games ADD COLUMN stack_turn    INTEGER;

    -- One partial index per leaderboard, for the same reason `kill_amount`
    -- has one: a game with no creature in it is rare, a game with no token in
    -- it is not, and neither board wants to read the games that have nothing
    -- to say.
    CREATE INDEX forge_games_by_big ON forge_games(big_power DESC)
        WHERE big_power IS NOT NULL;
    CREATE INDEX forge_games_by_stack ON forge_games(stack_count DESC)
        WHERE stack_count IS NOT NULL;
