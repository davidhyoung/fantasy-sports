-- Lets a user override the algorithm's tier grouping for a player on their own
-- board. NULL = no override, falls back to the computed tier the draft-values
-- response sends every request. 1-20 mirrors custom_rank's shape (a small
-- positive integer, generous upper bound) rather than reusing the algorithm's
-- own tier count, since a user should be free to invent a tier the algorithm
-- never produced (e.g. splitting a tier the algorithm left combined).
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS custom_tier SMALLINT
    CHECK (custom_tier IS NULL OR custom_tier BETWEEN 1 AND 20);
