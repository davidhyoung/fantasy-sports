DROP TABLE IF EXISTS league_dead_money;
DROP TABLE IF EXISTS league_team_seasons;
ALTER TABLE league_settings
    DROP COLUMN IF EXISTS taxi_cap_pct,
    DROP COLUMN IF EXISTS ir_cap_pct;
