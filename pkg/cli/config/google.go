package config

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"
)

// Google holds the OAuth client of the Google Workspace integration. The
// integration is enabled only when both values are set.
type Google struct {
	clientID     string
	clientSecret string
}

func (x *Google) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "google-client-id",
			Category:    "Google Workspace",
			Usage:       "Client ID of the Google OAuth client (Web application) for the Google Workspace integration",
			Sources:     cli.EnvVars("ARIEL_GOOGLE_CLIENT_ID"),
			Destination: &x.clientID,
		},
		&cli.StringFlag{
			Name:        "google-client-secret",
			Category:    "Google Workspace",
			Usage:       "Client secret of the Google OAuth client for the Google Workspace integration",
			Sources:     cli.EnvVars("ARIEL_GOOGLE_CLIENT_SECRET"),
			Destination: &x.clientSecret,
		},
	}
}

func (x *Google) Validate() error {
	if (x.clientID == "") != (x.clientSecret == "") {
		return goerr.New("--google-client-id and --google-client-secret must be set together")
	}
	return nil
}

// Enabled reports whether the Google Workspace integration is configured.
func (x *Google) Enabled() bool {
	return x.clientID != "" && x.clientSecret != ""
}

func (x *Google) ClientID() string     { return x.clientID }
func (x *Google) ClientSecret() string { return x.clientSecret }
