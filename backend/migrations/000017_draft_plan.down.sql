DROP INDEX IF EXISTS idx_draft_prep_plan;

-- Rows planned but not tagged carry nothing once the column is gone.
DELETE FROM draft_prep_players
WHERE planned_cost IS NOT NULL AND tag IS NULL AND custom_rank IS NULL AND note = '';

ALTER TABLE draft_prep_players DROP COLUMN IF EXISTS planned_cost;

-- Narrow the tag vocabulary back, clearing values the old constraint rejects.
UPDATE draft_prep_players SET tag = 'target' WHERE tag = 'must';

ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_tag_valid;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_tag_valid
    CHECK (tag IS NULL OR tag IN ('target', 'sleeper', 'avoid'));
