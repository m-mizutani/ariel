package config

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

// NoAuth turns off Slack authorization for E2E tests and local development:
// every sign-in becomes the configured user.
type NoAuth struct {
	userID string
}

func (x *NoAuth) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "no-auth",
			Category: "Development",
			Usage: "Sign every web login in as this Slack user ID (U...) of --slack-team-id without asking Slack. " +
				"For E2E tests and local development only; requires --repository-backend memory",
			Sources:     cli.EnvVars("ROBIN_NO_AUTH"),
			Destination: &x.userID,
		},
	}
}

func (x *NoAuth) Enabled() bool {
	return x.userID != ""
}

func (x *NoAuth) Validate() error {
	if !x.Enabled() {
		return nil
	}
	if err := model.SlackUserID(x.userID).Validate(); err != nil {
		return goerr.Wrap(err, "invalid --no-auth")
	}
	return nil
}

func (x *NoAuth) UserID() model.SlackUserID {
	return model.SlackUserID(x.userID)
}
