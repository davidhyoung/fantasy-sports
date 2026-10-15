// cmd/project-nba is the NBA analog of cmd/projections: a batch pipeline for
// comp-based projections, external consensus import/diff, and historical
// backtesting. It is deliberately separate from the live, on-demand
// per-player projection in internal/handlers/nba_projections.go (used by the
// player detail panel, left unchanged) — see docs/nba-projection-algorithm.md
// for why the two paths coexist.
//
// Usage:
//
//	go run ./cmd/project-nba -project -season 2026
//	go run ./cmd/project-nba -import-consensus f.json -season 2026
//	go run ./cmd/project-nba -import-darko f.json -season 2026
//	go run ./cmd/project-nba -consensus-diff -season 2026
//	go run ./cmd/project-nba -import-nba-salaries f.json -league-id 58 -season 2025
//	go run ./cmd/project-nba -backtest -from 2015 -to 2025
//	go run ./cmd/project-nba -autotune -from 2015 -to 2025 -train-to 2021
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const configPath = "nba_projection_config.json"

func loadProjConfig() projConfig {
	cfg := defaultProjConfig()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}
	var loaded projConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Printf("  warning: could not parse %s, using defaults: %v", configPath, err)
		return cfg
	}
	// Zero-valued weight fields would collapse the distance formula (divide
	// by zero totalWeight) — fall back to the default on any obviously-unset
	// config rather than silently producing garbage ranks.
	if loaded.WeightStatline <= 0 && loaded.WeightTrajectory <= 0 && loaded.WeightAge <= 0 {
		return cfg
	}
	return loaded
}

func saveProjConfig(cfg projConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func main() {
	doProject := flag.Bool("project", false, "batch-project every eligible player for -season")
	importConsensus := flag.String("import-consensus", "", "path to a consensus rankings JSON file to import")
	doConsensusDiff := flag.Bool("consensus-diff", false, "diff our projections' rank against imported consensus")
	importDarkoPath := flag.String("import-darko", "", "path to a DARKO projections JSON snapshot to import for -season (see darko.go)")
	importSalaries := flag.String("import-nba-salaries", "", "path to a league-scoped historical salary JSON file to import (requires -league-id)")
	leagueID := flag.Int64("league-id", 0, "league id for -import-nba-salaries (salary is league-internal, not external market data)")
	doBacktest := flag.Bool("backtest", false, "backtest the projection engine across -from..-to")
	doAutotune := flag.Bool("autotune", false, "coordinate-descent tune projConfig across -from..-train-to (train) / -train-to..-to (validate)")
	season := flag.Int("season", 2026, "target season for -project / -import-consensus / -consensus-diff")
	fromSeason := flag.Int("from", 2015, "first season for -backtest/-autotune")
	toSeason := flag.Int("to", 2025, "last season for -backtest/-autotune")
	trainTo := flag.Int("train-to", 2021, "last training season for -autotune; the rest of -from..-to is the validation range")
	flag.Parse()

	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	ranAnything := false

	if *doProject {
		ranAnything = true
		log.Printf("projecting NBA players for target season %d…", *season)
		if err := runProject(ctx, pool, *season, loadProjConfig()); err != nil {
			log.Fatalf("project: %v", err)
		}
	}

	if *importConsensus != "" {
		ranAnything = true
		log.Printf("importing NBA consensus rankings from %s…", *importConsensus)
		if err := importConsensusRankings(ctx, pool, *importConsensus, *season); err != nil {
			log.Fatalf("import-consensus: %v", err)
		}
	}

	if *importDarkoPath != "" {
		ranAnything = true
		log.Printf("importing DARKO projections for season %d from %s…", *season, *importDarkoPath)
		if err := importDarko(ctx, pool, *importDarkoPath, *season); err != nil {
			log.Fatalf("import-darko: %v", err)
		}
	}

	if *importSalaries != "" {
		ranAnything = true
		if *leagueID == 0 {
			log.Fatal("-import-nba-salaries requires -league-id")
		}
		log.Printf("importing NBA salaries for league %d, season %d, from %s…", *leagueID, *season, *importSalaries)
		if err := importNBASalaries(ctx, pool, *importSalaries, *leagueID, *season); err != nil {
			log.Fatalf("import-nba-salaries: %v", err)
		}
	}

	if *doConsensusDiff {
		ranAnything = true
		log.Printf("computing NBA consensus divergences for season %d…", *season)
		if err := computeConsensusDivergences(ctx, pool, *season); err != nil {
			log.Fatalf("consensus-diff: %v", err)
		}
	}

	if *doBacktest {
		ranAnything = true
		log.Printf("backtesting NBA projections %d-%d…", *fromSeason, *toSeason)
		if err := runBacktest(ctx, pool, *fromSeason, *toSeason, loadProjConfig()); err != nil {
			log.Fatalf("backtest: %v", err)
		}
	}

	if *doAutotune {
		ranAnything = true
		log.Printf("autotuning NBA projections: train %d-%d, validate %d-%d…", *fromSeason, *trainTo, *trainTo+1, *toSeason)
		if err := runAutotune(ctx, pool, *fromSeason, *trainTo, *trainTo+1, *toSeason); err != nil {
			log.Fatalf("autotune: %v", err)
		}
	}

	if !ranAnything {
		log.Fatal("nothing to do — pass -project, -import-consensus, -import-darko, -consensus-diff, -backtest, and/or -autotune")
	}
	log.Println("done")
}

// runProject batch-projects every eligible player (no season cutoff — "today"
// is the latest real data) and upserts the result plus each player's overall
// z-score fantasy rank into nba_projections.
func runProject(ctx context.Context, pool *pgxpool.Pool, season int, cfg projConfig) error {
	bySeason, err := loadSeasonAggregates(ctx, pool, 0)
	if err != nil {
		return err
	}
	applyRateStatShrinkage(bySeason, cfg.RateStatShrinkageK)
	projections := projectAll(bySeason, season, cfg)

	values := make(map[string]map[string]float64, len(projections))
	for _, p := range projections {
		values[p.espnID] = p.values
	}
	ranks := rankByFantasyValue(values)

	var upserted int
	for _, p := range projections {
		v := p.values
		_, err := pool.Exec(ctx, `
			INSERT INTO nba_projections
				(espn_id, target_season, base_season, points, rebounds, assists, steals, blocks,
				 turnovers, three_pm, fg_pct, ft_pct, comp_count, our_rank, computed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW())
			ON CONFLICT (espn_id, target_season) DO UPDATE SET
				base_season = EXCLUDED.base_season, points = EXCLUDED.points, rebounds = EXCLUDED.rebounds,
				assists = EXCLUDED.assists, steals = EXCLUDED.steals, blocks = EXCLUDED.blocks,
				turnovers = EXCLUDED.turnovers, three_pm = EXCLUDED.three_pm, fg_pct = EXCLUDED.fg_pct,
				ft_pct = EXCLUDED.ft_pct, comp_count = EXCLUDED.comp_count, our_rank = EXCLUDED.our_rank,
				computed_at = NOW()
		`, p.espnID, p.targetSeason, p.baseSeason, v["points"], v["rebounds"], v["assists"], v["steals"], v["blocks"],
			v["turnovers"], v["three_pm"], v["fg_pct"], v["ft_pct"], p.compCount, ranks[p.espnID])
		if err != nil {
			log.Printf("  upsert %s: %v", p.espnID, err)
			continue
		}
		upserted++
	}
	log.Printf("  projected and ranked %d players (base season varies per player)", upserted)
	return nil
}
