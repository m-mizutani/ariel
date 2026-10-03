package safe

import (
	"context"
	"io"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

// Close closes closer and records a failure through errutil.Handle. A nil
// closer is ignored.
func Close(ctx context.Context, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to close"), "failed to close")
	}
}

// Copy copies src to dst and records a failure through errutil.Handle.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) {
	if _, err := io.Copy(dst, src); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to copy"), "failed to copy")
	}
}
