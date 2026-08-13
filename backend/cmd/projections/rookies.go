package main

import (
	"math"
	"sort"
)

// Rookie projections: draft-capital comp model.
//
// The main comp engine (computeWeightedProjection) grows a target's OWN
// season profile forward using comps' growth rates — there's nothing to grow
// for a player who has never played an NFL down, so incoming rookies (no
// nfl_player_season_profiles row at all) are silently excluded from the
// targets loop in computeProjections. This file gives them a different
// treatment: comp a rookie against historical rookies (their first,
// years_exp == 0 season) at the same position, weighted by how close their
// draft slot was, and use those comps' actual rookie-season output directly
// as the projection — there's no prior season of the target's own to apply a
// growth rate to.
const (
	// rookieDraftSigma is the Gaussian kernel width (in picks) for draft-slot
	// similarity — roughly 1.5 rounds, so the same range of the same round
	// carries most of the weight but a round away still contributes some.
	rookieDraftSigma = 40.0
	// rookieUndraftedSlot stands in for "no pick" so undrafted rookies land on
	// the same numeric scale, past the last pick of a modern 7-round draft
	// (Mr. Irrelevant is usually ~257-262) — worse than any drafted slot.
	rookieUndraftedSlot = 300.0
	// rookieSimilarityThresh mirrors the veteran engine's inclusion bar, but
	// draft-slot-only similarity is coarser than a full stat-line match, so
	// rookieMinComps below guarantees a floor regardless of threshold.
	rookieSimilarityThresh = 0.15
	rookieMinComps         = 5
)

// rookieOutcome is one historical player's rookie-season profile, reduced to
// what the draft-capital comp model needs: a position, a draft slot on a
// single numeric scale, and their actual rookie-year per-game production.
type rookieOutcome struct {
	GsisID    string
	Season    int
	Age       int
	DraftSlot float64
	FptsPG    float64
	FptsPPRPG float64
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

// buildRookiePool groups every historical player's rookie-season profile by
// position group. years_exp == 0 reliably means "this IS their rookie
// season" — main.go derives it per-profile from season - entry_year, not
// from "earliest season we happen to have imported" — and profiles only
// exist for player-seasons with >= minGames games played, so redshirt/
// zero-snap rookie years are already excluded from the pool.
func buildRookiePool(profiles []seasonProfile) map[string][]rookieOutcome {
	pool := make(map[string][]rookieOutcome)
	for i := range profiles {
		p := &profiles[i]
		if p.YearsExp != 0 {
			continue
		}
		slot := rookieUndraftedSlot
		if p.DraftNumber != nil {
			slot = float64(*p.DraftNumber)
		}
		pool[p.PositionGroup] = append(pool[p.PositionGroup], rookieOutcome{
			GsisID: p.GsisID, Season: p.Season, Age: p.Age, DraftSlot: slot,
			FptsPG: p.FptsPG, FptsPPRPG: p.FptsPPRPG,
			PassYdsPG: p.PassYdsPG, PassTdPG: p.PassTdPG,
			RushYdsPG: p.RushYdsPG, RushTdPG: p.RushTdPG,
			RecPG: p.RecPG, RecYdsPG: p.RecYdsPG, RecTdPG: p.RecTdPG,
			FgMadePG: p.FgMadePG, PatMadePG: p.PatMadePG,
		})
	}
	return pool
}

// rookieComp is a scored historical rookie comp, carrying both its
// similarity and its share of the total comp weight.
type rookieComp struct {
	outcome rookieOutcome
	sim     float64
	weight  float64
}

// computeRookieProjections projects incoming rookies — nfl_players rows
// whose entry_year is targetSeason — that have no season profile of their
// own to grow from. existingTargets excludes anyone the main comp loop
// already projected (defensive; entry_year == targetSeason should already
// imply no prior-season profile exists).
func computeRookieProjections(targetSeason int, cfg projConfig, metaMap map[string]playerMeta, pool map[string][]rookieOutcome, existingTargets map[string]bool) []projection {
	var out []projection

	for gsisID, m := range metaMap {
		if existingTargets[gsisID] {
			continue
		}
		if m.EntryYear == nil || *m.EntryYear != targetSeason {
			continue
		}
		if m.PositionGroup == nil {
			continue
		}
		posGroup := *m.PositionGroup
		candidates := pool[posGroup]
		if len(candidates) == 0 {
			continue
		}

		targetSlot := rookieUndraftedSlot
		if m.DraftNumber != nil {
			targetSlot = float64(*m.DraftNumber)
		}

		scored := make([]rookieComp, len(candidates))
		for i, c := range candidates {
			d := math.Abs(targetSlot - c.DraftSlot)
			z := d / rookieDraftSigma
			scored[i] = rookieComp{outcome: c, sim: math.Exp(-0.5 * z * z)}
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].sim > scored[j].sim })

		var comps []rookieComp
		for _, sc := range scored {
			if sc.sim >= rookieSimilarityThresh {
				comps = append(comps, sc)
			}
		}
		if len(comps) < rookieMinComps {
			n := rookieMinComps
			if len(scored) < n {
				n = len(scored)
			}
			comps = scored[:n]
		}

		var wSum float64
		for i := range comps {
			comps[i].weight = comps[i].sim * comps[i].sim
			wSum += comps[i].weight
		}
		if wSum == 0 {
			// Defensive: the Gaussian kernel never hits exactly 0, but avoid a
			// divide-by-zero if every comp is somehow scored at 0 similarity.
			for i := range comps {
				comps[i].weight = 1
			}
			wSum = float64(len(comps))
		}

		var proj projection
		avg := func(get func(rookieOutcome) float64) float64 {
			var s float64
			for _, c := range comps {
				s += c.weight * get(c.outcome)
			}
			return s / wSum
		}
		proj.ProjFptsPG = avg(func(o rookieOutcome) float64 { return o.FptsPG })
		proj.ProjFptsPPRPG = avg(func(o rookieOutcome) float64 { return o.FptsPPRPG })
		proj.ProjPassYdsPG = avg(func(o rookieOutcome) float64 { return o.PassYdsPG })
		proj.ProjPassTdPG = avg(func(o rookieOutcome) float64 { return o.PassTdPG })
		proj.ProjRushYdsPG = avg(func(o rookieOutcome) float64 { return o.RushYdsPG })
		proj.ProjRushTdPG = avg(func(o rookieOutcome) float64 { return o.RushTdPG })
		proj.ProjRecPG = avg(func(o rookieOutcome) float64 { return o.RecPG })
		proj.ProjRecYdsPG = avg(func(o rookieOutcome) float64 { return o.RecYdsPG })
		proj.ProjRecTdPG = avg(func(o rookieOutcome) float64 { return o.RecTdPG })
		proj.ProjFgMadePG = avg(func(o rookieOutcome) float64 { return o.FgMadePG })
		proj.ProjPatMadePG = avg(func(o rookieOutcome) float64 { return o.PatMadePG })

		// Outcome distribution: the similarity²-weighted spread of comps' own
		// rookie-season output IS the uncertainty band here — there's no growth
		// rate to perturb, the comps' actual results are the raw material.
		type outcome struct {
			val float64
			w   float64
		}
		outcomes := make([]outcome, len(comps))
		for i, c := range comps {
			outcomes[i] = outcome{val: c.outcome.FptsPPRPG, w: c.weight}
		}
		if len(outcomes) >= 2 {
			var mu float64
			for _, o := range outcomes {
				mu += (o.w / wSum) * o.val
			}
			var variance float64
			for _, o := range outcomes {
				d := o.val - mu
				variance += (o.w / wSum) * d * d
			}
			proj.ProjFptsPPRStdev = math.Sqrt(variance)

			sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].val < outcomes[j].val })
			quantile := func(q float64) float64 {
				cum := 0.0
				for _, o := range outcomes {
					cum += o.w / wSum
					if cum >= q {
						return o.val
					}
				}
				return outcomes[len(outcomes)-1].val
			}
			proj.ProjFptsPPRP10 = quantile(0.10)
			proj.ProjFptsPPRP50 = quantile(0.50)
			proj.ProjFptsPPRP90 = quantile(0.90)
		} else {
			proj.ProjFptsPPRP10 = proj.ProjFptsPPRPG
			proj.ProjFptsPPRP50 = proj.ProjFptsPPRPG
			proj.ProjFptsPPRP90 = proj.ProjFptsPPRPG
		}

		proj.applyCalibration(calibrationFor(cfg, posGroup))

		proj.GsisID = gsisID
		proj.BaseSeason = targetSeason - 1
		proj.TargetSeason = targetSeason
		proj.ProjGames = defaultProjGames
		proj.ProjFpts = proj.ProjFptsPG * float64(proj.ProjGames)
		proj.ProjFptsPPR = proj.ProjFptsPPRPG * float64(proj.ProjGames)
		proj.ProjFptsHalf = (proj.ProjFptsPPR + proj.ProjFpts) / 2.0
		proj.ProjFptsPPRP10 *= float64(proj.ProjGames)
		proj.ProjFptsPPRP50 *= float64(proj.ProjGames)
		proj.ProjFptsPPRP90 *= float64(proj.ProjGames)

		proj.CompCount = len(comps)
		var sumSim float64
		for _, c := range comps {
			sumSim += c.sim
		}
		proj.AvgSimilarity = sumSim / float64(len(comps))
		proj.Uniqueness = uniquenessLabel(len(comps), proj.AvgSimilarity)

		// Confidence reuses the veteran path's weighted formula, but every
		// rookie starts from zero NFL data (ConfDataQuality = 0, since there's
		// no prior season of their own) and agreement is measured on the comps'
		// raw output rather than a growth rate.
		vals := make([]float64, len(comps))
		for i, c := range comps {
			vals[i] = c.outcome.FptsPPRPG
		}
		mu := mean(vals)
		agreement := 1.0
		if mu != 0 {
			agreement = 1.0 / (1.0 + stdev(vals, mu)/math.Abs(mu))
		}
		proj.ConfSimilarity = proj.AvgSimilarity
		proj.ConfCompCount = math.Min(1.0, float64(len(comps))/float64(commonArchetype))
		proj.ConfAgreement = agreement
		proj.ConfSampleDepth = 1.0 // every comp is a real, complete rookie season
		proj.ConfDataQuality = 0   // the target itself has zero NFL seasons
		proj.Confidence = 0.25*proj.ConfSimilarity + 0.20*proj.ConfCompCount +
			0.25*proj.ConfAgreement + 0.15*proj.ConfSampleDepth + 0.15*proj.ConfDataQuality

		compResults := make([]compResult, 0, len(comps))
		for _, c := range comps {
			cm := metaMap[c.outcome.GsisID]
			headshot := ""
			if cm.HeadshotURL != nil {
				headshot = *cm.HeadshotURL
			}
			var matching, divergent []string
			if math.Abs(targetSlot-c.outcome.DraftSlot) <= rookieDraftSigma {
				matching = append(matching, "draft_capital")
			} else {
				divergent = append(divergent, "draft_capital")
			}
			compResults = append(compResults, compResult{
				GsisID:      c.outcome.GsisID,
				Name:        cm.Name,
				MatchSeason: c.outcome.Season,
				MatchAge:    c.outcome.Age,
				Similarity:  c.sim,
				Weight:      c.weight / wSum,
				HeadshotURL: headshot,
				MatchProfile: map[string]float64{
					"fpts_pg":     c.outcome.FptsPG,
					"fpts_ppr_pg": c.outcome.FptsPPRPG,
					"pass_yds_pg": c.outcome.PassYdsPG,
					"pass_td_pg":  c.outcome.PassTdPG,
					"rush_yds_pg": c.outcome.RushYdsPG,
					"rush_td_pg":  c.outcome.RushTdPG,
					"rec_pg":      c.outcome.RecPG,
					"rec_yds_pg":  c.outcome.RecYdsPG,
					"rec_td_pg":   c.outcome.RecTdPG,
				},
				MatchingDims:  matching,
				DivergentDims: divergent,
			})
		}
		sort.Slice(compResults, func(i, j int) bool { return compResults[i].Similarity > compResults[j].Similarity })
		if len(compResults) > 10 {
			compResults = compResults[:10]
		}
		proj.Comps = compResults

		out = append(out, proj)
	}

	return out
}
