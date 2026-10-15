-- DARKO (darko.app) per-player forward projections — the projection engine
-- behind native-NBA auction values. Imported by cmd/project-nba -import-darko
-- (DARKO has no API this backend can call; data is snapshotted to JSON, same
-- manual-batch philosophy as -import-consensus).
--
-- season uses THIS APP's convention (start year: 2026 = the 2026-27 season,
-- matching nba_player_stats.season), not DARKO's ending-year convention.
-- darko_id is the NBA.com person id; espn_id is resolved by name at import
-- time and left NULL (kept, not dropped) when unmatched, for manual review.
CREATE TABLE IF NOT EXISTS nba_darko_projections (
    darko_id      BIGINT NOT NULL,
    season        INT NOT NULL,
    espn_id       TEXT REFERENCES nba_players(espn_id),
    player_name   TEXT NOT NULL,
    team          TEXT,
    position      TEXT,
    age           NUMERIC(5,2),
    snapshot_date DATE NOT NULL,            -- DARKO's own as-of date for this player
    x_minutes     NUMERIC(6,3) NOT NULL,     -- projected minutes per game
    x_pace        NUMERIC(7,3) NOT NULL,     -- projected possessions per 48
    pts_100       NUMERIC(7,3) NOT NULL,
    ast_100       NUMERIC(7,3) NOT NULL,
    orb_100       NUMERIC(7,3) NOT NULL,
    drb_100       NUMERIC(7,3) NOT NULL,
    stl_100       NUMERIC(7,3) NOT NULL,
    blk_100       NUMERIC(7,3) NOT NULL,
    tov_100       NUMERIC(7,3) NOT NULL,
    fga_100       NUMERIC(7,3) NOT NULL,
    fg3a_100      NUMERIC(7,3) NOT NULL,
    fta_100       NUMERIC(7,3) NOT NULL,
    fg_pct        NUMERIC(6,5) NOT NULL,
    fg3_pct       NUMERIC(6,5) NOT NULL,
    ft_pct        NUMERIC(6,5) NOT NULL,
    imported_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (darko_id, season)
);

CREATE INDEX idx_nba_darko_projections_espn ON nba_darko_projections (espn_id, season);
