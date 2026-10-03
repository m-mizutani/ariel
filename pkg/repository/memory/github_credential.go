package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

type githubCredentialRepository struct {
	mu    sync.Mutex
	creds map[model.UserKey]model.GitHubCredential
	// owners maps a GitHub account to the user it is connected to.
	owners map[model.GitHubUserID]model.UserKey
}

func newGitHubCredentialRepository() *githubCredentialRepository {
	return &githubCredentialRepository{
		creds:  make(map[model.UserKey]model.GitHubCredential),
		owners: make(map[model.GitHubUserID]model.UserKey),
	}
}

func copyGitHubCredential(c model.GitHubCredential) model.GitHubCredential {
	c.AccessToken.Ciphertext = slices.Clone(c.AccessToken.Ciphertext)
	if c.RefreshToken != nil {
		refresh := *c.RefreshToken
		refresh.Ciphertext = slices.Clone(refresh.Ciphertext)
		c.RefreshToken = &refresh
	}
	return c
}

func checkGitHubCredential(key model.UserKey, cred *model.GitHubCredential) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if err := cred.Validate(); err != nil {
		return goerr.Wrap(err, "invalid github credential")
	}
	if cred.Key() != key {
		return goerr.Wrap(interfaces.ErrKeyMismatch, "github credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func (r *githubCredentialRepository) Create(_ context.Context, key model.UserKey, cred *model.GitHubCredential) error {
	if err := checkGitHubCredential(key, cred); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.creds[key]; ok {
		return goerr.Wrap(interfaces.ErrAlreadyExists, "user already has a github credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	if owner, ok := r.owners[cred.GitHubUserID]; ok && owner != key {
		return goerr.Wrap(interfaces.ErrGitHubAccountInUse, "github account is connected to another user",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.V("github_user_id", cred.GitHubUserID))
	}
	r.creds[key] = copyGitHubCredential(*cred)
	r.owners[cred.GitHubUserID] = key
	return nil
}

func (r *githubCredentialRepository) Get(_ context.Context, key model.UserKey) (*model.GitHubCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cred, ok := r.creds[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "github credential not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	out := copyGitHubCredential(cred)
	return &out, nil
}

func (r *githubCredentialRepository) AccountInUse(_ context.Context, key model.UserKey, id model.GitHubUserID) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	if id <= 0 {
		return false, goerr.New("invalid github user ID", goerr.V("github_user_id", id))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	owner, ok := r.owners[id]
	return ok && owner != key, nil
}

func (r *githubCredentialRepository) AcquireRefreshLease(_ context.Context, key model.UserKey, leaseID string, now, expiresAt time.Time) (*model.GitHubCredential, bool, error) {
	if err := key.Validate(); err != nil {
		return nil, false, goerr.Wrap(err, "invalid user key")
	}
	if leaseID == "" || !now.Before(expiresAt) {
		return nil, false, goerr.New("invalid refresh lease", goerr.V("lease_id", leaseID))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cred, ok := r.creds[key]
	if !ok {
		return nil, false, goerr.Wrap(interfaces.ErrNotFound, "github credential not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	acquired := !cred.LeaseHeld(now)
	if acquired {
		cred.RefreshLeaseID = leaseID
		cred.RefreshLeaseExpiresAt = expiresAt
		r.creds[key] = cred
	}
	out := copyGitHubCredential(cred)
	return &out, acquired, nil
}

func (r *githubCredentialRepository) ReplaceIfLeaseHeld(_ context.Context, key model.UserKey, leaseID string, cred *model.GitHubCredential) (bool, error) {
	if err := checkGitHubCredential(key, cred); err != nil {
		return false, err
	}
	if cred.RefreshLeaseID != "" {
		return false, goerr.New("replacement github credential carries a lease")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || current.ConnectionID != cred.ConnectionID || current.RefreshLeaseID != leaseID {
		return false, nil
	}
	r.creds[key] = copyGitHubCredential(*cred)
	return true, nil
}

func (r *githubCredentialRepository) ReleaseRefreshLease(_ context.Context, key model.UserKey, leaseID string) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || current.RefreshLeaseID != leaseID {
		return nil
	}
	current.RefreshLeaseID = ""
	current.RefreshLeaseExpiresAt = time.Time{}
	r.creds[key] = current
	return nil
}

func (r *githubCredentialRepository) DeleteIfConnection(_ context.Context, key model.UserKey, connectionID string) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || current.ConnectionID != connectionID {
		return false, nil
	}
	delete(r.creds, key)
	if r.owners[current.GitHubUserID] == key {
		delete(r.owners, current.GitHubUserID)
	}
	return true, nil
}
