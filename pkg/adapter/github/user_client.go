package github

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/safe"
)

// UserClientFactory builds REST API clients authenticated with one user's
// access token.
type UserClientFactory struct {
	apiBaseURL string
	httpClient *http.Client
}

var _ interfaces.GitHubUserClientFactory = &UserClientFactory{}

// NewUserClientFactory takes the HTTP client that carries the request timeout.
func NewUserClientFactory(httpClient *http.Client) *UserClientFactory {
	return &UserClientFactory{apiBaseURL: apiBaseURL, httpClient: httpClient}
}

func (f *UserClientFactory) New(token model.GitHubAccessToken) interfaces.GitHubUserClient {
	return &userClient{apiBaseURL: f.apiBaseURL, httpClient: f.httpClient, token: token}
}

type userClient struct {
	apiBaseURL string
	httpClient *http.Client
	token      model.GitHubAccessToken
}

type userResponse struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func (c *userClient) GetUser(ctx context.Context) (*model.GitHubIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/user", nil)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to build github user request")
	}
	req.Header.Set("Authorization", "Bearer "+string(c.token))
	setAPIHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, goerr.Wrap(err, "github user request failed")
	}
	defer safe.Close(ctx, resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, goerr.Wrap(interfaces.ErrGitHubTokenInvalid, "github rejected the user token", goerr.V("status", resp.StatusCode))
	default:
		return nil, goerr.New("github user request failed", goerr.V("status", resp.StatusCode))
	}

	var body userResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, goerr.Wrap(err, "failed to decode github user response")
	}
	return &model.GitHubIdentity{ID: model.GitHubUserID(body.ID), Login: body.Login}, nil
}
