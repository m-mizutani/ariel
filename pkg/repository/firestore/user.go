package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

type userRepository struct {
	client *firestore.Client
}

func (r *userRepository) Put(ctx context.Context, user *model.User) error {
	if err := user.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user")
	}
	if _, err := userDoc(r.client, user.Key()).Set(ctx, user); err != nil {
		return goerr.Wrap(err, "failed to put user",
			goerr.V("team_id", user.TeamID), goerr.V("user_id", user.UserID))
	}
	return nil
}

func (r *userRepository) Get(ctx context.Context, key model.UserKey) (*model.User, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	snap, err := userDoc(r.client, key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "user not found",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to get user",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	var user model.User
	if err := snap.DataTo(&user); err != nil {
		return nil, goerr.Wrap(err, "failed to decode user")
	}
	if user.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored user belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &user, nil
}
