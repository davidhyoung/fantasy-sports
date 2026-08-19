// Package leaguesettings answers "what are this league's roster slots and
// scoring rules" — the fantasy-context half of what Yahoo used to provide
// exclusively (see the split in .claude/plans/yahoo-decoupling.md, which
// removed Yahoo as the *stats* source; this package is step one of removing
// it as the fantasy-context source too, for leagues that have no Yahoo
// league behind them at all).
//
// Everything here is provider-agnostic vocabulary: an editable slot name
// ("FLEX"), a roster-position shape ranking.ComputeReplacementLevels expects,
// and canonical per-stat point values. A Source resolves that vocabulary for
// one league, whether backed by a live Yahoo league (implemented here) or a
// native league's own settings row (added in a later phase).
package leaguesettings

import (
	"context"
	"strconv"
	"strings"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/ranking"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// ── roster-slot vocabulary ────────────────────────────────────────────────────
//
// The client edits a flat set of slot names; Yahoo expresses flex spots as
// slash-separated eligibility lists. These two helpers translate between them so
// the flex distribution itself stays in ranking.ComputeStarterSlots.

// SlotToPosition maps an editable slot name to the roster-position string it
// stands for (Yahoo's own vocabulary for flex eligibility, e.g. "W/R/T").
var SlotToPosition = map[string]string{
	"QB": "QB", "RB": "RB", "WR": "WR", "TE": "TE", "K": "K", "DEF": "DEF",
	"FLEX":  "W/R/T",
	"SFLEX": "Q/W/R/T",
	// Bench starts nobody, so ComputeStarterSlots ignores it and it cannot affect
	// replacement levels or prices. It is carried anyway because roster size is
	// what tells the team builder how many $1 slots a budget still has to cover.
	"BN": "BN",
}

// PositionToSlot maps a roster-position string back to an editable slot name.
// Bench and injury slots return false — they don't start anyone. Flex shapes we
// don't model exactly (e.g. "W/R") fold into FLEX, which is what the UI offers.
func PositionToSlot(pos string) (string, bool) {
	switch pos {
	case "BN":
		return "BN", true
	case "IR", "IL", "IL+", "NA":
		return "", false
	case "QB", "RB", "WR", "TE", "K":
		return pos, true
	case "DEF", "DST", "D":
		return "DEF", true
	}
	eligible := ranking.ParseFlexEligible(pos)
	if len(eligible) == 0 {
		return "", false
	}
	for _, e := range eligible {
		if e == "QB" {
			return "SFLEX", true
		}
	}
	return "FLEX", true
}

// SlotsFromPositions summarises a league's roster positions in the editable vocabulary.
func SlotsFromPositions(positions []ranking.RosterPosition) map[string]int {
	slots := map[string]int{}
	for _, rp := range positions {
		if name, ok := PositionToSlot(rp.Position); ok {
			slots[name] += rp.Count
		}
	}
	return slots
}

// ParseSlotOverride reads a `slots=QB:1,RB:2,FLEX:1,SFLEX:1` override into both
// the editable map and the roster-position shape ComputeStarterSlots expects.
func ParseSlotOverride(raw string) (map[string]int, []ranking.RosterPosition, bool) {
	slots := map[string]int{}
	var positions []ranking.RosterPosition
	for _, part := range strings.Split(raw, ",") {
		name, countStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		name = strings.ToUpper(strings.TrimSpace(name))
		pos, known := SlotToPosition[name]
		if !known {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil || count < 0 || count > 20 {
			continue
		}
		slots[name] = count
		if count > 0 {
			positions = append(positions, ranking.RosterPosition{Position: pos, Count: count})
		}
	}
	// A parse that yielded no startable slot is treated as absent rather than as
	// an empty roster, which would make every player worth his full point total.
	// Bench alone doesn't count as startable, for the same reason.
	startable := 0
	for _, p := range positions {
		if p.Position != "BN" {
			startable++
		}
	}
	return slots, positions, startable > 0
}

// ── pointing system (per-stat point values) ───────────────────────────────────
//
// The draft-values scoring override lets the settings panel edit exactly the
// stat categories we have per-game rate columns for (scoring.ProjectionRates):
// passing/rushing/receiving yards and TDs, receptions, FG made, PAT made.
// Yahoo's own FG scoring is distance-bucketed; the override collapses that into
// one flat "FG made" value rather than exposing five more fields.
var ScoringEditableStats = []scoring.CanonicalStat{
	scoring.StatPassYds, scoring.StatPassTD,
	scoring.StatRushYds, scoring.StatRushTD,
	scoring.StatRec, scoring.StatRecYds, scoring.StatRecTD,
	scoring.StatFGMade, scoring.StatPATMade,
}

var scoringEditableSet = func() map[scoring.CanonicalStat]bool {
	m := make(map[scoring.CanonicalStat]bool, len(ScoringEditableStats))
	for _, s := range ScoringEditableStats {
		m[s] = true
	}
	return m
}()

// DefaultScoringFallback seeds the pointing system when a league's own scoring
// couldn't be determined — a standard full-PPR point set.
var DefaultScoringFallback = map[scoring.CanonicalStat]float64{
	scoring.StatPassYds: 0.04, scoring.StatPassTD: 4,
	scoring.StatRushYds: 0.1, scoring.StatRushTD: 6,
	scoring.StatRec: 1, scoring.StatRecYds: 0.1, scoring.StatRecTD: 6,
	scoring.StatFGMade: 3, scoring.StatPATMade: 1,
}

// ParseScoringOverride reads a `scoring=pass_yds:0.04,pass_td:4,...` override
// into canonical-stat-keyed point values. Unknown keys and unparsable values
// are dropped rather than rejecting the whole override, same convention as
// ParseSlotOverride.
func ParseScoringOverride(raw string) (map[scoring.CanonicalStat]float64, bool) {
	out := map[scoring.CanonicalStat]float64{}
	for _, part := range strings.Split(raw, ",") {
		name, valStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		key := scoring.CanonicalStat(strings.ToLower(strings.TrimSpace(name)))
		if !scoringEditableSet[key] {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64)
		if err != nil {
			continue
		}
		out[key] = val
	}
	return out, len(out) > 0
}

// ── Source ─────────────────────────────────────────────────────────────────
//
// Source resolves one league's fantasy context. Split into two independent
// calls (rather than one combined method) because callers today treat their
// failures differently: a roster-positions fetch failure is fatal unless the
// caller has its own slots override; a scoring fetch failure just falls back
// to a default pointing system. FetchSettings below fetches both concurrently
// for callers that want both, same as the original two-goroutine fan-out in
// draft_values.go.
type Source interface {
	// RosterPositions returns the league's starter/bench slot configuration.
	RosterPositions(ctx context.Context) ([]ranking.RosterPosition, error)
	// ScoringMods returns the league's per-stat point values, translated to the
	// canonical vocabulary. ok is false when the league's scoring couldn't be
	// determined (caller should fall back to a standard scoring set).
	ScoringMods(ctx context.Context) (mods map[scoring.CanonicalStat]float64, ok bool)
}

// FetchSettings runs RosterPositions and ScoringMods concurrently, since
// callers always want both and (for a Yahoo-backed Source) both round-trip
// the same underlying settings endpoint.
func FetchSettings(ctx context.Context, src Source) (
	positions []ranking.RosterPosition, positionsErr error,
	mods map[scoring.CanonicalStat]float64, hasScoring bool,
) {
	type posResult struct {
		positions []ranking.RosterPosition
		err       error
	}
	type modResult struct {
		mods map[scoring.CanonicalStat]float64
		ok   bool
	}
	posCh := make(chan posResult, 1)
	modCh := make(chan modResult, 1)
	go func() {
		p, err := src.RosterPositions(ctx)
		posCh <- posResult{p, err}
	}()
	go func() {
		m, ok := src.ScoringMods(ctx)
		modCh <- modResult{m, ok}
	}()
	pr := <-posCh
	mr := <-modCh
	return pr.positions, pr.err, mr.mods, mr.ok
}

// ── Yahoo-backed Source ───────────────────────────────────────────────────────

// YahooSource resolves a league's settings from a live Yahoo league.
type YahooSource struct {
	yc        *yahoo.Client
	leagueKey string
}

// NewYahooSource builds a Source backed by the given Yahoo league.
func NewYahooSource(yc *yahoo.Client, leagueKey string) *YahooSource {
	return &YahooSource{yc: yc, leagueKey: leagueKey}
}

func (s *YahooSource) RosterPositions(ctx context.Context) ([]ranking.RosterPosition, error) {
	pos, err := s.yc.GetLeagueRosterPositions(ctx, s.leagueKey)
	if err != nil {
		return nil, err
	}
	out := make([]ranking.RosterPosition, len(pos))
	for i, p := range pos {
		out[i] = ranking.RosterPosition{Position: p.Position, Count: p.Count}
	}
	return out, nil
}

func (s *YahooSource) ScoringMods(ctx context.Context) (map[scoring.CanonicalStat]float64, bool) {
	stats, err := s.yc.GetLeagueScoringStats(ctx, s.leagueKey)
	if err != nil || len(stats) == 0 {
		return nil, false
	}
	yahooMods := make(map[string]float64, len(stats))
	for id, ls := range stats {
		yahooMods[id] = ls.Modifier
	}
	return scoring.CanonicalModifiersFromYahoo(yahooMods), true
}
