ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS tag TEXT;

-- Collapse the scale back to the nearest old name; the two middle negatives have
-- no distinct predecessor and fold into 'avoid'.
UPDATE draft_prep_players SET tag = CASE
    WHEN interest >= 3 THEN 'must'
    WHEN interest = 2  THEN 'target'
    WHEN interest = 1  THEN 'sleeper'
    WHEN interest < 0  THEN 'avoid'
END
WHERE interest IS NOT NULL AND tag IS NULL;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_tag_valid
    CHECK (tag IS NULL OR tag IN ('must', 'target', 'sleeper', 'avoid'));

ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_interest_valid;

ALTER TABLE draft_prep_players DROP COLUMN IF EXISTS interest;
