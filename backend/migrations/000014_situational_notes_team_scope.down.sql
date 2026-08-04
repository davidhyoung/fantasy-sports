ALTER TABLE nfl_player_situational_notes DROP CONSTRAINT IF EXISTS nfl_player_situational_notes_dedup;
DROP INDEX IF EXISTS idx_situational_notes_team_scope;
ALTER TABLE nfl_player_situational_notes
    DROP COLUMN IF EXISTS scope,
    DROP COLUMN IF EXISTS team;
