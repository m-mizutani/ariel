package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const (
	// githubRefreshMargin refreshes an access token this long before it
	// expires, so a caller never receives a token that expires mid-request.
	githubRefreshMargin = 5 * time.Minute
	// githubRefreshLeaseTTL bounds how long one instance may hold the right to
	// refresh. githubRefreshWorkTimeout bounds everything the holder does with
	// the lease (decrypting, calling GitHub, encrypting, saving) and is
	// shorter, so the holder has finished or given up before another instance
	// can take the lease and use the same refresh token.
	// githubRefreshCallTimeout bounds the call to GitHub inside that work.
	githubRefreshLeaseTTL    = 60 * time.Second
	githubRefreshWorkTimeout = 45 * time.Second
	githubRefreshCallTimeout = 30 * time.Second
	// A waiting instance polls at githubRefreshPollInterval. It waits longer
	// than githubRefreshLeaseTTL, so it takes over the refresh itself when the
	// holder stopped without releasing the lease.
	githubRefreshPollInterval = 500 * time.Millisecond
	githubRefreshWaitLimit    = 75 * time.Second
)

// GitHubUserAccess is the only component that reads or writes GitHub user
// tokens. Callers must pass a key derived from a verified web session or
// Slack event of that same user; a token is never used on behalf of anyone
// else.
type GitHubUserAccess struct {
	repo    interfaces.Repository
	cipher  interfaces.Cipher
	oauth   interfaces.GitHubOAuth
	factory interfaces.GitHubUserClientFactory
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error
	newID   func() string
}

func NewGitHubUserAccess(repo interfaces.Repository, cipher interfaces.Cipher,
	oauth interfaces.GitHubOAuth, factory interfaces.GitHubUserClientFactory) *GitHubUserAccess {
	return &GitHubUserAccess{
		repo:    repo,
		cipher:  cipher,
		oauth:   oauth,
		factory: factory,
		now:     time.Now,
		sleep:   sleepContext,
		newID:   uuid.NewString,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// The two AADs bind each ciphertext to its owner and to its kind, so a
// ciphertext copied into another user's document, or swapped between the two
// fields, cannot be decrypted. Changing either format makes every stored
// token undecryptable; bump the version and keep decrypting the old one
// instead.
func githubAccessTokenAAD(key model.UserKey) []byte {
	return []byte("robin:github-access-token:v1:" + string(key.TeamID) + ":" + string(key.UserID))
}

func githubRefreshTokenAAD(key model.UserKey) []byte {
	return []byte("robin:github-refresh-token:v1:" + string(key.TeamID) + ":" + string(key.UserID))
}

func keyValues(key model.UserKey) []goerr.Option {
	return []goerr.Option{goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID)}
}

// encryptTokens returns the ciphertexts of token. The refresh token is nil
// when the token does not expire.
func (a *GitHubUserAccess) encryptTokens(ctx context.Context, key model.UserKey, token *model.GitHubToken) (*model.EncryptedData, *model.EncryptedData, error) {
	access, err := a.cipher.Encrypt(ctx, []byte(token.AccessToken), githubAccessTokenAAD(key))
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to encrypt github access token", keyValues(key)...)
	}
	if token.RefreshToken == "" {
		return access, nil, nil
	}
	refresh, err := a.cipher.Encrypt(ctx, []byte(token.RefreshToken), githubRefreshTokenAAD(key))
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to encrypt github refresh token", keyValues(key)...)
	}
	return access, refresh, nil
}

// Store creates the user's connection and the ownership of the GitHub
// account. A stored connection whose refresh token has expired is deleted
// first. It fails with interfaces.ErrAlreadyExists when the user already has
// a usable connection and with interfaces.ErrGitHubAccountInUse when another
// user owns the account; nothing is stored in either case.
func (a *GitHubUserAccess) Store(ctx context.Context, key model.UserKey, token *model.GitHubToken, identity *model.GitHubIdentity) error {
	access, refresh, err := a.encryptTokens(ctx, key, token)
	if err != nil {
		return err
	}
	now := a.now()
	cred := &model.GitHubCredential{
		TeamID:                key.TeamID,
		UserID:                key.UserID,
		ConnectionID:          a.newID(),
		GitHubUserID:          identity.ID,
		GitHubLogin:           identity.Login,
		AccessToken:           *access,
		AccessTokenExpiresAt:  token.AccessTokenExpiresAt,
		RefreshToken:          refresh,
		RefreshTokenExpiresAt: token.RefreshTokenExpiresAt,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	err = a.repo.GitHubCredential().Create(ctx, key, cred)
	if !errors.Is(err, interfaces.ErrAlreadyExists) {
		if err != nil {
			return goerr.Wrap(err, "failed to save github credential")
		}
		return nil
	}

	existing, getErr := a.repo.GitHubCredential().Get(ctx, key)
	if getErr != nil {
		if errors.Is(getErr, interfaces.ErrNotFound) {
			// Deleted in the meantime; the retry below creates it.
			existing = nil
		} else {
			return goerr.Wrap(getErr, "failed to load github credential")
		}
	}
	if existing != nil {
		if !existing.RefreshExpired(now) {
			return goerr.Wrap(err, "failed to save github credential")
		}
		if _, err := a.repo.GitHubCredential().DeleteIfConnection(ctx, key, existing.ConnectionID); err != nil {
			return goerr.Wrap(err, "failed to delete expired github credential")
		}
	}
	if err := a.repo.GitHubCredential().Create(ctx, key, cred); err != nil {
		return goerr.Wrap(err, "failed to save github credential")
	}
	return nil
}

// AccountInUse reports whether the GitHub account is connected to a user
// other than key.
func (a *GitHubUserAccess) AccountInUse(ctx context.Context, key model.UserKey, id model.GitHubUserID) (bool, error) {
	inUse, err := a.repo.GitHubCredential().AccountInUse(ctx, key, id)
	if err != nil {
		return false, goerr.Wrap(err, "failed to check the owner of the github account")
	}
	return inUse, nil
}

// Client returns a REST API client whose access token stays valid for at
// least githubRefreshMargin, refreshing it first when needed. It returns
// ErrGitHubNotConnected when the user has no usable connection.
func (a *GitHubUserAccess) Client(ctx context.Context, key model.UserKey) (interfaces.GitHubUserClient, error) {
	token, _, err := a.validToken(ctx, key)
	if err != nil {
		return nil, err
	}
	return a.factory.New(token), nil
}

// Disconnect deletes the user's authorization of the app at GitHub, then the
// stored connection. When GitHub cannot be reached, the connection is kept so
// that the page never shows GitHub as disconnected while GitHub still honors
// the tokens. A user without a connection gets nil.
func (a *GitHubUserAccess) Disconnect(ctx context.Context, key model.UserKey) error {
	token, cred, err := a.validToken(ctx, key)
	if errors.Is(err, ErrGitHubNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := a.oauth.RevokeGrant(ctx, token); err != nil {
		if !errors.Is(err, interfaces.ErrGitHubTokenInvalid) {
			return goerr.Wrap(err, "failed to delete the github authorization",
				append(keyValues(key), goerr.V("connection_id", cred.ConnectionID))...)
		}
		errutil.Handle(ctx, goerr.Wrap(err, "github token was already invalid",
			append(keyValues(key), goerr.V("connection_id", cred.ConnectionID), goerr.T(errutil.TagBenign))...),
			"deleting an invalid github credential")
	}

	if _, err := a.repo.GitHubCredential().DeleteIfConnection(ctx, key, cred.ConnectionID); err != nil {
		return goerr.Wrap(err, "failed to delete github credential")
	}
	return nil
}

// Connection reads the stored connection without calling GitHub or
// decrypting a token. A connection whose refresh token has expired is
// reported as not connected: it cannot be used any more.
func (a *GitHubUserAccess) Connection(ctx context.Context, key model.UserKey) (*GitHubStatus, error) {
	cred, err := a.repo.GitHubCredential().Get(ctx, key)
	switch {
	case errors.Is(err, interfaces.ErrNotFound):
		return &GitHubStatus{}, nil
	case err != nil:
		return nil, goerr.Wrap(err, "failed to load github credential")
	case cred.RefreshExpired(a.now()):
		return &GitHubStatus{}, nil
	default:
		return &GitHubStatus{Connected: true, Login: cred.GitHubLogin}, nil
	}
}

// validToken returns an access token valid for at least githubRefreshMargin
// and the connection it belongs to. When the token is about to expire, only
// the instance holding the refresh lease refreshes it; the others wait for
// the refreshed token, because a refresh token can be used only once.
func (a *GitHubUserAccess) validToken(ctx context.Context, key model.UserKey) (model.GitHubAccessToken, *model.GitHubCredential, error) {
	deadline := a.now().Add(githubRefreshWaitLimit)
	for {
		cred, err := a.repo.GitHubCredential().Get(ctx, key)
		if err != nil {
			if errors.Is(err, interfaces.ErrNotFound) {
				return "", nil, goerr.Wrap(ErrGitHubNotConnected, "no github credential", keyValues(key)...)
			}
			return "", nil, goerr.Wrap(err, "failed to load github credential")
		}

		now := a.now()
		if cred.RefreshExpired(now) {
			return "", nil, a.forget(ctx, key, cred, "github refresh token has expired")
		}
		if !cred.NeedsRefresh(now, githubRefreshMargin) {
			return a.decryptAccess(ctx, key, cred)
		}

		leaseID := a.newID()
		cred, acquired, err := a.repo.GitHubCredential().AcquireRefreshLease(ctx, key, leaseID, now, now.Add(githubRefreshLeaseTTL))
		if err != nil {
			if errors.Is(err, interfaces.ErrNotFound) {
				return "", nil, goerr.Wrap(ErrGitHubNotConnected, "github credential was deleted", keyValues(key)...)
			}
			return "", nil, goerr.Wrap(err, "failed to acquire github refresh lease")
		}
		if acquired {
			if !cred.NeedsRefresh(now, githubRefreshMargin) {
				// Another instance refreshed between the read and the lease.
				a.releaseLease(ctx, key, leaseID)
				return a.decryptAccess(ctx, key, cred)
			}
			token, next, ok, err := a.refresh(ctx, key, cred, leaseID)
			if err != nil {
				return "", nil, err
			}
			if ok {
				return token, next, nil
			}
			// The lease was lost: the connection was replaced or deleted
			// during the refresh. Read it again.
			continue
		}

		if !a.now().Before(deadline) {
			return "", nil, goerr.Wrap(ErrGitHubRefreshTimeout, "another instance kept the github refresh lease",
				append(keyValues(key), goerr.V("lease_id", cred.RefreshLeaseID))...)
		}
		if err := a.sleep(ctx, githubRefreshPollInterval); err != nil {
			return "", nil, goerr.Wrap(err, "stopped waiting for the github token refresh")
		}
	}
}

// refresh exchanges the refresh token while holding leaseID. It returns
// ok=false when the lease was lost before the new tokens could be saved.
// Releasing the lease and forgetting the connection use ctx, not the bounded
// work context, so they still run after the work ran out of time.
func (a *GitHubUserAccess) refresh(ctx context.Context, key model.UserKey, cred *model.GitHubCredential, leaseID string) (model.GitHubAccessToken, *model.GitHubCredential, bool, error) {
	workCtx, cancel := context.WithTimeout(ctx, githubRefreshWorkTimeout)
	defer cancel()
	return a.refreshWithin(workCtx, ctx, key, cred, leaseID)
}

func (a *GitHubUserAccess) refreshWithin(workCtx, ctx context.Context, key model.UserKey, cred *model.GitHubCredential, leaseID string) (model.GitHubAccessToken, *model.GitHubCredential, bool, error) {
	vals := append(keyValues(key), goerr.V("connection_id", cred.ConnectionID))

	plaintext, err := a.cipher.Decrypt(workCtx, cred.RefreshToken, githubRefreshTokenAAD(key))
	if err != nil {
		a.releaseLease(ctx, key, leaseID)
		return "", nil, false, goerr.Wrap(err, "failed to decrypt github refresh token", vals...)
	}

	callCtx, cancel := context.WithTimeout(workCtx, githubRefreshCallTimeout)
	token, err := a.oauth.Refresh(callCtx, model.GitHubRefreshToken(plaintext))
	cancel()
	if err != nil {
		if errors.Is(err, interfaces.ErrGitHubTokenInvalid) {
			return "", nil, false, a.forget(ctx, key, cred, "github rejected the refresh token")
		}
		a.releaseLease(ctx, key, leaseID)
		return "", nil, false, goerr.Wrap(err, "failed to refresh github user token", vals...)
	}
	if err := token.Validate(); err != nil {
		a.releaseLease(ctx, key, leaseID)
		return "", nil, false, goerr.Wrap(err, "github returned an invalid token on refresh", vals...)
	}

	access, refresh, err := a.encryptTokens(workCtx, key, token)
	if err != nil {
		a.releaseLease(ctx, key, leaseID)
		return "", nil, false, err
	}
	next := *cred
	next.AccessToken = *access
	next.AccessTokenExpiresAt = token.AccessTokenExpiresAt
	next.RefreshToken = refresh
	next.RefreshTokenExpiresAt = token.RefreshTokenExpiresAt
	next.RefreshLeaseID = ""
	next.RefreshLeaseExpiresAt = time.Time{}
	next.UpdatedAt = a.now()

	replaced, err := a.repo.GitHubCredential().ReplaceIfLeaseHeld(workCtx, key, leaseID, &next)
	if err != nil {
		return "", nil, false, goerr.Wrap(err, "failed to save refreshed github tokens", vals...)
	}
	if !replaced {
		return "", nil, false, nil
	}
	return token.AccessToken, &next, true, nil
}

// forget deletes a connection that can no longer be used and reports the
// user as not connected.
func (a *GitHubUserAccess) forget(ctx context.Context, key model.UserKey, cred *model.GitHubCredential, reason string) error {
	vals := append(keyValues(key), goerr.V("connection_id", cred.ConnectionID))
	if _, err := a.repo.GitHubCredential().DeleteIfConnection(ctx, key, cred.ConnectionID); err != nil {
		return goerr.Wrap(err, "failed to delete unusable github credential", vals...)
	}
	return goerr.Wrap(ErrGitHubNotConnected, reason, vals...)
}

// releaseLease records a failed release instead of returning it: the lease
// expires by itself after githubRefreshLeaseTTL.
func (a *GitHubUserAccess) releaseLease(ctx context.Context, key model.UserKey, leaseID string) {
	if err := a.repo.GitHubCredential().ReleaseRefreshLease(ctx, key, leaseID); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to release github refresh lease",
			append(keyValues(key), goerr.V("lease_id", leaseID))...), "github refresh lease expires by itself")
	}
}

func (a *GitHubUserAccess) decryptAccess(ctx context.Context, key model.UserKey, cred *model.GitHubCredential) (model.GitHubAccessToken, *model.GitHubCredential, error) {
	plaintext, err := a.cipher.Decrypt(ctx, &cred.AccessToken, githubAccessTokenAAD(key))
	if err != nil {
		return "", nil, goerr.Wrap(err, "failed to decrypt github access token",
			append(keyValues(key), goerr.V("connection_id", cred.ConnectionID))...)
	}
	return model.GitHubAccessToken(plaintext), cred, nil
}
