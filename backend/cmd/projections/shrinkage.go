package main

// shrinkage.go — empirical-Bayes shrinkage of small-sample rate stats toward
// the position-group mean before z-scoring (docs/stats/bayesian-shrinkage.md).
//
//	shrunk = (n·observed + k·popMean) / (n + k)
//
// popMean is the attempt-weighted population rate (Σ rate·n / Σ n), and k is
// the median sample size in the pool — so a median-volume player keeps ~50%
// of their observed rate, high-volume players barely move, and tiny samples
// collapse toward the prior. Usage shares (target_share, wopr) are NOT shrunk:
// they reflect role, which is real even in short samples.

import "sort"

// shrinkableRates lists the rate fields subject to shrinkage.
var shrinkableRates = []string{
	"pass_ypa", "comp_pct", "pass_epa_play",
	"rush_ypc", "rush_epa_play",
	"rec_ypr", "rec_epa_play",
	"fg_pct",
}

// rateSampleSize returns the denominator count behind a rate stat for one
// profile (pass attempts, carries, receptions, targets, FG attempts), derived
// from per-game volume × games played. Returns 0 when the rate is undefined.
func rateSampleSize(field string, vals map[string]float64, games float64) float64 {
	switch field {
	case "pass_ypa", "comp_pct", "pass_epa_play":
		return vals["pass_att_pg"] * games
	case "rush_ypc", "rush_epa_play":
		return vals["rush_att_pg"] * games
	case "rec_ypr":
		return vals["rec_pg"] * games
	case "rec_epa_play":
		return vals["targets_pg"] * games
	case "fg_pct":
		if pct := vals["fg_pct"]; pct > 0 {
			return vals["fg_made_pg"] * games / (pct / 100)
		}
	}
	return 0
}

type ratePrior struct {
	popMean float64 // attempt-weighted population mean rate
	k       float64 // shrinkage constant = median sample size in the pool
}

// shrinkRate pulls an observed rate toward the population mean with strength
// inversely proportional to sample size.
func shrinkRate(observed, n float64, p ratePrior) float64 {
	if n <= 0 || p.k <= 0 {
		return observed
	}
	return (n*observed + p.k*p.popMean) / (n + p.k)
}

// applyShrinkage mutates the vals maps in place, replacing each shrinkable
// rate with its shrunk value. Priors are computed per (position group, field)
// from the same pool of profiles, so backtests that pass only historical
// profiles stay temporally clean.
func applyShrinkage(valsList []map[string]float64, games []float64, posGroups []string) {
	type key struct{ group, field string }
	type obs struct{ rate, n float64 }

	pool := make(map[key][]obs)
	for i, vals := range valsList {
		if vals == nil {
			continue
		}
		for _, f := range shrinkableRates {
			v, ok := vals[f]
			if !ok {
				continue
			}
			n := rateSampleSize(f, vals, games[i])
			if n <= 0 {
				continue
			}
			k := key{posGroups[i], f}
			pool[k] = append(pool[k], obs{v, n})
		}
	}

	priors := make(map[key]ratePrior, len(pool))
	for k, os := range pool {
		var num, den float64
		ns := make([]float64, len(os))
		for i, o := range os {
			num += o.rate * o.n
			den += o.n
			ns[i] = o.n
		}
		if den <= 0 {
			continue
		}
		sort.Float64s(ns)
		priors[k] = ratePrior{popMean: num / den, k: ns[len(ns)/2]}
	}

	for i, vals := range valsList {
		if vals == nil {
			continue
		}
		for _, f := range shrinkableRates {
			v, ok := vals[f]
			if !ok {
				continue
			}
			// n must be computed before the rate is overwritten (fg_pct's
			// sample size is derived from the raw fg_pct itself).
			n := rateSampleSize(f, vals, games[i])
			if n <= 0 {
				continue
			}
			pr, ok := priors[key{posGroups[i], f}]
			if !ok {
				continue
			}
			vals[f] = shrinkRate(v, n, pr)
		}
	}
}
