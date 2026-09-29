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

type slackCredentialRepository struct {
	mu    sync.Mutex
	creds map[model.UserKey]model.SlackCredential
}

func newSlackCredentialRepository() *slackCredentialRepository {
	return &slackCredentialRepository{creds: make(map[model.UserKey]model.SlackCredential)}
}

func copyCredential(c model.SlackCredential) model.SlackCredential {
	c.AccessToken.Ciphertext = slices.Clone(c.AccessToken.Ciphertext)
	c.Scopes = slices.Clone(c.Scopes)
	return c
}

func (r *slackCredentialRepository) Put(_ context.Context, key model.UserKey, cred *model.SlackCredential) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if err := cred.Validate(); err != nil {
		return goerr.Wrap(err, "invalid slack credential")
	}
	if cred.Key() != key {
		return goerr.Wrap(interfaces.ErrKeyMismatch, "slack credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creds[key] = copyCredential(*cred)
	return nil
}

func (r *slackCredentialRepository) Get(_ context.Context, key model.UserKey) (*model.SlackCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cred, ok := r.creds[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "slack credential not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	out := copyCredential(cred)
	return &out, nil
}

func (r *slackCredentialRepository) DeleteIfUnchanged(_ context.Context, key model.UserKey, expected *model.SlackCredential) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || !bytes.Equal(current.AccessToken.Ciphertext, expected.AccessToken.Ciphertext) {
		return false, nil
	}
	delete(r.creds, key)
	return true, nil
}
