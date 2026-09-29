package interfaces

import (
	"context"
	"encoding/json"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// NotionOAuth talks to the OAuth endpoints of a Notion public integration.
type NotionOAuth interface {
	AuthorizeURL(state, redirectURI string) string
	ExchangeCode(ctx context.Context, code, redirectURI string) (*model.NotionOAuthResult, error)
	// RefreshToken exchanges a refresh token for a new token pair. A refresh
	// token Notion rejects is reported as ErrNotionTokenInvalid.
	RefreshToken(ctx context.Context, refreshToken model.NotionRefreshToken) (*model.NotionOAuthResult, error)
	// Revoke revokes an access token. A token Notion rejects as invalid is
	// reported as ErrNotionTokenInvalid.
	Revoke(ctx context.Context, token model.NotionAccessToken) error
}

// NotionClientFactory builds a client authenticated with one user's access
// token.
type NotionClientFactory interface {
	New(token model.NotionAccessToken) NotionClient
}

// NotionClient calls the read-only Notion API with one user's access token.
// A rejected token is reported as ErrNotionTokenInvalid.
type NotionClient interface {
	Search(ctx context.Context, q model.NotionSearchQuery) (*model.NotionList, error)
	GetPage(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error)
	ListBlockChildren(ctx context.Context, id model.NotionObjectID, page model.NotionPagination) (*model.NotionList, error)
	GetDatabase(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error)
	GetDataSource(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error)
	QueryDataSource(ctx context.Context, id model.NotionObjectID, q model.NotionDataSourceQuery) (*model.NotionList, error)
}
