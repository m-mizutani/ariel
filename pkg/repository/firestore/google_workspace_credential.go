package firestore

import (
	"bytes"
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

type googleWorkspaceCredentialRepository struct {
	client *firestore.Client
}

func (r *googleWorkspaceCredentialRepository) doc(key model.UserKey) *firestore.DocumentRef {
	return userDoc(r.client, key).Collection(credentialsCollection).Doc(googleWorkspaceCredentialDocID)
}

// accountDoc holds the owner of one Google account. It lives outside the
// user's document because it has to be found by the Google account alone.
func (r *googleWorkspaceCredentialRepository) accountDoc(subject string) *firestore.DocumentRef {
	return r.client.Collection(googleWorkspaceAccountsCollection).Doc(subject)
}

// accountOwner reads the owner of subject inside tx. It returns nil when no
// user owns the account.
func (r *googleWorkspaceCredentialRepository) accountOwner(tx *firestore.Transaction, subject string) (*model.UserKey, error) {
	snap, err := tx.Get(r.accountDoc(subject))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get google workspace account")
	}
	var account model.GoogleWorkspaceAccount
	if err := snap.DataTo(&account); err != nil {
		return nil, goerr.Wrap(err, "failed to decode google workspace account")
	}
	owner := account.Key()
	return &owner, nil
}

func (r *googleWorkspaceCredentialRepository) Create(ctx context.Context, key model.UserKey, cred *model.GoogleWorkspaceCredential) error {
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

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(r.doc(key)); err == nil {
			return goerr.Wrap(interfaces.ErrAlreadyExists, "user already has a google workspace credential")
		} else if status.Code(err) != codes.NotFound {
			return goerr.Wrap(err, "failed to get google workspace credential")
		}

		owner, err := r.accountOwner(tx, cred.Subject)
		if err != nil {
			return err
		}
		if owner != nil && *owner != key {
			return goerr.Wrap(interfaces.ErrGoogleAccountInUse, "google account is connected to another user")
		}

		account := &model.GoogleWorkspaceAccount{
			Subject:   cred.Subject,
			TeamID:    key.TeamID,
			UserID:    key.UserID,
			CreatedAt: cred.CreatedAt,
		}
		if err := tx.Set(r.accountDoc(cred.Subject), account); err != nil {
			return goerr.Wrap(err, "failed to put google workspace account")
		}
		if err := tx.Create(r.doc(key), cred); err != nil {
			return goerr.Wrap(err, "failed to create google workspace credential")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "failed to create google workspace credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func (r *googleWorkspaceCredentialRepository) Get(ctx context.Context, key model.UserKey) (*model.GoogleWorkspaceCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	snap, err := r.doc(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "google workspace credential not found",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to get google workspace credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	var cred model.GoogleWorkspaceCredential
	if err := snap.DataTo(&cred); err != nil {
		return nil, goerr.Wrap(err, "failed to decode google workspace credential")
	}
	if cred.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored google workspace credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &cred, nil
}

func (r *googleWorkspaceCredentialRepository) AccountInUse(ctx context.Context, key model.UserKey, subject string) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	if subject == "" {
		return false, goerr.New("empty google account subject")
	}
	snap, err := r.accountDoc(subject).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, goerr.Wrap(err, "failed to get google workspace account",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	var account model.GoogleWorkspaceAccount
	if err := snap.DataTo(&account); err != nil {
		return false, goerr.Wrap(err, "failed to decode google workspace account")
	}
	return account.Key() != key, nil
}

func (r *googleWorkspaceCredentialRepository) DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.GoogleWorkspaceCredential) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}

	var deleted bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		deleted = false
		ref := r.doc(key)
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "failed to get google workspace credential")
		}
		var current model.GoogleWorkspaceCredential
		if err := snap.DataTo(&current); err != nil {
			return goerr.Wrap(err, "failed to decode google workspace credential")
		}
		if !bytes.Equal(current.RefreshToken.Ciphertext, expected.RefreshToken.Ciphertext) {
			return nil
		}
		owner, err := r.accountOwner(tx, current.Subject)
		if err != nil {
			return err
		}

		if err := tx.Delete(ref); err != nil {
			return goerr.Wrap(err, "failed to delete google workspace credential")
		}
		if owner != nil && *owner == key {
			if err := tx.Delete(r.accountDoc(current.Subject)); err != nil {
				return goerr.Wrap(err, "failed to delete google workspace account")
			}
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to delete google workspace credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return deleted, nil
}
