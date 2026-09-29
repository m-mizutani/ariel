package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// SlackUserAccess is the only component that reads or writes Slack user
// tokens. Callers must pass a key derived from a verified Slack event or a
// verified web session of that same user; a token is never used on behalf of
// anyone else.
type SlackUserAccess struct {
	repo    interfaces.Repository
	cipher  interfaces.Cipher
	factory interfaces.SlackUserClientFactory
}

func NewSlackUserAccess(repo interfaces.Repository, cipher interfaces.Cipher, factory interfaces.SlackUserClientFactory) *SlackUserAccess {
	return &SlackUserAccess{repo: repo, cipher: cipher, factory: factory}
}

// tokenAAD binds a ciphertext to its owner, so a ciphertext copied into
// another user's document cannot be decrypted. Changing this format makes
// every stored token undecryptable; bump the version and keep decrypting the
// old one instead.
func tokenAAD(key model.UserKey) []byte {
	return []byte("ariel:slack-user-token:v1:" + string(key.TeamID) + ":" + string(key.UserID))
}

// Store encrypts token and saves it with the granted scopes, keeping the
// original CreatedAt when a credential already exists.
func (a *SlackUserAccess) Store(ctx context.Context, key model.UserKey, token model.SlackUserToken, scopes []string, now time.Time) error {
	createdAt := now
	existing, err := a.repo.SlackCredential().Get(ctx, key)
	switch {
	case err == nil:
		createdAt = existing.CreatedAt
	case !errors.Is(err, interfaces.ErrNotFound):
		return goerr.Wrap(err, "failed to load existing slack credential")
	}

	encrypted, err := a.cipher.Encrypt(ctx, []byte(token), tokenAAD(key))
	if err != nil {
		return goerr.Wrap(err, "failed to encrypt slack user token",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	cred := &model.SlackCredential{
		TeamID:      key.TeamID,
		UserID:      key.UserID,
		AccessToken: *encrypted,
		Scopes:      scopes,
		CreatedAt:   createdAt,
		UpdatedAt:   now,
	}
	if err := a.repo.SlackCredential().Put(ctx, key, cred); err != nil {
		return goerr.Wrap(err, "failed to save slack credential")
	}
	return nil
}

// Client returns a Slack client authenticated with the user's own token. It
// returns ErrSlackNotConnected when no token is stored.
// UserClient is a Slack client authenticated with one user's stored token. It
// remembers which stored credential it was built from, so Disconnect removes
// that credential and not a newer one saved by a later sign-in.
type UserClient struct {
	interfaces.SlackUserClient
	key        model.UserKey
	credential *model.SlackCredential
}

func (a *SlackUserAccess) Client(ctx context.Context, key model.UserKey) (*UserClient, error) {
	cred, err := a.repo.SlackCredential().Get(ctx, key)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, goerr.Wrap(ErrSlackNotConnected, "no slack credential",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to load slack credential")
	}

	plaintext, err := a.cipher.Decrypt(ctx, &cred.AccessToken, tokenAAD(key))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to decrypt slack user token",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &UserClient{
		SlackUserClient: a.factory.New(model.SlackUserToken(plaintext)),
		key:             key,
		credential:      cred,
	}, nil
}

// Disconnect deletes the credential client was built from, after Slack
// reported its token as unusable. A credential replaced by a newer sign-in in
// the meantime is kept.
func (a *SlackUserAccess) Disconnect(ctx context.Context, client *UserClient) error {
	if _, err := a.repo.SlackCredential().DeleteIfUnchanged(ctx, client.key, client.credential); err != nil {
		return goerr.Wrap(err, "failed to delete slack credential")
	}
	return nil
}

// Connected reports whether a token is stored for the user.
func (a *SlackUserAccess) Connected(ctx context.Context, key model.UserKey) (bool, error) {
	_, err := a.repo.SlackCredential().Get(ctx, key)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, interfaces.ErrNotFound):
		return false, nil
	default:
		return false, goerr.Wrap(err, "failed to load slack credential")
	}
}
