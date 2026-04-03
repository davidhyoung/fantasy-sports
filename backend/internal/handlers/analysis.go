package handlers

import (
	"context"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/players"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/ranking"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// --- Response types ---

type categoryStatsResp struct {
	Label     string  `json:"label"`
	SortOrder string  `json:"sort_order"`
	Mean      float64 `json:"mean"`
	Stdev     float64 `json:"stdev"`
	Weight    float64 `json:"weight"` // mean-normalised; 1.0 = average category weight
}

type playerCategoryScore struct {
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	ZScore     float64 `json:"z_score"`
	Percentile int     `json:"percentile"`
}

type trajectoryPoint struct {
	Season    int     `json:"season"`
	FptsPPRPG float64 `json:"fpts_ppr_pg"`
}

type rankedPlayerResp struct {
	PlayerKey      string                `json:"player_key"`
	GsisID         string                `json:"gsis_id,omitempty"` // set when yahoo_id can be resolved to NFL player
	Name           string                `json:"name"`
	HeadshotURL    string                `json:"headshot_url,omitempty"`
	Position       string                `json:"position"`
	TeamAbbr       string                `json:"team_abbr"`
	OwnerTeamKey   string                `json:"owner_team_key"`
	OverallScore   float64               `json:"overall_score"`
	OverallRank    int                   `json:"overall_rank"`
	PositionScore  float64               `json:"position_score"` // z-score relative to same-position peers (categories) or unused (points)
	PositionRank   int                   `json:"position_rank"`
	TotalPoints    float64               `json:"total_points,omitempty"` // raw fantasy points (points leagues only)
	CategoryScores []playerCategoryScore `json:"category_scores"`
	Trajectory     []trajectoryPoint     `json:"trajectory,omitempty"` // year-over-year PPR/G from nfl_player_season_profiles
	PlayerGrade    *float64              `json:"player_grade,omitempty"`
	YoYTrend       *float64              `json:"yoy_trend,omitempty"`
}

type replacementLevelResp struct {
	Position  string  `json:"position"`
	Threshold int     `json:"threshold"` // number of starters league-wide at this position
	Points    float64 `json:"points"`    // fantasy points scored by the replacement-level player
}

type leagueRankingsResp struct {
	StatType          string                 `json:"stat_type"`
	ScoringMode       string                 `json:"scoring_mode"` // "points" (NFL) or "categories" (NBA)
	Categories        []categoryStatsResp    `json:"categories"`
	Players           []rankedPlayerResp     `json:"players"`
	ReplacementLevels []replacementLevelResp `json:"replacement_levels,omitempty"` // points leagues only
}

// GetLeagueRankings handles GET /api/leagues/{id}/rankings?stat_type=season.
func (h *Handler) GetLeagueRankings(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	yahooKey, status, msg := h.leagueYahooKey(r, id)
	if status != 0 {
		respondError(w, status, msg)
		return
	}

	// Determine scoring mode from the stored sport ("nfl" → points, others → categories).
	var leagueSport string
	_ = h.db.QueryRow(r.Context(), "SELECT sport FROM leagues WHERE id = $1", id).Scan(&leagueSport)
	isPointsLeague := strings.EqualFold(leagueSport, "nfl")

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[analysis] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	statType := r.URL.Query().Get("stat_type")
	if statType == "" {
		statType = "season"
	}
	if statType == "today" {
		statType = "date;date=" + time.Now().Format("2006-01-02")
	}

	// Fetch rosters, scoring categories, roster positions, and FA stats concurrently.
	type rostersResult struct {
		teams []yahoo.Team
		err   error
	}
	type catsResult struct {
		cats map[string]yahoo.LeagueStat
		err  error
	}
	type fasResult struct {
		players []yahoo.LeaguePlayer
		err     error
	}
	type rosterPosResult struct {
		positions []yahoo.RosterPosition
		err       error
	}

	rosterCh := make(chan rostersResult, 1)
	catsCh := make(chan catsResult, 1)
	faCh := make(chan fasResult, 1)
	rosterPosCh := make(chan rosterPosResult, 1)

	go func() {
		teams, err := yc.GetLeagueRostersWithStats(r.Context(), yahooKey, statType)
		rosterCh <- rostersResult{teams, err}
	}()
	go func() {
		cats, err := yc.GetLeagueScoringStats(r.Context(), yahooKey)
		catsCh <- catsResult{cats, err}
	}()
	go func() {
		fas, err := yc.GetAvailablePlayersWithStats(r.Context(), yahooKey, statType, 100)
		if err != nil {
			log.Printf("[analysis] GetAvailablePlayersWithStats %s: %v (scarcity factor will be 1.0)", yahooKey, err)
		}
		faCh <- fasResult{fas, err}
	}()
	go func() {
		pos, err := yc.GetLeagueRosterPositions(r.Context(), yahooKey)
		if err != nil {
			log.Printf("[analysis] GetLeagueRosterPositions %s: %v (VORP will fall back to raw points)", yahooKey, err)
		}
		rosterPosCh <- rosterPosResult{pos, err}
	}()

	rr := <-rosterCh
	if rr.err != nil {
		log.Printf("[analysis] GetLeagueRostersWithStats %s type=%s: %v", yahooKey, statType, rr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch rosters from Yahoo: "+rr.err.Error())
		return
	}
	cr := <-catsCh
	if cr.err != nil {
		log.Printf("[analysis] GetLeagueScoringStats %s: %v", yahooKey, cr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch league settings from Yahoo: "+cr.err.Error())
		return
	}
	fr := <-faCh
	rp := <-rosterPosCh

	statCats := cr.cats
	if len(statCats) == 0 {
		respondError(w, http.StatusUnprocessableEntity, "no scoring categories found for this league")
		return
	}

	// Build ordered category metadata.
	catMeta := buildCategoryMeta(statCats)

	// Convert Yahoo roster data → ranking.PlayerData.
	rosteredPlayers := yahooTeamsToPlayerData(rr.teams, statCats)
	faPlayerData := yahooFAToPlayerData(fr.players, statCats)

	// Build yahoo_id → gsis_id lookup for linking to player detail pages.
	yahooKeyToGsis := players.ResolveAllYahooToGsis(r.Context(), h.db)

	if isPointsLeague {
		rosterPositions := make([]ranking.RosterPosition, len(rp.positions))
		for i, p := range rp.positions {
			rosterPositions[i] = ranking.RosterPosition{Position: p.Position, Count: p.Count}
		}

		result := ranking.RankByPoints(rosteredPlayers, faPlayerData, catMeta, rosterPositions, len(rr.teams))
		h.writeRankingsResponse(w, r.Context(), statType, "points", result.CategoryStats, result.Players, result.ReplacementLevels, yahooKeyToGsis)
		return
	}

	result := ranking.RankByCategories(rosteredPlayers, catMeta, faPlayerData)
	h.writeRankingsResponse(w, r.Context(), statType, "categories", result.CategoryStats, result.Players, nil, yahooKeyToGsis)
}

// --- Helpers for converting Yahoo data to ranking service types ---

// buildCategoryMeta creates a sorted list of CategoryMeta from Yahoo stat categories.
func buildCategoryMeta(statCats map[string]yahoo.LeagueStat) []ranking.CategoryMeta {
	catIDs := make([]string, 0, len(statCats))
	for id := range statCats {
		catIDs = append(catIDs, id)
	}
	sort.Strings(catIDs)

	meta := make([]ranking.CategoryMeta, 0, len(catIDs))
	for _, id := range catIDs {
		cat := statCats[id]
		label := cat.DisplayName
		if label == "" {
			label = cat.Name
		}
		meta = append(meta, ranking.CategoryMeta{
			ID:        id,
			Label:     label,
			SortOrder: cat.SortOrder,
			Modifier:  cat.Modifier,
		})
	}
	return meta
}

// yahooTeamsToPlayerData converts Yahoo team rosters to ranking.PlayerData slices.
func yahooTeamsToPlayerData(teams []yahoo.Team, statCats map[string]yahoo.LeagueStat) []ranking.PlayerData {
	var players []ranking.PlayerData
	for _, team := range teams {
		if team.Roster == nil {
			continue
		}
		for _, p := range team.Roster.Players.Player {
			vals := make(map[string]float64)
			var total float64
			if p.PlayerStats != nil {
				for _, s := range p.PlayerStats.Stats {
					cat, ok := statCats[s.StatID]
					if !ok {
						continue
					}
					v, err := strconv.ParseFloat(s.Value, 64)
					if err != nil {
						continue
					}
					vals[s.StatID] = v
					total += v * cat.Modifier
				}
			}
			primaryPos := strings.SplitN(p.DisplayPosition, ",", 2)[0]
			players = append(players, ranking.PlayerData{
				PlayerKey:    p.PlayerKey,
				Name:         p.Name.Full,
				Position:     p.DisplayPosition,
				PrimaryPos:   primaryPos,
				TeamAbbr:     p.EditorialTeamAbbr,
				OwnerTeamKey: team.TeamKey,
				StatValues:   vals,
				TotalPoints:  math.Round(total*100) / 100,
				IsRostered:   true,
			})
		}
	}
	return players
}

// yahooFAToPlayerData converts Yahoo free agent players to ranking.PlayerData slices.
func yahooFAToPlayerData(faPlayers []yahoo.LeaguePlayer, statCats map[string]yahoo.LeagueStat) []ranking.PlayerData {
	var players []ranking.PlayerData
	for _, fa := range faPlayers {
		if fa.PlayerStats == nil {
			continue
		}
		vals := make(map[string]float64)
		var total float64
		for _, s := range fa.PlayerStats.Stats {
			cat, ok := statCats[s.StatID]
			if !ok {
				continue
			}
			v, err := strconv.ParseFloat(s.Value, 64)
			if err != nil {
				continue
			}
			vals[s.StatID] = v
			total += v * cat.Modifier
		}
		primaryPos := strings.SplitN(fa.DisplayPosition, ",", 2)[0]
		players = append(players, ranking.PlayerData{
			PlayerKey:    fa.PlayerKey,
			Name:         fa.Name.Full,
			Position:     fa.DisplayPosition,
			PrimaryPos:   primaryPos,
			TeamAbbr:     fa.EditorialTeamAbbr,
			OwnerTeamKey: "",
			StatValues:   vals,
			TotalPoints:  math.Round(total*100) / 100,
			IsRostered:   false,
		})
	}
	return players
}

// loadPlayerTrajectories batch-fetches year-over-year fpts_ppr_pg from
// nfl_player_season_profiles for the given gsis_ids (last 6 seasons).
func (h *Handler) loadPlayerTrajectories(ctx context.Context, gsisIDs []string) map[string][]trajectoryPoint {
	if len(gsisIDs) == 0 {
		return nil
	}
	rows, err := h.db.Query(ctx, `
		SELECT gsis_id, season, fpts_ppr_pg
		FROM nfl_player_season_profiles
		WHERE gsis_id = ANY($1) AND fpts_ppr_pg IS NOT NULL
		ORDER BY gsis_id, season ASC
	`, gsisIDs)
	if err != nil {
		log.Printf("[analysis] loadPlayerTrajectories: %v", err)
		return nil
	}
	defer rows.Close()

	result := make(map[string][]trajectoryPoint)
	for rows.Next() {
		var gsisID string
		var season int
		var fptsPPRPG float64
		if err := rows.Scan(&gsisID, &season, &fptsPPRPG); err != nil {
			continue
		}
		result[gsisID] = append(result[gsisID], trajectoryPoint{Season: season, FptsPPRPG: fptsPPRPG})
	}
	// Trim to last 6 seasons per player
	for id, pts := range result {
		if len(pts) > 6 {
			result[id] = pts[len(pts)-6:]
		}
	}
	return result
}

// loadPlayerHeadshots batch-fetches headshot URLs from nfl_players for the given gsis_ids.
func (h *Handler) loadPlayerHeadshots(ctx context.Context, gsisIDs []string) map[string]string {
	if len(gsisIDs) == 0 {
		return nil
	}
	rows, err := h.db.Query(ctx, `
		SELECT gsis_id, COALESCE(headshot_url, '')
		FROM nfl_players
		WHERE gsis_id = ANY($1) AND headshot_url IS NOT NULL AND headshot_url != ''
	`, gsisIDs)
	if err != nil {
		log.Printf("loadPlayerHeadshots query error: %v", err)
		return nil
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		result[id] = url
	}
	return result
}

type playerGradeInfo struct {
	Overall  float64
	YoYTrend *float64
}

// loadPlayerGrades batch-fetches the most recent grade for the given gsis_ids.
func (h *Handler) loadPlayerGrades(ctx context.Context, gsisIDs []string) map[string]playerGradeInfo {
	if len(gsisIDs) == 0 {
		return nil
	}
	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT ON (gsis_id) gsis_id, overall, yoy_trend
		FROM nfl_player_grades
		WHERE gsis_id = ANY($1)
		ORDER BY gsis_id, season DESC
	`, gsisIDs)
	if err != nil {
		log.Printf("loadPlayerGrades query error: %v", err)
		return nil
	}
	defer rows.Close()
	result := make(map[string]playerGradeInfo)
	for rows.Next() {
		var id string
		var g playerGradeInfo
		if err := rows.Scan(&id, &g.Overall, &g.YoYTrend); err != nil {
			continue
		}
		result[id] = g
	}
	return result
}

// writeRankingsResponse converts ranking service results to the HTTP response format.
func (h *Handler) writeRankingsResponse(
	w http.ResponseWriter,
	ctx context.Context,
	statType string,
	scoringMode string,
	catStats []ranking.CategoryStats,
	players []ranking.ScoredPlayer,
	replLevels []ranking.ReplacementLevel,
	yahooKeyToGsis func(string) string,
) {
	categories := make([]categoryStatsResp, len(catStats))
	for i, cs := range catStats {
		categories[i] = categoryStatsResp{
			Label:     cs.Label,
			SortOrder: cs.SortOrder,
			Mean:      cs.Mean,
			Stdev:     cs.Stdev,
			Weight:    cs.Weight,
		}
	}

	// Resolve gsis_ids and batch-load trajectories.
	gsisIDs := make([]string, 0, len(players))
	gsisForPlayer := make([]string, len(players))
	for i, sp := range players {
		gsis := yahooKeyToGsis(sp.PlayerKey)
		gsisForPlayer[i] = gsis
		if gsis != "" {
			gsisIDs = append(gsisIDs, gsis)
		}
	}
	trajectories := h.loadPlayerTrajectories(ctx, gsisIDs)
	headshots := h.loadPlayerHeadshots(ctx, gsisIDs)
	grades := h.loadPlayerGrades(ctx, gsisIDs)

	respPlayers := make([]rankedPlayerResp, len(players))
	for i, sp := range players {
		catScores := make([]playerCategoryScore, len(sp.CategoryScores))
		for j, cs := range sp.CategoryScores {
			catScores[j] = playerCategoryScore{
				Label:      cs.Label,
				Value:      cs.Value,
				ZScore:     cs.ZScore,
				Percentile: cs.Percentile,
			}
		}
		rp := rankedPlayerResp{
			PlayerKey:      sp.PlayerKey,
			GsisID:         gsisForPlayer[i],
			Name:           sp.Name,
			HeadshotURL:    headshots[gsisForPlayer[i]],
			Position:       sp.Position,
			TeamAbbr:       sp.TeamAbbr,
			OwnerTeamKey:   sp.OwnerTeamKey,
			OverallScore:   sp.OverallScore,
			OverallRank:    sp.OverallRank,
			PositionScore:  sp.PositionScore,
			PositionRank:   sp.PositionRank,
			TotalPoints:    sp.TotalPoints,
			CategoryScores: catScores,
			Trajectory:     trajectories[gsisForPlayer[i]],
		}
		if g, ok := grades[gsisForPlayer[i]]; ok {
			rp.PlayerGrade = &g.Overall
			rp.YoYTrend = g.YoYTrend
		}
		respPlayers[i] = rp
	}

	var replResp []replacementLevelResp
	if replLevels != nil {
		replResp = make([]replacementLevelResp, len(replLevels))
		for i, rl := range replLevels {
			replResp[i] = replacementLevelResp{
				Position:  rl.Position,
				Threshold: rl.Threshold,
				Points:    rl.Points,
			}
		}
	}

	respondJSON(w, http.StatusOK, leagueRankingsResp{
		StatType:          statType,
		ScoringMode:       scoringMode,
		Categories:        categories,
		Players:           respPlayers,
		ReplacementLevels: replResp,
	})
}
