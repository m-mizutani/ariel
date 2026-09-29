package slack_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	slackgo "github.com/slack-go/slack"

	"github.com/m-mizutani/ariel/pkg/adapter/slack"
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
)

func TestWrapError(t *testing.T) {
	t.Run("token error wraps ErrSlackTokenInvalid and keeps the code", func(t *testing.T) {
		err := slack.WrapErrorForTest(slackgo.SlackErrorResponse{Err: "token_revoked"}, "call failed")
		gt.Bool(t, errors.Is(err, interfaces.ErrSlackTokenInvalid)).True()
		gt.Value(t, goerr.Values(err)["slack_error"]).Equal("token_revoked")
	})

	t.Run("other slack error keeps the original error", func(t *testing.T) {
		original := slackgo.SlackErrorResponse{Err: "channel_not_found"}
		err := slack.WrapErrorForTest(original, "call failed")
		gt.Bool(t, errors.Is(err, interfaces.ErrSlackTokenInvalid)).False()
		var slackErr slackgo.SlackErrorResponse
		gt.Bool(t, errors.As(err, &slackErr)).True()
		gt.String(t, slackErr.Err).Equal("channel_not_found")
	})

	t.Run("non slack error is wrapped", func(t *testing.T) {
		original := errors.New("connection refused")
		err := slack.WrapErrorForTest(original, "call failed")
		gt.Bool(t, errors.Is(err, original)).True()
		gt.Bool(t, errors.Is(err, interfaces.ErrSlackTokenInvalid)).False()
	})
}
