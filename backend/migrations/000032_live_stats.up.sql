-- Best-effort, near-live NFL stats scraped from ESPN's unofficial scoreboard
-- API during games, kept fully isolated from nfl_player_stats (the real
-- nflverse batch import) so a bad/incomplete ESPN parse can never leak into
-- projections, grades, or backtesting. This table is a preview source only:
-- league_matchups.team1_points/team2_points/computed_at (the frozen, official
-- score) is never written from it.
--
-- stats is a map[CanonicalStat]float64 (see internal/services/scoring),
-- not individual numeric columns — we only ever read it back into Go as a
-- small map (same shape nflstats.LoadWeekStats returns), never aggregated
-- with SQL SUM(), so a JSONB blob avoids having to mirror nfl_player_stats'
-- ~20 columns for a subset ESPN's box score actually exposes.
CREATE TABLE IF NOT EXISTS nfl_live_player_stats (
    gsis_id    TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season     INT  NOT NULL,
    week       INT  NOT NULL,
    stats      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (gsis_id, season, week)
);
CREATE INDEX IF NOT EXISTS idx_nfl_live_player_stats_season_week ON nfl_live_player_stats (season, week);
