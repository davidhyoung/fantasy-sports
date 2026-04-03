package handlers

import (
	"log"
	"net/http"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

// Sync fetches the logged-in user's Yahoo Fantasy leagues (NFL + NBA) and
// upserts them — along with their teams — into our local database.
//
// POST /api/sync
//
// Why POST? Because it causes a side effect (writes to the DB). GET should be
// safe and repeatable; syncing is neither — it hits Yahoo's API and modifies data.
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	// --- Create an authenticated Yahoo API client for this user ---
	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[sync] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	// --- Fetch all leagues for NFL and NBA ---
	games, err := yc.GetUserLeagues(r.Context(), "nfl", "nba")
	if err != nil {
		log.Printf("[sync] GetUserLeagues failed for user %d: %v", user.ID, err)
		respondError(w, http.StatusBadGateway, "failed to fetch leagues from Yahoo: "+err.Error())
		return
	}

	// --- Upsert each league and its teams into our DB ---
	var synced []models.League

	for _, game := range games {
		for _, yl := range game.Leagues.League {
			// Map Yahoo's game code to a readable sport name.
			sport := game.Code // "nfl" or "nba"

			// Upsert the league. ON CONFLICT targets yahoo_key so re-syncing
			// updates the name/season rather than creating duplicates.
			var league models.League
			err := h.db.QueryRow(r.Context(), `
				INSERT INTO leagues (name, sport, season, yahoo_key, user_id, logo_url)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (yahoo_key) DO UPDATE
				SET name     = EXCLUDED.name,
				    season   = EXCLUDED.season,
				    user_id  = EXCLUDED.user_id,
				    logo_url = EXCLUDED.logo_url
				RETURNING id, name, sport, season, yahoo_key, created_at
			`, yl.Name, sport, yl.Season, yl.LeagueKey, user.ID, yl.LogoURL,
			).Scan(&league.ID, &league.Name, &league.Sport, &league.Season, &league.YahooKey, &league.CreatedAt)
			if err != nil {
				log.Printf("[sync] upsert league %s failed: %v", yl.LeagueKey, err)
				continue
			}

			// --- Fetch and upsert teams for this league ---
			teams, err := yc.GetLeagueTeams(r.Context(), yl.LeagueKey)
			if err != nil {
				log.Printf("[sync] GetLeagueTeams %s failed: %v", yl.LeagueKey, err)
				// Don't abort the whole sync — log and keep going.
			}

			for _, yt := range teams {
				// Determine user_id and is_commissioner for teams owned by the current user.
				var teamUserID *int64
				isCommissioner := false
				if yt.IsOwnedByCurrentUser == "1" {
					teamUserID = &user.ID
					for _, mgr := range yt.Managers.Manager {
						if mgr.IsCommissioner == "1" {
							isCommissioner = true
							break
						}
					}
				}

				_, err := h.db.Exec(r.Context(), `
					INSERT INTO teams (league_id, name, yahoo_key, user_id, logo_url, is_commissioner)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT (yahoo_key) DO UPDATE
					SET name             = EXCLUDED.name,
					    league_id        = EXCLUDED.league_id,
					    user_id          = EXCLUDED.user_id,
					    logo_url         = EXCLUDED.logo_url,
					    is_commissioner  = EXCLUDED.is_commissioner
				`, league.ID, yt.Name, yt.TeamKey, teamUserID, yt.LogoURL(), isCommissioner)
				if err != nil {
					log.Printf("[sync] upsert team %s failed: %v", yt.TeamKey, err)
				}
			}

			synced = append(synced, league)
		}
	}

	log.Printf("[sync] user %d synced %d leagues", user.ID, len(synced))

	respondJSON(w, http.StatusOK, synced)
}
