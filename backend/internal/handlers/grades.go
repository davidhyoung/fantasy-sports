package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ── response types ───────────────────────────────────────────────────────────

type gradePlayerItem struct {
	GsisID        string  `json:"gsis_id"`
	Name          string  `json:"name"`
	Position      string  `json:"position"`
	PositionGroup string  `json:"position_group"`
	Team          string  `json:"team"`
	HeadshotURL   string  `json:"headshot_url"`
	Age           int     `json:"age"`
	Overall       float64 `json:"overall"`
	Production    float64 `json:"production"`
	Efficiency    float64 `json:"efficiency"`
	Usage         float64 `json:"usage"`
	Durability    float64 `json:"durability"`
	CareerPhase   string  `json:"career_phase"`
	YoYTrend      *float64 `json:"yoy_trend"`
	OverallRank   int     `json:"overall_rank"`
	PositionRank  int     `json:"position_rank"`
}

type gradeListResp struct {
	Season  int               `json:"season"`
	Players []gradePlayerItem `json:"players"`
	Total   int               `json:"total"`
}

type gradeSeasonEntry struct {
	Season      int      `json:"season"`
	Overall     float64  `json:"overall"`
	Production  float64  `json:"production"`
	Efficiency  float64  `json:"efficiency"`
	Usage       float64  `json:"usage"`
	Durability  float64  `json:"durability"`
	CareerPhase string   `json:"career_phase"`
	YoYTrend    *float64 `json:"yoy_trend"`
}

type gradePlayerDetailResp struct {
	GsisID        string             `json:"gsis_id"`
	Name          string             `json:"name"`
	Position      string             `json:"position"`
	PositionGroup string             `json:"position_group"`
	Team          string             `json:"team"`
	HeadshotURL   string             `json:"headshot_url"`
	Seasons       []gradeSeasonEntry `json:"seasons"`
}

// ── handlers ─────────────────────────────────────────────────────────────────

// ListGrades returns players ranked by grade for a given season.
//
// GET /api/grades?season=2025&position=QB&limit=200&offset=0
func (h *Handler) ListGrades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	season := h.config.DefaultSeason - 1 // grades are for actual seasons (base), not projected
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	position := q.Get("position")
	limit := 200
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	posFilter := ""
	args := []any{season, limit, offset}
	if position != "" {
		positions := strings.Split(position, ",")
		posFilter = "AND g.position_group = ANY($4)"
		args = append(args, positions)
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT
			g.gsis_id,
			p.name,
			COALESCE(p.position, '') AS position,
			g.position_group,
			COALESCE(p.team, '') AS team,
			COALESCE(p.headshot_url, '') AS headshot_url,
			COALESCE(prof.age, 0) AS age,
			g.overall, g.production, g.efficiency, g.usage, g.durability,
			g.career_phase, g.yoy_trend
		FROM nfl_player_grades g
		JOIN nfl_players p ON p.gsis_id = g.gsis_id
		LEFT JOIN nfl_player_season_profiles prof
		       ON prof.gsis_id = g.gsis_id AND prof.season = g.season
		WHERE g.season = $1
		`+posFilter+`
		ORDER BY g.overall DESC
		LIMIT $2 OFFSET $3
	`, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	players := []gradePlayerItem{}
	for rows.Next() {
		var pl gradePlayerItem
		if err := rows.Scan(
			&pl.GsisID, &pl.Name, &pl.Position, &pl.PositionGroup,
			&pl.Team, &pl.HeadshotURL, &pl.Age,
			&pl.Overall, &pl.Production, &pl.Efficiency, &pl.Usage, &pl.Durability,
			&pl.CareerPhase, &pl.YoYTrend,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		players = append(players, pl)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Assign ranks
	for i := range players {
		players[i].OverallRank = i + 1 + offset
	}
	posRanks := make(map[string]int)
	for i := range players {
		pos := players[i].PositionGroup
		posRanks[pos]++
		players[i].PositionRank = posRanks[pos]
	}

	// Total count
	var total int
	countArgs := []any{season}
	countFilter := ""
	if position != "" {
		countFilter = "AND position_group = $2"
		countArgs = append(countArgs, position)
	}
	if err := h.db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM nfl_player_grades
		WHERE season = $1 `+countFilter, countArgs...,
	).Scan(&total); err != nil {
		total = len(players)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gradeListResp{
		Season:  season,
		Players: players,
		Total:   total,
	})
}

// GetPlayerGrades returns all seasons of grades for a single player.
//
// GET /api/grades/{gsisId}
func (h *Handler) GetPlayerGrades(w http.ResponseWriter, r *http.Request) {
	gsisID := chi.URLParam(r, "gsisId")
	if gsisID == "" {
		http.Error(w, "missing gsis_id", http.StatusBadRequest)
		return
	}

	// Player metadata
	var name, position, posGroup, team, headshot string
	err := h.db.QueryRow(r.Context(), `
		SELECT p.name,
		       COALESCE(p.position, ''),
		       COALESCE(p.position_group, ''),
		       COALESCE(p.team, ''),
		       COALESCE(p.headshot_url, '')
		FROM nfl_players p
		WHERE p.gsis_id = $1
	`, gsisID).Scan(&name, &position, &posGroup, &team, &headshot)
	if err != nil {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	// All grade seasons
	rows, err := h.db.Query(r.Context(), `
		SELECT season, overall, production, efficiency, usage, durability, career_phase, yoy_trend
		FROM nfl_player_grades
		WHERE gsis_id = $1
		ORDER BY season ASC
	`, gsisID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	seasons := []gradeSeasonEntry{}
	for rows.Next() {
		var s gradeSeasonEntry
		if err := rows.Scan(&s.Season, &s.Overall, &s.Production, &s.Efficiency, &s.Usage, &s.Durability, &s.CareerPhase, &s.YoYTrend); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		seasons = append(seasons, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gradePlayerDetailResp{
		GsisID:        gsisID,
		Name:          name,
		Position:      position,
		PositionGroup: posGroup,
		Team:          team,
		HeadshotURL:   headshot,
		Seasons:       seasons,
	})
}
