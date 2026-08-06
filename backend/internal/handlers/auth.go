package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

const sessionName = "fantasy-session"

// generateState creates a cryptographically random string used as the OAuth
// "state" parameter. This prevents CSRF attacks: we store it in the session
// before redirecting to Yahoo, then verify Yahoo echoed it back unchanged.
func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// Login starts the OAuth 2.0 flow by redirecting the user to Yahoo's login page.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateState()

	session, err := h.sessions.Get(r, sessionName)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	// Stash the state so we can verify it in Callback.
	session.Values["oauth_state"] = state
	if err := session.Save(r, w); err != nil {
		http.Error(w, "failed to save session", http.StatusInternalServerError)
		return
	}

	// AuthCodeURL builds the Yahoo login URL with our client ID, scopes, and state.
	http.Redirect(w, r, h.oauthConfig.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

// yahooUserInfo maps the fields we care about from Yahoo's OpenID Connect
// /userinfo endpoint. "sub" is Yahoo's stable unique identifier (GUID).
type yahooUserInfo struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Callback handles the redirect back from Yahoo after the user grants access.
// It: verifies the state, exchanges the auth code for tokens, fetches the
// user's profile, upserts them in our DB, and sets a session cookie.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	// Log all query params Yahoo sent back — useful for debugging auth issues.
	log.Printf("[auth/callback] query params: %v", r.URL.Query())

	session, err := h.sessions.Get(r, sessionName)
	if err != nil {
		// A decode error means the cookie is corrupted or the SESSION_SECRET changed.
		log.Printf("[auth/callback] session error: %v", err)
		http.Error(w, "session error — try clearing your cookies and logging in again", http.StatusInternalServerError)
		return
	}
	log.Printf("[auth/callback] session.IsNew=%v, stored state=%v", session.IsNew, session.Values["oauth_state"])

	// --- Check if Yahoo returned an error instead of a code ---
	// This happens when: the user denies access, or the app is misconfigured
	// (wrong scopes, missing permissions, redirect URI mismatch, etc.).
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		desc := r.URL.Query().Get("error_description")
		log.Printf("[auth/callback] Yahoo returned error: %s — %s", oauthErr, desc)
		http.Error(w, "Yahoo auth error: "+oauthErr+": "+desc, http.StatusBadRequest)
		return
	}

	// --- CSRF check ---
	expectedState, ok := session.Values["oauth_state"].(string)
	if !ok || expectedState == "" || r.URL.Query().Get("state") != expectedState {
		log.Printf("[auth/callback] state mismatch: expected=%q got=%q ok=%v", expectedState, r.URL.Query().Get("state"), ok)
		http.Error(w, "invalid oauth state — try logging in again", http.StatusBadRequest)
		return
	}
	delete(session.Values, "oauth_state") // no longer needed

	// --- Exchange the one-time code for access + refresh tokens ---
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Printf("[auth/callback] code param is empty — full query: %v", r.URL.RawQuery)
		http.Error(w, "missing code parameter from Yahoo", http.StatusBadRequest)
		return
	}
	token, err := h.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("[auth/callback] token exchange failed: %v", err)
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Yahoo omits the scope field even when the grant matches the request
	// (RFC 6749 §5.1 makes it optional), so an empty granted_scope proves
	// nothing on its own — but it's still the fastest signal when debugging
	// authorization failures.
	grantedScope, _ := token.Extra("scope").(string)
	if grantedScope != "" && !strings.Contains(grantedScope, "fspt-r") {
		log.Printf("[auth/callback] Yahoo granted a scope set without fspt-r (%q) — Fantasy API calls will fail", grantedScope)
	}

	// oauthConfig.Client returns an *http.Client that automatically attaches
	// the access token to every request it makes.
	yahooClient := h.oauthConfig.Client(r.Context(), token)

	// --- Resolve the user's GUID from the Fantasy API ---
	// We ask Fantasy (not OIDC /userinfo) because we no longer request the
	// openid scope — see yahoo.NewOAuthConfig for why. This doubles as a live
	// check that the token actually carries Fantasy access.
	var info yahooUserInfo
	guid, err := fetchFantasyGUID(yahooClient)
	if err != nil {
		log.Printf("[auth/callback] failed to resolve Yahoo GUID from Fantasy API: %v", err)
		http.Error(w, "could not reach Yahoo Fantasy API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	info.Sub = guid
	log.Printf("[auth/callback] resolved Yahoo GUID from Fantasy API: %s", guid)

	// --- Best-effort profile enrichment ---
	// Only succeeds if the token happens to carry OIDC scopes. Failure is
	// expected and non-fatal: name/email are cosmetic, and the upsert below
	// preserves any values already stored for an existing user.
	if resp, err := yahooClient.Get("https://api.login.yahoo.com/openid/v1/userinfo"); err == nil {
		defer resp.Body.Close()
		if body, err := io.ReadAll(resp.Body); err == nil && resp.StatusCode == http.StatusOK {
			var oidc yahooUserInfo
			if err := json.Unmarshal(body, &oidc); err == nil {
				info.Name, info.Email = oidc.Name, oidc.Email
			}
		} else {
			log.Printf("[auth/callback] userinfo unavailable (expected without openid scope): status=%d", resp.StatusCode)
		}
	}

	// --- Save / update user in our database ---
	user, err := h.upsertUser(r.Context(), info, token)
	if err != nil {
		http.Error(w, "failed to save user", http.StatusInternalServerError)
		return
	}

	// --- Store user ID in the session cookie ---
	session.Values["user_id"] = user.ID
	if err := session.Save(r, w); err != nil {
		http.Error(w, "failed to save session", http.StatusInternalServerError)
		return
	}

	// Redirect to the frontend root. Because we're running dev through Vite's
	// proxy, "/" resolves back to the React app on :5173.
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// fetchFantasyGUID resolves the authenticated user's Yahoo GUID via the
// Fantasy Sports API. Used instead of the OIDC /userinfo endpoint because we
// only request the fspt-r scope (see yahoo.NewOAuthConfig).
func fetchFantasyGUID(client *http.Client) (string, error) {
	resp, err := client.Get("https://fantasysports.yahooapis.com/fantasy/v2/users;use_login=1")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("yahoo returned %d: %s", resp.StatusCode, string(body))
	}

	var fc yahoo.FantasyContent
	if err := xml.Unmarshal(body, &fc); err != nil {
		return "", fmt.Errorf("parsing users response: %w", err)
	}
	if fc.Users == nil || len(fc.Users.User) == 0 || fc.Users.User[0].GUID == "" {
		return "", fmt.Errorf("no guid in response: %s", string(body))
	}
	return fc.Users.User[0].GUID, nil
}

// Me returns the profile of the currently logged-in user as JSON.
// The auth middleware must run before this handler to populate the context.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	respondJSON(w, http.StatusOK, user)
}

// Logout clears the session cookie, effectively signing the user out.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessions.Get(r, sessionName)
	// Setting MaxAge to -1 tells the browser to delete the cookie immediately.
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// upsertUser inserts a new user or updates their tokens and display name if
// they've logged in before. "ON CONFLICT ... DO UPDATE" is Postgres's way of
// saying "insert, but if the yahoo_guid already exists, update instead."
func (h *Handler) upsertUser(ctx context.Context, info yahooUserInfo, token *oauth2.Token) (*models.User, error) {
	// display_name is NOT NULL, so fall back to a placeholder for brand-new
	// users when OIDC profile data isn't available.
	displayName := info.Name
	if displayName == "" {
		displayName = "Yahoo User"
	}

	var user models.User
	// COALESCE/NULLIF on update: never overwrite a previously-stored name or
	// email with an empty value, since name/email are best-effort now that we
	// only request the fspt-r scope.
	err := h.db.QueryRow(ctx, `
		INSERT INTO users (yahoo_guid, display_name, email, access_token, refresh_token, token_expiry)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (yahoo_guid) DO UPDATE
		SET display_name  = COALESCE(NULLIF(EXCLUDED.display_name, 'Yahoo User'), users.display_name),
		    email         = COALESCE(NULLIF(EXCLUDED.email, ''), users.email),
		    access_token  = EXCLUDED.access_token,
		    refresh_token = EXCLUDED.refresh_token,
		    token_expiry  = EXCLUDED.token_expiry
		RETURNING id, yahoo_guid, display_name, email, created_at
	`, info.Sub, displayName, info.Email,
		token.AccessToken, token.RefreshToken, token.Expiry,
	).Scan(&user.ID, &user.YahooGUID, &user.DisplayName, &user.Email, &user.CreatedAt)

	if err != nil {
		return nil, err
	}
	return &user, nil
}
