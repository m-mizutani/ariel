package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/cli/config"
)

func TestLogger_Configure(t *testing.T) {
	unsetEnv(t, "ROBIN_LOG_LEVEL", "ROBIN_LOG_FORMAT")

	t.Run("defaults", func(t *testing.T) {
		var l config.Logger
		parse(t, l.Flags())
		gt.NoError(t, l.Configure())
	})

	t.Run("json and debug", func(t *testing.T) {
		var l config.Logger
		parse(t, l.Flags(), "--log-format", "json", "--log-level", "debug")
		gt.NoError(t, l.Configure())
	})

	t.Run("invalid level", func(t *testing.T) {
		var l config.Logger
		parse(t, l.Flags(), "--log-level", "verbose")
		gt.Error(t, l.Configure())
	})

	t.Run("invalid format", func(t *testing.T) {
		var l config.Logger
		parse(t, l.Flags(), "--log-format", "xml")
		gt.Error(t, l.Configure())
	})
}
