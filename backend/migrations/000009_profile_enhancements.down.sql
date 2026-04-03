ALTER TABLE nfl_player_season_profiles
  DROP COLUMN IF EXISTS sacks_pg,
  DROP COLUMN IF EXISTS passing_air_yards_pg,
  DROP COLUMN IF EXISTS passing_yac_pg,
  DROP COLUMN IF EXISTS air_yards_share,
  DROP COLUMN IF EXISTS rushing_first_downs_pg,
  DROP COLUMN IF EXISTS receiving_air_yards_pg,
  DROP COLUMN IF EXISTS receiving_yac_pg,
  DROP COLUMN IF EXISTS receiving_first_downs_pg,
  DROP COLUMN IF EXISTS fumbles_pg,
  DROP COLUMN IF EXISTS fg_pct,
  DROP COLUMN IF EXISTS tags;
