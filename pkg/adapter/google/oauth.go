// Package google wraps Google's OAuth 2.0 and OpenID Connect endpoints.
package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"golang.org/x/oauth2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/safe"
)

// Endpoints documented in "Using OAuth 2.0 for Web Server Applications" and in
// Google's OpenID Connect discovery document.
const (
	authURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenURL    = "https://oauth2.googleapis.com/token"
	userinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	revokeURL   = "https://oauth2.googleapis.com/revoke"
)

// OAuth calls Google's authorization, token, userinfo, and revocation
// endpoints for one OAuth client.
type OAuth struct {
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	userinfoURL  string
	revokeURL    string
	httpClient   *http.Client
}

var _ interfaces.GoogleOAuth = &OAuth{}

func NewOAuth(clientID, clientSecret string) *OAuth {
	return &OAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		authURL:      authURL,
		tokenURL:     tokenURL,
		userinfoURL:  userinfoURL,
		revokeURL:    revokeURL,
		httpClient:   http.DefaultClient,
	}
}

func (o *OAuth) config(redirectURI string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.clientID,
		ClientSecret: o.clientSecret,
		// Google's web server flow documents client_id and client_secret
		// in the request body; auto-detection would try a header first.
		Endpoint: oauth2.Endpoint{
			AuthURL:   o.authURL,
			TokenURL:  o.tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirectURI,
		Scopes:      scopes,
	}
}

// AuthorizeURL asks for offline access with the consent prompt: Google returns
// a refresh token only on the first authorization unless prompt=consent is
// set, and a user who disconnected and connects again needs a new one.
func (o *OAuth) AuthorizeURL(state, redirectURI string, scopes []string) string {
	return o.config(redirectURI, scopes).AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (o *OAuth) ExchangeCode(ctx context.Context, code, redirectURI string) (*model.GoogleOAuthResult, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.httpClient)
	tok, err := o.config(redirectURI, nil).Exchange(ctx, code)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to exchange google oauth code")
	}

	var scopes []string
	if s, ok := tok.Extra("scope").(string); ok {
		scopes = strings.Fields(s)
	}
	return &model.GoogleOAuthResult{
		AccessToken:  model.GoogleAccessToken(tok.AccessToken),
		RefreshToken: model.GoogleRefreshToken(tok.RefreshToken),
		Scopes:       scopes,
	}, nil
}

type userinfoResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

func (o *OAuth) FetchIdentity(ctx context.Context, accessToken model.GoogleAccessToken) (*model.GoogleIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.userinfoURL, nil)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to build google userinfo request")
	}
	req.Header.Set("Authorization", "Bearer "+string(accessToken))

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, goerr.Wrap(err, "google userinfo request failed")
	}
	defer safe.Close(ctx, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, goerr.New("google userinfo request failed", goerr.V("status", resp.StatusCode))
	}
	var body userinfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, goerr.Wrap(err, "failed to decode google userinfo response")
	}
	return &model.GoogleIdentity{Subject: body.Sub, Email: body.Email}, nil
}

// Revoke reports 400 as ErrGoogleTokenInvalid. Google does not document the
// error body of the revocation endpoint, so the status code is the only
// stable signal that the token itself was rejected.
func (o *OAuth) Revoke(ctx context.Context, token string) error {
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return goerr.Wrap(err, "failed to build google revocation request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return goerr.Wrap(err, "google token revocation request failed")
	}
	defer safe.Close(ctx, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return goerr.Wrap(interfaces.ErrGoogleTokenInvalid, "google rejected the token on revocation")
	default:
		return goerr.New("google token revocation failed", goerr.V("status", resp.StatusCode))
	}
}
