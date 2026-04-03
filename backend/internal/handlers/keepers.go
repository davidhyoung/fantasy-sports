package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/keepers"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// --- Response / request types ---

type keeperRulesResp struct {
	CostIncrease  int  `json:"cost_increase"`
	UndraftedBase int  `json:"undrafted_base"`
	MaxYears      *int `json:"max_years"` // nil = unlimited
}

// draftResultResp is one draft pick enriched with player info and keeper cost.
type draftResultResp struct {
	PlayerKey     string      `json:"player_key"`
	PlayerName    string      `json:"player_name"`
	Position      string      `json:"position"`
	ImageURL      string      `json:"image_url,omitempty"`
	TeamKey       string      `json:"team_key"`
	OwnerTeamID   int64       `json:"owner_team_id"`
	OwnerTeamName string      `json:"owner_team_name"`
	DraftCost     int         `json:"draft_cost"`      // original auction price (0 if undrafted/snake)
	KeeperCost    int         `json:"keeper_cost"`     // projected cost if kept
	YearsKept     int         `json:"years_kept"`      // 0 if not on wishlist
	NotKeepable   bool        `json:"not_keepable"`
	Undrafted     bool        `json:"undrafted"`       // true if picked up off waivers (draft_cost == 0)
	Stats         []statEntry `json:"stats,omitempty"` // season stats, mapped to scoring categories
}

type keeperPlayerResp struct {
	PlayerKey string `json:"player_key"`
	Name      string `json:"name"`
	Position  string `json:"position"`
}

type wishlistEntryResp struct {
	ID         int64  `json:"id"`
	TeamID     int64  `json:"team_id"`
	PlayerKey  string `json:"player_key"`
	PlayerName string `json:"player_name"`
	Position   string `json:"position"`
	DraftCost  *int   `json:"draft_cost"` // nil if undrafted
	YearsKept  int    `json:"years_kept"`
}

type addKeeperReq struct {
	PlayerName string `json:"player_name"`
	Position   string `json:"position"`
	DraftCost  *int   `json:"draft_cost"`
	YearsKept  int    `json:"years_kept"`
}

// --- Helpers ---

// teamInfo holds DB data for a team, used to enrich draft pick responses.
type teamInfo struct {
	id   int64
	name string
}

// loadKeeperRules loads keeper rules from the DB, returning defaults if none exist.
func (h *Handler) loadKeeperRules(r *http.Request, leagueID int64) keeperRulesResp {
	rules := keeperRulesResp{CostIncrease: 5, UndraftedBase: 10} // defaults
	var maxYears *int
	err := h.db.QueryRow(r.Context(),
		"SELECT cost_increase, undrafted_base, max_years FROM keeper_rules WHERE league_id = $1",
		leagueID,
	).Scan(&rules.CostIncrease, &rules.UndraftedBase, &maxYears)
	if err == nil {
		rules.MaxYears = maxYears
	}
	return rules
}

// --- GetKeeperRules ---

// GetKeeperRules handles GET /api/leagues/{id}/keeper-rules.
// Returns the league's keeper rules, or sensible defaults if none have been configured.
func (h *Handler) GetKeeperRules(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	resp := h.loadKeeperRules(r, id)
	respondJSON(w, http.StatusOK, resp)
}

// --- UpdateKeeperRules ---

// UpdateKeeperRules handles PUT /api/leagues/{id}/keeper-rules.
// Saves or updates the league's keeper rules. Returns the saved rules.
func (h *Handler) UpdateKeeperRules(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req keeperRulesResp
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err = h.db.Exec(r.Context(), `
		INSERT INTO keeper_rules (league_id, cost_increase, undrafted_base, max_years)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (league_id) DO UPDATE
			SET cost_increase  = EXCLUDED.cost_increase,
			    undrafted_base = EXCLUDED.undrafted_base,
			    max_years      = EXCLUDED.max_years
	`, id, req.CostIncrease, req.UndraftedBase, req.MaxYears)
	if err != nil {
		log.Printf("[keepers] UpdateKeeperRules league %d: %v", id, err)
		respondError(w, http.StatusInternalServerError, "failed to save keeper rules")
		return
	}

	respondJSON(w, http.StatusOK, req)
}

// --- GetLeagueDraftResults ---

// GetLeagueDraftResults handles GET /api/leagues/{id}/draftresults.
// Uses current team rosters (not draft history) as the primary data source so
// that trades and waiver adds are reflected. Draft results are fetched
// concurrently to supply original auction costs for keeper cost calculation.
// Player names come free from the roster data — no separate batch lookup needed.
func (h *Handler) GetLeagueDraftResults(w http.ResponseWriter, r *http.Request) {
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

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[keepers] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	// Load all teams in this league from DB: yahoo_key → {id, name}.
	teamsByYahooKey := make(map[string]teamInfo)
	rows, err := h.db.Query(r.Context(),
		"SELECT id, name, COALESCE(yahoo_key, '') FROM teams WHERE league_id = $1", id,
	)
	if err != nil {
		log.Printf("[keepers] failed to load teams for league %d: %v", id, err)
		respondError(w, http.StatusInternalServerError, "failed to load teams")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t teamInfo
		var yKey string
		if err := rows.Scan(&t.id, &t.name, &yKey); err != nil {
			continue
		}
		if yKey != "" {
			teamsByYahooKey[yKey] = t
		}
	}
	rows.Close()

	// Find the current user's team in this league (for wishlist lookup).
	var myTeamID int64
	h.db.QueryRow(r.Context(),
		"SELECT id FROM teams WHERE league_id = $1 AND user_id = $2", id, user.ID,
	).Scan(&myTeamID)

	// Load keeper rules (or defaults).
	rules := h.loadKeeperRules(r, id)

	// Load the user's wishlist: player_key → years_kept.
	wishlistYears := make(map[string]int)
	if myTeamID != 0 {
		wRows, err := h.db.Query(r.Context(),
			"SELECT player_key, years_kept FROM keeper_wishlist WHERE team_id = $1", myTeamID,
		)
		if err == nil {
			defer wRows.Close()
			for wRows.Next() {
				var pk string
				var yk int
				if err := wRows.Scan(&pk, &yk); err == nil {
					wishlistYears[pk] = yk
				}
			}
		}
	}

	// Fetch current rosters (with season stats), draft results, and scoring categories concurrently.
	type rosterResult struct {
		teams []yahoo.Team
		err   error
	}
	type draftResult struct {
		picks []yahoo.DraftPick
		err   error
	}
	type scoringResult struct {
		cats map[string]yahoo.LeagueStat
		err  error
	}
	rosterCh := make(chan rosterResult, 1)
	draftCh := make(chan draftResult, 1)
	scoringCh := make(chan scoringResult, 1)

	go func() {
		teams, err := yc.GetLeagueRostersWithStats(r.Context(), yahooKey, "season")
		rosterCh <- rosterResult{teams, err}
	}()
	go func() {
		picks, err := yc.GetLeagueDraftResults(r.Context(), yahooKey)
		draftCh <- draftResult{picks, err}
	}()
	go func() {
		cats, err := yc.GetLeagueScoringStats(r.Context(), yahooKey)
		scoringCh <- scoringResult{cats, err}
	}()

	rr := <-rosterCh
	if rr.err != nil {
		log.Printf("[keepers] GetLeagueRostersWithStats %s failed: %v", yahooKey, rr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch rosters: "+rr.err.Error())
		return
	}
	dr := <-draftCh
	if dr.err != nil {
		log.Printf("[keepers] GetLeagueDraftResults %s failed: %v", yahooKey, dr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch draft results: "+dr.err.Error())
		return
	}
	sr := <-scoringCh
	if sr.err != nil {
		log.Printf("[keepers] GetLeagueScoringStats %s: %v (non-fatal, stats omitted)", yahooKey, sr.err)
	}

	// Build a map: player_key → original auction draft cost.
	// Only players with cost > 0 appear here (snake picks and FA pickups don't).
	// Traded players retain their original draft cost regardless of current team.
	draftCostMap := make(map[string]int, len(dr.picks))
	for _, p := range dr.picks {
		if cost, err := strconv.Atoi(p.Cost); err == nil && cost > 0 {
			draftCostMap[p.PlayerKey] = cost
		}
	}

	// Build a map: player_key → season stats (populated if GetLeagueRostersWithStats succeeded).
	playerStatsMap := make(map[string][]yahoo.PlayerStat)
	for _, yTeam := range rr.teams {
		if yTeam.Roster == nil {
			continue
		}
		for _, player := range yTeam.Roster.Players.Player {
			if player.PlayerStats != nil {
				playerStatsMap[player.PlayerKey] = player.PlayerStats.Stats
			}
		}
	}

	// Build response from current rosters, enriched with draft costs and season stats.
	resp := make([]draftResultResp, 0)
	for _, yTeam := range rr.teams {
		if yTeam.Roster == nil {
			continue
		}
		t := teamsByYahooKey[yTeam.TeamKey]
		for _, player := range yTeam.Roster.Players.Player {
			// Undrafted = never appeared in the auction draft (FA pickup or snake league).
			// Traded players keep their original auction price.
			draftCost, wasDrafted := draftCostMap[player.PlayerKey]
			undrafted := !wasDrafted
			yearsKept := wishlistYears[player.PlayerKey]

			keeperCost, notKeepable := keepers.ComputeKeeperCost(
				keepers.KeeperRules{
					CostIncrease:  rules.CostIncrease,
					UndraftedBase: rules.UndraftedBase,
					MaxYears:      rules.MaxYears,
				},
				draftCost, undrafted, yearsKept,
			)

			// Map season stats to scoring category labels.
			var stats []statEntry
			if sr.cats != nil {
				for _, s := range playerStatsMap[player.PlayerKey] {
					if s.Value == "" || s.Value == "-" || s.Value == "0" || s.Value == "0.00" {
						continue
					}
					cat, ok := sr.cats[s.StatID]
					if !ok {
						continue
					}
					label := cat.DisplayName
					if label == "" {
						label = cat.Name
					}
					val := strings.TrimRight(strings.TrimRight(s.Value, "0"), ".")
					stats = append(stats, statEntry{Label: label, Value: val, SortOrder: cat.SortOrder})
				}
			}

			resp = append(resp, draftResultResp{
				PlayerKey:     player.PlayerKey,
				PlayerName:    player.Name.Full,
				Position:      player.DisplayPosition,
				ImageURL:      player.HeadshotURL(),
				TeamKey:       yTeam.TeamKey,
				OwnerTeamID:   t.id,
				OwnerTeamName: t.name,
				DraftCost:     draftCost,
				KeeperCost:    keeperCost,
				YearsKept:     yearsKept,
				NotKeepable:   notKeepable,
				Undrafted:     undrafted,
				Stats:         stats,
			})
		}
	}

	// Sort by team key, then draft cost descending, then name.
	sort.Slice(resp, func(i, j int) bool {
		if resp[i].TeamKey != resp[j].TeamKey {
			return resp[i].TeamKey < resp[j].TeamKey
		}
		if resp[i].DraftCost != resp[j].DraftCost {
			return resp[i].DraftCost > resp[j].DraftCost
		}
		return resp[i].PlayerName < resp[j].PlayerName
	})

	respondJSON(w, http.StatusOK, resp)
}

// --- GetLeagueKeepers ---

// GetLeagueKeepers handles GET /api/leagues/{id}/keepers.
// Returns players that have keeper designation (status=K) in Yahoo.
func (h *Handler) GetLeagueKeepers(w http.ResponseWriter, r *http.Request) {
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

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[keepers] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	players, err := yc.GetLeagueKeepers(r.Context(), yahooKey)
	if err != nil {
		log.Printf("[keepers] GetLeagueKeepers %s failed: %v", yahooKey, err)
		respondError(w, http.StatusBadGateway, "failed to fetch keepers: "+err.Error())
		return
	}

	resp := make([]keeperPlayerResp, 0, len(players))
	for _, p := range players {
		resp = append(resp, keeperPlayerResp{
			PlayerKey: p.PlayerKey,
			Name:      p.Name.Full,
			Position:  p.DisplayPosition,
		})
	}

	respondJSON(w, http.StatusOK, resp)
}

// --- ListTeamKeeperWishlist ---

// ListTeamKeeperWishlist handles GET /api/teams/{id}/keepers.
// Returns all keeper wishlist entries for the given team.
func (h *Handler) ListTeamKeeperWishlist(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, team_id, player_key, player_name, COALESCE(position, ''), draft_cost, years_kept
		FROM keeper_wishlist
		WHERE team_id = $1
		ORDER BY id
	`, teamID)
	if err != nil {
		log.Printf("[keepers] ListTeamKeeperWishlist team %d: %v", teamID, err)
		respondError(w, http.StatusInternalServerError, "failed to load wishlist")
		return
	}
	defer rows.Close()

	resp := make([]wishlistEntryResp, 0)
	for rows.Next() {
		var e wishlistEntryResp
		var pos string
		if err := rows.Scan(&e.ID, &e.TeamID, &e.PlayerKey, &e.PlayerName, &pos, &e.DraftCost, &e.YearsKept); err != nil {
			continue
		}
		e.Position = pos
		resp = append(resp, e)
	}

	respondJSON(w, http.StatusOK, resp)
}

// --- AddKeeperWishlist ---

// AddKeeperWishlist handles POST /api/teams/{id}/keepers/{playerKey}.
// Inserts or updates a keeper wishlist entry. Only the team owner may call this.
func (h *Handler) AddKeeperWishlist(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	teamID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	playerKey := chi.URLParam(r, "playerKey")

	// Verify the user owns this team.
	var ownerUserID int64
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(user_id, 0) FROM teams WHERE id = $1", teamID,
	).Scan(&ownerUserID); err != nil {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	if ownerUserID != user.ID {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Enforce keeper limit: at most maxKeepersPerTeam per team. Updates to existing
	// entries are always allowed; only new entries are counted against the limit.
	var totalCount, playerExists int
	h.db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM keeper_wishlist WHERE team_id = $1),
			(SELECT COUNT(*) FROM keeper_wishlist WHERE team_id = $1 AND player_key = $2)
	`, teamID, playerKey).Scan(&totalCount, &playerExists) //nolint:errcheck — non-fatal; limit still checked
	if playerExists == 0 && totalCount >= h.config.MaxKeepersPerTeam {
		respondError(w, http.StatusUnprocessableEntity, "keeper limit reached")
		return
	}

	// Decode body; use defaults if body is absent or partial.
	req := addKeeperReq{YearsKept: 1}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — defaults already set
	if req.YearsKept < 1 {
		req.YearsKept = 1
	}

	var e wishlistEntryResp
	var pos string
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO keeper_wishlist (team_id, player_key, player_name, position, draft_cost, years_kept)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (team_id, player_key) DO UPDATE
			SET player_name = EXCLUDED.player_name,
			    position    = EXCLUDED.position,
			    draft_cost  = EXCLUDED.draft_cost,
			    years_kept  = EXCLUDED.years_kept
		RETURNING id, team_id, player_key, player_name, COALESCE(position, ''), draft_cost, years_kept
	`, teamID, playerKey, req.PlayerName, req.Position, req.DraftCost, req.YearsKept,
	).Scan(&e.ID, &e.TeamID, &e.PlayerKey, &e.PlayerName, &pos, &e.DraftCost, &e.YearsKept)
	if err != nil {
		log.Printf("[keepers] AddKeeperWishlist team %d player %s: %v", teamID, playerKey, err)
		respondError(w, http.StatusInternalServerError, "failed to save keeper")
		return
	}
	e.Position = pos

	respondJSON(w, http.StatusOK, e)
}

// --- RemoveKeeperWishlist ---

// RemoveKeeperWishlist handles DELETE /api/teams/{id}/keepers/{playerKey}.
// Removes a keeper wishlist entry. Only the team owner may call this.
func (h *Handler) RemoveKeeperWishlist(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	teamID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	playerKey := chi.URLParam(r, "playerKey")

	// Verify the user owns this team.
	var ownerUserID int64
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(user_id, 0) FROM teams WHERE id = $1", teamID,
	).Scan(&ownerUserID); err != nil {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	if ownerUserID != user.ID {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err = h.db.Exec(r.Context(),
		"DELETE FROM keeper_wishlist WHERE team_id = $1 AND player_key = $2",
		teamID, playerKey,
	)
	if err != nil {
		log.Printf("[keepers] RemoveKeeperWishlist team %d player %s: %v", teamID, playerKey, err)
		respondError(w, http.StatusInternalServerError, "failed to remove keeper")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- SubmitKeepers ---

// SubmitKeepers handles POST /api/teams/{id}/keepers/submit.
// Records that the team owner has finalised their keeper selections.
// Only the team owner may call this.
func (h *Handler) SubmitKeepers(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	teamID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Verify the user owns this team.
	var ownerUserID int64
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(user_id, 0) FROM teams WHERE id = $1", teamID,
	).Scan(&ownerUserID); err != nil {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	if ownerUserID != user.ID {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err = h.db.Exec(r.Context(), `
		INSERT INTO keeper_submissions (team_id, submitted_at, submitter_user_id)
		VALUES ($1, NOW(), $2)
		ON CONFLICT (team_id) DO UPDATE
			SET submitted_at      = NOW(),
			    submitter_user_id = EXCLUDED.submitter_user_id
	`, teamID, user.ID)
	if err != nil {
		log.Printf("[keepers] SubmitKeepers team %d: %v", teamID, err)
		respondError(w, http.StatusInternalServerError, "failed to submit keepers")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- UnsubmitKeepers ---

// UnsubmitKeepers handles DELETE /api/teams/{id}/keepers/submit.
// Retracts a team's keeper submission so they can make changes.
// Only the team owner may call this.
func (h *Handler) UnsubmitKeepers(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	teamID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Verify the user owns this team.
	var ownerUserID int64
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(user_id, 0) FROM teams WHERE id = $1", teamID,
	).Scan(&ownerUserID); err != nil {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	if ownerUserID != user.ID {
		respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	_, err = h.db.Exec(r.Context(),
		"DELETE FROM keeper_submissions WHERE team_id = $1", teamID,
	)
	if err != nil {
		log.Printf("[keepers] UnsubmitKeepers team %d: %v", teamID, err)
		respondError(w, http.StatusInternalServerError, "failed to unsubmit keepers")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- GetKeeperSummary ---

// keeperSummaryEntry is one team's keeper state for the commissioner overview.
type keeperSummaryEntry struct {
	TeamID      int64              `json:"team_id"`
	TeamName    string             `json:"team_name"`
	LogoURL     string             `json:"logo_url,omitempty"`
	Submitted   bool               `json:"submitted"`
	SubmittedAt *time.Time         `json:"submitted_at,omitempty"`
	Keepers     []wishlistEntryResp `json:"keepers"`
}

// GetKeeperSummary handles GET /api/leagues/{id}/keeper-summary.
// Returns every team in the league along with their wishlist and submission status.
// Restricted to the commissioner of the league (user's team must have is_commissioner=true).
func (h *Handler) GetKeeperSummary(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Verify the current user is commissioner of this league.
	var isComm bool
	h.db.QueryRow(r.Context(),
		"SELECT is_commissioner FROM teams WHERE league_id = $1 AND user_id = $2",
		leagueID, user.ID,
	).Scan(&isComm) //nolint:errcheck
	if !isComm {
		respondError(w, http.StatusForbidden, "forbidden — commissioner only")
		return
	}

	// Load all teams in the league.
	tRows, err := h.db.Query(r.Context(),
		`SELECT t.id, t.name, COALESCE(t.logo_url, ''),
		        ks.submitted_at IS NOT NULL,
		        ks.submitted_at
		 FROM teams t
		 LEFT JOIN keeper_submissions ks ON ks.team_id = t.id
		 WHERE t.league_id = $1
		 ORDER BY t.id`,
		leagueID,
	)
	if err != nil {
		log.Printf("[keepers] GetKeeperSummary teams query league %d: %v", leagueID, err)
		respondError(w, http.StatusInternalServerError, "failed to load teams")
		return
	}
	defer tRows.Close()

	var entries []keeperSummaryEntry
	teamIdx := make(map[int64]int) // teamID → index in entries
	for tRows.Next() {
		var e keeperSummaryEntry
		var submittedAt *time.Time
		if err := tRows.Scan(&e.TeamID, &e.TeamName, &e.LogoURL, &e.Submitted, &submittedAt); err != nil {
			continue
		}
		e.SubmittedAt = submittedAt
		e.Keepers = []wishlistEntryResp{}
		teamIdx[e.TeamID] = len(entries)
		entries = append(entries, e)
	}
	tRows.Close()

	// Load all wishlist entries for this league in one query.
	wRows, err := h.db.Query(r.Context(), `
		SELECT kw.id, kw.team_id, kw.player_key, kw.player_name,
		       COALESCE(kw.position, ''), kw.draft_cost, kw.years_kept
		FROM keeper_wishlist kw
		JOIN teams t ON t.id = kw.team_id
		WHERE t.league_id = $1
		ORDER BY kw.team_id, kw.id
	`, leagueID)
	if err != nil {
		log.Printf("[keepers] GetKeeperSummary wishlist query league %d: %v", leagueID, err)
		respondError(w, http.StatusInternalServerError, "failed to load wishlists")
		return
	}
	defer wRows.Close()

	for wRows.Next() {
		var e wishlistEntryResp
		var pos string
		if err := wRows.Scan(&e.ID, &e.TeamID, &e.PlayerKey, &e.PlayerName, &pos, &e.DraftCost, &e.YearsKept); err != nil {
			continue
		}
		e.Position = pos
		if idx, ok := teamIdx[e.TeamID]; ok {
			entries[idx].Keepers = append(entries[idx].Keepers, e)
		}
	}

	respondJSON(w, http.StatusOK, entries)
}
