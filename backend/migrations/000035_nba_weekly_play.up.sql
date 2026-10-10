-- NBA has no existing "what calendar date is week N" concept the way NFL
-- week numbers already map onto real NFL weeks — a league's fantasy week N
-- is season_start_date + 7*(N-1) through +7*N-1. NULL until a commissioner
-- generates a schedule.
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS season_start_date DATE;

-- Regular-season schedule + frozen category results for native NBA leagues.
-- Same computed_at convention as league_matchups: NULL means not yet scored,
-- an explicit re-runnable commissioner action. No team1_points/team2_points
-- columns here — a roto matchup's result is a set of per-category outcomes
-- (league_matchup_nba_categories below), not a single point total.
CREATE TABLE IF NOT EXISTS league_matchups_nba (
    id          BIGSERIAL PRIMARY KEY,
    league_id   BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season      INT    NOT NULL,
    week        INT    NOT NULL CHECK (week > 0),
    team1_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    team2_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    is_playoffs BOOLEAN NOT NULL DEFAULT false,
    computed_at TIMESTAMPTZ,
    UNIQUE (league_id, season, week, team1_id),
    UNIQUE (league_id, season, week, team2_id)
);
CREATE INDEX IF NOT EXISTS idx_league_matchups_nba_league_season_week ON league_matchups_nba (league_id, season, week);

-- One row per team per category per matchup. FG%/FT% `value` is computed
-- from that team's starters' aggregated makes/attempts across the week's
-- games, never averaged per-game. `won` is NULL until the matchup is scored;
-- both teams' `won` can be false on an exact tie in that category.
CREATE TABLE IF NOT EXISTS league_matchup_nba_categories (
    matchup_id BIGINT NOT NULL REFERENCES league_matchups_nba(id) ON DELETE CASCADE,
    team_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    category   TEXT NOT NULL CHECK (category IN
        ('points','rebounds','assists','steals','blocks','turnovers','three_pm','fg_pct','ft_pct')),
    value      NUMERIC(10,4) NOT NULL DEFAULT 0,
    won        BOOLEAN,
    PRIMARY KEY (matchup_id, team_id, category)
);
