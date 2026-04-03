package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// leagueYahooKey loads the yahoo_key for a league by its internal DB id.
// Returns an error string and HTTP status if it fails.
func (h *Handler) leagueYahooKey(r *http.Request, id int64) (string, int, string) {
	var yahooKey string
	err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(yahoo_key, '') FROM leagues WHERE id = $1", id,
	).Scan(&yahooKey)
	if err != nil {
		return "", http.StatusNotFound, "league not found"
	}
	if yahooKey == "" {
		return "", http.StatusUnprocessableEntity, "league has no yahoo_key — sync from Yahoo first"
	}
	return yahooKey, 0, ""
}

// userTokens loads the OAuth tokens for the given user from the DB.
func (h *Handler) userTokens(r *http.Request, userID int64) (string, string, time.Time, error) {
	var accessToken, refreshToken string
	var expiry time.Time
	err := h.db.QueryRow(r.Context(),
		"SELECT access_token, refresh_token, token_expiry FROM users WHERE id = $1",
		userID,
	).Scan(&accessToken, &refreshToken, &expiry)
	return accessToken, refreshToken, expiry, err
}

// newYahooClient loads the user's tokens from the DB and creates an authenticated Yahoo client.
func (h *Handler) newYahooClient(r *http.Request, user *models.User) (*yahoo.Client, error) {
	accessToken, refreshToken, expiry, err := h.userTokens(r, user.ID)
	if err != nil {
		log.Printf("[yahoo] failed to load tokens for user %d: %v", user.ID, err)
		return nil, err
	}
	yc := yahoo.NewClient(r.Context(), h.db, h.oauthConfig, user.ID, accessToken, refreshToken, expiry)
	return yc, nil
}
