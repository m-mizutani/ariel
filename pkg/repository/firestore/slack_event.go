package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

type slackEventRepository struct {
	client *firestore.Client
}

func (r *slackEventRepository) Claim(ctx context.Context, claim *model.SlackEventClaim) (bool, error) {
	if err := claim.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid slack event claim")
	}
	_, err := r.client.Collection(slackEventsCollection).Doc(claim.EventID).Create(ctx, claim)
	if err == nil {
		return true, nil
	}
	if status.Code(err) == codes.AlreadyExists {
		return false, nil
	}
	return false, goerr.Wrap(err, "failed to claim slack event", goerr.V("event_id", claim.EventID))
}
