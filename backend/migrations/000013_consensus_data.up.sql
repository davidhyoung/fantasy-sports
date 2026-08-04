-- External consensus rankings/ADP, situational news, and the computed divergence
-- between our comp-based projections and the outside world.
-- See docs/stats/consensus-ensemble.md — divergence-flag only, non-mutating.

-- One row per (player, season, source, format, metric_type, captured_date).
-- gsis_id is nullable: unmatched rows are kept (not dropped) for manual review.
CREATE TABLE IF NOT EXISTS nfl_consensus_rankings (
    id              BIGSERIAL PRIMARY KEY,
    gsis_id         TEXT REFERENCES nfl_players(gsis_id),
    player_name_raw TEXT NOT NULL,
    position        TEXT,
    team            TEXT,
    season          INT NOT NULL,
    source          TEXT NOT NULL,             -- e.g. 'espn', 'bleacher_report', 'underdog_adp', 'keeptradecut'
    format          TEXT NOT NULL,              -- 'standard' | 'half_ppr' | 'ppr' | 'dynasty' | 'superflex'
    metric_type     TEXT NOT NULL,              -- 'rank' | 'adp' | 'dynasty_value'
    value           NUMERIC(8,2) NOT NULL,
    captured_date   DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (player_name_raw, season, source, format, metric_type, captured_date)
);

CREATE INDEX idx_consensus_rankings_gsis   ON nfl_consensus_rankings (gsis_id, season, format);
CREATE INDEX idx_consensus_rankings_season ON nfl_consensus_rankings (season, format);

-- Discrete news events (injury, depth-chart battle, scheme change, etc.) tagged to
-- a player. Surfaced alongside a projection, never used to alter it automatically.
CREATE TABLE IF NOT EXISTS nfl_player_situational_notes (
    id                BIGSERIAL PRIMARY KEY,
    gsis_id           TEXT REFERENCES nfl_players(gsis_id),
    player_name_raw   TEXT NOT NULL,
    season            INT NOT NULL,
    category          TEXT NOT NULL,            -- 'injury' | 'depth_chart' | 'scheme' | 'holdout' | 'suspension' | 'rookie_buzz' | 'trade'
    summary           TEXT NOT NULL,
    impact_direction  TEXT NOT NULL,             -- 'positive' | 'negative' | 'neutral'
    impact_magnitude  TEXT NOT NULL,             -- 'major' | 'moderate' | 'minor'
    confidence        TEXT NOT NULL,             -- 'multi_source' | 'single_source'
    source            TEXT,
    reported_date     DATE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_situational_notes_gsis ON nfl_player_situational_notes (gsis_id, season);

-- Computed gap between our projection rank and external consensus, regenerated each
-- time the -consensus-diff step runs.
CREATE TABLE IF NOT EXISTS nfl_projection_divergences (
    id                     BIGSERIAL PRIMARY KEY,
    gsis_id                TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season                 INT NOT NULL,
    format                 TEXT NOT NULL,
    our_rank               INT NOT NULL,
    consensus_rank_median  NUMERIC(8,2) NOT NULL,
    source_count           INT NOT NULL,
    rank_delta             NUMERIC(8,2) NOT NULL, -- our_rank - consensus_rank_median
    computed_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (gsis_id, season, format)
);

CREATE INDEX idx_projection_divergences_season ON nfl_projection_divergences (season, format, rank_delta);
