package http

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/model/auth"
)

// oauthStateCookieValue binds the OAuth state of a service connection to the
// session that started it, so a callback that arrives after another user
// signed in to the same browser is rejected instead of storing the first
// user's account.
func oauthStateCookieValue(state string, session *auth.Session) string {
	return state + "." + string(session.ID)
}

// verifyOAuthState checks the state against the cookie set when the
// connection started, and that the same session started it.
func verifyOAuthState(r *http.Request, cookieName, state string, session *auth.Session) error {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return goerr.Wrap(err, "oauth state cookie is missing", goerr.V("cookie", cookieName))
	}
	cookieState, cookieSessionID, found := strings.Cut(cookie.Value, ".")
	if !found {
		return goerr.New("oauth state cookie is malformed", goerr.V("cookie", cookieName))
	}
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return goerr.New("oauth state does not match", goerr.V("cookie", cookieName))
	}
	if subtle.ConstantTimeCompare([]byte(cookieSessionID), []byte(session.ID)) != 1 {
		return goerr.New("connection was started by another session", goerr.V("cookie", cookieName))
	}
	return nil
}
