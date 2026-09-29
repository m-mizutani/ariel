package http

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/usecase"
	"github.com/m-mizutani/ariel/pkg/utils/errutil"
)

const (
	githubStateCookieName = "ariel_github_oauth_state"
	// githubVerifierCookieName holds the PKCE code verifier. It is set and
	// cleared together with the state cookie, whose format is shared with the
	// other integrations.
	githubVerifierCookieName = "ariel_github_oauth_verifier"
	githubStateCookiePath    = apiV1Path + "/integrations/github"

	// The settings page reads githubResultParam to tell the user how the
	// connection ended. Only these fixed values are ever sent.
	githubResultParam        = "github"
	githubResultConnected    = "connected"
	githubResultAccessDenied = "access_denied"
	githubResultAccountInUse = "account_in_use"
	githubResultFailed       = "failed"

	githubErrorAccessDenied = "access_denied"
)

type githubStatusResponse struct {
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	Login     string `json:"login"`
}

// generateCodeVerifier returns a PKCE code verifier: 32 random bytes in
// base64url without padding, 43 characters as RFC 7636 recommends.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", goerr.Wrap(err, "failed to generate pkce code verifier")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func redirectGitHubResult(w http.ResponseWriter, r *http.Request, result string) {
	http.Redirect(w, r, "/settings?"+githubResultParam+"="+result, http.StatusFound)
}

func (s *Server) githubStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	if s.githubUC == nil {
		writeJSON(ctx, w, http.StatusOK, githubStatusResponse{})
		return
	}

	status, err := s.githubUC.Status(ctx, session.Key())
	if err != nil {
		errutil.Handle(ctx, err, "failed to load github status")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, githubStatusResponse{
		Available: true,
		Connected: status.Connected,
		Login:     status.Login,
	})
}

func (s *Server) githubConnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	state, err := generateState()
	if err != nil {
		errutil.Handle(ctx, err, "failed to start github connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		errutil.Handle(ctx, err, "failed to start github connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	authorizeURL, err := s.githubUC.AuthorizeURL(ctx, session.Key(), state, verifier)
	if err != nil {
		if errors.Is(err, usecase.ErrGitHubAlreadyConnected) {
			errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "github is already connected")
			redirectToSettings(w, r)
			return
		}
		errutil.Handle(ctx, err, "failed to start github connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	s.setCookie(w, githubStateCookieName, oauthStateCookieValue(state, session), githubStateCookiePath, stateCookieMaxAge, time.Time{})
	s.setCookie(w, githubVerifierCookieName, verifier, githubStateCookiePath, stateCookieMaxAge, time.Time{})
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (s *Server) githubCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	// Clearing only sets response headers; the request still carries both
	// cookies. They are single use, whatever the outcome.
	s.clearCookie(w, githubStateCookieName, githubStateCookiePath)
	s.clearCookie(w, githubVerifierCookieName, githubStateCookiePath)

	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	vals := []goerr.Option{goerr.V("team_id", session.TeamID), goerr.V("user_id", session.UserID)}

	if githubErr := q.Get("error"); githubErr != "" {
		if githubErr == githubErrorAccessDenied {
			errutil.Handle(ctx, goerr.New("user declined the github authorization",
				append(vals, goerr.T(errutil.TagBenign))...), "github connection cancelled")
			redirectGitHubResult(w, r, githubResultAccessDenied)
			return
		}
		errutil.Handle(ctx, goerr.New("github returned an authorization error",
			append(vals, goerr.V("github_error", githubErr))...), "github connection failed")
		redirectGitHubResult(w, r, githubResultFailed)
		return
	}

	if err := verifyOAuthState(r, githubStateCookieName, q.Get("state"), session); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "github oauth state check failed", vals...), "github connection failed")
		redirectGitHubResult(w, r, githubResultFailed)
		return
	}
	verifier, err := r.Cookie(githubVerifierCookieName)
	if err != nil || verifier.Value == "" {
		errutil.Handle(ctx, goerr.New("pkce code verifier cookie is missing", vals...), "github connection failed")
		redirectGitHubResult(w, r, githubResultFailed)
		return
	}
	code := q.Get("code")
	if code == "" {
		errutil.Handle(ctx, goerr.New("authorization code is missing", vals...), "github connection failed")
		redirectGitHubResult(w, r, githubResultFailed)
		return
	}

	err = s.githubUC.HandleCallback(ctx, session.Key(), code, verifier.Value)
	switch {
	case err == nil:
		redirectGitHubResult(w, r, githubResultConnected)
	case errors.Is(err, usecase.ErrGitHubAlreadyConnected):
		errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "github is already connected")
		redirectToSettings(w, r)
	case errors.Is(err, usecase.ErrGitHubAccountInUse):
		errutil.Handle(ctx, err, "github connection failed")
		redirectGitHubResult(w, r, githubResultAccountInUse)
	default:
		errutil.Handle(ctx, err, "github connection failed")
		redirectGitHubResult(w, r, githubResultFailed)
	}
}

func (s *Server) githubDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	if err := s.githubUC.Disconnect(ctx, session.Key()); err != nil {
		errutil.Handle(ctx, err, "failed to disconnect github")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, successResponse{Success: true})
}
