package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ── response types ────────────────────────────────────────────────────────────

type publicRankedPlayer struct {
	GsisID        string   `json:"gsis_id"`
	Name          string   `json:"name"`
	Position      string   `json:"position"`
	PositionGroup string   `json:"position_group"`
	Team          string   `json:"team"`
	HeadshotURL   string   `json:"headshot_url"`
	Age           int      `json:"age"`
	Games         int      `json:"games"`
	Fpts          float64  `json:"fpts"`    // total for chosen format
	FptsPG        float64  `json:"fpts_pg"` // per-game for chosen format
	Confidence    float64  `json:"confidence"`
	CompCount     int      `json:"comp_count"`
	Uniqueness    string   `json:"uniqueness"`
	OverallRank   int      `json:"overall_rank"`
	PositionRank  int      `json:"position_rank"`
	PlayerGrade   *float64 `json:"player_grade"`
}

type publicRankingsResp struct {
	Season  int                  `json:"season"`
	Format  string               `json:"format"`
	Players []publicRankedPlayer `json:"players"`
	Total   int                  `json:"total"`
}

// ListPublicRankings returns projection-based player rankings for a given season
// and scoring format. No Yahoo or league context required.
//
// GET /api/rankings?season=2026&format=ppr&position=RB&limit=200&offset=0
func (h *Handler) ListPublicRankings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	format := strings.ToLower(q.Get("format"))
	switch format {
	case "ppr", "half", "standard":
	case "":
		format = "ppr"
	default:
		respondError(w, http.StatusBadRequest, "format must be one of: ppr, half, standard")
		return
	}

	var fptsCol string
	switch format {
	case "ppr":
		fptsCol = "pr.proj_fpts_ppr"
	case "half":
		fptsCol = "pr.proj_fpts_half"
	default:
		fptsCol = "pr.proj_fpts"
	}

	position := q.Get("position") // comma-separated allowed: "RB,WR,TE"
	var positionList []string
	if position != "" {
		for _, p := range strings.Split(position, ",") {
			if p = strings.TrimSpace(strings.ToUpper(p)); p != "" {
				positionList = append(positionList, p)
			}
		}
	}

	limit := 200
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	args := []any{season, limit, offset}
	posClause := ""
	if len(positionList) > 0 {
		args = append(args, positionList)
		posClause = "AND p.position_group = ANY($4)"
	}

	query := `
		SELECT
			p.gsis_id, p.name,
			COALESCE(p.position, '') AS position,
			COALESCE(p.position_group, '') AS position_group,
			COALESCE(p.team, '') AS team,
			COALESCE(p.headshot_url, '') AS headshot_url,
			COALESCE(prof.age, 0) AS age,
			pr.proj_games,
			` + fptsCol + ` AS fpts,
			pr.confidence, pr.comp_count, pr.uniqueness,
			g.overall AS player_grade
		FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		LEFT JOIN nfl_player_season_profiles prof
		       ON prof.gsis_id = pr.gsis_id AND prof.season = pr.base_season
		LEFT JOIN nfl_player_grades g
		       ON g.gsis_id = pr.gsis_id AND g.season = pr.base_season
		WHERE pr.target_season = $1
		` + posClause + `
		ORDER BY ` + fptsCol + ` DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	players := []publicRankedPlayer{}
	for rows.Next() {
		var pl publicRankedPlayer
		var games int
		if err := rows.Scan(
			&pl.GsisID, &pl.Name, &pl.Position, &pl.PositionGroup, &pl.Team, &pl.HeadshotURL,
			&pl.Age, &games, &pl.Fpts,
			&pl.Confidence, &pl.CompCount, &pl.Uniqueness,
			&pl.PlayerGrade,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		pl.Games = games
		if games > 0 {
			pl.FptsPG = pl.Fpts / float64(games)
		}
		players = append(players, pl)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Ranks over the returned page (offset-aware).
	sort.SliceStable(players, func(i, j int) bool { return players[i].Fpts > players[j].Fpts })
	posRanks := map[string]int{}
	for i := range players {
		players[i].OverallRank = i + 1 + offset
		pos := players[i].PositionGroup
		posRanks[pos]++
		players[i].PositionRank = posRanks[pos]
	}

	// Total count (for pagination UI).
	countArgs := []any{season}
	countClause := ""
	if len(positionList) > 0 {
		countArgs = append(countArgs, positionList)
		countClause = "AND p.position_group = ANY($2)"
	}
	var total int
	_ = h.db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		WHERE pr.target_season = $1 `+countClause, countArgs...,
	).Scan(&total)

	respondJSON(w, http.StatusOK, publicRankingsResp{
		Season:  season,
		Format:  format,
		Players: players,
		Total:   total,
	})
}
