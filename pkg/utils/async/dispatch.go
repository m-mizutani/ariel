package async

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/utils/errutil"
)

var inflight sync.WaitGroup

// Dispatch runs handler in a new goroutine. The context keeps every value of
// the caller's ctx but not its cancellation, because the caller (an HTTP
// request that already answered Slack) returns before the handler finishes.
// Handler errors and panics are recorded through errutil.Handle.
func Dispatch(ctx context.Context, handler func(ctx context.Context) error) {
	bgCtx := context.WithoutCancel(ctx)

	inflight.Add(1)
	go func() {
		defer inflight.Done()
		defer func() {
			if r := recover(); r != nil {
				errutil.Handle(bgCtx, goerr.New("panic in async handler", goerr.V("panic", r)), "async handler panicked")
			}
		}()

		if err := handler(bgCtx); err != nil {
			errutil.Handle(bgCtx, err, "async handler failed")
		}
	}()
}

// Drain waits until every handler started by Dispatch has returned, or until
// ctx is done. The server calls it on shutdown, after it stopped accepting
// requests and before it closes the clients the handlers use.
func Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return goerr.Wrap(ctx.Err(), "background handlers did not finish before the deadline")
	}
}

// Wait blocks until every handler started by Dispatch has returned. It exists
// for tests that assert on side effects of the background work; production
// code uses Drain.
func Wait() {
	inflight.Wait()
}
