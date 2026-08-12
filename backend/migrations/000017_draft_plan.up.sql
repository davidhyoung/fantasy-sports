-- Draft prep gains two things:
--
-- 1. A level of interest rather than a flat tag. 'must' sits above 'target',
--    which sits above 'sleeper'; 'avoid' is the one negative. Existing rows keep
--    their meaning — this only widens what's allowed.
-- 2. A planned cost, which turns the board into a team you can assemble: a row
--    with planned_cost set is a player you intend to roster at that price.
--    NULL means "not in the plan", which is why it isn't defaulted to 0.
ALTER TABLE draft_prep_players
    DROP CONSTRAINT IF EXISTS draft_prep_tag_valid;

ALTER TABLE draft_prep_players
    ADD CONSTRAINT draft_prep_tag_valid
    CHECK (tag IS NULL OR tag IN ('must', 'target', 'sleeper', 'avoid'));

ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS planned_cost INT
    CHECK (planned_cost IS NULL OR (planned_cost >= 0 AND planned_cost <= 10000));

-- The team builder reads only the planned players, which is a small slice of the board.
CREATE INDEX IF NOT EXISTS idx_draft_prep_plan
    ON draft_prep_players (user_id, league_id, season)
    WHERE planned_cost IS NOT NULL;
