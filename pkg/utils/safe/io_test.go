package safe_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/utils/logging"
	"github.com/m-mizutani/ariel/pkg/utils/safe"
)

func capturingCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

type errCloser struct{ err error }

func (c *errCloser) Close() error { return c.err }

type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestClose(t *testing.T) {
	t.Run("nil closer is a no-op", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, nil)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("successful close records nothing", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, &errCloser{})
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("failing close is recorded", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		safe.Close(ctx, &errCloser{err: errors.New("disk failure")})
		gt.String(t, buf.String()).Contains("failed to close")
		gt.String(t, buf.String()).Contains("disk failure")
	})
}

func TestCopy(t *testing.T) {
	t.Run("successful copy delivers the bytes", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		var out bytes.Buffer
		safe.Copy(ctx, &out, strings.NewReader("payload"))
		gt.String(t, out.String()).Equal("payload")
		gt.String(t, logBuf.String()).Equal("")
	})

	t.Run("failing read is recorded", func(t *testing.T) {
		ctx, logBuf := capturingCtx(t)
		var out bytes.Buffer
		safe.Copy(ctx, &out, &errReader{err: errors.New("source gone")})
		gt.String(t, logBuf.String()).Contains("failed to copy")
		gt.String(t, logBuf.String()).Contains("source gone")
	})
}
