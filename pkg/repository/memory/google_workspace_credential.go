package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

type googleWorkspaceCredentialRepository struct {
	mu    sync.Mutex
	creds map[model.UserKey]model.GoogleWorkspaceCredential
}

func newGoogleWorkspaceCredentialRepository() *googleWorkspaceCredentialRepository {
	return &googleWorkspaceCredentialRepository{creds: make(map[model.UserKey]model.GoogleWorkspaceCredential)}
}

func copyGoogleCredential(c model.GoogleWorkspaceCredential) model.GoogleWorkspaceCredential {
	c.RefreshToken.Ciphertext = slices.Clone(c.RefreshToken.Ciphertext)
	c.Scopes = slices.Clone(c.Scopes)
	return c
}

func (r *googleWorkspaceCredentialRepository) Put(_ context.Context, key model.UserKey, cred *model.GoogleWorkspaceCredential) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if err := cred.Validate(); err != nil {
		return goerr.Wrap(err, "invalid google workspace credential")
	}
	if cred.Key() != key {
		return goerr.Wrap(interfaces.ErrKeyMismatch, "google workspace credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creds[key] = copyGoogleCredential(*cred)
	return nil
}

func (r *googleWorkspaceCredentialRepository) Get(_ context.Context, key model.UserKey) (*model.GoogleWorkspaceCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cred, ok := r.creds[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "google workspace credential not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	out := copyGoogleCredential(cred)
	return &out, nil
}

func (r *googleWorkspaceCredentialRepository) DeleteIfUnchanged(_ context.Context, key model.UserKey, expected *model.GoogleWorkspaceCredential) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || !bytes.Equal(current.RefreshToken.Ciphertext, expected.RefreshToken.Ciphertext) {
		return false, nil
	}
	delete(r.creds, key)
	return true, nil
}
