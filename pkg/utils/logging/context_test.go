package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/utils/logging"
)

func TestFrom(t *testing.T) {
	t.Run("returns the logger stored in the context", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		ctx := logging.With(context.Background(), logger)

		logging.From(ctx).Info("hello")
		gt.String(t, buf.String()).Contains("hello")
	})

	t.Run("falls back to the default logger", func(t *testing.T) {
		gt.Value(t, logging.From(context.Background())).Equal(logging.Default())
	})
}
