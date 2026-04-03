package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

type leaguePlayerResp struct {
	PlayerKey     string `json:"player_key"`
	Name          string `json:"name"`
	TeamAbbr      string `json:"team_abbr"`
	Position      string `json:"position"`
	Status        string `json:"status"`
	OwnershipType string `json:"ownership_type"`
	OwnedPercent  string `json:"owned_percent"`
	ImageURL      string `json:"image_url,omitempty"`
}

func toPlayerResp(p yahoo.LeaguePlayer) leaguePlayerResp {
	return leaguePlayerResp{
		PlayerKey:     p.PlayerKey,
		Name:          p.Name.Full,
		TeamAbbr:      p.EditorialTeamAbbr,
		Position:      p.DisplayPosition,
		Status:        p.Status,
		OwnershipType: p.Ownership.OwnershipType,
		OwnedPercent:  p.Ownership.OwnedPercent,
		ImageURL:      p.HeadshotURL(),
	}
}

// SearchLeaguePlayers handles GET /api/leagues/{id}/players?search={q}
// Searches Yahoo for players by name within the league context (for ownership %).
func (h *Handler) SearchLeaguePlayers(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	query := r.URL.Query().Get("search")
	if query == "" {
		respondError(w, http.StatusBadRequest, "search query required")
		return
	}

	yahooKey, status, msg := h.leagueYahooKey(r, id)
	if status != 0 {
		respondError(w, status, msg)
		return
	}

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[league_players] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	players, err := yc.SearchPlayers(r.Context(), yahooKey, query)
	if err != nil {
		log.Printf("[league_players] SearchPlayers %s %q failed: %v", yahooKey, query, err)
		respondError(w, http.StatusBadGateway, "failed to search players: "+err.Error())
		return
	}

	resp := make([]leaguePlayerResp, 0, len(players))
	for _, p := range players {
		resp = append(resp, toPlayerResp(p))
	}

	respondJSON(w, http.StatusOK, resp)
}

// GetAvailablePlayers handles GET /api/leagues/{id}/players/available?position={pos}&start={n}
// Returns free agent / waiver players, paginated in groups of 25.
func (h *Handler) GetAvailablePlayers(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	position := r.URL.Query().Get("position")
	start := 0
	if s := r.URL.Query().Get("start"); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed >= 0 {
			start = parsed
		}
	}
	// statusFilter controls which players Yahoo returns:
	// "A" = available (FA+W), "FA" = free agents, "W" = waivers, "" = all players.
	// Default to "A" so the endpoint is backward-compatible.
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "A"
	}
	// "all" is a frontend-friendly alias for the no-filter case.
	if statusFilter == "all" {
		statusFilter = ""
	}

	yahooKey, httpStatus, msg := h.leagueYahooKey(r, id)
	if httpStatus != 0 {
		respondError(w, httpStatus, msg)
		return
	}

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[league_players] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	players, err := yc.GetAvailablePlayers(r.Context(), yahooKey, statusFilter, position, start)
	if err != nil {
		log.Printf("[league_players] GetAvailablePlayers %s pos=%q start=%d failed: %v", yahooKey, position, start, err)
		respondError(w, http.StatusBadGateway, "failed to fetch available players: "+err.Error())
		return
	}

	resp := make([]leaguePlayerResp, 0, len(players))
	for _, p := range players {
		resp = append(resp, toPlayerResp(p))
	}

	respondJSON(w, http.StatusOK, resp)
}
