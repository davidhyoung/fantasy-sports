package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"

	"strings"

	"github.com/davidyoung/fantasy-sports/backend/internal/aging"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── grade sub-score configuration ────────────────────────────────────────────

// gradeWeights holds the relative weight for each sub-score per position group.
type gradeWeights struct {
	Production float64
	Efficiency float64
	Usage      float64
	Durability float64
}

var positionGradeWeights = map[string]gradeWeights{
	"QB": {Production: 0.30, Efficiency: 0.35, Usage: 0.20, Durability: 0.15},
	"RB": {Production: 0.25, Efficiency: 0.25, Usage: 0.25, Durability: 0.25},
	"WR": {Production: 0.25, Efficiency: 0.30, Usage: 0.30, Durability: 0.15},
	"TE": {Production: 0.30, Efficiency: 0.30, Usage: 0.25, Durability: 0.15},
	"K":  {Production: 0.40, Efficiency: 0.40, Usage: 0.05, Durability: 0.15},
}

var defaultGradeWeights = gradeWeights{
	Production: 0.30, Efficiency: 0.30, Usage: 0.25, Durability: 0.15,
}

// gradeDimConfig defines the z-score fields contributing to each sub-score.
// Fields prefixed with "-" are inverted (lower is better, e.g. interceptions).
type gradeDimConfig struct {
	Production []string
	Efficiency []string
	Usage      []string
}

var positionGradeDims = map[string]gradeDimConfig{
	"QB": {
		Production: []string{"pass_yds_pg", "pass_td_pg", "rush_yds_pg"},
		Efficiency: []string{"pass_epa_play", "comp_pct", "pass_ypa", "-int_pg", "-sacks_pg"},
		Usage:      []string{"pass_yds_pg"},
	},
	"RB": {
		Production: []string{"rush_yds_pg", "rush_td_pg", "rec_yds_pg", "rec_td_pg"},
		Efficiency: []string{"rush_epa_play", "rush_ypc", "rec_epa_play", "receiving_yac_pg"},
		Usage:      []string{"rush_yard_share", "target_share", "targets_pg"},
	},
	"WR": {
		Production: []string{"rec_yds_pg", "rec_td_pg", "rush_yds_pg"},
		Efficiency: []string{"rec_epa_play", "rec_ypr", "receiving_yac_pg", "receiving_air_yards_pg"},
		Usage:      []string{"target_share", "wopr", "targets_pg"},
	},
	"TE": {
		Production: []string{"rec_yds_pg", "rec_td_pg"},
		Efficiency: []string{"rec_epa_play", "rec_ypr", "receiving_yac_pg"},
		Usage:      []string{"target_share", "targets_pg"},
	},
	"K": {
		Production: []string{"fg_made_pg", "pat_made_pg", "fpts_pg"},
		Efficiency: []string{"fg_pct"},
		Usage:      []string{},
	},
}

// ── grade result ─────────────────────────────────────────────────────────────

type playerGrade struct {
	GsisID        string
	Season        int
	PositionGroup string
	Overall       float64
	Production    float64
	Efficiency    float64
	Usage         float64
	Durability    float64
	CareerPhase   string
	YoYTrend      *float64
	DimDetails    map[string]float64
}

type gradeRawScores struct {
	production float64
	efficiency float64
	usage      float64
	durability float64
	weighted   float64
}

// ── compute grades ───────────────────────────────────────────────────────────

func computeGrades(ctx context.Context, pool *pgxpool.Pool) error {
	log.Println("=== Computing player grades ===")

	// 1. Load all season profiles
	log.Println("  loading season profiles…")
	profiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	log.Printf("  loaded %d profiles", len(profiles))

	// 2. Group profiles by position_group (across ALL seasons)
	byPos := make(map[string][]*seasonProfile)
	for i := range profiles {
		p := &profiles[i]
		byPos[p.PositionGroup] = append(byPos[p.PositionGroup], p)
	}

	// 3. Compute grades with cross-season percentiles per position group
	log.Println("  computing grades (cross-season percentiles)…")
	var allGrades []playerGrade

	for pos, posProfiles := range byPos {
		dims, ok := positionGradeDims[pos]
		if !ok {
			continue
		}
		weights := positionGradeWeights[pos]

		grades := computePositionGrades(posProfiles, pos, dims, weights)
		allGrades = append(allGrades, grades...)
	}

	// 4. Index grades and compute YoY trends
	byPlayerSeason := make(map[string]map[int]*playerGrade)
	for i := range allGrades {
		g := &allGrades[i]
		if byPlayerSeason[g.GsisID] == nil {
			byPlayerSeason[g.GsisID] = make(map[int]*playerGrade)
		}
		byPlayerSeason[g.GsisID][g.Season] = g
	}

	for i := range allGrades {
		g := &allGrades[i]
		if prev, ok := byPlayerSeason[g.GsisID][g.Season-1]; ok {
			trend := (g.Overall - prev.Overall) / 100.0
			if trend > 1.0 {
				trend = 1.0
			} else if trend < -1.0 {
				trend = -1.0
			}
			g.YoYTrend = &trend
		}
	}

	// 5. Upsert into nfl_player_grades
	log.Printf("  writing %d grades to database…", len(allGrades))
	if err := upsertGrades(ctx, pool, allGrades); err != nil {
		return fmt.Errorf("upsert grades: %w", err)
	}

	log.Println("  done!")
	return nil
}

// computePositionGrades computes grades for all player-seasons in a position
// group. Percentiles are computed across ALL seasons so that 100 = best ever,
// not just best-of-year. The best player in any given season typically lands
// in the 90s.
func computePositionGrades(
	posProfiles []*seasonProfile,
	pos string,
	dims gradeDimConfig,
	weights gradeWeights,
) []playerGrade {
	if len(posProfiles) == 0 {
		return nil
	}

	// Phase 1: compute raw sub-scores for every player-season
	scores := make([]gradeRawScores, len(posProfiles))
	for i, p := range posProfiles {
		scores[i].production = avgZDims(p.ZScores, dims.Production)
		scores[i].efficiency = avgZDims(p.ZScores, dims.Efficiency)
		scores[i].usage = avgZDims(p.ZScores, dims.Usage)
		scores[i].durability = durabilityScore(p.GamesPlayed)
	}

	// Phase 2: convert to percentile across ALL seasons for this position
	prodVals := extractScores(scores, func(s gradeRawScores) float64 { return s.production })
	effVals := extractScores(scores, func(s gradeRawScores) float64 { return s.efficiency })
	useVals := extractScores(scores, func(s gradeRawScores) float64 { return s.usage })
	durVals := extractScores(scores, func(s gradeRawScores) float64 { return s.durability })

	for i := range scores {
		scores[i].production = empiricalPercentile(prodVals, scores[i].production)
		scores[i].efficiency = empiricalPercentile(effVals, scores[i].efficiency)
		scores[i].usage = empiricalPercentile(useVals, scores[i].usage)
		scores[i].durability = empiricalPercentile(durVals, scores[i].durability)

		scores[i].weighted = weights.Production*scores[i].production +
			weights.Efficiency*scores[i].efficiency +
			weights.Usage*scores[i].usage +
			weights.Durability*scores[i].durability
	}

	// Phase 3: overall percentile across ALL seasons
	overallVals := extractScores(scores, func(s gradeRawScores) float64 { return s.weighted })

	grades := make([]playerGrade, len(posProfiles))
	for i, p := range posProfiles {
		overall := empiricalPercentile(overallVals, scores[i].weighted)

		dimDetails := map[string]float64{
			"production_pctile": scores[i].production,
			"efficiency_pctile": scores[i].efficiency,
			"usage_pctile":      scores[i].usage,
			"durability_pctile": scores[i].durability,
		}
		if p.ZScores != nil {
			for _, d := range dims.Production {
				key := d
				if key[0] == '-' {
					key = key[1:]
				}
				if v, ok := p.ZScores[key]; ok {
					dimDetails["z_"+key] = v
				}
			}
			for _, d := range dims.Efficiency {
				key := d
				if key[0] == '-' {
					key = key[1:]
				}
				if v, ok := p.ZScores[key]; ok {
					dimDetails["z_"+key] = v
				}
			}
			for _, d := range dims.Usage {
				key := d
				if key[0] == '-' {
					key = key[1:]
				}
				if v, ok := p.ZScores[key]; ok {
					dimDetails["z_"+key] = v
				}
			}
		}

		grades[i] = playerGrade{
			GsisID:        p.GsisID,
			Season:        p.Season,
			PositionGroup: pos,
			Overall:       math.Round(overall*100) / 100,
			Production:    math.Round(scores[i].production*100) / 100,
			Efficiency:    math.Round(scores[i].efficiency*100) / 100,
			Usage:         math.Round(scores[i].usage*100) / 100,
			Durability:    math.Round(scores[i].durability*100) / 100,
			CareerPhase:   aging.Phase(pos, p.Age),
			DimDetails:    dimDetails,
		}
	}

	return grades
}

// avgZDims computes the average z-score across the given dimension keys.
// Keys prefixed with "-" are negated (lower raw value = better).
func avgZDims(zScores map[string]float64, dims []string) float64 {
	if len(dims) == 0 {
		return 0
	}
	var sum float64
	var count int
	for _, d := range dims {
		invert := false
		key := d
		if len(d) > 0 && d[0] == '-' {
			invert = true
			key = d[1:]
		}
		v, ok := zScores[key]
		if !ok {
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if invert {
			v = -v
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// durabilityScore converts games played into a 0-1 scale.
// 17 games = 1.0, scales linearly.
func durabilityScore(gamesPlayed int) float64 {
	if gamesPlayed <= 0 {
		return 0
	}
	d := float64(gamesPlayed) / 17.0
	if d > 1.0 {
		d = 1.0
	}
	return d
}

// empiricalPercentile returns the percentile (0–99) of value within the sorted slice.
// Uses the Hazen formula ((below + 0.5) / n) so that neither 0 nor 100 is reachable,
// then applies a soft ceiling above 90 so that 100 ≈ unreachable "best ever" territory.
// The best player in any given season typically lands in the low-to-mid 90s.
func empiricalPercentile(allValues []float64, value float64) float64 {
	n := len(allValues)
	if n <= 1 {
		return 50.0
	}
	below := 0
	for _, v := range allValues {
		if v < value {
			below++
		}
	}
	raw := (float64(below) + 0.5) / float64(n) * 100.0
	return compressTop(raw)
}

// compressTop applies diminishing returns above the knee (90) so that reaching
// 100 is practically impossible. Below the knee, values pass through unchanged.
//
// Above 90: grade = 90 + 10 * (1 - e^(-k * (raw - 90)))
// where k controls how quickly it approaches the ceiling.
// With k ≈ 0.18, raw 95 → ~93.2, raw 98 → ~95.5, raw 99.5 → ~96.3.
func compressTop(raw float64) float64 {
	const knee = 90.0
	const ceiling = 10.0 // room above knee
	const k = 0.18
	if raw <= knee {
		return raw
	}
	excess := raw - knee
	compressed := ceiling * (1 - math.Exp(-k*excess))
	return knee + compressed
}

func extractScores(scores []gradeRawScores, fn func(gradeRawScores) float64) []float64 {
	vals := make([]float64, len(scores))
	for i, s := range scores {
		vals[i] = fn(s)
	}
	sort.Float64s(vals)
	return vals
}

// ── database upsert ──────────────────────────────────────────────────────────

func upsertGrades(ctx context.Context, pool *pgxpool.Pool, grades []playerGrade) error {
	const batchSize = 500
	for i := 0; i < len(grades); i += batchSize {
		end := i + batchSize
		if end > len(grades) {
			end = len(grades)
		}
		batch := grades[i:end]

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		for _, g := range batch {
			dimJSON, _ := json.Marshal(g.DimDetails)
			_, err := tx.Exec(ctx, `
				INSERT INTO nfl_player_grades (
					gsis_id, season, position_group,
					overall, production, efficiency, usage, durability,
					career_phase, yoy_trend,
					dimension_details, computed_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
				ON CONFLICT (gsis_id, season) DO UPDATE SET
					position_group    = EXCLUDED.position_group,
					overall           = EXCLUDED.overall,
					production        = EXCLUDED.production,
					efficiency        = EXCLUDED.efficiency,
					usage             = EXCLUDED.usage,
					durability        = EXCLUDED.durability,
					career_phase      = EXCLUDED.career_phase,
					yoy_trend         = EXCLUDED.yoy_trend,
					dimension_details = EXCLUDED.dimension_details,
					computed_at       = EXCLUDED.computed_at
			`,
				g.GsisID, g.Season, g.PositionGroup,
				g.Overall, g.Production, g.Efficiency, g.Usage, g.Durability,
				g.CareerPhase, g.YoYTrend,
				dimJSON,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("upsert grade for %s season %d: %w", g.GsisID, g.Season, err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit batch: %w", err)
		}
	}
	return nil
}

// ── grade z-score enrichment ─────────────────────────────────────────────────

// enrichGradeZScores reads overall grades from nfl_player_grades, computes
// z-scores within each position group (globally across all seasons), then
// merges "overall_grade_z" into the z_scores JSONB of matching season profiles.
// This makes grade available as a similarity dimension for comp matching.
func enrichGradeZScores(ctx context.Context, pool *pgxpool.Pool) error {
	// 1. Load all grades keyed by (gsis_id, season).
	type gradeKey struct {
		gsisID string
		season int
	}
	type gradeEntry struct {
		posGroup string
		overall  float64
	}

	gradeRows, err := pool.Query(ctx,
		`SELECT gsis_id, season, position_group, overall FROM nfl_player_grades`)
	if err != nil {
		return fmt.Errorf("load grades: %w", err)
	}
	defer gradeRows.Close()

	gradeMap := make(map[gradeKey]gradeEntry)
	posByGroup := make(map[string][]gradeKey) // posGroup → keys
	for gradeRows.Next() {
		var k gradeKey
		var e gradeEntry
		if err := gradeRows.Scan(&k.gsisID, &k.season, &e.posGroup, &e.overall); err != nil {
			continue
		}
		gradeMap[k] = e
		posByGroup[e.posGroup] = append(posByGroup[e.posGroup], k)
	}
	if len(gradeMap) == 0 {
		log.Println("  no grades found — skipping enrichment")
		return nil
	}

	// 2. Compute mean/stdev of overall grade per position group (globally across seasons).
	type posGroupStats struct {
		mean, stdev float64
	}
	posStats := make(map[string]posGroupStats)
	for posGroup, keys := range posByGroup {
		vals := make([]float64, len(keys))
		for i, k := range keys {
			vals[i] = gradeMap[k].overall
		}
		mu := mean(vals)
		sd := stdev(vals, mu)
		posStats[posGroup] = posGroupStats{mu, sd}
	}

	// 3. Load all profile z_scores, merge overall_grade_z, batch update.
	profileRows, err := pool.Query(ctx,
		`SELECT id, gsis_id, season, position_group, z_scores::text
		 FROM nfl_player_season_profiles
		 WHERE z_scores IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("load profiles for grade enrichment: %w", err)
	}
	defer profileRows.Close()

	type profileUpdate struct {
		id    int64
		zJSON string
	}
	var updates []profileUpdate

	for profileRows.Next() {
		var id int64
		var gsisID, posGroup, zStr string
		var season int
		if err := profileRows.Scan(&id, &gsisID, &season, &posGroup, &zStr); err != nil {
			continue
		}

		// Look up grade for this player-season.
		ge, ok := gradeMap[gradeKey{gsisID, season}]
		if !ok {
			continue
		}

		// Compute z-score of overall grade within position group.
		ps, ok := posStats[posGroup]
		if !ok || ps.stdev == 0 {
			continue
		}
		gradeZ := (ge.overall - ps.mean) / ps.stdev

		// Parse existing z_scores, add overall_grade_z, re-serialize.
		var zMap map[string]float64
		if err := json.Unmarshal([]byte(zStr), &zMap); err != nil {
			continue
		}
		zMap["overall_grade_z"] = gradeZ

		zJSON, err := json.Marshal(zMap)
		if err != nil {
			continue
		}
		updates = append(updates, profileUpdate{id, string(zJSON)})
	}

	// 4. Batch update in groups of 500.
	const batchSize = 500
	for i := 0; i < len(updates); i += batchSize {
		end := i + batchSize
		if end > len(updates) {
			end = len(updates)
		}

		// Build a single UPDATE with a VALUES list for efficiency.
		batch := updates[i:end]
		vals := make([]string, len(batch))
		args := make([]any, 0, len(batch)*2)
		for j, u := range batch {
			vals[j] = fmt.Sprintf("($%d::bigint, $%d::jsonb)", j*2+1, j*2+2)
			args = append(args, u.id, u.zJSON)
		}

		query := `UPDATE nfl_player_season_profiles AS p
			SET z_scores = v.zs
			FROM (VALUES ` + strings.Join(vals, ",") + `) AS v(id, zs)
			WHERE p.id = v.id`

		if _, err := pool.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("batch update grade z-scores: %w", err)
		}
	}

	log.Printf("  enriched %d profiles with overall_grade_z", len(updates))
	return nil
}
