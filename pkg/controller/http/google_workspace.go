package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
	"github.com/m-mizutani/ariel/pkg/usecase"
	"github.com/m-mizutani/ariel/pkg/utils/errutil"
)

const (
	googleStateCookieName = "ariel_google_oauth_state"
	googleStateCookiePath = apiV1Path + "/integrations/google-workspace"

	// The settings page reads googleResultParam to tell the user how the
	// connection ended. Only these fixed values are ever sent.
	googleResultParam        = "google_workspace"
	googleResultConnected    = "connected"
	googleResultAccessDenied = "access_denied"
	googleResultMissingScope = "missing_scope"
	googleResultAccountInUse = "account_in_use"
	googleResultFailed       = "failed"

	googleErrorAccessDenied = "access_denied"
)

type googleStatusResponse struct {
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	Email     string `json:"email"`
}

func redirectGoogleResult(w http.ResponseWriter, r *http.Request, result string) {
	http.Redirect(w, r, "/settings?"+googleResultParam+"="+result, http.StatusFound)
}

// redirectToSettings returns to the settings page without a result: a user who
// already has a connected account is sent back and nothing changes.
func redirectToSettings(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings", http.StatusFound)
}

func sessionFromRequest(w http.ResponseWriter, r *http.Request) (*auth.Session, bool) {
	session, ok := auth.SessionFromContext(r.Context())
	if !ok {
		errutil.Handle(r.Context(), goerr.New("session is not in the request context"), "request failed")
		writeError(r.Context(), w, http.StatusInternalServerError, errCodeInternal)
	}
	return session, ok
}

func (s *Server) googleStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	if s.googleUC == nil {
		writeJSON(ctx, w, http.StatusOK, googleStatusResponse{})
		return
	}

	status, err := s.googleUC.Status(ctx, session.Key())
	if err != nil {
		errutil.Handle(ctx, err, "failed to load google workspace status")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, googleStatusResponse{
		Available: true,
		Connected: status.Connected,
		Email:     status.Email,
	})
}

func (s *Server) googleConnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	state, err := generateState()
	if err != nil {
		errutil.Handle(ctx, err, "failed to start google workspace connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	authorizeURL, err := s.googleUC.AuthorizeURL(ctx, session.Key(), state)
	if err != nil {
		if errors.Is(err, usecase.ErrGoogleWorkspaceAlreadyConnected) {
			errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "google workspace is already connected")
			redirectToSettings(w, r)
			return
		}
		errutil.Handle(ctx, err, "failed to start google workspace connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	s.setCookie(w, googleStateCookieName, oauthStateCookieValue(state, session), googleStateCookiePath, stateCookieMaxAge, time.Time{})
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (s *Server) googleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	// The state cookie is single use, whatever the outcome.
	s.clearCookie(w, googleStateCookieName, googleStateCookiePath)

	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	vals := []goerr.Option{goerr.V("team_id", session.TeamID), goerr.V("user_id", session.UserID)}

	if googleErr := q.Get("error"); googleErr != "" {
		if googleErr == googleErrorAccessDenied {
			errutil.Handle(ctx, goerr.New("user declined the google authorization",
				append(vals, goerr.T(errutil.TagBenign))...), "google workspace connection cancelled")
			redirectGoogleResult(w, r, googleResultAccessDenied)
			return
		}
		errutil.Handle(ctx, goerr.New("google returned an authorization error",
			append(vals, goerr.V("google_error", googleErr))...), "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultFailed)
		return
	}

	if err := verifyOAuthState(r, googleStateCookieName, q.Get("state"), session); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "google oauth state check failed", vals...), "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultFailed)
		return
	}
	code := q.Get("code")
	if code == "" {
		errutil.Handle(ctx, goerr.New("authorization code is missing", vals...), "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultFailed)
		return
	}

	err := s.googleUC.HandleCallback(ctx, session.Key(), code)
	switch {
	case err == nil:
		redirectGoogleResult(w, r, googleResultConnected)
	case errors.Is(err, usecase.ErrGoogleWorkspaceAlreadyConnected):
		errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "google workspace is already connected")
		redirectToSettings(w, r)
	case errors.Is(err, usecase.ErrGoogleScopeNotGranted):
		errutil.Handle(ctx, err, "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultMissingScope)
	case errors.Is(err, usecase.ErrGoogleAccountInUse):
		errutil.Handle(ctx, err, "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultAccountInUse)
	default:
		errutil.Handle(ctx, err, "google workspace connection failed")
		redirectGoogleResult(w, r, googleResultFailed)
	}
}

func (s *Server) googleDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	if err := s.googleUC.Disconnect(ctx, session.Key()); err != nil {
		errutil.Handle(ctx, err, "failed to disconnect google workspace")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, successResponse{Success: true})
}
