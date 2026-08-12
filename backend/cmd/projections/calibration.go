package main

// calibration.go — a multiplicative correction applied to every projected
// quantity, per position group.
//
// Why this exists: measured across 2015–2024 with temporal integrity
// (`-cohort-bias`), the engine projects startable players about 12% high, in
// every season sampled and at every projection level (docs/algorithm-review.md
// §7.7). The bias is proportional rather than additive, which is what makes a
// scalar the right shape of fix.
//
// What it does and does not move, given VOR = points − replacement and each
// auction price is a share of the VOR pool:
//
//	uniform factor across positions → ranks and auction values are UNCHANGED
//	                                  (k cancels: k·p − k·r = k·(p − r), and
//	                                  every share is computed from that pool)
//	per-position factors            → positions are repriced against each other
//
// The seeded configuration is deliberately uniform, so applying it corrects the
// displayed point totals without touching any draft board. Per-position factors
// are measurable (QB 0.953, RB 0.840, TE 0.880, WR 0.873) and are a config edit
// away, but they are a different decision — they change what players cost.
//
// A missing or empty map, or any factor <= 0, is an exact no-op.

// calibrationDefaultKey is the fallback used for position groups with no entry
// of their own (kickers, and anything new).
const calibrationDefaultKey = "default"

// calibrationFor returns the multiplicative factor for one position group.
func calibrationFor(cfg projConfig, posGroup string) float64 {
	if len(cfg.ProjectionCalibration) == 0 {
		return 1.0
	}
	if f, ok := cfg.ProjectionCalibration[posGroup]; ok && f > 0 {
		return f
	}
	if f, ok := cfg.ProjectionCalibration[calibrationDefaultKey]; ok && f > 0 {
		return f
	}
	return 1.0
}

// applyCalibration scales every projected quantity in place. Games played and
// the confidence components are counts and diagnostics, not projected
// production, so they are left alone.
//
// Per-stat rates are scaled alongside the point totals on purpose: fantasy
// points are a linear combination of those rates, so correcting the points
// without the rates would leave a player's displayed yardage implying a
// different score than his displayed points — and would quietly reprice kickers,
// whose league points are computed from the rates rather than a generic total.
func (p *projection) applyCalibration(factor float64) {
	if factor <= 0 || factor == 1.0 {
		return
	}
	scale := []*float64{
		&p.ProjFptsPG, &p.ProjFptsPPRPG,
		&p.ProjPassYdsPG, &p.ProjPassTdPG,
		&p.ProjRushYdsPG, &p.ProjRushTdPG,
		&p.ProjRecPG, &p.ProjRecYdsPG, &p.ProjRecTdPG,
		&p.ProjFgMadePG, &p.ProjPatMadePG,
		&p.ProjFpts, &p.ProjFptsPPR, &p.ProjFptsHalf,
		// The outcome distribution is in the same units and must move with the
		// point estimate, or the quantiles stop bracketing it.
		&p.ProjFptsPPRStdev, &p.ProjFptsPPRP10, &p.ProjFptsPPRP50, &p.ProjFptsPPRP90,
	}
	for _, v := range scale {
		*v *= factor
	}
}
