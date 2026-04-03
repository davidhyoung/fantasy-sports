package ranking

import (
	"math"
	"sort"
)

// ComputeStarterSlots returns fractional starter slots per position per team.
// FLEX slots (e.g. "W/R/T") are distributed evenly among eligible positions.
// Non-starter slots (BN, IR, IL, IL+) are excluded.
func ComputeStarterSlots(positions []RosterPosition) map[string]float64 {
	slots := make(map[string]float64)
	for _, rp := range positions {
		switch rp.Position {
		case "BN", "IR", "IL", "IL+":
			continue
		}
		eligible := ParseFlexEligible(rp.Position)
		if len(eligible) > 0 {
			share := float64(rp.Count) / float64(len(eligible))
			for _, pos := range eligible {
				slots[pos] += share
			}
		} else {
			if full, ok := FlexAbbrev[rp.Position]; ok {
				slots[full] += float64(rp.Count)
			} else {
				slots[rp.Position] += float64(rp.Count)
			}
		}
	}
	return slots
}

// ComputeReplacementLevels returns replacement level points per position.
// Each position group must be pre-sorted by TotalPoints descending.
// threshold = ceil(slots_per_team × numTeams); replacement = player at that index.
func ComputeReplacementLevels(posGroups map[string][]PlayerData, starterSlots map[string]float64, numTeams int) map[string]ReplacementLevel {
	levels := make(map[string]ReplacementLevel)
	for pos, slots := range starterSlots {
		threshold := int(math.Ceil(slots * float64(numTeams)))
		if threshold <= 0 {
			threshold = 1
		}
		pList := posGroups[pos]
		var replPoints float64
		if len(pList) == 0 {
			replPoints = 0
		} else if threshold >= len(pList) {
			replPoints = pList[len(pList)-1].TotalPoints
		} else {
			replPoints = pList[threshold].TotalPoints
		}
		levels[pos] = ReplacementLevel{
			Position:  pos,
			Threshold: threshold,
			Points:    replPoints,
		}
	}
	return levels
}

// RankByPoints computes VORP rankings for points-based leagues (NFL).
//
// Algorithm:
//  1. Compute total fantasy points per player = Σ(stat_value × stat_modifier).
//     (Callers must pre-compute TotalPoints on each PlayerData.)
//  2. Determine starter thresholds from roster settings.
//  3. Replacement level = points of the first non-starter at each position.
//  4. VORP = total_points − replacement_level[position].
//  5. Per-stat z-scores relative to rostered-player baseline.
func RankByPoints(rosteredPlayers []PlayerData, faPlayers []PlayerData, catMeta []CategoryMeta, rosterPositions []RosterPosition, numTeams int) RankPointsResult {
	// --- Group rostered players by position, sorted by total points ---
	posGroups := make(map[string][]PlayerData)
	for _, p := range rosteredPlayers {
		posGroups[p.PrimaryPos] = append(posGroups[p.PrimaryPos], p)
	}
	for pos := range posGroups {
		sort.Slice(posGroups[pos], func(i, j int) bool {
			return posGroups[pos][i].TotalPoints > posGroups[pos][j].TotalPoints
		})
	}

	// --- Compute replacement levels from rostered players only ---
	starterSlots := ComputeStarterSlots(rosterPositions)
	replLevels := ComputeReplacementLevels(posGroups, starterSlots, numTeams)

	// --- Compute per-stat mean + stdev across ROSTERED players ---
	type catAgg struct {
		mean  float64
		stdev float64
	}
	catStats := make(map[string]catAgg)
	for _, cat := range catMeta {
		var sum float64
		var count int
		for _, p := range rosteredPlayers {
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
		for _, p := range rosteredPlayers {
			if v, ok := p.StatValues[cat.ID]; ok {
				diff := v - mean
				sqDiffSum += diff * diff
			}
		}
		catStats[cat.ID] = catAgg{
			mean:  mean,
			stdev: math.Sqrt(sqDiffSum / float64(count)),
		}
	}

	// --- Combine rostered + FA players for ranking ---
	allPlayers := make([]PlayerData, 0, len(rosteredPlayers)+len(faPlayers))
	allPlayers = append(allPlayers, rosteredPlayers...)
	allPlayers = append(allPlayers, faPlayers...)

	// Rebuild position groups including FAs for position rank computation.
	posGroupsAll := make(map[string][]int) // primaryPos → indices into allPlayers
	for i := range allPlayers {
		posGroupsAll[allPlayers[i].PrimaryPos] = append(posGroupsAll[allPlayers[i].PrimaryPos], i)
	}

	// Compute VORP for all players.
	vorps := make([]float64, len(allPlayers))
	for i := range allPlayers {
		repl, ok := replLevels[allPlayers[i].PrimaryPos]
		if ok {
			vorps[i] = math.Round((allPlayers[i].TotalPoints-repl.Points)*100) / 100
		}
	}

	// Sort indices within each position group by total points desc for position ranks.
	posRanks := make(map[int]int) // index → position rank
	for _, indices := range posGroupsAll {
		sort.Slice(indices, func(a, b int) bool {
			return allPlayers[indices[a]].TotalPoints > allPlayers[indices[b]].TotalPoints
		})
		for rank, idx := range indices {
			posRanks[idx] = rank + 1
		}
	}

	// Sort all players by VORP descending.
	order := make([]int, len(allPlayers))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return vorps[order[a]] > vorps[order[b]]
	})

	// --- Build per-category z-scores and result ---
	resultPlayers := make([]ScoredPlayer, len(allPlayers))
	for rank, idx := range order {
		p := allPlayers[idx]
		catScores := make([]CategoryScore, 0, len(catMeta))
		for _, cat := range catMeta {
			value := p.StatValues[cat.ID]
			agg := catStats[cat.ID]
			var z float64
			if agg.stdev > 0 {
				z = (value - agg.mean) / agg.stdev
				if cat.SortOrder == "0" {
					z = -z
				}
			}
			catScores = append(catScores, CategoryScore{
				Label:      cat.Label,
				Value:      math.Round(value*100) / 100,
				ZScore:     math.Round(z*100) / 100,
				Percentile: 50, // not needed for points mode
			})
		}
		resultPlayers[rank] = ScoredPlayer{
			PlayerData:     p,
			OverallScore:   vorps[idx],
			OverallRank:    rank + 1,
			PositionRank:   posRanks[idx],
			VORP:           vorps[idx],
			CategoryScores: catScores,
		}
	}

	// --- Build category stats ---
	resultCatStats := make([]CategoryStats, 0, len(catMeta))
	for _, cat := range catMeta {
		agg := catStats[cat.ID]
		resultCatStats = append(resultCatStats, CategoryStats{
			Label:     cat.Label,
			SortOrder: cat.SortOrder,
			Mean:      math.Round(agg.mean*100) / 100,
			Stdev:     math.Round(agg.stdev*100) / 100,
			Weight:    math.Round(cat.Modifier*1000) / 1000,
		})
	}

	// --- Build replacement level response ---
	replResult := make([]ReplacementLevel, 0, len(replLevels))
	for _, rl := range replLevels {
		replResult = append(replResult, ReplacementLevel{
			Position:  rl.Position,
			Threshold: rl.Threshold,
			Points:    math.Round(rl.Points*100) / 100,
		})
	}
	sort.Slice(replResult, func(i, j int) bool {
		return replResult[i].Position < replResult[j].Position
	})

	return RankPointsResult{
		Players:           resultPlayers,
		CategoryStats:     resultCatStats,
		ReplacementLevels: replResult,
	}
}
