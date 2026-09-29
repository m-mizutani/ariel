package notion_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/notion"
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
)

// notionError is an error body of Notion. Its message stands for text that
// may quote user content, so it must not reach the returned error.
func notionError(status int, code string) string {
	return `{"object":"error","status":` + strconv.Itoa(status) + `,"code":"` + code + `","message":"secret page text"}`
}

func TestErrors_API(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"unauthorized":        {401, notionError(401, "unauthorized"), interfaces.ErrNotionTokenInvalid},
		"restricted":          {403, notionError(403, "restricted_resource"), interfaces.ErrNotionForbidden},
		"not found":           {404, notionError(404, "object_not_found"), interfaces.ErrNotionNotFound},
		"rate limited":        {429, notionError(429, "rate_limited"), interfaces.ErrNotionRateLimited},
		"validation error":    {400, notionError(400, "validation_error"), nil},
		"internal error":      {500, notionError(500, "internal_server_error"), nil},
		"body is not json":    {502, "<html>bad gateway</html>", nil},
		"success is not json": {200, "not json", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/pages/" + string(pageID): {tc.status, tc.body}})
			c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

			_, err := c.GetPage(context.Background(), pageID)
			gt.Value(t, err).NotNil().Required()
			for _, sentinel := range []error{
				interfaces.ErrNotionTokenInvalid, interfaces.ErrNotionForbidden,
				interfaces.ErrNotionNotFound, interfaces.ErrNotionRateLimited,
			} {
				gt.Value(t, errors.Is(err, sentinel)).Equal(sentinel == tc.want)
			}
			gt.String(t, err.Error()).NotContains("access-1")
			gt.String(t, err.Error()).NotContains("secret page text")

			var ge *goerr.Error
			gt.Bool(t, errors.As(err, &ge)).True().Required()
			gt.Value(t, ge.Values()["notion_status"]).Equal(tc.status)
		})
	}
}

func TestErrors_OAuth(t *testing.T) {
	t.Run("invalid grant on refresh", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/token": {400, notionError(400, "invalid_grant")}})
		o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

		_, err := o.RefreshToken(context.Background(), "refresh-secret")
		gt.Error(t, err).Is(interfaces.ErrNotionTokenInvalid)
		gt.String(t, err.Error()).NotContains("refresh-secret")

		var ge *goerr.Error
		gt.Bool(t, errors.As(err, &ge)).True().Required()
		gt.Value(t, ge.Values()["notion_code"]).Equal("invalid_grant")
	})

	// A rejected client is a configuration error of the server: it must not
	// make a user's stored token look invalid, which would mark the
	// connection for reconnection or delete it on disconnection.
	t.Run("invalid client is not a token error", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{
			"POST /v1/oauth/token":  {401, notionError(401, "invalid_client")},
			"POST /v1/oauth/revoke": {401, notionError(401, "invalid_client")},
		})
		o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

		_, err := o.ExchangeCode(context.Background(), "code-secret", testRedirectURI)
		gt.Value(t, err).NotNil()
		gt.String(t, err.Error()).NotContains("code-secret")
		gt.Bool(t, errors.Is(err, interfaces.ErrNotionTokenInvalid)).False()

		_, err = o.RefreshToken(context.Background(), "refresh-secret")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, interfaces.ErrNotionTokenInvalid)).False()

		err = o.Revoke(context.Background(), "access-1")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, interfaces.ErrNotionTokenInvalid)).False()
	})

	t.Run("revoke of an invalid token", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/revoke": {400, notionError(400, "invalid_grant")}})
		o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

		gt.Error(t, o.Revoke(context.Background(), "access-1")).Is(interfaces.ErrNotionTokenInvalid)
	})

	t.Run("revoke fails on server error", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/revoke": {503, notionError(503, "service_unavailable")}})
		o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

		err := o.Revoke(context.Background(), "access-1")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, interfaces.ErrNotionTokenInvalid)).False()
	})
}
