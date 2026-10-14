ALTER TABLE league_rosters_nba DROP CONSTRAINT league_rosters_nba_pkey;
DELETE FROM league_rosters_nba a USING league_rosters_nba b
 WHERE a.league_id = b.league_id AND a.espn_id = b.espn_id AND a.season < b.season;
ALTER TABLE league_rosters_nba ADD PRIMARY KEY (league_id, espn_id);
ALTER TABLE league_rosters_nba DROP COLUMN season;
