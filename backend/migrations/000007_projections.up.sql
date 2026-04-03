-- Pre-computed per-player, per-season profile used for similarity matching.
-- One row per player per season where they recorded meaningful playing time (4+ games).
CREATE TABLE IF NOT EXISTS nfl_player_season_profiles (
    id              BIGSERIAL PRIMARY KEY,
    gsis_id         TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season          INT NOT NULL,
    age             INT,                        -- player age at start of that season
    years_exp       INT,                        -- years of NFL experience
    draft_number    INT,                        -- overall draft pick (NULL = undrafted)
    position_group  TEXT NOT NULL,              -- QB, RB, WR, TE, K (primary)
    games_played    INT NOT NULL,

    -- Physical profile (snapshot from nfl_players at time of computation)
    height          INT,                        -- inches
    weight          INT,                        -- pounds

    -- Per-game production rates (regular season only)
    pass_att_pg     NUMERIC(8,3) DEFAULT 0,
    pass_yds_pg     NUMERIC(8,3) DEFAULT 0,
    pass_td_pg      NUMERIC(8,3) DEFAULT 0,
    int_pg          NUMERIC(8,3) DEFAULT 0,
    rush_att_pg     NUMERIC(8,3) DEFAULT 0,
    rush_yds_pg     NUMERIC(8,3) DEFAULT 0,
    rush_td_pg      NUMERIC(8,3) DEFAULT 0,
    targets_pg      NUMERIC(8,3) DEFAULT 0,
    rec_pg          NUMERIC(8,3) DEFAULT 0,
    rec_yds_pg      NUMERIC(8,3) DEFAULT 0,
    rec_td_pg       NUMERIC(8,3) DEFAULT 0,
    fpts_pg         NUMERIC(8,3) DEFAULT 0,     -- standard fantasy points/game
    fpts_ppr_pg     NUMERIC(8,3) DEFAULT 0,     -- PPR fantasy points/game
    fg_made_pg      NUMERIC(8,3) DEFAULT 0,
    pat_made_pg     NUMERIC(8,3) DEFAULT 0,

    -- Efficiency metrics
    pass_ypa        NUMERIC(8,3),               -- yards per pass attempt
    comp_pct        NUMERIC(6,3),               -- completion percentage (0-100)
    rush_ypc        NUMERIC(8,3),               -- yards per carry
    rec_ypr         NUMERIC(8,3),               -- yards per reception
    target_share    NUMERIC(6,4),               -- average weekly target share
    wopr            NUMERIC(6,4),               -- average weekly WOPR
    pass_epa_play   NUMERIC(8,4),               -- passing EPA per attempt
    rush_epa_play   NUMERIC(8,4),               -- rushing EPA per carry
    rec_epa_play    NUMERIC(8,4),               -- receiving EPA per target

    -- Usage balance: fraction of yards from rushing vs receiving (0=pure receiver, 1=pure rusher)
    rush_yard_share NUMERIC(6,4),

    -- Team context proxy (offensive quality of the player's team that season)
    team_fpts_pg    NUMERIC(8,3),               -- team total fantasy points per game
    team_pass_yds_pg NUMERIC(8,3),              -- team passing yards per game
    team_rush_yds_pg NUMERIC(8,3),              -- team rushing yards per game

    -- Pre-computed z-scores within position_group + season (for fast similarity lookup)
    -- Stored as {"pass_yds_pg": 1.23, "rush_td_pg": -0.5, ...}
    z_scores        JSONB NOT NULL DEFAULT '{}',

    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (gsis_id, season)
);

CREATE INDEX idx_profiles_pos_season   ON nfl_player_season_profiles (position_group, season);
CREATE INDEX idx_profiles_gsis_season  ON nfl_player_season_profiles (gsis_id, season);
CREATE INDEX idx_profiles_age_pos      ON nfl_player_season_profiles (age, position_group);

-- Stores the final comp-based projection for each player.
-- Computed by cmd/projections, updated each time projections are re-run.
CREATE TABLE IF NOT EXISTS nfl_projections (
    id              BIGSERIAL PRIMARY KEY,
    gsis_id         TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    base_season     INT NOT NULL,               -- season used as input (e.g. 2024)
    target_season   INT NOT NULL,               -- season being projected (e.g. 2025)

    -- Projected per-game rates
    proj_fpts_pg        NUMERIC(8,3) DEFAULT 0,
    proj_fpts_ppr_pg    NUMERIC(8,3) DEFAULT 0,
    proj_pass_yds_pg    NUMERIC(8,3) DEFAULT 0,
    proj_pass_td_pg     NUMERIC(8,3) DEFAULT 0,
    proj_rush_yds_pg    NUMERIC(8,3) DEFAULT 0,
    proj_rush_td_pg     NUMERIC(8,3) DEFAULT 0,
    proj_rec_pg         NUMERIC(8,3) DEFAULT 0,
    proj_rec_yds_pg     NUMERIC(8,3) DEFAULT 0,
    proj_rec_td_pg      NUMERIC(8,3) DEFAULT 0,
    proj_fg_made_pg     NUMERIC(8,3) DEFAULT 0,
    proj_pat_made_pg    NUMERIC(8,3) DEFAULT 0,

    -- Season totals (proj_*_pg * proj_games)
    proj_games          INT DEFAULT 17,
    proj_fpts           NUMERIC(8,2) DEFAULT 0,
    proj_fpts_ppr       NUMERIC(8,2) DEFAULT 0,
    proj_fpts_half      NUMERIC(8,2) DEFAULT 0, -- half-PPR

    -- Confidence breakdown (each 0–1)
    confidence          NUMERIC(5,3) DEFAULT 0, -- weighted overall
    conf_similarity     NUMERIC(5,3) DEFAULT 0, -- avg similarity of qualifying comps
    conf_comp_count     NUMERIC(5,3) DEFAULT 0, -- min(1, count/10)
    conf_agreement      NUMERIC(5,3) DEFAULT 0, -- 1/(1+stdev_of_growth_rates)
    conf_sample_depth   NUMERIC(5,3) DEFAULT 0, -- fraction of comps with future data
    conf_data_quality   NUMERIC(5,3) DEFAULT 0, -- min(1, target_seasons/3)

    -- Comp metadata
    comp_count          INT DEFAULT 0,          -- number of comps above similarity threshold
    avg_similarity      NUMERIC(5,3) DEFAULT 0, -- average similarity of those comps
    uniqueness          TEXT DEFAULT 'moderate', -- 'common', 'moderate', 'rare', 'unique'

    -- Full comp detail as JSONB (avoids a join table; always read together with projection)
    -- Array of: {gsis_id, name, match_season, match_age, similarity, weight,
    --            headshot_url, match_profile:{...}, trajectory:[{season,age,fpts_ppr_pg,growth}]}
    comps               JSONB NOT NULL DEFAULT '[]',

    computed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (gsis_id, base_season, target_season)
);

CREATE INDEX idx_projections_target  ON nfl_projections (target_season);
CREATE INDEX idx_projections_gsis    ON nfl_projections (gsis_id, target_season);
CREATE INDEX idx_projections_points  ON nfl_projections (target_season, proj_fpts_ppr DESC);
