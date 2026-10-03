package interfaces

import (
	"context"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

// GoogleOAuth talks to Google's OAuth 2.0 and OpenID Connect endpoints.
type GoogleOAuth interface {
	// AuthorizeURL returns the authorization URL. It always asks for offline
	// access with the consent prompt, so Google returns a refresh token.
	AuthorizeURL(state, redirectURI string, scopes []string) string
	ExchangeCode(ctx context.Context, code, redirectURI string) (*model.GoogleOAuthResult, error)
	FetchIdentity(ctx context.Context, accessToken model.GoogleAccessToken) (*model.GoogleIdentity, error)
	// Revoke revokes an access or refresh token. A token Google rejects as
	// invalid is reported as ErrGoogleTokenInvalid.
	Revoke(ctx context.Context, token string) error
}
