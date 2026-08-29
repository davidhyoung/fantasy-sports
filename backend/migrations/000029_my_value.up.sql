-- The user's own valuation for a player, distinct from the algorithm's
-- auction_value — a dollar figure they set themselves, or that gets filled in
-- automatically when they move the player around the board or the tiers view
-- (interpolated between whichever players now flank it). NULL = no opinion
-- yet, same convention planned_cost already uses; not defaulted to 0, since
-- $0 is a meaningful valuation.
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS my_value INT
    CHECK (my_value IS NULL OR (my_value >= 0 AND my_value <= 10000));
