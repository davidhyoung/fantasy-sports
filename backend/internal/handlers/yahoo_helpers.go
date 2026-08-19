package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/leaguesettings"
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

// leagueSettingsSource resolves the leaguesettings.Source for a league,
// dispatching on leagues.source: a native league reads its own league_settings
// row (no external league behind it at all); a yahoo league goes through the
// live Yahoo API (or mock client in dev). Same calling convention as
// leagueYahooKey — status 0 means success. defaultBudget is the league's own
// configured budget for native leagues (0 for yahoo leagues, which have no
// budget concept synced from Yahoo — callers fall back to a config default).
func (h *Handler) leagueSettingsSource(r *http.Request, user *models.User, leagueID int64) (src leaguesettings.Source, defaultBudget int, status int, msg string) {
	var source, yahooKey string
	err := h.db.QueryRow(r.Context(),
		"SELECT source, COALESCE(yahoo_key, '') FROM leagues WHERE id = $1", leagueID,
	).Scan(&source, &yahooKey)
	if err != nil {
		return nil, 0, http.StatusNotFound, "league not found"
	}
	if source == "native" {
		if budget, ok := leaguesettings.Budget(r.Context(), h.db, leagueID); ok {
			defaultBudget = budget
		}
		return leaguesettings.NewNativeSource(h.db, leagueID), defaultBudget, 0, ""
	}
	if yahooKey == "" {
		return nil, 0, http.StatusUnprocessableEntity, "league has no yahoo_key — sync from Yahoo first"
	}
	yc, err := h.newYahooClient(r, user)
	if err != nil {
		return nil, 0, http.StatusUnauthorized, "auth error"
	}
	return leaguesettings.NewYahooSource(yc, yahooKey), 0, 0, ""
}

// newYahooClient loads the user's tokens from the DB and creates an authenticated Yahoo client.
func (h *Handler) newYahooClient(r *http.Request, user *models.User) (*yahoo.Client, error) {
	// Mock mode short-circuits before touching tokens, so a dev session with no
	// valid Yahoo credentials still works.
	if h.config.MockYahoo {
		return yahoo.NewMockClient(), nil
	}

	accessToken, refreshToken, expiry, err := h.userTokens(r, user.ID)
	if err != nil {
		log.Printf("[yahoo] failed to load tokens for user %d: %v", user.ID, err)
		return nil, err
	}
	yc := yahoo.NewClient(r.Context(), h.db, h.oauthConfig, user.ID, accessToken, refreshToken, expiry)
	return yc, nil
}
