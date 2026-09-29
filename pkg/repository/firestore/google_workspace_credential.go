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

type googleWorkspaceCredentialRepository struct {
	client *firestore.Client
}

func (r *googleWorkspaceCredentialRepository) doc(key model.UserKey) *firestore.DocumentRef {
	return userDoc(r.client, key).Collection(credentialsCollection).Doc(googleWorkspaceCredentialDocID)
}

func (r *googleWorkspaceCredentialRepository) Put(ctx context.Context, key model.UserKey, cred *model.GoogleWorkspaceCredential) error {
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
	if _, err := r.doc(key).Set(ctx, cred); err != nil {
		return goerr.Wrap(err, "failed to put google workspace credential",
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
		if err := tx.Delete(ref); err != nil {
			return goerr.Wrap(err, "failed to delete google workspace credential")
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
