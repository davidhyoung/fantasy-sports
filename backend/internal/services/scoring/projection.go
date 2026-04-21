package scoring

// ProjectionRates holds projected per-game rates read from nfl_projections.
// Grouped into a struct so callers get a checked argument list instead of nine
// positional float64s.
type ProjectionRates struct {
	PassYdsPG float64
	PassTdPG  float64
	RushYdsPG float64
	RushTdPG  float64
	RecPG     float64
	RecYdsPG  float64
	RecTdPG   float64
	FgMadePG  float64
	PatMadePG float64
}

// ProjectionToCanonicalTotals multiplies per-game rates by games to produce
// canonical-keyed season totals. FG attempts are distributed across distance
// buckets via FGDistribution so Yahoo's distance-based FG scoring can be
// applied directly on top.
func ProjectionToCanonicalTotals(rates ProjectionRates, games float64) map[CanonicalStat]float64 {
	fg := rates.FgMadePG * games
	m := map[CanonicalStat]float64{
		StatPassYds: rates.PassYdsPG * games,
		StatPassTD:  rates.PassTdPG * games,
		StatRushYds: rates.RushYdsPG * games,
		StatRushTD:  rates.RushTdPG * games,
		StatRec:     rates.RecPG * games,
		StatRecYds:  rates.RecYdsPG * games,
		StatRecTD:   rates.RecTdPG * games,
		StatFGMade:  fg,
		StatPATMade: rates.PatMadePG * games,
	}
	for bucket, share := range FGDistribution {
		m[bucket] = fg * share
	}
	return m
}

// ScoreWithModifiers sums total × modifier across every canonical stat present
// in the modifiers map. Stats absent from totals contribute zero.
func ScoreWithModifiers(totals map[CanonicalStat]float64, modifiers map[CanonicalStat]float64) float64 {
	var pts float64
	for stat, mod := range modifiers {
		if mod == 0 {
			continue
		}
		if v, ok := totals[stat]; ok {
			pts += v * mod
		}
	}
	return pts
}

// CanonicalModifiersFromYahoo translates a map of Yahoo stat-ID → modifier
// into a canonical-keyed modifier map. Unknown Yahoo stat IDs are dropped.
func CanonicalModifiersFromYahoo(yahooMods map[string]float64) map[CanonicalStat]float64 {
	out := make(map[CanonicalStat]float64, len(yahooMods))
	for sid, mod := range yahooMods {
		if canon := YahooToCanonical(sid); canon != "" {
			out[canon] = mod
		}
	}
	return out
}
