package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ── response types ─────────────────────────────────────────────────────────

type nflPlayerMeta struct {
	GsisID        string  `json:"gsis_id"`
	Name          string  `json:"name"`
	Position      string  `json:"position"`
	PositionGroup string  `json:"position_group"`
	Team          string  `json:"team"`
	HeadshotURL   string  `json:"headshot_url"`
	BirthDate     string  `json:"birth_date"`
	Height        int     `json:"height"`  // inches
	Weight        int     `json:"weight"`  // lbs
	College       string  `json:"college"`
	YearsExp      int     `json:"years_exp"`
	EntryYear     int     `json:"entry_year"`
	RookieYear    int     `json:"rookie_year"`
	DraftClub     string  `json:"draft_club"`
	DraftNumber   int     `json:"draft_number"`
	JerseyNumber  int     `json:"jersey_number"`
	YahooID       string  `json:"yahoo_id"`
}

type nflSeasonStats struct {
	Season         int     `json:"season"`
	Age            int     `json:"age"`
	Team           string  `json:"team"`
	Games          int     `json:"games"`
	// Passing
	Completions    int     `json:"completions"`
	PassAttempts   int     `json:"pass_attempts"`
	PassYards      float64 `json:"pass_yards"`
	PassTDs        int     `json:"pass_tds"`
	Interceptions  int     `json:"interceptions"`
	Sacks          int     `json:"sacks"`
	// Rushing
	Carries        int     `json:"carries"`
	RushYards      float64 `json:"rush_yards"`
	RushTDs        int     `json:"rush_tds"`
	Fumbles        int     `json:"fumbles"`
	// Receiving
	Receptions     int     `json:"receptions"`
	Targets        int     `json:"targets"`
	RecYards       float64 `json:"rec_yards"`
	RecTDs         int     `json:"rec_tds"`
	// Kicking
	FgMade         int     `json:"fg_made"`
	FgAtt          int     `json:"fg_att"`
	FgLong         int     `json:"fg_long"`
	PatMade        int     `json:"pat_made"`
	// Fantasy
	FptsPPR        float64 `json:"fpts_ppr"`
	Fpts           float64 `json:"fpts"`
	FptsPPRPG      float64 `json:"fpts_ppr_pg"`
	FptsPG         float64 `json:"fpts_pg"`
	// Tags from profile
	Tags           []string `json:"tags"`
}

type nflPlayerGradeSeason struct {
	Season      int      `json:"season"`
	Overall     float64  `json:"overall"`
	Production  float64  `json:"production"`
	Efficiency  float64  `json:"efficiency"`
	Usage       float64  `json:"usage"`
	Durability  float64  `json:"durability"`
	CareerPhase string   `json:"career_phase"`
	YoYTrend    *float64 `json:"yoy_trend"`
}

type nflPlayerDetailResp struct {
	Player     nflPlayerMeta         `json:"player"`
	Seasons    []nflSeasonStats      `json:"seasons"`
	Projection *projDetailResp       `json:"projection"` // nil if no projection exists
	Grades     []nflPlayerGradeSeason `json:"grades"`
}

// ── handler ─────────────────────────────────────────────────────────────────

// GetNFLPlayer returns rich player info: metadata, year-over-year stats, and
// projection (if computed).
//
// GET /api/nfl/players/{gsisId}
func (h *Handler) GetNFLPlayer(w http.ResponseWriter, r *http.Request) {
	gsisID := chi.URLParam(r, "gsisId")
	if gsisID == "" {
		http.Error(w, "missing gsis_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// 1. Player metadata
	var meta nflPlayerMeta
	var birthDate *string
	err := h.db.QueryRow(ctx, `
		SELECT
			gsis_id, name,
			COALESCE(position, ''),
			COALESCE(position_group, ''),
			COALESCE(team, ''),
			COALESCE(headshot_url, ''),
			birth_date::text,
			COALESCE(height, 0),
			COALESCE(weight, 0),
			COALESCE(college, ''),
			COALESCE(years_exp, 0),
			COALESCE(entry_year, 0),
			COALESCE(rookie_year, 0),
			COALESCE(draft_club, ''),
			COALESCE(draft_number, 0),
			COALESCE(jersey_number, 0),
			COALESCE(yahoo_id, '')
		FROM nfl_players
		WHERE gsis_id = $1
	`, gsisID).Scan(
		&meta.GsisID, &meta.Name,
		&meta.Position, &meta.PositionGroup,
		&meta.Team, &meta.HeadshotURL,
		&birthDate,
		&meta.Height, &meta.Weight,
		&meta.College, &meta.YearsExp,
		&meta.EntryYear, &meta.RookieYear,
		&meta.DraftClub, &meta.DraftNumber,
		&meta.JerseyNumber, &meta.YahooID,
	)
	if err != nil {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}
	if birthDate != nil {
		// Trim to YYYY-MM-DD
		meta.BirthDate = strings.Split(*birthDate, "T")[0]
	}

	// 2. Year-over-year stats (aggregate weekly stats by season)
	statRows, err := h.db.Query(ctx, `
		SELECT
			s.season,
			COUNT(DISTINCT s.week)::int                          AS games,
			COALESCE(MAX(s.team), '')                            AS team,
			SUM(s.completions)::int                              AS completions,
			SUM(s.pass_attempts)::int                            AS pass_attempts,
			COALESCE(SUM(s.passing_yards), 0)                   AS pass_yards,
			SUM(s.passing_tds)::int                              AS pass_tds,
			SUM(s.interceptions)::int                            AS interceptions,
			SUM(s.sacks)::int                                    AS sacks,
			SUM(s.carries)::int                                  AS carries,
			COALESCE(SUM(s.rushing_yards), 0)                   AS rush_yards,
			SUM(s.rushing_tds)::int                              AS rush_tds,
			SUM(s.rushing_fumbles_lost + s.receiving_fumbles_lost)::int AS fumbles,
			SUM(s.receptions)::int                               AS receptions,
			SUM(s.targets)::int                                  AS targets,
			COALESCE(SUM(s.receiving_yards), 0)                 AS rec_yards,
			SUM(s.receiving_tds)::int                            AS rec_tds,
			SUM(s.fg_made)::int                                  AS fg_made,
			SUM(s.fg_att)::int                                   AS fg_att,
			COALESCE(MAX(s.fg_long), 0)                         AS fg_long,
			SUM(s.pat_made)::int                                 AS pat_made,
			COALESCE(SUM(s.fantasy_points_ppr), 0)              AS fpts_ppr,
			COALESCE(SUM(s.fantasy_points), 0)                  AS fpts
		FROM nfl_player_stats s
		WHERE s.gsis_id = $1
		  AND s.season_type = 'REG'
		GROUP BY s.season
		ORDER BY s.season DESC
	`, gsisID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer statRows.Close()

	seasons := []nflSeasonStats{}
	for statRows.Next() {
		var ss nflSeasonStats
		if err := statRows.Scan(
			&ss.Season, &ss.Games, &ss.Team,
			&ss.Completions, &ss.PassAttempts, &ss.PassYards, &ss.PassTDs, &ss.Interceptions, &ss.Sacks,
			&ss.Carries, &ss.RushYards, &ss.RushTDs, &ss.Fumbles,
			&ss.Receptions, &ss.Targets, &ss.RecYards, &ss.RecTDs,
			&ss.FgMade, &ss.FgAtt, &ss.FgLong, &ss.PatMade,
			&ss.FptsPPR, &ss.Fpts,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if ss.Games > 0 {
			ss.FptsPPRPG = ss.FptsPPR / float64(ss.Games)
			ss.FptsPG = ss.Fpts / float64(ss.Games)
		}
		seasons = append(seasons, ss)
	}
	if err := statRows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Enrich seasons with age and tags from season profiles
	profRows, err := h.db.Query(ctx, `
		SELECT season, COALESCE(age, 0), COALESCE(tags, '{}')
		FROM nfl_player_season_profiles
		WHERE gsis_id = $1
		ORDER BY season DESC
	`, gsisID)
	if err == nil {
		defer profRows.Close()
		type profData struct {
			age  int
			tags []string
		}
		profBySeasonMap := map[int]profData{}
		for profRows.Next() {
			var season, age int
			var tags []string
			if err := profRows.Scan(&season, &age, &tags); err == nil {
				profBySeasonMap[season] = profData{age, tags}
			}
		}
		for i := range seasons {
			if pd, ok := profBySeasonMap[seasons[i].Season]; ok {
				seasons[i].Age = pd.age
				seasons[i].Tags = pd.tags
			}
			if seasons[i].Tags == nil {
				seasons[i].Tags = []string{}
			}
		}
	}

	// 4. Projection (optional — may not exist)
	var proj *projDetailResp
	projRow := struct {
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
	}{}
	projErr := h.db.QueryRow(ctx, `
		SELECT
			base_season, target_season,
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
		WHERE gsis_id = $1
		ORDER BY target_season DESC
		LIMIT 1
	`, gsisID).Scan(
		&projRow.BaseSeason, &projRow.TargetSeason,
		&projRow.ProjFptsPG, &projRow.ProjFptsPPRPG,
		&projRow.ProjPassYdsPG, &projRow.ProjPassTdPG,
		&projRow.ProjRushYdsPG, &projRow.ProjRushTdPG,
		&projRow.ProjRecPG, &projRow.ProjRecYdsPG, &projRow.ProjRecTdPG,
		&projRow.ProjFgMadePG, &projRow.ProjPatMadePG,
		&projRow.ProjGames, &projRow.ProjFpts, &projRow.ProjFptsPPR, &projRow.ProjFptsHalf,
		&projRow.Confidence, &projRow.ConfSimilarity, &projRow.ConfCompCount, &projRow.ConfAgreement,
		&projRow.ConfSampleDepth, &projRow.ConfDataQuality,
		&projRow.CompCount, &projRow.AvgSimilarity, &projRow.Uniqueness,
		&projRow.CompsJSON,
	)
	if projErr == nil {
		var comps []projComp
		if projRow.CompsJSON != "" && projRow.CompsJSON != "[]" {
			_ = json.Unmarshal([]byte(projRow.CompsJSON), &comps)
		}
		if comps == nil {
			comps = []projComp{}
		}

		// Load historical seasons for projection detail
		histRows, err := h.db.Query(ctx, `
			SELECT season, COALESCE(age, 0), fpts_ppr_pg, fpts_pg, games_played
			FROM nfl_player_season_profiles
			WHERE gsis_id = $1
			ORDER BY season ASC
		`, gsisID)
		historical := []historicalSeason{}
		if err == nil {
			defer histRows.Close()
			for histRows.Next() {
				var hs historicalSeason
				if err := histRows.Scan(&hs.Season, &hs.Age, &hs.FptsPPRPG, &hs.FptsPG, &hs.Games); err == nil {
					historical = append(historical, hs)
				}
			}
		}

		proj = &projDetailResp{
			GsisID:        gsisID,
			Name:          meta.Name,
			Position:      meta.Position,
			PositionGroup: meta.PositionGroup,
			Team:          meta.Team,
			HeadshotURL:   meta.HeadshotURL,
			Age:           meta.YearsExp, // approximate; overridden below if profile exists
			BaseSeason:    projRow.BaseSeason,
			TargetSeason:  projRow.TargetSeason,
			Projection: projStats{
				FptsPG:    projRow.ProjFptsPG,
				FptsPPRPG: projRow.ProjFptsPPRPG,
				PassYdsPG: projRow.ProjPassYdsPG,
				PassTdPG:  projRow.ProjPassTdPG,
				RushYdsPG: projRow.ProjRushYdsPG,
				RushTdPG:  projRow.ProjRushTdPG,
				RecPG:     projRow.ProjRecPG,
				RecYdsPG:  projRow.ProjRecYdsPG,
				RecTdPG:   projRow.ProjRecTdPG,
				FgMadePG:  projRow.ProjFgMadePG,
				PatMadePG: projRow.ProjPatMadePG,
				Games:     projRow.ProjGames,
				Fpts:      projRow.ProjFpts,
				FptsPPR:   projRow.ProjFptsPPR,
				FptsHalf:  projRow.ProjFptsHalf,
			},
			Confidence: projConfidence{
				Overall:     projRow.Confidence,
				Similarity:  projRow.ConfSimilarity,
				CompCount:   projRow.ConfCompCount,
				Agreement:   projRow.ConfAgreement,
				SampleDepth: projRow.ConfSampleDepth,
				DataQuality: projRow.ConfDataQuality,
			},
			CompCount:  projRow.CompCount,
			Uniqueness: projRow.Uniqueness,
			Comps:      comps,
			Historical: historical,
		}
		// Use age from most recent season profile if available
		if len(seasons) > 0 && seasons[0].Age > 0 {
			proj.Age = seasons[0].Age
		}
	}

	w.Header().Set("Content-Type", "application/json")
	// Load grade history
	var grades []nflPlayerGradeSeason
	gradeRows, err := h.db.Query(r.Context(), `
		SELECT season, overall, production, efficiency, usage, durability, career_phase, yoy_trend
		FROM nfl_player_grades
		WHERE gsis_id = $1
		ORDER BY season DESC
	`, gsisID)
	if err == nil {
		defer gradeRows.Close()
		for gradeRows.Next() {
			var g nflPlayerGradeSeason
			if err := gradeRows.Scan(&g.Season, &g.Overall, &g.Production, &g.Efficiency, &g.Usage, &g.Durability, &g.CareerPhase, &g.YoYTrend); err == nil {
				grades = append(grades, g)
			}
		}
	}
	if grades == nil {
		grades = []nflPlayerGradeSeason{}
	}

	json.NewEncoder(w).Encode(nflPlayerDetailResp{
		Player:     meta,
		Seasons:    seasons,
		Projection: proj,
		Grades:     grades,
	})
}

// GetNFLPlayerByYahooID looks up a player's gsis_id given their Yahoo player key
// (e.g. "nfl.p.30977") and redirects to the player detail endpoint.
//
// GET /api/nfl/players/by-yahoo/{yahooKey}
func (h *Handler) GetNFLPlayerByYahooID(w http.ResponseWriter, r *http.Request) {
	yahooKey := chi.URLParam(r, "yahooKey")
	if yahooKey == "" {
		http.Error(w, "missing yahoo key", http.StatusBadRequest)
		return
	}

	// Extract numeric ID from key like "nfl.p.30977"
	yahooID := yahooKey
	if parts := strings.Split(yahooKey, "."); len(parts) == 3 {
		yahooID = parts[2]
	}

	var gsisID string
	err := h.db.QueryRow(r.Context(), `
		SELECT gsis_id FROM nfl_players WHERE yahoo_id = $1 LIMIT 1
	`, yahooID).Scan(&gsisID)
	if err != nil {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/api/nfl/players/"+gsisID, http.StatusFound)
}
