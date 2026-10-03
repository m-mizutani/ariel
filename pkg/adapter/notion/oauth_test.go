package notion_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/adapter/notion"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

// recordedRequest is what fakeNotion saw of one request.
type recordedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Header  http.Header
	Body    map[string]any
	RawBody []byte
}

// fakeNotion answers each "METHOD path" with a fixed status and body and
// records every request.
type fakeNotion struct {
	server    *httptest.Server
	mu        sync.Mutex
	requests  []recordedRequest
	responses map[string]fakeResponse
}

type fakeResponse struct {
	status int
	body   string
}

func newFakeNotion(t *testing.T, responses map[string]fakeResponse) *fakeNotion {
	t.Helper()
	f := &fakeNotion{responses: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.Query(), Header: r.Header.Clone(), Body: body, RawBody: raw,
		})
		f.mu.Unlock()

		resp, ok := f.responses[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"unexpected request"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeNotion) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

const (
	testClientID     = "client-id"
	testClientSecret = "client-secret"
	testRedirectURI  = "https://robin.example.com/api/v1/integrations/notion/callback"
)

const tokenResponseBody = `{
	"access_token": "access-1",
	"token_type": "bearer",
	"refresh_token": "refresh-1",
	"bot_id": "b1c2d3e4-0000-4000-8000-000000000001",
	"workspace_id": "0F4A2B1C-3D4E-4F50-8A6B-7C8D9E0F1A2B",
	"workspace_name": "Example",
	"workspace_icon": null,
	"owner": {"type": "user", "user": {"object": "user", "id": "7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e", "name": "Alice Example"}},
	"duplicated_template_id": null,
	"request_id": "r1"
}`

func assertBasicAuth(t *testing.T, req recordedRequest) {
	t.Helper()
	r := &http.Request{Header: req.Header}
	user, pass, ok := r.BasicAuth()
	gt.Bool(t, ok).True()
	gt.String(t, user).Equal(testClientID)
	gt.String(t, pass).Equal(testClientSecret)
	gt.String(t, req.Header.Get("Notion-Version")).Equal("2026-03-11")
	gt.String(t, req.Header.Get("Content-Type")).Equal("application/json")
}

func TestOAuth_AuthorizeURL(t *testing.T) {
	o := notion.NewOAuth(testClientID, testClientSecret, "https://notion.example.com/", http.DefaultClient)

	u, err := url.Parse(o.AuthorizeURL("state-1", testRedirectURI))
	gt.NoError(t, err).Required()
	gt.String(t, u.Scheme+"://"+u.Host+u.Path).Equal("https://notion.example.com/v1/oauth/authorize")
	q := u.Query()
	gt.String(t, q.Get("client_id")).Equal(testClientID)
	gt.String(t, q.Get("redirect_uri")).Equal(testRedirectURI)
	gt.String(t, q.Get("response_type")).Equal("code")
	gt.String(t, q.Get("owner")).Equal("user")
	gt.String(t, q.Get("state")).Equal("state-1")
}

func TestOAuth_ExchangeCode(t *testing.T) {
	fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/token": {200, tokenResponseBody}})
	o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

	res, err := o.ExchangeCode(context.Background(), "code-1", testRedirectURI)
	gt.NoError(t, err).Required()
	gt.Value(t, res).Equal(&model.NotionOAuthResult{
		Tokens:        model.NotionTokens{AccessToken: "access-1", RefreshToken: "refresh-1"},
		BotID:         "b1c2d3e4-0000-4000-8000-000000000001",
		WorkspaceID:   "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b",
		WorkspaceName: "Example",
		OwnerType:     "user",
		OwnerUserID:   "7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e",
		OwnerName:     "Alice Example",
	})

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	assertBasicAuth(t, reqs[0])
	gt.Value(t, reqs[0].Body).Equal(map[string]any{
		"grant_type": "authorization_code", "code": "code-1", "redirect_uri": testRedirectURI,
	})
}

func TestOAuth_ExchangeCode_NullFields(t *testing.T) {
	fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/token": {200, `{
		"access_token": "access-1", "refresh_token": null, "bot_id": "b",
		"workspace_id": "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b", "workspace_name": null,
		"owner": {"type": "workspace", "workspace": true}
	}`}})
	o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

	res, err := o.ExchangeCode(context.Background(), "code-1", testRedirectURI)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Tokens.RefreshToken).Equal(model.NotionRefreshToken(""))
	gt.String(t, res.WorkspaceName).Equal("")
	gt.String(t, res.OwnerType).Equal("workspace")
	gt.Value(t, res.OwnerUserID).Equal(model.NotionUserID(""))
}

func TestOAuth_RefreshToken(t *testing.T) {
	fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/token": {200, tokenResponseBody}})
	o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

	res, err := o.RefreshToken(context.Background(), "refresh-0")
	gt.NoError(t, err).Required()
	gt.Value(t, res.Tokens).Equal(model.NotionTokens{AccessToken: "access-1", RefreshToken: "refresh-1"})

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	assertBasicAuth(t, reqs[0])
	gt.Value(t, reqs[0].Body).Equal(map[string]any{"grant_type": "refresh_token", "refresh_token": "refresh-0"})
}

func TestOAuth_Revoke(t *testing.T) {
	fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/oauth/revoke": {200, `{"request_id":"r1"}`}})
	o := notion.NewOAuth(testClientID, testClientSecret, fake.server.URL, http.DefaultClient)

	gt.NoError(t, o.Revoke(context.Background(), "access-1"))

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	assertBasicAuth(t, reqs[0])
	gt.Value(t, reqs[0].Body).Equal(map[string]any{"token": "access-1"})
}
