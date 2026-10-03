package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

type notionCredentialRepository struct {
	mu    sync.Mutex
	creds map[model.UserKey]model.NotionCredential
	// owners maps a Notion account to the user it is connected to.
	owners map[model.NotionUserID]model.UserKey
}

func newNotionCredentialRepository() *notionCredentialRepository {
	return &notionCredentialRepository{
		creds:  make(map[model.UserKey]model.NotionCredential),
		owners: make(map[model.NotionUserID]model.UserKey),
	}
}

func copyNotionCredential(c model.NotionCredential) model.NotionCredential {
	c.Tokens.Ciphertext = slices.Clone(c.Tokens.Ciphertext)
	return c
}

func validateNotionCredential(key model.UserKey, cred *model.NotionCredential) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if err := cred.Validate(); err != nil {
		return goerr.Wrap(err, "invalid notion credential")
	}
	if cred.Key() != key {
		return goerr.Wrap(interfaces.ErrKeyMismatch, "notion credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func (r *notionCredentialRepository) Create(_ context.Context, key model.UserKey, cred *model.NotionCredential) error {
	if err := validateNotionCredential(key, cred); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.creds[key]; ok {
		return goerr.Wrap(interfaces.ErrAlreadyExists, "user already has a notion credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	if owner, ok := r.owners[cred.NotionUserID]; ok && owner != key {
		return goerr.Wrap(interfaces.ErrNotionAccountInUse, "notion account is connected to another user",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	r.creds[key] = copyNotionCredential(*cred)
	r.owners[cred.NotionUserID] = key
	return nil
}

func (r *notionCredentialRepository) Get(_ context.Context, key model.UserKey) (*model.NotionCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cred, ok := r.creds[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "notion credential not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	out := copyNotionCredential(cred)
	return &out, nil
}

func (r *notionCredentialRepository) AccountInUse(_ context.Context, key model.UserKey, notionUserID model.NotionUserID) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	if err := notionUserID.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid notion user ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	owner, ok := r.owners[notionUserID]
	return ok && owner != key, nil
}

func (r *notionCredentialRepository) UpdateIfUnchanged(_ context.Context, key model.UserKey, expected, next *model.NotionCredential) (bool, error) {
	if err := validateNotionCredential(key, next); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || !bytes.Equal(current.Tokens.Ciphertext, expected.Tokens.Ciphertext) {
		return false, nil
	}
	if next.NotionUserID != current.NotionUserID {
		if owner, ok := r.owners[next.NotionUserID]; ok && owner != key {
			return false, goerr.Wrap(interfaces.ErrNotionAccountInUse, "notion account is connected to another user",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		if r.owners[current.NotionUserID] == key {
			delete(r.owners, current.NotionUserID)
		}
		r.owners[next.NotionUserID] = key
	}
	r.creds[key] = copyNotionCredential(*next)
	return true, nil
}

func (r *notionCredentialRepository) DeleteIfUnchanged(_ context.Context, key model.UserKey, expected *model.NotionCredential) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.creds[key]
	if !ok || !bytes.Equal(current.Tokens.Ciphertext, expected.Tokens.Ciphertext) {
		return false, nil
	}
	delete(r.creds, key)
	if r.owners[current.NotionUserID] == key {
		delete(r.owners, current.NotionUserID)
	}
	return true, nil
}
