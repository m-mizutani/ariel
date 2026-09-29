package notion

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// OAuth calls the authorization, token, and revocation endpoints of one Notion
// public integration.
type OAuth struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
}

var _ interfaces.NotionOAuth = &OAuth{}

// NewOAuth builds the client. baseURL is the Notion API origin, such as
// https://api.notion.com.
func NewOAuth(clientID, clientSecret, baseURL string, httpClient *http.Client) *OAuth {
	return &OAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		httpClient:   httpClient,
	}
}

// AuthorizeURL returns the page where the user picks the workspace and the
// pages to share. owner=user is the only value Notion documents.
func (o *OAuth) AuthorizeURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", o.clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("owner", "user")
	params.Set("state", state)
	return o.baseURL + "/v1/oauth/authorize?" + params.Encode()
}

type tokenResponse struct {
	AccessToken   string  `json:"access_token"`
	RefreshToken  *string `json:"refresh_token"`
	BotID         string  `json:"bot_id"`
	WorkspaceID   string  `json:"workspace_id"`
	WorkspaceName *string `json:"workspace_name"`
	Owner         struct {
		Type string `json:"type"`
		User *struct {
			ID   string  `json:"id"`
			Name *string `json:"name"`
		} `json:"user"`
	} `json:"owner"`
}

func (r *tokenResponse) result() *model.NotionOAuthResult {
	res := &model.NotionOAuthResult{
		Tokens:        model.NotionTokens{AccessToken: model.NotionAccessToken(r.AccessToken)},
		BotID:         r.BotID,
		WorkspaceID:   model.NotionWorkspaceID(strings.ToLower(r.WorkspaceID)),
		WorkspaceName: deref(r.WorkspaceName),
		OwnerType:     r.Owner.Type,
	}
	if r.RefreshToken != nil {
		res.Tokens.RefreshToken = model.NotionRefreshToken(*r.RefreshToken)
	}
	if r.Owner.User != nil {
		res.OwnerUserID = model.NotionUserID(r.Owner.User.ID)
		res.OwnerName = deref(r.Owner.User.Name)
	}
	return res
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// token calls the token endpoint with client credentials in Basic auth, as
// Notion's "Create a token" and "Refresh a token" references specify.
func (o *OAuth) token(ctx context.Context, body map[string]string, msg string) (*model.NotionOAuthResult, error) {
	req, err := newRequest(ctx, http.MethodPost, o.baseURL+"/v1/oauth/token", body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(o.clientID, o.clientSecret)

	var resp tokenResponse
	if err := do(ctx, o.httpClient, req, &resp, msg); err != nil {
		return nil, err
	}
	return resp.result(), nil
}

func (o *OAuth) ExchangeCode(ctx context.Context, code, redirectURI string) (*model.NotionOAuthResult, error) {
	return o.token(ctx, map[string]string{
		"grant_type":   "authorization_code",
		"code":         code,
		"redirect_uri": redirectURI,
	}, "failed to exchange notion oauth code")
}

func (o *OAuth) RefreshToken(ctx context.Context, refreshToken model.NotionRefreshToken) (*model.NotionOAuthResult, error) {
	return o.token(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": string(refreshToken),
	}, "failed to refresh notion token")
}

func (o *OAuth) Revoke(ctx context.Context, token model.NotionAccessToken) error {
	req, err := newRequest(ctx, http.MethodPost, o.baseURL+"/v1/oauth/revoke", map[string]string{"token": string(token)})
	if err != nil {
		return err
	}
	req.SetBasicAuth(o.clientID, o.clientSecret)
	if err := do(ctx, o.httpClient, req, nil, "failed to revoke notion token"); err != nil {
		return err
	}
	return nil
}
