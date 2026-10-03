package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const (
	errCodeUnauthenticated = "unauthenticated"
	errCodeInternal        = "internal_error"
	errCodeNotFound        = "not_found"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to encode JSON response"), "failed to write response")
	}
}

// writeError writes a fixed error code. Error details go to the log, never to
// the response body.
func writeError(ctx context.Context, w http.ResponseWriter, status int, code string) {
	writeJSON(ctx, w, status, errorResponse{Error: code})
}
