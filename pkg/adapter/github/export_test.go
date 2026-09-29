package github

import (
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// NewOAuthForTest points the authorization and token endpoints at
// baseURL+"/login/oauth/..." and the REST API at baseURL, and fixes the clock.
func NewOAuthForTest(clientID, clientSecret, baseURL string, now func() time.Time) *OAuth {
	o := NewOAuth(clientID, clientSecret, http.DefaultClient)
	o.endpoint = oauth2.Endpoint{
		AuthURL:  baseURL + "/login/oauth/authorize",
		TokenURL: baseURL + "/login/oauth/access_token",
	}
	o.apiBaseURL = baseURL
	o.now = now
	return o
}

func NewUserClientFactoryForTest(baseURL string) *UserClientFactory {
	f := NewUserClientFactory(http.DefaultClient)
	f.apiBaseURL = baseURL
	return f
}
