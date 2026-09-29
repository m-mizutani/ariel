package google

import "net/http"

// NewOAuthForTest points every endpoint at baseURL (/auth, /token, /userinfo,
// /revoke) so tests can serve them with httptest.
func NewOAuthForTest(clientID, clientSecret, baseURL string) *OAuth {
	o := NewOAuth(clientID, clientSecret)
	o.authURL = baseURL + "/auth"
	o.tokenURL = baseURL + "/token"
	o.userinfoURL = baseURL + "/userinfo"
	o.revokeURL = baseURL + "/revoke"
	o.httpClient = http.DefaultClient
	return o
}
