package slack

import (
	"context"

	"github.com/slack-go/slack"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// UserClientFactory builds Slack clients authenticated with a user token.
type UserClientFactory struct {
	opts []slack.Option
}

var _ interfaces.SlackUserClientFactory = &UserClientFactory{}

func NewUserClientFactory(opts ...slack.Option) *UserClientFactory {
	return &UserClientFactory{opts: opts}
}

func (f *UserClientFactory) New(token model.SlackUserToken) interfaces.SlackUserClient {
	return &userClient{client: slack.New(string(token), f.opts...)}
}

type userClient struct {
	client *slack.Client
}

func (c *userClient) AuthTest(ctx context.Context) (*model.SlackIdentity, error) {
	resp, err := c.client.AuthTestContext(ctx)
	if err != nil {
		return nil, wrapError(err, "slack auth.test failed")
	}
	return &model.SlackIdentity{
		TeamID: model.SlackTeamID(resp.TeamID),
		UserID: model.SlackUserID(resp.UserID),
	}, nil
}
