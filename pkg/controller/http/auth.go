package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
	"github.com/m-mizutani/ariel/pkg/utils/errutil"
)

const (
	// apiV1Path is the prefix of every API route. Paths outside it are not
	// API routes: pages of the SPA, and /hooks/slack/event called by Slack.
	apiV1Path = "/api/v1"
	authPath  = apiV1Path + "/auth"

	stateCookieName         = "ariel_oauth_state"
	stateCookiePath         = authPath
	stateCookieMaxAge       = 600
	sessionIDCookieName     = "ariel_session_id"
	sessionSecretCookieName = "ariel_session_secret"

	loginErrorAccessDenied = "access_denied"
	loginErrorFailed       = "login_failed"

	// postLoginPath is where a successful sign-in lands. It is fixed, never
	// taken from the request, so the callback cannot redirect off-site.
	postLoginPath = "/settings"
)

type meResponse struct {
	TeamID         string `json:"team_id"`
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	SlackConnected bool   `json:"slack_connected"`
}

type successResponse struct {
	Success bool `json:"success"`
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", goerr.Wrap(err, "failed to generate oauth state")
	}
	return hex.EncodeToString(b), nil
}

// setCookie is the only place that writes a cookie, so every cookie is
// HttpOnly and SameSite=Lax. A negative maxAge deletes the cookie; zero leaves
// it unset and expires, when non-zero, bounds the cookie instead.
func (s *Server) setCookie(w http.ResponseWriter, name, value, path string, maxAge int, expires time.Time) {
	// #nosec G124 -- Secure follows the scheme of --base-url. It is false only
	// for an http base URL, which is meant for local development.
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name, path string) {
	s.setCookie(w, name, "", path, -1, time.Time{})
}

func (s *Server) authLoginHandler(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		errutil.Handle(r.Context(), err, "failed to start login")
		writeError(r.Context(), w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	s.setCookie(w, stateCookieName, state, stateCookiePath, stateCookieMaxAge, time.Time{})

	http.Redirect(w, r, s.authUC.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+code, http.StatusFound)
}

func (s *Server) authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	// The state cookie is single use, whatever the outcome.
	s.clearCookie(w, stateCookieName, stateCookiePath)

	if slackErr := q.Get("error"); slackErr != "" {
		if slackErr == loginErrorAccessDenied {
			errutil.Handle(ctx, goerr.New("user declined the slack authorization", goerr.T(errutil.TagBenign)), "login cancelled")
			s.redirectLoginError(w, r, loginErrorAccessDenied)
			return
		}
		errutil.Handle(ctx, goerr.New("slack returned an authorization error", goerr.V("slack_error", slackErr)), "login failed")
		s.redirectLoginError(w, r, loginErrorFailed)
		return
	}

	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "oauth state cookie is missing"), "login failed")
		s.redirectLoginError(w, r, loginErrorFailed)
		return
	}
	state := q.Get("state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(stateCookie.Value)) != 1 {
		errutil.Handle(ctx, goerr.New("oauth state does not match"), "login failed")
		s.redirectLoginError(w, r, loginErrorFailed)
		return
	}
	code := q.Get("code")
	if code == "" {
		errutil.Handle(ctx, goerr.New("authorization code is missing"), "login failed")
		s.redirectLoginError(w, r, loginErrorFailed)
		return
	}

	session, secret, err := s.authUC.HandleCallback(ctx, code)
	if err != nil {
		errutil.Handle(ctx, err, "login failed")
		s.redirectLoginError(w, r, loginErrorFailed)
		return
	}

	s.setCookie(w, sessionIDCookieName, string(session.ID), "/", 0, session.ExpiresAt)
	s.setCookie(w, sessionSecretCookieName, string(secret), "/", 0, session.ExpiresAt)

	http.Redirect(w, r, postLoginPath, http.StatusFound)
}

func (s *Server) authLogoutHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.clearCookie(w, sessionIDCookieName, "/")
	s.clearCookie(w, sessionSecretCookieName, "/")

	idCookie, idErr := r.Cookie(sessionIDCookieName)
	secretCookie, secretErr := r.Cookie(sessionSecretCookieName)
	if idErr == nil && secretErr == nil {
		if err := s.authUC.Logout(ctx, auth.SessionID(idCookie.Value), auth.SessionSecret(secretCookie.Value)); err != nil {
			errutil.Handle(ctx, err, "failed to logout")
			writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
			return
		}
	}

	writeJSON(ctx, w, http.StatusOK, successResponse{Success: true})
}

func (s *Server) authMeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := auth.SessionFromContext(ctx)
	if !ok {
		errutil.Handle(ctx, goerr.New("session is not in the request context"), "failed to load current user")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	me, err := s.authUC.Me(ctx, session.Key())
	if err != nil {
		errutil.Handle(ctx, err, "failed to load current user")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	writeJSON(ctx, w, http.StatusOK, meResponse{
		TeamID:         string(me.TeamID),
		UserID:         string(me.UserID),
		Name:           me.Name,
		SlackConnected: me.SlackConnected,
	})
}
