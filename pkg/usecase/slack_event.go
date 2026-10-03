package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack/slackevents"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

type SlackEventConfig struct {
	TeamID        model.SlackTeamID
	BaseURL       string
	EventClaimTTL time.Duration
}

type SlackEventUseCase struct {
	repo   interfaces.Repository
	bot    interfaces.SlackBot
	access *SlackUserAccess
	cfg    SlackEventConfig
	now    func() time.Time
}

func NewSlackEventUseCase(repo interfaces.Repository, bot interfaces.SlackBot, access *SlackUserAccess, cfg SlackEventConfig) *SlackEventUseCase {
	return &SlackEventUseCase{repo: repo, bot: bot, access: access, cfg: cfg, now: time.Now}
}

func fixedReplyText(userID model.SlackUserID) string {
	return fmt.Sprintf("Hi <@%s>! I received your message. I can only send this fixed reply for now.", userID)
}

func (uc *SlackEventUseCase) loginPromptText() string {
	return fmt.Sprintf("To use this bot, sign in with your Slack account first: %s/login", uc.cfg.BaseURL)
}

// HandleEvent processes one Events API callback. Only app_mention is handled;
// every other event type is ignored.
func (uc *SlackEventUseCase) HandleEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error {
	mention, ok := event.InnerEvent.Data.(*slackevents.AppMentionEvent)
	if !ok {
		return nil
	}
	callback, ok := event.Data.(*slackevents.EventsAPICallbackEvent)
	if !ok {
		return goerr.New("app_mention event has no callback envelope")
	}
	return uc.handleAppMention(ctx, model.SlackTeamID(event.TeamID), callback.EventID, mention)
}

func (uc *SlackEventUseCase) handleAppMention(ctx context.Context, teamID model.SlackTeamID, eventID string, mention *slackevents.AppMentionEvent) error {
	if mention.BotID != "" || teamID != uc.cfg.TeamID || mention.User == "" {
		return nil
	}

	now := uc.now()
	claimed, err := uc.repo.SlackEvent().Claim(ctx, &model.SlackEventClaim{
		EventID:   eventID,
		ClaimedAt: now,
		ExpiresAt: now.Add(uc.cfg.EventClaimTTL),
	})
	if err != nil {
		return goerr.Wrap(err, "failed to claim slack event")
	}
	if !claimed {
		return nil
	}

	threadTS := mention.ThreadTimeStamp
	if threadTS == "" {
		threadTS = mention.TimeStamp
	}
	key := model.UserKey{TeamID: teamID, UserID: model.SlackUserID(mention.User)}
	vals := []goerr.Option{
		goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID),
		goerr.V("channel_id", mention.Channel), goerr.V("event_id", eventID),
	}

	client, err := uc.access.Client(ctx, key)
	if errors.Is(err, ErrSlackNotConnected) {
		return uc.promptLogin(ctx, mention.Channel, key.UserID, threadTS)
	}
	if err != nil {
		return goerr.Wrap(err, "failed to get slack user client", vals...)
	}

	identity, err := client.AuthTest(ctx)
	switch {
	case errors.Is(err, interfaces.ErrSlackTokenInvalid):
		return uc.disconnectAndPrompt(ctx, client, key, mention.Channel, threadTS,
			goerr.Wrap(err, "slack user token is no longer usable", vals...))
	case err != nil:
		return goerr.Wrap(err, "failed to verify slack user token", vals...)
	case identity.TeamID != key.TeamID || identity.UserID != key.UserID:
		return uc.disconnectAndPrompt(ctx, client, key, mention.Channel, threadTS,
			goerr.New("stored slack user token belongs to another user",
				append(vals, goerr.V("auth_test_team_id", identity.TeamID), goerr.V("auth_test_user_id", identity.UserID))...))
	}

	if err := uc.bot.PostThreadReply(ctx, mention.Channel, threadTS, fixedReplyText(key.UserID)); err != nil {
		return goerr.Wrap(err, "failed to post fixed reply", vals...)
	}
	return nil
}

func (uc *SlackEventUseCase) promptLogin(ctx context.Context, channelID string, userID model.SlackUserID, threadTS string) error {
	if err := uc.bot.PostEphemeral(ctx, channelID, userID, threadTS, uc.loginPromptText()); err != nil {
		return goerr.Wrap(err, "failed to post login prompt",
			goerr.V("channel_id", channelID), goerr.V("user_id", userID))
	}
	return nil
}

// disconnectAndPrompt deletes a token Slack no longer accepts, records why,
// and asks the user to sign in again.
func (uc *SlackEventUseCase) disconnectAndPrompt(ctx context.Context, client *UserClient, key model.UserKey, channelID, threadTS string, cause error) error {
	if err := uc.access.Disconnect(ctx, client); err != nil {
		return goerr.Wrap(err, "failed to disconnect unusable slack user token")
	}
	errutil.Handle(ctx, cause, "user token is no longer usable")
	return uc.promptLogin(ctx, channelID, key.UserID, threadTS)
}
