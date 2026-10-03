package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/usecase"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const (
	notionStateCookieName = "robin_notion_oauth_state"
	notionStateCookiePath = apiV1Path + "/integrations/notion"

	// The settings page reads notionResultParam to tell the user how the
	// connection ended. Only these fixed values are ever sent.
	notionResultParam          = "notion"
	notionResultConnected      = "connected"
	notionResultAccessDenied   = "access_denied"
	notionResultWrongWorkspace = "wrong_workspace"
	notionResultAccountInUse   = "account_in_use"
	notionResultFailed         = "failed"

	notionErrorAccessDenied = "access_denied"
)

type notionStatusResponse struct {
	Available      bool   `json:"available"`
	Connected      bool   `json:"connected"`
	NeedsReconnect bool   `json:"needs_reconnect"`
	UserName       string `json:"user_name"`
	WorkspaceName  string `json:"workspace_name"`
}

func redirectNotionResult(w http.ResponseWriter, r *http.Request, result string) {
	http.Redirect(w, r, "/settings?"+notionResultParam+"="+result, http.StatusFound)
}

func (s *Server) notionStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	if s.notionUC == nil {
		writeJSON(ctx, w, http.StatusOK, notionStatusResponse{})
		return
	}

	status, err := s.notionUC.Status(ctx, session.Key())
	if err != nil {
		errutil.Handle(ctx, err, "failed to load notion status")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, notionStatusResponse{
		Available:      true,
		Connected:      status.Connected,
		NeedsReconnect: status.NeedsReconnect,
		UserName:       status.UserName,
		WorkspaceName:  status.WorkspaceName,
	})
}

func (s *Server) notionConnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	state, err := generateState()
	if err != nil {
		errutil.Handle(ctx, err, "failed to start notion connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	authorizeURL, err := s.notionUC.AuthorizeURL(ctx, session.Key(), state)
	if err != nil {
		if errors.Is(err, usecase.ErrNotionAlreadyConnected) {
			errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "notion is already connected")
			redirectToSettings(w, r)
			return
		}
		errutil.Handle(ctx, err, "failed to start notion connection")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}

	s.setCookie(w, notionStateCookieName, oauthStateCookieValue(state, session), notionStateCookiePath, stateCookieMaxAge, time.Time{})
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (s *Server) notionCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	// The state cookie is single use, whatever the outcome.
	s.clearCookie(w, notionStateCookieName, notionStateCookiePath)

	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	vals := []goerr.Option{goerr.V("team_id", session.TeamID), goerr.V("user_id", session.UserID)}

	if notionErr := q.Get("error"); notionErr != "" {
		if notionErr == notionErrorAccessDenied {
			errutil.Handle(ctx, goerr.New("user declined the notion authorization",
				append(vals, goerr.T(errutil.TagBenign))...), "notion connection cancelled")
			redirectNotionResult(w, r, notionResultAccessDenied)
			return
		}
		errutil.Handle(ctx, goerr.New("notion returned an authorization error",
			append(vals, goerr.V("notion_error", notionErr))...), "notion connection failed")
		redirectNotionResult(w, r, notionResultFailed)
		return
	}

	if err := verifyOAuthState(r, notionStateCookieName, q.Get("state"), session); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "notion oauth state check failed", vals...), "notion connection failed")
		redirectNotionResult(w, r, notionResultFailed)
		return
	}
	code := q.Get("code")
	if code == "" {
		errutil.Handle(ctx, goerr.New("authorization code is missing", vals...), "notion connection failed")
		redirectNotionResult(w, r, notionResultFailed)
		return
	}

	err := s.notionUC.HandleCallback(ctx, session.Key(), code)
	switch {
	case err == nil:
		redirectNotionResult(w, r, notionResultConnected)
	case errors.Is(err, usecase.ErrNotionAlreadyConnected):
		errutil.Handle(ctx, goerr.Wrap(err, "connection ignored", goerr.T(errutil.TagBenign)), "notion is already connected")
		redirectToSettings(w, r)
	case errors.Is(err, usecase.ErrNotionWrongWorkspace):
		errutil.Handle(ctx, err, "notion connection failed")
		redirectNotionResult(w, r, notionResultWrongWorkspace)
	case errors.Is(err, usecase.ErrNotionAccountInUse):
		errutil.Handle(ctx, err, "notion connection failed")
		redirectNotionResult(w, r, notionResultAccountInUse)
	default:
		errutil.Handle(ctx, err, "notion connection failed")
		redirectNotionResult(w, r, notionResultFailed)
	}
}

func (s *Server) notionDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}

	if err := s.notionUC.Disconnect(ctx, session.Key()); err != nil {
		errutil.Handle(ctx, err, "failed to disconnect notion")
		writeError(ctx, w, http.StatusInternalServerError, errCodeInternal)
		return
	}
	writeJSON(ctx, w, http.StatusOK, successResponse{Success: true})
}
