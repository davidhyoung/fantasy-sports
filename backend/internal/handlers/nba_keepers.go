package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/keepers"
)

// nbaKeeperCandidate is one player from last season's league-wide roster,
// priced as a keeper for the upcoming season. Candidate pool is every team's
// roster, not just the requesting team's — "the Brian Rule" lets any team
// reclaim a player who was dropped, so the whole league has to be visible.
type nbaKeeperCandidate struct {
	ESPNID         string `json:"espn_id"`
	Name           string `json:"name"`
	Position       string `json:"position"`
	Team           string `json:"team"`
	HeadshotURL    string `json:"headshot_url"`
	HeldByTeamID   int64  `json:"held_by_team_id"`
	HeldByTeamName string `json:"held_by_team_name"`
	LastSalary     int    `json:"last_salary"`
	// KeeperCost is only populated when ?team_id= was given — cost depends
	// on whether that team is the one that held the player last season.
	KeeperCost     *int    `json:"keeper_cost"`
	KeptByTeamID   *int64  `json:"kept_by_team_id"`
	KeptByTeamName *string `json:"kept_by_team_name"`
}

type nbaKeeperCandidatesResp struct {
	Season     int                  `json:"season"`
	MaxKeepers int                  `json:"max_keepers"`
	Candidates []nbaKeeperCandidate `json:"candidates"`
}

// maxNBAKeepersPerTeam is this league's constitution, not a per-league
// setting — the user's explicit rule, same fixed-3 convention as every
// other native-NBA business rule that isn't yet exposed as a league setting.
const maxNBAKeepersPerTeam = 3

// GetNBAKeeperCandidates returns every player from last season's league-wide
// roster, priced as a keeper for `season`.
//
// GET /api/leagues/{id}/keepers-nba?season=&team_id=
func (h *Handler) GetNBAKeeperCandidates(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	q := r.URL.Query()
	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	var teamID int64
	if t := q.Get("team_id"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			teamID = v
		}
	}
	ctx := r.Context()

	rows, err := h.db.Query(ctx, `
		SELECT lr.espn_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       lr.team_id, t.name, COALESCE(lr.salary, 0)
		FROM league_rosters_nba lr
		JOIN nba_players p ON p.espn_id = lr.espn_id
		JOIN teams t ON t.id = lr.team_id
		WHERE lr.league_id = $1 AND lr.season = $2
		ORDER BY p.name
	`, leagueID, season-1)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	candidates := []nbaKeeperCandidate{}
	espnIDs := make([]string, 0)
	for rows.Next() {
		var c nbaKeeperCandidate
		if err := rows.Scan(
			&c.ESPNID, &c.Name, &c.Position, &c.Team, &c.HeadshotURL,
			&c.HeldByTeamID, &c.HeldByTeamName, &c.LastSalary,
		); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if teamID != 0 {
			cost := keepers.ComputeNBAKeeperCost(c.LastSalary, c.HeldByTeamID == teamID)
			c.KeeperCost = &cost
		}
		candidates = append(candidates, c)
		espnIDs = append(espnIDs, c.ESPNID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	keptRows, err := h.db.Query(ctx, `
		SELECT k.espn_id, k.team_id, t.name
		FROM nba_keepers k JOIN teams t ON t.id = k.team_id
		WHERE k.league_id = $1 AND k.season = $2 AND k.espn_id = ANY($3)
	`, leagueID, season, espnIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	kept := make(map[string]struct {
		teamID   int64
		teamName string
	}, len(espnIDs))
	for keptRows.Next() {
		var espnID, teamName string
		var tID int64
		if err := keptRows.Scan(&espnID, &tID, &teamName); err != nil {
			keptRows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		kept[espnID] = struct {
			teamID   int64
			teamName string
		}{tID, teamName}
	}
	keptRows.Close()
	if err := keptRows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range candidates {
		if k, ok := kept[candidates[i].ESPNID]; ok {
			id := k.teamID
			name := k.teamName
			candidates[i].KeptByTeamID = &id
			candidates[i].KeptByTeamName = &name
		}
	}

	respondJSON(w, http.StatusOK, nbaKeeperCandidatesResp{
		Season: season, MaxKeepers: maxNBAKeepersPerTeam, Candidates: candidates,
	})
}

type designateNBAKeeperReq struct {
	Season int    `json:"season"`
	TeamID int64  `json:"team_id"`
	ESPNID string `json:"espn_id"`
}

type nbaKeeperResp struct {
	ESPNID     string `json:"espn_id"`
	TeamID     int64  `json:"team_id"`
	KeeperCost int    `json:"keeper_cost"`
}

// DesignateNBAKeeper marks a player as one of a team's (up to 3) keepers for
// `season`, computing the cost server-side — never trusts a client-supplied
// price.
//
// POST /api/leagues/{id}/keepers-nba
func (h *Handler) DesignateNBAKeeper(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}
	var req designateNBAKeeperReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Season == 0 || req.TeamID == 0 || req.ESPNID == "" {
		respondError(w, http.StatusBadRequest, "season, team_id, and espn_id are required")
		return
	}
	ctx := r.Context()

	var teamInLeague bool
	if err := h.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", req.TeamID, leagueID,
	).Scan(&teamInLeague); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !teamInLeague {
		respondError(w, http.StatusBadRequest, "team is not in this league")
		return
	}

	var keeperCount int
	if err := h.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM nba_keepers WHERE league_id = $1 AND season = $2 AND team_id = $3",
		leagueID, req.Season, req.TeamID,
	).Scan(&keeperCount); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keeperCount >= maxNBAKeepersPerTeam {
		respondError(w, http.StatusUnprocessableEntity, "this team already has its maximum of 3 keepers")
		return
	}

	// Last season's holder + recorded salary — the cost basis. A player who
	// never appeared in last season's roster (lastSalary=0, heldByTeamID=0,
	// which can never equal a real team_id) is still a valid keeper
	// candidate at the floor cost ("The Brian Rule"'s never-rostered case).
	var heldByTeamID int64
	var lastSalary int
	_ = h.db.QueryRow(ctx, `
		SELECT team_id, COALESCE(salary, 0) FROM league_rosters_nba
		WHERE league_id = $1 AND season = $2 AND espn_id = $3
	`, leagueID, req.Season-1, req.ESPNID).Scan(&heldByTeamID, &lastSalary)

	cost := keepers.ComputeNBAKeeperCost(lastSalary, heldByTeamID == req.TeamID)

	var resp nbaKeeperResp
	err = h.db.QueryRow(ctx, `
		INSERT INTO nba_keepers (league_id, season, team_id, espn_id, keeper_cost)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING espn_id, team_id, keeper_cost
	`, leagueID, req.Season, req.TeamID, req.ESPNID, cost).Scan(&resp.ESPNID, &resp.TeamID, &resp.KeeperCost)
	if err != nil {
		respondError(w, http.StatusConflict, "this player is already kept by a team this season")
		return
	}

	respondJSON(w, http.StatusCreated, resp)
}

// RemoveNBAKeeper un-designates a keeper.
//
// DELETE /api/leagues/{id}/keepers-nba/{espnId}?season=
func (h *Handler) RemoveNBAKeeper(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}
	espnID := chi.URLParam(r, "espnId")
	season := h.config.DefaultSeason
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	tag, err := h.db.Exec(r.Context(),
		"DELETE FROM nba_keepers WHERE league_id = $1 AND season = $2 AND espn_id = $3",
		leagueID, season, espnID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "no keeper designation found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"removed": true})
}
