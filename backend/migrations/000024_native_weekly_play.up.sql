ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS regular_season_weeks INT NOT NULL DEFAULT 14;

-- Regular-season schedule + frozen scores for native leagues. computed_at
-- NULL means not yet scored — scoring is an explicit commissioner action
-- (the underlying stats come from a manual batch import, never live), not
-- something that happens automatically when a matchup row is created.
CREATE TABLE IF NOT EXISTS league_matchups (
    id            BIGSERIAL PRIMARY KEY,
    league_id     BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season        INT    NOT NULL,
    week          INT    NOT NULL CHECK (week > 0),
    team1_id      BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    team2_id      BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    team1_points  DOUBLE PRECISION NOT NULL DEFAULT 0,
    team2_points  DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_playoffs   BOOLEAN NOT NULL DEFAULT false,
    computed_at   TIMESTAMPTZ,
    -- Both constraints (not just one) are what actually stops a team being
    -- double-booked in a week, regardless of which column it lands in.
    UNIQUE (league_id, season, week, team1_id),
    UNIQUE (league_id, season, week, team2_id)
);
CREATE INDEX IF NOT EXISTS idx_league_matchups_league_season_week ON league_matchups (league_id, season, week);
