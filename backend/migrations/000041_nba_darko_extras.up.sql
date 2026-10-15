-- DARKO historical comps and DPM-by-season trend, stored on the player's
-- projection row (imported by cmd/project-nba -import-darko-extras).
--   comps: [{"name":"Carmelo Anthony","season":2004,"age":19.9,"similarity":83.8}, ...]
--   trend: [{"season":2019,"dpm":1.9}, ...]  season-end DPM (DARKO ending-year seasons)
ALTER TABLE nba_darko_projections
    ADD COLUMN IF NOT EXISTS comps JSONB,
    ADD COLUMN IF NOT EXISTS trend JSONB;
