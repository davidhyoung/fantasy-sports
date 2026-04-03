-- Stores backtesting results for projection accuracy analysis.
CREATE TABLE IF NOT EXISTS nfl_backtest_results (
    id              BIGSERIAL PRIMARY KEY,
    target_season   INT NOT NULL,
    position_group  TEXT,                  -- NULL = overall
    rmse            NUMERIC(8,2),
    mae             NUMERIC(8,2),
    correlation     NUMERIC(6,4),          -- Pearson r
    rank_correlation NUMERIC(6,4),         -- Spearman ρ
    tier_accuracy   NUMERIC(6,4),          -- % of top-N correctly identified
    player_count    INT,
    config_used     JSONB,                 -- snapshot of weights/threshold
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_backtest_season ON nfl_backtest_results (target_season);
