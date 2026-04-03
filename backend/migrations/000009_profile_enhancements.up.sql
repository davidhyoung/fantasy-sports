-- Add advanced stats and player tags to season profiles
ALTER TABLE nfl_player_season_profiles
  ADD COLUMN IF NOT EXISTS sacks_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS passing_air_yards_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS passing_yac_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS air_yards_share NUMERIC(6,4),
  ADD COLUMN IF NOT EXISTS rushing_first_downs_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS receiving_air_yards_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS receiving_yac_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS receiving_first_downs_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fumbles_pg NUMERIC(8,3) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fg_pct NUMERIC(6,4),
  ADD COLUMN IF NOT EXISTS tags TEXT[];
