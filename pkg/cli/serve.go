package cli

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	googleadapter "github.com/m-mizutani/ariel/pkg/adapter/google"
	slackadapter "github.com/m-mizutani/ariel/pkg/adapter/slack"
	"github.com/m-mizutani/ariel/pkg/cli/config"
	httpctrl "github.com/m-mizutani/ariel/pkg/controller/http"
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/usecase"
	"github.com/m-mizutani/ariel/pkg/utils/async"
	"github.com/m-mizutani/ariel/pkg/utils/logging"
	"github.com/m-mizutani/ariel/pkg/utils/safe"
)

const (
	// slackEventClaimTTL covers Slack's retry window many times over; the
	// claims only exist to drop redelivered events.
	slackEventClaimTTL = 24 * time.Hour

	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type serveConfig struct {
	server     config.Server
	repository config.Repository
	slack      config.Slack
	kms        config.KMS
	google     config.Google
	noAuth     config.NoAuth
}

func (c *serveConfig) flags() []cli.Flag {
	var flags []cli.Flag
	flags = append(flags, c.server.Flags()...)
	flags = append(flags, c.repository.Flags()...)
	flags = append(flags, c.slack.Flags()...)
	flags = append(flags, c.kms.Flags()...)
	flags = append(flags, c.google.Flags()...)
	flags = append(flags, c.noAuth.Flags()...)
	return flags
}

func (c *serveConfig) validate() error {
	for _, v := range []interface{ Validate() error }{&c.server, &c.repository, &c.google, &c.noAuth} {
		if err := v.Validate(); err != nil {
			return err
		}
	}

	if !c.noAuth.Enabled() {
		if err := c.slack.Validate(); err != nil {
			return err
		}
		return c.kms.Validate()
	}

	// --no-auth lets anyone who opens the page act as the configured user.
	// Restricting it to the in-memory repository keeps it away from any
	// deployment that holds real data.
	if !c.repository.IsMemory() {
		return goerr.New("--no-auth requires --repository-backend memory")
	}
	if err := c.slack.ValidateForNoAuth(); err != nil {
		return err
	}
	if c.kms.IsSet() {
		return c.kms.Validate()
	}
	return nil
}

// unavailableCipher stands in for Cloud KMS in no-auth mode without a key.
// No token is stored in that mode, so it is never expected to be called.
type unavailableCipher struct{}

func (unavailableCipher) Encrypt(context.Context, []byte, []byte) (*model.EncryptedData, error) {
	return nil, goerr.New("KMS is not configured")
}

func (unavailableCipher) Decrypt(context.Context, *model.EncryptedData, []byte) ([]byte, error) {
	return nil, goerr.New("KMS is not configured")
}

func cmdServe() *cli.Command {
	var cfg serveConfig
	return &cli.Command{
		Name:  "serve",
		Usage: "Run the HTTP server for the web UI and the Slack bot",
		Flags: cfg.flags(),
		Action: func(ctx context.Context, _ *cli.Command) error {
			return runServe(ctx, &cfg)
		},
	}
}

func runServe(ctx context.Context, cfg *serveConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	repo, err := cfg.repository.Configure(ctx)
	if err != nil {
		return err
	}
	defer safe.Close(ctx, repo)

	var cipher interfaces.Cipher = unavailableCipher{}
	if cfg.kms.IsSet() {
		kmsClient, err := cfg.kms.Configure(ctx)
		if err != nil {
			return goerr.Wrap(err, "failed to initialize KMS")
		}
		defer safe.Close(ctx, kmsClient)
		cipher = kmsClient
	}

	var bot interfaces.SlackBot
	if cfg.slack.BotToken() != "" {
		bot = slackadapter.NewBot(cfg.slack.BotToken())
	}
	oauth := slackadapter.NewOAuth(cfg.slack.ClientID(), cfg.slack.ClientSecret())
	userClients := slackadapter.NewUserClientFactory()

	authCfg := usecase.AuthConfig{
		ClientID:   cfg.slack.ClientID(),
		BaseURL:    cfg.server.BaseURL(),
		TeamID:     cfg.slack.TeamID(),
		SessionTTL: cfg.server.SessionTTL(),
	}
	if cfg.noAuth.Enabled() {
		authCfg.NoAuthUserID = cfg.noAuth.UserID()
		logging.Default().Warn("authentication is disabled: every web sign-in becomes this user",
			"team_id", cfg.slack.TeamID(), "user_id", cfg.noAuth.UserID())
	}

	access := usecase.NewSlackUserAccess(repo, cipher, userClients)
	authUC := usecase.NewAuthUseCase(repo, oauth, bot, access, userClients, authCfg)

	var httpOpts []httpctrl.Option
	if cfg.slack.EventsEnabled() {
		slackUC := usecase.NewSlackEventUseCase(repo, bot, access, usecase.SlackEventConfig{
			TeamID:        cfg.slack.TeamID(),
			BaseURL:       cfg.server.BaseURL(),
			EventClaimTTL: slackEventClaimTTL,
		})
		httpOpts = append(httpOpts, httpctrl.WithSlackEvents(slackUC, cfg.slack.SigningSecret()))
	}
	if cfg.google.Enabled() {
		googleUC := usecase.NewGoogleWorkspaceUseCase(
			googleadapter.NewOAuth(cfg.google.ClientID(), cfg.google.ClientSecret()),
			usecase.NewGoogleWorkspaceAccess(repo, cipher),
			usecase.GoogleWorkspaceConfig{BaseURL: cfg.server.BaseURL()},
		)
		httpOpts = append(httpOpts, httpctrl.WithGoogleWorkspace(googleUC))
	}

	handler, err := httpctrl.New(authUC, httpctrl.Config{BaseURL: cfg.server.BaseURL()}, httpOpts...)
	if err != nil {
		return goerr.Wrap(err, "failed to build HTTP server")
	}

	server := &http.Server{
		Addr:              cfg.server.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// A plain goroutine instead of async.Dispatch: its error decides the
	// command's exit status, so it has to come back here, not to the log.
	serveErr := make(chan error, 1)
	go func() {
		logging.Default().Info("starting server", "addr", cfg.server.Addr(), "base_url", cfg.server.BaseURL())
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return goerr.Wrap(err, "HTTP server stopped", goerr.V("addr", cfg.server.Addr()))
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return goerr.Wrap(err, "failed to shut down HTTP server")
	}
	// Shutdown waits for HTTP handlers only. Slack events are still being
	// handled in the background, and they use the repository and KMS clients
	// that the deferred calls close when this function returns.
	if err := async.Drain(shutdownCtx); err != nil {
		return err
	}
	logging.Default().Info("server stopped")
	return nil
}
