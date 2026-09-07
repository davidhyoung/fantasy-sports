-- Marks a player as being kept (already locked to a team, unavailable in this
-- year's draft) rather than a personal interest opinion — a fact about
-- draft-day availability, so it defaults to false instead of NULL the way
-- interest's "no opinion" does.
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS kept BOOLEAN NOT NULL DEFAULT false;

-- The keeper's locked-in salary, when known. NULL = kept but cost unknown or
-- not applicable (e.g. flagging a rival team's keeper as off the board).
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS kept_cost INT
    CHECK (kept_cost IS NULL OR (kept_cost >= 0 AND kept_cost <= 10000));
