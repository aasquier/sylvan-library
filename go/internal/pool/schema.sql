CREATE TABLE IF NOT EXISTS oracle_cards (
    oracle_id       VARCHAR PRIMARY KEY,
    name            VARCHAR,
    mana_cost       VARCHAR,
    cmc             DOUBLE,
    type_line       VARCHAR,
    oracle_text     VARCHAR,
    colors          VARCHAR[],
    color_identity  VARCHAR[],
    keywords        VARCHAR[],
    produced_mana   VARCHAR[],
    legalities      JSON,
    layout          VARCHAR,
    card_faces      JSON,
    reserved        BOOLEAN,
    edhrec_rank     INTEGER,
    released_at     DATE,
    set_code        VARCHAR,
    scryfall_uri    VARCHAR,
    image_normal    VARCHAR,
    image_art_crop  VARCHAR,
    -- Text, not numbers. Scryfall gives "*", "1+*", "7-*" and similar for
    -- cards whose stats are computed, and coercing those to integers either
    -- throws or silently invents a number. A caller that wants arithmetic can
    -- ask for it; a caller that wants to know what is printed on the card gets
    -- what is printed on the card.
    power           VARCHAR,
    toughness       VARCHAR,
    loyalty         VARCHAR,
    defense         VARCHAR,
    -- The official Commander bracket criterion, and the one thing a bracket
    -- declaration could never be checked against before. 53 cards carry it.
    game_changer    BOOLEAN,
    flavor_text     VARCHAR,
    artist          VARCHAR
);

CREATE TABLE IF NOT EXISTS printings (
    id              VARCHAR PRIMARY KEY,
    oracle_id       VARCHAR,
    name            VARCHAR,
    set_code        VARCHAR,
    set_name        VARCHAR,
    collector_number VARCHAR,
    rarity          VARCHAR,
    released_at     DATE,
    digital         BOOLEAN,
    promo           BOOLEAN,
    finishes        VARCHAR[],
    image_normal    VARCHAR,
    price_usd       DOUBLE,
    price_usd_foil  DOUBLE,
    price_eur       DOUBLE,
    tcg_product_id  VARCHAR,
    -- These two live here as well as on `oracle_cards`, and that is the whole
    -- point: a painter and a piece of flavour text belong to a *printing*.
    -- The oracle row carries Scryfall's representative one, which may be a
    -- different painting from the one a deck that pinned an art is showing --
    -- so a deck page read the set name off the chosen printing and the
    -- painter off another, and credited the wrong person in the same
    -- sentence. See `api.service._chosen_arts` for the deck it cost.
    flavor_text     VARCHAR,
    artist          VARCHAR
);

-- Append-only daily snapshots. This is what makes deal-watching possible:
-- "cheap right now" is meaningless without a baseline to compare against.
CREATE TABLE IF NOT EXISTS price_history (
    snapshot_date   DATE,
    printing_id     VARCHAR,
    oracle_id       VARCHAR,
    name            VARCHAR,
    price_usd       DOUBLE,
    price_usd_foil  DOUBLE,
    PRIMARY KEY (snapshot_date, printing_id)
);

CREATE INDEX IF NOT EXISTS idx_oracle_name ON oracle_cards(name);
CREATE INDEX IF NOT EXISTS idx_printings_oracle ON printings(oracle_id);
