package http

import (
	"context"
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

// authenticateRequest returns the session of the request cookies. Missing
// cookies and rejected sessions are usecase.ErrUnauthenticated.
func authenticateRequest(ctx context.Context, authUC authUseCase, r *http.Request) (*auth.Session, error) {
	idCookie, idErr := r.Cookie(sessionIDCookieName)
	secretCookie, secretErr := r.Cookie(sessionSecretCookieName)
	if idErr != nil || secretErr != nil {
		return nil, goerr.Wrap(usecase.ErrUnauthenticated, "session cookie is missing")
	}
	return authUC.Authenticate(ctx, auth.SessionID(idCookie.Value), auth.SessionSecret(secretCookie.Value))
}

// withSession authenticates the session cookies and puts the session into the
// request context; onUnauthenticated answers a request without a valid
// session. The user scope of the request comes only from this verified
// session.
func withSession(authUC authUseCase, onUnauthenticated http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			session, err := authenticateRequest(ctx, authUC, r)
			if err != nil {
				if errors.Is(err, usecase.ErrUnauthenticated) {
					errutil.Handle(ctx, goerr.Wrap(err, "session rejected", goerr.T(errutil.TagBenign)), "request rejected")
					onUnauthenticated(w, r)
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

// requireSession is for API calls: an unauthenticated request gets 401.
func requireSession(authUC authUseCase) func(http.Handler) http.Handler {
	return withSession(authUC, func(w http.ResponseWriter, r *http.Request) {
		writeError(r.Context(), w, http.StatusUnauthorized, errCodeUnauthenticated)
	})
}

// requireSessionOrLogin is for URLs the browser navigates to: an
// unauthenticated request is redirected to the login page, since a JSON error
// would leave the user on a page with no way forward.
func requireSessionOrLogin(authUC authUseCase) func(http.Handler) http.Handler {
	return withSession(authUC, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	})
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
