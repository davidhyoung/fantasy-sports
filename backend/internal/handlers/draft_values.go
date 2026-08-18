package handlers

import (
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/davidyoung/fantasy-sports/backend/internal/aging"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/ranking"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/tiers"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// Yahoo reception stat ID — still referenced directly to detect PPR/Half/Standard
// from the league scoring modifier. Other stat-ID mappings live in services/scoring.
const yahooStatIDRec = "11"

// ── response types ────────────────────────────────────────────────────────────

type replacementLevel struct {
	Position     string  `json:"position"`
	StarterSlots float64 `json:"starter_slots"`
	Threshold    int     `json:"threshold"`
	Points       float64 `json:"points"`
}

type draftPlayer struct {
	GsisID         string  `json:"gsis_id"`
	Name           string  `json:"name"`
	Position       string  `json:"position"`
	PositionGroup  string  `json:"position_group"`
	Team           string  `json:"team"`
	HeadshotURL    string  `json:"headshot_url"`
	Age            int     `json:"age"`
	ProjFpts       float64 `json:"proj_fpts"`
	ProjFptsPPR    float64 `json:"proj_fpts_ppr"`
	ProjFptsHalf   float64 `json:"proj_fpts_half"`
	ProjFptsPPRPG  float64 `json:"proj_fpts_ppr_pg"`
	ProjLeagueFpts float64 `json:"proj_league_fpts"`
	Confidence     float64 `json:"confidence"`
	CompCount      int     `json:"comp_count"`
	Uniqueness     string  `json:"uniqueness"`
	// League-scoring per-game figure, so PPG on the board matches Proj Pts
	// instead of a fixed full-PPR number meant for cross-league comparison.
	ProjLeaguePPG  float64 `json:"proj_league_ppg"`
	VOR            float64 `json:"vor"`
	AuctionValue   int     `json:"auction_value"`
	OverallRank    int     `json:"overall_rank"`
	PositionRank   int     `json:"position_rank"`
	// Tier groups players at the same position whose value is close enough that
	// the choice between them is a coin flip. 1 = best tier.
	Tier int `json:"tier"`
	// Consensus columns are nil where no external source covers the player —
	// coverage runs to roughly the top 100 picks, not the whole pool.
	ConsensusAuctionValue *int              `json:"consensus_auction_value"`
	ConsensusPositionRank *float64          `json:"consensus_position_rank"`
	ConsensusSources      int               `json:"consensus_sources"`
	ConsensusDerived      bool              `json:"consensus_derived"`
	Trajectory            []trajectoryPoint `json:"trajectory,omitempty"`
	PlayerGrade           *float64          `json:"player_grade"`
}

// draftSettings echoes the settings a board was computed with, in the same
// editable vocabulary the client sends back as overrides. Returning it means the
// UI never has to reverse-engineer roster slots from the distributed values.
type draftSettings struct {
	NumTeams int            `json:"num_teams"`
	Budget   int            `json:"budget"`
	Format   string         `json:"format"`
	Slots    map[string]int `json:"slots"`
	// Points per stat category — the "pointing system" the settings panel edits.
	// Keyed by the same canonical stat names the `scoring=` override uses.
	Scoring map[string]float64 `json:"scoring"`
	// Overridden reports whether any setting came from the query rather than the league.
	Overridden bool `json:"overridden"`
}

type draftValuesResp struct {
	Season            int                `json:"season"`
	BudgetPerTeam     int                `json:"budget_per_team"`
	NumTeams          int                `json:"num_teams"`
	ScoringFormat     string             `json:"scoring_format"`
	Settings          draftSettings      `json:"settings"`
	ReplacementLevels []replacementLevel `json:"replacement_levels"`
	Players           []draftPlayer      `json:"players"`
}

// ── roster-slot vocabulary ────────────────────────────────────────────────────
//
// The client edits a flat set of slot names; Yahoo expresses flex spots as
// slash-separated eligibility lists. These two helpers translate between them so
// the flex distribution itself stays in ranking.ComputeStarterSlots.

// slotToYahoo maps an editable slot name to the Yahoo roster position it stands for.
var slotToYahoo = map[string]string{
	"QB": "QB", "RB": "RB", "WR": "WR", "TE": "TE", "K": "K", "DEF": "DEF",
	"FLEX":  "W/R/T",
	"SFLEX": "Q/W/R/T",
	// Bench starts nobody, so ComputeStarterSlots ignores it and it cannot affect
	// replacement levels or prices. It is carried anyway because roster size is
	// what tells the team builder how many $1 slots a budget still has to cover.
	"BN": "BN",
}

// yahooToSlot maps a Yahoo roster position back to an editable slot name.
// Bench and injury slots return false — they don't start anyone. Flex shapes we
// don't model exactly (e.g. "W/R") fold into FLEX, which is what the UI offers.
func yahooToSlot(pos string) (string, bool) {
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

// slotsFromYahoo summarises a league's Yahoo roster in the editable vocabulary.
func slotsFromYahoo(positions []yahoo.RosterPosition) map[string]int {
	slots := map[string]int{}
	for _, rp := range positions {
		if name, ok := yahooToSlot(rp.Position); ok {
			slots[name] += rp.Count
		}
	}
	return slots
}

// parseSlotOverride reads a `slots=QB:1,RB:2,FLEX:1,SFLEX:1` override into both
// the editable map and the Yahoo-shaped positions ComputeStarterSlots expects.
func parseSlotOverride(raw string) (map[string]int, []ranking.RosterPosition, bool) {
	slots := map[string]int{}
	var positions []ranking.RosterPosition
	for _, part := range strings.Split(raw, ",") {
		name, countStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		name = strings.ToUpper(strings.TrimSpace(name))
		yahooPos, known := slotToYahoo[name]
		if !known {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil || count < 0 || count > 20 {
			continue
		}
		slots[name] = count
		if count > 0 {
			positions = append(positions, ranking.RosterPosition{Position: yahooPos, Count: count})
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
var scoringEditableStats = []scoring.CanonicalStat{
	scoring.StatPassYds, scoring.StatPassTD,
	scoring.StatRushYds, scoring.StatRushTD,
	scoring.StatRec, scoring.StatRecYds, scoring.StatRecTD,
	scoring.StatFGMade, scoring.StatPATMade,
}

var scoringEditableSet = func() map[scoring.CanonicalStat]bool {
	m := make(map[scoring.CanonicalStat]bool, len(scoringEditableStats))
	for _, s := range scoringEditableStats {
		m[s] = true
	}
	return m
}()

// defaultScoringFallback seeds the pointing system when a league's own Yahoo
// scoring couldn't be fetched — a standard full-PPR point set, matching the
// "ppr" fallback used elsewhere in this handler.
var defaultScoringFallback = map[scoring.CanonicalStat]float64{
	scoring.StatPassYds: 0.04, scoring.StatPassTD: 4,
	scoring.StatRushYds: 0.1, scoring.StatRushTD: 6,
	scoring.StatRec: 1, scoring.StatRecYds: 0.1, scoring.StatRecTD: 6,
	scoring.StatFGMade: 3, scoring.StatPATMade: 1,
}

// parseScoringOverride reads a `scoring=pass_yds:0.04,pass_td:4,...` override
// into canonical-stat-keyed point values. Unknown keys and unparsable values
// are dropped rather than rejecting the whole override, same convention as
// parseSlotOverride.
func parseScoringOverride(raw string) (map[scoring.CanonicalStat]float64, bool) {
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

// GetDraftValues returns projected players with league-specific auction draft values.
//
// GET /api/leagues/{id}/draft-values?season=2026&budget=200
//
// Every setting the scoring depends on can be overridden, so the client can ask
// "what would this board look like as a 14-team superflex PPR league?" without a
// second implementation of the draft math:
//
//	season  target projection season
//	budget  auction dollars per team
//	teams   team count (drives replacement levels and the money pool)
//	format  league|ppr|half|standard — league keeps the league's own scoring
//	slots   roster override, e.g. QB:1,RB:2,WR:3,TE:1,FLEX:1,SFLEX:1,K:1,DEF:1
//	scoring per-stat point values, e.g. pass_yds:0.04,pass_td:4,rec:1 — present
//	        for any of the categories in scoringEditableStats; when given it
//	        replaces format entirely and prices every position (not just
//	        kickers) from these exact weights
//
// Anything omitted falls back to the league's real settings. The response echoes
// what was used in `settings`, in the same vocabulary, so the client never has to
// reverse-engineer it.
func (h *Handler) GetDraftValues(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	yahooKey, status, msg := h.leagueYahooKey(r, leagueID)
	if status != 0 {
		respondError(w, status, msg)
		return
	}

	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	budget := h.config.DefaultBudget
	overridden := false
	if b := q.Get("budget"); b != "" {
		if v, err := strconv.Atoi(b); err == nil && v > 0 && v <= 100000 {
			budget = v
			overridden = true
		}
	}

	// Scoring override: "league" (or absent) keeps the league's own reception
	// scoring; the rest force a format so "what if we went full PPR" is answerable.
	formatOverride := ""
	switch f := strings.ToLower(q.Get("format")); f {
	case "ppr", "half", "half_ppr", "standard":
		formatOverride = strings.TrimSuffix(f, "_ppr")
		overridden = true
	}

	slotsOverride, rosterOverride, hasSlotsOverride := map[string]int{}, []ranking.RosterPosition(nil), false
	if s := q.Get("slots"); s != "" {
		slotsOverride, rosterOverride, hasSlotsOverride = parseSlotOverride(s)
		if hasSlotsOverride {
			overridden = true
		}
	}

	scoringOverride, hasScoringOverride := map[scoring.CanonicalStat]float64{}, false
	if s := q.Get("scoring"); s != "" {
		scoringOverride, hasScoringOverride = parseScoringOverride(s)
		if hasScoringOverride {
			overridden = true
		}
	}

	// 1. Get number of teams in league
	var numTeams int
	if err := h.db.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM teams WHERE league_id = $1", leagueID,
	).Scan(&numTeams); err != nil || numTeams == 0 {
		respondError(w, http.StatusUnprocessableEntity, "no teams found — sync your league first")
		return
	}
	// Team-count override changes replacement levels and the total money pool.
	if t := q.Get("teams"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v >= 2 && v <= 32 {
			numTeams = v
			overridden = true
		}
	}

	// 2. Build Yahoo client and fetch roster positions + scoring stats concurrently.
	//    Both calls hit the same /settings endpoint but are kept separate for clarity.
	yc, err := h.newYahooClient(r, user)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "auth error")
		return
	}

	type rosterPosResult struct {
		positions []yahoo.RosterPosition
		err       error
	}
	type scoringResult struct {
		stats map[string]yahoo.LeagueStat
		err   error
	}

	rosterPosCh := make(chan rosterPosResult, 1)
	scoringCh := make(chan scoringResult, 1)

	go func() {
		pos, err := yc.GetLeagueRosterPositions(r.Context(), yahooKey)
		rosterPosCh <- rosterPosResult{pos, err}
	}()
	go func() {
		stats, err := yc.GetLeagueScoringStats(r.Context(), yahooKey)
		scoringCh <- scoringResult{stats, err}
	}()

	rpr := <-rosterPosCh
	if rpr.err != nil {
		// With an explicit roster override we don't need Yahoo's roster at all, so a
		// failure there is only fatal when we'd otherwise have no starter slots.
		log.Printf("[draft-values] GetLeagueRosterPositions %s: %v (slots override: %t)", yahooKey, rpr.err, hasSlotsOverride)
		if !hasSlotsOverride {
			respondError(w, http.StatusBadGateway, "failed to fetch league roster settings")
			return
		}
	}

	sr := <-scoringCh
	if sr.err != nil {
		log.Printf("[draft-values] GetLeagueScoringStats %s: %v (falling back to PPR)", yahooKey, sr.err)
	}
	scoringStats := sr.stats

	// 3. Resolve roster positions using ranking service. An override replaces
	//    the league's roster wholesale; the flex/superflex pooling itself stays
	//    in ranking.ComputeReplacementLevels either way, so both paths score
	//    identically.
	rankingPositions := rosterOverride
	effectiveSlots := slotsOverride
	if !hasSlotsOverride {
		rankingPositions = make([]ranking.RosterPosition, len(rpr.positions))
		for i, rp := range rpr.positions {
			rankingPositions[i] = ranking.RosterPosition{Position: rp.Position, Count: rp.Count}
		}
		effectiveSlots = slotsFromYahoo(rpr.positions)
	}

	// 4. Load projections from DB (per-game rates + season totals for fallback display)
	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.gsis_id, p.name,
			COALESCE(p.position, '') AS position,
			COALESCE(p.position_group, '') AS position_group,
			COALESCE(p.team, '') AS team,
			COALESCE(p.headshot_url, '') AS headshot_url,
			COALESCE(prof.age, 0) AS age,
			pr.proj_fpts, pr.proj_fpts_ppr, pr.proj_fpts_half, pr.proj_fpts_ppr_pg,
			pr.proj_pass_yds_pg, pr.proj_pass_td_pg,
			pr.proj_rush_yds_pg, pr.proj_rush_td_pg,
			pr.proj_rec_pg, pr.proj_rec_yds_pg, pr.proj_rec_td_pg,
			pr.proj_fg_made_pg, pr.proj_pat_made_pg,
			pr.proj_games,
			pr.confidence, pr.comp_count, pr.uniqueness,
			g.overall AS player_grade
		FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		LEFT JOIN nfl_player_season_profiles prof
		       ON prof.gsis_id = pr.gsis_id AND prof.season = pr.base_season
		LEFT JOIN nfl_player_grades g
		       ON g.gsis_id = pr.gsis_id AND g.season = pr.base_season
		WHERE pr.target_season = $1
		ORDER BY pr.proj_fpts_ppr DESC
	`, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	// Determine the effective scoring format from the league's reception modifier.
	// The generic format totals (proj_fpts_ppr/half/fpts) are reliable; per-stat
	// rate columns can be inconsistent with them, so we use the generic total that
	// most closely matches the league's actual scoring as the base for skill players.
	// Kickers have no generic projection, so we compute theirs from per-stat rates.
	recMod := scoringStats[yahooStatIDRec].Modifier // 1.0=PPR, 0.5=half, 0=standard
	var effectiveFormat string
	switch {
	case recMod >= 0.9:
		effectiveFormat = "ppr"
	case recMod >= 0.35:
		effectiveFormat = "half"
	default:
		effectiveFormat = "standard"
	}

	hasLeagueScoring := len(scoringStats) > 0
	scoringFormat := effectiveFormat
	if !hasLeagueScoring {
		scoringFormat = "ppr"
		effectiveFormat = "ppr"
	}
	// An explicit format override wins. Kickers keep scoring off the league's own
	// modifiers below — reception rules can't move a kicker either way.
	if formatOverride != "" {
		effectiveFormat = formatOverride
		scoringFormat = formatOverride
	}

	// Translate Yahoo scoring → canonical-keyed modifiers once, for kicker scoring below.
	yahooMods := make(map[string]float64, len(scoringStats))
	for id, ls := range scoringStats {
		yahooMods[id] = ls.Modifier
	}
	canonicalMods := scoring.CanonicalModifiersFromYahoo(yahooMods)

	// The pointing system: default point values come from the league's own
	// translated Yahoo scoring (FG's distance buckets collapse into one flat
	// average, weighted by FGDistribution, since the panel edits a single FG
	// value rather than five), falling back to a standard PPR set when the
	// league's scoring couldn't be fetched. A `scoring=` override replaces
	// individual categories on top of that.
	defaultScoring := map[scoring.CanonicalStat]float64{}
	if hasLeagueScoring {
		var avgFGMod float64
		for bucket, share := range scoring.FGDistribution {
			avgFGMod += share * canonicalMods[bucket]
		}
		defaultScoring[scoring.StatPassYds] = canonicalMods[scoring.StatPassYds]
		defaultScoring[scoring.StatPassTD] = canonicalMods[scoring.StatPassTD]
		defaultScoring[scoring.StatRushYds] = canonicalMods[scoring.StatRushYds]
		defaultScoring[scoring.StatRushTD] = canonicalMods[scoring.StatRushTD]
		defaultScoring[scoring.StatRec] = canonicalMods[scoring.StatRec]
		defaultScoring[scoring.StatRecYds] = canonicalMods[scoring.StatRecYds]
		defaultScoring[scoring.StatRecTD] = canonicalMods[scoring.StatRecTD]
		defaultScoring[scoring.StatFGMade] = avgFGMod
		defaultScoring[scoring.StatPATMade] = canonicalMods[scoring.StatPATMade]
	} else {
		for k, v := range defaultScoringFallback {
			defaultScoring[k] = v
		}
	}
	activeScoring := map[scoring.CanonicalStat]float64{}
	for k, v := range defaultScoring {
		activeScoring[k] = v
	}
	for k, v := range scoringOverride {
		activeScoring[k] = v
	}

	// A scoring override replaces the format entirely — "custom" reports that
	// honestly rather than reusing the reception-derived ppr/half/standard label.
	if hasScoringOverride {
		scoringFormat = "custom"
	}

	// Consensus data only comes bucketed as ppr/half_ppr/standard, so custom
	// scoring still needs a bucket to diff against; approximate it from the
	// reception point value the same way the league's own scoring is classified.
	consensusFormat := effectiveFormat
	if hasScoringOverride {
		switch {
		case activeScoring[scoring.StatRec] >= 0.9:
			consensusFormat = "ppr"
		case activeScoring[scoring.StatRec] >= 0.35:
			consensusFormat = "half"
		default:
			consensusFormat = "standard"
		}
	}

	var players []draftPlayer
	for rows.Next() {
		var dp draftPlayer
		var (
			passYdsPG, passTdPG      float64
			rushYdsPG, rushTdPG      float64
			recPG, recYdsPG, recTdPG float64
			fgMadePG, patMadePG      float64
			games                    int
		)
		if err := rows.Scan(
			&dp.GsisID, &dp.Name, &dp.Position, &dp.PositionGroup, &dp.Team, &dp.HeadshotURL,
			&dp.Age,
			&dp.ProjFpts, &dp.ProjFptsPPR, &dp.ProjFptsHalf, &dp.ProjFptsPPRPG,
			&passYdsPG, &passTdPG,
			&rushYdsPG, &rushTdPG,
			&recPG, &recYdsPG, &recTdPG,
			&fgMadePG, &patMadePG,
			&games,
			&dp.Confidence, &dp.CompCount, &dp.Uniqueness,
			&dp.PlayerGrade,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		switch {
		case hasScoringOverride:
			// A custom pointing system prices every position — not just kickers —
			// from the same rate-based math kickers always used, just with
			// user-supplied weights instead of the league's Yahoo modifiers.
			totals := scoring.ProjectionToCanonicalTotals(scoring.ProjectionRates{
				PassYdsPG: passYdsPG, PassTdPG: passTdPG,
				RushYdsPG: rushYdsPG, RushTdPG: rushTdPG,
				RecPG: recPG, RecYdsPG: recYdsPG, RecTdPG: recTdPG,
				FgMadePG: fgMadePG, PatMadePG: patMadePG,
			}, float64(games))
			dp.ProjLeagueFpts = scoring.ScoreWithModifiers(totals, activeScoring)
		case dp.PositionGroup == "K":
			// Kickers have no reliable generic projection total; compute from
			// per-game rates × league modifiers. Cap unrealistic values (bad data).
			if hasLeagueScoring {
				totals := scoring.ProjectionToCanonicalTotals(scoring.ProjectionRates{
					PassYdsPG: passYdsPG, PassTdPG: passTdPG,
					RushYdsPG: rushYdsPG, RushTdPG: rushTdPG,
					RecPG: recPG, RecYdsPG: recYdsPG, RecTdPG: recTdPG,
					FgMadePG: fgMadePG, PatMadePG: patMadePG,
				}, float64(games))
				dp.ProjLeagueFpts = scoring.ScoreWithModifiers(totals, canonicalMods)
				// Sanity cap: top kickers score ~150–175 pts in a normal season.
				// If the computed value exceeds this, the per-game rate data is bad.
				if dp.ProjLeagueFpts > 180 {
					dp.ProjLeagueFpts = 0
				}
			}
		default:
			// Skill positions: use the reliable generic format total that best
			// matches the league's reception scoring. Per-stat rate columns are
			// often inconsistent with the stored totals and cannot be trusted.
			switch effectiveFormat {
			case "ppr":
				dp.ProjLeagueFpts = dp.ProjFptsPPR
			case "half":
				dp.ProjLeagueFpts = dp.ProjFptsHalf
			default:
				dp.ProjLeagueFpts = dp.ProjFpts
			}
		}
		if games > 0 {
			dp.ProjLeaguePPG = dp.ProjLeagueFpts / float64(games)
		}

		players = append(players, dp)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 5. Compute replacement levels per position (using league-specific points).
	//    Flex/superflex slots pool their eligible candidates by value rather than
	//    splitting evenly — an even split undercounts QB demand badly in
	//    superflex leagues, since QB dwarfs WR/RB/TE per game in points formats.
	posGroupsForRepl := make(map[string][]ranking.PlayerData)
	for i := range players {
		pos := primaryPosition(players[i].PositionGroup)
		posGroupsForRepl[pos] = append(posGroupsForRepl[pos], ranking.PlayerData{
			PrimaryPos:  pos,
			TotalPoints: players[i].ProjLeagueFpts,
		})
	}
	for pos := range posGroupsForRepl {
		sort.Slice(posGroupsForRepl[pos], func(i, j int) bool {
			return posGroupsForRepl[pos][i].TotalPoints > posGroupsForRepl[pos][j].TotalPoints
		})
	}

	rankLevels := ranking.ComputeReplacementLevels(rankingPositions, posGroupsForRepl, numTeams)

	replLevels := make(map[string]float64)
	var replResp []replacementLevel
	for pos, rl := range rankLevels {
		replLevels[pos] = rl.Points
		replResp = append(replResp, replacementLevel{
			Position:     pos,
			StarterSlots: float64(rl.Threshold) / float64(numTeams),
			Threshold:    rl.Threshold,
			Points:       rl.Points,
		})
	}
	sort.Slice(replResp, func(i, j int) bool {
		return replResp[i].Position < replResp[j].Position
	})

	// 6. Compute VOR for each player (with age-based draft value adjustment)
	draftMult := aging.DefaultDraftMultipliers
	var totalVOR float64
	for i := range players {
		pos := primaryPosition(players[i].PositionGroup)
		vor := players[i].ProjLeagueFpts - replLevels[pos]
		if vor < 0 {
			vor = 0
		}
		vor *= draftMult.Multiplier(players[i].PositionGroup, players[i].Age)
		players[i].VOR = vor
		totalVOR += vor
	}

	// 7. Compute auction values: $1 reserved for every roster spot in the league
	//    (every drafted player costs at least a dollar, VOR-positive or not — the
	//    standard VBD convention this is meant to follow, docs/stats/auction-values.md),
	//    then the surplus above that split proportionally by VOR. Skipping the
	//    reservation and instead splitting the *full* nominal budget across only
	//    the VOR-positive players — the previous behavior — systematically
	//    overpriced the top of the board: every $1-floor bench spot was extra
	//    money the formula never set aside, so the league's true spendable total
	//    was smaller than what got divided among the players who actually earned
	//    a share.
	totalRosterSpots := 0
	for _, count := range effectiveSlots {
		totalRosterSpots += count
	}
	totalRosterSpots *= numTeams
	totalBudget := float64(budget * numTeams)
	surplus := totalBudget - float64(totalRosterSpots)
	if surplus < 0 {
		surplus = 0
	}

	// Raw VOR share is linear, which real auction rooms aren't: nobody actually
	// bids 40-50% of a $200 budget on one player even when the points say they
	// "should", and shallow single-QB rosters (few flex-eligible starter slots,
	// e.g. Yahoo's RB2/WR2/FLEX1 default) concentrate VOR onto so few players
	// that linear sharing pushes the very top of the board past $100 — well
	// above what real bidders pay (observed ceiling: roughly $70-80 for the
	// consensus #1 overall in a 12-team/$200 league). Raising VOR to a <1
	// exponent before sharing compresses that spread — the top of the curve
	// gives up relatively more than the middle, both in dollars and in the
	// share of the pool it can command — while leaving the ranking (and the
	// zero/non-zero VOR boundary) untouched, since x^p is monotonic for x>0.
	// 0.75 was picked empirically against this league's board to land the top
	// price in that observed real-world range; it isn't tuned per-league.
	const auctionVORCompressionExponent = 0.75

	// Kickers are excluded from the shared VOR pool and priced separately,
	// capped to a $1-3 band. Real auction rooms don't bid up a kicker for
	// projected points the way they do skill positions — kicker output is
	// famously the least predictable in fantasy football, so a real bidder
	// discounts kicker VOR far more aggressively than the shared x^0.75
	// compression does. Sharing the same pool also meant kicker VOR — once
	// scored correctly — quietly took dollars away from every other position's
	// share of the surplus. $1-3 mirrors what kickers actually go for in a
	// live 12-team draft regardless of the point spread underneath.
	var maxKickerVOR float64
	for i := range players {
		if primaryPosition(players[i].PositionGroup) == "K" && players[i].VOR > maxKickerVOR {
			maxKickerVOR = players[i].VOR
		}
	}

	var totalCompressedVOR float64
	for i := range players {
		if primaryPosition(players[i].PositionGroup) == "K" {
			continue
		}
		if players[i].VOR > 0 {
			totalCompressedVOR += math.Pow(players[i].VOR, auctionVORCompressionExponent)
		}
	}
	for i := range players {
		if primaryPosition(players[i].PositionGroup) == "K" {
			if maxKickerVOR > 0 && players[i].VOR > 0 {
				players[i].AuctionValue = 1 + int(math.Round(2*players[i].VOR/maxKickerVOR))
			} else {
				players[i].AuctionValue = 1
			}
			continue
		}
		if totalCompressedVOR > 0 && players[i].VOR > 0 {
			compressed := math.Pow(players[i].VOR, auctionVORCompressionExponent)
			dollarVal := 1 + (compressed/totalCompressedVOR)*surplus
			players[i].AuctionValue = int(math.Max(1, math.Round(dollarVal)))
		} else {
			players[i].AuctionValue = 1
		}
	}

	// 8. Sort by VOR descending, assign ranks
	sort.Slice(players, func(i, j int) bool {
		return players[i].VOR > players[j].VOR
	})
	posRanks := make(map[string]int)
	for i := range players {
		players[i].OverallRank = i + 1
		pos := primaryPosition(players[i].PositionGroup)
		posRanks[pos]++
		players[i].PositionRank = posRanks[pos]
	}

	// 9. Tier players within position: a tier answers "if I wait, do I still get
	//    someone like this?", which only means anything among players who compete
	//    for the same roster spot. The draftable range is the starter pool plus a
	//    half, so the replacement-level tail can't flatten the top of the board.
	byPositionForTiers := map[string][]tiers.Player{}
	for _, p := range players {
		pos := primaryPosition(p.PositionGroup)
		byPositionForTiers[pos] = append(byPositionForTiers[pos], tiers.Player{
			ID: p.GsisID, Value: p.ProjLeagueFpts,
		})
	}
	tierByPlayer := map[string]int{}
	for pos, group := range byPositionForTiers {
		draftable := int(math.Ceil(float64(rankLevels[pos].Threshold) * 1.5))
		for id, tier := range tiers.Assign(group, tiers.Options{Draftable: draftable}) {
			tierByPlayer[id] = tier
		}
	}
	for i := range players {
		players[i].Tier = tierByPlayer[players[i].GsisID]
	}

	// 10. Attach consensus values — what the market pays for the same players, on
	//    this league's dollar scale. Runs after ranking, since the derived form
	//    reads our own value-per-position-rank curve.
	consensus := h.loadConsensusValues(r.Context(), season, consensusFormat, players, budget*numTeams)
	for i := range players {
		cv, ok := consensus[players[i].GsisID]
		if !ok {
			continue
		}
		auction, posRank := cv.Auction, cv.PositionRank
		players[i].ConsensusAuctionValue = &auction
		if posRank > 0 {
			players[i].ConsensusPositionRank = &posRank
		}
		players[i].ConsensusSources = cv.SourceCount
		players[i].ConsensusDerived = cv.Derived
	}

	// 11. Attach year-over-year trajectory from season profiles
	gsisIDs := make([]string, len(players))
	for i, p := range players {
		gsisIDs[i] = p.GsisID
	}
	trajectories := h.loadPlayerTrajectories(r.Context(), gsisIDs)
	for i := range players {
		players[i].Trajectory = trajectories[players[i].GsisID]
	}

	// A season with no projections yields nil slices, which marshal to JSON `null`.
	// The client expects arrays, so normalise to empty.
	if players == nil {
		players = []draftPlayer{}
	}
	if replResp == nil {
		replResp = []replacementLevel{}
	}

	scoringOut := make(map[string]float64, len(scoringEditableStats))
	for _, k := range scoringEditableStats {
		scoringOut[string(k)] = activeScoring[k]
	}

	respondJSON(w, http.StatusOK, draftValuesResp{
		Season:        season,
		BudgetPerTeam: budget,
		NumTeams:      numTeams,
		ScoringFormat: scoringFormat,
		Settings: draftSettings{
			NumTeams:   numTeams,
			Budget:     budget,
			Format:     scoringFormat,
			Slots:      effectiveSlots,
			Scoring:    scoringOut,
			Overridden: overridden,
		},
		ReplacementLevels: replResp,
		Players:           players,
	})
}

// primaryPosition extracts the first/primary position from a position group string.
func primaryPosition(posGroup string) string {
	if i := strings.Index(posGroup, ","); i >= 0 {
		return posGroup[:i]
	}
	return posGroup
}
