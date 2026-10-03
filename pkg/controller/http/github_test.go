package http_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	httpctrl "github.com/m-mizutani/robin/pkg/controller/http"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/usecase"
)

type githubAuthorize struct {
	Key          model.UserKey
	State        string
	CodeVerifier string
}

type githubCallback struct {
	Key          model.UserKey
	Code         string
	CodeVerifier string
}

type fakeGitHubUseCase struct {
	mu            sync.Mutex
	authorizes    []githubAuthorize
	authorizeErr  error
	callbacks     []githubCallback
	callbackErr   error
	status        *usecase.GitHubStatus
	statusErr     error
	statusKeys    []model.UserKey
	disconnects   []model.UserKey
	disconnectErr error
}

func newFakeGitHubUseCase() *fakeGitHubUseCase {
	return &fakeGitHubUseCase{status: &usecase.GitHubStatus{Connected: true, Login: "octocat"}}
}

func (f *fakeGitHubUseCase) AuthorizeURL(_ context.Context, key model.UserKey, state, codeVerifier string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorizes = append(f.authorizes, githubAuthorize{Key: key, State: state, CodeVerifier: codeVerifier})
	if f.authorizeErr != nil {
		return "", f.authorizeErr
	}
	return "https://github.com/login/oauth/authorize?client_id=x&state=" + state, nil
}

func (f *fakeGitHubUseCase) HandleCallback(_ context.Context, key model.UserKey, code, codeVerifier string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = append(f.callbacks, githubCallback{Key: key, Code: code, CodeVerifier: codeVerifier})
	return f.callbackErr
}

func (f *fakeGitHubUseCase) Status(_ context.Context, key model.UserKey) (*usecase.GitHubStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusKeys = append(f.statusKeys, key)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeGitHubUseCase) Disconnect(_ context.Context, key model.UserKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects = append(f.disconnects, key)
	return f.disconnectErr
}

const githubBase = "/api/v1/integrations/github"

func newGitHubTestServer(t *testing.T, authUC *fakeAuthUseCase, githubUC *fakeGitHubUseCase) *httpctrl.Server {
	t.Helper()
	opts := []httpctrl.Option{httpctrl.WithSlackEvents(&fakeSlackEventUseCase{}, testSigningSecret)}
	if githubUC != nil {
		opts = append(opts, httpctrl.WithGitHub(githubUC))
	}
	srv, err := httpctrl.New(authUC, httpctrl.Config{BaseURL: "https://robin.example.com", Static: testStatic}, opts...)
	gt.NoError(t, err).Required()
	return srv
}

func TestGitHubStatus(t *testing.T) {
	t.Run("integration not configured", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newGitHubTestServer(t, authUC, nil)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": false, "connected": false, "login": "",
		})
	})

	t.Run("connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": true, "connected": true, "login": "octocat",
		})
		gt.Value(t, githubUC.statusKeys).Equal([]model.UserKey{sessionKey})
	})

	t.Run("not connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		githubUC.status = &usecase.GitHubStatus{}
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": true, "connected": false, "login": "",
		})
	})

	t.Run("without session", func(t *testing.T) {
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, newFakeAuthUseCase(), githubUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, githubBase, nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "unauthenticated"})
		gt.Array(t, githubUC.statusKeys).Length(0)
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		githubUC.statusErr = errors.New("firestore unavailable")
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})
}

func TestGitHubEndpointsWithoutIntegration(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newGitHubTestServer(t, authUC, nil)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, githubBase + "/connect"},
		{http.MethodGet, githubBase + "/callback?code=c&state=s"},
		{http.MethodPost, githubBase + "/disconnect"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := serve(srv, withSession(httptest.NewRequest(tc.method, tc.path, nil), authUC))
			gt.Number(t, resp.StatusCode).Equal(http.StatusNotFound)
			gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "not_found"})
		})
	}
}

func assertGitHubCookie(t *testing.T, cookie *http.Cookie) {
	t.Helper()
	gt.Value(t, cookie).NotNil().Required()
	gt.String(t, cookie.Path).Equal("/api/v1/integrations/github")
	gt.Number(t, cookie.MaxAge).Equal(600)
	gt.Bool(t, cookie.HttpOnly).True()
	gt.Bool(t, cookie.Secure).True()
	gt.Value(t, cookie.SameSite).Equal(http.SameSiteLaxMode)
}

func TestGitHubConnect(t *testing.T) {
	t.Run("with session", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.Array(t, githubUC.authorizes).Length(1).Required()
		call := githubUC.authorizes[0]
		gt.Value(t, call.Key).Equal(sessionKey)
		gt.String(t, call.State).NotEqual("")
		gt.String(t, resp.Header.Get("Location")).Equal("https://github.com/login/oauth/authorize?client_id=x&state=" + call.State)

		verifier, err := base64.RawURLEncoding.DecodeString(call.CodeVerifier)
		gt.NoError(t, err)
		gt.Number(t, len(call.CodeVerifier)).Equal(43)
		gt.Number(t, len(verifier)).Equal(32)

		state := findCookie(resp, "robin_github_oauth_state")
		assertGitHubCookie(t, state)
		gt.String(t, state.Value).Equal(call.State + "." + string(authUC.session.ID))
		verifierCookie := findCookie(resp, "robin_github_oauth_verifier")
		assertGitHubCookie(t, verifierCookie)
		gt.String(t, verifierCookie.Value).Equal(call.CodeVerifier)
	})

	t.Run("already connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		githubUC.authorizeErr = goerr.Wrap(usecase.ErrGitHubAlreadyConnected, "connected")
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/settings")
		gt.Value(t, findCookie(resp, "robin_github_oauth_state")).Nil()
		gt.Value(t, findCookie(resp, "robin_github_oauth_verifier")).Nil()
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		githubUC.authorizeErr = errors.New("firestore unavailable")
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
		gt.Value(t, findCookie(resp, "robin_github_oauth_state")).Nil()
	})

	t.Run("without session", func(t *testing.T) {
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, newFakeAuthUseCase(), githubUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, githubBase+"/connect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/login")
		gt.Value(t, findCookie(resp, "robin_github_oauth_state")).Nil()
		gt.Array(t, githubUC.authorizes).Length(0)
	})
}

func githubCallbackRequest(authUC *fakeAuthUseCase, query, stateCookie, verifierCookie string) *http.Request {
	r := withSession(httptest.NewRequest(http.MethodGet, githubBase+"/callback?"+query, nil), authUC)
	if stateCookie != "" {
		r.AddCookie(&http.Cookie{Name: "robin_github_oauth_state", Value: stateCookie})
	}
	if verifierCookie != "" {
		r.AddCookie(&http.Cookie{Name: "robin_github_oauth_verifier", Value: verifierCookie})
	}
	return r
}

func assertGitHubCookiesCleared(t *testing.T, resp *http.Response) {
	t.Helper()
	for _, name := range []string{"robin_github_oauth_state", "robin_github_oauth_verifier"} {
		cookie := findCookie(resp, name)
		gt.Value(t, cookie).NotNil().Required()
		gt.Bool(t, cookie.MaxAge < 0).True()
		gt.String(t, cookie.Path).Equal("/api/v1/integrations/github")
	}
}

func TestGitHubCallback_Success(t *testing.T) {
	authUC := newFakeAuthUseCase()
	githubUC := newFakeGitHubUseCase()
	srv := newGitHubTestServer(t, authUC, githubUC)

	resp := serve(srv, githubCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID), "verifier-1"))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings?github=connected")
	gt.Value(t, githubUC.callbacks).Equal([]githubCallback{{Key: sessionKey, Code: "c1", CodeVerifier: "verifier-1"}})
	assertGitHubCookiesCleared(t, resp)
}

func TestGitHubCallback_Failures(t *testing.T) {
	cases := map[string]struct {
		query       string
		stateCookie func(sessionID string) string
		verifier    string
		callbackErr error
		wantResult  string
		wantCalled  bool
	}{
		"user declined": {
			query:       "error=access_denied&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			wantResult:  "access_denied",
		},
		"other github error": {
			query:       "error=application_suspended&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			wantResult:  "failed",
		},
		"no state cookie": {
			query:      "code=c&state=s1",
			verifier:   "v",
			wantResult: "failed",
		},
		"malformed state cookie": {
			query:       "code=c&state=s1",
			stateCookie: func(string) string { return "s1" },
			verifier:    "v",
			wantResult:  "failed",
		},
		"state mismatch": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s2." + id },
			verifier:    "v",
			wantResult:  "failed",
		},
		"started by another session": {
			query:       "code=c&state=s1",
			stateCookie: func(string) string { return "s1.0190c6a4-0000-7000-8000-000000000000" },
			verifier:    "v",
			wantResult:  "failed",
		},
		"no verifier cookie": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			wantResult:  "failed",
		},
		"no code": {
			query:       "state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			wantResult:  "failed",
		},
		"github account connected to another user": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			callbackErr: goerr.Wrap(usecase.ErrGitHubAccountInUse, "in use"),
			wantResult:  "account_in_use",
			wantCalled:  true,
		},
		"connection rejected": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			callbackErr: goerr.Wrap(usecase.ErrGitHubConnectRejected, "bad token"),
			wantResult:  "failed",
			wantCalled:  true,
		},
		"usecase error": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			verifier:    "v",
			callbackErr: errors.New("boom"),
			wantResult:  "failed",
			wantCalled:  true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			authUC := newFakeAuthUseCase()
			githubUC := newFakeGitHubUseCase()
			githubUC.callbackErr = tc.callbackErr
			srv := newGitHubTestServer(t, authUC, githubUC)

			cookie := ""
			if tc.stateCookie != nil {
				cookie = tc.stateCookie(string(authUC.session.ID))
			}
			resp := serve(srv, githubCallbackRequest(authUC, tc.query, cookie, tc.verifier))
			gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
			gt.String(t, resp.Header.Get("Location")).Equal("/settings?github=" + tc.wantResult)
			gt.Value(t, len(githubUC.callbacks) > 0).Equal(tc.wantCalled)
			assertGitHubCookiesCleared(t, resp)
		})
	}
}

func TestGitHubCallback_AlreadyConnected(t *testing.T) {
	authUC := newFakeAuthUseCase()
	githubUC := newFakeGitHubUseCase()
	githubUC.callbackErr = goerr.Wrap(usecase.ErrGitHubAlreadyConnected, "connected")
	srv := newGitHubTestServer(t, authUC, githubUC)

	resp := serve(srv, githubCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID), "v"))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings")
}

func TestGitHubCallback_WithoutSession(t *testing.T) {
	githubUC := newFakeGitHubUseCase()
	srv := newGitHubTestServer(t, newFakeAuthUseCase(), githubUC)

	r := httptest.NewRequest(http.MethodGet, githubBase+"/callback?code=c&state=s1", nil)
	r.AddCookie(&http.Cookie{Name: "robin_github_oauth_state", Value: "s1.x"})
	resp := serve(srv, r)
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/login")
	gt.Array(t, githubUC.callbacks).Length(0)
}

func TestGitHubDisconnect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, githubBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"success": true})
		gt.Value(t, githubUC.disconnects).Equal([]model.UserKey{sessionKey})
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		githubUC.disconnectErr = errors.New("github unavailable")
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, githubBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})

	t.Run("without session", func(t *testing.T) {
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, newFakeAuthUseCase(), githubUC)

		resp := serve(srv, httptest.NewRequest(http.MethodPost, githubBase+"/disconnect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Array(t, githubUC.disconnects).Length(0)
	})

	t.Run("GET is not allowed", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		githubUC := newFakeGitHubUseCase()
		srv := newGitHubTestServer(t, authUC, githubUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, githubBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusMethodNotAllowed)
		gt.Array(t, githubUC.disconnects).Length(0)
	})
}
