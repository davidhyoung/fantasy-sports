-- Replace the tag vocabulary with a signed interest scale:
--
--   +3 must draft   +2 love    +1 like
--   -1 dislike      -2 hate    -3 do not draft
--
-- A scale is orderable and comparable, which a set of names never was: "sort my
-- board by how much I want the player" is a column sort, not a lookup table.
-- NULL means no opinion; 0 is disallowed so there is exactly one way to say that.
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS interest SMALLINT;

-- Carry existing tags across at their nearest strength. 'avoid' was the single
-- negative and reads as "I will not draft him", so it lands at the bottom.
UPDATE draft_prep_players SET interest = CASE tag
    WHEN 'must'    THEN 3
    WHEN 'target'  THEN 2
    WHEN 'sleeper' THEN 1
    WHEN 'avoid'   THEN -3
END
WHERE tag IS NOT NULL AND interest IS NULL;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_interest_valid
    CHECK (interest IS NULL OR (interest BETWEEN -3 AND 3 AND interest <> 0));

ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_tag_valid;

ALTER TABLE draft_prep_players DROP COLUMN IF EXISTS tag;
