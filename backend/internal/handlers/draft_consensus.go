package handlers

import (
	"context"
	"log"
	"sort"
)

// consensusValue is what the outside world thinks a player is worth, expressed
// on the same dollar scale as our own board so the two can sit side by side.
type consensusValue struct {
	PositionRank float64 // median rank within position across sources
	Auction      int     // dollars, on this league's budget scale
	SourceCount  int
	Derived      bool // true = inferred from consensus rank, false = imported market price
}

// consensusFormat maps our internal scoring format to the format naming used by
// nfl_consensus_rankings. Sources must be format-matched — redraft PPR and
// half-PPR price the same player differently, and dynasty differs enormously.
func consensusFormat(effectiveFormat string) string {
	if effectiveFormat == "half" {
		return "half_ppr"
	}
	return effectiveFormat // "ppr" | "standard"
}

func medianOf(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// marketBudgetPool is the league shape external auction values are published
// against — a 12-team, $200 league. Imported prices are rescaled from this to
// whatever the league actually plays, since a $300 budget inflates every price.
const marketBudgetPool = 200 * 12

// loadConsensusValues returns a consensus dollar value per player, keyed by gsis_id.
//
// Two sources, in order of preference:
//
//  1. **Imported auction prices** (`metric_type = 'auction'`) — real market values,
//     rescaled from a standard 12-team/$200 pool to this league's pool. Nothing
//     publishes these into our schema yet, but the shape is generic and the import
//     path already accepts them, so this branch lights up as soon as they exist.
//
//  2. **Derived from consensus rank** — the dollar value *our* board assigns to the
//     position rank the market gives the player. If four outlets call someone the
//     RB5 and our board pays $44 for its own RB5, his consensus value is $44.
//
// The derived form deliberately reuses our own value curve rather than inventing a
// second one: the question it answers is "what would this player be worth if the
// market's ordering were right?", which isolates the disagreement to ranking alone
// and keeps both columns on one scale. It is *not* a market clearing price — see
// docs/stats/auction-values.md for what that distinction costs.
//
// Ranks are read within position (not overall), matching the convention established
// in docs/stats/consensus-ensemble.md: overall ranks conflate raw point volume,
// which favours QBs, with scarcity-adjusted draft value.
func (h *Handler) loadConsensusValues(
	ctx context.Context, season int, effectiveFormat string, players []draftPlayer, budgetPool int,
) map[string]consensusValue {
	format := consensusFormat(effectiveFormat)
	out := map[string]consensusValue{}

	// --- 1. Imported auction prices, if any ---
	priceRows, err := h.db.Query(ctx, `
		SELECT gsis_id, value
		FROM nfl_consensus_rankings
		WHERE season = $1 AND format = $2 AND gsis_id IS NOT NULL AND metric_type = 'auction'
	`, season, format)
	if err != nil {
		log.Printf("[draft-values] consensus auction lookup: %v", err)
	} else {
		prices := map[string][]float64{}
		for priceRows.Next() {
			var gsisID string
			var value float64
			if err := priceRows.Scan(&gsisID, &value); err != nil {
				break
			}
			prices[gsisID] = append(prices[gsisID], value)
		}
		priceRows.Close()

		scale := float64(budgetPool) / float64(marketBudgetPool)
		for gsisID, vals := range prices {
			dollars := int(medianOf(vals)*scale + 0.5)
			if dollars < 1 {
				dollars = 1
			}
			out[gsisID] = consensusValue{Auction: dollars, SourceCount: len(vals)}
		}
	}

	// --- 2. Consensus rank, for everyone without an imported price ---
	//
	// RANK() over a source+position partition recovers a position-relative rank
	// whether the source published an overall list or a positional one — only the
	// order within the partition matters.
	rankRows, err := h.db.Query(ctx, `
		SELECT gsis_id,
		       RANK() OVER (PARTITION BY source, position ORDER BY value ASC) AS pos_rank
		FROM nfl_consensus_rankings
		WHERE season = $1 AND format = $2 AND gsis_id IS NOT NULL
		  AND metric_type IN ('rank', 'adp') AND position IS NOT NULL
	`, season, format)
	if err != nil {
		log.Printf("[draft-values] consensus rank lookup: %v", err)
		return out
	}
	defer rankRows.Close()

	ranks := map[string][]float64{}
	for rankRows.Next() {
		var gsisID string
		var posRank int
		if err := rankRows.Scan(&gsisID, &posRank); err != nil {
			log.Printf("[draft-values] scan consensus rank: %v", err)
			return out
		}
		ranks[gsisID] = append(ranks[gsisID], float64(posRank))
	}
	if err := rankRows.Err(); err != nil {
		log.Printf("[draft-values] consensus rank rows: %v", err)
		return out
	}
	if len(ranks) == 0 {
		return out
	}

	// Our own dollar value at each position rank — the curve the derived value reads.
	// Players are already ranked within position by the caller.
	byPositionRank := map[string][]int{}
	for _, p := range players {
		pos := primaryPosition(p.PositionGroup)
		if p.PositionRank <= 0 {
			continue
		}
		for len(byPositionRank[pos]) < p.PositionRank {
			byPositionRank[pos] = append(byPositionRank[pos], 0)
		}
		byPositionRank[pos][p.PositionRank-1] = p.AuctionValue
	}

	position := map[string]string{}
	for _, p := range players {
		position[p.GsisID] = primaryPosition(p.PositionGroup)
	}

	for gsisID, vals := range ranks {
		if _, priced := out[gsisID]; priced {
			// An imported market price beats anything we can infer from ranks; keep it
			// but record how many sources agreed on the ranking too.
			existing := out[gsisID]
			existing.PositionRank = medianOf(vals)
			out[gsisID] = existing
			continue
		}
		pos, ok := position[gsisID]
		if !ok {
			continue // ranked by the market but absent from our projections
		}
		curve := byPositionRank[pos]
		if len(curve) == 0 {
			continue
		}
		medianRank := medianOf(vals)
		slot := int(medianRank + 0.5)
		if slot < 1 {
			slot = 1
		}
		if slot > len(curve) {
			slot = len(curve)
		}
		out[gsisID] = consensusValue{
			PositionRank: medianRank,
			Auction:      curve[slot-1],
			SourceCount:  len(vals),
			Derived:      true,
		}
	}

	return out
}
