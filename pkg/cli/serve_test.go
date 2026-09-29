package cli_test

import (
	"context"
	"os"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli"
)

func clearServeEnv(t *testing.T) {
	t.Helper()
	// Unset rather than set to "": urfave/cli takes an empty variable as the
	// flag value and skips the default.
	for _, name := range []string{
		"ARIEL_ADDR", "ARIEL_BASE_URL", "ARIEL_SESSION_TTL", "ARIEL_LOG_LEVEL", "ARIEL_LOG_FORMAT",
		"ARIEL_REPOSITORY_BACKEND", "ARIEL_FIRESTORE_PROJECT_ID", "ARIEL_FIRESTORE_DATABASE_ID",
		"ARIEL_SLACK_CLIENT_ID", "ARIEL_SLACK_CLIENT_SECRET", "ARIEL_SLACK_SIGNING_SECRET",
		"ARIEL_SLACK_BOT_TOKEN", "ARIEL_SLACK_TEAM_ID", "ARIEL_KMS_KEY_NAME",
	} {
		t.Setenv(name, "")
		gt.NoError(t, os.Unsetenv(name)).Required()
	}
}

func validServeArgs() []string {
	return []string{
		"ariel", "--log-format", "json", "serve",
		"--base-url", "https://ariel.example.com",
		"--repository-backend", "memory",
		"--slack-client-id", "client-id",
		"--slack-client-secret", "client-secret",
		"--slack-signing-secret", "signing-secret",
		"--slack-bot-token", "xoxb-token",
		"--slack-team-id", "T0123ABCD",
		"--kms-key-name", "projects/p/locations/l/keyRings/r/cryptoKeys/k",
	}
}

func without(args []string, flag string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// Each required setting stops the command before anything is connected, and
// the error names the missing flag.
func TestServe_RequiredFlags(t *testing.T) {
	clearServeEnv(t)
	for _, flag := range []string{
		"--base-url",
		"--slack-client-id",
		"--slack-client-secret",
		"--slack-signing-secret",
		"--slack-bot-token",
		"--slack-team-id",
		"--kms-key-name",
	} {
		t.Run(flag, func(t *testing.T) {
			err := cli.Run(context.Background(), without(validServeArgs(), flag), "test")
			gt.Value(t, err).NotNil().Required()
			gt.String(t, err.Error()).Contains(flag)
		})
	}
}

func TestServe_FirestoreRequiresProjectID(t *testing.T) {
	clearServeEnv(t)
	args := without(validServeArgs(), "--repository-backend")
	err := cli.Run(context.Background(), args, "test")
	gt.Value(t, err).NotNil().Required()
	gt.String(t, err.Error()).Contains("--firestore-project-id")
}

func TestRun_InvalidLogLevel(t *testing.T) {
	clearServeEnv(t)
	err := cli.Run(context.Background(), []string{"ariel", "--log-level", "verbose", "serve"}, "test")
	gt.Value(t, err).NotNil()
}
