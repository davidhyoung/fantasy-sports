-- our_rank / consensus_rank_median in nfl_projection_divergences now represent
-- within-position rank (QB vs QB, RB vs RB, ...) rather than overall rank across
-- all positions — comparing overall ranks conflated raw fantasy-point volume
-- (which favors QBs) with consensus draft value (which is scarcity-adjusted).
-- See docs/stats/consensus-ensemble.md.
ALTER TABLE nfl_projection_divergences
    ADD COLUMN IF NOT EXISTS position_group TEXT;

CREATE INDEX IF NOT EXISTS idx_projection_divergences_position
    ON nfl_projection_divergences (position_group, season, format);
