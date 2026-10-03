package memory

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

type slackEventRepository struct {
	mu     sync.Mutex
	claims map[string]model.SlackEventClaim
}

func newSlackEventRepository() *slackEventRepository {
	return &slackEventRepository{claims: make(map[string]model.SlackEventClaim)}
}

func (r *slackEventRepository) Claim(_ context.Context, claim *model.SlackEventClaim) (bool, error) {
	if err := claim.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid slack event claim")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.claims[claim.EventID]; ok {
		return false, nil
	}
	r.claims[claim.EventID] = *claim
	return true, nil
}
