package slack

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// Bot calls the Slack Web API with the bot token.
type Bot struct {
	client *slack.Client
}

var _ interfaces.SlackBot = &Bot{}

func NewBot(botToken string, opts ...slack.Option) *Bot {
	return &Bot{client: slack.New(botToken, opts...)}
}

func (b *Bot) PostThreadReply(ctx context.Context, channelID, threadTS, text string) error {
	if _, _, err := b.client.PostMessageContext(ctx, channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		return wrapError(err, "failed to post thread reply",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	return nil
}

func (b *Bot) PostEphemeral(ctx context.Context, channelID string, userID model.SlackUserID, threadTS, text string) error {
	if _, err := b.client.PostEphemeralContext(ctx, channelID, string(userID),
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		return wrapError(err, "failed to post ephemeral message",
			goerr.V("channel_id", channelID), goerr.V("user_id", userID), goerr.V("thread_ts", threadTS))
	}
	return nil
}

func (b *Bot) GetUserName(ctx context.Context, userID model.SlackUserID) (string, error) {
	user, err := b.client.GetUserInfoContext(ctx, string(userID))
	if err != nil {
		return "", wrapError(err, "failed to get slack user info", goerr.V("user_id", userID))
	}
	if user.RealName != "" {
		return user.RealName, nil
	}
	if user.Name != "" {
		return user.Name, nil
	}
	return "", goerr.New("slack user has no name", goerr.V("user_id", userID))
}
