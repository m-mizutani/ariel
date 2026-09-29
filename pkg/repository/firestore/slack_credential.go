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

type slackCredentialRepository struct {
	client *firestore.Client
}

func (r *slackCredentialRepository) doc(key model.UserKey) *firestore.DocumentRef {
	return userDoc(r.client, key).Collection(credentialsCollection).Doc(slackCredentialDocID)
}

func (r *slackCredentialRepository) Put(ctx context.Context, key model.UserKey, cred *model.SlackCredential) error {
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
	if _, err := r.doc(key).Set(ctx, cred); err != nil {
		return goerr.Wrap(err, "failed to put slack credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func (r *slackCredentialRepository) Get(ctx context.Context, key model.UserKey) (*model.SlackCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	snap, err := r.doc(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "slack credential not found",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to get slack credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	var cred model.SlackCredential
	if err := snap.DataTo(&cred); err != nil {
		return nil, goerr.Wrap(err, "failed to decode slack credential")
	}
	if cred.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored slack credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &cred, nil
}

func (r *slackCredentialRepository) DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.SlackCredential) (bool, error) {
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
			return goerr.Wrap(err, "failed to get slack credential")
		}
		var current model.SlackCredential
		if err := snap.DataTo(&current); err != nil {
			return goerr.Wrap(err, "failed to decode slack credential")
		}
		if !bytes.Equal(current.AccessToken.Ciphertext, expected.AccessToken.Ciphertext) {
			return nil
		}
		if err := tx.Delete(ref); err != nil {
			return goerr.Wrap(err, "failed to delete slack credential")
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to delete slack credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return deleted, nil
}
