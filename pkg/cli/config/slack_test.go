package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/cli/config"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

func slackArgs() map[string]string {
	return map[string]string{
		"--slack-client-id":      "client-id",
		"--slack-client-secret":  "client-secret",
		"--slack-signing-secret": "signing-secret",
		"--slack-bot-token":      "xoxb-token",
		"--slack-team-id":        "T0123ABCD",
	}
}

func toArgs(m map[string]string) []string {
	var args []string
	for k, v := range m {
		args = append(args, k, v)
	}
	return args
}

func clearSlackEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, "ROBIN_SLACK_CLIENT_ID", "ROBIN_SLACK_CLIENT_SECRET", "ROBIN_SLACK_SIGNING_SECRET",
		"ROBIN_SLACK_BOT_TOKEN", "ROBIN_SLACK_TEAM_ID")
}

func TestSlack_Validate(t *testing.T) {
	clearSlackEnv(t)

	t.Run("all set", func(t *testing.T) {
		var s config.Slack
		parse(t, s.Flags(), toArgs(slackArgs())...)
		gt.NoError(t, s.Validate()).Required()
		gt.String(t, s.ClientID()).Equal("client-id")
		gt.String(t, s.ClientSecret()).Equal("client-secret")
		gt.String(t, s.SigningSecret()).Equal("signing-secret")
		gt.String(t, s.BotToken()).Equal("xoxb-token")
		gt.Value(t, s.TeamID()).Equal(model.SlackTeamID("T0123ABCD"))
	})

	for flag := range slackArgs() {
		t.Run("missing "+flag, func(t *testing.T) {
			args := slackArgs()
			delete(args, flag)
			var s config.Slack
			parse(t, s.Flags(), toArgs(args)...)
			err := s.Validate()
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains(flag)
		})
	}

	t.Run("invalid team ID", func(t *testing.T) {
		args := slackArgs()
		args["--slack-team-id"] = "U0123ABCD"
		var s config.Slack
		parse(t, s.Flags(), toArgs(args)...)
		gt.Error(t, s.Validate())
	})
}
