package memory

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

type userRepository struct {
	mu    sync.Mutex
	users map[model.UserKey]model.User
}

func newUserRepository() *userRepository {
	return &userRepository{users: make(map[model.UserKey]model.User)}
}

func (r *userRepository) Put(_ context.Context, user *model.User) error {
	if err := user.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[user.Key()] = *user
	return nil
}

func (r *userRepository) Get(_ context.Context, key model.UserKey) (*model.User, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "user not found",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &user, nil
}
