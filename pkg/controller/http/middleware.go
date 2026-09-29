package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
	"github.com/m-mizutani/ariel/pkg/usecase"
	"github.com/m-mizutani/ariel/pkg/utils/errutil"
	"github.com/m-mizutani/ariel/pkg/utils/logging"
)

// requireSession authenticates the session cookies and puts the session into
// the request context. The user scope of the request comes only from this
// verified session.
func requireSession(authUC authUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			idCookie, idErr := r.Cookie(sessionIDCookieName)
			secretCookie, secretErr := r.Cookie(sessionSecretCookieName)
			if idErr != nil || secretErr != nil {
				errutil.Handle(ctx, goerr.New("session cookie is missing", goerr.T(errutil.TagBenign)), "request rejected")
				writeError(ctx, w, http.StatusUnauthorized, errCodeUnauthenticated)
				return
			}

			session, err := authUC.Authenticate(ctx, auth.SessionID(idCookie.Value), auth.SessionSecret(secretCookie.Value))
			if err != nil {
				if errors.Is(err, usecase.ErrUnauthenticated) {
					errutil.Handle(ctx, goerr.Wrap(err, "session rejected", goerr.T(errutil.TagBenign)), "request rejected")
					writeError(ctx, w, http.StatusUnauthorized, errCodeUnauthenticated)
					return
				}
				errutil.Handle(ctx, err, "failed to authenticate session")
				writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.ContextWithSession(ctx, session)))
		})
	}
}

func accessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			logging.From(r.Context()).Info("access",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"remote", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}
