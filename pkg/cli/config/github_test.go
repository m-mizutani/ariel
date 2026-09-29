package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
)

func TestGitHub_Validate(t *testing.T) {
	unsetEnv(t, "ARIEL_GITHUB_CLIENT_ID", "ARIEL_GITHUB_CLIENT_SECRET")

	t.Run("both set", func(t *testing.T) {
		var g config.GitHub
		parse(t, g.Flags(), "--github-client-id", "Iv1.client", "--github-client-secret", "client-secret")
		gt.NoError(t, g.Validate()).Required()
		gt.Bool(t, g.Enabled()).True()
		gt.String(t, g.ClientID()).Equal("Iv1.client")
		gt.String(t, g.ClientSecret()).Equal("client-secret")
	})

	t.Run("neither set", func(t *testing.T) {
		var g config.GitHub
		parse(t, g.Flags())
		gt.NoError(t, g.Validate())
		gt.Bool(t, g.Enabled()).False()
	})

	for name, args := range map[string][]string{
		"only client ID":     {"--github-client-id", "Iv1.client"},
		"only client secret": {"--github-client-secret", "client-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			var g config.GitHub
			parse(t, g.Flags(), args...)
			err := g.Validate()
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("--github-client-id and --github-client-secret must be set together")
			gt.Bool(t, g.Enabled()).False()
		})
	}
}

func TestGitHub_FromEnv(t *testing.T) {
	t.Setenv("ARIEL_GITHUB_CLIENT_ID", "Iv1.env")
	t.Setenv("ARIEL_GITHUB_CLIENT_SECRET", "env-client-secret")

	var g config.GitHub
	parse(t, g.Flags())
	gt.NoError(t, g.Validate()).Required()
	gt.String(t, g.ClientID()).Equal("Iv1.env")
	gt.String(t, g.ClientSecret()).Equal("env-client-secret")
}
