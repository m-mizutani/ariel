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

type googleCallback struct {
	Key  model.UserKey
	Code string
}

type fakeGoogleWorkspaceUseCase struct {
	mu            sync.Mutex
	states        []string
	authorizeKeys []model.UserKey
	authorizeErr  error
	callbacks     []googleCallback
	callbackErr   error
	status        *usecase.GoogleWorkspaceStatus
	statusErr     error
	statusKeys    []model.UserKey
	disconnects   []model.UserKey
	disconnectErr error
}

func newFakeGoogleWorkspaceUseCase() *fakeGoogleWorkspaceUseCase {
	return &fakeGoogleWorkspaceUseCase{
		status: &usecase.GoogleWorkspaceStatus{Connected: true, Email: "alice@example.com"},
	}
}

func (f *fakeGoogleWorkspaceUseCase) AuthorizeURL(_ context.Context, key model.UserKey, state string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorizeKeys = append(f.authorizeKeys, key)
	if f.authorizeErr != nil {
		return "", f.authorizeErr
	}
	f.states = append(f.states, state)
	return "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=" + state, nil
}

func (f *fakeGoogleWorkspaceUseCase) HandleCallback(_ context.Context, key model.UserKey, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callbacks = append(f.callbacks, googleCallback{Key: key, Code: code})
	return f.callbackErr
}

func (f *fakeGoogleWorkspaceUseCase) Status(_ context.Context, key model.UserKey) (*usecase.GoogleWorkspaceStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusKeys = append(f.statusKeys, key)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeGoogleWorkspaceUseCase) Disconnect(_ context.Context, key model.UserKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects = append(f.disconnects, key)
	return f.disconnectErr
}

const googleBase = "/api/integrations/google-workspace"

var sessionKey = model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"}

func newGoogleTestServer(t *testing.T, authUC *fakeAuthUseCase, googleUC *fakeGoogleWorkspaceUseCase) *httpctrl.Server {
	t.Helper()
	opts := []httpctrl.Option{httpctrl.WithSlackEvents(&fakeSlackEventUseCase{}, testSigningSecret)}
	if googleUC != nil {
		opts = append(opts, httpctrl.WithGoogleWorkspace(googleUC))
	}
	srv, err := httpctrl.New(authUC, httpctrl.Config{BaseURL: "https://ariel.example.com", Static: testStatic}, opts...)
	gt.NoError(t, err).Required()
	return srv
}

func serve(srv *httpctrl.Server, r *http.Request) *http.Response {
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w.Result()
}

func TestGoogleStatus(t *testing.T) {
	t.Run("integration not configured", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newGoogleTestServer(t, authUC, nil)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": false, "connected": false, "email": "",
		})
	})

	t.Run("connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": true, "connected": true, "email": "alice@example.com",
		})
		gt.Value(t, googleUC.statusKeys).Equal([]model.UserKey{sessionKey})
	})

	t.Run("not connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		googleUC.status = &usecase.GoogleWorkspaceStatus{}
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{
			"available": true, "connected": false, "email": "",
		})
	})

	t.Run("without session", func(t *testing.T) {
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, newFakeAuthUseCase(), googleUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, googleBase, nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "unauthenticated"})
		gt.Array(t, googleUC.statusKeys).Length(0)
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		googleUC.statusErr = errors.New("firestore unavailable")
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase, nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})
}

func TestGoogleEndpointsWithoutIntegration(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newGoogleTestServer(t, authUC, nil)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, googleBase + "/connect"},
		{http.MethodGet, googleBase + "/callback?code=c&state=s"},
		{http.MethodPost, googleBase + "/disconnect"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := serve(srv, withSession(httptest.NewRequest(tc.method, tc.path, nil), authUC))
			gt.Number(t, resp.StatusCode).Equal(http.StatusNotFound)
			gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "not_found"})
		})
	}
}

func TestGoogleConnect(t *testing.T) {
	t.Run("with session", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.Array(t, googleUC.states).Length(1).Required()
		state := googleUC.states[0]
		gt.String(t, state).NotEqual("")
		gt.String(t, resp.Header.Get("Location")).Equal("https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=" + state)

		cookie := findCookie(resp, "ariel_google_oauth_state")
		gt.Value(t, cookie).NotNil().Required()
		gt.String(t, cookie.Value).Equal(state + "." + string(authUC.session.ID))
		gt.String(t, cookie.Path).Equal("/api/integrations/google-workspace")
		gt.Number(t, cookie.MaxAge).Equal(600)
		gt.Bool(t, cookie.HttpOnly).True()
		gt.Bool(t, cookie.Secure).True()
		gt.Value(t, cookie.SameSite).Equal(http.SameSiteLaxMode)
		gt.Value(t, googleUC.authorizeKeys).Equal([]model.UserKey{sessionKey})
	})

	t.Run("already connected", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		googleUC.authorizeErr = goerr.Wrap(usecase.ErrGoogleWorkspaceAlreadyConnected, "connected")
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/settings")
		gt.Value(t, findCookie(resp, "ariel_google_oauth_state")).Nil()
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		googleUC.authorizeErr = errors.New("firestore unavailable")
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase+"/connect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
		gt.Value(t, findCookie(resp, "ariel_google_oauth_state")).Nil()
	})

	t.Run("without session", func(t *testing.T) {
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, newFakeAuthUseCase(), googleUC)

		resp := serve(srv, httptest.NewRequest(http.MethodGet, googleBase+"/connect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
		gt.String(t, resp.Header.Get("Location")).Equal("/login")
		gt.Value(t, findCookie(resp, "ariel_google_oauth_state")).Nil()
		gt.Array(t, googleUC.states).Length(0)
	})
}

func googleCallbackRequest(authUC *fakeAuthUseCase, query, stateCookie string) *http.Request {
	r := withSession(httptest.NewRequest(http.MethodGet, googleBase+"/callback?"+query, nil), authUC)
	if stateCookie != "" {
		r.AddCookie(&http.Cookie{Name: "ariel_google_oauth_state", Value: stateCookie})
	}
	return r
}

func TestGoogleCallback_Success(t *testing.T) {
	authUC := newFakeAuthUseCase()
	googleUC := newFakeGoogleWorkspaceUseCase()
	srv := newGoogleTestServer(t, authUC, googleUC)

	resp := serve(srv, googleCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID)))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings?google_workspace=connected")
	gt.Value(t, googleUC.callbacks).Equal([]googleCallback{{Key: sessionKey, Code: "c1"}})

	state := findCookie(resp, "ariel_google_oauth_state")
	gt.Value(t, state).NotNil().Required()
	gt.Bool(t, state.MaxAge < 0).True()
	gt.String(t, state.Path).Equal("/api/integrations/google-workspace")
}

func TestGoogleCallback_RedirectIgnoresRequest(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newGoogleTestServer(t, authUC, newFakeGoogleWorkspaceUseCase())

	r := googleCallbackRequest(authUC, "code=c1&state=s1&next=https%3A%2F%2Fevil.example%2F", "s1."+string(authUC.session.ID))
	r.Header.Set("Referer", "https://evil.example/")
	resp := serve(srv, r)
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings?google_workspace=connected")
}

func TestGoogleCallback_Failures(t *testing.T) {
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
		"other google error": {
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
		"scope not granted": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrGoogleScopeNotGranted, "missing gmail"),
			wantResult:  "missing_scope",
			wantCalled:  true,
		},
		"google account connected to another user": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrGoogleAccountInUse, "in use"),
			wantResult:  "account_in_use",
			wantCalled:  true,
		},
		"connection rejected": {
			query:       "code=c&state=s1",
			stateCookie: func(id string) string { return "s1." + id },
			callbackErr: goerr.Wrap(usecase.ErrGoogleConnectRejected, "no refresh token"),
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
			googleUC := newFakeGoogleWorkspaceUseCase()
			googleUC.callbackErr = tc.callbackErr
			srv := newGoogleTestServer(t, authUC, googleUC)

			cookie := ""
			if tc.stateCookie != nil {
				cookie = tc.stateCookie(string(authUC.session.ID))
			}
			resp := serve(srv, googleCallbackRequest(authUC, tc.query, cookie))
			gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
			gt.String(t, resp.Header.Get("Location")).Equal("/settings?google_workspace=" + tc.wantResult)
			gt.Value(t, len(googleUC.callbacks) > 0).Equal(tc.wantCalled)

			state := findCookie(resp, "ariel_google_oauth_state")
			gt.Value(t, state).NotNil().Required()
			gt.Bool(t, state.MaxAge < 0).True()
		})
	}
}

// A user who is already connected returns to the settings page without a
// result, and nothing changes.
func TestGoogleCallback_AlreadyConnected(t *testing.T) {
	authUC := newFakeAuthUseCase()
	googleUC := newFakeGoogleWorkspaceUseCase()
	googleUC.callbackErr = goerr.Wrap(usecase.ErrGoogleWorkspaceAlreadyConnected, "connected")
	srv := newGoogleTestServer(t, authUC, googleUC)

	resp := serve(srv, googleCallbackRequest(authUC, "code=c1&state=s1", "s1."+string(authUC.session.ID)))
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/settings")
	gt.Value(t, googleUC.callbacks).Equal([]googleCallback{{Key: sessionKey, Code: "c1"}})
}

func TestGoogleCallback_WithoutSession(t *testing.T) {
	googleUC := newFakeGoogleWorkspaceUseCase()
	srv := newGoogleTestServer(t, newFakeAuthUseCase(), googleUC)

	r := httptest.NewRequest(http.MethodGet, googleBase+"/callback?code=c&state=s1", nil)
	r.AddCookie(&http.Cookie{Name: "ariel_google_oauth_state", Value: "s1.x"})
	resp := serve(srv, r)
	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/login")
	gt.Array(t, googleUC.callbacks).Length(0)
}

func TestGoogleDisconnect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, googleBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"success": true})
		gt.Value(t, googleUC.disconnects).Equal([]model.UserKey{sessionKey})
	})

	t.Run("usecase error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		googleUC.disconnectErr = errors.New("google unavailable")
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodPost, googleBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)).Equal(map[string]any{"error": "internal_error"})
	})

	t.Run("without session", func(t *testing.T) {
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, newFakeAuthUseCase(), googleUC)

		resp := serve(srv, httptest.NewRequest(http.MethodPost, googleBase+"/disconnect", nil))
		gt.Number(t, resp.StatusCode).Equal(http.StatusUnauthorized)
		gt.Array(t, googleUC.disconnects).Length(0)
	})

	t.Run("GET is not allowed", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		googleUC := newFakeGoogleWorkspaceUseCase()
		srv := newGoogleTestServer(t, authUC, googleUC)

		resp := serve(srv, withSession(httptest.NewRequest(http.MethodGet, googleBase+"/disconnect", nil), authUC))
		gt.Number(t, resp.StatusCode).Equal(http.StatusMethodNotAllowed)
		gt.Array(t, googleUC.disconnects).Length(0)
	})
}
