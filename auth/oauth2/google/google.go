package google

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// OAuth2Config creates a preconfigured *oauth2.Config for Google OAuth2 / OIDC.
// The request scopes are the Google sign-in scopes.
func OAuth2Config(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"openid",
			"email",
		},
		Endpoint: google.Endpoint,
	}
}
