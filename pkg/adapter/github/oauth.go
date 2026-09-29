// Package github wraps the OAuth endpoints of a GitHub App and the parts of
// the GitHub REST API that Ariel calls with a user access token.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/utils/safe"
)

const (
	apiBaseURL = "https://api.github.com"
	// apiVersion is the REST API version the requests are written against,
	// as given in the "REST API endpoints for OAuth authorizations" reference.
	apiVersion = "2026-03-10"

	// errorBadRefreshToken is the error code GitHub returns for a refresh
	// token that is invalid, expired, or already used.
	errorBadRefreshToken = "bad_refresh_token"
)

// OAuth calls the authorization, token, and authorization-deletion endpoints
// of one GitHub App.
type OAuth struct {
	clientID     string
	clientSecret string
	endpoint     oauth2.Endpoint
	apiBaseURL   string
	httpClient   *http.Client
	now          func() time.Time
}

var _ interfaces.GitHubOAuth = &OAuth{}

// NewOAuth builds the client of the GitHub App identified by clientID.
// httpClient carries the request timeout.
func NewOAuth(clientID, clientSecret string, httpClient *http.Client) *OAuth {
	return &OAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		endpoint:     endpoints.GitHub,
		apiBaseURL:   apiBaseURL,
		httpClient:   httpClient,
		now:          time.Now,
	}
}

func (o *OAuth) config(redirectURI string) *oauth2.Config {
	endpoint := o.endpoint
	// GitHub documents client_id and client_secret in the request body;
	// auto-detection would try a header first.
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	return &oauth2.Config{
		ClientID:     o.clientID,
		ClientSecret: o.clientSecret,
		Endpoint:     endpoint,
		RedirectURL:  redirectURI,
	}
}

// AuthorizeURL asks GitHub for a user access token with a PKCE challenge.
// allow_signup=false keeps the page from offering to create a GitHub account,
// since the account has to belong to the organization that installed the app.
func (o *OAuth) AuthorizeURL(redirectURI, state, codeVerifier string) string {
	return o.config(redirectURI).AuthCodeURL(state,
		oauth2.S256ChallengeOption(codeVerifier),
		oauth2.SetAuthURLParam("allow_signup", "false"))
}

func (o *OAuth) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (*model.GitHubToken, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.httpClient)
	tok, err := o.config(redirectURI).Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return nil, wrapTokenError(err, "failed to exchange github oauth code")
	}
	return o.toGitHubToken(tok), nil
}

func (o *OAuth) Refresh(ctx context.Context, refreshToken model.GitHubRefreshToken) (*model.GitHubToken, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.httpClient)
	// An expiry in the past makes the token source refresh at once.
	expired := &oauth2.Token{RefreshToken: string(refreshToken), Expiry: time.Unix(1, 0)}
	tok, err := o.config("").TokenSource(ctx, expired).Token()
	if err != nil {
		return nil, wrapTokenError(err, "failed to refresh github user token")
	}
	return o.toGitHubToken(tok), nil
}

// wrapTokenError keeps only GitHub's error code; the response body can echo
// request values.
func wrapTokenError(err error, msg string) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		vals := []goerr.Option{goerr.V("github_error", retrieve.ErrorCode)}
		if retrieve.Response != nil {
			vals = append(vals, goerr.V("status", retrieve.Response.StatusCode))
		}
		if retrieve.ErrorCode == errorBadRefreshToken {
			return goerr.Wrap(interfaces.ErrGitHubTokenInvalid, msg, vals...)
		}
		return goerr.New(msg, vals...)
	}
	return goerr.Wrap(err, msg)
}

// toGitHubToken reads both lifetimes from the response fields. oauth2 fills
// ExpiresIn only for a JSON body and keeps refresh_token_expires_in only as
// an extra field, so both are read the same way. A token without expires_in
// does not expire.
func (o *OAuth) toGitHubToken(tok *oauth2.Token) *model.GitHubToken {
	now := o.now()
	out := &model.GitHubToken{
		AccessToken:  model.GitHubAccessToken(tok.AccessToken),
		RefreshToken: model.GitHubRefreshToken(tok.RefreshToken),
	}
	if secs := extraSeconds(tok.Extra("expires_in")); secs > 0 {
		out.AccessTokenExpiresAt = now.Add(time.Duration(secs) * time.Second)
	}
	if secs := extraSeconds(tok.Extra("refresh_token_expires_in")); secs > 0 {
		out.RefreshTokenExpiresAt = now.Add(time.Duration(secs) * time.Second)
	}
	return out
}

// extraSeconds reads a number that oauth2 decoded from a JSON body (float64)
// or a form body (int64).
func extraSeconds(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	default:
		return 0
	}
}

type revokeRequest struct {
	AccessToken string `json:"access_token"`
}

// RevokeGrant calls "Delete an app authorization", which deletes every token
// of this app for the user, not only accessToken.
func (o *OAuth) RevokeGrant(ctx context.Context, accessToken model.GitHubAccessToken) error {
	body, err := json.Marshal(revokeRequest{AccessToken: string(accessToken)})
	if err != nil {
		return goerr.Wrap(err, "failed to encode github grant deletion request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		o.apiBaseURL+"/applications/"+o.clientID+"/grant", bytes.NewReader(body))
	if err != nil {
		return goerr.Wrap(err, "failed to build github grant deletion request")
	}
	req.SetBasicAuth(o.clientID, o.clientSecret)
	req.Header.Set("Content-Type", "application/json")
	setAPIHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return goerr.Wrap(err, "github grant deletion request failed")
	}
	defer safe.Close(ctx, resp.Body)

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return goerr.Wrap(interfaces.ErrGitHubTokenInvalid, "github does not know the token", goerr.V("status", resp.StatusCode))
	default:
		return goerr.New("github grant deletion failed", goerr.V("status", resp.StatusCode))
	}
}

func setAPIHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
}
