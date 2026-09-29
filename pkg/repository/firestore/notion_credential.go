package firestore

import (
	"bytes"
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

type notionCredentialRepository struct {
	client *firestore.Client
}

func (r *notionCredentialRepository) doc(key model.UserKey) *firestore.DocumentRef {
	return userDoc(r.client, key).Collection(credentialsCollection).Doc(notionCredentialDocID)
}

// accountDoc holds the owner of one Notion account. It lives outside the
// user's document because it has to be found by the Notion account alone.
func (r *notionCredentialRepository) accountDoc(notionUserID model.NotionUserID) *firestore.DocumentRef {
	return r.client.Collection(notionAccountsCollection).Doc(string(notionUserID))
}

// accountOwner reads the owner of notionUserID inside tx. It returns nil when
// no user owns the account.
func (r *notionCredentialRepository) accountOwner(tx *firestore.Transaction, notionUserID model.NotionUserID) (*model.UserKey, error) {
	snap, err := tx.Get(r.accountDoc(notionUserID))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get notion account")
	}
	var account model.NotionAccount
	if err := snap.DataTo(&account); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion account")
	}
	owner := account.Key()
	return &owner, nil
}

// currentCredential reads the credential of key inside tx. It returns nil when
// none is stored.
func (r *notionCredentialRepository) currentCredential(tx *firestore.Transaction, key model.UserKey) (*model.NotionCredential, error) {
	snap, err := tx.Get(r.doc(key))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get notion credential")
	}
	var current model.NotionCredential
	if err := snap.DataTo(&current); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion credential")
	}
	return &current, nil
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

func (r *notionCredentialRepository) Create(ctx context.Context, key model.UserKey, cred *model.NotionCredential) error {
	if err := validateNotionCredential(key, cred); err != nil {
		return err
	}

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		current, err := r.currentCredential(tx, key)
		if err != nil {
			return err
		}
		if current != nil {
			return goerr.Wrap(interfaces.ErrAlreadyExists, "user already has a notion credential")
		}

		owner, err := r.accountOwner(tx, cred.NotionUserID)
		if err != nil {
			return err
		}
		if owner != nil && *owner != key {
			return goerr.Wrap(interfaces.ErrNotionAccountInUse, "notion account is connected to another user")
		}

		if err := tx.Set(r.accountDoc(cred.NotionUserID), newNotionAccount(key, cred)); err != nil {
			return goerr.Wrap(err, "failed to put notion account")
		}
		if err := tx.Create(r.doc(key), cred); err != nil {
			return goerr.Wrap(err, "failed to create notion credential")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "failed to create notion credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func newNotionAccount(key model.UserKey, cred *model.NotionCredential) *model.NotionAccount {
	return &model.NotionAccount{
		NotionUserID: cred.NotionUserID,
		TeamID:       key.TeamID,
		UserID:       key.UserID,
		CreatedAt:    cred.UpdatedAt,
	}
}

func (r *notionCredentialRepository) Get(ctx context.Context, key model.UserKey) (*model.NotionCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	snap, err := r.doc(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "notion credential not found",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to get notion credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	var cred model.NotionCredential
	if err := snap.DataTo(&cred); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion credential")
	}
	if cred.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored notion credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &cred, nil
}

func (r *notionCredentialRepository) AccountInUse(ctx context.Context, key model.UserKey, notionUserID model.NotionUserID) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	if err := notionUserID.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid notion user ID")
	}
	snap, err := r.accountDoc(notionUserID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, goerr.Wrap(err, "failed to get notion account",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	var account model.NotionAccount
	if err := snap.DataTo(&account); err != nil {
		return false, goerr.Wrap(err, "failed to decode notion account")
	}
	return account.Key() != key, nil
}

func (r *notionCredentialRepository) UpdateIfUnchanged(ctx context.Context, key model.UserKey, expected, next *model.NotionCredential) (bool, error) {
	if err := validateNotionCredential(key, next); err != nil {
		return false, err
	}

	var updated bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		updated = false
		current, err := r.currentCredential(tx, key)
		if err != nil {
			return err
		}
		if current == nil || !bytes.Equal(current.Tokens.Ciphertext, expected.Tokens.Ciphertext) {
			return nil
		}

		if next.NotionUserID != current.NotionUserID {
			// All reads of a transaction come before its writes.
			newOwner, err := r.accountOwner(tx, next.NotionUserID)
			if err != nil {
				return err
			}
			oldOwner, err := r.accountOwner(tx, current.NotionUserID)
			if err != nil {
				return err
			}
			if newOwner != nil && *newOwner != key {
				return goerr.Wrap(interfaces.ErrNotionAccountInUse, "notion account is connected to another user")
			}
			if oldOwner != nil && *oldOwner == key {
				if err := tx.Delete(r.accountDoc(current.NotionUserID)); err != nil {
					return goerr.Wrap(err, "failed to delete notion account")
				}
			}
			if err := tx.Set(r.accountDoc(next.NotionUserID), newNotionAccount(key, next)); err != nil {
				return goerr.Wrap(err, "failed to put notion account")
			}
		}

		if err := tx.Set(r.doc(key), next); err != nil {
			return goerr.Wrap(err, "failed to put notion credential")
		}
		updated = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to update notion credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return updated, nil
}

func (r *notionCredentialRepository) DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.NotionCredential) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}

	var deleted bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		deleted = false
		current, err := r.currentCredential(tx, key)
		if err != nil {
			return err
		}
		if current == nil || !bytes.Equal(current.Tokens.Ciphertext, expected.Tokens.Ciphertext) {
			return nil
		}
		owner, err := r.accountOwner(tx, current.NotionUserID)
		if err != nil {
			return err
		}

		if err := tx.Delete(r.doc(key)); err != nil {
			return goerr.Wrap(err, "failed to delete notion credential")
		}
		if owner != nil && *owner == key {
			if err := tx.Delete(r.accountDoc(current.NotionUserID)); err != nil {
				return goerr.Wrap(err, "failed to delete notion account")
			}
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to delete notion credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return deleted, nil
}
