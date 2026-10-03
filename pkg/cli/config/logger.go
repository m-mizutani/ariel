package config

import (
	"log/slog"
	"os"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/robin/pkg/utils/logging"
)

type Logger struct {
	level  string
	format string
}

func (x *Logger) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "log-level",
			Category:    "Logging",
			Usage:       "Log level [debug|info|warn|error]",
			Value:       "info",
			Sources:     cli.EnvVars("ROBIN_LOG_LEVEL"),
			Destination: &x.level,
		},
		&cli.StringFlag{
			Name:        "log-format",
			Category:    "Logging",
			Usage:       "Log format [console|json]",
			Value:       "console",
			Sources:     cli.EnvVars("ROBIN_LOG_FORMAT"),
			Destination: &x.format,
		},
	}
}

// Configure installs the default logger writing to stdout.
func (x *Logger) Configure() error {
	levels := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	level, ok := levels[x.level]
	if !ok {
		return goerr.New("invalid --log-level", goerr.V("log_level", x.level))
	}

	formats := map[string]logging.Format{
		"console": logging.FormatConsole,
		"json":    logging.FormatJSON,
	}
	format, ok := formats[x.format]
	if !ok {
		return goerr.New("invalid --log-format", goerr.V("log_format", x.format))
	}

	logging.SetDefault(logging.New(os.Stdout, level, format, format == logging.FormatConsole))
	return nil
}
