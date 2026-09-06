
    -- The killing blow (Aaron, 2026-09-06): how the game actually ended, and a
    -- top ten of the biggest ones.
    --
    -- **A blow is a swing rather than a line**, which is the whole reason the
    -- amount is worth storing at all. Forge announces combat damage one source
    -- at a time and prints the victim's new life once afterwards, so the
    -- recorded corpus ends its first game with six sources — 5, 4, 2, 2, 2 and
    -- 1 — and then a single life of −13. Sixteen damage arriving at once on a
    -- player who had three. Reading the last line instead would have recorded a
    -- Spirit Token for one and made every leaderboard row name the smallest
    -- creature in an alpha strike. `tier3.KillingBlow.add` holds the argument.
    --
    -- Every column is nullable and NULL is the ordinary case: a game can end by
    -- commander damage (which Forge keeps on its own tracker, not as a blow),
    -- by decking, by an effect that says you lose, by a concession, or by the
    -- clock — and none of those is a blow. NULL means "nothing killed anybody
    -- here", never "we failed to look".
    ALTER TABLE forge_games ADD COLUMN kill_amount  INTEGER;
    -- The hardest-hitting contributor by name, which is the one a player would
    -- say did it.
    ALTER TABLE forge_games ADD COLUMN kill_card    TEXT;
    -- How many cards joined the swing: 1 is a haymaker, 6 is a team.
    ALTER TABLE forge_games ADD COLUMN kill_sources INTEGER;
    ALTER TABLE forge_games ADD COLUMN kill_combat  INTEGER;
    -- The chair that died, 1-based, the same numbering `forge_seats.seat` and
    -- `forge_games.winner_seat` use.
    ALTER TABLE forge_games ADD COLUMN kill_seat    INTEGER;
    -- Player-turns, not rounds -- deliberately unlike `forge_games.turns`,
    -- which is halved to match what Forge's own outcome line prints. A blow
    -- lands on a turn, and halving it would put half the kills in the game on
    -- a turn nobody played.
    ALTER TABLE forge_games ADD COLUMN kill_turn    INTEGER;

    -- The top ten's whole query, and partial so it indexes only the games that
    -- had a blow at all -- which on a night of clock-outs and commander kills
    -- is a small fraction of the table.
    CREATE INDEX forge_games_by_kill ON forge_games(kill_amount DESC)
        WHERE kill_amount IS NOT NULL;
