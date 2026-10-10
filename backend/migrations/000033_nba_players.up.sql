-- NBA player metadata, sourced from ESPN's team roster endpoints (see
-- internal/services/espnnba, cmd/import-nba). Unlike nfl_players (keyed by
-- nflverse's gsis_id, with espn_id as a secondary crosswalk column), there is
-- no nflverse-equivalent bulk NBA source with a maintained ID crosswalk —
-- ESPN is the sole real-stats source for native NBA leagues, so its athlete
-- id is the primary key directly, not a crosswalk.
CREATE TABLE IF NOT EXISTS nba_players (
    espn_id       TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    position      TEXT,                        -- PG/SG/SF/PF/C
    team          TEXT,                         -- most recent team abbreviation
    birth_date    DATE,
    height        INT,                          -- inches
    weight        INT,                          -- pounds
    college       TEXT,
    years_exp     INT,
    jersey_number INT,
    headshot_url  TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nba_players_position ON nba_players (position);
CREATE INDEX idx_nba_players_team ON nba_players (team);

-- One row per real NBA game a player appeared in (not per week — NBA teams
-- play 2-4x/week, so a fantasy week's category totals are summed across
-- however many of these rows fall in that week's date range at scoring
-- time). fg_made/fg_att and ft_made/ft_att are stored raw rather than as a
-- precomputed percentage — FG%/FT% roto categories must be computed from
-- totals aggregated across a team's games, never averaged per-game.
CREATE TABLE IF NOT EXISTS nba_player_stats (
    id            BIGSERIAL PRIMARY KEY,
    espn_id       TEXT NOT NULL REFERENCES nba_players(espn_id),
    espn_event_id TEXT NOT NULL,
    game_date     DATE NOT NULL,
    season        INT NOT NULL,
    team          TEXT,
    opponent      TEXT,
    minutes           NUMERIC(5,2) DEFAULT 0,
    points            INT DEFAULT 0,
    rebounds          INT DEFAULT 0,
    offensive_rebounds INT DEFAULT 0,
    defensive_rebounds INT DEFAULT 0,
    assists           INT DEFAULT 0,
    steals            INT DEFAULT 0,
    blocks            INT DEFAULT 0,
    turnovers         INT DEFAULT 0,
    fouls             INT DEFAULT 0,
    fg_made           INT DEFAULT 0,
    fg_att            INT DEFAULT 0,
    fg3_made          INT DEFAULT 0,
    fg3_att           INT DEFAULT 0,
    ft_made           INT DEFAULT 0,
    ft_att            INT DEFAULT 0,

    UNIQUE (espn_id, espn_event_id)
);

CREATE INDEX idx_nba_player_stats_season ON nba_player_stats (season);
CREATE INDEX idx_nba_player_stats_player_season ON nba_player_stats (espn_id, season);
CREATE INDEX idx_nba_player_stats_game_date ON nba_player_stats (game_date);
