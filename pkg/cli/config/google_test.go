package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
)

func TestGoogle_Validate(t *testing.T) {
	unsetEnv(t, "ARIEL_GOOGLE_CLIENT_ID", "ARIEL_GOOGLE_CLIENT_SECRET")

	t.Run("both set", func(t *testing.T) {
		var g config.Google
		parse(t, g.Flags(), "--google-client-id", "client-id", "--google-client-secret", "client-secret")
		gt.NoError(t, g.Validate()).Required()
		gt.Bool(t, g.Enabled()).True()
		gt.String(t, g.ClientID()).Equal("client-id")
		gt.String(t, g.ClientSecret()).Equal("client-secret")
	})

	t.Run("neither set", func(t *testing.T) {
		var g config.Google
		parse(t, g.Flags())
		gt.NoError(t, g.Validate())
		gt.Bool(t, g.Enabled()).False()
	})

	for name, args := range map[string][]string{
		"only client ID":     {"--google-client-id", "client-id"},
		"only client secret": {"--google-client-secret", "client-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			var g config.Google
			parse(t, g.Flags(), args...)
			err := g.Validate()
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains("--google-client-id and --google-client-secret must be set together")
			gt.Bool(t, g.Enabled()).False()
		})
	}
}

func TestGoogle_FromEnv(t *testing.T) {
	t.Setenv("ARIEL_GOOGLE_CLIENT_ID", "env-client-id")
	t.Setenv("ARIEL_GOOGLE_CLIENT_SECRET", "env-client-secret")

	var g config.Google
	parse(t, g.Flags())
	gt.NoError(t, g.Validate()).Required()
	gt.String(t, g.ClientID()).Equal("env-client-id")
	gt.String(t, g.ClientSecret()).Equal("env-client-secret")
}
