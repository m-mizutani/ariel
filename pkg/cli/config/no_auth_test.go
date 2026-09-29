package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func TestNoAuth(t *testing.T) {
	unsetEnv(t, "ARIEL_NO_AUTH")

	t.Run("disabled by default", func(t *testing.T) {
		var n config.NoAuth
		parse(t, n.Flags())
		gt.Bool(t, n.Enabled()).False()
		gt.NoError(t, n.Validate())
	})

	t.Run("user ID", func(t *testing.T) {
		var n config.NoAuth
		parse(t, n.Flags(), "--no-auth", "U0E2ETEST")
		gt.Bool(t, n.Enabled()).True()
		gt.NoError(t, n.Validate())
		gt.Value(t, n.UserID()).Equal(model.SlackUserID("U0E2ETEST"))
	})

	t.Run("invalid user ID", func(t *testing.T) {
		var n config.NoAuth
		parse(t, n.Flags(), "--no-auth", "alice")
		gt.Error(t, n.Validate())
	})
}

func TestSlack_ValidateForNoAuth(t *testing.T) {
	clearSlackEnv(t)

	t.Run("team ID only", func(t *testing.T) {
		var s config.Slack
		parse(t, s.Flags(), "--slack-team-id", "T0123ABCD")
		gt.NoError(t, s.ValidateForNoAuth())
		gt.Bool(t, s.EventsEnabled()).False()
	})

	t.Run("with bot token and signing secret", func(t *testing.T) {
		var s config.Slack
		parse(t, s.Flags(), "--slack-team-id", "T0123ABCD", "--slack-bot-token", "xoxb-x", "--slack-signing-secret", "s")
		gt.NoError(t, s.ValidateForNoAuth())
		gt.Bool(t, s.EventsEnabled()).True()
	})

	t.Run("missing team ID", func(t *testing.T) {
		var s config.Slack
		parse(t, s.Flags())
		gt.Error(t, s.ValidateForNoAuth())
	})

	t.Run("bot token without signing secret", func(t *testing.T) {
		var s config.Slack
		parse(t, s.Flags(), "--slack-team-id", "T0123ABCD", "--slack-bot-token", "xoxb-x")
		gt.Error(t, s.ValidateForNoAuth())
	})
}
