package model

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// NotionWorkspaceID is the ID of a Notion workspace: a lower-case UUID with
// hyphens.
type NotionWorkspaceID string

// NotionObjectID is the ID of a Notion page, block, database, or data source.
// Notion accepts it with or without hyphens.
type NotionObjectID string

// NotionUserID is the ID of a Notion user. It names the owner record of a
// Notion account, so it is used as a Firestore document ID.
type NotionUserID string

// NotionAccessToken is a plaintext Notion access token. It is never persisted
// and never logged; only its KMS ciphertext is stored.
type NotionAccessToken string

// NotionRefreshToken is a plaintext Notion refresh token. It is never
// persisted and never logged; only its KMS ciphertext is stored.
type NotionRefreshToken string

var (
	notionWorkspaceIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	notionUUIDPattern        = regexp.MustCompile(`^(?i:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
)

func (x NotionWorkspaceID) Validate() error {
	if !notionWorkspaceIDPattern.MatchString(string(x)) {
		return goerr.New("invalid notion workspace ID", goerr.V("workspace_id", string(x)))
	}
	return nil
}

// Validate accepts only a UUID, so the ID can be put into a URL path.
func (x NotionObjectID) Validate() error {
	if !notionUUIDPattern.MatchString(string(x)) {
		return goerr.New("invalid notion object ID", goerr.V("object_id", string(x)))
	}
	return nil
}

// Validate accepts only a UUID, so the ID can be used as a document ID.
func (x NotionUserID) Validate() error {
	if !notionUUIDPattern.MatchString(string(x)) {
		return goerr.New("invalid notion user ID", goerr.V("notion_user_id", string(x)))
	}
	return nil
}

// NotionTokens is the token pair Notion issues together. It is encrypted as
// one JSON document, so both tokens are always replaced together.
type NotionTokens struct {
	AccessToken  NotionAccessToken  `json:"access_token" masq:"secret"`
	RefreshToken NotionRefreshToken `json:"refresh_token" masq:"secret"`
}

// NotionOAuthResult is what Notion's token endpoint returns for an
// authorization code or a refresh token.
type NotionOAuthResult struct {
	// Tokens.RefreshToken is empty when Notion returned none.
	Tokens        NotionTokens
	BotID         string
	WorkspaceID   NotionWorkspaceID
	WorkspaceName string
	// OwnerType is "user" for an authorization by a user, "workspace" for an
	// internal integration.
	OwnerType   string
	OwnerUserID NotionUserID
	OwnerName   string
}

// NotionCredential holds one user's Notion tokens as KMS ciphertext and the
// Notion user and workspace they belong to.
type NotionCredential struct {
	TeamID         SlackTeamID
	UserID         SlackUserID
	Tokens         EncryptedData
	WorkspaceID    NotionWorkspaceID
	WorkspaceName  string
	BotID          string
	NotionUserID   NotionUserID
	NotionUserName string
	// NeedsReconnect is set when Notion rejected the refresh token: the user
	// has to authorize Ariel again.
	NeedsReconnect bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (c *NotionCredential) Key() UserKey {
	return UserKey{TeamID: c.TeamID, UserID: c.UserID}
}

func (c *NotionCredential) Validate() error {
	if err := c.Key().Validate(); err != nil {
		return goerr.Wrap(err, "invalid notion credential key")
	}
	if err := c.Tokens.Validate(); err != nil {
		return goerr.Wrap(err, "invalid notion credential tokens")
	}
	if err := c.WorkspaceID.Validate(); err != nil {
		return goerr.Wrap(err, "invalid notion credential workspace")
	}
	if c.BotID == "" {
		return goerr.New("empty notion credential bot_id")
	}
	if err := c.NotionUserID.Validate(); err != nil {
		return goerr.Wrap(err, "invalid notion credential user")
	}
	if c.CreatedAt.IsZero() {
		return goerr.New("empty notion credential created_at")
	}
	if c.UpdatedAt.IsZero() {
		return goerr.New("empty notion credential updated_at")
	}
	return nil
}

// NotionAccount records which user a Notion account is connected to, so that
// one Notion account is never connected to two users. Whether Notion issues
// one shared connection for two authorizations by the same Notion user is not
// documented; if it does, one user's disconnection would end the other's.
type NotionAccount struct {
	NotionUserID NotionUserID
	TeamID       SlackTeamID
	UserID       SlackUserID
	CreatedAt    time.Time
}

func (a *NotionAccount) Key() UserKey {
	return UserKey{TeamID: a.TeamID, UserID: a.UserID}
}

// NotionPagination selects one page of a list. PageSize 0 lets Notion choose
// (100).
type NotionPagination struct {
	StartCursor string
	PageSize    int
}

// notionMaxPageSize is the largest page_size Notion accepts.
const notionMaxPageSize = 100

func (x NotionPagination) Validate() error {
	if x.PageSize < 0 || x.PageSize > notionMaxPageSize {
		return goerr.New("notion page size is out of range", goerr.V("page_size", x.PageSize))
	}
	return nil
}

// NotionObjectType narrows a search to one kind of object.
type NotionObjectType string

const (
	NotionObjectTypeAny        NotionObjectType = ""
	NotionObjectTypePage       NotionObjectType = "page"
	NotionObjectTypeDataSource NotionObjectType = "data_source"
)

func (x NotionObjectType) Validate() error {
	switch x {
	case NotionObjectTypeAny, NotionObjectTypePage, NotionObjectTypeDataSource:
		return nil
	default:
		return goerr.New("invalid notion object type", goerr.V("object_type", string(x)))
	}
}

// NotionSearchQuery searches the pages and data sources shared with the
// integration by title. An empty Query lists all of them.
type NotionSearchQuery struct {
	Query      string
	ObjectType NotionObjectType
	Page       NotionPagination
}

func (x NotionSearchQuery) Validate() error {
	if err := x.ObjectType.Validate(); err != nil {
		return err
	}
	return x.Page.Validate()
}

// NotionDataSourceQuery queries the rows of a data source. Filter and Sorts
// are sent to Notion as they are, in Notion's own JSON format.
type NotionDataSourceQuery struct {
	Filter json.RawMessage
	Sorts  json.RawMessage
	Page   NotionPagination
}

func (x NotionDataSourceQuery) Validate() error {
	if len(x.Filter) > 0 && !json.Valid(x.Filter) {
		return goerr.New("notion filter is not valid JSON")
	}
	if len(x.Sorts) > 0 && !json.Valid(x.Sorts) {
		return goerr.New("notion sorts is not valid JSON")
	}
	return x.Page.Validate()
}

// NotionList is one page of a Notion list response. Results are Notion
// objects as Notion returned them.
type NotionList struct {
	Results    []json.RawMessage
	NextCursor string
	HasMore    bool
}
