package logging_test

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/utils/logging"
)

func TestIsTerminal(t *testing.T) {
	gt.Bool(t, logging.IsTerminalForTest(&bytes.Buffer{})).False()
	gt.Bool(t, logging.IsTerminalForTest(io.Discard)).False()
}

func TestNew_ConsoleExpandsGoerrValues(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatConsole, false)

	logger.Info("something failed", slog.Any("err", goerr.New("boom", goerr.V("op", "ping"))))

	out := buf.String()
	gt.String(t, out).Contains("err.op=")
	gt.String(t, out).Contains("ping")
	gt.String(t, out).Contains("boom")
	gt.Bool(t, strings.Contains(out, "\x1b[")).False()
}

func TestNew_JSONRedactsSecretFields(t *testing.T) {
	type payload struct {
		ID     string
		Secret string `masq:"secret"`
	}
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatJSON, false)

	logger.Info("value", slog.Any("payload", payload{ID: "visible-id", Secret: "hidden-value"}))

	out := buf.String()
	gt.String(t, out).Contains("visible-id")
	gt.Bool(t, strings.Contains(out, "hidden-value")).False()
}

func TestNew_LevelFiltersLowerMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelWarn, logging.FormatJSON, false)

	logger.Info("info message")
	logger.Warn("warn message")

	out := buf.String()
	gt.Bool(t, strings.Contains(out, "info message")).False()
	gt.String(t, out).Contains("warn message")
}
