package config

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

type Slack struct {
	clientID      string
	clientSecret  string
	signingSecret string
	botToken      string
	teamID        string
}

func (x *Slack) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "slack-client-id",
			Category:    "Slack",
			Usage:       "Client ID of the Slack app",
			Sources:     cli.EnvVars("ROBIN_SLACK_CLIENT_ID"),
			Destination: &x.clientID,
		},
		&cli.StringFlag{
			Name:        "slack-client-secret",
			Category:    "Slack",
			Usage:       "Client secret of the Slack app",
			Sources:     cli.EnvVars("ROBIN_SLACK_CLIENT_SECRET"),
			Destination: &x.clientSecret,
		},
		&cli.StringFlag{
			Name:        "slack-signing-secret",
			Category:    "Slack",
			Usage:       "Signing secret of the Slack app, used to verify Events API requests",
			Sources:     cli.EnvVars("ROBIN_SLACK_SIGNING_SECRET"),
			Destination: &x.signingSecret,
		},
		&cli.StringFlag{
			Name:        "slack-bot-token",
			Category:    "Slack",
			Usage:       "Bot user OAuth token (xoxb-...)",
			Sources:     cli.EnvVars("ROBIN_SLACK_BOT_TOKEN"),
			Destination: &x.botToken,
		},
		&cli.StringFlag{
			Name:        "slack-team-id",
			Category:    "Slack",
			Usage:       "ID of the Slack workspace (T...) this server accepts",
			Sources:     cli.EnvVars("ROBIN_SLACK_TEAM_ID"),
			Destination: &x.teamID,
		},
	}
}

func (x *Slack) Validate() error {
	required := []struct {
		flag  string
		value string
	}{
		{"--slack-client-id", x.clientID},
		{"--slack-client-secret", x.clientSecret},
		{"--slack-signing-secret", x.signingSecret},
		{"--slack-bot-token", x.botToken},
		{"--slack-team-id", x.teamID},
	}
	for _, r := range required {
		if r.value == "" {
			return goerr.New(r.flag + " is required")
		}
	}
	if err := model.SlackTeamID(x.teamID).Validate(); err != nil {
		return goerr.Wrap(err, "invalid --slack-team-id")
	}
	return nil
}

// ValidateForNoAuth is the check used with --no-auth: only the workspace is
// required, since nobody signs in through Slack. The bot token and the signing
// secret receive Slack events, so they are set together or not at all.
func (x *Slack) ValidateForNoAuth() error {
	if x.teamID == "" {
		return goerr.New("--slack-team-id is required")
	}
	if err := model.SlackTeamID(x.teamID).Validate(); err != nil {
		return goerr.Wrap(err, "invalid --slack-team-id")
	}
	if (x.botToken == "") != (x.signingSecret == "") {
		return goerr.New("--slack-bot-token and --slack-signing-secret must be set together")
	}
	return nil
}

// EventsEnabled reports whether Slack events can be received and answered.
func (x *Slack) EventsEnabled() bool {
	return x.botToken != "" && x.signingSecret != ""
}

func (x *Slack) ClientID() string          { return x.clientID }
func (x *Slack) ClientSecret() string      { return x.clientSecret }
func (x *Slack) SigningSecret() string     { return x.signingSecret }
func (x *Slack) BotToken() string          { return x.botToken }
func (x *Slack) TeamID() model.SlackTeamID { return model.SlackTeamID(x.teamID) }
