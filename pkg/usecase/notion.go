package usecase

import (
	"context"
	"errors"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const notionCallbackPath = "/api/v1/integrations/notion/callback"

type NotionConfig struct {
	BaseURL string // scheme://host[:port], no trailing slash
	// WorkspaceID is the only Notion workspace a connection is accepted for.
	WorkspaceID model.NotionWorkspaceID
}

func (c NotionConfig) callbackURL() string {
	return c.BaseURL + notionCallbackPath
}

// NotionUseCase connects and disconnects a user's Notion account. Reading
// Notion with the stored tokens is done by NotionAccess.
type NotionUseCase struct {
	oauth  interfaces.NotionOAuth
	access *NotionAccess
	cfg    NotionConfig
}

func NewNotionUseCase(oauth interfaces.NotionOAuth, access *NotionAccess, cfg NotionConfig) *NotionUseCase {
	return &NotionUseCase{oauth: oauth, access: access, cfg: cfg}
}

// connectable returns the user's current connection. A user with a working
// connection gets ErrNotionAlreadyConnected: the connection has to be
// disconnected first. A connection that needs a reconnection can be replaced.
func (uc *NotionUseCase) connectable(ctx context.Context, key model.UserKey) (*NotionStatus, error) {
	status, err := uc.access.Connection(ctx, key)
	if err != nil {
		return nil, err
	}
	if status.Connected && !status.NeedsReconnect {
		return nil, goerr.Wrap(ErrNotionAlreadyConnected, "notion is already connected",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return status, nil
}

// AuthorizeURL returns Notion's authorization URL for key.
func (uc *NotionUseCase) AuthorizeURL(ctx context.Context, key model.UserKey, state string) (string, error) {
	if _, err := uc.connectable(ctx, key); err != nil {
		return "", err
	}
	return uc.oauth.AuthorizeURL(state, uc.cfg.callbackURL()), nil
}

// HandleCallback exchanges code, checks the authorization, and stores the
// tokens for key.
//
// Whether Notion revokes one connection per token or per Notion user is not
// documented, so a rejected authorization is revoked only once its Notion
// account is known not to be connected to another user; revoking it
// otherwise could end that user's connection. When the account cannot be
// identified, nothing is revoked.
func (uc *NotionUseCase) HandleCallback(ctx context.Context, key model.UserKey, code string) error {
	status, err := uc.connectable(ctx, key)
	if err != nil {
		return err
	}

	res, err := uc.oauth.ExchangeCode(ctx, code, uc.cfg.callbackURL())
	if err != nil {
		return goerr.Wrap(err, "failed to exchange notion oauth code",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	vals := []goerr.Option{
		goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID),
		goerr.V("workspace_id", res.WorkspaceID), goerr.V("bot_id", res.BotID),
	}
	if res.OwnerType != "user" || res.OwnerUserID.Validate() != nil {
		return goerr.Wrap(ErrNotionConnectRejected, "notion authorization is not by a user",
			append(vals, goerr.V("owner_type", res.OwnerType))...)
	}
	vals = append(vals, goerr.V("notion_user_id", res.OwnerUserID))

	inUse, err := uc.access.AccountInUse(ctx, key, res.OwnerUserID)
	if err != nil {
		return goerr.Wrap(err, "failed to check the notion account", vals...)
	}
	if inUse {
		return goerr.Wrap(ErrNotionAccountInUse, "notion account is connected to another user", vals...)
	}

	if res.WorkspaceID != uc.cfg.WorkspaceID {
		return uc.reject(ctx, res, goerr.Wrap(ErrNotionWrongWorkspace, "notion authorization is for another workspace",
			append(vals, goerr.V("workspace_name", res.WorkspaceName), goerr.V("expected_workspace_id", uc.cfg.WorkspaceID))...))
	}
	if res.Tokens.AccessToken == "" || res.Tokens.RefreshToken == "" || res.BotID == "" {
		return uc.reject(ctx, res, goerr.Wrap(ErrNotionConnectRejected, "notion authorization is incomplete",
			append(vals,
				goerr.V("has_access_token", res.Tokens.AccessToken != ""),
				goerr.V("has_refresh_token", res.Tokens.RefreshToken != ""))...))
	}

	grant := &NotionGrant{
		Tokens:        res.Tokens,
		BotID:         res.BotID,
		WorkspaceID:   res.WorkspaceID,
		WorkspaceName: res.WorkspaceName,
		UserID:        res.OwnerUserID,
		UserName:      res.OwnerName,
	}
	if status.Connected {
		err = uc.access.Replace(ctx, key, grant)
	} else {
		err = uc.access.Store(ctx, key, grant)
	}
	switch {
	case err == nil:
		return nil
	// Another request connected Notion in the meantime; the authorization may
	// belong to that connection, so it is not revoked.
	case errors.Is(err, interfaces.ErrAlreadyExists), errors.Is(err, ErrNotionAlreadyConnected):
		return goerr.Wrap(ErrNotionAlreadyConnected, "notion was connected by another request", vals...)
	case errors.Is(err, interfaces.ErrNotionAccountInUse):
		return goerr.Wrap(ErrNotionAccountInUse, "notion account was connected to another user by another request", vals...)
	default:
		return uc.reject(ctx, res, goerr.Wrap(err, "failed to store notion tokens", vals...))
	}
}

// reject revokes the access token res came with and returns cause. A failed
// revocation is recorded here because cause is what the caller needs.
// Callers must have made sure that the Notion account is not connected to
// another user.
func (uc *NotionUseCase) reject(ctx context.Context, res *model.NotionOAuthResult, cause error) error {
	if res.Tokens.AccessToken == "" {
		return cause
	}
	if err := uc.oauth.Revoke(ctx, res.Tokens.AccessToken); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to revoke a rejected notion authorization",
			goerr.V("bot_id", res.BotID)), "notion authorization may remain")
	}
	return cause
}

// Status reads only the stored credential. It does not ask Notion whether the
// token is still valid.
func (uc *NotionUseCase) Status(ctx context.Context, key model.UserKey) (*NotionStatus, error) {
	return uc.access.Connection(ctx, key)
}

// Disconnect revokes the stored access token at Notion and then deletes the
// credential. When Notion cannot be reached, the credential is kept so that
// the page never shows Notion as disconnected while Notion still honors the
// token.
func (uc *NotionUseCase) Disconnect(ctx context.Context, key model.UserKey) error {
	token, err := uc.access.Token(ctx, key)
	if errors.Is(err, ErrNotionNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := uc.oauth.Revoke(ctx, token.Tokens.AccessToken); err != nil {
		if !errors.Is(err, interfaces.ErrNotionTokenInvalid) {
			return goerr.Wrap(err, "failed to revoke notion token",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		errutil.Handle(ctx, goerr.Wrap(err, "notion token was already invalid",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.T(errutil.TagBenign)),
			"deleting an invalid notion token")
	}

	return uc.access.Delete(ctx, token)
}
