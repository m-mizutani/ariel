package errutil_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/utils/errutil"
	"github.com/m-mizutani/ariel/pkg/utils/logging"
)

func capturingCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var record map[string]any
	gt.NoError(t, json.Unmarshal(buf.Bytes(), &record)).Required()
	return record
}

func TestHandle(t *testing.T) {
	t.Run("nil error writes nothing", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		errutil.Handle(ctx, nil, "should not appear")
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("plain error is logged at ERROR", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		errutil.Handle(ctx, errors.New("plain failure"), "operation failed")

		record := decode(t, buf)
		gt.Value(t, record["level"]).Equal("ERROR")
		gt.Value(t, record["msg"]).Equal("operation failed")
		gt.Value(t, record["error"]).Equal("plain failure")
	})

	t.Run("goerr values are logged", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		errutil.Handle(ctx, goerr.New("typed failure", goerr.V("user_id", "U123")), "operation failed")

		record := decode(t, buf)
		gt.Value(t, record["level"]).Equal("ERROR")
		values, ok := record["values"].(map[string]any)
		gt.Bool(t, ok).True().Required()
		gt.Value(t, values["user_id"]).Equal("U123")
	})

	t.Run("benign error is logged at INFO", func(t *testing.T) {
		ctx, buf := capturingCtx(t)
		err := goerr.Wrap(errors.New("no cookie"), "unauthenticated", goerr.T(errutil.TagBenign))
		errutil.Handle(ctx, err, "request rejected")

		record := decode(t, buf)
		gt.Value(t, record["level"]).Equal("INFO")
	})
}
