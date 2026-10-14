-- External NBA consensus rankings/ADP, our own batch-computed projections, and
-- the divergence between them. The NBA analog of migrations/000013_consensus_data,
-- simplified: NBA has exactly one scoring format in this app (9-cat roto), so
-- there's no 'format' dimension to track, and category-league rankings are
-- published overall rather than by position, so no position-group split either.
-- See docs/stats/nba-consensus-rankings.md.

-- One row per (player, season, source, metric_type, captured_date). espn_id is
-- nullable: unmatched rows are kept (not dropped) for manual review, same
-- convention as nfl_consensus_rankings.
CREATE TABLE IF NOT EXISTS nba_consensus_rankings (
    id              BIGSERIAL PRIMARY KEY,
    espn_id         TEXT REFERENCES nba_players(espn_id),
    player_name_raw TEXT NOT NULL,
    position        TEXT,
    team            TEXT,
    season          INT NOT NULL,
    source          TEXT NOT NULL,             -- e.g. 'espn', 'cbs', 'hashtag_basketball', 'rotowire', 'nba_com'
    metric_type     TEXT NOT NULL,              -- 'rank' | 'adp'
    value           NUMERIC(8,2) NOT NULL,
    captured_date   DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (player_name_raw, season, source, metric_type, captured_date)
);

CREATE INDEX idx_nba_consensus_rankings_espn   ON nba_consensus_rankings (espn_id, season);
CREATE INDEX idx_nba_consensus_rankings_season ON nba_consensus_rankings (season);

-- Our own batch-computed comp-based projection per player per target season —
-- the persisted analog of nfl_projections. Unlike the live, on-demand
-- per-player projection in internal/handlers/nba_projections.go (unchanged,
-- still used by the player detail panel), this is computed offline by
-- cmd/project-nba so it can be backtested (temporal-integrity-restricted) and
-- read cheaply by the consensus-divergence/rankings API without recomputing
-- the comp search on every request.
CREATE TABLE IF NOT EXISTS nba_projections (
    id             BIGSERIAL PRIMARY KEY,
    espn_id        TEXT NOT NULL REFERENCES nba_players(espn_id),
    target_season  INT NOT NULL,
    base_season    INT NOT NULL,
    points         NUMERIC(6,2) NOT NULL DEFAULT 0,
    rebounds       NUMERIC(6,2) NOT NULL DEFAULT 0,
    assists        NUMERIC(6,2) NOT NULL DEFAULT 0,
    steals         NUMERIC(6,2) NOT NULL DEFAULT 0,
    blocks         NUMERIC(6,2) NOT NULL DEFAULT 0,
    turnovers      NUMERIC(6,2) NOT NULL DEFAULT 0,
    three_pm       NUMERIC(6,2) NOT NULL DEFAULT 0,
    fg_pct         NUMERIC(5,4) NOT NULL DEFAULT 0,
    ft_pct         NUMERIC(5,4) NOT NULL DEFAULT 0,
    comp_count     INT NOT NULL DEFAULT 0,
    our_rank       INT,                         -- overall z-score fantasy-value rank among qualifying players
    computed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (espn_id, target_season)
);

CREATE INDEX idx_nba_projections_season ON nba_projections (target_season, our_rank);

-- Computed gap between our projection rank and external consensus, regenerated
-- each time cmd/project-nba -consensus-diff runs.
CREATE TABLE IF NOT EXISTS nba_projection_divergences (
    id                    BIGSERIAL PRIMARY KEY,
    espn_id               TEXT NOT NULL REFERENCES nba_players(espn_id),
    season                INT NOT NULL,
    our_rank              INT NOT NULL,
    consensus_rank_median NUMERIC(8,2) NOT NULL,
    source_count          INT NOT NULL,
    rank_delta            NUMERIC(8,2) NOT NULL, -- our_rank - consensus_rank_median
    computed_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (espn_id, season)
);

CREATE INDEX idx_nba_projection_divergences_season ON nba_projection_divergences (season, rank_delta);

-- Backtest telemetry — the NBA analog of nfl_backtest_results, trimmed: no
-- position_group or eval_basis split (NBA ranking is overall and inherently
-- per-game, so neither dimension applies the way it does for NFL).
CREATE TABLE IF NOT EXISTS nba_backtest_results (
    id               BIGSERIAL PRIMARY KEY,
    target_season    INT NOT NULL,
    rank_correlation NUMERIC(6,4),
    rmse             NUMERIC(8,2),
    mae              NUMERIC(8,2),
    player_count     INT,
    config_used      JSONB,
    computed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nba_backtest_results_season ON nba_backtest_results (target_season);
