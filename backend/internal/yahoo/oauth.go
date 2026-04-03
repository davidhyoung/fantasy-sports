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
//   - "fspt-r"  — read access to Yahoo Fantasy Sports data (write not available for 3rd-party apps)
//   - "openid"  — enables OpenID Connect; lets us call Yahoo's /userinfo endpoint
//   - "profile" — includes name in the /userinfo response
//   - "email"   — includes email address in the /userinfo response
func NewOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"fspt-r", "openid", "profile", "email"},
		Endpoint:     Endpoint,
	}
}
