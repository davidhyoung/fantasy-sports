-- Uncertainty quantification: outcome distribution alongside the point estimate
-- (see docs/stats/uncertainty-quantification.md).
ALTER TABLE nfl_projections
    ADD COLUMN IF NOT EXISTS proj_fpts_ppr_stdev numeric(8,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS proj_fpts_ppr_p10 numeric(8,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS proj_fpts_ppr_p50 numeric(8,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS proj_fpts_ppr_p90 numeric(8,2) NOT NULL DEFAULT 0;

-- Backtests now run on two evaluation bases: season totals and per-game.
ALTER TABLE nfl_backtest_results
    ADD COLUMN IF NOT EXISTS eval_basis text NOT NULL DEFAULT 'total';

-- Quantile calibration: fraction of realized outcomes below P10 / above P90
-- (well-calibrated ≈ 0.10 each). NULL for rows computed before calibration existed.
ALTER TABLE nfl_backtest_results
    ADD COLUMN IF NOT EXISTS p10_coverage numeric(6,4),
    ADD COLUMN IF NOT EXISTS p90_coverage numeric(6,4);
