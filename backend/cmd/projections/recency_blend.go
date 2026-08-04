// recency_blend.go implements recency-weighted blending of a target player's
// base-season profile with their immediately preceding season, so a small-sample
// season (e.g. 4 games missed to injury) doesn't stand alone as the entire input
// to comp matching and growth projection. See docs/stats/recency-weighted-profiles.md.
//
// This is deliberately NOT a shrinkage-toward-population-mean technique (that's
// shrinkage.go/growth_baseline.go) — it blends two observations of the SAME
// player across time, more like an exponentially-weighted moving average.
package main

import "math"

// blendWeight returns the contribution weight for a season observation:
// games played, discounted by decay^ageInSeasons for how many seasons old it is
// relative to the target base season (0 = the base season itself).
func blendWeight(games float64, decay float64, ageInSeasons int) float64 {
	if ageInSeasons == 0 {
		return games
	}
	return games * math.Pow(decay, float64(ageInSeasons))
}

// blendFloat returns the weighted average of two observations, falling back to
// cur if the combined weight is zero (avoids divide-by-zero on data-free input).
func blendFloat(cur, prior float64, wCur, wPrior float64) float64 {
	total := wCur + wPrior
	if total == 0 {
		return cur
	}
	return (cur*wCur + prior*wPrior) / total
}

// blendPtr mirrors blendFloat for the profile's nullable efficiency fields.
// Missing values on either side fall back to whichever side has data.
func blendPtr(cur, prior *float64, wCur, wPrior float64) *float64 {
	if cur == nil {
		return prior
	}
	if prior == nil || wPrior == 0 {
		v := *cur
		return &v
	}
	v := blendFloat(*cur, *prior, wCur, wPrior)
	return &v
}

// blendTargetProfile returns the profile to use as a projection TARGET (comp
// search + growth-rate input) — never as a comp candidate, which stays a plain
// single-season snapshot. Skeleton fields (age, years_exp, draft_number,
// position_group, height, weight, gsis_id, season, games_played) are point-in-time
// facts copied from the base season unchanged; every per-game rate and z-score is
// games-weighted across the base season and (if present) the immediately
// preceding one, discounted by decay.
//
// decay == 0 (the seeded default) or a missing prior-season profile makes this an
// EXACT no-op — the base season is returned unmodified, preserving today's
// behavior until autotune (or manual experimentation) finds a decay value that
// actually improves held-out validation accuracy.
func blendTargetProfile(seasonMap map[int]*seasonProfile, baseSeason int, decay float64) *seasonProfile {
	base := seasonMap[baseSeason]
	if base == nil {
		return nil
	}
	prior := seasonMap[baseSeason-1]
	if prior == nil || decay <= 0 {
		return base
	}

	wCur := blendWeight(float64(base.GamesPlayed), decay, 0)
	wPrior := blendWeight(float64(prior.GamesPlayed), decay, 1)

	blended := *base // shallow copy — skeleton fields carry over unchanged

	blended.PassAttPG = blendFloat(base.PassAttPG, prior.PassAttPG, wCur, wPrior)
	blended.PassYdsPG = blendFloat(base.PassYdsPG, prior.PassYdsPG, wCur, wPrior)
	blended.PassTdPG = blendFloat(base.PassTdPG, prior.PassTdPG, wCur, wPrior)
	blended.IntPG = blendFloat(base.IntPG, prior.IntPG, wCur, wPrior)
	blended.RushAttPG = blendFloat(base.RushAttPG, prior.RushAttPG, wCur, wPrior)
	blended.RushYdsPG = blendFloat(base.RushYdsPG, prior.RushYdsPG, wCur, wPrior)
	blended.RushTdPG = blendFloat(base.RushTdPG, prior.RushTdPG, wCur, wPrior)
	blended.TargetsPG = blendFloat(base.TargetsPG, prior.TargetsPG, wCur, wPrior)
	blended.RecPG = blendFloat(base.RecPG, prior.RecPG, wCur, wPrior)
	blended.RecYdsPG = blendFloat(base.RecYdsPG, prior.RecYdsPG, wCur, wPrior)
	blended.RecTdPG = blendFloat(base.RecTdPG, prior.RecTdPG, wCur, wPrior)
	blended.FptsPG = blendFloat(base.FptsPG, prior.FptsPG, wCur, wPrior)
	blended.FptsPPRPG = blendFloat(base.FptsPPRPG, prior.FptsPPRPG, wCur, wPrior)
	blended.FgMadePG = blendFloat(base.FgMadePG, prior.FgMadePG, wCur, wPrior)
	blended.PatMadePG = blendFloat(base.PatMadePG, prior.PatMadePG, wCur, wPrior)
	blended.SacksPG = blendFloat(base.SacksPG, prior.SacksPG, wCur, wPrior)
	blended.PassingAirYardsPG = blendFloat(base.PassingAirYardsPG, prior.PassingAirYardsPG, wCur, wPrior)
	blended.PassingYACPG = blendFloat(base.PassingYACPG, prior.PassingYACPG, wCur, wPrior)
	blended.RushingFirstDownsPG = blendFloat(base.RushingFirstDownsPG, prior.RushingFirstDownsPG, wCur, wPrior)
	blended.ReceivingAirYardsPG = blendFloat(base.ReceivingAirYardsPG, prior.ReceivingAirYardsPG, wCur, wPrior)
	blended.ReceivingYACPG = blendFloat(base.ReceivingYACPG, prior.ReceivingYACPG, wCur, wPrior)
	blended.ReceivingFirstDownsPG = blendFloat(base.ReceivingFirstDownsPG, prior.ReceivingFirstDownsPG, wCur, wPrior)
	blended.FumblesPG = blendFloat(base.FumblesPG, prior.FumblesPG, wCur, wPrior)

	blended.PassYPA = blendPtr(base.PassYPA, prior.PassYPA, wCur, wPrior)
	blended.CompPct = blendPtr(base.CompPct, prior.CompPct, wCur, wPrior)
	blended.RushYPC = blendPtr(base.RushYPC, prior.RushYPC, wCur, wPrior)
	blended.RecYPR = blendPtr(base.RecYPR, prior.RecYPR, wCur, wPrior)
	blended.TargetShare = blendPtr(base.TargetShare, prior.TargetShare, wCur, wPrior)
	blended.WOPR = blendPtr(base.WOPR, prior.WOPR, wCur, wPrior)
	blended.PassEPAPlay = blendPtr(base.PassEPAPlay, prior.PassEPAPlay, wCur, wPrior)
	blended.RushEPAPlay = blendPtr(base.RushEPAPlay, prior.RushEPAPlay, wCur, wPrior)
	blended.RecEPAPlay = blendPtr(base.RecEPAPlay, prior.RecEPAPlay, wCur, wPrior)
	blended.RushYardShare = blendPtr(base.RushYardShare, prior.RushYardShare, wCur, wPrior)
	blended.AirYardsShare = blendPtr(base.AirYardsShare, prior.AirYardsShare, wCur, wPrior)
	blended.FgPct = blendPtr(base.FgPct, prior.FgPct, wCur, wPrior)
	blended.TeamFptsPG = blendPtr(base.TeamFptsPG, prior.TeamFptsPG, wCur, wPrior)
	blended.TeamPassYdsPG = blendPtr(base.TeamPassYdsPG, prior.TeamPassYdsPG, wCur, wPrior)
	blended.TeamRushYdsPG = blendPtr(base.TeamRushYdsPG, prior.TeamRushYdsPG, wCur, wPrior)

	// Z-scores are globally normalized across all seasons (computeZScores), so a
	// z-score of 1.0 means the same thing regardless of season — safe to blend
	// directly the same way as the raw rates.
	blended.ZScores = make(map[string]float64, len(base.ZScores))
	for field, v := range base.ZScores {
		if pv, ok := prior.ZScores[field]; ok {
			blended.ZScores[field] = blendFloat(v, pv, wCur, wPrior)
		} else {
			blended.ZScores[field] = v
		}
	}

	return &blended
}
