-- Per-league keeper rules (auction-style cost model).
CREATE TABLE keeper_rules (
    id              BIGSERIAL PRIMARY KEY,
    league_id       BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE UNIQUE,
    cost_increase   INT NOT NULL DEFAULT 5,    -- annual $ increase per year kept
    undrafted_base  INT NOT NULL DEFAULT 1,    -- base $ cost for undrafted/FA players
    max_years       INT,                        -- NULL = unlimited years keepable
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Local keeper wishlist with years tracking.
-- Yahoo's write API is not available to 3rd-party apps, so keeper selections
-- are stored locally and not pushed back to Yahoo.
CREATE TABLE keeper_wishlist (
    id           BIGSERIAL PRIMARY KEY,
    team_id      BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    player_key   TEXT NOT NULL,         -- Yahoo player_key (e.g. "nba.p.6014")
    player_name  TEXT NOT NULL,
    position     TEXT,
    draft_cost   INT,                   -- original auction cost (NULL if undrafted)
    years_kept   INT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(team_id, player_key)
);
