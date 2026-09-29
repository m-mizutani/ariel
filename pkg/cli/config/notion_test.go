package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

var notionEnv = []string{"ARIEL_NOTION_CLIENT_ID", "ARIEL_NOTION_CLIENT_SECRET", "ARIEL_NOTION_WORKSPACE_ID", "ARIEL_NOTION_API_URL"}

func notionArgs() []string {
	return []string{
		"--notion-client-id", "client-id",
		"--notion-client-secret", "client-secret",
		"--notion-workspace-id", "0F4A2B1C-3D4E-4F50-8A6B-7C8D9E0F1A2B",
	}
}

func TestNotion_Validate(t *testing.T) {
	unsetEnv(t, notionEnv...)

	t.Run("all set", func(t *testing.T) {
		var n config.Notion
		parse(t, n.Flags(), notionArgs()...)
		gt.NoError(t, n.Validate(false)).Required()
		gt.Bool(t, n.Enabled()).True()
		gt.String(t, n.ClientID()).Equal("client-id")
		gt.String(t, n.ClientSecret()).Equal("client-secret")
		gt.Value(t, n.WorkspaceID()).Equal(model.NotionWorkspaceID("0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b"))
		gt.String(t, n.APIURL()).Equal("https://api.notion.com")
	})

	t.Run("none set", func(t *testing.T) {
		var n config.Notion
		parse(t, n.Flags())
		gt.NoError(t, n.Validate(false))
		gt.Bool(t, n.Enabled()).False()
	})

	for name, args := range map[string][]string{
		"only client ID":        {"--notion-client-id", "client-id"},
		"without workspace ID":  {"--notion-client-id", "client-id", "--notion-client-secret", "client-secret"},
		"without client secret": {"--notion-client-id", "client-id", "--notion-workspace-id", "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b"},
	} {
		t.Run(name, func(t *testing.T) {
			var n config.Notion
			parse(t, n.Flags(), args...)
			err := n.Validate(false)
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("--notion-client-id, --notion-client-secret, and --notion-workspace-id must be set together")
			gt.Bool(t, n.Enabled()).False()
		})
	}

	t.Run("malformed workspace ID", func(t *testing.T) {
		var n config.Notion
		parse(t, n.Flags(), "--notion-client-id", "c", "--notion-client-secret", "s", "--notion-workspace-id", "Example")
		err := n.Validate(false)
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("--notion-workspace-id")
	})

	t.Run("another API URL needs --no-auth", func(t *testing.T) {
		var n config.Notion
		parse(t, n.Flags(), append(notionArgs(), "--notion-api-url", "http://127.0.0.1:18082/")...)
		err := n.Validate(false)
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("--notion-api-url can be changed only with --no-auth")

		gt.NoError(t, n.Validate(true))
		gt.String(t, n.APIURL()).Equal("http://127.0.0.1:18082")
	})

	for name, u := range map[string]string{"no scheme": "api.notion.com", "other scheme": "ftp://api.notion.com", "no host": "https://"} {
		t.Run("invalid API URL: "+name, func(t *testing.T) {
			var n config.Notion
			parse(t, n.Flags(), "--notion-api-url", u)
			err := n.Validate(true)
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("invalid --notion-api-url")
		})
	}
}

func TestNotion_FromEnv(t *testing.T) {
	t.Setenv("ARIEL_NOTION_CLIENT_ID", "env-client-id")
	t.Setenv("ARIEL_NOTION_CLIENT_SECRET", "env-client-secret")
	t.Setenv("ARIEL_NOTION_WORKSPACE_ID", "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b")
	t.Setenv("ARIEL_NOTION_API_URL", "http://127.0.0.1:18082")

	var n config.Notion
	parse(t, n.Flags())
	gt.NoError(t, n.Validate(true)).Required()
	gt.String(t, n.ClientID()).Equal("env-client-id")
	gt.String(t, n.ClientSecret()).Equal("env-client-secret")
	gt.Value(t, n.WorkspaceID()).Equal(model.NotionWorkspaceID("0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b"))
	gt.String(t, n.APIURL()).Equal("http://127.0.0.1:18082")
}
