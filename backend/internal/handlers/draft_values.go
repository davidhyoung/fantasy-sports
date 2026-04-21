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
	GsisID         string             `json:"gsis_id"`
	Name           string             `json:"name"`
	Position       string             `json:"position"`
	PositionGroup  string             `json:"position_group"`
	Team           string             `json:"team"`
	HeadshotURL    string             `json:"headshot_url"`
	Age            int                `json:"age"`
	ProjFpts       float64            `json:"proj_fpts"`
	ProjFptsPPR    float64            `json:"proj_fpts_ppr"`
	ProjFptsHalf   float64            `json:"proj_fpts_half"`
	ProjFptsPPRPG  float64            `json:"proj_fpts_ppr_pg"`
	ProjLeagueFpts float64            `json:"proj_league_fpts"`
	Confidence     float64            `json:"confidence"`
	CompCount      int                `json:"comp_count"`
	Uniqueness     string             `json:"uniqueness"`
	VOR            float64            `json:"vor"`
	AuctionValue   int                `json:"auction_value"`
	OverallRank    int                `json:"overall_rank"`
	PositionRank   int                `json:"position_rank"`
	Trajectory     []trajectoryPoint  `json:"trajectory,omitempty"`
	PlayerGrade    *float64           `json:"player_grade"`
}

type draftValuesResp struct {
	Season            int                `json:"season"`
	BudgetPerTeam     int                `json:"budget_per_team"`
	NumTeams          int                `json:"num_teams"`
	ScoringFormat     string             `json:"scoring_format"`
	ReplacementLevels []replacementLevel `json:"replacement_levels"`
	Players           []draftPlayer      `json:"players"`
}

// GetDraftValues returns projected players with league-specific auction draft values.
//
// GET /api/leagues/{id}/draft-values?season=2026&budget=200
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
	if b := q.Get("budget"); b != "" {
		if v, err := strconv.Atoi(b); err == nil && v > 0 {
			budget = v
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
		log.Printf("[draft-values] GetLeagueRosterPositions %s: %v", yahooKey, rpr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch league roster settings")
		return
	}

	sr := <-scoringCh
	if sr.err != nil {
		log.Printf("[draft-values] GetLeagueScoringStats %s: %v (falling back to PPR)", yahooKey, sr.err)
	}
	scoringStats := sr.stats

	// 3. Compute starter slots per position using ranking service
	rankingPositions := make([]ranking.RosterPosition, len(rpr.positions))
	for i, rp := range rpr.positions {
		rankingPositions[i] = ranking.RosterPosition{Position: rp.Position, Count: rp.Count}
	}
	starterSlots := ranking.ComputeStarterSlots(rankingPositions)

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

	// Translate Yahoo scoring → canonical-keyed modifiers once, for kicker scoring below.
	yahooMods := make(map[string]float64, len(scoringStats))
	for id, ls := range scoringStats {
		yahooMods[id] = ls.Modifier
	}
	canonicalMods := scoring.CanonicalModifiersFromYahoo(yahooMods)

	var players []draftPlayer
	for rows.Next() {
		var dp draftPlayer
		var (
			passYdsPG, passTdPG             float64
			rushYdsPG, rushTdPG             float64
			recPG, recYdsPG, recTdPG        float64
			fgMadePG, patMadePG             float64
			games                           int
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

		players = append(players, dp)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 5. Compute replacement levels per position (using league-specific points)
	posPlayers := make(map[string][]float64)
	for i := range players {
		pos := primaryPosition(players[i].PositionGroup)
		posPlayers[pos] = append(posPlayers[pos], players[i].ProjLeagueFpts)
	}
	for pos := range posPlayers {
		sort.Sort(sort.Reverse(sort.Float64Slice(posPlayers[pos])))
	}

	replLevels := make(map[string]float64)
	var replResp []replacementLevel
	for pos, slots := range starterSlots {
		threshold := int(math.Ceil(slots * float64(numTeams)))
		pts := posPlayers[pos]
		var replPts float64
		if threshold >= len(pts) {
			if len(pts) > 0 {
				replPts = pts[len(pts)-1]
			}
		} else {
			replPts = pts[threshold]
		}
		replLevels[pos] = replPts
		replResp = append(replResp, replacementLevel{
			Position:     pos,
			StarterSlots: slots,
			Threshold:    threshold,
			Points:       replPts,
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

	// 7. Compute auction values: proportional share of total budget
	totalBudget := float64(budget * numTeams)
	for i := range players {
		if totalVOR > 0 && players[i].VOR > 0 {
			dollarVal := (players[i].VOR / totalVOR) * totalBudget
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

	// 9. Attach year-over-year trajectory from season profiles
	gsisIDs := make([]string, len(players))
	for i, p := range players {
		gsisIDs[i] = p.GsisID
	}
	trajectories := h.loadPlayerTrajectories(r.Context(), gsisIDs)
	for i := range players {
		players[i].Trajectory = trajectories[players[i].GsisID]
	}

	respondJSON(w, http.StatusOK, draftValuesResp{
		Season:            season,
		BudgetPerTeam:     budget,
		NumTeams:          numTeams,
		ScoringFormat:     scoringFormat,
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
