package usecase

import (
	"context"
	"errors"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const githubCallbackPath = "/api/v1/integrations/github/callback"

type GitHubConfig struct {
	BaseURL string // scheme://host[:port], no trailing slash
}

func (c GitHubConfig) callbackURL() string {
	return c.BaseURL + githubCallbackPath
}

type GitHubStatus struct {
	Connected bool
	Login     string // empty when not connected
}

// GitHubUseCase connects and disconnects a user's GitHub account through the
// GitHub App. It only obtains and keeps the user tokens; reading GitHub with
// them is not implemented yet.
type GitHubUseCase struct {
	oauth   interfaces.GitHubOAuth
	factory interfaces.GitHubUserClientFactory
	access  *GitHubUserAccess
	cfg     GitHubConfig
}

func NewGitHubUseCase(oauth interfaces.GitHubOAuth, factory interfaces.GitHubUserClientFactory,
	access *GitHubUserAccess, cfg GitHubConfig) *GitHubUseCase {
	return &GitHubUseCase{oauth: oauth, factory: factory, access: access, cfg: cfg}
}

// AuthorizeURL returns GitHub's authorization URL for key. A user who already
// has a connected account gets ErrGitHubAlreadyConnected: the account has to
// be disconnected before another connection is made.
func (uc *GitHubUseCase) AuthorizeURL(ctx context.Context, key model.UserKey, state, codeVerifier string) (string, error) {
	if err := uc.ensureNotConnected(ctx, key); err != nil {
		return "", err
	}
	return uc.oauth.AuthorizeURL(uc.cfg.callbackURL(), state, codeVerifier), nil
}

func (uc *GitHubUseCase) ensureNotConnected(ctx context.Context, key model.UserKey) error {
	status, err := uc.access.Connection(ctx, key)
	if err != nil {
		return err
	}
	if status.Connected {
		return goerr.Wrap(ErrGitHubAlreadyConnected, "github is already connected", keyValues(key)...)
	}
	return nil
}

// HandleCallback exchanges code, identifies the GitHub account, and stores
// the tokens for key.
//
// GitHub deletes an authorization per GitHub account, not per token, so a
// rejected authorization is revoked only once its account is known not to
// be connected to anyone: revoking it otherwise would end that connection.
// The account already connected to key is protected by refusing the callback
// before the exchange; an account connected to another user is protected by
// AccountInUse. When the account cannot be identified, nothing is revoked.
func (uc *GitHubUseCase) HandleCallback(ctx context.Context, key model.UserKey, code, codeVerifier string) error {
	if err := uc.ensureNotConnected(ctx, key); err != nil {
		return err
	}

	token, err := uc.oauth.ExchangeCode(ctx, code, uc.cfg.callbackURL(), codeVerifier)
	if err != nil {
		return goerr.Wrap(err, "failed to exchange github oauth code", keyValues(key)...)
	}

	identity, err := uc.factory.New(token.AccessToken).GetUser(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to identify the github account", keyValues(key)...)
	}
	if err := identity.Validate(); err != nil {
		return goerr.Wrap(ErrGitHubConnectRejected, "github account identity is incomplete",
			append(keyValues(key), goerr.V("cause", err.Error()))...)
	}
	vals := append(keyValues(key), goerr.V("github_user_id", identity.ID))

	inUse, err := uc.access.AccountInUse(ctx, key, identity.ID)
	if err != nil {
		return goerr.Wrap(err, "failed to check the github account", vals...)
	}
	if inUse {
		return goerr.Wrap(ErrGitHubAccountInUse, "github account is connected to another user", vals...)
	}

	if err := token.Validate(); err != nil {
		return uc.reject(ctx, key, identity, token, goerr.Wrap(ErrGitHubConnectRejected, "github returned an invalid token",
			append(vals, goerr.V("cause", err.Error()))...))
	}

	// A failed store is not revoked: a storage error does not tell whether the
	// connection was written, and revoking a stored connection would leave it
	// unusable. The unused authorization stays listed in the user's GitHub
	// settings, and no one holds its tokens.
	err = uc.access.Store(ctx, key, token, identity)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, interfaces.ErrAlreadyExists):
		return goerr.Wrap(ErrGitHubAlreadyConnected, "github was connected by another request", vals...)
	case errors.Is(err, interfaces.ErrGitHubAccountInUse):
		return goerr.Wrap(ErrGitHubAccountInUse, "github account was connected to another user by another request", vals...)
	default:
		return goerr.Wrap(err, "failed to store github tokens", vals...)
	}
}

// reject deletes the authorization token came with and returns cause. The
// owner of the GitHub account is checked again right before, so a connection
// another request stored since the first check is not ended. A failed check or
// deletion is recorded here because cause is what the caller needs.
func (uc *GitHubUseCase) reject(ctx context.Context, key model.UserKey, identity *model.GitHubIdentity, token *model.GitHubToken, cause error) error {
	inUse, err := uc.access.AccountInUse(ctx, key, identity.ID)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to check the github account before revoking"), "github authorization may remain")
		return cause
	}
	if inUse {
		return cause
	}
	if err := uc.oauth.RevokeGrant(ctx, token.AccessToken); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to revoke a rejected github authorization"), "github authorization may remain")
	}
	return cause
}

// Status reads only the stored connection. It does not ask GitHub whether
// the token is still valid.
func (uc *GitHubUseCase) Status(ctx context.Context, key model.UserKey) (*GitHubStatus, error) {
	return uc.access.Connection(ctx, key)
}

// Disconnect deletes the authorization at GitHub and then the stored tokens.
func (uc *GitHubUseCase) Disconnect(ctx context.Context, key model.UserKey) error {
	return uc.access.Disconnect(ctx, key)
}
