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
// Each position group in posGroups must be pre-sorted by TotalPoints descending.
//
// Dedicated (single-position) slots claim starters directly: count × numTeams
// league-wide. Flex-type slots (e.g. "W/R/T", "Q/W/R/T") are NOT pre-split
// evenly across their eligible positions — that approximation only holds when
// the eligible positions have comparable value, which breaks down hard for
// SFLEX (QB dwarfs WR/RB/TE per game in points formats, so an even split
// drastically undercounts real QB demand). Instead each flex slot pools its
// still-unclaimed eligible candidates by value and lets the actual best
// players claim the spots — see poolFlexSlots.
func ComputeReplacementLevels(positions []RosterPosition, posGroups map[string][]PlayerData, numTeams int) map[string]ReplacementLevel {
	// cursor[pos] = how many of posGroups[pos] (sorted desc) are already
	// claimed by some starter slot, dedicated or flex. Track it for every
	// position mentioned by any roster slot, dedicated or flex-eligible, so
	// the output key set matches ComputeStarterSlots' (positions with zero
	// slots still get a threshold of 0, not omitted).
	cursor := make(map[string]int)
	var flexGroups []struct {
		eligible []string
		spots    int
	}
	for _, rp := range positions {
		switch rp.Position {
		case "BN", "IR", "IL", "IL+":
			continue
		}
		eligible := ParseFlexEligible(rp.Position)
		if len(eligible) > 0 {
			for _, pos := range eligible {
				if _, ok := cursor[pos]; !ok {
					cursor[pos] = 0
				}
			}
			flexGroups = append(flexGroups, struct {
				eligible []string
				spots    int
			}{eligible, rp.Count * numTeams})
			continue
		}
		pos := rp.Position
		if full, ok := FlexAbbrev[pos]; ok {
			pos = full
		}
		cursor[pos] += rp.Count * numTeams
	}

	// Flex groups are processed in roster order. A player pulled from position
	// P by any flex group is always the next-highest unclaimed player at P
	// (every candidate pool is a suffix of P's own sorted list), so a plain
	// claimed-count per position is sufficient — no need to track identities.
	for _, fg := range flexGroups {
		remaining := fg.spots
		for remaining > 0 {
			bestPos := ""
			bestVal := math.Inf(-1)
			for _, pos := range fg.eligible {
				idx := cursor[pos]
				if idx < len(posGroups[pos]) && posGroups[pos][idx].TotalPoints > bestVal {
					bestVal = posGroups[pos][idx].TotalPoints
					bestPos = pos
				}
			}
			if bestPos == "" {
				break // no eligible candidates left anywhere
			}
			cursor[bestPos]++
			remaining--
		}
	}

	levels := make(map[string]ReplacementLevel)
	for pos, threshold := range cursor {
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
	replLevels := ComputeReplacementLevels(rosterPositions, posGroups, numTeams)

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
