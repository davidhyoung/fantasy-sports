-- Rows carrying only my_value carry nothing once the column is gone.
DELETE FROM draft_prep_players
WHERE my_value IS NOT NULL AND interest IS NULL AND custom_rank IS NULL
    AND custom_tier IS NULL AND note = '' AND planned_cost IS NULL;

ALTER TABLE draft_prep_players DROP COLUMN IF EXISTS my_value;
