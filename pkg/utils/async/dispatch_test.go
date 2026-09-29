package async_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/utils/async"
	"github.com/m-mizutani/ariel/pkg/utils/logging"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func capturingCtx() (context.Context, *syncBuffer) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), buf
}

func TestDispatch_RunsHandler(t *testing.T) {
	var called atomic.Bool
	async.Dispatch(context.Background(), func(_ context.Context) error {
		called.Store(true)
		return nil
	})
	async.Wait()
	gt.Bool(t, called.Load()).True()
}

func TestDispatch_HandlerErrorIsLogged(t *testing.T) {
	ctx, buf := capturingCtx()
	async.Dispatch(ctx, func(_ context.Context) error {
		return goerr.New("synthetic handler failure")
	})
	async.Wait()

	gt.String(t, buf.String()).Contains("async handler failed")
	gt.String(t, buf.String()).Contains("synthetic handler failure")
}

func TestDispatch_PanicIsRecoveredAndLogged(t *testing.T) {
	ctx, buf := capturingCtx()
	async.Dispatch(ctx, func(_ context.Context) error {
		panic("boom")
	})
	async.Wait()

	gt.String(t, buf.String()).Contains("async handler panicked")
	gt.String(t, buf.String()).Contains("boom")
}

func TestDrain(t *testing.T) {
	t.Run("returns after running handlers finish", func(t *testing.T) {
		release := make(chan struct{})
		var finished atomic.Bool
		async.Dispatch(context.Background(), func(_ context.Context) error {
			<-release
			finished.Store(true)
			return nil
		})
		close(release)

		gt.NoError(t, async.Drain(context.Background())).Required()
		gt.Bool(t, finished.Load()).True()
	})

	t.Run("gives up at the deadline", func(t *testing.T) {
		release := make(chan struct{})
		async.Dispatch(context.Background(), func(_ context.Context) error {
			<-release
			return nil
		})
		t.Cleanup(func() {
			close(release)
			async.Wait()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		gt.Error(t, async.Drain(ctx)).Is(context.DeadlineExceeded)
	})
}

func TestDispatch_CallerCancellationDoesNotCancelHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var handlerCtxErr atomic.Value
	async.Dispatch(ctx, func(ctx context.Context) error {
		handlerCtxErr.Store(ctx.Err() == nil)
		return nil
	})
	async.Wait()
	gt.Value(t, handlerCtxErr.Load()).Equal(true)
}
