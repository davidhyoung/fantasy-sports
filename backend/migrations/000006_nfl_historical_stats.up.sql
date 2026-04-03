-- nflverse player metadata (from roster CSVs)
CREATE TABLE IF NOT EXISTS nfl_players (
    gsis_id       TEXT PRIMARY KEY,           -- e.g. "00-0023459"
    name          TEXT NOT NULL,
    position      TEXT,
    position_group TEXT,
    team          TEXT,                        -- most recent team
    birth_date    DATE,
    height        INT,                         -- inches
    weight        INT,                         -- pounds
    college       TEXT,
    years_exp     INT,
    entry_year    INT,
    rookie_year   INT,
    draft_club    TEXT,
    draft_number  INT,
    jersey_number INT,
    headshot_url  TEXT,
    -- cross-reference IDs for joining with Yahoo, ESPN, etc.
    yahoo_id      TEXT,
    espn_id       TEXT,
    sportradar_id TEXT,
    rotowire_id   TEXT,
    sleeper_id    TEXT,
    pfr_id        TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nfl_players_position ON nfl_players (position);
CREATE INDEX idx_nfl_players_yahoo_id ON nfl_players (yahoo_id);
CREATE INDEX idx_nfl_players_team ON nfl_players (team);

-- nflverse weekly player stats (from player_stats CSVs)
CREATE TABLE IF NOT EXISTS nfl_player_stats (
    id            BIGSERIAL PRIMARY KEY,
    gsis_id       TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season        INT NOT NULL,
    week          INT NOT NULL,
    season_type   TEXT NOT NULL DEFAULT 'REG',  -- REG or POST
    team          TEXT,
    opponent_team TEXT,
    -- passing
    completions       INT DEFAULT 0,
    pass_attempts     INT DEFAULT 0,
    passing_yards     NUMERIC(8,2) DEFAULT 0,
    passing_tds       INT DEFAULT 0,
    interceptions     INT DEFAULT 0,
    sacks             INT DEFAULT 0,
    sack_yards        NUMERIC(8,2) DEFAULT 0,
    passing_air_yards NUMERIC(8,2) DEFAULT 0,
    passing_yac       NUMERIC(8,2) DEFAULT 0,
    passing_first_downs INT DEFAULT 0,
    passing_epa       NUMERIC(10,4),
    passing_2pt       INT DEFAULT 0,
    -- rushing
    carries           INT DEFAULT 0,
    rushing_yards     NUMERIC(8,2) DEFAULT 0,
    rushing_tds       INT DEFAULT 0,
    rushing_fumbles   INT DEFAULT 0,
    rushing_fumbles_lost INT DEFAULT 0,
    rushing_first_downs INT DEFAULT 0,
    rushing_epa       NUMERIC(10,4),
    rushing_2pt       INT DEFAULT 0,
    -- receiving
    receptions        INT DEFAULT 0,
    targets           INT DEFAULT 0,
    receiving_yards   NUMERIC(8,2) DEFAULT 0,
    receiving_tds     INT DEFAULT 0,
    receiving_fumbles INT DEFAULT 0,
    receiving_fumbles_lost INT DEFAULT 0,
    receiving_air_yards NUMERIC(8,2) DEFAULT 0,
    receiving_yac     NUMERIC(8,2) DEFAULT 0,
    receiving_first_downs INT DEFAULT 0,
    receiving_epa     NUMERIC(10,4),
    receiving_2pt     INT DEFAULT 0,
    target_share      NUMERIC(6,4),
    wopr              NUMERIC(6,4),
    -- kicking
    fg_made           INT DEFAULT 0,
    fg_att            INT DEFAULT 0,
    fg_missed         INT DEFAULT 0,
    fg_long           INT DEFAULT 0,
    pat_made          INT DEFAULT 0,
    pat_att           INT DEFAULT 0,
    -- special teams
    special_teams_tds INT DEFAULT 0,
    -- fantasy
    fantasy_points     NUMERIC(8,2) DEFAULT 0,
    fantasy_points_ppr NUMERIC(8,2) DEFAULT 0,

    UNIQUE (gsis_id, season, week, season_type)
);

CREATE INDEX idx_nfl_player_stats_season ON nfl_player_stats (season);
CREATE INDEX idx_nfl_player_stats_player_season ON nfl_player_stats (gsis_id, season);
CREATE INDEX idx_nfl_player_stats_team_season ON nfl_player_stats (team, season);
