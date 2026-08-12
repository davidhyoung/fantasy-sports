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
	SimilarityThreshold float64                 `json:"similarity_threshold"`
	AgeWindow           int                     `json:"age_window"`
	MaxGrowth           float64                 `json:"max_growth"`
	MinGrowth           float64                 `json:"min_growth"`
	QBWeights           map[string]float64      `json:"qb_weights"`
	RBWeights           map[string]float64      `json:"rb_weights"`
	WRWeights           map[string]float64      `json:"wr_weights"`
	TEWeights           map[string]float64      `json:"te_weights"`
	KWeights            map[string]float64      `json:"k_weights"`
	AgingMultipliers    *aging.AgingMultipliers `json:"aging_multipliers,omitempty"`
	TunedAt             string                  `json:"tuned_at,omitempty"`
	TrainSeasons        string                  `json:"train_seasons,omitempty"`
	ValidateSeasons     string                  `json:"validate_seasons,omitempty"`
	ValidationRMSE      float64                 `json:"validation_rmse,omitempty"` // legacy objective (pre rank-corr)
	ObjectiveMetric     string                  `json:"objective_metric,omitempty"`
	ValidationScore     float64                 `json:"validation_score,omitempty"`

	// TargetBlendDecay (docs/stats/recency-weighted-profiles.md) weights the
	// target's immediately preceding season into their base-season profile,
	// discounted by this factor. 0 = exact no-op (today's single-season
	// behavior); must stay in [0, 1) — sanitizeConfig resets bad values.
	TargetBlendDecay float64 `json:"target_blend_decay"`
	// TargetBlendDecayUp is the decay used when the base season came in ABOVE
	// the preceding one — a breakout. Gains are given back far more often than
	// drops are recovered (docs/algorithm-review.md §7.4), so regressing risers
	// harder than decliners is the asymmetry that symmetric blending can't
	// express. 0 = fall back to TargetBlendDecay, i.e. exact today's behavior.
	TargetBlendDecayUp float64 `json:"target_blend_decay_up,omitempty"`
	// GrowthShrinkageK (docs/stats/bayesian-shrinkage.md, growth-rate
	// extension) shrinks the comp-derived growth rate toward an age/position
	// baseline, weighted as if it were a comp with this much combined
	// similarity² weight. 0 = exact no-op; must stay >= 0.
	GrowthShrinkageK float64 `json:"growth_shrinkage_k"`
}

// defaultConfig returns the default projection parameters (matching constants
// and positionGroups in main.go).
func defaultConfig() projConfig {
	cfg := projConfig{
		SimilarityThreshold: similarityThresh,
		AgeWindow:           2,
		MaxGrowth:           maxGrowthCap,
		MinGrowth:           minGrowthFloor,
		TargetBlendDecay:    0.0, // no-op until autotune finds a better value
		GrowthShrinkageK:    0.0, // no-op until autotune finds a better value
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
	return sanitizeConfig(cfg)
}

// sanitizeConfig repairs configs written by older tool versions. Weight maps
// are keyed by dimension-group name ("passing", "value", …); older configs
// keyed them by stat name ("rec_pg", "age", …), which groupsFromConfig would
// silently ignore — falling back to defaults without telling anyone.
func sanitizeConfig(cfg projConfig) projConfig {
	def := defaultConfig()
	fix := func(pos string, got, want map[string]float64) map[string]float64 {
		if len(got) == 0 {
			return want
		}
		for k := range got {
			if _, ok := want[k]; !ok {
				log.Printf("warn: %s weights in %s use a stale per-stat format — using default group weights (re-run -autotune to regenerate)", pos, configPath)
				return want
			}
		}
		return got
	}
	cfg.QBWeights = fix("QB", cfg.QBWeights, def.QBWeights)
	cfg.RBWeights = fix("RB", cfg.RBWeights, def.RBWeights)
	cfg.WRWeights = fix("WR", cfg.WRWeights, def.WRWeights)
	cfg.TEWeights = fix("TE", cfg.TEWeights, def.TEWeights)
	cfg.KWeights = fix("K", cfg.KWeights, def.KWeights)
	if cfg.SimilarityThreshold <= 0 || cfg.SimilarityThreshold >= 1 {
		cfg.SimilarityThreshold = def.SimilarityThreshold
	}
	if cfg.AgeWindow <= 0 {
		cfg.AgeWindow = def.AgeWindow
	}
	if cfg.TargetBlendDecay < 0 || cfg.TargetBlendDecay >= 1 {
		cfg.TargetBlendDecay = def.TargetBlendDecay
	}
	if cfg.TargetBlendDecayUp < 0 || cfg.TargetBlendDecayUp >= 1 {
		cfg.TargetBlendDecayUp = def.TargetBlendDecayUp
	}
	if cfg.GrowthShrinkageK < 0 {
		cfg.GrowthShrinkageK = def.GrowthShrinkageK
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
	EvalBasis     string // "total" (season PPR points) or "per_game"
	RMSE          float64
	MAE           float64
	Correlation   float64
	RankCorr      float64
	TierAccuracy  float64
	PlayerCount   int
	P10Coverage   *float64 // per_game only: fraction of actuals below projected P10 (calibrated ≈ 0.10)
	P90Coverage   *float64 // per_game only: fraction of actuals above projected P90 (calibrated ≈ 0.10)
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

// projOutcome is one player's backtest projection on both evaluation bases,
// plus per-game outcome quantiles for calibration checks.
type projOutcome struct {
	Pos     string
	PerGame float64
	Total   float64
	P10PG   float64
	P90PG   float64
}

// projectSeasonBacktest projects every player with a (targetSeason-1) profile
// using only profiles from seasons < targetSeason (temporal integrity: z-scores
// and shrinkage priors are recomputed from the restricted pool). This is the
// single projection path shared by runBacktest and the autotuner, and mirrors
// the production path in computeProjections.
func projectSeasonBacktest(cfg projConfig, allProfiles []seasonProfile, targetSeason int) map[string]projOutcome {
	baseSeason := targetSeason - 1

	var historicProfiles []seasonProfile
	for _, p := range allProfiles {
		if p.Season < targetSeason {
			historicProfiles = append(historicProfiles, p)
		}
	}
	recomputeZScores(historicProfiles)

	byPlayerSeason := make(map[string]map[int]*seasonProfile, len(historicProfiles))
	byGroup := make(map[string][]*seasonProfile)
	maxDataSeason := 0
	for i := range historicProfiles {
		p := &historicProfiles[i]
		if byPlayerSeason[p.GsisID] == nil {
			byPlayerSeason[p.GsisID] = make(map[int]*seasonProfile)
		}
		byPlayerSeason[p.GsisID][p.Season] = p
		byGroup[p.PositionGroup] = append(byGroup[p.PositionGroup], p)
		if p.Season > maxDataSeason {
			maxDataSeason = p.Season
		}
	}
	groupMeans := computeGroupMeans(byGroup)
	growthBaselines := computeGrowthBaselines(historicProfiles)

	var targets []*seasonProfile
	for _, seasonMap := range byPlayerSeason {
		if _, ok := seasonMap[baseSeason]; ok {
			targets = append(targets, blendTargetProfile(seasonMap, baseSeason,
				effectiveBlendDecay(seasonMap, baseSeason, cfg.TargetBlendDecay, cfg.TargetBlendDecayUp)))
		}
	}

	out := make(map[string]projOutcome, len(targets))
	for _, target := range targets {
		candidates := byGroup[target.PositionGroup]
		groups := groupsFromConfig(cfg, target.PositionGroup, target.YearsExp)
		var comps []compResult
		for _, cand := range candidates {
			if cand.GsisID == target.GsisID {
				continue
			}
			if target.Age > 0 && cand.Age > 0 && abs(target.Age-cand.Age) > cfg.AgeWindow {
				continue
			}
			sim := computeSimilarity(target, cand, groups)
			if sim < cfg.SimilarityThreshold {
				continue
			}
			candMatchSeason := baseSeason - target.Season + cand.Season
			trajectory := buildTrajectory(cand, byPlayerSeason[cand.GsisID], candMatchSeason)
			comps = append(comps, compResult{
				GsisID:      cand.GsisID,
				MatchSeason: cand.Season,
				MatchAge:    cand.Age,
				Similarity:  sim,
				WashedOut:   len(trajectory) == 0 && candMatchSeason+1 <= maxDataSeason,
				// MatchProfile is required by getGrowth — without it every
				// comp's growth silently defaults to 1.0 and the backtest
				// measures a "repeat last season" baseline instead of the
				// comp engine (this was a real bug).
				MatchProfile: map[string]float64{
					"fpts_pg":     cand.FptsPG,
					"fpts_ppr_pg": cand.FptsPPRPG,
				},
				Trajectory: trajectory,
			})
		}
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
		proj := computeWeightedProjection(target, comps, targetSeason, agingMult, groupMeans[target.PositionGroup], cfg.GrowthShrinkageK, growthBaselines)
		out[target.GsisID] = projOutcome{
			Pos:     target.PositionGroup,
			PerGame: proj.ProjFptsPPRPG,
			Total:   proj.ProjFptsPPRPG * defaultProjGames,
			P10PG:   proj.ProjFptsPPRP10, // per-game at this point (totals conversion happens in computeProjections)
			P90PG:   proj.ProjFptsPPRP90,
		}
	}
	return out
}

// actualSeason holds a player's realized season: total PPR points and games played.
type actualSeason struct {
	Total float64
	Games int
}

// minEvalGames is the games floor for per-game evaluation — fewer games make
// the per-game rate too noisy to judge a projection against.
const minEvalGames = 4

// loadActuals loads actual PPR fantasy points and games played per player per
// season from nfl_player_stats. Returns map keyed as "gsis_id|season".
// Games counts use the same activity definition as profile building.
func loadActuals(ctx context.Context, pool *pgxpool.Pool) (map[string]actualSeason, error) {
	rows, err := pool.Query(ctx, `
		SELECT gsis_id, season, SUM(fantasy_points_ppr) AS fpts,
		       COUNT(*) FILTER (WHERE (passing_yards + rushing_yards + receiving_yards + targets + fg_made) > 0) AS games
		FROM nfl_player_stats
		WHERE season_type = 'REG'
		GROUP BY gsis_id, season
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]actualSeason)
	for rows.Next() {
		var gsisID string
		var season, games int
		var fpts float64
		if err := rows.Scan(&gsisID, &season, &fpts, &games); err != nil {
			return nil, err
		}
		result[fmt.Sprintf("%s|%d", gsisID, season)] = actualSeason{Total: fpts, Games: games}
	}
	return result, rows.Err()
}

// actualsForSeason extracts gsis_id → actualSeason for one target season.
func actualsForSeason(actuals map[string]actualSeason, targetSeason int) map[string]actualSeason {
	suffix := fmt.Sprintf("|%d", targetSeason)
	out := make(map[string]actualSeason)
	for key, val := range actuals {
		if strings.HasSuffix(key, suffix) {
			out[key[:len(key)-len(suffix)]] = val
		}
	}
	return out
}

// evaluateSeason computes metrics for one target season on both evaluation
// bases ("total" and "per_game"), overall and per position group, including
// quantile calibration coverage on the per_game basis.
func evaluateSeason(outcomes map[string]projOutcome, actuals map[string]actualSeason, targetSeason int) []backtestResult {
	bases := []string{"total", "per_game"}
	posSets := []string{"", "QB", "RB", "WR", "TE"}

	var results []backtestResult
	for _, basis := range bases {
		for _, pos := range posSets {
			proj := make(map[string]float64)
			act := make(map[string]float64)
			for gsis, o := range outcomes {
				if pos != "" && o.Pos != pos {
					continue
				}
				a, ok := actuals[gsis]
				if !ok {
					continue
				}
				if basis == "per_game" {
					if a.Games < minEvalGames {
						continue
					}
					proj[gsis] = o.PerGame
					act[gsis] = a.Total / float64(a.Games)
				} else {
					proj[gsis] = o.Total
					act[gsis] = a.Total
				}
			}
			r := computeBacktestMetrics(proj, act, pos, targetSeason)
			r.EvalBasis = basis

			// Quantile calibration: only meaningful on the per-game basis,
			// and only for players whose comp distribution is non-degenerate.
			if basis == "per_game" {
				var below, above, n int
				for gsis := range proj {
					o := outcomes[gsis]
					if o.P90PG <= o.P10PG {
						continue
					}
					n++
					if act[gsis] < o.P10PG {
						below++
					}
					if act[gsis] > o.P90PG {
						above++
					}
				}
				if n >= 10 {
					p10 := float64(below) / float64(n)
					p90 := float64(above) / float64(n)
					r.P10Coverage = &p10
					r.P90Coverage = &p90
				}
			}
			results = append(results, r)
		}
	}
	return results
}

// runBacktest runs projections for each target season in [fromYear, toYear]
// using only data from prior seasons, then computes accuracy metrics.
// Results are printed and stored in nfl_backtest_results.
func runBacktest(ctx context.Context, pool *pgxpool.Pool, fromYear, toYear int, cfg projConfig) ([]backtestResult, error) {
	log.Printf("=== Backtesting seasons %d–%d ===", fromYear, toYear)

	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	log.Printf("  loaded %d total profiles", len(allProfiles))

	actuals, err := loadActuals(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load actuals: %w", err)
	}
	log.Printf("  loaded actuals for %d player-seasons", len(actuals))

	var allResults []backtestResult
	for targetSeason := fromYear; targetSeason <= toYear; targetSeason++ {
		log.Printf("  backtesting %d (using data through %d)…", targetSeason, targetSeason-1)
		outcomes := projectSeasonBacktest(cfg, allProfiles, targetSeason)
		seasonResults := evaluateSeason(outcomes, actualsForSeason(actuals, targetSeason), targetSeason)
		allResults = append(allResults, seasonResults...)

		for _, r := range seasonResults {
			if r.PlayerCount < 3 {
				continue
			}
			label := r.PositionGroup
			if label == "" {
				label = "ALL"
			}
			cal := ""
			if r.P10Coverage != nil && r.P90Coverage != nil {
				cal = fmt.Sprintf("  cal=[%.0f%%|%.0f%%]", *r.P10Coverage*100, *r.P90Coverage*100)
			}
			log.Printf("    %-8s %-3s n=%-3d RMSE=%.1f  MAE=%.1f  r=%.3f  ρ=%.3f  tier=%.0f%%%s",
				r.EvalBasis, label, r.PlayerCount, r.RMSE, r.MAE, r.Correlation, r.RankCorr,
				r.TierAccuracy*100, cal)
		}
	}

	// Store once, after the loop — storing inside the loop with the
	// accumulated slice previously re-inserted every prior season's rows.
	if err := storeBacktestResults(ctx, pool, allResults, cfg); err != nil {
		log.Printf("  warn: could not store backtest results: %v", err)
	}

	return allResults, nil
}

// recomputeZScores computes z-scores for the given profiles in place,
// grouping only by position_group (no season grouping, for temporal integrity).
// Small-sample rates are shrunk toward the group mean first; shrinkage priors
// come from the same (historical-only) pool, so no future data leaks in.
func recomputeZScores(profiles []seasonProfile) {
	// Fields to z-score
	fields := []string{
		"pass_att_pg", "pass_yds_pg", "pass_td_pg", "pass_ypa", "comp_pct", "int_pg", "pass_epa_play",
		"rush_yds_pg", "rush_td_pg", "rush_ypc", "rush_att_pg", "rush_epa_play", "rush_yard_share",
		"rec_pg", "rec_yds_pg", "rec_td_pg", "targets_pg", "target_share", "wopr", "rec_ypr",
		"rec_epa_play", "air_yards_share", "fpts_pg", "fpts_ppr_pg",
		"fg_made_pg", "pat_made_pg", "fg_pct",
		"age", "height", "weight", "draft_number",
		"team_pass_yds_pg", "team_rush_yds_pg", "team_fpts_pg",
		"sacks_pg", "passing_air_yards_pg", "passing_yac_pg",
		"rushing_first_downs_pg", "receiving_air_yards_pg", "receiving_yac_pg",
		"receiving_first_downs_pg", "fumbles_pg",
	}

	// Materialise per-profile value maps so shrinkage can adjust rates in place.
	valsList := make([]map[string]float64, len(profiles))
	games := make([]float64, len(profiles))
	posGroups := make([]string, len(profiles))
	for i := range profiles {
		p := &profiles[i]
		m := make(map[string]float64, len(fields))
		for _, f := range fields {
			if v := profileField(p, f); v != nil {
				m[f] = *v
			}
		}
		valsList[i] = m
		games[i] = float64(p.GamesPlayed)
		posGroups[i] = p.PositionGroup
	}
	applyShrinkage(valsList, games, posGroups)

	// Collect values per (posGroup, field)
	type pgField struct {
		posGroup string
		field    string
	}
	vals := make(map[pgField][]float64)
	for i := range profiles {
		for _, f := range fields {
			v, ok := valsList[i][f]
			if !ok {
				continue
			}
			k := pgField{posGroups[i], f}
			vals[k] = append(vals[k], v)
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
			v, ok := valsList[i][f]
			if !ok {
				continue
			}
			k := pgField{p.PositionGroup, f}
			st := stats2[k]
			if st.s > 0 {
				p.ZScores[f] = (v - st.m) / st.s
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
	case "pass_att_pg":
		v = p.PassAttPG
	case "fg_made_pg":
		v = p.FgMadePG
	case "pat_made_pg":
		v = p.PatMadePG
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
	case "fg_pct":
		return derefF64(p.FgPct)
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

// storeBacktestResults inserts backtest results into nfl_backtest_results.
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
				(target_season, position_group, eval_basis, rmse, mae, correlation, rank_correlation,
				 tier_accuracy, player_count, p10_coverage, p90_coverage, config_used, computed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		`,
			r.TargetSeason, posGroup, r.EvalBasis, r.RMSE, r.MAE, r.Correlation, r.RankCorr,
			r.TierAccuracy, r.PlayerCount, r.P10Coverage, r.P90Coverage, cfgJSON, time.Now(),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ── auto-tuner ────────────────────────────────────────────────────────────────

// runAutotune uses coordinate ascent to find the projConfig that maximises
// per-game Spearman rank correlation (the draft-prep objective: getting the
// player *order* right matters more than exact point totals), then saves the
// winner to projection_config.json.
//
// trainYears:    seasons used to tune parameters
// validateYears: held-out seasons; the tuned config must beat the default
//
//	config here or the default is kept (overfitting guard)
func runAutotune(ctx context.Context, pool *pgxpool.Pool, trainFrom, trainTo, valFrom, valTo int) error {
	log.Printf("=== Auto-tuning (train %d–%d, validate %d–%d, objective: per-game rank correlation) ===",
		trainFrom, trainTo, valFrom, valTo)

	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	actuals, err := loadActuals(ctx, pool)
	if err != nil {
		return fmt.Errorf("load actuals: %w", err)
	}

	// scoreConfig returns the mean per-game Spearman rank correlation across
	// seasons (higher = better).
	scoreConfig := func(cfg projConfig, fromYear, toYear int) float64 {
		var total float64
		count := 0
		for targetSeason := fromYear; targetSeason <= toYear; targetSeason++ {
			outcomes := projectSeasonBacktest(cfg, allProfiles, targetSeason)
			seasonActs := actualsForSeason(actuals, targetSeason)
			proj := make(map[string]float64)
			act := make(map[string]float64)
			for gsis, o := range outcomes {
				a, ok := seasonActs[gsis]
				if !ok || a.Games < minEvalGames {
					continue
				}
				proj[gsis] = o.PerGame
				act[gsis] = a.Total / float64(a.Games)
			}
			r := computeBacktestMetrics(proj, act, "", targetSeason)
			if r.PlayerCount >= 5 {
				total += r.RankCorr
				count++
			}
		}
		if count == 0 {
			return -1
		}
		return total / float64(count)
	}

	// Minimum improvement to accept a coordinate move — guards against
	// chasing noise in the objective.
	const minGain = 0.002

	bestCfg := defaultConfig()
	bestTrainScore := scoreConfig(bestCfg, trainFrom, trainTo)
	log.Printf("  baseline train ρ: %.4f", bestTrainScore)

	// Coordinate ascent: tune similarity threshold
	log.Println("  tuning similarity threshold…")
	for _, thresh := range []float64{0.50, 0.55, 0.60, 0.65, 0.70} {
		candidate := bestCfg
		candidate.SimilarityThreshold = thresh
		score := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    threshold=%.2f → ρ=%.4f", thresh, score)
		if score > bestTrainScore+minGain {
			bestTrainScore = score
			bestCfg = candidate
		}
	}

	// Tune age window
	log.Println("  tuning age window…")
	for _, aw := range []int{1, 2, 3} {
		candidate := bestCfg
		candidate.AgeWindow = aw
		score := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    age_window=%d → ρ=%.4f", aw, score)
		if score > bestTrainScore+minGain {
			bestTrainScore = score
			bestCfg = candidate
		}
	}

	// Tune target-blend decay (docs/stats/recency-weighted-profiles.md)
	log.Println("  tuning target-blend decay…")
	for _, decay := range []float64{0.0, 0.25, 0.5, 0.75} {
		candidate := bestCfg
		candidate.TargetBlendDecay = decay
		score := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    target_blend_decay=%.2f → ρ=%.4f", decay, score)
		if score > bestTrainScore+minGain {
			bestTrainScore = score
			bestCfg = candidate
		}
	}

	// Tune growth-shrinkage k (docs/stats/bayesian-shrinkage.md, growth-rate extension)
	log.Println("  tuning growth-shrinkage k…")
	for _, k := range []float64{0.0, 5.0, 10.0, 20.0} {
		candidate := bestCfg
		candidate.GrowthShrinkageK = k
		score := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    growth_shrinkage_k=%.1f → ρ=%.4f", k, score)
		if score > bestTrainScore+minGain {
			bestTrainScore = score
			bestCfg = candidate
		}
	}

	// Tune aging multipliers
	log.Println("  tuning aging multipliers…")
	agingCandidates := []aging.AgingMultipliers{
		{Developing: 1.00, Prime: 1.00, PostPrime: 1.00, LateCareer: 1.00}, // off
		{Developing: 1.02, Prime: 1.00, PostPrime: 0.97, LateCareer: 0.93}, // default
		{Developing: 1.05, Prime: 1.00, PostPrime: 0.95, LateCareer: 0.88}, // aggressive
	}
	for _, am := range agingCandidates {
		am := am
		candidate := bestCfg
		candidate.AgingMultipliers = &am
		score := scoreConfig(candidate, trainFrom, trainTo)
		log.Printf("    aging={%.2f %.2f %.2f %.2f} → ρ=%.4f", am.Developing, am.Prime, am.PostPrime, am.LateCareer, score)
		if score > bestTrainScore+minGain {
			bestTrainScore = score
			bestCfg = candidate
		}
	}

	// Tune per-position group weights: scale individual weights up/down by 25%
	log.Println("  tuning dimension-group weights (coordinate ascent)…")
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
		// Deterministic field order (map iteration is randomised in Go).
		fieldNames := make([]string, 0)
		for field := range pg.get(&bestCfg) {
			fieldNames = append(fieldNames, field)
		}
		sort.Strings(fieldNames)

		improved := true
		for improved {
			improved = false
			for _, field := range fieldNames {
				for _, multiplier := range []float64{0.75, 1.25} {
					candidate := bestCfg
					newWM := copyMap(pg.get(&bestCfg))
					newWM[field] = math.Max(0.1, newWM[field]*multiplier)
					pg.set(&candidate, newWM)
					score := scoreConfig(candidate, trainFrom, trainTo)
					if score > bestTrainScore+minGain {
						bestTrainScore = score
						bestCfg = candidate
						improved = true
						log.Printf("    %s.%s ×%.2f → ρ=%.4f", pg.name, field, multiplier, score)
					}
				}
			}
		}
	}

	// Overfitting guard: the tuned config must beat the default on held-out
	// validation seasons, otherwise keep the default.
	defaultVal := scoreConfig(defaultConfig(), valFrom, valTo)
	tunedVal := scoreConfig(bestCfg, valFrom, valTo)
	log.Printf("  validation ρ: default=%.4f  tuned=%.4f", defaultVal, tunedVal)
	if tunedVal < defaultVal {
		log.Printf("  tuned config does NOT beat default on validation — keeping defaults")
		bestCfg = defaultConfig()
		tunedVal = defaultVal
	}

	bestCfg.TunedAt = time.Now().Format("2006-01-02")
	bestCfg.TrainSeasons = fmt.Sprintf("%d-%d", trainFrom, trainTo)
	bestCfg.ValidateSeasons = fmt.Sprintf("%d-%d", valFrom, valTo)
	bestCfg.ObjectiveMetric = "per_game_rank_correlation"
	bestCfg.ValidationScore = tunedVal
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
