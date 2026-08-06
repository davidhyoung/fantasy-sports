// Package yahoo provides OAuth 2.0 configuration and API helpers for
// the Yahoo Fantasy Sports API.
package yahoo

import "golang.org/x/oauth2"

// Endpoint contains Yahoo's OAuth 2.0 authorization and token URLs.
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://api.login.yahoo.com/oauth2/request_auth",
	TokenURL: "https://api.login.yahoo.com/oauth2/get_token",
}

// NewOAuthConfig builds the oauth2.Config used throughout the auth flow.
//
// Scopes:
//   - "fspt-r" — read access to Yahoo Fantasy Sports data (write not available for 3rd-party apps)
//
// We deliberately request ONLY fspt-r. Previously this also requested
// "openid", "profile" and "email" so we could call Yahoo's OIDC /userinfo
// endpoint for the user's name and email. That combination produced a token
// which authenticated fine against /userinfo but returned
// 403 "This application is not authorized to perform this action" on EVERY
// Fantasy Sports endpoint — including unprivileged ones like /game/nfl —
// which silently broke all live Yahoo data since ~March 2026.
//
// Yahoo's authorize endpoint accepts the combined scope set, so the failure
// only shows up at API-call time. Requesting fspt-r alone yields a plain
// Fantasy-scoped token. The cost is that /userinfo is no longer available, so
// Callback resolves the user's GUID from the Fantasy API instead and treats
// name/email as best-effort (see handlers/auth.go).
func NewOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"fspt-r"},
		Endpoint:     Endpoint,
	}
}
