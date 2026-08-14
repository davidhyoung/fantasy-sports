package main

// short_season_shrinkage.go — regresses a target player's production stats
// toward the position-group mean when their base season is short AND there is
// no prior season to blend against.
//
// recency_blend.go already solves "a small-sample season doesn't stand alone"
// — but only when a prior season exists to blend with. A rookie (or any
// player's first tracked season) whose debut was cut short by injury has no
// prior season, so blendTargetProfile's prior==nil branch used to return the
// partial season completely unregressed: an 8-game hot stretch would drive
// comp matching and growth projection exactly as if it were a proven
// full-season role (see docs/algorithm-review.md's Cam Skattebo case study —
// 2025 rookie season ended by injury after week 8, comped and grown forward
// as if his per-game rate over those 8 games was his established level).
//
// Scope: only the "production" fields — counting stats that combine role,
// efficiency, and TD variance (fpts_pg, rush_yds_pg, rec_td_pg, ...) and are
// genuinely noisy over a handful of games. Raw opportunity counts
// (pass_att_pg, rush_att_pg, targets_pg) are deliberately left alone, matching
// shrinkage.go's usage-share exception: how many looks a player got is a real
// role signal even in a short sample. Efficiency rates (rush_ypc,
// pass_epa_play, ...) are pointer fields already Bayesian-shrunk by
// shrinkage.go and are untouched here to avoid double-shrinking. Physical,
// context, grade, and draft-capital dimensions aren't sample-size-dependent
// and are also untouched.

const shortSeasonFullGames = 17.0

// productionFields lists the raw per-game float64 fields shrunk by
// shrinkShortSeasonTarget, keyed the same way as seasonProfile.ZScores and
// the DB columns (snake_case).
var productionFields = []string{
	"pass_yds_pg", "pass_td_pg", "int_pg",
	"rush_yds_pg", "rush_td_pg",
	"rec_pg", "rec_yds_pg", "rec_td_pg",
	"fpts_pg", "fpts_ppr_pg", "fg_made_pg", "pat_made_pg",
	"sacks_pg", "passing_air_yards_pg", "passing_yac_pg",
	"rushing_first_downs_pg", "receiving_air_yards_pg", "receiving_yac_pg",
	"receiving_first_downs_pg", "fumbles_pg",
}

// groupMeanProfile holds position-group mean values for productionFields.
type groupMeanProfile map[string]float64

// computeGroupMeanProfiles returns, per position group, the mean of each
// productionFields stat across every profile in the pool — the regression
// target for shrinkShortSeasonTarget.
func computeGroupMeanProfiles(byGroup map[string][]*seasonProfile) map[string]groupMeanProfile {
	out := make(map[string]groupMeanProfile, len(byGroup))
	for g, ps := range byGroup {
		sums := make(map[string]float64, len(productionFields))
		for _, p := range ps {
			vals := profileFieldValues(p)
			for _, f := range productionFields {
				sums[f] += vals[f]
			}
		}
		mean := make(groupMeanProfile, len(productionFields))
		if n := float64(len(ps)); n > 0 {
			for _, f := range productionFields {
				mean[f] = sums[f] / n
			}
		}
		out[g] = mean
	}
	return out
}

// profileFieldValues exposes a seasonProfile's productionFields by name, for
// the generic mean/shrink loops that operate on that shared field list.
func profileFieldValues(p *seasonProfile) map[string]float64 {
	return map[string]float64{
		"pass_yds_pg":              p.PassYdsPG,
		"pass_td_pg":               p.PassTdPG,
		"int_pg":                   p.IntPG,
		"rush_yds_pg":              p.RushYdsPG,
		"rush_td_pg":               p.RushTdPG,
		"rec_pg":                   p.RecPG,
		"rec_yds_pg":               p.RecYdsPG,
		"rec_td_pg":                p.RecTdPG,
		"fpts_pg":                  p.FptsPG,
		"fpts_ppr_pg":              p.FptsPPRPG,
		"fg_made_pg":               p.FgMadePG,
		"pat_made_pg":              p.PatMadePG,
		"sacks_pg":                 p.SacksPG,
		"passing_air_yards_pg":     p.PassingAirYardsPG,
		"passing_yac_pg":           p.PassingYACPG,
		"rushing_first_downs_pg":   p.RushingFirstDownsPG,
		"receiving_air_yards_pg":   p.ReceivingAirYardsPG,
		"receiving_yac_pg":         p.ReceivingYACPG,
		"receiving_first_downs_pg": p.ReceivingFirstDownsPG,
		"fumbles_pg":               p.FumblesPG,
	}
}

// setProfileField mutates one of productionFields on p by name — the write
// side of profileFieldValues.
func setProfileField(p *seasonProfile, field string, v float64) {
	switch field {
	case "pass_yds_pg":
		p.PassYdsPG = v
	case "pass_td_pg":
		p.PassTdPG = v
	case "int_pg":
		p.IntPG = v
	case "rush_yds_pg":
		p.RushYdsPG = v
	case "rush_td_pg":
		p.RushTdPG = v
	case "rec_pg":
		p.RecPG = v
	case "rec_yds_pg":
		p.RecYdsPG = v
	case "rec_td_pg":
		p.RecTdPG = v
	case "fpts_pg":
		p.FptsPG = v
	case "fpts_ppr_pg":
		p.FptsPPRPG = v
	case "fg_made_pg":
		p.FgMadePG = v
	case "pat_made_pg":
		p.PatMadePG = v
	case "sacks_pg":
		p.SacksPG = v
	case "passing_air_yards_pg":
		p.PassingAirYardsPG = v
	case "passing_yac_pg":
		p.PassingYACPG = v
	case "rushing_first_downs_pg":
		p.RushingFirstDownsPG = v
	case "receiving_air_yards_pg":
		p.ReceivingAirYardsPG = v
	case "receiving_yac_pg":
		p.ReceivingYACPG = v
	case "receiving_first_downs_pg":
		p.ReceivingFirstDownsPG = v
	case "fumbles_pg":
		p.FumblesPG = v
	}
}

// shrinkShortSeasonTarget pulls a target's production stats — and their
// matching z-scores, since computeSimilarity reads z-scores rather than raw
// fields — toward the position-group mean, weighted by
// games_played/shortSeasonFullGames. A full (or longer) season is an exact
// no-op. Z-scores are population-standardized (mean 0), so shrinking one
// toward the mean is just scaling it by the same weight used for the raw
// value.
func shrinkShortSeasonTarget(base *seasonProfile, means groupMeanProfile) *seasonProfile {
	weight := float64(base.GamesPlayed) / shortSeasonFullGames
	if weight >= 1 || means == nil {
		return base
	}
	if weight < 0 {
		weight = 0
	}

	shrunk := *base // shallow copy — non-production fields carry over unchanged
	shrunk.ZScores = make(map[string]float64, len(base.ZScores))
	for k, v := range base.ZScores {
		shrunk.ZScores[k] = v
	}

	vals := profileFieldValues(base)
	for _, f := range productionFields {
		mean, ok := means[f]
		if !ok {
			continue
		}
		setProfileField(&shrunk, f, weight*vals[f]+(1-weight)*mean)
		if z, ok := shrunk.ZScores[f]; ok {
			shrunk.ZScores[f] = weight * z
		}
	}
	return &shrunk
}
