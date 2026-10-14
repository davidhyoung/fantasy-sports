-- Rosters become per-season so a native NBA league can look back at prior
-- years' rosters while the current season starts empty (aside from keepers
-- the commissioner re-adds). Existing rows were built against the prior
-- NBA season (the league's season label is the year the season *ends*
-- minus one for stats purposes: stats season N = the N-(N+1) season), so
-- they're backfilled to leagues.season - 1 when it parses, else left at
-- leagues.season.
ALTER TABLE league_rosters_nba ADD COLUMN IF NOT EXISTS season INT;
UPDATE league_rosters_nba lr
SET season = COALESCE(
    (SELECT CASE WHEN l.season ~ '^\d+$' THEN l.season::int - 1 END FROM leagues l WHERE l.id = lr.league_id),
    0)
WHERE season IS NULL;
ALTER TABLE league_rosters_nba ALTER COLUMN season SET NOT NULL;
ALTER TABLE league_rosters_nba DROP CONSTRAINT league_rosters_nba_pkey;
ALTER TABLE league_rosters_nba ADD PRIMARY KEY (league_id, season, espn_id);
