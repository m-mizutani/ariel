package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	httpctrl "github.com/m-mizutani/ariel/pkg/controller/http"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/usecase"
)

type notionCallback struct {
	Key  model.UserKey
	Code string
}

type fakeNotionUseCase struct {
	mu            sync.Mutex
	states        []string
	authorizeKeys []model.UserKey
	authorizeErr  error
	callbacks     []notionCallback
	callbackErr   error
	status        *usecase.NotionStatus
	statusErr     error
	statusKeys    []model.UserKey
	disconnects   []model.UserKey
	disconnectErr error
}

func newFakeNotionUseCase() *fakeNotionUseCase {
	return &fakeNotionUseCase{
		status: &usecase.NotionStatus{Connected: true, UserName: "Alice Example", WorkspaceName: "Example"},
	}
}

func (f *fakeNotionUseCase) AuthorizeURL(_ context.Context, key model.UserKey, state string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorizeKeys = append(f.authorizeKeys, key)
	if f.authorizeErr != nil {
		return "", f.authorizeErr
	}
	f.states = append(f.states, state)
	return "https://api.notion.com/v1/oauth/authorize?client_id=x&state=" + state, nil
}

func (f *fakeNotionUseCase) HandleCallback(_ context.Context, key model.UserKey, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = append(f.callbacks, notionCallback{Key: key, Code: code})
	return f.callbackErr
}

func (f *fakeNotionUseCase) Status(_ context.Context, key model.UserKey) (*usecase.NotionStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusKeys = append(f.statusKeys, key)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeNotionUseCase) Disconnect(_ context.Context, key model.UserKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects = append(f.disconnects, key)
	return f.disconnectErr
}

const notionBase = "/api/v1/integrations/notion"

func newNotionTestServer(t *testing.T, authUC *fakeAuthUseCase, notionUC *fakeNotionUseCase) *httpctrl.Server {
	t.Helper()
	opts := []httpctrl.Option{httpctrl.WithSlackEvents(&fakeSlackEventUseCase{}, testSigningSecret)}
	if notionUC != nil {
		opts = append(opts, httpctrl.WithNotion(notionUC))
	}
	srv, err := httpctrl.New(authUC, httpctrl.Config{BaseURL: "https://ariel.example.com", Static: testStatic}, opts...)
	gt.NoError(t, err).Required()
	return srv
}

func TestNotionStatus(t *testing.T) {
	t.Run("integration not configured", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newNotionTestServer(t, authUC, nil)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": false, "connected": false, "needs_reconnect": false, "user_name": "", "workspace_name": "",
		})
	})

	t.Run("connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": true, "connected": true, "needs_reconnect": false, "user_name": "Alice Example", "workspace_name": "Example",
		})
		gt.Value(t, notionUC.statusKeys).Equal([]model.UserKey{sessionKey})
	})

	t.Run("needs reconnect", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		notionUC.status = &usecase.NotionStatus{Connected: true, NeedsReconnect: true, UserName: "Alice Example", WorkspaceName: "Example"}
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase, nil), authUC))
		gt.Value(t, decodeJSON(t, resp.Body)["needs_reconnect"]).Equal(true)
	})

	t.Run("without session", func(t *testing.T) {
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, newFakeAuthUseCase(), notionUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, notionBase, nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "unauthenticated"})
		gt.Array(t, notionUC.statusKeys).Length(0)
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		notionUC.statusErr = errors.New("firestore unavailable")
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})
}

func TestNotionEndpointsWithoutIntegration(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newNotionTestServer(t, authUC, nil)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, notionBase + "/connect"},
		{http.MethodGet, notionBase + "/callback?code=c&state=s"},
		{http.MethodPost, notionBase + "/disconnect"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := serve(srv, withSession(httptest.NewRequest(tc.method, tc.path, nil), authUC))
			gt.Number(t, resp.StatusCode).Equal(http.StatusNotFound)
			gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "not_found"})
		})
	}
}

func TestNotionConnect(t *testing.T) {
	t.Run("with session", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.Array(t, notionUC.states).Length(1).Required()
		state := notionUC.states[0]
		gt.String(t, state).NotEqual("")
		gt.String(t, resp.Header.Get("Location")).Equal("https://api.notion.com/v1/oauth/authorize?client_id=x&state=" + state)

		cookie := findCookie(resp, "ariel_notion_oauth_state")
		gt.Value(t, cookie).NotNil().Required()
		gt.String(t, cookie.Value).Equal(state + "." + string(authUC.session.ID))
		gt.String(t, cookie.Path).Equal("/api/v1/integrations/notion")
		gt.Number(t, cookie.MaxAge).Equal(600)
		gt.Bool(t, cookie.HttpOnly).True()
		gt.Bool(t, cookie.Secure).True()
		gt.Value(t, cookie.SameSite).Equal(http.SameSiteLaxMode)
		gt.Value(t, notionUC.authorizeKeys).Equal([]model.UserKey{sessionKey})
	})

	t.Run("already connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		notionUC.authorizeErr = goerr.Wrap(usecase.ErrNotionAlreadyConnected, "connected")
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/settings")
		gt.Value(t, findCookie(resp, "ariel_notion_oauth_state")).Nil()
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		notionUC.authorizeErr = errors.New("firestore unavailable")
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
		gt.Value(t, findCookie(resp, "ariel_notion_oauth_state")).Nil()
	})

	t.Run("without session", func(t *testing.T) {
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, newFakeAuthUseCase(), notionUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, notionBase+"/connect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/login")
		gt.Value(t, findCookie(resp, "ariel_notion_oauth_state")).Nil()
		gt.Array(t, notionUC.states).Length(0)
	})
}

func notionCallbackRequest(authUC *fakeAuthUseCase, query, stateCookie string) *http.Request {
	r := withSession(httptest.NewRequest(http.MethodGet, notionBase+"/callback?"+query, nil), authUC)
	if stateCookie != "" {
		r.AddCookie(&http.Cookie{Name: "ariel_notion_oauth_state", Value: stateCookie})
	}
	return r
}

func TestNotionCallback_Success(t *testing.T) {
	authUC := newFakeAuthUseCase()
	notionUC := newFakeNotionUseCase()
	srv := newNotionTestServer(t, authUC, notionUC)

	resp := serve(srv, notionCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID)))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings?notion=connected")
	gt.Value(t, notionUC.callbacks).Equal([]notionCallback{{Key: sessionKey, Code: "c1"}})

	state := findCookie(resp, "ariel_notion_oauth_state")
	gt.Value(t, state).NotNil().Required()
	gt.Bool(t, state.MaxAge < 0).True()
	gt.String(t, state.Path).Equal("/api/v1/integrations/notion")
}

func TestNotionCallback_RedirectIgnoresRequest(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newNotionTestServer(t, authUC, newFakeNotionUseCase())

	r := notionCallbackRequest(authUC, "code=c1&state=s1&next=https%3A%2F%2Fevil.example%2F", "s1."+string(authUC.session.ID))
	r.Header.Set("Referer", "https://evil.example/")
	resp := serve(srv, r)
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings?notion=connected")
}

func TestNotionCallback_Failures(t *testing.T) {
	cases := map[string]struct {
		query       string
		stateCookie func(sessionID string) string
		callbackErr error
		wantResult  string
		wantCalled  bool
	}{
		"user declined": {
			query:       "error=access_denied&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			wantResult:  "access_denied",
		},
		"other notion error": {
			query:       "error=server_error&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			wantResult:  "failed",
		},
		"no state cookie": {
			query:      "code=c&state=s1",
			wantResult: "failed",
		},
		"state mismatch": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s2." + id },
			wantResult:  "failed",
		},
		"empty state": {
			query:       "code=c&state=",
			stateCookie: func(id string) string { return "." + id },
			wantResult:  "failed",
		},
		"started by another session": {
			query:       "code=c&state=s1",
			stateCookie: func(string) string { return "s1.0190c6a4-0000-7000-8000-000000000000" },
			wantResult:  "failed",
		},
		"malformed state cookie": {
			query:       "code=c&state=s1",
			stateCookie: func(string) string { return "s1" },
			wantResult:  "failed",
		},
		"no code": {
			query:       "state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			wantResult:  "failed",
		},
		"another workspace": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrNotionWrongWorkspace, "other workspace"),
			wantResult:  "wrong_workspace",
			wantCalled:  true,
		},
		"notion account connected to another user": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrNotionAccountInUse, "in use"),
			wantResult:  "account_in_use",
			wantCalled:  true,
		},
		"connection rejected": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrNotionConnectRejected, "no refresh token"),
			wantResult:  "failed",
			wantCalled:  true,
		},
		"usecase error": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: errors.New("boom"),
			wantResult:  "failed",
			wantCalled:  true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			authUC := newFakeAuthUseCase()
			notionUC := newFakeNotionUseCase()
			notionUC.callbackErr = tc.callbackErr
			srv := newNotionTestServer(t, authUC, notionUC)

			cookie := ""
			if tc.stateCookie != nil {
				cookie = tc.stateCookie(string(authUC.session.ID))
			}
			resp := serve(srv, notionCallbackRequest(authUC, tc.query, cookie))
			gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
			gt.String(t, resp.Header.Get("Location")).Equal("/settings?notion=" + tc.wantResult)
			gt.Value(t, len(notionUC.callbacks) > 0).Equal(tc.wantCalled)

			state := findCookie(resp, "ariel_notion_oauth_state")
			gt.Value(t, state).NotNil().Required()
			gt.Bool(t, state.MaxAge < 0).True()
		})
	}
}

func TestNotionCallback_AlreadyConnected(t *testing.T) {
	authUC := newFakeAuthUseCase()
	notionUC := newFakeNotionUseCase()
	notionUC.callbackErr = goerr.Wrap(usecase.ErrNotionAlreadyConnected, "connected")
	srv := newNotionTestServer(t, authUC, notionUC)

	resp := serve(srv, notionCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID)))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings")
}

func TestNotionCallback_WithoutSession(t *testing.T) {
	notionUC := newFakeNotionUseCase()
	srv := newNotionTestServer(t, newFakeAuthUseCase(), notionUC)

	r := httptest.NewRequest(http.MethodGet, notionBase+"/callback?code=c&state=s1", nil)
	r.AddCookie(&http.Cookie{Name: "ariel_notion_oauth_state", Value: "s1.x"})
	resp := serve(srv, r)
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/login")
	gt.Array(t, notionUC.callbacks).Length(0)
}

func TestNotionDisconnect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, notionBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"success": true})
		gt.Value(t, notionUC.disconnects).Equal([]model.UserKey{sessionKey})
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		notionUC.disconnectErr = errors.New("notion unavailable")
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, notionBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})

	t.Run("without session", func(t *testing.T) {
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, newFakeAuthUseCase(), notionUC)

		resp := serve(srv, httptest.NewRequest(http.MethodPost, notionBase+"/disconnect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Array(t, notionUC.disconnects).Length(0)
	})

	t.Run("GET is not allowed", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		notionUC := newFakeNotionUseCase()
		srv := newNotionTestServer(t, authUC, notionUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, notionBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusMethodNotAllowed)
		gt.Array(t, notionUC.disconnects).Length(0)
	})
}
