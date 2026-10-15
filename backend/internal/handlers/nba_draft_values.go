package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/tiers"
)

// nbaPositions is the roster-slot vocabulary auction pricing groups players
// into — ESPN's own taxonomy (G/F/C only, see nbaCategories' sibling comment
// in league_rosters_nba.go), not NBA category labels.
var nbaPositions = []string{"G", "F", "C"}

// nbaDraftPlayer is one priced player on the NBA auction-value board — the
// NBA analog of draft_values.go's draftPlayer, keyed by espn_id and priced
// off a league-category-weighted z-score sum instead of fantasy points.
type nbaDraftPlayer struct {
	ESPNID      string             `json:"espn_id"`
	Name        string             `json:"name"`
	Position    string             `json:"position"`
	Team        string             `json:"team"`
	HeadshotURL string             `json:"headshot_url"`
	Stats       []nbaCategoryEntry `json:"stats"`
	ZScores     []nbaCategoryEntry `json:"z_scores"`
	// FantasyValue is the z-score sum over only the categories this league
	// scores — the "points" analog that VOR/auction pricing is computed from.
	FantasyValue float64 `json:"fantasy_value"`
	// Minutes is DARKO's projected minutes per game.
	Minutes float64 `json:"minutes"`
	// CompCount is retained for API compatibility; DARKO is not comp-based.
	CompCount    int     `json:"comp_count"`
	VOR          float64 `json:"vor"`
	AuctionValue int     `json:"auction_value"`
	OverallRank  int     `json:"overall_rank"`
	PositionRank int     `json:"position_rank"`
	Tier         int     `json:"tier"`
	// Consensus columns are nil until at least one external source row exists
	// for the season.
	ConsensusAuctionValue *int     `json:"consensus_auction_value"`
	ConsensusPositionRank *float64 `json:"consensus_position_rank"`
	ConsensusSources      int      `json:"consensus_sources"`
	ConsensusDerived      bool     `json:"consensus_derived"`
	// SourceRanks is each individual outlet's position rank for this player
	// (G/F/C, per nba_players.position — see attachNBAConsensusValues' doc
	// comment) — the raw inputs ConsensusPositionRank's median is computed
	// from. There is no per-source dollar figure to show here: no outlet
	// publishes real auction prices we've imported (metric_type='auction' is
	// a generic, not-yet-populated path, same as NFL's), so this is rank
	// only, not a second column of dollar opinions.
	SourceRanks []nbaConsensusSourceRank `json:"source_ranks,omitempty"`
}

type nbaConsensusSourceRank struct {
	Source string `json:"source"`
	Rank   int    `json:"rank"`
}

type nbaDraftSettings struct {
	NumTeams int            `json:"num_teams"`
	Budget   int            `json:"budget"`
	Slots    map[string]int `json:"slots"`
	// Inclusion flags (0/1), not point weights — see league_settings.scoring's
	// native-NBA meaning (CLAUDE.md's "Native NBA leagues" section).
	Scoring    map[string]float64 `json:"scoring"`
	Overridden bool               `json:"overridden"`
}

type nbaReplacementLevel struct {
	Position  string  `json:"position"`
	Threshold int     `json:"threshold"`
	Value     float64 `json:"value"`
}

type nbaDraftValuesResp struct {
	Season            int                   `json:"season"`
	BudgetPerTeam     int                   `json:"budget_per_team"`
	NumTeams          int                   `json:"num_teams"`
	Settings          nbaDraftSettings      `json:"settings"`
	ReplacementLevels []nbaReplacementLevel `json:"replacement_levels"`
	Players           []nbaDraftPlayer      `json:"players"`
}

// parseNBASlotsOverride reads a `slots=G:2,F:2,C:1,UTIL:2,BN:3,IR:1` override.
// Unknown keys and unparsable counts are dropped rather than rejecting the
// whole override, same convention as leaguesettings.ParseSlotOverride.
func parseNBASlotsOverride(raw string) (map[string]int, bool) {
	valid := map[string]bool{"G": true, "F": true, "C": true, "UTIL": true, "BN": true, "IR": true}
	slots := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		name, countStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		name = strings.ToUpper(strings.TrimSpace(name))
		if !valid[name] {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil || count < 0 || count > 20 {
			continue
		}
		slots[name] = count
	}
	startable := slots["G"] + slots["F"] + slots["C"] + slots["UTIL"]
	return slots, startable > 0
}

// parseNBAScoringOverride reads a `scoring=points:1,turnovers:0,...` override
// — which of the 9 categories count, same inclusion-flag vocabulary
// league_settings.scoring already uses.
func parseNBAScoringOverride(raw string) (map[string]float64, bool) {
	valid := map[string]bool{}
	for _, c := range nbaCategories {
		valid[c] = true
	}
	out := map[string]float64{}
	for _, part := range strings.Split(raw, ",") {
		name, valStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(name))
		if !valid[key] {
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

// nbaComputeReplacementLevels mirrors ranking.ComputeReplacementLevels'
// algorithm (dedicated slots claim starters directly; a shared slot pools
// its still-unclaimed candidates by value) but is written directly for
// NBA's fixed G/F/C/UTIL vocabulary rather than forced through the NFL
// package's Yahoo-flex-string parser (see the plan's Context section for
// why — "W/R/T"-style abbreviations don't cover G/F/C at all).
//
// posGroups must already be sorted descending by value per position.
func nbaComputeReplacementLevels(posGroups map[string][]float64, slots map[string]int, numTeams int) map[string]nbaReplacementLevel {
	cursor := map[string]int{}
	for _, pos := range nbaPositions {
		cursor[pos] = slots[pos] * numTeams
	}
	utilSpots := slots["UTIL"] * numTeams
	for utilSpots > 0 {
		bestPos := ""
		bestVal := math.Inf(-1)
		for _, pos := range nbaPositions {
			idx := cursor[pos]
			if idx < len(posGroups[pos]) && posGroups[pos][idx] > bestVal {
				bestVal = posGroups[pos][idx]
				bestPos = pos
			}
		}
		if bestPos == "" {
			break // no eligible candidates left anywhere
		}
		cursor[bestPos]++
		utilSpots--
	}

	levels := make(map[string]nbaReplacementLevel, len(nbaPositions))
	for _, pos := range nbaPositions {
		threshold := cursor[pos]
		if threshold <= 0 {
			threshold = 1
		}
		pList := posGroups[pos]
		var val float64
		switch {
		case len(pList) == 0:
			val = 0
		case threshold >= len(pList):
			val = pList[len(pList)-1]
		default:
			val = pList[threshold]
		}
		levels[pos] = nbaReplacementLevel{Position: pos, Threshold: threshold, Value: val}
	}
	return levels
}

// nbaMarketBudgetPool is the reference league shape external $-auction data
// would be published against — same 12-team/$200 convention NFL's own
// marketBudgetPool uses (draft_consensus.go), and this app's own NBA default.
const nbaMarketBudgetPool = 200 * 12

// nbaAuctionVORCompressionExponent reuses NFL's empirically-tuned constant
// (docs/stats/auction-values.md) as a starting point. Unvalidated for NBA —
// no real auction-room data exists yet to re-derive it against.
const nbaAuctionVORCompressionExponent = 0.75

// GetNBADraftValues returns every batch-projected NBA player with a
// league-specific auction dollar value, computed from a z-score sum over
// only the categories this league scores (league_settings.scoring) and
// position-scarcity replacement levels from this league's own roster slots
// (league_settings.slots) — the NBA analog of draft_values.go, native-NBA
// leagues only (league_settings has no Yahoo-NBA equivalent; those leagues
// never get auction economics in this app).
//
// GET /api/leagues/{id}/draft-values-nba?season=2026&budget=200&teams=12&
//
//	slots=G:2,F:2,C:1,UTIL:2,BN:3,IR:1&scoring=points:1,turnovers:0,...
//
// Every setting is overridable, same convention as NFL's draft-values; the
// response echoes what was actually used.
func (h *Handler) GetNBADraftValues(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	ctx := r.Context()
	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	var numTeams, budget int
	var slotsRaw, scoringRaw []byte
	err = h.db.QueryRow(ctx, `
		SELECT num_teams, budget, slots, scoring FROM league_settings WHERE league_id = $1
	`, leagueID).Scan(&numTeams, &budget, &slotsRaw, &scoringRaw)
	if err != nil {
		respondError(w, http.StatusNotFound, "no settings for this league (native NBA leagues only)")
		return
	}
	slots := map[string]int{}
	if err := json.Unmarshal(slotsRaw, &slots); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scoring := map[string]float64{}
	if err := json.Unmarshal(scoringRaw, &scoring); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	overridden := false
	if t := q.Get("teams"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v >= 2 && v <= 32 {
			numTeams = v
			overridden = true
		}
	}
	if b := q.Get("budget"); b != "" {
		if v, err := strconv.Atoi(b); err == nil && v > 0 && v <= 100000 {
			budget = v
			overridden = true
		}
	}
	if s := q.Get("slots"); s != "" {
		if parsed, ok := parseNBASlotsOverride(s); ok {
			slots = parsed
			overridden = true
		}
	}
	if s := q.Get("scoring"); s != "" {
		if parsed, ok := parseNBAScoringOverride(s); ok {
			scoring = parsed
			overridden = true
		}
	}

	included := make(map[string]bool, len(nbaCategories))
	for _, cat := range nbaCategories {
		included[cat] = scoring[cat] != 0
	}

	// Load DARKO's projected pool (cmd/project-nba -import-darko), restricted
	// to rows matched to an espn_id and to the three real roster positions —
	// a player with no position on file can never actually be rostered, so
	// including them would just distort the replacement-level math.
	rows, err := h.db.Query(ctx, `
		SELECT d.espn_id, p.name, p.position, COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       d.x_minutes, d.x_pace, d.pts_100, d.ast_100, d.orb_100, d.drb_100, d.stl_100, d.blk_100,
		       d.tov_100, d.fga_100, d.fg3a_100, d.fta_100, d.fg_pct, d.fg3_pct, d.ft_pct
		FROM nba_darko_projections d
		JOIN nba_players p ON p.espn_id = d.espn_id
		WHERE d.season = $1 AND p.position = ANY($2)
	`, season, nbaPositions)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var players []nbaDraftPlayer
	rawByPlayer := make(map[string]nbaDarkoStats)
	for rows.Next() {
		var dp nbaDraftPlayer
		var d nbaDarkoRates
		if err := rows.Scan(
			&dp.ESPNID, &dp.Name, &dp.Position, &dp.Team, &dp.HeadshotURL,
			&d.minutes, &d.pace, &d.pts100, &d.ast100, &d.orb100, &d.drb100, &d.stl100, &d.blk100,
			&d.tov100, &d.fga100, &d.fg3a100, &d.fta100, &d.fgPct, &d.fg3Pct, &d.ftPct,
		); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dp.Minutes = d.minutes
		rawByPlayer[dp.ESPNID] = d.perGame()
		players = append(players, dp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Mean/stdev come from the draftable pool — the top
	// teams × (G+F+C+UTIL+BN) players by projected minutes — not every
	// projected body. Including deep-bench players would inflate each
	// category's stdev and compress the value of everyone who matters.
	draftableN := numTeams * (slots["G"] + slots["F"] + slots["C"] + slots["UTIL"] + slots["BN"])
	byMinutes := make([]nbaDraftPlayer, len(players))
	copy(byMinutes, players)
	sort.Slice(byMinutes, func(i, j int) bool { return byMinutes[i].Minutes > byMinutes[j].Minutes })
	if draftableN < 1 || draftableN > len(byMinutes) {
		draftableN = len(byMinutes)
	}
	pool := byMinutes[:draftableN]

	// FG%/FT% are volume-weighted: a player's value is the makes they add
	// over what a league-average shooter would make on the same attempts
	// (the roto-correct "impact"), not the raw percentage — a 60% shooter on
	// 2 attempts must not outrank a 52% shooter on 15. DARKO supplies
	// attempts, which is what makes this possible.
	var poolFGM, poolFGA, poolFTM, poolFTA float64
	for _, p := range pool {
		s := rawByPlayer[p.ESPNID]
		poolFGM += s.fgm
		poolFGA += s.fga
		poolFTM += s.ftm
		poolFTA += s.fta
	}
	var leagueFG, leagueFT float64
	if poolFGA > 0 {
		leagueFG = poolFGM / poolFGA
	}
	if poolFTA > 0 {
		leagueFT = poolFTM / poolFTA
	}
	zInput := func(s nbaDarkoStats, cat string) float64 {
		switch cat {
		case "fg_pct":
			return s.fgm - leagueFG*s.fga
		case "ft_pct":
			return s.ftm - leagueFT*s.fta
		}
		return s.value(cat)
	}

	type catAgg struct{ mean, stdev float64 }
	catStats := make(map[string]catAgg, len(nbaCategories))
	if n := float64(len(pool)); n > 0 {
		for _, cat := range nbaCategories {
			var sum float64
			for _, p := range pool {
				sum += zInput(rawByPlayer[p.ESPNID], cat)
			}
			mean := sum / n
			var sqDiff float64
			for _, p := range pool {
				d := zInput(rawByPlayer[p.ESPNID], cat) - mean
				sqDiff += d * d
			}
			catStats[cat] = catAgg{mean: mean, stdev: math.Sqrt(sqDiff / n)}
		}
	}

	// Fantasy value = z-score sum over only the categories this league
	// scores (league_settings.scoring).
	for i := range players {
		rs := rawByPlayer[players[i].ESPNID]
		players[i].Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
		players[i].ZScores = make([]nbaCategoryEntry, 0, len(nbaCategories))
		var total float64
		for _, cat := range nbaCategories {
			players[i].Stats = append(players[i].Stats, nbaCategoryEntry{Category: cat, Value: rs.value(cat)})
			agg := catStats[cat]
			var z float64
			if agg.stdev > 0 {
				z = (zInput(rs, cat) - agg.mean) / agg.stdev
				if cat == "turnovers" {
					z = -z
				}
			}
			players[i].ZScores = append(players[i].ZScores, nbaCategoryEntry{Category: cat, Value: z})
			if included[cat] {
				total += z
			}
		}
		players[i].FantasyValue = total
	}

	// Replacement levels: group by position, sorted descending by fantasy value.
	posGroups := map[string][]float64{}
	for _, p := range players {
		posGroups[p.Position] = append(posGroups[p.Position], p.FantasyValue)
	}
	for pos := range posGroups {
		sort.Slice(posGroups[pos], func(i, j int) bool { return posGroups[pos][i] > posGroups[pos][j] })
	}
	replLevels := nbaComputeReplacementLevels(posGroups, slots, numTeams)

	for i := range players {
		repl := replLevels[players[i].Position]
		vor := players[i].FantasyValue - repl.Value
		if vor < 0 {
			vor = 0
		}
		players[i].VOR = vor
	}

	// Auction $: $1 reserved per roster spot off the top (standard VBD
	// convention, docs/stats/auction-values.md), surplus split proportional
	// to compressed VOR share. Same formula/constant NFL uses — no
	// position-specific carve-out analogous to NFL's kickers is needed here.
	totalRosterSpots := 0
	for _, count := range slots {
		totalRosterSpots += count
	}
	totalRosterSpots *= numTeams
	totalBudget := float64(budget * numTeams)
	surplus := totalBudget - float64(totalRosterSpots)
	if surplus < 0 {
		surplus = 0
	}
	var totalCompressedVOR float64
	for _, p := range players {
		if p.VOR > 0 {
			totalCompressedVOR += math.Pow(p.VOR, nbaAuctionVORCompressionExponent)
		}
	}
	for i := range players {
		if totalCompressedVOR > 0 && players[i].VOR > 0 {
			compressed := math.Pow(players[i].VOR, nbaAuctionVORCompressionExponent)
			dollarVal := 1 + (compressed/totalCompressedVOR)*surplus
			players[i].AuctionValue = int(math.Max(1, math.Round(dollarVal)))
		} else {
			players[i].AuctionValue = 1
		}
	}

	// Rank by VOR descending (overall + within position).
	sort.Slice(players, func(i, j int) bool { return players[i].VOR > players[j].VOR })
	posRanks := map[string]int{}
	for i := range players {
		players[i].OverallRank = i + 1
		posRanks[players[i].Position]++
		players[i].PositionRank = posRanks[players[i].Position]
	}

	// Tiers within position — draftable range is the starter pool plus a
	// half, same convention draft_values.go uses, so the replacement-level
	// tail can't flatten the top of the board.
	byPositionForTiers := map[string][]tiers.Player{}
	for _, p := range players {
		byPositionForTiers[p.Position] = append(byPositionForTiers[p.Position], tiers.Player{ID: p.ESPNID, Value: p.FantasyValue})
	}
	tierByPlayer := map[string]int{}
	for pos, group := range byPositionForTiers {
		draftable := int(math.Ceil(float64(replLevels[pos].Threshold) * 1.5))
		for id, tier := range tiers.Assign(group, tiers.Options{Draftable: draftable}) {
			tierByPlayer[id] = tier
		}
	}
	for i := range players {
		players[i].Tier = tierByPlayer[players[i].ESPNID]
	}

	// Consensus $ — see the plan's Context section for why this joins to
	// nba_players.position rather than partitioning on the raw stored
	// position text the way NFL's loadConsensusValues does: external NBA
	// outlets publish PG/SG/SF/PF/C while our own vocabulary is the
	// ESPN-taxonomy G/F/C bucket, so the stored column doesn't line up
	// (confirmed empty on every imported row today).
	h.attachNBAConsensusValues(ctx, season, players, budget*numTeams)

	if players == nil {
		players = []nbaDraftPlayer{}
	}
	replResp := make([]nbaReplacementLevel, 0, len(replLevels))
	for _, pos := range nbaPositions {
		replResp = append(replResp, replLevels[pos])
	}

	respondJSON(w, http.StatusOK, nbaDraftValuesResp{
		Season:        season,
		BudgetPerTeam: budget,
		NumTeams:      numTeams,
		Settings: nbaDraftSettings{
			NumTeams:   numTeams,
			Budget:     budget,
			Slots:      slots,
			Scoring:    scoring,
			Overridden: overridden,
		},
		ReplacementLevels: replResp,
		Players:           players,
	})
}

// attachNBAConsensusValues fills in each player's consensus auction columns
// in place, preferring an imported market price (metric_type = 'auction',
// none exist yet — generic path, same as NFL's current state) over a value
// derived from the external consensus rank.
func (h *Handler) attachNBAConsensusValues(ctx context.Context, season int, players []nbaDraftPlayer, budgetPool int) {
	byID := make(map[string]int, len(players)) // espn_id -> index
	for i, p := range players {
		byID[p.ESPNID] = i
	}

	// --- 1. Imported auction prices, if any ---
	priced := map[string]bool{}
	priceRows, err := h.db.Query(ctx, `
		SELECT espn_id, value FROM nba_consensus_rankings
		WHERE season = $1 AND espn_id IS NOT NULL AND metric_type = 'auction'
	`, season)
	if err == nil {
		prices := map[string][]float64{}
		for priceRows.Next() {
			var espnID string
			var value float64
			if err := priceRows.Scan(&espnID, &value); err != nil {
				break
			}
			prices[espnID] = append(prices[espnID], value)
		}
		priceRows.Close()

		scale := float64(budgetPool) / float64(nbaMarketBudgetPool)
		for espnID, vals := range prices {
			i, ok := byID[espnID]
			if !ok {
				continue
			}
			dollars := int(medianOf(vals)*scale + 0.5)
			if dollars < 1 {
				dollars = 1
			}
			players[i].ConsensusAuctionValue = &dollars
			players[i].ConsensusSources = len(vals)
			priced[espnID] = true
		}
	}

	// --- 2. Consensus rank, for everyone without an imported price ---
	// RANK() partitions on our own resolved position (joined from
	// nba_players), not the raw stored text — see the handler doc comment.
	rankRows, err := h.db.Query(ctx, `
		SELECT ncr.espn_id, ncr.source,
		       RANK() OVER (PARTITION BY ncr.source, np.position ORDER BY ncr.value ASC) AS pos_rank
		FROM nba_consensus_rankings ncr
		JOIN nba_players np ON np.espn_id = ncr.espn_id
		WHERE ncr.season = $1 AND ncr.espn_id IS NOT NULL
		  AND ncr.metric_type IN ('rank', 'adp') AND np.position = ANY($2)
	`, season, nbaPositions)
	if err != nil {
		return
	}
	defer rankRows.Close()

	ranks := map[string][]float64{}
	for rankRows.Next() {
		var espnID, source string
		var posRank int
		if err := rankRows.Scan(&espnID, &source, &posRank); err != nil {
			return
		}
		ranks[espnID] = append(ranks[espnID], float64(posRank))
		if i, ok := byID[espnID]; ok {
			players[i].SourceRanks = append(players[i].SourceRanks, nbaConsensusSourceRank{Source: source, Rank: posRank})
		}
	}
	if rankRows.Err() != nil || len(ranks) == 0 {
		return
	}
	for i := range players {
		sort.Slice(players[i].SourceRanks, func(a, b int) bool {
			return players[i].SourceRanks[a].Source < players[i].SourceRanks[b].Source
		})
	}

	// Our own dollar value at each position rank — the curve the derived
	// value reads. Players are already position-ranked by the caller.
	byPositionRank := map[string][]int{}
	for _, p := range players {
		if p.PositionRank <= 0 {
			continue
		}
		for len(byPositionRank[p.Position]) < p.PositionRank {
			byPositionRank[p.Position] = append(byPositionRank[p.Position], 0)
		}
		byPositionRank[p.Position][p.PositionRank-1] = p.AuctionValue
	}

	for espnID, vals := range ranks {
		i, ok := byID[espnID]
		if !ok {
			continue
		}
		medianRank := medianOf(vals)
		if priced[espnID] {
			players[i].ConsensusPositionRank = &medianRank
			continue
		}
		curve := byPositionRank[players[i].Position]
		if len(curve) == 0 {
			continue
		}
		slot := int(medianRank + 0.5)
		if slot < 1 {
			slot = 1
		}
		if slot > len(curve) {
			slot = len(curve)
		}
		auction := curve[slot-1]
		players[i].ConsensusAuctionValue = &auction
		players[i].ConsensusPositionRank = &medianRank
		players[i].ConsensusSources = len(vals)
		players[i].ConsensusDerived = true
	}
}

// nbaDarkoRates is one row of nba_darko_projections: DARKO's per-100-possession
// rates plus the minutes and pace that convert them to per-game.
type nbaDarkoRates struct {
	minutes, pace                                          float64
	pts100, ast100, orb100, drb100, stl100, blk100, tov100 float64
	fga100, fg3a100, fta100, fgPct, fg3Pct, ftPct          float64
}

// nbaDarkoStats is a projected per-game line, including the raw makes and
// attempts the percentage categories need.
type nbaDarkoStats struct {
	points, rebounds, assists, steals, blocks, turnovers, threePM float64
	fgm, fga, ftm, fta                                            float64
}

// perGame converts per-100 rates to per-game: a player's possessions are the
// team's pace (per 48) scaled by the share of the game they play.
func (d nbaDarkoRates) perGame() nbaDarkoStats {
	poss := d.pace * d.minutes / 48 / 100
	fga := d.fga100 * poss
	fta := d.fta100 * poss
	return nbaDarkoStats{
		points:    d.pts100 * poss,
		rebounds:  (d.orb100 + d.drb100) * poss,
		assists:   d.ast100 * poss,
		steals:    d.stl100 * poss,
		blocks:    d.blk100 * poss,
		turnovers: d.tov100 * poss,
		threePM:   d.fg3a100 * d.fg3Pct * poss,
		fgm:       fga * d.fgPct,
		fga:       fga,
		ftm:       fta * d.ftPct,
		fta:       fta,
	}
}

// value is the displayed per-game number for a category (the percentage
// categories show the raw percentage, not the z-score input).
func (s nbaDarkoStats) value(cat string) float64 {
	switch cat {
	case "points":
		return s.points
	case "rebounds":
		return s.rebounds
	case "assists":
		return s.assists
	case "steals":
		return s.steals
	case "blocks":
		return s.blocks
	case "turnovers":
		return s.turnovers
	case "three_pm":
		return s.threePM
	case "fg_pct":
		if s.fga > 0 {
			return s.fgm / s.fga
		}
	case "ft_pct":
		if s.fta > 0 {
			return s.ftm / s.fta
		}
	}
	return 0
}
