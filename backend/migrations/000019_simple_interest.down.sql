-- Widen the range back. Existing ±1 values stay valid, so no data change is
-- needed — the finer grades simply become available again.
ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_interest_valid;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_interest_valid
    CHECK (interest IS NULL OR (interest BETWEEN -3 AND 3 AND interest <> 0));
