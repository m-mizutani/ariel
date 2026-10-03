package config

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"
)

// GitHub holds the OAuth credentials of the GitHub App of this deployment.
// The integration is enabled only when both values are set.
type GitHub struct {
	clientID     string
	clientSecret string
}

func (x *GitHub) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "github-client-id",
			Category:    "GitHub",
			Usage:       "Client ID of the GitHub App for the GitHub integration (Iv...)",
			Sources:     cli.EnvVars("ROBIN_GITHUB_CLIENT_ID"),
			Destination: &x.clientID,
		},
		&cli.StringFlag{
			Name:        "github-client-secret",
			Category:    "GitHub",
			Usage:       "Client secret of the GitHub App for the GitHub integration",
			Sources:     cli.EnvVars("ROBIN_GITHUB_CLIENT_SECRET"),
			Destination: &x.clientSecret,
		},
	}
}

func (x *GitHub) Validate() error {
	if (x.clientID == "") != (x.clientSecret == "") {
		return goerr.New("--github-client-id and --github-client-secret must be set together")
	}
	return nil
}

// Enabled reports whether the GitHub integration is configured.
func (x *GitHub) Enabled() bool {
	return x.clientID != "" && x.clientSecret != ""
}

func (x *GitHub) ClientID() string     { return x.clientID }
func (x *GitHub) ClientSecret() string { return x.clientSecret }
