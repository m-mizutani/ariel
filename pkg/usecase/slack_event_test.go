package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/slack-go/slack/slackevents"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/usecase"
)

const (
	testChannel   = "C0123ABCD"
	testMessageTS = "1700000000.000100"
	testThreadTS  = "1699999999.000001"
)

type eventFixture struct {
	*authFixture
	events *usecase.SlackEventUseCase
}

func newEventFixture(t *testing.T) *eventFixture {
	t.Helper()
	f := &eventFixture{authFixture: newAuthFixture(t)}
	f.events = usecase.NewSlackEventUseCase(f.repo, f.bot, f.access, usecase.SlackEventConfig{
		TeamID:        testKey.TeamID,
		BaseURL:       testBaseURL,
		EventClaimTTL: 24 * time.Hour,
	})
	f.events.SetNowForTest(func() time.Time { return f.now })
	return f
}

func (f *eventFixture) connect(t *testing.T) {
	t.Helper()
	gt.NoError(t, f.access.Store(context.Background(), testKey, testUserToken, []string{"search:read"}, f.now)).Required()
}

func mentionEvent(eventID string, mention *slackevents.AppMentionEvent) *slackevents.EventsAPIEvent {
	return &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: string(testKey.TeamID),
		Data:   &slackevents.EventsAPICallbackEvent{EventID: eventID, TeamID: string(testKey.TeamID)},
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: string(slackevents.AppMention),
			Data: mention,
		},
	}
}

func topLevelMention() *slackevents.AppMentionEvent {
	return &slackevents.AppMentionEvent{
		Type:      string(slackevents.AppMention),
		User:      string(testKey.UserID),
		Text:      "<@UBOT> hello",
		TimeStamp: testMessageTS,
		Channel:   testChannel,
	}
}

func TestSlackEventUseCase_ConnectedUserTopLevel(t *testing.T) {
	f := newEventFixture(t)
	f.connect(t)

	gt.NoError(t, f.events.HandleEvent(context.Background(), mentionEvent("Ev001", topLevelMention()))).Required()

	gt.Number(t, f.factory.authTestCount()).Equal(1)
	replies := f.bot.replies()
	gt.Array(t, replies).Length(1).Required()
	gt.String(t, replies[0].ChannelID).Equal(testChannel)
	gt.String(t, replies[0].ThreadTS).Equal(testMessageTS)
	gt.String(t, replies[0].Text).Contains("<@U0123ABCD>")
	gt.String(t, replies[0].Text).Contains("fixed reply")
	gt.Array(t, f.bot.ephemeralMessages()).Length(0)
}

func TestSlackEventUseCase_ConnectedUserInThread(t *testing.T) {
	f := newEventFixture(t)
	f.connect(t)
	mention := topLevelMention()
	mention.ThreadTimeStamp = testThreadTS

	gt.NoError(t, f.events.HandleEvent(context.Background(), mentionEvent("Ev001", mention))).Required()

	replies := f.bot.replies()
	gt.Array(t, replies).Length(1).Required()
	gt.String(t, replies[0].ThreadTS).Equal(testThreadTS)
}

func TestSlackEventUseCase_NotConnected(t *testing.T) {
	f := newEventFixture(t)
	mention := topLevelMention()
	mention.ThreadTimeStamp = testThreadTS

	gt.NoError(t, f.events.HandleEvent(context.Background(), mentionEvent("Ev001", mention))).Required()

	ephemerals := f.bot.ephemeralMessages()
	gt.Array(t, ephemerals).Length(1).Required()
	gt.String(t, ephemerals[0].ChannelID).Equal(testChannel)
	gt.Value(t, ephemerals[0].UserID).Equal(testKey.UserID)
	gt.String(t, ephemerals[0].ThreadTS).Equal(testThreadTS)
	gt.String(t, ephemerals[0].Text).Contains(testBaseURL + "/login")
	gt.Array(t, f.bot.replies()).Length(0)
	gt.Number(t, f.cipher.decryptCount()).Equal(0)
}

func TestSlackEventUseCase_TokenInvalid(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)
	f.connect(t)
	f.factory.errs[testUserToken] = goerr.Wrap(interfaces.ErrSlackTokenInvalid, "token_revoked")

	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).Required()

	_, err := f.repo.SlackCredential().Get(ctx, testKey)
	gt.Error(t, err).Is(interfaces.ErrNotFound)
	gt.Array(t, f.bot.ephemeralMessages()).Length(1)
	gt.Array(t, f.bot.replies()).Length(0)
}

func TestSlackEventUseCase_TokenOfAnotherUser(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)
	f.connect(t)
	f.factory.identities[testUserToken] = &model.SlackIdentity{TeamID: testKey.TeamID, UserID: "U9999ZZZZ"}

	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).Required()

	_, err := f.repo.SlackCredential().Get(ctx, testKey)
	gt.Error(t, err).Is(interfaces.ErrNotFound)
	gt.Array(t, f.bot.ephemeralMessages()).Length(1)
	gt.Array(t, f.bot.replies()).Length(0)
}

func TestSlackEventUseCase_AuthTestOtherError(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)
	f.connect(t)
	f.factory.errs[testUserToken] = errors.New("ratelimited")

	gt.Value(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).NotNil()

	_, err := f.repo.SlackCredential().Get(ctx, testKey)
	gt.NoError(t, err)
	gt.Array(t, f.bot.ephemeralMessages()).Length(0)
	gt.Array(t, f.bot.replies()).Length(0)
}

func TestSlackEventUseCase_DecryptError(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)
	f.connect(t)
	f.cipher.decryptErr = errors.New("permission denied")

	gt.Value(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).NotNil()

	_, err := f.repo.SlackCredential().Get(ctx, testKey)
	gt.NoError(t, err)
	gt.Array(t, f.bot.ephemeralMessages()).Length(0)
	gt.Array(t, f.bot.replies()).Length(0)
}

func TestSlackEventUseCase_PostFailures(t *testing.T) {
	t.Run("thread reply", func(t *testing.T) {
		f := newEventFixture(t)
		f.connect(t)
		f.bot.replyErr = errors.New("channel_not_found")
		gt.Value(t, f.events.HandleEvent(context.Background(), mentionEvent("Ev001", topLevelMention()))).NotNil()
	})

	t.Run("login prompt", func(t *testing.T) {
		f := newEventFixture(t)
		f.bot.ephemeralErr = errors.New("channel_not_found")
		gt.Value(t, f.events.HandleEvent(context.Background(), mentionEvent("Ev001", topLevelMention()))).NotNil()
	})
}

func TestSlackEventUseCase_DuplicateEvent(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)
	f.connect(t)

	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).Required()
	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).Required()

	gt.Array(t, f.bot.replies()).Length(1)
	gt.Number(t, f.factory.authTestCount()).Equal(1)
}

func TestSlackEventUseCase_IgnoredEvents(t *testing.T) {
	cases := map[string]func() *slackevents.EventsAPIEvent{
		"bot mention": func() *slackevents.EventsAPIEvent {
			m := topLevelMention()
			m.BotID = "B0123"
			return mentionEvent("Ev001", m)
		},
		"another team": func() *slackevents.EventsAPIEvent {
			ev := mentionEvent("Ev001", topLevelMention())
			ev.TeamID = "T9999ZZZZ"
			return ev
		},
		"empty user": func() *slackevents.EventsAPIEvent {
			m := topLevelMention()
			m.User = ""
			return mentionEvent("Ev001", m)
		},
		"message event": func() *slackevents.EventsAPIEvent {
			return &slackevents.EventsAPIEvent{
				Type:   slackevents.CallbackEvent,
				TeamID: string(testKey.TeamID),
				Data:   &slackevents.EventsAPICallbackEvent{EventID: "Ev001"},
				InnerEvent: slackevents.EventsAPIInnerEvent{
					Type: string(slackevents.Message),
					Data: &slackevents.MessageEvent{User: string(testKey.UserID), Channel: testChannel},
				},
			}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := newEventFixture(t)
			f.connect(t)

			gt.NoError(t, f.events.HandleEvent(ctx, build())).Required()
			gt.Array(t, f.bot.replies()).Length(0)
			gt.Array(t, f.bot.ephemeralMessages()).Length(0)
			gt.Number(t, f.factory.authTestCount()).Equal(0)

			// No claim was recorded, so the same event ID is still claimable.
			claimed, err := f.repo.SlackEvent().Claim(ctx, &model.SlackEventClaim{
				EventID: "Ev001", ClaimedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
			})
			gt.NoError(t, err).Required()
			gt.Bool(t, claimed).True()
		})
	}
}

func TestLifecycle_LoginThenMention(t *testing.T) {
	ctx := context.Background()
	f := newEventFixture(t)

	// 1. Not signed in: the mention gets only the login prompt.
	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev001", topLevelMention()))).Required()
	gt.Array(t, f.bot.ephemeralMessages()).Length(1).Required()
	gt.Bool(t, strings.Contains(f.bot.ephemeralMessages()[0].Text, testBaseURL+"/login")).True()
	gt.Array(t, f.bot.replies()).Length(0)
	_, err := f.repo.SlackCredential().Get(ctx, testKey)
	gt.Error(t, err).Is(interfaces.ErrNotFound)

	// 2. Sign in on the web.
	_, _, err = f.uc.HandleCallback(ctx, "auth-code")
	gt.NoError(t, err).Required()
	_, err = f.repo.SlackCredential().Get(ctx, testKey)
	gt.NoError(t, err).Required()

	// 3. The next mention gets the fixed reply in the thread.
	gt.NoError(t, f.events.HandleEvent(ctx, mentionEvent("Ev002", topLevelMention()))).Required()
	replies := f.bot.replies()
	gt.Array(t, replies).Length(1).Required()
	gt.String(t, replies[0].ThreadTS).Equal(testMessageTS)
	gt.String(t, replies[0].Text).Contains("<@U0123ABCD>")
	gt.Array(t, f.bot.ephemeralMessages()).Length(1)
}
