package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
)

func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestAuthLogin(t *testing.T) {
	for _, tc := range []struct {
		baseURL    string
		wantSecure bool
	}{
		{baseURL: "https://ariel.example.com", wantSecure: true},
		{baseURL: "http://localhost:8080", wantSecure: false},
	} {
		t.Run(tc.baseURL, func(t *testing.T) {
			authUC := newFakeAuthUseCase()
			srv := newTestServer(t, tc.baseURL, authUC, &fakeSlackEventUseCase{})

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/login", nil))
			resp := w.Result()

			gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
			location, err := url.Parse(resp.Header.Get("Location"))
			gt.NoError(t, err).Required()
			gt.Bool(t, strings.HasPrefix(location.String(), "https://slack.com/oauth/v2/authorize?")).True()

			cookie := findCookie(resp, "ariel_oauth_state")
			gt.Value(t, cookie).NotNil().Required()
			gt.String(t, cookie.Value).NotEqual("")
			gt.String(t, location.Query().Get("state")).Equal(cookie.Value)
			gt.Array(t, authUC.states).Length(1).Required()
			gt.String(t, authUC.states[0]).Equal(cookie.Value)
			gt.Bool(t, cookie.HttpOnly).True()
			gt.Value(t, cookie.SameSite).Equal(http.SameSiteLaxMode)
			gt.Number(t, cookie.MaxAge).Equal(600)
			gt.Value(t, cookie.Secure).Equal(tc.wantSecure)
		})
	}
}

func callbackRequest(query, stateCookie string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/auth/callback?"+query, nil)
	if stateCookie != "" {
		r.AddCookie(&http.Cookie{Name: "ariel_oauth_state", Value: stateCookie})
	}
	return r
}

func TestAuthCallback_Success(t *testing.T) {
	authUC := newFakeAuthUseCase()
	srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, callbackRequest("code=auth-code&state=s1", "s1"))
	resp := w.Result()

	gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
	gt.String(t, resp.Header.Get("Location")).Equal("/")
	gt.Value(t, authUC.callbackCodes).Equal([]string{"auth-code"})

	id := findCookie(resp, "ariel_session_id")
	gt.Value(t, id).NotNil().Required()
	gt.String(t, id.Value).Equal(string(authUC.session.ID))
	gt.Bool(t, id.Expires.Equal(authUC.session.ExpiresAt)).True()
	gt.Bool(t, id.HttpOnly).True()
	gt.Bool(t, id.Secure).True()
	gt.String(t, id.Path).Equal("/")

	secret := findCookie(resp, "ariel_session_secret")
	gt.Value(t, secret).NotNil().Required()
	gt.String(t, secret.Value).Equal(string(authUC.secret))
	gt.Bool(t, secret.Expires.Equal(authUC.session.ExpiresAt)).True()

	state := findCookie(resp, "ariel_oauth_state")
	gt.Value(t, state).NotNil().Required()
	gt.Bool(t, state.MaxAge < 0).True()
}

func TestAuthCallback_Failures(t *testing.T) {
	cases := map[string]struct {
		query       string
		stateCookie string
		callbackErr error
		wantError   string
		wantCalled  bool
	}{
		"no state cookie":   {query: "code=c&state=s1", wantError: "login_failed"},
		"state mismatch":    {query: "code=c&state=s1", stateCookie: "s2", wantError: "login_failed"},
		"empty state":       {query: "code=c&state=", stateCookie: "s1", wantError: "login_failed"},
		"no code":           {query: "state=s1", stateCookie: "s1", wantError: "login_failed"},
		"usecase error":     {query: "code=c&state=s1", stateCookie: "s1", callbackErr: errors.New("boom"), wantError: "login_failed", wantCalled: true},
		"user declined":     {query: "error=access_denied&state=s1", stateCookie: "s1", wantError: "access_denied"},
		"other slack error": {query: "error=invalid_scope&state=s1", stateCookie: "s1", wantError: "login_failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			authUC := newFakeAuthUseCase()
			authUC.callbackErr = tc.callbackErr
			srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, callbackRequest(tc.query, tc.stateCookie))
			resp := w.Result()

			gt.Number(t, resp.StatusCode).Equal(http.StatusFound)
			gt.String(t, resp.Header.Get("Location")).Equal("/login?error=" + tc.wantError)
			gt.Value(t, findCookie(resp, "ariel_session_id")).Nil()
			gt.Value(t, findCookie(resp, "ariel_session_secret")).Nil()
			state := findCookie(resp, "ariel_oauth_state")
			gt.Value(t, state).NotNil().Required()
			gt.Bool(t, state.MaxAge < 0).True()
			gt.Value(t, len(authUC.callbackCodes) > 0).Equal(tc.wantCalled)
		})
	}
}

func withSession(r *http.Request, authUC *fakeAuthUseCase) *http.Request {
	r.AddCookie(&http.Cookie{Name: "ariel_session_id", Value: string(authUC.session.ID)})
	r.AddCookie(&http.Cookie{Name: "ariel_session_secret", Value: string(authUC.secret)})
	return r
}

func TestAuthMe(t *testing.T) {
	t.Run("authenticated", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, withSession(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil), authUC))

		gt.Number(t, w.Code).Equal(http.StatusOK)
		body := decodeJSON(t, w.Body)
		gt.Value(t, body["team_id"]).Equal("T0123ABCD")
		gt.Value(t, body["user_id"]).Equal("U0123ABCD")
		gt.Value(t, body["name"]).Equal("Alice Example")
		gt.Value(t, body["slack_connected"]).Equal(true)
		gt.Array(t, authUC.meKeys).Length(1).Required()
		gt.Value(t, authUC.meKeys[0]).Equal(authUC.session.Key())
	})

	t.Run("no cookie", func(t *testing.T) {
		srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), &fakeSlackEventUseCase{})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))

		gt.Number(t, w.Code).Equal(http.StatusUnauthorized)
		gt.Value(t, decodeJSON(t, w.Body)["error"]).Equal("unauthenticated")
	})

	t.Run("wrong secret", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})
		r := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		r.AddCookie(&http.Cookie{Name: "ariel_session_id", Value: string(authUC.session.ID)})
		r.AddCookie(&http.Cookie{Name: "ariel_session_secret", Value: "wrong"})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		gt.Number(t, w.Code).Equal(http.StatusUnauthorized)
		gt.Value(t, decodeJSON(t, w.Body)["error"]).Equal("unauthenticated")
	})

	t.Run("authentication backend error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		authUC.authErr = errors.New("firestore unavailable")
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, withSession(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil), authUC))
		gt.Number(t, w.Code).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, w.Body)["error"]).Equal("internal_error")
	})

	t.Run("me error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		authUC.meErr = errors.New("firestore unavailable")
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, withSession(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil), authUC))
		gt.Number(t, w.Code).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, w.Body)["error"]).Equal("internal_error")
	})
}

func assertSessionCookiesCleared(t *testing.T, resp *http.Response) {
	t.Helper()
	for _, name := range []string{"ariel_session_id", "ariel_session_secret"} {
		c := findCookie(resp, name)
		gt.Value(t, c).NotNil().Required()
		gt.Bool(t, c.MaxAge < 0).True()
	}
}

func TestAuthLogout(t *testing.T) {
	t.Run("with session", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, withSession(httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil), authUC))
		resp := w.Result()

		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Value(t, decodeJSON(t, resp.Body)["success"]).Equal(true)
		gt.Array(t, authUC.logouts).Length(1).Required()
		gt.Value(t, authUC.logouts[0]).Equal(authUC.session.ID)
		gt.Value(t, authUC.logoutSecrets[0]).Equal(authUC.secret)
		assertSessionCookiesCleared(t, resp)
	})

	t.Run("without session", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
		resp := w.Result()

		gt.Number(t, resp.StatusCode).Equal(http.StatusOK)
		gt.Array(t, authUC.logouts).Length(0)
		assertSessionCookiesCleared(t, resp)
	})

	t.Run("logout error", func(t *testing.T) {
		authUC := newFakeAuthUseCase()
		authUC.logoutErr = errors.New("firestore unavailable")
		srv := newTestServer(t, "https://ariel.example.com", authUC, &fakeSlackEventUseCase{})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, withSession(httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil), authUC))
		resp := w.Result()

		gt.Number(t, resp.StatusCode).Equal(http.StatusInternalServerError)
		gt.Value(t, decodeJSON(t, resp.Body)["error"]).Equal("internal_error")
		assertSessionCookiesCleared(t, resp)
	})
}
