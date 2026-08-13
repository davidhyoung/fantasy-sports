-- Collapse the six-level interest scale to a binary: target (+1) or avoid (-1).
--
-- The finer grades cost more to maintain than they paid back — deciding between
-- "love" and "like" is a judgment you make once and then can't act on
-- differently. The sign is kept (rather than a boolean) so ordering still works
-- and so re-expanding the scale later is a constraint change, not a data model
-- change.
UPDATE draft_prep_players SET interest = sign(interest) WHERE interest IS NOT NULL;

ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_interest_valid;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_interest_valid
    CHECK (interest IS NULL OR interest IN (-1, 1));
