package interfaces

import (
	"context"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

// SlackOAuth exchanges an OAuth v2 authorization code.
type SlackOAuth interface {
	ExchangeCode(ctx context.Context, code, redirectURI string) (*model.SlackOAuthResult, error)
}

// SlackBot calls the Slack Web API with the bot token.
type SlackBot interface {
	PostThreadReply(ctx context.Context, channelID, threadTS, text string) error
	PostEphemeral(ctx context.Context, channelID string, userID model.SlackUserID, threadTS, text string) error
	GetUserName(ctx context.Context, userID model.SlackUserID) (string, error)
}

// SlackUserClientFactory builds a client authenticated with one user's token.
type SlackUserClientFactory interface {
	New(token model.SlackUserToken) SlackUserClient
}

// SlackUserClient calls the Slack Web API with one user's token. Methods that
// act on the user's behalf, such as message search, belong here.
type SlackUserClient interface {
	AuthTest(ctx context.Context) (*model.SlackIdentity, error)
}
