CREATE TABLE nfl_player_grades (
    gsis_id           TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season            INT NOT NULL,
    position_group    TEXT NOT NULL,

    overall           NUMERIC(5,2) NOT NULL,   -- 0-100 percentile within position group
    production        NUMERIC(5,2) NOT NULL,   -- raw statistical output sub-score
    efficiency        NUMERIC(5,2) NOT NULL,   -- EPA, comp%, YAC, etc. sub-score
    usage             NUMERIC(5,2) NOT NULL,   -- target share, snap proxy, volume sub-score
    durability        NUMERIC(5,2) NOT NULL,   -- games played / expected sub-score

    career_phase      TEXT NOT NULL,            -- developing / prime / post-prime / late-career
    yoy_trend         NUMERIC(5,3),            -- change in overall vs prior season (-1 to +1)

    dimension_details JSONB NOT NULL DEFAULT '{}',
    computed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (gsis_id, season)
);

CREATE INDEX idx_pg_ranking ON nfl_player_grades (season, position_group, overall DESC);
CREATE INDEX idx_pg_gsis    ON nfl_player_grades (gsis_id);
