// Package errutil records errors at the point where they stop propagating.
// Every error that is not returned to a caller (an HTTP handler that writes
// the response, the tail of a background task, a recovered panic, the CLI
// entry point) must be passed to Handle, whatever its severity, so each error
// is logged exactly once.
package errutil

import (
	"context"
	"errors"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/utils/logging"
)

// TagBenign marks an error that occurs in normal operation (a user
// declining the Slack authorization, a request without a session cookie).
// Such errors are logged at INFO level instead of ERROR.
var TagBenign = goerr.NewTag("benign")

// Handle logs err with its goerr values and stack. It is a no-op for nil.
func Handle(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}

	level := slog.LevelError
	if goerr.HasTag(err, TagBenign) {
		level = slog.LevelInfo
	}

	logger := logging.From(ctx)
	var ge *goerr.Error
	if errors.As(err, &ge) {
		logger.Log(ctx, level, msg,
			"error", err.Error(),
			"values", goerr.Values(err),
			"stack", ge.Stacks(),
		)
		return
	}
	logger.Log(ctx, level, msg, "error", err.Error())
}
