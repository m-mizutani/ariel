package config_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/robin/pkg/cli/config"
)

// unsetEnv removes the variables for the test. Setting them to "" is not
// enough: urfave/cli takes an empty variable as the flag value and skips the
// default.
func unsetEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		t.Setenv(name, "") // registers the restore on cleanup
		gt.NoError(t, os.Unsetenv(name)).Required()
	}
}

// parse runs a command holding only flags, so the Destination fields are
// filled exactly as `robin serve` fills them.
func parse(t *testing.T, flags []cli.Flag, args ...string) {
	t.Helper()
	cmd := &cli.Command{
		Name:   "test",
		Flags:  flags,
		Action: func(context.Context, *cli.Command) error { return nil },
	}
	gt.NoError(t, cmd.Run(context.Background(), append([]string{"test"}, args...))).Required()
}

func TestServer_Validate(t *testing.T) {
	unsetEnv(t, "ROBIN_ADDR", "ROBIN_BASE_URL", "ROBIN_SESSION_TTL")

	t.Run("defaults", func(t *testing.T) {
		var s config.Server
		parse(t, s.Flags(), "--base-url", "https://robin.example.com")
		gt.NoError(t, s.Validate()).Required()
		gt.String(t, s.Addr()).Equal(":8080")
		gt.Value(t, s.SessionTTL()).Equal(7 * 24 * time.Hour)
		gt.String(t, s.BaseURL()).Equal("https://robin.example.com")
	})

	t.Run("trailing slash is removed", func(t *testing.T) {
		var s config.Server
		parse(t, s.Flags(), "--base-url", "https://robin.example.com/")
		gt.NoError(t, s.Validate()).Required()
		gt.String(t, s.BaseURL()).Equal("https://robin.example.com")
	})

	t.Run("http with port", func(t *testing.T) {
		var s config.Server
		parse(t, s.Flags(), "--base-url", "http://localhost:8080")
		gt.NoError(t, s.Validate())
	})

	invalid := map[string][]string{
		"missing base url": {},
		"ftp scheme":       {"--base-url", "ftp://robin.example.com"},
		"with path":        {"--base-url", "https://robin.example.com/x"},
		"with query":       {"--base-url", "https://robin.example.com?a=b"},
		"no host":          {"--base-url", "https://"},
		"with credentials": {"--base-url", "https://user:pass@robin.example.com"},
		"zero session ttl": {"--base-url", "https://robin.example.com", "--session-ttl", "0s"},
		"negative ttl":     {"--base-url", "https://robin.example.com", "--session-ttl", "-1h"},
	}
	for name, args := range invalid {
		t.Run(name, func(t *testing.T) {
			var s config.Server
			parse(t, s.Flags(), args...)
			gt.Error(t, s.Validate())
		})
	}

	t.Run("missing base url names the flag", func(t *testing.T) {
		var s config.Server
		parse(t, s.Flags())
		gt.String(t, s.Validate().Error()).Contains("--base-url")
	})

	t.Run("environment variable", func(t *testing.T) {
		t.Setenv("ROBIN_BASE_URL", "https://env.example.com")
		t.Setenv("ROBIN_SESSION_TTL", "1h")
		var s config.Server
		parse(t, s.Flags())
		gt.NoError(t, s.Validate()).Required()
		gt.String(t, s.BaseURL()).Equal("https://env.example.com")
		gt.Value(t, s.SessionTTL()).Equal(time.Hour)
	})
}
