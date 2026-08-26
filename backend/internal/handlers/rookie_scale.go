package handlers

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/leaguesettings"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// Rookie-scale defaults, used for any field a league hasn't customized (see
// models.RookieScale). TopPct/BottomPct are the percentile range of the
// league's own auction-value board a draft class spans: pick 1.01 prices at
// the 3rd percentile (very good, but a proven veteran at the top of the
// board still costs more — the whole point of a scale is that an unproven
// rookie is a discount to that), the last pick of the class prices at the
// 55th percentile (solidly replacement-level). Single global constants
// picked to be defensible, not tuned per league — same posture as
// auctionVORCompressionExponent in draft_values.go.
const (
	defaultRookieScaleTopPct    = 0.03
	defaultRookieScaleBottomPct = 0.55
)

var defaultRookieScaleYearsByRound = map[string]int{"1": 4, "2": 3, "3": 2}

// rookieScalePctRange resolves a league's top/bottom percentile, falling
// back to the defaults for any zero field — every field of RookieScale is
// meaningless at its zero value, so a plain zero-check tells "not
// configured" from "configured" with no pointer indirection needed.
func rookieScalePctRange(cfg models.RookieScale) (top, bottom float64) {
	top, bottom = cfg.TopPct, cfg.BottomPct
	if top <= 0 {
		top = defaultRookieScaleTopPct
	}
	if bottom <= 0 {
		bottom = defaultRookieScaleBottomPct
	}
	return top, bottom
}

// rookieScaleYears resolves a rookie contract's length from its draft round.
// A round beyond what's configured falls back to the last configured
// round's length rather than 0 — a 4th-round pick in a league that only
// configured rounds 1-3 should get at least as short a deal as round 3, not
// an contract-length of zero.
func rookieScaleYears(cfg models.RookieScale, round int) int {
	byRound := cfg.YearsByRound
	if len(byRound) == 0 {
		byRound = defaultRookieScaleYearsByRound
	}
	if y, ok := byRound[strconv.Itoa(round)]; ok && y > 0 {
		return y
	}
	maxConfiguredRound, yearsAtMax := 0, 1
	for k, y := range byRound {
		if r, err := strconv.Atoi(k); err == nil && y > 0 && r > maxConfiguredRound {
			maxConfiguredRound, yearsAtMax = r, y
		}
	}
	return yearsAtMax
}

// priceableSeason clamps a requested season down to the latest season
// nfl_projections actually has rows for. nfl_projections only ever carries
// one "current" target season (the frontend's PROJECTION_SEASON constant —
// draft-prep, the league Draft tab, and keepers all clamp their own target
// season the same way, Math.min(natural, PROJECTION_SEASON)). A rookie
// pick's own season, or a free-agency window's, is routinely further out
// than that (a 2028 pick generated today), and computeDraftBoard would then
// find zero projections and silently price everyone at the $1 floor —
// clamping to whatever season actually has data avoids hardcoding a year
// that would need updating every season. Shared by rookie-scale pricing and
// free-agency valuation, which both price off computeDraftBoard the same way.
func (h *Handler) priceableSeason(ctx context.Context, season int) (int, error) {
	var maxProjSeason int
	if err := h.db.QueryRow(ctx, "SELECT COALESCE(MAX(target_season), 0) FROM nfl_projections").Scan(&maxProjSeason); err != nil {
		return 0, err
	}
	if maxProjSeason > 0 && season > maxProjSeason {
		return maxProjSeason, nil
	}
	return season, nil
}

// resolveNativeDraftBoardInputs builds computeDraftBoard's inputs from a
// native league's own real settings — no query overrides, unlike
// GetDraftValues, since a rookie's actual contract has to be priced off the
// board the league actually plays under, not a hypothetical one. Mirrors
// GetDraftValues' no-override branch; kept as a separate, smaller function
// rather than threading a "no overrides" mode through that HTTP handler,
// since the two have essentially no other logic in common (request parsing,
// Yahoo-vs-native dispatch) that a rookie contract ever needs.
func (h *Handler) resolveNativeDraftBoardInputs(ctx context.Context, leagueID int64, season int) (draftBoardInputs, error) {
	var numTeams int
	if err := h.db.QueryRow(ctx, "SELECT COUNT(*) FROM teams WHERE league_id = $1", leagueID).Scan(&numTeams); err != nil {
		return draftBoardInputs{}, err
	}
	if numTeams == 0 {
		return draftBoardInputs{}, fmt.Errorf("league has no teams")
	}

	budget := h.config.DefaultBudget
	if b, ok := leaguesettings.Budget(ctx, h.db, leagueID); ok {
		budget = b
	}

	src := leaguesettings.NewNativeSource(h.db, leagueID)
	fetchedPositions, _, canonicalMods, hasLeagueScoring := leaguesettings.FetchSettings(ctx, src)
	effectiveSlots := leaguesettings.SlotsFromPositions(fetchedPositions)

	recMod := canonicalMods[scoring.StatRec]
	var effectiveFormat string
	switch {
	case recMod >= 0.9:
		effectiveFormat = "ppr"
	case recMod >= 0.35:
		effectiveFormat = "half"
	default:
		effectiveFormat = "standard"
	}
	if !hasLeagueScoring {
		effectiveFormat = "ppr"
	}

	// Same pointing-system resolution as GetDraftValues' no-override path —
	// see that function's step comments for why FG collapses to one average.
	activeScoring := map[scoring.CanonicalStat]float64{}
	if hasLeagueScoring {
		var avgFGMod float64
		for bucket, share := range scoring.FGDistribution {
			avgFGMod += share * canonicalMods[bucket]
		}
		activeScoring[scoring.StatPassYds] = canonicalMods[scoring.StatPassYds]
		activeScoring[scoring.StatPassTD] = canonicalMods[scoring.StatPassTD]
		activeScoring[scoring.StatRushYds] = canonicalMods[scoring.StatRushYds]
		activeScoring[scoring.StatRushTD] = canonicalMods[scoring.StatRushTD]
		activeScoring[scoring.StatRec] = canonicalMods[scoring.StatRec]
		activeScoring[scoring.StatRecYds] = canonicalMods[scoring.StatRecYds]
		activeScoring[scoring.StatRecTD] = canonicalMods[scoring.StatRecTD]
		activeScoring[scoring.StatFGMade] = avgFGMod
		activeScoring[scoring.StatPATMade] = canonicalMods[scoring.StatPATMade]
	} else {
		for k, v := range leaguesettings.DefaultScoringFallback {
			activeScoring[k] = v
		}
	}

	return draftBoardInputs{
		Season:             season,
		Budget:             budget,
		NumTeams:           numTeams,
		RankingPositions:   fetchedPositions,
		EffectiveSlots:     effectiveSlots,
		EffectiveFormat:    effectiveFormat,
		HasScoringOverride: false,
		ActiveScoring:      activeScoring,
		CanonicalMods:      canonicalMods,
		HasLeagueScoring:   hasLeagueScoring,
	}, nil
}

// rookieScaleSalary derives a rookie's contract salary from his draft slot
// alone — not from who's actually selected there. Trading a future pick
// only makes sense if its price is knowable in advance, independent of
// which player eventually gets drafted with it (and UseLeagueDraftPick lets
// a pick be spent on any unrostered player, rookie or not, so pricing off
// the specific player picked would let a late pick buy an established
// veteran for pennies). The mechanism mirrors draft_consensus.go's
// loadConsensusValues: read the league's own real auction-value curve at a
// rank derived from where this pick falls in its class, between the
// league's configured top/bottom percentiles.
func (h *Handler) rookieScaleSalary(ctx context.Context, leagueID int64, season, overallPick, totalPicks int, cfg models.RookieScale) (int, error) {
	priceSeason, err := h.priceableSeason(ctx, season)
	if err != nil {
		return 0, err
	}

	in, err := h.resolveNativeDraftBoardInputs(ctx, leagueID, priceSeason)
	if err != nil {
		return 0, err
	}
	players, _, _, err := h.computeDraftBoard(ctx, in)
	if err != nil {
		return 0, err
	}
	if len(players) == 0 {
		return 1, nil // no projections for this season at all — nothing to price off
	}

	// computeDraftBoard already sorts by VOR descending, which very rarely
	// diverges from dollar order (kickers are priced off a separate $1-3
	// band, not the shared VOR curve) — sort by AuctionValue explicitly
	// since that's the actual curve being read.
	sort.SliceStable(players, func(i, j int) bool {
		return players[i].AuctionValue > players[j].AuctionValue
	})

	topPct, bottomPct := rookieScalePctRange(cfg)
	pickFraction := 0.0
	if totalPicks > 1 {
		pickFraction = float64(overallPick-1) / float64(totalPicks-1)
	}
	effectivePct := topPct + pickFraction*(bottomPct-topPct)

	rank := int(math.Round(effectivePct * float64(len(players)-1)))
	if rank < 0 {
		rank = 0
	}
	if rank >= len(players) {
		rank = len(players) - 1
	}
	return players[rank].AuctionValue, nil
}
