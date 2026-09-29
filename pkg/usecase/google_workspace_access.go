package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// GoogleWorkspaceAccess is the only component that reads or writes Google
// refresh tokens. Callers must pass a key derived from a verified web session
// of that same user; a token is never used on behalf of anyone else.
type GoogleWorkspaceAccess struct {
	repo   interfaces.Repository
	cipher interfaces.Cipher
}

func NewGoogleWorkspaceAccess(repo interfaces.Repository, cipher interfaces.Cipher) *GoogleWorkspaceAccess {
	return &GoogleWorkspaceAccess{repo: repo, cipher: cipher}
}

// googleTokenAAD binds a ciphertext to its owner, so a ciphertext copied into
// another user's document cannot be decrypted. Changing this format makes
// every stored token undecryptable; bump the version and keep decrypting the
// old one instead.
func googleTokenAAD(key model.UserKey) []byte {
	return []byte("ariel:google-refresh-token:v1:" + string(key.TeamID) + ":" + string(key.UserID))
}

// GoogleWorkspaceToken is a decrypted refresh token together with the stored
// credential it came from, so Delete removes that credential and not a newer
// one saved by a later connection.
type GoogleWorkspaceToken struct {
	RefreshToken model.GoogleRefreshToken `masq:"secret"`
	key          model.UserKey
	credential   *model.GoogleWorkspaceCredential
}

// Store encrypts token and saves it with the granted scopes and the account it
// belongs to, keeping the original CreatedAt when a credential already exists.
func (a *GoogleWorkspaceAccess) Store(ctx context.Context, key model.UserKey, token model.GoogleRefreshToken,
	scopes []string, identity *model.GoogleIdentity, now time.Time) error {
	createdAt := now
	existing, err := a.repo.GoogleWorkspaceCredential().Get(ctx, key)
	switch {
	case err == nil:
		createdAt = existing.CreatedAt
	case !errors.Is(err, interfaces.ErrNotFound):
		return goerr.Wrap(err, "failed to load existing google workspace credential")
	}

	encrypted, err := a.cipher.Encrypt(ctx, []byte(token), googleTokenAAD(key))
	if err != nil {
		return goerr.Wrap(err, "failed to encrypt google refresh token",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	cred := &model.GoogleWorkspaceCredential{
		TeamID:       key.TeamID,
		UserID:       key.UserID,
		RefreshToken: *encrypted,
		Scopes:       scopes,
		Subject:      identity.Subject,
		Email:        identity.Email,
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	}
	if err := a.repo.GoogleWorkspaceCredential().Put(ctx, key, cred); err != nil {
		return goerr.Wrap(err, "failed to save google workspace credential")
	}
	return nil
}

// Token decrypts the user's refresh token. It returns
// ErrGoogleWorkspaceNotConnected when none is stored.
func (a *GoogleWorkspaceAccess) Token(ctx context.Context, key model.UserKey) (*GoogleWorkspaceToken, error) {
	cred, err := a.repo.GoogleWorkspaceCredential().Get(ctx, key)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, goerr.Wrap(ErrGoogleWorkspaceNotConnected, "no google workspace credential",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to load google workspace credential")
	}

	plaintext, err := a.cipher.Decrypt(ctx, &cred.RefreshToken, googleTokenAAD(key))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to decrypt google refresh token",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &GoogleWorkspaceToken{
		RefreshToken: model.GoogleRefreshToken(plaintext),
		key:          key,
		credential:   cred,
	}, nil
}

// Delete removes the credential token was read from. A credential replaced by
// a newer connection in the meantime is kept.
func (a *GoogleWorkspaceAccess) Delete(ctx context.Context, token *GoogleWorkspaceToken) error {
	if _, err := a.repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, token.key, token.credential); err != nil {
		return goerr.Wrap(err, "failed to delete google workspace credential")
	}
	return nil
}

// Connection reports whether a credential is stored and which account it is
// for, without decrypting the token.
func (a *GoogleWorkspaceAccess) Connection(ctx context.Context, key model.UserKey) (*GoogleWorkspaceStatus, error) {
	cred, err := a.repo.GoogleWorkspaceCredential().Get(ctx, key)
	switch {
	case err == nil:
		return &GoogleWorkspaceStatus{Connected: true, Email: cred.Email}, nil
	case errors.Is(err, interfaces.ErrNotFound):
		return &GoogleWorkspaceStatus{}, nil
	default:
		return nil, goerr.Wrap(err, "failed to load google workspace credential")
	}
}
