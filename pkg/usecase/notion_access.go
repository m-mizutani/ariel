package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// NotionAccess is the only component that reads, writes, encrypts, or
// decrypts Notion tokens. Callers must pass a key derived from a verified web
// session or Slack event of that same user; a token is never used on behalf
// of anyone else.
type NotionAccess struct {
	repo    interfaces.Repository
	cipher  interfaces.Cipher
	oauth   interfaces.NotionOAuth
	clients interfaces.NotionClientFactory
	now     func() time.Time
}

func NewNotionAccess(repo interfaces.Repository, cipher interfaces.Cipher,
	oauth interfaces.NotionOAuth, clients interfaces.NotionClientFactory) *NotionAccess {
	return &NotionAccess{repo: repo, cipher: cipher, oauth: oauth, clients: clients, now: time.Now}
}

// notionTokenAAD binds a ciphertext to its owner, so a ciphertext copied into
// another user's document cannot be decrypted. Changing this format makes
// every stored token undecryptable; bump the version and keep decrypting the
// old one instead.
func notionTokenAAD(key model.UserKey) []byte {
	return []byte("ariel:notion-token:v1:" + string(key.TeamID) + ":" + string(key.UserID))
}

type NotionStatus struct {
	Connected bool
	// NeedsReconnect is true when Notion rejected the stored refresh token.
	NeedsReconnect bool
	UserName       string
	WorkspaceName  string
}

// NotionGrant is an authorization result that passed every check and is to be
// stored.
type NotionGrant struct {
	Tokens        model.NotionTokens
	BotID         string
	WorkspaceID   model.NotionWorkspaceID
	WorkspaceName string
	UserID        model.NotionUserID
	UserName      string
}

// NotionToken is a decrypted token pair together with the stored credential
// it came from, so Delete removes that credential and not a newer one saved by
// a later connection.
type NotionToken struct {
	Tokens     model.NotionTokens `masq:"secret"`
	key        model.UserKey
	credential *model.NotionCredential
}

func (a *NotionAccess) encrypt(ctx context.Context, key model.UserKey, tokens model.NotionTokens) (*model.EncryptedData, error) {
	plaintext, err := json.Marshal(tokens)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to encode notion tokens")
	}
	encrypted, err := a.cipher.Encrypt(ctx, plaintext, notionTokenAAD(key))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to encrypt notion tokens",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return encrypted, nil
}

func (a *NotionAccess) decrypt(ctx context.Context, key model.UserKey, data *model.EncryptedData) (model.NotionTokens, error) {
	plaintext, err := a.cipher.Decrypt(ctx, data, notionTokenAAD(key))
	if err != nil {
		return model.NotionTokens{}, goerr.Wrap(err, "failed to decrypt notion tokens",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	var tokens model.NotionTokens
	if err := json.Unmarshal(plaintext, &tokens); err != nil {
		return model.NotionTokens{}, goerr.Wrap(err, "failed to decode notion tokens",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return tokens, nil
}

func (a *NotionAccess) newCredential(ctx context.Context, key model.UserKey, grant *NotionGrant, createdAt, now time.Time) (*model.NotionCredential, error) {
	encrypted, err := a.encrypt(ctx, key, grant.Tokens)
	if err != nil {
		return nil, err
	}
	return &model.NotionCredential{
		TeamID:         key.TeamID,
		UserID:         key.UserID,
		Tokens:         *encrypted,
		WorkspaceID:    grant.WorkspaceID,
		WorkspaceName:  grant.WorkspaceName,
		BotID:          grant.BotID,
		NotionUserID:   grant.UserID,
		NotionUserName: grant.UserName,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}, nil
}

// Store creates the user's credential. It fails with
// interfaces.ErrAlreadyExists when the user already has one and with
// interfaces.ErrNotionAccountInUse when the Notion account is connected to
// another user; nothing is stored in either case.
func (a *NotionAccess) Store(ctx context.Context, key model.UserKey, grant *NotionGrant) error {
	now := a.now()
	cred, err := a.newCredential(ctx, key, grant, now, now)
	if err != nil {
		return err
	}
	if err := a.repo.NotionCredential().Create(ctx, key, cred); err != nil {
		return goerr.Wrap(err, "failed to save notion credential")
	}
	return nil
}

// Replace replaces a credential marked NeedsReconnect with grant, keeping its
// CreatedAt. It fails with ErrNotionAlreadyConnected when the credential is
// missing, not marked, or changed in the meantime, and with
// interfaces.ErrNotionAccountInUse when the Notion account is connected to
// another user.
func (a *NotionAccess) Replace(ctx context.Context, key model.UserKey, grant *NotionGrant) error {
	current, err := a.repo.NotionCredential().Get(ctx, key)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return goerr.Wrap(ErrNotionAlreadyConnected, "notion credential was removed in the meantime",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return goerr.Wrap(err, "failed to load notion credential")
	}
	if !current.NeedsReconnect {
		return goerr.Wrap(ErrNotionAlreadyConnected, "notion credential does not need a reconnection",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	next, err := a.newCredential(ctx, key, grant, current.CreatedAt, a.now())
	if err != nil {
		return err
	}
	replaced, err := a.repo.NotionCredential().UpdateIfUnchanged(ctx, key, current, next)
	if err != nil {
		return goerr.Wrap(err, "failed to replace notion credential")
	}
	if !replaced {
		return goerr.Wrap(ErrNotionAlreadyConnected, "notion credential changed in the meantime",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

// AccountInUse reports whether the Notion account is connected to a user
// other than key.
func (a *NotionAccess) AccountInUse(ctx context.Context, key model.UserKey, notionUserID model.NotionUserID) (bool, error) {
	inUse, err := a.repo.NotionCredential().AccountInUse(ctx, key, notionUserID)
	if err != nil {
		return false, goerr.Wrap(err, "failed to check the owner of the notion account")
	}
	return inUse, nil
}

// Connection reports the stored connection without decrypting the tokens or
// asking Notion.
func (a *NotionAccess) Connection(ctx context.Context, key model.UserKey) (*NotionStatus, error) {
	cred, err := a.repo.NotionCredential().Get(ctx, key)
	switch {
	case err == nil:
		return &NotionStatus{
			Connected:      true,
			NeedsReconnect: cred.NeedsReconnect,
			UserName:       cred.NotionUserName,
			WorkspaceName:  cred.WorkspaceName,
		}, nil
	case errors.Is(err, interfaces.ErrNotFound):
		return &NotionStatus{}, nil
	default:
		return nil, goerr.Wrap(err, "failed to load notion credential")
	}
}

// Token decrypts the user's tokens. It returns ErrNotionNotConnected when none
// are stored.
func (a *NotionAccess) Token(ctx context.Context, key model.UserKey) (*NotionToken, error) {
	cred, err := a.loadCredential(ctx, key)
	if err != nil {
		return nil, err
	}
	tokens, err := a.decrypt(ctx, key, &cred.Tokens)
	if err != nil {
		return nil, err
	}
	return &NotionToken{Tokens: tokens, key: key, credential: cred}, nil
}

// Delete removes the credential token was read from. A credential replaced by
// a newer connection in the meantime is kept.
func (a *NotionAccess) Delete(ctx context.Context, token *NotionToken) error {
	if _, err := a.repo.NotionCredential().DeleteIfUnchanged(ctx, token.key, token.credential); err != nil {
		return goerr.Wrap(err, "failed to delete notion credential")
	}
	return nil
}

func (a *NotionAccess) loadCredential(ctx context.Context, key model.UserKey) (*model.NotionCredential, error) {
	cred, err := a.repo.NotionCredential().Get(ctx, key)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, goerr.Wrap(ErrNotionNotConnected, "no notion credential",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to load notion credential")
	}
	return cred, nil
}

// invalidNotionRequest keeps the validation error, with its values, next to
// ErrNotionInvalidRequest so callers can discriminate it with errors.Is.
func invalidNotionRequest(err error) error {
	return goerr.Wrap(errors.Join(ErrNotionInvalidRequest, err), "invalid notion request")
}

// Search searches the pages and data sources the user shared with Ariel.
func (a *NotionAccess) Search(ctx context.Context, key model.UserKey, q model.NotionSearchQuery) (*model.NotionList, error) {
	if err := q.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (*model.NotionList, error) {
		return c.Search(ctx, q)
	})
}

func (a *NotionAccess) GetPage(ctx context.Context, key model.UserKey, id model.NotionObjectID) (json.RawMessage, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (json.RawMessage, error) {
		return c.GetPage(ctx, id)
	})
}

// ListBlockChildren returns the first level of blocks under a page or block.
func (a *NotionAccess) ListBlockChildren(ctx context.Context, key model.UserKey, id model.NotionObjectID, page model.NotionPagination) (*model.NotionList, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	if err := page.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (*model.NotionList, error) {
		return c.ListBlockChildren(ctx, id, page)
	})
}

// GetDatabase returns a database with the list of its data sources.
func (a *NotionAccess) GetDatabase(ctx context.Context, key model.UserKey, id model.NotionObjectID) (json.RawMessage, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (json.RawMessage, error) {
		return c.GetDatabase(ctx, id)
	})
}

// GetDataSource returns a data source with its property schema.
func (a *NotionAccess) GetDataSource(ctx context.Context, key model.UserKey, id model.NotionObjectID) (json.RawMessage, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (json.RawMessage, error) {
		return c.GetDataSource(ctx, id)
	})
}

// QueryDataSource returns the rows of a data source.
func (a *NotionAccess) QueryDataSource(ctx context.Context, key model.UserKey, id model.NotionObjectID, q model.NotionDataSourceQuery) (*model.NotionList, error) {
	if err := id.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	if err := q.Validate(); err != nil {
		return nil, invalidNotionRequest(err)
	}
	return callNotion(ctx, a, key, func(c interfaces.NotionClient) (*model.NotionList, error) {
		return c.QueryDataSource(ctx, id, q)
	})
}

// callNotion runs fn with the user's access token. When Notion rejects the
// token, it refreshes the token pair and runs fn once more.
func callNotion[T any](ctx context.Context, a *NotionAccess, key model.UserKey, fn func(interfaces.NotionClient) (T, error)) (T, error) {
	var zero T
	vals := []goerr.Option{goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID)}

	cred, err := a.loadCredential(ctx, key)
	if err != nil {
		return zero, err
	}
	if cred.NeedsReconnect {
		return zero, goerr.Wrap(ErrNotionReconnectRequired, "notion connection needs to be reconnected", vals...)
	}
	tokens, err := a.decrypt(ctx, key, &cred.Tokens)
	if err != nil {
		return zero, err
	}

	res, err := fn(a.clients.New(tokens.AccessToken))
	if err == nil {
		return res, nil
	}
	if !errors.Is(err, interfaces.ErrNotionTokenInvalid) {
		return zero, goerr.Wrap(err, "notion request failed", vals...)
	}

	accessToken, err := a.refresh(ctx, key, cred, tokens)
	if err != nil {
		return zero, err
	}
	res, err = fn(a.clients.New(accessToken))
	if err != nil {
		return zero, goerr.Wrap(err, "notion request failed after refreshing the token", vals...)
	}
	return res, nil
}

// refresh obtains an access token to retry with after Notion rejected the one
// in cred. Notion replaces the refresh token on every refresh, and several
// instances may refresh the same credential at once: a new pair is stored only
// while the credential is unchanged, and a rejected refresh token is taken as
// final only when no other instance stored a newer pair in the meantime.
func (a *NotionAccess) refresh(ctx context.Context, key model.UserKey, cred *model.NotionCredential, tokens model.NotionTokens) (model.NotionAccessToken, error) {
	vals := []goerr.Option{goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID)}

	res, err := a.oauth.RefreshToken(ctx, tokens.RefreshToken)
	if err == nil {
		next := res.Tokens
		if next.AccessToken == "" {
			return "", goerr.New("notion returned no access token on refresh", vals...)
		}
		if next.RefreshToken == "" {
			next.RefreshToken = tokens.RefreshToken
		}
		encrypted, err := a.encrypt(ctx, key, next)
		if err != nil {
			return "", err
		}
		updated := *cred
		updated.Tokens = *encrypted
		updated.NeedsReconnect = false
		updated.UpdatedAt = a.now()
		// A false result means another request stored a newer credential; the
		// token obtained here is still valid for this request.
		if _, err := a.repo.NotionCredential().UpdateIfUnchanged(ctx, key, cred, &updated); err != nil {
			return "", goerr.Wrap(err, "failed to save refreshed notion tokens", vals...)
		}
		return next.AccessToken, nil
	}
	if !errors.Is(err, interfaces.ErrNotionTokenInvalid) {
		return "", goerr.Wrap(err, "failed to refresh notion token", vals...)
	}

	if token, err := a.storedAfter(ctx, key, cred); err != nil || token != "" {
		return token, err
	}

	marked := *cred
	marked.NeedsReconnect = true
	marked.UpdatedAt = a.now()
	updated, err := a.repo.NotionCredential().UpdateIfUnchanged(ctx, key, cred, &marked)
	if err != nil {
		return "", goerr.Wrap(err, "failed to mark notion credential for reconnection", vals...)
	}
	if !updated {
		// Another instance stored new tokens between the read and the mark.
		if token, err := a.storedAfter(ctx, key, cred); err != nil || token != "" {
			return token, err
		}
	}
	return "", goerr.Wrap(ErrNotionReconnectRequired, "notion rejected the refresh token", vals...)
}

// storedAfter returns the access token of the credential stored in place of
// cred, or an empty token when cred is still the stored one. It fails with
// ErrNotionNotConnected when the credential was deleted and with
// ErrNotionReconnectRequired when the stored one needs a reconnection.
func (a *NotionAccess) storedAfter(ctx context.Context, key model.UserKey, cred *model.NotionCredential) (model.NotionAccessToken, error) {
	current, err := a.loadCredential(ctx, key)
	if err != nil {
		return "", err
	}
	if bytes.Equal(current.Tokens.Ciphertext, cred.Tokens.Ciphertext) {
		return "", nil
	}
	if current.NeedsReconnect {
		return "", goerr.Wrap(ErrNotionReconnectRequired, "notion connection needs to be reconnected",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	newer, err := a.decrypt(ctx, key, &current.Tokens)
	if err != nil {
		return "", err
	}
	return newer.AccessToken, nil
}
