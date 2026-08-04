DROP INDEX IF EXISTS idx_projection_divergences_position;
ALTER TABLE nfl_projection_divergences DROP COLUMN IF EXISTS position_group;
