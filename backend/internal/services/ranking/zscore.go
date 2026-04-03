package ranking

import (
	"math"
	"sort"
)

// RankByCategories computes weighted z-score rankings for category-based leagues (NBA etc.).
//
// Algorithm:
//  1. Compute per-category mean + stdev across all rostered players.
//  2. Compute category weights: CV × scarcity (normalised to mean = 1.0).
//  3. Compute weighted z-scores per player; overall_score = Σ weighted z-scores.
//  4. position_score = z-score within same-position group.
//  5. Percentile per category = % of rostered players this player beats.
func RankByCategories(players []PlayerData, catMeta []CategoryMeta, faPlayers []PlayerData) RankCategoriesResult {
	if len(players) == 0 {
		return RankCategoriesResult{
			Players:       []ScoredPlayer{},
			CategoryStats: []CategoryStats{},
		}
	}

	// --- Step 1: Compute rostered-player mean + stdev per category ---
	type catAgg struct {
		mean  float64
		stdev float64
	}
	catStats := make(map[string]catAgg)

	for _, cat := range catMeta {
		var sum float64
		var count int
		for _, p := range players {
			if v, ok := p.StatValues[cat.ID]; ok {
				sum += v
				count++
			}
		}
		if count == 0 {
			catStats[cat.ID] = catAgg{}
			continue
		}
		mean := sum / float64(count)
		var sqDiffSum float64
		for _, p := range players {
			if v, ok := p.StatValues[cat.ID]; ok {
				diff := v - mean
				sqDiffSum += diff * diff
			}
		}
		stdev := math.Sqrt(sqDiffSum / float64(count))
		catStats[cat.ID] = catAgg{mean: mean, stdev: stdev}
	}

	// --- Step 2: Compute FA z-scores per category (relative to rostered baseline) ---
	faValues := make(map[string][]float64) // catID → []z-score
	for _, fa := range faPlayers {
		for _, cat := range catMeta {
			v, ok := fa.StatValues[cat.ID]
			if !ok {
				continue
			}
			agg := catStats[cat.ID]
			if agg.stdev == 0 {
				continue
			}
			z := (v - agg.mean) / agg.stdev
			if cat.SortOrder == "0" {
				z = -z
			}
			faValues[cat.ID] = append(faValues[cat.ID], z)
		}
	}

	// --- Step 3: Compute category weights ---
	catWeights := make(map[string]float64)
	for _, cat := range catMeta {
		agg := catStats[cat.ID]
		if agg.stdev == 0 {
			catWeights[cat.ID] = 0
			continue
		}
		absMean := math.Abs(agg.mean)
		var cv float64
		if absMean >= 0.01 {
			cv = agg.stdev / absMean
		} else {
			cv = agg.stdev
		}

		scarcity := 1.0
		if zs, ok := faValues[cat.ID]; ok && len(zs) > 0 {
			var sumZ float64
			for _, z := range zs {
				sumZ += z
			}
			faAvgZ := sumZ / float64(len(zs))
			scarcity = 1.0 / (1.0 + math.Max(0, faAvgZ))
		}

		catWeights[cat.ID] = cv * scarcity
	}

	// Normalise weights so mean = 1.0.
	var weightSum float64
	var nonZeroCount int
	for _, cat := range catMeta {
		if catWeights[cat.ID] > 0 {
			weightSum += catWeights[cat.ID]
			nonZeroCount++
		}
	}
	if nonZeroCount > 0 {
		meanWeight := weightSum / float64(nonZeroCount)
		for _, cat := range catMeta {
			if catWeights[cat.ID] > 0 {
				catWeights[cat.ID] /= meanWeight
			}
		}
	} else {
		for _, cat := range catMeta {
			catWeights[cat.ID] = 1.0
		}
	}

	// --- Step 4: Compute weighted z-scores and overall scores ---
	type scoredEntry struct {
		data       PlayerData
		catZScores map[string]float64
		overall    float64
	}
	scored := make([]scoredEntry, len(players))

	for i, p := range players {
		se := scoredEntry{
			data:       p,
			catZScores: make(map[string]float64),
		}
		for _, cat := range catMeta {
			v, hasValue := p.StatValues[cat.ID]
			agg := catStats[cat.ID]
			if !hasValue || agg.stdev == 0 {
				se.catZScores[cat.ID] = 0
				continue
			}
			z := (v - agg.mean) / agg.stdev
			if cat.SortOrder == "0" {
				z = -z
			}
			se.catZScores[cat.ID] = z
			se.overall += catWeights[cat.ID] * z
		}
		scored[i] = se
	}

	// Sort by overall score descending.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].overall > scored[j].overall
	})

	// --- Step 5: Compute position_score (z-score within position group) ---
	posGroups := make(map[string][]int)
	for i, se := range scored {
		posGroups[se.data.PrimaryPos] = append(posGroups[se.data.PrimaryPos], i)
	}

	posScores := make(map[int]float64)
	posRanks := make(map[int]int)

	for _, indices := range posGroups {
		if len(indices) < 2 {
			posScores[indices[0]] = 0
			posRanks[indices[0]] = 1
			continue
		}
		var posSum float64
		for _, idx := range indices {
			posSum += scored[idx].overall
		}
		posMean := posSum / float64(len(indices))
		var posSqDiff float64
		for _, idx := range indices {
			d := scored[idx].overall - posMean
			posSqDiff += d * d
		}
		posStdev := math.Sqrt(posSqDiff / float64(len(indices)))

		if posStdev == 0 {
			for _, idx := range indices {
				posScores[idx] = 0
			}
		} else {
			for _, idx := range indices {
				posScores[idx] = (scored[idx].overall - posMean) / posStdev
			}
		}

		sortedIdx := make([]int, len(indices))
		copy(sortedIdx, indices)
		sort.Slice(sortedIdx, func(a, b int) bool {
			return posScores[sortedIdx[a]] > posScores[sortedIdx[b]]
		})
		for rank, idx := range sortedIdx {
			posRanks[idx] = rank + 1
		}
	}

	// --- Step 6: Compute percentiles per category ---
	catPercentiles := make(map[string]map[string]int) // catID → playerKey → percentile
	for _, cat := range catMeta {
		type pv struct {
			playerKey string
			value     float64
		}
		var pvs []pv
		for _, se := range scored {
			if v, ok := se.data.StatValues[cat.ID]; ok {
				pvs = append(pvs, pv{se.data.PlayerKey, v})
			}
		}
		if len(pvs) == 0 {
			continue
		}
		ascending := cat.SortOrder == "0"
		sort.Slice(pvs, func(i, j int) bool {
			if ascending {
				return pvs[i].value < pvs[j].value
			}
			return pvs[i].value > pvs[j].value
		})
		m := make(map[string]int)
		n := len(pvs)
		for rank, entry := range pvs {
			pct := int(math.Round(float64(n-rank) / float64(n) * 100))
			m[entry.playerKey] = pct
		}
		catPercentiles[cat.ID] = m
	}

	// --- Build results ---
	resultPlayers := make([]ScoredPlayer, len(scored))
	for i, se := range scored {
		catScores := make([]CategoryScore, 0, len(catMeta))
		for _, cat := range catMeta {
			value := se.data.StatValues[cat.ID]
			z := se.catZScores[cat.ID]
			pct := 50
			if m, ok := catPercentiles[cat.ID]; ok {
				if p, ok := m[se.data.PlayerKey]; ok {
					pct = p
				}
			}
			catScores = append(catScores, CategoryScore{
				Label:      cat.Label,
				Value:      math.Round(value*100) / 100,
				ZScore:     math.Round(z*100) / 100,
				Percentile: pct,
			})
		}
		resultPlayers[i] = ScoredPlayer{
			PlayerData:     se.data,
			OverallScore:   math.Round(se.overall*100) / 100,
			OverallRank:    i + 1,
			PositionScore:  math.Round(posScores[i]*100) / 100,
			PositionRank:   posRanks[i],
			CategoryScores: catScores,
		}
	}

	resultCatStats := make([]CategoryStats, 0, len(catMeta))
	for _, cat := range catMeta {
		agg := catStats[cat.ID]
		resultCatStats = append(resultCatStats, CategoryStats{
			Label:     cat.Label,
			SortOrder: cat.SortOrder,
			Mean:      math.Round(agg.mean*100) / 100,
			Stdev:     math.Round(agg.stdev*100) / 100,
			Weight:    math.Round(catWeights[cat.ID]*1000) / 1000,
		})
	}

	return RankCategoriesResult{
		Players:       resultPlayers,
		CategoryStats: resultCatStats,
	}
}
