ALTER TABLE nfl_projections
    DROP COLUMN IF EXISTS proj_fpts_ppr_stdev,
    DROP COLUMN IF EXISTS proj_fpts_ppr_p10,
    DROP COLUMN IF EXISTS proj_fpts_ppr_p50,
    DROP COLUMN IF EXISTS proj_fpts_ppr_p90;

ALTER TABLE nfl_backtest_results
    DROP COLUMN IF EXISTS eval_basis,
    DROP COLUMN IF EXISTS p10_coverage,
    DROP COLUMN IF EXISTS p90_coverage;
