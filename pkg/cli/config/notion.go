package config

import (
	"net/url"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

const defaultNotionAPIURL = "https://api.notion.com"

// Notion holds the public integration of the Notion integration. It is
// enabled only when the client ID, the client secret, and the workspace ID
// are all set.
type Notion struct {
	clientID     string
	clientSecret string
	workspaceID  string
	apiURL       string
}

func (x *Notion) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "notion-client-id",
			Category:    "Notion",
			Usage:       "OAuth client ID of the Notion public integration",
			Sources:     cli.EnvVars("ARIEL_NOTION_CLIENT_ID"),
			Destination: &x.clientID,
		},
		&cli.StringFlag{
			Name:        "notion-client-secret",
			Category:    "Notion",
			Usage:       "OAuth client secret of the Notion public integration",
			Sources:     cli.EnvVars("ARIEL_NOTION_CLIENT_SECRET"),
			Destination: &x.clientSecret,
		},
		&cli.StringFlag{
			Name:        "notion-workspace-id",
			Category:    "Notion",
			Usage:       "ID of the only Notion workspace users can connect (a UUID)",
			Sources:     cli.EnvVars("ARIEL_NOTION_WORKSPACE_ID"),
			Destination: &x.workspaceID,
		},
		&cli.StringFlag{
			Name:        "notion-api-url",
			Category:    "Development",
			Usage:       "Origin of the Notion API. Values other than the default are accepted only with --no-auth",
			Value:       defaultNotionAPIURL,
			Sources:     cli.EnvVars("ARIEL_NOTION_API_URL"),
			Destination: &x.apiURL,
		},
	}
}

// Validate checks the flags. noAuth reports whether --no-auth is set: only
// then may the Notion API URL point somewhere other than Notion.
func (x *Notion) Validate(noAuth bool) error {
	set := 0
	for _, v := range []string{x.clientID, x.clientSecret, x.workspaceID} {
		if v != "" {
			set++
		}
	}
	if set != 0 && set != 3 {
		return goerr.New("--notion-client-id, --notion-client-secret, and --notion-workspace-id must be set together")
	}
	if set == 3 {
		if err := x.WorkspaceID().Validate(); err != nil {
			return goerr.Wrap(err, "invalid --notion-workspace-id")
		}
	}

	u, err := url.Parse(x.apiURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return goerr.New("invalid --notion-api-url", goerr.V("notion_api_url", x.apiURL))
	}
	if x.APIURL() != defaultNotionAPIURL && !noAuth {
		return goerr.New("--notion-api-url can be changed only with --no-auth", goerr.V("notion_api_url", x.apiURL))
	}
	return nil
}

// Enabled reports whether the Notion integration is configured.
func (x *Notion) Enabled() bool {
	return x.clientID != "" && x.clientSecret != "" && x.workspaceID != ""
}

func (x *Notion) ClientID() string     { return x.clientID }
func (x *Notion) ClientSecret() string { return x.clientSecret }

// WorkspaceID is lower-cased, the form Notion returns in token responses.
func (x *Notion) WorkspaceID() model.NotionWorkspaceID {
	return model.NotionWorkspaceID(strings.ToLower(x.workspaceID))
}

func (x *Notion) APIURL() string { return strings.TrimSuffix(x.apiURL, "/") }
