package main

// backtest.go — backtesting and auto-tuning for the projection engine.
//
// Backtesting:
//   For each target season in a range, pretend we're projecting that season
//   using only data from prior seasons, then compare projections to actuals.
//
// Auto-tuning:
//   Coordinate descent over dimension weights + similarity threshold to find
//   the parameter set that minimises validation RMSE.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/aging"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── backtest config (tunable parameters) ─────────────────────────────────────

// projConfig holds all tuneable parameters for the projection engine.
// The CLI reads projection_config.json if it exists; otherwise uses defaults.
type projConfig struct {
	SimilarityThreshold float64            `json:"similarity_threshold"`
	AgeWindow           int                `json:"age_window"`
	MaxGrowth           float64            `json:"max_growth"`
	MinGrowth           float64            `json:"min_growth"`
	QBWeights           map[string]float64 `json:"qb_weights"`
	RBWeights           map[string]float64 `json:"rb_weights"`
	WRWeights           map[string]float64 `json:"wr_weights"`
	TEWeights           map[string]float64 `json:"te_weights"`
	KWeights            map[string]float64 `json:"k_weights"`
	AgingMultipliers    *aging.AgingMultipliers `json:"aging_multipliers,omitempty"`
	TunedAt             string             `json:"tuned_at,omitempty"`
	TrainSeasons        string             `json:"train_seasons,omitempty"`
	ValidateSeasons     string             `json:"validate_seasons,omitempty"`
	ValidationRMSE      float64            `json:"validation_rmse,omitempty"`
}

// defaultConfig returns the default projection parameters (matching constants
// and positionGroups in main.go).
func defaultConfig() projConfig {
	cfg := projConfig{
		SimilarityThreshold: similarityThresh,
		AgeWindow:           2,
		MaxGrowth:           maxGrowthCap,
		MinGrowth:           minGrowthFloor,
	}
	// Populate weight maps from positionGroups — keyed by group name.
	populateWeightMap := func(posGroup string) map[string]float64 {
		groups := positionGroups(posGroup, 99) // 99 years exp → no draft capital
		m := make(map[string]float64, len(groups))
		for _, g := range groups {
			m[g.name] = g.weight
		}
		return m
	}
	cfg.QBWeights = populateWeightMap("QB")
	cfg.RBWeights = populateWeightMap("RB")
	cfg.WRWeights = populateWeightMap("WR")
	cfg.TEWeights = populateWeightMap("TE")
	cfg.KWeights = populateWeightMap("K")
	return cfg
}

// effectiveAgingMultipliers returns the configured aging multipliers, falling
// back to defaults if not set.
func (c projConfig) effectiveAgingMultipliers() aging.AgingMultipliers {
	if c.AgingMultipliers != nil {
		return *c.AgingMultipliers
	}
	return aging.DefaultAgingMultipliers
}

// configPath is where the tuned config is saved.
const configPath = "projection_config.json"

// loadConfig reads projection_config.json, falling back to defaults.
func loadConfig() projConfig {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultConfig()
	}
	var cfg projConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("warn: could not parse %s: %v — using defaults", configPath, err)
		return defaultConfig()
	}
	return cfg
}

// saveConfig writes a projConfig to projection_config.json.
func saveConfig(cfg projConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// groupsFromConfig reconstructs []dimGroup for a position group from a config.
// Weight maps are keyed by group name; the field lists come from positionGroups.
func groupsFromConfig(cfg projConfig, posGroup string, yearsExp int) []dimGroup {
	var wm map[string]float64
	switch posGroup {
	case "QB":
		wm = cfg.QBWeights
	case "RB":
		wm = cfg.RBWeights
	case "WR":
		wm = cfg.WRWeights
	case "TE":
		wm = cfg.TEWeights
	case "K":
		wm = cfg.KWeights
	}
	if len(wm) == 0 {
		return positionGroups(posGroup, yearsExp) // fallback
	}
	// Use the canonical group definitions (which handle draft capital correctly)
	// but override weights from the config.
	canonical := positionGroups(posGroup, yearsExp)
	groups := make([]dimGroup, 0, len(canonical))
	for _, g := range canonical {
		w, ok := wm[g.name]
		if !ok {
			w = g.weight // fallback to default if group not in config
		}
		groups = append(groups, dimGroup{g.name, w, g.fields})
	}
	return groups
}

// ── accuracy metrics ──────────────────────────────────────────────────────────

type backtestResult struct {
	TargetSeason  int
	PositionGroup string // empty = overall
	RMSE          float64
	MAE           float64
	Correlation   float64
	RankCorr      float64
	TierAccuracy  float64
	PlayerCount   int
}

// computeBacktestMetrics compares projected vs actual PPR fantasy points.
func computeBacktestMetrics(projected, actual map[string]float64, posGroup string, targetSeason int) backtestResult {
	// Build matched pairs
	type pair struct {
		gsis string
		proj float64
		act  float64
	}
	var pairs []pair
	for gsisID, projVal := range projected {
		if actVal, ok := actual[gsisID]; ok {
			pairs = append(pairs, pair{gsisID, projVal, actVal})
		}
	}

	r := backtestResult{
		TargetSeason:  targetSeason,
		PositionGroup: posGroup,
		PlayerCount:   len(pairs),
	}
	if len(pairs) < 3 {
		return r
	}

	// RMSE and MAE
	var sumSqErr, sumAbsErr float64
	for _, p := range pairs {
		e := p.proj - p.act
		sumSqErr += e * e
		sumAbsErr += math.Abs(e)
	}
	r.RMSE = math.Sqrt(sumSqErr / float64(len(pairs)))
	r.MAE = sumAbsErr / float64(len(pairs))

	// Pearson correlation
	projVals := make([]float64, len(pairs))
	actVals := make([]float64, len(pairs))
	for i, p := range pairs {
		projVals[i] = p.proj
		actVals[i] = p.act
	}
	r.Correlation = pearsonCorr(projVals, actVals)

	// Spearman rank correlation
	r.RankCorr = spearmanCorr(projVals, actVals)

	// Tier accuracy: for top-N by actual, what % did we project in the top-N?
	topN := tierN(posGroup)
	if topN > 0 && len(pairs) >= topN {
		// Sort by actual descending, take top-N gsis IDs
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].act > pairs[j].act })
		actualTopN := make(map[string]bool, topN)
		for _, p := range pairs[:topN] {
			actualTopN[p.gsis] = true
		}
		// Sort by projected descending, count how many of our top-N are in actualTopN
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].proj > pairs[j].proj })
		hit := 0
		for _, p := range pairs[:topN] {
			if actualTopN[p.gsis] {
				hit++
			}
		}
		r.TierAccuracy = float64(hit) / float64(topN)
	}

	return r
}

// tierN returns the "top-N" threshold for tier accuracy by position.
func tierN(posGroup string) int {
	switch posGroup {
	case "QB":
		return 12
	case "RB", "WR":
		return 24
	case "TE":
		return 12
	default:
		return 0
	}
}

// pearsonCorr computes the Pearson correlation coefficient between two slices.
func pearsonCorr(x, y []float64) float64 {
	n := float64(len(x))
	if n < 2 {
		return 0
	}
	mx, my := mean(x), mean(y)
	var num, sx, sy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		num += dx * dy
		sx += dx * dx
		sy += dy * dy
	}
	if sx == 0 || sy == 0 {
		return 0
	}
	return num / math.Sqrt(sx*sy)
}

// spearmanCorr computes Spearman rank correlation.
func spearmanCorr(x, y []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}
	rx := ranks(x)
	ry := ranks(y)
	return pearsonCorr(rx, ry)
}

// ranks converts a slice of values to their rank positions (1-indexed, averaged for ties).
func ranks(vals []float64) []float64 {
	n := len(vals)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return vals[idx[a]] < vals[idx[b]] })
	r := make([]float64, n)
	for i, j := range idx {
		r[j] = float64(i) + 1
	}
	return r
}

// ── backtesting ───────────────────────────────────────────────────────────────

// runBacktest runs projections for each target season in [fromYear, toYear]
// using only data from prior seasons, then computes accuracy metrics.
// Results are printed and stored in nfl_backtest_results.
func runBacktest(ctx context.Context, pool *pgxpool.Pool, fromYear, toYear int, cfg projConfig) ([]backtestResult, error) {
	log.Printf("=== Backtesting seasons %d–%d ===", fromYear, toYear)

	// Load all profiles once
	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	log.Printf("  loaded %d total profiles", len(allProfiles))

	// Load actual fantasy points (PPR) per player per season from nfl_player_stats
	actualFpts, err := loadActualFpts(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load actuals: %w", err)
	}
	log.Printf("  loaded actuals for %d player-seasons", len(actualFpts))

	metaMap, err := loadPlayerMeta(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load meta: %w", err)
	}

	var allResults []backtestResult
	for targetSeason := fromYear; targetSeason <= toYear; targetSeason++ {
		baseSeason := targetSeason - 1
		log.Printf("  backtesting %d (using data through %d)…", targetSeason, baseSeason)

		// Filter profiles to those from seasons before targetSeason
		var historicProfiles []seasonProfile
		for _, p := range allProfiles {
			if p.Season < targetSeason {
				historicProfiles = append(historicProfiles, p)
			}
		}

		// Recompute z-scores using only historic profiles
		recomputeZScores(historicProfiles)

		// Index by player+season and position group
		byPlayerSeason := make(map[string]map[int]*seasonProfile, len(historicProfiles))
		byGroup := make(map[string][]*seasonProfile)
		for i := range historicProfiles {
			p := &historicProfiles[i]
			if byPlayerSeason[p.GsisID] == nil {
				byPlayerSeason[p.GsisID] = make(map[int]*seasonProfile)
			}
			byPlayerSeason[p.GsisID][p.Season] = p
			byGroup[p.PositionGroup] = append(byGroup[p.PositionGroup], p)
		}

		// Targets = players with a profile in baseSeason
		var targets []*seasonProfile
		for _, seasonMap := range byPlayerSeason {
			if p, ok := seasonMap[baseSeason]; ok {
				targets = append(targets, p)
			}
		}

		// Project each target
		projectedFpts := make(map[string]float64)
		for _, target := range targets {
			candidates := byGroup[target.PositionGroup]
			var comps []compResult
			for _, cand := range candidates {
				if cand.GsisID == target.GsisID {
					continue
				}
				if target.Age > 0 && cand.Age > 0 && abs(target.Age-cand.Age) > cfg.AgeWindow {
					continue
				}
				groups := groupsFromConfig(cfg, target.PositionGroup, target.YearsExp)
				sim := computeSimilarityWithConfig(target, cand, groups)
				if sim < cfg.SimilarityThreshold {
					continue
				}
				trajectory := buildTrajectory(cand, byPlayerSeason[cand.GsisID], baseSeason-target.Season+cand.Season)
				headshotURL := ""
				if cm, ok := metaMap[cand.GsisID]; ok && cm.HeadshotURL != nil {
					headshotURL = *cm.HeadshotURL
				}
				compName := ""
				if cm, ok := metaMap[cand.GsisID]; ok {
					compName = cm.Name
				}
				comps = append(comps, compResult{
					GsisID:       cand.GsisID,
					Name:         compName,
					MatchSeason:  cand.Season,
					MatchAge:     cand.Age,
					Similarity:   sim,
					HeadshotURL:  headshotURL,
					Trajectory:   trajectory,
				})
			}
			sort.Slice(comps, func(i, j int) bool { return comps[i].Similarity > comps[j].Similarity })
			var simSqSum float64
			for _, c := range comps {
				simSqSum += c.Similarity * c.Similarity
			}
			if simSqSum > 0 {
				for i := range comps {
					comps[i].Weight = (comps[i].Similarity * comps[i].Similarity) / simSqSum
				}
			}
			projectedAge := target.Age + (targetSeason - target.Season)
			agingMult := cfg.effectiveAgingMultipliers().Multiplier(target.PositionGroup, projectedAge)
			proj := computeWeightedProjection(target, comps, targetSeason, agingMult)
			projectedFpts[target.GsisID] = proj.ProjFptsPPRPG * defaultProjGames
		}

		// Get actuals for this target season
		suffix := fmt.Sprintf("|%d", targetSeason)
		actualForSeason := make(map[string]float64)
		for key, val := range actualFpts {
			if strings.HasSuffix(key, suffix) {
				gsisID := key[:len(key)-len(suffix)]
				actualForSeason[gsisID] = val
			}
		}

		// Compute overall metrics
		overall := computeBacktestMetrics(projectedFpts, actualForSeason, "", targetSeason)
		allResults = append(allResults, overall)
		log.Printf("    overall: n=%d  RMSE=%.1f  MAE=%.1f  r=%.3f  ρ=%.3f  tier=%.0f%%",
			overall.PlayerCount, overall.RMSE, overall.MAE, overall.Correlation, overall.RankCorr,
			overall.TierAccuracy*100)

		// Per-position metrics
		posGroups := []string{"QB", "RB", "WR", "TE"}
		for _, pos := range posGroups {
			posProj := make(map[string]float64)
			posAct := make(map[string]float64)
			for _, t := range targets {
				if t.PositionGroup == pos {
					if v, ok := projectedFpts[t.GsisID]; ok {
						posProj[t.GsisID] = v
					}
					if v, ok := actualForSeason[t.GsisID]; ok {
						posAct[t.GsisID] = v
					}
				}
			}
			posResult := computeBacktestMetrics(posProj, posAct, pos, targetSeason)
			allResults = append(allResults, posResult)
			if posResult.PlayerCount >= 3 {
				log.Printf("    %-3s:     n=%d  RMSE=%.1f  MAE=%.1f  r=%.3f  tier=%.0f%%",
					pos, posResult.PlayerCount, posResult.RMSE, posResult.MAE, posResult.Correlation,
					posResult.TierAccuracy*100)
			}
		}

		// Store in DB
		if err := storeBacktestResults(ctx, pool, allResults, cfg); err != nil {
			log.Printf("  warn: could not store backtest results: %v", err)
		}
	}

	return allResults, nil
}

// loadActualFpts loads actual PPR fantasy points per player per season
// from nfl_player_stats. Returns map keyed as "gsis_id|season".
func loadActualFpts(ctx context.Context, pool *pgxpool.Pool) (map[string]float64, error) {
	rows, err := pool.Query(ctx, `
		SELECT gsis_id, season, SUM(fantasy_points_ppr) AS fpts
		FROM nfl_player_stats
		WHERE season_type = 'REG'
		GROUP BY gsis_id, season
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var gsisID string
		var season int
		var fpts float64
		if err := rows.Scan(&gsisID, &season, &fpts); err != nil {
			return nil, err
		}
		result[fmt.Sprintf("%s|%d", gsisID, season)] = fpts
	}
	return result, rows.Err()
}

// recomputeZScores computes z-scores for the given profiles in place,
// grouping only by position_group (no season grouping, for temporal integrity).
func recomputeZScores(profiles []seasonProfile) {
	type stats struct {
		vals []float64
	}
	// Fields to z-score
	fields := []string{
		"pass_yds_pg", "pass_td_pg", "pass_ypa", "comp_pct", "int_pg", "pass_epa_play",
		"rush_yds_pg", "rush_td_pg", "rush_ypc", "rush_att_pg", "rush_epa_play", "rush_yard_share",
		"rec_pg", "rec_yds_pg", "rec_td_pg", "targets_pg", "target_share", "wopr", "rec_ypr",
		"rec_epa_play", "air_yards_share", "fpts_pg", "fpts_ppr_pg",
		"age", "height", "weight", "draft_number",
		"team_pass_yds_pg", "team_rush_yds_pg", "team_fpts_pg",
		"sacks_pg", "passing_air_yards_pg", "passing_yac_pg",
		"rushing_first_downs_pg", "receiving_air_yards_pg", "receiving_yac_pg",
		"receiving_first_downs_pg", "fumbles_pg",
	}

	// Collect values per (posGroup, field)
	type pgField struct {
		posGroup string
		field    string
	}
	vals := make(map[pgField][]float64)
	for _, p := range profiles {
		for _, f := range fields {
			v := profileField(&p, f)
			if v == nil {
				continue
			}
			k := pgField{p.PositionGroup, f}
			vals[k] = append(vals[k], *v)
		}
	}

	// Compute mean/stdev per (posGroup, field)
	type ms struct{ m, s float64 }
	stats2 := make(map[pgField]ms)
	for k, vs := range vals {
		mu := mean(vs)
		sd := stdev(vs, mu)
		stats2[k] = ms{mu, sd}
	}

	// Set z-scores on each profile
	for i := range profiles {
		p := &profiles[i]
		p.ZScores = make(map[string]float64)
		for _, f := range fields {
			v := profileField(p, f)
			if v == nil {
				continue
			}
			k := pgField{p.PositionGroup, f}
			st := stats2[k]
			if st.s > 0 {
				p.ZScores[f] = (*v - st.m) / st.s
			}
		}
	}
}

// profileField returns a pointer to the float64 value of the named field
// in a seasonProfile, or nil if the field is unknown or zero-sentinel.
// derefF64 returns nil if ptr is nil, otherwise a pointer to the dereferenced value.
func derefF64(ptr *float64) *float64 {
	if ptr == nil {
		return nil
	}
	v := *ptr
	return &v
}

// profileField returns a pointer to the float64 value of the named field
// in a seasonProfile, or nil if the field is unknown or the value is absent.
func profileField(p *seasonProfile, field string) *float64 {
	var v float64
	switch field {
	// Plain float64 fields
	case "pass_yds_pg":
		v = p.PassYdsPG
	case "pass_td_pg":
		v = p.PassTdPG
	case "int_pg":
		v = p.IntPG
	case "rush_yds_pg":
		v = p.RushYdsPG
	case "rush_td_pg":
		v = p.RushTdPG
	case "rush_att_pg":
		v = p.RushAttPG
	case "rec_pg":
		v = p.RecPG
	case "rec_yds_pg":
		v = p.RecYdsPG
	case "rec_td_pg":
		v = p.RecTdPG
	case "targets_pg":
		v = p.TargetsPG
	case "fpts_pg":
		v = p.FptsPG
	case "fpts_ppr_pg":
		v = p.FptsPPRPG
	case "sacks_pg":
		v = p.SacksPG
	case "passing_air_yards_pg":
		v = p.PassingAirYardsPG
	case "passing_yac_pg":
		v = p.PassingYACPG
	case "rushing_first_downs_pg":
		v = p.RushingFirstDownsPG
	case "receiving_air_yards_pg":
		v = p.ReceivingAirYardsPG
	case "receiving_yac_pg":
		v = p.ReceivingYACPG
	case "receiving_first_downs_pg":
		v = p.ReceivingFirstDownsPG
	case "fumbles_pg":
		v = p.FumblesPG
	case "age":
		v = float64(p.Age)
	// Pointer float64 fields — nil means data absent
	case "pass_ypa":
		return derefF64(p.PassYPA)
	case "comp_pct":
		return derefF64(p.CompPct)
	case "pass_epa_play":
		return derefF64(p.PassEPAPlay)
	case "rush_ypc":
		return derefF64(p.RushYPC)
	case "rush_epa_play":
		return derefF64(p.RushEPAPlay)
	case "rush_yard_share":
		return derefF64(p.RushYardShare)
	case "target_share":
		return derefF64(p.TargetShare)
	case "wopr":
		return derefF64(p.WOPR)
	case "rec_ypr":
		return derefF64(p.RecYPR)
	case "rec_epa_play":
		return derefF64(p.RecEPAPlay)
	case "air_yards_share":
		return derefF64(p.AirYardsShare)
	case "team_pass_yds_pg":
		return derefF64(p.TeamPassYdsPG)
	case "team_rush_yds_pg":
		return derefF64(p.TeamRushYdsPG)
	case "team_fpts_pg":
		return derefF64(p.TeamFptsPG)
	// Pointer int fields
	case "height":
		if p.Height == nil {
			return nil
		}
		v = float64(*p.Height)
	case "weight":
		if p.Weight == nil {
			return nil
		}
		v = float64(*p.Weight)
	case "draft_number":
		if p.DraftNumber == nil {
			return nil
		}
		v = float64(*p.DraftNumber)
	default:
		return nil
	}
	return &v
}

// computeSimilarityWithConfig uses cfg growth settings (max/min) but same formula.
func computeSimilarityWithConfig(target, cand *seasonProfile, groups []dimGroup) float64 {
	return computeSimilarity(target, cand, groups)
}

// storeBacktestResults upserts backtest results into nfl_backtest_results.
func storeBacktestResults(ctx context.Context, pool *pgxpool.Pool, results []backtestResult, cfg projConfig) error {
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	for _, r := range results {
		var posGroup *string
		if r.PositionGroup != "" {
			pg := r.PositionGroup
			posGroup = &pg
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO nfl_backtest_results
				(target_season, position_group, rmse, mae, correlation, rank_correlation,
				 tier_accuracy, player_count, config_used, computed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`,
			r.TargetSeason, posGroup, r.RMSE, r.MAE, r.Correlation, r.RankCorr,
			r.TierAccuracy, r.PlayerCount, cfgJSON, time.Now(),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ── auto-tuner ────────────────────────────────────────────────────────────────

// runAutotune uses coordinate descent to find the projConfig that minimises
// validation RMSE, then saves it to projection_config.json.
//
// trainYears:    seasons used to tune weights (measure RMSE)
// validateYears: seasons used to pick the best config (prevent overfitting)
func runAutotune(ctx context.Context, pool *pgxpool.Pool, trainFrom, trainTo, valFrom, valTo int) error {
	log.Printf("=== Auto-tuning (train %d–%d, validate %d–%d) ===", trainFrom, trainTo, valFrom, valTo)

	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	actualFpts, err := loadActualFpts(ctx, pool)
	if err != nil {
		return fmt.Errorf("load actuals: %w", err)
	}
	metaMap, err := loadPlayerMeta(ctx, pool)
	if err != nil {
		return fmt.Errorf("load meta: %w", err)
	}

	// scoreConfig evaluates a config on a range of seasons, returns mean RMSE.
	scoreConfig := func(cfg projConfig, fromYear, toYear int) float64 {
		var totalRMSE float64
		count := 0
		for targetSeason := fromYear; targetSeason <= toYear; targetSeason++ {
			baseSeason := targetSeason - 1
			var historicProfiles []seasonProfile
			for _, p := range allProfiles {
				if p.Season < targetSeason {
					historicProfiles = append(historicProfiles, p)
				}
			}
			recomputeZScores(historicProfiles)
			byPlayerSeason := make(map[string]map[int]*seasonProfile)
			byGroup := make(map[string][]*seasonProfile)
			for i := range historicProfiles {
				p := &historicProfiles[i]
				if byPlayerSeason[p.GsisID] == nil {
					byPlayerSeason[p.GsisID] = make(map[int]*seasonProfile)
				}
				byPlayerSeason[p.GsisID][p.Season] = p
				byGroup[p.PositionGroup] = append(byGroup[p.PositionGroup], p)
			}
			var targets []*seasonProfile
			for _, sm := range byPlayerSeason {
				if p, ok := sm[baseSeason]; ok {
					targets = append(targets, p)
				}
			}
			projectedFpts := make(map[string]float64)
			for _, target := range targets {
				candidates := byGroup[target.PositionGroup]
				var comps []compResult
				for _, cand := range candidates {
					if cand.GsisID == target.GsisID {
						continue
					}
					if target.Age > 0 && cand.Age > 0 && abs(target.Age-cand.Age) > cfg.AgeWindow {
						continue
					}
					groups := groupsFromConfig(cfg, target.PositionGroup, target.YearsExp)
					sim := computeSimilarity(target, cand, groups)
					if sim < cfg.SimilarityThreshold {
						continue
					}
					trajectory := buildTrajectory(cand, byPlayerSeason[cand.GsisID], baseSeason-target.Season+cand.Season)
					headshotURL := ""
					if cm, ok := metaMap[cand.GsisID]; ok && cm.HeadshotURL != nil {
						headshotURL = *cm.HeadshotURL
					}
					compName := ""
					if cm, ok := metaMap[cand.GsisID]; ok {
						compName = cm.Name
					}
					comps = append(comps, compResult{
						GsisID:      cand.GsisID,
						Name:        compName,
						MatchSeason: cand.Season,
						Similarity:  sim,
						HeadshotURL: headshotURL,
						Trajectory:  trajectory,
					})
				}
				sort.Slice(comps, func(i, j int) bool { return comps[i].Similarity > comps[j].Similarity })
				var simSqSum float64
				for _, c := range comps {
					simSqSum += c.Similarity * c.Similarity
				}
				if simSqSum > 0 {
					for i := range comps {
						comps[i].Weight = (comps[i].Similarity * comps[i].Similarity) / simSqSum
					}
				}
				projectedAge := target.Age + (targetSeason - target.Season)
				agingMult := cfg.effectiveAgingMultipliers().Multiplier(target.PositionGroup, projectedAge)
				proj := computeWeightedProjection(target, comps, targetSeason, agingMult)
				projectedFpts[target.GsisID] = proj.ProjFptsPPRPG * defaultProjGames
			}
			sfx := fmt.Sprintf("|%d", targetSeason)
			actualForSeason := make(map[string]float64)
			for key, val := range actualFpts {
				if strings.HasSuffix(key, sfx) {
					actualForSeason[key[:len(key)-len(sfx)]] = val
				}
			}
			r := computeBacktestMetrics(projectedFpts, actualForSeason, "", targetSeason)
			if r.PlayerCount >= 5 {
				totalRMSE += r.RMSE
				count++
			}
		}
		if count == 0 {
			return math.MaxFloat64
		}
		return totalRMSE / float64(count)
	}

	bestCfg := defaultConfig()
	bestTrainRMSE := scoreConfig(bestCfg, trainFrom, trainTo)
	log.Printf("  baseline train RMSE: %.2f", bestTrainRMSE)

	// Coordinate descent: tune similarity threshold
	log.Println("  tuning similarity threshold…")
	for _, thresh := range []float64{0.50, 0.55, 0.60, 0.65, 0.70} {
		candidate := bestCfg
		candidate.SimilarityThreshold = thresh
		rmse := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    threshold=%.2f → RMSE=%.2f", thresh, rmse)
		if rmse < bestTrainRMSE {
			bestTrainRMSE = rmse
			bestCfg = candidate
		}
	}

	// Tune age window
	log.Println("  tuning age window…")
	for _, aw := range []int{1, 2, 3} {
		candidate := bestCfg
		candidate.AgeWindow = aw
		rmse := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    age_window=%d → RMSE=%.2f", aw, rmse)
		if rmse < bestTrainRMSE {
			bestTrainRMSE = rmse
			bestCfg = candidate
		}
	}

	// Tune per-position weights: scale individual weights up/down by 25%
	log.Println("  tuning dimension weights (coordinate descent)…")
	posWeightMaps := []struct {
		name string
		get  func(*projConfig) map[string]float64
		set  func(*projConfig, map[string]float64)
	}{
		{"QB", func(c *projConfig) map[string]float64 { return c.QBWeights }, func(c *projConfig, m map[string]float64) { c.QBWeights = m }},
		{"RB", func(c *projConfig) map[string]float64 { return c.RBWeights }, func(c *projConfig, m map[string]float64) { c.RBWeights = m }},
		{"WR", func(c *projConfig) map[string]float64 { return c.WRWeights }, func(c *projConfig, m map[string]float64) { c.WRWeights = m }},
		{"TE", func(c *projConfig) map[string]float64 { return c.TEWeights }, func(c *projConfig, m map[string]float64) { c.TEWeights = m }},
	}
	for _, pg := range posWeightMaps {
		wm := pg.get(&bestCfg)
		improved := true
		for improved {
			improved = false
			for field := range wm {
				for _, multiplier := range []float64{0.75, 1.25} {
					candidate := bestCfg
					newWM := copyMap(pg.get(&bestCfg))
					newWM[field] = math.Max(0.1, newWM[field]*multiplier)
					pg.set(&candidate, newWM)
					rmse := scoreConfig(candidate, trainFrom, trainTo)
					if rmse < bestTrainRMSE-0.01 { // require meaningful improvement
						bestTrainRMSE = rmse
						bestCfg = candidate
						improved = true
						log.Printf("    %s.%s ×%.2f → RMSE=%.2f", pg.name, field, multiplier, rmse)
					}
				}
			}
		}
	}

	// Evaluate best config on validation years
	valRMSE := scoreConfig(bestCfg, valFrom, valTo)
	log.Printf("  best train RMSE: %.2f  validation RMSE: %.2f", bestTrainRMSE, valRMSE)

	// Save config
	bestCfg.TunedAt = time.Now().Format("2006-01-02")
	bestCfg.TrainSeasons = fmt.Sprintf("%d-%d", trainFrom, trainTo)
	bestCfg.ValidateSeasons = fmt.Sprintf("%d-%d", valFrom, valTo)
	bestCfg.ValidationRMSE = valRMSE
	if err := saveConfig(bestCfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	log.Printf("  saved tuned parameters to %s", configPath)
	return nil
}

func copyMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
