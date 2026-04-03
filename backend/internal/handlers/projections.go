package handlers

import (
	"encoding/json"
	"net/http"
	gosort "sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// ── response types ────────────────────────────────────────────────────────────

type projTrajPoint struct {
	Season    int     `json:"season"`
	Age       int     `json:"age"`
	FptsPPRPG float64 `json:"fpts_ppr_pg"`
	FptsPG    float64 `json:"fpts_pg"`
	Growth    float64 `json:"growth"`
}

type projComp struct {
	GsisID       string             `json:"gsis_id"`
	Name         string             `json:"name"`
	MatchSeason  int                `json:"match_season"`
	MatchAge     int                `json:"match_age"`
	Similarity   float64            `json:"similarity"`
	Weight       float64            `json:"weight"`
	HeadshotURL  string             `json:"headshot_url"`
	MatchProfile map[string]float64 `json:"match_profile"`
	PreMatch     []projTrajPoint    `json:"pre_match"`
	Trajectory   []projTrajPoint    `json:"trajectory"`
}

type projConfidence struct {
	Overall     float64 `json:"overall"`
	Similarity  float64 `json:"similarity"`
	CompCount   float64 `json:"comp_count"`
	Agreement   float64 `json:"agreement"`
	SampleDepth float64 `json:"sample_depth"`
	DataQuality float64 `json:"data_quality"`
}

type projStats struct {
	FptsPG    float64 `json:"fpts_pg"`
	FptsPPRPG float64 `json:"fpts_ppr_pg"`
	PassYdsPG float64 `json:"pass_yds_pg"`
	PassTdPG  float64 `json:"pass_td_pg"`
	RushYdsPG float64 `json:"rush_yds_pg"`
	RushTdPG  float64 `json:"rush_td_pg"`
	RecPG     float64 `json:"rec_pg"`
	RecYdsPG  float64 `json:"rec_yds_pg"`
	RecTdPG   float64 `json:"rec_td_pg"`
	FgMadePG  float64 `json:"fg_made_pg"`
	PatMadePG float64 `json:"pat_made_pg"`
	Games     int     `json:"games"`
	Fpts      float64 `json:"fpts"`
	FptsPPR   float64 `json:"fpts_ppr"`
	FptsHalf  float64 `json:"fpts_half"`
}

type projPlayerListItem struct {
	GsisID        string  `json:"gsis_id"`
	Name          string  `json:"name"`
	Position      string  `json:"position"`
	PositionGroup string  `json:"position_group"`
	Team          string  `json:"team"`
	HeadshotURL   string  `json:"headshot_url"`
	Age           int     `json:"age"`
	TargetSeason  int     `json:"target_season"`
	ProjFpts      float64 `json:"proj_fpts"`
	ProjFptsPPR   float64 `json:"proj_fpts_ppr"`
	ProjFptsHalf  float64 `json:"proj_fpts_half"`
	ProjFptsPPRPG float64 `json:"proj_fpts_ppr_pg"`
	Confidence    float64 `json:"confidence"`
	CompCount     int     `json:"comp_count"`
	Uniqueness    string  `json:"uniqueness"`
	OverallRank   int      `json:"overall_rank"`
	PositionRank  int      `json:"position_rank"`
	PlayerGrade   *float64 `json:"player_grade"`
	GradeRank     *int     `json:"grade_rank"`
}

type projListResp struct {
	Season  int                  `json:"season"`
	Players []projPlayerListItem `json:"players"`
	Total   int                  `json:"total"`
}

type historicalSeason struct {
	Season    int     `json:"season"`
	Age       int     `json:"age"`
	FptsPPRPG float64 `json:"fpts_ppr_pg"`
	FptsPG    float64 `json:"fpts_pg"`
	Games     int     `json:"games"`
}

type projDetailResp struct {
	GsisID        string           `json:"gsis_id"`
	Name          string           `json:"name"`
	Position      string           `json:"position"`
	PositionGroup string           `json:"position_group"`
	Team          string           `json:"team"`
	HeadshotURL   string           `json:"headshot_url"`
	Age           int              `json:"age"`
	BaseSeason    int              `json:"base_season"`
	TargetSeason  int              `json:"target_season"`
	Projection    projStats        `json:"projection"`
	Confidence    projConfidence   `json:"confidence"`
	CompCount     int              `json:"comp_count"`
	Uniqueness    string           `json:"uniqueness"`
	Comps         []projComp         `json:"comps"`
	Historical    []historicalSeason `json:"historical"`
	PlayerGrade   *float64           `json:"player_grade"`
}

// ── handlers ──────────────────────────────────────────────────────────────────

// ListProjections returns projected players for a given season, filterable by
// position and sortable by projected PPR points.
//
// GET /api/projections?season=2025&position=QB&sort=proj_fpts_ppr&limit=50&offset=0
func (h *Handler) ListProjections(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	position := q.Get("position") // e.g. "QB", "RB", "WR", "TE", "K"
	sort := q.Get("sort")
	if sort == "" {
		sort = "proj_fpts_ppr"
	}
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

	// Build the ORDER BY expression — whitelist to prevent SQL injection
	orderBy := "pr.proj_fpts_ppr DESC"
	switch sort {
	case "proj_fpts":
		orderBy = "pr.proj_fpts DESC"
	case "proj_fpts_half":
		orderBy = "pr.proj_fpts_half DESC"
	case "confidence":
		orderBy = "pr.confidence DESC"
	case "name":
		orderBy = "p.name ASC"
	}

	posFilter := ""
	args := []any{season, limit, offset}
	if position != "" {
		posFilter = "AND p.position_group = $4"
		args = append(args, position)
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.gsis_id, p.name,
			COALESCE(p.position, '') AS position,
			COALESCE(p.position_group, '') AS position_group,
			COALESCE(p.team, '') AS team,
			COALESCE(p.headshot_url, '') AS headshot_url,
			COALESCE(prof.age, 0) AS age,
			pr.target_season,
			pr.proj_fpts, pr.proj_fpts_ppr, pr.proj_fpts_half, pr.proj_fpts_ppr_pg,
			pr.confidence, pr.comp_count, pr.uniqueness,
			g.overall AS player_grade
		FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		LEFT JOIN nfl_player_season_profiles prof
		       ON prof.gsis_id = pr.gsis_id AND prof.season = pr.base_season
		LEFT JOIN nfl_player_grades g
		       ON g.gsis_id = pr.gsis_id AND g.season = pr.base_season
		WHERE pr.target_season = $1
		`+posFilter+`
		ORDER BY `+orderBy+`
		LIMIT $2 OFFSET $3
	`, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	players := []projPlayerListItem{}
	for rows.Next() {
		var pl projPlayerListItem
		if err := rows.Scan(
			&pl.GsisID, &pl.Name, &pl.Position, &pl.PositionGroup, &pl.Team, &pl.HeadshotURL,
			&pl.Age, &pl.TargetSeason,
			&pl.ProjFpts, &pl.ProjFptsPPR, &pl.ProjFptsHalf, &pl.ProjFptsPPRPG,
			&pl.Confidence, &pl.CompCount, &pl.Uniqueness,
			&pl.PlayerGrade,
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

	// Assign overall and position ranks
	for i := range players {
		players[i].OverallRank = i + 1 + offset
	}
	posRanks := make(map[string]int)
	for i := range players {
		pos := players[i].PositionGroup
		posRanks[pos]++
		players[i].PositionRank = posRanks[pos]
	}

	// Assign grade ranks (sorted by grade descending among those that have a grade)
	type idxGrade struct {
		idx   int
		grade float64
	}
	var withGrade []idxGrade
	for i, pl := range players {
		if pl.PlayerGrade != nil {
			withGrade = append(withGrade, idxGrade{i, *pl.PlayerGrade})
		}
	}
	gosort.Slice(withGrade, func(a, b int) bool { return withGrade[a].grade > withGrade[b].grade })
	for rank, ig := range withGrade {
		r := rank + 1
		players[ig.idx].GradeRank = &r
	}

	// Get total count
	var total int
	countArgs := []any{season}
	countFilter := ""
	if position != "" {
		countFilter = "AND p.position_group = $2"
		countArgs = append(countArgs, position)
	}
	if err := h.db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		WHERE pr.target_season = $1 `+countFilter, countArgs...,
	).Scan(&total); err != nil {
		total = len(players)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projListResp{
		Season:  season,
		Players: players,
		Total:   total,
	})
}

// GetProjectionDetail returns a single player's projection with full comp detail
// and historical season stats.
//
// GET /api/projections/{gsisId}?season=2025
func (h *Handler) GetProjectionDetail(w http.ResponseWriter, r *http.Request) {
	// Extract gsis_id from URL — we use the path segment after /api/projections/
	// Chi router registers /{gsisId}, so use chi.URLParam
	gsisID := chi.URLParam(r, "gsisId")
	if gsisID == "" {
		http.Error(w, "missing gsis_id", http.StatusBadRequest)
		return
	}

	season := h.config.DefaultSeason
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	// Load the projection row
	var pr struct {
		GsisID          string
		BaseSeason      int
		TargetSeason    int
		ProjFptsPG      float64
		ProjFptsPPRPG   float64
		ProjPassYdsPG   float64
		ProjPassTdPG    float64
		ProjRushYdsPG   float64
		ProjRushTdPG    float64
		ProjRecPG       float64
		ProjRecYdsPG    float64
		ProjRecTdPG     float64
		ProjFgMadePG    float64
		ProjPatMadePG   float64
		ProjGames       int
		ProjFpts        float64
		ProjFptsPPR     float64
		ProjFptsHalf    float64
		Confidence      float64
		ConfSimilarity  float64
		ConfCompCount   float64
		ConfAgreement   float64
		ConfSampleDepth float64
		ConfDataQuality float64
		CompCount       int
		AvgSimilarity   float64
		Uniqueness      string
		CompsJSON       string
	}

	err := h.db.QueryRow(r.Context(), `
		SELECT
			gsis_id, base_season, target_season,
			proj_fpts_pg, proj_fpts_ppr_pg,
			proj_pass_yds_pg, proj_pass_td_pg,
			proj_rush_yds_pg, proj_rush_td_pg,
			proj_rec_pg, proj_rec_yds_pg, proj_rec_td_pg,
			proj_fg_made_pg, proj_pat_made_pg,
			proj_games, proj_fpts, proj_fpts_ppr, proj_fpts_half,
			confidence, conf_similarity, conf_comp_count, conf_agreement,
			conf_sample_depth, conf_data_quality,
			comp_count, avg_similarity, uniqueness,
			comps::text
		FROM nfl_projections
		WHERE gsis_id = $1 AND target_season = $2
	`, gsisID, season).Scan(
		&pr.GsisID, &pr.BaseSeason, &pr.TargetSeason,
		&pr.ProjFptsPG, &pr.ProjFptsPPRPG,
		&pr.ProjPassYdsPG, &pr.ProjPassTdPG,
		&pr.ProjRushYdsPG, &pr.ProjRushTdPG,
		&pr.ProjRecPG, &pr.ProjRecYdsPG, &pr.ProjRecTdPG,
		&pr.ProjFgMadePG, &pr.ProjPatMadePG,
		&pr.ProjGames, &pr.ProjFpts, &pr.ProjFptsPPR, &pr.ProjFptsHalf,
		&pr.Confidence, &pr.ConfSimilarity, &pr.ConfCompCount, &pr.ConfAgreement,
		&pr.ConfSampleDepth, &pr.ConfDataQuality,
		&pr.CompCount, &pr.AvgSimilarity, &pr.Uniqueness,
		&pr.CompsJSON,
	)
	if err != nil {
		http.Error(w, "projection not found", http.StatusNotFound)
		return
	}

	// Load player metadata
	var name, position, posGroup, team, headshot string
	var age int
	h.db.QueryRow(r.Context(), `
		SELECT p.name,
		       COALESCE(p.position, ''),
		       COALESCE(p.position_group, ''),
		       COALESCE(p.team, ''),
		       COALESCE(p.headshot_url, ''),
		       COALESCE(prof.age, 0)
		FROM nfl_players p
		LEFT JOIN nfl_player_season_profiles prof
		       ON prof.gsis_id = p.gsis_id AND prof.season = $2
		WHERE p.gsis_id = $1
	`, gsisID, pr.BaseSeason).Scan(&name, &position, &posGroup, &team, &headshot, &age)

	// Parse comps JSON
	var comps []projComp
	if pr.CompsJSON != "" && pr.CompsJSON != "[]" {
		_ = json.Unmarshal([]byte(pr.CompsJSON), &comps)
	}
	if comps == nil {
		comps = []projComp{}
	}

	// Load historical seasons
	histRows, err := h.db.Query(r.Context(), `
		SELECT season, COALESCE(age, 0), fpts_ppr_pg, fpts_pg, games_played
		FROM nfl_player_season_profiles
		WHERE gsis_id = $1
		ORDER BY season ASC
	`, gsisID)
	var historical []historicalSeason
	if err == nil {
		defer histRows.Close()
		for histRows.Next() {
			var hs historicalSeason
			if err := histRows.Scan(&hs.Season, &hs.Age, &hs.FptsPPRPG, &hs.FptsPG, &hs.Games); err == nil {
				historical = append(historical, hs)
			}
		}
	}
	if historical == nil {
		historical = []historicalSeason{}
	}

	resp := projDetailResp{
		GsisID:        gsisID,
		Name:          name,
		Position:      position,
		PositionGroup: posGroup,
		Team:          team,
		HeadshotURL:   headshot,
		Age:           age,
		BaseSeason:    pr.BaseSeason,
		TargetSeason:  pr.TargetSeason,
		Projection: projStats{
			FptsPG:    pr.ProjFptsPG,
			FptsPPRPG: pr.ProjFptsPPRPG,
			PassYdsPG: pr.ProjPassYdsPG,
			PassTdPG:  pr.ProjPassTdPG,
			RushYdsPG: pr.ProjRushYdsPG,
			RushTdPG:  pr.ProjRushTdPG,
			RecPG:     pr.ProjRecPG,
			RecYdsPG:  pr.ProjRecYdsPG,
			RecTdPG:   pr.ProjRecTdPG,
			FgMadePG:  pr.ProjFgMadePG,
			PatMadePG: pr.ProjPatMadePG,
			Games:     pr.ProjGames,
			Fpts:      pr.ProjFpts,
			FptsPPR:   pr.ProjFptsPPR,
			FptsHalf:  pr.ProjFptsHalf,
		},
		Confidence: projConfidence{
			Overall:     pr.Confidence,
			Similarity:  pr.ConfSimilarity,
			CompCount:   pr.ConfCompCount,
			Agreement:   pr.ConfAgreement,
			SampleDepth: pr.ConfSampleDepth,
			DataQuality: pr.ConfDataQuality,
		},
		CompCount:  pr.CompCount,
		Uniqueness: pr.Uniqueness,
		Comps:      comps,
		Historical: historical,
	}

	// Look up player grade for the base season
	var grade *float64
	_ = h.db.QueryRow(r.Context(), `
		SELECT overall FROM nfl_player_grades WHERE gsis_id = $1 AND season = $2
	`, gsisID, pr.BaseSeason).Scan(&grade)
	resp.PlayerGrade = grade

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ensure time import is used (avoids unused import in some Go versions)
var _ = time.Now
